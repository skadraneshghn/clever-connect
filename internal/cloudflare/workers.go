package cloudflare

import (
	"archive/zip"
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"strings"
	"time"

	"clever-connect/internal/db"
	"clever-connect/internal/logger"
	"clever-connect/internal/models"

	"github.com/cloudflare/cloudflare-go"
	"golang.org/x/oauth2"
)

// DeployWorkerScript reads worker.js from disk, uploads it, resolves the subdomain, and attaches a custom domain (if specified)
func DeployWorkerScript(ctx context.Context, oauthConfig *oauth2.Config, account *models.CloudflareAccount, scriptName string, customDomain string, zoneID string) (*models.CloudflareWorkerDeployment, error) {
	// 1. Refresh OAuth token if needed
	if err := RefreshAccountToken(ctx, oauthConfig, account); err != nil {
		return nil, fmt.Errorf("failed to refresh account token: %w", err)
	}

	// 2. Read worker code from local file (download from GitHub zip archive if missing)
	localPath := "data/worker.js"
	if _, err := os.Stat(localPath); os.IsNotExist(err) {
		logger.Info("CloudflareWorkers", "worker.js not found locally. Downloading from GitHub zip archive...", "url", "https://github.com/IRNova/Nova-Proxy/archive/refs/tags/V3.0.0.zip")
		
		// Ensure parent directory exists
		if err := os.MkdirAll("data", 0755); err != nil {
			return nil, fmt.Errorf("failed to create data directory: %w", err)
		}

		// Download zip archive
		downloadURL := "https://github.com/IRNova/Nova-Proxy/archive/refs/tags/V3.0.0.zip"
		req, err := http.NewRequestWithContext(ctx, "GET", downloadURL, nil)
		if err != nil {
			return nil, fmt.Errorf("failed to create download request: %w", err)
		}

		client := &http.Client{Timeout: 60 * time.Second}
		resp, err := client.Do(req)
		if err != nil {
			return nil, fmt.Errorf("failed to download worker zip from GitHub: %w", err)
		}
		defer resp.Body.Close()

		if resp.StatusCode != http.StatusOK {
			return nil, fmt.Errorf("failed to download worker zip from GitHub: received status %d", resp.StatusCode)
		}

		// Read the entire zip body into memory
		zipBytes, err := io.ReadAll(resp.Body)
		if err != nil {
			return nil, fmt.Errorf("failed to read zip body response: %w", err)
		}

		zipReader, err := zip.NewReader(bytes.NewReader(zipBytes), int64(len(zipBytes)))
		if err != nil {
			return nil, fmt.Errorf("failed to parse zip archive: %w", err)
		}

		var found bool
		for _, file := range zipReader.File {
			if file.FileInfo().Name() == "worker.js" && !file.FileInfo().IsDir() {
				rcFile, err := file.Open()
				if err != nil {
					return nil, fmt.Errorf("failed to open worker.js inside zip: %w", err)
				}
				defer rcFile.Close()

				out, err := os.Create(localPath)
				if err != nil {
					return nil, fmt.Errorf("failed to create file at %s: %w", localPath, err)
				}
				defer out.Close()

				if _, err = io.Copy(out, rcFile); err != nil {
					return nil, fmt.Errorf("failed to save extracted worker script: %w", err)
				}
				found = true
				break
			}
		}

		if !found {
			return nil, fmt.Errorf("worker.js not found inside the downloaded zip archive")
		}

		logger.Info("CloudflareWorkers", "worker.js extracted and saved successfully from V3.0.0 zip", "path", localPath)
	}

	scriptBytes, err := os.ReadFile(localPath)
	if err != nil {
		return nil, fmt.Errorf("failed to read worker script at %s: %w", localPath, err)
	}

	// 3. Initialize Cloudflare SDK API Client
	api, err := GetAPIClient(account)
	if err != nil {
		return nil, fmt.Errorf("failed to initialize Cloudflare client: %w", err)
	}

	// 4. Manage KV Namespace (Find or Create 'KV' namespace)
	rc := cloudflare.AccountIdentifier(account.AccountID)
	logger.Info("CloudflareWorkers", "Listing KV namespaces to check for 'KV' namespace", "accountID", account.AccountID)
	kvNamespaces, _, err := api.ListWorkersKVNamespaces(ctx, rc, cloudflare.ListWorkersKVNamespacesParams{})
	if err != nil {
		logger.Warn("CloudflareWorkers", "Failed to list KV namespaces (ignoring and trying to create)", "error", err)
	}

	var targetNamespaceID string
	for _, ns := range kvNamespaces {
		if strings.EqualFold(ns.Title, "KV") {
			targetNamespaceID = ns.ID
			logger.Info("CloudflareWorkers", "Found existing 'KV' namespace", "namespaceID", targetNamespaceID)
			break
		}
	}

	if targetNamespaceID == "" {
		logger.Info("CloudflareWorkers", "No existing 'KV' namespace found. Creating new namespace 'KV'...", "accountID", account.AccountID)
		createResp, err := api.CreateWorkersKVNamespace(ctx, rc, cloudflare.CreateWorkersKVNamespaceParams{
			Title: "KV",
		})
		if err != nil {
			return nil, fmt.Errorf("failed to create 'KV' namespace: %w", err)
		}
		targetNamespaceID = createResp.Result.ID
		logger.Info("CloudflareWorkers", "Created 'KV' namespace successfully", "namespaceID", targetNamespaceID)
	}

	// Detect if the script is an ES module
	isModule := false
	scriptContent := string(scriptBytes)
	if strings.Contains(scriptContent, "export default") || strings.Contains(scriptContent, "export {") || strings.Contains(scriptContent, "export const") {
		isModule = true
		logger.Info("CloudflareWorkers", "Detected ES module format for worker script", "scriptName", scriptName)
	} else {
		logger.Info("CloudflareWorkers", "Detected standard Service Worker format for worker script", "scriptName", scriptName)
	}

	// 5. Upload Worker Script to Cloudflare with KV Binding
	params := cloudflare.CreateWorkerParams{
		ScriptName:        scriptName,
		Script:            scriptContent,
		Module:            isModule,
		CompatibilityDate: "2024-01-01",
		Bindings: map[string]cloudflare.WorkerBinding{
			"KV": cloudflare.WorkerKvNamespaceBinding{
				NamespaceID: targetNamespaceID,
			},
		},
	}

	logger.Info("CloudflareWorkers", "Uploading worker script with KV binding to Cloudflare", "scriptName", scriptName, "accountID", account.AccountID, "isModule", isModule)
	_, err = api.UploadWorker(ctx, rc, params)
	if err != nil {
		return nil, fmt.Errorf("failed to upload worker script: %w", err)
	}

	// 6. Get Subdomain configured for this account to build default workers.dev link
	logger.Info("CloudflareWorkers", "Fetching account Workers subdomain", "accountID", account.AccountID)
	subdomainInfo, err := api.WorkersGetSubdomain(ctx, rc)
	if err != nil {
		return nil, fmt.Errorf("failed to get account workers subdomain: %w", err)
	}

	defaultURL := fmt.Sprintf("https://%s.%s.workers.dev", scriptName, subdomainInfo.Name)

	// 7. Enable Subdomain routing for this script via Cloudflare REST API (raw POST call)
	// Since cloudflare-go doesn't expose a subdomain toggle method, we use standard HTTP request.
	subdomainToggleURL := fmt.Sprintf("https://api.cloudflare.com/client/v4/accounts/%s/workers/scripts/%s/subdomain", account.AccountID, scriptName)
	payload := map[string]bool{"enabled": true}
	payloadBytes, err := json.Marshal(payload)
	if err != nil {
		return nil, fmt.Errorf("failed to marshal subdomain payload: %w", err)
	}

	req, err := http.NewRequestWithContext(ctx, "POST", subdomainToggleURL, bytes.NewBuffer(payloadBytes))
	if err != nil {
		return nil, fmt.Errorf("failed to create subdomain toggle request: %w", err)
	}
	req.Header.Set("Authorization", "Bearer "+account.AccessToken)
	req.Header.Set("Content-Type", "application/json")

	httpClient := &http.Client{Timeout: 15 * time.Second}
	resp, err := httpClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("failed to execute subdomain routing activation call: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		bodyBytes, _ := io.ReadAll(resp.Body)
		return nil, fmt.Errorf("failed to enable subdomain routing (status %d): %s", resp.StatusCode, string(bodyBytes))
	}
	logger.Info("CloudflareWorkers", "Subdomain routing enabled successfully", "defaultURL", defaultURL)

	// 8. Attach Custom Domain if provided
	if customDomain != "" && zoneID != "" {
		logger.Info("CloudflareWorkers", "Attaching custom domain to Worker script", "customDomain", customDomain, "zoneID", zoneID)
		attachParams := cloudflare.AttachWorkersDomainParams{
			ZoneID:    zoneID,
			Hostname:  customDomain,
			Service:   scriptName,
			Environment: "production",
		}
		_, err = api.AttachWorkersDomain(ctx, rc, attachParams)
		if err != nil {
			return nil, fmt.Errorf("failed to attach custom domain: %w", err)
		}
		logger.Info("CloudflareWorkers", "Custom domain attached successfully", "customDomain", customDomain)
	}

	// 9. Construct GORM Model record
	deployment := &models.CloudflareWorkerDeployment{
		AccountID:    account.AccountID,
		ScriptName:   scriptName,
		LocalPath:    localPath,
		DefaultURL:   defaultURL,
		CustomDomain: customDomain,
		ZoneID:       zoneID,
		HealthStatus: "unknown",
		Message:      "Deployment created successfully.",
	}

	// Save to DB
	if db.DB != nil {
		if err := db.DB.Create(deployment).Error; err != nil {
			return nil, fmt.Errorf("failed to save deployment record to database: %w", err)
		}
	}

	return deployment, nil
}

// CheckWorkerHealth probes the worker default/custom URLs and reads HTTP headers/bodies to assert execution state
func CheckWorkerHealth(ctx context.Context, deployment *models.CloudflareWorkerDeployment) (string, string, error) {
	// Resolve target URL to probe. Prefer custom domain if configured.
	targetURL := deployment.DefaultURL
	if deployment.CustomDomain != "" {
		targetURL = "https://" + deployment.CustomDomain
	}

	if targetURL == "" {
		return "error", "No routing URLs defined for this deployment", nil
	}

	logger.Info("CloudflareWorkers", "Checking health of worker deployment", "url", targetURL)

	req, err := http.NewRequestWithContext(ctx, "GET", targetURL, nil)
	if err != nil {
		return "error", fmt.Sprintf("Failed to construct probe request: %v", err), nil
	}
	// Avoid caching on probe
	req.Header.Set("Cache-Control", "no-cache")
	req.Header.Set("User-Agent", "CleverConnect-HealthProbe/1.0")

	// 10 second timeout for checking
	client := &http.Client{
		Timeout: 10 * time.Second,
		CheckRedirect: func(req *http.Request, via []*http.Request) error {
			// Do not follow redirects, inspect the direct edge response
			return http.ErrUseLastResponse
		},
	}

	resp, err := client.Do(req)
	if err != nil {
		return "unhealthy", fmt.Sprintf("Connection failed: %v", err), nil
	}
	defer resp.Body.Close()

	// Read first 2KB of response body to search for Cloudflare error flags
	bodyBytes, _ := io.ReadAll(io.LimitReader(resp.Body, 2048))
	bodyStr := string(bodyBytes)

	// Check if request reached Cloudflare edge (validated via CF-Ray header or Server header)
	cfRay := resp.Header.Get("CF-Ray")
	server := resp.Header.Get("Server")
	isCF := cfRay != "" || strings.Contains(strings.ToLower(server), "cloudflare")

	// Standard Cloudflare Worker exceptions return HTTP 530 / 1101 code.
	// Let's inspect the status code and text.
	if resp.StatusCode == 530 || strings.Contains(bodyStr, "1101") || strings.Contains(bodyStr, "Worker threw exception") {
		return "unhealthy", "Runtime error: Worker script threw an unhandled exception (Error 1101)", nil
	}

	if !isCF {
		return "unhealthy", "Routing check failed: Response did not pass through Cloudflare edge proxy (missing CF headers)", nil
	}

	// If it reached CF and did not crash with 1101, it is healthy.
	// Even 400 Bad Request, 404 Not Found, etc. are correct HTTP behaviors of a running proxy.
	msg := fmt.Sprintf("Worker active. Response Code: %d (Ray: %s)", resp.StatusCode, cfRay)
	return "healthy", msg, nil
}
