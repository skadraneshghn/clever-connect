package soroush

import (
	"context"
	"encoding/base64"
	"encoding/binary"
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
// If the dynamic token resolution flow fails, it will automatically fall back to the manually-configured
// FallbackLiveKitToken saved in the database config.
func GetOrRefreshLiveKitToken(ctx context.Context, cfg *models.SoroushTunnelConfig, acct *models.SoroushAccount, isServer bool) (string, error) {
	// 1. Memory Cache Layer Validation
	if acct.LiveKitToken != "" && !IsTokenExpired(acct.LiveKitToken) {
		logger.Debug(componentJit, "JIT: Reusing active unexpired token from cache database.")
		return acct.LiveKitToken, nil
	}

	// 2. Attempt dynamic token acquisition
	token, err := getOrRefreshDynamicToken(ctx, cfg, acct, isServer)
	if err != nil {
		// Dynamic flow failed. Check if fallback token is available.
		if cfg.FallbackLiveKitToken != "" {
			logger.Warn(componentJit, "JIT: Dynamic token acquisition failed. Using manual fallback token from configuration.", "error", err)
			
			// Cache the fallback token to the account so it doesn't try again immediately
			acct.LiveKitToken = cfg.FallbackLiveKitToken
			db.DB.Save(acct)
			
			return cfg.FallbackLiveKitToken, nil
		}
		// No fallback token, return the original error.
		return "", err
	}

	return token, nil
}

// getOrRefreshDynamicToken performs the MTProto handshake to fetch a fresh LiveKit token.
func getOrRefreshDynamicToken(ctx context.Context, cfg *models.SoroushTunnelConfig, acct *models.SoroushAccount, isServer bool) (string, error) {
	logger.Info(componentJit, "JIT: Synchronizing signaling session with wss://im-server.splus.ir/apiws...")

	// 1. Establish custom obfuscated signaling session
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
		// Salt is likely stale — clear it from DB and return a retriable error
		acct.ServerSalt = nil
		db.DB.Save(acct)
		return "", fmt.Errorf("warm up: %w (server_salt reset, will retry)", err)
	}

	// Persist potentially updated ServerSalt if modified during session handshake
	if session.ServerSalt != 0 {
		newSaltBytes := make([]byte, 8)
		binary.LittleEndian.PutUint64(newSaltBytes, uint64(session.ServerSalt))
		acct.ServerSalt = newSaltBytes
		db.DB.Save(acct)
	}

	// 2. Resolve active CallID and CallAccessHash (either via static override or dynamic resolution)
	var callID, callAccessHash int64
	if cfg.CallID != 0 && cfg.CallAccessHash != 0 {
		logger.Info(componentJit, "JIT: Static Call Routing detected. Skipping auto-resolution loops...", "call_id", cfg.CallID)
		callID = cfg.CallID
		callAccessHash = cfg.CallAccessHash
	} else {
		var err error
		callID, callAccessHash, err = soroushlib.ResolveGroupCall(ctx, session, cfg.GroupChatID, cfg.GroupAccessHash)
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
	}

	// 3. Handshake to fetch the LiveKit Token using the active session
	return fetchTokenWithSession(ctx, session, callID, callAccessHash, acct)
}

// fetchTokenWithSession executes direct target handshake over the MTProto signaling gateway using a pre-warmed session.
func fetchTokenWithSession(ctx context.Context, session *soroushlib.MTProtoSession, callID int64, callAccessHash int64, acct *models.SoroushAccount) (string, error) {
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

	// Update local memory and DB salt if changed during signaling
	if session.ServerSalt != 0 {
		newSaltBytes := make([]byte, 8)
		binary.LittleEndian.PutUint64(newSaltBytes, uint64(session.ServerSalt))
		acct.ServerSalt = newSaltBytes
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
