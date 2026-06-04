package filecore

import (
	"archive/zip"
	"context"
	"fmt"
	"io"
	"io/fs"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"

	"github.com/mholt/archives"
)

var (
	rxPartSuffix1 = regexp.MustCompile(`(?i)\.part(\d+)\.(rar|zip|7z|tar\.gz)$`)
	rxPartSuffix2 = regexp.MustCompile(`(?i)\.(rar|zip|7z)\.part(\d+)$`)
	rxPartSuffix3 = regexp.MustCompile(`(?i)\.(\d+)$`)

	rxPartFolder1 = regexp.MustCompile(`(?i)\.part\d+\.(rar|zip|7z|tar\.gz)$`)
	rxPartFolder2 = regexp.MustCompile(`(?i)\.(rar|zip|7z)\.part\d+$`)
	rxPartFolder3 = regexp.MustCompile(`(?i)\.\d+$`)
)

// FindFirstMultiPart locates the first file segment of a multi-part archive sequence.
func FindFirstMultiPart(archivePath string) string {
	dir := filepath.Dir(archivePath)
	filename := filepath.Base(archivePath)

	var prefix string
	var matchType int

	if loc := rxPartSuffix1.FindStringIndex(filename); loc != nil {
		prefix = filename[:loc[0]]
		matchType = 1
	} else if loc := rxPartSuffix2.FindStringIndex(filename); loc != nil {
		prefix = filename[:loc[0]]
		matchType = 2
	} else if loc := rxPartSuffix3.FindStringIndex(filename); loc != nil {
		prefix = filename[:loc[0]]
		matchType = 3
	}

	if matchType == 0 {
		return archivePath
	}

	files, err := os.ReadDir(dir)
	if err != nil {
		return archivePath
	}

	var minPart int = -1
	var minFile string

	for _, f := range files {
		if f.IsDir() {
			continue
		}
		name := f.Name()
		if !strings.HasPrefix(name, prefix) {
			continue
		}

		var partNum int
		var found bool

		switch matchType {
		case 1:
			if loc := rxPartSuffix1.FindStringSubmatch(name); loc != nil {
				if num, err := strconv.Atoi(loc[1]); err == nil {
					partNum = num
					found = true
				}
			}
		case 2:
			if loc := rxPartSuffix2.FindStringSubmatch(name); loc != nil {
				if num, err := strconv.Atoi(loc[2]); err == nil {
					partNum = num
					found = true
				}
			}
		case 3:
			if loc := rxPartSuffix3.FindStringSubmatch(name); loc != nil {
				if num, err := strconv.Atoi(loc[1]); err == nil {
					partNum = num
					found = true
				}
			}
		}

		if found {
			if minPart == -1 || partNum < minPart {
				minPart = partNum
				minFile = name
			}
		}
	}

	if minFile != "" {
		return filepath.Join(dir, minFile)
	}

	return archivePath
}

// IsArchivePasswordProtected checks if an archive requires a password for extraction.
func IsArchivePasswordProtected(archivePath string) (bool, error) {
	archivePath = FindFirstMultiPart(archivePath)

	// 1. Check if Zip file using Go native library (fastest)
	if strings.HasSuffix(strings.ToLower(archivePath), ".zip") {
		zr, err := zip.OpenReader(archivePath)
		if err == nil {
			defer zr.Close()
			for _, f := range zr.File {
				if f.Flags & 1 != 0 {
					return true, nil
				}
			}
			return false, nil
		}
	}

	// 2. Use 7z to test extraction without password
	// If it fails with wrong password / header error, it is encrypted.
	if _, err := exec.LookPath("7z"); err == nil {
		cmd := exec.Command("7z", "t", "-p-", archivePath)
		output, _ := cmd.CombinedOutput()
		outStr := string(output)
		if strings.Contains(outStr, "Wrong password") || 
			strings.Contains(outStr, "Password?") || 
			strings.Contains(outStr, "encrypted file") || 
			strings.Contains(outStr, "Headers Error") {
			return true, nil
		}
	}
	return false, nil
}

// ExtractArchive extracts an archive into a folder named after the archive itself.
func ExtractArchive(ctx context.Context, archivePath string, password string, updateProgress func(progress int)) error {
	// First locate the first part if it's a multi-part archive
	archivePath = FindFirstMultiPart(archivePath)

	file, err := os.Open(archivePath)
	if err != nil {
		return err
	}
	defer file.Close()

	// Create a destination directory named after the archive (without extension and part suffix)
	baseName := filepath.Base(archivePath)
	if rxPartFolder1.MatchString(baseName) {
		baseName = rxPartFolder1.ReplaceAllString(baseName, "")
	} else if rxPartFolder2.MatchString(baseName) {
		baseName = rxPartFolder2.ReplaceAllString(baseName, "")
	} else if rxPartFolder3.MatchString(baseName) {
		baseName = rxPartFolder3.ReplaceAllString(baseName, "")
	} else {
		// Fallback to normal single archive naming
		baseName = strings.TrimSuffix(baseName, filepath.Ext(baseName))
		if strings.HasSuffix(strings.ToLower(filepath.Base(archivePath)), ".tar.gz") {
			baseName = strings.TrimSuffix(filepath.Base(archivePath), ".tar.gz")
		}
	}

	// Second pass: strip standard archive extensions if they remain (e.g. from .7z.001 -> .7z)
	lowerBase := strings.ToLower(baseName)
	if strings.HasSuffix(lowerBase, ".tar.gz") {
		baseName = baseName[:len(baseName)-7]
	} else if strings.HasSuffix(lowerBase, ".zip") || strings.HasSuffix(lowerBase, ".rar") || strings.HasSuffix(lowerBase, ".tar") {
		baseName = baseName[:len(baseName)-4]
	} else if strings.HasSuffix(lowerBase, ".7z") {
		baseName = baseName[:len(baseName)-3]
	}
	
	destDir := filepath.Join(filepath.Dir(archivePath), baseName)
	if err := os.MkdirAll(destDir, 0755); err != nil {
		return err
	}

	// Detect if it is a multi-part archive
	isMultiPart := false
	baseNameLower := strings.ToLower(filepath.Base(archivePath))
	if rxPartFolder1.MatchString(baseNameLower) || rxPartFolder2.MatchString(baseNameLower) || rxPartFolder3.MatchString(baseNameLower) {
		isMultiPart = true
	}

	// Always prefer 7z for multi-part archives or if a password is provided (if 7z is available)
	if isMultiPart || password != "" {
		if _, err := exec.LookPath("7z"); err == nil {
			cmdArgs := []string{"x"}
			if password != "" {
				cmdArgs = append(cmdArgs, "-p"+password)
			} else {
				cmdArgs = append(cmdArgs, "-p-")
			}
			cmdArgs = append(cmdArgs, "-y", "-o"+destDir, archivePath)
			cmd := exec.CommandContext(ctx, "7z", cmdArgs...)
			output, err := cmd.CombinedOutput()
			if err != nil {
				return fmt.Errorf("extraction with 7z failed: %s, %w", string(output), err)
			}
			updateProgress(100)
			return nil
		}
	}

	// Identify the format automatically (e.g., zip, tar.gz, rar)
	format, stream, err := archives.Identify(ctx, archivePath, file)
	if err != nil {
		return fmt.Errorf("unsupported archive format: %w", err)
	}

	extractor, ok := format.(archives.Extractor)
	if !ok {
		return fmt.Errorf("format does not support extraction")
	}

	// Configure password on native format if applicable and 7z was not used
	if password != "" {
		if rar, ok := format.(*archives.Rar); ok {
			rar.Password = password
		} else if sz, ok := format.(*archives.SevenZip); ok {
			sz.Password = password
		}
	}

	// Get file size for progress calculation
	stat, _ := file.Stat()
	totalBytes := stat.Size()
	var readBytes int64

	// Seek file back to the beginning before extracting
	_, _ = file.Seek(0, io.SeekStart)

	var archiveReader io.Reader = stream
	// Zip requires io.ReaderAt and io.Seeker
	if _, isZip := format.(archives.Zip); isZip {
		archiveReader = file
	}

	progressReader := &ProgressReadSeekerAt{
		reader: archiveReader,
		OnRead: func(n int) {
			readBytes += int64(n)
			if totalBytes > 0 {
				progress := int((float64(readBytes) / float64(totalBytes)) * 100)
				if progress > 100 {
					progress = 100
				}
				updateProgress(progress)
			}
		},
	}

	// Extract files
	err = extractor.Extract(ctx, progressReader, func(ctx context.Context, f archives.FileInfo) error {
		destPath := filepath.Join(destDir, f.NameInArchive)

		// Prevent ZipSlip vulnerability
		if !strings.HasPrefix(filepath.Clean(destPath), filepath.Clean(destDir)) {
			return fmt.Errorf("illegal file path: %s", destPath)
		}

		if f.IsDir() {
			return os.MkdirAll(destPath, f.Mode())
		}

		if err := os.MkdirAll(filepath.Dir(destPath), 0755); err != nil {
			return err
		}

		rc, err := f.Open()
		if err != nil {
			return err
		}
		defer rc.Close()

		outFile, err := os.OpenFile(destPath, os.O_CREATE|os.O_WRONLY|os.O_TRUNC, f.Mode())
		if err != nil {
			return err
		}
		defer outFile.Close()

		_, err = io.Copy(outFile, rc)
		return err
	})

	if err == nil {
		updateProgress(100)
	}

	return err
}

// CompressFiles archives a list of absolute file paths into a target destination.
func CompressFiles(ctx context.Context, filePaths []string, destArchivePath string, updateProgress func(progress int)) error {
	ext := strings.ToLower(filepath.Ext(destArchivePath))

	// 1. Handle RAR compression
	if ext == ".rar" {
		var rarPath string
		candidatePaths := []string{
			"./bin/rar-bin",
			"../bin/rar-bin",
			"../../bin/rar-bin",
		}
		for _, cp := range candidatePaths {
			absPath, err := filepath.Abs(cp)
			if err == nil {
				if _, err := os.Stat(absPath); err == nil {
					rarPath = absPath
					break
				}
			}
		}
		if rarPath == "" {
			return fmt.Errorf("rar-bin executable not found. Please ensure the rar binary is present in ./bin/")
		}

		var baseDir string
		if len(filePaths) > 0 {
			baseDir = filepath.Dir(filePaths[0])
		}

		cmdArgs := []string{"a", "-ep1", destArchivePath}
		for _, p := range filePaths {
			if rel, err := filepath.Rel(baseDir, p); err == nil {
				cmdArgs = append(cmdArgs, rel)
			} else {
				cmdArgs = append(cmdArgs, p)
			}
		}

		cmd := exec.CommandContext(ctx, rarPath, cmdArgs...)
		cmd.Dir = baseDir
		output, err := cmd.CombinedOutput()
		if err != nil {
			return fmt.Errorf("rar compression failed: %s, %w", string(output), err)
		}
		updateProgress(100)
		return nil
	}

	// 2. Handle 7Z compression
	if ext == ".7z" {
		sevenZipPath, err := exec.LookPath("7z")
		if err != nil {
			return fmt.Errorf("7z executable not found: %w", err)
		}

		var baseDir string
		if len(filePaths) > 0 {
			baseDir = filepath.Dir(filePaths[0])
		}

		cmdArgs := []string{"a", "-y", destArchivePath}
		for _, p := range filePaths {
			if rel, err := filepath.Rel(baseDir, p); err == nil {
				cmdArgs = append(cmdArgs, rel)
			} else {
				cmdArgs = append(cmdArgs, p)
			}
		}

		cmd := exec.CommandContext(ctx, sevenZipPath, cmdArgs...)
		cmd.Dir = baseDir
		output, err := cmd.CombinedOutput()
		if err != nil {
			return fmt.Errorf("7z compression failed: %s, %w", string(output), err)
		}
		updateProgress(100)
		return nil
	}

	// 3. Handle standard formats via mholt/archives
	outFile, err := os.Create(destArchivePath)
	if err != nil {
		return err
	}
	defer outFile.Close()

	var format archives.Archiver
	if strings.HasSuffix(strings.ToLower(destArchivePath), ".tar.gz") || strings.HasSuffix(strings.ToLower(destArchivePath), ".tgz") {
		format = archives.CompressedArchive{
			Compression: archives.Gz{},
			Archival:    archives.Tar{},
		}
	} else if strings.HasSuffix(strings.ToLower(destArchivePath), ".tar.bz2") || strings.HasSuffix(strings.ToLower(destArchivePath), ".tbz2") {
		format = archives.CompressedArchive{
			Compression: archives.Bz2{},
			Archival:    archives.Tar{},
		}
	} else if strings.HasSuffix(strings.ToLower(destArchivePath), ".tar.xz") || strings.HasSuffix(strings.ToLower(destArchivePath), ".txz") {
		format = archives.CompressedArchive{
			Compression: archives.Xz{},
			Archival:    archives.Tar{},
		}
	} else if strings.HasSuffix(strings.ToLower(destArchivePath), ".tar.zst") || strings.HasSuffix(strings.ToLower(destArchivePath), ".tzst") {
		format = archives.CompressedArchive{
			Compression: archives.Zstd{},
			Archival:    archives.Tar{},
		}
	} else if ext == ".tar" {
		format = archives.Tar{}
	} else {
		format = archives.Zip{}
	}

	// Map files to archives.FileInfo structure
	fileMap := make(map[string]string)
	for _, p := range filePaths {
		fileMap[p] = filepath.Base(p)
	}
	
	filesToArchive, err := archives.FilesFromDisk(ctx, nil, fileMap)
	if err != nil {
		return err
	}

	// Track total size of input files for compression progress estimation
	var totalSize int64
	for _, p := range filePaths {
		if fi, err := os.Stat(p); err == nil {
			if fi.IsDir() {
				_ = filepath.Walk(p, func(_ string, info fs.FileInfo, err error) error {
					if err == nil && !info.IsDir() {
						totalSize += info.Size()
					}
					return nil
				})
			} else {
				totalSize += fi.Size()
			}
		}
	}

	var writtenBytes int64
	progressWriter := &ProgressWriter{
		Writer: outFile,
		OnWrite: func(n int) {
			writtenBytes += int64(n)
			if totalSize > 0 {
				progress := int((float64(writtenBytes) / float64(totalSize)) * 100)
				if progress > 99 {
					progress = 99
				}
				updateProgress(progress)
			}
		},
	}

	err = format.Archive(ctx, progressWriter, filesToArchive)
	if err == nil {
		updateProgress(100)
	}
	return err
}

// ProgressReader wraps an io.Reader to track progress
type ProgressReader struct {
	io.Reader
	OnRead func(n int)
}

func (pr *ProgressReader) Read(p []byte) (n int, err error) {
	n, err = pr.Reader.Read(p)
	if n > 0 && pr.OnRead != nil {
		pr.OnRead(n)
	}
	return n, err
}

type ProgressReadSeekerAt struct {
	reader io.Reader
	OnRead func(n int)
}

func (pr *ProgressReadSeekerAt) Read(p []byte) (n int, err error) {
	n, err = pr.reader.Read(p)
	if n > 0 && pr.OnRead != nil {
		pr.OnRead(n)
	}
	return n, err
}

func (pr *ProgressReadSeekerAt) ReadAt(p []byte, off int64) (n int, err error) {
	if ra, ok := pr.reader.(io.ReaderAt); ok {
		n, err = ra.ReadAt(p, off)
		if n > 0 && pr.OnRead != nil {
			pr.OnRead(n)
		}
		return n, err
	}
	return 0, fmt.Errorf("underlying reader does not support ReadAt")
}

func (pr *ProgressReadSeekerAt) Seek(offset int64, whence int) (int64, error) {
	if s, ok := pr.reader.(io.Seeker); ok {
		return s.Seek(offset, whence)
	}
	return 0, fmt.Errorf("underlying reader does not support Seek")
}

// ProgressWriter wraps an io.Writer to track progress
type ProgressWriter struct {
	io.Writer
	OnWrite func(n int)
}

func (pw *ProgressWriter) Write(p []byte) (n int, err error) {
	n, err = pw.Writer.Write(p)
	if n > 0 && pw.OnWrite != nil {
		pw.OnWrite(n)
	}
	return n, err
}
