package core

import (
	"bufio"
	"context"
	"fmt"
	"net"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"sync/atomic"
	"syscall"
	"time"
	"clever-connect/internal/db"
	"clever-connect/internal/logger"
	"clever-connect/internal/models"
	"clever-connect/internal/v2ray/sysproxy"
	"clever-connect/internal/v2ray/traffic/desync"
)

// ClientProxyMetrics reports real-time metrics to the UI layer
type ClientProxyMetrics struct {
	ActiveConnections int32 `json:"active_connections"`
	BytesTx           int64 `json:"bytes_tx"` // uploaded
	BytesRx           int64 `json:"bytes_rx"` // downloaded
}

var (
	// Client process state
	clientCmd        *exec.Cmd
	clientMu         sync.Mutex
	clientShouldRun  bool
	clientConfigPath string
	clientRunning    bool
	clientCancel     context.CancelFunc
	clientErr        error

	// Restart tracking
	clientCrashes int
	clientLastStart time.Time

	// Local Proxy Engine state
	socksListener  net.Listener
	httpListener   net.Listener
	activeConns    int32
	totalBytesTx   int64 // upload
	totalBytesRx   int64 // download
	proxyWG        sync.WaitGroup
	proxyStopChan  chan struct{}
	proxyMu        sync.Mutex
	proxyRunning   bool

	// Metrics channel
	MetricsChan = make(chan ClientProxyMetrics, 100)

	// Settings defaults
	maxConns    int32         = 500
	idleTimeout time.Duration = 30 * time.Second
)

// ExtractCoreBinary extracts the embedded Xray binary to a temporary folder
func ExtractCoreBinary() (string, error) {
	tempPath := filepath.Join(os.TempDir(), "xray_client_core")
	
	// Check if already extracted
	if info, err := os.Stat(tempPath); err == nil {
		if info.Mode()&0111 != 0 {
			return tempPath, nil
		}
		// Set executable permissions if not already set
		_ = os.Chmod(tempPath, 0755)
		return tempPath, nil
	}

	logger.Info("ClientV2Ray", "Extracting embedded Xray binary to temp path", "path", tempPath)
	data, err := EmbeddedCore.ReadFile("assets/xray")
	if err != nil {
		return "", fmt.Errorf("failed to read embedded core asset: %w", err)
	}

	if err := os.WriteFile(tempPath, data, 0755); err != nil {
		return "", fmt.Errorf("failed to write core binary: %w", err)
	}

	return tempPath, nil
}

// CheckPortAvailable checks if a port is free to listen on 127.0.0.1
func CheckPortAvailable(port int) bool {
	addr := net.JoinHostPort("127.0.0.1", strconv.Itoa(port))
	ln, err := net.Listen("tcp", addr)
	if err == nil {
		ln.Close()
		return true
	}
	return false
}

// FindAvailablePort returns the requested port if available, or searches for the next free port, avoiding excluded ports
func FindAvailablePort(startPort int, excludePorts ...int) int {
	for port := startPort; port < startPort+100; port++ {
		excluded := false
		for _, ep := range excludePorts {
			if port == ep {
				excluded = true
				break
			}
		}
		if !excluded && CheckPortAvailable(port) {
			return port
		}
	}
	return startPort
}

// IsClientRunning returns whether the client engine is active
func IsClientRunning() bool {
	clientMu.Lock()
	defer clientMu.Unlock()
	return clientRunning
}

// StartClientCore starts the client proxy engine with supervisor
func StartClientCore(configPath string) error {
	clientMu.Lock()
	defer clientMu.Unlock()

	if clientRunning {
		return fmt.Errorf("client proxy engine is already running")
	}

	clientConfigPath = configPath
	clientShouldRun = true
	clientRunning = true
	clientCrashes = 0
	proxyStopChan = make(chan struct{})

	// Reset counters
	atomic.StoreInt32(&activeConns, 0)
	atomic.StoreInt64(&totalBytesTx, 0)
	atomic.StoreInt64(&totalBytesRx, 0)

	ctx, cancel := context.WithCancel(context.Background())
	clientCancel = cancel

	go clientSupervisor(ctx)

	return nil
}

// StopClientCore stops the client proxy engine and terminates Xray gracefully
func StopClientCore() error {
	clientMu.Lock()
	clientShouldRun = false
	if clientCancel != nil {
		clientCancel()
	}
	clientMu.Unlock()

	// Stop wrappers
	StopLocalProxyEngine()

	// Stop process
	clientMu.Lock()
	defer clientMu.Unlock()

	if !clientRunning {
		return nil
	}

	if clientCmd != nil && clientCmd.Process != nil {
		logger.Info("ClientV2Ray", "Terminating client Xray core process")
		pgid, err := syscall.Getpgid(clientCmd.Process.Pid)
		if err == nil {
			_ = syscall.Kill(-pgid, syscall.SIGTERM)
		} else {
			_ = clientCmd.Process.Signal(syscall.SIGTERM)
		}

		done := make(chan error, 1)
		go func() {
			done <- clientCmd.Wait()
		}()

		select {
		case <-done:
		case <-time.After(3 * time.Second):
			logger.Warn("ClientV2Ray", "Client core did not exit, sending SIGKILL")
			if err == nil {
				_ = syscall.Kill(-pgid, syscall.SIGKILL)
			} else {
				_ = clientCmd.Process.Kill()
			}
			<-done
		}
		clientCmd = nil
	}

	clientRunning = false
	logger.Info("ClientV2Ray", "Client proxy engine stopped successfully")
	return nil
}

// RestartClientCore restarts the client engine
func RestartClientCore() error {
	path := ""
	clientMu.Lock()
	path = clientConfigPath
	clientMu.Unlock()

	if path == "" {
		return fmt.Errorf("no active client configuration to restart")
	}

	_ = StopClientCore()
	return StartClientCore(path)
}

func clientSupervisor(ctx context.Context) {
	binPath, err := GetClientBinPath()
	if err != nil {
		logger.Error("ClientV2Ray", "Failed to resolve core binary path", "error", err)
		clientMu.Lock()
		clientErr = err
		clientRunning = false
		clientMu.Unlock()
		return
	}
	if abs, err := filepath.Abs(binPath); err == nil {
		binPath = abs
	}

	for {
		clientMu.Lock()
		if !clientShouldRun {
			clientMu.Unlock()
			return
		}
		clientMu.Unlock()

		clientLastStart = time.Now()
		runCtx, runCancel := context.WithCancel(ctx)

		cmd := exec.CommandContext(runCtx, binPath, "run", "-c", clientConfigPath)
		cmd.SysProcAttr = &syscall.SysProcAttr{Setpgid: true}
		absBinDir, err := filepath.Abs(filepath.Dir(binPath))
		if err == nil {
			cmd.Dir = absBinDir
		}

		// Pipe stdout & stderr to read logs in real time
		stdout, errStdout := cmd.StdoutPipe()
		stderr, errStderr := cmd.StderrPipe()

		clientMu.Lock()
		clientCmd = cmd
		clientMu.Unlock()

		logger.Info("ClientV2Ray", "Supervisor starting client core process", "bin", binPath)
		if err := cmd.Start(); err != nil {
			runCancel()
			logger.Error("ClientV2Ray", "Failed to start supervisor client process", "error", err)
		} else {
			// Scan stdout and stderr asynchronously
			var pipeWG sync.WaitGroup
			if errStdout == nil {
				pipeWG.Add(1)
				go func() {
					defer pipeWG.Done()
					scanner := bufio.NewScanner(stdout)
					for scanner.Scan() {
						AddClientLog(scanner.Text())
					}
				}()
			}
			if errStderr == nil {
				pipeWG.Add(1)
				go func() {
					defer pipeWG.Done()
					scanner := bufio.NewScanner(stderr)
					for scanner.Scan() {
						AddClientLog(scanner.Text())
					}
				}()
			}

			_ = cmd.Wait()
			pipeWG.Wait()
		}
		runCancel()

		clientMu.Lock()
		if !clientShouldRun {
			clientMu.Unlock()
			return
		}

		// Calculate backoff
		uptime := time.Since(clientLastStart)
		if uptime > 30*time.Second {
			clientCrashes = 0
		} else {
			clientCrashes++
		}

		backoff := time.Duration(1<<uint(clientCrashes)) * time.Second
		if backoff > 45*time.Second {
			backoff = 45 * time.Second
		}

		logger.Warn("ClientV2Ray", "Client core crashed, restarting", "backoff", backoff, "consecutive_crashes", clientCrashes)
		clientMu.Unlock()

		select {
		case <-ctx.Done():
			return
		case <-time.After(backoff):
		}
	}
}

// StartLocalProxyEngine sets up local listeners to proxy connections to internal ports
func StartLocalProxyEngine(socksPublic, socksInternal, httpPublic, httpInternal int) {
	proxyMu.Lock()
	if proxyRunning {
		proxyMu.Unlock()
		return
	}
	proxyRunning = true
	proxyStopChan = make(chan struct{})
	proxyMu.Unlock()

	// Toggle OS system proxy if enabled in settings
	sysProxyEnabled := false
	if db.DB != nil {
		var setting models.V2RayClientSetting
		if err := db.DB.Where("key = ?", "sys_proxy_enabled").First(&setting).Error; err == nil {
			sysProxyEnabled = setting.Value == "true"
		}
	}
	if sysProxyEnabled {
		logger.Info("ClientProxy", "Setting OS system proxy", "socksPort", socksPublic, "httpPort", httpPublic)
		_ = sysproxy.SetSystemProxy(socksPublic, httpPublic)
	}

	// Start SOCKS5 Local Proxy Wrapper
	go func() {
		addr := net.JoinHostPort("127.0.0.1", strconv.Itoa(socksPublic))
		l, err := net.Listen("tcp", addr)
		if err != nil {
			logger.Error("ClientProxy", "Failed to start local SOCKS5 wrapper listener", "error", err)
			return
		}
		socksListener = l
		logger.Info("ClientProxy", "Local SOCKS5 wrapper engine listening", "public", socksPublic, "internal", socksInternal)

		for {
			conn, err := l.Accept()
			if err != nil {
				return
			}
			go handleProxyConn(conn, net.JoinHostPort("127.0.0.1", strconv.Itoa(socksInternal)), true)
		}
	}()

	// Start HTTP Local Proxy Wrapper
	go func() {
		addr := net.JoinHostPort("127.0.0.1", strconv.Itoa(httpPublic))
		l, err := net.Listen("tcp", addr)
		if err != nil {
			logger.Error("ClientProxy", "Failed to start local HTTP wrapper listener", "error", err)
			return
		}
		httpListener = l
		logger.Info("ClientProxy", "Local HTTP wrapper engine listening", "public", httpPublic, "internal", httpInternal)

		for {
			conn, err := l.Accept()
			if err != nil {
				return
			}
			go handleProxyConn(conn, net.JoinHostPort("127.0.0.1", strconv.Itoa(httpInternal)), false)
		}
	}()

	// Start metrics ticker
	go metricsReporter()
}

// StopLocalProxyEngine terminates listeners and waits for in-flight conns to complete (graceful shutdown)
func StopLocalProxyEngine() {
	proxyMu.Lock()
	if !proxyRunning {
		proxyMu.Unlock()
		return
	}
	proxyRunning = false

	// Always clear OS system proxy on stop
	logger.Info("ClientProxy", "Clearing OS system proxy")
	_ = sysproxy.ClearSystemProxy()

	if socksListener != nil {
		_ = socksListener.Close()
		socksListener = nil
	}
	if httpListener != nil {
		_ = httpListener.Close()
		httpListener = nil
	}
	if proxyStopChan != nil {
		close(proxyStopChan)
		proxyStopChan = nil
	}
	proxyMu.Unlock()

	// Wait for in-flight connections to complete (graceful shutdown)
	done := make(chan struct{})
	go func() {
		proxyWG.Wait()
		close(done)
	}()

	select {
	case <-done:
		logger.Info("ClientProxy", "Graceful shutdown of local wrapper proxy engine complete")
	case <-time.After(3 * time.Second):
		logger.Warn("ClientProxy", "Graceful shutdown timed out, cutting remaining active connections")
	}
}

func handleProxyConn(src net.Conn, target string, isSocks bool) {
	// Concurrent Connection Limiter
	conns := atomic.LoadInt32(&activeConns)
	if conns >= maxConns {
		logger.Warn("ClientProxy", "Rejecting incoming connection: connection limit exceeded", "limit", maxConns)
		src.Close()
		return
	}

	atomic.AddInt32(&activeConns, 1)
	proxyWG.Add(1)
	defer func() {
		src.Close()
		atomic.AddInt32(&activeConns, -1)
		proxyWG.Done()
	}()

	// Enforce Idle Timeout
	_ = src.SetDeadline(time.Now().Add(idleTimeout))

	if isSocks && desync.IsDesyncEnabled() {
		// Use Desync SOCKS5 Handshake and Injection
		v2rayPortStr := strings.Split(target, ":")[1]
		v2rayPort, _ := strconv.Atoi(v2rayPortStr)
		desync.HandleSOCKS5(src, v2rayPort, func(n int64) {
			atomic.AddInt64(&totalBytesTx, n)
		}, func(n int64) {
			atomic.AddInt64(&totalBytesRx, n)
		})
		return
	}

	dst, err := net.DialTimeout("tcp", target, 4*time.Second)
	if err != nil {
		return
	}
	defer dst.Close()
	_ = dst.SetDeadline(time.Now().Add(idleTimeout))

	// Channel to signal complete
	done := make(chan struct{})

	// Pipe Upload
	go func() {
		buf := make([]byte, 32*1024)
		for {
			_ = src.SetDeadline(time.Now().Add(idleTimeout))
			n, err := src.Read(buf)
			if n > 0 {
				_ = dst.SetDeadline(time.Now().Add(idleTimeout))
				nw, ew := dst.Write(buf[:n])
				if nw > 0 {
					atomic.AddInt64(&totalBytesTx, int64(nw))
				}
				if ew != nil || n != nw {
					break
				}
			}
			if err != nil {
				break
			}
		}
		close(done)
	}()

	// Pipe Download
	buf := make([]byte, 32*1024)
	for {
		select {
		case <-done:
			return
		default:
		}
		_ = dst.SetDeadline(time.Now().Add(idleTimeout))
		n, err := dst.Read(buf)
		if n > 0 {
			_ = src.SetDeadline(time.Now().Add(idleTimeout))
			nw, ew := src.Write(buf[:n])
			if nw > 0 {
				atomic.AddInt64(&totalBytesRx, int64(nw))
			}
			if ew != nil || n != nw {
				break
			}
		}
		if err != nil {
			break
		}
	}
}

func metricsReporter() {
	ticker := time.NewTicker(1 * time.Second)
	defer ticker.Stop()

	for {
		select {
		case <-proxyStopChan:
			return
		case <-ticker.C:
			metrics := ClientProxyMetrics{
				ActiveConnections: atomic.LoadInt32(&activeConns),
				BytesTx:           atomic.LoadInt64(&totalBytesTx),
				BytesRx:           atomic.LoadInt64(&totalBytesRx),
			}
			// Non-blocking write to UI metric channel
			select {
			case MetricsChan <- metrics:
			default:
			}
		}
	}
}

var (
	clientLogs   []string
	clientLogsMu sync.Mutex
)

// AddClientLog appends a new log line, maintaining a maximum buffer size of 500 lines
func AddClientLog(line string) {
	clientLogsMu.Lock()
	defer clientLogsMu.Unlock()
	
	// Add timestamp to log
	timestamped := fmt.Sprintf("[%s] %s", time.Now().Format("2006-01-02 15:04:05"), line)

	if len(clientLogs) >= 500 {
		clientLogs = clientLogs[1:]
	}
	clientLogs = append(clientLogs, timestamped)
}

// GetClientLogs retrieves client logs with optional keyword filtering
func GetClientLogs(keyword string) []string {
	clientLogsMu.Lock()
	defer clientLogsMu.Unlock()

	if keyword == "" {
		copied := make([]string, len(clientLogs))
		copy(copied, clientLogs)
		return copied
	}

	var filtered []string
	kw := strings.ToLower(keyword)
	for _, line := range clientLogs {
		if strings.Contains(strings.ToLower(line), kw) {
			filtered = append(filtered, line)
		}
	}
	return filtered
}
