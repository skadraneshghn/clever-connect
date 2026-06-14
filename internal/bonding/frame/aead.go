package frame

import (
	"crypto/aes"
	"crypto/cipher"
	"crypto/rand"
	"encoding/binary"
	"errors"
	"fmt"
	"io"
)

// ──────────────────────────────────────────────────────────────────────────────
// AEAD Envelope — Optional encryption layer for DPI resistance
// ──────────────────────────────────────────────────────────────────────────────
//
// DESIGN NOTE (from expert review): Even though each artery runs TLS 1.3, the
// frame header's predictable structure (constant version byte, monotonic seqs,
// small type set) can be fingerprinted by DPI systems doing traffic shape
// heuristics or active probing on the inner payload.
//
// The AEAD envelope wraps the entire encoded frame (header + payload) with
// AES-256-GCM, producing ciphertext that is indistinguishable from random.
// The envelope also prepends a 4-byte SessionID to allow the combiner to
// group artery connections from the same client across different IP paths.
//
// Envelope layout:
//
//	┌──────────────┬───────────┬─────────────────────────────────────────────┐
//	│ SessionID(4) │ Nonce(12) │ AEAD Ciphertext (header + payload + tag)   │
//	└──────────────┴───────────┴─────────────────────────────────────────────┘
//
// Total overhead: 4 (session) + 12 (nonce) + 16 (GCM tag) = 32 bytes.

const (
	// SessionIDSize is the size of the session identifier prefix.
	SessionIDSize = 4

	// NonceSize is the AES-GCM nonce size.
	NonceSize = 12

	// TagSize is the AES-GCM authentication tag size.
	TagSize = 16

	// EnvelopeOverhead is the total overhead added by the AEAD envelope.
	EnvelopeOverhead = SessionIDSize + NonceSize + TagSize
)

// Errors for AEAD operations.
var (
	ErrEnvelopeTooShort = errors.New("aead: envelope too short")
	ErrDecryptFailed    = errors.New("aead: decryption failed (invalid key or corrupted)")
	ErrInvalidKeySize   = errors.New("aead: key must be 32 bytes (AES-256)")
)

// Envelope wraps/unwraps frames with AEAD encryption and a session identifier.
type Envelope struct {
	sessionID uint32
	gcm       cipher.AEAD
}

// NewEnvelope creates an AEAD envelope with the given session ID and 32-byte PSK.
// The PSK is used as the AES-256-GCM key directly.
func NewEnvelope(sessionID uint32, psk []byte) (*Envelope, error) {
	if len(psk) != 32 {
		return nil, fmt.Errorf("%w: got %d bytes", ErrInvalidKeySize, len(psk))
	}

	block, err := aes.NewCipher(psk)
	if err != nil {
		return nil, fmt.Errorf("aead: failed to create cipher: %w", err)
	}

	gcm, err := cipher.NewGCM(block)
	if err != nil {
		return nil, fmt.Errorf("aead: failed to create GCM: %w", err)
	}

	return &Envelope{
		sessionID: sessionID,
		gcm:       gcm,
	}, nil
}

// SessionID returns the envelope's session identifier.
func (e *Envelope) SessionID() uint32 {
	return e.sessionID
}

// Seal encrypts an encoded frame and prepends the session ID + nonce.
// Input: raw encoded frame bytes (from Frame.Encode()).
// Output: [SessionID(4)] [Nonce(12)] [Ciphertext + GCM Tag].
func (e *Envelope) Seal(plaintext []byte) ([]byte, error) {
	nonce := make([]byte, NonceSize)
	if _, err := io.ReadFull(rand.Reader, nonce); err != nil {
		return nil, fmt.Errorf("aead: failed to generate nonce: %w", err)
	}

	// Allocate output: sessionID + nonce + ciphertext (plaintext + tag)
	output := make([]byte, SessionIDSize+NonceSize+len(plaintext)+TagSize)

	// Write session ID
	binary.BigEndian.PutUint32(output[0:SessionIDSize], e.sessionID)

	// Write nonce
	copy(output[SessionIDSize:SessionIDSize+NonceSize], nonce)

	// Encrypt (AEAD with session ID as additional authenticated data)
	aad := output[0:SessionIDSize] // session ID is authenticated but not encrypted
	ciphertext := e.gcm.Seal(output[SessionIDSize+NonceSize:SessionIDSize+NonceSize], nonce, plaintext, aad)
	_ = ciphertext // Seal writes in-place

	return output[:SessionIDSize+NonceSize+len(plaintext)+TagSize], nil
}

// Open decrypts an AEAD envelope and returns the plaintext frame bytes.
// Also returns the session ID from the envelope.
func (e *Envelope) Open(envelope []byte) (plaintext []byte, sessionID uint32, err error) {
	minLen := SessionIDSize + NonceSize + TagSize
	if len(envelope) < minLen {
		return nil, 0, ErrEnvelopeTooShort
	}

	sessionID = binary.BigEndian.Uint32(envelope[0:SessionIDSize])
	nonce := envelope[SessionIDSize : SessionIDSize+NonceSize]
	ciphertext := envelope[SessionIDSize+NonceSize:]
	aad := envelope[0:SessionIDSize]

	plaintext, err = e.gcm.Open(nil, nonce, ciphertext, aad)
	if err != nil {
		return nil, sessionID, fmt.Errorf("%w: %v", ErrDecryptFailed, err)
	}

	return plaintext, sessionID, nil
}

// OpenStatic decrypts without requiring a pre-configured Envelope instance.
// Useful on the server side to peek at the session ID before routing.
func OpenStatic(envelope []byte, psk []byte) (plaintext []byte, sessionID uint32, err error) {
	if len(psk) != 32 {
		return nil, 0, ErrInvalidKeySize
	}

	minLen := SessionIDSize + NonceSize + TagSize
	if len(envelope) < minLen {
		return nil, 0, ErrEnvelopeTooShort
	}

	sessionID = binary.BigEndian.Uint32(envelope[0:SessionIDSize])

	block, err := aes.NewCipher(psk)
	if err != nil {
		return nil, sessionID, err
	}
	gcm, err := cipher.NewGCM(block)
	if err != nil {
		return nil, sessionID, err
	}

	nonce := envelope[SessionIDSize : SessionIDSize+NonceSize]
	ciphertext := envelope[SessionIDSize+NonceSize:]
	aad := envelope[0:SessionIDSize]

	plaintext, err = gcm.Open(nil, nonce, ciphertext, aad)
	if err != nil {
		return nil, sessionID, fmt.Errorf("%w: %v", ErrDecryptFailed, err)
	}

	return plaintext, sessionID, nil
}

// ExtractSessionID reads just the session ID from an envelope without decrypting.
// Useful for routing on the combiner before full decryption.
func ExtractSessionID(envelope []byte) (uint32, error) {
	if len(envelope) < SessionIDSize {
		return 0, ErrEnvelopeTooShort
	}
	return binary.BigEndian.Uint32(envelope[0:SessionIDSize]), nil
}
