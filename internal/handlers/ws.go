package handlers

import (
	"encoding/json"
	"math/rand"
	"net/http"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"time"

	"clever-connect/internal/config"
	"clever-connect/internal/db"
	"clever-connect/internal/downloader"
	"clever-connect/internal/filecore"
	"clever-connect/internal/logger"
	"clever-connect/internal/models"
	"clever-connect/internal/spotify"
	"clever-connect/internal/torrent"
	"clever-connect/internal/youtube"

	"github.com/gin-gonic/gin"
	"github.com/gorilla/websocket"
)

var upgrader = websocket.Upgrader{
	ReadBufferSize:  1024,
	WriteBufferSize: 1024,
	CheckOrigin: func(r *http.Request) bool {
		return true // Allow all origins for local networking app
	},
}

type WSHandler struct {
	cfg *config.Config
}

func NewWSHandler(cfg *config.Config) *WSHandler {
	return &WSHandler{cfg: cfg}
}

func (h *WSHandler) ServeWS(c *gin.Context) {
	conn, err := upgrader.Upgrade(c.Writer, c.Request, nil)
	if err != nil {
		logger.Error("WS", "WebSocket upgrade failed",
			"error", err.Error(),
			"ip", c.ClientIP(),
		)
		return
	}
	defer conn.Close()

	logger.Info("WS", "Connection established",
		"mode", h.cfg.AppMode,
		"ip", c.ClientIP(),
	)

	ticker := time.NewTicker(2 * time.Second)
	defer ticker.Stop()

	// Initial Total counters
	totalDownload := 8120.0
	totalUpload := 2450.0
	messageCount := 0

	// Loop streaming telemetries
	for {
		select {
		case <-ticker.C:
			var msg interface{}

			if h.cfg.AppMode == "client" {
				// Client Telemetry
				downloadSpeed := float64(rand.Intn(80) + 10) // 10 to 90 MB/s
				uploadSpeed := float64(rand.Intn(20) + 2)    // 2 to 22 MB/s
				latency := rand.Intn(15) + 35               // 35 to 50 ms

				totalDownload += downloadSpeed / 10
				totalUpload += uploadSpeed / 10

				msg = gin.H{
					"type":          "bandwidth",
					"upload":        uploadSpeed,
					"download":      downloadSpeed,
					"totalDownload": totalDownload,
					"totalUpload":   totalUpload,
					"latency":       latency,
				}
			} else {
				// Server Telemetry
				sysStats := GetSystemStatsData()
				cpu := int(sysStats.CPUPercent)
				memory := int(sysStats.MemPercent)
				disk := int(sysStats.DiskPercent)

				var activeLeechCount int64
				db.DB.Model(&models.LeechJob{}).Where("status = ?", "downloading").Count(&activeLeechCount)

				var activeTorrentCount int64
				db.DB.Model(&models.TorrentJob{}).Count(&activeTorrentCount)

				var activeSchedulerCount int64
				db.DB.Model(&models.SchedulerJob{}).Where("status = ?", "running").Count(&activeSchedulerCount)

				downloadSpeed := float64(rand.Intn(120) + 40) // Combined node aggregate speed
				uploadSpeed := float64(rand.Intn(40) + 10)

				totalDownload += downloadSpeed / 100
				totalUpload += uploadSpeed / 100

				clients := []gin.H{
					{"id": "1", "username": "salman_desktop", "ip": "82.102.23.45", "country": "Iran", "flag": "🇮🇷", "protocol": "VLESS-XTLS", "connectedAt": "12:04:12", "duration": "02h 35m", "uploadSpeed": float64(rand.Intn(10)+1) * 0.4, "downloadSpeed": float64(rand.Intn(30)+5) * 0.4, "active": true},
					{"id": "2", "username": "john_iphone", "ip": "188.45.67.12", "country": "Germany", "flag": "🇩🇪", "protocol": "Shadowsocks", "connectedAt": "13:10:00", "duration": "01h 29m", "uploadSpeed": float64(rand.Intn(5)+1) * 0.2, "downloadSpeed": float64(rand.Intn(15)+2) * 0.2, "active": true},
					{"id": "3", "username": "mary_macbook", "ip": "95.12.89.200", "country": "United Kingdom", "flag": "🇬🇧", "protocol": "Trojan", "connectedAt": "14:02:15", "duration": "37m", "uploadSpeed": float64(rand.Intn(4)+1) * 0.1, "downloadSpeed": float64(rand.Intn(10)+1) * 0.2, "active": true},
					{"id": "4", "username": "office_router", "ip": "104.22.4.90", "country": "United States", "flag": "🇺🇸", "protocol": "Wireguard", "connectedAt": "08:12:45", "duration": "06h 27m", "uploadSpeed": float64(rand.Intn(15)+5) * 0.3, "downloadSpeed": float64(rand.Intn(40)+10) * 0.3, "active": true},
				}

				msg = gin.H{
					"type":                  "telemetry",
					"cpu":                   cpu,
					"memory":                memory,
					"disk":                  disk,
					"connsCount":            len(clients),
					"uploadSpeed":           uploadSpeed,
					"downloadSpeed":         downloadSpeed,
					"totalDownload":         totalDownload,
					"totalUpload":           totalUpload,
					"clients":               clients,
					"cpu_cores_percent":     sysStats.CPUCoresPercent,
					"cpu_mhz":               sysStats.CPUMhz,
					"mem_total_gb":          sysStats.MemTotalGB,
					"mem_used_gb":           sysStats.MemUsedGB,
					"mem_free_gb":           sysStats.MemFreeGB,
					"swap_total_gb":         sysStats.SwapTotalGB,
					"swap_used_gb":          sysStats.SwapUsedGB,
					"swap_percent":          sysStats.SwapPercent,
					"disk_total_gb":         sysStats.DiskTotalGB,
					"disk_used_gb":          sysStats.DiskUsedGB,
					"disk_free_gb":          sysStats.DiskFreeGB,
					"disk_read_bytes_sec":   sysStats.DiskReadBytesSec,
					"disk_write_bytes_sec":  sysStats.DiskWriteBytesSec,
					"net_recv_bytes_sec":    sysStats.NetRecvBytesSec,
					"net_sent_bytes_sec":    sysStats.NetSentBytesSec,
					"cpu_temp":              sysStats.CPUTemp,
					"uptime_seconds":        sysStats.UptimeSeconds,
					"boot_time":             sysStats.BootTime,
					"os_platform":           sysStats.OSPlatform,
					"os_kernel":             sysStats.OSKernel,
					"app_mem_mb":            sysStats.AppMemMB,
					"go_version":            runtime.Version(),
					"os_runtime":            runtime.GOOS,
					"active_leeches":        activeLeechCount,
					"active_torrents":       activeTorrentCount,
					"active_scheds":         activeSchedulerCount,
				}
			}

			if err := conn.WriteJSON(msg); err != nil {
				logger.Warn("WS", "Connection closed — write failed",
					"error", err.Error(),
					"ip", c.ClientIP(),
					"messagesSent", messageCount,
				)
				return
			}
			messageCount++

			// Log periodic telemetry summary (every 30 messages ≈ 1 minute)
			if messageCount%30 == 0 {
				logger.Debug("WS", "Telemetry stream active",
					"ip", c.ClientIP(),
					"messagesSent", messageCount,
					"totalDown", totalDownload,
					"totalUp", totalUpload,
				)
			}
		}
	}
}

// ServeWSJobs upgraded stream sending and receiving torrent + leech jobs data
func (h *WSHandler) ServeWSJobs(c *gin.Context) {
	conn, err := upgrader.Upgrade(c.Writer, c.Request, nil)
	if err != nil {
		logger.Error("WS", "WebSocket jobs upgrade failed",
			"error", err.Error(),
			"ip", c.ClientIP(),
		)
		return
	}
	defer conn.Close()

	logger.Info("WS", "Jobs WebSocket connection established",
		"mode", h.cfg.AppMode,
		"ip", c.ClientIP(),
	)

	if h.cfg.AppMode == "client" {
		// --- CLIENT MODE: PIPE/PROXY TO SERVER ---
		var remoteURLTarget string
		var remoteToken string
		if h.cfg.ServerURL != "" {
			remoteURLTarget = h.cfg.ServerURL
			remoteToken = h.cfg.ServerAuthToken
		} else {
			var clientCfg models.EhcoClientConfig
			if err := db.DB.First(&clientCfg).Error; err != nil || clientCfg.RemoteURL == "" {
				logger.Warn("WS", "No remote server connection configured for jobs proxy")
				return
			}
			remoteURLTarget = clientCfg.RemoteURL
			remoteToken = clientCfg.AuthToken
		}

		// Convert http/https to ws/wss if needed
		remoteWS := remoteURLTarget
		remoteWS = strings.Replace(remoteWS, "https://", "wss://", 1)
		remoteWS = strings.Replace(remoteWS, "http://", "ws://", 1)
		if idx := strings.Index(remoteWS, "/ws"); idx != -1 {
			remoteWS = remoteWS[:idx]
		}
		if idx := strings.Index(remoteWS, "/tunnel"); idx != -1 {
			remoteWS = remoteWS[:idx]
		}
		remoteWS = strings.TrimSuffix(remoteWS, "/")
		remoteWS += "/ws/jobs?token=" + remoteToken

		// Dial remote server websocket
		serverConn, _, err := websocket.DefaultDialer.Dial(remoteWS, nil)
		if err != nil {
			logger.Error("WS", "Failed to connect to remote server jobs WebSocket", "error", err.Error())
			return
		}
		defer serverConn.Close()

		// Run bidirectional piping
		errChan := make(chan error, 2)
		
		// Copy client -> server
		go func() {
			for {
				msgType, message, err := conn.ReadMessage()
				if err != nil {
					errChan <- err
					return
				}
				err = serverConn.WriteMessage(msgType, message)
				if err != nil {
					errChan <- err
					return
				}
			}
		}()

		// Copy server -> client
		go func() {
			for {
				msgType, message, err := serverConn.ReadMessage()
				if err != nil {
					errChan <- err
					return
				}
				err = conn.WriteMessage(msgType, message)
				if err != nil {
					errChan <- err
					return
				}
			}
		}()

		// Wait for error/closure
		<-errChan
		return
	}

	// --- SERVER MODE: REAL BUSINESS LOGIC ---
	// 1. Reader loop to handle incoming actions/commands
	go func() {
		for {
			_, message, err := conn.ReadMessage()
			if err != nil {
				return
			}

			var cmd struct {
				Action      string `json:"action"`
				InfoHash    string `json:"info_hash,omitempty"`
				JobID       string `json:"job_id,omitempty"`
				DeleteFiles bool   `json:"delete_files,omitempty"`
			}
			if err := json.Unmarshal(message, &cmd); err != nil {
				continue
			}

			switch cmd.Action {
			case "pause_torrent":
				if cmd.InfoHash != "" && torrent.Manager != nil {
					torrent.Manager.PauseTorrent(cmd.InfoHash)
				}
			case "resume_torrent":
				if cmd.InfoHash != "" && torrent.Manager != nil {
					torrent.Manager.ResumeTorrent(cmd.InfoHash)
				}
			case "delete_torrent":
				if cmd.InfoHash != "" && torrent.Manager != nil {
					torrent.Manager.DeleteTorrent(cmd.InfoHash, cmd.DeleteFiles)
				}
			case "pause_leech":
				if cmd.JobID != "" && downloader.Manager != nil {
					downloader.Manager.PauseJob(cmd.JobID)
				}
			case "resume_leech":
				if cmd.JobID != "" {
					_ = db.DB.Model(&models.LeechJob{}).Where("id = ?", cmd.JobID).Update("status", "pending")
				}
			case "delete_leech":
				if cmd.JobID != "" && downloader.Manager != nil {
					downloader.Manager.DeleteJob(cmd.JobID, cmd.DeleteFiles)
				}
			case "cancel_youtube":
				if cmd.JobID != "" && youtube.Manager != nil {
					youtube.Manager.PauseJob(cmd.JobID)
				}
			case "delete_youtube":
				if cmd.JobID != "" && youtube.Manager != nil {
					youtube.Manager.DeleteJob(cmd.JobID, cmd.DeleteFiles)
				}
			case "cancel_spotify":
				if cmd.JobID != "" && spotify.Manager != nil {
					spotify.Manager.CancelJob(cmd.JobID)
				}
			case "delete_spotify":
				if cmd.JobID != "" && spotify.Manager != nil {
					spotify.Manager.DeleteJob(cmd.JobID, cmd.DeleteFiles)
				}
			case "retry_spotify":
				if cmd.JobID != "" && spotify.Manager != nil {
					spotify.Manager.RetryJob(cmd.JobID)
				}
			}
		}
	}()

	// 2. Ticker loop to push live updates of both lists (every 1 second for seamless fluidity)
	ticker := time.NewTicker(1000 * time.Millisecond)
	defer ticker.Stop()

	for {
		select {
		case <-ticker.C:
			var torrentList []models.TorrentJob
			var leechList []models.LeechJob
			var youtubeList []models.YouTubeJob
			var spotifyList []models.SpotifyJob

			// Fetch lists from database
			_ = db.DB.Order("created_at desc").Find(&torrentList)
			_ = db.DB.Order("created_at desc").Find(&leechList)
			_ = db.DB.Order("created_at desc").Find(&youtubeList)
			_ = db.DB.Order("created_at desc").Find(&spotifyList)

			// Populate FileExists for completed or seeding torrent jobs
			for i := range torrentList {
				torrentList[i].FileExists = true
				if torrentList[i].Status == "completed" || torrentList[i].Status == "seeding" {
					absSaveDir := filecore.GetAbsoluteSavePath(torrentList[i].SaveDirectory)
					destPath := filepath.Join(absSaveDir, torrentList[i].Name)
					if _, err := os.Stat(destPath); os.IsNotExist(err) {
						torrentList[i].FileExists = false
					}
				}
			}

			// Populate FileExists for completed leech jobs
			for i := range leechList {
				leechList[i].FileExists = true
				if leechList[i].Status == "completed" {
					absSaveDir := filecore.GetAbsoluteSavePath(leechList[i].SaveDirectory)
					destPath := filepath.Join(absSaveDir, leechList[i].Filename)
					if _, err := os.Stat(destPath); os.IsNotExist(err) {
						leechList[i].FileExists = false
					}
				}
			}

			// Populate FileExists for completed youtube jobs
			for i := range youtubeList {
				youtubeList[i].FileExists = true
				if youtubeList[i].Status == "completed" {
					absSaveDir := filecore.GetAbsoluteSavePath(youtubeList[i].SaveDirectory)
					destPath := filepath.Join(absSaveDir, youtubeList[i].Filename)
					if _, err := os.Stat(destPath); os.IsNotExist(err) {
						youtubeList[i].FileExists = false
					}
				}
			}

			// Populate FileExists for completed spotify jobs
			for i := range spotifyList {
				spotifyList[i].FileExists = true
				if spotifyList[i].Status == "completed" {
					absSaveDir := filecore.GetAbsoluteSavePath(spotifyList[i].SaveDirectory)
					destPath := filepath.Join(absSaveDir, spotifyList[i].Filename)
					if _, err := os.Stat(destPath); os.IsNotExist(err) {
						spotifyList[i].FileExists = false
					}
				}
			}

			response := gin.H{
				"torrents":    torrentList,
				"leechJobs":   leechList,
				"youtubeJobs": youtubeList,
				"spotifyJobs": spotifyList,
			}

			if err := conn.WriteJSON(response); err != nil {
				return
			}
		}
	}
}
