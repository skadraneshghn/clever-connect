package main

import (
	"embed"
	"io/fs"
	"net/http"
	"strings"

	"clever-connect/internal/config"
	"clever-connect/internal/db"
	"clever-connect/internal/downloader"
	"clever-connect/internal/ehcocore"
	"clever-connect/internal/handlers"
	"clever-connect/internal/logger"
	"clever-connect/internal/models"
	"clever-connect/internal/scheduler"
	"clever-connect/internal/soroush"
	"clever-connect/internal/spotify"
	"clever-connect/internal/telegram"
	"clever-connect/internal/torrent"
	"clever-connect/internal/youtube"

	"github.com/gin-gonic/gin"
)

//go:embed all:web/client/dist
var clientDist embed.FS

//go:embed all:web/server/dist
var serverDist embed.FS

func main() {
	// Load configuration first (before logger, since we need AppMode)
	cfg := config.LoadConfig()

	// Initialize the async structured logging system
	logger.Init("logs", cfg.AppMode)
	defer logger.Shutdown()

	logger.Info("Core", "Starting CleverConnect VPN Orchestrator",
		"mode", strings.ToUpper(cfg.AppMode),
		"port", cfg.Port,
	)

	// Initialize Database
	database := db.InitDB(cfg)
	_ = database // keep reference

	// Initialize Downloader Engine on server only
	if cfg.AppMode == "server" {
		downloader.Init()
		if err := torrent.Init(); err != nil {
			logger.Error("Torrent", "Failed to initialize torrent manager", "error", err)
		} else {
			defer torrent.Manager.Close()
		}

		// Initialize YouTube Download Engine
		youtube.Init()

		// Initialize Spotify Download Engine
		spotify.Init()

		// Initialize Enterprise Job Scheduler Engine
		scheduler.Init()
	}

	// Auto-start active tunnel engine on bootstrap
	if cfg.AppMode == "server" {
		var serverCfg models.EhcoServerConfig
		if err := db.DB.First(&serverCfg).Error; err == nil && serverCfg.IsActive {
			logger.Info("Ehco", "Auto-starting active server tunnel engine")
			if err := ehcocore.StartServerEngine(&serverCfg); err != nil {
				logger.Error("Ehco", "Failed to auto-start server tunnel", "error", err)
			}
		}
	} else {
		var clientCfg models.EhcoClientConfig
		if err := db.DB.First(&clientCfg).Error; err == nil && clientCfg.IsActive {
			logger.Info("Ehco", "Auto-starting active client tunnel engine")
			if err := ehcocore.StartClientEngine(&clientCfg); err != nil {
				logger.Error("Ehco", "Failed to auto-start client tunnel", "error", err)
			}
		}
	}

	// Auto-start Telegram bot engine if configured and active
	if cfg.AppMode == "server" {
		var telegramCfg models.TelegramConfig
		if err := db.DB.First(&telegramCfg).Error; err == nil && telegramCfg.IsActive && telegramCfg.BotToken != "" {
			logger.Info("Telegram", "Auto-starting Telegram bot engine")
			if err := telegram.StartEngine(&telegramCfg); err != nil {
				logger.Error("Telegram", "Failed to auto-start Telegram bot", "error", err)
			}
		}
	}

	// Auto-start Soroush WebRTC tunnel engine if configured and active
	{
		var soroushCfg models.SoroushTunnelConfig
		if err := db.DB.First(&soroushCfg).Error; err == nil && soroushCfg.IsActive {
			var accounts []models.SoroushAccount
			db.DB.Where("status = ?", "verified").Find(&accounts)
			if len(accounts) > 0 {
				isServer := cfg.AppMode == "server"
				logger.Info("Soroush", "Auto-starting Soroush tunnel engine",
					"mode", cfg.AppMode,
					"accounts", len(accounts),
				)
				if err := soroush.StartEngine(&soroushCfg, accounts, isServer); err != nil {
					logger.Error("Soroush", "Failed to auto-start tunnel engine", "error", err)
				}
			}
		}
	}

	// Setup Gin Router in release mode
	gin.SetMode(gin.ReleaseMode)
	gin.DefaultWriter = logger.GinWriter()
	gin.DefaultErrorWriter = logger.GinWriter()
	router := gin.New()

	// Use our structured logging middleware instead of gin.Logger()
	router.Use(logger.GinRecoveryMiddleware())
	router.Use(logger.GinMiddleware())

	router.GET("/swagger", handlers.ServeSwagger)

	// Setup API Route Handlers
	authHandler := handlers.NewAuthHandler(cfg)
	wsHandler := handlers.NewWSHandler(cfg)
	ehcoHandler := handlers.NewEhcoHandler(cfg)
	fileHandler := handlers.NewFileHandler(cfg)
	leechHandler := handlers.NewLeechHandler(cfg)
	torrentHandler := handlers.NewTorrentHandler(cfg)
	youtubeHandler := handlers.NewYouTubeHandler(cfg)
	spotifyHandler := handlers.NewSpotifyHandler(cfg)
	telegramHandler := handlers.NewTelegramHandler(cfg)
	schedulerHandler := handlers.NewSchedulerHandler(cfg)
	soroushHandler := handlers.NewSoroushHandler(cfg)

	// API Group
	api := router.Group("/api")
	{
		api.POST("/auth/login", authHandler.Login)

		// Protected API routes
		protected := api.Group("")
		protected.Use(handlers.AuthMiddleware(cfg.JWTSecret))
		{
			protected.POST("/clients/disconnect/:id", func(c *gin.Context) {
				id := c.Param("id")
				logger.Info("API", "Client disconnect requested", "clientId", id, "ip", c.ClientIP())
				c.JSON(http.StatusOK, gin.H{"status": "disconnected", "id": id})
			})

			protected.GET("/logs/download", handlers.DownloadTodayLog)

			// Ehco Tunneling routes
			protected.GET("/ehco/config", ehcoHandler.GetConfig)
			protected.POST("/ehco/config", ehcoHandler.SaveConfig)
			protected.POST("/ehco/start", ehcoHandler.StartEngine)
			protected.POST("/ehco/stop", ehcoHandler.StopEngine)

			// System monitoring route
			protected.GET("/system/stats", handlers.GetSystemStats)

			// File Manager API Endpoints
			protected.GET("/files/list", fileHandler.ListDirectory)
			protected.GET("/files/search", fileHandler.SearchFiles)
			protected.GET("/files/stream", fileHandler.StreamOrDownload)
			protected.GET("/files/content", fileHandler.GetContent)
			protected.POST("/files/save", fileHandler.SaveContent)
			protected.POST("/files/create-folder", fileHandler.CreateFolder)
			protected.POST("/files/delete", fileHandler.DeleteItem)
			protected.POST("/files/upload", fileHandler.UploadFile)
			protected.POST("/files/move", fileHandler.MoveItem)
			protected.POST("/files/copy", fileHandler.CopyItem)
			protected.POST("/files/compress", fileHandler.CompressItems)
			protected.POST("/files/decompress", fileHandler.DecompressItem)

			// Remote Downloader (Leecher) API Endpoints
			protected.GET("/leech/jobs", leechHandler.ListJobs)
			protected.POST("/leech/add", leechHandler.AddJob)
			protected.POST("/leech/pause", leechHandler.PauseJob)
			protected.POST("/leech/start", leechHandler.StartJob)
			protected.POST("/leech/delete", leechHandler.DeleteJob)
			protected.GET("/leech/config", leechHandler.GetConfig)
			protected.POST("/leech/config", leechHandler.SaveConfig)

			// BitTorrent Client API Endpoints
			protected.GET("/torrent/list", torrentHandler.ListTorrents)
			protected.POST("/torrent/add", torrentHandler.AddTorrent)
			protected.POST("/torrent/pause", torrentHandler.PauseTorrent)
			protected.POST("/torrent/resume", torrentHandler.ResumeTorrent)
			protected.POST("/torrent/delete", torrentHandler.DeleteTorrent)
			protected.GET("/torrent/files", torrentHandler.ListTorrentFiles)
			protected.POST("/torrent/select-files", torrentHandler.SelectTorrentFiles)
			protected.GET("/torrent/config", torrentHandler.GetConfig)
			protected.POST("/torrent/config", torrentHandler.SaveConfig)

			// YouTube Downloader API Endpoints
			protected.POST("/youtube/info", youtubeHandler.FetchInfo)
			protected.POST("/youtube/add", youtubeHandler.AddJob)
			protected.GET("/youtube/jobs", youtubeHandler.ListJobs)
			protected.POST("/youtube/cancel", youtubeHandler.CancelJob)
			protected.POST("/youtube/delete", youtubeHandler.DeleteJob)
			protected.GET("/youtube/config", youtubeHandler.GetConfig)
			protected.POST("/youtube/config", youtubeHandler.SaveConfig)

			// Spotify Downloader API Endpoints
			protected.POST("/spotify/info", spotifyHandler.FetchInfo)
			protected.POST("/spotify/add", spotifyHandler.AddJob)
			protected.GET("/spotify/jobs", spotifyHandler.ListJobs)
			protected.POST("/spotify/cancel", spotifyHandler.CancelJob)
			protected.POST("/spotify/delete", spotifyHandler.DeleteJob)
			protected.POST("/spotify/retry", spotifyHandler.RetryJob)
			protected.GET("/spotify/config", spotifyHandler.GetConfig)
			protected.POST("/spotify/config", spotifyHandler.SaveConfig)

			// Telegram Bot Core API Endpoints
			protected.GET("/telegram/config", telegramHandler.GetConfig)
			protected.POST("/telegram/config", telegramHandler.SaveConfig)
			protected.POST("/telegram/test", telegramHandler.TestConnection)
			protected.POST("/telegram/start", telegramHandler.StartBot)
			protected.POST("/telegram/stop", telegramHandler.StopBot)
			protected.GET("/telegram/status", telegramHandler.GetStatus)
			protected.POST("/telegram/send-file", telegramHandler.SendFile)
			protected.POST("/telegram/set-avatar", telegramHandler.SetBotAvatar)
			protected.POST("/telegram/broadcast", telegramHandler.BroadcastMessage)
			protected.POST("/telegram/auth/send-code", telegramHandler.SendAuthCode)
			protected.POST("/telegram/auth/verify-code", telegramHandler.VerifyAuthCode)
			protected.POST("/telegram/auth/verify-password", telegramHandler.VerifyAuthPassword)
			protected.POST("/settings/favicon", handlers.UploadFavicon)

			// Enterprise Job Scheduler API Endpoints
			protected.GET("/scheduler/jobs", schedulerHandler.ListJobs)
			protected.POST("/scheduler/jobs", schedulerHandler.CreateJob)
			protected.POST("/scheduler/jobs/:id/cancel", schedulerHandler.CancelJob)
			protected.POST("/scheduler/jobs/:id/retry", schedulerHandler.RetryJob)
			protected.POST("/scheduler/jobs/:id/force", schedulerHandler.ForceRunJob)
			protected.POST("/scheduler/jobs/:id/delete", schedulerHandler.DeleteJob)
			protected.POST("/scheduler/jobs/reorder", schedulerHandler.ReorderJobs)
			protected.GET("/scheduler/jobs/:id/logs", schedulerHandler.GetJobLogs)
			protected.GET("/scheduler/config", schedulerHandler.GetConfig)
			protected.POST("/scheduler/config", schedulerHandler.SaveConfig)
			protected.GET("/scheduler/stats", schedulerHandler.GetStats)
			protected.POST("/scheduler/purge", schedulerHandler.PurgeJobs)

			// Soroush WebRTC Tunnel API Endpoints (ADDITIVE — parallel to Ehco)
			protected.GET("/soroush/accounts", soroushHandler.GetSoroushAccounts)
			protected.POST("/soroush/accounts", soroushHandler.AddSoroushAccount)
			protected.DELETE("/soroush/accounts/:id", soroushHandler.DeleteSoroushAccount)
			protected.POST("/soroush/accounts/:id/send-code", soroushHandler.SendVerificationCode)
			protected.POST("/soroush/accounts/:id/verify", soroushHandler.VerifyAccount)
			protected.GET("/soroush/config", soroushHandler.GetSoroushConfig)
			protected.PUT("/soroush/config", soroushHandler.UpdateSoroushConfig)
			protected.POST("/soroush/engine/start", soroushHandler.StartSoroushEngine)
			protected.POST("/soroush/engine/stop", soroushHandler.StopSoroushEngine)
			protected.GET("/soroush/engine/status", soroushHandler.GetSoroushEngineStatus)
			protected.POST("/soroush/test-token", soroushHandler.TestTokenFetch)
			protected.GET("/soroush/sync", soroushHandler.SyncConfig)
			protected.POST("/soroush/sync", soroushHandler.IngestSync)
		}
	}

	// Real-time WebSocket endpoints (protected via token query param handled in middleware)
	router.GET("/ws", handlers.AuthMiddleware(cfg.JWTSecret), wsHandler.ServeWS)
	router.GET("/ws/jobs", handlers.AuthMiddleware(cfg.JWTSecret), wsHandler.ServeWSJobs)
	router.GET("/ws/logs", handlers.AuthMiddleware(cfg.JWTSecret), handlers.ServeLogWS)

	// Static Assets & SPA Fallback Serving
	var embedFS fs.FS
	var err error

	if cfg.AppMode == "server" {
		embedFS, err = fs.Sub(serverDist, "web/server/dist")
		if err != nil {
			logger.Fatal("Static", "Failed to sub server embed FS", "error", err)
		}
		logger.Info("Static", "Serving CleverConnect Server Panel UI")
	} else {
		embedFS, err = fs.Sub(clientDist, "web/client/dist")
		if err != nil {
			logger.Fatal("Static", "Failed to sub client embed FS", "error", err)
		}
		logger.Info("Static", "Serving CleverConnect Client Panel UI")
	}

	router.GET("/favicon.ico", handlers.ServeFavicon)
	router.GET("/favicon.png", handlers.ServeFavicon)

	router.Use(serveEmbeddedSPA(embedFS))

	// Start Gin Server
	logger.Info("Core", "Orchestrator listening", "addr", "http://127.0.0.1:"+cfg.Port)
	if err := router.Run(":" + cfg.Port); err != nil {
		logger.Fatal("Core", "Failed to run server", "error", err)
	}
}



func serveEmbeddedSPA(embedFS fs.FS) gin.HandlerFunc {
	fileServer := http.FileServer(http.FS(embedFS))

	return func(c *gin.Context) {
		path := c.Request.URL.Path

		// If route matches API endpoints, continue to other middleware/handlers
		if strings.HasPrefix(path, "/api") || strings.HasPrefix(path, "/ws") || strings.HasPrefix(path, "/swagger") {
			c.Next()
			return
		}

		// Format file path for searching inside embedded FS
		filePath := strings.TrimPrefix(path, "/")
		if filePath == "" {
			filePath = "index.html"
		}

		// Check if target file exists in embed FS
		_, err := embedFS.Open(filePath)
		if err != nil {
			// Serve index.html as fallback for Single Page App router
			indexFile, err := embedFS.Open("index.html")
			if err != nil {
				c.String(http.StatusInternalServerError, "Embedded index.html not found")
				return
			}
			defer indexFile.Close()

			stat, err := indexFile.Stat()
			if err != nil {
				c.String(http.StatusInternalServerError, "Failed to inspect index.html")
				return
			}

			c.DataFromReader(http.StatusOK, stat.Size(), "text/html; charset=utf-8", indexFile, nil)
			c.Abort()
			return
		}

		// File exists - serve it
		fileServer.ServeHTTP(c.Writer, c.Request)
		c.Abort()
	}
}
