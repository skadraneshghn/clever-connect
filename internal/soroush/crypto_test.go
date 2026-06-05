package soroush

import (
	"encoding/json"
	"testing"
)

type FallbackConfigPayload struct {
	GroupChatID     int64  `json:"group_chat_id"`
	GroupAccessHash int64  `json:"group_access_hash"`
	PSK             string `json:"psk"`
}

func TestEncryptDecrypt(t *testing.T) {
	pin := "123456"
	plaintext := "WAKEUP"

	ciphertext, err := EncryptPayload(plaintext, pin)
	if err != nil {
		t.Fatalf("Failed to encrypt: %v", err)
	}

	decrypted, err := DecryptPayload(ciphertext, pin)
	if err != nil {
		t.Fatalf("Failed to decrypt: %v", err)
	}

	if decrypted != plaintext {
		t.Errorf("Expected decrypted text to be '%s', got '%s'", plaintext, decrypted)
	}

	// Test JSON payload
	payload := FallbackConfigPayload{
		GroupChatID:     12345678,
		GroupAccessHash: 87654321,
		PSK:             "super-secret-key-123",
	}

	rawJSON, err := json.Marshal(payload)
	if err != nil {
		t.Fatalf("Failed to marshal: %v", err)
	}

	encryptedJSON, err := EncryptPayload(string(rawJSON), pin)
	if err != nil {
		t.Fatalf("Failed to encrypt JSON: %v", err)
	}

	decryptedJSON, err := DecryptPayload(encryptedJSON, pin)
	if err != nil {
		t.Fatalf("Failed to decrypt JSON: %v", err)
	}

	var decryptedPayload FallbackConfigPayload
	if err := json.Unmarshal([]byte(decryptedJSON), &decryptedPayload); err != nil {
		t.Fatalf("Failed to unmarshal JSON: %v", err)
	}

	if decryptedPayload.GroupChatID != payload.GroupChatID || decryptedPayload.PSK != payload.PSK {
		t.Errorf("Payload mismatch: %+v vs %+v", decryptedPayload, payload)
	}
}
