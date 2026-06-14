// Package hotswap provides runtime outbound addition/removal via the Xray
// HandlerService gRPC API. This allows per-artery swaps without restarting
// the entire core process — critical for maintaining other arteries' connections.
package hotswap

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sync"
	"time"

	"clever-connect/internal/logger"
	"clever-connect/internal/models"
	"clever-connect/internal/v2ray/compiler"

	pxcmd "github.com/xtls/xray-core/app/proxyman/command"
	xcore "github.com/xtls/xray-core/core"
	"github.com/xtls/xray-core/infra/conf/serial"
	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials/insecure"
)

const (
	defaultAPIAddr   = "127.0.0.1:10085"
	dialTimeout      = 3 * time.Second
	operationTimeout = 5 * time.Second
)

// Manager holds a persistent gRPC connection to the running Xray instance.
type Manager struct {
	mu      sync.Mutex
	apiAddr string
	conn    *grpc.ClientConn
	handler pxcmd.HandlerServiceClient
}

// NewManager creates a new hot-swap manager targeting the given gRPC address.
// Call Connect() before using any swap operations.
func NewManager(apiAddr string) *Manager {
	if apiAddr == "" {
		apiAddr = defaultAPIAddr
	}
	return &Manager{apiAddr: apiAddr}
}

// Connect establishes the gRPC connection to the running Xray instance.
func (m *Manager) Connect() error {
	m.mu.Lock()
	defer m.mu.Unlock()

	if m.conn != nil {
		return nil // already connected
	}

	ctx, cancel := context.WithTimeout(context.Background(), dialTimeout)
	defer cancel()

	conn, err := grpc.DialContext(ctx, m.apiAddr,
		grpc.WithTransportCredentials(insecure.NewCredentials()),
		grpc.WithBlock(),
	)
	if err != nil {
		return fmt.Errorf("hotswap: failed to connect to Xray gRPC API at %s: %w", m.apiAddr, err)
	}

	m.conn = conn
	m.handler = pxcmd.NewHandlerServiceClient(conn)

	logger.Info("Bonding", "Hot-swap manager connected to Xray gRPC API", "addr", m.apiAddr)
	return nil
}

// Close shuts down the gRPC connection.
func (m *Manager) Close() {
	m.mu.Lock()
	defer m.mu.Unlock()

	if m.conn != nil {
		m.conn.Close()
		m.conn = nil
		m.handler = nil
	}
}

// IsConnected returns true if the gRPC connection is alive.
func (m *Manager) IsConnected() bool {
	m.mu.Lock()
	defer m.mu.Unlock()
	return m.conn != nil
}

// configToOutboundPB converts a V2RayClientConfig to a protobuf OutboundHandlerConfig
// by compiling to JSON, writing a temp file, and using Xray's own config loader
// for maximum compatibility with all protocols and transport types.
func configToOutboundPB(cfg models.V2RayClientConfig, tag string) (*xcore.OutboundHandlerConfig, error) {
	// Compile to our standard JSON outbound
	outbound := compiler.CompileOutbound(cfg, true, tag)

	// Build a minimal Xray config containing just this outbound
	wrapper := map[string]interface{}{
		"outbounds": []interface{}{outbound},
	}

	configJSON, err := json.MarshalIndent(wrapper, "", "  ")
	if err != nil {
		return nil, fmt.Errorf("marshal wrapper: %w", err)
	}

	// Use Xray's serial.DecodeJSONConfig to parse into protobuf
	tmpDir := filepath.Join(os.TempDir(), "clever-connect-data")
	os.MkdirAll(tmpDir, 0755)
	tmpFile := filepath.Join(tmpDir, fmt.Sprintf("hotswap_%s.json", tag))

	if err := os.WriteFile(tmpFile, configJSON, 0644); err != nil {
		return nil, fmt.Errorf("write temp config: %w", err)
	}
	defer os.Remove(tmpFile)

	// Parse via serial (Xray's standard JSON → protobuf path)
	f, err := os.Open(tmpFile)
	if err != nil {
		return nil, fmt.Errorf("open temp config: %w", err)
	}
	defer f.Close()

	jsonConfig, err := serial.DecodeJSONConfig(f)
	if err != nil {
		return nil, fmt.Errorf("decode JSON config: %w", err)
	}

	pbConfig, err := jsonConfig.Build()
	if err != nil {
		return nil, fmt.Errorf("build protobuf config: %w", err)
	}

	if len(pbConfig.Outbound) == 0 {
		return nil, fmt.Errorf("xray loader produced no outbound handlers")
	}

	return pbConfig.Outbound[0], nil
}

// AddOutbound compiles a V2RayClientConfig into a protobuf OutboundHandlerConfig
// and adds it to the running Xray core via the HandlerService.
func (m *Manager) AddOutbound(cfg models.V2RayClientConfig, tag string) error {
	m.mu.Lock()
	handler := m.handler
	m.mu.Unlock()

	if handler == nil {
		return fmt.Errorf("hotswap: not connected to Xray gRPC API")
	}

	pb, err := configToOutboundPB(cfg, tag)
	if err != nil {
		return fmt.Errorf("hotswap: config conversion failed: %w", err)
	}

	ctx, cancel := context.WithTimeout(context.Background(), operationTimeout)
	defer cancel()

	_, err = handler.AddOutbound(ctx, &pxcmd.AddOutboundRequest{
		Outbound: pb,
	})
	if err != nil {
		return fmt.Errorf("hotswap: AddOutbound(%s) failed: %w", tag, err)
	}

	logger.Info("Bonding", "Hot-swap: outbound added",
		"tag", tag,
		"protocol", cfg.Protocol,
		"address", cfg.Address,
		"port", cfg.Port,
	)
	return nil
}

// RemoveOutbound removes an outbound handler by its tag from the running core.
func (m *Manager) RemoveOutbound(tag string) error {
	m.mu.Lock()
	handler := m.handler
	m.mu.Unlock()

	if handler == nil {
		return fmt.Errorf("hotswap: not connected to Xray gRPC API")
	}

	ctx, cancel := context.WithTimeout(context.Background(), operationTimeout)
	defer cancel()

	_, err := handler.RemoveOutbound(ctx, &pxcmd.RemoveOutboundRequest{
		Tag: tag,
	})
	if err != nil {
		return fmt.Errorf("hotswap: RemoveOutbound(%s) failed: %w", tag, err)
	}

	logger.Info("Bonding", "Hot-swap: outbound removed", "tag", tag)
	return nil
}

// SwapOutbound atomically replaces one outbound with another (Remove + Add).
// If the remove fails (e.g., tag doesn't exist), the add is still attempted.
func (m *Manager) SwapOutbound(oldTag string, newCfg models.V2RayClientConfig, newTag string) error {
	// Remove old (best-effort — it might not exist on first call)
	if err := m.RemoveOutbound(oldTag); err != nil {
		logger.Warn("Bonding", "Hot-swap: remove old outbound failed (may not exist)",
			"tag", oldTag, "error", err)
	}

	// Add new
	if err := m.AddOutbound(newCfg, newTag); err != nil {
		return fmt.Errorf("hotswap: swap failed on add phase: %w", err)
	}

	return nil
}
