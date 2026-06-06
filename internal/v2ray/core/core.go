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

	"clever-connect/internal/logger"
)

var (
	cmdInstance *exec.Cmd
	mu          sync.Mutex
	cancelFunc  context.CancelFunc
)

// GetXrayBinPath returns the absolute or relative path to the xray or v2ray binary
func GetXrayBinPath() string {
	// 1. Check core/xray/xray
	if _, err := os.Stat("core/xray/xray"); err == nil {
		return "core/xray/xray"
	}
	// 2. Check core/v2ray/v2ray
	if _, err := os.Stat("core/v2ray/v2ray"); err == nil {
		return "core/v2ray/v2ray"
	}

	exe, err := os.Executable()
	if err == nil {
		localPath := filepath.Join(filepath.Dir(exe), "xray")
		if _, err := os.Stat(localPath); err == nil {
			return localPath
		}
	}
	return "bin/xray"
}

// StartCore starts the Xray/V2Ray process with the given config file
func StartCore(configPath string) error {
	mu.Lock()
	defer mu.Unlock()

	if err := StopCoreLocked(); err != nil {
		return fmt.Errorf("failed to stop existing core: %w", err)
	}

	binPath := GetXrayBinPath()
	if _, err := os.Stat(binPath); err != nil {
		// Fallback: check if 'xray' is available in system PATH
		if path, err := exec.LookPath("xray"); err == nil {
			binPath = path
		} else if path, err := exec.LookPath("v2ray"); err == nil {
			binPath = path
		} else {
			return fmt.Errorf("xray/v2ray binary not found at %s or in system PATH. Please place the binary inside the project", binPath)
		}
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
