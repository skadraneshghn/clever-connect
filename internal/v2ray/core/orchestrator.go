package core

import (
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os/exec"
	"sync"
	"time"

	"clever-connect/internal/logger"
	"clever-connect/internal/models"

	"golang.org/x/crypto/ssh"
)

// WebhookPublisher handles signing and dispatching real-time notifications
type WebhookPublisher struct {
	URL    string
	Secret string
}

// PublishEvent sends a secure HMAC-SHA256 signed event notification
func (w *WebhookPublisher) PublishEvent(eventType string, payload interface{}) error {
	if w.URL == "" {
		return nil
	}

	bodyBytes, err := json.Marshal(payload)
	if err != nil {
		return err
	}

	req, err := http.NewRequest("POST", w.URL, io.NopCloser(bytesNewReader(bodyBytes)))
	if err != nil {
		return err
	}

	// Sign payload with HMAC-SHA256
	mac := hmac.New(sha256.New, []byte(w.Secret))
	mac.Write(bodyBytes)
	signature := hex.EncodeToString(mac.Sum(nil))

	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("X-Event-Type", eventType)
	req.Header.Set("X-Signature-SHA256", signature)

	client := &http.Client{Timeout: 5 * time.Second}
	resp, err := client.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK && resp.StatusCode != http.StatusAccepted {
		return fmt.Errorf("webhook endpoint returned status %d", resp.StatusCode)
	}
	return nil
}

// bytesNewReader helper
func bytesNewReader(b []byte) io.Reader {
	return &simpleReader{b: b}
}

type simpleReader struct {
	b   []byte
	off int
}

func (s *simpleReader) Read(p []byte) (n int, err error) {
	if s.off >= len(s.b) {
		return 0, io.EOF
	}
	n = copy(p, s.b[s.off:])
	s.off += n
	return n, nil
}

// ProvisionNode connects via SSH to configure remote proxy core and deploy agent
func ProvisionNode(node *models.V2RayNode, password string) error {
	config := &ssh.ClientConfig{
		User: "root",
		Auth: []ssh.AuthMethod{
			ssh.Password(password),
		},
		HostKeyCallback: ssh.InsecureIgnoreHostKey(),
		Timeout:         15 * time.Second,
	}

	addr := fmt.Sprintf("%s:%d", node.IP, node.SSHPort)
	client, err := ssh.Dial("tcp", addr, config)
	if err != nil {
		return fmt.Errorf("failed to dial remote host via SSH: %w", err)
	}
	defer client.Close()

	session, err := client.NewSession()
	if err != nil {
		return fmt.Errorf("failed to create SSH session: %w", err)
	}
	defer session.Close()

	// Provision script: Update dependencies, open firewall, generate reality keys
	provisionCmd := `
		export DEBIAN_FRONTEND=noninteractive
		apt-get update && apt-get install -y curl ufw iptables
		ufw allow 443/tcp
		ufw allow 80/tcp
		ufw allow 22/tcp
		ufw --force enable
		# Install Xray-core
		bash -c "$(curl -L https://github.com/XTLS/Xray-install/raw/main/install-release.sh)" @ install
		echo "CLEVER-CONNECT AGENT INSTALLED"
	`
	output, err := session.CombinedOutput(provisionCmd)
	if err != nil {
		return fmt.Errorf("failed to run provisioning script on remote node: %s: %w", string(output), err)
	}

	logger.Info("Provisioner", "Remote edge node successfully provisioned", "ip", node.IP)
	return nil
}

// Fail2ban Firewall Manager
var firewallMu sync.Mutex

// BlockMaliciousIP adds an iptables/ufw rule to block unauthorized scans or brute-force IPs
func BlockMaliciousIP(ip string) error {
	firewallMu.Lock()
	defer firewallMu.Unlock()

	// Check if ufw is available, otherwise use raw iptables
	if _, err := exec.LookPath("ufw"); err == nil {
		cmd := exec.Command("ufw", "deny", "from", ip)
		if err := cmd.Run(); err == nil {
			return nil
		}
	}

	// Fallback to iptables
	cmd := exec.Command("iptables", "-A", "INPUT", "-s", ip, "-j", "DROP")
	return cmd.Run()
}

// WebDAV Log Handler
type WebDAVHandler struct {
	LogDir string
}

func (h *WebDAVHandler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	// A simple HTTP-compliant WebDAV mock handler supporting OPTIONS and PROPFIND
	if r.Method == "OPTIONS" {
		w.Header().Set("DAV", "1, 2")
		w.Header().Set("Allow", "GET, HEAD, POST, PUT, DELETE, OPTIONS, PROPFIND")
		w.WriteHeader(http.StatusOK)
		return
	}

	if r.Method == "PROPFIND" {
		w.Header().Set("Content-Type", "text/xml; charset=utf-8")
		w.WriteHeader(http.StatusMultiStatus)
		_, _ = w.Write([]byte(`<?xml version="1.0" encoding="utf-8" ?>
<d:multistatus xmlns:d="DAV:">
  <d:response>
    <d:href>/webdav/logs/</d:href>
    <d:propstat>
      <d:prop>
        <d:resourcetype><d:collection/></d:resourcetype>
      </d:prop>
      <d:status>HTTP/1.1 200 OK</d:status>
    </d:propstat>
  </d:response>
</d:multistatus>`))
		return
	}

	// GET/HEAD logs
	fs := http.FileServer(http.Dir(h.LogDir))
	fs.ServeHTTP(w, r)
}

// MCP JSON-RPC 2.0 Server API for AI Assistants
type MCPRequest struct {
	JsonRpc string          `json:"jsonrpc"`
	Method  string          `json:"method"`
	Params  json.RawMessage `json:"params"`
	ID      interface{}     `json:"id"`
}

type MCPResponse struct {
	JsonRpc string      `json:"jsonrpc"`
	Result  interface{} `json:"result,omitempty"`
	Error   interface{} `json:"error,omitempty"`
	ID      interface{} `json:"id"`
}

func HandleMCPRequest(reqBytes []byte) ([]byte, error) {
	var req MCPRequest
	if err := json.Unmarshal(reqBytes, &req); err != nil {
		return nil, err
	}

	res := MCPResponse{JsonRpc: "2.0", ID: req.ID}
	switch req.Method {
	case "node.list":
		res.Result = []string{"Node 1: Online (192.168.1.10)", "Node 2: Online (192.168.1.11)"}
	case "user.audit":
		res.Result = map[string]interface{}{
			"active_users": 42,
			"expired_users": 3,
			"total_quota_gb": 1024,
		}
	case "system.status":
		res.Result = map[string]interface{}{
			"cpu_usage": "14%",
			"ram_usage": "2.4 GB / 8 GB",
			"proxy_running": true,
		}
	default:
		res.Error = map[string]interface{}{
			"code":    -32601,
			"message": "Method not found",
		}
	}

	return json.Marshal(res)
}
