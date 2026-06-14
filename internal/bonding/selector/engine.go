// Package selector implements Mode A of the DMB Engine — a smart selector/failover
// engine that continuously monitors multiple V2Ray proxy lines, selects the fastest,
// and automatically fails over when lines degrade or die.
//
// This engine works entirely client-side with zero server changes. It leverages
// Xray's native balancer + observatory for traffic routing, while adding intelligent
// health-check, anti-oscillation, and pool replacement logic on top.
package selector

import (
	"context"
	"fmt"
	"sync"
	"sync/atomic"
	"time"

	"clever-connect/internal/bonding/control"
	"clever-connect/internal/bonding/hotswap"
	"clever-connect/internal/db"
	"clever-connect/internal/db/pebble"
	"clever-connect/internal/logger"
	"clever-connect/internal/models"
	"clever-connect/internal/v2ray/core"

	obscommand "github.com/xtls/xray-core/app/observatory/command"
	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials/insecure"
)

// EngineState represents the lifecycle state of the selector engine.
type EngineState string

const (
	EngineStateStopped  EngineState = "stopped"
	EngineStateStarting EngineState = "starting"
	EngineStateRunning  EngineState = "running"
	EngineStateStopping EngineState = "stopping"
)

// Engine is the Mode A selector/failover engine singleton.
type Engine struct {
	mu sync.RWMutex

	state      EngineState
	config     *models.BondingEngineConfig
	cancelFunc context.CancelFunc

	// Active artery pool
	arteries []*ArteryEntry

	// State machine for health monitoring
	stateMachine *control.StateMachine

	// Per-artery performance metrics
	metrics map[string]*control.PathMetrics

	// Pool of candidate nodes (from scanner results)
	candidatePool []models.V2RayClientConfig

	// Hot-swap manager for per-artery outbound swaps via gRPC
	hotswapMgr *hotswap.Manager

	// Telemetry channels
	TelemetryChan chan EngineStatus

	// Traffic tracking for speed calculation
	prevBytesTx int64
	prevBytesRx int64
}

// ArteryEntry tracks one active line in the selector pool.
type ArteryEntry struct {
	Config   models.V2RayClientConfig
	Tag      string // "artery-0", "artery-1", ...
	DBRecord *models.BondingArtery
}

// EngineStatus is the snapshot sent over the telemetry channel.
type EngineStatus struct {
	State         string                `json:"state"`
	Mode          string                `json:"mode"`
	ActiveCount   int                   `json:"active_count"`
	TotalPool     int                   `json:"total_pool"`
	Arteries      []ArteryStatus        `json:"arteries"`
	LastEvalAt    time.Time             `json:"last_eval_at"`
	// Real-time traffic counters (populated from local proxy wrapper)
	BytesTx       int64                 `json:"bytes_tx"`
	BytesRx       int64                 `json:"bytes_rx"`
	UplinkBps     int64                 `json:"uplink_bps"`
	DownlinkBps   int64                 `json:"downlink_bps"`
	ActiveConns   int                   `json:"active_conns"`
}

// ArteryStatus is per-artery telemetry.
type ArteryStatus struct {
	Tag            string  `json:"tag"`
	NodeName       string  `json:"node_name"`
	Address        string  `json:"address"`
	Port           int     `json:"port"`
	State          string  `json:"state"`
	SrttMs         float64 `json:"srtt_ms"`
	LossPct        float64 `json:"loss_pct"`
	WinRate        float64 `json:"win_rate"`
	ThroughputMBps float64 `json:"throughput_mbps"`
	ErrorCount     int     `json:"error_count"`
}

var (
	engineOnce   sync.Once
	globalEngine *Engine
)

// GetEngine returns the singleton Engine instance.
func GetEngine() *Engine {
	engineOnce.Do(func() {
		globalEngine = &Engine{
			state:         EngineStateStopped,
			metrics:       make(map[string]*control.PathMetrics),
			TelemetryChan: make(chan EngineStatus, 50),
		}
	})
	return globalEngine
}

// State returns the current engine state.
func (e *Engine) State() EngineState {
	e.mu.RLock()
	defer e.mu.RUnlock()
	return e.state
}

// StartEngine starts the selector/failover engine with the given configuration.
func (e *Engine) StartEngine(cfg *models.BondingEngineConfig) error {
	e.mu.Lock()
	if e.state == EngineStateRunning || e.state == EngineStateStarting {
		e.mu.Unlock()
		return fmt.Errorf("selector engine is already running")
	}
	e.state = EngineStateStarting
	e.config = cfg
	e.mu.Unlock()

	logger.Info("Bonding", "Starting selector/failover engine",
		"max_arteries", cfg.MaxArteries,
		"socks_port", cfg.SocksPort,
		"http_port", cfg.HTTPPort,
	)

	// Initialize thresholds from config
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

	// Build initial pool from best available nodes
	if err := e.refreshCandidatePool(); err != nil {
		e.mu.Lock()
		e.state = EngineStateStopped
		e.mu.Unlock()
		return fmt.Errorf("failed to build candidate pool: %w", err)
	}

	// Select top-N nodes for the active pool
	if err := e.buildActivePool(); err != nil {
		e.mu.Lock()
		e.state = EngineStateStopped
		e.mu.Unlock()
		return fmt.Errorf("failed to build active pool: %w", err)
	}

	// Compile and start xray core with balancer config
	if err := e.compileAndStartCore(); err != nil {
		e.mu.Lock()
		if e.state == EngineStateStarting {
			e.state = EngineStateStopped
		}
		e.mu.Unlock()
		return fmt.Errorf("failed to start core: %w", err)
	}

	e.mu.Lock()
	if e.state != EngineStateStarting {
		// StopEngine was called during startup!
		_ = core.StopCore()
		e.state = EngineStateStopped
		e.arteries = nil
		e.stateMachine = nil
		e.metrics = make(map[string]*control.PathMetrics)
		e.mu.Unlock()
		return fmt.Errorf("selector engine was stopped during startup")
	}

	// Start background health-check loop
	ctx, cancel := context.WithCancel(context.Background())
	e.cancelFunc = cancel
	e.state = EngineStateRunning
	e.mu.Unlock()

	// Reset traffic counters
	core.ResetClientTraffic()

	// Start strong SOCKS5+HTTP proxy wrapper (xray runs on internal ports socksPort+2000 and httpPort+2000)
	socksPort := cfg.SocksPort
	if socksPort <= 0 {
		socksPort = 10646
	}
	httpPort := cfg.HTTPPort
	if httpPort <= 0 {
		httpPort = 10545
	}
	core.StartLocalProxyEngine(socksPort, socksPort+2000, httpPort, httpPort+2000)

	go e.healthCheckLoop(ctx)
	go e.telemetryLoop(ctx)

	// Initialize hot-swap manager (best-effort — used for artery replacement)
	go func() {
		// Give the core 2 seconds to fully start before connecting gRPC
		select {
		case <-ctx.Done():
			return
		case <-time.After(2 * time.Second):
		}

		mgr := hotswap.NewManager("127.0.0.1:10085")
		if err := mgr.Connect(); err != nil {
			logger.Warn("Bonding", "Hot-swap manager failed to connect (will fallback to core restart)", "error", err)
		} else {
			e.mu.Lock()
			if e.state != EngineStateRunning {
				mgr.Close()
				e.mu.Unlock()
				return
			}
			e.hotswapMgr = mgr
			e.mu.Unlock()
			logger.Info("Bonding", "Hot-swap manager ready for per-artery swaps")
		}
	}()

	logger.Info("Bonding", "Selector engine started successfully",
		"active_arteries", len(e.arteries),
		"candidate_pool", len(e.candidatePool),
	)

	return nil
}

// StopEngine gracefully stops the selector/failover engine.
func (e *Engine) StopEngine() error {
	e.mu.Lock()
	if e.state != EngineStateRunning && e.state != EngineStateStarting {
		e.mu.Unlock()
		return nil
	}
	e.state = EngineStateStopping
	if e.cancelFunc != nil {
		e.cancelFunc()
		e.cancelFunc = nil
	}
	e.mu.Unlock()

	logger.Info("Bonding", "Stopping selector engine")

	// Stop local proxy wrapper
	core.StopLocalProxyEngine()

	// Stop the xray core process
	_ = core.StopCore()

	e.mu.Lock()
	if e.hotswapMgr != nil {
		e.hotswapMgr.Close()
		e.hotswapMgr = nil
	}
	e.state = EngineStateStopped
	e.arteries = nil
	e.stateMachine = nil
	e.metrics = make(map[string]*control.PathMetrics)
	e.mu.Unlock()

	logger.Info("Bonding", "Selector engine stopped")
	return nil
}

// GetStatus returns a snapshot of the current engine status.
func (e *Engine) GetStatus() EngineStatus {
	e.mu.RLock()
	defer e.mu.RUnlock()

	status := EngineStatus{
		State:       string(e.state),
		Mode:        "selector",
		ActiveCount: len(e.arteries),
		TotalPool:   len(e.candidatePool),
		LastEvalAt:  time.Now(),
	}

	if e.config != nil {
		status.Mode = e.config.Mode
	}

	for _, a := range e.arteries {
		as := ArteryStatus{
			Tag:      a.Tag,
			NodeName: a.Config.Name,
			Address:  a.Config.Address,
			Port:     a.Config.Port,
		}

		if m, ok := e.metrics[a.Tag]; ok {
			snap := m.Snapshot()
			as.SrttMs = snap.SRTT
			as.LossPct = snap.LossPct
			as.WinRate = snap.WinRate
		}

		if a.DBRecord != nil {
			as.State = a.DBRecord.State
			as.ThroughputMBps = a.DBRecord.ThroughputMBps
			as.ErrorCount = a.DBRecord.ErrorCount
		}

		status.Arteries = append(status.Arteries, as)
	}

	return status
}

// refreshCandidatePool loads the best nodes from PebbleDB sorted by latency.
func (e *Engine) refreshCandidatePool() error {
	configs, total := pebble.ListClientConfigs(pebble.ConfigFilter{
		SortBy:     "latency",
		PingStatus: "pass",
	}, 0, 0)

	if total == 0 {
		return fmt.Errorf("no healthy nodes available in PebbleDB")
	}

	// Filter out nodes that are already active
	e.mu.Lock()
	activeAddrs := make(map[string]bool)
	for _, a := range e.arteries {
		activeAddrs[fmt.Sprintf("%s:%d", a.Config.Address, a.Config.Port)] = true
	}
	e.mu.Unlock()

	var candidates []models.V2RayClientConfig
	for _, cfg := range configs {
		key := fmt.Sprintf("%s:%d", cfg.Address, cfg.Port)
		if !activeAddrs[key] && cfg.LatencyMs > 0 {
			candidates = append(candidates, cfg)
		}
	}

	e.mu.Lock()
	e.candidatePool = candidates
	e.mu.Unlock()

	return nil
}

// buildActivePool selects the top-N nodes for the active artery pool.
func (e *Engine) buildActivePool() error {
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
		return fmt.Errorf("no candidate nodes available for the active pool")
	}

	e.arteries = make([]*ArteryEntry, 0, count)

	for i := 0; i < count; i++ {
		tag := fmt.Sprintf("artery-%d", i)
		localPort := 21001 + i

		metrics := control.NewPathMetrics(tag)
		metrics.UpdateRTT(float64(e.candidatePool[i].LatencyMs))
		e.metrics[tag] = metrics

		// Create DB record
		dbArtery := &models.BondingArtery{
			NodeConfigID: e.candidatePool[i].ID,
			Tag:          tag,
			LocalPort:    localPort,
			State:        "active",
			SrttMs:       float64(e.candidatePool[i].LatencyMs),
		}
		if db.DB != nil {
			db.DB.Create(dbArtery)
		}

		entry := &ArteryEntry{
			Config:   e.candidatePool[i],
			Tag:      tag,
			DBRecord: dbArtery,
		}
		e.arteries = append(e.arteries, entry)

		// Register with state machine
		pathEntry := control.NewPathEntry(tag, metrics)
		e.stateMachine.AddPath(pathEntry)

		logger.Info("Bonding", "Artery added to pool",
			"tag", tag,
			"name", e.candidatePool[i].Name,
			"address", e.candidatePool[i].Address,
			"port", e.candidatePool[i].Port,
			"latency_ms", e.candidatePool[i].LatencyMs,
		)
	}

	// Remove used nodes from candidate pool
	e.candidatePool = e.candidatePool[count:]

	return nil
}

// healthCheckLoop runs continuous evaluation cycles.
func (e *Engine) healthCheckLoop(ctx context.Context) {
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

// runEvaluation executes one health-check cycle.
func (e *Engine) runEvaluation(ctx context.Context) {
	e.queryObservatoryAndUpdateMetrics()

	e.mu.RLock()
	if e.stateMachine == nil || len(e.arteries) == 0 {
		e.mu.RUnlock()
		return
	}

	// Collect metrics for state machine evaluation
	var pathMetrics []*control.PathMetrics
	for _, m := range e.metrics {
		pathMetrics = append(pathMetrics, m)
	}
	e.mu.RUnlock()

	// Compute median and best SRTT
	sched := control.NewScheduler(control.ModeAuto, pathMetrics)
	medianSRTT := sched.MedianSRTT()
	bestSRTT := sched.BestSRTT()

	// Run state machine evaluation
	events := e.stateMachine.Evaluate(medianSRTT, bestSRTT)

	// Process transition events
	for _, event := range events {
		logger.Info("Bonding", "Path state transition",
			"path", event.PathID,
			"old_state", event.OldState.String(),
			"new_state", event.NewState.String(),
			"reason", event.Reason,
		)

		// Update DB record
		e.mu.RLock()
		for _, a := range e.arteries {
			if a.Tag == event.PathID && a.DBRecord != nil {
				a.DBRecord.State = event.NewState.String()
				if db.DB != nil {
					db.DB.Save(a.DBRecord)
				}
			}
		}
		e.mu.RUnlock()

		// Handle dead paths — replace with fresh node
		if event.NewState == control.PathDead {
			go e.replaceDeadArtery(ctx, event.PathID)
		}
	}

	// Persist metrics to DB
	e.persistMetrics()
}

// replaceDeadArtery swaps a dead artery with a fresh node from the candidate pool.
func (e *Engine) replaceDeadArtery(ctx context.Context, pathID string) {
	if ctx.Err() != nil {
		return
	}

	logger.Info("Bonding", "Replacing dead artery", "path", pathID)

	// Refresh pool to get latest nodes
	_ = e.refreshCandidatePool()

	e.mu.Lock()
	defer e.mu.Unlock()

	if e.state != EngineStateRunning {
		return
	}

	if len(e.candidatePool) == 0 {
		logger.Warn("Bonding", "No replacement candidates available", "path", pathID)
		return
	}

	// Find the dead artery
	var deadIdx int = -1
	for i, a := range e.arteries {
		if a.Tag == pathID {
			deadIdx = i
			break
		}
	}
	if deadIdx == -1 {
		return
	}

	// Pick the best candidate
	replacement := e.candidatePool[0]
	e.candidatePool = e.candidatePool[1:]

	oldEntry := e.arteries[deadIdx]

	// Update the artery entry
	oldEntry.Config = replacement
	if oldEntry.DBRecord != nil {
		oldEntry.DBRecord.NodeConfigID = replacement.ID
		oldEntry.DBRecord.State = "active"
		oldEntry.DBRecord.ErrorCount = 0
		oldEntry.DBRecord.SrttMs = float64(replacement.LatencyMs)
		oldEntry.DBRecord.LastSwapAt = time.Now()
		if db.DB != nil {
			db.DB.Save(oldEntry.DBRecord)
		}
	}

	// Reset metrics
	metrics := control.NewPathMetrics(pathID)
	metrics.UpdateRTT(float64(replacement.LatencyMs))
	e.metrics[pathID] = metrics

	// Update state machine
	e.stateMachine.RemovePath(pathID)
	e.stateMachine.AddPath(control.NewPathEntry(pathID, metrics))

	logger.Info("Bonding", "Artery replaced successfully",
		"path", pathID,
		"new_node", replacement.Name,
		"new_address", replacement.Address,
		"new_latency_ms", replacement.LatencyMs,
	)

	// Hot-swap the outbound via gRPC if available; fallback to full core restart
	hotswapMgr := e.hotswapMgr
	newConfig := replacement
	swapTag := pathID

	go func() {
		if ctx.Err() != nil {
			return
		}
		e.mu.RLock()
		running := (e.state == EngineStateRunning)
		e.mu.RUnlock()
		if !running {
			return
		}

		if hotswapMgr != nil && hotswapMgr.IsConnected() {
			if err := hotswapMgr.SwapOutbound(swapTag, newConfig, swapTag); err != nil {
				logger.Warn("Bonding", "Hot-swap failed, falling back to core restart",
					"tag", swapTag, "error", err)
				e.mu.RLock()
				running = (e.state == EngineStateRunning)
				e.mu.RUnlock()
				if !running {
					return
				}
				if err := e.compileAndStartCore(); err != nil {
					logger.Error("Bonding", "Failed to reload core after replacement", "error", err)
				}
			} else {
				logger.Info("Bonding", "Hot-swap completed — zero downtime for other arteries",
					"tag", swapTag)
			}
		} else {
			// Fallback: full core restart
			if err := e.compileAndStartCore(); err != nil {
				logger.Error("Bonding", "Failed to reload core after replacement", "error", err)
			}
		}
	}()
}

// persistMetrics saves per-artery metrics to the database.
func (e *Engine) persistMetrics() {
	e.mu.RLock()
	defer e.mu.RUnlock()

	if db.DB == nil {
		return
	}

	for _, a := range e.arteries {
		if a.DBRecord == nil {
			continue
		}
		if m, ok := e.metrics[a.Tag]; ok {
			snap := m.Snapshot()
			a.DBRecord.SrttMs = snap.SRTT
			a.DBRecord.LossPct = snap.LossPct
			a.DBRecord.WinRate = snap.WinRate
			db.DB.Save(a.DBRecord)
		}
	}
}

// telemetryLoop periodically emits status to the TelemetryChan.
func (e *Engine) telemetryLoop(ctx context.Context) {
	ticker := time.NewTicker(1 * time.Second)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			status := e.GetStatus()

			// Attach real-time traffic metrics from local proxy wrapper
			tx, rx, conns := core.GetClientTraffic()
			status.BytesTx = tx
			status.BytesRx = rx
			status.ActiveConns = conns

			// Calculate per-second speed using atomic stored previous values
			prevTx := atomic.LoadInt64(&e.prevBytesTx)
			prevRx := atomic.LoadInt64(&e.prevBytesRx)
			if tx >= prevTx {
				status.UplinkBps = tx - prevTx
			}
			if rx >= prevRx {
				status.DownlinkBps = rx - prevRx
			}
			atomic.StoreInt64(&e.prevBytesTx, tx)
			atomic.StoreInt64(&e.prevBytesRx, rx)

			select {
			case e.TelemetryChan <- status:
			default: // drop if channel full
			}
		}
	}
}

// queryObservatoryAndUpdateMetrics queries Xray's observatory gRPC service to update path metrics.
func (e *Engine) queryObservatoryAndUpdateMetrics() {
	grpcConn, err := grpc.Dial("127.0.0.1:10085", grpc.WithTransportCredentials(insecure.NewCredentials()))
	if err != nil {
		return
	}
	defer grpcConn.Close()

	client := obscommand.NewObservatoryServiceClient(grpcConn)
	ctx, cancel := context.WithTimeout(context.Background(), 1*time.Second)
	defer cancel()

	resp, err := client.GetOutboundStatus(ctx, &obscommand.GetOutboundStatusRequest{})
	if err != nil || resp == nil || resp.Status == nil {
		return
	}

	e.mu.Lock()
	defer e.mu.Unlock()

	if e.state != EngineStateRunning {
		return
	}

	for _, status := range resp.Status.Status {
		if status == nil {
			continue
		}
		tag := status.OutboundTag
		metrics, ok := e.metrics[tag]
		if !ok {
			continue
		}

		// Simulate send so EWMA updates correctly
		metrics.RecordSend()

		if status.Alive {
			// Update EWMA RTT
			if status.Delay > 0 {
				metrics.UpdateRTT(float64(status.Delay))
			}
			metrics.SetAlive(true)
			// Mode A doesn't have frame logic, but we can simulate Acks to keep WinRate and EWMA loss correct
			metrics.RecordAck(0, float64(status.Delay))
		} else {
			metrics.SetAlive(false)
			metrics.RecordLoss()
		}
	}
}
