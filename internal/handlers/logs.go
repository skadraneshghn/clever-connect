package handlers

import (
	"bufio"
	"net/http"
	"os"
	"strings"
	"time"

	"clever-connect/internal/logger"

	"github.com/gin-gonic/gin"
	"github.com/gorilla/websocket"
)

type LogMessage struct {
	Timestamp string                 `json:"timestamp"`
	Level     string                 `json:"level"`
	Component string                 `json:"component"`
	Message   string                 `json:"message"`
	Caller    string                 `json:"caller"`
	Fields    map[string]interface{} `json:"fields,omitempty"`
	Raw       string                 `json:"raw"`
}

// ServeLogWS upgrades connection and streams real-time system logs.
func ServeLogWS(c *gin.Context) {
	conn, err := upgrader.Upgrade(c.Writer, c.Request, nil)
	if err != nil {
		logger.Error("WS", "WebSocket log upgrade failed", "error", err.Error(), "ip", c.ClientIP())
		return
	}
	defer conn.Close()

	logger.Info("WS", "Log streaming client connected", "ip", c.ClientIP())

	// Create buffered channel for this log listener
	logChan := make(chan *logger.Entry, 1024)
	logger.RegisterListener(logChan)
	defer logger.UnregisterListener(logChan)

	// Stream existing log entries from today first so the user sees some recent history!
	sendHistory(conn)

	// Read loop to detect client disconnect
	disconnectChan := make(chan struct{})
	go func() {
		for {
			if _, _, err := conn.ReadMessage(); err != nil {
				close(disconnectChan)
				return
			}
		}
	}()

	// Loop pumping logs
	for {
		select {
		case entry := <-logChan:
			msg := LogMessage{
				Timestamp: entry.Time.Format("15:04:05.000"),
				Level:     entry.Level.String(),
				Component: entry.Component,
				Message:   entry.Message,
				Caller:    entry.Caller,
				Fields:    entry.Fields,
				Raw:       strings.TrimSuffix(logger.FormatEntry(entry), "\n"),
			}

			if err := conn.WriteJSON(msg); err != nil {
				logger.Warn("WS", "Log streaming client disconnected — write failed", "ip", c.ClientIP())
				return
			}

		case <-disconnectChan:
			logger.Info("WS", "Log streaming client disconnected", "ip", c.ClientIP())
			return
		}
	}
}

// DownloadTodayLog handles the HTTP request to download the raw daily log file.
func DownloadTodayLog(c *gin.Context) {
	path := logger.GetTodayLogFilePath()
	if path == "" {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to determine log file path"})
		return
	}

	// Check if file exists
	if _, err := os.Stat(path); os.IsNotExist(err) {
		c.JSON(http.StatusNotFound, gin.H{"error": "Today's log file does not exist yet"})
		return
	}

	// Serve the file as download
	c.Header("Content-Description", "File Transfer")
	c.Header("Content-Disposition", "attachment; filename="+os.Getenv("APP_MODE")+"_today.log")
	c.Header("Content-Type", "text/plain")
	c.File(path)
}

// sendHistory parses today's daily log file and sends the last 150 lines to the socket
// so that the logging panel has initial immediate context.
func sendHistory(conn *websocket.Conn) {
	path := logger.GetTodayLogFilePath()
	if path == "" {
		return
	}

	file, err := os.Open(path)
	if err != nil {
		return
	}
	defer file.Close()

	// Read all lines
	var lines []string
	scanner := bufio.NewScanner(file)
	for scanner.Scan() {
		lines = append(lines, scanner.Text())
	}

	// Take the last 150 lines max
	start := 0
	if len(lines) > 150 {
		start = len(lines) - 150
	}

	for i := start; i < len(lines); i++ {
		line := lines[i]
		if strings.TrimSpace(line) == "" || strings.HasPrefix(line, "═══") {
			continue
		}

		// Parse the line back into structured format if possible, otherwise send as raw log
		// Format: 2026-05-31 20:06:00.524 │ INFO  │ Core     │ Message  ‹fields›  @ caller
		parts := strings.SplitN(line, " │ ", 4)
		if len(parts) < 4 {
			// Send raw line
			_ = conn.WriteJSON(LogMessage{
				Timestamp: time.Now().Format("15:04:05.000"),
				Level:     "INFO",
				Component: "History",
				Message:   line,
				Raw:       line,
			})
			continue
		}

		timeStr := parts[0]
		level := strings.TrimSpace(parts[1])
		comp := strings.TrimSpace(parts[2])
		rest := parts[3]

		// Extract caller if present (ends with @ filename:line)
		caller := ""
		if idx := strings.LastIndex(rest, "  @ "); idx > 0 {
			caller = strings.TrimSpace(rest[idx+4:])
			rest = rest[:idx]
		}

		// Extract fields if present (surrounded by  ‹...›)
		var fields map[string]interface{}
		if idxStart := strings.LastIndex(rest, "  ‹"); idxStart > 0 {
			fieldsStr := rest[idxStart+3:]
			if idxEnd := strings.Index(fieldsStr, "›"); idxEnd > 0 {
				fieldsStr = fieldsStr[:idxEnd]
				fields = make(map[string]interface{})
				for _, pair := range strings.Split(fieldsStr, ", ") {
					kv := strings.SplitN(pair, "=", 2)
					if len(kv) == 2 {
						fields[kv[0]] = kv[1]
					}
				}
			}
			rest = rest[:idxStart]
		}

		// Get standard 15:04:05.000 format from full YYYY-MM-DD time
		tParts := strings.Split(timeStr, " ")
		stamp := timeStr
		if len(tParts) == 2 {
			stamp = tParts[1]
		}

		_ = conn.WriteJSON(LogMessage{
			Timestamp: stamp,
			Level:     level,
			Component: comp,
			Message:   strings.TrimSpace(rest),
			Caller:    caller,
			Fields:    fields,
			Raw:       line,
		})
	}
}
