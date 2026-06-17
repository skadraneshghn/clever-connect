package handlers

import (
	"crypto/rand"
	"encoding/hex"
	"fmt"
	"io"
	"net/http"
	"strconv"
	"strings"

	"clever-connect/internal/cloudflare"
	"clever-connect/internal/config"
	"clever-connect/internal/db"
	"clever-connect/internal/logger"
	"clever-connect/internal/models"

	cfSdk "github.com/cloudflare/cloudflare-go"
	"github.com/gin-gonic/gin"
)

type CloudflareHandler struct {
	cfg *config.Config
}

func NewCloudflareHandler(cfg *config.Config) *CloudflareHandler {
	return &CloudflareHandler{cfg: cfg}
}

// proxyToServer automatically forwards requests from the Client Panel to the remote Clever Cloud server.
func (h *CloudflareHandler) proxyToServer(c *gin.Context, method string, apiPath string) bool {
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

	req, err := http.NewRequest(method, remoteURL, c.Request.Body)
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

type UpdateAccountRequest struct {
	AccountName string `json:"account_name" binding:"required"`
}

func generateState() string {
	b := make([]byte, 16)
	_, _ = rand.Read(b)
	return hex.EncodeToString(b)
}

// ListAccounts GET /api/cloudflare/accounts
func (h *CloudflareHandler) ListAccounts(c *gin.Context) {
	if h.proxyToServer(c, c.Request.Method, c.Request.URL.Path) {
		return
	}
	var accounts []models.CloudflareAccount
	if err := db.DB.Find(&accounts).Error; err != nil {
		logger.Error("CloudflareAPI", "Failed to retrieve accounts from database", "error", err)
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to retrieve accounts"})
		return
	}

	c.JSON(http.StatusOK, accounts)
}

// OAuthLogin GET /api/cloudflare/oauth/login
func (h *CloudflareHandler) OAuthLogin(c *gin.Context) {
	if h.proxyToServer(c, c.Request.Method, c.Request.URL.Path) {
		return
	}
	oauthConfig := cloudflare.GetOAuthConfig(h.cfg)
	if oauthConfig.ClientID == "" || oauthConfig.ClientSecret == "" {
		c.String(http.StatusInternalServerError, "Cloudflare OAuth is not configured on this server (missing Client ID or Client Secret)")
		return
	}

	alias := c.DefaultQuery("alias", "Cloudflare Account")
	c.SetCookie("cloudflare_oauth_alias", alias, 300, "/api/cloudflare/oauth", "", false, true)

	state := generateState()
	// Save state in secure HTTP-only cookie for verification in callback
	c.SetCookie("cloudflare_oauth_state", state, 300, "/api/cloudflare/oauth", "", false, true)

	url := oauthConfig.AuthCodeURL(state)
	c.Redirect(http.StatusTemporaryRedirect, url)
}

// OAuthCallback GET /api/cloudflare/oauth/callback
func (h *CloudflareHandler) OAuthCallback(c *gin.Context) {
	if h.proxyToServer(c, c.Request.Method, c.Request.URL.Path) {
		return
	}
	oauthConfig := cloudflare.GetOAuthConfig(h.cfg)

	// Verify state to prevent CSRF
	stateQuery := c.Query("state")
	stateCookie, err := c.Cookie("cloudflare_oauth_state")
	if err != nil || stateCookie == "" || stateCookie != stateQuery {
		logger.Warn("CloudflareAPI", "OAuth Callback state verification failed", "query", stateQuery, "cookie", stateCookie)
		c.String(http.StatusBadRequest, "OAuth state verification failed. Possible CSRF attack.")
		return
	}

	// Clear the state cookie
	c.SetCookie("cloudflare_oauth_state", "", -1, "/api/cloudflare/oauth", "", false, true)

	aliasCookie, _ := c.Cookie("cloudflare_oauth_alias")
	if aliasCookie == "" {
		aliasCookie = "Cloudflare Account"
	}
	// Clear the alias cookie
	c.SetCookie("cloudflare_oauth_alias", "", -1, "/api/cloudflare/oauth", "", false, true)

	code := c.Query("code")
	if code == "" {
		errParam := c.Query("error")
		errDesc := c.Query("error_description")
		if errParam != "" {
			logger.Warn("CloudflareAPI", "OAuth callback returned error", "error", errParam, "description", errDesc)
			c.String(http.StatusBadRequest, fmt.Sprintf("Cloudflare OAuth Error: %s - %s", errParam, errDesc))
			return
		}
		c.String(http.StatusBadRequest, "Missing authorization code from Cloudflare")
		return
	}

	// Verify token and auto-discover accounts
	discovered, err := cloudflare.VerifyOAuthToken(c.Request.Context(), oauthConfig, code, aliasCookie)
	if err != nil {
		logger.Warn("CloudflareAPI", "Failed to verify Cloudflare OAuth token", "error", err.Error())
		c.String(http.StatusInternalServerError, "Authentication failed: "+err.Error())
		return
	}

	for _, acc := range discovered {
		var existing models.CloudflareAccount
		if err := db.DB.Where("account_id = ?", acc.AccountID).First(&existing).Error; err == nil {
			// Update credentials of existing account ID
			existing.AccessToken = acc.AccessToken
			existing.RefreshToken = acc.RefreshToken
			existing.TokenExpiry = acc.TokenExpiry
			existing.Status = "active"
			if err := db.DB.Save(&existing).Error; err != nil {
				logger.Error("CloudflareAPI", "Failed to update existing account ID in database", "accountID", acc.AccountID, "error", err)
				c.String(http.StatusInternalServerError, "Failed to update account credentials")
				return
			}
		} else {
			// Create new account entry
			if err := db.DB.Create(&acc).Error; err != nil {
				logger.Error("CloudflareAPI", "Failed to save new account in database", "accountID", acc.AccountID, "error", err)
				c.String(http.StatusInternalServerError, "Failed to save account details")
				return
			}
		}
	}

	// Success response: Close the popup window and post message to parent React application
	c.Header("Content-Type", "text/html; charset=utf-8")
	c.String(http.StatusOK, `<!DOCTYPE html>
<html>
<head>
    <title>Auth Success</title>
    <script>
        if (window.opener) {
            window.opener.postMessage("cloudflare_auth_success", "*");
        }
        window.close();
    </script>
</head>
<body style="font-family:-apple-system,BlinkMacSystemFont,'Segoe UI',Roboto,Helvetica,Arial,sans-serif;text-align:center;padding-top:50px;color:#333;background:#f9f9f9;">
    <div style="max-width:400px;margin:0 auto;background:white;padding:30px;border-radius:8px;box-shadow:0 2px 10px rgba(0,0,0,0.05);">
        <h2 style="color:#22c55e;margin-top:0;">Authentication Successful!</h2>
        <p>Your Cloudflare account was connected successfully.</p>
        <p style="color:#666;font-size:14px;">This window will close automatically.</p>
        <button onclick="window.close();" style="background:#ff6b2c;color:white;border:none;padding:10px 20px;border-radius:6px;font-size:14px;cursor:pointer;font-weight:600;margin-top:10px;">Close Window</button>
    </div>
</body>
</html>`)
}

// UpdateAccount PUT /api/cloudflare/accounts/:id
func (h *CloudflareHandler) UpdateAccount(c *gin.Context) {
	if h.proxyToServer(c, c.Request.Method, c.Request.URL.Path) {
		return
	}
	idParam := c.Param("id")
	id, err := strconv.ParseUint(idParam, 10, 32)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Invalid account ID"})
		return
	}

	var req UpdateAccountRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Invalid request payload"})
		return
	}

	var account models.CloudflareAccount
	if err := db.DB.First(&account, id).Error; err != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "Account not found"})
		return
	}

	account.AccountName = req.AccountName
	if err := db.DB.Save(&account).Error; err != nil {
		logger.Error("CloudflareAPI", "Failed to update account alias name", "id", id, "error", err)
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to update account"})
		return
	}

	c.JSON(http.StatusOK, account)
}

// DeleteAccount DELETE /api/cloudflare/accounts/:id
func (h *CloudflareHandler) DeleteAccount(c *gin.Context) {
	if h.proxyToServer(c, c.Request.Method, c.Request.URL.Path) {
		return
	}
	idParam := c.Param("id")
	id, err := strconv.ParseUint(idParam, 10, 32)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Invalid account ID"})
		return
	}

	if err := db.DB.Delete(&models.CloudflareAccount{}, id).Error; err != nil {
		logger.Error("CloudflareAPI", "Failed to delete account", "id", id, "error", err)
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to delete account"})
		return
	}

	c.JSON(http.StatusOK, gin.H{"message": "Account deleted successfully"})
}

type AddAccountRequest struct {
	AccountName string `json:"account_name" binding:"required"`
	AuthType    string `json:"auth_type" binding:"required,oneof=token key"`
	Token       string `json:"token" binding:"required"`
	Email       string `json:"email"`
}

// AddAccount POST /api/cloudflare/accounts
func (h *CloudflareHandler) AddAccount(c *gin.Context) {
	if h.proxyToServer(c, c.Request.Method, c.Request.URL.Path) {
		return
	}
	var req AddAccountRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Invalid request payload: " + err.Error()})
		return
	}

	if req.AuthType == "key" && req.Email == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Email is required when using a Global API Key"})
		return
	}

	// Verify manual token and discover accounts
	discovered, err := cloudflare.VerifyManualToken(c.Request.Context(), req.AuthType, req.Token, req.Email, req.AccountName)
	if err != nil {
		logger.Warn("CloudflareAPI", "Failed to verify manual Cloudflare credentials", "error", err.Error())
		c.JSON(http.StatusBadRequest, gin.H{"error": "Authentication failed: " + err.Error()})
		return
	}

	for _, acc := range discovered {
		var existing models.CloudflareAccount
		if err := db.DB.Where("account_id = ?", acc.AccountID).First(&existing).Error; err == nil {
			// Update credentials of existing account
			existing.AccountName = acc.AccountName
			existing.AccessToken = acc.AccessToken
			existing.Email = acc.Email
			existing.AuthType = acc.AuthType
			existing.Status = "active"
			if err := db.DB.Save(&existing).Error; err != nil {
				logger.Error("CloudflareAPI", "Failed to update existing account ID in database", "accountID", acc.AccountID, "error", err)
				c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to update account credentials"})
				return
			}
		} else {
			// Create new account entry
			if err := db.DB.Create(&acc).Error; err != nil {
				logger.Error("CloudflareAPI", "Failed to save new account in database", "accountID", acc.AccountID, "error", err)
				c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to save account details"})
				return
			}
		}
	}

	c.JSON(http.StatusOK, gin.H{"message": "Manual account(s) added successfully", "count": len(discovered)})
}

// GetAccountStats GET /api/cloudflare/accounts/:id/stats
func (h *CloudflareHandler) GetAccountStats(c *gin.Context) {
	if h.proxyToServer(c, c.Request.Method, c.Request.URL.Path) {
		return
	}
	idParam := c.Param("id")
	id, err := strconv.ParseUint(idParam, 10, 32)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Invalid account ID"})
		return
	}

	var account models.CloudflareAccount
	if err := db.DB.First(&account, id).Error; err != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "Account not found"})
		return
	}

	oauthConfig := cloudflare.GetOAuthConfig(h.cfg)
	stats, err := cloudflare.GetStats(c.Request.Context(), oauthConfig, &account)
	if err != nil {
		// Update status in db on connection/refresh error
		account.Status = "error"
		_ = db.DB.Save(&account)

		logger.Error("CloudflareAPI", "Failed to fetch stats from Cloudflare", "id", id, "error", err)
		c.JSON(http.StatusBadGateway, gin.H{"error": "Failed to fetch stats from Cloudflare: " + err.Error()})
		return
	}

	// Make sure status is active if stats fetch is successful
	if account.Status != "active" {
		account.Status = "active"
		_ = db.DB.Save(&account)
	}

	c.JSON(http.StatusOK, stats)
}

// GetZones GET /api/cloudflare/zones?account_id=xxx
func (h *CloudflareHandler) GetZones(c *gin.Context) {
	if h.proxyToServer(c, c.Request.Method, c.Request.URL.Path) {
		return
	}
	accountID := c.Query("account_id")
	if accountID == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Missing account_id parameter"})
		return
	}

	var account models.CloudflareAccount
	if err := db.DB.Where("account_id = ?", accountID).First(&account).Error; err != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "Cloudflare account not found"})
		return
	}

	oauthConfig := cloudflare.GetOAuthConfig(h.cfg)
	if err := cloudflare.RefreshAccountToken(c.Request.Context(), oauthConfig, &account); err != nil {
		c.JSON(http.StatusBadGateway, gin.H{"error": "Failed to refresh token: " + err.Error()})
		return
	}

	api, err := cloudflare.GetAPIClient(&account)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to initialize Cloudflare client: " + err.Error()})
		return
	}

	zones, err := api.ListZones(c.Request.Context())
	if err != nil {
		c.JSON(http.StatusBadGateway, gin.H{"error": "Failed to fetch zones from Cloudflare: " + err.Error()})
		return
	}

	var accountZones []cfSdk.Zone
	for _, z := range zones {
		if z.Account.ID == accountID {
			accountZones = append(accountZones, z)
		}
	}

	c.JSON(http.StatusOK, accountZones)
}

type DeployWorkerRequest struct {
	AccountID    string `json:"account_id" binding:"required"`
	ScriptName   string `json:"script_name" binding:"required"`
	CustomDomain string `json:"custom_domain"`
	ZoneID       string `json:"zone_id"`
}

// DeployWorker POST /api/cloudflare/workers/deploy
func (h *CloudflareHandler) DeployWorker(c *gin.Context) {
	if h.proxyToServer(c, c.Request.Method, c.Request.URL.Path) {
		return
	}
	var req DeployWorkerRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	var account models.CloudflareAccount
	if err := db.DB.Where("account_id = ?", req.AccountID).First(&account).Error; err != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "Cloudflare account not found"})
		return
	}

	oauthConfig := cloudflare.GetOAuthConfig(h.cfg)

	deployment, err := cloudflare.DeployWorkerScript(c.Request.Context(), oauthConfig, &account, req.ScriptName, req.CustomDomain, req.ZoneID)
	if err != nil {
		logger.Error("CloudflareAPI", "Failed to deploy worker", "scriptName", req.ScriptName, "error", err)
		c.JSON(http.StatusBadGateway, gin.H{"error": err.Error()})
		return
	}

	status, msg, err := cloudflare.CheckWorkerHealth(c.Request.Context(), deployment)
	if err == nil {
		deployment.HealthStatus = status
		deployment.Message = msg
		_ = db.DB.Save(deployment)
	}

	c.JSON(http.StatusOK, deployment)
}

// ListDeployments GET /api/cloudflare/workers/deployments
func (h *CloudflareHandler) ListDeployments(c *gin.Context) {
	if h.proxyToServer(c, c.Request.Method, c.Request.URL.Path) {
		return
	}
	var deployments []models.CloudflareWorkerDeployment
	if err := db.DB.Find(&deployments).Error; err != nil {
		logger.Error("CloudflareAPI", "Failed to retrieve worker deployments from DB", "error", err)
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to retrieve deployments"})
		return
	}
	c.JSON(http.StatusOK, deployments)
}

// DeleteDeployment DELETE /api/cloudflare/workers/deployments/:id
func (h *CloudflareHandler) DeleteDeployment(c *gin.Context) {
	if h.proxyToServer(c, c.Request.Method, c.Request.URL.Path) {
		return
	}
	idParam := c.Param("id")
	id, err := strconv.ParseUint(idParam, 10, 32)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Invalid deployment ID"})
		return
	}

	if err := db.DB.Delete(&models.CloudflareWorkerDeployment{}, id).Error; err != nil {
		logger.Error("CloudflareAPI", "Failed to delete worker deployment", "id", id, "error", err)
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to delete deployment"})
		return
	}

	c.JSON(http.StatusOK, gin.H{"message": "Deployment record deleted successfully"})
}

// CheckDeploymentHealth POST /api/cloudflare/workers/deployments/:id/check-health
func (h *CloudflareHandler) CheckDeploymentHealth(c *gin.Context) {
	if h.proxyToServer(c, c.Request.Method, c.Request.URL.Path) {
		return
	}
	idParam := c.Param("id")
	id, err := strconv.ParseUint(idParam, 10, 32)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Invalid deployment ID"})
		return
	}

	var deployment models.CloudflareWorkerDeployment
	if err := db.DB.First(&deployment, id).Error; err != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "Deployment not found"})
		return
	}

	status, msg, err := cloudflare.CheckWorkerHealth(c.Request.Context(), &deployment)
	if err != nil {
		deployment.HealthStatus = "error"
		deployment.Message = err.Error()
	} else {
		deployment.HealthStatus = status
		deployment.Message = msg
	}

	if err := db.DB.Save(&deployment).Error; err != nil {
		logger.Error("CloudflareAPI", "Failed to update deployment health status in DB", "id", id, "error", err)
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to update health status"})
		return
	}

	c.JSON(http.StatusOK, deployment)
}

