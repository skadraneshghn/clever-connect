package compiler

import (
	"strings"
	"testing"

	"clever-connect/internal/db"
	sqlite "clever-connect/internal/db/sqlite"
	"clever-connect/internal/models"
	"gorm.io/gorm"
	gormlogger "gorm.io/gorm/logger"
)

func TestRandomizeCase(t *testing.T) {
	input := "www.google.com"
	output := RandomizeCase(input)

	// Length should be the same
	if len(input) != len(output) {
		t.Errorf("expected length %d, got %d", len(input), len(output))
	}

	// Lowercased versions should match
	if strings.ToLower(input) != strings.ToLower(output) {
		t.Errorf("lowercased input %q and output %q do not match", input, output)
	}

	// Non-alpha characters like dots should not change
	for i := 0; i < len(input); i++ {
		if input[i] == '.' && output[i] != '.' {
			t.Errorf("character at index %d changed from '.' to %q", i, string(output[i]))
		}
	}
}

func TestCompilePadding(t *testing.T) {
	// Initialize in-memory SQLite DB for testing
	gormCfg := &gorm.Config{
		Logger: gormlogger.Default.LogMode(gormlogger.Silent),
	}
	testDB, err := gorm.Open(sqlite.Open("file::memory:?cache=shared"), gormCfg)
	if err != nil {
		t.Fatalf("failed to open test db: %v", err)
	}

	err = testDB.AutoMigrate(&models.V2RayClientSetting{})
	if err != nil {
		t.Fatalf("failed to auto-migrate: %v", err)
	}

	// Back up and restore original global DB
	origDB := db.DB
	defer func() {
		db.DB = origDB
	}()
	db.DB = testDB

	// Seed evasion_padding = true
	testDB.Create(&models.V2RayClientSetting{Key: "evasion_padding", Value: "true"})
	// Seed evasion_fragment = true (should be mutually excluded/disabled automatically)
	testDB.Create(&models.V2RayClientSetting{Key: "evasion_fragment", Value: "true"})

	// Compile outbound config
	cfg := models.V2RayClientConfig{
		Protocol:    "vless",
		Address:     "example.com",
		Port:        443,
		Network:     "tcp",
		TLSSettings: `{"security":"tls","sni":"example.com"}`,
	}

	outbound := CompileOutbound(cfg, true, "proxy")

	// Verify padding is set
	if outbound.StreamSettings == nil || outbound.StreamSettings.TlsSettings == nil {
		t.Fatalf("expected stream settings and TLS settings to be configured")
	}
	tlsSettings := outbound.StreamSettings.TlsSettings
	if tlsSettings.Padding == nil {
		t.Errorf("expected Padding to be configured when evasion_padding is enabled")
	} else {
		if tlsSettings.Padding.Type != "random" || tlsSettings.Padding.Size != "100-500" {
			t.Errorf("unexpected padding configuration: type=%s size=%s", tlsSettings.Padding.Type, tlsSettings.Padding.Size)
		}
	}

	// Verify fragmentation is disabled due to mutual exclusion
	if outbound.StreamSettings.Fragment != nil {
		t.Errorf("expected Fragment to be nil due to mutual exclusion with padding")
	}

	// Compile Sing-Box outbound config
	sbOutbound := CompileSingBoxOutbound(cfg, true, "proxy")

	if sbOutbound.TLS == nil {
		t.Fatalf("expected Sing-Box TLS config to be configured")
	}
	if sbOutbound.TLS.Padding == nil {
		t.Errorf("expected Sing-Box TLS Padding to be configured when evasion_padding is enabled")
	} else {
		if !sbOutbound.TLS.Padding.Enabled || sbOutbound.TLS.Padding.Type != "random" || sbOutbound.TLS.Padding.Size != "100-500" {
			t.Errorf("unexpected Sing-Box padding configuration: enabled=%t type=%s size=%s", sbOutbound.TLS.Padding.Enabled, sbOutbound.TLS.Padding.Type, sbOutbound.TLS.Padding.Size)
		}
	}

	if sbOutbound.TLS.Fragment != nil {
		t.Errorf("expected Sing-Box Fragment to be nil due to mutual exclusion with padding")
	}
}
