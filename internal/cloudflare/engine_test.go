package cloudflare

import (
	"context"
	"testing"

	"clever-connect/internal/config"
	"clever-connect/internal/models"
)

func TestGetOAuthConfig(t *testing.T) {
	cfg := &config.Config{
		CloudflareClientID:     "id",
		CloudflareClientSecret: "secret",
		CloudflareRedirectURL:  "url",
	}
	oauthCfg := GetOAuthConfig(cfg)
	if oauthCfg.ClientID != "id" || oauthCfg.ClientSecret != "secret" || oauthCfg.RedirectURL != "url" {
		t.Errorf("OAuth config not matched: %v", oauthCfg)
	}
}

func TestVerifyOAuthTokenInvalid(t *testing.T) {
	cfg := &config.Config{}
	oauthCfg := GetOAuthConfig(cfg)
	_, err := VerifyOAuthToken(context.Background(), oauthCfg, "invalid_code", "My Account")
	if err == nil {
		t.Error("Expected exchange error for invalid code, got nil")
	}
}

func TestGetStatsInvalid(t *testing.T) {
	cfg := &config.Config{}
	oauthCfg := GetOAuthConfig(cfg)
	account := &models.CloudflareAccount{
		AccessToken:  "invalid",
		RefreshToken: "invalid",
	}
	_, err := GetStats(context.Background(), oauthCfg, account)
	if err == nil {
		t.Error("Expected error for invalid credentials, got nil")
	}
}
