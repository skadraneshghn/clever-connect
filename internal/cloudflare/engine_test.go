package cloudflare

import (
	"testing"
)

func TestNewClientEmptyToken(t *testing.T) {
	_, err := NewClient("")
	if err == nil {
		t.Error("Expected error for empty token, got nil")
	}
}

func TestVerifyTokenInvalid(t *testing.T) {
	_, err := VerifyToken("invalid_token", "My CF Token")
	if err == nil {
		t.Error("Expected error for invalid token, got nil")
	}
}

func TestGetStatsInvalid(t *testing.T) {
	_, err := GetStats("invalid_token", "dummy_acc_id")
	if err == nil {
		t.Error("Expected error for invalid credentials, got nil")
	}
}
