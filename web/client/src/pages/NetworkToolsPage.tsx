import React, { useState, useEffect, useRef } from 'react';
import { 
  FiRefreshCw, FiPlay, FiSquare, FiDownload, FiUpload, FiSettings, 
  FiSearch, FiClipboard, FiAlertCircle, FiCheckCircle, FiFileText, FiList,
  FiTerminal, FiTrash2
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

  // Logs state
  const [scannerLogs, setScannerLogs] = useState<string[]>([]);
  const [logsFilter, setLogsFilter] = useState('');

  // Input states (ScanConfig alignment)
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
  const [totalTargetCount, setTotalTargetCount] = useState(0);

  // Drag and drop state
  const [isDragging, setIsDragging] = useState(false);
  const [message, setMessage] = useState<{ type: 'success' | 'error' | 'info'; text: string } | null>(null);

  const dragCounter = useRef(0);
  const logsContainerRef = useRef<HTMLDivElement | null>(null);

  // Auto scroll logic for logs terminal
  useEffect(() => {
    if (logsContainerRef.current) {
      logsContainerRef.current.scrollTop = logsContainerRef.current.scrollHeight;
    }
  }, [scannerLogs]);

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

          if (msg.event === 'scanner.log' && msg.data) {
            setScannerLogs((prev) => [...prev, String(msg.data)]);
          }

          if (msg.event === 'scanner.error' && msg.data) {
            setMessage({ type: 'error', text: String(msg.data) });
            setScannerLogs((prev) => [...prev, `[ERROR] ${msg.data}`]);
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
        const parsed: ScannedCandidate[] = (list.data || list)
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

  const downloadTxt = () => {
    const healthyList = candidates
      .filter((c) => c.status === 'healthy')
      .map((c) => `${c.ip}:${c.port}`)
      .join('\n');
    if (!healthyList) {
      setMessage({ type: 'info', text: 'No verified candidates available to download.' });
      return;
    }
    const blob = new Blob([healthyList], { type: 'text/plain;charset=utf-8' });
    const url = URL.createObjectURL(blob);
    const link = document.createElement('a');
    link.href = url;
    link.download = 'healthy_ips.txt';
    document.body.appendChild(link);
    link.click();
    document.body.removeChild(link);
    URL.revokeObjectURL(url);
    setMessage({ type: 'success', text: 'Downloaded healthy_ips.txt successfully.' });
  };

  const downloadCsv = () => {
    const healthy = candidates.filter((c) => c.status === 'healthy');
    if (healthy.length === 0) {
      setMessage({ type: 'info', text: 'No verified candidates available to download.' });
      return;
    }
    const headers = ['IP', 'Port', 'Protocol', 'Latency(ms)', 'Speed(Mbps)'];
    const rows = healthy.map((c) => [
      c.ip,
      c.port,
      c.protocol,
      c.latencyMs,
      c.speedMbps.toFixed(2)
    ]);
    const content = [headers.join(','), ...rows.map((r) => r.join(','))].join('\n');
    const blob = new Blob([content], { type: 'text/csv;charset=utf-8' });
    const url = URL.createObjectURL(blob);
    const link = document.createElement('a');
    link.href = url;
    link.download = 'healthy_ips.csv';
    document.body.appendChild(link);
    link.click();
    document.body.removeChild(link);
    URL.revokeObjectURL(url);
    setMessage({ type: 'success', text: 'Downloaded healthy_ips.csv successfully.' });
  };

  useEffect(() => {
    fetchSavedConfigs();
  }, []);

  // Keyboard shortcut listener (c key to copy results)
  useEffect(() => {
    const handleKeyDown = (e: KeyboardEvent) => {
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
  const handleStartScan = (retry = false, scanDiscoveredOnly = false) => {
    if (!wsConnected || !ws) {
      setMessage({ type: 'error', text: 'WebSocket connection not ready!' });
      return;
    }

    let msg = 'Launching network scan sweep...';
    if (retry) {
      msg = 'Retrying last configuration scan sweep...';
    } else if (scanDiscoveredOnly) {
      msg = 'Rerunning scan on saved healthy discovered list...';
    }

    setMessage({ type: 'info', text: msg });
    setIsScanning(true);
    setCandidates([]);
    setScannerLogs([]); // Clear logs for the new sweep session
    
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
        total_target_count: Number(totalTargetCount),
        retry: retry,
        scan_discovered_only: scanDiscoveredOnly,
      },
    };

    ws.send(JSON.stringify(payload));
  };

  const handleCleanupDiscovered = async () => {
    if (!confirm('Are you sure you want to clean up all failed candidates from the database?')) {
      return;
    }
    try {
      const res = await fetch('/api/v2ray/client/configs/failed', {
        method: 'DELETE',
        headers: {
          'Authorization': `Bearer ${token}`
        }
      });
      if (res.ok) {
        const data = await res.json();
        setMessage({ type: 'success', text: `Cleaned up ${data.count || 0} failed candidates.` });
        fetchSavedConfigs();
      } else {
        setMessage({ type: 'error', text: 'Failed to clean up configs.' });
      }
    } catch (err) {
      console.error(err);
      setMessage({ type: 'error', text: 'Network error during cleanup.' });
    }
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

  // Filtering and sorting logic for candidates
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
      return 0;
    });

  // Filtering logs
  const filteredLogs = scannerLogs.filter((log) => {
    return log.toLowerCase().includes(logsFilter.toLowerCase());
  });

  return (
    <div className="page-container animate-fade-in" style={{ padding: '4px 0', fontFamily: 'var(--font-sans)' }}>
      
      {/* Header section */}
      <div style={{ display: 'flex', justifyContent: 'space-between', alignItems: 'center', marginBottom: 20 }}>
        <div>
          <h1 style={{ fontSize: 22, fontWeight: 700, color: 'var(--color-brand-heading)', margin: 0 }}>
            V2Ray Network Scanner
          </h1>
          <p style={{ fontSize: 12, color: 'var(--color-brand-text)', margin: '4px 0 0' }}>
            High-velocity deep-packet inspection (DPI) bypass verification engine. Emulates TLS and WebSocket handshakes.
          </p>
        </div>
        
        <div style={{ display: 'flex', alignItems: 'center', gap: 12 }}>
          <div style={{ display: 'flex', alignItems: 'center', gap: 8, padding: '6px 14px', background: 'var(--color-brand-light)', border: '1px solid var(--color-brand-border)', borderRadius: 20 }}>
            <span style={{ display: 'flex', width: 8, height: 8, position: 'relative' }}>
              <span style={{ position: 'absolute', width: '100%', height: '100%', borderRadius: '50%', backgroundColor: wsConnected ? 'var(--color-brand-green)' : 'var(--color-brand-red)', opacity: 0.7, animation: 'ping 1s cubic-bezier(0, 0, 0.2, 1) infinite' }} />
              <span style={{ position: 'relative', width: '100%', height: '100%', borderRadius: '50%', backgroundColor: wsConnected ? 'var(--color-brand-green)' : 'var(--color-brand-red)' }} />
            </span>
            <span style={{ fontSize: 12, fontWeight: 600, color: 'var(--color-brand-heading)' }}>
              {wsConnected ? 'Telemetry Connected' : 'Offline'}
            </span>
          </div>
          <button className="btn btn--sm btn--secondary" onClick={fetchSavedConfigs}>
            <FiRefreshCw style={{ marginRight: 6 }} /> Refresh List
          </button>
        </div>
      </div>

      {message && (
        <div style={{
          padding: '12px 18px',
          borderRadius: 10,
          marginBottom: 20,
          fontSize: 13,
          fontWeight: 500,
          display: 'flex',
          alignItems: 'center',
          gap: 10,
          background: message.type === 'success' ? 'var(--color-brand-light)' : 'rgba(239, 68, 68, 0.08)',
          border: '1px solid var(--color-brand-border)',
          color: message.type === 'success' ? 'var(--color-brand)' : '#ef4444'
        }}>
          {message.type === 'success' ? <FiCheckCircle size={16} /> : <FiAlertCircle size={16} />}
          <span>{message.text}</span>
        </div>
      )}

      {/* Summary Cards */}
      <div style={{ display: 'grid', gridTemplateColumns: 'repeat(auto-fit, minmax(180px, 1fr))', gap: 16, marginBottom: 20 }}>
        <div className="g-card" style={{ padding: '14px 20px', borderLeft: '3px solid var(--color-brand-text)' }}>
          <div style={{ fontSize: 11, color: 'var(--color-brand-text)', fontWeight: 600, textTransform: 'uppercase', letterSpacing: '0.5px' }}>Tested Targets</div>
          <div style={{ fontSize: 24, fontWeight: 700, color: 'var(--color-brand-heading)', marginTop: 4 }}>{stats.tested}</div>
        </div>
        <div className="g-card" style={{ padding: '14px 20px', borderLeft: '3px solid var(--color-brand-green)' }}>
          <div style={{ fontSize: 11, color: 'var(--color-brand-green)', fontWeight: 600, textTransform: 'uppercase', letterSpacing: '0.5px' }}>Healthy Nodes</div>
          <div style={{ fontSize: 24, fontWeight: 700, color: 'var(--color-brand-green)', marginTop: 4 }}>{stats.healthy}</div>
        </div>
        <div className="g-card" style={{ padding: '14px 20px', borderLeft: '3px solid var(--color-brand-red)' }}>
          <div style={{ fontSize: 11, color: 'var(--color-brand-red)', fontWeight: 600, textTransform: 'uppercase', letterSpacing: '0.5px' }}>Failed Nodes</div>
          <div style={{ fontSize: 24, fontWeight: 700, color: 'var(--color-brand-red)', marginTop: 4 }}>{stats.failed}</div>
        </div>
        <div className="g-card" style={{ padding: '14px 20px', borderLeft: '3px solid var(--color-brand-blue)' }}>
          <div style={{ fontSize: 11, color: 'var(--color-brand-blue)', fontWeight: 600, textTransform: 'uppercase', letterSpacing: '0.5px' }}>Active Workers</div>
          <div style={{ fontSize: 24, fontWeight: 700, color: 'var(--color-brand-blue)', marginTop: 4 }}>{stats.in_flight}</div>
        </div>
      </div>

      {/* Main Grid Layout */}
      <div style={{ display: 'grid', gridTemplateColumns: '360px 1fr', gap: 20 }}>
        
        {/* Left Column: Tuner Configuration Controls */}
        <div style={{ display: 'flex', flexDirection: 'column', gap: 16 }}>
          
          {/* Card 1: Connection Link Parser */}
          <div className="g-card" style={{ padding: 16 }}>
            <h3 style={{ fontSize: 13, fontWeight: 600, color: 'var(--color-brand-heading)', margin: '0 0 12px' }}>Connection Link Parser</h3>
            <div style={{ display: 'flex', flexDirection: 'column', gap: 10 }}>
              <input
                type="text"
                value={rawConfigLink}
                onChange={(e) => setRawConfigLink(e.target.value)}
                placeholder="Paste vless:// or trojan:// outbound URL..."
                style={{
                  width: '100%',
                  padding: '8px 12px',
                  borderRadius: 6,
                  border: '1px solid var(--color-brand-border)',
                  background: 'var(--color-brand-bg)',
                  color: 'var(--color-brand-heading)',
                  fontSize: 12,
                  outline: 'none'
                }}
              />
              <button className="btn btn--primary btn--sm" onClick={handleParseLink} style={{ justifyContent: 'center' }}>
                Parse URI Link
              </button>
            </div>
          </div>

          {/* Card 2: Sweep Parameters Tuning */}
          <div className="g-card" style={{ padding: 16 }}>
            <h3 style={{ fontSize: 13, fontWeight: 600, color: 'var(--color-brand-heading)', margin: '0 0 16px' }}>Scan Settings Center</h3>
            
            <div style={{ display: 'flex', flexDirection: 'column', gap: 12 }}>
              
              {/* Drag and Drop Zone */}
              <div 
                onDragEnter={handleDragEnter}
                onDragOver={handleDragOver}
                onDragLeave={handleDragLeave}
                onDrop={handleDrop}
                style={{
                  border: isDragging ? '2px dashed var(--color-brand)' : '1px dashed var(--color-brand-border)',
                  background: isDragging ? 'var(--color-brand-light)' : 'rgba(255,255,255,0.01)',
                  borderRadius: 8,
                  padding: 14,
                  textAlign: 'center',
                  cursor: 'pointer',
                  transition: 'all 0.15s ease'
                }}
              >
                <FiUpload size={18} style={{ color: 'var(--color-brand-muted)', marginBottom: 6 }} />
                <div style={{ fontSize: 11, fontWeight: 600, color: 'var(--color-brand-heading)' }}>
                  Drag & Drop CIDR Subnets File
                </div>
                <div style={{ fontSize: 10, color: 'var(--color-brand-muted)', marginTop: 2 }}>
                  Accepts standard text (.txt) files
                </div>
              </div>

              {/* CIDRs subnets textarea */}
              <div>
                <label style={{ display: 'block', fontSize: 10, fontWeight: 700, color: 'var(--color-brand-text)', marginBottom: 4, textTransform: 'uppercase', letterSpacing: '0.5px' }}>Target Subnet CIDRs</label>
                <textarea
                  value={targetCidrs}
                  onChange={(e) => setTargetCidrs(e.target.value)}
                  rows={3}
                  style={{
                    width: '100%',
                    padding: '8px 10px',
                    borderRadius: 6,
                    border: '1px solid var(--color-brand-border)',
                    background: 'var(--color-brand-bg)',
                    fontSize: 11,
                    color: 'var(--color-brand-heading)',
                    resize: 'none',
                    outline: 'none'
                  }}
                  placeholder="One subnet per line (e.g. 1.1.1.0/24)"
                />
              </div>

              {/* Ports and Concurrency */}
              <div style={{ display: 'grid', gridTemplateColumns: '1fr 1fr', gap: 10 }}>
                <div>
                  <label style={{ display: 'block', fontSize: 10, fontWeight: 700, color: 'var(--color-brand-text)', marginBottom: 4, textTransform: 'uppercase' }}>Ports</label>
                  <input
                    type="text"
                    value={selectedPorts}
                    onChange={(e) => setSelectedPorts(e.target.value)}
                    style={{ width: '100%', padding: '6px 8px', borderRadius: 6, border: '1px solid var(--color-brand-border)', background: 'var(--color-brand-bg)', color: 'var(--color-brand-heading)', fontSize: 11, outline: 'none' }}
                  />
                </div>
                <div>
                  <label style={{ display: 'block', fontSize: 10, fontWeight: 700, color: 'var(--color-brand-text)', marginBottom: 4, textTransform: 'uppercase' }}>Concurrency</label>
                  <input
                    type="number"
                    value={concurrencyLimit}
                    onChange={(e) => setConcurrencyLimit(Number(e.target.value))}
                    style={{ width: '100%', padding: '6px 8px', borderRadius: 6, border: '1px solid var(--color-brand-border)', background: 'var(--color-brand-bg)', color: 'var(--color-brand-heading)', fontSize: 11, outline: 'none' }}
                  />
                </div>
              </div>

              {/* Timeout and Max Rate */}
              <div style={{ display: 'grid', gridTemplateColumns: '1fr 1fr', gap: 10 }}>
                <div>
                  <label style={{ display: 'block', fontSize: 10, fontWeight: 700, color: 'var(--color-brand-text)', marginBottom: 4, textTransform: 'uppercase' }}>Timeout (ms)</label>
                  <input
                    type="number"
                    value={networkTimeoutMs}
                    onChange={(e) => setNetworkTimeoutMs(Number(e.target.value))}
                    style={{ width: '100%', padding: '6px 8px', borderRadius: 6, border: '1px solid var(--color-brand-border)', background: 'var(--color-brand-bg)', color: 'var(--color-brand-heading)', fontSize: 11, outline: 'none' }}
                  />
                </div>
                <div>
                  <label style={{ display: 'block', fontSize: 10, fontWeight: 700, color: 'var(--color-brand-text)', marginBottom: 4, textTransform: 'uppercase' }}>Max Rate Limit</label>
                  <input
                    type="number"
                    value={maxRateLimit}
                    onChange={(e) => setMaxRateLimit(Number(e.target.value))}
                    style={{ width: '100%', padding: '6px 8px', borderRadius: 6, border: '1px solid var(--color-brand-border)', background: 'var(--color-brand-bg)', color: 'var(--color-brand-heading)', fontSize: 11, outline: 'none' }}
                  />
                </div>
              </div>

              {/* Attempts and Top Limit */}
              <div style={{ display: 'grid', gridTemplateColumns: '1fr 1fr', gap: 10 }}>
                <div>
                  <label style={{ display: 'block', fontSize: 10, fontWeight: 700, color: 'var(--color-brand-text)', marginBottom: 4, textTransform: 'uppercase' }}>Probe Attempts</label>
                  <input
                    type="number"
                    value={probeAttempts}
                    onChange={(e) => setProbeAttempts(Number(e.target.value))}
                    style={{ width: '100%', padding: '6px 8px', borderRadius: 6, border: '1px solid var(--color-brand-border)', background: 'var(--color-brand-bg)', color: 'var(--color-brand-heading)', fontSize: 11, outline: 'none' }}
                  />
                </div>
                <div>
                  <label style={{ display: 'block', fontSize: 10, fontWeight: 700, color: 'var(--color-brand-text)', marginBottom: 4, textTransform: 'uppercase' }}>Top Save Limit</label>
                  <input
                    type="number"
                    value={topLimit}
                    onChange={(e) => setTopLimit(Number(e.target.value))}
                    style={{ width: '100%', padding: '6px 8px', borderRadius: 6, border: '1px solid var(--color-brand-border)', background: 'var(--color-brand-bg)', color: 'var(--color-brand-heading)', fontSize: 11, outline: 'none' }}
                  />
                </div>
              </div>

              {/* Total Target Count */}
              <div>
                <label style={{ display: 'block', fontSize: 10, fontWeight: 700, color: 'var(--color-brand-text)', marginBottom: 4, textTransform: 'uppercase' }}>Total Targets Cap (0 = Unlimited)</label>
                <input
                  type="number"
                  value={totalTargetCount}
                  onChange={(e) => setTotalTargetCount(Number(e.target.value))}
                  style={{ width: '100%', padding: '6px 8px', borderRadius: 6, border: '1px solid var(--color-brand-border)', background: 'var(--color-brand-bg)', color: 'var(--color-brand-heading)', fontSize: 11, outline: 'none' }}
                />
              </div>

              {/* Target Mode */}
              <div>
                <label style={{ display: 'block', fontSize: 10, fontWeight: 700, color: 'var(--color-brand-text)', marginBottom: 4, textTransform: 'uppercase' }}>Target Mode</label>
                <select
                  value={targetMode}
                  onChange={(e: any) => setTargetMode(e.target.value)}
                  style={{ width: '100%', padding: '6px 8px', borderRadius: 6, border: '1px solid var(--color-brand-border)', background: 'var(--color-brand-bg)', color: 'var(--color-brand-heading)', fontSize: 11, outline: 'none' }}
                >
                  <option value="ws">WebSocket (WS)</option>
                  <option value="tls">Direct TLS</option>
                  <option value="http">HTTP Direct</option>
                </select>
              </div>

              {/* Target SNI */}
              <div>
                <label style={{ display: 'block', fontSize: 10, fontWeight: 700, color: 'var(--color-brand-text)', marginBottom: 4, textTransform: 'uppercase' }}>Target SNI Fingerprint</label>
                <input
                  type="text"
                  value={targetSni}
                  onChange={(e) => setTargetSni(e.target.value)}
                  style={{ width: '100%', padding: '6px 8px', borderRadius: 6, border: '1px solid var(--color-brand-border)', background: 'var(--color-brand-bg)', color: 'var(--color-brand-heading)', fontSize: 11, outline: 'none' }}
                />
              </div>

              {/* Conditionally reveal WebSocket Host and Path settings */}
              {targetMode === 'ws' && (
                <div style={{ display: 'flex', flexDirection: 'column', gap: 10, borderLeft: '2px solid var(--color-brand)', paddingLeft: 10 }}>
                  <div>
                    <label style={{ display: 'block', fontSize: 10, fontWeight: 700, color: 'var(--color-brand-text)', marginBottom: 4, textTransform: 'uppercase' }}>WS Host Header</label>
                    <input
                      type="text"
                      value={websocketHost}
                      onChange={(e) => setWebsocketHost(e.target.value)}
                      style={{ width: '100%', padding: '6px 8px', borderRadius: 6, border: '1px solid var(--color-brand-border)', background: 'var(--color-brand-bg)', color: 'var(--color-brand-heading)', fontSize: 11, outline: 'none' }}
                    />
                  </div>
                  <div>
                    <label style={{ display: 'block', fontSize: 10, fontWeight: 700, color: 'var(--color-brand-text)', marginBottom: 4, textTransform: 'uppercase' }}>WS Query Path</label>
                    <input
                      type="text"
                      value={websocketPath}
                      onChange={(e) => setWebsocketPath(e.target.value)}
                      style={{ width: '100%', padding: '6px 8px', borderRadius: 6, border: '1px solid var(--color-brand-border)', background: 'var(--color-brand-bg)', color: 'var(--color-brand-heading)', fontSize: 11, outline: 'none' }}
                    />
                  </div>
                </div>
              )}

              {/* Checkboxes parameters */}
              <div style={{ display: 'flex', flexDirection: 'column', gap: 8, marginTop: 4 }}>
                <div style={{ display: 'flex', justifyContent: 'space-between', alignItems: 'center' }}>
                  <div>
                    <span style={{ fontSize: 11, fontWeight: 600, color: 'var(--color-brand-heading)' }}>Require WS Nodes</span>
                    <p style={{ margin: 0, fontSize: 9, color: 'var(--color-brand-muted)' }}>Scan WebSocket-compatible ports only.</p>
                  </div>
                  <input
                    type="checkbox"
                    checked={requireWs}
                    onChange={(e) => setRequireWs(e.target.checked)}
                    style={{ width: 14, height: 14, accentColor: 'var(--color-brand)' }}
                  />
                </div>
                <div style={{ display: 'flex', justifyContent: 'space-between', alignItems: 'center' }}>
                  <div>
                    <span style={{ fontSize: 11, fontWeight: 600, color: 'var(--color-brand-heading)' }}>Auto-Discover Neighbors</span>
                    <p style={{ margin: 0, fontSize: 9, color: 'var(--color-brand-muted)' }}>Probes adjacent subnets of healthy IPs.</p>
                  </div>
                  <input
                    type="checkbox"
                    checked={enableNeighbors}
                    onChange={(e) => setEnableNeighbors(e.target.checked)}
                    style={{ width: 14, height: 14, accentColor: 'var(--color-brand)' }}
                  />
                </div>
              </div>

              {/* Action sweeps control buttons */}
              <div style={{ display: 'flex', flexDirection: 'column', gap: 8, marginTop: 12 }}>
                {isScanning ? (
                  <button className="btn" onClick={handleStopScan} style={{ flex: 1, justifyContent: 'center', background: 'var(--color-brand-red)', borderColor: 'var(--color-brand-red)', color: '#fff' }}>
                    <FiSquare style={{ marginRight: 6 }} /> Stop Sweep
                  </button>
                ) : (
                  <>
                    <div style={{ display: 'flex', gap: 8 }}>
                      <button className="btn btn--primary" onClick={() => handleStartScan(false)} style={{ flex: 1, justifyContent: 'center' }}>
                        <FiPlay style={{ marginRight: 4 }} /> Start Sweep
                      </button>
                      <button className="btn btn--secondary" onClick={() => handleStartScan(true)} title="Reload and rerun historical settings from Pebble DB cache">
                        Retry Last
                      </button>
                    </div>
                    <div style={{ display: 'flex', gap: 8 }}>
                      <button 
                        className="btn btn--secondary" 
                        onClick={() => handleStartScan(false, true)} 
                        style={{ flex: 1, justifyContent: 'center' }}
                        title="Scan only previously saved discovered nodes to verify if they are still healthy"
                      >
                        <FiRefreshCw style={{ marginRight: 4 }} /> Rescan Healthy
                      </button>
                      <button 
                        className="btn btn--secondary" 
                        onClick={handleCleanupDiscovered} 
                        style={{ flex: 1, justifyContent: 'center', color: '#ef4444', borderColor: 'rgba(239, 68, 68, 0.2)' }}
                        title="Delete failed discovered nodes from database"
                      >
                        <FiTrash2 style={{ marginRight: 4 }} /> Cleanup Failed
                      </button>
                    </div>
                  </>
                )}
              </div>

            </div>
          </div>
        </div>

        {/* Right Column: Split panel (Top: Candidates list table, Bottom: Live logs terminal) */}
        <div style={{ display: 'flex', flexDirection: 'column', gap: 20 }}>
          
          {/* Top card: Discovered Candidates table */}
          <div className="g-card" style={{ padding: 20, display: 'flex', flexDirection: 'column', minHeight: 320, maxHeight: 420 }}>
            <div style={{ display: 'flex', justifyContent: 'space-between', alignItems: 'center', marginBottom: 16, gap: 16 }}>
              <div style={{ display: 'flex', alignItems: 'center', gap: 6 }}>
                <FiList style={{ color: 'var(--color-brand)', fontSize: 16 }} />
                <span style={{ fontSize: 14, fontWeight: 600, color: 'var(--color-brand-heading)' }}>
                  Discovered Proxy Candidates ({filteredCandidates.length})
                </span>
              </div>
              
              <div style={{ display: 'flex', alignItems: 'center', gap: 10 }}>
                <div style={{ position: 'relative', width: 180 }}>
                  <FiSearch style={{ position: 'absolute', left: 8, top: 8, color: 'var(--color-brand-muted)', fontSize: 12 }} />
                  <input
                    type="text"
                    value={searchQuery}
                    onChange={(e) => setSearchQuery(e.target.value)}
                    placeholder="Search candidates..."
                    style={{
                      width: '100%',
                      padding: '5px 10px 5px 26px',
                      borderRadius: 6,
                      border: '1px solid var(--color-brand-border)',
                      background: 'var(--color-brand-bg)',
                      color: 'var(--color-brand-heading)',
                      fontSize: 11,
                      outline: 'none'
                    }}
                  />
                </div>

                <select
                  value={sortBy}
                  onChange={(e: any) => setSortBy(e.target.value)}
                  style={{
                    padding: '5px 8px',
                    borderRadius: 6,
                    border: '1px solid var(--color-brand-border)',
                    background: 'var(--color-brand-bg)',
                    color: 'var(--color-brand-heading)',
                    fontSize: 11,
                    outline: 'none'
                  }}
                >
                  <option value="latency">Latency</option>
                  <option value="speed">Speed</option>
                  <option value="time">Time Added</option>
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
                      setMessage({ type: 'success', text: 'Copied all verified IP:port endpoints to clipboard.' });
                    } else {
                      setMessage({ type: 'info', text: 'No verified candidates available to copy.' });
                    }
                  }}
                  title="Copy verified hosts (shortcut: C)"
                >
                  <FiClipboard /> Copy All
                </button>

                <button 
                  className="btn btn--sm btn--secondary" 
                  onClick={downloadTxt}
                  title="Export healthy results to TXT file"
                >
                  <FiFileText /> TXT
                </button>

                <button 
                  className="btn btn--sm btn--secondary" 
                  onClick={downloadCsv}
                  title="Export healthy results to CSV file"
                >
                  <FiDownload /> CSV
                </button>
              </div>
            </div>

            {/* Candidates Table List */}
            <div style={{ flex: 1, overflowY: 'auto' }}>
              <table style={{ width: '100%', borderCollapse: 'collapse', fontSize: 12, textAlign: 'left' }}>
                <thead>
                  <tr style={{ borderBottom: '1px solid var(--color-brand-border)' }}>
                    <th style={{ padding: '8px 6px', color: 'var(--color-brand-muted)', fontWeight: 600 }}>Endpoint IP</th>
                    <th style={{ padding: '8px 6px', color: 'var(--color-brand-muted)', fontWeight: 600 }}>Port</th>
                    <th style={{ padding: '8px 6px', color: 'var(--color-brand-muted)', fontWeight: 600 }}>Mode</th>
                    <th style={{ padding: '8px 6px', color: 'var(--color-brand-muted)', fontWeight: 600 }}>Latency</th>
                    <th style={{ padding: '8px 6px', color: 'var(--color-brand-muted)', fontWeight: 600 }}>Speed</th>
                    <th style={{ padding: '8px 6px', color: 'var(--color-brand-muted)', fontWeight: 600 }}>Status</th>
                    <th style={{ padding: '8px 6px', color: 'var(--color-brand-muted)', fontWeight: 600, textAlign: 'right' }}>Actions</th>
                  </tr>
                </thead>
                <tbody>
                  {filteredCandidates.length === 0 ? (
                    <tr>
                      <td colSpan={7} style={{ padding: 30, textAlign: 'center', color: 'var(--color-brand-muted)' }}>
                        <FiFileText size={20} style={{ marginBottom: 8, opacity: 0.3 }} />
                        <div>No candidates found.</div>
                      </td>
                    </tr>
                  ) : (
                    filteredCandidates.map((c, idx) => (
                      <tr key={idx} style={{ borderBottom: '1px solid var(--color-brand-border)', verticalAlign: 'middle' }}>
                        <td style={{ padding: '8px 6px', fontWeight: 600, color: 'var(--color-brand-heading)' }}>{c.ip}</td>
                        <td style={{ padding: '8px 6px', color: 'var(--color-brand-heading)' }}>{c.port}</td>
                        <td style={{ padding: '8px 6px', color: 'var(--color-brand-muted)' }}>
                          <span style={{ fontSize: 9, padding: '2px 5px', background: 'var(--color-brand-light)', border: '1px solid var(--color-brand-border)', borderRadius: 4, textTransform: 'uppercase' }}>
                            {c.protocol}
                          </span>
                        </td>
                        <td style={{ padding: '8px 6px', fontWeight: 600, color: c.latencyMs > 0 ? 'var(--color-brand-green)' : 'var(--color-brand-red)' }}>
                          {c.latencyMs > 0 ? `${c.latencyMs} ms` : '-'}
                        </td>
                        <td style={{ padding: '8px 6px', color: 'var(--color-brand-blue)', fontWeight: 600 }}>
                          {c.speedMbps > 0 ? `${c.speedMbps.toFixed(2)} Mbps` : '-'}
                        </td>
                        <td style={{ padding: '8px 6px' }}>
                          <span style={{
                            fontSize: 9,
                            fontWeight: 600,
                            padding: '3px 8px',
                            borderRadius: 12,
                            background: c.status === 'healthy' ? 'rgba(34,197,94,0.1)' : c.status === 'failed' ? 'rgba(239,68,68,0.1)' : 'rgba(59,130,246,0.1)',
                            color: c.status === 'healthy' ? 'var(--color-brand-green)' : c.status === 'failed' ? 'var(--color-brand-red)' : 'var(--color-brand-blue)',
                          }}>
                            {c.status.toUpperCase()}
                          </span>
                        </td>
                        <td style={{ padding: '8px 6px', textAlign: 'right' }}>
                          <button
                            className="btn btn--xs btn--secondary"
                            onClick={() => {
                              navigator.clipboard.writeText(`${c.ip}:${c.port}`);
                              setMessage({ type: 'success', text: `Copied ${c.ip}:${c.port} to clipboard.` });
                            }}
                            style={{ padding: '3px 6px', fontSize: 10 }}
                          >
                            Copy IP
                          </button>
                        </td>
                      </tr>
                    ))
                  )}
                </tbody>
              </table>
            </div>
          </div>

          {/* Bottom card: Live Monospace Scanner Logs Terminal */}
          <div className="g-card" style={{ padding: 20, display: 'flex', flexDirection: 'column', height: 280 }}>
            <div style={{ display: 'flex', alignItems: 'center', justifyContent: 'space-between', marginBottom: 12 }}>
              <div style={{ display: 'flex', alignItems: 'center', gap: 6 }}>
                <FiTerminal style={{ color: 'var(--color-brand)', fontSize: 16 }} />
                <span style={{ fontSize: 13, fontWeight: 600, color: 'var(--color-brand-heading)' }}>
                  Live Diagnostic Scanner Logs
                </span>
              </div>
              <div style={{ display: 'flex', alignItems: 'center', gap: 8 }}>
                <input
                  type="text"
                  placeholder="Filter logs..."
                  value={logsFilter}
                  onChange={(e) => setLogsFilter(e.target.value)}
                  style={{
                    width: 140,
                    padding: '4px 8px',
                    borderRadius: 6,
                    border: '1px solid var(--color-brand-border)',
                    background: 'var(--color-brand-bg)',
                    fontSize: 11,
                    color: 'var(--color-brand-heading)',
                    outline: 'none'
                  }}
                />
                <button 
                  className="btn btn--sm btn--secondary" 
                  onClick={() => setScannerLogs([])}
                  title="Clear scanner logs"
                  style={{ display: 'flex', alignItems: 'center', gap: 4 }}
                >
                  <FiTrash2 size={12} /> Clear
                </button>
              </div>
            </div>

            {/* Terminal logs content */}
            <div
              ref={logsContainerRef}
              style={{
                flex: 1,
                background: 'var(--color-brand-bg)',
                border: '1px solid var(--color-brand-border)',
                borderRadius: 8,
                padding: 12,
                fontFamily: 'Fira Code, Courier, monospace',
                fontSize: 11,
                color: 'var(--color-brand-text)',
                overflowY: 'auto',
                display: 'flex',
                flexDirection: 'column',
                gap: 4
              }}
            >
              {filteredLogs.length === 0 ? (
                <div style={{ color: 'var(--color-brand-muted)', textAlign: 'center', marginTop: 70 }}>
                  No diagnostic scanner logs available. Click "Start Sweep" to stream.
                </div>
              ) : (
                filteredLogs.map((log, idx) => {
                  let color = 'var(--color-brand-text)';
                  if (log.includes('[ERROR]') || log.includes('Critical:') || log.includes('Failed candidate:')) {
                    color = 'var(--color-brand-red)';
                  } else if (log.includes('Healthy candidate:') || log.includes('Success')) {
                    color = 'var(--color-brand-green)';
                  } else if (log.includes('Initiating') || log.includes('Parameters:')) {
                    color = 'var(--color-brand)';
                  }
                  
                  return (
                    <div key={idx} style={{ wordBreak: 'break-all', whiteSpace: 'pre-wrap', color }}>
                      {log}
                    </div>
                  );
                })
              )}
            </div>
          </div>

        </div>

      </div>

    </div>
  );
};
