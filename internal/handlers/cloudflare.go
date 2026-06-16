package handlers

import (
	"crypto/rand"
	"encoding/hex"
	"net/http"
	"strconv"

	"clever-connect/internal/cloudflare"
	"clever-connect/internal/config"
	"clever-connect/internal/db"
	"clever-connect/internal/logger"
	"clever-connect/internal/models"

	"github.com/gin-gonic/gin"
)

type CloudflareHandler struct {
	cfg *config.Config
}

func NewCloudflareHandler(cfg *config.Config) *CloudflareHandler {
	return &CloudflareHandler{cfg: cfg}
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

// GetAccountStats GET /api/cloudflare/accounts/:id/stats
func (h *CloudflareHandler) GetAccountStats(c *gin.Context) {
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
