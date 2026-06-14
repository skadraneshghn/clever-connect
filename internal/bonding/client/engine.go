// Package client implements the client-side bonding engine (Mode B) for the DMB Engine.
// It accepts user traffic via SOCKS5/HTTP, frames it using the DMB wire protocol,
// and dispatches framed data across multiple xray artery inbounds to the server combiner.
package client

import (
	"context"
	"errors"
	"fmt"
	"sync"
	"time"

	"clever-connect/internal/bonding/control"
	"clever-connect/internal/bonding/frame"
	"clever-connect/internal/bonding/session"
	"clever-connect/internal/db"
	"clever-connect/internal/db/pebble"
	"clever-connect/internal/logger"
	"clever-connect/internal/models"
	"clever-connect/internal/v2ray/core"
	"clever-connect/internal/v2ray/sysproxy"
)

// ErrNoActiveArteries is returned when no artery connections are available.
var ErrNoActiveArteries = errors.New("bonding: no active arteries available")

// BondingEngineState represents the lifecycle state of the bonding engine.
type BondingEngineState string

const (
	BondingStateStopped  BondingEngineState = "stopped"
	BondingStateStarting BondingEngineState = "starting"
	BondingStateRunning  BondingEngineState = "running"
	BondingStateStopping BondingEngineState = "stopping"
)

// BondingEngine is the Mode B client-side bonding engine singleton.
type BondingEngine struct {
	mu sync.RWMutex

	state      BondingEngineState
	config     *models.BondingEngineConfig
	cancelFunc context.CancelFunc

	// Core components
	session    *session.Session
	dispatcher *Dispatcher
	frontend   *Frontend
	arteries   []*ArteryConn

	// Intelligence
	stateMachine   *control.StateMachine
	metrics        map[string]*control.PathMetrics
	adaptiveCtrl   *control.AdaptiveController

	// Pool of candidate nodes
	candidatePool []models.V2RayClientConfig
	activeNodes   []models.V2RayClientConfig

	// Telemetry
	TelemetryChan chan BondingStatus
}

// BondingStatus is the telemetry snapshot for the bonding engine.
type BondingStatus struct {
	State       string           `json:"state"`
	Mode        string           `json:"mode"`
	ActiveCount int              `json:"active_count"`
	TotalPool   int              `json:"total_pool"`
	Arteries    []ArteryStatus   `json:"arteries"`
	Session     session.SessionStats `json:"session"`
	LastEvalAt  time.Time        `json:"last_eval_at"`
}

// ArteryStatus is per-artery telemetry.
type ArteryStatus struct {
	Tag       string  `json:"tag"`
	Alive     bool    `json:"alive"`
	SrttMs    float64 `json:"srtt_ms"`
	LossPct   float64 `json:"loss_pct"`
	WinRate   float64 `json:"win_rate"`
	LocalPort int     `json:"local_port"`
}

var (
	bondingOnce   sync.Once
	globalBonding *BondingEngine
)

// GetBondingEngine returns the singleton bonding engine instance.
func GetBondingEngine() *BondingEngine {
	bondingOnce.Do(func() {
		globalBonding = &BondingEngine{
			state:         BondingStateStopped,
			metrics:       make(map[string]*control.PathMetrics),
			TelemetryChan: make(chan BondingStatus, 50),
		}
	})
	return globalBonding
}

// State returns the current engine state.
func (e *BondingEngine) State() BondingEngineState {
	e.mu.RLock()
	defer e.mu.RUnlock()
	return e.state
}

// StartEngine starts the bonding engine with the given configuration.
func (e *BondingEngine) StartEngine(cfg *models.BondingEngineConfig) error {
	e.mu.Lock()
	if e.state == BondingStateRunning || e.state == BondingStateStarting {
		e.mu.Unlock()
		return fmt.Errorf("bonding engine is already running")
	}
	e.state = BondingStateStarting
	e.config = cfg
	e.mu.Unlock()

	logger.Info("Bonding", "Starting Mode B bonding engine",
		"max_arteries", cfg.MaxArteries,
		"combiner_url", cfg.CombinerURL,
		"socks_port", cfg.SocksPort,
		"http_port", cfg.HTTPPort,
		"striping_mode", cfg.StripingMode,
	)

	if cfg.CombinerURL == "" {
		e.mu.Lock()
		e.state = BondingStateStopped
		e.mu.Unlock()
		return fmt.Errorf("combiner URL is required for Mode B bonding")
	}

	// Initialize state machine
	thresholds := control.Thresholds{
		DemoteRTTx:     cfg.DemoteRTTx,
		PromoteRTTx:    cfg.PromoteRTTx,
		LossDemotePct:  cfg.LossDemotePct,
		CooldownDur:    time.Duration(cfg.CooldownSec) * time.Second,
		EvalWindow:     time.Duration(cfg.EvalWindowMs) * time.Millisecond,
		ConfirmWindows: 3,
		ErrorBudget:    cfg.ErrorBudget,
	}

	e.mu.Lock()
	e.stateMachine = control.NewStateMachine(thresholds)
	e.metrics = make(map[string]*control.PathMetrics)
	e.mu.Unlock()

	// Build candidate pool from PebbleDB
	if err := e.refreshCandidatePool(); err != nil {
		e.mu.Lock()
		if e.state == BondingStateStarting {
			e.state = BondingStateStopped
		}
		e.mu.Unlock()
		return fmt.Errorf("failed to build candidate pool: %w", err)
	}

	// Select top-N nodes
	if err := e.buildActivePool(); err != nil {
		e.mu.Lock()
		if e.state == BondingStateStarting {
			e.state = BondingStateStopped
		}
		e.mu.Unlock()
		return fmt.Errorf("failed to build active pool: %w", err)
	}

	// Compile and start xray core with dokodemo-door arteries
	if err := e.compileAndStartCore(); err != nil {
		e.mu.Lock()
		if e.state == BondingStateStarting {
			e.state = BondingStateStopped
		}
		e.mu.Unlock()
		return fmt.Errorf("failed to start core: %w", err)
	}

	// Initialize session + dispatcher
	e.mu.Lock()
	e.session = session.NewSession(256)

	schedMode := control.ParseScheduleMode(cfg.StripingMode)
	e.arteries = make([]*ArteryConn, 0, len(e.activeNodes))
	basePort := 21001

	for i := range e.activeNodes {
		tag := fmt.Sprintf("artery-%d", i)
		ac := NewArteryConn(tag, basePort+i)
		e.arteries = append(e.arteries, ac)
	}

	e.dispatcher = NewDispatcher(schedMode, e.arteries, e.metrics)
	e.frontend = NewFrontend(cfg.SocksPort, cfg.HTTPPort, cfg.FrameSize, e.dispatcher, e.session)

	// Initialize adaptive controller
	pathSlice := make([]*control.PathMetrics, 0, len(e.metrics))
	for _, m := range e.metrics {
		pathSlice = append(pathSlice, m)
	}
	e.adaptiveCtrl = control.NewAdaptiveController(
		e.stateMachine,
		control.NewScheduler(schedMode, pathSlice),
		e.metrics,
		thresholds,
	)
	e.mu.Unlock()

	// Start context
	ctx, cancel := context.WithCancel(context.Background())
	e.mu.Lock()
	e.cancelFunc = cancel
	e.mu.Unlock()

	// Connect arteries (with backoff)
	for _, ac := range e.arteries {
		go func(a *ArteryConn) {
			if err := a.ConnectWithBackoff(ctx); err != nil {
				logger.Error("Bonding", "Artery failed to connect",
					"tag", a.Tag(), "error", err)
				return
			}
			// Start reading frames from this artery
			metrics := e.metrics[a.Tag()]
			a.ReadFrameLoop(ctx, e.session, func(rtt float64) {
				if metrics != nil {
					metrics.UpdateRTT(rtt)
					metrics.RecordPongReceived()
				}
			})
		}(ac)
	}

	// Start frontend listeners
	if err := e.frontend.Start(ctx); err != nil {
		cancel()
		e.mu.Lock()
		if e.state == BondingStateStarting {
			e.state = BondingStateStopped
		}
		e.mu.Unlock()
		return fmt.Errorf("failed to start frontend: %w", err)
	}

	e.mu.Lock()
	if e.state != BondingStateStarting {
		// StopEngine was called during startup!
		if e.frontend != nil {
			e.frontend.Stop()
		}
		for _, ac := range e.arteries {
			ac.Close()
		}
		if e.session != nil {
			e.session.Close()
		}
		_ = core.StopCore()
		e.state = BondingStateStopped
		e.arteries = nil
		e.dispatcher = nil
		e.frontend = nil
		e.session = nil
		e.stateMachine = nil
		e.metrics = make(map[string]*control.PathMetrics)
		e.mu.Unlock()
		return fmt.Errorf("bonding engine was stopped during startup")
	}

	e.state = BondingStateRunning
	e.mu.Unlock()

	// Reset traffic counters
	core.ResetClientTraffic()

	// Toggle OS system proxy if enabled in settings
	sysProxyEnabled := false
	if db.DB != nil {
		var setting models.V2RayClientSetting
		if err := db.DB.Where("key = ?", "sys_proxy_enabled").First(&setting).Error; err == nil {
			sysProxyEnabled = setting.Value == "true"
		}
	}
	if sysProxyEnabled {
		logger.Info("ClientProxy", "Setting OS system proxy for bonding client", "socksPort", cfg.SocksPort, "httpPort", cfg.HTTPPort)
		_ = sysproxy.SetSystemProxy(cfg.SocksPort, cfg.HTTPPort)
	}

	// Start background loops
	go e.controllerLoop(ctx)
	go e.telemetryLoop(ctx)
	go e.pingLoop(ctx)

	logger.Info("Bonding", "Bonding engine started successfully",
		"active_arteries", len(e.arteries),
		"candidate_pool", len(e.candidatePool),
	)

	return nil
}

// StopEngine gracefully stops the bonding engine.
func (e *BondingEngine) StopEngine() error {
	e.mu.Lock()
	if e.state != BondingStateRunning && e.state != BondingStateStarting {
		e.mu.Unlock()
		return nil
	}
	e.state = BondingStateStopping
	if e.cancelFunc != nil {
		e.cancelFunc()
		e.cancelFunc = nil
	}
	e.mu.Unlock()

	logger.Info("Bonding", "Stopping bonding engine")

	// Stop frontend
	e.mu.RLock()
	if e.frontend != nil {
		e.frontend.Stop()
	}
	e.mu.RUnlock()

	// Close arteries
	e.mu.RLock()
	for _, ac := range e.arteries {
		ac.Close()
	}
	e.mu.RUnlock()

	// Close session
	e.mu.RLock()
	if e.session != nil {
		e.session.Close()
	}
	e.mu.RUnlock()

	// Stop xray core
	_ = core.StopCore()

	// Always clear OS system proxy on stop
	logger.Info("ClientProxy", "Clearing OS system proxy for bonding client")
	_ = sysproxy.ClearSystemProxy()

	e.mu.Lock()
	e.state = BondingStateStopped
	e.arteries = nil
	e.dispatcher = nil
	e.frontend = nil
	e.session = nil
	e.stateMachine = nil
	e.metrics = make(map[string]*control.PathMetrics)
	e.mu.Unlock()

	logger.Info("Bonding", "Bonding engine stopped")
	return nil
}

// GetStatus returns a snapshot of the current engine status.
func (e *BondingEngine) GetStatus() BondingStatus {
	e.mu.RLock()
	defer e.mu.RUnlock()

	status := BondingStatus{
		State:       string(e.state),
		Mode:        "bonding",
		ActiveCount: len(e.arteries),
		TotalPool:   len(e.candidatePool),
		LastEvalAt:  time.Now(),
	}

	if e.config != nil {
		status.Mode = e.config.Mode
	}

	if e.session != nil {
		status.Session = e.session.Stats()
	}

	for _, ac := range e.arteries {
		as := ArteryStatus{
			Tag:       ac.Tag(),
			Alive:     ac.IsAlive(),
			LocalPort: ac.LocalPort(),
		}
		if m, ok := e.metrics[ac.Tag()]; ok {
			snap := m.Snapshot()
			as.SrttMs = snap.SRTT
			as.LossPct = snap.LossPct
			as.WinRate = snap.WinRate
		}
		status.Arteries = append(status.Arteries, as)
	}

	return status
}

// refreshCandidatePool loads the best nodes from PebbleDB.
func (e *BondingEngine) refreshCandidatePool() error {
	configs, total := pebble.ListClientConfigs(pebble.ConfigFilter{
		SortBy:     "latency",
		PingStatus: "pass",
	}, 0, 0)

	if total == 0 {
		return fmt.Errorf("no healthy nodes available in PebbleDB")
	}

	e.mu.Lock()
	e.candidatePool = configs
	e.mu.Unlock()

	return nil
}

// buildActivePool selects the top-N nodes for the active pool.
func (e *BondingEngine) buildActivePool() error {
	e.mu.Lock()
	defer e.mu.Unlock()

	maxArteries := e.config.MaxArteries
	if maxArteries <= 0 {
		maxArteries = 5
	}

	count := maxArteries
	if count > len(e.candidatePool) {
		count = len(e.candidatePool)
	}

	if count < e.config.MinArteries && len(e.candidatePool) >= e.config.MinArteries {
		count = e.config.MinArteries
	}

	if count == 0 {
		return fmt.Errorf("no candidate nodes available")
	}

	e.activeNodes = make([]models.V2RayClientConfig, count)
	copy(e.activeNodes, e.candidatePool[:count])
	e.candidatePool = e.candidatePool[count:]

	// Initialize metrics and state machine
	for i := range e.activeNodes {
		tag := fmt.Sprintf("artery-%d", i)

		metrics := control.NewPathMetrics(tag)
		metrics.UpdateRTT(float64(e.activeNodes[i].LatencyMs))
		e.metrics[tag] = metrics

		pathEntry := control.NewPathEntry(tag, metrics)
		e.stateMachine.AddPath(pathEntry)

		// Create DB record
		dbArtery := &models.BondingArtery{
			NodeConfigID: e.activeNodes[i].ID,
			Tag:          tag,
			LocalPort:    21001 + i,
			State:        "active",
			SrttMs:       float64(e.activeNodes[i].LatencyMs),
		}
		if db.DB != nil {
			db.DB.Create(dbArtery)
		}

		logger.Info("Bonding", "Artery selected for bonding pool",
			"tag", tag,
			"name", e.activeNodes[i].Name,
			"address", e.activeNodes[i].Address,
			"latency_ms", e.activeNodes[i].LatencyMs,
		)
	}

	return nil
}

// compileAndStartCore compiles the multi-inbound xray config and starts the core.
func (e *BondingEngine) compileAndStartCore() error {
	e.mu.RLock()
	state := e.state
	nodes := make([]models.V2RayClientConfig, len(e.activeNodes))
	copy(nodes, e.activeNodes)
	cfg := e.config
	e.mu.RUnlock()

	if state != BondingStateRunning && state != BondingStateStarting {
		return fmt.Errorf("bonding engine is not running")
	}

	configPath, err := CompileBondingClientConfig(
		nodes,
		cfg.CombinerURL,
		21001,
		cfg.SocksPort,
		cfg.HTTPPort,
	)
	if err != nil {
		return fmt.Errorf("failed to compile bonding config: %w", err)
	}

	return core.StartCore(configPath)
}

// controllerLoop runs the adaptive controller evaluation cycle.
func (e *BondingEngine) controllerLoop(ctx context.Context) {
	e.mu.RLock()
	evalWindow := time.Duration(e.config.EvalWindowMs) * time.Millisecond
	e.mu.RUnlock()

	if evalWindow <= 0 {
		evalWindow = 5 * time.Second
	}

	ticker := time.NewTicker(evalWindow)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			e.runEvaluation(ctx)
		}
	}
}

// runEvaluation delegates to the AdaptiveController for unified evaluation.
func (e *BondingEngine) runEvaluation(ctx context.Context) {
	e.mu.RLock()
	ctrl := e.adaptiveCtrl
	if ctrl == nil || len(e.arteries) == 0 {
		e.mu.RUnlock()
		return
	}
	e.mu.RUnlock()

	// Run unified evaluation (state machine + cwnd + mode promotion)
	ctrlEvents, transitions := ctrl.Evaluate()

	// Log controller events (mode changes, cwnd adjustments)
	for _, event := range ctrlEvents {
		logger.Debug("Bonding", "Controller event",
			"type", event.Type,
			"path", event.PathID,
			"detail", event.Detail,
			"old", event.OldValue,
			"new", event.NewValue,
		)
	}

	// Process path transitions
	for _, event := range transitions {
		logger.Info("Bonding", "Path state transition",
			"path", event.PathID,
			"old_state", event.OldState.String(),
			"new_state", event.NewState.String(),
			"reason", event.Reason,
		)

		if db.DB != nil {
			db.DB.Model(&models.BondingArtery{}).
				Where("tag = ?", event.PathID).
				Update("state", event.NewState.String())
		}

		if event.NewState == control.PathDead {
			go e.replaceDeadArtery(ctx, event.PathID)
		}
	}
}

// pingLoop periodically sends PING frames through each artery for RTT measurement.
func (e *BondingEngine) pingLoop(ctx context.Context) {
	ticker := time.NewTicker(3 * time.Second)
	defer ticker.Stop()

	var pingSeq uint64

	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			e.mu.RLock()
			arteries := e.arteries
			metrics := e.metrics
			e.mu.RUnlock()

			for _, ac := range arteries {
				if !ac.IsAlive() {
					continue
				}

				nowNs := uint64(time.Now().UnixNano())
				pingFrame := frame.NewPingFrame(uint32(pingSeq), nowNs)
				pingSeq++

				if err := ac.WriteFrame(pingFrame); err != nil {
					logger.Warn("Bonding", "PING send failed",
						"artery", ac.Tag(), "error", err)
					continue
				}

				if m, ok := metrics[ac.Tag()]; ok {
					m.RecordPingSent()
				}
			}
		}
	}
}

// replaceDeadArtery swaps a dead artery with a fresh node.
func (e *BondingEngine) replaceDeadArtery(ctx context.Context, pathID string) {
	if ctx.Err() != nil {
		return
	}

	logger.Info("Bonding", "Replacing dead artery", "path", pathID)

	_ = e.refreshCandidatePool()

	e.mu.Lock()
	defer e.mu.Unlock()

	if e.state != BondingStateRunning {
		return
	}

	if len(e.candidatePool) == 0 {
		logger.Warn("Bonding", "No replacement candidates", "path", pathID)
		return
	}

	replacement := e.candidatePool[0]
	e.candidatePool = e.candidatePool[1:]

	// Reset metrics
	metrics := control.NewPathMetrics(pathID)
	metrics.UpdateRTT(float64(replacement.LatencyMs))
	e.metrics[pathID] = metrics

	// Update state machine
	e.stateMachine.RemovePath(pathID)
	e.stateMachine.AddPath(control.NewPathEntry(pathID, metrics))

	// Update dispatcher
	if e.dispatcher != nil {
		e.dispatcher.UpdateArteries(e.arteries, e.metrics)
	}

	logger.Info("Bonding", "Artery replaced",
		"path", pathID,
		"new_node", replacement.Name,
		"new_latency", replacement.LatencyMs,
	)

	// Trigger core reload
	go func() {
		if ctx.Err() != nil {
			return
		}
		e.mu.RLock()
		running := (e.state == BondingStateRunning)
		e.mu.RUnlock()
		if !running {
			return
		}

		if err := e.compileAndStartCore(); err != nil {
			logger.Error("Bonding", "Failed to reload core after replacement", "error", err)
		}
	}()
}

// telemetryLoop periodically emits status to the TelemetryChan.
func (e *BondingEngine) telemetryLoop(ctx context.Context) {
	ticker := time.NewTicker(1 * time.Second)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			status := e.GetStatus()
			select {
			case e.TelemetryChan <- status:
			default: // drop if full
			}
		}
	}
}

// newOpenFrame is a helper that creates an OPEN frame.
func newOpenFrame(streamID uint32, seq uint64, targetAddr string) *frame.Frame {
	return frame.NewOpenFrame(streamID, seq, targetAddr)
}
