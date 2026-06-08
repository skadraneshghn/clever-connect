import React, { useState, useEffect, useRef } from 'react';
import { 
  FiRefreshCw, FiPlay, FiSquare, FiDownload, FiUpload, FiSettings, 
  FiSearch, FiClipboard, FiAlertCircle, FiCheckCircle, FiFileText, FiList
} from 'react-icons/fi';
import { useAuthStore } from '../store/authStore';

interface ScannedCandidate {
  ip: string;
  port: number;
  protocol: string;
  latencyMs: number;
  speedMbps: number;
  status: 'healthy' | 'failed' | 'in_flight';
  time: string;
}

export const NetworkToolsPage: React.FC = () => {
  const { token } = useAuthStore();
  const [ws, setWs] = useState<WebSocket | null>(null);
  const [wsConnected, setWsConnected] = useState(false);
  const [isScanning, setIsScanning] = useState(false);

  // Scanner state metrics
  const [stats, setStats] = useState({
    tested: 0,
    healthy: 0,
    failed: 0,
    in_flight: 0,
  });

  // Candidates list
  const [candidates, setCandidates] = useState<ScannedCandidate[]>([]);
  const [searchQuery, setSearchQuery] = useState('');
  const [sortBy, setSortBy] = useState<'latency' | 'speed' | 'time'>('latency');

  // Input states
  const [rawConfigLink, setRawConfigLink] = useState('');
  const [targetCidrs, setTargetCidrs] = useState('108.162.192.0/18\n103.21.244.0/22');
  const [selectedPorts, setSelectedPorts] = useState('443, 80, 8443');
  const [concurrencyLimit, setConcurrencyLimit] = useState(100);
  const [maxRateLimit, setMaxRateLimit] = useState(0);
  const [networkTimeoutMs, setNetworkTimeoutMs] = useState(3000);
  const [probeAttempts, setProbeAttempts] = useState(1);
  const [targetMode, setTargetMode] = useState<'ws' | 'tls' | 'http'>('ws');
  const [targetSni, setTargetSni] = useState('speed.cloudflare.com');
  const [websocketHost, setWebsocketHost] = useState('speed.cloudflare.com');
  const [websocketPath, setWebsocketPath] = useState('/__down');
  const [requireWs, setRequireWs] = useState(true);
  const [enableNeighbors, setEnableNeighbors] = useState(true);
  const [topLimit, setTopLimit] = useState(20);

  // Drag and drop state
  const [isDragging, setIsDragging] = useState(false);
  const [message, setMessage] = useState<{ type: 'success' | 'error' | 'info'; text: string } | null>(null);

  const dragCounter = useRef(0);

  // Parse connection link logic
  const handleParseLink = () => {
    if (!rawConfigLink.trim()) {
      setMessage({ type: 'error', text: 'Connection link is empty!' });
      return;
    }

    try {
      let cleaned = rawConfigLink.trim();
      const urlObj = new URL(cleaned);
      const protocol = urlObj.protocol.replace(':', '');
      
      if (protocol !== 'vless' && protocol !== 'trojan') {
        setMessage({ type: 'error', text: `Unsupported protocol scheme: ${protocol}` });
        return;
      }

      const params = new URLSearchParams(urlObj.search);
      const host = urlObj.hostname;
      const port = Number(urlObj.port) || (protocol === 'trojan' ? 443 : 80);

      // Populate form state fields
      setTargetSni(params.get('sni') || host);
      setWebsocketHost(params.get('host') || host);
      setWebsocketPath(params.get('path') || '/');
      setRequireWs(params.get('type') === 'ws');
      setTargetMode(params.get('type') === 'ws' ? 'ws' : (params.get('security') === 'tls' ? 'tls' : 'http'));

      setMessage({ type: 'success', text: `Parsed outbound configuration link: ${protocol.toUpperCase()}://${host}:${port}` });
    } catch (err: any) {
      setMessage({ type: 'error', text: `Failed parsing configuration link: ${err.message}` });
    }
  };

  // Connect WebSocket channel
  useEffect(() => {
    const protocol = window.location.protocol === 'https:' ? 'wss:' : 'ws:';
    const activeToken = token || localStorage.getItem('cc_client_token') || '';
    const wsUrl = `${protocol}//${window.location.host}/ws?token=${activeToken}`;

    const socket = new WebSocket(wsUrl);

    socket.onopen = () => {
      setWsConnected(true);
      // Fetch initial scanner telemetry state
      socket.send(JSON.stringify({ type: 'scanner:telemetry', data: {} }));
    };

    socket.onmessage = (event) => {
      try {
        const msg = JSON.parse(event.data);
        if (msg.type === 'scanner:telemetry') {
          if (msg.stats) {
            setStats({
              tested: msg.stats.tested || 0,
              healthy: msg.stats.healthy || 0,
              failed: msg.stats.failed || 0,
              in_flight: msg.stats.in_flight || 0,
            });
            setIsScanning((msg.stats.in_flight || 0) > 0 || (msg.event === 'scanner.progress'));
          }

          if (msg.event === 'scanner.candidate' && msg.data) {
            const c = msg.data;
            setCandidates((prev) => {
              // Deduplicate
              const filtered = prev.filter((item) => item.ip !== c.ip || item.port !== c.port);
              return [
                {
                  ip: c.ip,
                  port: c.port,
                  protocol: c.protocol || 'ws',
                  latencyMs: c.latency_ms || c.latencyMs || 0,
                  speedMbps: c.speed_mbps || c.speedMbps || 0.0,
                  status: (c.latency_ms || c.latencyMs) > 0 ? 'healthy' : 'failed',
                  time: new Date().toLocaleTimeString(),
                },
                ...filtered,
              ];
            });
          }

          if (msg.event === 'scanner.finished') {
            setIsScanning(false);
            setMessage({ type: 'success', text: 'Scanning operations completed.' });
            // Fetch final list
            fetchSavedConfigs();
          }
        }
      } catch (err) {
        // Suppress json parsing issues
      }
    };

    socket.onclose = () => {
      setWsConnected(false);
      setIsScanning(false);
    };

    setWs(socket);

    return () => {
      socket.close();
    };
  }, [token]);

  // Load existing discovered configs
  const fetchSavedConfigs = async () => {
    try {
      const activeToken = token || localStorage.getItem('cc_client_token') || '';
      const response = await fetch('/api/v2ray/client/configs', {
        headers: { 'Authorization': `Bearer ${activeToken}` }
      });
      if (response.ok) {
        const list = await response.json();
        const parsed: ScannedCandidate[] = list
          .filter((item: any) => item.name && item.name.startsWith('Discovered-'))
          .map((item: any) => ({
            ip: item.address,
            port: item.port,
            protocol: item.network || 'tcp',
            latencyMs: item.latency_ms || item.latency || 0,
            speedMbps: item.speed_mbps || item.speed || 0.0,
            status: item.latency_ms > 0 ? 'healthy' : 'failed',
            time: 'Saved',
          }));
        setCandidates(parsed);
      }
    } catch (err) {
      console.error('Failed to load configs:', err);
    }
  };

  useEffect(() => {
    fetchSavedConfigs();
  }, []);

  // Keyboard shortcut listener (c key to copy results)
  useEffect(() => {
    const handleKeyDown = (e: KeyboardEvent) => {
      // Don't copy if user is typing in forms
      const tag = document.activeElement?.tagName.toLowerCase();
      if (tag === 'input' || tag === 'textarea') {
        return;
      }

      if (e.key === 'c' || e.key === 'C') {
        const healthyList = candidates
          .filter((c) => c.status === 'healthy')
          .map((c) => `${c.ip}:${c.port} (latency: ${c.latencyMs}ms, speed: ${c.speedMbps.toFixed(2)} Mbps)`)
          .join('\n');

        if (healthyList) {
          navigator.clipboard.writeText(healthyList);
          setMessage({ type: 'success', text: 'Copied verified IPs list to clipboard!' });
        } else {
          setMessage({ type: 'info', text: 'No verified candidates available to copy.' });
        }
      }
    };

    window.addEventListener('keydown', handleKeyDown);
    return () => {
      window.removeEventListener('keydown', handleKeyDown);
    };
  }, [candidates]);

  // Start scanning routine
  const handleStartScan = (retry = false) => {
    if (!wsConnected || !ws) {
      setMessage({ type: 'error', text: 'WebSocket connection not ready!' });
      return;
    }

    setMessage({ type: 'info', text: retry ? 'Retrying last configuration scan sweep...' : 'Launching network scan sweep...' });
    setIsScanning(true);
    setCandidates([]);
    
    const ports = selectedPorts
      .split(',')
      .map((p) => parseInt(p.trim()))
      .filter((p) => !isNaN(p));

    const cidrs = targetCidrs
      .split('\n')
      .map((c) => c.trim())
      .filter((c) => c.length > 0);

    const payload = {
      type: 'scanner:start',
      data: {
        target_cidrs: cidrs,
        selected_ports: ports,
        concurrency_limit: Number(concurrencyLimit),
        max_rate_limit: Number(maxRateLimit),
        network_timeout_ms: Number(networkTimeoutMs),
        probe_attempts: Number(probeAttempts),
        target_mode: targetMode,
        target_sni: targetSni,
        websocket_host: websocketHost,
        websocket_path: websocketPath,
        require_ws: requireWs,
        enable_neighbors: enableNeighbors,
        top_limit: Number(topLimit),
        total_target_count: 0,
        retry: retry,
      },
    };

    ws.send(JSON.stringify(payload));
  };

  const handleStopScan = () => {
    if (!ws) return;
    ws.send(JSON.stringify({ type: 'scanner:stop', data: {} }));
    setIsScanning(false);
    setMessage({ type: 'info', text: 'Scan canceled by user.' });
  };

  // Drag and drop target files processing
  const handleDragEnter = (e: React.DragEvent) => {
    e.preventDefault();
    e.stopPropagation();
    dragCounter.current++;
    if (e.dataTransfer.items && e.dataTransfer.items.length > 0) {
      setIsDragging(true);
    }
  };

  const handleDragLeave = (e: React.DragEvent) => {
    e.preventDefault();
    e.stopPropagation();
    dragCounter.current--;
    if (dragCounter.current === 0) {
      setIsDragging(false);
    }
  };

  const handleDragOver = (e: React.DragEvent) => {
    e.preventDefault();
    e.stopPropagation();
  };

  const handleDrop = (e: React.DragEvent) => {
    e.preventDefault();
    e.stopPropagation();
    setIsDragging(false);
    dragCounter.current = 0;

    const files = e.dataTransfer.files;
    if (files && files.length > 0) {
      const file = files[0];
      const reader = new FileReader();
      reader.onload = (event) => {
        const text = event.target?.result as string;
        if (text) {
          // Clean empty lines and extract valid entries
          const lines = text
            .split('\n')
            .map((line) => line.trim())
            .filter((line) => line.length > 0 && !line.startsWith('#'));
          
          setTargetCidrs(lines.join('\n'));
          setMessage({ type: 'success', text: `Imported ${lines.length} network targets from ${file.name}` });
        }
      };
      reader.readAsText(file);
    }
  };

  // Filtering and sorting logic
  const filteredCandidates = candidates
    .filter((c) => {
      const q = searchQuery.toLowerCase().trim();
      return c.ip.toLowerCase().includes(q) || c.port.toString().includes(q);
    })
    .sort((a, b) => {
      if (sortBy === 'latency') {
        return a.latencyMs - b.latencyMs;
      } else if (sortBy === 'speed') {
        return b.speedMbps - a.speedMbps;
      }
      return 0; // standard order
    });

  return (
    <div style={{ padding: '32px 40px', background: 'var(--color-brand-bg, #0f111a)', minHeight: '100vh', fontFamily: '"Inter", sans-serif' }}>
      
      {/* Header section */}
      <div style={{ display: 'flex', justifyContent: 'space-between', alignItems: 'center', marginBottom: 24 }}>
        <div>
          <h1 style={{ fontSize: 24, fontWeight: 'bold', color: 'var(--color-brand-heading, #fff)', margin: 0, letterSpacing: '-0.5px' }}>V2Ray Network Scanner</h1>
          <p style={{ fontSize: 13, color: 'var(--color-brand-muted, #94a3b8)', margin: '4px 0 0' }}>
            High-velocity deep-packet inspection (DPI) bypass verification engine. Emulates browser handshakes and tests proxy throughput in memory.
          </p>
        </div>
        <div style={{ display: 'flex', alignItems: 'center', gap: 12 }}>
          <div style={{ display: 'flex', alignItems: 'center', gap: 8, padding: '6px 14px', background: 'rgba(255,255,255,0.03)', border: '1px solid rgba(255,255,255,0.05)', borderRadius: 20 }}>
            <span style={{ display: 'flex', width: 8, height: 8, position: 'relative' }}>
              <span style={{ position: 'absolute', width: '100%', height: '100%', borderRadius: '50%', backgroundColor: wsConnected ? '#4ade80' : '#f87171', opacity: 0.7, animation: 'ping 1s cubic-bezier(0, 0, 0.2, 1) infinite' }} />
              <span style={{ position: 'relative', width: '100%', height: '100%', borderRadius: '50%', backgroundColor: wsConnected ? '#22c55e' : '#ef4444' }} />
            </span>
            <span style={{ fontSize: 12, fontWeight: 500, color: 'var(--color-brand-heading, #fff)' }}>
              {wsConnected ? 'Telemetry Stream Connected' : 'Offline'}
            </span>
          </div>
          <button className="btn btn--sm btn--secondary" onClick={fetchSavedConfigs}>
            <FiRefreshCw style={{ marginRight: 6 }} /> Refresh
          </button>
        </div>
      </div>

      {message && (
        <div style={{
          padding: '12px 18px',
          borderRadius: 10,
          marginBottom: 24,
          fontSize: 13,
          fontWeight: 500,
          display: 'flex',
          alignItems: 'center',
          gap: 10,
          background: message.type === 'success' ? 'rgba(34, 197, 94, 0.1)' : message.type === 'error' ? 'rgba(239, 68, 68, 0.1)' : 'rgba(59, 130, 246, 0.1)',
          border: message.type === 'success' ? '1px solid rgba(34, 197, 94, 0.2)' : message.type === 'error' ? '1px solid rgba(239, 68, 68, 0.2)' : '1px solid rgba(59, 130, 246, 0.2)',
          color: message.type === 'success' ? '#4ade80' : message.type === 'error' ? '#f87171' : '#60a5fa'
        }}>
          {message.type === 'success' ? <FiCheckCircle size={16} /> : <FiAlertCircle size={16} />}
          <span>{message.text}</span>
        </div>
      )}

      {/* Summary Cards */}
      <div style={{ display: 'grid', gridTemplateColumns: 'repeat(auto-fit, minmax(220px, 1fr))', gap: 16, marginBottom: 24 }}>
        <div style={{ background: 'var(--color-brand-card, #111827)', border: '1px solid var(--color-brand-border, rgba(255,255,255,0.05))', borderRadius: 12, padding: '16px 20px' }}>
          <div style={{ fontSize: 12, color: 'var(--color-brand-muted, #94a3b8)', fontWeight: 500, textTransform: 'uppercase', letterSpacing: '0.5px' }}>Tested Targets</div>
          <div style={{ fontSize: 24, fontWeight: 700, color: 'var(--color-brand-heading, #fff)', marginTop: 8 }}>{stats.tested}</div>
        </div>
        <div style={{ background: 'var(--color-brand-card, #111827)', border: '1px solid var(--color-brand-border, rgba(255,255,255,0.05))', borderRadius: 12, padding: '16px 20px' }}>
          <div style={{ fontSize: 12, color: '#22c55e', fontWeight: 500, textTransform: 'uppercase', letterSpacing: '0.5px' }}>Healthy Nodes</div>
          <div style={{ fontSize: 24, fontWeight: 700, color: '#4ade80', marginTop: 8 }}>{stats.healthy}</div>
        </div>
        <div style={{ background: 'var(--color-brand-card, #111827)', border: '1px solid var(--color-brand-border, rgba(255,255,255,0.05))', borderRadius: 12, padding: '16px 20px' }}>
          <div style={{ fontSize: 12, color: '#ef4444', fontWeight: 500, textTransform: 'uppercase', letterSpacing: '0.5px' }}>Failed Nodes</div>
          <div style={{ fontSize: 24, fontWeight: 700, color: '#f87171', marginTop: 8 }}>{stats.failed}</div>
        </div>
        <div style={{ background: 'var(--color-brand-card, #111827)', border: '1px solid var(--color-brand-border, rgba(255,255,255,0.05))', borderRadius: 12, padding: '16px 20px' }}>
          <div style={{ fontSize: 12, color: '#3b82f6', fontWeight: 500, textTransform: 'uppercase', letterSpacing: '0.5px' }}>Active Workers</div>
          <div style={{ fontSize: 24, fontWeight: 700, color: '#60a5fa', marginTop: 8 }}>{stats.in_flight}</div>
        </div>
      </div>

      {/* Main Grid */}
      <div style={{ display: 'grid', gridTemplateColumns: '380px 1fr', gap: 24 }}>
        
        {/* Left column: inputs / parser controls */}
        <div style={{ display: 'flex', flexDirection: 'column', gap: 20 }}>
          
          {/* Link Parser box */}
          <div className="g-card" style={{ padding: 20, background: 'var(--color-brand-card, #111827)', border: '1px solid var(--color-brand-border, rgba(255,255,255,0.05))', borderRadius: 12 }}>
            <h3 style={{ fontSize: 14, fontWeight: 600, color: 'var(--color-brand-heading, #fff)', margin: '0 0 12px' }}>Connection Link Parser</h3>
            <div style={{ display: 'flex', flexDirection: 'column', gap: 10 }}>
              <input
                type="text"
                value={rawConfigLink}
                onChange={(e) => setRawConfigLink(e.target.value)}
                placeholder="Paste VLESS / Trojan outbound URL here..."
                style={{
                  width: '100%',
                  padding: '10px 12px',
                  borderRadius: 8,
                  border: '1px solid var(--color-brand-border)',
                  background: 'var(--color-brand-bg, #0f111a)',
                  color: 'var(--color-brand-heading)',
                  fontSize: 12
                }}
              />
              <button className="btn btn--primary btn--sm" onClick={handleParseLink} style={{ justifyContent: 'center' }}>
                Parse Outbound Link
              </button>
            </div>
          </div>

          {/* Sweep parameter inputs */}
          <div className="g-card" style={{ padding: 20, background: 'var(--color-brand-card, #111827)', border: '1px solid var(--color-brand-border, rgba(255,255,255,0.05))', borderRadius: 12 }}>
            <h3 style={{ fontSize: 14, fontWeight: 600, color: 'var(--color-brand-heading, #fff)', margin: '0 0 16px' }}>Scanner Parameters</h3>
            
            <div style={{ display: 'flex', flexDirection: 'column', gap: 14 }}>
              
              {/* Drag and Drop Area */}
              <div 
                onDragEnter={handleDragEnter}
                onDragOver={handleDragOver}
                onDragLeave={handleDragLeave}
                onDrop={handleDrop}
                style={{
                  border: isDragging ? '2px dashed var(--color-brand, #3b82f6)' : '1px dashed var(--color-brand-border)',
                  background: isDragging ? 'rgba(59, 130, 246, 0.05)' : 'rgba(255,255,255,0.01)',
                  borderRadius: 8,
                  padding: 16,
                  textAlign: 'center',
                  cursor: 'pointer',
                  transition: 'all 0.2s ease-in-out'
                }}
              >
                <FiUpload size={20} style={{ color: 'var(--color-brand-muted)', marginBottom: 8 }} />
                <div style={{ fontSize: 12, fontWeight: 500, color: 'var(--color-brand-heading)' }}>
                  Drag & Drop CIDR Target File
                </div>
                <div style={{ fontSize: 10, color: 'var(--color-brand-muted)', marginTop: 4 }}>
                  Supports plain text (.txt) and CSV files
                </div>
              </div>

              <div>
                <label style={{ display: 'block', fontSize: 11, fontWeight: 600, color: 'var(--color-brand-muted)', marginBottom: 6, textTransform: 'uppercase' }}>Target CIDR Subnets</label>
                <textarea
                  value={targetCidrs}
                  onChange={(e) => setTargetCidrs(e.target.value)}
                  rows={4}
                  style={{
                    width: '100%',
                    padding: '8px 10px',
                    borderRadius: 8,
                    border: '1px solid var(--color-brand-border)',
                    background: 'var(--color-brand-bg)',
                    fontSize: 12,
                    color: 'var(--color-brand-heading)',
                    resize: 'none'
                  }}
                />
              </div>

              <div style={{ display: 'flex', flexDirection: 'column', gap: 10 }}>
                <div>
                  <label style={{ display: 'block', fontSize: 11, fontWeight: 600, color: 'var(--color-brand-muted)', marginBottom: 6, textTransform: 'uppercase' }}>Ports (comma-separated)</label>
                  <input
                    type="text"
                    value={selectedPorts}
                    onChange={(e) => setSelectedPorts(e.target.value)}
                    style={{ width: '100%', padding: 8, borderRadius: 6, border: '1px solid var(--color-brand-border)', background: 'var(--color-brand-bg)', color: 'var(--color-brand-heading)', fontSize: 12 }}
                  />
                </div>
                <div>
                  <label style={{ display: 'block', fontSize: 11, fontWeight: 600, color: 'var(--color-brand-muted)', marginBottom: 6, textTransform: 'uppercase' }}>Concurrency Limit</label>
                  <input
                    type="number"
                    value={concurrencyLimit}
                    onChange={(e) => setConcurrencyLimit(Number(e.target.value))}
                    style={{ width: '100%', padding: 8, borderRadius: 6, border: '1px solid var(--color-brand-border)', background: 'var(--color-brand-bg)', color: 'var(--color-brand-heading)', fontSize: 12 }}
                  />
                </div>
              </div>

              <div style={{ display: 'flex', flexDirection: 'column', gap: 10 }}>
                <div>
                  <label style={{ display: 'block', fontSize: 11, fontWeight: 600, color: 'var(--color-brand-muted)', marginBottom: 6, textTransform: 'uppercase' }}>Timeout (ms)</label>
                  <input
                    type="number"
                    value={networkTimeoutMs}
                    onChange={(e) => setNetworkTimeoutMs(Number(e.target.value))}
                    style={{ width: '100%', padding: 8, borderRadius: 6, border: '1px solid var(--color-brand-border)', background: 'var(--color-brand-bg)', color: 'var(--color-brand-heading)', fontSize: 12 }}
                  />
                </div>
                <div>
                  <label style={{ display: 'block', fontSize: 11, fontWeight: 600, color: 'var(--color-brand-muted)', marginBottom: 6, textTransform: 'uppercase' }}>Max Rate Limit</label>
                  <input
                    type="number"
                    value={maxRateLimit}
                    onChange={(e) => setMaxRateLimit(Number(e.target.value))}
                    style={{ width: '100%', padding: 8, borderRadius: 6, border: '1px solid var(--color-brand-border)', background: 'var(--color-brand-bg)', color: 'var(--color-brand-heading)', fontSize: 12 }}
                  />
                </div>
              </div>

              <div style={{ display: 'flex', flexDirection: 'column', gap: 10 }}>
                <div>
                  <label style={{ display: 'block', fontSize: 11, fontWeight: 600, color: 'var(--color-brand-muted)', marginBottom: 6, textTransform: 'uppercase' }}>Target Mode</label>
                  <select
                    value={targetMode}
                    onChange={(e: any) => setTargetMode(e.target.value)}
                    style={{ width: '100%', padding: 8, borderRadius: 6, border: '1px solid var(--color-brand-border)', background: 'var(--color-brand-bg)', color: 'var(--color-brand-heading)', fontSize: 12 }}
                  >
                    <option value="ws">WebSocket (WS)</option>
                    <option value="tls">Direct TLS</option>
                    <option value="http">HTTP Direct</option>
                  </select>
                </div>
                <div>
                  <label style={{ display: 'block', fontSize: 11, fontWeight: 600, color: 'var(--color-brand-muted)', marginBottom: 6, textTransform: 'uppercase' }}>Attempts</label>
                  <input
                    type="number"
                    value={probeAttempts}
                    onChange={(e) => setProbeAttempts(Number(e.target.value))}
                    style={{ width: '100%', padding: 8, borderRadius: 6, border: '1px solid var(--color-brand-border)', background: 'var(--color-brand-bg)', color: 'var(--color-brand-heading)', fontSize: 12 }}
                  />
                </div>
              </div>

              <div>
                <label style={{ display: 'block', fontSize: 11, fontWeight: 600, color: 'var(--color-brand-muted)', marginBottom: 6, textTransform: 'uppercase' }}>Target SNI</label>
                <input
                  type="text"
                  value={targetSni}
                  onChange={(e) => setTargetSni(e.target.value)}
                  style={{ width: '100%', padding: 8, borderRadius: 6, border: '1px solid var(--color-brand-border)', background: 'var(--color-brand-bg)', color: 'var(--color-brand-heading)', fontSize: 12 }}
                />
              </div>

              {targetMode === 'ws' && (
                <div style={{ display: 'flex', flexDirection: 'column', gap: 10 }}>
                  <div>
                    <label style={{ display: 'block', fontSize: 11, fontWeight: 600, color: 'var(--color-brand-muted)', marginBottom: 6, textTransform: 'uppercase' }}>WS Host</label>
                    <input
                      type="text"
                      value={websocketHost}
                      onChange={(e) => setWebsocketHost(e.target.value)}
                      style={{ width: '100%', padding: 8, borderRadius: 6, border: '1px solid var(--color-brand-border)', background: 'var(--color-brand-bg)', color: 'var(--color-brand-heading)', fontSize: 12 }}
                    />
                  </div>
                  <div>
                    <label style={{ display: 'block', fontSize: 11, fontWeight: 600, color: 'var(--color-brand-muted)', marginBottom: 6, textTransform: 'uppercase' }}>WS Path</label>
                    <input
                      type="text"
                      value={websocketPath}
                      onChange={(e) => setWebsocketPath(e.target.value)}
                      style={{ width: '100%', padding: 8, borderRadius: 6, border: '1px solid var(--color-brand-border)', background: 'var(--color-brand-bg)', color: 'var(--color-brand-heading)', fontSize: 12 }}
                    />
                  </div>
                </div>
              )}

              <div style={{ display: 'flex', justifyContent: 'space-between', alignItems: 'center', marginTop: 6 }}>
                <div>
                  <span style={{ fontSize: 12, fontWeight: 600, color: 'var(--color-brand-heading)' }}>Auto-Discovery of Neighbors</span>
                  <p style={{ margin: 0, fontSize: 10, color: 'var(--color-brand-muted)' }}>Scan +/- 5 adjacent subnets hosts.</p>
                </div>
                <input
                  type="checkbox"
                  checked={enableNeighbors}
                  onChange={(e) => setEnableNeighbors(e.target.checked)}
                  style={{ width: 16, height: 16, accentColor: 'var(--color-brand)' }}
                />
              </div>

              <div style={{ display: 'flex', gap: 10, marginTop: 12 }}>
                {isScanning ? (
                  <button className="btn" onClick={handleStopScan} style={{ flex: 1, justifyContent: 'center', background: '#ef4444', borderColor: '#ef4444', color: '#fff' }}>
                    <FiSquare style={{ marginRight: 6 }} /> Stop Scan
                  </button>
                ) : (
                  <>
                    <button className="btn btn--primary" onClick={() => handleStartScan(false)} style={{ flex: 1, justifyContent: 'center' }}>
                      <FiPlay style={{ marginRight: 6 }} /> Start Sweep
                    </button>
                    <button className="btn btn--secondary" onClick={() => handleStartScan(true)} title="Run the last saved settings scanner sweep">
                      Retry Last
                    </button>
                  </>
                )}
              </div>

            </div>
          </div>
        </div>

        {/* Right column: results grid table */}
        <div className="g-card" style={{ padding: 24, background: 'var(--color-brand-card, #111827)', border: '1px solid var(--color-brand-border, rgba(255,255,255,0.05))', borderRadius: 12, display: 'flex', flexDirection: 'column' }}>
          
          {/* Controls line */}
          <div style={{ display: 'flex', justifyContent: 'space-between', alignItems: 'center', marginBottom: 20, gap: 16 }}>
            <div style={{ display: 'flex', alignItems: 'center', gap: 8 }}>
              <FiList style={{ color: 'var(--color-brand)', fontSize: 18 }} />
              <span style={{ fontSize: 15, fontWeight: 600, color: 'var(--color-brand-heading)' }}>Discovered Proxy Candidates ({filteredCandidates.length})</span>
            </div>
            
            <div style={{ display: 'flex', alignItems: 'center', gap: 10 }}>
              <div style={{ position: 'relative', width: 220 }}>
                <FiSearch style={{ position: 'absolute', left: 10, top: 10, color: 'var(--color-brand-muted)' }} />
                <input
                  type="text"
                  value={searchQuery}
                  onChange={(e) => setSearchQuery(e.target.value)}
                  placeholder="Search candidate IP/port..."
                  style={{
                    width: '100%',
                    padding: '8px 12px 8px 30px',
                    borderRadius: 8,
                    border: '1px solid var(--color-brand-border)',
                    background: 'var(--color-brand-bg, #0f111a)',
                    color: 'var(--color-brand-heading)',
                    fontSize: 12
                  }}
                />
              </div>

              <select
                value={sortBy}
                onChange={(e: any) => setSortBy(e.target.value)}
                style={{
                  padding: '8px 12px',
                  borderRadius: 8,
                  border: '1px solid var(--color-brand-border)',
                  background: 'var(--color-brand-bg, #0f111a)',
                  color: 'var(--color-brand-heading)',
                  fontSize: 12
                }}
              >
                <option value="latency">Sort by Latency</option>
                <option value="speed">Sort by Speed</option>
                <option value="time">Sort by Date</option>
              </select>

              <button 
                className="btn btn--sm btn--secondary" 
                onClick={() => {
                  const healthyList = candidates
                    .filter((c) => c.status === 'healthy')
                    .map((c) => `${c.ip}:${c.port}`)
                    .join('\n');
                  if (healthyList) {
                    navigator.clipboard.writeText(healthyList);
                    setMessage({ type: 'success', text: 'Copied verified IPs to clipboard!' });
                  } else {
                    setMessage({ type: 'info', text: 'No verified candidates available to copy.' });
                  }
                }}
                title="Copy all healthy candidates (shortcut: C)"
              >
                <FiClipboard /> Copy All
              </button>
            </div>
          </div>

          {/* Results table */}
          <div style={{ flex: 1, overflowY: 'auto', maxHeight: 600 }}>
            <table style={{ width: '100%', borderCollapse: 'collapse', fontSize: 13, textAlign: 'left' }}>
              <thead>
                <tr style={{ borderBottom: '1px solid var(--color-brand-border)' }}>
                  <th style={{ padding: '12px 8px', color: 'var(--color-brand-muted)', fontWeight: 600 }}>Endpoint Address</th>
                  <th style={{ padding: '12px 8px', color: 'var(--color-brand-muted)', fontWeight: 600 }}>Port</th>
                  <th style={{ padding: '12px 8px', color: 'var(--color-brand-muted)', fontWeight: 600 }}>Protocol</th>
                  <th style={{ padding: '12px 8px', color: 'var(--color-brand-muted)', fontWeight: 600 }}>Latency</th>
                  <th style={{ padding: '12px 8px', color: 'var(--color-brand-muted)', fontWeight: 600 }}>Throughput</th>
                  <th style={{ padding: '12px 8px', color: 'var(--color-brand-muted)', fontWeight: 600 }}>Status</th>
                  <th style={{ padding: '12px 8px', color: 'var(--color-brand-muted)', fontWeight: 600, textAlign: 'right' }}>Actions</th>
                </tr>
              </thead>
              <tbody>
                {filteredCandidates.length === 0 ? (
                  <tr>
                    <td colSpan={7} style={{ padding: 40, textAlign: 'center', color: 'var(--color-brand-muted)' }}>
                      <FiFileText size={24} style={{ marginBottom: 12, opacity: 0.3 }} />
                      <div>No discovered candidates matching filters.</div>
                      <div style={{ fontSize: 11, marginTop: 4 }}>Drag-and-drop subnets list or click "Start Sweep" to begin.</div>
                    </td>
                  </tr>
                ) : (
                  filteredCandidates.map((c, idx) => (
                    <tr key={idx} style={{ borderBottom: '1px solid rgba(255,255,255,0.02)', verticalAlign: 'middle' }}>
                      <td style={{ padding: '12px 8px', fontWeight: 600, color: 'var(--color-brand-heading)' }}>{c.ip}</td>
                      <td style={{ padding: '12px 8px', color: 'var(--color-brand-heading)' }}>{c.port}</td>
                      <td style={{ padding: '12px 8px', color: 'var(--color-brand-muted)' }}>
                        <span style={{ fontSize: 10, padding: '2px 6px', background: 'rgba(255,255,255,0.05)', borderRadius: 4, textTransform: 'uppercase' }}>
                          {c.protocol}
                        </span>
                      </td>
                      <td style={{ padding: '12px 8px', fontWeight: 500, color: c.latencyMs > 0 ? '#4ade80' : '#ef4444' }}>
                        {c.latencyMs > 0 ? `${c.latencyMs} ms` : '-'}
                      </td>
                      <td style={{ padding: '12px 8px', color: '#60a5fa', fontWeight: 500 }}>
                        {c.speedMbps > 0 ? `${c.speedMbps.toFixed(2)} Mbps` : '-'}
                      </td>
                      <td style={{ padding: '12px 8px' }}>
                        <span style={{
                          fontSize: 10,
                          fontWeight: 600,
                          padding: '3px 8px',
                          borderRadius: 20,
                          background: c.status === 'healthy' ? 'rgba(34,197,94,0.1)' : c.status === 'failed' ? 'rgba(239,68,68,0.1)' : 'rgba(59,130,246,0.1)',
                          color: c.status === 'healthy' ? '#22c55e' : c.status === 'failed' ? '#ef4444' : '#3b82f6',
                        }}>
                          {c.status.toUpperCase()}
                        </span>
                      </td>
                      <td style={{ padding: '12px 8px', textAlign: 'right' }}>
                        <button
                          className="btn btn--xs btn--secondary"
                          onClick={() => {
                            navigator.clipboard.writeText(`${c.ip}:${c.port}`);
                            setMessage({ type: 'success', text: `Copied endpoint address ${c.ip}:${c.port} to clipboard.` });
                          }}
                        >
                          Copy
                        </button>
                      </td>
                    </tr>
                  ))
                )}
              </tbody>
            </table>
          </div>

        </div>

      </div>

    </div>
  );
};
