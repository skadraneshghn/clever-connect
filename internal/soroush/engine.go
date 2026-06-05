// Package soroush implements the Soroush WebRTC "The Hive" tunnel engine.
// It operates as an additive, parallel service to the existing Ehco infrastructure,
// routing traffic through Soroush's domestic LiveKit SFU group calls to bypass DPI.
package soroush

import (
	"context"
	"fmt"
	"sync"
	"sync/atomic"
	"time"

	"clever-connect/internal/logger"
	"clever-connect/internal/models"
)

const component = "Soroush"

var (
	engineMu     sync.Mutex
	engineCtx    context.Context
	engineCancel context.CancelFunc
	running      bool

	// Telemetry counters
	totalStreams  atomic.Int64
	bytesRelayed atomic.Int64
	startedAt    time.Time
)

// StartEngine starts the Soroush tunnel in either server or client mode.
// Mode is determined by isServer parameter (derived from APP_MODE env var).
// Server mode: joins LiveKit room, accepts authenticated DataChannels → relays to internet
// Client mode: joins LiveKit room, opens DataChannels → SOCKS5 proxy
func StartEngine(cfg *models.SoroushTunnelConfig, accounts []models.SoroushAccount, isServer bool) error {
	engineMu.Lock()
	defer engineMu.Unlock()

	if running {
		return fmt.Errorf("soroush engine is already running")
	}

	if len(accounts) == 0 {
		return fmt.Errorf("no soroush accounts configured")
	}

	if cfg.PSK == "" {
		return fmt.Errorf("PSK is required for in-band DataChannel authentication")
	}

	engineCtx, engineCancel = context.WithCancel(context.Background())
	running = true
	startedAt = time.Now()
	totalStreams.Store(0)
	bytesRelayed.Store(0)

	mode := "client"
	if isServer {
		mode = "server"
	}

	logger.Info(component, "Starting Soroush tunnel engine",
		"mode", mode,
		"accounts", len(accounts),
		"group_chat_id", cfg.GroupChatID,
		"socks_port", cfg.SocksPort,
		"max_workers", cfg.MaxWorkers,
	)

	if isServer {
		go runServer(engineCtx, cfg, accounts)
	} else {
		go runClient(engineCtx, cfg, accounts)
	}

	return nil
}

// runServer starts the server-side (Queen) engine.
// Each account gets a TokenManager → LiveKitTransport → yamux.Server → relay handler.
func runServer(ctx context.Context, cfg *models.SoroushTunnelConfig, accounts []models.SoroushAccount) {
	logger.Info(component, "Server engine goroutine started")

	// Use the first account as the server's room participant
	acct := accounts[0]
	tm := NewTokenManager(&acct, cfg)

	if err := tm.Start(ctx); err != nil {
		logger.Error(component, "TokenManager failed to start", "error", err)
		return
	}
	defer tm.Stop()

	token := tm.CurrentToken()
	if token == nil {
		logger.Error(component, "No token available after TokenManager start")
		return
	}

	transport := NewLiveKitTransport(token, cfg, true)
	if err := transport.Connect(ctx); err != nil {
		logger.Error(component, "LiveKit transport connect failed", "error", err)
		return
	}
	defer transport.Close()

	logger.Info(component, "Server joined LiveKit room, waiting for authenticated workers")

	// Block until context is cancelled
	<-ctx.Done()
	logger.Info(component, "Server engine shutting down")
}

// runClient starts the client-side (Swarm) engine.
// Manages a pool of workers, each with their own TokenManager + Transport.
func runClient(ctx context.Context, cfg *models.SoroushTunnelConfig, accounts []models.SoroushAccount) {
	logger.Info(component, "Client engine goroutine started")

	pool := NewMultiplexerPool(cfg.LoadBalanceAlgo)

	// Start SOCKS5 listener
	go func() {
		if err := StartSOCKS5Listener(ctx, cfg.SocksPort, pool); err != nil {
			logger.Error(component, "SOCKS5 listener error", "error", err)
		}
	}()

	// Start health checker
	go pool.HealthCheck(ctx)

	// Launch worker goroutines (up to MaxWorkers or len(accounts))
	maxWorkers := cfg.MaxWorkers
	if maxWorkers > len(accounts) {
		maxWorkers = len(accounts)
	}

	var wg sync.WaitGroup
	for i := 0; i < maxWorkers; i++ {
		wg.Add(1)
		go func(acct models.SoroushAccount) {
			defer wg.Done()
			runWorker(ctx, cfg, &acct, pool)
		}(accounts[i])
	}

	wg.Wait()
	logger.Info(component, "Client engine shutting down — all workers exited")
}

// runWorker manages a single worker connection lifecycle with auto-reconnect.
func runWorker(ctx context.Context, cfg *models.SoroushTunnelConfig, acct *models.SoroushAccount, pool *MultiplexerPool) {
	for {
		select {
		case <-ctx.Done():
			return
		default:
		}

		logger.Info(component, "Worker starting", "phone", maskPhone(acct.PhoneNumber))

		tm := NewTokenManager(acct, cfg)
		if err := tm.Start(ctx); err != nil {
			logger.Error(component, "Worker token manager failed", "phone", maskPhone(acct.PhoneNumber), "error", err)
			sleepWithContext(ctx, 10*time.Second)
			continue
		}

		token := tm.CurrentToken()
		if token == nil {
			logger.Error(component, "Worker got nil token", "phone", maskPhone(acct.PhoneNumber))
			tm.Stop()
			sleepWithContext(ctx, 10*time.Second)
			continue
		}

		transport := NewLiveKitTransport(token, cfg, false)
		if err := transport.Connect(ctx); err != nil {
			logger.Error(component, "Worker transport connect failed", "phone", maskPhone(acct.PhoneNumber), "error", err)
			tm.Stop()
			sleepWithContext(ctx, 10*time.Second)
			continue
		}

		yamuxSess := transport.YamuxSession()
		if yamuxSess == nil {
			logger.Error(component, "Worker yamux session is nil", "phone", maskPhone(acct.PhoneNumber))
			transport.Close()
			tm.Stop()
			sleepWithContext(ctx, 10*time.Second)
			continue
		}

		wc := &WorkerChannel{
			AccountID:    fmt.Sprintf("%d", acct.ID),
			Transport:    transport,
			YamuxSession: yamuxSess,
			Healthy:      true,
		}
		pool.Inject(wc)

		logger.Info(component, "Worker connected and injected into pool", "phone", maskPhone(acct.PhoneNumber))

		// Block until yamux session closes or context cancels
		select {
		case <-ctx.Done():
			pool.Purge(wc.AccountID)
			transport.Close()
			tm.Stop()
			return
		case <-yamuxSess.CloseChan():
			logger.Warn(component, "Worker yamux session closed, reconnecting", "phone", maskPhone(acct.PhoneNumber))
			pool.Purge(wc.AccountID)
			transport.Close()
			tm.Stop()
			sleepWithContext(ctx, 5*time.Second)
		}
	}
}

// StopEngine gracefully stops the Soroush tunnel engine.
func StopEngine() {
	engineMu.Lock()
	defer engineMu.Unlock()

	if !running {
		return
	}

	logger.Info(component, "Stopping Soroush tunnel engine")
	engineCancel()
	running = false
}

// IsRunning returns whether the engine is currently active.
func IsRunning() bool {
	engineMu.Lock()
	defer engineMu.Unlock()
	return running
}

// TunnelStatus contains the current state of the Soroush tunnel engine.
type TunnelStatus struct {
	Running       bool      `json:"running"`
	Mode          string    `json:"mode"`
	ActiveWorkers int       `json:"active_workers"`
	TotalStreams   int64     `json:"total_streams"`
	BytesRelayed  int64     `json:"bytes_relayed"`
	Uptime        string    `json:"uptime"`
	TokenExpiry   time.Time `json:"token_expiry"`
	PoolStats     PoolStats `json:"pool_stats"`
}

// GetStatus returns the current tunnel status snapshot.
func GetStatus() *TunnelStatus {
	engineMu.Lock()
	isRunning := running
	engineMu.Unlock()

	status := &TunnelStatus{
		Running:      isRunning,
		TotalStreams:  totalStreams.Load(),
		BytesRelayed: bytesRelayed.Load(),
	}

	if isRunning {
		status.Uptime = time.Since(startedAt).Truncate(time.Second).String()
	}

	return status
}

// sleepWithContext sleeps for the given duration or until context is cancelled.
func sleepWithContext(ctx context.Context, d time.Duration) {
	select {
	case <-time.After(d):
	case <-ctx.Done():
	}
}

// maskPhone masks a phone number for safe logging.
func maskPhone(phone string) string {
	if len(phone) < 4 {
		return "****"
	}
	return phone[:3] + "****" + phone[len(phone)-2:]
}
