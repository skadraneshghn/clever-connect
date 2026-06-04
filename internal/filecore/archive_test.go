package filecore

import (
	"context"
	"os"
	"path/filepath"
	"testing"
)

func TestCompressFilesAndExtract(t *testing.T) {
	// Create temporary directory for tests
	tmpDir, err := os.MkdirTemp("", "archive_test_*")
	if err != nil {
		t.Fatalf("failed to create temp dir: %v", err)
	}
	defer os.RemoveAll(tmpDir)

	// Create dummy files
	file1 := filepath.Join(tmpDir, "file1.txt")
	if err := os.WriteFile(file1, []byte("hello from file 1"), 0644); err != nil {
		t.Fatalf("failed to write file1: %v", err)
	}
	file2 := filepath.Join(tmpDir, "file2.txt")
	if err := os.WriteFile(file2, []byte("hello from file 2"), 0644); err != nil {
		t.Fatalf("failed to write file2: %v", err)
	}

	files := []string{file1, file2}

	formats := []struct {
		ext string
	}{
		{".zip"},
		{".tar"},
		{".tar.gz"},
		{".7z"},
		{".rar"},
	}

	for _, tc := range formats {
		t.Run(tc.ext, func(t *testing.T) {
			archivePath := filepath.Join(tmpDir, "archive"+tc.ext)

			err := CompressFiles(context.Background(), files, archivePath, func(progress int) {})
			if err != nil {
				t.Fatalf("failed to compress to %s: %v", tc.ext, err)
			}

			// Verify file exists and has size > 0
			fi, err := os.Stat(archivePath)
			if err != nil {
				t.Fatalf("archive file does not exist: %v", err)
			}
			if fi.Size() == 0 {
				t.Fatalf("archive is empty")
			}

			// Test extraction
			err = ExtractArchive(context.Background(), archivePath, "", func(progress int) {})
			if err != nil {
				t.Fatalf("failed to extract %s: %v", tc.ext, err)
			}

			// Verify extracted folder exists and contains files
			extractedFolder := filepath.Join(tmpDir, "archive")
			defer os.RemoveAll(extractedFolder)

			extFile1 := filepath.Join(extractedFolder, "file1.txt")
			if _, err := os.Stat(extFile1); err != nil {
				t.Fatalf("extracted file1 not found: %v", err)
			}
		})
	}
}
