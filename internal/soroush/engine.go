// Package soroush implements the Soroush SFU P2P Swarm tunnel engine.
// It operates as an additive, parallel service to the existing Ehco infrastructure,
// routing traffic securely through LiveKit WebRTC DataChannels signaled via tokens.
package soroush

import (
	"context"
	"fmt"
	"io"
	"math/rand"
	"sync"
	"sync/atomic"
	"time"

	"clever-connect/internal/db"
	"clever-connect/internal/logger"
	"clever-connect/internal/models"

	"github.com/hashicorp/yamux"
	lksdk "github.com/livekit/server-sdk-go/v2"
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

// StartEngine starts the LiveKit tunnel in either server or client mode.
func StartEngine(cfg *models.SoroushTunnelConfig, accounts []models.SoroushAccount, isServer bool) error {
	engineMu.Lock()
	defer engineMu.Unlock()

	if running {
		return fmt.Errorf("soroush engine is already running")
	}

	if cfg.PSK == "" {
		return fmt.Errorf("PSK is required for in-band DataChannel authentication")
	}

	if len(accounts) == 0 {
		return fmt.Errorf("no soroush accounts configured")
	}

	engineCtx, engineCancel = context.WithCancel(context.Background())
	running = true
	startedAt = time.Now()
	totalStreams.Store(0)
	bytesRelayed.Store(0)

	mode := "client"
	if isServer {
		mode = "server"
		if cfg.ServerIdentity == "" {
			logger.Warn(component, "ServerIdentity is empty; clients will not know who to route to")
		}
	} else {
		if cfg.ServerIdentity == "" {
			return fmt.Errorf("ServerIdentity is required for clients to locate the server in the SFU room")
		}
	}

	logger.Info(component, "Starting Soroush LiveKit engine",
		"mode", mode,
		"accounts", len(accounts),
		"socks_port", cfg.SocksPort,
		"max_workers", cfg.MaxWorkers,
	)

	if isServer {
		// Server uses the first account as the host
		go runServer(engineCtx, cfg, &accounts[0])
	} else {
		go runClient(engineCtx, cfg, accounts)
	}

	return nil
}

// runServer starts the server-side (Queen) engine bound to the LiveKit Room.
// Uses the host account's per-account LiveKitToken for connection.
func runServer(ctx context.Context, cfg *models.SoroushTunnelConfig, hostAccount *models.SoroushAccount) {
	logger.Info(component, "Server engine goroutine started, initializing SFU Listener")

	for {
		select {
		case <-ctx.Done():
			return
		default:
		}

		// Reload configuration dynamically
		var latestCfg models.SoroushTunnelConfig
		if err := db.DB.First(&latestCfg).Error; err == nil {
			cfg = &latestCfg
		}

		// Reload account to pick up refreshed token
		var latestAcct models.SoroushAccount
		if err := db.DB.First(&latestAcct, hostAccount.ID).Error; err == nil {
			hostAccount = &latestAcct
		}

		token, err := GetOrRefreshLiveKitToken(ctx, cfg, hostAccount, true)
		if err != nil {
			logger.Error(component, "Server: Failed to get or refresh LiveKitToken. Retrying in 10s...", "error", err)
			sleepWithContext(ctx, 10*time.Second)
			continue
		}
		hostAccount.LiveKitToken = token

		url := cfg.LiveKitURL
		if url == "" {
			url = "wss://k.splus.ir" // Default Soroush LiveKit endpoint
		}

		// Create listener + callback BEFORE connecting (room.callback is unexported in v2 SDK)
		listener, listenerCb := NewLiveKitListener()

		room, err := lksdk.ConnectToRoomWithToken(url, hostAccount.LiveKitToken, listenerCb)
		if err != nil {
			logger.Error(component, "Server: Failed to connect to LiveKit Room", "error", err)
			listener.Close()
			sleepWithContext(ctx, 10*time.Second)
			continue
		}

		// Bind the room reference so LiveKitConn.Write() can publish data
		listener.BindRoom(room)

		logger.Info(component, "Server: Connected to SFU. Virtual Listener active, awaiting worker traffic...",
			"local_identity", room.LocalParticipant.Identity(),
		)

		// Spin off context cancellation and disconnect watcher
		stopChan := make(chan struct{})
		go func() {
			select {
			case <-ctx.Done():
			case <-stopChan:
			}
			room.Disconnect()
			listener.Close()
		}()

		acceptErr := func() error {
			defer close(stopChan)
			defer room.Disconnect()
			defer listener.Close()

			for {
				conn, err := listener.Accept()
				if err != nil {
					return err
				}
				go handleIncomingWorker(ctx, conn.(*LiveKitConn), cfg)
			}
		}()

		if acceptErr != nil {
			logger.Warn(component, "Server: SFU Listener disconnected, reconnecting in 5s...", "error", acceptErr)
			sleepWithContext(ctx, 5*time.Second)
		}
	}
}

func handleIncomingWorker(ctx context.Context, conn *LiveKitConn, cfg *models.SoroushTunnelConfig) {
	logger.Info(component, "Server: Worker connection detected, executing Handshake", "target", conn.targetIdentity)

	// 5-second zero-trust handshake (generous for Iranian SFU latency)
	challenge := make([]byte, 64)
	errChan := make(chan error, 1)
	go func() {
		_, err := io.ReadFull(conn, challenge)
		errChan <- err
	}()

	select {
	case <-time.After(5 * time.Second):
		logger.Warn(component, "Server: Handshake timeout, rejecting SFU pipe", "target", conn.targetIdentity)
		conn.Close()
		return
	case err := <-errChan:
		if err != nil {
			logger.Warn(component, "Server: Handshake read error", "error", err)
			conn.Close()
			return
		}
	}

	if err := VerifyHandshakeChallenge(cfg.PSK, challenge); err != nil {
		logger.Warn(component, "Server: Unauthorized handshake, dropping pipe", "error", err)
		conn.Close()
		return
	}

	logger.Info(component, "Server: Handshake verified, mounting Yamux", "target", conn.targetIdentity)

	yamuxCfg := yamux.DefaultConfig()
	yamuxCfg.LogOutput = nil
	yamuxCfg.MaxStreamWindowSize = 1024 * 1024
	yamuxCfg.ConnectionWriteTimeout = 5 * time.Second
	yamuxCfg.AcceptBacklog = 1024

	yamuxSess, err := yamux.Server(conn, yamuxCfg)
	if err != nil {
		logger.Error(component, "Server: Failed to map Yamux over SFU pipe", "error", err)
		conn.Close()
		return
	}

	StartRelayHandler(ctx, yamuxSess)
}

// runClient starts the client-side (Swarm) engine.
func runClient(ctx context.Context, cfg *models.SoroushTunnelConfig, accounts []models.SoroushAccount) {
	logger.Info(component, "Client engine goroutine started")

	pool := NewMultiplexerPool(cfg.LoadBalanceAlgo)

	go func() {
		if err := StartSOCKS5Listener(ctx, cfg.SocksPort, pool); err != nil {
			logger.Error(component, "SOCKS5 listener error", "error", err)
		}
	}()

	go pool.HealthCheck(ctx)

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

// runWorker manages a single worker connection to the SFU Room.
// Each worker uses its own per-account LiveKitToken to avoid identity collisions.
func runWorker(ctx context.Context, cfg *models.SoroushTunnelConfig, acct *models.SoroushAccount, pool *MultiplexerPool) {
	defer func() {
		db.DB.Model(acct).Update("status", "idle")
	}()

	for {
		select {
		case <-ctx.Done():
			return
		default:
		}

		logger.Info(component, "Worker starting SFU connection phase", "phone", maskPhone(acct.PhoneNumber))
		db.DB.Model(acct).Update("status", "connecting")

		jitter := time.Duration(1000+rand.Intn(2000)) * time.Millisecond
		sleepWithContext(ctx, jitter)

		// Reload configuration dynamically
		var latestCfg models.SoroushTunnelConfig
		if err := db.DB.First(&latestCfg).Error; err == nil {
			cfg = &latestCfg
		}

		// Reload account to pick up refreshed LiveKitToken
		var latestAcct models.SoroushAccount
		if err := db.DB.First(&latestAcct, acct.ID).Error; err == nil {
			acct = &latestAcct
		}

		token, err := GetOrRefreshLiveKitToken(ctx, cfg, acct, false)
		if err != nil {
			logger.Error(component, "Worker: Failed to get or refresh LiveKitToken. Skipping.", "phone", maskPhone(acct.PhoneNumber), "error", err)
			db.DB.Model(acct).Update("status", "error")
			sleepWithContext(ctx, 10*time.Second)
			continue
		}
		acct.LiveKitToken = token

		url := cfg.LiveKitURL
		if url == "" {
			url = "wss://k.splus.ir" // Default Soroush LiveKit endpoint
		}

		serverTargetIdentity := cfg.ServerIdentity

		// [FIX #3] Initialize the io.Pipe and LiveKitConn BEFORE connecting to
		// prevent nil-pointer race: if the server sends data the microsecond we
		// connect, the callback must already have a live pipe to write into.
		pr, pw := io.Pipe()
		conn := &LiveKitConn{
			targetIdentity: serverTargetIdentity,
			pr:             pr,
			pw:             pw,
		}

		roomCb := lksdk.NewRoomCallback()
		roomCb.OnDataReceived = func(data []byte, params lksdk.DataReceiveParams) {
			// Only accept data originating from the Queen Server identity
			if params.SenderIdentity == serverTargetIdentity {
				_ = conn.WriteIncoming(data)
			}
		}

		// Each worker connects with its OWN per-account token (unique identity)
		room, err := lksdk.ConnectToRoomWithToken(url, acct.LiveKitToken, roomCb)
		if err != nil {
			logger.Error(component, "Worker: Failed to connect to SFU Room", "error", err)
			db.DB.Model(acct).Update("status", "error")
			conn.Close() // cleanup pipe
			sleepWithContext(ctx, 10*time.Second)
			continue
		}

		// Attach room reference after successful connection so Write() can publish
		conn.room = room

		// Send 64-byte HKDF challenge
		challenge, err := BuildHandshakeChallenge(cfg.PSK)
		if err != nil {
			logger.Error(component, "Worker: Failed to build handshake challenge", "error", err)
			conn.Close()
			room.Disconnect()
			continue
		}

		if _, err := conn.Write(challenge); err != nil {
			logger.Error(component, "Worker: Failed to write handshake challenge", "error", err)
			conn.Close()
			room.Disconnect()
			continue
		}

		logger.Info(component, "Worker: SFU connection established, initiating Yamux", "phone", maskPhone(acct.PhoneNumber))

		yamuxCfg := yamux.DefaultConfig()
		yamuxCfg.LogOutput = nil
		yamuxCfg.MaxStreamWindowSize = 1024 * 1024
		yamuxCfg.ConnectionWriteTimeout = 5 * time.Second
		yamuxCfg.AcceptBacklog = 1024

		yamuxSess, yamuxError := yamux.Client(conn, yamuxCfg)
		if yamuxError != nil {
			logger.Error(component, "Worker: Failed to map Yamux client over SFU pipe", "error", yamuxError)
			conn.Close()
			room.Disconnect()
			continue
		}

		// Inject into load balancer pool
		wc := &WorkerChannel{
			AccountID:    fmt.Sprintf("%d", acct.ID),
			Transport:    &WebRTCTransport{}, // Stub for pool logic
			YamuxSession: yamuxSess,
			Healthy:      true,
		}
		pool.Inject(wc)

		logger.Info(component, "Worker connection active and routed", "phone", maskPhone(acct.PhoneNumber))
		db.DB.Model(acct).Update("status", "tunnel_active")

		select {
		case <-ctx.Done():
			pool.Purge(wc.AccountID)
			conn.Close()
			room.Disconnect()
			return
		case <-yamuxSess.CloseChan():
			logger.Warn(component, "Worker yamux session closed by peer, resetting", "phone", maskPhone(acct.PhoneNumber))
			db.DB.Model(acct).Update("status", "error")
			pool.Purge(wc.AccountID)
			conn.Close()
			room.Disconnect()
			sleepWithContext(ctx, 3*time.Second)
		}
	}
}

// StopEngine gracefully stops the tunnel engine.
func StopEngine() {
	engineMu.Lock()
	runningVal := running
	engineMu.Unlock()

	if !runningVal {
		return
	}

	logger.Info(component, "Stopping LiveKit Soroush tunnel engine")
	engineCancel()

	engineMu.Lock()
	running = false
	engineMu.Unlock()
}

// IsRunning returns whether the engine is currently active.
func IsRunning() bool {
	engineMu.Lock()
	defer engineMu.Unlock()
	return running
}

// TunnelStatus contains the current state of the Soroush tunnel engine.
type TunnelStatus struct {
	Running      bool      `json:"running"`
	Mode         string    `json:"mode"`
	TotalStreams  int64     `json:"total_streams"`
	BytesRelayed int64     `json:"bytes_relayed"`
	Uptime       string    `json:"uptime"`
	PoolStats    PoolStats `json:"pool_stats"`
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

func sleepWithContext(ctx context.Context, d time.Duration) {
	select {
	case <-time.After(d):
	case <-ctx.Done():
	}
}

func maskPhone(phone string) string {
	if len(phone) < 4 {
		return "****"
	}
	return phone[:3] + "****" + phone[len(phone)-2:]
}
