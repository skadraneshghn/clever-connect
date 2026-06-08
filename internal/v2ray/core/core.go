package core

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"sync"
	"syscall"
	"time"

	"clever-connect/internal/db"
	"clever-connect/internal/logger"
	"clever-connect/internal/models"
)

var (
	cmdInstance *exec.Cmd
	mu          sync.Mutex
	cancelFunc  context.CancelFunc
)

// GetSelectedCoreName returns the selected core name ("xray", "v2ray", or "sing-box") from settings, or fallbacks.
func GetSelectedCoreName() string {
	if db.DB != nil {
		var setting models.V2RayClientSetting
		if err := db.DB.Where("key = ?", "v2ray_core").First(&setting).Error; err == nil && setting.Value != "" {
			return setting.Value
		}
	}
	// Fallback detection
	if _, err := os.Stat("core/xray/xray"); err == nil {
		return "xray"
	}
	if _, err := os.Stat("core/v2ray/v2ray"); err == nil {
		return "v2ray"
	}
	if _, err := os.Stat("core/sing-box/sing-box"); err == nil {
		return "sing-box"
	}
	return "xray" // default fallback
}

// GetXrayBinPath returns the absolute or relative path to the xray, v2ray or sing-box binary
func GetXrayBinPath() string {
	coreName := GetSelectedCoreName()
	var binPath string
	switch coreName {
	case "v2ray":
		binPath = "core/v2ray/v2ray"
	case "sing-box":
		binPath = "core/sing-box/sing-box"
	default: // "xray"
		binPath = "core/xray/xray"
	}

	if _, err := os.Stat(binPath); err == nil {
		return binPath
	}

	exe, err := os.Executable()
	if err == nil {
		localPath := filepath.Join(filepath.Dir(exe), coreName)
		if _, err := os.Stat(localPath); err == nil {
			return localPath
		}
	}
	return filepath.Join("bin", coreName)
}

// GetClientBinPath returns the client-side binary path for the selected core with proper fallbacks.
func GetClientBinPath() (string, error) {
	coreName := GetSelectedCoreName()

	// 1. Check local core folder
	localPath := filepath.Join("core", coreName, coreName)
	if _, err := os.Stat(localPath); err == nil {
		_ = os.Chmod(localPath, 0755)
		return localPath, nil
	}

	// 2. Check local executable dir
	exe, err := os.Executable()
	if err == nil {
		localPathExe := filepath.Join(filepath.Dir(exe), coreName)
		if _, err := os.Stat(localPathExe); err == nil {
			_ = os.Chmod(localPathExe, 0755)
			return localPathExe, nil
		}
	}

	// 3. Fallback: system LookPath
	if path, err := exec.LookPath(coreName); err == nil {
		return path, nil
	}

	// 4. Extract embedded xray if selected core is xray
	if coreName == "xray" {
		return ExtractCoreBinary()
	}

	return "", fmt.Errorf("binary for selected core %q not found in core folder or system PATH", coreName)
}

// StartCore starts the Xray/V2Ray/Sing-Box process with the given config file
func StartCore(configPath string) error {
	mu.Lock()
	defer mu.Unlock()

	if err := StopCoreLocked(); err != nil {
		return fmt.Errorf("failed to stop existing core: %w", err)
	}

	binPath := GetXrayBinPath()
	if _, err := os.Stat(binPath); err != nil {
		coreName := GetSelectedCoreName()
		// Fallback: check if selected core is available in system PATH
		if path, err := exec.LookPath(coreName); err == nil {
			binPath = path
		} else {
			return fmt.Errorf("%s binary not found at %s or in system PATH. Please place the binary inside the project", coreName, binPath)
		}
	}
	if abs, err := filepath.Abs(binPath); err == nil {
		binPath = abs
	}

	logger.Info("V2Ray", "Starting proxy core process", "bin", binPath, "config", configPath)

	ctx, cancel := context.WithCancel(context.Background())
	cancelFunc = cancel

	// Convert configPath to absolute path so Xray can find it even if cmd.Dir is set to binPath folder
	absConfigPath, err := filepath.Abs(configPath)
	if err != nil {
		absConfigPath = configPath
	}

	cmd := exec.CommandContext(ctx, binPath, "run", "-c", absConfigPath)
	
	// Set Cwd to the folder containing xray/v2ray so it resolves geosite.dat and geoip.dat locally
	absBinDir, err := filepath.Abs(filepath.Dir(binPath))
	if err == nil {
		cmd.Dir = absBinDir
	}

	// Separate process group to allow clean termination of children if needed
	cmd.SysProcAttr = &syscall.SysProcAttr{Setpgid: true}

	// We don't want stdout/stderr clogging, but we can capture it in debug logs or suppress
	cmd.Stdout = nil
	cmd.Stderr = nil

	if err := cmd.Start(); err != nil {
		cancel()
		cancelFunc = nil
		return fmt.Errorf("failed to start proxy process: %w", err)
	}

	cmdInstance = cmd

	// Start supervisor goroutine
	go func(c *exec.Cmd) {
		err := c.Wait()
		mu.Lock()
		defer mu.Unlock()
		if cmdInstance == c {
			if err != nil {
				logger.Error("V2Ray", "Xray process exited with error", "error", err)
			} else {
				logger.Info("V2Ray", "Xray process exited cleanly")
			}
			cmdInstance = nil
			if cancelFunc != nil {
				cancelFunc()
				cancelFunc = nil
			}
		}
	}(cmd)

	return nil
}

// StopCore stops the running Xray process
func StopCore() error {
	mu.Lock()
	defer mu.Unlock()
	return StopCoreLocked()
}

// StopCoreLocked terminates the Xray process (internal usage)
func StopCoreLocked() error {
	if cmdInstance != nil {
		logger.Info("V2Ray", "Terminating active Xray core process")
		
		// Try graceful SIGTERM first
		pgid, err := syscall.Getpgid(cmdInstance.Process.Pid)
		if err == nil {
			_ = syscall.Kill(-pgid, syscall.SIGTERM) // Kill the whole process group
		} else {
			_ = cmdInstance.Process.Signal(syscall.SIGTERM)
		}

		// Wait for a brief moment
		done := make(chan error, 1)
		go func() {
			done <- cmdInstance.Wait()
		}()

		select {
		case <-done:
			// Gracefully stopped
		case <-time.After(3 * time.Second):
			// Force kill if it didn't exit in 3s
			logger.Warn("V2Ray", "Xray core did not exit gracefully, sending SIGKILL")
			if err == nil {
				_ = syscall.Kill(-pgid, syscall.SIGKILL)
			} else {
				_ = cmdInstance.Process.Kill()
			}
			<-done
		}

		cmdInstance = nil
	}

	if cancelFunc != nil {
		cancelFunc()
		cancelFunc = nil
	}

	return nil
}

// IsRunning checks if the core process is currently active
func IsRunning() bool {
	mu.Lock()
	defer mu.Unlock()
	return cmdInstance != nil && cmdInstance.Process != nil && cmdInstance.ProcessState == nil
}
