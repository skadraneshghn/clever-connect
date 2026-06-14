package handlers

import (
	"context"
	"encoding/json"
	"net/http"
	"time"

	"clever-connect/internal/logger"
	"clever-connect/internal/models"
	"clever-connect/internal/v2ray/tester"

	"github.com/gin-gonic/gin"
	"github.com/gorilla/websocket"
)

// ServeWSV2RayTest upgrades connection to websocket and handles real-time testing
func (h *V2RayHandler) ServeWSV2RayTest(c *gin.Context) {
	conn, err := upgrader.Upgrade(c.Writer, c.Request, nil)
	if err != nil {
		logger.Error("WS-Test", "WebSocket upgrade failed", "error", err.Error())
		return
	}
	defer conn.Close()

	logger.Info("WS-Test", "WebSocket client connected for config testing")

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	defer tester.StopTesting()

	msgChan := make(chan tester.ResultMessage, 100)

	// Writer loop
	go func() {
		for {
			select {
			case <-ctx.Done():
				return
			case msg, ok := <-msgChan:
				if !ok {
					return
				}
				bytes, err := json.Marshal(msg)
				if err != nil {
					continue
				}
				if err := conn.WriteMessage(websocket.TextMessage, bytes); err != nil {
					logger.Warn("WS-Test", "Failed to write message to client", "error", err.Error())
					cancel()
					return
				}
			}
		}
	}()

	// Reader loop
	for {
		_, messageBytes, err := conn.ReadMessage()
		if err != nil {
			logger.Info("WS-Test", "WebSocket client disconnected")
			break
		}

		var req struct {
			Action      string `json:"action"` // "start" or "stop"
			IDs         []uint `json:"ids"`
			TestType    string `json:"test_type"` // "tcp_ping", "tls_ping", "real_url"
			Concurrency int    `json:"concurrency"`
			TimeoutMs   int    `json:"timeout_ms"`
			URL         string `json:"url"`
			Core        string `json:"core"`
			DelayMs     int    `json:"delay_ms"`
		}

		if err := json.Unmarshal(messageBytes, &req); err != nil {
			logger.Warn("WS-Test", "Failed to parse incoming message", "error", err.Error())
			continue
		}

		if req.Action == "stop" {
			tester.StopTesting()
			msgChan <- tester.ResultMessage{
				Type:   "status",
				Status: "stopped",
			}
			continue
		}

		if req.Action == "start" {
			tester.StopTesting()

			timeout := 5 * time.Second
			if req.TimeoutMs > 0 {
				timeout = time.Duration(req.TimeoutMs) * time.Millisecond
			}

			delay := 200 * time.Millisecond
			if req.DelayMs > 0 {
				delay = time.Duration(req.DelayMs) * time.Millisecond
			}

			opts := tester.TestOptions{
				IDs:                req.IDs,
				TestType:           req.TestType,
				Concurrency:        req.Concurrency,
				Timeout:            timeout,
				URL:                req.URL,
				Core:               req.Core,
				DelayBetweenSameIP: delay,
			}

			err := tester.StartTesting(opts, func(msg tester.ResultMessage) {
				select {
				case <-ctx.Done():
				case msgChan <- msg:
				default:
				}
			})

			if err != nil {
				msgChan <- tester.ResultMessage{
					Type:   "status",
					Status: "error",
					Result: &tester.ConfigTestResult{Error: err.Error()},
				}
			}
		}
	}
}

func (h *V2RayHandler) TestConfigDirect(c *gin.Context) {
	var req struct {
		Config     models.V2RayClientConfig `json:"config"`
		TestType   string                   `json:"test_type"`
		Core       string                   `json:"core"`
		TimeoutSec int                      `json:"timeout_sec"`
		URL        string                   `json:"url"`
	}

	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Invalid request payload: " + err.Error()})
		return
	}

	timeout := 5 * time.Second
	if req.TimeoutSec > 0 {
		timeout = time.Duration(req.TimeoutSec) * time.Second
	}

	opts := tester.TestOptions{
		TestType: req.TestType,
		Core:     req.Core,
		Timeout:  timeout,
		URL:      req.URL,
	}

	if opts.TestType == "" {
		opts.TestType = "tls_ping"
	}

	ctx, cancel := context.WithTimeout(context.Background(), timeout+3*time.Second)
	defer cancel()

	res := tester.TestSingleConfig(ctx, req.Config, opts)

	c.JSON(http.StatusOK, res)
}

