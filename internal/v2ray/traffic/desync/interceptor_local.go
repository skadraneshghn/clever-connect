package desync

import (
	"encoding/binary"
	"fmt"
	"io"
	"net"
	"sync"
	"time"

	"clever-connect/internal/logger"
)

var (
	desyncEngine *DesyncEngine
	mu           sync.Mutex
	socksLis     net.Listener
)

func UpdateDesyncConfig(args string, enabled bool) {
	mu.Lock()
	defer mu.Unlock()

	cfg := ParseCLIArgs(args)
	cfg.Enabled = enabled
	desyncEngine = NewDesyncEngine(cfg)
	logger.Info("Desync", "Updated DPI Evasion Engine config")
}

func IsDesyncEnabled() bool {
	mu.Lock()
	defer mu.Unlock()
	return desyncEngine != nil && desyncEngine.Config.Enabled
}

// StartLocalSOCKSInterceptor starts a local SOCKS5 server that wraps V2Ray's SOCKS port
func StartLocalSOCKSInterceptor(listenPort int, v2raySocksPort int) error {
	mu.Lock()
	defer mu.Unlock()

	if socksLis != nil {
		socksLis.Close()
	}

	addr := fmt.Sprintf("127.0.0.1:%d", listenPort)
	l, err := net.Listen("tcp", addr)
	if err != nil {
		return err
	}
	socksLis = l

	logger.Info("Desync", fmt.Sprintf("Started Local SOCKS5 Interceptor on %s -> V2Ray:%d", addr, v2raySocksPort))

	go func() {
		for {
			conn, err := l.Accept()
			if err != nil {
				return
			}
			go HandleSOCKS5(conn, v2raySocksPort, nil, nil)
		}
	}()

	return nil
}

func HandleSOCKS5(clientConn net.Conn, v2rayPort int, addTx func(int64), addRx func(int64)) {
	defer clientConn.Close()

	// Read SOCKS5 handshake
	buf := make([]byte, 256)
	n, err := io.ReadFull(clientConn, buf[:2])
	if err != nil || n < 2 || buf[0] != 0x05 {
		return
	}

	numMethods := int(buf[1])
	_, err = io.ReadFull(clientConn, buf[:numMethods])
	if err != nil {
		return
	}

	// Send auth success
	clientConn.Write([]byte{0x05, 0x00})

	// Read connect request
	n, err = io.ReadFull(clientConn, buf[:4])
	if err != nil || n < 4 || buf[1] != 0x01 {
		return
	}

	var targetIP net.IP
	var targetHost string
	atyp := buf[3]

	reqBuf := []byte{0x05, 0x01, 0x00, atyp}

	switch atyp {
	case 0x01: // IPv4
		ipBuf := make([]byte, 4)
		io.ReadFull(clientConn, ipBuf)
		targetIP = net.IP(ipBuf)
		reqBuf = append(reqBuf, ipBuf...)
	case 0x03: // Domain
		lenBuf := make([]byte, 1)
		io.ReadFull(clientConn, lenBuf)
		domainLen := int(lenBuf[0])
		domainBuf := make([]byte, domainLen)
		io.ReadFull(clientConn, domainBuf)
		targetHost = string(domainBuf)
		reqBuf = append(reqBuf, lenBuf[0])
		reqBuf = append(reqBuf, domainBuf...)
	case 0x04: // IPv6
		ipBuf := make([]byte, 16)
		io.ReadFull(clientConn, ipBuf)
		targetIP = net.IP(ipBuf)
		reqBuf = append(reqBuf, ipBuf...)
	default:
		return
	}

	portBuf := make([]byte, 2)
	io.ReadFull(clientConn, portBuf)
	targetPort := binary.BigEndian.Uint16(portBuf)
	reqBuf = append(reqBuf, portBuf...)

	if targetIP == nil && targetHost != "" {
		ips, err := net.LookupIP(targetHost)
		if err == nil && len(ips) > 0 {
			targetIP = ips[0]
		}
	}

	// Handle Traffic using the engine
	if targetIP != nil {
		mu.Lock()
		engine := desyncEngine
		mu.Unlock()

		if engine != nil && engine.Config.Enabled {
			// Only inject for specific ports like 443, 80
			for _, p := range engine.Config.TargetPorts {
				if int(targetPort) == p {
					HandleLocalTraffic(clientConn, targetIP, targetPort, engine)
					break
				}
			}
		}
	}

	// Connect to V2Ray
	v2rayConn, err := net.DialTimeout("tcp", fmt.Sprintf("127.0.0.1:%d", v2rayPort), 5*time.Second)
	if err != nil {
		return
	}
	defer v2rayConn.Close()

	// Forward SOCKS5 request to V2Ray
	v2rayConn.Write([]byte{0x05, 0x01, 0x00}) // Client hello
	io.ReadFull(v2rayConn, buf[:2])           // V2ray hello reply

	v2rayConn.Write(reqBuf) // Connect req

	// Read V2ray reply
	replyBuf := make([]byte, 256)
	v2rayConn.Read(replyBuf)

	// Send reply to client
	clientConn.Write(replyBuf)

	// Channel to signal complete
	done := make(chan struct{})

	// Pipe Upload
	go func() {
		buf := make([]byte, 32*1024)
		for {
			n, err := clientConn.Read(buf)
			if n > 0 {
				nw, ew := v2rayConn.Write(buf[:n])
				if nw > 0 && addTx != nil {
					addTx(int64(nw))
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
	bufPipe := make([]byte, 32*1024)
	for {
		select {
		case <-done:
			return
		default:
		}
		n, err := v2rayConn.Read(bufPipe)
		if n > 0 {
			nw, ew := clientConn.Write(bufPipe[:n])
			if nw > 0 && addRx != nil {
				addRx(int64(nw))
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

// HandleLocalTraffic intercepts traffic before sending to V2Ray
func HandleLocalTraffic(clientConn net.Conn, targetIP net.IP, targetPort uint16, engine *DesyncEngine) {
	localAddr, ok := clientConn.LocalAddr().(*net.TCPAddr)
	if !ok {
		return
	}

	go func() {
		engine.InjectFakePacket(localAddr.IP, targetIP, uint16(localAddr.Port), targetPort, 123456)
	}()
}
