package handlers

import (
	"context"
	"encoding/json"
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

	// Defensive check: Wipe any soft-deleted records with the same phone number
	var existingAccount models.SoroushAccount
	if err := db.DB.Unscoped().Where("phone_number = ?", req.PhoneNumber).First(&existingAccount).Error; err == nil {
		if err := db.DB.Unscoped().Delete(&existingAccount).Error; err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to purge existing ghost account: " + err.Error()})
			return
		}
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

	// Unscoped permanent deletion to free unique constraints immediately
	if err := db.DB.Unscoped().Delete(&models.SoroushAccount{}, id).Error; err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to delete account"})
		return
	}

	c.JSON(http.StatusOK, gin.H{"status": "deleted", "id": id})
}

// UpdateSoroushAccountToken handles PUT /api/soroush/accounts/:id/token
// Sets the per-account LiveKit JWT token for a specific worker account.
func (h *SoroushHandler) UpdateSoroushAccountToken(c *gin.Context) {
	id, err := strconv.ParseUint(c.Param("id"), 10, 64)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Invalid account ID"})
		return
	}

	var req struct {
		LiveKitToken string `json:"livekit_token" binding:"required"`
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

	account.LiveKitToken = req.LiveKitToken
	db.DB.Save(&account)

	logger.Info("Soroush", "Account LiveKit token updated", "phone", maskPhoneForLog(account.PhoneNumber))
	c.JSON(http.StatusOK, gin.H{"status": "updated", "account": account})
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
		GroupChatID          int64  `json:"group_chat_id"`
		GroupAccessHash      int64  `json:"group_access_hash"`
		CallID               int64  `json:"call_id"`
		CallAccessHash       string `json:"call_access_hash"`
		ServerIdentity       string `json:"server_identity"`
		PSK                  string `json:"psk"`
		LiveKitURL           string `json:"livekit_url"`
		FallbackLiveKitToken string `json:"fallback_livekit_token"`
		SocksPort            int    `json:"socks_port"`
		MaxWorkers           int    `json:"max_workers"`
		LoadBalanceAlgo      string `json:"load_balance_algo"`
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

	// Update fields (allow 0 or empty to clear them)
	cfg.GroupChatID = req.GroupChatID
	cfg.GroupAccessHash = req.GroupAccessHash
	cfg.CallID = req.CallID
	cfg.CallAccessHash = req.CallAccessHash
	cfg.FallbackLiveKitToken = req.FallbackLiveKitToken

	if req.ServerIdentity != "" {
		cfg.ServerIdentity = req.ServerIdentity
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

	db.DB.Save(&cfg)

	logger.Info("Soroush", "Tunnel configuration updated")
	c.JSON(http.StatusOK, gin.H{"status": "saved", "config": cfg})
}

// GetSoroushGroups handles GET /api/soroush/groups
// Fetches dialogs using the first verified account and returns groups.
func (h *SoroushHandler) GetSoroushGroups(c *gin.Context) {
	var accounts []models.SoroushAccount
	db.DB.Where("status = ?", "verified").Find(&accounts)
	if len(accounts) == 0 {
		c.JSON(http.StatusBadRequest, gin.H{"error": "No verified Soroush accounts available to fetch groups"})
		return
	}

	resolverAcct := accounts[0]
	session, transport := soroushlib.RestoreSession(resolverAcct.AuthKey, resolverAcct.AuthKeyID, resolverAcct.ServerSalt)

	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()

	if err := transport.Connect(ctx); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to connect to Soroush: " + err.Error()})
		return
	}
	defer transport.Disconnect()

	if err := session.WarmUpSession(ctx); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to warm up Soroush session: " + err.Error()})
		return
	}

	body := soroushlib.BuildGetDialogsRequest()
	wrapped := soroushlib.WrapInitConnection(soroushlib.SoroushAppID, body)
	cid, reader, err := session.SendAndWait(ctx, wrapped, true)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to fetch dialogs: " + err.Error()})
		return
	}

	groups, err := soroushlib.ParseDialogsForGroups(cid, reader)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to parse dialogs: " + err.Error()})
		return
	}

	// Filter for only group chats (group or supergroup)
	var filtered []soroushlib.DialogInfo
	for _, g := range groups {
		if g.Type == "group" || g.Type == "supergroup" {
			filtered = append(filtered, g)
		}
	}

	c.JSON(http.StatusOK, filtered)
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
		"config":     cfg,
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

// TestTokenFetch handles POST /api/soroush/test-token
func (h *SoroushHandler) TestTokenFetch(c *gin.Context) {
	c.JSON(http.StatusOK, gin.H{
		"status":  "not_needed",
		"message": "LiveKit SFU tokens are now set per-account via PUT /api/soroush/accounts/:id/token.",
	})
}

// SyncConfig handles GET /api/soroush/sync
// Server-side: Serializes the tunnel configuration into a JSON payload
// that the client can fetch during the temporary open internet window.
func (h *SoroushHandler) SyncConfig(c *gin.Context) {
	var cfg models.SoroushTunnelConfig
	if err := db.DB.First(&cfg).Error; err != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "Soroush tunnel config not found"})
		return
	}

	verifyToken, err := soroush.DeriveVerificationToken(cfg.PSK)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to derive verification token"})
		return
	}

	logger.Info("Soroush", "Sync config served to client")

	c.JSON(http.StatusOK, gin.H{
		"sync_payload": gin.H{
			"group_chat_id":          cfg.GroupChatID,
			"group_access_hash":      cfg.GroupAccessHash,
			"call_id":                cfg.CallID,
			"call_access_hash":      cfg.CallAccessHash,
			"server_identity":        cfg.ServerIdentity,
			"psk":                    cfg.PSK,
			"livekit_url":            cfg.LiveKitURL,
			"fallback_livekit_token": cfg.FallbackLiveKitToken,
			"socks_port":             cfg.SocksPort,
			"max_workers":            cfg.MaxWorkers,
			"load_balance_algo":      cfg.LoadBalanceAlgo,
			"verification_token":     verifyToken,
		},
	})
}

// IngestSync handles POST /api/soroush/sync
// Client-side: Receives the sync payload from the server and commits it
// to the local SQLite database.
func (h *SoroushHandler) IngestSync(c *gin.Context) {
	var req struct {
		ServerURL string `json:"server_url" binding:"required"`
		Token     string `json:"token" binding:"required"`
	}

	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Invalid request: " + err.Error()})
		return
	}

	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()

	httpReq, err := http.NewRequestWithContext(ctx, "GET", req.ServerURL+"/api/soroush/sync", nil)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Invalid server URL: " + err.Error()})
		return
	}
	httpReq.Header.Set("Authorization", "Bearer "+req.Token)

	resp, err := http.DefaultClient.Do(httpReq)
	if err != nil {
		c.JSON(http.StatusBadGateway, gin.H{"error": "Failed to reach server: " + err.Error()})
		return
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		c.JSON(http.StatusBadGateway, gin.H{"error": "Server returned status " + strconv.Itoa(resp.StatusCode)})
		return
	}

	var syncResp struct {
		SyncPayload struct {
			GroupChatID          int64  `json:"group_chat_id"`
			GroupAccessHash      int64  `json:"group_access_hash"`
			CallID               int64  `json:"call_id"`
			CallAccessHash       string `json:"call_access_hash"`
			ServerIdentity       string `json:"server_identity"`
			PSK                  string `json:"psk"`
			LiveKitURL           string `json:"livekit_url"`
			FallbackLiveKitToken string `json:"fallback_livekit_token"`
			SocksPort            int    `json:"socks_port"`
			MaxWorkers           int    `json:"max_workers"`
			LoadBalanceAlgo      string `json:"load_balance_algo"`
			VerifyToken          string `json:"verification_token"`
		} `json:"sync_payload"`
	}

	if err := json.NewDecoder(resp.Body).Decode(&syncResp); err != nil {
		c.JSON(http.StatusBadGateway, gin.H{"error": "Failed to parse server response: " + err.Error()})
		return
	}

	p := syncResp.SyncPayload

	localVerify, err := soroush.DeriveVerificationToken(p.PSK)
	if err != nil || localVerify != p.VerifyToken {
		c.JSON(http.StatusUnauthorized, gin.H{"error": "Sync payload integrity check failed — PSK verification mismatch"})
		return
	}

	var cfg models.SoroushTunnelConfig
	if err := db.DB.First(&cfg).Error; err != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "Local Soroush tunnel config not initialized"})
		return
	}

	if p.GroupChatID != 0 {
		cfg.GroupChatID = p.GroupChatID
	}
	if p.GroupAccessHash != 0 {
		cfg.GroupAccessHash = p.GroupAccessHash
	}
	if p.ServerIdentity != "" {
		cfg.ServerIdentity = p.ServerIdentity
	}
	cfg.PSK = p.PSK
	if p.LiveKitURL != "" {
		cfg.LiveKitURL = p.LiveKitURL
	}
	if p.FallbackLiveKitToken != "" {
		cfg.FallbackLiveKitToken = p.FallbackLiveKitToken
	}
	if p.CallID != 0 {
		cfg.CallID = p.CallID
	}
	if p.CallAccessHash != "" {
		cfg.CallAccessHash = p.CallAccessHash
	}
	if p.SocksPort > 0 {
		cfg.SocksPort = p.SocksPort
	}
	if p.MaxWorkers > 0 {
		cfg.MaxWorkers = p.MaxWorkers
	}
	if p.LoadBalanceAlgo != "" {
		cfg.LoadBalanceAlgo = p.LoadBalanceAlgo
	}

	db.DB.Save(&cfg)

	logger.Info("Soroush", "Client synced with server",
		"server_identity", cfg.ServerIdentity,
	)

	c.JSON(http.StatusOK, gin.H{
		"status":  "synced",
		"message": "Configuration ingested from server. Global internet window can now be closed.",
		"config":  cfg,
	})
}

// maskPhoneForLog masks a phone number for safe logging output.
func maskPhoneForLog(phone string) string {
	if len(phone) < 4 {
		return "****"
	}
	return phone[:3] + "****" + phone[len(phone)-2:]
}
