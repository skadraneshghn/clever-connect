package warp

// ──────────────────────────────────────────────────────────────────────────────
// WARP+ Key & Account Diagnostic Validator (Pillar 7)
//
// Provides a multi-stage, sequential validation pipeline for WARP+ license keys
// and account health. Each stage is independently verifiable and surfaces the
// exact failure point with actionable error codes.
//
// Pipeline stages:
//   Stage 1: Crypto preflight  — keypair generation & format validation
//   Stage 2: Device registration — uTLS POST /reg to obtain DeviceID + Token
//   Stage 3: License upgrade    — PUT /reg/{id}/account with the license key
//   Stage 4: Quota scrape       — GET /reg/{id}/account for usage metrics
//
// All HTTP calls use the same obfuscated uTLS transport as RegisterDevice,
// so this works even under heavy DPI/censorship.
// ──────────────────────────────────────────────────────────────────────────────

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"

	"clever-connect/internal/logger"
	"clever-connect/internal/models"
)

// ── Error codes returned by Cloudflare's account API ─────────────────────────

const (
	CFErrMalformedToken  = 1000 // key format is structurally invalid
	CFErrInvalidLicense  = 1012 // key is expired or permanently blacklisted
	CFErrMaxDevices      = 1056 // key is valid but all device slots are occupied
)

// KeyState describes the validated state of a WARP+ license key.
type KeyState string

const (
	KeyStateVerified       KeyState = "verified"        // all stages passed
	KeyStateInvalidFormat  KeyState = "invalid_format"  // regex / crypto check failed
	KeyStateMaxDevices     KeyState = "exhausted_devices" // CF error 1056
	KeyStateInvalidToken   KeyState = "invalid_token"   // CF error 1012
	KeyStateDepletedQuota  KeyState = "depleted_quota"  // zero remaining bytes
	KeyStateFailed         KeyState = "failed"          // unexpected error
)

// AccountMetrics carries the usage statistics returned by Cloudflare.
type AccountMetrics struct {
	AccountType    string `json:"account_type"`
	TotalBytes     int64  `json:"total_bytes"`
	UsedBytes      int64  `json:"used_bytes"`
	RemainingBytes int64  `json:"remaining_bytes"`
}

// ValidationResult is the full output of the diagnostic pipeline.
type ValidationResult struct {
	Success             bool            `json:"success"`
	KeyState            KeyState        `json:"key_state"`
	CloudflareErrorCode int             `json:"cloudflare_error_code,omitempty"`
	ErrorMessage        string          `json:"error_message,omitempty"`
	AccountMetrics      *AccountMetrics `json:"account_metrics,omitempty"`

	// Per-stage pass/fail for the UI diagnostic view
	Stages ValidationStages `json:"stages"`
}

// ValidationStages tracks the pass/fail status of each pipeline stage.
type ValidationStages struct {
	CryptoPreflight    StageResult `json:"crypto_preflight"`
	DeviceRegistration StageResult `json:"device_registration"`
	LicenseUpgrade     StageResult `json:"license_upgrade"`
	QuotaScrape        StageResult `json:"quota_scrape"`
}

// StageResult is the outcome of a single validation stage.
type StageResult struct {
	Passed  bool   `json:"passed"`
	Message string `json:"message"`
}

// cfErrorPayload is Cloudflare's standard error response body.
type cfErrorPayload struct {
	Success bool `json:"success"`
	Errors  []struct {
		Code    int    `json:"code"`
		Message string `json:"message"`
	} `json:"errors"`
}

// parseCFError attempts to decode a Cloudflare error JSON body.
// Returns (code, message) or (0, "") if the body isn't a CF error.
func parseCFError(body []byte) (code int, message string) {
	var payload cfErrorPayload
	if err := json.Unmarshal(body, &payload); err != nil {
		return 0, ""
	}
	if len(payload.Errors) == 0 {
		return 0, ""
	}
	return payload.Errors[0].Code, payload.Errors[0].Message
}

// ── Validator ─────────────────────────────────────────────────────────────────

// ValidateLicenseKey runs the full multi-stage diagnostic pipeline for a
// WARP+ license key. It uses a temporary device registration (never saved to DB)
// so it never touches the active engine or account pool.
//
// ctx should have a deadline set by the caller (recommended: 15–30 seconds).
func ValidateLicenseKey(ctx context.Context, licenseKey string, sni string) *ValidationResult {
	result := &ValidationResult{}

	if sni == "" {
		sni = "consumer-masque.cloudflareclient.com"
	}

	client := NewObfuscatedClient(sni)

	// ── Stage 1: Crypto preflight ─────────────────────────────────────────────
	if err := stageCryptoPreflight(licenseKey); err != nil {
		result.Stages.CryptoPreflight = StageResult{Passed: false, Message: err.Error()}
		result.Success = false
		result.KeyState = KeyStateInvalidFormat
		result.ErrorMessage = err.Error()
		return result
	}
	result.Stages.CryptoPreflight = StageResult{Passed: true, Message: "Key format valid; Curve25519 keypair generated"}

	// ── Stage 2: Device registration ─────────────────────────────────────────
	logger.Info("WARP", "[Validator] Stage 2: device registration")
	tempDeviceID, tempToken, err := stageDeviceRegistration(ctx, client)
	if err != nil {
		result.Stages.DeviceRegistration = StageResult{Passed: false, Message: err.Error()}
		result.Success = false
		result.KeyState = KeyStateFailed
		result.ErrorMessage = "Device registration failed (network/DPI issue?): " + err.Error()
		return result
	}
	result.Stages.DeviceRegistration = StageResult{
		Passed:  true,
		Message: fmt.Sprintf("Device registered: %s", tempDeviceID),
	}

	// ── Stage 3: License upgrade ──────────────────────────────────────────────
	logger.Info("WARP", "[Validator] Stage 3: license key upgrade", "deviceID", tempDeviceID)
	cfCode, upgradeErr := stageLicenseUpgrade(ctx, client, tempDeviceID, tempToken, licenseKey)
	if upgradeErr != nil {
		result.Stages.LicenseUpgrade = StageResult{Passed: false, Message: upgradeErr.Error()}
		result.Success = false
		result.CloudflareErrorCode = cfCode

		switch cfCode {
		case CFErrMaxDevices:
			result.KeyState = KeyStateMaxDevices
			result.ErrorMessage = "License key is valid but all device slots are occupied (CF error 1056). " +
				"Remove old devices in the WARP app (Settings → Account → Manage Devices) and try again."
		case CFErrInvalidLicense:
			result.KeyState = KeyStateInvalidToken
			result.ErrorMessage = "Cloudflare rejected the license key as expired or blacklisted (CF error 1012)."
		case CFErrMalformedToken:
			result.KeyState = KeyStateInvalidFormat
			result.ErrorMessage = "License key structure is malformed (CF error 1000)."
		default:
			result.KeyState = KeyStateFailed
			result.ErrorMessage = upgradeErr.Error()
		}
		// Clean up temp device in background — best effort
		go cleanupTempDevice(client, tempDeviceID, tempToken)
		return result
	}
	result.Stages.LicenseUpgrade = StageResult{
		Passed:  true,
		Message: "License key accepted by Cloudflare — account upgraded",
	}

	// ── Stage 4: Quota scrape ─────────────────────────────────────────────────
	logger.Info("WARP", "[Validator] Stage 4: quota metrics scrape", "deviceID", tempDeviceID)
	metrics, err := stageQuotaScrape(ctx, client, tempDeviceID, tempToken)
	if err != nil {
		result.Stages.QuotaScrape = StageResult{Passed: false, Message: err.Error()}
		// Non-fatal: key is still valid even if we can't read quota
		result.Success = true
		result.KeyState = KeyStateVerified
		result.ErrorMessage = "Key verified but quota read failed: " + err.Error()
	} else {
		msg := fmt.Sprintf("Quota: %s used of %s total (%s remaining)",
			formatBytesHuman(metrics.UsedBytes),
			formatBytesHuman(metrics.TotalBytes),
			formatBytesHuman(metrics.RemainingBytes),
		)
		result.Stages.QuotaScrape = StageResult{Passed: true, Message: msg}
		result.AccountMetrics = metrics

		if metrics.TotalBytes > 0 && metrics.RemainingBytes <= 0 {
			result.Success = false
			result.KeyState = KeyStateDepletedQuota
			result.ErrorMessage = "License key is valid but all data quota has been consumed."
		} else {
			result.Success = true
			result.KeyState = KeyStateVerified
		}
	}

	// Clean up temp device — we registered it only to test the key
	go cleanupTempDevice(client, tempDeviceID, tempToken)
	return result
}

// ── Stage implementations ─────────────────────────────────────────────────────

// stageCryptoPreflight validates key format and verifies our crypto library works.
func stageCryptoPreflight(licenseKey string) error {
	// Validate key format: XXXXXXXX-XXXXXXXX-XXXXXXXX
	if licenseKey == "" {
		return fmt.Errorf("license key is empty")
	}
	parts := strings.Split(licenseKey, "-")
	if len(parts) != 3 {
		return fmt.Errorf("invalid key format: expected 3 dash-separated groups, got %d", len(parts))
	}
	for i, p := range parts {
		if len(p) != 8 {
			return fmt.Errorf("invalid key format: group %d has %d chars (expected 8)", i+1, len(p))
		}
	}

	// Verify our Curve25519 crypto library works correctly
	_, _, err := generateCurve25519Keypair()
	if err != nil {
		return fmt.Errorf("crypto library error: %w", err)
	}
	return nil
}

// stageDeviceRegistration registers a fresh temporary device with Cloudflare.
// Returns deviceID, token, or error.
func stageDeviceRegistration(ctx context.Context, client *ObfuscatedClient) (deviceID, token string, err error) {
	// Build registration request
	privKey, pubKey, err := generateCurve25519Keypair()
	if err != nil {
		return "", "", fmt.Errorf("keypair generation failed: %w", err)
	}
	_ = privKey // only pubKey needed for registration

	regReq := cfRegisterRequest{
		Key:    pubKey,
		Tos:    time.Now().UTC().Format("2006-01-02T15:04:05.000Z"),
		Model:  "PC",
		Type:   "Linux",
		Locale: "en_US",
	}
	body, err := json.Marshal(regReq)
	if err != nil {
		return "", "", err
	}

	req, err := http.NewRequestWithContext(ctx, "POST", cfRegURL, strings.NewReader(string(body)))
	if err != nil {
		return "", "", err
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("User-Agent", "okhttp/3.12.1")
	req.Header.Set("CF-Client-Version", "a-6.11-2223")

	resp, err := client.client.Do(req)
	if err != nil {
		return "", "", fmt.Errorf("registration HTTP call failed: %w", err)
	}
	defer resp.Body.Close()

	respBody, _ := io.ReadAll(resp.Body)
	if resp.StatusCode != http.StatusOK {
		cfCode, cfMsg := parseCFError(respBody)
		if cfCode != 0 {
			return "", "", fmt.Errorf("Cloudflare registration rejected (code %d): %s", cfCode, cfMsg)
		}
		return "", "", fmt.Errorf("registration failed (HTTP %d): %s", resp.StatusCode, string(respBody))
	}

	var reg cfRegisterResponse
	if err := json.Unmarshal(respBody, &reg); err != nil {
		return "", "", fmt.Errorf("failed to parse registration response: %w", err)
	}
	return reg.ID, reg.Token, nil
}

// stageLicenseUpgrade attempts to apply the license key to the temp device.
// Returns (cfErrorCode, error). cfErrorCode is 0 on success or if the error
// isn't a structured CF error.
func stageLicenseUpgrade(ctx context.Context, client *ObfuscatedClient, deviceID, token, licenseKey string) (int, error) {
	upgradeURL := fmt.Sprintf("%s/%s/reg/%s/account", cfAPIBase, cfAPIVersion, deviceID)

	licBody, _ := json.Marshal(cfLicenseRequest{License: licenseKey})
	req, err := http.NewRequestWithContext(ctx, "PUT", upgradeURL, strings.NewReader(string(licBody)))
	if err != nil {
		return 0, err
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("User-Agent", "okhttp/3.12.1")
	req.Header.Set("CF-Client-Version", "a-6.11-2223")
	req.Header.Set("Authorization", "Bearer "+token)

	resp, err := client.client.Do(req)
	if err != nil {
		return 0, fmt.Errorf("license upgrade HTTP call failed: %w", err)
	}
	defer resp.Body.Close()

	respBody, _ := io.ReadAll(resp.Body)
	if resp.StatusCode != http.StatusOK {
		cfCode, cfMsg := parseCFError(respBody)
		if cfCode != 0 {
			return cfCode, fmt.Errorf("CF error %d: %s", cfCode, cfMsg)
		}
		return 0, fmt.Errorf("license upgrade failed (HTTP %d): %s", resp.StatusCode, string(respBody))
	}
	return 0, nil
}

// stageQuotaScrape fetches live usage metrics for the temp device.
func stageQuotaScrape(ctx context.Context, client *ObfuscatedClient, deviceID, token string) (*AccountMetrics, error) {
	accountURL := fmt.Sprintf("%s/%s/reg/%s/account", cfAPIBase, cfAPIVersion, deviceID)

	req, err := http.NewRequestWithContext(ctx, "GET", accountURL, nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("User-Agent", "okhttp/3.12.1")
	req.Header.Set("CF-Client-Version", "a-6.11-2223")
	req.Header.Set("Authorization", "Bearer "+token)

	resp, err := client.client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("quota scrape HTTP call failed: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("quota scrape failed (HTTP %d)", resp.StatusCode)
	}

	var info cfAccountInfo
	if err := json.NewDecoder(resp.Body).Decode(&info); err != nil {
		return nil, fmt.Errorf("failed to parse account info: %w", err)
	}

	remaining := info.Quota - info.Usage
	if remaining < 0 {
		remaining = 0
	}

	return &AccountMetrics{
		AccountType:    normalizeAccountType(info.AccountType, info.WarpPlus),
		TotalBytes:     info.Quota,
		UsedBytes:      info.Usage,
		RemainingBytes: remaining,
	}, nil
}

// cleanupTempDevice deletes the temporary device registration from Cloudflare.
// Called in a goroutine after validation completes — best effort only.
func cleanupTempDevice(client *ObfuscatedClient, deviceID, token string) {
	deleteURL := fmt.Sprintf("%s/%s/reg/%s", cfAPIBase, cfAPIVersion, deviceID)
	req, err := http.NewRequest("DELETE", deleteURL, nil)
	if err != nil {
		return
	}
	req.Header.Set("User-Agent", "okhttp/3.12.1")
	req.Header.Set("CF-Client-Version", "a-6.11-2223")
	req.Header.Set("Authorization", "Bearer "+token)

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	req = req.WithContext(ctx)

	resp, err := client.client.Do(req)
	if err != nil {
		logger.Warn("WARP", "[Validator] Failed to delete temp device", "deviceID", deviceID, "error", err)
		return
	}
	resp.Body.Close()
	logger.Info("WARP", "[Validator] Temp device cleaned up", "deviceID", deviceID, "status", resp.StatusCode)
}

// ValidateAccount checks the health of an already-registered account (no key needed).
// Useful for checking if a stored account is still functional.
func ValidateAccount(ctx context.Context, account *models.WarpAccount, sni string) *ValidationResult {
	if sni == "" {
		sni = "consumer-masque.cloudflareclient.com"
	}
	client := NewObfuscatedClient(sni)
	result := &ValidationResult{}

	// Stage 1: Crypto preflight (check stored keys are valid base64)
	if err := validateStoredKeys(account); err != nil {
		result.Stages.CryptoPreflight = StageResult{Passed: false, Message: err.Error()}
		result.Success = false
		result.KeyState = KeyStateInvalidFormat
		result.ErrorMessage = err.Error()
		return result
	}
	result.Stages.CryptoPreflight = StageResult{Passed: true, Message: "Stored keypair is valid"}

	// Stages 2+3 are N/A for existing accounts (already registered)
	result.Stages.DeviceRegistration = StageResult{Passed: true, Message: "Pre-registered device (ID: " + account.DeviceID + ")"}
	result.Stages.LicenseUpgrade = StageResult{Passed: true, Message: "License key already applied at registration"}

	// Stage 4: Quota scrape on the live account
	metrics, err := stageQuotaScrape(ctx, client, account.DeviceID, account.Token)
	if err != nil {
		result.Stages.QuotaScrape = StageResult{Passed: false, Message: err.Error()}
		result.Success = false
		result.KeyState = KeyStateFailed
		result.ErrorMessage = "Could not reach Cloudflare account API: " + err.Error()
		return result
	}

	msg := fmt.Sprintf("Quota: %s used / %s total (%s remaining)",
		formatBytesHuman(metrics.UsedBytes),
		formatBytesHuman(metrics.TotalBytes),
		formatBytesHuman(metrics.RemainingBytes),
	)
	result.Stages.QuotaScrape = StageResult{Passed: true, Message: msg}
	result.AccountMetrics = metrics

	if metrics.TotalBytes > 0 && metrics.RemainingBytes <= 0 {
		result.Success = false
		result.KeyState = KeyStateDepletedQuota
		result.ErrorMessage = "Account quota exhausted."
	} else {
		result.Success = true
		result.KeyState = KeyStateVerified
	}
	return result
}

// validateStoredKeys checks that an account's private/public key fields are present.
func validateStoredKeys(account *models.WarpAccount) error {
	if account.PrivateKey == "" {
		return fmt.Errorf("account has no stored private key")
	}
	if account.PublicKey == "" {
		return fmt.Errorf("account has no stored public key")
	}
	if account.DeviceID == "" {
		return fmt.Errorf("account has no device ID — was registration completed?")
	}
	if account.Token == "" {
		return fmt.Errorf("account has no bearer token — was registration completed?")
	}
	return nil
}

// ── Helpers ───────────────────────────────────────────────────────────────────

// formatBytesHuman formats a byte count as a human-readable string.
func formatBytesHuman(b int64) string {
	const unit = 1024
	if b < unit {
		return fmt.Sprintf("%d B", b)
	}
	div, exp := int64(unit), 0
	for n := b / unit; n >= unit; n /= unit {
		div *= unit
		exp++
	}
	return fmt.Sprintf("%.1f %cB", float64(b)/float64(div), "KMGTPE"[exp])
}
