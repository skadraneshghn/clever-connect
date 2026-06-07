package core

import (
	"net"
	"strconv"
	"testing"
	"time"
)

func TestPortHelpers(t *testing.T) {
	// Find a free port
	port1 := FindAvailablePort(28200)
	if port1 < 28200 {
		t.Fatalf("Expected port >= 28200, got %d", port1)
	}

	// Bind to that port to make it busy
	addr := net.JoinHostPort("127.0.0.1", strconv.Itoa(port1))
	l, err := net.Listen("tcp", addr)
	if err != nil {
		t.Fatalf("Failed to listen on port %d: %v", port1, err)
	}
	defer l.Close()

	// Check if port is not available anymore
	if CheckPortAvailable(port1) {
		t.Fatalf("Port %d should be reported as busy", port1)
	}

	// Find the next available port, which should be port1 + 1 (or more)
	port2 := FindAvailablePort(port1)
	if port2 <= port1 {
		t.Fatalf("Expected next available port > %d, got %d", port1, port2)
	}
}

func TestLocalProxyWrapperGracefulShutdown(t *testing.T) {
	socksPublic := FindAvailablePort(28300)
	socksInternal := FindAvailablePort(socksPublic + 10)
	httpPublic := FindAvailablePort(socksInternal + 10)
	httpInternal := FindAvailablePort(httpPublic + 10)

	// Mock internal servers
	socksMockAddr := net.JoinHostPort("127.0.0.1", strconv.Itoa(socksInternal))
	socksMockListener, err := net.Listen("tcp", socksMockAddr)
	if err != nil {
		t.Fatalf("Failed to start mock SOCKS internal listener: %v", err)
	}
	defer socksMockListener.Close()

	go func() {
		for {
			conn, err := socksMockListener.Accept()
			if err != nil {
				return
			}
			go func(c net.Conn) {
				defer c.Close()
				buf := make([]byte, 1024)
				n, err := c.Read(buf)
				if err == nil && n > 0 {
					_, _ = c.Write([]byte("socks-response-" + string(buf[:n])))
				}
			}(conn)
		}
	}()

	// Start local proxy engine
	StartLocalProxyEngine(socksPublic, socksInternal, httpPublic, httpInternal)
	time.Sleep(50 * time.Millisecond)

	// Connect to SOCKS public wrapper port
	clientConn, err := net.Dial("tcp", net.JoinHostPort("127.0.0.1", strconv.Itoa(socksPublic)))
	if err != nil {
		t.Fatalf("Failed to connect to SOCKS public wrapper: %v", err)
	}
	defer clientConn.Close()

	// Write request
	requestData := "hello-socks"
	_, err = clientConn.Write([]byte(requestData))
	if err != nil {
		t.Fatalf("Failed to write request: %v", err)
	}

	// Read response
	responseBuf := make([]byte, 1024)
	_ = clientConn.SetReadDeadline(time.Now().Add(2 * time.Second))
	n, err := clientConn.Read(responseBuf)
	if err != nil {
		t.Fatalf("Failed to read response: %v", err)
	}

	expectedResponse := "socks-response-" + requestData
	if string(responseBuf[:n]) != expectedResponse {
		t.Fatalf("Expected response %q, got %q", expectedResponse, string(responseBuf[:n]))
	}

	// Stop Local Proxy Engine (verify graceful shutdown does not hang)
	StopLocalProxyEngine()
}
