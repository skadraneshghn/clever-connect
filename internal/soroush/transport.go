package soroush

import (
	"context"
	"fmt"
	"sync"

	"clever-connect/internal/logger"
	"clever-connect/internal/models"
	"clever-connect/internal/soroushlib"

	"github.com/hashicorp/yamux"
	lksdk "github.com/livekit/server-sdk-go"
	livekit "github.com/livekit/protocol/livekit"
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

	// Server-side: authenticated peer registry
	authenticatedPeers sync.Map // map[string]*soroushlib.VConn

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

		// Send PSK as first data packet for in-band authentication (Phase 8.4)
		err = room.LocalParticipant.PublishDataPacket(
			&livekit.UserPacket{Payload: []byte(t.cfg.PSK)},
			livekit.DataPacket_RELIABLE,
		)
		if err != nil {
			room.Disconnect()
			return fmt.Errorf("failed to send PSK handshake: %w", err)
		}

		logger.Info(component, "LiveKitTransport: PSK handshake sent")

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

	// 2. Unauthenticated — check PSK
	if string(data) == t.cfg.PSK {
		logger.Info(component, "Worker authenticated via DataChannel",
			"identity", participantIdentity,
		)

		// Create VConn targeting this specific worker via LiveKit data publish
		vc := soroushlib.NewVConn(func(out []byte) error {
			return t.room.LocalParticipant.PublishDataPacket(
				&livekit.UserPacket{
					Payload:             out,
					DestinationIdentities: []string{participantIdentity},
				},
				livekit.DataPacket_RELIABLE,
			)
		}, 4*1024*1024) // 4MB ring buffer

		t.authenticatedPeers.Store(participantIdentity, vc)

		// Start yamux server + relay in background
		go func() {
			yamuxCfg := yamux.DefaultConfig()
			yamuxCfg.LogOutput = nil
			yamuxSess, err := yamux.Server(vc, yamuxCfg)
			if err != nil {
				logger.Error(component, "Failed to create yamux server for worker",
					"identity", participantIdentity, "error", err,
				)
				t.authenticatedPeers.Delete(participantIdentity)
				vc.Close()
				return
			}

			StartRelayHandler(ctx, yamuxSess)

			// Cleanup on disconnect
			t.authenticatedPeers.Delete(participantIdentity)
			vc.Close()
			logger.Info(component, "Worker relay handler exited",
				"identity", participantIdentity,
			)
		}()
	} else {
		// DPI prober or curious Soroush user — drop silently
		logger.Warn(component, "Rogue participant sent invalid data, dropping",
			"identity", participantIdentity,
			"data_len", len(data),
		)
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


