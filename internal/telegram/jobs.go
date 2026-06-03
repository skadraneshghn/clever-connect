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

// TelegramDownloadPayload represents the JSON payload for a Telegram download job.
type TelegramDownloadPayload struct {
	ChatID    int64 `json:"chat_id"`
	MessageID int   `json:"message_id"`
}

// getMessage retrieves a specific message from Telegram by its ID.
func (e *Engine) getMessage(ctx context.Context, api *tg.Client, peer tg.InputPeerClass, msgID int) (*tg.Message, error) {
	var messagesSlice []tg.MessageClass

	switch p := peer.(type) {
	case *tg.InputPeerChannel:
		channelInput := &tg.InputChannel{
			ChannelID:  p.ChannelID,
			AccessHash: p.AccessHash,
		}
		res, err := api.ChannelsGetMessages(ctx, &tg.ChannelsGetMessagesRequest{
			Channel: channelInput,
			ID:      []tg.InputMessageClass{&tg.InputMessageID{ID: msgID}},
		})
		if err != nil {
			return nil, err
		}
		switch val := res.(type) {
		case *tg.MessagesMessagesSlice:
			messagesSlice = val.Messages
		case *tg.MessagesMessages:
			messagesSlice = val.Messages
		case *tg.MessagesChannelMessages:
			messagesSlice = val.Messages
		}
	default:
		res, err := api.MessagesGetMessages(ctx, []tg.InputMessageClass{&tg.InputMessageID{ID: msgID}})
		if err != nil {
			return nil, err
		}
		switch val := res.(type) {
		case *tg.MessagesMessagesSlice:
			messagesSlice = val.Messages
		case *tg.MessagesMessages:
			messagesSlice = val.Messages
		}
	}

	if len(messagesSlice) == 0 {
		return nil, fmt.Errorf("message not found")
	}

	msg, ok := messagesSlice[0].(*tg.Message)
	if !ok {
		return nil, fmt.Errorf("not a message type")
	}

	return msg, nil
}

// RunTelegramDownloadJob executes a parallel multi-connection file download from Telegram.
func RunTelegramDownloadJob(ctx context.Context, job *models.SchedulerJob, logFn func(level, message string)) error {
	logFn("INFO", "Telegram download job started")

	var payload TelegramDownloadPayload
	if err := json.Unmarshal([]byte(job.Payload), &payload); err != nil {
		return fmt.Errorf("invalid payload: %w", err)
	}

	eng := GetEngine()
	if eng == nil {
		return fmt.Errorf("telegram bot engine is not initialized or running")
	}


	if eng.gotdClient == nil {
		return fmt.Errorf("MTProto client is not initialized")
	}

	api := tg.NewClient(eng.gotdClient)

	// 1. Resolve peer
	peer, err := resolveInputPeer(eng.gotdCtx, api, payload.ChatID)
	if err != nil {
		return fmt.Errorf("failed to resolve peer for chat ID: %w", err)
	}

	// 2. Fetch the Telegram message containing the file
	logFn("INFO", fmt.Sprintf("Fetching message ID %d from chat %d", payload.MessageID, payload.ChatID))
	msg, err := eng.getMessage(eng.gotdCtx, api, peer, payload.MessageID)
	if err != nil {
		return fmt.Errorf("failed to fetch message: %w", err)
	}

	// 3. Inspect message media
	if msg.Media == nil {
		return fmt.Errorf("message does not contain any media/file")
	}

	var fileLocation tg.InputFileLocationClass
	var fileSize int64
	var fileName string
	var hasFile bool

	switch media := msg.Media.(type) {
	case *tg.MessageMediaDocument:
		if doc, ok := media.Document.(*tg.Document); ok {
			fileSize = doc.Size
			hasFile = true
			fileLocation = &tg.InputDocumentFileLocation{
				ID:            doc.ID,
				AccessHash:    doc.AccessHash,
				FileReference: doc.FileReference,
			}
			for _, attr := range doc.Attributes {
				if fAttr, ok := attr.(*tg.DocumentAttributeFilename); ok {
					fileName = fAttr.FileName
					break
				}
			}
			if fileName == "" {
				ext := "bin"
				if mimeType := doc.MimeType; mimeType != "" {
					if exts, err := mime.ExtensionsByType(mimeType); err == nil && len(exts) > 0 {
						ext = strings.TrimPrefix(exts[0], ".")
					}
				}
				fileName = fmt.Sprintf("document_%d.%s", doc.ID, ext)
			}
		}
	case *tg.MessageMediaPhoto:
		if photo, ok := media.Photo.(*tg.Photo); ok {
			hasFile = true
			fileName = fmt.Sprintf("photo_%d.jpg", photo.ID)
			fileLocation = &tg.InputPhotoFileLocation{
				ID:            photo.ID,
				AccessHash:    photo.AccessHash,
				FileReference: photo.FileReference,
				ThumbSize:     "x",
			}
			for _, size := range photo.Sizes {
				switch s := size.(type) {
				case *tg.PhotoSize:
					if s.Size > int(fileSize) {
						fileSize = int64(s.Size)
					}
				case *tg.PhotoSizeProgressive:
					if len(s.Sizes) > 0 {
						last := s.Sizes[len(s.Sizes)-1]
						if last > int(fileSize) {
							fileSize = int64(last)
						}
					}
				}
			}
			if fileSize == 0 {
				fileSize = 1024 * 1024
			}
		}
	}

	if !hasFile || fileLocation == nil {
		return fmt.Errorf("no downloadable document or photo found in message media")
	}

	// 4. Determine save path
	relPath := filepath.Join("Downloads/telegram/files", fileName)
	safePath, err := securePath(relPath)
	if err != nil {
		return fmt.Errorf("invalid save path: %w", err)
	}

	logFn("INFO", fmt.Sprintf("Downloading %s (size %s) to %s", fileName, FormatFileSize(fileSize), safePath))

	// 5. Send initial progress message
	var progressMsg *tele.Message
	var pMsgID int
	pBar := makeProgressBar(0, 20)
	initialText := fmt.Sprintf("📥 *Starting Download*\n\n📄 *File:* `%s`\n📏 *Size:* %s\n\n%s `0%%`",
		fileName, FormatFileSize(fileSize), pBar)

	if eng.Bot != nil {
		msg, err := eng.Bot.Send(tele.ChatID(payload.ChatID), initialText, &tele.SendOptions{ParseMode: tele.ModeMarkdown})
		if err != nil {
			logFn("WARN", fmt.Sprintf("Failed to send initial download progress message to Telegram: %v", err))
		} else {
			progressMsg = msg
		}
	} else {
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

	// 6. Download with progress callback
	lastUpdate := time.Now()
	startTime := time.Now()

	err = FastDownloadFile(eng.gotdCtx, eng.gotdClient, fileLocation, safePath, fileSize, func(downloaded, total int64) {
		percent := int(100 * float64(downloaded) / float64(total))
		if percent > 100 {
			percent = 100
		}

		// Update job progress in database
		db.DB.Model(job).Updates(map[string]interface{}{
			"progress": percent,
			"message":  fmt.Sprintf("Downloading: %s / %s (%d%%)", FormatFileSize(downloaded), FormatFileSize(total), percent),
		})

		// Throttle updates
		if time.Since(lastUpdate) > 1500*time.Millisecond {
			lastUpdate = time.Now()
			elapsed := time.Since(startTime).Seconds()
			speed := 0.0
			if elapsed > 0 {
				speed = float64(downloaded) / elapsed / (1024 * 1024) // MB/s
			}
			pBar := makeProgressBar(percent, 20)
			progressText := fmt.Sprintf(
				"📥 *Downloading File*\n\n"+
					"📄 *File:* `%s`\n"+
					"📏 *Downloaded:* %s of %s (%d%%)\n"+
					"⚡ *Speed:* %.2f MB/s\n\n"+
					"%s",
				fileName,
				FormatFileSize(downloaded),
				FormatFileSize(total),
				percent,
				speed,
				pBar,
			)

			if progressMsg != nil && eng.Bot != nil {
				_, _ = eng.Bot.Edit(progressMsg, progressText, &tele.SendOptions{ParseMode: tele.ModeMarkdown})
			} else if pMsgID != 0 {
				htmlText := mdToHTML(progressText)
				_, _ = api.MessagesEditMessage(eng.gotdCtx, &tg.MessagesEditMessageRequest{
					Peer:    peer,
					ID:      pMsgID,
					Message: htmlText,
				})
			}
		}
	})

	if err != nil {
		errMsg := fmt.Sprintf("Failed to download file: %v", err)
		if progressMsg != nil && eng.Bot != nil {
			_, _ = eng.Bot.Edit(progressMsg, fmt.Sprintf("❌ *Download Failed*\n\nReason: %s", errMsg), &tele.SendOptions{ParseMode: tele.ModeMarkdown})
		} else if pMsgID != 0 {
			_, _ = api.MessagesEditMessage(eng.gotdCtx, &tg.MessagesEditMessageRequest{
				Peer:    peer,
				ID:      pMsgID,
				Message: fmt.Sprintf("❌ <b>Download Failed</b>\n\nReason: %s", errMsg),
			})
		}
		return err
	}

	// 7. Success! Generate download URL
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

	var absoluteDownloadURL string
	downloadPath := fmt.Sprintf("/api/files/stream?path=%s&download=true", url.QueryEscape(relPath))
	if tokenString != "" {
		downloadPath += fmt.Sprintf("&token=%s", url.QueryEscape(tokenString))
	}

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
		absoluteDownloadURL = fmt.Sprintf("https://ondata.ir%s", downloadPath)
	}

	successText := fmt.Sprintf(
		"✅ *Download Completed!*\n\n"+
			"📄 *File Name:* `%s`\n"+
			"📏 *File Size:* `%s`\n"+
			"📁 *Saved Path:* `%s`\n\n"+
			"⚡ _Powered by CleverConnect Job Scheduler_",
		fileName,
		FormatFileSize(fileSize),
		relPath,
	)

	logFn("INFO", "File downloaded successfully. Updating Telegram message with download link...")

	if progressMsg != nil && eng.Bot != nil {
		kb := &tele.ReplyMarkup{}
		btn := kb.URL("📥 Download Direct Link", absoluteDownloadURL)
		kb.Inline(kb.Row(btn))
		_, _ = eng.Bot.Edit(progressMsg, successText, &tele.SendOptions{ParseMode: tele.ModeMarkdown, ReplyMarkup: kb})
	} else if pMsgID != 0 {
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
		htmlText := mdToHTML(successText)
		_, _ = api.MessagesEditMessage(eng.gotdCtx, &tg.MessagesEditMessageRequest{
			Peer:        peer,
			ID:          pMsgID,
			Message:     htmlText,
			ReplyMarkup: kbMarkup,
		})
	}

	return nil
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
