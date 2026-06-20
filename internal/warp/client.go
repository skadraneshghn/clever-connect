package warp

import (
	"bytes"
	"context"
	"crypto/rand"
	"crypto/tls"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"io"
	"net"
	"net/http"
	"time"

	"clever-connect/internal/logger"
	"clever-connect/internal/models"

	utls "github.com/refraction-networking/utls"
	"golang.org/x/crypto/curve25519"
	"golang.org/x/net/http2"
)

// ──────────────────────────────────────────────────────────────────────────────
// Anti-Censorship Obfuscated API Client (Pillar 2)
//
// Uses uTLS to mimic browser ClientHello fingerprints when communicating with
// api.cloudflareclient.com, defeating deep packet inspection (DPI) systems
// that fingerprint standard Go TLS handshakes.
// ──────────────────────────────────────────────────────────────────────────────

const (
	cfAPIBase    = "https://api.cloudflareclient.com"
	cfAPIVersion = "v0a2158"
	cfRegURL     = cfAPIBase + "/" + cfAPIVersion + "/reg"
)

// ObfuscatedClient wraps an http.Client with a uTLS-based transport
// that mimics browser TLS fingerprints for anti-censorship purposes.
type ObfuscatedClient struct {
	client *http.Client
	sni    string
}

// NewObfuscatedClient creates an HTTP client backed by a uTLS h2 transport
// that mimics Chrome browser TLS fingerprints. The Cloudflare API only speaks
// HTTP/2, so we negotiate h2 in ALPN and use golang.org/x/net/http2 transport.
func NewObfuscatedClient(sni string) *ObfuscatedClient {
	if sni == "" {
		sni = "api.cloudflareclient.com"
	}

	// http2.Transport with a custom DialTLSContext that uses uTLS.
	// This properly negotiates h2 via ALPN in the TLS ClientHello.
	h2Transport := &http2.Transport{
		DialTLSContext: func(ctx context.Context, network, addr string, cfg *tls.Config) (net.Conn, error) {
			return dialUTLSH2(ctx, network, addr, sni)
		},
		AllowHTTP: false,
	}

	return &ObfuscatedClient{
		client: &http.Client{
			Transport: h2Transport,
			Timeout:   30 * time.Second,
		},
		sni: sni,
	}
}

// dialUTLSH2 creates a uTLS connection advertising h2 in ALPN, required for
// api.cloudflareclient.com which speaks HTTP/2 only. The h2 negotiation
// happens inside the TLS handshake via ALPN, and golang.org/x/net/http2
// uses the resulting conn to multiplex HTTP/2 streams.
func dialUTLSH2(ctx context.Context, network, addr, sni string) (net.Conn, error) {
	dialer := &net.Dialer{
		Timeout: 15 * time.Second,
	}

	rawConn, err := dialer.DialContext(ctx, network, addr)
	if err != nil {
		return nil, fmt.Errorf("warp: failed to dial %s: %w", addr, err)
	}

	// Extract the actual hostname for SNI (use provided SNI override)
	host, _, err := net.SplitHostPort(addr)
	if err != nil {
		host = addr
	}
	serverName := sni
	if serverName == "" {
		serverName = host
	}

	// Build uTLS connection with Chrome Auto fingerprint
	uConn := utls.UClient(rawConn, &utls.Config{
		ServerName:         serverName,
		InsecureSkipVerify: false,
		MinVersion:         utls.VersionTLS12,
	}, utls.HelloChrome_Auto)

	// Apply custom spec with h2 ALPN — this is the critical fix.
	// The previous code advertised only "http/1.1" which caused the server
	// to respond with an HTTP/2 SETTINGS frame that the h1 parser cannot read.
	if err := applyClientHelloH2(uConn, serverName); err != nil {
		logger.Warn("WARP", "Custom ClientHello failed, falling back to Chrome_Auto+h2", "error", err)
		// Patch ALPN on the bare Chrome_Auto spec as fallback
		uConn = utls.UClient(rawConn, &utls.Config{
			ServerName: serverName,
			NextProtos: []string{"h2", "http/1.1"},
		}, utls.HelloChrome_Auto)
	}

	if err := uConn.HandshakeContext(ctx); err != nil {
		rawConn.Close()
		return nil, fmt.Errorf("warp: uTLS h2 handshake failed: %w", err)
	}

	negotiated := uConn.ConnectionState().NegotiatedProtocol
	if negotiated != "h2" {
		logger.Warn("WARP", "Expected h2 ALPN, got", "protocol", negotiated)
	}

	return uConn, nil
}

// applyClientHelloH2 injects a Chrome-like ClientHello spec with:
//   - "h2" and "http/1.1" in ALPN (critical for HTTP/2 negotiation)
//   - BoringSSL-style padding to defeat packet-length DPI signatures
func applyClientHelloH2(conn *utls.UConn, serverName string) error {
	spec := utls.ClientHelloSpec{
		TLSVersMax: utls.VersionTLS13,
		TLSVersMin: utls.VersionTLS12,
		CipherSuites: []uint16{
			utls.GREASE_PLACEHOLDER,
			utls.TLS_AES_128_GCM_SHA256,
			utls.TLS_AES_256_GCM_SHA384,
			utls.TLS_CHACHA20_POLY1305_SHA256,
			utls.TLS_ECDHE_ECDSA_WITH_AES_128_GCM_SHA256,
			utls.TLS_ECDHE_RSA_WITH_AES_128_GCM_SHA256,
			utls.TLS_ECDHE_ECDSA_WITH_AES_256_GCM_SHA384,
			utls.TLS_ECDHE_RSA_WITH_AES_256_GCM_SHA384,
			utls.TLS_ECDHE_ECDSA_WITH_CHACHA20_POLY1305,
			utls.TLS_ECDHE_RSA_WITH_CHACHA20_POLY1305,
		},
		CompressionMethods: []byte{0x00},
		Extensions: []utls.TLSExtension{
			&utls.UtlsGREASEExtension{},
			&utls.SNIExtension{ServerName: serverName},
			&utls.UtlsExtendedMasterSecretExtension{},
			&utls.RenegotiationInfoExtension{Renegotiation: utls.RenegotiateOnceAsClient},
			&utls.SupportedCurvesExtension{Curves: []utls.CurveID{
				utls.GREASE_PLACEHOLDER,
				utls.X25519,
				utls.CurveP256,
				utls.CurveP384,
			}},
			&utls.SupportedPointsExtension{SupportedPoints: []byte{0x00}},
			&utls.SessionTicketExtension{},
			// ▼ THE FIX: advertise h2 first so the server negotiates HTTP/2
			&utls.ALPNExtension{AlpnProtocols: []string{"h2", "http/1.1"}},
			&utls.StatusRequestExtension{},
			&utls.SignatureAlgorithmsExtension{SupportedSignatureAlgorithms: []utls.SignatureScheme{
				utls.ECDSAWithP256AndSHA256,
				utls.PSSWithSHA256,
				utls.PKCS1WithSHA256,
				utls.ECDSAWithP384AndSHA384,
				utls.PSSWithSHA384,
				utls.PKCS1WithSHA384,
				utls.PSSWithSHA512,
				utls.PKCS1WithSHA512,
			}},
			&utls.SCTExtension{},
			&utls.KeyShareExtension{KeyShares: []utls.KeyShare{
				{Group: utls.GREASE_PLACEHOLDER, Data: []byte{0}},
				{Group: utls.X25519},
			}},
			&utls.PSKKeyExchangeModesExtension{Modes: []uint8{utls.PskModeDHE}},
			&utls.SupportedVersionsExtension{Versions: []uint16{
				utls.GREASE_PLACEHOLDER,
				utls.VersionTLS13,
				utls.VersionTLS12,
			}},
			&utls.UtlsPaddingExtension{GetPaddingLen: utls.BoringPaddingStyle},
		},
	}

	return conn.ApplyPreset(&spec)
}

// ──────────────────────────────────────────────────────────────────────────────
// Cloudflare WARP Device Registration
// ──────────────────────────────────────────────────────────────────────────────

// cfRegisterRequest is the POST body for WARP device registration.
type cfRegisterRequest struct {
	Key       string `json:"key"`
	InstallID string `json:"install_id,omitempty"`
	FcmToken  string `json:"fcm_token,omitempty"`
	Tos       string `json:"tos"`
	Model     string `json:"model"`
	Type      string `json:"type"`
	Locale    string `json:"locale"`
}

// cfRegisterResponse is the parsed response from CF registration.
type cfRegisterResponse struct {
	ID      string `json:"id"`
	Token   string `json:"token"`
	Account struct {
		ID                string `json:"id"`
		AccountType       string `json:"account_type"`
		License           string `json:"license"`
		PremiumData       int64  `json:"premium_data"`
		WarpPlusEnabled   bool   `json:"warp_plus"`
		Quota             int64  `json:"quota"`
		Usage             int64  `json:"usage"`
		ReferralCount     int    `json:"referral_count"`
		ReferralRenewable bool   `json:"referral_renewable"`
	} `json:"account"`
	Config struct {
		ClientID string `json:"client_id"`
		Peers    []struct {
			PublicKey string `json:"public_key"`
			Endpoint  struct {
				V4   string `json:"v4"`
				V6   string `json:"v6"`
				Host string `json:"host"`
			} `json:"endpoint"`
		} `json:"peers"`
		Interface struct {
			Addresses struct {
				V4 string `json:"v4"`
				V6 string `json:"v6"`
			} `json:"addresses"`
		} `json:"interface"`
	} `json:"config"`
}

// cfLicenseRequest is the PUT body for upgrading to WARP+.
type cfLicenseRequest struct {
	License string `json:"license"`
}

// RegisterDevice performs a full WARP device registration with Cloudflare.
// If licenseKey is provided, it also upgrades the account to WARP+.
//
// Steps:
//  1. Generate Curve25519 keypair
//  2. POST to /reg with browser-like uTLS handshake
//  3. If licenseKey given, PUT to /reg/{device_id}/account
//  4. Return populated WarpAccount
func (c *ObfuscatedClient) RegisterDevice(licenseKey string) (*models.WarpAccount, error) {
	logger.Info("WARP", "Starting device registration with Cloudflare API")

	// Step 1: Generate Curve25519 keypair
	privateKey, publicKey, err := generateCurve25519Keypair()
	if err != nil {
		return nil, fmt.Errorf("warp: failed to generate keypair: %w", err)
	}

	logger.Info("WARP", "Generated Curve25519 keypair", "publicKey", publicKey)

	// Step 2: Register device
	regReq := cfRegisterRequest{
		Key:    publicKey,
		Tos:    time.Now().UTC().Format("2006-01-02T15:04:05.000Z"),
		Model:  "PC",
		Type:   "Linux",
		Locale: "en_US",
	}

	regBody, err := json.Marshal(regReq)
	if err != nil {
		return nil, fmt.Errorf("warp: failed to marshal registration request: %w", err)
	}

	req, err := http.NewRequest("POST", cfRegURL, bytes.NewReader(regBody))
	if err != nil {
		return nil, fmt.Errorf("warp: failed to create registration request: %w", err)
	}

	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("User-Agent", "okhttp/3.12.1")
	req.Header.Set("CF-Client-Version", "a-6.11-2223")

	resp, err := c.client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("warp: registration API call failed: %w", err)
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("warp: failed to read registration response: %w", err)
	}

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("warp: registration failed with status %d: %s", resp.StatusCode, string(body))
	}

	var regResp cfRegisterResponse
	if err := json.Unmarshal(body, &regResp); err != nil {
		return nil, fmt.Errorf("warp: failed to parse registration response: %w", err)
	}

	logger.Info("WARP", "Device registered successfully",
		"deviceID", regResp.ID,
		"accountType", regResp.Account.AccountType,
	)

	// Build account model
	// Extract Cloudflare's server peer public key
	peerPubKey := ""
	if len(regResp.Config.Peers) > 0 {
		peerPubKey = regResp.Config.Peers[0].PublicKey
	}

	account := &models.WarpAccount{
		DeviceID:      regResp.ID,
		Token:         regResp.Token,
		PrivateKey:    privateKey,
		PublicKey:     publicKey,
		PeerPublicKey: peerPubKey,
		ClientID:      regResp.Config.ClientID,
		AssignedIPv4:  regResp.Config.Interface.Addresses.V4,
		AssignedIPv6:  regResp.Config.Interface.Addresses.V6,
		AccountType:   regResp.Account.AccountType,
		TotalQuota:    regResp.Account.Quota,
		UsedQuota:     regResp.Account.Usage,
		IsFunctional:  true,
	}

	// Step 3: Upgrade to WARP+ if license key provided.
	// If a key was given and the upgrade fails, we return an error to the caller
	// rather than silently registering a free account — the user intentionally
	// provided a key, so a failure is always surfaced.
	if licenseKey != "" {
		account.LicenseKey = licenseKey

		upgraded, err := c.upgradeLicense(regResp.ID, regResp.Token, licenseKey)
		if err != nil {
			logger.Error("WARP", "License upgrade failed", "error", err)
			return nil, fmt.Errorf("warp: license key rejected by Cloudflare: %w", err)
		}

		// Map CF account type → internal type.
		// CF returns "unlimited" for WARP+, not "warp_plus".
		account.AccountType = normalizeAccountType(upgraded.AccountType, upgraded.WarpPlus)
		account.TotalQuota = upgraded.Quota
		account.UsedQuota = upgraded.Usage

		logger.Info("WARP", "Account upgraded to WARP+",
			"deviceID", regResp.ID,
			"cfAccountType", upgraded.AccountType,
			"warpPlus", upgraded.WarpPlus,
		)
	}

	return account, nil
}

// cfAccountInfo is the full account object returned by the /account endpoint.
// CF returns account_type = "unlimited" for WARP+ accounts and warp_plus = true.
type cfAccountInfo struct {
	ID          string `json:"id"`
	AccountType string `json:"account_type"`
	License     string `json:"license"`
	PremiumData int64  `json:"premium_data"`
	WarpPlus    bool   `json:"warp_plus"`
	Quota       int64  `json:"quota"`
	Usage       int64  `json:"usage"`
}

// normalizeAccountType converts Cloudflare's account_type strings and
// the warp_plus boolean into a consistent internal type label.
// CF returns "unlimited" for paid accounts, not "warp_plus".
func normalizeAccountType(cfType string, warpPlus bool) string {
	if warpPlus || cfType == "unlimited" || cfType == "premium" {
		return "warp_plus"
	}
	if cfType != "" {
		return cfType
	}
	return "free"
}

// upgradeLicense sends a PUT to /reg/{deviceID}/account with the license key
// and returns the updated account info from Cloudflare's response body.
func (c *ObfuscatedClient) upgradeLicense(deviceID, token, licenseKey string) (*cfAccountInfo, error) {
	url := fmt.Sprintf("%s/%s/reg/%s/account", cfAPIBase, cfAPIVersion, deviceID)

	licReq := cfLicenseRequest{License: licenseKey}
	reqBody, err := json.Marshal(licReq)
	if err != nil {
		return nil, err
	}

	req, err := http.NewRequest("PUT", url, bytes.NewReader(reqBody))
	if err != nil {
		return nil, err
	}

	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("User-Agent", "okhttp/3.12.1")
	req.Header.Set("CF-Client-Version", "a-6.11-2223")
	req.Header.Set("Authorization", "Bearer "+token)

	resp, err := c.client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("license upgrade API call failed: %w", err)
	}
	defer resp.Body.Close()

	respBody, _ := io.ReadAll(resp.Body)

	if resp.StatusCode != http.StatusOK {
		// Parse Cloudflare's structured error response: {"code":N,"message":"..."}
		var cfErr struct {
			Code    int    `json:"code"`
			Message string `json:"message"`
		}
		if jsonErr := json.Unmarshal(respBody, &cfErr); jsonErr == nil && cfErr.Code != 0 {
			switch cfErr.Code {
			case 1056:
				// "Too many connected devices" — license key is already bound to the
				// maximum number of devices in Cloudflare's system.
				// The user must remove devices via the official WARP app (Settings →
				// Account → Manage devices) before adding another.
				return nil, fmt.Errorf(
					"license key rejected: too many devices registered (CF error 1056). "+
						"Remove old devices in the WARP app (Settings → Account → Manage Devices) and try again",
				)
			default:
				return nil, fmt.Errorf("license upgrade rejected by Cloudflare (code %d): %s", cfErr.Code, cfErr.Message)
			}
		}
		return nil, fmt.Errorf("license upgrade failed (HTTP %d): %s", resp.StatusCode, string(respBody))
	}

	// Parse the account info directly from the PUT response body.
	// This avoids a redundant GET request and gives us the fresh state.
	var info cfAccountInfo
	if err := json.Unmarshal(respBody, &info); err != nil {
		return nil, fmt.Errorf("failed to parse upgrade response: %w", err)
	}

	logger.Info("WARP", "License upgrade response parsed",
		"account_type", info.AccountType,
		"warp_plus", info.WarpPlus,
		"quota", info.Quota,
	)

	return &info, nil
}

// getAccountInfo fetches current account information for a registered device.
func (c *ObfuscatedClient) getAccountInfo(deviceID, token string) (*cfAccountInfo, error) {
	url := fmt.Sprintf("%s/%s/reg/%s/account", cfAPIBase, cfAPIVersion, deviceID)

	req, err := http.NewRequest("GET", url, nil)
	if err != nil {
		return nil, err
	}

	req.Header.Set("User-Agent", "okhttp/3.12.1")
	req.Header.Set("CF-Client-Version", "a-6.11-2223")
	req.Header.Set("Authorization", "Bearer "+token)

	resp, err := c.client.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("account info request failed with status %d", resp.StatusCode)
	}

	var info cfAccountInfo
	if err := json.NewDecoder(resp.Body).Decode(&info); err != nil {
		return nil, err
	}

	return &info, nil
}

type cfKeyUpdateRequest struct {
	Key        string `json:"key"`
	KeyType    string `json:"key_type"`
	TunnelType string `json:"tunnel_type"`
}

// updateRegistrationKey updates the registered public key and tunnel type on Cloudflare's server.
func (c *ObfuscatedClient) updateRegistrationKey(deviceID, token, pubKeyB64, keyType, tunnelType string) error {
	url := fmt.Sprintf("%s/%s/reg/%s", cfAPIBase, cfAPIVersion, deviceID)

	updateReq := cfKeyUpdateRequest{
		Key:        pubKeyB64,
		KeyType:    keyType,
		TunnelType: tunnelType,
	}
	reqBody, err := json.Marshal(updateReq)
	if err != nil {
		return err
	}

	req, err := http.NewRequest("PATCH", url, bytes.NewReader(reqBody))
	if err != nil {
		return err
	}

	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("User-Agent", "okhttp/3.12.1")
	req.Header.Set("CF-Client-Version", "a-6.11-2223")
	req.Header.Set("Authorization", "Bearer "+token)

	resp, err := c.client.Do(req)
	if err != nil {
		return fmt.Errorf("registration update API call failed: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		respBody, _ := io.ReadAll(resp.Body)
		return fmt.Errorf("registration update failed (HTTP %d): %s", resp.StatusCode, string(respBody))
	}

	return nil
}

// ──────────────────────────────────────────────────────────────────────────────
// Cryptographic Key Generation
// ──────────────────────────────────────────────────────────────────────────────

// generateCurve25519Keypair generates a new Curve25519 keypair for WireGuard.
// Returns base64-encoded private and public keys.
func generateCurve25519Keypair() (privateKeyB64, publicKeyB64 string, err error) {
	// Generate 32 random bytes for the private key
	var privateKey [32]byte
	if _, err := rand.Read(privateKey[:]); err != nil {
		return "", "", fmt.Errorf("failed to generate random key material: %w", err)
	}

	// Clamp the private key per Curve25519 requirements
	privateKey[0] &= 248
	privateKey[31] &= 127
	privateKey[31] |= 64

	// Derive the public key
	publicKey, err := curve25519.X25519(privateKey[:], curve25519.Basepoint)
	if err != nil {
		return "", "", fmt.Errorf("failed to derive public key: %w", err)
	}

	return base64.StdEncoding.EncodeToString(privateKey[:]),
		base64.StdEncoding.EncodeToString(publicKey),
		nil
}
