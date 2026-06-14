package frame

import (
	"bytes"
	"crypto/rand"
	"testing"
)

func generateTestKey() []byte {
	key := make([]byte, 32)
	rand.Read(key)
	return key
}

func TestEnvelopeSealOpen(t *testing.T) {
	key := generateTestKey()
	env, err := NewEnvelope(0xDEADBEEF, key)
	if err != nil {
		t.Fatalf("NewEnvelope failed: %v", err)
	}

	// Create and encode a frame
	f := NewDataFrame(42, 7, []byte("hello world"))
	plaintext, err := f.Encode()
	if err != nil {
		t.Fatalf("Encode failed: %v", err)
	}

	// Seal
	sealed, err := env.Seal(plaintext)
	if err != nil {
		t.Fatalf("Seal failed: %v", err)
	}

	// Verify overhead
	expectedLen := len(plaintext) + EnvelopeOverhead
	if len(sealed) != expectedLen {
		t.Errorf("sealed length %d, expected %d", len(sealed), expectedLen)
	}

	// Open
	opened, sessionID, err := env.Open(sealed)
	if err != nil {
		t.Fatalf("Open failed: %v", err)
	}

	if sessionID != 0xDEADBEEF {
		t.Errorf("sessionID mismatch: got %x, want DEADBEEF", sessionID)
	}

	if !bytes.Equal(opened, plaintext) {
		t.Error("decrypted plaintext doesn't match original")
	}

	// Decode the plaintext back into a frame
	f2, err := Decode(opened)
	if err != nil {
		t.Fatalf("Decode failed: %v", err)
	}
	if f2.StreamID != 42 || f2.Seq != 7 || string(f2.Payload) != "hello world" {
		t.Error("frame fields don't match after round-trip through AEAD")
	}
}

func TestEnvelopeWrongKey(t *testing.T) {
	key1 := generateTestKey()
	key2 := generateTestKey()

	env1, _ := NewEnvelope(1, key1)
	env2, _ := NewEnvelope(1, key2)

	plaintext := []byte("secret data")
	sealed, _ := env1.Seal(plaintext)

	// Try to open with wrong key
	_, _, err := env2.Open(sealed)
	if err == nil {
		t.Error("expected error when decrypting with wrong key")
	}
}

func TestEnvelopeTamperDetection(t *testing.T) {
	key := generateTestKey()
	env, _ := NewEnvelope(1, key)

	plaintext := []byte("tamper test data")
	sealed, _ := env.Seal(plaintext)

	// Tamper with ciphertext
	sealed[len(sealed)-5] ^= 0xFF

	_, _, err := env.Open(sealed)
	if err == nil {
		t.Error("expected error when opening tampered envelope")
	}
}

func TestEnvelopeTooShort(t *testing.T) {
	key := generateTestKey()
	env, _ := NewEnvelope(1, key)

	_, _, err := env.Open([]byte{1, 2, 3})
	if err == nil {
		t.Error("expected error for short envelope")
	}
}

func TestExtractSessionID(t *testing.T) {
	key := generateTestKey()
	env, _ := NewEnvelope(0x12345678, key)

	sealed, _ := env.Seal([]byte("test"))
	sid, err := ExtractSessionID(sealed)
	if err != nil {
		t.Fatalf("ExtractSessionID failed: %v", err)
	}
	if sid != 0x12345678 {
		t.Errorf("session ID mismatch: got %x, want 12345678", sid)
	}
}

func TestOpenStatic(t *testing.T) {
	key := generateTestKey()
	env, _ := NewEnvelope(99, key)

	plaintext := []byte("static open test")
	sealed, _ := env.Seal(plaintext)

	opened, sid, err := OpenStatic(sealed, key)
	if err != nil {
		t.Fatalf("OpenStatic failed: %v", err)
	}
	if sid != 99 {
		t.Errorf("session ID: got %d, want 99", sid)
	}
	if !bytes.Equal(opened, plaintext) {
		t.Error("plaintext mismatch")
	}
}

func TestEnvelopeInvalidKeySize(t *testing.T) {
	_, err := NewEnvelope(1, []byte("short"))
	if err == nil {
		t.Error("expected error for invalid key size")
	}
}

func TestEnvelopeRandomness(t *testing.T) {
	key := generateTestKey()
	env, _ := NewEnvelope(1, key)

	plaintext := []byte("same data")

	// Two seals of the same data should produce different ciphertexts (random nonce)
	sealed1, _ := env.Seal(plaintext)
	sealed2, _ := env.Seal(plaintext)

	if bytes.Equal(sealed1, sealed2) {
		t.Error("two seals produced identical output — nonce not random")
	}
}
