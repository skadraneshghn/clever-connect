package trusttunnel

import (
	"encoding/pem"
	"os"
	"path/filepath"
	"strings"
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
		AuthFailureStatusCode:     407,
		ClientRandomPrefix:        "a0b0/f0f0",
		H2InitialStreamWindowSize: 131072,
		TlsHandshakeTimeoutSecs:   4,
		TlsServerCert:             "-----BEGIN CERTIFICATE-----\nmy-test-cert-pem\n-----END CERTIFICATE-----",
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
	if params["cert"] != cfg.TlsServerCert {
		t.Errorf("Expected cert %s, got %s", cfg.TlsServerCert, params["cert"])
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
		AuthFailureStatusCode:     407,
		ClientRandomPrefix:        "a0b0/f0f0",
		H2InitialStreamWindowSize: 131072,
		H2InitialConnWindowSize:   262144,
		TlsHandshakeTimeoutSecs:   4,
		KillSwitchEnabled:         true,
		ServerHostname:            "server.sni.com",
		TlsCertPath:               "/path/to/cert.pem",
		TlsKeyPath:                "/path/to/key.pem",
		ClientUsername:            "my-client-user",
		ClientPassword:            "my-client-pass",
		TlsServerCert:             "-----BEGIN CERTIFICATE-----\ncustom-cert\n-----END CERTIFICATE-----",
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

	clientFiles := []string{"client.toml"}
	for _, f := range clientFiles {
		path := filepath.Join(dir, f)
		if _, err := os.Stat(path); os.IsNotExist(err) {
			t.Errorf("Expected client file %s to be created", f)
		}
	}

	// Verify client.toml contains the fields we set
	data, err := os.ReadFile(filepath.Join(dir, "client.toml"))
	if err != nil {
		t.Fatalf("failed to read generated client.toml: %v", err)
	}
	content := string(data)
	if !strings.Contains(content, `username = "my-client-user"`) {
		t.Errorf("Expected client.toml to contain username setting, got:\n%s", content)
	}
	if !strings.Contains(content, `password = "my-client-pass"`) {
		t.Errorf("Expected client.toml to contain password setting, got:\n%s", content)
	}
	if !strings.Contains(content, `custom-cert`) {
		t.Errorf("Expected client.toml to contain custom-cert setting, got:\n%s", content)
	}
	if !strings.Contains(content, `vpn_mode = "general"`) {
		t.Errorf("Expected client.toml to contain vpn_mode at root, got:\n%s", content)
	}
	if !strings.Contains(content, `client_random = "a0b0/f0f0"`) {
		t.Errorf("Expected client.toml to contain client_random, got:\n%s", content)
	}
	if !strings.Contains(content, `load_certificate = """`) {
		t.Errorf("Expected client.toml to contain load_certificate key, got:\n%s", content)
	}
	if !strings.Contains(content, `change_system_dns = false`) {
		t.Errorf("Expected client.toml to contain change_system_dns, got:\n%s", content)
	}
	if !strings.Contains(content, `excluded_routes = ["10.0.0.0/8"]`) {
		t.Errorf("Expected client.toml to contain excluded_routes, got:\n%s", content)
	}
}

func TestResolveConnectAddress(t *testing.T) {
	tests := []struct {
		name       string
		addr       string
		hostname   string
		expected   string
	}{
		{
			name:     "Wildcard IPv4",
			addr:     "0.0.0.0:443",
			hostname: "vpn.example.com",
			expected: "vpn.example.com:443",
		},
		{
			name:     "Wildcard IPv6",
			addr:     "[::]:8080",
			hostname: "vpn.example.com",
			expected: "vpn.example.com:8080",
		},
		{
			name:     "Loopback IPv4",
			addr:     "127.0.0.1:3000",
			hostname: "vpn.example.com",
			expected: "vpn.example.com:3000",
		},
		{
			name:     "Localhost",
			addr:     "localhost:443",
			hostname: "vpn.example.com",
			expected: "vpn.example.com:443",
		},
		{
			name:     "Valid public address unchanged",
			addr:     "1.2.3.4:443",
			hostname: "vpn.example.com",
			expected: "1.2.3.4:443",
		},
		{
			name:     "Empty address with hostname",
			addr:     "",
			hostname: "vpn.example.com",
			expected: "vpn.example.com:443",
		},
		{
			name:     "Empty address and no hostname",
			addr:     "",
			hostname: "",
			expected: "127.0.0.1:443",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := ResolveConnectAddress(tt.addr, tt.hostname)
			if result != tt.expected {
				t.Errorf("expected %q, got %q", tt.expected, result)
			}
		})
	}
}

func TestGenerateSelfSignedCert(t *testing.T) {
	tempDir, err := os.MkdirTemp("", "tt-test-certs")
	if err != nil {
		t.Fatalf("failed to create temp dir: %v", err)
	}
	defer os.RemoveAll(tempDir)

	certPath, keyPath, err := GenerateSelfSignedCert("selfsigned.example.com", tempDir)
	if err != nil {
		t.Fatalf("GenerateSelfSignedCert failed: %v", err)
	}

	if _, err := os.Stat(certPath); os.IsNotExist(err) {
		t.Errorf("Expected cert file to be created at %s", certPath)
	}
	if _, err := os.Stat(keyPath); os.IsNotExist(err) {
		t.Errorf("Expected key file to be created at %s", keyPath)
	}

	// Verify they are valid PEM files
	certData, err := os.ReadFile(certPath)
	if err != nil {
		t.Fatalf("failed to read cert file: %v", err)
	}
	block, _ := pem.Decode(certData)
	if block == nil || block.Type != "CERTIFICATE" {
		t.Errorf("Invalid certificate PEM structure: %v", block)
	}
}
