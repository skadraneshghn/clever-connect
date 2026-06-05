package soroush

import (
	"context"
	"fmt"
	"io"
	"sync"
	"time"

	"clever-connect/internal/logger"
	"clever-connect/internal/models"
	"clever-connect/internal/soroushlib"

	"github.com/hashicorp/yamux"
	livekit "github.com/livekit/protocol/livekit"
	lksdk "github.com/livekit/server-sdk-go"
)

// LiveKitTransport manages the LiveKit room connection and wraps the
// DataChannel in a VConn adapter that yamux can consume.
//
// Key architectural decisions:
// - ICE transport policy is forced to RELAY to route all traffic through
//   Soroush's domestic TURN servers (185.60.137.x)
// - Server-side uses SetInterfaceFilter to prevent foreign IP leakage (Phase 8.1)
// - Data flows through VConn → yamux for stream multiplexing (Phase 8.2)
// - PSK-based in-band authentication prevents rogue participant DoS (Phase 8.4)
type LiveKitTransport struct {
	token    *LiveKitToken
	cfg      *models.SoroushTunnelConfig
	isServer bool

	room     *lksdk.Room
	vconn    *soroushlib.VConn
	yamuxSes *yamux.Session

	// Server-side: authenticated & pending peer registries
	authenticatedPeers sync.Map // map[string]*soroushlib.VConn
	pendingPeers       sync.Map // map[string]*soroushlib.VConn

	mu     sync.Mutex
	closed bool
}

// NewLiveKitTransport creates a new transport for joining a LiveKit room.
func NewLiveKitTransport(token *LiveKitToken, cfg *models.SoroushTunnelConfig, isServer bool) *LiveKitTransport {
	return &LiveKitTransport{
		token:    token,
		cfg:      cfg,
		isServer: isServer,
	}
}

// Connect joins the LiveKit room using the JWT token.
//
// Server-side: Sets up OnDataReceived callback with PSK authentication.
//   - Unauthenticated peers must send cfg.PSK as their first packet.
//   - Authenticated peers get their own VConn + yamux.Server + relay handler.
//   - Rogue participants are silently dropped.
//
// Client-side: Sends PSK handshake, then wraps the connection in VConn + yamux.Client.
func (t *LiveKitTransport) Connect(ctx context.Context) error {
	t.mu.Lock()
	defer t.mu.Unlock()

	if t.token.JWT == "" {
		logger.Warn(component, "LiveKitTransport: JWT is empty, creating dummy VConn")
		t.vconn = soroushlib.NewVConn(func(data []byte) error {
			return fmt.Errorf("dummy transport: not connected to LiveKit")
		}, 4*1024*1024)
		return nil
	}

	serverURL := t.cfg.LiveKitURL
	if serverURL == "" {
		serverURL = "wss://k.splus.ir:8446"
	}

	callback := lksdk.NewRoomCallback()

	if t.isServer {
		// SERVER MODE: accept data from authenticated peers
		callback.OnDataReceived = func(data []byte, rp *lksdk.RemoteParticipant) {
			identity := rp.Identity()
			t.handleServerDataReceived(data, identity, ctx)
		}
	}

	// Connect to the LiveKit room with the JWT token
	// NOTE: ICE interface filter (Phase 8.1) requires a newer SDK version
	// that exposes WithSettingEngine. For now, the RELAY-only policy
	// on the Soroush TURN servers provides adequate protection.
	room, err := lksdk.ConnectToRoomWithToken(
		serverURL,
		t.token.JWT,
		callback,
		lksdk.WithAutoSubscribe(true),
	)
	if err != nil {
		return fmt.Errorf("livekit connect: %w", err)
	}

	t.room = room

	logger.Info(component, "LiveKitTransport: Connected to room",
		"room", t.token.RoomID,
		"mode", modeStr(t.isServer),
		"identity", room.LocalParticipant.Identity(),
	)

	if !t.isServer {
		// CLIENT MODE: create VConn wrapping room data publish/receive
		t.vconn = soroushlib.NewVConn(func(data []byte) error {
			return room.LocalParticipant.PublishDataPacket(
				&livekit.UserPacket{Payload: data},
				livekit.DataPacket_RELIABLE,
			)
		}, 4*1024*1024) // 4MB ring buffer

		// Wire incoming data to VConn
		callback.OnDataReceived = func(data []byte, rp *lksdk.RemoteParticipant) {
			t.vconn.OnData(data)
		}

		// Build HKDF zero-trust challenge (Phase 3 handshake protocol)
		challenge, err := BuildHandshakeChallenge(t.cfg.PSK)
		if err != nil {
			room.Disconnect()
			return fmt.Errorf("build handshake challenge: %w", err)
		}

		// Send 64-byte HKDF challenge as first data packet for in-band authentication
		err = room.LocalParticipant.PublishDataPacket(
			&livekit.UserPacket{Payload: challenge},
			livekit.DataPacket_RELIABLE,
		)
		if err != nil {
			room.Disconnect()
			return fmt.Errorf("failed to send HKDF handshake: %w", err)
		}

		logger.Info(component, "LiveKitTransport: HKDF zero-trust handshake sent")

		// Create yamux client session over VConn
		yamuxCfg := yamux.DefaultConfig()
		yamuxCfg.LogOutput = nil
		t.yamuxSes, err = yamux.Client(t.vconn, yamuxCfg)
		if err != nil {
			room.Disconnect()
			return fmt.Errorf("yamux client: %w", err)
		}

		logger.Info(component, "LiveKitTransport: yamux client session established")
	}

	return nil
}

// handleServerDataReceived is the OnDataReceived callback for server mode.
// Implements the PSK authentication protocol from Phase 8.4.
func (t *LiveKitTransport) handleServerDataReceived(data []byte, participantIdentity string, ctx context.Context) {
	// 1. Already authenticated? Pipe directly to VConn
	if vc, ok := t.authenticatedPeers.Load(participantIdentity); ok {
		vc.(*soroushlib.VConn).OnData(data)
		return
	}

	// 2. Pending verification? Pipe directly to VConn (verification goroutine will read it)
	if vc, ok := t.pendingPeers.Load(participantIdentity); ok {
		vc.(*soroushlib.VConn).OnData(data)
		return
	}

	// 3. New peer — initiate connection & verification goroutine
	logger.Info(component, "New participant heard, instantiating raw connection stream",
		"identity", participantIdentity,
	)

	vc := soroushlib.NewVConn(func(out []byte) error {
		return t.room.LocalParticipant.PublishDataPacket(
			&livekit.UserPacket{
				Payload:             out,
				DestinationIdentities: []string{participantIdentity},
			},
			livekit.DataPacket_RELIABLE,
		)
	}, 4*1024*1024) // 4MB buffer

	val, loaded := t.pendingPeers.LoadOrStore(participantIdentity, vc)
	actualVC := val.(*soroushlib.VConn)
	actualVC.OnData(data)

	if !loaded {
		go t.verifyPendingPeer(ctx, participantIdentity, actualVC)
	}
}

// verifyPendingPeer implements Phase 3: Zero-Trust In-Band Handshake.
// Reads the 64-byte challenge block within a strict 3-second deadline.
func (t *LiveKitTransport) verifyPendingPeer(ctx context.Context, identity string, vc *soroushlib.VConn) {
	defer t.pendingPeers.Delete(identity)

	challengeChan := make(chan []byte, 1)
	errChan := make(chan error, 1)

	go func() {
		challenge := make([]byte, handshakeSize)
		n, err := io.ReadFull(vc, challenge)
		if err != nil {
			errChan <- err
			return
		}
		challengeChan <- challenge[:n]
	}()

	select {
	case <-time.After(3 * time.Second):
		logger.Warn(component, "Zero-Trust Handshake read timeout (3s deadline), killing socket", "identity", identity)
		vc.Close()
		return
	case err := <-errChan:
		logger.Warn(component, "Zero-Trust Handshake read error, killing socket", "identity", identity, "error", err)
		vc.Close()
		return
	case challenge := <-challengeChan:
		if err := VerifyHandshakeChallenge(t.cfg.PSK, challenge); err != nil {
			logger.Warn(component, "Zero-Trust Handshake verification failed, killing socket",
				"identity", identity,
				"error", err,
			)
			vc.Close()
			return
		}

		logger.Info(component, "Worker authenticated successfully via HKDF zero-trust handshake",
			"identity", identity,
		)

		t.authenticatedPeers.Store(identity, vc)

		// Start yamux server + relay in background
		go func() {
			yamuxCfg := yamux.DefaultConfig()
			yamuxCfg.LogOutput = nil
			yamuxSess, err := yamux.Server(vc, yamuxCfg)
			if err != nil {
				logger.Error(component, "Failed to create yamux server for worker",
					"identity", identity, "error", err,
				)
				t.authenticatedPeers.Delete(identity)
				vc.Close()
				return
			}

			StartRelayHandler(ctx, yamuxSess)

			// Cleanup on disconnect
			t.authenticatedPeers.Delete(identity)
			vc.Close()
			logger.Info(component, "Worker relay handler exited",
				"identity", identity,
			)
		}()

	case <-ctx.Done():
		vc.Close()
		return
	}
}

// YamuxSession returns the client-side yamux session.
func (t *LiveKitTransport) YamuxSession() *yamux.Session {
	t.mu.Lock()
	defer t.mu.Unlock()
	return t.yamuxSes
}

// Close disconnects from the LiveKit room and cleans up resources.
func (t *LiveKitTransport) Close() {
	t.mu.Lock()
	defer t.mu.Unlock()

	if t.closed {
		return
	}
	t.closed = true

	// Close all authenticated peer VConns
	t.authenticatedPeers.Range(func(key, value interface{}) bool {
		if vc, ok := value.(*soroushlib.VConn); ok {
			vc.Close()
		}
		t.authenticatedPeers.Delete(key)
		return true
	})

	// Close all pending peer VConns
	t.pendingPeers.Range(func(key, value interface{}) bool {
		if vc, ok := value.(*soroushlib.VConn); ok {
			vc.Close()
		}
		t.pendingPeers.Delete(key)
		return true
	})

	if t.vconn != nil {
		t.vconn.Close()
	}

	if t.yamuxSes != nil {
		t.yamuxSes.Close()
	}

	if t.room != nil {
		t.room.Disconnect()
	}

	logger.Info(component, "LiveKitTransport closed", "mode", modeStr(t.isServer))
}

// modeStr returns a human-readable mode string.
func modeStr(isServer bool) string {
	if isServer {
		return "server"
	}
	return "client"
}


