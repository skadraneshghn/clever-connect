package soroush

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"strings"
	"time"

	"clever-connect/internal/db"
	"clever-connect/internal/logger"
	"clever-connect/internal/models"
	"clever-connect/internal/soroushlib"
)

const componentJit = "SoroushJit"

// GetOrRefreshLiveKitToken coordinates automatic token extraction with static infrastructure overrides.
func GetOrRefreshLiveKitToken(ctx context.Context, cfg *models.SoroushTunnelConfig, acct *models.SoroushAccount, isServer bool) (string, error) {
	// 1. Memory Cache Layer Validation
	if acct.LiveKitToken != "" && !IsTokenExpired(acct.LiveKitToken) {
		logger.Debug(componentJit, "JIT: Reusing active unexpired token from cache database.")
		return acct.LiveKitToken, nil
	}

	// 2. STRATEGIC OVERRIDE: Check if static Call Routing is configured
	if cfg.CallID != 0 && cfg.CallAccessHash != 0 {
		logger.Info(componentJit, "JIT: Static Call Routing detected. Skipping auto-resolution loops...", "call_id", cfg.CallID)
		return fetchTokenWithIdentifiers(ctx, cfg.CallID, cfg.CallAccessHash, acct)
	}

	logger.Info(componentJit, "JIT: Synchronizing signaling session with wss://im-server.splus.ir/apiws...")

	// 3. Establish custom obfuscated signaling session
	session, transport := soroushlib.RestoreSession(
		acct.AuthKey,
		acct.AuthKeyID,
		acct.ServerSalt,
	)

	if err := transport.Connect(ctx); err != nil {
		return "", fmt.Errorf("failed to establish WebSocket transport: %w", err)
	}
	defer transport.Disconnect()

	if err := session.WarmUpSession(ctx); err != nil {
		return "", fmt.Errorf("session warmup verification rejected: %w", err)
	}

	// 4. Resolve Active Call Meta Structures
	callID, callAccessHash, err := soroushlib.ResolveGroupCall(ctx, session, cfg.GroupChatID, cfg.GroupAccessHash)
	if err != nil {
		// If we are the server and no call is active, we attempt to create it
		if isServer {
			logger.Info(componentJit, "JIT: No active group call found, creating one...", "chat_id", cfg.GroupChatID)
			if createErr := soroushlib.CreateGroupCall(ctx, session, cfg.GroupChatID, cfg.GroupAccessHash); createErr != nil {
				return "", fmt.Errorf("failed to create group call: %w", createErr)
			}
			// Resolve again after creating
			callID, callAccessHash, err = soroushlib.ResolveGroupCall(ctx, session, cfg.GroupChatID, cfg.GroupAccessHash)
			if err != nil {
				return "", fmt.Errorf("failed to resolve group call after creation: %w", err)
			}
		} else {
			logger.Error(componentJit, "JIT: Auto-resolution failed to locate a running call instance. Optimization required.", "error", err)
			return "", fmt.Errorf("resolution failed. Prerequisite: Ensure your account has joined group %d or use Static Call Overrides. Error: %w", cfg.GroupChatID, err)
		}
	}

	logger.Info(componentJit, "JIT: Dynamic call instance resolved successfully", "call_id", callID)
	return fetchTokenWithIdentifiers(ctx, callID, callAccessHash, acct)
}

// fetchTokenWithIdentifiers executes the direct target handshake over the MTProto signaling gateway.
func fetchTokenWithIdentifiers(ctx context.Context, callID int64, callAccessHash int64, acct *models.SoroushAccount) (string, error) {
	session, transport := soroushlib.RestoreSession(acct.AuthKey, acct.AuthKeyID, acct.ServerSalt)
	if err := transport.Connect(ctx); err != nil {
		return "", err
	}
	defer transport.Disconnect()
	_ = session.WarmUpSession(ctx)

	// Build direct join frame matching official web application parameters
	joinBody := soroushlib.BuildJoinGroupCallRequest(
		callID,
		callAccessHash,
		acct.SoroushUserID,
		acct.AccessHash,
		true, // Mute media tracks (Tunnel data framing mode)
	)

	cid, reader, err := session.SendAndWait(ctx, joinBody, true)
	if err != nil {
		return "", fmt.Errorf("phone.joinGroupCall signaling rejected by platform: %w", err)
	}

	gcToken, err := soroushlib.ParseJoinGroupCallResponse(cid, reader)
	if err != nil {
		return "", fmt.Errorf("failed to extract token from payload structure: %w", err)
	}

	if gcToken.JWT == "" {
		return "", fmt.Errorf("server responded successfully but token block string was empty")
	}

	// Persist synchronized token to caching layers
	acct.LiveKitToken = gcToken.JWT
	db.DB.Save(acct)

	logger.Info(componentJit, "JIT: LiveKit Access Token extracted successfully from custom transport loop.")
	return gcToken.JWT, nil
}

// IsTokenExpired checks if a JWT token is expired or close to expiration (within 2 minutes)
func IsTokenExpired(tokenString string) bool {
	parts := strings.Split(tokenString, ".")
	if len(parts) < 2 {
		return true // invalid token is treated as expired
	}

	payloadSegment := parts[1]
	// base64 standard/url decoding
	switch len(payloadSegment) % 4 {
	case 2:
		payloadSegment += "=="
	case 3:
		payloadSegment += "="
	}

	decoded, err := base64.URLEncoding.DecodeString(payloadSegment)
	if err != nil {
		// Try standard base64 decoding if URL encoding fails
		decoded, err = base64.StdEncoding.DecodeString(payloadSegment)
		if err != nil {
			return true
		}
	}

	var payload struct {
		Exp int64 `json:"exp"`
	}
	if err := json.Unmarshal(decoded, &payload); err != nil {
		return true
	}

	// Buffer of 120 seconds before actual expiration
	return time.Now().Unix() >= (payload.Exp - 120)
}
