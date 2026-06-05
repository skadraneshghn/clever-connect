package soroush

import (
	"context"
	"fmt"
	"math/rand"
	"sync"
	"time"

	"clever-connect/internal/logger"
	"clever-connect/internal/models"
	"clever-connect/internal/soroushlib"
)

// LiveKitToken holds the JWT and metadata fetched from Soroush via MTProto.
type LiveKitToken struct {
	JWT       string
	RoomID    string
	ExpiresAt time.Time
}

// TokenManager handles persistent MTProto connection and periodic JWT token
// refresh with random jitter to avoid behavioral fingerprinting (Phase 8.3 fix).
//
// Instead of ephemeral connect-fetch-disconnect cycles, it keeps the MTProto
// WebSocket alive with periodic activity noise, mimicking a real Soroush browser tab.
type TokenManager struct {
	mu           sync.Mutex
	account      *models.SoroushAccount
	cfg          *models.SoroushTunnelConfig
	session      *soroushlib.MTProtoSession
	transport    *soroushlib.ObfuscatedTransport
	currentToken *LiveKitToken
	onNewToken   func(*LiveKitToken)
	cancel       context.CancelFunc
	wg           sync.WaitGroup
}

// NewTokenManager creates a new TokenManager for the given account.
func NewTokenManager(account *models.SoroushAccount, cfg *models.SoroushTunnelConfig) *TokenManager {
	return &TokenManager{
		account: account,
		cfg:     cfg,
	}
}

// Start connects to MTProto, authenticates, fetches initial token,
// then keeps the connection alive with periodic refresh + activity noise.
func (tm *TokenManager) Start(ctx context.Context) error {
	tm.mu.Lock()
	defer tm.mu.Unlock()

	ctx, tm.cancel = context.WithCancel(ctx)

	// Restore session from saved auth credentials
	session, transport := soroushlib.RestoreSession(
		tm.account.AuthKey,
		tm.account.AuthKeyID,
		tm.account.ServerSalt,
	)
	tm.session = session
	tm.transport = transport

	// Connect the transport (WebSocket + obfuscation handshake)
	if err := transport.Connect(ctx); err != nil {
		return err
	}

	logger.Info(component, "TokenManager: MTProto connected",
		"phone", maskPhone(tm.account.PhoneNumber),
	)

	// Warm up session to prime the server salt
	if err := session.WarmUpSession(ctx); err != nil {
		transport.Disconnect()
		return err
	}

	// Fetch initial token
	token, err := tm.fetchToken(ctx)
	if err != nil {
		transport.Disconnect()
		return err
	}
	tm.currentToken = token

	logger.Info(component, "TokenManager: Initial token fetched",
		"room", token.RoomID,
		"expires", token.ExpiresAt.Format(time.RFC3339),
	)

	// Start background refresh loop with jitter
	tm.wg.Add(1)
	go tm.refreshLoop(ctx)

	// Start activity noise to blend in with real Soroush sessions (Phase 8.3 fix)
	tm.wg.Add(1)
	go tm.startActivityNoise(ctx)

	return nil
}

// CurrentToken returns the latest valid JWT token.
func (tm *TokenManager) CurrentToken() *LiveKitToken {
	tm.mu.Lock()
	defer tm.mu.Unlock()
	return tm.currentToken
}

// Stop gracefully shuts down the TokenManager.
func (tm *TokenManager) Stop() {
	if tm.cancel != nil {
		tm.cancel()
	}
	tm.wg.Wait()

	if tm.transport != nil {
		tm.transport.Disconnect()
	}

	logger.Info(component, "TokenManager stopped", "phone", maskPhone(tm.account.PhoneNumber))
}

// refreshLoop periodically fetches a new JWT token with random jitter.
// Jitter range is [TokenRefreshMinSec, TokenRefreshMaxSec] (default 7-9 minutes).
func (tm *TokenManager) refreshLoop(ctx context.Context) {
	defer tm.wg.Done()

	minSec := tm.cfg.TokenRefreshMinSec
	maxSec := tm.cfg.TokenRefreshMaxSec
	if minSec <= 0 {
		minSec = 420 // 7 minutes
	}
	if maxSec <= minSec {
		maxSec = minSec + 120 // 2 minute spread
	}

	for {
		// Random jitter between min and max seconds
		jitterSec := minSec + rand.Intn(maxSec-minSec)
		jitter := time.Duration(jitterSec) * time.Second

		logger.Info(component, "TokenManager: Next refresh scheduled",
			"seconds", jitterSec,
			"phone", maskPhone(tm.account.PhoneNumber),
		)

		select {
		case <-time.After(jitter):
			token, err := tm.fetchToken(ctx)
			if err != nil {
				logger.Warn(component, "TokenManager: Token refresh failed, will retry",
					"error", err,
					"phone", maskPhone(tm.account.PhoneNumber),
				)
				continue
			}

			tm.mu.Lock()
			tm.currentToken = token
			tm.mu.Unlock()

			logger.Info(component, "TokenManager: Token refreshed",
				"room", token.RoomID,
				"expires", token.ExpiresAt.Format(time.RFC3339),
			)

			if tm.onNewToken != nil {
				tm.onNewToken(token)
			}

		case <-ctx.Done():
			return
		}
	}
}

// startActivityNoise simulates natural Soroush user behavior by periodically
// calling getDialogs. This makes the persistent MTProto connection look like
// an idle browser tab rather than a robotic tunnel bot (Phase 8.3 fix).
func (tm *TokenManager) startActivityNoise(ctx context.Context) {
	defer tm.wg.Done()

	for {
		// Random interval: 30-90 seconds (mimics user checking chat list)
		interval := 30*time.Second + time.Duration(rand.Int63n(int64(60*time.Second)))

		select {
		case <-time.After(interval):
			// Send getDialogs wrapped in initConnection — looks like app refreshing
			body := soroushlib.BuildGetDialogsRequest()
			wrapped := soroushlib.WrapInitConnection(soroushlib.SoroushAppID, body)
			_, _, err := tm.session.SendAndWait(ctx, wrapped, true)
			if err != nil {
				// Non-fatal: just means the noise request failed
				logger.Debug(component, "TokenManager: Activity noise failed (non-fatal)", "error", err)
			}
		case <-ctx.Done():
			return
		}
	}
}

// fetchToken calls the Soroush MTProto API to get a LiveKit group call JWT token.
//
// Flow (reverse-engineered from web.splus.ir JS bundle + WebSocket captures):
//   1. phone.getGroupCall(InputGroupCall{id, access_hash}) → phone.GroupCall
//      Gets the group call's current state and confirms it exists.
//   2. phone.joinGroupCall(InputGroupCall{id, access_hash}, join_as=self, params={}) → Updates
//      The server responds with an Updates wrapper containing
//      updateGroupCallConnection{params: DataJSON{data: "<JWT>"}}
//   3. Parse the JWT and extract room name from payload.video.room
func (tm *TokenManager) fetchToken(ctx context.Context) (*LiveKitToken, error) {
	groupCallID := tm.cfg.GroupChatID
	groupAccessHash := tm.cfg.GroupAccessHash

	// Step 1: phone.getGroupCall — verify the group call exists
	getCallBody := soroushlib.BuildGetGroupCallRequest(groupCallID, groupAccessHash)
	wrapped := soroushlib.WrapInitConnection(soroushlib.SoroushAppID, getCallBody)

	cid, reader, err := tm.session.SendAndWait(ctx, wrapped, true)
	if err != nil {
		return nil, fmt.Errorf("phone.getGroupCall failed: %w", err)
	}

	callInfo, err := soroushlib.ParseGetGroupCallResponse(cid, reader)
	if err != nil {
		return nil, fmt.Errorf("parse getGroupCall response: %w", err)
	}

	logger.Info(component, "TokenManager: Group call found",
		"call_id", callInfo.ID,
		"participants", callInfo.ParticipantCount,
		"title", callInfo.Title,
	)

	// Step 2: phone.joinGroupCall — get the LiveKit JWT
	joinBody := soroushlib.BuildJoinGroupCallRequest(
		callInfo.ID,
		callInfo.AccessHash,
		tm.account.SoroushUserID,
		tm.account.AccessHash,
		true, // muted = true (data-only, no audio)
	)

	cid, reader, err = tm.session.SendAndWait(ctx, joinBody, true)
	if err != nil {
		return nil, fmt.Errorf("phone.joinGroupCall failed: %w", err)
	}

	gcToken, err := soroushlib.ParseJoinGroupCallResponse(cid, reader)
	if err != nil {
		return nil, fmt.Errorf("parse joinGroupCall response: %w", err)
	}

	if gcToken.JWT == "" {
		return nil, fmt.Errorf("joinGroupCall returned empty JWT token")
	}

	logger.Info(component, "TokenManager: LiveKit JWT acquired",
		"room", gcToken.RoomID,
		"server", gcToken.ServerURL,
		"jwt_len", len(gcToken.JWT),
	)

	return &LiveKitToken{
		JWT:       gcToken.JWT,
		RoomID:    gcToken.RoomID,
		ExpiresAt: time.Now().Add(10 * time.Minute), // Soroush tokens typically expire in ~10-15 min
	}, nil
}

