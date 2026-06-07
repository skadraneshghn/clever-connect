package soroush

import (
	"crypto/hmac"
	"crypto/rand"
	"crypto/sha256"
	"encoding/binary"
	"fmt"
	"io"
	"sync"
	"time"

	"golang.org/x/crypto/hkdf"
)

var (
	replayMutex        sync.Mutex
	verifiedSignatures = make(map[string]time.Time)
)

// ──────────────────────────────────────────────────────────────────────────────
// Zero-Trust In-Band Handshake Protocol (Phase 3 of Sync Architecture)
//
// Instead of sending the raw PSK over the DataChannel, we use HKDF to derive
// a 32-byte verification token from the PSK, mixed with an 8-byte millisecond
// timestamp nonce. The server verifies both the HMAC and the timestamp freshness.
//
// Wire format (64 bytes total):
//   [0:32]  HKDF-derived auth signature = HKDF(PSK, salt="soroush-hive-v2", info=timestamp_bytes)
//   [32:40] Timestamp nonce (unix milliseconds, little-endian uint64)
//   [40:64] Random padding (24 bytes of entropy to fill 64 bytes)
//
// Server verification rules:
//   - Recompute HKDF with received timestamp → compare to received signature
//   - Reject if timestamp deviates > 5 seconds from server clock
//   - Reject if signature mismatch (wrong PSK)
//   - 3-second read deadline on first packet
// ──────────────────────────────────────────────────────────────────────────────

const (
	handshakeSize     = 64
	signatureSize     = 32
	timestampSize     = 8
	paddingSize       = 24
	hkdfSalt          = "soroush-hive-v2"
	maxClockSkewMs    = 5000 // 5 seconds max clock deviation
)

// BuildHandshakeChallenge creates the 64-byte HKDF authentication challenge
// that the client sends as its first DataChannel packet.
func BuildHandshakeChallenge(psk string) ([]byte, error) {
	nowMs := uint64(time.Now().UnixMilli())

	// Encode timestamp as LE uint64
	tsBytes := make([]byte, timestampSize)
	binary.LittleEndian.PutUint64(tsBytes, nowMs)

	// Derive 32-byte signature via HKDF
	sig, err := deriveHKDFSignature(psk, tsBytes)
	if err != nil {
		return nil, fmt.Errorf("hkdf derive: %w", err)
	}

	// Build the 64-byte challenge block
	challenge := make([]byte, handshakeSize)
	copy(challenge[0:signatureSize], sig)
	copy(challenge[signatureSize:signatureSize+timestampSize], tsBytes)

	// Fill padding with random bytes
	if _, err := rand.Read(challenge[signatureSize+timestampSize:]); err != nil {
		return nil, fmt.Errorf("random padding: %w", err)
	}

	return challenge, nil
}

// VerifyHandshakeChallenge validates an incoming 64-byte challenge block.
// Returns nil on success, error describing the failure reason otherwise.
func VerifyHandshakeChallenge(psk string, challenge []byte) error {
	if len(challenge) != handshakeSize {
		return fmt.Errorf("invalid handshake size: got %d, want %d", len(challenge), handshakeSize)
	}

	// Extract fields
	receivedSig := challenge[0:signatureSize]
	tsBytes := challenge[signatureSize : signatureSize+timestampSize]
	receivedMs := binary.LittleEndian.Uint64(tsBytes)

	// 1. Timestamp freshness check (±5 seconds)
	nowMs := uint64(time.Now().UnixMilli())
	var drift int64
	if nowMs > receivedMs {
		drift = int64(nowMs - receivedMs)
	} else {
		drift = int64(receivedMs - nowMs)
	}
	if drift > maxClockSkewMs {
		return fmt.Errorf("timestamp drift too large: %dms (max %dms) — possible replay attack", drift, maxClockSkewMs)
	}

	// 2. HKDF signature verification
	expectedSig, err := deriveHKDFSignature(psk, tsBytes)
	if err != nil {
		return fmt.Errorf("hkdf derive for verification: %w", err)
	}

	if !hmac.Equal(receivedSig, expectedSig) {
		return fmt.Errorf("signature mismatch — wrong PSK or tampered challenge")
	}

	// 3. Handshake Time Nonce Guard (Anti-Replay sliding window check)
	replayMutex.Lock()
	defer replayMutex.Unlock()

	now := time.Now()
	// Evict entries older than 10s (outside the max clock skew window)
	for sig, addedTime := range verifiedSignatures {
		if now.Sub(addedTime) > 10*time.Second {
			delete(verifiedSignatures, sig)
		}
	}

	sigStr := string(receivedSig)
	if _, exists := verifiedSignatures[sigStr]; exists {
		return fmt.Errorf("handshake signature already used — replay attack blocked")
	}
	verifiedSignatures[sigStr] = now

	return nil
}

// DeriveVerificationToken creates a short-lived verification token from the PSK
// that can safely cross the open network during the sync phase (Phase 1).
// This is NOT the full handshake — it's a derived value for the sync API.
func DeriveVerificationToken(psk string) (string, error) {
	// Use HKDF with a different info context to derive a hex verification token
	hkdfReader := hkdf.New(sha256.New, []byte(psk), []byte("soroush-sync-token"), []byte("verification"))
	token := make([]byte, 32)
	if _, err := io.ReadFull(hkdfReader, token); err != nil {
		return "", err
	}
	return fmt.Sprintf("%x", token), nil
}

// deriveHKDFSignature derives a 32-byte HMAC signature using HKDF.
func deriveHKDFSignature(psk string, info []byte) ([]byte, error) {
	hkdfReader := hkdf.New(sha256.New, []byte(psk), []byte(hkdfSalt), info)
	sig := make([]byte, signatureSize)
	if _, err := io.ReadFull(hkdfReader, sig); err != nil {
		return nil, err
	}
	return sig, nil
}
