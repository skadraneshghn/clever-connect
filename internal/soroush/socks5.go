package soroush

import (
	"context"
	"encoding/binary"
	"fmt"
	"io"
	"net"

	"clever-connect/internal/logger"

	"github.com/quic-go/quic-go"
)

// SOCKS5 protocol constants
const (
	socks5Version  = 0x05
	socks5AuthNone = 0x00
	socks5CmdConn  = 0x01
	socks5AtypIPv4 = 0x01
	socks5AtypDom  = 0x03
	socks5AtypIPv6 = 0x04
)

// StartSOCKS5Listener starts a local SOCKS5 proxy server on the given port.
// Each incoming connection is authenticated, then a QUIC stream is opened
// from the session, and traffic is bidirectionally proxied.
func StartSOCKS5Listener(ctx context.Context, port int, quicConn *quic.Conn) error {
	addr := fmt.Sprintf("127.0.0.1:%d", port)

	lc := &net.ListenConfig{}
	listener, err := lc.Listen(ctx, "tcp", addr)
	if err != nil {
		return fmt.Errorf("socks5 listen: %w", err)
	}
	defer listener.Close()

	logger.Info(component, "SOCKS5 proxy listening",
		"address", addr,
	)

	go func() {
		<-ctx.Done()
		listener.Close()
	}()

	for {
		conn, err := listener.Accept()
		if err != nil {
			select {
			case <-ctx.Done():
				return ctx.Err()
			default:
				logger.Warn(component, "SOCKS5 accept error", "error", err)
				continue
			}
		}

		go handleSOCKS5(ctx, conn, quicConn)
	}
}

// handleSOCKS5 handles a single SOCKS5 client connection.
func handleSOCKS5(ctx context.Context, conn net.Conn, quicConn *quic.Conn) {
	defer conn.Close()

	// 1. Auth negotiation + CONNECT request parsing
	target, err := socks5Handshake(conn)
	if err != nil {
		logger.Debug(component, "SOCKS5 handshake failed", "error", err)
		return
	}

	// 2. Send success reply to SOCKS5 client
	socks5Reply(conn, 0x00) // succeeded

	// 3. Open a QUIC stream and relay traffic
	dialQuicStream(ctx, quicConn, target, conn)
}

// socks5Handshake performs the SOCKS5 handshake and returns the target address.
func socks5Handshake(conn net.Conn) (string, error) {
	// Read version + nmethods
	buf := make([]byte, 2)
	if _, err := io.ReadFull(conn, buf); err != nil {
		return "", fmt.Errorf("read version: %w", err)
	}
	if buf[0] != socks5Version {
		return "", fmt.Errorf("unsupported SOCKS version: %d", buf[0])
	}

	nMethods := int(buf[1])
	methods := make([]byte, nMethods)
	if _, err := io.ReadFull(conn, methods); err != nil {
		return "", fmt.Errorf("read methods: %w", err)
	}

	// Reply: no auth required
	if _, err := conn.Write([]byte{socks5Version, socks5AuthNone}); err != nil {
		return "", fmt.Errorf("write auth reply: %w", err)
	}

	// Read request: VER CMD RSV ATYP DST.ADDR DST.PORT
	header := make([]byte, 4)
	if _, err := io.ReadFull(conn, header); err != nil {
		return "", fmt.Errorf("read request header: %w", err)
	}
	if header[0] != socks5Version {
		return "", fmt.Errorf("request: wrong version %d", header[0])
	}
	if header[1] != socks5CmdConn {
		return "", fmt.Errorf("unsupported command: %d", header[1])
	}

	// Parse target address
	var host string
	switch header[3] {
	case socks5AtypIPv4:
		addr := make([]byte, 4)
		if _, err := io.ReadFull(conn, addr); err != nil {
			return "", fmt.Errorf("read IPv4: %w", err)
		}
		host = net.IP(addr).String()

	case socks5AtypDom:
		domLen := make([]byte, 1)
		if _, err := io.ReadFull(conn, domLen); err != nil {
			return "", fmt.Errorf("read domain length: %w", err)
		}
		domain := make([]byte, domLen[0])
		if _, err := io.ReadFull(conn, domain); err != nil {
			return "", fmt.Errorf("read domain: %w", err)
		}
		host = string(domain)

	case socks5AtypIPv6:
		addr := make([]byte, 16)
		if _, err := io.ReadFull(conn, addr); err != nil {
			return "", fmt.Errorf("read IPv6: %w", err)
		}
		host = net.IP(addr).String()

	default:
		return "", fmt.Errorf("unsupported address type: %d", header[3])
	}

	// Read port (2 bytes, big-endian)
	portBuf := make([]byte, 2)
	if _, err := io.ReadFull(conn, portBuf); err != nil {
		return "", fmt.Errorf("read port: %w", err)
	}
	port := binary.BigEndian.Uint16(portBuf)

	return fmt.Sprintf("%s:%d", host, port), nil
}

// socks5Reply sends a SOCKS5 reply to the client.
func socks5Reply(conn net.Conn, rep byte) {
	// VER REP RSV ATYP BND.ADDR BND.PORT
	reply := []byte{socks5Version, rep, 0x00, socks5AtypIPv4, 0, 0, 0, 0, 0, 0}
	conn.Write(reply)
}
