package ehcocore

import (
	"bufio"
	"crypto/tls"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
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

	_ "github.com/Ehco1996/ehco/pkg/xray"
)

// Ehco JSON Config schemas matching its internal structure
type WSConfig struct {
	Path       string `json:"path,omitempty"`
	RemoteAddr string `json:"remote_addr,omitempty"`
}

type RelayOptions struct {
	EnableUDP          bool      `json:"enable_udp,omitempty"`
	EnableMultipathTCP bool      `json:"enable_multipath_tcp,omitempty"`
	WSConfig           *WSConfig `json:"ws_config,omitempty"`
	IdleTimeoutSec     int       `json:"idle_timeout_sec,omitempty"`
	DialTimeoutSec     int       `json:"dial_timeout_sec,omitempty"`
	ReadTimeoutSec     int       `json:"read_timeout_sec,omitempty"`
}

type RelayConfig struct {
	Listen        string        `json:"listen"`
	ListenType    string        `json:"listen_type"`
	TransportType string        `json:"transport_type"`
	Remotes       []string      `json:"remotes"`
	Options       *RelayOptions `json:"options,omitempty"`
}

type EhcoConfig struct {
	WebPort      int            `json:"web_port"`
	WebToken     string         `json:"web_token"`
	EnablePing   bool           `json:"enable_ping"`
	LogLevel     string         `json:"log_level"`
	RelayConfigs []*RelayConfig `json:"relay_configs"`
}

var (
	cmdInstance  *exec.Cmd
	mu           sync.Mutex
	expectedRun  bool          // whether the engine is expected to be running
	stopChan     chan struct{} // to signal supervisor loop to stop
	isServerMode bool          // whether we are running in server mode vs client mode
	restartDelay time.Duration // current restart delay for backoff
	activeURL    string        // tracks which URL (primary vs secondary) is currently running
)

// getEhcoBinPath ensures we look for 'ehco' in the exact same directory as 'clever-connect'
func getEhcoBinPath() string {
	exe, err := os.Executable()
	if err != nil {
		return "bin/ehco" // Fallback
	}
	return filepath.Join(filepath.Dir(exe), "ehco")
}

// getConfigDir uses the OS Temp directory to guarantee write permissions on cloud platforms
func getConfigDir() string {
	return filepath.Join(os.TempDir(), "clever-connect-data")
}

// EnsureBinary checks if the ehco binary exists, and compiles it if missing.
func EnsureBinary() error {
	binPath := getEhcoBinPath()
	if _, err := os.Stat(binPath); err == nil {
		return nil // File found, no compilation needed
	}

	logger.Info("Ehco", "ehco binary missing. Starting automatic self-compilation.", "path", binPath)

	if err := os.MkdirAll(filepath.Dir(binPath), 0755); err != nil {
		return fmt.Errorf("failed to create bin directory: %w", err)
	}

	buildCmd := exec.Command("go", "build", "-o", binPath, "github.com/Ehco1996/ehco/cmd/ehco")

	out, err := buildCmd.CombinedOutput()
	if err != nil {
		return fmt.Errorf("failed to compile ehco at %s: %w\nCompiler Output:\n%s", binPath, err, string(out))
	}

	logger.Info("Ehco", "ehco binary compiled successfully", "path", binPath)
	return nil
}

// StartServerEngine launches the ehco relayer using Server DB configs
func StartServerEngine(dbCfg *models.EhcoServerConfig) error {
	mu.Lock()
	defer mu.Unlock()

	if err := StopEngineLocked(); err != nil {
		return err
	}

	if err := startServerEngineLocked(dbCfg); err != nil {
		return err
	}

	expectedRun = true
	isServerMode = true
	restartDelay = 1 * time.Second
	stopChan = make(chan struct{})
	startSupervisor()

	return nil
}

// StartClientEngine runs locally, capturing a local port and proxying to the remote Clever Cloud WebSocket tunnel
func StartClientEngine(dbCfg *models.EhcoClientConfig) error {
	mu.Lock()
	defer mu.Unlock()

	if err := StopEngineLocked(); err != nil {
		return err
	}

	primaryURL := dbCfg.RemoteURL
	if dbCfg.EnableBridge && dbCfg.BridgeURL != "" {
		primaryURL = dbCfg.BridgeURL
	}
	activeURL = primaryURL

	if err := startClientEngineLocked(dbCfg); err != nil {
		return err
	}

	expectedRun = true
	isServerMode = false
	restartDelay = 1 * time.Second
	stopChan = make(chan struct{})
	startSupervisor()

	if dbCfg.SecondaryURL != "" {
		go startFailoverMonitor(dbCfg, stopChan)
	}

	return nil
}

// startServerEngineLocked launches the ehco relayer using Server DB configs (must hold mu)
func startServerEngineLocked(dbCfg *models.EhcoServerConfig) error {
	if err := EnsureBinary(); err != nil {
		return err
	}

	// Format secure auth path
	authPath := "/tunnel"
	if dbCfg.AuthToken != "" {
		authPath = "/tunnel/" + dbCfg.AuthToken
	}

	// Configure query params for Multiplexing if active
	if dbCfg.EnableMux {
		authPath += "?mux=true"
	} else {
		authPath += "?mux=false"
	}

	// Default keep-alive interval — enforce a minimum of 60s so bursty
	// streaming traffic (YouTube/Instagram) doesn't get killed during
	// natural gaps between buffer fills.
	idleTimeout := dbCfg.KeepAlive
	if idleTimeout < 60 {
		idleTimeout = 60
	}

	// Build JSON config
	cfg := &EhcoConfig{
		WebPort:    0,
		WebToken:   "",
		EnablePing: true, // Optimized: Enable ping/keepalive for stability
		LogLevel:   "info",
		RelayConfigs: []*RelayConfig{
			{
				Listen:        "0.0.0.0:" + dbCfg.ListenPort,
				ListenType:    "ws",
				TransportType: "raw",
				Remotes:       []string{dbCfg.TargetHost},
				Options: &RelayOptions{
					EnableUDP:          true,
					EnableMultipathTCP: true,
					IdleTimeoutSec:     idleTimeout,
					DialTimeoutSec:     10,
					ReadTimeoutSec:     0, // Disabled: don't kill slow CDN responses (YouTube/Instagram)
					WSConfig: &WSConfig{
						Path: authPath,
					},
				},
			},
		},
	}

	// Write config to data folder
	dataDir := getConfigDir()
	if err := os.MkdirAll(dataDir, 0755); err != nil {
		return fmt.Errorf("failed to create data directory: %w", err)
	}

	configPath := filepath.Join(dataDir, "ehco_server.json")
	configBytes, err := json.MarshalIndent(cfg, "", "  ")
	if err != nil {
		return fmt.Errorf("failed to marshal config: %w", err)
	}

	if err := os.WriteFile(configPath, configBytes, 0644); err != nil {
		return fmt.Errorf("failed to write config file: %w", err)
	}

	logger.Info("Ehco", "Starting Server Tunnel Process",
		"listen_port", dbCfg.ListenPort,
		"target_host", dbCfg.TargetHost,
		"enable_mux", dbCfg.EnableMux,
		"keep_alive", idleTimeout,
	)

	// Launch process with high performance buffer size: 1048576 (1MB) for streaming
	binPath := getEhcoBinPath()
	cmdInstance = exec.Command(binPath, "-c", configPath, "--buffer_size", "1048576")

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
		return fmt.Errorf("failed to start ehco server process: %w", err)
	}

	// Read stdout in background
	go func(pipe io.ReadCloser) {
		defer pipe.Close()
		scanner := bufio.NewScanner(pipe)
		for scanner.Scan() {
			logger.Info("EhcoSub", scanner.Text())
		}
	}(stdoutPipe)

	// Read stderr in background
	go func(pipe io.ReadCloser) {
		defer pipe.Close()
		scanner := bufio.NewScanner(pipe)
		for scanner.Scan() {
			logger.Error("EhcoSub", scanner.Text())
		}
	}(stderrPipe)

	// Reap process and log when it exits
	go func(cmd *exec.Cmd) {
		err := cmd.Wait()
		logger.Warn("Ehco", "Subprocess exited", "error", err)
	}(cmdInstance)

	return nil
}

// startClientEngineLocked runs locally, capturing a local port and proxying to the remote Clever Cloud WebSocket tunnel (must hold mu)
func startClientEngineLocked(dbCfg *models.EhcoClientConfig) error {
	if err := EnsureBinary(); err != nil {
		return err
	}

	transportType := "wss"
	baseAddr := "wss://0.0.0.0:8080"
	authPath := "/tunnel"
	if dbCfg.AuthToken != "" {
		authPath = "/tunnel/" + dbCfg.AuthToken
	}

	// Select URL to parse (activeURL vs database config fallback)
	urlToParse := activeURL
	if urlToParse == "" {
		urlToParse = dbCfg.RemoteURL
		if dbCfg.EnableBridge && dbCfg.BridgeURL != "" {
			urlToParse = dbCfg.BridgeURL
		}
		activeURL = urlToParse
	}

	// Parse selected URL
	if urlToParse != "" {
		if !strings.Contains(urlToParse, "://") {
			urlToParse = "wss://" + urlToParse
		}

		u, err := url.Parse(urlToParse)
		if err == nil && u.Host != "" {
			scheme := u.Scheme
			if scheme == "https" {
				scheme = "wss"
			} else if scheme == "http" {
				scheme = "ws"
			}

			host := u.Host
			if !strings.Contains(host, ":") {
				if scheme == "wss" {
					host = host + ":443"
				} else {
					host = host + ":80"
				}
			}

			baseAddr = fmt.Sprintf("%s://%s", scheme, host)
			transportType = scheme

			// Format WebSocket path
			path := u.Path
			if path == "" || path == "/" {
				path = "/tunnel"
			}
			if dbCfg.AuthToken != "" && !strings.HasSuffix(path, dbCfg.AuthToken) {
				path = strings.TrimSuffix(path, "/") + "/" + dbCfg.AuthToken
			}
			authPath = path
		}
	}

	// Configure query parameters for Multiplexing and SNI Spoofing
	params := url.Values{}
	if dbCfg.EnableMux {
		params.Add("mux", "true")
	} else {
		params.Add("mux", "false")
	}

	// Add params to WS Path query string
	if strings.Contains(authPath, "?") {
		authPath += "&" + params.Encode()
	} else {
		authPath += "?" + params.Encode()
	}

	// Default keep-alive interval — enforce a minimum of 60s so bursty
	// streaming traffic (YouTube/Instagram) doesn't get killed during
	// natural gaps between buffer fills.
	idleTimeout := dbCfg.KeepAlive
	if idleTimeout < 60 {
		idleTimeout = 60
	}

	// Build JSON config
	cfg := &EhcoConfig{
		WebPort:    0,
		WebToken:   "",
		EnablePing: true, // Optimized: Enable ping/keepalive for stability
		LogLevel:   "info",
		RelayConfigs: []*RelayConfig{
			{
				Listen:        "0.0.0.0:" + dbCfg.LocalPort,
				ListenType:    "raw",
				TransportType: transportType,
				Remotes:       []string{baseAddr},
				Options: &RelayOptions{
					EnableUDP:          true,
					EnableMultipathTCP: true,
					IdleTimeoutSec:     idleTimeout,
					DialTimeoutSec:     10,
					ReadTimeoutSec:     0, // Disabled: don't kill slow CDN responses (YouTube/Instagram)
					WSConfig: &WSConfig{
						Path: authPath,
					},
				},
			},
		},
	}

	// Write config to data folder
	dataDir := getConfigDir()
	if err := os.MkdirAll(dataDir, 0755); err != nil {
		return fmt.Errorf("failed to create data directory: %w", err)
	}

	configPath := filepath.Join(dataDir, "ehco_client.json")
	configBytes, err := json.MarshalIndent(cfg, "", "  ")
	if err != nil {
		return fmt.Errorf("failed to marshal config: %w", err)
	}

	if err := os.WriteFile(configPath, configBytes, 0644); err != nil {
		return fmt.Errorf("failed to write config file: %w", err)
	}

	logger.Info("Ehco", "Starting Client Tunnel Process",
		"local_port", dbCfg.LocalPort,
		"remote_url", baseAddr,
		"path", authPath,
		"sni", dbCfg.SNI,
		"enable_mux", dbCfg.EnableMux,
		"keep_alive", idleTimeout,
		"bypass_ir", dbCfg.BypassIR,
	)

	// Launch process with high performance buffer size: 1048576 (1MB) for streaming
	binPath := getEhcoBinPath()
	cmdInstance = exec.Command(binPath, "-c", configPath, "--buffer_size", "1048576")

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
		return fmt.Errorf("failed to start ehco client process: %w", err)
	}

	// Read stdout in background
	go func(pipe io.ReadCloser) {
		defer pipe.Close()
		scanner := bufio.NewScanner(pipe)
		for scanner.Scan() {
			logger.Info("EhcoSub", scanner.Text())
		}
	}(stdoutPipe)

	// Read stderr in background
	go func(pipe io.ReadCloser) {
		defer pipe.Close()
		scanner := bufio.NewScanner(pipe)
		for scanner.Scan() {
			logger.Error("EhcoSub", scanner.Text())
		}
	}(stderrPipe)

	// Reap process and log when it exits
	go func(cmd *exec.Cmd) {
		err := cmd.Wait()
		logger.Warn("Ehco", "Subprocess exited", "error", err)
	}(cmdInstance)

	return nil
}

// StopEngine gracefully shuts down the active ehco tunnel
func StopEngine() {
	mu.Lock()
	defer mu.Unlock()
	_ = StopEngineLocked()
}

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
		logger.Info("Ehco", "Terminating active ehco tunnel process")
		if cmdInstance.Process != nil {
			if err := cmdInstance.Process.Kill(); err != nil {
				logger.Error("Ehco", "Failed to kill ehco process", "error", err)
			}
		}
		_ = cmdInstance.Wait()
		cmdInstance = nil
	}
	return nil
}

// IsRunning returns true if the engine process is active
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

// startSupervisor spawns the background monitoring loop (must hold lock to setup/initialize variables)
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
				logger.Warn("Ehco", "Subprocess is not running. Initiating auto-restart.")

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

				// Double check if expectedRun was disabled during the sleep
				mu.Lock()
				stillExpected := expectedRun
				mu.Unlock()

				if !stillExpected {
					return
				}

				if err := restartEngineFromDB(); err != nil {
					logger.Error("Ehco", "Failed to auto-restart engine", "error", err)
				} else {
					logger.Info("Ehco", "Subprocess auto-restarted successfully")
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

// restartEngineFromDB loads settings from SQLite and restarts the process (holds internal lock)
func restartEngineFromDB() error {
	mu.Lock()
	defer mu.Unlock()

	if !expectedRun {
		return nil
	}

	if isServerMode {
		var serverCfg models.EhcoServerConfig
		if err := db.DB.First(&serverCfg).Error; err != nil {
			return err
		}
		if !serverCfg.IsActive {
			return fmt.Errorf("server tunnel config is no longer active in DB")
		}
		_ = stopEngineLockedOnly()
		return startServerEngineLocked(&serverCfg)
	} else {
		var clientCfg models.EhcoClientConfig
		if err := db.DB.First(&clientCfg).Error; err != nil {
			return err
		}
		if !clientCfg.IsActive {
			return fmt.Errorf("client tunnel config is no longer active in DB")
		}
		_ = stopEngineLockedOnly()
		return startClientEngineLocked(&clientCfg)
	}
}

// checkAddressHealth performs a WebSocket upgrade HTTP handshake probe
func checkAddressHealth(address string, authToken string, enableMux bool) bool {
	if !strings.Contains(address, "://") {
		address = "wss://" + address
	}
	u, err := url.Parse(address)
	if err != nil {
		return false
	}

	scheme := u.Scheme
	if scheme == "wss" || scheme == "https" {
		scheme = "https"
	} else {
		scheme = "http"
	}

	path := u.Path
	if path == "" || path == "/" {
		path = "/tunnel"
	}
	if authToken != "" && !strings.HasSuffix(path, authToken) {
		path = strings.TrimSuffix(path, "/") + "/" + authToken
	}

	probeURL := fmt.Sprintf("%s://%s%s", scheme, u.Host, path)
	if enableMux {
		probeURL += "%3Fmux=true"
	} else {
		probeURL += "%3Fmux=false"
	}

	client := &http.Client{
		Timeout: 4 * time.Second,
		Transport: &http.Transport{
			TLSClientConfig: &tls.Config{InsecureSkipVerify: true},
		},
	}

	req, err := http.NewRequest("GET", probeURL, nil)
	if err != nil {
		return false
	}
	req.Header.Set("Upgrade", "websocket")
	req.Header.Set("Connection", "Upgrade")
	req.Header.Set("Sec-WebSocket-Key", "dGhlIHNhbXBsZSBub25jZQ==")
	req.Header.Set("Sec-WebSocket-Version", "13")

	resp, err := client.Do(req)
	if err != nil {
		return false
	}
	defer resp.Body.Close()

	if resp.StatusCode != 101 {
		return false
	}
	return true
}

// startFailoverMonitor monitors the primary endpoint and fails over to secondary, or fails back
func startFailoverMonitor(dbCfg *models.EhcoClientConfig, currentStopChan chan struct{}) {
	primaryURL := dbCfg.RemoteURL
	if dbCfg.EnableBridge && dbCfg.BridgeURL != "" {
		primaryURL = dbCfg.BridgeURL
	}
	secondaryURL := dbCfg.SecondaryURL

	ticker := time.NewTicker(5 * time.Second)
	defer ticker.Stop()

	consecutiveFailures := 0
	consecutiveSuccesses := 0

	for {
		select {
		case <-currentStopChan:
			return
		case <-ticker.C:
			mu.Lock()
			if !expectedRun || isServerMode {
				mu.Unlock()
				return
			}
			mu.Unlock()

			primaryHealthy := checkAddressHealth(primaryURL, dbCfg.AuthToken, dbCfg.EnableMux)

			mu.Lock()
			if !expectedRun {
				mu.Unlock()
				return
			}

			if activeURL == primaryURL {
				if !primaryHealthy {
					consecutiveFailures++
					consecutiveSuccesses = 0
					logger.Warn("EhcoFailover", "Primary URL probe failed", "url", primaryURL, "consecutiveFailures", consecutiveFailures)
					if consecutiveFailures >= 3 {
						logger.Error("EhcoFailover", "Primary URL down. Failing over to secondary URL.", "primary", primaryURL, "secondary", secondaryURL)
						activeURL = secondaryURL
						consecutiveFailures = 0
						_ = stopEngineLockedOnly()
						if err := startClientEngineLocked(dbCfg); err != nil {
							logger.Error("EhcoFailover", "Failed to start client engine with secondary URL", "error", err)
						}
					}
				} else {
					consecutiveFailures = 0
				}
			} else if activeURL == secondaryURL {
				if primaryHealthy {
					consecutiveSuccesses++
					consecutiveFailures = 0
					logger.Info("EhcoFailover", "Primary URL probe succeeded while on secondary", "url", primaryURL, "consecutiveSuccesses", consecutiveSuccesses)
					if consecutiveSuccesses >= 3 {
						logger.Info("EhcoFailover", "Primary URL has recovered. Failing back to primary URL.", "primary", primaryURL)
						activeURL = primaryURL
						consecutiveSuccesses = 0
						_ = stopEngineLockedOnly()
						if err := startClientEngineLocked(dbCfg); err != nil {
							logger.Error("EhcoFailover", "Failed to start client engine with primary URL", "error", err)
						}
					}
				} else {
					consecutiveSuccesses = 0
				}
			}
			mu.Unlock()
		}
	}
}
