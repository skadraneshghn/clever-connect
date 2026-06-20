package warp

import (
	"context"
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/tls"
	"crypto/x509"
	"encoding/base64"
	"fmt"
	"math/big"
	"net"
	"net/http"
	"net/netip"
	"strings"
	"sync"
	"time"

	"clever-connect/internal/db"
	"clever-connect/internal/logger"
	"clever-connect/internal/models"

	connectip "github.com/Diniboy1123/connect-ip-go"
	"github.com/yosida95/uritemplate/v3"
	"golang.org/x/net/http2"
	"golang.zx2c4.com/wireguard/tun/netstack"
)

// LoadMasqueKeys retrieves or generates the P-256 ECDSA key pair for the WARP account.
func LoadMasqueKeys(account *models.WarpAccount) (*ecdsa.PrivateKey, string, error) {
	if account.MasquePrivateKey == "" || account.MasquePublicKey == "" {
		logger.Info("WARP", "Generating new P-256 ECDSA key pair for MASQUE...")
		privKey, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
		if err != nil {
			return nil, "", fmt.Errorf("failed to generate P-256 key: %w", err)
		}

		privBytes, err := x509.MarshalECPrivateKey(privKey)
		if err != nil {
			return nil, "", fmt.Errorf("failed to marshal EC private key: %w", err)
		}
		privB64 := base64.StdEncoding.EncodeToString(privBytes)

		pubBytes, err := x509.MarshalPKIXPublicKey(&privKey.PublicKey)
		if err != nil {
			return nil, "", fmt.Errorf("failed to marshal PKIX public key: %w", err)
		}
		pubB64 := base64.StdEncoding.EncodeToString(pubBytes)

		account.MasquePrivateKey = privB64
		account.MasquePublicKey = pubB64
		account.MasqueActive = false

		if db.DB != nil {
			if err := db.DB.Model(account).Updates(map[string]interface{}{
				"masque_private_key": privB64,
				"masque_public_key":  pubB64,
				"masque_active":      false,
			}).Error; err != nil {
				return nil, "", fmt.Errorf("failed to save MASQUE keys to DB: %w", err)
			}
		}
	}

	privKeyB64, err := base64.StdEncoding.DecodeString(account.MasquePrivateKey)
	if err != nil {
		return nil, "", fmt.Errorf("failed to decode private key base64: %w", err)
	}
	privKey, err := x509.ParseECPrivateKey(privKeyB64)
	if err != nil {
		return nil, "", fmt.Errorf("failed to parse EC private key: %w", err)
	}

	return privKey, account.MasquePublicKey, nil
}

// StartMASQUEH2Transport initializes the MASQUE-over-HTTP/2 CONNECT-IP transport.
func StartMASQUEH2Transport(
	ctx context.Context,
	cfg *models.WarpGlobalConfig,
	account *models.WarpAccount,
	host string,
	port int,
) (dialFn func(ctx context.Context, network, addr string) (net.Conn, error), cancel func(), err error) {

	cfAddr := fmt.Sprintf("%s:%d", host, port)

	// 1. Generate/Load P-256 ECDSA key pair
	privKey, pubKeyB64, err := LoadMasqueKeys(account)
	if err != nil {
		return nil, nil, fmt.Errorf("failed to load/generate MASQUE keys: %w", err)
	}

	// 2. Register/Enroll the key with Cloudflare if not active
	if !account.MasqueActive {
		logger.Info("WARP", "Enrolling MASQUE ECDSA key with Cloudflare API...", "deviceID", account.DeviceID)
		apiClient := NewObfuscatedClient("api.cloudflareclient.com")
		err = apiClient.updateRegistrationKey(account.DeviceID, account.Token, pubKeyB64, "secp256r1", "masque")
		if err != nil {
			return nil, nil, fmt.Errorf("failed to enroll MASQUE key: %w", err)
		}

		account.MasqueActive = true
		if db.DB != nil {
			if err := db.DB.Model(account).Updates(map[string]interface{}{
				"masque_active": true,
			}).Error; err != nil {
				logger.Warn("WARP", "Failed to update masque_active in DB", "error", err)
			}
		}
		logger.Info("WARP", "MASQUE ECDSA key enrolled successfully")
	}

	// 3. Generate self-signed certificate for TLS Client Cert Auth
	logger.Info("WARP", "Generating self-signed client certificate...")
	template := x509.Certificate{
		SerialNumber: big.NewInt(0),
		NotBefore:    time.Now().Add(-24 * time.Hour),
		NotAfter:     time.Now().Add(365 * 24 * time.Hour),
	}
	certBytes, err := x509.CreateCertificate(rand.Reader, &template, &template, &privKey.PublicKey, privKey)
	if err != nil {
		return nil, nil, fmt.Errorf("failed to generate self-signed cert: %w", err)
	}

	clientCert := tls.Certificate{
		Certificate: [][]byte{certBytes},
		PrivateKey:  privKey,
	}

	// 4. Configure TLS mTLS with client certificate
	tlsConfig := &tls.Config{
		Certificates:       []tls.Certificate{clientCert},
		ServerName:         cfg.TargetSNI,
		NextProtos:         []string{"h2"},
		InsecureSkipVerify: true,
	}

	// 5. Build HTTP/2 mTLS client
	h2Endpoint := &net.TCPAddr{
		IP:   net.ParseIP(host),
		Port: port,
	}

	transport := &http2.Transport{
		DialTLSContext: func(ctx context.Context, network, _ string, _ *tls.Config) (net.Conn, error) {
			dialer := &net.Dialer{}
			conn, err := dialer.DialContext(ctx, network, h2Endpoint.String())
			if err != nil {
				return nil, err
			}

			tlsConn := tls.Client(conn, tlsConfig)
			if err := tlsConn.HandshakeContext(ctx); err != nil {
				_ = conn.Close()
				return nil, err
			}
			return tlsConn, nil
		},
	}

	h2Client := &http.Client{Transport: transport}

	// 6. Parse URI template
	templateURI, err := uritemplate.New("https://cloudflareaccess.com/")
	if err != nil {
		return nil, nil, fmt.Errorf("failed to parse template: %w", err)
	}

	h2Headers := http.Header{}
	h2Headers.Set("cf-connect-proto", "cf-connect-ip")
	h2Headers.Set("pq-enabled", "false")
	h2Headers.Set("User-Agent", "okhttp/3.12.1")

	logger.Info("WARP", "Establishing MASQUE CONNECT-IP tunnel...", "endpoint", cfAddr)
	ipConn, _, err := connectip.DialH2(ctx, h2Client, templateURI, h2Headers)
	if err != nil {
		return nil, nil, fmt.Errorf("failed to dial CONNECT-IP over HTTP/2: %w", err)
	}

	// 7. Setup virtual interfaces/routes for netstack
	ipv4Str := account.AssignedIPv4
	if ipv4Str == "" {
		ipv4Str = "172.16.0.2"
	}
	if idx := strings.IndexByte(ipv4Str, '/'); idx >= 0 {
		ipv4Str = ipv4Str[:idx]
	}
	ipNet := net.ParseIP(ipv4Str)
	if ipNet == nil {
		_ = ipConn.Close()
		return nil, nil, fmt.Errorf("invalid assigned IPv4 %q", ipv4Str)
	}
	localAddr, ok := netip.AddrFromSlice(ipNet.To4())
	if !ok {
		_ = ipConn.Close()
		return nil, nil, fmt.Errorf("cannot convert %q to netip.Addr", ipv4Str)
	}
	tunAddrs := []netip.Addr{localAddr}

	ipv6Str := account.AssignedIPv6
	if ipv6Str != "" {
		if idx := strings.IndexByte(ipv6Str, '/'); idx >= 0 {
			ipv6Str = ipv6Str[:idx]
		}
		ipv6 := net.ParseIP(ipv6Str)
		if ipv6 != nil {
			if addr6, ok := netip.AddrFromSlice(ipv6.To16()); ok {
				tunAddrs = append(tunAddrs, addr6)
			}
		}
	}

	dnsAddrs := []netip.Addr{
		netip.MustParseAddr("1.1.1.1"),
		netip.MustParseAddr("2606:4700:4700::1111"),
	}

	// 8. Create gVisor netstack TUN
	tunDev, tnet, err := netstack.CreateNetTUN(
		tunAddrs,
		dnsAddrs,
		1280, // wgMTU = 1280
	)
	if err != nil {
		_ = ipConn.Close()
		return nil, nil, fmt.Errorf("netstack TUN creation failed: %w", err)
	}

	// 9. Start packet pumps
	packetBufPool := sync.Pool{
		New: func() interface{} {
			b := make([]byte, 1280+1) // MTU + 1 byte headroom
			return &b
		},
	}

	pumpCtx, cancelPumps := context.WithCancel(ctx)
	errChan := make(chan error, 2)

	// Pump 1: TUN -> CONNECT-IP
	go func() {
		bufs := make([][]byte, 1)
		sizes := make([]int, 1)
		for {
			select {
			case <-pumpCtx.Done():
				return
			default:
			}

			bufPtr := packetBufPool.Get().(*[]byte)
			buf := *bufPtr

			bufs[0] = buf[1:]
			sizes[0] = 0

			_, err := tunDev.Read(bufs, sizes, 0)
			if err != nil {
				packetBufPool.Put(bufPtr)
				if pumpCtx.Err() == nil {
					errChan <- fmt.Errorf("TUN read failed: %w", err)
				}
				return
			}

			packetLen := sizes[0]
			if packetLen > 0 {
				icmp, err := ipConn.WritePacketBuffer(buf, 1, packetLen)
				if err != nil {
					packetBufPool.Put(bufPtr)
					if pumpCtx.Err() == nil {
						errChan <- fmt.Errorf("CONNECT-IP write failed: %w", err)
					}
					return
				}
				if len(icmp) > 0 {
					wBufs := [][]byte{icmp}
					_, _ = tunDev.Write(wBufs, 0)
				}
			}
			packetBufPool.Put(bufPtr)
		}
	}()

	// Pump 2: CONNECT-IP -> TUN
	go func() {
		wBufs := make([][]byte, 1)
		for {
			select {
			case <-pumpCtx.Done():
				return
			default:
			}

			packet, err := ipConn.ReadPacketZeroCopy(true)
			if err != nil {
				if pumpCtx.Err() == nil {
					errChan <- fmt.Errorf("CONNECT-IP read failed: %w", err)
				}
				return
			}

			if len(packet) > 0 {
				wBufs[0] = packet
				_, err = tunDev.Write(wBufs, 0)
				if err != nil {
					if pumpCtx.Err() == nil {
						errChan <- fmt.Errorf("TUN write failed: %w", err)
					}
					return
				}
			}
		}
	}()

	// Watch for errors and log them
	go func() {
		select {
		case <-pumpCtx.Done():
		case e := <-errChan:
			logger.Error("WARP", "MASQUE H2 packet pump error", "error", e)
		}
	}()

	// 10. Probe tunnel connectivity
	logger.Info("WARP", "Probing MASQUE H2 tunnel connectivity...")
	probeCtx, probeCancel := context.WithTimeout(ctx, 5*time.Second)
	defer probeCancel()
	probeConn, probeErr := tnet.DialContext(probeCtx, "tcp", "1.1.1.1:443")
	if probeErr != nil {
		logger.Warn("WARP", "MASQUE H2 tunnel probe failed", "error", probeErr)
	} else {
		probeConn.Close()
		logger.Info("WARP", "MASQUE H2 tunnel probe succeeded — connection is functional")
	}

	dialFn = func(dialCtx context.Context, network, addr string) (net.Conn, error) {
		return tnet.DialContext(dialCtx, network, addr)
	}

	cancel = func() {
		cancelPumps()
		_ = ipConn.Close()
		_ = tunDev.Close()
	}

	return dialFn, cancel, nil
}
