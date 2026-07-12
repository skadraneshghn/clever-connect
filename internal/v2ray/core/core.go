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
	waitChan    chan struct{}
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

// GetBinPathForCore returns the binary path for the given core name with proper robust fallbacks.
func GetBinPathForCore(coreName string) (string, error) {
	// 1. Check local core folder relative to current working directory
	localPath := filepath.Join("core", coreName, coreName)
	if _, err := os.Stat(localPath); err == nil {
		_ = os.Chmod(localPath, 0755)
		return localPath, nil
	}

	// 2. Check local executable dir and its parent (in case running from a bin/ directory)
	exe, err := os.Executable()
	if err == nil {
		exeDir := filepath.Dir(exe)

		// e.g. /path/to/project/bin/xray
		localPathExe := filepath.Join(exeDir, coreName)
		if _, err := os.Stat(localPathExe); err == nil {
			_ = os.Chmod(localPathExe, 0755)
			return localPathExe, nil
		}

		// e.g. /path/to/project/bin/core/xray/xray
		localPathExeCore := filepath.Join(exeDir, "core", coreName, coreName)
		if _, err := os.Stat(localPathExeCore); err == nil {
			_ = os.Chmod(localPathExeCore, 0755)
			return localPathExeCore, nil
		}

		// e.g. /path/to/project/core/xray/xray when executable is in /path/to/project/bin/clever-connect
		parentCorePath := filepath.Join(filepath.Dir(exeDir), "core", coreName, coreName)
		if _, err := os.Stat(parentCorePath); err == nil {
			_ = os.Chmod(parentCorePath, 0755)
			return parentCorePath, nil
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

	return "", fmt.Errorf("binary for core %q not found in core folder or system PATH", coreName)
}

// GetXrayBinPath returns the absolute or relative path to the xray, v2ray or sing-box binary
func GetXrayBinPath() string {
	coreName := GetSelectedCoreName()
	path, err := GetBinPathForCore(coreName)
	if err == nil {
		return path
	}
	return filepath.Join("bin", coreName)
}

// GetClientBinPath returns the client-side binary path for the selected core with proper fallbacks.
func GetClientBinPath() (string, error) {
	coreName := GetSelectedCoreName()
	return GetBinPathForCore(coreName)
}

// openCoreLogFile resolves a writable location for the proxy core log file and
// opens it for writing. The path is resolved dynamically (next to the
// executable, then the current working directory) instead of a hardcoded
// absolute path that only exists on a single developer machine. On failure it
// returns an error so callers can fall back to the parent process stdout/stderr
// instead of silently discarding core output (which hides boot failures).
func openCoreLogFile() (*os.File, error) {
	candidates := []string{}

	if exe, err := os.Executable(); err == nil {
		candidates = append(candidates, filepath.Join(filepath.Dir(exe), "logs", "xray_core.log"))
	}
	candidates = append(candidates, filepath.Join("logs", "xray_core.log"))

	for _, p := range candidates {
		if dir := filepath.Dir(p); dir != "" {
			_ = os.MkdirAll(dir, 0755)
		}
		f, err := os.OpenFile(p, os.O_CREATE|os.O_WRONLY|os.O_TRUNC, 0644)
		if err == nil {
			return f, nil
		}
		logger.Warn("V2Ray", "Could not open core log file", "path", p, "error", err)
	}
	return nil, fmt.Errorf("no writable core log file location available")
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
	waitChan = make(chan struct{})
	wChan := waitChan

	// Convert configPath to absolute path so Xray can find it even if cmd.Dir is set to binPath folder
	absConfigPath, err := filepath.Abs(configPath)
	if err != nil {
		absConfigPath = configPath
	}

	var cmd *exec.Cmd
	if GetSelectedCoreName() == "v2ray" {
		cmd = exec.CommandContext(ctx, binPath, "-config", absConfigPath)
	} else {
		cmd = exec.CommandContext(ctx, binPath, "run", "-c", absConfigPath)
	}
	// Resolve the core's working directory (folder containing the binary) so
	// geosite.dat / geoip.dat are discovered locally. Also expose it via the
	// asset location env vars so Xray/V2Ray find routing databases regardless
	// of the process working directory.
	absBinDir, errDir := filepath.Abs(filepath.Dir(binPath))
	if errDir == nil {
		cmd.Dir = absBinDir
	} else {
		absBinDir = filepath.Dir(binPath)
	}

	cmd.Env = append(os.Environ(),
		"ENABLE_DEPRECATED_LEGACY_DNS_SERVERS=true",
		"ENABLE_DEPRECATED_MISSING_DOMAIN_RESOLVER=true",
		"ENABLE_DEPRECATED_SPECIAL_OUTBOUNDS=true",
		"XRAY_LOCATION_ASSET="+absBinDir,
		"V2RAY_LOCATION_ASSET="+absBinDir,
	)

	// Separate process group to allow clean termination of children if needed
	cmd.SysProcAttr = &syscall.SysProcAttr{Setpgid: true}

	// Redirect stdout/stderr to a dynamically-resolved log file for debugging.
	// If no log file can be opened, fall back to the parent process stdout/stderr
	// so core initialization errors are NEVER silently discarded (the previous
	// hardcoded path failed on every machine except one and then set Stdout/Stderr
	// to nil, hiding all boot failures).
	var coreLogFile *os.File
	if logFile, err := openCoreLogFile(); err == nil {
		cmd.Stdout = logFile
		cmd.Stderr = logFile
		coreLogFile = logFile
		logger.Info("V2Ray", "Core stdout/stderr redirected to log file", "path", logFile.Name())
	} else {
		logger.Warn("V2Ray", "No core log file available, redirecting core output to console", "error", err)
		cmd.Stdout = os.Stdout
		cmd.Stderr = os.Stderr
	}

	if err := cmd.Start(); err != nil {
		if coreLogFile != nil {
			_ = coreLogFile.Close()
		}
		cancel()
		cancelFunc = nil
		return fmt.Errorf("failed to start proxy process: %w", err)
	}

	cmdInstance = cmd

	// Start supervisor goroutine
	go func(c *exec.Cmd, wc chan struct{}, lf *os.File) {
		err := c.Wait()
		if lf != nil {
			_ = lf.Close()
		}
		close(wc)
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
	}(cmd, wChan, coreLogFile)

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
			if errKill := syscall.Kill(-pgid, syscall.SIGTERM); errKill != nil {
				logger.Warn("V2Ray", "Failed to kill process group with SIGTERM", "pgid", pgid, "error", errKill)
				_ = cmdInstance.Process.Signal(syscall.SIGTERM)
			}
		} else {
			logger.Warn("V2Ray", "Failed to get pgid", "error", err)
			_ = cmdInstance.Process.Signal(syscall.SIGTERM)
		}

		// Wait for a brief moment
		wChan := waitChan
		select {
		case <-wChan:
			// Gracefully stopped
		case <-time.After(3 * time.Second):
			// Force kill if it didn't exit in 3s
			logger.Warn("V2Ray", "Xray core did not exit gracefully, sending SIGKILL")
			if err == nil {
				_ = syscall.Kill(-pgid, syscall.SIGKILL)
			} else {
				_ = cmdInstance.Process.Kill()
			}
			if wChan != nil {
				<-wChan
			}
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
