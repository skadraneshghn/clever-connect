package dns

import (
	"bytes"
	"context"
	"crypto/tls"
	"encoding/binary"
	"fmt"
	"io"
	"net"
	"net/http"
	"time"

	"github.com/miekg/dns"
	"github.com/quic-go/quic-go"
)

// QueryUDP queries a DNS server over UDP.
func QueryUDP(ctx context.Context, server string, msg *dns.Msg, timeout time.Duration) (*dns.Msg, time.Duration, error) {
	client := &dns.Client{
		Net:     "udp",
		Timeout: timeout,
	}
	targetAddr := server
	if _, _, err := net.SplitHostPort(server); err != nil {
		targetAddr = net.JoinHostPort(server, "53")
	}
	resp, rtt, err := client.ExchangeContext(ctx, msg, targetAddr)
	return resp, rtt, err
}

// QueryTCP queries a DNS server over TCP.
func QueryTCP(ctx context.Context, server string, msg *dns.Msg, timeout time.Duration) (*dns.Msg, time.Duration, error) {
	t0 := time.Now()
	dialer := &net.Dialer{Timeout: timeout}
	targetAddr := server
	if _, _, err := net.SplitHostPort(server); err != nil {
		targetAddr = net.JoinHostPort(server, "53")
	}
	conn, err := dialer.DialContext(ctx, "tcp", targetAddr)
	if err != nil {
		return nil, 0, err
	}
	defer conn.Close()

	dnsConn := &dns.Conn{Conn: conn}
	_ = dnsConn.SetWriteDeadline(time.Now().Add(timeout))
	if err := dnsConn.WriteMsg(msg); err != nil {
		return nil, 0, err
	}

	_ = dnsConn.SetReadDeadline(time.Now().Add(timeout))
	resp, err := dnsConn.ReadMsg()
	if err != nil {
		return nil, 0, err
	}

	return resp, time.Since(t0), nil
}

// QueryDoT queries a DNS server over DNS-over-TLS (DoT).
func QueryDoT(ctx context.Context, server string, msg *dns.Msg, timeout time.Duration) (*dns.Msg, time.Duration, error) {
	tlsConfig := &tls.Config{
		MinVersion:         tls.VersionTLS13,
		InsecureSkipVerify: true, // Self-signed or private resolv certs bypass validation dynamically
	}
	client := &dns.Client{
		Net:       "tcp-tls",
		Timeout:   timeout,
		TLSConfig: tlsConfig,
	}
	targetAddr := server
	if _, _, err := net.SplitHostPort(server); err != nil {
		targetAddr = net.JoinHostPort(server, "853")
	}
	resp, rtt, err := client.ExchangeContext(ctx, msg, targetAddr)
	return resp, rtt, err
}

// QueryDoH queries a DNS server over DNS-over-HTTPS (DoH).
func QueryDoH(ctx context.Context, server string, msg *dns.Msg, timeout time.Duration) (*dns.Msg, time.Duration, error) {
	rawMsg, err := msg.Pack()
	if err != nil {
		return nil, 0, err
	}

	url := server
	if !bytes.HasPrefix([]byte(url), []byte("http://")) && !bytes.HasPrefix([]byte(url), []byte("https://")) {
		url = fmt.Sprintf("https://%s/dns-query", server)
	}

	req, err := http.NewRequestWithContext(ctx, "POST", url, bytes.NewReader(rawMsg))
	if err != nil {
		return nil, 0, err
	}
	req.Header.Set("Content-Type", "application/dns-message")
	req.Header.Set("Accept", "application/dns-message")

	client := &http.Client{
		Timeout: timeout,
	}

	t0 := time.Now()
	resp, err := client.Do(req)
	if err != nil {
		return nil, 0, err
	}
	defer resp.Body.Close()
	rtt := time.Since(t0)

	if resp.StatusCode != http.StatusOK {
		return nil, 0, fmt.Errorf("bad status code: %d", resp.StatusCode)
	}

	respBody, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, 0, err
	}

	respMsg := new(dns.Msg)
	if err := respMsg.Unpack(respBody); err != nil {
		return nil, 0, err
	}

	return respMsg, rtt, nil
}

// QueryDoQ queries a DNS server over DNS-over-QUIC (DoQ).
func QueryDoQ(ctx context.Context, server string, msg *dns.Msg, timeout time.Duration) (*dns.Msg, time.Duration, error) {
	t0 := time.Now()
	tlsConfig := &tls.Config{
		MinVersion:         tls.VersionTLS13,
		NextProtos:         []string{"doq"},
		InsecureSkipVerify: true,
	}

	quicConfig := &quic.Config{
		HandshakeIdleTimeout: timeout,
		MaxIdleTimeout:       timeout,
	}

	targetAddr := net.JoinHostPort(server, "853")
	// If the server address already contains a port, don't append 853
	if _, _, err := net.SplitHostPort(server); err == nil {
		targetAddr = server
	}

	conn, err := quic.DialAddr(ctx, targetAddr, tlsConfig, quicConfig)
	if err != nil {
		// Try fallback port 784 if 853 fails
		targetAddr = net.JoinHostPort(server, "784")
		conn, err = quic.DialAddr(ctx, targetAddr, tlsConfig, quicConfig)
		if err != nil {
			return nil, 0, err
		}
	}
	defer conn.CloseWithError(0, "")

	stream, err := conn.OpenStreamSync(ctx)
	if err != nil {
		return nil, 0, err
	}
	defer stream.Close()

	rawMsg, err := msg.Pack()
	if err != nil {
		return nil, 0, err
	}

	// DoQ prefix: 2 bytes of length
	lenBuf := make([]byte, 2)
	binary.BigEndian.PutUint16(lenBuf, uint16(len(rawMsg)))

	if _, err := stream.Write(lenBuf); err != nil {
		return nil, 0, err
	}
	if _, err := stream.Write(rawMsg); err != nil {
		return nil, 0, err
	}

	// Read response prefix length
	lenPrefix := make([]byte, 2)
	if _, err := io.ReadFull(stream, lenPrefix); err != nil {
		return nil, 0, err
	}

	respLen := binary.BigEndian.Uint16(lenPrefix)
	respBuf := make([]byte, respLen)
	if _, err := io.ReadFull(stream, respBuf); err != nil {
		return nil, 0, err
	}

	respMsg := new(dns.Msg)
	if err := respMsg.Unpack(respBuf); err != nil {
		return nil, 0, err
	}

	return respMsg, time.Since(t0), nil
}
