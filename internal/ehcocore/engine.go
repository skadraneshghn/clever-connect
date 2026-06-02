package ehcocore

import (
	"encoding/json"
	"fmt"
	"net/url"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"sync"

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
	cmdInstance *exec.Cmd
	mu          sync.Mutex
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

	// Default keep-alive interval
	idleTimeout := dbCfg.KeepAlive
	if idleTimeout <= 0 {
		idleTimeout = 15
	}

	// Build JSON config
	cfg := &EhcoConfig{
		WebPort:    0,
		WebToken:   "",
		EnablePing: false,
		LogLevel:   "info",
		RelayConfigs: []*RelayConfig{
			{
				Listen:        "127.0.0.1:" + dbCfg.ListenPort,
				ListenType:    "ws",
				TransportType: "raw",
				Remotes:       []string{dbCfg.TargetHost},
				Options: &RelayOptions{
					EnableUDP:          true,
					EnableMultipathTCP: true,
					IdleTimeoutSec:     idleTimeout,
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

	// Launch process
	binPath := getEhcoBinPath()
	cmdInstance = exec.Command(binPath, "-c", configPath)
	
	// --- Suppress noisy I/O streams in production ---
	cmdInstance.Stdout = nil
	cmdInstance.Stderr = nil

	if err := cmdInstance.Start(); err != nil {
		cmdInstance = nil
		return fmt.Errorf("failed to start ehco server process: %w", err)
	}

	return nil
}

// StartClientEngine runs locally, capturing a local port and proxying to the remote Clever Cloud WebSocket tunnel
func StartClientEngine(dbCfg *models.EhcoClientConfig) error {
	mu.Lock()
	defer mu.Unlock()

	if err := StopEngineLocked(); err != nil {
		return err
	}

	if err := EnsureBinary(); err != nil {
		return err
	}

	transportType := "wss"
	baseAddr := "wss://127.0.0.1:8080"
	authPath := "/tunnel"
	if dbCfg.AuthToken != "" {
		authPath = "/tunnel/" + dbCfg.AuthToken
	}

	// Select URL to parse (dynamic bridge override vs direct)
	urlToParse := dbCfg.RemoteURL
	if dbCfg.EnableBridge && dbCfg.BridgeURL != "" {
		urlToParse = dbCfg.BridgeURL
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

	// Dynamic SNI Selection
	sniToUse := ""
	if dbCfg.EnableBridge {
		if dbCfg.BridgeSNI != "" {
			sniToUse = dbCfg.BridgeSNI
		} else if dbCfg.BridgeURL != "" {
			// Auto extract SNI from BridgeURL
			uBridge, err := url.Parse(dbCfg.BridgeURL)
			if err == nil {
				host := uBridge.Host
				if strings.Contains(host, ":") {
					host = strings.Split(host, ":")[0]
				}
				sniToUse = host
			}
		}
	} else {
		if dbCfg.SNI != "" {
			sniToUse = dbCfg.SNI
		} else if dbCfg.RemoteURL != "" {
			// Auto extract SNI from RemoteURL
			uRemote, err := url.Parse(dbCfg.RemoteURL)
			if err == nil {
				host := uRemote.Host
				if strings.Contains(host, ":") {
					host = strings.Split(host, ":")[0]
				}
				sniToUse = host
			}
		}
	}

	if sniToUse != "" {
		params.Add("sni", sniToUse)
	}

	// Add params to WS Path query string
	if strings.Contains(authPath, "?") {
		authPath += "&" + params.Encode()
	} else {
		authPath += "?" + params.Encode()
	}

	// Default keep-alive interval
	idleTimeout := dbCfg.KeepAlive
	if idleTimeout <= 0 {
		idleTimeout = 15
	}

	// Build JSON config
	cfg := &EhcoConfig{
		WebPort:    0,
		WebToken:   "",
		EnablePing: false,
		LogLevel:   "info",
		RelayConfigs: []*RelayConfig{
			{
				Listen:        "127.0.0.1:" + dbCfg.LocalPort,
				ListenType:    "raw",
				TransportType: transportType,
				Remotes:       []string{baseAddr},
				Options: &RelayOptions{
					EnableUDP:          true,
					EnableMultipathTCP: true,
					IdleTimeoutSec:     idleTimeout,
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

	// Launch process
	binPath := getEhcoBinPath()
	cmdInstance = exec.Command(binPath, "-c", configPath)
	
	// --- Suppress noisy I/O streams in production ---
	cmdInstance.Stdout = nil
	cmdInstance.Stderr = nil

	if err := cmdInstance.Start(); err != nil {
		cmdInstance = nil
		return fmt.Errorf("failed to start ehco client process: %w", err)
	}

	return nil
}

// StopEngine gracefully shuts down the active ehco tunnel
func StopEngine() {
	mu.Lock()
	defer mu.Unlock()
	_ = StopEngineLocked()
}

func StopEngineLocked() error {
	if cmdInstance != nil {
		logger.Info("Ehco", "Terminating active ehco tunnel process")
		// Kill the process group or process directly
		if err := cmdInstance.Process.Kill(); err != nil {
			logger.Error("Ehco", "Failed to kill ehco process", "error", err)
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
	return cmdInstance != nil && cmdInstance.Process != nil
}
