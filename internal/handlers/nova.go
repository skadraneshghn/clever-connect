package handlers

import (
	"crypto/rand"
	"crypto/subtle"
	"encoding/hex"
	"io"
	"net/http"
	"net/http/httputil"
	"net/url"
	"strconv"
	"strings"

	"clever-connect/internal/config"
	"clever-connect/internal/db"
	"clever-connect/internal/logger"
	"clever-connect/internal/models"

	"github.com/gin-gonic/gin"
)

type NovaForwarderHandler struct {
	cfg *config.Config
}

func NewNovaForwarderHandler(cfg *config.Config) *NovaForwarderHandler {
	return &NovaForwarderHandler{cfg: cfg}
}

// GenerateNovaRandomToken generates a hex-encoded random security token for the Nova proxy auth
func GenerateNovaRandomToken() string {
	b := make([]byte, 16)
	if _, err := rand.Read(b); err != nil {
		return "clever-nova-default-token"
	}
	return hex.EncodeToString(b)
}

// proxyToServer automatically forwards requests from the Client Panel to the remote Clever Cloud server.
func (h *NovaForwarderHandler) proxyToServer(c *gin.Context, method string, apiPath string) bool {
	if h.cfg.AppMode == "server" {
		return false
	}

	if h.cfg.ServerURL == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "No remote server API connection configured (missing SERVER_URL in environment)"})
		return true
	}

	remoteURLTarget := strings.TrimSpace(h.cfg.ServerURL)
	remoteToken := strings.TrimSpace(h.cfg.ServerAuthToken)

	remoteHost := remoteURLTarget
	remoteHost = strings.Replace(remoteHost, "wss://", "https://", 1)
	remoteHost = strings.Replace(remoteHost, "ws://", "http://", 1)
	if idx := strings.Index(remoteHost, "/ws"); idx != -1 {
		remoteHost = remoteHost[:idx]
	}
	if idx := strings.Index(remoteHost, "/tunnel"); idx != -1 {
		remoteHost = remoteHost[:idx]
	}
	remoteHost = strings.TrimSuffix(remoteHost, "/")

	remoteURL := remoteHost + apiPath
	if c.Request.URL.RawQuery != "" {
		remoteURL += "?" + c.Request.URL.RawQuery
	}

	var reqBody io.Reader
	if method != http.MethodGet && method != http.MethodHead && method != http.MethodOptions && method != http.MethodDelete {
		reqBody = c.Request.Body
	}

	req, err := http.NewRequest(method, remoteURL, reqBody)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to create proxy request", "details": err.Error()})
		return true
	}

	for k, vv := range c.Request.Header {
		for _, v := range vv {
			req.Header.Add(k, v)
		}
	}
	if remoteToken != "" {
		req.Header.Set("Authorization", "Bearer "+remoteToken)
	}

	client := &http.Client{}
	resp, err := client.Do(req)
	if err != nil {
		c.JSON(http.StatusBadGateway, gin.H{"error": "Remote server connection refused or timed out", "details": err.Error()})
		return true
	}
	defer resp.Body.Close()

	if resp.StatusCode == http.StatusUnauthorized {
		c.JSON(http.StatusBadGateway, gin.H{"error": "Remote server rejected proxy token (401). Please update the remote server or verify your Auth Token."})
		return true
	}

	for k, vv := range resp.Header {
		for _, v := range vv {
			c.Writer.Header().Add(k, v)
		}
	}
	c.Writer.WriteHeader(resp.StatusCode)
	_, _ = io.Copy(c.Writer, resp.Body)
	return true
}

// GetTargetPort resolves the internal target loopback port dynamically
func (h *NovaForwarderHandler) GetTargetPort() int {
	var config models.NovaForwarderConfig
	if err := db.DB.First(&config).Error; err == nil {
		if config.AssignedCorePort > 0 {
			return config.AssignedCorePort
		}
	}

	// Fallback dynamic lookup: check first enabled inbound
	var inbound models.V2RayInbound
	if err := db.DB.Where("enabled = ?", true).First(&inbound).Error; err == nil && inbound.Port > 0 {
		return inbound.Port
	}

	return 8081 // standard default fallback
}

// HandleForwardStream is the transparent network frame forward pass-through endpoint
func (h *NovaForwarderHandler) HandleForwardStream(c *gin.Context) {
	var config models.NovaForwarderConfig
	if err := db.DB.First(&config).Error; err != nil {
		// Auto-seed/generate config if not initialized to prevent blocking first-time setup
		config = models.NovaForwarderConfig{
			SecretAuthKey:    GenerateNovaRandomToken(),
			AssignedCorePort: 8081,
			IsEnabled:        true,
		}
		if err := db.DB.Create(&config).Error; err != nil {
			logger.Error("NovaForwarder", "Failed to auto-seed configuration", "error", err)
			c.AbortWithStatusJSON(http.StatusInternalServerError, gin.H{"error": "Failed to initialize Nova forwarder configuration"})
			return
		}
	}

	if !config.IsEnabled {
		c.AbortWithStatusJSON(http.StatusForbidden, gin.H{"error": "Nova forwarder pipeline is disabled"})
		return
	}

	authKey := c.GetHeader("X-Nova-Auth")
	if subtle.ConstantTimeCompare([]byte(authKey), []byte(config.SecretAuthKey)) != 1 {
		c.AbortWithStatusJSON(http.StatusUnauthorized, gin.H{"error": "Unauthorized pipeline ingress"})
		return
	}

	targetPort := h.GetTargetPort()
	targetURL, err := url.Parse("http://127.0.0.1:" + strconv.Itoa(targetPort))
	if err != nil {
		c.AbortWithStatusJSON(http.StatusInternalServerError, gin.H{"error": "Failed to parse target loopback URL"})
		return
	}

	logger.Info("NovaForwarder", "Multiplexing proxy traffic to loopback core", "port", targetPort, "path", c.Request.URL.Path)

	proxy := httputil.NewSingleHostReverseProxy(targetURL)
	proxy.Director = func(req *http.Request) {
		req.Header = c.Request.Header
		req.Host = targetURL.Host
		req.URL.Scheme = targetURL.Scheme
		req.URL.Host = targetURL.Host
		req.URL.Path = c.Request.URL.Path
	}

	proxy.ServeHTTP(c.Writer, c.Request)
}

// GetConfig handles GET /api/cloudflare/forwarder
func (h *NovaForwarderHandler) GetConfig(c *gin.Context) {
	if h.proxyToServer(c, c.Request.Method, c.Request.URL.Path) {
		return
	}

	var config models.NovaForwarderConfig
	if err := db.DB.First(&config).Error; err != nil {
		// Auto-seed/generate config
		config = models.NovaForwarderConfig{
			SecretAuthKey:    GenerateNovaRandomToken(),
			AssignedCorePort: 8081,
			IsEnabled:        true,
		}
		if err := db.DB.Create(&config).Error; err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to seed forwarder config: " + err.Error()})
			return
		}
	}
	c.JSON(http.StatusOK, config)
}

// UpdateConfig handles POST /api/cloudflare/forwarder
func (h *NovaForwarderHandler) UpdateConfig(c *gin.Context) {
	if h.proxyToServer(c, c.Request.Method, c.Request.URL.Path) {
		return
	}

	var req struct {
		SecretAuthKey    string `json:"secret_auth_key"`
		AssignedCorePort int    `json:"assigned_core_port"`
		IsEnabled        bool   `json:"is_enabled"`
	}

	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Invalid request body"})
		return
	}

	var config models.NovaForwarderConfig
	if err := db.DB.First(&config).Error; err != nil {
		config = models.NovaForwarderConfig{
			SecretAuthKey:    req.SecretAuthKey,
			AssignedCorePort: req.AssignedCorePort,
			IsEnabled:        req.IsEnabled,
		}
		if err := db.DB.Create(&config).Error; err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to create forwarder config: " + err.Error()})
			return
		}
	} else {
		config.SecretAuthKey = req.SecretAuthKey
		config.AssignedCorePort = req.AssignedCorePort
		config.IsEnabled = req.IsEnabled
		if err := db.DB.Save(&config).Error; err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to update forwarder config: " + err.Error()})
			return
		}
	}

	c.JSON(http.StatusOK, config)
}
