package handlers

import (
	"context"
	"fmt"
	"net"
	"net/http"
	"net/url"
	"strings"
	"time"

	bonding_client "clever-connect/internal/bonding/client"
	"clever-connect/internal/bonding/selector"
	"clever-connect/internal/config"
	"clever-connect/internal/db"
	"clever-connect/internal/db/pebble"
	"clever-connect/internal/logger"
	"clever-connect/internal/models"

	"github.com/gin-gonic/gin"
	"github.com/gorilla/websocket"
)

// BondingHandler handles all DMB Engine API requests.
type BondingHandler struct {
	cfg *config.Config
}

// NewBondingHandler creates a new bonding handler.
func NewBondingHandler(cfg *config.Config) *BondingHandler {
	return &BondingHandler{cfg: cfg}
}

// GetConfig returns the current bonding engine configuration.
func (h *BondingHandler) GetConfig(c *gin.Context) {
	var cfg models.BondingEngineConfig
	if err := db.DB.First(&cfg).Error; err != nil {
		// Return defaults if no config exists
		cfg = models.BondingEngineConfig{
			Mode:          "selector",
			StripingMode:  "auto",
			MaxArteries:   h.cfg.BondingMaxArteries,
			MinArteries:   2,
			SocksPort:     h.cfg.BondingSocksPort,
			HTTPPort:      h.cfg.BondingHTTPPort,
			FrameSize:     h.cfg.BondingFrameSize,
			CombinerURL:   h.cfg.BondingCombinerURL,
			EvalWindowMs:  5000,
			DemoteRTTx:    1.5,
			PromoteRTTx:   1.2,
			LossDemotePct: 5.0,
			CooldownSec:   30,
			ErrorBudget:   5,
		}
	}
	c.JSON(http.StatusOK, cfg)
}

// SaveConfig updates the bonding engine configuration.
func (h *BondingHandler) SaveConfig(c *gin.Context) {
	var input models.BondingEngineConfig
	if err := c.ShouldBindJSON(&input); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	// Upsert: find or create
	var existing models.BondingEngineConfig
	if err := db.DB.First(&existing).Error; err != nil {
		// Create new
		input.ID = 0
		if err := db.DB.Create(&input).Error; err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
			return
		}
	} else {
		// Update existing
		input.ID = existing.ID
		if err := db.DB.Save(&input).Error; err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
			return
		}
	}

	c.JSON(http.StatusOK, gin.H{"status": "ok", "config": input})
}

// StartEngine starts the bonding engine (selector or bonding mode).
func (h *BondingHandler) StartEngine(c *gin.Context) {
	var cfg models.BondingEngineConfig
	if err := db.DB.First(&cfg).Error; err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "no bonding configuration found; save a configuration first"})
		return
	}

	var startErr error
	if cfg.Mode == "bonding" {
		// Mode B: full multipath bonding
		engine := bonding_client.GetBondingEngine()
		startErr = engine.StartEngine(&cfg)
	} else {
		// Mode A: selector/failover (default)
		engine := selector.GetEngine()
		startErr = engine.StartEngine(&cfg)
	}

	if startErr != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": startErr.Error()})
		return
	}

	// Mark as active in DB
	cfg.IsActive = true
	db.DB.Save(&cfg)

	c.JSON(http.StatusOK, gin.H{
		"status": "started",
		"mode":   cfg.Mode,
	})
}

// StopEngine stops the bonding engine.
func (h *BondingHandler) StopEngine(c *gin.Context) {
	// Stop both engines (only the running one will do work)
	selectorEngine := selector.GetEngine()
	_ = selectorEngine.StopEngine()

	bondingEngine := bonding_client.GetBondingEngine()
	_ = bondingEngine.StopEngine()

	// Mark as inactive in DB
	var cfg models.BondingEngineConfig
	if err := db.DB.First(&cfg).Error; err == nil {
		cfg.IsActive = false
		db.DB.Save(&cfg)
	}

	c.JSON(http.StatusOK, gin.H{"status": "stopped"})
}

// GetStatus returns the current engine status with artery details.
func (h *BondingHandler) GetStatus(c *gin.Context) {
	// Check which engine is running
	selectorEngine := selector.GetEngine()
	if selectorEngine.State() == selector.EngineStateRunning {
		c.JSON(http.StatusOK, selectorEngine.GetStatus())
		return
	}

	bondingEngine := bonding_client.GetBondingEngine()
	if bondingEngine.State() == bonding_client.BondingStateRunning {
		c.JSON(http.StatusOK, bondingEngine.GetStatus())
		return
	}

	// Neither is running — return selector status (shows "stopped")
	c.JSON(http.StatusOK, selectorEngine.GetStatus())
}

// ListArteries returns all arteries in the bonding pool from the database.
func (h *BondingHandler) ListArteries(c *gin.Context) {
	var arteries []models.BondingArtery
	if err := db.DB.Find(&arteries).Error; err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, arteries)
}

// ServeTelemetryWS serves a WebSocket connection for live bonding telemetry.
// Routes to whichever engine is currently active (selector or bonding).
func (h *BondingHandler) ServeTelemetryWS(c *gin.Context) {
	upgrader := websocket.Upgrader{
		CheckOrigin: func(r *http.Request) bool { return true },
	}

	conn, err := upgrader.Upgrade(c.Writer, c.Request, nil)
	if err != nil {
		logger.Error("Bonding", "Failed to upgrade WebSocket", "error", err)
		return
	}
	defer conn.Close()

	// Set a read deadline so we detect client disconnects
	conn.SetReadDeadline(time.Now().Add(60 * time.Second))
	conn.SetPongHandler(func(string) error {
		conn.SetReadDeadline(time.Now().Add(60 * time.Second))
		return nil
	})

	// Start a goroutine to read pong/close frames from client
	clientGone := make(chan struct{})
	go func() {
		defer close(clientGone)
		for {
			if _, _, err := conn.ReadMessage(); err != nil {
				return
			}
		}
	}()

	pingTicker := time.NewTicker(20 * time.Second)
	defer pingTicker.Stop()

	// Determine which engine is running and stream from the correct channel
	bondingEngine := bonding_client.GetBondingEngine()
	selectorEngine := selector.GetEngine()

	if bondingEngine.State() == bonding_client.BondingStateRunning {
		// Mode B: stream from bonding engine telemetry
		for {
			select {
			case <-c.Request.Context().Done():
				return
			case <-clientGone:
				return
			case <-pingTicker.C:
				if err := conn.WriteMessage(websocket.PingMessage, nil); err != nil {
					return
				}
			case status, ok := <-bondingEngine.TelemetryChan:
				if !ok {
					return
				}
				if err := conn.WriteJSON(status); err != nil {
					return
				}
			}
		}
	} else {
		// Mode A / default: stream from selector engine telemetry
		for {
			select {
			case <-c.Request.Context().Done():
				return
			case <-clientGone:
				return
			case <-pingTicker.C:
				if err := conn.WriteMessage(websocket.PingMessage, nil); err != nil {
					return
				}
			case status, ok := <-selectorEngine.TelemetryChan:
				if !ok {
					return
				}
				if err := conn.WriteJSON(status); err != nil {
					return
				}
			}
		}
	}
}

// DiagnosticStep represents a single verification step.
type DiagnosticStep struct {
	Name         string `json:"name"`
	Description  string `json:"description"`
	Status       string `json:"status"` // "success", "warning", "error", "pending"
	ErrorMessage string `json:"error_message,omitempty"`
	Details      string `json:"details,omitempty"`
}

// DiagnoseEngine runs step-by-step diagnostic checks on client configuration.
func (h *BondingHandler) DiagnoseEngine(c *gin.Context) {
	var steps []DiagnosticStep

	// Load config from DB
	var cfg models.BondingEngineConfig
	if err := db.DB.First(&cfg).Error; err != nil {
		steps = append(steps, DiagnosticStep{
			Name:         "Database Configuration",
			Description:  "Verify engine configuration exists in database",
			Status:       "error",
			ErrorMessage: "No bonding configuration found. Please save settings first.",
		})
		c.JSON(http.StatusOK, steps)
		return
	}

	steps = append(steps, DiagnosticStep{
		Name:        "Database Configuration",
		Description: "Verify engine configuration exists in database",
		Status:      "success",
		Details:     fmt.Sprintf("Configuration loaded (Mode: %s).", cfg.Mode),
	})

	// 1. Local Ports Check
	socksAddr := fmt.Sprintf("127.0.0.1:%d", cfg.SocksPort)
	socksListener, err := net.Listen("tcp", socksAddr)
	var socksAvailable bool
	if err != nil {
		steps = append(steps, DiagnosticStep{
			Name:         "Local SOCKS Port Availability",
			Description:  fmt.Sprintf("Verify local port %d is free", cfg.SocksPort),
			Status:       "error",
			ErrorMessage: fmt.Sprintf("SOCKS port %d is currently occupied or unavailable: %v", cfg.SocksPort, err),
		})
	} else {
		socksListener.Close()
		socksAvailable = true
		steps = append(steps, DiagnosticStep{
			Name:        "Local SOCKS Port Availability",
			Description: fmt.Sprintf("Verify local port %d is free", cfg.SocksPort),
			Status:      "success",
			Details:     fmt.Sprintf("Local port %d is available.", cfg.SocksPort),
		})
	}

	httpAddr := fmt.Sprintf("127.0.0.1:%d", cfg.HTTPPort)
	httpListener, err := net.Listen("tcp", httpAddr)
	if err != nil {
		steps = append(steps, DiagnosticStep{
			Name:         "Local HTTP Port Availability",
			Description:  fmt.Sprintf("Verify local port %d is free", cfg.HTTPPort),
			Status:       "error",
			ErrorMessage: fmt.Sprintf("HTTP port %d is currently occupied or unavailable: %v", cfg.HTTPPort, err),
		})
	} else {
		httpListener.Close()
		steps = append(steps, DiagnosticStep{
			Name:        "Local HTTP Port Availability",
			Description: fmt.Sprintf("Verify local port %d is free", cfg.HTTPPort),
			Status:      "success",
			Details:     fmt.Sprintf("Local port %d is available.", cfg.HTTPPort),
		})
	}

	// 2. Scanner Pool Check
	configs, _ := pebble.ListClientConfigs(pebble.ConfigFilter{
		PingStatus: "pass",
	}, 0, 0)
	if len(configs) < 2 {
		steps = append(steps, DiagnosticStep{
			Name:         "Scanner Candidate Pool Check",
			Description:  "Verify at least 2 healthy nodes exist in the scanner pool",
			Status:       "warning",
			ErrorMessage: fmt.Sprintf("Found %d healthy nodes. Multipath aggregation requires at least 2 nodes.", len(configs)),
		})
	} else {
		steps = append(steps, DiagnosticStep{
			Name:        "Scanner Candidate Pool Check",
			Description: "Verify at least 2 healthy nodes exist in the scanner pool",
			Status:      "success",
			Details:     fmt.Sprintf("Found %d healthy nodes in the scanner pool.", len(configs)),
		})
	}

	// 3. Mode-specific network combiner dial (only for bonding mode)
	if cfg.Mode == "bonding" {
		if cfg.CombinerURL == "" {
			steps = append(steps, DiagnosticStep{
				Name:         "Server Combiner Connectivity",
				Description:  "Verify reachability of remote combiner server",
				Status:       "error",
				ErrorMessage: "Combiner URL is empty. Please enter a valid combiner URL.",
			})
		} else {
			parsed, err := url.Parse(cfg.CombinerURL)
			if err != nil {
				steps = append(steps, DiagnosticStep{
					Name:         "Server Combiner Connectivity",
					Description:  "Verify reachability of remote combiner server",
					Status:       "error",
					ErrorMessage: fmt.Sprintf("Invalid Combiner URL: %v", err),
				})
			} else {
				hostPort := parsed.Host
				if !strings.Contains(hostPort, ":") {
					if parsed.Scheme == "wss" || parsed.Scheme == "https" {
						hostPort += ":443"
					} else {
						hostPort += ":80"
					}
				}
				conn, err := net.DialTimeout("tcp", hostPort, 2*time.Second)
				if err != nil {
					steps = append(steps, DiagnosticStep{
						Name:         "Server Combiner Connectivity",
						Description:  "Verify reachability of remote combiner server",
						Status:       "error",
						ErrorMessage: fmt.Sprintf("Failed to reach combiner server at %s: %v", hostPort, err),
					})
				} else {
					conn.Close()
					steps = append(steps, DiagnosticStep{
						Name:        "Server Combiner Connectivity",
						Description: "Verify reachability of remote combiner server",
						Status:      "success",
						Details:     fmt.Sprintf("Successfully connected to combiner server at %s.", hostPort),
					})
				}
			}
		}
	}

	// 4. Live Core/Routing Loopback (Checks SOCKS routing if running)
	engineRunning := selector.GetEngine().State() == selector.EngineStateRunning ||
		bonding_client.GetBondingEngine().State() == bonding_client.BondingStateRunning

	if engineRunning && socksAvailable {
		// Test exit path routing through local SOCKS port
		client := &http.Client{
			Transport: &http.Transport{
				DialContext: func(ctx context.Context, network, addr string) (net.Conn, error) {
					return net.DialTimeout("tcp", socksAddr, 2*time.Second)
				},
			},
			Timeout: 3 * time.Second,
		}
		resp, err := client.Get("http://www.google.com/generate_204")
		if err != nil {
			steps = append(steps, DiagnosticStep{
				Name:         "Proxy Traffic Routing Check",
				Description:  "Verify traffic can route through the engine proxy",
				Status:       "warning",
				ErrorMessage: fmt.Sprintf("Traffic routing loopback failed: %v", err),
			})
		} else {
			resp.Body.Close()
			steps = append(steps, DiagnosticStep{
				Name:        "Proxy Traffic Routing Check",
				Description: "Verify traffic can route through the engine proxy",
				Status:      "success",
				Details:     "Traffic successfully routed through proxy (Google 204 received).",
			})
		}
	} else {
		steps = append(steps, DiagnosticStep{
			Name:        "Proxy Traffic Routing Check",
			Description: "Verify traffic can route through the engine proxy",
			Status:      "warning",
			Details:     "Skipped: Proxy loopback test requires the engine to be running.",
		})
	}

	c.JSON(http.StatusOK, steps)
}

