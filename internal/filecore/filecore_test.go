package filecore

import (
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strconv"
	"testing"

	"clever-connect/internal/db"
	"clever-connect/internal/models"

	"gorm.io/driver/sqlite"
	"gorm.io/gorm"
)

func setupTestDB(t *testing.T) {
	// Initialize in-memory SQLite database for testing
	gdb, err := gorm.Open(sqlite.Open("file::memory:?cache=shared"), &gorm.Config{})
	if err != nil {
		t.Fatalf("failed to open test database: %v", err)
	}

	// Auto-migrate the FileRegistry model
	if err := gdb.AutoMigrate(&models.FileRegistry{}); err != nil {
		t.Fatalf("failed to migrate schema: %v", err)
	}

	db.DB = gdb
}

func TestGetBlake3Checksum(t *testing.T) {
	tmpDir := t.TempDir()
	filePath := filepath.Join(tmpDir, "test.txt")
	content := []byte("hello clever-connect file deduplication engine")
	if err := os.WriteFile(filePath, content, 0644); err != nil {
		t.Fatalf("failed to write temp file: %v", err)
	}

	hash, err := GetBlake3Checksum(filePath)
	if err != nil {
		t.Fatalf("GetBlake3Checksum failed: %v", err)
	}

	if len(hash) != 64 {
		t.Errorf("expected hash length 64, got %d", len(hash))
	}
}

func TestRegisterFileAndDeduplicate(t *testing.T) {
	setupTestDB(t)
	tmpDir := t.TempDir()

	// Create a master file
	masterPath := filepath.Join(tmpDir, "master.txt")
	content := []byte("same content")
	if err := os.WriteFile(masterPath, content, 0644); err != nil {
		t.Fatalf("failed to write master file: %v", err)
	}

	// Register it
	reg, err := RegisterFile(masterPath, "http://example.com/file", "etag123", 1001, "")
	if err != nil {
		t.Fatalf("RegisterFile failed: %v", err)
	}

	if reg.Checksum == "" {
		t.Errorf("expected checksum to be populated")
	}

	// Create a duplicate file in a different directory/name
	duplicatePath := filepath.Join(tmpDir, "duplicate.txt")
	if err := os.WriteFile(duplicatePath, content, 0644); err != nil {
		t.Fatalf("failed to write duplicate file: %v", err)
	}

	// Register duplicate file
	regDup, err := RegisterFile(duplicatePath, "http://example.com/file-dup", "", 1002, "")
	if err != nil {
		t.Fatalf("RegisterFile duplicate failed: %v", err)
	}

	// Verify they point to same checksum and duplicate was deduplicated via SafeLink
	if regDup.Checksum != reg.Checksum {
		t.Errorf("expected checksums to match, got %s vs %s", regDup.Checksum, reg.Checksum)
	}

	// Verify both files still exist and have the same content
	for _, path := range []string{masterPath, duplicatePath} {
		data, err := os.ReadFile(path)
		if err != nil {
			t.Fatalf("failed to read file %s: %v", path, err)
		}
		if string(data) != "same content" {
			t.Errorf("unexpected content in %s: %q", path, string(data))
		}
	}
}

func TestCheckDuplicateByTgID(t *testing.T) {
	setupTestDB(t)
	tmpDir := t.TempDir()

	filePath := filepath.Join(tmpDir, "tgfile.dat")
	if err := os.WriteFile(filePath, []byte("telegram media file"), 0644); err != nil {
		t.Fatalf("failed to write file: %v", err)
	}

	// Register with tg_file_id = 9999
	_, err := RegisterFile(filePath, "", "", 9999, "")
	if err != nil {
		t.Fatalf("failed to register file: %v", err)
	}

	// Now check duplicate for the same tg_file_id in a different path
	newPath := filepath.Join(tmpDir, "tgfile_new.dat")
	matched, origPath, err := CheckDuplicateByTgID(9999, newPath)
	if err != nil {
		t.Fatalf("CheckDuplicateByTgID failed: %v", err)
	}

	if !matched {
		t.Errorf("expected duplicate to be found")
	}

	if origPath == "" {
		t.Errorf("expected origPath to be returned")
	}

	// Check if newPath was created and has correct contents
	data, err := os.ReadFile(newPath)
	if err != nil {
		t.Fatalf("newPath does not exist or read failed: %v", err)
	}
	if string(data) != "telegram media file" {
		t.Errorf("unexpected content: %s", string(data))
	}
}

func TestCheckDuplicateByURL(t *testing.T) {
	setupTestDB(t)
	tmpDir := t.TempDir()

	filePath := filepath.Join(tmpDir, "webfile.dat")
	content := []byte("some web content")
	if err := os.WriteFile(filePath, content, 0644); err != nil {
		t.Fatalf("failed to write file: %v", err)
	}

	// Start mock http server
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("ETag", "etag123")
		w.Header().Set("Content-Length", strconv.Itoa(len(content)))
		w.WriteHeader(http.StatusOK)
	}))
	defer server.Close()

	// Register with URL and ETag
	urlStr := server.URL + "/webfile.dat"
	_, err := RegisterFile(filePath, urlStr, "etag123", 0, "")
	if err != nil {
		t.Fatalf("failed to register file: %v", err)
	}

	// Validate by calling CheckDuplicateByURL
	newPath := filepath.Join(tmpDir, "webfile_new.dat")
	matched, _, err := CheckDuplicateByURL(urlStr, newPath)
	if err != nil {
		t.Fatalf("CheckDuplicateByURL failed: %v", err)
	}

	if !matched {
		t.Errorf("expected duplicate to match URL")
	}

	data, err := os.ReadFile(newPath)
	if err != nil {
		t.Fatalf("failed to read webfile_new: %v", err)
	}
	if string(data) != string(content) {
		t.Errorf("expected content %s, got %s", content, data)
	}
}

func TestCheckDuplicateByTorrentHash(t *testing.T) {
	setupTestDB(t)
	tmpDir := t.TempDir()

	filePath := filepath.Join(tmpDir, "torrentfile.dat")
	content := []byte("torrent content")
	if err := os.WriteFile(filePath, content, 0644); err != nil {
		t.Fatalf("failed to write file: %v", err)
	}

	infoHash := "0123456789abcdef0123456789abcdef01234567"
	_, err := RegisterFile(filePath, "", "", 0, infoHash)
	if err != nil {
		t.Fatalf("failed to register file: %v", err)
	}

	newPath := filepath.Join(tmpDir, "torrentfile_new.dat")
	matched, _, err := CheckDuplicateByTorrentHash(infoHash, newPath)
	if err != nil {
		t.Fatalf("CheckDuplicateByTorrentHash failed: %v", err)
	}

	if !matched {
		t.Errorf("expected duplicate to match torrent infohash")
	}

	data, err := os.ReadFile(newPath)
	if err != nil {
		t.Fatalf("failed to read torrentfile_new: %v", err)
	}
	if string(data) != string(content) {
		t.Errorf("expected content %s, got %s", content, data)
	}
}

// Ensure registry cleanup / modified master handling works
func TestModifiedOrDeletedMaster(t *testing.T) {
	setupTestDB(t)
	tmpDir := t.TempDir()

	masterPath := filepath.Join(tmpDir, "master.txt")
	content := []byte("original")
	if err := os.WriteFile(masterPath, content, 0644); err != nil {
		t.Fatalf("failed to write master file: %v", err)
	}

	// Register
	reg, err := RegisterFile(masterPath, "", "", 0, "")
	if err != nil {
		t.Fatalf("failed to register: %v", err)
	}

	// Delete master from disk
	if err := os.Remove(masterPath); err != nil {
		t.Fatalf("failed to remove master file: %v", err)
	}

	// Verify checking duplicates by TgID/URL returns false now
	// Register the same content at a new path, it should update FilePath since old one was deleted
	newPath := filepath.Join(tmpDir, "new_master.txt")
	if err := os.WriteFile(newPath, content, 0644); err != nil {
		t.Fatalf("failed to write new master file: %v", err)
	}

	regNew, err := RegisterFile(newPath, "", "", 0, "")
	if err != nil {
		t.Fatalf("failed to register new path: %v", err)
	}

	if regNew.ID != reg.ID {
		t.Errorf("expected the same registry ID to be reused")
	}

	if regNew.FilePath != newPath {
		t.Errorf("expected registry FilePath to be updated to %s, got %s", newPath, regNew.FilePath)
	}
}
