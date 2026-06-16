package cloudflare

import (
	"context"
	"fmt"
	"time"

	"clever-connect/internal/logger"
	"clever-connect/internal/models"

	"github.com/cloudflare/cloudflare-go"
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

// NewClient helper to instantiate a cloudflare client
func NewClient(apiToken string) (*cloudflare.API, error) {
	if apiToken == "" {
		return nil, fmt.Errorf("API token is empty")
	}
	return cloudflare.NewWithAPIToken(apiToken)
}

// VerifyToken validates the Cloudflare token and returns the discovered accounts
func VerifyToken(token string, defaultName string) ([]models.CloudflareAccount, error) {
	api, err := NewClient(token)
	if err != nil {
		return nil, fmt.Errorf("failed to create client: %w", err)
	}

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	// Verify the API token first
	_, err = api.VerifyAPIToken(ctx)
	if err != nil {
		return nil, fmt.Errorf("invalid API token: %w", err)
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
			AccountName: name,
			AccountID:   acc.ID,
			APIToken:    token,
			Status:      "active",
		})
	}

	return results, nil
}

// GetStats retrieves zone counts, worker scripts, and 30-day bandwidth/request analytics
func GetStats(token string, accountID string) (*AccountStats, error) {
	api, err := NewClient(token)
	if err != nil {
		return nil, fmt.Errorf("failed to create client: %w", err)
	}

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	stats := &AccountStats{}

	// 1. Fetch Zones list
	zones, err := api.ListZones(ctx)
	if err != nil {
		logger.Error("CloudflareEngine", "Failed to list zones", "accountID", accountID, "error", err)
		return nil, fmt.Errorf("failed to list zones: %w", err)
	}

	// Filter zones belonging to this specific account and extract status
	var targetZoneIDs []string
	for _, z := range zones {
		if z.Account.ID == accountID {
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
	workers, _, err := api.ListWorkers(ctx, cloudflare.AccountIdentifier(accountID), cloudflare.ListWorkersParams{})
	if err != nil {
		logger.Warn("CloudflareEngine", "Failed to list workers (lack of permission or workers not enabled)", "accountID", accountID, "error", err)
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
