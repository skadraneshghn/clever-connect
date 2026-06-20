package warp

// ──────────────────────────────────────────────────────────────────────────────
// Real Userspace WireGuard Transport  (wireguard-go + gVisor netstack)
//
// Cloudflare WARP uses standard WireGuard with one extension:
//   bytes 1-3 of every UDP packet are a "reserved" / "client_id" field used
//   for Cloudflare's billing/routing accounting.
//
// Standard wireguard-go always sets bytes 1-3 to zero (they are reserved in
// the WireGuard spec). Cloudflare's edge expects the client's 3-byte ID there.
//
// Solution: wrap conn.NewDefaultBind() with a reservedBind that:
//   - On Send: sets buf[1..3] = clientID before transmitting
//   - On Receive: zeros buf[1..3] so wireguard-go's parser sees standard packets
//
// The `reserved` field is NOT a UAPI field and must NOT appear in IpcSet().
// ──────────────────────────────────────────────────────────────────────────────

import (
	"context"
	"encoding/base64"
	"fmt"
	"net"
	"net/netip"
	"strings"
	"time"

	"clever-connect/internal/logger"
	"clever-connect/internal/models"

	"golang.zx2c4.com/wireguard/conn"
	"golang.zx2c4.com/wireguard/device"
	"golang.zx2c4.com/wireguard/tun/netstack"
)

const (
	// Cloudflare WARP fallback peer public key — used when PeerPublicKey is
	// absent (accounts registered before the model migration).
	cfWARPFallbackPeerKey = "bmXOC+F1FxEMF9dyiK2H5/1SUtzH0JuVo51h2wPfgyo="

	// MTU deliberately clamped to 1280 bytes.
	// This is the IPv6 minimum MTU and ensures WireGuard packets are never
	// fragmented by restrictive ISPs (Iran national uplink drops fragments).
	wgMTU = 1280
)

// ──────────────────────────────────────────────────────────────────────────────
// reservedBind — wraps conn.Bind to inject Cloudflare's 3 reserved bytes
// ──────────────────────────────────────────────────────────────────────────────

// reservedBind wraps an underlying conn.Bind and injects the 3-byte
// Cloudflare client ID into WireGuard packet bytes [1:4] on every send,
// and clears those bytes on every receive so the WireGuard stack sees
// RFC-compliant packets.
type reservedBind struct {
	conn.Bind
	reserved [3]byte
}

// Send injects reserved bytes into each outgoing WireGuard packet.
// WireGuard packet layout:
//
//	byte 0   : message type (1=initiation, 2=response, 3=cookie, 4=transport)
//	bytes 1-3: reserved — standard WireGuard uses 0x00; Cloudflare uses clientID
//	bytes 4+ : message body
func (b *reservedBind) Send(bufs [][]byte, ep conn.Endpoint) error {
	for _, buf := range bufs {
		if len(buf) >= 4 {
			buf[1] = b.reserved[0]
			buf[2] = b.reserved[1]
			buf[3] = b.reserved[2]
		}
	}
	return b.Bind.Send(bufs, ep)
}

// Open wraps the underlying Open and patches each ReceiveFunc to zero out
// bytes 1-3 of incoming packets (so wireguard-go's parser is happy).
func (b *reservedBind) Open(port uint16) ([]conn.ReceiveFunc, uint16, error) {
	fns, actualPort, err := b.Bind.Open(port)
	if err != nil {
		return nil, 0, err
	}
	patched := make([]conn.ReceiveFunc, len(fns))
	for i, fn := range fns {
		fn := fn // capture
		patched[i] = func(packets [][]byte, sizes []int, eps []conn.Endpoint) (int, error) {
			n, err := fn(packets, sizes, eps)
			// Zero out reserved bytes in every received packet
			for j := 0; j < n; j++ {
				if sizes[j] >= 4 {
					packets[j][1] = 0
					packets[j][2] = 0
					packets[j][3] = 0
				}
			}
			return n, err
		}
	}
	return patched, actualPort, nil
}

// ──────────────────────────────────────────────────────────────────────────────
// Helper functions
// ──────────────────────────────────────────────────────────────────────────────

// reservedBytes decodes the base64 ClientID into a 3-byte array.
// Tries both padded and raw base64 encodings (Cloudflare uses padded StdEncoding).
// Returns [0,0,0] on any error.
func reservedBytes(clientID string) [3]byte {
	if clientID == "" {
		return [3]byte{}
	}
	// Try padded first, then raw
	for _, enc := range []*base64.Encoding{base64.StdEncoding, base64.RawStdEncoding} {
		b, err := enc.DecodeString(clientID)
		if err == nil && len(b) >= 3 {
			return [3]byte{b[0], b[1], b[2]}
		}
	}
	logger.Warn("WARP", "Could not decode ClientID as base64, using zero reserved bytes", "clientID", clientID)
	return [3]byte{}
}

// buildWGIPC generates the wireguard-go UAPI IPC configuration string.
//
// Fix #1: host and port are separate parameters — never concatenated before
// this point — to prevent the "IP:PORT:PORT" duplication bug.
//
// NOTE: "reserved" is intentionally NOT included here; it is handled at the
// packet level inside reservedBind.
func buildWGIPC(account *models.WarpAccount, host string, port int) (string, error) {
	// Decode private key from base64 → raw bytes → hex (UAPI format)
	privRaw, err := base64.StdEncoding.DecodeString(account.PrivateKey)
	if err != nil {
		privRaw, err = base64.RawStdEncoding.DecodeString(account.PrivateKey)
		if err != nil {
			return "", fmt.Errorf("cannot decode private key: %w", err)
		}
	}
	if len(privRaw) != 32 {
		return "", fmt.Errorf("private key must be 32 bytes, got %d", len(privRaw))
	}

	// Decode peer public key
	peerKey := account.PeerPublicKey
	if peerKey == "" {
		peerKey = cfWARPFallbackPeerKey
		logger.Warn("WARP", "PeerPublicKey not stored, using Cloudflare WARP fallback")
	}
	peerRaw, err := base64.StdEncoding.DecodeString(peerKey)
	if err != nil {
		peerRaw, err = base64.RawStdEncoding.DecodeString(peerKey)
		if err != nil {
			return "", fmt.Errorf("cannot decode peer public key: %w", err)
		}
	}
	if len(peerRaw) != 32 {
		return "", fmt.Errorf("peer public key must be 32 bytes, got %d", len(peerRaw))
	}

	// UAPI uses lowercase hex for keys
	privHex := fmt.Sprintf("%x", privRaw)
	peerHex := fmt.Sprintf("%x", peerRaw)

	// Build UAPI string (no "reserved" field — handled by reservedBind)
	lines := []string{
		"private_key=" + privHex,
		"public_key=" + peerHex,
		fmt.Sprintf("endpoint=%s:%d", host, port),
		"allowed_ip=0.0.0.0/0",
		"allowed_ip=::/0",
		"persistent_keepalive_interval=25",
	}

	return strings.Join(lines, "\n"), nil
}

// ──────────────────────────────────────────────────────────────────────────────
// StartWireGuardUserspace — public entry point called by engine.go
// ──────────────────────────────────────────────────────────────────────────────

// StartWireGuardUserspace creates a real userspace WireGuard tunnel and returns
// a DialContext function that routes connections through the gVisor netstack.
//
// Parameters:
//   - ctx       : parent context; cancelling tears down the tunnel
//   - account   : active WARP account (provides keys, clientID, assigned IPs)
//   - host      : Cloudflare edge IP (already split, no port suffix)
//   - port      : Cloudflare edge port (usually 2408 for WireGuard)
//
// Returns:
//   - dialFn    : used as the SOCKS5 server's Dial function
//   - cancel    : call this to tear down the WireGuard device + TUN
//   - err       : non-nil if setup fails
func StartWireGuardUserspace(
	ctx context.Context,
	account *models.WarpAccount,
	host string,
	port int,
) (dialFn func(ctx context.Context, network, addr string) (net.Conn, error), cancel func(), err error) {

	// ── Determine virtual interface address ──────────────────────────────────
	ipv4Str := account.AssignedIPv4
	if ipv4Str == "" {
		ipv4Str = "172.16.0.2"
		logger.Warn("WARP", "AssignedIPv4 not stored; using 172.16.0.2 (re-register account to fix)")
	}
	// Strip CIDR suffix if stored with prefix (e.g. "172.16.0.2/32")
	if idx := strings.IndexByte(ipv4Str, '/'); idx >= 0 {
		ipv4Str = ipv4Str[:idx]
	}

	ip := net.ParseIP(ipv4Str)
	if ip == nil {
		return nil, nil, fmt.Errorf("invalid assigned IPv4 %q", ipv4Str)
	}
	localAddr, ok := netip.AddrFromSlice(ip.To4())
	if !ok {
		return nil, nil, fmt.Errorf("cannot convert %q to netip.Addr", ipv4Str)
	}

	// Use Cloudflare's own DNS resolver inside the tunnel
	dnsAddr := netip.MustParseAddr("1.1.1.1")

	// ── Create gVisor netstack TUN (Fix #3: MTU=1280) ───────────────────────
	tunDev, tnet, err := netstack.CreateNetTUN(
		[]netip.Addr{localAddr},
		[]netip.Addr{dnsAddr},
		wgMTU,
	)
	if err != nil {
		return nil, nil, fmt.Errorf("netstack TUN creation failed: %w", err)
	}

	// ── Decode reserved bytes (Fix #2) ───────────────────────────────────────
	rsv := reservedBytes(account.ClientID)
	logger.Info("WARP", "Reserved bytes decoded",
		"clientID", account.ClientID,
		"bytes", fmt.Sprintf("[%d,%d,%d]", rsv[0], rsv[1], rsv[2]),
	)

	// ── Create reservedBind wrapping the default UDP bind ───────────────────
	underlying := conn.NewDefaultBind()
	bind := &reservedBind{
		Bind:     underlying,
		reserved: rsv,
	}

	// ── Create WireGuard device ──────────────────────────────────────────────
	wgLogger := device.NewLogger(device.LogLevelError, "[wg] ")
	wgDev := device.NewDevice(tunDev, bind, wgLogger)

	// ── Build and apply IPC config (Fix #1: no port dup, Fix #2: no reserved in UAPI) ─
	ipc, err := buildWGIPC(account, host, port)
	if err != nil {
		wgDev.Close() // wgDev.Close() already closes tunDev internally
		return nil, nil, fmt.Errorf("WireGuard IPC build failed: %w", err)
	}

	if err := wgDev.IpcSet(ipc); err != nil {
		wgDev.Close()
		return nil, nil, fmt.Errorf("WireGuard IpcSet failed: %w", err)
	}

	if err := wgDev.Up(); err != nil {
		wgDev.Close()
		return nil, nil, fmt.Errorf("WireGuard device Up() failed: %w", err)
	}

	logger.Info("WARP", "WireGuard userspace device UP",
		"endpoint", fmt.Sprintf("%s:%d", host, port),
		"localIP", ipv4Str,
		"mtu", wgMTU,
		"reserved", fmt.Sprintf("[%d,%d,%d]", rsv[0], rsv[1], rsv[2]),
	)

	// Brief pause for handshake initiation
	time.Sleep(300 * time.Millisecond)

	// ── Return dial function and cancel ─────────────────────────────────────
	dialFn = func(dialCtx context.Context, network, addr string) (net.Conn, error) {
		return tnet.DialContext(dialCtx, network, addr)
	}

	// NOTE: wgDev.Close() internally calls tunDev.Close() — do NOT call
	// tunDev.Close() separately or it will panic with "close of closed channel".
	cancel = func() {
		wgDev.Close()
	}

	return dialFn, cancel, nil
}
