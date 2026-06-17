package ehcocore

import (
	"testing"
	"time"

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

	if err := gdb.AutoMigrate(&models.EhcoServerConfig{}, &models.EhcoClientConfig{}); err != nil {
		t.Fatalf("failed to migrate schemas: %v", err)
	}

	db.DB = gdb
}

func TestEhcoSupervisor(t *testing.T) {
	setupTestDB(t)

	// Create test server configuration
	serverCfg := models.EhcoServerConfig{
		ListenPort: "23001",
		AuthToken:  "test-token",
		TargetMode: "direct",
		TargetHost: "127.0.0.1:23080",
		EnableMux:  true,
		KeepAlive:  10,
		IsActive:   true,
	}
	if err := db.DB.Create(&serverCfg).Error; err != nil {
		t.Fatalf("failed to create server config: %v", err)
	}

	// Make sure we stop the engine at the end of the test
	defer StopEngine()

	// Start the server engine
	if err := StartServerEngine(&serverCfg); err != nil {
		t.Fatalf("StartServerEngine failed: %v", err)
	}

	// Verify it is running
	if !IsRunning() {
		t.Fatalf("Expected IsRunning() to be true after starting")
	}

	// Simulate crash: kill the process
	mu.Lock()
	p := cmdInstance.Process
	mu.Unlock()

	if p != nil {
		// Kill the process
		if err := p.Kill(); err != nil {
			t.Fatalf("failed to kill subprocess: %v", err)
		}
	}

	// Wait for supervisor to detect crash and restart it
	// (backoff starts at 1 second, supervisor checks every 3 seconds)
	time.Sleep(5 * time.Second)

	// Verify it has been restarted automatically and is running again
	if !IsRunning() {
		t.Fatalf("Expected IsRunning() to be true after supervisor auto-restart")
	}
}
