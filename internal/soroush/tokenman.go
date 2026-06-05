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

// GetOrRefreshLiveKitToken checks if the account's LiveKitToken is valid and unexpired.
// If it is invalid or expiring within 2 minutes, it connects to Soroush via MTProto,
// requests a new token, saves it to the database, and returns it.
func GetOrRefreshLiveKitToken(ctx context.Context, cfg *models.SoroushTunnelConfig, acct *models.SoroushAccount, isServer bool) (string, error) {
	if acct.LiveKitToken != "" && !IsTokenExpired(acct.LiveKitToken) {
		return acct.LiveKitToken, nil
	}

	logger.Info(component, "JIT: LiveKitToken is missing or expired. Fetching fresh token...", "phone", soroushlib.MaskPhone(acct.PhoneNumber))

	if cfg.GroupChatID == 0 {
		return "", fmt.Errorf("GroupChatID is not configured in Soroush Tunnel Config")
	}

	// Restore session from saved auth credentials
	session, transport := soroushlib.RestoreSession(
		acct.AuthKey,
		acct.AuthKeyID,
		acct.ServerSalt,
	)

	// Connect the transport (WebSocket + obfuscation handshake)
	if err := transport.Connect(ctx); err != nil {
		return "", fmt.Errorf("failed to connect to Soroush: %w", err)
	}
	defer transport.Disconnect()

	// Warm up session to prime the server salt
	if err := session.WarmUpSession(ctx); err != nil {
		return "", fmt.Errorf("failed to warm up Soroush session: %w", err)
	}

	// Step 1: Resolve the active group call ID and access hash
	callID, callAccessHash, err := soroushlib.ResolveGroupCall(ctx, session, cfg.GroupChatID, cfg.GroupAccessHash)
	if err != nil {
		// If we are the server and no call is active, we attempt to create it
		if isServer {
			logger.Info(component, "JIT: No active group call found, creating one...", "chat_id", cfg.GroupChatID)
			if createErr := soroushlib.CreateGroupCall(ctx, session, cfg.GroupChatID, cfg.GroupAccessHash); createErr != nil {
				return "", fmt.Errorf("failed to create group call: %w", createErr)
			}
			// Resolve again after creating
			callID, callAccessHash, err = soroushlib.ResolveGroupCall(ctx, session, cfg.GroupChatID, cfg.GroupAccessHash)
			if err != nil {
				return "", fmt.Errorf("failed to resolve group call after creation: %w", err)
			}
		} else {
			return "", fmt.Errorf("failed to resolve active group call: %w", err)
		}
	}

	logger.Info(component, "JIT: Active group call resolved",
		"call_id", callID,
		"call_access_hash", callAccessHash,
	)

	// Step 2: phone.joinGroupCall — get the LiveKit JWT
	joinBody := soroushlib.BuildJoinGroupCallRequest(
		callID,
		callAccessHash,
		acct.SoroushUserID,
		acct.AccessHash,
		true, // muted = true (data-only, no audio)
	)

	cid, reader, err := session.SendAndWait(ctx, joinBody, true)
	if err != nil {
		return "", fmt.Errorf("phone.joinGroupCall failed: %w", err)
	}

	gcToken, err := soroushlib.ParseJoinGroupCallResponse(cid, reader)
	if err != nil {
		return "", fmt.Errorf("parse joinGroupCall response: %w", err)
	}

	if gcToken.JWT == "" {
		return "", fmt.Errorf("joinGroupCall returned empty JWT token")
	}

	logger.Info(component, "JIT: LiveKit JWT acquired successfully",
		"room", gcToken.RoomID,
		"jwt_len", len(gcToken.JWT),
	)

	// Update the database so it persists across runs and can be viewed
	acct.LiveKitToken = gcToken.JWT
	if err := db.DB.Save(acct).Error; err != nil {
		logger.Warn(component, "JIT: Failed to save refreshed token to DB", "error", err)
	}

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
