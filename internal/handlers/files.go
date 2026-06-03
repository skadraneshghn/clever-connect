package handlers

import (
	"archive/zip"
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

// proxyToServer automatically forwards requests from the Client Panel to the remote Clever Cloud server.
// This ensures that the local client UI only shows and acts on server-side files, not local ones!
func (h *FileHandler) proxyToServer(c *gin.Context, method string, apiPath string) bool {
	if h.cfg.AppMode == "server" {
		return false
	}

	var remoteURLTarget string
	var remoteToken string

	// 1. Check if configured via environment variables
	if h.cfg.ServerURL != "" {
		remoteURLTarget = h.cfg.ServerURL
		remoteToken = h.cfg.ServerAuthToken
	} else {
		// 2. Fall back to reading remote server client config from database
		var clientCfg models.EhcoClientConfig
		if err := db.DB.First(&clientCfg).Error; err != nil || clientCfg.RemoteURL == "" {
			c.JSON(http.StatusBadRequest, gin.H{"error": "No remote server connection configured in client panel"})
			return true
		}
		remoteURLTarget = clientCfg.RemoteURL
		remoteToken = clientCfg.AuthToken
	}

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

	// Create proxy request
	req, err := http.NewRequest(method, remoteURL, c.Request.Body)
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

	c.JSON(http.StatusOK, gin.H{
		"current_path": displayPath,
		"files":        files,
		"disk_total":   diskTotal,
		"disk_free":    diskFree,
		"disk_used":    diskUsed,
	})
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

	// 2. Fall back to standard disk file streaming (either non-torrent file, or fully completed torrent file)
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

	if err := os.RemoveAll(safePath); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to delete item", "details": err.Error()})
		return
	}

	logger.Info("Files", "File system item deleted successfully", "path", req.Path, "ip", c.ClientIP())
	c.JSON(http.StatusOK, gin.H{"status": "success", "message": "Item deleted successfully"})
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

	// Register uploaded file (deduplicates automatically if already existing)
	if _, err := filecore.RegisterFile(safeFilePath, "", "", 0, ""); err != nil {
		logger.Error("Files", "Failed to register uploaded file in registry", "path", safeFilePath, "error", err)
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
	if err := os.Rename(safeSrc, safeDst); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to move item", "details": err.Error()})
		return
	}
	logger.Info("Files", "Item moved/renamed successfully", "src", req.SrcPath, "dst", req.DstPath)
	c.JSON(http.StatusOK, gin.H{"status": "success", "message": "Item moved successfully"})
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

	zipPath := filepath.Join(req.ParentPath, req.ZipName)
	safeZipPath, err := h.securePath(zipPath)
	if err != nil {
		c.JSON(http.StatusForbidden, gin.H{"error": "Access denied"})
		return
	}

	newZipFile, err := os.Create(safeZipPath)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to create ZIP archive"})
		return
	}
	defer newZipFile.Close()

	zipWriter := zip.NewWriter(newZipFile)
	defer zipWriter.Close()

	for _, item := range req.Items {
		itemPath := filepath.Join(req.ParentPath, item)
		safeItemPath, err := h.securePath(itemPath)
		if err != nil {
			continue
		}

		info, err := os.Stat(safeItemPath)
		if err != nil {
			continue
		}

		if info.IsDir() {
			err = filepath.Walk(safeItemPath, func(path string, f os.FileInfo, err error) error {
				if err != nil {
					return err
				}

				relPath, err := filepath.Rel(filepath.Dir(safeItemPath), path)
				if err != nil {
					return err
				}

				header, err := zip.FileInfoHeader(f)
				if err != nil {
					return err
				}

				header.Name = filepath.ToSlash(relPath)
				if f.IsDir() {
					header.Name += "/"
				} else {
					header.Method = zip.Deflate
				}

				writer, err := zipWriter.CreateHeader(header)
				if err != nil {
					return err
				}

				if f.IsDir() {
					return nil
				}

				fileToZip, err := os.Open(path)
				if err != nil {
					return err
				}
				defer fileToZip.Close()
				_, err = io.Copy(writer, fileToZip)
				return err
			})
		} else {
			header, err := zip.FileInfoHeader(info)
			if err != nil {
				continue
			}
			header.Name = item
			header.Method = zip.Deflate

			writer, err := zipWriter.CreateHeader(header)
			if err != nil {
				continue
			}

			fileToZip, err := os.Open(safeItemPath)
			if err != nil {
				continue
			}
			defer fileToZip.Close()
			_, err = io.Copy(writer, fileToZip)
		}
	}

	logger.Info("Files", "Created ZIP archive successfully", "zipPath", zipPath)
	c.JSON(http.StatusOK, gin.H{"status": "success", "message": "ZIP archive created successfully"})
}

// DecompressItem handles POST /api/files/decompress to extract ZIP archives
func (h *FileHandler) DecompressItem(c *gin.Context) {
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
	safePath, err := h.securePath(req.Path)
	if err != nil {
		c.JSON(http.StatusForbidden, gin.H{"error": "Access denied"})
		return
	}

	reader, err := zip.OpenReader(safePath)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Failed to open ZIP archive", "details": err.Error()})
		return
	}
	defer reader.Close()

	destDir := filepath.Dir(safePath)

	for _, f := range reader.File {
		fpath := filepath.Join(destDir, f.Name)

		// Traversal check for each file inside zip
		if !strings.HasPrefix(filepath.Clean(fpath), h.rootDir) {
			continue
		}

		if f.FileInfo().IsDir() {
			_ = os.MkdirAll(fpath, os.ModePerm)
			continue
		}

		if err = os.MkdirAll(filepath.Dir(fpath), os.ModePerm); err != nil {
			continue
		}

		outFile, err := os.OpenFile(fpath, os.O_WRONLY|os.O_CREATE|os.O_TRUNC, f.Mode())
		if err != nil {
			continue
		}

		rc, err := f.Open()
		if err != nil {
			outFile.Close()
			continue
		}

		_, err = io.Copy(outFile, rc)
		outFile.Close()
		rc.Close()
	}

	logger.Info("Files", "Extracted ZIP archive successfully", "path", req.Path)
	c.JSON(http.StatusOK, gin.H{"status": "success", "message": "ZIP archive extracted successfully"})
}
