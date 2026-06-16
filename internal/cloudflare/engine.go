package cloudflare

import (
	"context"
	"fmt"
	"time"

	"clever-connect/internal/config"
	"clever-connect/internal/db"
	"clever-connect/internal/logger"
	"clever-connect/internal/models"

	"github.com/cloudflare/cloudflare-go"
	"golang.org/x/oauth2"
)

// AccountStats represents the aggregated usage and limit statistics for a Cloudflare account
type AccountStats struct {
	TotalZones      int   `json:"total_zones"`
	ActiveZones     int   `json:"active_zones"`
	PendingZones    int   `json:"pending_zones"`
	WorkerScripts   int   `json:"worker_scripts"`
	TotalBandwidth  int64 `json:"total_bandwidth"`  // in bytes
	CachedBandwidth int64 `json:"cached_bandwidth"` // in bytes
	TotalRequests   int64 `json:"total_requests"`
	CachedRequests  int64 `json:"cached_requests"`
}

// GetOAuthConfig constructs the oauth2.Config for Cloudflare OAuth flow
func GetOAuthConfig(cfg *config.Config) *oauth2.Config {
	return &oauth2.Config{
		ClientID:     cfg.CloudflareClientID,
		ClientSecret: cfg.CloudflareClientSecret,
		RedirectURL:  cfg.CloudflareRedirectURL,
		Endpoint: oauth2.Endpoint{
			AuthURL:  "https://dash.cloudflare.com/oauth2/auth",
			TokenURL: "https://dash.cloudflare.com/oauth2/token",
		},
		Scopes: []string{"account:read", "zone:read", "zone:write", "workers:read", "workers:write"},
	}
}

// VerifyOAuthToken exchanges authorization code for OAuth tokens and returns discovered accounts
func VerifyOAuthToken(ctx context.Context, oauthConfig *oauth2.Config, code string, defaultName string) ([]models.CloudflareAccount, error) {
	token, err := oauthConfig.Exchange(ctx, code)
	if err != nil {
		return nil, fmt.Errorf("oauth exchange failed: %w", err)
	}

	api, err := cloudflare.NewWithAPIToken(token.AccessToken)
	if err != nil {
		return nil, fmt.Errorf("failed to create client: %w", err)
	}

	// Discover accounts accessible by this token
	accounts, _, err := api.Accounts(ctx, cloudflare.AccountsListParams{})
	if err != nil {
		return nil, fmt.Errorf("failed to list accounts: %w", err)
	}

	if len(accounts) == 0 {
		return nil, fmt.Errorf("no accounts found for this token")
	}

	var results []models.CloudflareAccount
	for _, acc := range accounts {
		name := defaultName
		if len(accounts) > 1 {
			name = fmt.Sprintf("%s (%s)", defaultName, acc.Name)
		}
		results = append(results, models.CloudflareAccount{
			AccountName:  name,
			AccountID:    acc.ID,
			AccessToken:  token.AccessToken,
			RefreshToken: token.RefreshToken,
			TokenExpiry:  token.Expiry,
			Status:       "active",
		})
	}

	return results, nil
}

// RefreshAccountToken checks if the token is expired, refreshes it using OAuth client credentials, and persists it to database.
func RefreshAccountToken(ctx context.Context, oauthConfig *oauth2.Config, account *models.CloudflareAccount) error {
	srcToken := &oauth2.Token{
		AccessToken:  account.AccessToken,
		RefreshToken: account.RefreshToken,
		Expiry:       account.TokenExpiry,
	}

	tokenSource := oauthConfig.TokenSource(ctx, srcToken)
	freshToken, err := tokenSource.Token()
	if err != nil {
		return fmt.Errorf("oauth token refresh failed: %w", err)
	}

	if freshToken.AccessToken != account.AccessToken {
		account.AccessToken = freshToken.AccessToken
		account.RefreshToken = freshToken.RefreshToken
		account.TokenExpiry = freshToken.Expiry
		account.Status = "active"

		if db.DB != nil {
			if err := db.DB.Save(account).Error; err != nil {
				logger.Error("CloudflareEngine", "Failed to save refreshed token details to GORM", "accountID", account.AccountID, "error", err)
			} else {
				logger.Info("CloudflareEngine", "OAuth token auto-refreshed successfully", "accountID", account.AccountID)
			}
		}
	}
	return nil
}

// GetStats retrieves zone counts, worker scripts, and 30-day bandwidth/request analytics using OAuth2,
// auto-refreshing the token if it has expired.
func GetStats(ctx context.Context, oauthConfig *oauth2.Config, account *models.CloudflareAccount) (*AccountStats, error) {
	if err := RefreshAccountToken(ctx, oauthConfig, account); err != nil {
		return nil, err
	}

	// Instantiate the Cloudflare SDK with the current access token
	api, err := cloudflare.NewWithAPIToken(account.AccessToken)
	if err != nil {
		return nil, fmt.Errorf("failed to create client: %w", err)
	}

	stats := &AccountStats{}

	// 1. Fetch Zones list
	zones, err := api.ListZones(ctx)
	if err != nil {
		logger.Error("CloudflareEngine", "Failed to list zones", "accountID", account.AccountID, "error", err)
		return nil, fmt.Errorf("failed to list zones: %w", err)
	}

	// Filter zones belonging to this specific account and extract status
	var targetZoneIDs []string
	for _, z := range zones {
		if z.Account.ID == account.AccountID {
			stats.TotalZones++
			if z.Status == "active" {
				stats.ActiveZones++
				targetZoneIDs = append(targetZoneIDs, z.ID)
			} else {
				stats.PendingZones++
			}
		}
	}

	// 2. Fetch Worker script counts
	workers, _, err := api.ListWorkers(ctx, cloudflare.AccountIdentifier(account.AccountID), cloudflare.ListWorkersParams{})
	if err != nil {
		logger.Warn("CloudflareEngine", "Failed to list workers (lack of permission or workers not enabled)", "accountID", account.AccountID, "error", err)
		stats.WorkerScripts = 0
	} else {
		stats.WorkerScripts = len(workers.WorkerList)
	}

	// 3. Fetch analytics for active zones (last 30 days)
	since := time.Now().Add(-30 * 24 * time.Hour)
	until := time.Now()
	options := cloudflare.ZoneAnalyticsOptions{
		Since: &since,
		Until: &until,
	}

	for _, zoneID := range targetZoneIDs {
		analytics, err := api.ZoneAnalyticsDashboard(ctx, zoneID, options)
		if err != nil {
			logger.Warn("CloudflareEngine", "Failed to fetch zone analytics dashboard (analytics not supported or permissions missing)", "zoneID", zoneID, "error", err)
			continue
		}
		stats.TotalBandwidth += int64(analytics.Totals.Bandwidth.All)
		stats.CachedBandwidth += int64(analytics.Totals.Bandwidth.Cached)
		stats.TotalRequests += int64(analytics.Totals.Requests.All)
		stats.CachedRequests += int64(analytics.Totals.Requests.Cached)
	}

	return stats, nil
}
