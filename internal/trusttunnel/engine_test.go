package trusttunnel

import (
	"os"
	"path/filepath"
	"testing"

	"clever-connect/internal/db"
	"clever-connect/internal/models"

	sqlite "clever-connect/internal/db/sqlite"
	"gorm.io/gorm"
)

func setupTestDB(t *testing.T) {
	gdb, err := gorm.Open(sqlite.Open("file::memory:?cache=shared"), &gorm.Config{})
	if err != nil {
		t.Fatalf("failed to open test database: %v", err)
	}

	if err := gdb.AutoMigrate(
		&models.TrustTunnelConfig{},
		&models.TrustTunnelUser{},
		&models.TrustTunnelFirewallRule{},
	); err != nil {
		t.Fatalf("failed to migrate schemas: %v", err)
	}

	db.DB = gdb
}

func TestTokenExportImport(t *testing.T) {
	setupTestDB(t)

	cfg := &models.TrustTunnelConfig{
		ListenAddress:             "0.0.0.0:443",
		ServerHostname:            "stealth.example.com",
		ForcedTransport:           "http2",
		AuthFailureStatusCode:     404,
		ClientRandomPrefix:        "a0b0/f0f0",
		H2InitialStreamWindowSize: 131072,
		TlsHandshakeTimeoutSecs:   4,
	}

	// Create a dummy user
	user := models.TrustTunnelUser{
		Username: "test-user",
		Password: "test-password",
		IsActive: true,
	}
	if err := db.DB.Create(&user).Error; err != nil {
		t.Fatalf("failed to create test user: %v", err)
	}

	token := GenerateExportToken(cfg)
	if token == "" {
		t.Fatalf("Expected token to not be empty")
	}

	params, err := ParseImportToken(token)
	if err != nil {
		t.Fatalf("Failed to parse exported token: %v", err)
	}

	if params["addr"] != cfg.ListenAddress {
		t.Errorf("Expected addr %s, got %s", cfg.ListenAddress, params["addr"])
	}
	if params["hostname"] != cfg.ServerHostname {
		t.Errorf("Expected hostname %s, got %s", cfg.ServerHostname, params["hostname"])
	}
	if params["transport"] != cfg.ForcedTransport {
		t.Errorf("Expected transport %s, got %s", cfg.ForcedTransport, params["transport"])
	}
	if params["prefix"] != cfg.ClientRandomPrefix {
		t.Errorf("Expected prefix %s, got %s", cfg.ClientRandomPrefix, params["prefix"])
	}
	if params["user"] != "test-user" {
		t.Errorf("Expected user test-user, got %s", params["user"])
	}
	if params["pass"] != "test-password" {
		t.Errorf("Expected pass test-password, got %s", params["pass"])
	}
}

func TestGenerateTOMLConfigs(t *testing.T) {
	setupTestDB(t)

	cfg := &models.TrustTunnelConfig{
		ListenAddress:             "0.0.0.0:443",
		ConnectAddress:            "127.0.0.1:443",
		Socks5Port:                1088,
		HttpPort:                  1089,
		ForcedTransport:           "http2",
		AuthFailureStatusCode:     404,
		ClientRandomPrefix:        "a0b0/f0f0",
		H2InitialStreamWindowSize: 131072,
		H2InitialConnWindowSize:   262144,
		TlsHandshakeTimeoutSecs:   4,
		KillSwitchEnabled:         true,
		ServerHostname:            "server.sni.com",
		TlsCertPath:               "/path/to/cert.pem",
		TlsKeyPath:                "/path/to/key.pem",
	}

	// Add a dummy user and firewall rule
	user := models.TrustTunnelUser{Username: "john", Password: "doe", IsActive: true}
	db.DB.Create(&user)

	rule := models.TrustTunnelFirewallRule{TargetCIDR: "10.0.0.0/8", BypassStrategy: "direct-route"}
	db.DB.Create(&rule)

	// Clean up config directory before generating
	dir := getConfigDir()
	os.RemoveAll(dir)

	// Test server TOML generation
	err := generateServerTOML(cfg)
	if err != nil {
		t.Fatalf("generateServerTOML failed: %v", err)
	}

	// Check files exist
	files := []string{"vpn.toml", "hosts.toml", "credentials.toml", "rules.toml"}
	for _, f := range files {
		path := filepath.Join(dir, f)
		if _, err := os.Stat(path); os.IsNotExist(err) {
			t.Errorf("Expected file %s to be created", f)
		}
	}

	// Test client TOML generation
	os.RemoveAll(dir)
	err = generateClientTOML(cfg)
	if err != nil {
		t.Fatalf("generateClientTOML failed: %v", err)
	}

	clientFiles := []string{"client.toml", "rules.toml"}
	for _, f := range clientFiles {
		path := filepath.Join(dir, f)
		if _, err := os.Stat(path); os.IsNotExist(err) {
			t.Errorf("Expected client file %s to be created", f)
		}
	}
}
