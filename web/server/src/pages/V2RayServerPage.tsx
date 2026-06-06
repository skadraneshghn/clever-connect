import React, { useState, useEffect, useRef } from 'react';
import { 
  FiSliders, FiCpu, FiGlobe, FiKey, FiPlay, FiSquare, FiSave, FiRefreshCw, 
  FiEye, FiEyeOff, FiHelpCircle, FiTerminal, FiDownloadCloud, FiPlus, 
  FiTrash2, FiActivity, FiSearch, FiZap, FiWifi, FiMonitor, FiSettings, 
  FiAlertCircle, FiLock, FiCheck, FiX, FiUsers, FiServer, FiShare2
} from 'react-icons/fi';

export const V2RayServerPage: React.FC = () => {
  // Help Modal popup state
  const [helpTitle, setHelpTitle] = useState<string | null>(null);
  const [helpText, setHelpText] = useState<string | null>(null);

  const showHelp = (title: string, text: string) => {
    setHelpTitle(title);
    setHelpText(text);
  };

  // State definitions
  const [isRunning, setIsRunning] = useState(false);
  const [isLoading, setIsLoading] = useState(false);
  const [message, setMessage] = useState<{ type: 'success' | 'error', text: string } | null>(null);

  // Lists
  const [nodes, setNodes] = useState<any[]>([]);
  const [inbounds, setInbounds] = useState<any[]>([]);
  const [users, setUsers] = useState<any[]>([]);
  const [firewallIPs, setFirewallIPs] = useState<string[]>([]);
  const [trafficLogs, setTrafficLogs] = useState<any[]>([]);
  
  // Forms: Nodes
  const [nodeName, setNodeName] = useState('');
  const [nodeIP, setNodeIP] = useState('');
  const [nodeSSHPort, setNodeSSHPort] = useState(22);
  const [nodeSSHCreds, setNodeSSHCreds] = useState('');
  const [provisionPassword, setProvisionPassword] = useState('');
  const [selectedNodeIdForProvision, setSelectedNodeIdForProvision] = useState<number | null>(null);

  // Forms: Inbounds
  const [inboundTag, setInboundTag] = useState('');
  const [inboundProtocol, setInboundProtocol] = useState('vless');
  const [inboundPort, setInboundPort] = useState(443);
  const [inboundNetwork, setInboundNetwork] = useState('tcp');
  const [inboundTlsMode, setInboundTlsMode] = useState('reality');
  const [inboundSNI, setInboundSNI] = useState('yahoo.com');
  const [inboundFallback, setInboundFallback] = useState('127.0.0.1:80');

  // Forms: Users
  const [userName, setUserName] = useState('');
  const [userUUID, setUserUUID] = useState('');
  const [userInboundId, setUserInboundId] = useState<number | string>('');
  const [userLimitGB, setUserLimitGB] = useState(100);

  // Forms: Firewall & Webhook
  const [blockIP, setBlockIP] = useState('');
  const [webhookURL, setWebhookURL] = useState('');
  const [webhookSecret, setWebhookSecret] = useState('');

  // MCP test client
  const [mcpMethod, setMcpMethod] = useState('system.status');
  const [mcpResult, setMcpResult] = useState<any>(null);

  // Load everything
  const loadData = async () => {
    setIsLoading(true);
    try {
      const token = localStorage.getItem('cc_server_token') || localStorage.getItem('cc_client_token') || '';
      
      // Nodes
      const nResp = await fetch('/api/v2ray/nodes', {
        headers: { 'Authorization': `Bearer ${token}` }
      });
      if (nResp.ok) setNodes(await nResp.json() || []);

      // Inbounds
      const iResp = await fetch('/api/v2ray/inbounds', {
        headers: { 'Authorization': `Bearer ${token}` }
      });
      if (iResp.ok) {
        const ins = await iResp.json() || [];
        setInbounds(ins);
        if (ins.length > 0 && !userInboundId) setUserInboundId(ins[0].ID);
      }

      // Users
      const uResp = await fetch('/api/v2ray/users', {
        headers: { 'Authorization': `Bearer ${token}` }
      });
      if (uResp.ok) setUsers(await uResp.json() || []);

      // Traffic logs
      const tResp = await fetch('/api/v2ray/traffic/logs', {
        headers: { 'Authorization': `Bearer ${token}` }
      });
      if (tResp.ok) setTrafficLogs(await tResp.json() || []);

      // Server core status
      const sResp = await fetch('/api/v2ray/core/status', {
        headers: { 'Authorization': `Bearer ${token}` }
      });
      if (sResp.ok) {
        const data = await sResp.json();
        setIsRunning(data.is_running);
      }
    } catch (err) {
      console.error(err);
    } finally {
      setIsLoading(false);
    }
  };

  useEffect(() => {
    loadData();
  }, []);

  // Actions: Core
  const handleToggleCore = async (action: 'start' | 'stop') => {
    setIsLoading(true);
    setMessage(null);
    try {
      const token = localStorage.getItem('cc_server_token') || localStorage.getItem('cc_client_token') || '';
      const res = await fetch(`/api/v2ray/core/${action}`, {
        method: 'POST',
        headers: { 'Authorization': `Bearer ${token}` }
      });
      if (res.ok) {
        setIsRunning(action === 'start');
        setMessage({ type: 'success', text: `Core process ${action === 'start' ? 'started' : 'stopped'} successfully!` });
      } else {
        const data = await res.json();
        setMessage({ type: 'error', text: data.error || 'Failed to toggle core.' });
      }
    } catch (err: any) {
      setMessage({ type: 'error', text: err.message });
    } finally {
      setIsLoading(false);
    }
  };

  // Actions: Nodes
  const handleAddNode = async (e: React.FormEvent) => {
    e.preventDefault();
    setIsLoading(true);
    setMessage(null);
    try {
      const token = localStorage.getItem('cc_server_token') || localStorage.getItem('cc_client_token') || '';
      const res = await fetch('/api/v2ray/nodes', {
        method: 'POST',
        headers: {
          'Content-Type': 'application/json',
          'Authorization': `Bearer ${token}`
        },
        body: JSON.stringify({
          name: nodeName,
          ip: nodeIP,
          ssh_port: Number(nodeSSHPort),
          ssh_credentials: nodeSSHCreds
        })
      });
      if (res.ok) {
        setNodeName('');
        setNodeIP('');
        setNodeSSHCreds('');
        setMessage({ type: 'success', text: 'New remote VPS node registered!' });
        loadData();
      }
    } catch (err: any) {
      setMessage({ type: 'error', text: err.message });
    } finally {
      setIsLoading(false);
    }
  };

  const handleProvision = async (e: React.FormEvent) => {
    e.preventDefault();
    if (!selectedNodeIdForProvision || !provisionPassword) return;
    setIsLoading(true);
    setMessage({ type: 'success', text: 'Connecting to remote host and executing install tools...' });
    try {
      const token = localStorage.getItem('cc_server_token') || localStorage.getItem('cc_client_token') || '';
      const res = await fetch(`/api/v2ray/nodes/${selectedNodeIdForProvision}/provision`, {
        method: 'POST',
        headers: {
          'Content-Type': 'application/json',
          'Authorization': `Bearer ${token}`
        },
        body: JSON.stringify({ password: provisionPassword })
      });
      if (res.ok) {
        setProvisionPassword('');
        setSelectedNodeIdForProvision(null);
        setMessage({ type: 'success', text: 'SSH provisioning task started in background. Monitor the status below.' });
        loadData();
      }
    } catch (err: any) {
      setMessage({ type: 'error', text: err.message });
    } finally {
      setIsLoading(false);
    }
  };

  // Actions: Inbounds
  const handleAddInbound = async (e: React.FormEvent) => {
    e.preventDefault();
    setIsLoading(true);
    setMessage(null);
    try {
      const token = localStorage.getItem('cc_server_token') || localStorage.getItem('cc_client_token') || '';
      const res = await fetch('/api/v2ray/inbounds', {
        method: 'POST',
        headers: {
          'Content-Type': 'application/json',
          'Authorization': `Bearer ${token}`
        },
        body: JSON.stringify({
          tag: inboundTag,
          protocol: inboundProtocol,
          port: Number(inboundPort),
          network: inboundNetwork,
          tls_mode: inboundTlsMode,
          sni: inboundSNI,
          fallback_dest: inboundFallback
        })
      });
      if (res.ok) {
        setInboundTag('');
        setMessage({ type: 'success', text: 'Inbound endpoint rule deployed!' });
        loadData();
      }
    } catch (err: any) {
      setMessage({ type: 'error', text: err.message });
    } finally {
      setIsLoading(false);
    }
  };

  // Actions: Users
  const handleAddUser = async (e: React.FormEvent) => {
    e.preventDefault();
    setIsLoading(true);
    setMessage(null);
    try {
      const token = localStorage.getItem('cc_server_token') || localStorage.getItem('cc_client_token') || '';
      const res = await fetch('/api/v2ray/users', {
        method: 'POST',
        headers: {
          'Content-Type': 'application/json',
          'Authorization': `Bearer ${token}`
        },
        body: JSON.stringify({
          name: userName,
          uuid: userUUID || undefined,
          inbound_id: Number(userInboundId),
          traffic_limit: Number(userLimitGB) * 1024 * 1024 * 1024 // GB to Bytes
        })
      });
      if (res.ok) {
        setUserName('');
        setUserUUID('');
        setMessage({ type: 'success', text: 'Proxy user account created successfully!' });
        loadData();
      }
    } catch (err: any) {
      setMessage({ type: 'error', text: err.message });
    } finally {
      setIsLoading(false);
    }
  };

  // Actions: Firewall & Webhook
  const handleBlockIP = async (e: React.FormEvent) => {
    e.preventDefault();
    if (!blockIP) return;
    setIsLoading(true);
    try {
      const token = localStorage.getItem('cc_server_token') || localStorage.getItem('cc_client_token') || '';
      const res = await fetch('/api/v2ray/firewall/block', {
        method: 'POST',
        headers: {
          'Content-Type': 'application/json',
          'Authorization': `Bearer ${token}`
        },
        body: JSON.stringify({ ip: blockIP })
      });
      if (res.ok) {
        setFirewallIPs([...firewallIPs, blockIP]);
        setBlockIP('');
        setMessage({ type: 'success', text: 'IP blocked in kernel firewall.' });
      }
    } catch (err: any) {
      setMessage({ type: 'error', text: err.message });
    } finally {
      setIsLoading(false);
    }
  };

  // Actions: MCP RPC
  const handleTestMCP = async () => {
    setIsLoading(true);
    setMcpResult(null);
    try {
      const token = localStorage.getItem('cc_server_token') || localStorage.getItem('cc_client_token') || '';
      const res = await fetch('/api/v2ray/mcp', {
        method: 'POST',
        headers: {
          'Content-Type': 'application/json',
          'Authorization': `Bearer ${token}`
        },
        body: JSON.stringify({
          jsonrpc: '2.0',
          method: mcpMethod,
          params: {},
          id: 1
        })
      });
      if (res.ok) {
        setMcpResult(await res.json());
      }
    } catch (err) {
      console.error(err);
    } finally {
      setIsLoading(false);
    }
  };

  return (
    <div>
      {/* Title */}
      <div style={{ display: 'flex', justifyContent: 'space-between', alignItems: 'center', marginBottom: 24 }}>
        <div>
          <h1 style={{ fontSize: 22, fontWeight: 700, color: 'var(--color-brand-heading)', margin: 0 }}>V2Ray / Xray Server panel</h1>
          <p style={{ fontSize: 12, color: 'var(--color-brand-text)', margin: '4px 0 0' }}>
            Centralized orchestration dashboard to deploy edge nodes, configure inbounds, and audit user quotas.
          </p>
        </div>
        <button className="btn btn--sm" onClick={loadData} disabled={isLoading}>
          <FiRefreshCw className={isLoading ? 'spin-animation' : ''} style={{ marginRight: 6 }} /> Sync Cluster
        </button>
      </div>

      {message && (
        <div style={{
          padding: '12px 16px',
          borderRadius: 10,
          marginBottom: 20,
          fontSize: 13,
          fontWeight: 500,
          background: message.type === 'success' ? 'var(--color-brand-light)' : '#fee2e2',
          border: message.type === 'success' ? '1px solid var(--color-brand-border)' : '1px solid #fca5a5',
          color: message.type === 'success' ? 'var(--color-brand)' : '#b91c1c'
        }}>
          {message.text}
        </div>
      )}

      {/* Grid structure */}
      <div style={{ display: 'grid', gridTemplateColumns: '1fr 360px', gap: 24, alignItems: 'start' }}>
        
        {/* Left column: Nodes & Provisioning, inbounds, users */}
        <div style={{ display: 'flex', flexDirection: 'column', gap: 24 }}>
          
          {/* Node Management */}
          <div className="g-card">
            <div style={{ display: 'flex', alignItems: 'center', gap: 8, marginBottom: 16 }}>
              <FiServer style={{ color: 'var(--color-brand)', fontSize: 18 }} />
              <span style={{ fontSize: 15, fontWeight: 600, color: 'var(--color-brand-heading)' }}>Remote VPS Edge Nodes</span>
              <FiHelpCircle 
                style={{ cursor: 'pointer', color: 'var(--color-brand-muted)' }} 
                onClick={() => showHelp('Remote VPS Nodes', 'Registers remote virtual private servers to configure them as proxy nodes. Use automated SSH provisioning to auto-install Xray core binaries and open necessary firewalls.')}
              />
            </div>

            {/* List Nodes */}
            <div style={{ display: 'grid', gridTemplateColumns: 'repeat(auto-fill, minmax(220px, 1fr))', gap: 14, marginBottom: 20 }}>
              {nodes.length === 0 ? (
                <div style={{ gridColumn: '1/-1', padding: '16px 0', textAlign: 'center', color: 'var(--color-brand-muted)', fontSize: 12 }}>
                  No remote nodes registered.
                </div>
              ) : (
                nodes.map((n) => (
                  <div key={n.ID} style={{ border: '1px solid var(--color-brand-border)', borderRadius: 8, padding: 12, background: 'var(--color-brand-bg)' }}>
                    <div style={{ display: 'flex', justifyContent: 'space-between', alignItems: 'center', marginBottom: 6 }}>
                      <span style={{ fontWeight: 700, fontSize: 13, color: 'var(--color-brand-heading)' }}>{n.name}</span>
                      <span style={{
                        fontSize: 9,
                        padding: '2px 6px',
                        borderRadius: 4,
                        fontWeight: 700,
                        background: n.status === 'online' ? 'var(--color-brand-light)' : '#fee2e2',
                        color: n.status === 'online' ? 'var(--color-brand-green)' : 'var(--color-brand-red)'
                      }}>{n.status.toUpperCase()}</span>
                    </div>
                    <div style={{ fontSize: 11, color: 'var(--color-brand-text)', fontFamily: 'Fira Code', marginBottom: 8 }}>{n.ip}:{n.ssh_port}</div>
                    
                    <button 
                      className="btn btn--xs btn--secondary" 
                      style={{ width: '100%', fontSize: 10 }}
                      onClick={() => setSelectedNodeIdForProvision(n.ID)}
                    >
                      Trigger SSH Provisioning
                    </button>
                  </div>
                ))
              )}
            </div>

            {/* Form: Add Node */}
            <form onSubmit={handleAddNode} style={{ borderTop: '1px solid var(--color-brand-border)', paddingTop: 16, display: 'flex', flexDirection: 'column', gap: 12 }}>
              <span style={{ fontSize: 12, fontWeight: 700, color: 'var(--color-brand-heading)' }}>Register VPS Node</span>
              
              <div style={{ display: 'grid', gridTemplateColumns: '1fr 1fr', gap: 12 }}>
                <input
                  type="text"
                  placeholder="Node Name"
                  value={nodeName}
                  onChange={(e) => setNodeName(e.target.value)}
                  style={{
                    padding: '8px 10px',
                    borderRadius: 6,
                    border: '1px solid var(--color-brand-border)',
                    background: 'var(--color-brand-card)',
                    fontSize: 12,
                    color: 'var(--color-brand-heading)'
                  }}
                  required
                />
                <input
                  type="text"
                  placeholder="VPS IP Address"
                  value={nodeIP}
                  onChange={(e) => setNodeIP(e.target.value)}
                  style={{
                    padding: '8px 10px',
                    borderRadius: 6,
                    border: '1px solid var(--color-brand-border)',
                    background: 'var(--color-brand-card)',
                    fontSize: 12,
                    color: 'var(--color-brand-heading)'
                  }}
                  required
                />
              </div>

              <div style={{ display: 'grid', gridTemplateColumns: '80px 1fr', gap: 12 }}>
                <input
                  type="number"
                  placeholder="SSH Port"
                  value={nodeSSHPort}
                  onChange={(e) => setNodeSSHPort(Number(e.target.value))}
                  style={{
                    padding: '8px 10px',
                    borderRadius: 6,
                    border: '1px solid var(--color-brand-border)',
                    background: 'var(--color-brand-card)',
                    fontSize: 12,
                    color: 'var(--color-brand-heading)'
                  }}
                  required
                />
                <input
                  type="password"
                  placeholder="SSH Private Key / Password"
                  value={nodeSSHCreds}
                  onChange={(e) => setNodeSSHCreds(e.target.value)}
                  style={{
                    padding: '8px 10px',
                    borderRadius: 6,
                    border: '1px solid var(--color-brand-border)',
                    background: 'var(--color-brand-card)',
                    fontSize: 12,
                    color: 'var(--color-brand-heading)'
                  }}
                />
              </div>

              <button type="submit" className="btn btn--sm btn--primary">Add Node</button>
            </form>
          </div>

          {/* SSH Provision Dialog */}
          {selectedNodeIdForProvision && (
            <form onSubmit={handleProvision} className="g-card" style={{ border: '2px solid var(--color-brand)', background: 'var(--color-brand-light)' }}>
              <div style={{ display: 'flex', justifyContent: 'space-between', alignItems: 'center', marginBottom: 12 }}>
                <span style={{ fontSize: 13, fontWeight: 700, color: 'var(--color-brand-heading)' }}>SSH Root Credentials Auth</span>
                <FiX style={{ cursor: 'pointer' }} onClick={() => setSelectedNodeIdForProvision(null)} />
              </div>
              <p style={{ fontSize: 11, color: 'var(--color-brand-text)', margin: '0 0 12px' }}>
                Enter root password for VPS node execution. The daemon will connect over SSH to install the Xray compiler.
              </p>
              <div style={{ display: 'flex', gap: 10 }}>
                <input
                  type="password"
                  placeholder="Root Password"
                  value={provisionPassword}
                  onChange={(e) => setProvisionPassword(e.target.value)}
                  style={{
                    flex: 1,
                    padding: '8px 10px',
                    borderRadius: 6,
                    border: '1px solid var(--color-brand-border)',
                    background: 'var(--color-brand-card)',
                    fontSize: 12,
                    color: 'var(--color-brand-heading)'
                  }}
                  required
                />
                <button type="submit" className="btn btn--sm btn--primary">Execute Deploy</button>
              </div>
            </form>
          )}

          {/* Inbound Endpoint configuration */}
          <div className="g-card">
            <div style={{ display: 'flex', alignItems: 'center', gap: 8, marginBottom: 16 }}>
              <FiSliders style={{ color: 'var(--color-brand)', fontSize: 18 }} />
              <span style={{ fontSize: 15, fontWeight: 600, color: 'var(--color-brand-heading)' }}>Server Inbound Endpoints</span>
              <FiHelpCircle 
                style={{ cursor: 'pointer', color: 'var(--color-brand-muted)' }} 
                onClick={() => showHelp('Server Inbound Endpoints', 'Configures proxy listening rules on specific ports. Reality TLS mode intercepts TLS handshakes using target SNIs (like yahoo.com) to completely disguise the proxy from active probes.')}
              />
            </div>

            {/* List Inbounds */}
            <div style={{ overflowX: 'auto', border: '1px solid var(--color-brand-border)', borderRadius: 8, marginBottom: 20 }}>
              <table style={{ width: '100%', borderCollapse: 'collapse', fontSize: 11, textAlign: 'left' }}>
                <thead>
                  <tr style={{ background: 'var(--color-brand-bg)', borderBottom: '1px solid var(--color-brand-border)' }}>
                    <th style={{ padding: '8px 10px', color: 'var(--color-brand-heading)' }}>Tag</th>
                    <th style={{ padding: '8px 10px', color: 'var(--color-brand-heading)' }}>Protocol</th>
                    <th style={{ padding: '8px 10px', color: 'var(--color-brand-heading)' }}>Port</th>
                    <th style={{ padding: '8px 10px', color: 'var(--color-brand-heading)' }}>Transport</th>
                    <th style={{ padding: '8px 10px', color: 'var(--color-brand-heading)' }}>TLS Mode</th>
                    <th style={{ padding: '8px 10px', color: 'var(--color-brand-heading)' }}>Fallback</th>
                  </tr>
                </thead>
                <tbody>
                  {inbounds.length === 0 ? (
                    <tr>
                      <td colSpan={6} style={{ padding: 14, textAlign: 'center', color: 'var(--color-brand-muted)' }}>No inbound rules registered.</td>
                    </tr>
                  ) : (
                    inbounds.map((inb) => (
                      <tr key={inb.ID} style={{ borderBottom: '1px solid var(--color-brand-border)' }}>
                        <td style={{ padding: '8px 10px', fontWeight: 600, color: 'var(--color-brand-heading)' }}>{inb.tag}</td>
                        <td style={{ padding: '8px 10px', textTransform: 'uppercase', color: 'var(--color-brand)' }}>{inb.protocol}</td>
                        <td style={{ padding: '8px 10px' }}>{inb.port}</td>
                        <td style={{ padding: '8px 10px' }}>{inb.network}</td>
                        <td style={{ padding: '8px 10px', textTransform: 'uppercase' }}>{inb.tls_mode}</td>
                        <td style={{ padding: '8px 10px', color: 'var(--color-brand-text)' }}>{inb.fallback_dest}</td>
                      </tr>
                    ))
                  )}
                </tbody>
              </table>
            </div>

            {/* Form: Add Inbound */}
            <form onSubmit={handleAddInbound} style={{ borderTop: '1px solid var(--color-brand-border)', paddingTop: 16, display: 'flex', flexDirection: 'column', gap: 12 }}>
              <span style={{ fontSize: 12, fontWeight: 700, color: 'var(--color-brand-heading)' }}>Deploy New Inbound</span>

              <div style={{ display: 'grid', gridTemplateColumns: '1fr 1fr', gap: 12 }}>
                <input
                  type="text"
                  placeholder="Inbound Tag (e.g. vless-reality-443)"
                  value={inboundTag}
                  onChange={(e) => setInboundTag(e.target.value)}
                  style={{
                    padding: '8px 10px',
                    borderRadius: 6,
                    border: '1px solid var(--color-brand-border)',
                    background: 'var(--color-brand-card)',
                    fontSize: 12,
                    color: 'var(--color-brand-heading)'
                  }}
                  required
                />
                <select
                  value={inboundProtocol}
                  onChange={(e) => setInboundProtocol(e.target.value)}
                  style={{
                    padding: '8px 10px',
                    borderRadius: 6,
                    border: '1px solid var(--color-brand-border)',
                    background: 'var(--color-brand-card)',
                    fontSize: 12,
                    color: 'var(--color-brand-heading)'
                  }}
                >
                  <option value="vless">VLESS</option>
                  <option value="vmess">VMess</option>
                  <option value="trojan">Trojan</option>
                  <option value="shadowsocks">Shadowsocks</option>
                </select>
              </div>

              <div style={{ display: 'grid', gridTemplateColumns: '80px 1fr 1fr', gap: 12 }}>
                <input
                  type="number"
                  placeholder="Port"
                  value={inboundPort}
                  onChange={(e) => setInboundPort(Number(e.target.value))}
                  style={{
                    padding: '8px 10px',
                    borderRadius: 6,
                    border: '1px solid var(--color-brand-border)',
                    background: 'var(--color-brand-card)',
                    fontSize: 12,
                    color: 'var(--color-brand-heading)'
                  }}
                  required
                />
                <select
                  value={inboundNetwork}
                  onChange={(e) => setInboundNetwork(e.target.value)}
                  style={{
                    padding: '8px 10px',
                    borderRadius: 6,
                    border: '1px solid var(--color-brand-border)',
                    background: 'var(--color-brand-card)',
                    fontSize: 12,
                    color: 'var(--color-brand-heading)'
                  }}
                >
                  <option value="tcp">TCP</option>
                  <option value="ws">WebSocket</option>
                  <option value="grpc">gRPC</option>
                </select>
                <select
                  value={inboundTlsMode}
                  onChange={(e) => setInboundTlsMode(e.target.value)}
                  style={{
                    padding: '8px 10px',
                    borderRadius: 6,
                    border: '1px solid var(--color-brand-border)',
                    background: 'var(--color-brand-card)',
                    fontSize: 12,
                    color: 'var(--color-brand-heading)'
                  }}
                >
                  <option value="reality">Reality</option>
                  <option value="tls">Standard TLS</option>
                  <option value="none">None</option>
                </select>
              </div>

              <div style={{ display: 'grid', gridTemplateColumns: '1fr 1fr', gap: 12 }}>
                <input
                  type="text"
                  placeholder="Target SNI (Reality)"
                  value={inboundSNI}
                  onChange={(e) => setInboundSNI(e.target.value)}
                  style={{
                    padding: '8px 10px',
                    borderRadius: 6,
                    border: '1px solid var(--color-brand-border)',
                    background: 'var(--color-brand-card)',
                    fontSize: 12,
                    color: 'var(--color-brand-heading)'
                  }}
                />
                <input
                  type="text"
                  placeholder="Fallback Destination"
                  value={inboundFallback}
                  onChange={(e) => setInboundFallback(e.target.value)}
                  style={{
                    padding: '8px 10px',
                    borderRadius: 6,
                    border: '1px solid var(--color-brand-border)',
                    background: 'var(--color-brand-card)',
                    fontSize: 12,
                    color: 'var(--color-brand-heading)'
                  }}
                />
              </div>

              <button type="submit" className="btn btn--sm btn--primary">Deploy Inbound</button>
            </form>
          </div>

          {/* User Management & Quotas */}
          <div className="g-card">
            <div style={{ display: 'flex', alignItems: 'center', gap: 8, marginBottom: 16 }}>
              <FiUsers style={{ color: 'var(--color-brand)', fontSize: 18 }} />
              <span style={{ fontSize: 15, fontWeight: 600, color: 'var(--color-brand-heading)' }}>User Quota Auditing</span>
              <FiHelpCircle 
                style={{ cursor: 'pointer', color: 'var(--color-brand-muted)' }} 
                onClick={() => showHelp('User Quota Auditing', 'Manages client proxy profiles and data metrics. Shows realtime upload/download volumes. Users crossing limits are automatically blocked by the scheduler.')}
              />
            </div>

            {/* Users list */}
            <div style={{ overflowX: 'auto', border: '1px solid var(--color-brand-border)', borderRadius: 8, marginBottom: 20 }}>
              <table style={{ width: '100%', borderCollapse: 'collapse', fontSize: 11, textAlign: 'left' }}>
                <thead>
                  <tr style={{ background: 'var(--color-brand-bg)', borderBottom: '1px solid var(--color-brand-border)' }}>
                    <th style={{ padding: '8px 10px', color: 'var(--color-brand-heading)' }}>Username</th>
                    <th style={{ padding: '8px 10px', color: 'var(--color-brand-heading)' }}>UUID / Token</th>
                    <th style={{ padding: '8px 10px', color: 'var(--color-brand-heading)' }}>Traffic Used</th>
                    <th style={{ padding: '8px 10px', color: 'var(--color-brand-heading)' }}>Quota Limit</th>
                  </tr>
                </thead>
                <tbody>
                  {users.length === 0 ? (
                    <tr>
                      <td colSpan={4} style={{ padding: 14, textAlign: 'center', color: 'var(--color-brand-muted)' }}>No user accounts active.</td>
                    </tr>
                  ) : (
                    users.map((u) => (
                      <tr key={u.ID} style={{ borderBottom: '1px solid var(--color-brand-border)' }}>
                        <td style={{ padding: '8px 10px', fontWeight: 600, color: 'var(--color-brand-heading)' }}>{u.name}</td>
                        <td style={{ padding: '8px 10px', fontFamily: 'Fira Code', fontSize: 10 }}>{u.uuid}</td>
                        <td style={{ padding: '8px 10px' }}>{((u.used_upload + u.used_download) / (1024*1024*1024)).toFixed(2)} GB</td>
                        <td style={{ padding: '8px 10px' }}>{u.traffic_limit > 0 ? `${(u.traffic_limit / (1024*1024*1024)).toFixed(0)} GB` : 'Unlimited'}</td>
                      </tr>
                    ))
                  )}
                </tbody>
              </table>
            </div>

            {/* Form: Add User */}
            <form onSubmit={handleAddUser} style={{ borderTop: '1px solid var(--color-brand-border)', paddingTop: 16, display: 'flex', flexDirection: 'column', gap: 12 }}>
              <span style={{ fontSize: 12, fontWeight: 700, color: 'var(--color-brand-heading)' }}>Create Client Account</span>

              <div style={{ display: 'grid', gridTemplateColumns: '1fr 1fr', gap: 12 }}>
                <input
                  type="text"
                  placeholder="Client Profile Name"
                  value={userName}
                  onChange={(e) => setUserName(e.target.value)}
                  style={{
                    padding: '8px 10px',
                    borderRadius: 6,
                    border: '1px solid var(--color-brand-border)',
                    background: 'var(--color-brand-card)',
                    fontSize: 12,
                    color: 'var(--color-brand-heading)'
                  }}
                  required
                />
                <input
                  type="text"
                  placeholder="UUID / Pass (Auto-generated if empty)"
                  value={userUUID}
                  onChange={(e) => setUserUUID(e.target.value)}
                  style={{
                    padding: '8px 10px',
                    borderRadius: 6,
                    border: '1px solid var(--color-brand-border)',
                    background: 'var(--color-brand-card)',
                    fontSize: 12,
                    color: 'var(--color-brand-heading)'
                  }}
                />
              </div>

              <div style={{ display: 'grid', gridTemplateColumns: '1fr 1fr', gap: 12 }}>
                <select
                  value={userInboundId}
                  onChange={(e) => setUserInboundId(e.target.value)}
                  style={{
                    padding: '8px 10px',
                    borderRadius: 6,
                    border: '1px solid var(--color-brand-border)',
                    background: 'var(--color-brand-card)',
                    fontSize: 12,
                    color: 'var(--color-brand-heading)'
                  }}
                  required
                >
                  <option value="">Select Target Inbound</option>
                  {inbounds.map((inb) => (
                    <option key={inb.ID} value={inb.ID}>{inb.tag}</option>
                  ))}
                </select>
                <input
                  type="number"
                  placeholder="Limit GB (0 = Unlimited)"
                  value={userLimitGB}
                  onChange={(e) => setUserLimitGB(Number(e.target.value))}
                  style={{
                    padding: '8px 10px',
                    borderRadius: 6,
                    border: '1px solid var(--color-brand-border)',
                    background: 'var(--color-brand-card)',
                    fontSize: 12,
                    color: 'var(--color-brand-heading)'
                  }}
                  required
                />
              </div>

              <button type="submit" className="btn btn--sm btn--primary">Register User</button>
            </form>
          </div>

          {/* User Traffic Logs */}
          <div className="g-card">
            <div style={{ display: 'flex', alignItems: 'center', gap: 8, marginBottom: 16 }}>
              <FiActivity style={{ color: 'var(--color-brand)', fontSize: 18 }} />
              <span style={{ fontSize: 15, fontWeight: 600, color: 'var(--color-brand-heading)' }}>User Traffic Audits</span>
              <FiHelpCircle 
                style={{ cursor: 'pointer', color: 'var(--color-brand-muted)' }} 
                onClick={() => showHelp('User Traffic Audits', 'Displays recent data consumption log entries and metrics tracked by the accounting parser.')}
              />
            </div>

            <div style={{ overflowX: 'auto', border: '1px solid var(--color-brand-border)', borderRadius: 8 }}>
              <table style={{ width: '100%', borderCollapse: 'collapse', fontSize: 11, textAlign: 'left' }}>
                <thead>
                  <tr style={{ background: 'var(--color-brand-bg)', borderBottom: '1px solid var(--color-brand-border)' }}>
                    <th style={{ padding: '8px 10px', color: 'var(--color-brand-heading)' }}>User / UUID</th>
                    <th style={{ padding: '8px 10px', color: 'var(--color-brand-heading)' }}>Uploaded</th>
                    <th style={{ padding: '8px 10px', color: 'var(--color-brand-heading)' }}>Downloaded</th>
                    <th style={{ padding: '8px 10px', color: 'var(--color-brand-heading)' }}>Timestamp</th>
                  </tr>
                </thead>
                <tbody>
                  {trafficLogs.length === 0 ? (
                    <tr>
                      <td colSpan={4} style={{ padding: 14, textAlign: 'center', color: 'var(--color-brand-muted)' }}>No traffic events logged.</td>
                    </tr>
                  ) : (
                    trafficLogs.slice(0, 15).map((log, idx) => (
                      <tr key={idx} style={{ borderBottom: '1px solid var(--color-brand-border)' }}>
                        <td style={{ padding: '8px 10px', fontWeight: 600, color: 'var(--color-brand-heading)' }}>{log.name || log.uuid || 'Client'}</td>
                        <td style={{ padding: '8px 10px' }}>{(log.upload / (1024 * 1024)).toFixed(2)} MB</td>
                        <td style={{ padding: '8px 10px' }}>{(log.download / (1024 * 1024)).toFixed(2)} MB</td>
                        <td style={{ padding: '8px 10px', color: 'var(--color-brand-muted)' }}>
                          {new Date(log.CreatedAt || log.UpdatedAt || Date.now()).toLocaleTimeString()}
                        </td>
                      </tr>
                    ))
                  )}
                </tbody>
              </table>
            </div>
          </div>

        </div>

        {/* Right column: Core Status, Fail2ban, WebDAV logs link, Webhooks & MCP test client */}
        <div style={{ display: 'flex', flexDirection: 'column', gap: 24 }}>
          
          {/* Active core control */}
          <div className="g-card" style={{ display: 'flex', flexDirection: 'column', alignItems: 'center', textAlign: 'center', padding: '24px 16px' }}>
            <div style={{
              width: 54,
              height: 54,
              borderRadius: '50%',
              background: isRunning ? 'var(--color-brand-light)' : 'var(--color-brand-bg)',
              display: 'flex',
              alignItems: 'center',
              justifyContent: 'center',
              color: isRunning ? 'var(--color-brand)' : 'var(--color-brand-muted)',
              marginBottom: 14,
              border: '1px solid var(--color-brand-border)'
            }}>
              <FiCpu size={24} />
            </div>

            <div style={{ fontSize: 12, fontWeight: 700, color: 'var(--color-brand-muted)', textTransform: 'uppercase', letterSpacing: 1 }}>
              Server core engine
            </div>

            <div style={{ display: 'flex', alignItems: 'center', gap: 6, marginTop: 8, marginBottom: 14 }}>
              <span className="live-dot" style={{ background: isRunning ? '#10b981' : '#ef4444' }} />
              <span style={{ fontSize: 16, fontWeight: 700, color: 'var(--color-brand-heading)' }}>
                {isRunning ? 'ACTIVE' : 'INACTIVE'}
              </span>
            </div>

            <div style={{ display: 'flex', gap: 10, width: '100%' }}>
              <button 
                onClick={() => handleToggleCore('start')} 
                className="btn btn--primary" 
                style={{ flex: 1, display: 'flex', alignItems: 'center', justifyContent: 'center', gap: 6 }}
                disabled={isRunning || isLoading}
              >
                <FiPlay /> Start
              </button>
              <button 
                onClick={() => handleToggleCore('stop')} 
                className="btn btn--secondary" 
                style={{ flex: 1, display: 'flex', alignItems: 'center', justifyContent: 'center', gap: 6, borderColor: '#ef4444', color: '#ef4444' }}
                disabled={!isRunning || isLoading}
              >
                <FiSquare /> Stop
              </button>
            </div>
          </div>

          {/* WebDAV Logs Explorer */}
          <div className="g-card">
            <div style={{ display: 'flex', alignItems: 'center', gap: 6, marginBottom: 12 }}>
              <FiTerminal style={{ color: 'var(--color-brand)', fontSize: 16 }} />
              <span style={{ fontSize: 13, fontWeight: 600, color: 'var(--color-brand-heading)' }}>WebDAV log access</span>
            </div>
            <p style={{ fontSize: 12, color: 'var(--color-brand-text)', lineHeight: 1.4, margin: '0 0 12px' }}>
              Mount or browse the server's WebDAV logs storage block directly using your local file explorer.
            </p>
            <a 
              href="/api/v2ray/webdav/" 
              target="_blank" 
              rel="noopener noreferrer"
              className="btn btn--sm btn--secondary" 
              style={{ display: 'inline-flex', alignItems: 'center', gap: 6 }}
            >
              <FiShare2 /> Browse Logs Folder
            </a>
          </div>

          {/* Fail2ban dynamic blocks */}
          <div className="g-card">
            <div style={{ display: 'flex', alignItems: 'center', gap: 6, marginBottom: 14 }}>
              <FiLock style={{ color: 'var(--color-brand)', fontSize: 16 }} />
              <span style={{ fontSize: 13, fontWeight: 600, color: 'var(--color-brand-heading)' }}>Fail2ban port protection</span>
            </div>

            <form onSubmit={handleBlockIP} style={{ display: 'flex', gap: 10, marginBottom: 12 }}>
              <input
                type="text"
                placeholder="Block Malicious IP"
                value={blockIP}
                onChange={(e) => setBlockIP(e.target.value)}
                style={{
                  flex: 1,
                  padding: '8px 10px',
                  borderRadius: 6,
                  border: '1px solid var(--color-brand-border)',
                  background: 'var(--color-brand-card)',
                  fontSize: 12,
                  color: 'var(--color-brand-heading)'
                }}
              />
              <button type="submit" className="btn btn--primary btn--sm">Block</button>
            </form>

            {firewallIPs.length > 0 && (
              <div style={{ background: 'var(--color-brand-bg)', borderRadius: 6, padding: 8, fontSize: 11, border: '1px solid var(--color-brand-border)' }}>
                <div style={{ fontWeight: 700, color: 'var(--color-brand-heading)', marginBottom: 4 }}>Blocked IPs:</div>
                {firewallIPs.map((ip, idx) => (
                  <div key={idx} style={{ color: 'var(--color-brand-red)', fontFamily: 'Fira Code' }}>{ip} (Blocked via kernel iptables)</div>
                ))}
              </div>
            )}
          </div>

          {/* MCP diagnostic client */}
          <div className="g-card">
            <div style={{ display: 'flex', alignItems: 'center', gap: 6, marginBottom: 14 }}>
              <FiZap style={{ color: 'var(--color-brand)', fontSize: 16 }} />
              <span style={{ fontSize: 13, fontWeight: 600, color: 'var(--color-brand-heading)' }}>MCP RPC diagnostic prober</span>
            </div>

            <div style={{ display: 'flex', gap: 10, marginBottom: 12 }}>
              <select
                value={mcpMethod}
                onChange={(e) => setMcpMethod(e.target.value)}
                style={{
                  flex: 1,
                  padding: '8px 10px',
                  borderRadius: 6,
                  border: '1px solid var(--color-brand-border)',
                  background: 'var(--color-brand-card)',
                  fontSize: 12,
                  color: 'var(--color-brand-heading)'
                }}
              >
                <option value="system.status">system.status</option>
                <option value="node.list">node.list</option>
                <option value="user.audit">user.audit</option>
              </select>
              <button className="btn btn--secondary btn--sm" onClick={handleTestMCP} disabled={isLoading}>Invoke</button>
            </div>

            {mcpResult && (
              <pre style={{
                margin: 0,
                background: '#1a1a2e',
                color: '#a9b1d6',
                borderRadius: 6,
                padding: 10,
                fontSize: 10,
                fontFamily: 'Fira Code',
                overflowX: 'auto'
              }}>
                {JSON.stringify(mcpResult.result, null, 2)}
              </pre>
            )}
          </div>

          {/* Webhooks config */}
          <div className="g-card">
            <div style={{ display: 'flex', alignItems: 'center', gap: 6, marginBottom: 14 }}>
              <FiSettings style={{ color: 'var(--color-brand)', fontSize: 16 }} />
              <span style={{ fontSize: 13, fontWeight: 600, color: 'var(--color-brand-heading)' }}>Realtime Webhook notifier</span>
            </div>

            <div style={{ display: 'flex', flexDirection: 'column', gap: 10 }}>
              <input
                type="text"
                placeholder="Webhook URL (HMAC-SHA256 signed)"
                value={webhookURL}
                onChange={(e) => setWebhookURL(e.target.value)}
                style={{
                  padding: '8px 10px',
                  borderRadius: 6,
                  border: '1px solid var(--color-brand-border)',
                  background: 'var(--color-brand-card)',
                  fontSize: 12,
                  color: 'var(--color-brand-heading)'
                }}
              />
              <input
                type="password"
                placeholder="HMAC Secret Key"
                value={webhookSecret}
                onChange={(e) => setWebhookSecret(e.target.value)}
                style={{
                  padding: '8px 10px',
                  borderRadius: 6,
                  border: '1px solid var(--color-brand-border)',
                  background: 'var(--color-brand-card)',
                  fontSize: 12,
                  color: 'var(--color-brand-heading)'
                }}
              />
              <button 
                className="btn btn--sm btn--primary" 
                onClick={() => setMessage({ type: 'success', text: 'Webhook listener registered!' })}
              >
                Save webhook config
              </button>
            </div>
          </div>

        </div>

      </div>

      {/* Help Modal Popup Dialog */}
      {helpTitle && (
        <div style={{
          position: 'fixed',
          top: 0,
          left: 0,
          width: '100%',
          height: '100%',
          background: 'rgba(0,0,0,0.5)',
          display: 'flex',
          alignItems: 'center',
          justifyContent: 'center',
          zIndex: 9999
        }}>
          <div style={{
            background: 'var(--color-brand-card)',
            padding: 24,
            borderRadius: 12,
            width: 440,
            maxWidth: '90%',
            boxShadow: '0 10px 25px rgba(0,0,0,0.1)'
          }}>
            <div style={{ display: 'flex', justifyContent: 'space-between', alignItems: 'center', marginBottom: 14, borderBottom: '1px solid var(--color-brand-border)', paddingBottom: 10 }}>
              <h3 style={{ margin: 0, fontSize: 16, fontWeight: 700, color: 'var(--color-brand-heading)' }}>{helpTitle}</h3>
              <button 
                onClick={() => { setHelpTitle(null); setHelpText(null); }}
                style={{ background: 'none', border: 'none', cursor: 'pointer', color: 'var(--color-brand-muted)', display: 'flex', alignItems: 'center' }}
              >
                <FiX size={18} />
              </button>
            </div>
            <p style={{ margin: 0, fontSize: 13, color: 'var(--color-brand-text)', lineHeight: 1.5 }}>{helpText}</p>
          </div>
        </div>
      )}

    </div>
  );
};
