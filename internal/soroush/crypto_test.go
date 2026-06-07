package soroush

import (
	"testing"
)

func TestHandshakeChallenge(t *testing.T) {
	psk := "super-secret-key-123"

	// Build challenge from PSK
	challenge, err := BuildHandshakeChallenge(psk)
	if err != nil {
		t.Fatalf("Failed to build handshake challenge: %v", err)
	}

	if len(challenge) != 64 {
		t.Errorf("Expected challenge length 64, got %d", len(challenge))
	}

	// Verify the challenge with the same PSK
	if err := VerifyHandshakeChallenge(psk, challenge); err != nil {
		t.Fatalf("Failed to verify handshake challenge: %v", err)
	}

	// Verify with wrong PSK should fail
	if err := VerifyHandshakeChallenge("wrong-key", challenge); err == nil {
		t.Error("Expected verification to fail with wrong PSK, but it succeeded")
	}
}

func TestDeriveVerificationToken(t *testing.T) {
	psk := "super-secret-key-123"

	token1, err := DeriveVerificationToken(psk)
	if err != nil {
		t.Fatalf("Failed to derive verification token: %v", err)
	}

	token2, err := DeriveVerificationToken(psk)
	if err != nil {
		t.Fatalf("Failed to derive verification token: %v", err)
	}

	// Same PSK should produce same token (deterministic)
	if token1 != token2 {
		t.Errorf("Expected same token for same PSK, got '%s' vs '%s'", token1, token2)
	}

	// Different PSK should produce different token
	token3, err := DeriveVerificationToken("different-key")
	if err != nil {
		t.Fatalf("Failed to derive verification token: %v", err)
	}

	if token1 == token3 {
		t.Error("Expected different tokens for different PSKs")
	}
}
