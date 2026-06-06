// Package soroush implements the Soroush SFU RTP-based QUIC tunnel engine.
// It operates as an additive, parallel service to the existing Ehco infrastructure,
// routing traffic securely through LiveKit WebRTC Audio Tracks disguised as voice calls.
//
// Architecture: QUIC → RtpPacketConn → LiveKit Audio Track (Opus) → SFU → Remote
package soroush

import (
	"context"
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/tls"
	"crypto/x509"
	"encoding/pem"
	"fmt"
	"math/big"
	mrand "math/rand"
	"sync"
	"sync/atomic"
	"time"

	"clever-connect/internal/db"
	"clever-connect/internal/logger"
	"clever-connect/internal/models"

	lksdk "github.com/livekit/server-sdk-go/v2"
	livekit "github.com/livekit/protocol/livekit"
	"github.com/pion/webrtc/v4"
	"github.com/quic-go/quic-go"
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
		return fmt.Errorf("PSK is required for QUIC TLS authentication")
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

	logger.Info(component, "Starting Soroush LiveKit QUIC engine",
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

// ──────────────────────────────────────────────────────────────────────────────
// Server Engine
// ──────────────────────────────────────────────────────────────────────────────

// runServer starts the server-side (Queen) engine bound to the LiveKit Room.
// It publishes a fake audio track and accepts QUIC sessions from workers.
func runServer(ctx context.Context, cfg *models.SoroushTunnelConfig, hostAccount *models.SoroushAccount) {
	logger.Info(component, "Server engine goroutine started, initializing RTP+QUIC listener")

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

		// 1. Create the fake Audio Track
		localTrack, err := webrtc.NewTrackLocalStaticSample(webrtc.RTPCodecCapability{
			MimeType:  webrtc.MimeTypeOpus,
			ClockRate: 48000,
			Channels:  2,
		}, "tunnel-quic", "tunnel")
		if err != nil {
			logger.Error(component, "Server: Failed to create local audio track", "error", err)
			sleepWithContext(ctx, 5*time.Second)
			continue
		}

		packetConn := NewRtpPacketConn(localTrack)

		// 2. Setup LiveKit Callbacks to intercept incoming audio tracks
		roomCb := lksdk.NewRoomCallback()
		roomCb.OnTrackSubscribed = func(track *webrtc.TrackRemote, pub *lksdk.RemoteTrackPublication, rp *lksdk.RemoteParticipant) {
			if track.Kind() == webrtc.RTPCodecTypeAudio {
				go func() {
					for {
						rtpPacket, _, err := track.ReadRTP()
						if err != nil {
							return
						}
						packetConn.PushRx(rtpPacket.Payload)
					}
				}()
			}
		}

		// 3. Connect to the Room using the auto-generated JWT
		room, err := lksdk.ConnectToRoomWithToken(url, hostAccount.LiveKitToken, roomCb)
		if err != nil {
			logger.Error(component, "Server: Failed to connect to LiveKit Room", "error", err)
			packetConn.Close()
			sleepWithContext(ctx, 10*time.Second)
			continue
		}

		// 4. Publish the fake Microphone
		_, err = room.LocalParticipant.PublishTrack(localTrack, &lksdk.TrackPublicationOptions{
			Source: livekit.TrackSource_MICROPHONE,
		})
		if err != nil {
			logger.Error(component, "Server: Failed to publish audio track", "error", err)
			room.Disconnect()
			packetConn.Close()
			sleepWithContext(ctx, 5*time.Second)
			continue
		}

		logger.Info(component, "Server: Connected to SFU. Audio track published, starting QUIC listener...",
			"local_identity", room.LocalParticipant.Identity(),
		)

		// 5. Start QUIC over the Audio Track (server mode)
		quicErr := runQuicServer(ctx, packetConn)

		// Cleanup
		room.Disconnect()
		packetConn.Close()

		if quicErr != nil {
			logger.Warn(component, "Server: QUIC session ended, reconnecting in 5s...", "error", quicErr)
			sleepWithContext(ctx, 5*time.Second)
		}
	}
}

// runQuicServer starts a QUIC listener on the given PacketConn and
// accepts streams from the connected client.
func runQuicServer(ctx context.Context, conn *RtpPacketConn) error {
	tlsCert, err := generateSelfSignedCert()
	if err != nil {
		return fmt.Errorf("generate TLS cert: %w", err)
	}

	tlsConf := &tls.Config{
		Certificates: []tls.Certificate{tlsCert},
		NextProtos:   []string{"clever-connect"},
	}

	// CRITICAL: Disable MTU Discovery so QUIC pads packets to exactly 1200 bytes.
	// This fits perfectly inside the 1275-byte Opus limit on LiveKit without fragmentation.
	quicConf := &quic.Config{
		DisablePathMTUDiscovery: true,
		KeepAlivePeriod:         time.Second * 15,
	}

	listener, err := quic.Listen(conn, tlsConf, quicConf)
	if err != nil {
		return fmt.Errorf("quic listen: %w", err)
	}
	defer listener.Close()

	logger.Info(component, "Server: QUIC listener active, awaiting client sessions")

	for {
		session, err := listener.Accept(ctx)
		if err != nil {
			select {
			case <-ctx.Done():
				return ctx.Err()
			default:
				return fmt.Errorf("quic accept: %w", err)
			}
		}

		logger.Info(component, "Server: QUIC session established from worker")

		go func(sess *quic.Conn) {
			for {
				stream, err := sess.AcceptStream(ctx)
				if err != nil {
					logger.Debug(component, "Server: QUIC stream accept ended", "error", err)
					return
				}
				go HandleServerRelay(stream)
			}
		}(session)
	}
}

// ──────────────────────────────────────────────────────────────────────────────
// Client Engine
// ──────────────────────────────────────────────────────────────────────────────

// runClient starts the client-side (Swarm) engine.
func runClient(ctx context.Context, cfg *models.SoroushTunnelConfig, accounts []models.SoroushAccount) {
	logger.Info(component, "Client engine goroutine started")

	maxWorkers := cfg.MaxWorkers
	if maxWorkers > len(accounts) {
		maxWorkers = len(accounts)
	}

	var wg sync.WaitGroup
	for i := 0; i < maxWorkers; i++ {
		wg.Add(1)
		go func(acct models.SoroushAccount) {
			defer wg.Done()
			runWorker(ctx, cfg, &acct)
		}(accounts[i])
	}

	wg.Wait()
	logger.Info(component, "Client engine shutting down — all workers exited")
}

// runWorker manages a single worker connection to the SFU Room.
// Each worker uses its own per-account LiveKitToken to avoid identity collisions.
func runWorker(ctx context.Context, cfg *models.SoroushTunnelConfig, acct *models.SoroushAccount) {
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

		jitter := time.Duration(1000+mrand.Intn(2000)) * time.Millisecond
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

		// 1. Create the fake Audio Track
		localTrack, err := webrtc.NewTrackLocalStaticSample(webrtc.RTPCodecCapability{
			MimeType:  webrtc.MimeTypeOpus,
			ClockRate: 48000,
			Channels:  2,
		}, "tunnel-quic", "tunnel")
		if err != nil {
			logger.Error(component, "Worker: Failed to create local audio track", "error", err)
			db.DB.Model(acct).Update("status", "error")
			sleepWithContext(ctx, 5*time.Second)
			continue
		}

		packetConn := NewRtpPacketConn(localTrack)

		// 2. Setup LiveKit Callbacks to intercept incoming audio tracks
		roomCb := lksdk.NewRoomCallback()
		roomCb.OnTrackSubscribed = func(track *webrtc.TrackRemote, pub *lksdk.RemoteTrackPublication, rp *lksdk.RemoteParticipant) {
			if track.Kind() == webrtc.RTPCodecTypeAudio {
				go func() {
					for {
						rtpPacket, _, err := track.ReadRTP()
						if err != nil {
							return
						}
						packetConn.PushRx(rtpPacket.Payload)
					}
				}()
			}
		}

		// 3. Connect to the Room using the per-account JWT (unique identity)
		room, err := lksdk.ConnectToRoomWithToken(url, acct.LiveKitToken, roomCb)
		if err != nil {
			logger.Error(component, "Worker: Failed to connect to SFU Room", "error", err)
			db.DB.Model(acct).Update("status", "error")
			packetConn.Close()
			sleepWithContext(ctx, 10*time.Second)
			continue
		}

		// 4. Publish the fake Microphone
		_, err = room.LocalParticipant.PublishTrack(localTrack, &lksdk.TrackPublicationOptions{
			Source: livekit.TrackSource_MICROPHONE,
		})
		if err != nil {
			logger.Error(component, "Worker: Failed to publish audio track", "error", err)
			room.Disconnect()
			packetConn.Close()
			db.DB.Model(acct).Update("status", "error")
			sleepWithContext(ctx, 5*time.Second)
			continue
		}

		logger.Info(component, "Worker: SFU connection established, starting QUIC client", "phone", maskPhone(acct.PhoneNumber))

		// 5. Start QUIC client over the Audio Track
		quicErr := runQuicClient(ctx, packetConn, cfg)

		// Cleanup
		room.Disconnect()
		packetConn.Close()

		if quicErr != nil {
			logger.Warn(component, "Worker: QUIC session ended, reconnecting in 3s...",
				"phone", maskPhone(acct.PhoneNumber), "error", quicErr,
			)
			db.DB.Model(acct).Update("status", "error")
			sleepWithContext(ctx, 3*time.Second)
		}
	}
}

// runQuicClient dials a QUIC connection over the RTP PacketConn and
// starts the local SOCKS5 listener that proxies through it.
func runQuicClient(ctx context.Context, conn *RtpPacketConn, cfg *models.SoroushTunnelConfig) error {
	tlsConf := &tls.Config{
		InsecureSkipVerify: true,
		NextProtos:         []string{"clever-connect"},
	}

	// CRITICAL: Disable MTU Discovery so QUIC pads packets to exactly 1200 bytes.
	quicConf := &quic.Config{
		DisablePathMTUDiscovery: true,
		KeepAlivePeriod:         time.Second * 15,
	}

	session, err := quic.Dial(ctx, conn, fakeAddr, tlsConf, quicConf)
	if err != nil {
		return fmt.Errorf("quic dial: %w", err)
	}

	logger.Info(component, "Client: QUIC session established with server")
	db.DB.Model(&models.SoroushAccount{}).Where("status = ?", "connecting").Update("status", "tunnel_active")

	// Start SOCKS5 proxy that uses this QUIC session
	socksErr := StartSOCKS5Listener(ctx, cfg.SocksPort, session)
	return socksErr
}

// ──────────────────────────────────────────────────────────────────────────────
// Engine Control
// ──────────────────────────────────────────────────────────────────────────────

// StopEngine gracefully stops the tunnel engine.
func StopEngine() {
	engineMu.Lock()
	runningVal := running
	engineMu.Unlock()

	if !runningVal {
		return
	}

	logger.Info(component, "Stopping LiveKit Soroush QUIC tunnel engine")
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
	Running      bool   `json:"running"`
	Mode         string `json:"mode"`
	TotalStreams  int64  `json:"total_streams"`
	BytesRelayed int64  `json:"bytes_relayed"`
	Uptime       string `json:"uptime"`
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

// ──────────────────────────────────────────────────────────────────────────────
// Helpers
// ──────────────────────────────────────────────────────────────────────────────

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

// generateSelfSignedCert creates a self-signed TLS certificate for the QUIC server.
// Since both sides are authenticated via the Soroush PSK handshake, the client
// uses InsecureSkipVerify and this cert serves only as the TLS transport key.
func generateSelfSignedCert() (tls.Certificate, error) {
	key, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	if err != nil {
		return tls.Certificate{}, err
	}

	template := x509.Certificate{
		SerialNumber: big.NewInt(1),
		NotBefore:    time.Now(),
		NotAfter:     time.Now().Add(365 * 24 * time.Hour),
	}

	certDER, err := x509.CreateCertificate(rand.Reader, &template, &template, &key.PublicKey, key)
	if err != nil {
		return tls.Certificate{}, err
	}

	keyBytes, err := x509.MarshalECPrivateKey(key)
	if err != nil {
		return tls.Certificate{}, err
	}

	certPEM := pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: certDER})
	keyPEM := pem.EncodeToMemory(&pem.Block{Type: "EC PRIVATE KEY", Bytes: keyBytes})

	return tls.X509KeyPair(certPEM, keyPEM)
}
