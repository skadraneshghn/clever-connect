package scanner

import (
	"fmt"
	"io"
	"net"
	"net/http"
	"sync"
	"time"
)

var (
	debugProxyServer *http.Server
	debugProxyMu     sync.Mutex
	debugProxyLogs   []string
	debugProxyLogsMu sync.Mutex
)

// AddProxyLog records an intercepted connection log
func AddProxyLog(line string) {
	debugProxyLogsMu.Lock()
	defer debugProxyLogsMu.Unlock()
	if len(debugProxyLogs) >= 500 {
		debugProxyLogs = debugProxyLogs[1:]
	}
	debugProxyLogs = append(debugProxyLogs, fmt.Sprintf("[%s] %s", time.Now().Format("15:04:05"), line))
}

// GetProxyLogs retrieves all intercepted connection logs
func GetProxyLogs() []string {
	debugProxyLogsMu.Lock()
	defer debugProxyLogsMu.Unlock()
	copied := make([]string, len(debugProxyLogs))
	copy(copied, debugProxyLogs)
	return copied
}

// ClearProxyLogs clears the log buffer
func ClearProxyLogs() {
	debugProxyLogsMu.Lock()
	defer debugProxyLogsMu.Unlock()
	debugProxyLogs = nil
}

// StartDebugProxy starts an interception forward proxy on the specified local port
func StartDebugProxy(port int) error {
	debugProxyMu.Lock()
	defer debugProxyMu.Unlock()

	if debugProxyServer != nil {
		return fmt.Errorf("debug proxy is already running")
	}

	handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		AddProxyLog(fmt.Sprintf("%s -> %s %s", r.RemoteAddr, r.Method, r.Host))

		if r.Method == http.MethodConnect {
			// Tunneling (HTTPS CONNECT)
			destConn, err := net.DialTimeout("tcp", r.Host, 5*time.Second)
			if err != nil {
				http.Error(w, err.Error(), http.StatusServiceUnavailable)
				return
			}
			w.WriteHeader(http.StatusOK)
			hijacker, ok := w.(http.Hijacker)
			if !ok {
				destConn.Close()
				http.Error(w, "Hijacking not supported", http.StatusInternalServerError)
				return
			}
			clientConn, _, err := hijacker.Hijack()
			if err != nil {
				destConn.Close()
				return
			}
			
			go func() {
				defer clientConn.Close()
				defer destConn.Close()
				_, _ = io.Copy(destConn, clientConn)
			}()
			go func() {
				defer clientConn.Close()
				defer destConn.Close()
				_, _ = io.Copy(clientConn, destConn)
			}()
			return
		}

		// Normal HTTP Request forwarding
		req, err := http.NewRequest(r.Method, r.URL.String(), r.Body)
		if err != nil {
			http.Error(w, err.Error(), http.StatusBadRequest)
			return
		}
		// Copy headers
		for k, vv := range r.Header {
			for _, v := range vv {
				req.Header.Add(k, v)
			}
		}

		tr := &http.Transport{}
		resp, err := tr.RoundTrip(req)
		if err != nil {
			http.Error(w, err.Error(), http.StatusBadGateway)
			return
		}
		defer resp.Body.Close()

		// Copy response headers
		for k, vv := range resp.Header {
			for _, v := range vv {
				w.Header().Add(k, v)
			}
		}
		w.WriteHeader(resp.StatusCode)
		_, _ = io.Copy(w, resp.Body)
	})

	server := &http.Server{
		Addr:    fmt.Sprintf("127.0.0.1:%d", port),
		Handler: handler,
	}

	debugProxyServer = server

	go func() {
		_ = server.ListenAndServe()
	}()

	AddProxyLog(fmt.Sprintf("Debug proxy started on port %d", port))
	return nil
}

// StopDebugProxy stops the running proxy
func StopDebugProxy() error {
	debugProxyMu.Lock()
	defer debugProxyMu.Unlock()

	if debugProxyServer == nil {
		return nil
	}

	_ = debugProxyServer.Close()
	debugProxyServer = nil
	AddProxyLog("Debug proxy stopped")
	return nil
}

// IsDebugProxyRunning checks if proxy is running
func IsDebugProxyRunning() bool {
	debugProxyMu.Lock()
	defer debugProxyMu.Unlock()
	return debugProxyServer != nil
}
