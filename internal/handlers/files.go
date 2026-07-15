package handlers

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"syscall"
	"time"

	"clever-connect/internal/config"
	"clever-connect/internal/db"
	"clever-connect/internal/filecore"
	"clever-connect/internal/logger"
	"clever-connect/internal/models"
	"clever-connect/internal/scheduler"
	"clever-connect/internal/torrent"
	anacrolixTorrent "github.com/anacrolix/torrent"
	"github.com/gin-gonic/gin"
)

type FileItem struct {
	Name      string    `json:"name"`
	IsDir     bool      `json:"is_dir"`
	Size      int64     `json:"size"`
	ModTime   time.Time `json:"mod_time"`
	Extension string    `json:"extension"`
	S3Key     string    `json:"s3_key"` // non-empty when the file is archived in S3 object storage
}

type FileHandler struct {
	cfg     *config.Config
	rootDir string
}

func NewFileHandler(cfg *config.Config) *FileHandler {
	rootDir, err := filepath.Abs("./data/manager")
	if err != nil {
		rootDir = "./data/manager"
	}
	// Ensure the root path exists
	if err := os.MkdirAll(rootDir, 0755); err != nil {
		logger.Error("Files", "Failed to create root directory", "error", err)
	}

	logger.Info("Files", "Initialized file manager base directory", "rootDir", rootDir)
	return &FileHandler{
		cfg:     cfg,
		rootDir: rootDir,
	}
}

// securePath guarantees that no user can bypass the sandbox rootDir boundary
func (h *FileHandler) securePath(requestedPath string) (string, error) {
	// Ensure absolute root format in local context
	cleanRel := filepath.Clean("/" + requestedPath)
	fullPath := filepath.Clean(filepath.Join(h.rootDir, cleanRel))

	// Guard against directory traversal attacks
	if !strings.HasPrefix(fullPath, h.rootDir) {
		return "", os.ErrPermission
	}
	return fullPath, nil
}

func (h *FileHandler) proxyToServer(c *gin.Context, method string, apiPath string) bool {
	if h.cfg.AppMode == "server" {
		return false
	}

	if h.cfg.ServerURL == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "No remote server API connection configured (missing SERVER_URL in environment)"})
		return true
	}

	remoteURLTarget := strings.TrimSpace(h.cfg.ServerURL)
	remoteToken := strings.TrimSpace(h.cfg.ServerAuthToken)

	// Convert ws/wss to http/https
	remoteHost := remoteURLTarget
	remoteHost = strings.Replace(remoteHost, "wss://", "https://", 1)
	remoteHost = strings.Replace(remoteHost, "ws://", "http://", 1)

	// Strip trailing path segments like /ws or /tunnel
	if idx := strings.Index(remoteHost, "/ws"); idx != -1 {
		remoteHost = remoteHost[:idx]
	}
	if idx := strings.Index(remoteHost, "/tunnel"); idx != -1 {
		remoteHost = remoteHost[:idx]
	}
	// Strip trailing slashes
	remoteHost = strings.TrimSuffix(remoteHost, "/")

	// Build remote URL
	remoteURL := remoteHost + apiPath
	if c.Request.URL.RawQuery != "" {
		remoteURL += "?" + c.Request.URL.RawQuery
	}

	var reqBody io.Reader
	if method != http.MethodGet && method != http.MethodHead && method != http.MethodOptions && method != http.MethodDelete {
		reqBody = c.Request.Body
	}

	// Create proxy request
	req, err := http.NewRequest(method, remoteURL, reqBody)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to create proxy request", "details": err.Error()})
		return true
	}

	// Copy original request headers
	for k, vv := range c.Request.Header {
		for _, v := range vv {
			req.Header.Add(k, v)
		}
	}

	// Overwrite local credentials with the actual remote server's Ehco client auth_token!
	if remoteToken != "" {
		req.Header.Set("Authorization", "Bearer " + remoteToken)
	}

	// Execute proxy request to remote server
	client := &http.Client{}
	resp, err := client.Do(req)
	if err != nil {
		c.JSON(http.StatusBadGateway, gin.H{"error": "Remote server connection refused or timed out", "details": err.Error()})
		return true
	}
	defer resp.Body.Close()

	if resp.StatusCode == http.StatusUnauthorized {
		c.JSON(http.StatusBadGateway, gin.H{"error": "Remote server rejected proxy token (401). Please update the remote server or verify your Auth Token."})
		return true
	}

	// Copy response headers
	for k, vv := range resp.Header {
		for _, v := range vv {
			c.Writer.Header().Add(k, v)
		}
	}
	c.Writer.WriteHeader(resp.StatusCode)

	// Pipe remote file stream/content back directly
	_, _ = io.Copy(c.Writer, resp.Body)
	return true
}

// getDiskInfo queries the file system statistics using syscall.Statfs.
func getDiskInfo(path string) (total uint64, free uint64, used uint64) {
	var stat syscall.Statfs_t
	if err := syscall.Statfs(path, &stat); err == nil {
		total = stat.Blocks * uint64(stat.Bsize)
		free = stat.Bfree * uint64(stat.Bsize)
		used = total - free
	}
	return
}

func (h *FileHandler) findActiveTorrentFile(absolutePath string) (*anacrolixTorrent.File, bool) {
	if torrent.Manager == nil || torrent.Manager.Client() == nil {
		return nil, false
	}

	cleanPath := filepath.Clean(absolutePath)

	// Fetch all jobs to know their save directories
	var jobs []models.TorrentJob
	if err := db.DB.Find(&jobs).Error; err != nil {
		return nil, false
	}

	jobMap := make(map[string]string) // infoHash -> saveDir
	for _, job := range jobs {
		jobMap[job.InfoHash] = job.SaveDirectory
	}

	for _, t := range torrent.Manager.Client().Torrents() {
		infoHash := t.InfoHash().HexString()
		saveDir, ok := jobMap[infoHash]
		if !ok {
			saveDir = "./data/manager/downloads"
		}
		absSaveDir, err := filepath.Abs(saveDir)
		if err != nil {
			absSaveDir = saveDir
		}

		select {
		case <-t.GotInfo():
			files := t.Files()
			for i := range files {
				torrentFilePath := filepath.Clean(filepath.Join(absSaveDir, files[i].Path()))
				if torrentFilePath == cleanPath {
					return files[i], true
				}
			}
		default:
			// Info not resolved yet
		}
	}
	return nil, false
}

// mergeS3VirtualFiles injects S3-backed FileRegistry records into a directory
// listing as virtual entries. This is what makes the file manager browseable
// when the stateless leecher stores files only in object storage: even though
// the local copy was deleted, the file (or the virtual folder leading to it)
// still shows up here.
func (h *FileHandler) mergeS3VirtualFiles(safeDir string, virtualFiles map[string]FileItem) {
	if !filecore.IsS3Enabled() {
		return
	}

	prefix := safeDir
	if !strings.HasSuffix(prefix, string(filepath.Separator)) {
		prefix += string(filepath.Separator)
	}

	var records []models.FileRegistry
	if err := db.DB.Where("s3_key <> '' AND file_path LIKE ?", prefix+"%").
		Find(&records).Error; err != nil {
		return
	}

	for _, rec := range records {
		rel := strings.TrimPrefix(rec.FilePath, prefix)
		if rel == "" || rel == rec.FilePath {
			continue
		}
		parts := strings.SplitN(filepath.ToSlash(rel), "/", 2)
		name := parts[0]
		if name == "" {
			continue
		}
		// A physical or torrent-virtual entry already wins for this name.
		if _, exists := virtualFiles[name]; exists {
			continue
		}
		if len(parts) == 1 {
			// The object lives directly in this directory.
			virtualFiles[name] = FileItem{
				Name:      name,
				IsDir:     false,
				Size:      rec.FileSize,
				ModTime:   rec.CreatedAt,
				Extension: filepath.Ext(name),
				S3Key:     rec.S3Key,
			}
		} else {
			// The object lives in a deeper path — expose its top-level folder.
			virtualFiles[name] = FileItem{
				Name:    name,
				IsDir:   true,
				Size:    0,
				ModTime: rec.CreatedAt,
			}
		}
	}
}

// ListDirectory handles GET /api/files/list
func (h *FileHandler) ListDirectory(c *gin.Context) {
	if h.proxyToServer(c, c.Request.Method, c.Request.URL.Path) {
		return
	}
	reqPath := c.DefaultQuery("path", "")
	safePath, err := h.securePath(reqPath)
	if err != nil {
		logger.Warn("Files", "Access denied — directory traversal attempt detected", "path", reqPath, "ip", c.ClientIP())
		c.JSON(http.StatusForbidden, gin.H{"error": "Access denied"})
		return
	}

	entries, err := os.ReadDir(safePath)
	if err != nil && !os.IsNotExist(err) {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to read directory", "details": err.Error()})
		return
	}

	files := make([]FileItem, 0, len(entries))
	for _, entry := range entries {
		info, err := entry.Info()
		if err != nil {
			continue
		}

		files = append(files, FileItem{
			Name:      entry.Name(),
			IsDir:     entry.IsDir(),
			Size:      info.Size(),
			ModTime:   info.ModTime(),
			Extension: filepath.Ext(entry.Name()),
		})
	}

	// Annotate physical files with their S3 key so the UI can show an
	// S3-storage badge.  Batch-query the registry for all S3-backed records
	// whose file_path falls under this directory.
	if filecore.IsS3Enabled() {
		prefix := safePath
		if !strings.HasSuffix(prefix, string(filepath.Separator)) {
			prefix += string(filepath.Separator)
		}
		var s3Records []models.FileRegistry
		db.DB.Where("s3_key <> '' AND file_path LIKE ?", prefix+"%").Find(&s3Records)
		for _, rec := range s3Records {
			rel := strings.TrimPrefix(rec.FilePath, prefix)
			if rel == "" || rel == rec.FilePath || strings.Contains(filepath.ToSlash(rel), "/") {
				continue // not a direct child or in a subdirectory
			}
			for i := range files {
				if files[i].Name == rel && !files[i].IsDir {
					files[i].S3Key = rec.S3Key
					break
				}
			}
		}
	}

	// Merge in virtual files for active torrents that should be in this folder
	virtualFiles := make(map[string]FileItem)
	if torrent.Manager != nil && torrent.Manager.Client() != nil {
		var jobs []models.TorrentJob
		if err := db.DB.Find(&jobs).Error; err == nil {
			jobMap := make(map[string]string)
			for _, job := range jobs {
				jobMap[job.InfoHash] = job.SaveDirectory
			}

			for _, t := range torrent.Manager.Client().Torrents() {
				infoHash := t.InfoHash().HexString()
				saveDir, ok := jobMap[infoHash]
				if !ok {
					saveDir = "./data/manager/downloads"
				}
				absSaveDir, err := filepath.Abs(saveDir)
				if err != nil {
					absSaveDir = saveDir
				}

				select {
				case <-t.GotInfo():
					for _, f := range t.Files() {
						torrentFilePath := filepath.Clean(filepath.Join(absSaveDir, f.Path()))
						parentDir := filepath.Dir(torrentFilePath)

						if parentDir == safePath {
							name := filepath.Base(torrentFilePath)
							virtualFiles[name] = FileItem{
								Name:      name,
								IsDir:     false,
								Size:      f.Length(),
								ModTime:   time.Now(),
								Extension: filepath.Ext(name),
							}
						} else if strings.HasPrefix(parentDir, safePath) {
							rel, err := filepath.Rel(safePath, parentDir)
							if err == nil && rel != "." && rel != ".." {
								parts := strings.Split(filepath.ToSlash(rel), "/")
								if len(parts) > 0 && parts[0] != "" {
									dirName := parts[0]
									virtualFiles[dirName] = FileItem{
										Name:      dirName,
										IsDir:     true,
										Size:      0,
										ModTime:   time.Now(),
										Extension: "",
									}
								}
							}
						}
					}
				default:
				}
			}
		}
	}

	// Merge S3-backed registry records as virtual entries. Files archived to
	// S3 by the stateless leecher (local copy removed) still appear here so the
	// file manager stays fully browsable from object storage.
	h.mergeS3VirtualFiles(safePath, virtualFiles)

	// Merge virtual files with physical ones
	for _, vf := range virtualFiles {
		foundIdx := -1
		for idx, pf := range files {
			if pf.Name == vf.Name {
				foundIdx = idx
				break
			}
		}

		if foundIdx != -1 {
			if !files[foundIdx].IsDir && vf.Size > files[foundIdx].Size {
				files[foundIdx].Size = vf.Size
			}
		} else {
			files = append(files, vf)
		}
	}

	// Clean standard absolute visual path for display
	displayPath := filepath.Clean("/" + reqPath)
	if displayPath == "." {
		displayPath = "/"
	}

	diskTotal, diskFree, diskUsed := getDiskInfo(h.rootDir)

	response := gin.H{
		"current_path": displayPath,
		"files":        files,
		"disk_total":   diskTotal,
		"disk_free":    diskFree,
		"disk_used":    diskUsed,
		"s3_enabled":   filecore.IsS3Enabled(),
	}

	// Include S3 bucket usage stats so the file manager UI can show them
	// alongside local disk info.
	if filecore.IsS3Enabled() {
		var s3Count int64
		var s3Size int64
		db.DB.Model(&models.FileRegistry{}).Where("s3_key <> ''").Count(&s3Count)
		db.DB.Model(&models.FileRegistry{}).Where("s3_key <> ''").
			Select("COALESCE(SUM(file_size), 0)").Scan(&s3Size)
		response["s3_object_count"] = s3Count
		response["s3_total_size"] = s3Size
	}

	c.JSON(http.StatusOK, response)
}

// SearchFiles handles GET /api/files/search
func (h *FileHandler) SearchFiles(c *gin.Context) {
	if h.proxyToServer(c, c.Request.Method, c.Request.URL.Path) {
		return
	}
	reqPath := c.DefaultQuery("path", "")
	query := c.DefaultQuery("q", "")

	if len(query) <= 3 {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Query must be more than 3 characters"})
		return
	}

	safePath, err := h.securePath(reqPath)
	if err != nil {
		c.JSON(http.StatusForbidden, gin.H{"error": "Access denied"})
		return
	}

	results := make([]gin.H, 0)
	limit := 100

	err = filepath.WalkDir(safePath, func(path string, d os.DirEntry, err error) error {
		if err != nil {
			return nil
		}
		if len(results) >= limit {
			return filepath.SkipDir
		}
		name := d.Name()
		if strings.Contains(strings.ToLower(name), strings.ToLower(query)) {
			info, err := d.Info()
			if err != nil {
				return nil
			}
			relPath, err := filepath.Rel(h.rootDir, path)
			if err != nil {
				relPath = name
			}
			relPath = "/" + filepath.ToSlash(relPath)
			results = append(results, gin.H{
				"name":      name,
				"is_dir":    d.IsDir(),
				"size":      info.Size(),
				"mod_time":  info.ModTime(),
				"extension": filepath.Ext(name),
				"path":      relPath,
			})
		}
		return nil
	})

	if err != nil && err != filepath.SkipDir {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Search failed", "details": err.Error()})
		return
	}

	// Also search S3-backed files in the database — these have no local copy so
	// WalkDir above cannot see them, but their metadata is fully queryable.
	if filecore.IsS3Enabled() {
		var s3Records []models.FileRegistry
		likeQuery := "%" + query + "%"
		// Search by file_path basename (the filename is part of the path) and
		// limit to files under the requested directory prefix.
		dbErr := db.DB.Where(
			"s3_key <> '' AND file_path LIKE ? AND LOWER(file_path) LIKE LOWER(?)",
			safePath+"%",
			"%"+likeQuery,
		).Limit(limit - len(results)).Find(&s3Records).Error
		if dbErr == nil {
			seen := make(map[string]struct{}, len(results))
			for _, r := range results {
				if name, ok := r["name"].(string); ok {
					seen[name] = struct{}{}
				}
			}
			for _, rec := range s3Records {
				name := filepath.Base(rec.FilePath)
				if _, already := seen[name]; already {
					continue
				}
				relPath, _ := filepath.Rel(h.rootDir, rec.FilePath)
				if relPath == "" {
					relPath = name
				}
				relPath = "/" + filepath.ToSlash(relPath)
				results = append(results, gin.H{
					"name":      name,
					"is_dir":    false,
					"size":      rec.FileSize,
					"mod_time":  rec.CreatedAt,
					"extension": filepath.Ext(name),
					"path":      relPath,
					"s3_key":    rec.S3Key,
				})
				seen[name] = struct{}{}
			}
		}
	}

	c.JSON(http.StatusOK, results)
}

// StreamOrDownload handles GET /api/files/stream
// Crucial: Automatically handles HTTP Range headers (HTTP 206 Partial Content)
// for HLS/MP4 video streaming seeking and multi-connection fast download engines!
func (h *FileHandler) StreamOrDownload(c *gin.Context) {
	if h.proxyToServer(c, c.Request.Method, c.Request.URL.Path) {
		return
	}
	target := c.Query("path")
	safePath, err := h.securePath(target)
	if err != nil {
		c.JSON(http.StatusForbidden, gin.H{"error": "Access denied"})
		return
	}

	// Set high-performance HTTP streaming headers
	c.Header("Accept-Ranges", "bytes")
	c.Header("Connection", "keep-alive")
	c.Header("Cache-Control", "public, max-age=3600")

	// 1. Check if the file is part of an active torrent and is still downloading!
	if tFile, found := h.findActiveTorrentFile(safePath); found {
		if tFile.BytesCompleted() < tFile.Length() {
			if c.Query("download") == "true" {
				c.Header("Content-Disposition", "attachment; filename=\""+filepath.Base(safePath)+"\"")
			}

			reader := tFile.NewReader()
			reader.SetReadahead(32 * 1024 * 1024) // 32MB aggressive read-ahead buffer for zero stuttering
			defer reader.Close()

			// Stream content using the torrent client's reader
			http.ServeContent(c.Writer, c.Request, filepath.Base(safePath), time.Now(), reader)
			return
		}
	}

	// 2. If the file is mirrored in S3 object storage, stream it straight from
	// there with HTTP Range passthrough (sub-second seeking, zero disk I/O).
	// This is the fast path on Clever Cloud where local disk is ephemeral.
	if reg, ok := filecore.LookupRegistryByPath(safePath); ok {
		if filecore.IsS3Stored(reg) {
			disposition := ""
			if c.Query("download") == "true" {
				disposition = `attachment; filename="` + filepath.Base(safePath) + `"`
			}
			if filecore.StreamS3Object(c.Request.Context(), c.Writer, reg.S3Key,
				c.GetHeader("Range"), reg.MimeType, disposition) {
				return
			}
			// S3 fetch failed — fall through to local disk below.
		} else {
			// Registry record found but S3 upload not yet complete (upload in
			// progress or failed). If local copy is also missing, surface a
			// clear 503 instead of a cryptic 404.
			if _, statErr := os.Stat(safePath); statErr != nil {
				c.JSON(http.StatusServiceUnavailable, gin.H{
					"error":   "File is being archived to object storage — please retry in a moment",
					"details": "The file has been downloaded but its S3 upload has not yet completed.",
				})
				return
			}
		}
	}

	// 3. Fall back to standard disk file streaming (either non-torrent file, or fully completed torrent file)
	file, err := os.Open(safePath)
	if err != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "File not found"})
		return
	}
	defer file.Close()

	stat, err := file.Stat()
	if err != nil || stat.IsDir() {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Invalid request target"})
		return
	}

	// Forces browser to download instead of streaming if download query is specified
	if c.Query("download") == "true" {
		c.Header("Content-Disposition", "attachment; filename=\""+filepath.Base(safePath)+"\"")
	}

	// Using the optimal standard http.ServeContent seeking framework
	http.ServeContent(c.Writer, c.Request, stat.Name(), stat.ModTime(), file)
}

// RawDownload handles GET /api/files/download
// It serves the file aggressively from disk directly without any stream mechanisms (no HTTP Range support, no torrent reader readahead, etc.)
// When the file is mirrored to S3, it 302-redirects to a short-lived presigned
// URL so the client downloads directly from Cellar, fully offloading the server.
func (h *FileHandler) RawDownload(c *gin.Context) {
	if h.proxyToServer(c, c.Request.Method, c.Request.URL.Path) {
		return
	}
	target := c.Query("path")
	safePath, err := h.securePath(target)
	if err != nil {
		c.JSON(http.StatusForbidden, gin.H{"error": "Access denied"})
		return
	}

	// Fast path: redirect straight to a presigned S3 URL when available.
	if reg, ok := filecore.LookupRegistryByPath(safePath); ok && filecore.IsS3Stored(reg) {
		baseName := filepath.Base(safePath)
		if url := filecore.PresignDownloadRedirect(c.Request.Context(), reg.S3Key, baseName, time.Hour); url != "" {
			c.Redirect(http.StatusFound, url)
			return
		}
		// Presign failed — fall through to local disk below.
	}

	file, err := os.Open(safePath)
	if err != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "File not found"})
		return
	}
	defer file.Close()

	stat, err := file.Stat()
	if err != nil || stat.IsDir() {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Invalid request target"})
		return
	}

	// Set headers for raw download
	c.Header("Content-Description", "File Transfer")
	c.Header("Content-Type", "application/octet-stream")
	c.Header("Content-Disposition", "attachment; filename=\""+filepath.Base(safePath)+"\"")
	c.Header("Content-Length", fmt.Sprintf("%d", stat.Size()))
	c.Header("Expires", "0")
	c.Header("Cache-Control", "must-revalidate")
	c.Header("Pragma", "public")

	// Read and write directly to response writer aggressively (without range/stream support)
	_, _ = io.Copy(c.Writer, file)
}

// GetContent handles GET /api/files/content for text editor integrations
func (h *FileHandler) GetContent(c *gin.Context) {
	if h.proxyToServer(c, c.Request.Method, c.Request.URL.Path) {
		return
	}
	target := c.Query("path")
	safePath, err := h.securePath(target)
	if err != nil {
		c.JSON(http.StatusForbidden, gin.H{"error": "Access denied"})
		return
	}

	stat, err := os.Stat(safePath)
	if err != nil || stat.IsDir() {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Invalid target path"})
		return
	}

	// Prevent reading huge files into memory (max 10MB edit limit)
	if stat.Size() > 10*1024*1024 {
		c.JSON(http.StatusBadRequest, gin.H{"error": "File size exceeds 10MB limit"})
		return
	}

	contentBytes, err := os.ReadFile(safePath)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to read file", "details": err.Error()})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"content": string(contentBytes),
	})
}

// SaveContent handles POST /api/files/save to write changes back from text editors
func (h *FileHandler) SaveContent(c *gin.Context) {
	if h.proxyToServer(c, c.Request.Method, c.Request.URL.Path) {
		return
	}
	var req struct {
		Path    string `json:"path" binding:"required"`
		Content string `json:"content"`
	}

	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Invalid request payload"})
		return
	}

	safePath, err := h.securePath(req.Path)
	if err != nil {
		c.JSON(http.StatusForbidden, gin.H{"error": "Access denied"})
		return
	}

	// Ensure it's a file, not a directory
	stat, err := os.Stat(safePath)
	if err == nil && stat.IsDir() {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Cannot overwrite directory with text file content"})
		return
	}

	if err := os.WriteFile(safePath, []byte(req.Content), 0644); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to save file", "details": err.Error()})
		return
	}

	logger.Info("Files", "File content updated successfully", "path", req.Path, "ip", c.ClientIP())
	c.JSON(http.StatusOK, gin.H{"status": "success", "message": "File saved successfully"})
}

// CreateFolder handles POST /api/files/create-folder
func (h *FileHandler) CreateFolder(c *gin.Context) {
	if h.proxyToServer(c, c.Request.Method, c.Request.URL.Path) {
		return
	}
	var req struct {
		ParentPath string `json:"parent_path"`
		FolderName string `json:"folder_name" binding:"required"`
	}

	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Invalid request payload"})
		return
	}

	// Sanitize parent path and target folder name
	targetPath := filepath.Join(req.ParentPath, req.FolderName)
	safePath, err := h.securePath(targetPath)
	if err != nil {
		c.JSON(http.StatusForbidden, gin.H{"error": "Access denied"})
		return
	}

	if err := os.MkdirAll(safePath, 0755); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to create directory", "details": err.Error()})
		return
	}

	logger.Info("Files", "Directory created successfully", "path", targetPath, "ip", c.ClientIP())
	c.JSON(http.StatusOK, gin.H{"status": "success", "message": "Folder created successfully"})
}

// DeleteItem handles POST /api/files/delete
func (h *FileHandler) DeleteItem(c *gin.Context) {
	if h.proxyToServer(c, c.Request.Method, c.Request.URL.Path) {
		return
	}
	var req struct {
		Path string `json:"path" binding:"required"`
	}

	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Invalid request payload"})
		return
	}

	// Prevent deleting the root directory
	if req.Path == "" || req.Path == "/" {
		c.JSON(http.StatusForbidden, gin.H{"error": "Cannot delete root directory"})
		return
	}

	safePath, err := h.securePath(req.Path)
	if err != nil {
		c.JSON(http.StatusForbidden, gin.H{"error": "Access denied"})
		return
	}

	// Clean up S3 objects + registry rows for every S3-backed file at or
	// beneath this path.  This must run BEFORE os.RemoveAll so the registry
	// rows (keyed by file_path) are still available.
	h.deleteS3Entries(safePath)

	if err := os.RemoveAll(safePath); err != nil {
		// Local file may already be gone (stateless S3 flow) — that's OK.
		if !os.IsNotExist(err) {
			c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to delete item", "details": err.Error()})
			return
		}
	}

	logger.Info("Files", "File system item deleted successfully", "path", req.Path, "ip", c.ClientIP())
	c.JSON(http.StatusOK, gin.H{"status": "success", "message": "Item deleted successfully"})
}

// deleteS3Entries removes all S3 objects and FileRegistry rows whose
// file_path equals or is nested under safePath.  Safe to call when S3 is
// disabled (no-op).
func (h *FileHandler) deleteS3Entries(safePath string) {
	if !filecore.IsS3Enabled() {
		// Even without S3, clean up stale registry rows so they don't
		// reappear as virtual files in listings.
		db.DB.Where("file_path = ? OR file_path LIKE ?", safePath, safePath+string(filepath.Separator)+"%").
			Delete(&models.FileRegistry{})
		return
	}

	prefix := safePath
	if !strings.HasSuffix(prefix, string(filepath.Separator)) {
		prefix += string(filepath.Separator)
	}

	var records []models.FileRegistry
	db.DB.Where("file_path = ? OR file_path LIKE ?", safePath, prefix+"%").Find(&records)

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Minute)
	defer cancel()

	for _, rec := range records {
		if rec.S3Key != "" {
			if err := filecore.DeleteFromS3(ctx, rec.S3Key); err != nil {
				logger.Warn("Files", "Failed to delete S3 object", "key", rec.S3Key, "error", err)
			}
		}
		db.DB.Unscoped().Delete(&rec)
	}

	if len(records) > 0 {
		logger.Info("Files", "S3 objects and registry rows cleaned up", "count", len(records), "path", safePath)
	}
}

// UploadFile handles POST /api/files/upload
func (h *FileHandler) UploadFile(c *gin.Context) {
	if h.proxyToServer(c, c.Request.Method, c.Request.URL.Path) {
		return
	}
	targetFolder := c.PostForm("path")
	safeFolder, err := h.securePath(targetFolder)
	if err != nil {
		c.JSON(http.StatusForbidden, gin.H{"error": "Access denied"})
		return
	}

	file, err := c.FormFile("file")
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Missing file form payload"})
		return
	}

	// Ensure the base directory exists
	_ = os.MkdirAll(safeFolder, 0755)

	// Combine to build absolute local path
	filename := filepath.Base(file.Filename)
	safeFilePath := filepath.Join(safeFolder, filename)

	if err := c.SaveUploadedFile(file, safeFilePath); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to write file on disk", "details": err.Error()})
		return
	}

	// Register + archive to S3 (synchronous: upload completes before we return
	// the response, then the local copy is removed). This guarantees the file is
	// permanently stored in object storage even on ephemeral Clever Cloud disk.
	if filecore.IsS3Enabled() {
		if _, err := filecore.RegisterAndArchiveToS3(safeFilePath, "", "", 0, ""); err != nil {
			logger.Error("Files", "S3 archive failed for uploaded file — keeping local copy",
				"path", safeFilePath, "error", err)
			// Fall back: at least register it locally so it is discoverable.
			if _, regErr := filecore.RegisterFile(safeFilePath, "", "", 0, ""); regErr != nil {
				logger.Error("Files", "Local registration also failed", "path", safeFilePath, "error", regErr)
			}
		}
	} else {
		// S3 disabled: register locally (async background push when S3 is re-enabled).
		if _, err := filecore.RegisterFile(safeFilePath, "", "", 0, ""); err != nil {
			logger.Error("Files", "Failed to register uploaded file in registry", "path", safeFilePath, "error", err)
		}
	}

	logger.Info("Files", "File uploaded successfully", "folder", targetFolder, "filename", filename, "ip", c.ClientIP())
	c.JSON(http.StatusOK, gin.H{"status": "success", "message": "File uploaded successfully"})
}

// copyFile copies a single file from src to dst.
func copyFile(src, dst string) error {
	in, err := os.Open(src)
	if err != nil {
		return err
	}
	defer in.Close()

	out, err := os.Create(dst)
	if err != nil {
		return err
	}
	defer out.Close()

	if _, err = io.Copy(out, in); err != nil {
		return err
	}

	si, err := os.Stat(src)
	if err != nil {
		return err
	}
	return os.Chmod(dst, si.Mode())
}

// copyDir recursively copies a directory tree from src to dst.
func copyDir(src, dst string) error {
	si, err := os.Stat(src)
	if err != nil {
		return err
	}

	if err = os.MkdirAll(dst, si.Mode()); err != nil {
		return err
	}

	entries, err := os.ReadDir(src)
	if err != nil {
		return err
	}

	for _, entry := range entries {
		srcPath := filepath.Join(src, entry.Name())
		dstPath := filepath.Join(dst, entry.Name())

		if entry.IsDir() {
			if err = copyDir(srcPath, dstPath); err != nil {
				return err
			}
		} else {
			if err = copyFile(srcPath, dstPath); err != nil {
				return err
			}
		}
	}
	return nil
}

// MoveItem handles POST /api/files/move for renaming and moving
func (h *FileHandler) MoveItem(c *gin.Context) {
	if h.proxyToServer(c, c.Request.Method, c.Request.URL.Path) {
		return
	}
	var req struct {
		SrcPath string `json:"src_path" binding:"required"`
		DstPath string `json:"dst_path" binding:"required"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Invalid request payload"})
		return
	}
	safeSrc, err := h.securePath(req.SrcPath)
	if err != nil {
		c.JSON(http.StatusForbidden, gin.H{"error": "Access denied"})
		return
	}
	safeDst, err := h.securePath(req.DstPath)
	if err != nil {
		c.JSON(http.StatusForbidden, gin.H{"error": "Access denied"})
		return
	}
	// Ensure parent dir of destination exists
	if err := os.MkdirAll(filepath.Dir(safeDst), 0755); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to create destination parent folder"})
		return
	}

	// Update FileRegistry rows so S3-backed files remain findable at their
	// new path.  S3 objects are keyed by content checksum (not by path), so
	// only the registry file_path needs updating — no S3 copy/delete.
	h.relocateS3Entries(safeSrc, safeDst)

	if err := os.Rename(safeSrc, safeDst); err != nil {
		// Local file may not exist (stateless S3 flow) — that's OK if the
		// registry was already updated above.
		if !os.IsNotExist(err) {
			c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to move item", "details": err.Error()})
			return
		}
	}
	logger.Info("Files", "Item moved/renamed successfully", "src", req.SrcPath, "dst", req.DstPath)
	c.JSON(http.StatusOK, gin.H{"status": "success", "message": "Item moved successfully"})
}

// relocateS3Entries updates the file_path of all FileRegistry rows that
// match safeSrc (exact or nested under it) to the corresponding path under
// safeDst.  This keeps S3-backed files discoverable after a rename/move.
func (h *FileHandler) relocateS3Entries(safeSrc, safeDst string) {
	// Single file — exact match.
	var reg models.FileRegistry
	if err := db.DB.Where("file_path = ?", safeSrc).First(&reg).Error; err == nil {
		reg.FilePath = safeDst
		db.DB.Model(&reg).Update("file_path", safeDst)
		return
	}

	// Directory — update every nested file.
	prefix := safeSrc
	if !strings.HasSuffix(prefix, string(filepath.Separator)) {
		prefix += string(filepath.Separator)
	}
	var records []models.FileRegistry
	db.DB.Where("file_path LIKE ?", prefix+"%").Find(&records)
	dstPrefix := safeDst
	if !strings.HasSuffix(dstPrefix, string(filepath.Separator)) {
		dstPrefix += string(filepath.Separator)
	}
	for _, rec := range records {
		rel := strings.TrimPrefix(rec.FilePath, prefix)
		newPath := dstPrefix + rel
		db.DB.Model(&models.FileRegistry{}).Where("id = ?", rec.ID).Update("file_path", newPath)
	}
}

// CopyItem handles POST /api/files/copy for duplicating items
func (h *FileHandler) CopyItem(c *gin.Context) {
	if h.proxyToServer(c, c.Request.Method, c.Request.URL.Path) {
		return
	}
	var req struct {
		SrcPath string `json:"src_path" binding:"required"`
		DstPath string `json:"dst_path" binding:"required"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Invalid request payload"})
		return
	}
	safeSrc, err := h.securePath(req.SrcPath)
	if err != nil {
		c.JSON(http.StatusForbidden, gin.H{"error": "Access denied"})
		return
	}
	safeDst, err := h.securePath(req.DstPath)
	if err != nil {
		c.JSON(http.StatusForbidden, gin.H{"error": "Access denied"})
		return
	}
	stat, err := os.Stat(safeSrc)
	if err != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "Source item not found"})
		return
	}
	if stat.IsDir() {
		err = copyDir(safeSrc, safeDst)
	} else {
		err = copyFile(safeSrc, safeDst)
	}
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to copy item", "details": err.Error()})
		return
	}
	logger.Info("Files", "Item copied successfully", "src", req.SrcPath, "dst", req.DstPath)
	c.JSON(http.StatusOK, gin.H{"status": "success", "message": "Item copied successfully"})
}

// CompressItems handles POST /api/files/compress to ZIP multiple files/directories
func (h *FileHandler) CompressItems(c *gin.Context) {
	if h.proxyToServer(c, c.Request.Method, c.Request.URL.Path) {
		return
	}
	var req struct {
		ParentPath string   `json:"parent_path"`
		Items      []string `json:"items" binding:"required"`
		ZipName    string   `json:"zip_name" binding:"required"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Invalid request payload"})
		return
	}

	var absolutePaths []string
	for _, item := range req.Items {
		itemPath := filepath.Join(req.ParentPath, item)
		safeItemPath, err := h.securePath(itemPath)
		if err != nil {
			c.JSON(http.StatusForbidden, gin.H{"error": "Access denied for: " + item})
			return
		}
		absolutePaths = append(absolutePaths, safeItemPath)
	}

	destName := req.ZipName
	if filepath.Ext(destName) == "" {
		destName = destName + ".zip"
	}

	payloadObj := struct {
		Files    []string `json:"files"`
		DestName string   `json:"dest_name"`
	}{
		Files:    absolutePaths,
		DestName: destName,
	}

	payloadBytes, err := json.Marshal(payloadObj)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to marshal payload"})
		return
	}

	job, err := scheduler.Engine.SubmitJob(
		"file_compress",
		fmt.Sprintf("Compress %d items", len(absolutePaths)),
		fmt.Sprintf("Compressing %s to %s/Compressed", strings.Join(req.Items, ", "), h.rootDir),
		"files",
		5,
		string(payloadBytes),
		"",
	)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to submit compress job", "details": err.Error()})
		return
	}

	logger.Info("Files", "Submitted compression job successfully", "jobID", job.ID, "zipName", destName)
	c.JSON(http.StatusOK, gin.H{
		"status":  "success",
		"message": "Compression job queued successfully",
		"job_id":  job.ID,
	})
}

// DecompressItem handles POST /api/files/decompress to extract ZIP/TAR/RAR/7Z archives
func (h *FileHandler) DecompressItem(c *gin.Context) {
	if h.proxyToServer(c, c.Request.Method, c.Request.URL.Path) {
		return
	}
	var req struct {
		Path     string `json:"path" binding:"required"`
		Password string `json:"password"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Invalid request payload"})
		return
	}
	safePath, err := h.securePath(req.Path)
	if err != nil {
		c.JSON(http.StatusForbidden, gin.H{"error": "Access denied"})
		return
	}

	info, err := os.Stat(safePath)
	if err != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "Archive file not found"})
		return
	}
	if info.IsDir() {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Target is a directory, not an archive file"})
		return
	}

	// Verify if archive requires password
	requiresPassword, err := filecore.IsArchivePasswordProtected(safePath)
	if err == nil && requiresPassword && req.Password == "" {
		c.JSON(http.StatusOK, gin.H{
			"status":            "password_required",
			"message":           "Archive is password-protected",
			"requires_password": true,
		})
		return
	}

	payload := safePath
	if req.Password != "" {
		pData, _ := json.Marshal(map[string]string{
			"path":     safePath,
			"password": req.Password,
		})
		payload = string(pData)
	}

	job, err := scheduler.Engine.SubmitJob(
		"file_decompress",
		fmt.Sprintf("Decompress %s", filepath.Base(safePath)),
		fmt.Sprintf("Extracting %s near the archive file", filepath.Base(safePath)),
		"files",
		5,
		payload,
		"",
	)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to submit decompress job", "details": err.Error()})
		return
	}

	logger.Info("Files", "Submitted decompression job successfully", "jobID", job.ID, "path", req.Path)
	c.JSON(http.StatusOK, gin.H{
		"status":  "success",
		"message": "Decompression job queued successfully",
		"job_id":  job.ID,
	})
}
