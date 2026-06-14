package server

import (
	"context"
	"fmt"
	"net/http"
	"sync"
	"time"

	"clever-connect/internal/logger"
	"clever-connect/internal/models"

	"github.com/gorilla/websocket"
)

// ──────────────────────────────────────────────────────────────────────────────
// Combiner — Server-Side Bonding Engine
// ──────────────────────────────────────────────────────────────────────────────

var (
	combinerOnce     sync.Once
	globalCombiner   *Combiner
)

// Combiner is the server-side bonding engine singleton.
// It accepts artery WebSocket connections, groups them by origin,
// and manages per-session dedup/reorder/relay.
type Combiner struct {
	mu sync.RWMutex

	state    CombinerState
	config   *models.BondingEngineConfig
	server   *http.Server
	sessions map[string]*BondingSession // OriginID → session

	ctx    context.Context
	cancel context.CancelFunc
}

// CombinerState represents the lifecycle state of the combiner.
type CombinerState string

const (
	CombinerStopped  CombinerState = "stopped"
	CombinerStarting CombinerState = "starting"
	CombinerRunning  CombinerState = "running"
	CombinerStopping CombinerState = "stopping"
)

// GetCombiner returns the singleton Combiner instance.
func GetCombiner() *Combiner {
	combinerOnce.Do(func() {
		globalCombiner = &Combiner{
			state:    CombinerStopped,
			sessions: make(map[string]*BondingSession),
		}
	})
	return globalCombiner
}

// State returns the current combiner state.
func (c *Combiner) State() CombinerState {
	c.mu.RLock()
	defer c.mu.RUnlock()
	return c.state
}

// StartCombiner starts the server-side bonding combiner.
// It listens on port 3002 (behind nginx /bond route).
func StartCombiner(cfg *models.BondingEngineConfig) error {
	c := GetCombiner()

	c.mu.Lock()
	if c.state == CombinerRunning || c.state == CombinerStarting {
		c.mu.Unlock()
		return fmt.Errorf("combiner is already running")
	}
	c.state = CombinerStarting
	c.config = cfg
	c.mu.Unlock()

	logger.Info("Combiner", "Starting server-side bonding combiner", "port", 3002)

	ctx, cancel := context.WithCancel(context.Background())

	mux := http.NewServeMux()
	mux.HandleFunc("/bond", c.handleBondWS)
	mux.HandleFunc("/bond/health", c.handleHealth)

	server := &http.Server{
		Addr:    ":3002",
		Handler: mux,
	}

	c.mu.Lock()
	c.ctx = ctx
	c.cancel = cancel
	c.server = server
	c.state = CombinerRunning
	c.mu.Unlock()

	// Start HTTP server in background
	go func() {
		if err := server.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			logger.Error("Combiner", "HTTP server error", "error", err)
		}
	}()

	logger.Info("Combiner", "Combiner started successfully", "port", 3002)
	return nil
}

// StopCombiner gracefully shuts down the combiner.
func StopCombiner() error {
	c := GetCombiner()

	c.mu.Lock()
	if c.state != CombinerRunning {
		c.mu.Unlock()
		return nil
	}
	c.state = CombinerStopping
	c.mu.Unlock()

	logger.Info("Combiner", "Stopping combiner")

	// Cancel all sessions
	c.mu.RLock()
	for _, s := range c.sessions {
		s.Close()
	}
	c.mu.RUnlock()

	// Shutdown HTTP server
	c.mu.Lock()
	if c.cancel != nil {
		c.cancel()
	}
	if c.server != nil {
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		_ = c.server.Shutdown(ctx)
	}
	c.sessions = make(map[string]*BondingSession)
	c.state = CombinerStopped
	c.mu.Unlock()

	logger.Info("Combiner", "Combiner stopped")
	return nil
}

// WebSocket upgrader for artery connections
var wsUpgrader = websocket.Upgrader{
	ReadBufferSize:  65536, // 64KB — enough for max frame + header
	WriteBufferSize: 65536,
	CheckOrigin:     func(r *http.Request) bool { return true },
}

// handleBondWS handles a new artery WebSocket connection.
// Each artery sends an X-Bonding-Origin header (or query param) to identify
// which bonding session it belongs to.
func (c *Combiner) handleBondWS(w http.ResponseWriter, r *http.Request) {
	// Extract origin ID from header or query param
	originID := r.Header.Get("X-Bonding-Origin")
	if originID == "" {
		originID = r.URL.Query().Get("origin")
	}
	if originID == "" {
		http.Error(w, "missing X-Bonding-Origin header or origin query param", http.StatusBadRequest)
		return
	}

	// Upgrade to WebSocket
	conn, err := wsUpgrader.Upgrade(w, r, nil)
	if err != nil {
		logger.Error("Combiner", "WebSocket upgrade failed", "error", err)
		return
	}

	// Get or create bonding session for this origin
	session := c.getOrCreateSession(originID)

	// Register artery
	arteryID := nextArteryID()
	session.AddArtery(arteryID, conn)

	// Start keepalive
	go keepaliveLoop(session.ctx, conn, defaultKeepaliveInterval, arteryID)

	// Read frames from this artery (blocks until disconnect)
	session.ReadArteryLoop(conn, arteryID)

	// Cleanup on disconnect
	session.RemoveArtery(arteryID)
	_ = conn.Close()

	// If no arteries left, schedule session cleanup after grace period
	if session.ArteryCount() == 0 {
		go c.scheduleSessionCleanup(originID, 30*time.Second)
	}
}

// handleHealth is a simple health-check endpoint.
func (c *Combiner) handleHealth(w http.ResponseWriter, r *http.Request) {
	c.mu.RLock()
	state := c.state
	sessionCount := len(c.sessions)
	c.mu.RUnlock()

	w.Header().Set("Content-Type", "application/json")
	fmt.Fprintf(w, `{"state":"%s","sessions":%d}`, state, sessionCount)
}

// getOrCreateSession retrieves an existing session or creates a new one.
func (c *Combiner) getOrCreateSession(originID string) *BondingSession {
	c.mu.Lock()
	defer c.mu.Unlock()

	if s, ok := c.sessions[originID]; ok {
		return s
	}

	reorderCap := 256
	if c.config != nil && c.config.FrameSize > 0 {
		// Derive reorder cap from frame size
		reorderCap = 256
	}

	s := NewBondingSession(originID, reorderCap)
	c.sessions[originID] = s

	logger.Info("Combiner", "New bonding session created", "origin", originID)
	return s
}

// scheduleSessionCleanup removes a session after a grace period if no arteries reconnected.
func (c *Combiner) scheduleSessionCleanup(originID string, grace time.Duration) {
	time.Sleep(grace)

	c.mu.Lock()
	defer c.mu.Unlock()

	if s, ok := c.sessions[originID]; ok {
		if s.ArteryCount() == 0 {
			s.Close()
			delete(c.sessions, originID)
			logger.Info("Combiner", "Session cleaned up after grace period", "origin", originID)
		}
	}
}

// GetStatus returns combiner status for telemetry.
func (c *Combiner) GetStatus() CombinerStatus {
	c.mu.RLock()
	defer c.mu.RUnlock()

	status := CombinerStatus{
		State:    string(c.state),
		Sessions: make([]SessionStats, 0, len(c.sessions)),
	}

	for _, s := range c.sessions {
		status.Sessions = append(status.Sessions, s.Stats())
	}

	return status
}

// CombinerStatus is the combiner telemetry snapshot.
type CombinerStatus struct {
	State    string         `json:"state"`
	Sessions []SessionStats `json:"sessions"`
}
