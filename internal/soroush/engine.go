// Package soroush implements the Soroush Message-Signaled P2P Swarm tunnel engine.
// It operates as an additive, parallel service to the existing Ehco infrastructure,
// routing traffic through Pion WebRTC P2P DataChannels signaled via encrypted text messages.
package soroush

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"math/rand"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"clever-connect/internal/db"
	"clever-connect/internal/logger"
	"clever-connect/internal/models"
	"clever-connect/internal/soroushlib"

	"github.com/hashicorp/yamux"
	"github.com/pion/webrtc/v4"
)

const component = "Soroush"

var (
	engineMu     sync.Mutex
	engineCtx    context.Context
	engineCancel context.CancelFunc
	running      bool

	// Telemetry counters
	totalStreams atomic.Int64
	bytesRelayed atomic.Int64
	startedAt    time.Time
)

// StartEngine starts the Soroush tunnel in either server or client mode.
// Mode is determined by isServer parameter (derived from APP_MODE env var).
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

	if cfg.PairingPIN == "" {
		return fmt.Errorf("PairingPIN is required to encrypt SDP messages")
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
		"server_phone", cfg.ServerPhoneNumber,
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
func runServer(ctx context.Context, cfg *models.SoroushTunnelConfig, accounts []models.SoroushAccount) {
	logger.Info(component, "Server engine goroutine started")

	// Use the first account as the server's listener
	acct := accounts[0]
	session, transport := soroushlib.RestoreSession(acct.AuthKey, acct.AuthKeyID, acct.ServerSalt)
	if err := transport.Connect(ctx); err != nil {
		logger.Error(component, "Server: Failed to connect to Soroush", "error", err)
		return
	}
	defer transport.Disconnect()

	if err := session.WarmUpSession(ctx); err != nil {
		logger.Error(component, "Server: Failed to warm up Soroush session", "error", err)
		return
	}

	router := soroushlib.NewMessageRouter(session)
	go func() {
		if err := router.Run(ctx); err != nil && ctx.Err() == nil {
			logger.Error(component, "Server MessageRouter error", "error", err)
		}
	}()

	msgCh := router.SubscribeText()
	defer router.UnsubscribeText(msgCh)

	logger.Info(component, "Server SDP Listening Engine is active, polling for incoming offers...")

	for {
		select {
		case <-ctx.Done():
			logger.Info(component, "Server engine shutting down")
			return
		case msg, ok := <-msgCh:
			if !ok {
				return
			}
			// Filter out standard group/channels and only process DMs
			if msg.IsGroup {
				continue
			}
			if !strings.HasPrefix(msg.Text, "OFFER:") {
				continue
			}

			// Handle the incoming offer in a separate goroutine
			go handleIncomingOffer(ctx, session, router, cfg, msg)
		}
	}
}

func handleIncomingOffer(ctx context.Context, session *soroushlib.MTProtoSession, router *soroushlib.MessageRouter, cfg *models.SoroushTunnelConfig, msg soroushlib.IncomingMessage) {
	logger.Info(component, "Server: Received SDP Offer", "from_user", msg.FromUserID)

	// 1. Decrypt Offer
	ciphertext := strings.TrimPrefix(msg.Text, "OFFER:")
	decrypted := DecryptSDP(cfg.PairingPIN, ciphertext)
	if len(decrypted) == 0 {
		logger.Error(component, "Server: Failed to decrypt SDP Offer")
		return
	}

	var offer webrtc.SessionDescription
	if err := json.Unmarshal(decrypted, &offer); err != nil {
		logger.Error(component, "Server: Failed to unmarshal SDP Offer JSON", "error", err)
		return
	}

	// 2. Create PeerConnection
	s := webrtc.SettingEngine{}
	s.DetachDataChannels()
	api := webrtc.NewAPI(webrtc.WithSettingEngine(s))

	pc, err := api.NewPeerConnection(webrtc.Configuration{
		ICEServers: []webrtc.ICEServer{
			{
				URLs: []string{"stun:stun.l.google.com:19302"},
			},
		},
	})
	if err != nil {
		logger.Error(component, "Server: Failed to create PeerConnection", "error", err)
		return
	}

	pc.OnDataChannel(func(dc *webrtc.DataChannel) {
		if dc.Label() != "clever-tunnel" {
			dc.Close()
			return
		}

		logger.Info(component, "Server: DataChannel opened, waiting to detach")

		dc.OnOpen(func() {
			logger.Info(component, "Server: DataChannel OnOpen fired, detaching")
			raw, err := dc.Detach()
			if err != nil {
				logger.Error(component, "Server: Failed to detach DataChannel", "error", err)
				return
			}

			conn := &DataChannelConn{
				ReadWriteCloser: raw,
				localAddr:       &WebRTCAddr{network: "webrtc", address: "server"},
				remoteAddr:      &WebRTCAddr{network: "webrtc", address: fmt.Sprintf("user-%d", msg.FromUserID)},
			}

			// 3-second zero-trust handshake
			challenge := make([]byte, 64)
			errChan := make(chan error, 1)
			go func() {
				_, err := io.ReadFull(conn, challenge)
				errChan <- err
			}()

			select {
			case <-time.After(3 * time.Second):
				logger.Warn(component, "Server: Handshake timeout (3s deadline), closing connection")
				conn.Close()
				return
			case err := <-errChan:
				if err != nil {
					logger.Warn(component, "Server: Handshake read error, closing connection", "error", err)
					conn.Close()
					return
				}
			}

			if err := VerifyHandshakeChallenge(cfg.PSK, challenge); err != nil {
				logger.Warn(component, "Server: Handshake challenge verification failed", "error", err)
				conn.Close()
				return
			}

			logger.Info(component, "Server: Handshake verified, starting Yamux server")

			yamuxCfg := yamux.DefaultConfig()
			yamuxCfg.LogOutput = nil
			// Defense: Configure strict window and write buffer sizes to prevent stream drops under high speed P2P DataChannels.
			yamuxCfg.MaxStreamWindowSize = 1024 * 1024       // 1MB Stream window
			yamuxCfg.ConnectionWriteTimeout = 5 * time.Second // Strict write deadline
			yamuxCfg.AcceptBacklog = 1024

			yamuxSess, err := yamux.Server(conn, yamuxCfg)
			if err != nil {
				logger.Error(component, "Server: Failed to create Yamux server", "error", err)
				conn.Close()
				return
			}

			StartRelayHandler(ctx, yamuxSess)
		})
	})

	if err := pc.SetRemoteDescription(offer); err != nil {
		logger.Error(component, "Server: Failed to set remote description", "error", err)
		pc.Close()
		return
	}

	answer, err := pc.CreateAnswer(nil)
	if err != nil {
		logger.Error(component, "Server: Failed to create SDP Answer", "error", err)
		pc.Close()
		return
	}

	if err := pc.SetLocalDescription(answer); err != nil {
		logger.Error(component, "Server: Failed to set local description", "error", err)
		pc.Close()
		return
	}

	gatherComplete := webrtc.GatheringCompletePromise(pc)
	select {
	case <-gatherComplete:
	case <-time.After(10 * time.Second):
		logger.Warn(component, "Server: ICE gathering timeout")
	case <-ctx.Done():
		pc.Close()
		return
	}

	answerSDP, err := json.Marshal(pc.LocalDescription())
	if err != nil {
		logger.Error(component, "Server: Failed to marshal local answer SDP", "error", err)
		pc.Close()
		return
	}

	encryptedAnswer := EncryptSDP(cfg.PairingPIN, answerSDP)
	if encryptedAnswer == "" {
		logger.Error(component, "Server: Failed to encrypt Answer SDP")
		pc.Close()
		return
	}

	accessHash := router.GetUserAccessHash(msg.FromUserID)
	err = soroushlib.SendTextMessage(ctx, session, msg.FromUserID, accessHash, "ANSWER:"+encryptedAnswer)
	if err != nil {
		logger.Error(component, "Server: Failed to send ANSWER message via Soroush", "error", err)
		pc.Close()
		return
	}

	logger.Info(component, "Server: Answer successfully sent back to client")
}

// runClient starts the client-side (Swarm) engine.
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

type SoroushSyncPayload struct {
	ServerPhoneNumber string `json:"server_phone_number"`
	PairingPIN        string `json:"pairing_pin"`
	PSK               string `json:"psk"`
	SocksPort         int    `json:"socks_port"`
	MaxWorkers        int    `json:"max_workers"`
	LoadBalanceAlgo   string `json:"load_balance_algo"`
}

// runWorker manages a single worker connection lifecycle with auto-reconnect.
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

		logger.Info(component, "Worker starting", "phone", maskPhone(acct.PhoneNumber))
		db.DB.Model(acct).Update("status", "connecting")

		jitter := time.Duration(2000+rand.Intn(2000)) * time.Millisecond
		sleepWithContext(ctx, jitter)

		// Load the latest config from database on every connect/reconnect loop iteration
		var latestCfg models.SoroushTunnelConfig
		if err := db.DB.First(&latestCfg).Error; err == nil {
			cfg = &latestCfg
		}

		if cfg.ServerPhoneNumber == "" {
			logger.Error(component, "Worker: ServerPhoneNumber is missing, cannot start client worker")
			db.DB.Model(acct).Update("status", "error")
			sleepWithContext(ctx, 10*time.Second)
			continue
		}

		// Connect to Soroush MTProto
		session, transport := soroushlib.RestoreSession(acct.AuthKey, acct.AuthKeyID, acct.ServerSalt)
		if err := transport.Connect(ctx); err != nil {
			logger.Error(component, "Worker: Failed to connect to Soroush", "phone", maskPhone(acct.PhoneNumber), "error", err)
			db.DB.Model(acct).Update("status", "error")
			sleepWithContext(ctx, 10*time.Second)
			continue
		}

		if err := session.WarmUpSession(ctx); err != nil {
			logger.Error(component, "Worker: Failed to warm up session", "phone", maskPhone(acct.PhoneNumber), "error", err)
			transport.Disconnect()
			db.DB.Model(acct).Update("status", "error")
			sleepWithContext(ctx, 10*time.Second)
			continue
		}

		// Resolve Server's phone number
		serverUserID, serverAccessHash, err := session.ResolvePhone(ctx, cfg.ServerPhoneNumber)
		if err != nil {
			logger.Error(component, "Worker: Failed to resolve server phone number", "phone", maskPhone(acct.PhoneNumber), "error", err)
			transport.Disconnect()
			db.DB.Model(acct).Update("status", "error")
			sleepWithContext(ctx, 10*time.Second)
			continue
		}

		logger.Info(component, "Worker resolved server phone successfully", "user_id", serverUserID, "access_hash", serverAccessHash)

		// Defense: Query history for any bootstrapped config to avoid spamming Soroush history gateways on application restarts.
		// Only perform this if we don't have a fully established PSK or PairingPIN yet.
		if cfg.PSK == "" || cfg.PairingPIN == "" {
			logger.Info(component, "Worker: Querying recent history for bootstrapped config payload", "phone", maskPhone(acct.PhoneNumber))
			if msgs, err := session.FetchHistory(ctx, serverUserID, serverAccessHash, 5); err == nil {
				for _, m := range msgs {
					if m.FromUserID == serverUserID && strings.HasPrefix(m.Text, "CONFIG:") {
						ciphertext := strings.TrimPrefix(m.Text, "CONFIG:")
						pin := cfg.PairingPIN
						if pin == "" {
							pin = "123456" // Default pairing pin fallback
						}
						decrypted := DecryptSDP(pin, ciphertext)
						if len(decrypted) > 0 {
							var syncPayload SoroushSyncPayload
							if err := json.Unmarshal(decrypted, &syncPayload); err == nil {
								var localCfg models.SoroushTunnelConfig
								if err := db.DB.First(&localCfg).Error; err == nil {
									localCfg.ServerPhoneNumber = syncPayload.ServerPhoneNumber
									localCfg.PairingPIN = syncPayload.PairingPIN
									localCfg.PSK = syncPayload.PSK
									if syncPayload.SocksPort > 0 {
										localCfg.SocksPort = syncPayload.SocksPort
									}
									if syncPayload.MaxWorkers > 0 {
										localCfg.MaxWorkers = syncPayload.MaxWorkers
									}
									if syncPayload.LoadBalanceAlgo != "" {
										localCfg.LoadBalanceAlgo = syncPayload.LoadBalanceAlgo
									}
									db.DB.Save(&localCfg)
									cfg = &localCfg
									logger.Info(component, "Worker: Bootstrapped configuration successfully from chat history")
									break
								}
							}
						}
					}
				}
			}
		}

		// Create WebRTC PeerConnection
		s := webrtc.SettingEngine{}
		s.DetachDataChannels()
		api := webrtc.NewAPI(webrtc.WithSettingEngine(s))

		pc, err := api.NewPeerConnection(webrtc.Configuration{
			ICEServers: []webrtc.ICEServer{
				{
					URLs: []string{"stun:stun.l.google.com:19302"},
				},
			},
		})
		if err != nil {
			logger.Error(component, "Worker: Failed to create PeerConnection", "error", err)
			transport.Disconnect()
			db.DB.Model(acct).Update("status", "error")
			sleepWithContext(ctx, 10*time.Second)
			continue
		}

		dc, err := pc.CreateDataChannel("clever-tunnel", nil)
		if err != nil {
			logger.Error(component, "Worker: Failed to create DataChannel", "error", err)
			pc.Close()
			transport.Disconnect()
			db.DB.Model(acct).Update("status", "error")
			sleepWithContext(ctx, 10*time.Second)
			continue
		}

		clientTransport := NewWebRTCTransport()
		clientTransport.pc = pc
		clientTransport.dc = dc

		var yamuxSess *yamux.Session
		var yamuxError error
		yamuxEstablished := make(chan struct{})

		dc.OnOpen(func() {
			logger.Info(component, "Worker: DataChannel opened, detaching")
			raw, err := dc.Detach()
			if err != nil {
				logger.Error(component, "Worker: Failed to detach DataChannel", "error", err)
				pc.Close()
				return
			}

			conn := &DataChannelConn{
				ReadWriteCloser: raw,
				localAddr:       &WebRTCAddr{network: "webrtc", address: "client"},
				remoteAddr:      &WebRTCAddr{network: "webrtc", address: "server"},
			}

			// Send 64-byte HKDF challenge
			challenge, err := BuildHandshakeChallenge(cfg.PSK)
			if err != nil {
				logger.Error(component, "Worker: Failed to build handshake challenge", "error", err)
				conn.Close()
				pc.Close()
				return
			}

			if _, err := conn.Write(challenge); err != nil {
				logger.Error(component, "Worker: Failed to write handshake challenge", "error", err)
				conn.Close()
				pc.Close()
				return
			}

			logger.Info(component, "Worker: Handshake challenge sent, starting yamux client")

			yamuxCfg := yamux.DefaultConfig()
			yamuxCfg.LogOutput = nil
			// Defense: Configure strict window and write buffer sizes to prevent stream drops under high speed P2P DataChannels.
			yamuxCfg.MaxStreamWindowSize = 1024 * 1024       // 1MB Stream window
			yamuxCfg.ConnectionWriteTimeout = 5 * time.Second // Strict write deadline
			yamuxCfg.AcceptBacklog = 1024

			yamuxSess, yamuxError = yamux.Client(conn, yamuxCfg)
			if yamuxError != nil {
				logger.Error(component, "Worker: Failed to create Yamux client", "error", yamuxError)
				conn.Close()
				pc.Close()
				return
			}

			clientTransport.rawConn = conn
			clientTransport.yamuxSes = yamuxSess
			close(yamuxEstablished)
		})

		// Create Offer
		offer, err := pc.CreateOffer(nil)
		if err != nil {
			logger.Error(component, "Worker: Failed to create Offer", "error", err)
			clientTransport.Close()
			transport.Disconnect()
			continue
		}

		if err := pc.SetLocalDescription(offer); err != nil {
			logger.Error(component, "Worker: Failed to set local description", "error", err)
			clientTransport.Close()
			transport.Disconnect()
			continue
		}

		// Wait for ICE gathering
		gatherComplete := webrtc.GatheringCompletePromise(pc)
		select {
		case <-gatherComplete:
		case <-time.After(10 * time.Second):
			logger.Warn(component, "Worker: ICE gathering timeout")
		case <-ctx.Done():
			clientTransport.Close()
			transport.Disconnect()
			return
		}

		// Encrypt and send OFFER
		offerSDP, err := json.Marshal(pc.LocalDescription())
		if err != nil {
			logger.Error(component, "Worker: Failed to marshal offer SDP", "error", err)
			clientTransport.Close()
			transport.Disconnect()
			continue
		}

		encryptedOffer := EncryptSDP(cfg.PairingPIN, offerSDP)
		if err := soroushlib.SendTextMessage(ctx, session, serverUserID, serverAccessHash, "OFFER:"+encryptedOffer); err != nil {
			logger.Error(component, "Worker: Failed to send OFFER message", "error", err)
			clientTransport.Close()
			transport.Disconnect()
			continue
		}

		logger.Info(component, "Worker: OFFER sent, starting message router to wait for ANSWER")

		router := soroushlib.NewMessageRouter(session)
		go func() {
			if err := router.Run(ctx); err != nil && ctx.Err() == nil {
				logger.Error(component, "Worker MessageRouter error", "error", err)
			}
		}()

		msgCh := router.SubscribeText()

		// Await ANSWER
		var answerStr string
		timeout := time.After(30 * time.Second)
		cancelled := false

	waitLoop:
		for {
			select {
			case <-ctx.Done():
				cancelled = true
				break waitLoop
			case <-timeout:
				logger.Error(component, "Worker: Timeout waiting for ANSWER from server")
				break waitLoop
			case msg, ok := <-msgCh:
				if !ok {
					break waitLoop
				}
				if msg.FromUserID == serverUserID && !msg.IsGroup {
					if strings.HasPrefix(msg.Text, "CONFIG:") {
						ciphertext := strings.TrimPrefix(msg.Text, "CONFIG:")
						decrypted := DecryptSDP(cfg.PairingPIN, ciphertext)
						if len(decrypted) > 0 {
							var syncPayload SoroushSyncPayload
							if err := json.Unmarshal(decrypted, &syncPayload); err == nil {
								var localCfg models.SoroushTunnelConfig
								if err := db.DB.First(&localCfg).Error; err == nil {
									localCfg.ServerPhoneNumber = syncPayload.ServerPhoneNumber
									localCfg.PairingPIN = syncPayload.PairingPIN
									localCfg.PSK = syncPayload.PSK
									if syncPayload.SocksPort > 0 {
										localCfg.SocksPort = syncPayload.SocksPort
									}
									if syncPayload.MaxWorkers > 0 {
										localCfg.MaxWorkers = syncPayload.MaxWorkers
									}
									if syncPayload.LoadBalanceAlgo != "" {
										localCfg.LoadBalanceAlgo = syncPayload.LoadBalanceAlgo
									}
									db.DB.Save(&localCfg)
									cfg = &localCfg
									logger.Info(component, "Worker: Successfully parsed and cached configuration from CONFIG: update")
								}
							}
						}
					} else if strings.HasPrefix(msg.Text, "ANSWER:") {
						answerStr = strings.TrimPrefix(msg.Text, "ANSWER:")
						break waitLoop
					}
				}
			}
		}

		router.UnsubscribeText(msgCh)

		if cancelled {
			clientTransport.Close()
			transport.Disconnect()
			return
		}

		if answerStr == "" {
			logger.Warn(component, "Worker: Timeout waiting for ANSWER, running Solution 2 fallback (chat history lookup)")
			if msgs, err := session.FetchHistory(ctx, serverUserID, serverAccessHash, 5); err == nil {
				for _, m := range msgs {
					if m.FromUserID == serverUserID && !m.IsGroup && strings.HasPrefix(m.Text, "ANSWER:") {
						answerStr = strings.TrimPrefix(m.Text, "ANSWER:")
						logger.Info(component, "Worker: Found ANSWER in chat history via Solution 2 fallback")
						break
					}
				}
			}
		}

		if answerStr == "" {
			logger.Error(component, "Worker: Did not receive valid ANSWER")
			clientTransport.Close()
			transport.Disconnect()
			db.DB.Model(acct).Update("status", "error")
			sleepWithContext(ctx, 10*time.Second)
			continue
		}

		// Decrypt and apply ANSWER description
		decryptedAnswer := DecryptSDP(cfg.PairingPIN, answerStr)
		if len(decryptedAnswer) == 0 {
			logger.Error(component, "Worker: Failed to decrypt ANSWER SDP")
			clientTransport.Close()
			transport.Disconnect()
			db.DB.Model(acct).Update("status", "error")
			sleepWithContext(ctx, 10*time.Second)
			continue
		}

		var answer webrtc.SessionDescription
		if err := json.Unmarshal(decryptedAnswer, &answer); err != nil {
			logger.Error(component, "Worker: Failed to unmarshal ANSWER SDP JSON", "error", err)
			clientTransport.Close()
			transport.Disconnect()
			db.DB.Model(acct).Update("status", "error")
			sleepWithContext(ctx, 10*time.Second)
			continue
		}

		if err := pc.SetRemoteDescription(answer); err != nil {
			logger.Error(component, "Worker: Failed to set remote description", "error", err)
			clientTransport.Close()
			transport.Disconnect()
			db.DB.Model(acct).Update("status", "error")
			sleepWithContext(ctx, 10*time.Second)
			continue
		}

		logger.Info(component, "Worker: Applied remote description (ANSWER), waiting for DataChannel connection and Yamux session")

		// Wait for yamux to be established
		select {
		case <-ctx.Done():
			clientTransport.Close()
			transport.Disconnect()
			return
		case <-time.After(15 * time.Second):
			logger.Error(component, "Worker: Timeout waiting for DataChannel/Yamux session to establish")
			clientTransport.Close()
			transport.Disconnect()
			db.DB.Model(acct).Update("status", "error")
			sleepWithContext(ctx, 10*time.Second)
			continue
		case <-yamuxEstablished:
			if yamuxError != nil {
				logger.Error(component, "Worker: Yamux establishment error", "error", yamuxError)
				clientTransport.Close()
				transport.Disconnect()
				db.DB.Model(acct).Update("status", "error")
				sleepWithContext(ctx, 10*time.Second)
				continue
			}
		}

		wc := &WorkerChannel{
			AccountID:    fmt.Sprintf("%d", acct.ID),
			Transport:    clientTransport,
			YamuxSession: yamuxSess,
			Healthy:      true,
		}
		pool.Inject(wc)

		logger.Info(component, "Worker connected and injected into pool", "phone", maskPhone(acct.PhoneNumber))
		db.DB.Model(acct).Update("status", "tunnel_active")

		// Block until yamux session closes or context cancels
		select {
		case <-ctx.Done():
			pool.Purge(wc.AccountID)
			clientTransport.Close()
			transport.Disconnect()
			return
		case <-yamuxSess.CloseChan():
			logger.Warn(component, "Worker yamux session closed, reconnecting", "phone", maskPhone(acct.PhoneNumber))
			db.DB.Model(acct).Update("status", "error")
			pool.Purge(wc.AccountID)
			clientTransport.Close()
			transport.Disconnect()
			sleepWithContext(ctx, 5*time.Second)
		}
	}
}

// StopEngine gracefully stops the Soroush tunnel engine.
func StopEngine() {
	engineMu.Lock()
	runningVal := running
	engineMu.Unlock()

	if !runningVal {
		return
	}

	logger.Info(component, "Stopping Soroush tunnel engine")
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
	TotalStreams int64     `json:"total_streams"`
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
		TotalStreams: totalStreams.Load(),
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
