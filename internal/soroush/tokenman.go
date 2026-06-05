package soroush

import (
	"context"
	"encoding/json"
	"fmt"
	"math/rand"
	"sync"
	"time"

	"clever-connect/internal/db"
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
	isServer     bool
}

// NewTokenManager creates a new TokenManager for the given account.
func NewTokenManager(account *models.SoroushAccount, cfg *models.SoroushTunnelConfig, isServer bool) *TokenManager {
	return &TokenManager{
		account:  account,
		cfg:      cfg,
		isServer: isServer,
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

	// Start fallback ping-pong listener if PairingPIN is configured
	if tm.cfg.PairingPIN != "" {
		tm.wg.Add(1)
		go tm.startFallbackListener(ctx)
	}

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
//   3. Parse the JWT and extract room name from payload.video
func (tm *TokenManager) fetchToken(ctx context.Context) (*LiveKitToken, error) {
	groupChatID := tm.cfg.GroupChatID
	groupAccessHash := tm.cfg.GroupAccessHash

	// Step 1: Resolve the active group call ID and access hash
	callID, callAccessHash, err := soroushlib.ResolveGroupCall(ctx, tm.session, groupChatID, groupAccessHash)
	if err != nil {
		// If we are the server and no call is active, we attempt to create it
		if tm.isServer {
			logger.Info(component, "TokenManager: No active group call found, creating one...", "chat_id", groupChatID)
			if createErr := soroushlib.CreateGroupCall(ctx, tm.session, groupChatID, groupAccessHash); createErr != nil {
				return nil, fmt.Errorf("failed to create group call: %w", createErr)
			}
			// Resolve again after creating
			callID, callAccessHash, err = soroushlib.ResolveGroupCall(ctx, tm.session, groupChatID, groupAccessHash)
			if err != nil {
				return nil, fmt.Errorf("failed to resolve group call after creation: %w", err)
			}
		} else {
			return nil, fmt.Errorf("failed to resolve active group call: %w", err)
		}
	}

	logger.Info(component, "TokenManager: Active group call resolved",
		"call_id", callID,
		"call_access_hash", callAccessHash,
	)

	// Step 2: phone.joinGroupCall — get the LiveKit JWT
	joinBody := soroushlib.BuildJoinGroupCallRequest(
		callID,
		callAccessHash,
		tm.account.SoroushUserID,
		tm.account.AccessHash,
		true, // muted = true (data-only, no audio)
	)

	cid, reader, err := tm.session.SendAndWait(ctx, joinBody, true)
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

// startFallbackListener runs a message router and listens to text updates for fallback pings.
func (tm *TokenManager) startFallbackListener(ctx context.Context) {
	defer tm.wg.Done()

	logger.Info(component, "Fallback Listener: Starting message routing", "phone", maskPhone(tm.account.PhoneNumber))

	router := soroushlib.NewMessageRouter(tm.session)
	go func() {
		if err := router.Run(ctx); err != nil && ctx.Err() == nil {
			logger.Error(component, "Fallback MessageRouter stopped with error", "error", err)
		}
	}()

	msgCh := router.SubscribeText()
	defer router.UnsubscribeText(msgCh)

	for {
		select {
		case msg, ok := <-msgCh:
			if !ok {
				logger.Info(component, "Fallback Listener: Channel closed, stopping", "phone", maskPhone(tm.account.PhoneNumber))
				return
			}
			tm.handleIncomingFallbackMessage(ctx, msg, router)
		case <-ctx.Done():
			logger.Info(component, "Fallback Listener: Context cancelled, stopping", "phone", maskPhone(tm.account.PhoneNumber))
			return
		}
	}
}

// FallbackConfigPayload is serialized and sent back in response to a WAKEUP ping.
type FallbackConfigPayload struct {
	GroupChatID     int64  `json:"group_chat_id"`
	GroupAccessHash int64  `json:"group_access_hash"`
	PSK             string `json:"psk"`
}

// handleIncomingFallbackMessage attempts to decrypt and verify an incoming trigger message.
func (tm *TokenManager) handleIncomingFallbackMessage(ctx context.Context, msg soroushlib.IncomingMessage, router *soroushlib.MessageRouter) {
	decrypted, err := DecryptPayload(msg.Text, tm.cfg.PairingPIN)
	if err != nil {
		// Normal case: not for us or not ciphertext
		return
	}

	if decrypted != "WAKEUP" {
		logger.Debug(component, "Fallback Listener: Decrypted payload is not WAKEUP", "payload", decrypted)
		return
	}

	logger.Info(component, "Fallback Listener: Received WAKEUP ping", "from_user_id", msg.FromUserID)

	// Verify sender role (must be a registered account)
	var acct models.SoroushAccount
	if err := db.DB.Where("soroush_user_id = ?", msg.FromUserID).First(&acct).Error; err != nil {
		logger.Warn(component, "Fallback Listener: Rejected WAKEUP from unregistered user", "from_user_id", msg.FromUserID)
		return
	}

	// Prepare config response payload
	payload := FallbackConfigPayload{
		GroupChatID:     tm.cfg.GroupChatID,
		GroupAccessHash: tm.cfg.GroupAccessHash,
		PSK:             tm.cfg.PSK,
	}

	rawJSON, err := json.Marshal(payload)
	if err != nil {
		logger.Error(component, "Fallback Listener: Failed to marshal payload", "error", err)
		return
	}

	encryptedReply, err := EncryptPayload(string(rawJSON), tm.cfg.PairingPIN)
	if err != nil {
		logger.Error(component, "Fallback Listener: Failed to encrypt payload", "error", err)
		return
	}

	// Resolve target access hash (either from DB, router cache, or default to msg.FromUserID)
	accessHash := acct.AccessHash
	if accessHash == 0 {
		accessHash = router.GetUserAccessHash(msg.FromUserID)
	}

	logger.Info(component, "Fallback Listener: Sending encrypted configuration reply", "to_user_id", msg.FromUserID)

	if err := soroushlib.SendTextMessage(ctx, tm.session, msg.FromUserID, accessHash, encryptedReply); err != nil {
		logger.Error(component, "Fallback Listener: Failed to send reply message", "error", err)
	} else {
		logger.Info(component, "Fallback Listener: Reply message sent successfully", "to_user_id", msg.FromUserID)
	}
}

