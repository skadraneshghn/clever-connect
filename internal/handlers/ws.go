package handlers

import (
	"math/rand"
	"net/http"
	"time"

	"clever-connect/internal/config"
	"clever-connect/internal/logger"

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
				cpu := rand.Intn(20) + 15
				memory := 44 + rand.Intn(4) - 2
				disk := 18
				conns := 4

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
					"type":          "telemetry",
					"cpu":           cpu,
					"memory":        memory,
					"disk":          disk,
					"connsCount":    conns,
					"uploadSpeed":   uploadSpeed,
					"downloadSpeed": downloadSpeed,
					"totalDownload": totalDownload,
					"totalUpload":   totalUpload,
					"clients":       clients,
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
