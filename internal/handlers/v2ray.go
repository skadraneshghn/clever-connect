package handlers

import (
	"bytes"
	"crypto/rand"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"clever-connect/internal/config"
	"clever-connect/internal/db"
	"clever-connect/internal/logger"
	"clever-connect/internal/models"
	"clever-connect/internal/v2ray/compiler"
	"clever-connect/internal/v2ray/core"
	"clever-connect/internal/v2ray/scanner"
	"clever-connect/internal/v2ray/speed"
	"clever-connect/internal/v2ray/sub"
	"clever-connect/internal/v2ray/traffic"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
	"github.com/jung-kurt/gofpdf/v2"
	"github.com/tuotoo/qrcode"
	"rsc.io/qr"
)

type V2RayHandler struct {
	cfg *config.Config
}

func NewV2RayHandler(cfg *config.Config) *V2RayHandler {
	return &V2RayHandler{cfg: cfg}
}

// GenerateRandomUUID generates a new UUID v4
func GenerateRandomUUID() string {
	return uuid.New().String()
}

// GenerateSubToken generates a random sub token
func GenerateSubToken() string {
	b := make([]byte, 16)
	_, _ = rand.Read(b)
	return hex.EncodeToString(b)
}

// ──────────────────────────────────────────────────────────────────────────────
// SERVER CORE CONTROL API
// ──────────────────────────────────────────────────────────────────────────────

// GetCoreStatus handles GET /api/v2ray/core/status
func (h *V2RayHandler) GetCoreStatus(c *gin.Context) {
	c.JSON(http.StatusOK, gin.H{
		"is_running": core.IsRunning(),
		"bin_path":   core.GetXrayBinPath(),
	})
}

// StartCore handles POST /api/v2ray/core/start
func (h *V2RayHandler) StartCore(c *gin.Context) {
	if err := traffic.ReloadCoreConfig(); err != nil {
		logger.Error("V2Ray", "Failed to start/reload Xray core", "error", err)
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	traffic.StartInterceptor()
	c.JSON(http.StatusOK, gin.H{"status": "started", "is_running": true})
}

// StopCore handles POST /api/v2ray/core/stop
func (h *V2RayHandler) StopCore(c *gin.Context) {
	traffic.StopInterceptor()
	if err := core.StopCore(); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, gin.H{"status": "stopped", "is_running": false})
}

// ──────────────────────────────────────────────────────────────────────────────
// SERVER INBOUNDS API
// ──────────────────────────────────────────────────────────────────────────────

// ListInbounds handles GET /api/v2ray/inbounds
func (h *V2RayHandler) ListInbounds(c *gin.Context) {
	var inbounds []models.V2RayInbound
	if err := db.DB.Find(&inbounds).Error; err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, inbounds)
}

// CreateInbound handles POST /api/v2ray/inbounds
func (h *V2RayHandler) CreateInbound(c *gin.Context) {
	var in models.V2RayInbound
	if err := c.ShouldBindJSON(&in); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	if in.Protocol == "vless" && in.TLSMode == "reality" {
		if in.RealityPrivateKey == "" || in.RealityPublicKey == "" {
			// Generate short keys if missing
			// We can generate standard reality keys, or let the user supply them.
			// Let's set placeholders or log a warning.
		}
	}

	if err := db.DB.Create(&in).Error; err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	// Auto-reload to apply changes if running
	if core.IsRunning() {
		_ = traffic.ReloadCoreConfig()
	}

	c.JSON(http.StatusCreated, in)
}

// UpdateInbound handles PUT /api/v2ray/inbounds/:id
func (h *V2RayHandler) UpdateInbound(c *gin.Context) {
	id, _ := strconv.Atoi(c.Param("id"))
	var in models.V2RayInbound
	if err := db.DB.First(&in, id).Error; err != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "Inbound not found"})
		return
	}

	if err := c.ShouldBindJSON(&in); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	if err := db.DB.Save(&in).Error; err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	if core.IsRunning() {
		_ = traffic.ReloadCoreConfig()
	}

	c.JSON(http.StatusOK, in)
}

// DeleteInbound handles DELETE /api/v2ray/inbounds/:id
func (h *V2RayHandler) DeleteInbound(c *gin.Context) {
	id, _ := strconv.Atoi(c.Param("id"))
	if err := db.DB.Delete(&models.V2RayInbound{}, id).Error; err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	if core.IsRunning() {
		_ = traffic.ReloadCoreConfig()
	}

	c.JSON(http.StatusOK, gin.H{"status": "deleted"})
}

// ──────────────────────────────────────────────────────────────────────────────
// SERVER USERS API
// ──────────────────────────────────────────────────────────────────────────────

// ListUsers handles GET /api/v2ray/users
func (h *V2RayHandler) ListUsers(c *gin.Context) {
	var users []models.V2RayUser
	if err := db.DB.Find(&users).Error; err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, users)
}

// CreateUser handles POST /api/v2ray/users
func (h *V2RayHandler) CreateUser(c *gin.Context) {
	var u models.V2RayUser
	if err := c.ShouldBindJSON(&u); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	if u.UUID == "" {
		u.UUID = GenerateRandomUUID()
	}
	if u.SubToken == "" {
		u.SubToken = GenerateSubToken()
	}

	if err := db.DB.Create(&u).Error; err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	if core.IsRunning() {
		_ = traffic.ReloadCoreConfig()
	}

	c.JSON(http.StatusCreated, u)
}

// UpdateUser handles PUT /api/v2ray/users/:id
func (h *V2RayHandler) UpdateUser(c *gin.Context) {
	id, _ := strconv.Atoi(c.Param("id"))
	var u models.V2RayUser
	if err := db.DB.First(&u, id).Error; err != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "User not found"})
		return
	}

	if err := c.ShouldBindJSON(&u); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	if err := db.DB.Save(&u).Error; err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	if core.IsRunning() {
		_ = traffic.ReloadCoreConfig()
	}

	c.JSON(http.StatusOK, u)
}

// DeleteUser handles DELETE /api/v2ray/users/:id
func (h *V2RayHandler) DeleteUser(c *gin.Context) {
	id, _ := strconv.Atoi(c.Param("id"))
	if err := db.DB.Delete(&models.V2RayUser{}, id).Error; err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	if core.IsRunning() {
		_ = traffic.ReloadCoreConfig()
	}

	c.JSON(http.StatusOK, gin.H{"status": "deleted"})
}

// GetUserTrafficLogs handles GET /api/v2ray/traffic/logs
func (h *V2RayHandler) GetUserTrafficLogs(c *gin.Context) {
	var logs []models.V2RayTrafficLog
	if err := db.DB.Order("timestamp desc").Limit(100).Find(&logs).Error; err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, logs)
}

// ──────────────────────────────────────────────────────────────────────────────
// SERVER ROUTING RULES API
// ──────────────────────────────────────────────────────────────────────────────

// ListRoutingRules handles GET /api/v2ray/routing
func (h *V2RayHandler) ListRoutingRules(c *gin.Context) {
	var rules []models.V2RayRoutingRule
	if err := db.DB.Find(&rules).Error; err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, rules)
}

// CreateRoutingRule handles POST /api/v2ray/routing
func (h *V2RayHandler) CreateRoutingRule(c *gin.Context) {
	var rule models.V2RayRoutingRule
	if err := c.ShouldBindJSON(&rule); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	if err := db.DB.Create(&rule).Error; err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	if core.IsRunning() {
		_ = traffic.ReloadCoreConfig()
	}

	c.JSON(http.StatusCreated, rule)
}

// DeleteRoutingRule handles DELETE /api/v2ray/routing/:id
func (h *V2RayHandler) DeleteRoutingRule(c *gin.Context) {
	id, _ := strconv.Atoi(c.Param("id"))
	if err := db.DB.Delete(&models.V2RayRoutingRule{}, id).Error; err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	if core.IsRunning() {
		_ = traffic.ReloadCoreConfig()
	}

	c.JSON(http.StatusOK, gin.H{"status": "deleted"})
}

// ──────────────────────────────────────────────────────────────────────────────
// CLIENT API CONFIGS
// ──────────────────────────────────────────────────────────────────────────────

// GetClientStatus handles GET /api/v2ray/client/status
func (h *V2RayHandler) GetClientStatus(c *gin.Context) {
	c.JSON(http.StatusOK, gin.H{
		"is_running": core.IsClientRunning(),
	})
}

// StartClientCore handles POST /api/v2ray/client/start
func (h *V2RayHandler) StartClientCore(c *gin.Context) {
	var activeConfig models.V2RayClientConfig
	if err := db.DB.Order("updated_at desc").First(&activeConfig).Error; err != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "No imported client profiles found"})
		return
	}

	// Fetch client settings
	var socksPort, httpPort int
	socksPort = 10808
	httpPort = 10809
	evasion := true

	var socksPortSetting models.V2RayClientSetting
	if err := db.DB.Where("key = ?", "socks_port").First(&socksPortSetting).Error; err == nil {
		socksPort, _ = strconv.Atoi(socksPortSetting.Value)
	}
	var httpPortSetting models.V2RayClientSetting
	if err := db.DB.Where("key = ?", "http_port").First(&httpPortSetting).Error; err == nil {
		httpPort, _ = strconv.Atoi(httpPortSetting.Value)
	}
	var evasionSetting models.V2RayClientSetting
	if err := db.DB.Where("key = ?", "evasion_enabled").First(&evasionSetting).Error; err == nil {
		evasion = evasionSetting.Value == "true"
	}

	// Port availability verification & allocation
	socksPortPublic := core.FindAvailablePort(socksPort)
	socksPortInternal := core.FindAvailablePort(socksPortPublic + 1000)
	httpPortPublic := core.FindAvailablePort(httpPort)
	httpPortInternal := core.FindAvailablePort(httpPortPublic + 1000)

	configBytes, err := compiler.CompileClientConfig(activeConfig, socksPortInternal, httpPortInternal, evasion, "")
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to compile config: " + err.Error()})
		return
	}

	// Write client config
	tempPath := filepath.Join(os.TempDir(), "xray_client.json")
	_ = os.WriteFile(tempPath, configBytes, 0644)

	if err := core.StartClientCore(tempPath); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to start core: " + err.Error()})
		return
	}

	// Start strong SOCKS5+HTTP proxy wrapper with connection limit/timeout
	core.StartLocalProxyEngine(socksPortPublic, socksPortInternal, httpPortPublic, httpPortInternal)

	c.JSON(http.StatusOK, gin.H{
		"status":     "started",
		"socks_port": socksPortPublic,
		"http_port":  httpPortPublic,
	})
}

// StopClientCore handles POST /api/v2ray/client/stop
func (h *V2RayHandler) StopClientCore(c *gin.Context) {
	if err := core.StopClientCore(); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, gin.H{"status": "stopped"})
}

// Helper to rebuild active URI link from config
func BuildProxyLink(cfg models.V2RayClientConfig) string {
	switch cfg.Protocol {
	case "vless":
		link := fmt.Sprintf("vless://%s@%s:%d?", cfg.UUID, cfg.Address, cfg.Port)
		var params []string
		var tlsMap map[string]interface{}
		_ = json.Unmarshal([]byte(cfg.TLSSettings), &tlsMap)
		if tlsMap != nil {
			if security, ok := tlsMap["security"].(string); ok {
				params = append(params, "security="+security)
			}
			if sni, ok := tlsMap["sni"].(string); ok && sni != "" {
				params = append(params, "sni="+sni)
			}
			if pbk, ok := tlsMap["publicKey"].(string); ok && pbk != "" {
				params = append(params, "pbk="+pbk)
			}
			if sid, ok := tlsMap["shortId"].(string); ok && sid != "" {
				params = append(params, "sid="+sid)
			}
			if path, ok := tlsMap["path"].(string); ok && path != "" {
				params = append(params, "path="+path)
			}
		}
		params = append(params, "type="+cfg.Network)
		link += strings.Join(params, "&")
		link += "#" + url.PathEscape(cfg.Name)
		return link

	case "vmess":
		var tlsMap map[string]interface{}
		_ = json.Unmarshal([]byte(cfg.TLSSettings), &tlsMap)
		tlsMode := "none"
		sni := ""
		path := ""
		if tlsMap != nil {
			if security, ok := tlsMap["security"].(string); ok {
				tlsMode = security
			}
			if s, ok := tlsMap["sni"].(string); ok {
				sni = s
			}
			if p, ok := tlsMap["path"].(string); ok {
				path = p
			}
		}
		configMap := map[string]interface{}{
			"v":    "2",
			"ps":   cfg.Name,
			"add":  cfg.Address,
			"port": cfg.Port,
			"id":   cfg.UUID,
			"aid":  0,
			"net":  cfg.Network,
			"type": "none",
			"host": sni,
			"path": path,
			"tls":  tlsMode,
		}
		jsonBytes, _ := json.Marshal(configMap)
		return "vmess://" + base64.StdEncoding.EncodeToString(jsonBytes)

	case "trojan":
		link := fmt.Sprintf("trojan://%s@%s:%d?", cfg.UUID, cfg.Address, cfg.Port)
		var tlsMap map[string]interface{}
		_ = json.Unmarshal([]byte(cfg.TLSSettings), &tlsMap)
		if tlsMap != nil {
			if sni, ok := tlsMap["sni"].(string); ok && sni != "" {
				link += "sni=" + sni
			}
		}
		link += "#" + url.PathEscape(cfg.Name)
		return link

	case "shadowsocks":
		var tlsMap map[string]interface{}
		_ = json.Unmarshal([]byte(cfg.TLSSettings), &tlsMap)
		method := "aes-256-gcm"
		if tlsMap != nil {
			if m, ok := tlsMap["method"].(string); ok && m != "" {
				method = m
			}
		}
		userinfo := method + ":" + cfg.UUID
		b64Userinfo := base64.URLEncoding.EncodeToString([]byte(userinfo))
		return fmt.Sprintf("ss://%s@%s:%d#%s", b64Userinfo, cfg.Address, cfg.Port, url.PathEscape(cfg.Name))
	}
	return ""
}

// ListClientConfigs handles GET /api/v2ray/client/configs
func (h *V2RayHandler) ListClientConfigs(c *gin.Context) {
	var configs []models.V2RayClientConfig
	query := db.DB.Order("priority asc, name asc")

	if subIDStr := c.Query("subscription_id"); subIDStr != "" {
		if subID, err := strconv.Atoi(subIDStr); err == nil {
			query = query.Where("subscription_id = ?", subID)
		}
	}
	if search := c.Query("search"); search != "" {
		query = query.Where("name LIKE ? OR address LIKE ?", "%"+search+"%", "%"+search+"%")
	}

	if err := query.Find(&configs).Error; err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, configs)
}

// CreateClientConfig handles POST /api/v2ray/client/configs
func (h *V2RayHandler) CreateClientConfig(c *gin.Context) {
	var cfg models.V2RayClientConfig
	if err := c.ShouldBindJSON(&cfg); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	if err := db.DB.Create(&cfg).Error; err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusCreated, cfg)
}

// UpdateClientConfig handles PUT /api/v2ray/client/configs/:id
func (h *V2RayHandler) UpdateClientConfig(c *gin.Context) {
	id, _ := strconv.Atoi(c.Param("id"))
	var existing models.V2RayClientConfig
	if err := db.DB.First(&existing, id).Error; err != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "Profile not found"})
		return
	}

	var req models.V2RayClientConfig
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	existing.Name = req.Name
	existing.Protocol = req.Protocol
	existing.Address = req.Address
	existing.Port = req.Port
	existing.UUID = req.UUID
	existing.Network = req.Network
	existing.TLSSettings = req.TLSSettings
	existing.MuxEnabled = req.MuxEnabled
	existing.Priority = req.Priority

	if err := db.DB.Save(&existing).Error; err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, existing)
}

// DeleteClientConfig handles DELETE /api/v2ray/client/configs/:id
func (h *V2RayHandler) DeleteClientConfig(c *gin.Context) {
	id, _ := strconv.Atoi(c.Param("id"))
	if err := db.DB.Delete(&models.V2RayClientConfig{}, id).Error; err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, gin.H{"status": "deleted"})
}

// SetActiveClientConfig handles POST /api/v2ray/client/configs/:id/active
func (h *V2RayHandler) SetActiveClientConfig(c *gin.Context) {
	id, _ := strconv.Atoi(c.Param("id"))
	var target models.V2RayClientConfig
	if err := db.DB.First(&target, id).Error; err != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "Profile not found"})
		return
	}

	tx := db.DB.Begin()
	// Deactivate all
	tx.Model(&models.V2RayClientConfig{}).Where("1 = 1").Update("is_active", false)
	// Activate target
	target.IsActive = true
	tx.Save(&target)
	tx.Commit()

	c.JSON(http.StatusOK, gin.H{"status": "activated", "id": id})
}

// ReorderClientConfigs handles POST /api/v2ray/client/configs/reorder
func (h *V2RayHandler) ReorderClientConfigs(c *gin.Context) {
	var req struct {
		IDs []uint `json:"ids"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	tx := db.DB.Begin()
	for idx, id := range req.IDs {
		tx.Model(&models.V2RayClientConfig{}).Where("id = ?", id).Update("priority", idx)
	}
	tx.Commit()

	c.JSON(http.StatusOK, gin.H{"status": "reordered"})
}

// ImportSubscription handles POST /api/v2ray/client/import
func (h *V2RayHandler) ImportSubscription(c *gin.Context) {
	var req struct {
		Name string `json:"name"`
		URL  string `json:"url"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	configs, err := sub.FetchAndImportSubscription(req.URL)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to fetch subscription: " + err.Error()})
		return
	}

	// Create/Update V2RayClientSubscription record
	var subRecord models.V2RayClientSubscription
	if err := db.DB.Where("url = ?", req.URL).First(&subRecord).Error; err != nil {
		subName := req.Name
		if subName == "" {
			subName = "Sub " + req.URL
		}
		subRecord = models.V2RayClientSubscription{
			Name:           subName,
			URL:            req.URL,
			UpdateInterval: 12,
			LastUpdatedAt:  time.Now(),
		}
		db.DB.Create(&subRecord)
	} else {
		if req.Name != "" {
			subRecord.Name = req.Name
		}
		subRecord.LastUpdatedAt = time.Now()
		db.DB.Save(&subRecord)
	}

	// Save all configs associated with this subscription ID
	tx := db.DB.Begin()
	for _, cfg := range configs {
		cfg.SubscriptionID = subRecord.ID
		var existing models.V2RayClientConfig
		if err := tx.Where("uuid = ? AND address = ? AND port = ?", cfg.UUID, cfg.Address, cfg.Port).First(&existing).Error; err != nil {
			tx.Create(&cfg)
		} else {
			existing.Name = cfg.Name
			existing.TLSSettings = cfg.TLSSettings
			existing.SubscriptionID = subRecord.ID
			tx.Save(&existing)
		}
	}
	tx.Commit()

	c.JSON(http.StatusOK, gin.H{"status": "imported", "count": len(configs), "subscription_id": subRecord.ID})
}

// ImportManualConfig handles POST /api/v2ray/client/configs/import-manual
func (h *V2RayHandler) ImportManualConfig(c *gin.Context) {
	var req struct {
		Content string `json:"content"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	raw := strings.TrimSpace(req.Content)
	if raw == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Content cannot be empty"})
		return
	}

	// Check if JSON block
	if strings.HasPrefix(raw, "{") && strings.HasSuffix(raw, "}") {
		cfg, err := parseJSONOutbound(raw)
		if err != nil {
			c.JSON(http.StatusBadRequest, gin.H{"error": "Invalid JSON outbound block: " + err.Error()})
			return
		}
		db.DB.Create(&cfg)
		c.JSON(http.StatusOK, gin.H{"status": "imported", "config": cfg})
		return
	}

	// Check if Base64 multi-line block
	if decodedBytes, err := base64.StdEncoding.DecodeString(raw); err == nil {
		decodedStr := string(decodedBytes)
		lines := strings.Split(decodedStr, "\n")
		importedCount := 0
		var lastImported models.V2RayClientConfig
		for _, line := range lines {
			line = strings.TrimSpace(line)
			if line == "" {
				continue
			}
			cfg, err := sub.ParseProxyLink(line)
			if err == nil {
				db.DB.Create(&cfg)
				lastImported = cfg
				importedCount++
			}
		}
		if importedCount == 0 {
			c.JSON(http.StatusBadRequest, gin.H{"error": "No valid proxy links found in base64 payload"})
			return
		}
		c.JSON(http.StatusOK, gin.H{"status": "imported", "count": importedCount, "last": lastImported})
		return
	}

	// Otherwise assume single URI link
	cfg, err := sub.ParseProxyLink(raw)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Invalid proxy link URI: " + err.Error()})
		return
	}

	db.DB.Create(&cfg)
	c.JSON(http.StatusOK, gin.H{"status": "imported", "config": cfg})
}

// ImportQRConfig handles POST /api/v2ray/client/configs/import-qr
func (h *V2RayHandler) ImportQRConfig(c *gin.Context) {
	file, err := c.FormFile("file")
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "No file uploaded"})
		return
	}

	src, err := file.Open()
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to open file"})
		return
	}
	defer src.Close()

	// Decode QR image using tuotoo/qrcode
	qrmatrix, err := qrcode.Decode(src)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Failed to decode QR code: " + err.Error()})
		return
	}

	link := strings.TrimSpace(qrmatrix.Content)
	cfg, err := sub.ParseProxyLink(link)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "QR code does not contain a valid proxy URI: " + err.Error()})
		return
	}

	db.DB.Create(&cfg)
	c.JSON(http.StatusOK, gin.H{"status": "imported", "config": cfg})
}

// ImportBulkConfigs handles POST /api/v2ray/client/configs/import-bulk
func (h *V2RayHandler) ImportBulkConfigs(c *gin.Context) {
	var req struct {
		Uris []string `json:"uris"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	var configsToInsert []models.V2RayClientConfig
	for _, uri := range req.Uris {
		uri = strings.TrimSpace(uri)
		if uri == "" {
			continue
		}
		cfg, err := sub.ParseProxyLink(uri)
		if err == nil {
			configsToInsert = append(configsToInsert, cfg)
		}
	}

	importedCount := 0
	if len(configsToInsert) > 0 {
		tx := db.DB.Begin()
		if err := tx.CreateInBatches(&configsToInsert, 500).Error; err != nil {
			tx.Rollback()
			c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
			return
		}
		tx.Commit()
		importedCount = len(configsToInsert)
	}

	c.JSON(http.StatusOK, gin.H{"status": "imported", "count": importedCount})
}

// ListSubscriptions handles GET /api/v2ray/client/subscriptions
func (h *V2RayHandler) ListSubscriptions(c *gin.Context) {
	var subs []models.V2RayClientSubscription
	if err := db.DB.Find(&subs).Error; err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, subs)
}

// DeleteSubscription handles DELETE /api/v2ray/client/subscriptions/:id
func (h *V2RayHandler) DeleteSubscription(c *gin.Context) {
	id, _ := strconv.Atoi(c.Param("id"))
	tx := db.DB.Begin()
	// Delete associated configs first
	tx.Where("subscription_id = ?", id).Delete(&models.V2RayClientConfig{})
	// Delete subscription record
	tx.Delete(&models.V2RayClientSubscription{}, id)
	tx.Commit()

	c.JSON(http.StatusOK, gin.H{"status": "deleted"})
}

// ExportSelectedConfigsPDF handles POST /api/v2ray/client/export-pdf
func (h *V2RayHandler) ExportSelectedConfigsPDF(c *gin.Context) {
	var req struct {
		IDs []uint `json:"ids"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	var configs []models.V2RayClientConfig
	if err := db.DB.Where("id IN ?", req.IDs).Find(&configs).Error; err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	if len(configs) == 0 {
		c.JSON(http.StatusBadRequest, gin.H{"error": "No configs selected for export"})
		return
	}

	// Generate PDF using gofpdf
	pdf := gofpdf.New("P", "mm", "A4", "")
	pdf.SetMargins(15, 15, 15)
	pdf.AddPage()

	// Header Styling (Premium dark-navy and gray)
	pdf.SetFillColor(26, 36, 43)
	pdf.Rect(0, 0, 210, 45, "F")

	pdf.SetTextColor(255, 255, 255)
	pdf.SetFont("Arial", "B", 18)
	pdf.Text(15, 20, "Clever Connect VPN Orchestrator")
	
	pdf.SetFont("Arial", "", 12)
	pdf.SetTextColor(200, 200, 200)
	pdf.Text(15, 28, "Exported Client Connection Profiles")
	pdf.Text(15, 34, "Scan QR codes to import profiles into mobile/desktop clients.")

	pdf.SetY(55)

	for idx, cfg := range configs {
		uri := BuildProxyLink(cfg)
		if uri == "" {
			continue
		}

		// Keep spacing clean
		yStart := pdf.GetY()
		if yStart > 240 {
			pdf.AddPage()
			pdf.SetY(20)
			yStart = pdf.GetY()
		}

		// Draw card boundary border
		pdf.SetDrawColor(220, 220, 220)
		pdf.SetFillColor(250, 250, 250)
		pdf.Rect(15, yStart, 180, 52, "FD")

		// Badges / Protocol Styling
		pdf.SetTextColor(30, 30, 30)
		pdf.SetFont("Arial", "B", 12)
		pdf.Text(20, yStart+10, cfg.Name)

		pdf.SetFont("Arial", "", 9)
		pdf.SetTextColor(100, 100, 100)
		pdf.Text(20, yStart+17, fmt.Sprintf("Protocol: %s | Server: %s:%d | Network: %s", strings.ToUpper(cfg.Protocol), cfg.Address, cfg.Port, cfg.Network))

		// Render QR code from uri
		code, err := qr.Encode(uri, qr.L)
		if err == nil {
			pngBytes := code.PNG()
			reader := bytes.NewReader(pngBytes)
			qrName := fmt.Sprintf("qr_exp_%d_%d", cfg.ID, idx)
			pdf.RegisterImageOptionsReader(qrName, gofpdf.ImageOptions{ImageType: "PNG"}, reader)
			pdf.ImageOptions(qrName, 155, yStart+3, 36, 36, false, gofpdf.ImageOptions{ImageType: "PNG"}, 0, "")
		}

		// Draw URI textbox
		pdf.SetDrawColor(240, 240, 240)
		pdf.SetFillColor(245, 245, 245)
		pdf.Rect(20, yStart+23, 130, 22, "FD")

		pdf.SetFont("Courier", "", 8)
		pdf.SetTextColor(50, 50, 50)
		
		// Wrap text inside box
		lines := pdf.SplitText(uri, 126)
		yText := yStart + 27
		for i, line := range lines {
			if i < 3 { // fit up to 3 lines
				pdf.Text(22, yText, line)
				yText += 4
			}
		}

		pdf.SetY(yStart + 58)
	}

	var buf bytes.Buffer
	if err := pdf.Output(&buf); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to generate PDF: " + err.Error()})
		return
	}

	c.Header("Content-Disposition", "attachment; filename=clever_configs_export.pdf")
	c.Data(http.StatusOK, "application/pdf", buf.Bytes())
}

// Helper to parse JSON outbound blocks
func parseJSONOutbound(raw string) (models.V2RayClientConfig, error) {
	var cfg models.V2RayClientConfig
	var outbound struct {
		Protocol string          `json:"protocol"`
		Settings json.RawMessage `json:"settings"`
		StreamSettings struct {
			Network        string `json:"network"`
			Security       string `json:"security"`
			TLSSettings struct {
				ServerName string `json:"serverName"`
			} `json:"tlsSettings"`
			RealitySettings struct {
				PublicKey  string `json:"publicKey"`
				ShortID    string `json:"shortId"`
				ServerName string `json:"serverName"`
			} `json:"realitySettings"`
			WSConfig struct {
				Path string `json:"path"`
			} `json:"wsSettings"`
			GRPCConfig struct {
				ServiceName string `json:"serviceName"`
			} `json:"grpcSettings"`
		} `json:"streamSettings"`
	}

	if err := json.Unmarshal([]byte(raw), &outbound); err != nil {
		return cfg, err
	}

	cfg.Protocol = outbound.Protocol
	cfg.Network = outbound.StreamSettings.Network
	if cfg.Network == "" {
		cfg.Network = "tcp"
	}

	// Extract based on protocol
	switch outbound.Protocol {
	case "vless", "vmess":
		var settings struct {
			Vnext []struct {
				Address string `json:"address"`
				Port    int    `json:"port"`
				Users   []struct {
					ID string `json:"id"`
				} `json:"users"`
			} `json:"vnext"`
		}
		_ = json.Unmarshal(outbound.Settings, &settings)
		if len(settings.Vnext) > 0 {
			cfg.Address = settings.Vnext[0].Address
			cfg.Port = settings.Vnext[0].Port
			if len(settings.Vnext[0].Users) > 0 {
				cfg.UUID = settings.Vnext[0].Users[0].ID
			}
		}

	case "trojan":
		var settings struct {
			Servers []struct {
				Address  string `json:"address"`
				Port     int    `json:"port"`
				Password string `json:"password"`
			} `json:"servers"`
		}
		_ = json.Unmarshal(outbound.Settings, &settings)
		if len(settings.Servers) > 0 {
			cfg.Address = settings.Servers[0].Address
			cfg.Port = settings.Servers[0].Port
			cfg.UUID = settings.Servers[0].Password
		}

	case "shadowsocks":
		var settings struct {
			Servers []struct {
				Address  string `json:"address"`
				Port     int    `json:"port"`
				Password string `json:"password"`
				Method   string `json:"method"`
			} `json:"servers"`
		}
		_ = json.Unmarshal(outbound.Settings, &settings)
		if len(settings.Servers) > 0 {
			cfg.Address = settings.Servers[0].Address
			cfg.Port = settings.Servers[0].Port
			cfg.UUID = settings.Servers[0].Password
		}
	}

	if cfg.Address == "" {
		return cfg, fmt.Errorf("could not extract server address from JSON block")
	}

	tlsMap := make(map[string]interface{})
	tlsMap["security"] = outbound.StreamSettings.Security
	if outbound.StreamSettings.Security == "tls" {
		tlsMap["sni"] = outbound.StreamSettings.TLSSettings.ServerName
	} else if outbound.StreamSettings.Security == "reality" {
		tlsMap["publicKey"] = outbound.StreamSettings.RealitySettings.PublicKey
		tlsMap["shortId"] = outbound.StreamSettings.RealitySettings.ShortID
		tlsMap["sni"] = outbound.StreamSettings.RealitySettings.ServerName
	}

	if outbound.StreamSettings.Network == "ws" {
		tlsMap["path"] = outbound.StreamSettings.WSConfig.Path
	} else if outbound.StreamSettings.Network == "grpc" {
		tlsMap["path"] = outbound.StreamSettings.GRPCConfig.ServiceName
	}

	tlsBytes, _ := json.Marshal(tlsMap)
	cfg.TLSSettings = string(tlsBytes)
	cfg.Name = fmt.Sprintf("Imported_%s_%s", cfg.Protocol, cfg.Address)

	return cfg, nil
}


// GetClientSettings handles GET /api/v2ray/client/settings
func (h *V2RayHandler) GetClientSettings(c *gin.Context) {
	var settings []models.V2RayClientSetting
	if err := db.DB.Find(&settings).Error; err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	result := make(map[string]string)
	for _, s := range settings {
		result[s.Key] = s.Value
	}

	// Ensure defaults
	if _, ok := result["socks_port"]; !ok {
		result["socks_port"] = "10808"
	}
	if _, ok := result["http_port"]; !ok {
		result["http_port"] = "10809"
	}
	if _, ok := result["evasion_enabled"]; !ok {
		result["evasion_enabled"] = "true"
	}

	c.JSON(http.StatusOK, result)
}

// SaveClientSettings handles POST /api/v2ray/client/settings
func (h *V2RayHandler) SaveClientSettings(c *gin.Context) {
	var req map[string]string
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	tx := db.DB.Begin()
	for k, v := range req {
		var setting models.V2RayClientSetting
		if err := tx.Where("key = ?", k).First(&setting).Error; err == nil {
			setting.Value = v
			tx.Save(&setting)
		} else {
			tx.Create(&models.V2RayClientSetting{Key: k, Value: v})
		}
	}
	tx.Commit()

	c.JSON(http.StatusOK, gin.H{"status": "saved"})
}

// ──────────────────────────────────────────────────────────────────────────────
// PUBLIC SUBSCRIPTION ENDPOINT (SERVER PANEL)
// ──────────────────────────────────────────────────────────────────────────────

// ServeSubscription handles GET /sub/:token
func (h *V2RayHandler) ServeSubscription(c *gin.Context) {
	token := c.Param("token")
	if token == "" {
		c.String(http.StatusBadRequest, "Token required")
		return
	}

	// Determine host header or request IP
	host := c.Request.Host

	base64Content, err := sub.GenerateSubscription(token, host)
	if err != nil {
		c.String(http.StatusForbidden, err.Error())
		return
	}

	c.Header("Content-Type", "text/plain; charset=utf-8")
	c.Header("Cache-Control", "no-store, no-cache, must-revalidate")
	c.String(http.StatusOK, base64Content)
}

// ──────────────────────────────────────────────────────────────────────────────
// CLIENT DIAGNOSTICS & SCANNING API
// ──────────────────────────────────────────────────────────────────────────────

// TestClientProfile handles POST /api/v2ray/client/test-profile/:id
func (h *V2RayHandler) TestClientProfile(c *gin.Context) {
	id, _ := strconv.Atoi(c.Param("id"))
	var cfg models.V2RayClientConfig
	if err := db.DB.First(&cfg, id).Error; err != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "Profile not found"})
		return
	}

	var req struct {
		MeasureSpeed bool `json:"measure_speed"`
		TimeoutSec   int  `json:"timeout_sec"`
	}
	_ = c.ShouldBindJSON(&req)
	if req.TimeoutSec <= 0 {
		req.TimeoutSec = 8
	}

	res := speed.TestProfile(cfg, 22000, 22001, req.MeasureSpeed, req.TimeoutSec)
	
	// Store latency to local DB
	if res.OK {
		cfg.LatencyMs = res.RelayMs
	} else {
		cfg.LatencyMs = -1
	}
	db.DB.Save(&cfg)

	c.JSON(http.StatusOK, res)
}

// TestMassProfiles handles POST /api/v2ray/client/test-mass
func (h *V2RayHandler) TestMassProfiles(c *gin.Context) {
	var req struct {
		IDs          []uint `json:"ids"`
		MeasureSpeed bool   `json:"measure_speed"`
		Concurrency  int    `json:"concurrency"`
		TimeoutSec   int    `json:"timeout_sec"`
	}
	_ = c.ShouldBindJSON(&req)

	var configs []models.V2RayClientConfig
	var err error
	if len(req.IDs) > 0 {
		err = db.DB.Where("id IN ?", req.IDs).Find(&configs).Error
	} else {
		err = db.DB.Find(&configs).Error
	}

	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	results := speed.MassTestProfiles(configs, req.Concurrency, req.MeasureSpeed, req.TimeoutSec)
	
	// Store latency results to local DB
	tx := db.DB.Begin()
	for _, res := range results {
		latency := -1
		if res.OK {
			latency = res.RelayMs
		}
		tx.Model(&models.V2RayClientConfig{}).Where("id = ?", res.ConfigID).Update("latency_ms", latency)
	}
	tx.Commit()

	c.JSON(http.StatusOK, results)
}

// ScanCDN handles POST /api/v2ray/client/scan-cdn
func (h *V2RayHandler) ScanCDN(c *gin.Context) {
	var opts scanner.CDNConfigsOptions
	if err := c.ShouldBindJSON(&opts); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	state, err := scanner.StartScan(opts)
	if err != nil {
		c.JSON(http.StatusConflict, gin.H{"error": err.Error()})
		return
	}

	c.JSON(http.StatusOK, state.Snapshot())
}

// GetScanStatus handles GET /api/v2ray/client/scan-cdn/status
func (h *V2RayHandler) GetScanStatus(c *gin.Context) {
	state := scanner.GetActiveScan()
	if state == nil {
		c.JSON(http.StatusOK, gin.H{"status": "idle"})
		return
	}
	c.JSON(http.StatusOK, state.Snapshot())
}

// StopScan handles POST /api/v2ray/client/scan-cdn/stop
func (h *V2RayHandler) StopScan(c *gin.Context) {
	scanner.CancelActiveScan()
	c.JSON(http.StatusOK, gin.H{"status": "stopped"})
}

// RunDetailedSpeedTest handles POST /api/v2ray/client/speed-test
func (h *V2RayHandler) RunDetailedSpeedTest(c *gin.Context) {
	if !core.IsClientRunning() {
		c.JSON(http.StatusBadRequest, gin.H{"error": "V2Ray client proxy is not running. Please connect first."})
		return
	}

	var req struct {
		SizeBytes int `json:"size_bytes"`
	}
	_ = c.ShouldBindJSON(&req)

	socksPort := 10808
	var setting models.V2RayClientSetting
	if err := db.DB.Where("key = ?", "socks_port").First(&setting).Error; err == nil && setting.Value != "" {
		if val, err := strconv.Atoi(setting.Value); err == nil {
			socksPort = val
		}
	}

	res, err := speed.RunSpeedTestWithBreakdown(socksPort, req.SizeBytes, 20*time.Second)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	c.JSON(http.StatusOK, res)
}

// GetClientLogs handles GET /api/v2ray/client/logs
func (h *V2RayHandler) GetClientLogs(c *gin.Context) {
	q := c.Query("q")
	logs := core.GetClientLogs(q)
	c.JSON(http.StatusOK, logs)
}

// ProbePorts handles POST /api/v2ray/client/probe-ports
func (h *V2RayHandler) ProbePorts(c *gin.Context) {
	var req struct {
		IP       string `json:"ip"`
		Ports    []int  `json:"ports"`
		Protocol string `json:"protocol"` // "tcp" or "udp"
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	if req.IP == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "IP target is required"})
		return
	}
	if len(req.Ports) == 0 {
		c.JSON(http.StatusBadRequest, gin.H{"error": "At least one port is required"})
		return
	}
	if req.Protocol == "" {
		req.Protocol = "tcp"
	}

	results := scanner.ProbePorts(req.IP, req.Ports, req.Protocol, 4*time.Second)
	c.JSON(http.StatusOK, results)
}

// WakeOnLAN handles POST /api/v2ray/client/wol
func (h *V2RayHandler) WakeOnLAN(c *gin.Context) {
	var req struct {
		MAC         string `json:"mac"`
		BroadcastIP string `json:"broadcast_ip"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	if req.MAC == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "MAC address is required"})
		return
	}

	err := scanner.SendWakeOnLAN(req.MAC, req.BroadcastIP)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, gin.H{"status": "magic packet sent"})
}

// DiscoverDevices handles GET /api/v2ray/client/discover
func (h *V2RayHandler) DiscoverDevices(c *gin.Context) {
	devices, err := scanner.DiscoverDevices(3 * time.Second)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, devices)
}

// StartDebugProxy handles POST /api/v2ray/client/debug-proxy/start
func (h *V2RayHandler) StartDebugProxy(c *gin.Context) {
	var req struct {
		Port int `json:"port"`
	}
	_ = c.ShouldBindJSON(&req)
	if req.Port <= 0 {
		req.Port = 8080
	}

	err := scanner.StartDebugProxy(req.Port)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, gin.H{"status": "running", "port": req.Port})
}

// StopDebugProxy handles POST /api/v2ray/client/debug-proxy/stop
func (h *V2RayHandler) StopDebugProxy(c *gin.Context) {
	err := scanner.StopDebugProxy()
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, gin.H{"status": "stopped"})
}

// GetDebugProxyLogs handles GET /api/v2ray/client/debug-proxy/logs
func (h *V2RayHandler) GetDebugProxyLogs(c *gin.Context) {
	logs := scanner.GetProxyLogs()
	c.JSON(http.StatusOK, logs)
}

// GetHotkeys handles GET /api/v2ray/client/hotkeys
func (h *V2RayHandler) GetHotkeys(c *gin.Context) {
	var setting models.V2RayClientSetting
	if err := db.DB.Where("key = ?", "keyboard_shortcuts").First(&setting).Error; err != nil {
		c.JSON(http.StatusOK, gin.H{"shortcuts": "[]"})
		return
	}
	c.JSON(http.StatusOK, gin.H{"shortcuts": setting.Value})
}

// SaveHotkeys handles POST /api/v2ray/client/hotkeys
func (h *V2RayHandler) SaveHotkeys(c *gin.Context) {
	var req struct {
		Shortcuts string `json:"shortcuts"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	var setting models.V2RayClientSetting
	db.DB.Where("key = ?", "keyboard_shortcuts").FirstOrCreate(&setting, models.V2RayClientSetting{Key: "keyboard_shortcuts"})
	setting.Value = req.Shortcuts
	db.DB.Save(&setting)

	c.JSON(http.StatusOK, gin.H{"status": "saved"})
}

// GetSystemTrayConfig handles GET /api/v2ray/client/system-tray
func (h *V2RayHandler) GetSystemTrayConfig(c *gin.Context) {
	var setting models.V2RayClientSetting
	if err := db.DB.Where("key = ?", "system_tray_config").First(&setting).Error; err != nil {
		c.JSON(http.StatusOK, gin.H{"config": "{}"})
		return
	}
	c.JSON(http.StatusOK, gin.H{"config": setting.Value})
}

// SaveSystemTrayConfig handles POST /api/v2ray/client/system-tray
func (h *V2RayHandler) SaveSystemTrayConfig(c *gin.Context) {
	var req struct {
		Config string `json:"config"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	var setting models.V2RayClientSetting
	db.DB.Where("key = ?", "system_tray_config").FirstOrCreate(&setting, models.V2RayClientSetting{Key: "system_tray_config"})
	setting.Value = req.Config
	db.DB.Save(&setting)

	c.JSON(http.StatusOK, gin.H{"status": "saved"})
}

// ProvisionNode handles POST /api/v2ray/nodes/:id/provision
func (h *V2RayHandler) ProvisionNode(c *gin.Context) {
	id := c.Param("id")
	var req struct {
		Password string `json:"password"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	var node models.V2RayNode
	if err := db.DB.First(&node, id).Error; err != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "node not found"})
		return
	}

	node.Status = "provisioning"
	db.DB.Save(&node)

	// Run async to avoid blocking HTTP response
	go func() {
		err := core.ProvisionNode(&node, req.Password)
		if err != nil {
			node.Status = "offline"
			db.DB.Save(&node)
			logger.Error("Provisioner", "Failed to provision remote node", "id", node.ID, "error", err)
			return
		}
		node.Status = "online"
		db.DB.Save(&node)
	}()

	c.JSON(http.StatusOK, gin.H{"status": "provisioning started"})
}

// BlockFirewallIP handles POST /api/v2ray/firewall/block
func (h *V2RayHandler) BlockFirewallIP(c *gin.Context) {
	var req struct {
		IP string `json:"ip"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	err := core.BlockMaliciousIP(req.IP)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, gin.H{"status": "blocked", "ip": req.IP})
}

// HandleMCP handles POST /api/v2ray/mcp
func (h *V2RayHandler) HandleMCP(c *gin.Context) {
	bodyBytes, err := io.ReadAll(c.Request.Body)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	respBytes, err := core.HandleMCPRequest(bodyBytes)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	c.Data(http.StatusOK, "application/json", respBytes)
}

// ServeWebDAV handles WebDAV routing for logs
func (h *V2RayHandler) ServeWebDAV(c *gin.Context) {
	handler := &core.WebDAVHandler{LogDir: "logs"}
	handler.ServeHTTP(c.Writer, c.Request)
}
