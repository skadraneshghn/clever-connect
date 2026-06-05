package handlers

import (
	"context"
	"net/http"
	"strconv"
	"time"

	"clever-connect/internal/config"
	"clever-connect/internal/db"
	"clever-connect/internal/logger"
	"clever-connect/internal/models"
	"clever-connect/internal/soroush"
	"clever-connect/internal/soroushlib"

	"github.com/gin-gonic/gin"
)

// SoroushHandler handles all /api/soroush/* routes.
// Completely independent from EhcoHandler — no shared state.
type SoroushHandler struct {
	cfg *config.Config
}

// NewSoroushHandler creates a new SoroushHandler.
func NewSoroushHandler(cfg *config.Config) *SoroushHandler {
	return &SoroushHandler{cfg: cfg}
}

// ════════════════════════════════════════════════════════════════════════════════
// Account Management
// ════════════════════════════════════════════════════════════════════════════════

// GetSoroushAccounts handles GET /api/soroush/accounts
// Returns all SoroushAccount records from DB.
func (h *SoroushHandler) GetSoroushAccounts(c *gin.Context) {
	var accounts []models.SoroushAccount
	if err := db.DB.Find(&accounts).Error; err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to fetch accounts"})
		return
	}
	c.JSON(http.StatusOK, gin.H{"accounts": accounts})
}

// AddSoroushAccount handles POST /api/soroush/accounts
// Creates a new SoroushAccount (auth_key will be populated during OTP flow).
func (h *SoroushHandler) AddSoroushAccount(c *gin.Context) {
	var req struct {
		PhoneNumber  string `json:"phone_number" binding:"required"`
		Name         string `json:"name"`
		Role         string `json:"role"`
		IsServerNode bool   `json:"is_server_node"`
	}

	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Invalid request: " + err.Error()})
		return
	}

	if req.Role == "" {
		req.Role = "worker"
	}

	account := models.SoroushAccount{
		PhoneNumber:  req.PhoneNumber,
		Name:         req.Name,
		Role:         req.Role,
		IsServerNode: req.IsServerNode,
		Status:       "idle",
	}

	if err := db.DB.Create(&account).Error; err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to create account: " + err.Error()})
		return
	}

	logger.Info("Soroush", "Account added", "phone", maskPhoneForLog(req.PhoneNumber))
	c.JSON(http.StatusCreated, gin.H{"account": account})
}

// DeleteSoroushAccount handles DELETE /api/soroush/accounts/:id
func (h *SoroushHandler) DeleteSoroushAccount(c *gin.Context) {
	id, err := strconv.ParseUint(c.Param("id"), 10, 64)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Invalid account ID"})
		return
	}

	if err := db.DB.Delete(&models.SoroushAccount{}, id).Error; err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to delete account"})
		return
	}

	c.JSON(http.StatusOK, gin.H{"status": "deleted", "id": id})
}

// SendVerificationCode handles POST /api/soroush/accounts/:id/send-code
// Triggers MTProto auth.sendCode() for the account.
func (h *SoroushHandler) SendVerificationCode(c *gin.Context) {
	id, err := strconv.ParseUint(c.Param("id"), 10, 64)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Invalid account ID"})
		return
	}

	var account models.SoroushAccount
	if err := db.DB.First(&account, id).Error; err != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "Account not found"})
		return
	}

	// Create a fresh MTProto session for auth
	transport := soroushlib.NewTransport()
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	if err := transport.Connect(ctx); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to connect to Soroush: " + err.Error()})
		return
	}
	defer transport.Disconnect()

	session := soroushlib.NewSession(transport)

	// Perform DH key exchange to get auth key
	if err := session.CreateAuthKey(ctx); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "DH key exchange failed: " + err.Error()})
		return
	}

	// Send verification code
	body := soroushlib.BuildSendCodeRequest(account.PhoneNumber, soroushlib.SoroushAppID, soroushlib.SoroushAppHash)
	wrapped := soroushlib.WrapInitConnection(soroushlib.SoroushAppID, body)

	cid, reader, err := session.SendAndWait(ctx, wrapped, true)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Send code failed: " + err.Error()})
		return
	}

	phoneCodeHash, timeout, err := soroushlib.ParseSentCodeResponse(cid, reader)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Parse response failed: " + err.Error()})
		return
	}

	// Save the auth key material for later use during verification
	account.AuthKey = session.AuthKey
	account.AuthKeyID = soroushlib.Int64ToBytes(session.AuthKeyID)
	account.ServerSalt = soroushlib.Int64ToBytes(session.ServerSalt)
	account.Status = "pending_verification"
	db.DB.Save(&account)

	logger.Info("Soroush", "Verification code sent",
		"phone", maskPhoneForLog(account.PhoneNumber),
		"timeout", timeout,
	)

	c.JSON(http.StatusOK, gin.H{
		"status":          "code_sent",
		"phone_code_hash": phoneCodeHash,
		"timeout":         timeout,
	})
}

// VerifyAccount handles POST /api/soroush/accounts/:id/verify
// Completes auth.signIn(), saves auth key material to DB.
func (h *SoroushHandler) VerifyAccount(c *gin.Context) {
	id, err := strconv.ParseUint(c.Param("id"), 10, 64)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Invalid account ID"})
		return
	}

	var req struct {
		Code          string `json:"code" binding:"required"`
		PhoneCodeHash []byte `json:"phone_code_hash" binding:"required"`
	}

	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Invalid request: " + err.Error()})
		return
	}

	var account models.SoroushAccount
	if err := db.DB.First(&account, id).Error; err != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "Account not found"})
		return
	}

	if len(account.AuthKey) == 0 {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Account has no auth key — send verification code first"})
		return
	}

	// Restore session from saved auth credentials
	session, transport := soroushlib.RestoreSession(account.AuthKey, account.AuthKeyID, account.ServerSalt)

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	if err := transport.Connect(ctx); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to connect: " + err.Error()})
		return
	}
	defer transport.Disconnect()

	// Sign in
	body := soroushlib.BuildSignInRequest(account.PhoneNumber, req.PhoneCodeHash, req.Code)
	wrapped := soroushlib.WrapInitConnection(soroushlib.SoroushAppID, body)

	cid, reader, err := session.SendAndWait(ctx, wrapped, true)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Sign in failed: " + err.Error()})
		return
	}

	userID, firstName, lastName, accessHash, err := soroushlib.ParseAuthorizationResponse(cid, reader)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Parse auth response failed: " + err.Error()})
		return
	}

	// Update account with verified credentials
	account.SoroushUserID = userID
	account.AccessHash = accessHash
	account.DisplayName = firstName + " " + lastName
	account.AuthKey = session.AuthKey
	account.AuthKeyID = soroushlib.Int64ToBytes(session.AuthKeyID)
	account.ServerSalt = soroushlib.Int64ToBytes(session.ServerSalt)
	account.Status = "verified"
	account.LastActive = time.Now().Format(time.RFC3339)
	db.DB.Save(&account)

	logger.Info("Soroush", "Account verified",
		"phone", maskPhoneForLog(account.PhoneNumber),
		"user_id", userID,
		"name", account.DisplayName,
	)

	c.JSON(http.StatusOK, gin.H{
		"status":   "verified",
		"user_id":  userID,
		"name":     account.DisplayName,
		"account":  account,
	})
}

// ════════════════════════════════════════════════════════════════════════════════
// Tunnel Configuration
// ════════════════════════════════════════════════════════════════════════════════

// GetSoroushConfig handles GET /api/soroush/config
// Returns the SoroushTunnelConfig singleton.
func (h *SoroushHandler) GetSoroushConfig(c *gin.Context) {
	var cfg models.SoroushTunnelConfig
	if err := db.DB.First(&cfg).Error; err != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "Soroush tunnel config not found"})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"config":     cfg,
		"is_running": soroush.IsRunning(),
	})
}

// UpdateSoroushConfig handles PUT /api/soroush/config
func (h *SoroushHandler) UpdateSoroushConfig(c *gin.Context) {
	var req struct {
		GroupChatID        int64  `json:"group_chat_id"`
		GroupAccessHash    int64  `json:"group_access_hash"`
		PSK                string `json:"psk"`
		LiveKitURL         string `json:"livekit_url"`
		SocksPort          int    `json:"socks_port"`
		MaxWorkers         int    `json:"max_workers"`
		LoadBalanceAlgo    string `json:"load_balance_algo"`
		TokenRefreshMinSec int    `json:"token_refresh_min_sec"`
		TokenRefreshMaxSec int    `json:"token_refresh_max_sec"`
	}

	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Invalid request: " + err.Error()})
		return
	}

	var cfg models.SoroushTunnelConfig
	if err := db.DB.First(&cfg).Error; err != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "Soroush tunnel config not found"})
		return
	}

	// Update only non-zero fields
	if req.GroupChatID != 0 {
		cfg.GroupChatID = req.GroupChatID
	}
	if req.GroupAccessHash != 0 {
		cfg.GroupAccessHash = req.GroupAccessHash
	}
	if req.PSK != "" {
		cfg.PSK = req.PSK
	}
	if req.LiveKitURL != "" {
		cfg.LiveKitURL = req.LiveKitURL
	}
	if req.SocksPort != 0 {
		cfg.SocksPort = req.SocksPort
	}
	if req.MaxWorkers != 0 {
		cfg.MaxWorkers = req.MaxWorkers
	}
	if req.LoadBalanceAlgo != "" {
		cfg.LoadBalanceAlgo = req.LoadBalanceAlgo
	}
	if req.TokenRefreshMinSec != 0 {
		cfg.TokenRefreshMinSec = req.TokenRefreshMinSec
	}
	if req.TokenRefreshMaxSec != 0 {
		cfg.TokenRefreshMaxSec = req.TokenRefreshMaxSec
	}

	db.DB.Save(&cfg)

	logger.Info("Soroush", "Tunnel configuration updated")
	c.JSON(http.StatusOK, gin.H{"status": "saved", "config": cfg})
}

// ════════════════════════════════════════════════════════════════════════════════
// Engine Control
// ════════════════════════════════════════════════════════════════════════════════

// StartSoroushEngine handles POST /api/soroush/engine/start
func (h *SoroushHandler) StartSoroushEngine(c *gin.Context) {
	var cfg models.SoroushTunnelConfig
	if err := db.DB.First(&cfg).Error; err != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "Soroush tunnel config not found"})
		return
	}

	var accounts []models.SoroushAccount
	db.DB.Where("status = ?", "verified").Find(&accounts)
	if len(accounts) == 0 {
		c.JSON(http.StatusBadRequest, gin.H{"error": "No verified Soroush accounts available"})
		return
	}

	isServer := h.cfg.AppMode == "server"
	if err := soroush.StartEngine(&cfg, accounts, isServer); err != nil {
		logger.Error("Soroush", "Failed to start engine", "error", err)
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	cfg.IsActive = true
	db.DB.Save(&cfg)

	c.JSON(http.StatusOK, gin.H{
		"status":     "started",
		"is_running": true,
		"workers":    len(accounts),
	})
}

// StopSoroushEngine handles POST /api/soroush/engine/stop
func (h *SoroushHandler) StopSoroushEngine(c *gin.Context) {
	soroush.StopEngine()

	var cfg models.SoroushTunnelConfig
	if err := db.DB.First(&cfg).Error; err == nil {
		cfg.IsActive = false
		db.DB.Save(&cfg)
	}

	c.JSON(http.StatusOK, gin.H{"status": "stopped", "is_running": false})
}

// GetSoroushEngineStatus handles GET /api/soroush/engine/status
func (h *SoroushHandler) GetSoroushEngineStatus(c *gin.Context) {
	status := soroush.GetStatus()
	c.JSON(http.StatusOK, gin.H{"status": status})
}

// ════════════════════════════════════════════════════════════════════════════════
// Token Testing / Debug
// ════════════════════════════════════════════════════════════════════════════════

// TestTokenFetch handles POST /api/soroush/test-token
// Tests GetGroupCallToken() for a specific account — useful for debugging.
func (h *SoroushHandler) TestTokenFetch(c *gin.Context) {
	var req struct {
		AccountID uint `json:"account_id" binding:"required"`
	}

	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Invalid request: " + err.Error()})
		return
	}

	var account models.SoroushAccount
	if err := db.DB.First(&account, req.AccountID).Error; err != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "Account not found"})
		return
	}

	if len(account.AuthKey) == 0 {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Account not verified — no auth key"})
		return
	}

	var cfg models.SoroushTunnelConfig
	if err := db.DB.First(&cfg).Error; err != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "Soroush tunnel config not found"})
		return
	}

	// TODO: Actually call fetchToken when the TL constructor is implemented
	logger.Info("Soroush", "Token test requested",
		"account_id", req.AccountID,
		"phone", maskPhoneForLog(account.PhoneNumber),
	)

	c.JSON(http.StatusOK, gin.H{
		"status":  "placeholder",
		"message": "Token fetch TL constructor not yet implemented — pending JS bundle reverse engineering",
		"account": gin.H{
			"id":    account.ID,
			"phone": maskPhoneForLog(account.PhoneNumber),
			"name":  account.DisplayName,
		},
		"config": gin.H{
			"group_chat_id": cfg.GroupChatID,
			"livekit_url":   cfg.LiveKitURL,
		},
	})
}

// maskPhoneForLog masks a phone number for safe logging output.
func maskPhoneForLog(phone string) string {
	if len(phone) < 4 {
		return "****"
	}
	return phone[:3] + "****" + phone[len(phone)-2:]
}
