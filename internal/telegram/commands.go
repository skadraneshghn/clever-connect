package telegram

import (
	"fmt"
	"runtime"
	"strings"
	"time"

	"clever-connect/internal/db"
	"clever-connect/internal/downloader"
	"clever-connect/internal/logger"
	"clever-connect/internal/models"

	tele "gopkg.in/telebot.v4"
)

// registerCommands wires all bot commands and callback handlers.
func (e *Engine) registerCommands() {

	// ────────────────── /start ──────────────────
	e.Bot.Handle("/start", func(c tele.Context) error {
		e.commandsProcessed.Add(1)

		e.mu.RLock()
		welcome := e.Config.WelcomeMessage
		e.mu.RUnlock()

		if welcome == "" {
			welcome = "👋 Welcome to *CleverConnect Bot*!\n\nUse /help to see available commands."
		}

		// Replace placeholders
		welcome = strings.ReplaceAll(welcome, "{name}", c.Sender().FirstName)
		welcome = strings.ReplaceAll(welcome, "{username}", c.Sender().Username)

		return c.Send(welcome, &tele.SendOptions{ParseMode: tele.ModeMarkdown})
	})

	// ────────────────── /help ──────────────────
	e.Bot.Handle("/help", func(c tele.Context) error {
		e.commandsProcessed.Add(1)

		isAdmin := e.IsAdmin(c.Sender().ID)

		help := "🤖 *CleverConnect Bot Commands*\n\n"
		help += "📌 `/start` — Welcome message\n"
		help += "❓ `/help` — This help menu\n"
		help += "📊 `/status` — Bot & server status\n"
		help += "🆔 `/myid` — Get your Telegram user ID\n"

		if isAdmin {
			help += "\n🔐 *Admin Commands:*\n"
			help += "📁 `/files` — Browse server files\n"
			help += "⚙️ `/settings` — View bot configuration\n"
			help += "🔄 `/reload` — Hot-reload config from DB\n"
		}

		return c.Send(help, &tele.SendOptions{ParseMode: tele.ModeMarkdown})
	})

	// ────────────────── /myid ──────────────────
	e.Bot.Handle("/myid", func(c tele.Context) error {
		e.commandsProcessed.Add(1)
		msg := fmt.Sprintf("🆔 Your Telegram User ID: `%d`\n👤 Username: @%s",
			c.Sender().ID, c.Sender().Username)
		return c.Send(msg, &tele.SendOptions{ParseMode: tele.ModeMarkdown})
	})

	// ────────────────── /status ──────────────────
	e.Bot.Handle("/status", func(c tele.Context) error {
		e.commandsProcessed.Add(1)

		uptime := time.Since(e.startedAt)
		stats := fmt.Sprintf(
			"📊 *CleverConnect Bot Status*\n\n"+
				"🟢 *Status:* Online\n"+
				"⏱ *Uptime:* %s\n"+
				"🧵 *Workers:* %d (all CPU cores)\n"+
				"📨 *Messages Processed:* %d\n"+
				"⚡ *Commands Processed:* %d\n"+
				"📁 *Files Sent:* %d\n"+
				"❌ *Errors:* %d\n"+
				"🤖 *Bot:* @%s",
			formatUptime(uptime),
			runtime.NumCPU(),
			e.messagesProcessed.Load(),
			e.commandsProcessed.Load(),
			e.filesSent.Load(),
			e.errors.Load(),
			e.Bot.Me.Username,
		)

		// Fetch active downloads
		var activeDownloads []models.LeechJob
		if err := db.DB.Where("status = ?", "downloading").Find(&activeDownloads).Error; err == nil && len(activeDownloads) > 0 {
			stats += "\n\n📥 *Active Downloads:*"
			for _, job := range activeDownloads {
				stats += fmt.Sprintf("\n• `%s`: %.1f%% (⚡ %.1f MB/s)", job.Filename, job.Progress, job.Speed)
			}
		}

		// Fetch active uploads (telegram parallel uploads)
		var activeUploads []models.SchedulerJob
		if err := db.DB.Where("job_type = ? AND status = ?", "telegram_upload", "running").Find(&activeUploads).Error; err == nil && len(activeUploads) > 0 {
			stats += "\n\n📤 *Active Uploads:*"
			for _, job := range activeUploads {
				stats += fmt.Sprintf("\n• `%s`: %d%%", job.Name, job.Progress)
			}
		}

		return c.Send(stats, &tele.SendOptions{ParseMode: tele.ModeMarkdown})
	})

	// ────────────────── /settings (Admin only) ──────────────────
	e.Bot.Handle("/settings", e.AdminOnly(func(c tele.Context) error {
		e.commandsProcessed.Add(1)

		e.mu.RLock()
		cfg := e.Config
		e.mu.RUnlock()

		adminIDs := cfg.AdminUserIDs
		if adminIDs == "" {
			adminIDs = "(none configured)"
		}

		features := []string{}
		if cfg.EnableFileSharing {
			features = append(features, "📁 File Sharing")
		}
		if cfg.EnableNotifications {
			features = append(features, "🔔 Notifications")
		}
		if len(features) == 0 {
			features = append(features, "None enabled")
		}

		msg := fmt.Sprintf(
			"⚙️ *Bot Configuration*\n\n"+
				"🤖 *Bot Username:* @%s\n"+
				"👥 *Admin IDs:* `%s`\n"+
				"⏱ *Polling Interval:* %ds\n"+
				"🎯 *Enabled Features:*\n%s\n"+
				"📝 *Welcome Message:*\n_%s_",
			e.Bot.Me.Username,
			adminIDs,
			cfg.PollingInterval,
			strings.Join(features, "\n"),
			truncate(cfg.WelcomeMessage, 100),
		)
		return c.Send(msg, &tele.SendOptions{ParseMode: tele.ModeMarkdown})
	}))

	// ────────────────── /reload (Admin only) ──────────────────
	e.Bot.Handle("/reload", e.AdminOnly(func(c tele.Context) error {
		e.commandsProcessed.Add(1)

		if err := e.ReloadConfig(); err != nil {
			logger.Error("Telegram", "Config reload failed", "error", err)
			return c.Send("❌ Failed to reload configuration: " + err.Error())
		}
		return c.Send("✅ Configuration reloaded successfully from database.")
	}))

	// ────────────────── /files (Admin only) ──────────────────
	e.Bot.Handle("/files", e.AdminOnly(func(c tele.Context) error {
		e.commandsProcessed.Add(1)
		return e.handleFileBrowse(c, "/")
	}))

	// ────────────────── Callback query router (for inline keyboards) ──────────────────
	e.Bot.Handle(tele.OnCallback, func(c tele.Context) error {
		data := c.Callback().Data
		logger.Info("Telegram", "Callback received", "data", data, "user_id", c.Sender().ID)

		switch {
		case strings.HasPrefix(data, "fb:"):
			// File browser navigation
			if !e.IsAdmin(c.Sender().ID) {
				return c.Respond(&tele.CallbackResponse{Text: "⛔ Admin only"})
			}
			path := strings.TrimPrefix(data, "fb:")
			return e.handleFileBrowse(c, path)

		case strings.HasPrefix(data, "send:"):
			// Send file to chat
			if !e.IsAdmin(c.Sender().ID) {
				return c.Respond(&tele.CallbackResponse{Text: "⛔ Admin only"})
			}
			filePath := strings.TrimPrefix(data, "send:")
			// Dispatch file sending to worker pool for parallel processing
			e.Dispatch(func() {
				if err := e.sendFileToChat(c, filePath); err != nil {
					logger.Error("Telegram", "Failed to send file", "path", filePath, "error", err)
				}
			})
			return c.Respond(&tele.CallbackResponse{Text: "📤 Sending file..."})

		default:
			return c.Respond(&tele.CallbackResponse{Text: "Unknown action"})
		}
	})

	// ────────────────── Register bot commands menu ──────────────────
	commands := []tele.Command{
		{Text: "start", Description: "Welcome message"},
		{Text: "help", Description: "Show available commands"},
		{Text: "status", Description: "Bot & server status"},
		{Text: "myid", Description: "Get your Telegram user ID"},
		{Text: "files", Description: "Browse server files (admin)"},
		{Text: "settings", Description: "View bot config (admin)"},
		{Text: "reload", Description: "Reload config from DB (admin)"},
	}

	if err := e.Bot.SetCommands(commands); err != nil {
		logger.Warn("Telegram", "Failed to set bot commands menu", "error", err)
	}

	// ────────────────── OnText Message Handler (Admin only) ──────────────────
	e.Bot.Handle(tele.OnText, e.AdminOnly(func(c tele.Context) error {
		e.messagesProcessed.Add(1)
		text := strings.TrimSpace(c.Text())
		if text == "" {
			return nil
		}

		// Ignore standard slash commands
		if strings.HasPrefix(text, "/") {
			return nil
		}

		// Extract any URL starting with http:// or https://
		var downloadURL string
		for _, word := range strings.Fields(text) {
			if strings.HasPrefix(word, "http://") || strings.HasPrefix(word, "https://") {
				downloadURL = word
				break
			}
		}

		if downloadURL != "" {
			return e.handleDownloadLink(c, downloadURL)
		}

		return c.Send("ℹ️ Send a valid HTTP/HTTPS link to download it directly to the server file manager.")
	}))

	// Seed default Telegram config if none exists
	seedTelegramConfig()
}

// handleDownloadLink queues a file download from a given URL via the download manager.
func (e *Engine) handleDownloadLink(c tele.Context, downloadURL string) error {
	if downloader.Manager == nil {
		return c.Send("❌ Downloader engine is not running on the server.")
	}

	jobID, err := downloader.Manager.AddJob(
		downloadURL,
		"",    // default save directory
		"",    // extract filename from URL
		"",    // username
		"",    // password
		4,     // threads
		false, // usePremium
	)
	if err != nil {
		return c.Send("❌ Failed to queue download: " + err.Error())
	}

	return c.Send(fmt.Sprintf("📥 *Download Queued!*\n\nJob ID: `%s`\nLink: %s\n\nUse `/status` to monitor the download progress.", jobID, downloadURL), &tele.SendOptions{ParseMode: tele.ModeMarkdown})
}

// seedTelegramConfig ensures a default config row exists in the database.
func seedTelegramConfig() {
	var cfg models.TelegramConfig
	if err := db.DB.First(&cfg).Error; err != nil {
		logger.Info("Telegram", "Seeding default Telegram bot configuration")
		db.DB.Create(&models.TelegramConfig{
			PollingInterval:     10,
			MaxFileSize:         50,
			EnableFileSharing:   true,
			EnableNotifications: true,
			WelcomeMessage:      "👋 Welcome to *CleverConnect Bot*, {name}!\n\nUse /help to see available commands.",
		})
	}
}
