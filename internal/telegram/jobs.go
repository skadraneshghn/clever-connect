package telegram

import (
	"context"
	"encoding/json"
	"fmt"
	"mime"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"time"

	"clever-connect/internal/config"
	"clever-connect/internal/db"
	"clever-connect/internal/models"

	"github.com/golang-jwt/jwt/v5"
	"github.com/gotd/td/telegram"
	"github.com/gotd/td/telegram/message"
	"github.com/gotd/td/telegram/message/html"
	"github.com/gotd/td/telegram/message/styling"
	"github.com/gotd/td/telegram/uploader"
	"github.com/gotd/td/tg"
	tele "gopkg.in/telebot.v4"
)

// We use the public AppID and AppHash from Telegram Desktop
const (
	PublicAppID   = 2040
	PublicAppHash = "b18441a1ff607e10a989891a5624e0d4"
)

type TelegramUploadPayload struct {
	FilePath string `json:"file_path"`
	ChatID   int64  `json:"chat_id"`
}

// uploadProgress tracks the multi-connection upload progress and throttles Telegram updates.
type uploadProgress struct {
	job         *models.SchedulerJob
	eng         *Engine
	progressMsg *tele.Message
	fileName    string
	startTime   time.Time
	lastUpdate  time.Time
	threads     int
	logFn       func(level, message string)

	// gotd message update support
	gotdClient *telegram.Client
	gotdPeer   tg.InputPeerClass
	gotdMsgID  int
}

// Chunk satisfies the uploader.Progress interface.
func (p *uploadProgress) Chunk(ctx context.Context, state uploader.ProgressState) error {
	percent := int(100 * float64(state.Uploaded) / float64(state.Total))
	if percent > 100 {
		percent = 100
	}

	// Update job status in database
	db.DB.Model(p.job).Updates(map[string]interface{}{
		"progress": percent,
		"message":  fmt.Sprintf("Uploading: %s / %s (%d%%)", formatFileSize(state.Uploaded), formatFileSize(state.Total), percent),
	})

	// Throttle Telegram status message updates (max once per 1.5 seconds) to avoid rate limits
	if time.Since(p.lastUpdate) > 1500*time.Millisecond {
		p.lastUpdate = time.Now()
		
		elapsed := time.Since(p.startTime).Seconds()
		speed := 0.0
		if elapsed > 0 {
			speed = float64(state.Uploaded) / elapsed / (1024 * 1024) // MB/s
		}

		pBar := makeProgressBar(percent, 20)
		progressText := fmt.Sprintf(
			"📤 *Uploading File*\n\n"+
				"📄 *File:* `%s`\n"+
				"📏 *Uploaded:* %s of %s (%d%%)\n"+
				"⚡ *Speed:* %.2f MB/s\n\n"+
				"%s",
			p.fileName,
			formatFileSize(state.Uploaded),
			formatFileSize(state.Total),
			percent,
			speed,
			pBar,
		)

		if p.progressMsg != nil && p.eng.Bot != nil {
			_, _ = p.eng.Bot.Edit(p.progressMsg, progressText, &tele.SendOptions{ParseMode: tele.ModeMarkdown})
		} else if p.gotdClient != nil && p.gotdPeer != nil && p.gotdMsgID != 0 {
			api := tg.NewClient(p.gotdClient)
			htmlText := mdToHTML(progressText)
			_, _ = api.MessagesEditMessage(ctx, &tg.MessagesEditMessageRequest{
				Peer:    p.gotdPeer,
				ID:      p.gotdMsgID,
				Message: htmlText,
			})
		}
	}
	return nil
}

// RunTelegramUploadJob executes a parallel multi-connection file upload to Telegram.
func RunTelegramUploadJob(ctx context.Context, job *models.SchedulerJob, logFn func(level, message string)) error {
	logFn("INFO", "Telegram upload job started")

	var payload TelegramUploadPayload
	if err := json.Unmarshal([]byte(job.Payload), &payload); err != nil {
		return fmt.Errorf("invalid payload: %w", err)
	}

	// Resolve absolute path from file manager sandbox
	safePath, err := securePath(payload.FilePath)
	if err != nil {
		return fmt.Errorf("invalid file path: %w", err)
	}

	info, err := os.Stat(safePath)
	if err != nil {
		return fmt.Errorf("file not found on disk: %w", err)
	}

	if info.IsDir() {
		return fmt.Errorf("target path is a directory: %s", payload.FilePath)
	}

	eng := GetEngine()
	if eng == nil {
		return fmt.Errorf("telegram bot engine is not initialized or running")
	}

	eng.mu.RLock()
	cfg := eng.Config
	eng.mu.RUnlock()

	if cfg.AuthType == "bot" && cfg.BotToken == "" {
		return fmt.Errorf("telegram bot token is empty or unconfigured")
	}

	// Determine chat ID to upload to
	chatID := payload.ChatID
	if chatID == 0 {
		// Use the first admin user ID as default
		adminIDs := strings.Split(cfg.AdminUserIDs, ",")
		if len(adminIDs) > 0 && adminIDs[0] != "" {
			var parsed int64
			if _, err := fmt.Sscanf(strings.TrimSpace(adminIDs[0]), "%d", &parsed); err == nil {
				chatID = parsed
			}
		}
	}

	if chatID == 0 {
		return fmt.Errorf("no target chat ID or admin ID found to send the file to")
	}

	fileName := filepath.Base(safePath)
	logFn("INFO", fmt.Sprintf("Preparing upload of %s (size %s) to chat %d", fileName, formatFileSize(info.Size()), chatID))

	// Send initial progress text message via the active telebot instance
	var progressMsg *tele.Message
	if eng.Bot != nil {
		pBar := makeProgressBar(0, 20)
		initialText := fmt.Sprintf("📤 *Starting Upload*\n\n📄 *File:* `%s`\n📏 *Size:* %s\n\n%s `0%%`",
			fileName, formatFileSize(info.Size()), pBar)
		
		msg, err := eng.Bot.Send(tele.ChatID(chatID), initialText, &tele.SendOptions{ParseMode: tele.ModeMarkdown})
		if err != nil {
			logFn("WARN", fmt.Sprintf("Failed to send initial progress message to Telegram: %v", err))
		} else {
			progressMsg = msg
		}
	}

	// Reuse the engine's already-running MTProto client instead of creating a new one.
	// This eliminates cold auth handshakes and halves connection overhead.
	if eng.gotdClient == nil {
		return fmt.Errorf("MTProto client is not initialized — cannot upload via MTProto")
	}

	var mediaSentErr error
	var pPeer tg.InputPeerClass
	var pMsgID int

	// The engine's gotdClient is already running inside client.Run().
	// We can use the engine's gotdCtx to execute API calls directly.
	api := tg.NewClient(eng.gotdClient)

	peer, err := resolveInputPeer(eng.gotdCtx, api, chatID)
	if err != nil {
		return fmt.Errorf("failed to resolve peer for chat ID: %w", err)
	}
	pPeer = peer

	if eng.Bot == nil {
		// User mode: send initial progress message via MTProto client
		pBar := makeProgressBar(0, 20)
		initialText := fmt.Sprintf("📤 *Starting Upload*\n\n📄 *File:* `%s`\n📏 *Size:* %s\n\n%s `0%%`",
			fileName, formatFileSize(info.Size()), pBar)

		sender := message.NewSender(api)
		htmlText := mdToHTML(initialText)
		msg, err := sender.To(peer).StyledText(eng.gotdCtx, html.String(nil, htmlText))
		if err == nil {
			if upd, ok := msg.(*tg.UpdateShortSentMessage); ok {
				pMsgID = upd.ID
			} else if updates, ok := msg.(*tg.Updates); ok {
				for _, u := range updates.Updates {
					if newMessage, ok := u.(*tg.UpdateNewMessage); ok {
						pMsgID = newMessage.Message.GetID()
						break
					}
				}
			}
		} else {
			logFn("WARN", fmt.Sprintf("Failed to send initial progress message to Telegram (MTProto): %v", err))
		}
	}

	// Use dynamic thread count based on file size (devgagantools-style)
	threads := calculateOptimalThreads(info.Size())

	// Initialize uploader progress tracker
	progressTracker := &uploadProgress{
		job:         job,
		eng:         eng,
		progressMsg: progressMsg,
		fileName:    fileName,
		startTime:   time.Now(),
		lastUpdate:  time.Now(),
		threads:     threads,
		logFn:       logFn,
		gotdClient:  eng.gotdClient,
		gotdPeer:    peer,
		gotdMsgID:   pMsgID,
	}

	logFn("INFO", fmt.Sprintf("Uploading file with %d parallel threads...", threads))
	inputFile, err := FastUploadFile(eng.gotdCtx, eng.gotdClient, safePath, progressTracker)
	if err != nil {
		return fmt.Errorf("file upload failed: %w", err)
	}

	// Generate a JWT download token for the direct download button
	appCfg := config.LoadConfig()
	token := jwt.NewWithClaims(jwt.SigningMethodHS256, jwt.MapClaims{
		"username": "admin",
		"role":     "admin",
		"exp":      time.Now().Add(7 * 24 * time.Hour).Unix(),
	})
	tokenString, err := token.SignedString(appCfg.JWTSecret)
	if err != nil {
		logFn("WARN", fmt.Sprintf("Failed to sign JWT download token: %v", err))
	}

	// Construct absolute download link
	var absoluteDownloadURL string
	downloadPath := fmt.Sprintf("/api/files/stream?path=%s&download=true", url.QueryEscape(payload.FilePath))
	if tokenString != "" {
		downloadPath += fmt.Sprintf("&token=%s", url.QueryEscape(tokenString))
	}

	// Determine base host from Ehco config
	var ehcoCfg models.EhcoClientConfig
	if err := db.DB.First(&ehcoCfg).Error; err == nil && ehcoCfg.RemoteURL != "" {
		domain := ehcoCfg.RemoteURL
		domain = strings.Replace(domain, "wss://", "https://", 1)
		domain = strings.Replace(domain, "ws://", "http://", 1)
		domain = strings.TrimSuffix(domain, "/ws")
		domain = strings.TrimSuffix(domain, "/tunnel")
		domain = strings.TrimSuffix(domain, "/")
		absoluteDownloadURL = fmt.Sprintf("%s%s", domain, downloadPath)
	} else {
		// Fall back to public domain
		absoluteDownloadURL = fmt.Sprintf("https://ondata.ir%s", downloadPath)
	}

	logFn("INFO", "Assembling media post...")
	ext := strings.ToLower(filepath.Ext(fileName))
	caption := fmt.Sprintf("🎬 *CleverConnect Professional Share*\n\n"+
		"📄 *File Name:* `%s`\n"+
		"📏 *File Size:* `%s`\n"+
		"🕒 *Uploaded At:* `%s`\n\n"+
		"⚡ _Powered by CleverConnect Job Scheduler_",
		fileName,
		formatFileSize(info.Size()),
		time.Now().Format("2006-01-02 15:04:05"),
	)

	var mediaOption message.MediaOption
	mimeType := mime.TypeByExtension(ext)
	if mimeType == "" {
		mimeType = "application/octet-stream"
	}

	switch ext {
	case ".jpg", ".jpeg", ".png", ".gif":
		mediaOption = message.UploadedPhoto(inputFile, styling.Plain(caption))
	case ".mp4":
		doc := message.UploadedDocument(inputFile, styling.Plain(caption))
		mediaOption = doc.MIME("video/mp4").Filename(fileName).Video().SupportsStreaming()
	case ".mp3", ".m4a":
		doc := message.UploadedDocument(inputFile, styling.Plain(caption))
		mediaOption = doc.MIME(mimeType).Filename(fileName).Audio()
	case ".ogg", ".opus":
		doc := message.UploadedDocument(inputFile, styling.Plain(caption))
		mediaOption = doc.MIME(mimeType).Filename(fileName).Audio().Voice()
	default:
		doc := message.UploadedDocument(inputFile, styling.Plain(caption))
		doc.MIME(mimeType).Filename(fileName)
		mediaOption = doc
	}

	// Send media post — only attach download button if URL is a valid public HTTPS link
	sender := message.NewSender(api)

	if strings.HasPrefix(absoluteDownloadURL, "https://") {
		kbMarkup := &tg.ReplyInlineMarkup{
			Rows: []tg.KeyboardButtonRow{
				{
					Buttons: []tg.KeyboardButtonClass{
						&tg.KeyboardButtonURL{
							Text: "📥 Download Direct Link",
							URL:  absoluteDownloadURL,
						},
					},
				},
			},
		}
		_, mediaSentErr = sender.To(peer).Markup(kbMarkup).Media(eng.gotdCtx, mediaOption)
	} else {
		_, mediaSentErr = sender.To(peer).Media(eng.gotdCtx, mediaOption)
	}

	if mediaSentErr != nil {
		// Attempt to update the progress message with error
		errMsg := fmt.Sprintf("failed to send media post: %v", mediaSentErr)
		if progressMsg != nil && eng.Bot != nil {
			_, _ = eng.Bot.Edit(progressMsg, fmt.Sprintf("❌ *Upload Failed*\n\nReason: %s", errMsg), &tele.SendOptions{ParseMode: tele.ModeMarkdown})
		} else if pMsgID != 0 && pPeer != nil {
			_, _ = api.MessagesEditMessage(eng.gotdCtx, &tg.MessagesEditMessageRequest{
				Peer:    pPeer,
				ID:      pMsgID,
				Message: fmt.Sprintf("❌ <b>Upload Failed</b>\n\nReason: %s", errMsg),
			})
		}
		return fmt.Errorf("%s", errMsg)
	}

	logFn("INFO", "Media post sent successfully. Cleaning up progress message...")
	if progressMsg != nil && eng.Bot != nil {
		_ = eng.Bot.Delete(progressMsg)
	} else if pMsgID != 0 {
		_, _ = api.MessagesDeleteMessages(eng.gotdCtx, &tg.MessagesDeleteMessagesRequest{
			ID:     []int{pMsgID},
			Revoke: true,
		})
	}

	return nil
}

// resolveInputPeer queries dialogs to resolve chat ID with AccessHash if required.
func resolveInputPeer(ctx context.Context, api *tg.Client, chatID int64) (tg.InputPeerClass, error) {
	// Query dialogs to find peer details
	res, err := api.MessagesGetDialogs(ctx, &tg.MessagesGetDialogsRequest{
		Limit: 100,
	})
	if err == nil {
		switch dialogs := res.(type) {
		case *tg.MessagesDialogsSlice:
			for _, user := range dialogs.Users {
				if user.GetID() == chatID {
					userObj, ok := user.(*tg.User)
					if ok && userObj.AccessHash != 0 {
						return &tg.InputPeerUser{UserID: userObj.ID, AccessHash: userObj.AccessHash}, nil
					}
				}
			}
			for _, chat := range dialogs.Chats {
				if chat.GetID() == chatID {
					switch c := chat.(type) {
					case *tg.Chat:
						return &tg.InputPeerChat{ChatID: c.ID}, nil
					case *tg.Channel:
						if c.AccessHash != 0 {
							return &tg.InputPeerChannel{ChannelID: c.ID, AccessHash: c.AccessHash}, nil
						}
					}
				}
			}
		case *tg.MessagesDialogs:
			for _, user := range dialogs.Users {
				if user.GetID() == chatID {
					userObj, ok := user.(*tg.User)
					if ok && userObj.AccessHash != 0 {
						return &tg.InputPeerUser{UserID: userObj.ID, AccessHash: userObj.AccessHash}, nil
					}
				}
			}
			for _, chat := range dialogs.Chats {
				if chat.GetID() == chatID {
					switch c := chat.(type) {
					case *tg.Chat:
						return &tg.InputPeerChat{ChatID: c.ID}, nil
					case *tg.Channel:
						if c.AccessHash != 0 {
							return &tg.InputPeerChannel{ChannelID: c.ID, AccessHash: c.AccessHash}, nil
						}
					}
				}
			}
		}
	}

	// Fallbacks
	if chatID > 0 {
		return &tg.InputPeerUser{UserID: chatID, AccessHash: 0}, nil
	}

	// Check channel vs chat prefix
	strID := fmt.Sprintf("%d", chatID)
	if strings.HasPrefix(strID, "-100") {
		cleanID := strings.TrimPrefix(strID, "-100")
		var parsed int64
		_, _ = fmt.Sscanf(cleanID, "%d", &parsed)
		return &tg.InputPeerChannel{ChannelID: parsed, AccessHash: 0}, nil
	}

	absID := chatID
	if absID < 0 {
		absID = -absID
	}
	return &tg.InputPeerChat{ChatID: absID}, nil
}

// makeProgressBar creates a sleek visual progress bar
func makeProgressBar(percent int, width int) string {
	completed := percent * width / 100
	if completed < 0 {
		completed = 0
	}
	if completed > width {
		completed = width
	}
	remaining := width - completed

	var sb strings.Builder
	sb.WriteString("[")
	for i := 0; i < completed; i++ {
		sb.WriteString("■")
	}
	for i := 0; i < remaining; i++ {
		sb.WriteString("░")
	}
	sb.WriteString("]")
	return sb.String()
}
