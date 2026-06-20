package trusttunnel

import (
	"bufio"
	"encoding/base64"
	"fmt"
	"io"
	"net/url"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"sync"
	"syscall"
	"time"

	"clever-connect/internal/db"
	"clever-connect/internal/logger"
	"clever-connect/internal/models"
)

var (
	cmdInstance  *exec.Cmd
	mu          sync.Mutex
	expectedRun bool          // whether the engine is expected to be running
	stopChan    chan struct{} // to signal supervisor loop to stop
	isServerMode bool
	restartDelay time.Duration
)

// getBinPath resolves the trusttunnel binary path alongside the main clever-connect binary.
func getBinPath(mode string) string {
	binName := "trusttunnel_client"
	if mode == "server" {
		binName = "trusttunnel_endpoint"
	}

	// 1. Try relative to the executable's directory
	if exe, err := os.Executable(); err == nil {
		path := filepath.Join(filepath.Dir(exe), binName)
		if _, err := os.Stat(path); err == nil {
			if absPath, err := filepath.Abs(path); err == nil {
				return absPath
			}
			return path
		}
	}

	// 2. Fall back to current working directory (e.g., bin/trusttunnel_client)
	path := filepath.Join("bin", binName)
	if _, err := os.Stat(path); err == nil {
		if absPath, err := filepath.Abs(path); err == nil {
			return absPath
		}
		return path
	}

	// 3. Fallback to just the binary name in case it's in the PATH
	return binName
}

// getConfigDir returns a writable temp directory for TrustTunnel TOML configs.
func getConfigDir() string {
	return filepath.Join(os.TempDir(), "clever-connect-data", "trusttunnel")
}

// generateServerTOML writes the TOML configuration files for the TrustTunnel endpoint.
func generateServerTOML(cfg *models.TrustTunnelConfig) error {
	dir := getConfigDir()
	if err := os.MkdirAll(dir, 0755); err != nil {
		return fmt.Errorf("failed to create trusttunnel config dir: %w", err)
	}

	// Format listen_protocols configuration based on forced transport
	var listenProtocols string
	switch cfg.ForcedTransport {
	case "http1":
		listenProtocols = `[listen_protocols.http1]
upload_buffer_size = 32768`
	case "quic":
		listenProtocols = `[listen_protocols.quic]
recv_udp_payload_size = 1350
send_udp_payload_size = 1350`
	default: // http2
		listenProtocols = fmt.Sprintf(`[listen_protocols.http2]
initial_connection_window_size = %d
initial_stream_window_size = %d`, cfg.H2InitialConnWindowSize, cfg.H2InitialStreamWindowSize)
	}

	// vpn.toml — Core server settings (root properties in Settings struct)
	vpnTOML := fmt.Sprintf(`listen_address = "%s"
tls_handshake_timeout_secs = %d
auth_failure_status_code = %d
credentials_file = "credentials.toml"
rules_file = "rules.toml"
ipv6_available = true
allow_private_network_connections = false

[listen_protocols]
%s
`,
		cfg.ListenAddress,
		cfg.TlsHandshakeTimeoutSecs,
		cfg.AuthFailureStatusCode,
		listenProtocols,
	)

	if err := os.WriteFile(filepath.Join(dir, "vpn.toml"), []byte(vpnTOML), 0644); err != nil {
		return fmt.Errorf("failed to write vpn.toml: %w", err)
	}

	// hosts.toml — TLS certificate configuration (uses main_hosts array)
	hostsTOML := ""
	if cfg.ServerHostname != "" {
		hostsTOML = fmt.Sprintf(`[[main_hosts]]
hostname = "%s"
`, cfg.ServerHostname)
		if cfg.TlsCertPath != "" && cfg.TlsKeyPath != "" {
			hostsTOML += fmt.Sprintf(`cert_chain_path = "%s"
private_key_path = "%s"
`, cfg.TlsCertPath, cfg.TlsKeyPath)
		}
	}
	if err := os.WriteFile(filepath.Join(dir, "hosts.toml"), []byte(hostsTOML), 0644); err != nil {
		return fmt.Errorf("failed to write hosts.toml: %w", err)
	}

	// credentials.toml — User authentication (uses client array)
	var users []models.TrustTunnelUser
	db.DB.Where("is_active = ?", true).Find(&users)

	credsTOML := ""
	for _, u := range users {
		credsTOML += fmt.Sprintf(`[[client]]
username = "%s"
password = "%s"

`, u.Username, u.Password)
	}
	if err := os.WriteFile(filepath.Join(dir, "credentials.toml"), []byte(credsTOML), 0644); err != nil {
		return fmt.Errorf("failed to write credentials.toml: %w", err)
	}

	// rules.toml — Firewall / bypass rules (uses rule array)
	rulesToml := ""
	if cfg.ClientRandomPrefix != "" {
		rulesToml += fmt.Sprintf(`[[rule]]
client_random_prefix = "%s"
action = "allow"

`, cfg.ClientRandomPrefix)
	}

	var rules []models.TrustTunnelFirewallRule
	db.DB.Find(&rules)

	for _, r := range rules {
		action := "allow"
		if r.BypassStrategy == "deny" {
			action = "deny"
		}
		rulesToml += fmt.Sprintf(`[[rule]]
cidr = "%s"
action = "%s"

`, r.TargetCIDR, action)
	}
	if err := os.WriteFile(filepath.Join(dir, "rules.toml"), []byte(rulesToml), 0644); err != nil {
		return fmt.Errorf("failed to write rules.toml: %w", err)
	}

	return nil
}

// generateClientTOML writes the TOML configuration files for the TrustTunnel client.
func generateClientTOML(cfg *models.TrustTunnelConfig) error {
	dir := getConfigDir()
	if err := os.MkdirAll(dir, 0755); err != nil {
		return fmt.Errorf("failed to create trusttunnel config dir: %w", err)
	}

	clientTOML := fmt.Sprintf(`[client]
connect = "%s"
socks5_port = %d
http_port = %d
forced_transport = "%s"
kill_switch = %t

[obfuscation]
client_random_prefix = "%s"

[http2]
h2_initial_stream_window_size = %d
h2_initial_connection_window_size = %d
`,
		cfg.ConnectAddress,
		cfg.Socks5Port,
		cfg.HttpPort,
		cfg.ForcedTransport,
		cfg.KillSwitchEnabled,
		cfg.ClientRandomPrefix,
		cfg.H2InitialStreamWindowSize,
		cfg.H2InitialConnWindowSize,
	)

	if err := os.WriteFile(filepath.Join(dir, "client.toml"), []byte(clientTOML), 0644); err != nil {
		return fmt.Errorf("failed to write client.toml: %w", err)
	}

	// rules.toml — Split-tunneling rules
	var rules []models.TrustTunnelFirewallRule
	db.DB.Find(&rules)

	rulesToml := ""
	for _, r := range rules {
		rulesToml += fmt.Sprintf(`[[rules]]
target_cidr = "%s"
bypass_strategy = "%s"

`, r.TargetCIDR, r.BypassStrategy)
	}
	if err := os.WriteFile(filepath.Join(dir, "rules.toml"), []byte(rulesToml), 0644); err != nil {
		return fmt.Errorf("failed to write rules.toml: %w", err)
	}

	return nil
}

// StartServerEngine launches the TrustTunnel endpoint process with server configurations.
func StartServerEngine(cfg *models.TrustTunnelConfig) error {
	mu.Lock()
	defer mu.Unlock()

	if err := StopEngineLocked(); err != nil {
		return err
	}

	if err := startServerEngineLocked(cfg); err != nil {
		return err
	}

	expectedRun = true
	isServerMode = true
	restartDelay = 1 * time.Second
	stopChan = make(chan struct{})
	startSupervisor()

	return nil
}

// StartClientEngine launches the TrustTunnel client process.
func StartClientEngine(cfg *models.TrustTunnelConfig) error {
	mu.Lock()
	defer mu.Unlock()

	if err := StopEngineLocked(); err != nil {
		return err
	}

	if err := startClientEngineLocked(cfg); err != nil {
		return err
	}

	expectedRun = true
	isServerMode = false
	restartDelay = 1 * time.Second
	stopChan = make(chan struct{})
	startSupervisor()

	return nil
}

func startServerEngineLocked(cfg *models.TrustTunnelConfig) error {
	binPath := getBinPath("server")
	if _, err := os.Stat(binPath); err != nil {
		return fmt.Errorf("trusttunnel_endpoint binary not found at %s: %w", binPath, err)
	}

	if err := generateServerTOML(cfg); err != nil {
		return fmt.Errorf("failed to generate server TOML configs: %w", err)
	}

	dir := getConfigDir()
	vpnPath := filepath.Join(dir, "vpn.toml")
	hostsPath := filepath.Join(dir, "hosts.toml")

	logger.Info("TrustTunnel", "Starting Endpoint Process",
		"listen", cfg.ListenAddress,
		"transport", cfg.ForcedTransport,
		"probe_mask", cfg.AuthFailureStatusCode,
	)

	cmdInstance = exec.Command(binPath, vpnPath, hostsPath)
	cmdInstance.Dir = dir

	return startAndPipeProcess("TrustTunnelEndpoint")
}

func startClientEngineLocked(cfg *models.TrustTunnelConfig) error {
	binPath := getBinPath("client")
	if _, err := os.Stat(binPath); err != nil {
		return fmt.Errorf("trusttunnel_client binary not found at %s: %w", binPath, err)
	}

	if err := generateClientTOML(cfg); err != nil {
		return fmt.Errorf("failed to generate client TOML configs: %w", err)
	}

	dir := getConfigDir()
	clientPath := filepath.Join(dir, "client.toml")

	logger.Info("TrustTunnel", "Starting Client Process",
		"connect", cfg.ConnectAddress,
		"socks5_port", cfg.Socks5Port,
		"http_port", cfg.HttpPort,
		"transport", cfg.ForcedTransport,
		"kill_switch", cfg.KillSwitchEnabled,
	)

	cmdInstance = exec.Command(binPath, "--config", clientPath)
	cmdInstance.Dir = dir

	return startAndPipeProcess("TrustTunnelClient")
}

// startAndPipeProcess starts the exec.Cmd and pipes stdout/stderr to the logger.
func startAndPipeProcess(tag string) error {
	stdoutPipe, err := cmdInstance.StdoutPipe()
	if err != nil {
		return fmt.Errorf("failed to get stdout pipe: %w", err)
	}
	stderrPipe, err := cmdInstance.StderrPipe()
	if err != nil {
		return fmt.Errorf("failed to get stderr pipe: %w", err)
	}

	if err := cmdInstance.Start(); err != nil {
		cmdInstance = nil
		return fmt.Errorf("failed to start %s process: %w", tag, err)
	}

	// Read stdout in background
	go func(pipe io.ReadCloser) {
		defer pipe.Close()
		scanner := bufio.NewScanner(pipe)
		for scanner.Scan() {
			logger.Info(tag, scanner.Text())
		}
	}(stdoutPipe)

	// Read stderr in background
	go func(pipe io.ReadCloser) {
		defer pipe.Close()
		scanner := bufio.NewScanner(pipe)
		for scanner.Scan() {
			logger.Error(tag, scanner.Text())
		}
	}(stderrPipe)

	// Reap process and log when it exits
	go func(cmd *exec.Cmd) {
		err := cmd.Wait()
		logger.Warn("TrustTunnel", "Subprocess exited", "error", err)
	}(cmdInstance)

	return nil
}

// StopEngine gracefully shuts down the active TrustTunnel process.
func StopEngine() {
	mu.Lock()
	defer mu.Unlock()
	_ = StopEngineLocked()
}

// StopEngineLocked stops the engine (caller must hold mu).
func StopEngineLocked() error {
	expectedRun = false
	if stopChan != nil {
		close(stopChan)
		stopChan = nil
	}
	return stopEngineLockedOnly()
}

func stopEngineLockedOnly() error {
	if cmdInstance != nil {
		logger.Info("TrustTunnel", "Terminating active TrustTunnel process")
		if cmdInstance.Process != nil {
			// Try graceful SIGTERM first
			if err := cmdInstance.Process.Signal(syscall.SIGTERM); err != nil {
				// Fallback to SIGKILL
				if err := cmdInstance.Process.Kill(); err != nil {
					logger.Error("TrustTunnel", "Failed to kill process", "error", err)
				}
			}
		}
		_ = cmdInstance.Wait()
		cmdInstance = nil
	}
	return nil
}

// IsRunning returns true if the engine process is active.
func IsRunning() bool {
	mu.Lock()
	defer mu.Unlock()
	if cmdInstance == nil || cmdInstance.Process == nil {
		return false
	}
	if cmdInstance.ProcessState != nil && cmdInstance.ProcessState.Exited() {
		return false
	}
	err := cmdInstance.Process.Signal(syscall.Signal(0))
	return err == nil
}

// startSupervisor spawns the background monitoring loop (must hold lock to setup/initialize variables).
func startSupervisor() {
	go func() {
		for {
			mu.Lock()
			if !expectedRun {
				mu.Unlock()
				return
			}

			// Check liveness
			isAlive := false
			if cmdInstance != nil && cmdInstance.Process != nil {
				if cmdInstance.ProcessState != nil && cmdInstance.ProcessState.Exited() {
					isAlive = false
				} else {
					err := cmdInstance.Process.Signal(syscall.Signal(0))
					isAlive = (err == nil)
				}
			}
			curStopChan := stopChan
			mu.Unlock()

			if !isAlive {
				logger.Warn("TrustTunnel", "Subprocess is not running. Initiating auto-restart.")

				mu.Lock()
				if restartDelay == 0 {
					restartDelay = 1 * time.Second
				} else {
					restartDelay *= 2
					if restartDelay > 30*time.Second {
						restartDelay = 30 * time.Second
					}
				}
				delay := restartDelay
				mu.Unlock()

				select {
				case <-curStopChan:
					return
				case <-time.After(delay):
				}

				mu.Lock()
				stillExpected := expectedRun
				mu.Unlock()

				if !stillExpected {
					return
				}

				if err := restartEngineFromDB(); err != nil {
					logger.Error("TrustTunnel", "Failed to auto-restart engine", "error", err)
				} else {
					logger.Info("TrustTunnel", "Subprocess auto-restarted successfully")
					mu.Lock()
					restartDelay = 1 * time.Second
					mu.Unlock()
				}
			} else {
				select {
				case <-curStopChan:
					return
				case <-time.After(3 * time.Second):
				}
			}
		}
	}()
}

// restartEngineFromDB loads settings from the database and restarts the process.
func restartEngineFromDB() error {
	mu.Lock()
	defer mu.Unlock()

	if !expectedRun {
		return nil
	}

	var cfg models.TrustTunnelConfig
	if err := db.DB.First(&cfg).Error; err != nil {
		return fmt.Errorf("failed to load TrustTunnel config from DB: %w", err)
	}
	if !cfg.IsActive {
		return fmt.Errorf("TrustTunnel config is no longer active in DB")
	}

	_ = stopEngineLockedOnly()

	if isServerMode {
		return startServerEngineLocked(&cfg)
	}
	return startClientEngineLocked(&cfg)
}

// GenerateExportToken creates an encoded tt:// connection token string
// containing the server endpoint address and obfuscation parameters.
func GenerateExportToken(cfg *models.TrustTunnelConfig) string {
	params := url.Values{}
	params.Set("addr", cfg.ListenAddress)
	params.Set("hostname", cfg.ServerHostname)
	params.Set("transport", cfg.ForcedTransport)
	params.Set("probe", fmt.Sprintf("%d", cfg.AuthFailureStatusCode))
	params.Set("prefix", cfg.ClientRandomPrefix)
	params.Set("h2win", fmt.Sprintf("%d", cfg.H2InitialStreamWindowSize))
	params.Set("timeout", fmt.Sprintf("%d", cfg.TlsHandshakeTimeoutSecs))

	// Encode user credentials if any exist
	var users []models.TrustTunnelUser
	db.DB.Where("is_active = ?", true).Find(&users)
	if len(users) > 0 {
		params.Set("user", users[0].Username)
		params.Set("pass", users[0].Password)
	}

	raw := "tt://?" + params.Encode()
	return base64.StdEncoding.EncodeToString([]byte(raw))
}

// ParseImportToken decodes a tt:// encoded token and returns the parsed parameters.
func ParseImportToken(token string) (map[string]string, error) {
	decoded, err := base64.StdEncoding.DecodeString(token)
	if err != nil {
		// Try URL-safe base64
		decoded, err = base64.URLEncoding.DecodeString(token)
		if err != nil {
			return nil, fmt.Errorf("invalid token encoding: %w", err)
		}
	}

	raw := string(decoded)
	if !strings.HasPrefix(raw, "tt://") {
		return nil, fmt.Errorf("invalid token format: missing tt:// prefix")
	}

	u, err := url.Parse(raw)
	if err != nil {
		return nil, fmt.Errorf("failed to parse token URL: %w", err)
	}

	result := make(map[string]string)
	for key, vals := range u.Query() {
		if len(vals) > 0 {
			result[key] = vals[0]
		}
	}

	return result, nil
}
