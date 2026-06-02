package ehcocore

import (
	"encoding/json"
	"fmt"
	"net/url"
	"os"
	"os/exec"
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
	TCPRemotes    []string      `json:"tcp_remotes"`
	UDPRemotes    []string      `json:"udp_remotes,omitempty"`
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

// EnsureBinary checks if the ehco binary exists in bin/ehco, and compiles it if missing.
func EnsureBinary() error {
	binPath := "bin/ehco"
	if _, err := os.Stat(binPath); err == nil {
		return nil
	}

	logger.Info("Ehco", "ehco binary missing. Starting automatic self-compilation.")
	
	// Create bin folder if not exists
	if err := os.MkdirAll("bin", 0755); err != nil {
		return fmt.Errorf("failed to create bin directory: %w", err)
	}

	// Run go build
	buildCmd := exec.Command("go", "build", "-o", binPath, "github.com/Ehco1996/ehco/cmd/ehco")
	buildCmd.Stdout = os.Stdout
	buildCmd.Stderr = os.Stderr
	
	if err := buildCmd.Run(); err != nil {
		return fmt.Errorf("failed to compile ehco: %w", err)
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
				TCPRemotes:    []string{dbCfg.TargetHost},
				UDPRemotes:    []string{dbCfg.TargetHost},
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
	if err := os.MkdirAll("data", 0755); err != nil {
		return fmt.Errorf("failed to create data directory: %w", err)
	}

	configPath := "data/ehco_server.json"
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
	cmdInstance = exec.Command("bin/ehco", "-c", configPath)
	
	// Stream logs to Clever Connect logger
	cmdInstance.Stdout = logger.GinWriter()
	cmdInstance.Stderr = logger.GinWriter()

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

	// Parse remoteURL
	if dbCfg.RemoteURL != "" {
		urlToParse := dbCfg.RemoteURL
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

	if dbCfg.SNI != "" {
		params.Add("sni", dbCfg.SNI)
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
				TCPRemotes:    []string{baseAddr},
				UDPRemotes:    []string{baseAddr},
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
	if err := os.MkdirAll("data", 0755); err != nil {
		return fmt.Errorf("failed to create data directory: %w", err)
	}

	configPath := "data/ehco_client.json"
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
	cmdInstance = exec.Command("bin/ehco", "-c", configPath)
	cmdInstance.Stdout = logger.GinWriter()
	cmdInstance.Stderr = logger.GinWriter()

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
