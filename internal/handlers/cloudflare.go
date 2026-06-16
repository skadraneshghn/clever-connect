package handlers

import (
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

type AddAccountRequest struct {
	AccountName string `json:"account_name" binding:"required"`
	APIToken    string `json:"api_token" binding:"required"`
}

type UpdateAccountRequest struct {
	AccountName string `json:"account_name" binding:"required"`
	APIToken    string `json:"api_token"`
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

// AddAccount POST /api/cloudflare/accounts
func (h *CloudflareHandler) AddAccount(c *gin.Context) {
	var req AddAccountRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Invalid request payload"})
		return
	}

	logger.Info("CloudflareAPI", "Adding new Cloudflare account", "name", req.AccountName)

	// Validate token and auto-discover accounts
	discovered, err := cloudflare.VerifyToken(req.APIToken, req.AccountName)
	if err != nil {
		logger.Warn("CloudflareAPI", "Failed to verify Cloudflare token", "error", err.Error())
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	var saved []models.CloudflareAccount
	for _, acc := range discovered {
		var existing models.CloudflareAccount
		if err := db.DB.Where("account_id = ?", acc.AccountID).First(&existing).Error; err == nil {
			// Update credentials of existing account ID
			existing.APIToken = acc.APIToken
			existing.AccountName = acc.AccountName
			existing.Status = "active"
			if err := db.DB.Save(&existing).Error; err != nil {
				logger.Error("CloudflareAPI", "Failed to update existing account ID in database", "accountID", acc.AccountID, "error", err)
				c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to update account"})
				return
			}
			saved = append(saved, existing)
		} else {
			// Create new account entry
			if err := db.DB.Create(&acc).Error; err != nil {
				logger.Error("CloudflareAPI", "Failed to save new account in database", "accountID", acc.AccountID, "error", err)
				c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to save account"})
				return
			}
			saved = append(saved, acc)
		}
	}

	c.JSON(http.StatusCreated, saved)
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
	if req.APIToken != "" {
		// Verify new token
		discovered, err := cloudflare.VerifyToken(req.APIToken, req.AccountName)
		if err != nil {
			c.JSON(http.StatusBadRequest, gin.H{"error": "Failed to verify new API token: " + err.Error()})
			return
		}
		account.APIToken = req.APIToken
		// Update account ID in case it changed
		if len(discovered) > 0 {
			account.AccountID = discovered[0].AccountID
		}
	}

	account.Status = "active"
	if err := db.DB.Save(&account).Error; err != nil {
		logger.Error("CloudflareAPI", "Failed to update account", "id", id, "error", err)
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

	stats, err := cloudflare.GetStats(account.APIToken, account.AccountID)
	if err != nil {
		// Update status in db on connection error
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
