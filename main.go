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
	"clever-connect/internal/telegram"
	"clever-connect/internal/torrent"

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
	telegramHandler := handlers.NewTelegramHandler(cfg)

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

			// Telegram Bot Core API Endpoints
			protected.GET("/telegram/config", telegramHandler.GetConfig)
			protected.POST("/telegram/config", telegramHandler.SaveConfig)
			protected.POST("/telegram/test", telegramHandler.TestConnection)
			protected.POST("/telegram/start", telegramHandler.StartBot)
			protected.POST("/telegram/stop", telegramHandler.StopBot)
			protected.GET("/telegram/status", telegramHandler.GetStatus)
			protected.POST("/telegram/send-file", telegramHandler.SendFile)
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
