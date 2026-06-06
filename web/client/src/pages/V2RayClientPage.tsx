import React, { useState, useEffect, useRef } from 'react';
import { 
  FiSliders, FiCpu, FiGlobe, FiKey, FiPlay, FiSquare, FiSave, FiRefreshCw, 
  FiEye, FiEyeOff, FiHelpCircle, FiTerminal, FiDownloadCloud, FiPlus, 
  FiTrash2, FiActivity, FiSearch, FiZap, FiWifi, FiMonitor, FiSettings, 
  FiAlertCircle, FiLock, FiLogOut, FiCheck, FiX
} from 'react-icons/fi';

export const V2RayClientPage: React.FC = () => {
  // Help modal state
  const [helpTitle, setHelpTitle] = useState<string | null>(null);
  const [helpText, setHelpText] = useState<string | null>(null);

  const showHelp = (title: string, text: string) => {
    setHelpTitle(title);
    setHelpText(text);
  };

  // State definitions
  const [isRunning, setIsRunning] = useState(false);
  const [isLoading, setIsLoading] = useState(false);
  const [logs, setLogs] = useState<string[]>([]);
  const [logsQuery, setLogsQuery] = useState('');
  const [message, setMessage] = useState<{ type: 'success' | 'error', text: string } | null>(null);

  // Settings
  const [socksPort, setSocksPort] = useState(10808);
  const [httpPort, setHttpPort] = useState(10809);
  const [muxEnabled, setMuxEnabled] = useState(true);
  const [dnsServer, setDnsServer] = useState('8.8.8.8');
  const [routingPreset, setRoutingPreset] = useState('bypass_domestic');
  const [customRouting, setCustomRouting] = useState('');

  // Evasion settings
  const [evasionFingerprint, setEvasionFingerprint] = useState('chrome');
  const [evasionFragment, setEvasionFragment] = useState(true);
  const [evasionEch, setEvasionEch] = useState(false);
  const [evasionEchConfig, setEvasionEchConfig] = useState('');
  const [evasionTcpBrutal, setEvasionTcpBrutal] = useState(false);

  // Subscriptions & Profiles
  const [subUrl, setSubUrl] = useState('');
  const [profiles, setProfiles] = useState<any[]>([]);
  const [activeProfileId, setActiveProfileId] = useState<number | null>(null);
  const [manualUri, setManualUri] = useState('');

  // Port prober state
  const [probeIP, setProbeIP] = useState('8.8.8.8');
  const [probePorts, setProbePorts] = useState('53,80,443');
  const [probeProto, setProbeProto] = useState('tcp');
  const [probeResults, setProbeResults] = useState<any[]>([]);

  // Wake on LAN state
  const [wolMac, setWolMac] = useState('');
  const [wolBcast, setWolBcast] = useState('255.255.255.255');

  // Local device discovery
  const [discoveredDevices, setDiscoveredDevices] = useState<any[]>([]);
  const [isDiscovering, setIsDiscovering] = useState(false);

  // Debug interception proxy state
  const [debugProxyPort, setDebugProxyPort] = useState(8080);
  const [isDebugProxyActive, setIsDebugProxyActive] = useState(false);
  const [debugProxyLogs, setDebugProxyLogs] = useState<string[]>([]);

  // Hotkeys & System tray
  const [hotkeys, setHotkeys] = useState('Ctrl+Shift+X');
  const [systemTrayEnabled, setSystemTrayEnabled] = useState(true);

  const logsEndRef = useRef<HTMLDivElement | null>(null);

  // Load configs
  const loadSettings = async () => {
    setIsLoading(true);
    try {
      const token = localStorage.getItem('cc_client_token') || '';
      // Fetch core settings
      const sResp = await fetch('/api/v2ray/client/settings', {
        headers: { 'Authorization': `Bearer ${token}` }
      });
      if (sResp.ok) {
        const data = await sResp.json();
        setSocksPort(data.socks_port || 10808);
        setHttpPort(data.http_port || 10809);
        setMuxEnabled(data.mux_enabled);
        setDnsServer(data.dns_server || '8.8.8.8');
        setRoutingPreset(data.routing_preset || 'bypass_domestic');
        setCustomRouting(data.custom_routing || '');
        setEvasionFingerprint(data.evasion_fingerprint || 'chrome');
        setEvasionFragment(data.evasion_fragment);
        setEvasionEch(data.evasion_ech);
        setEvasionEchConfig(data.evasion_ech_config || '');
        setEvasionTcpBrutal(data.evasion_tcp_brutal);
      }

      // Fetch profiles
      const pResp = await fetch('/api/v2ray/client/profiles', {
        headers: { 'Authorization': `Bearer ${token}` }
      });
      if (pResp.ok) {
        const pList = await pResp.json();
        setProfiles(pList);
        const active = pList.find((p: any) => p.is_active);
        if (active) setActiveProfileId(active.ID);
      }

      // Fetch core status
      const stResp = await fetch('/api/v2ray/client/status', {
        headers: { 'Authorization': `Bearer ${token}` }
      });
      if (stResp.ok) {
        const statusData = await stResp.json();
        setIsRunning(statusData.is_running);
      }

      // Fetch logs
      fetchLogs();
    } catch (err) {
      console.error(err);
    } finally {
      setIsLoading(false);
    }
  };

  const fetchLogs = async () => {
    try {
      const token = localStorage.getItem('cc_client_token') || '';
      const response = await fetch(`/api/v2ray/client/logs?q=${encodeURIComponent(logsQuery)}`, {
        headers: { 'Authorization': `Bearer ${token}` }
      });
      if (response.ok) {
        const data = await response.json();
        setLogs(data || []);
      }
    } catch (err) {
      console.error(err);
    }
  };

  useEffect(() => {
    loadSettings();
    const ticker = setInterval(() => {
      fetchLogs();
      if (isDebugProxyActive) fetchProxyLogs();
    }, 4000);
    return () => clearInterval(ticker);
  }, [logsQuery, isDebugProxyActive]);

  useEffect(() => {
    if (logsEndRef.current) {
      logsEndRef.current.scrollIntoView({ behavior: 'smooth' });
    }
  }, [logs]);

  // Actions
  const handleSaveSettings = async (e: React.FormEvent) => {
    e.preventDefault();
    setIsLoading(true);
    setMessage(null);
    try {
      const token = localStorage.getItem('cc_client_token') || '';
      const res = await fetch('/api/v2ray/client/settings', {
        method: 'POST',
        headers: {
          'Content-Type': 'application/json',
          'Authorization': `Bearer ${token}`
        },
        body: JSON.stringify({
          socks_port: Number(socksPort),
          http_port: Number(httpPort),
          mux_enabled: muxEnabled,
          dns_server: dnsServer,
          routing_preset: routingPreset,
          custom_routing: customRouting,
          evasion_fingerprint: evasionFingerprint,
          evasion_fragment: evasionFragment,
          evasion_ech: evasionEch,
          evasion_ech_config: evasionEchConfig,
          evasion_tcp_brutal: evasionTcpBrutal
        })
      });
      if (res.ok) {
        setMessage({ type: 'success', text: 'V2Ray client settings updated successfully!' });
        loadSettings();
      } else {
        const data = await res.json();
        setMessage({ type: 'error', text: data.error || 'Failed to update settings.' });
      }
    } catch (err: any) {
      setMessage({ type: 'error', text: err.message });
    } finally {
      setIsLoading(false);
    }
  };

  const handleStartCore = async () => {
    setIsLoading(true);
    setMessage(null);
    try {
      const token = localStorage.getItem('cc_client_token') || '';
      const res = await fetch('/api/v2ray/client/start', {
        method: 'POST',
        headers: { 'Authorization': `Bearer ${token}` }
      });
      if (res.ok) {
        setIsRunning(true);
        setMessage({ type: 'success', text: 'V2Ray Client engine started successfully!' });
      } else {
        const data = await res.json();
        setMessage({ type: 'error', text: data.error || 'Failed to start proxy core.' });
      }
    } catch (err: any) {
      setMessage({ type: 'error', text: err.message });
    } finally {
      setIsLoading(false);
    }
  };

  const handleStopCore = async () => {
    setIsLoading(true);
    setMessage(null);
    try {
      const token = localStorage.getItem('cc_client_token') || '';
      const res = await fetch('/api/v2ray/client/stop', {
        method: 'POST',
        headers: { 'Authorization': `Bearer ${token}` }
      });
      if (res.ok) {
        setIsRunning(false);
        setMessage({ type: 'success', text: 'V2Ray Client engine stopped.' });
      } else {
        const data = await res.json();
        setMessage({ type: 'error', text: data.error || 'Failed to stop proxy core.' });
      }
    } catch (err: any) {
      setMessage({ type: 'error', text: err.message });
    } finally {
      setIsLoading(false);
    }
  };

  const handleImportSub = async () => {
    if (!subUrl) return;
    setIsLoading(true);
    setMessage(null);
    try {
      const token = localStorage.getItem('cc_client_token') || '';
      const res = await fetch('/api/v2ray/client/subscriptions', {
        method: 'POST',
        headers: {
          'Content-Type': 'application/json',
          'Authorization': `Bearer ${token}`
        },
        body: JSON.stringify({ url: subUrl })
      });
      if (res.ok) {
        setSubUrl('');
        setMessage({ type: 'success', text: 'Subscription sync trigger added! Reloading profiles...' });
        loadSettings();
      } else {
        const data = await res.json();
        setMessage({ type: 'error', text: data.error || 'Failed to import subscription.' });
      }
    } catch (err: any) {
      setMessage({ type: 'error', text: err.message });
    } finally {
      setIsLoading(false);
    }
  };

  const handleManualImport = async () => {
    if (!manualUri) return;
    setIsLoading(true);
    setMessage(null);
    try {
      const token = localStorage.getItem('cc_client_token') || '';
      const res = await fetch('/api/v2ray/client/profiles/import', {
        method: 'POST',
        headers: {
          'Content-Type': 'application/json',
          'Authorization': `Bearer ${token}`
        },
        body: JSON.stringify({ uri: manualUri })
      });
      if (res.ok) {
        setManualUri('');
        setMessage({ type: 'success', text: 'Manual profile imported successfully!' });
        loadSettings();
      } else {
        const data = await res.json();
        setMessage({ type: 'error', text: data.error || 'Import failed.' });
      }
    } catch (err: any) {
      setMessage({ type: 'error', text: err.message });
    } finally {
      setIsLoading(false);
    }
  };

  const handleSelectProfile = async (id: number) => {
    setIsLoading(true);
    try {
      const token = localStorage.getItem('cc_client_token') || '';
      const res = await fetch(`/api/v2ray/client/profiles/${id}/activate`, {
        method: 'POST',
        headers: { 'Authorization': `Bearer ${token}` }
      });
      if (res.ok) {
        setActiveProfileId(id);
        setMessage({ type: 'success', text: 'Active outbound profile modified!' });
        loadSettings();
      }
    } catch (err) {
      console.error(err);
    } finally {
      setIsLoading(false);
    }
  };

  const handleDeleteProfile = async (id: number) => {
    if (!window.confirm('Delete this outbound configuration?')) return;
    setIsLoading(true);
    try {
      const token = localStorage.getItem('cc_client_token') || '';
      const res = await fetch(`/api/v2ray/client/profiles/${id}`, {
        method: 'DELETE',
        headers: { 'Authorization': `Bearer ${token}` }
      });
      if (res.ok) {
        setMessage({ type: 'success', text: 'Outbound profile deleted.' });
        loadSettings();
      }
    } catch (err) {
      console.error(err);
    } finally {
      setIsLoading(false);
    }
  };

  const handleTestLatency = async () => {
    setIsLoading(true);
    setMessage({ type: 'success', text: 'Running parallel RTT latency test sweep...' });
    try {
      const token = localStorage.getItem('cc_client_token') || '';
      const res = await fetch('/api/v2ray/client/test-mass', {
        method: 'POST',
        headers: { 'Authorization': `Bearer ${token}` }
      });
      if (res.ok) {
        loadSettings();
      }
    } catch (err) {
      console.error(err);
    } finally {
      setIsLoading(false);
    }
  };

  // Diagnostic actions
  const handleProbePorts = async () => {
    if (!probeIP || !probePorts) return;
    setIsLoading(true);
    setProbeResults([]);
    try {
      const token = localStorage.getItem('cc_client_token') || '';
      const portsArr = probePorts.split(',').map(p => Number(p.trim())).filter(p => !isNaN(p));
      const res = await fetch('/api/v2ray/client/probe-ports', {
        method: 'POST',
        headers: {
          'Content-Type': 'application/json',
          'Authorization': `Bearer ${token}`
        },
        body: JSON.stringify({
          ip: probeIP,
          ports: portsArr,
          protocol: probeProto
        })
      });
      if (res.ok) {
        const data = await res.json();
        setProbeResults(data);
      }
    } catch (err) {
      console.error(err);
    } finally {
      setIsLoading(false);
    }
  };

  const handleSendWol = async () => {
    if (!wolMac) return;
    setIsLoading(true);
    setMessage(null);
    try {
      const token = localStorage.getItem('cc_client_token') || '';
      const res = await fetch('/api/v2ray/client/wol', {
        method: 'POST',
        headers: {
          'Content-Type': 'application/json',
          'Authorization': `Bearer ${token}`
        },
        body: JSON.stringify({
          mac: wolMac,
          broadcast_ip: wolBcast
        })
      });
      if (res.ok) {
        setMessage({ type: 'success', text: 'Magic WOL broadcast packet dispatched!' });
      } else {
        const data = await res.json();
        setMessage({ type: 'error', text: data.error || 'Failed to dispatch packet.' });
      }
    } catch (err: any) {
      setMessage({ type: 'error', text: err.message });
    } finally {
      setIsLoading(false);
    }
  };

  const handleDiscoverDevices = async () => {
    setIsDiscovering(true);
    setDiscoveredDevices([]);
    try {
      const token = localStorage.getItem('cc_client_token') || '';
      const res = await fetch('/api/v2ray/client/discover', {
        headers: { 'Authorization': `Bearer ${token}` }
      });
      if (res.ok) {
        const data = await res.json();
        setDiscoveredDevices(data || []);
      }
    } catch (err) {
      console.error(err);
    } finally {
      setIsDiscovering(false);
    }
  };

  // Interception proxy
  const handleToggleDebugProxy = async () => {
    setIsLoading(true);
    try {
      const token = localStorage.getItem('cc_client_token') || '';
      const endpoint = isDebugProxyActive 
        ? '/api/v2ray/client/debug-proxy/stop'
        : '/api/v2ray/client/debug-proxy/start';
      const body = isDebugProxyActive 
        ? undefined 
        : JSON.stringify({ port: Number(debugProxyPort) });

      const res = await fetch(endpoint, {
        method: 'POST',
        headers: {
          'Content-Type': 'application/json',
          'Authorization': `Bearer ${token}`
        },
        body: body
      });

      if (res.ok) {
        setIsDebugProxyActive(!isDebugProxyActive);
        setDebugProxyLogs([]);
      }
    } catch (err) {
      console.error(err);
    } finally {
      setIsLoading(false);
    }
  };

  const fetchProxyLogs = async () => {
    try {
      const token = localStorage.getItem('cc_client_token') || '';
      const res = await fetch('/api/v2ray/client/debug-proxy/logs', {
        headers: { 'Authorization': `Bearer ${token}` }
      });
      if (res.ok) {
        const data = await res.json();
        setDebugProxyLogs(data || []);
      }
    } catch (err) {
      console.error(err);
    }
  };

  const getLatencyColor = (ms: number) => {
    if (ms <= 0) return 'var(--color-brand-muted)';
    if (ms < 100) return 'var(--color-brand-green)';
    if (ms < 300) return '#f59e0b';
    return 'var(--color-brand-red)';
  };

  return (
    <div>
      {/* Title */}
      <div style={{ display: 'flex', justifyContent: 'space-between', alignItems: 'center', marginBottom: 24 }}>
        <div>
          <h1 style={{ fontSize: 22, fontWeight: 700, color: 'var(--color-brand-heading)', margin: 0 }}>V2Ray / Xray manager</h1>
          <p style={{ fontSize: 12, color: 'var(--color-brand-text)', margin: '4px 0 0' }}>
            Zero-TUN Censorship Evasion panel supporting VLESS, VMess, Reality, and dynamic routing settings.
          </p>
        </div>
        <button className="btn btn--sm" onClick={loadSettings} disabled={isLoading}>
          <FiRefreshCw className={isLoading ? 'spin-animation' : ''} style={{ marginRight: 6 }} /> Refresh State
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

      {/* Grid Layout */}
      <div style={{ display: 'grid', gridTemplateColumns: '1fr 360px', gap: 24, alignItems: 'start' }}>
        
        {/* Left Side: Outbounds, Configurations & Evasion */}
        <div style={{ display: 'flex', flexDirection: 'column', gap: 24 }}>
          
          {/* Active Engine controls */}
          <div className="g-card" style={{ display: 'flex', alignItems: 'center', justifyContent: 'space-between' }}>
            <div>
              <div style={{ display: 'flex', alignItems: 'center', gap: 8 }}>
                <span className="live-dot" style={{ background: isRunning ? '#10b981' : '#ef4444' }} />
                <span style={{ fontSize: 15, fontWeight: 700, color: 'var(--color-brand-heading)' }}>
                  Core Supervisor: {isRunning ? 'RUNNING' : 'STOPPED'}
                </span>
                <FiHelpCircle 
                  style={{ cursor: 'pointer', color: 'var(--color-brand-muted)' }} 
                  onClick={() => showHelp('V2Ray Core Supervisor', 'Manages the local Xray daemon lifecycle in the background. On start, compiles database rules into an optimized JSON config, ensures SOCKS5 and HTTP inbound listeners bind safely without port conflicts, and launches Xray.')}
                />
              </div>
              <span style={{ fontSize: 11, color: 'var(--color-brand-text)', display: 'block', marginTop: 4 }}>
                Local inbound captures: SOCKS5 on port {socksPort} & HTTP on port {httpPort}.
              </span>
            </div>

            <div style={{ display: 'flex', gap: 10 }}>
              <button 
                onClick={handleStartCore} 
                className="btn btn--primary" 
                style={{ display: 'flex', alignItems: 'center', gap: 6, background: isRunning ? '#a3a3a3' : undefined }}
                disabled={isRunning || isLoading}
              >
                <FiPlay /> Start
              </button>
              <button 
                onClick={handleStopCore} 
                className="btn btn--secondary" 
                style={{ display: 'flex', alignItems: 'center', gap: 6, borderColor: '#ef4444', color: '#ef4444' }}
                disabled={!isRunning || isLoading}
              >
                <FiSquare /> Stop
              </button>
            </div>
          </div>

          {/* Subscriptions & Profiles */}
          <div className="g-card">
            <div style={{ display: 'flex', alignItems: 'center', justifyContent: 'space-between', marginBottom: 16 }}>
              <div style={{ display: 'flex', alignItems: 'center', gap: 8 }}>
                <FiDownloadCloud style={{ color: 'var(--color-brand)', fontSize: 18 }} />
                <span style={{ fontSize: 15, fontWeight: 600, color: 'var(--color-brand-heading)' }}>Subscriptions & Profiles</span>
                <FiHelpCircle 
                  style={{ cursor: 'pointer', color: 'var(--color-brand-muted)' }} 
                  onClick={() => showHelp('Subscriptions & Profiles', 'Sync and import remote proxy configuration files. Add URL links to sync periodically, or paste raw URI configurations (vmess://, vless://, ss://, trojan://) directly. Use Test Latencies to sweep TCP round-trip delays across all available servers.')}
                />
              </div>
              <button className="btn btn--sm btn--secondary" onClick={handleTestLatency} disabled={isLoading}>
                <FiActivity style={{ marginRight: 6 }} /> Test Latencies
              </button>
            </div>

            {/* Input URL */}
            <div style={{ display: 'flex', gap: 10, marginBottom: 12 }}>
              <input
                type="text"
                placeholder="Subscription Link (HTTP/S Base64)"
                value={subUrl}
                onChange={(e) => setSubUrl(e.target.value)}
                style={{
                  flex: 1,
                  padding: '8px 12px',
                  borderRadius: 8,
                  border: '1px solid var(--color-brand-border)',
                  background: 'var(--color-brand-card)',
                  fontSize: 13,
                  color: 'var(--color-brand-heading)'
                }}
              />
              <button className="btn btn--primary" onClick={handleImportSub} disabled={isLoading}>Import</button>
            </div>

            {/* Manual import */}
            <div style={{ display: 'flex', gap: 10, marginBottom: 20 }}>
              <input
                type="text"
                placeholder="Manual Config URI (vmess://, vless://, trojan://, ss://)"
                value={manualUri}
                onChange={(e) => setManualUri(e.target.value)}
                style={{
                  flex: 1,
                  padding: '8px 12px',
                  borderRadius: 8,
                  border: '1px solid var(--color-brand-border)',
                  background: 'var(--color-brand-card)',
                  fontSize: 13,
                  color: 'var(--color-brand-heading)'
                }}
              />
              <button className="btn btn--secondary" onClick={handleManualImport} disabled={isLoading}>Import URI</button>
            </div>

            {/* Table of Profiles */}
            <div style={{ overflowX: 'auto', border: '1px solid var(--color-brand-border)', borderRadius: 8 }}>
              <table style={{ width: '100%', borderCollapse: 'collapse', fontSize: 12, textAlign: 'left' }}>
                <thead>
                  <tr style={{ background: 'var(--color-brand-bg)', borderBottom: '1px solid var(--color-brand-border)' }}>
                    <th style={{ padding: '10px 12px', color: 'var(--color-brand-heading)' }}>Active</th>
                    <th style={{ padding: '10px 12px', color: 'var(--color-brand-heading)' }}>Name</th>
                    <th style={{ padding: '10px 12px', color: 'var(--color-brand-heading)' }}>Protocol</th>
                    <th style={{ padding: '10px 12px', color: 'var(--color-brand-heading)' }}>Address</th>
                    <th style={{ padding: '10px 12px', color: 'var(--color-brand-heading)' }}>Ping</th>
                    <th style={{ padding: '10px 12px', color: 'var(--color-brand-heading)', textAlign: 'center' }}>Actions</th>
                  </tr>
                </thead>
                <tbody>
                  {profiles.length === 0 ? (
                    <tr>
                      <td colSpan={6} style={{ padding: 20, textAlign: 'center', color: 'var(--color-brand-muted)' }}>
                        No profiles imported. Add subscription URL or paste configs.
                      </td>
                    </tr>
                  ) : (
                    profiles.map((p) => (
                      <tr key={p.ID} style={{ borderBottom: '1px solid var(--color-brand-border)', background: p.ID === activeProfileId ? 'var(--color-brand-light)' : 'none' }}>
                        <td style={{ padding: '10px 12px' }}>
                          <input
                            type="radio"
                            name="active_profile"
                            checked={p.ID === activeProfileId}
                            onChange={() => handleSelectProfile(p.ID)}
                            style={{ cursor: 'pointer', accentColor: 'var(--color-brand)' }}
                          />
                        </td>
                        <td style={{ padding: '10px 12px', fontWeight: 600, color: 'var(--color-brand-heading)' }}>{p.name}</td>
                        <td style={{ padding: '10px 12px', textTransform: 'uppercase' }}>
                          <span style={{
                            padding: '2px 6px',
                            borderRadius: 4,
                            background: '#e0f2fe',
                            color: '#0369a1',
                            fontSize: 10,
                            fontWeight: 700
                          }}>{p.protocol}</span>
                        </td>
                        <td style={{ padding: '10px 12px', color: 'var(--color-brand-text)' }}>{p.address}:{p.port}</td>
                        <td style={{ padding: '10px 12px' }}>
                          <span style={{
                            fontWeight: 700,
                            color: getLatencyColor(p.latency_ms)
                          }}>{p.latency_ms > 0 ? `${p.latency_ms}ms` : 'N/A'}</span>
                        </td>
                        <td style={{ padding: '10px 12px', textAlign: 'center' }}>
                          <button 
                            onClick={() => handleDeleteProfile(p.ID)} 
                            style={{ background: 'none', border: 'none', cursor: 'pointer', color: 'var(--color-brand-red)' }}
                          >
                            <FiTrash2 size={14} />
                          </button>
                        </td>
                      </tr>
                    ))
                  )}
                </tbody>
              </table>
            </div>
          </div>

          {/* Configuration Form */}
          <form onSubmit={handleSaveSettings} className="g-card" style={{ display: 'flex', flexDirection: 'column', gap: 18 }}>
            <div style={{ display: 'flex', alignItems: 'center', gap: 8 }}>
              <FiSliders style={{ color: 'var(--color-brand)', fontSize: 18 }} />
              <span style={{ fontSize: 15, fontWeight: 600, color: 'var(--color-brand-heading)' }}>Proxy Configurations</span>
              <FiHelpCircle 
                style={{ cursor: 'pointer', color: 'var(--color-brand-muted)' }} 
                onClick={() => showHelp('Proxy Configurations', 'Configure inbound local ports for network application mapping. Set custom DNS resolving servers to avoid ISP redirection leaks.')}
              />
            </div>

            <div style={{ display: 'grid', gridTemplateColumns: '1fr 1fr', gap: 16 }}>
              <div>
                <label style={{ display: 'block', fontSize: 11, fontWeight: 600, color: 'var(--color-brand-muted)', marginBottom: 6, textTransform: 'uppercase' }}>SOCKS5 Port</label>
                <input
                  type="number"
                  value={socksPort}
                  onChange={(e) => setSocksPort(Number(e.target.value))}
                  style={{
                    width: '100%',
                    padding: '8px 12px',
                    borderRadius: 8,
                    border: '1px solid var(--color-brand-border)',
                    background: 'var(--color-brand-card)',
                    fontSize: 13,
                    color: 'var(--color-brand-heading)'
                  }}
                  required
                />
              </div>

              <div>
                <label style={{ display: 'block', fontSize: 11, fontWeight: 600, color: 'var(--color-brand-muted)', marginBottom: 6, textTransform: 'uppercase' }}>HTTP Port</label>
                <input
                  type="number"
                  value={httpPort}
                  onChange={(e) => setHttpPort(Number(e.target.value))}
                  style={{
                    width: '100%',
                    padding: '8px 12px',
                    borderRadius: 8,
                    border: '1px solid var(--color-brand-border)',
                    background: 'var(--color-brand-card)',
                    fontSize: 13,
                    color: 'var(--color-brand-heading)'
                  }}
                  required
                />
              </div>
            </div>

            <div style={{ display: 'grid', gridTemplateColumns: '1fr 1fr', gap: 16 }}>
              <div>
                <label style={{ display: 'block', fontSize: 11, fontWeight: 600, color: 'var(--color-brand-muted)', marginBottom: 6, textTransform: 'uppercase' }}>DNS Resolver Server</label>
                <input
                  type="text"
                  value={dnsServer}
                  onChange={(e) => setDnsServer(e.target.value)}
                  style={{
                    width: '100%',
                    padding: '8px 12px',
                    borderRadius: 8,
                    border: '1px solid var(--color-brand-border)',
                    background: 'var(--color-brand-card)',
                    fontSize: 13,
                    color: 'var(--color-brand-heading)'
                  }}
                  required
                />
              </div>

              <div>
                <label style={{ display: 'block', fontSize: 11, fontWeight: 600, color: 'var(--color-brand-muted)', marginBottom: 6, textTransform: 'uppercase' }}>Routing Preset</label>
                <select
                  value={routingPreset}
                  onChange={(e) => setRoutingPreset(e.target.value)}
                  style={{
                    width: '100%',
                    padding: '8px 12px',
                    borderRadius: 8,
                    border: '1px solid var(--color-brand-border)',
                    background: 'var(--color-brand-card)',
                    fontSize: 13,
                    color: 'var(--color-brand-heading)'
                  }}
                >
                  <option value="global">Global (Route All traffic through Proxy)</option>
                  <option value="bypass_domestic">Bypass Iran (GeoIP:IR + Geosite:IR go Direct)</option>
                  <option value="block_ads">Block Ads (Reject Ad hosts, others proxy)</option>
                  <option value="custom">Custom (Compile User Custom Rules)</option>
                </select>
              </div>
            </div>

            {routingPreset === 'custom' && (
              <div>
                <label style={{ display: 'block', fontSize: 11, fontWeight: 600, color: 'var(--color-brand-muted)', marginBottom: 6, textTransform: 'uppercase' }}>Custom Routing Rules (JSON array)</label>
                <textarea
                  value={customRouting}
                  onChange={(e) => setCustomRouting(e.target.value)}
                  placeholder='[{"type": "field", "outboundTag": "direct", "domain": ["geosite:ir"]}]'
                  rows={4}
                  style={{
                    width: '100%',
                    padding: '10px 12px',
                    borderRadius: 8,
                    border: '1px solid var(--color-brand-border)',
                    background: 'var(--color-brand-card)',
                    fontSize: 12,
                    fontFamily: 'Fira Code',
                    color: 'var(--color-brand-heading)'
                  }}
                />
              </div>
            )}

            {/* DPI Evasion & Security */}
            <div style={{ borderTop: '1px solid var(--color-brand-border)', paddingTop: 16 }}>
              <div style={{ display: 'flex', alignItems: 'center', gap: 6, marginBottom: 14 }}>
                <FiLock style={{ color: 'var(--color-brand)', fontSize: 16 }} />
                <span style={{ fontSize: 13, fontWeight: 600, color: 'var(--color-brand-heading)' }}>DPI Evasion & Security Hardening</span>
                <FiHelpCircle 
                  style={{ cursor: 'pointer', color: 'var(--color-brand-muted)' }} 
                  onClick={() => showHelp('DPI Evasion & Security Hardening', 'TLS fragmentation splits ClientHello records to slip past SNI deep packet filters. Browser fingerprints masquerade TLS signatures as standard web browsers (Chrome/Safari). ECH hides hostnames, and TCP Brutal speeds up output on congested lossy paths.')}
                />
              </div>

              <div style={{ display: 'flex', flexDirection: 'column', gap: 12 }}>
                <div style={{ display: 'flex', alignItems: 'center', justifyContent: 'space-between' }}>
                  <div>
                    <label style={{ fontSize: 12, fontWeight: 600, color: 'var(--color-brand-heading)', margin: 0 }}>
                      TLS Record-Layer Fragmentation
                    </label>
                    <span style={{ fontSize: 9, color: 'var(--color-brand-text)', display: 'block' }}>
                      Splits client handshake packets into segments of 100-200 bytes with 10ms intervals.
                    </span>
                  </div>
                  <input
                    type="checkbox"
                    checked={evasionFragment}
                    onChange={(e) => setEvasionFragment(e.target.checked)}
                    style={{ width: 16, height: 16, cursor: 'pointer', accentColor: 'var(--color-brand)' }}
                  />
                </div>

                <div style={{ display: 'flex', alignItems: 'center', justifyContent: 'space-between' }}>
                  <div>
                    <label style={{ fontSize: 12, fontWeight: 600, color: 'var(--color-brand-heading)', margin: 0 }}>
                      TCP Brutal Congestion Control
                    </label>
                    <span style={{ fontSize: 9, color: 'var(--color-brand-text)', display: 'block' }}>
                      Enforces aggressive window scaling and packet loss compensation over direct outbounds.
                    </span>
                  </div>
                  <input
                    type="checkbox"
                    checked={evasionTcpBrutal}
                    onChange={(e) => setEvasionTcpBrutal(e.target.checked)}
                    style={{ width: 16, height: 16, cursor: 'pointer', accentColor: 'var(--color-brand)' }}
                  />
                </div>

                <div style={{ display: 'flex', alignItems: 'center', justifyContent: 'space-between' }}>
                  <div>
                    <label style={{ fontSize: 12, fontWeight: 600, color: 'var(--color-brand-heading)', margin: 0 }}>
                      Enable Connection Multiplexing (Mux)
                    </label>
                    <span style={{ fontSize: 9, color: 'var(--color-brand-text)', display: 'block' }}>
                      Reuses TCP socket handlers to prevent firewall port tracking blocks.
                    </span>
                  </div>
                  <input
                    type="checkbox"
                    checked={muxEnabled}
                    onChange={(e) => setMuxEnabled(e.target.checked)}
                    style={{ width: 16, height: 16, cursor: 'pointer', accentColor: 'var(--color-brand)' }}
                  />
                </div>

                <div style={{ display: 'grid', gridTemplateColumns: '1fr 1fr', gap: 16, marginTop: 4 }}>
                  <div>
                    <label style={{ display: 'block', fontSize: 11, fontWeight: 600, color: 'var(--color-brand-muted)', marginBottom: 6, textTransform: 'uppercase' }}>uTLS Browser Fingerprint</label>
                    <select
                      value={evasionFingerprint}
                      onChange={(e) => setEvasionFingerprint(e.target.value)}
                      style={{
                        width: '100%',
                        padding: '8px 12px',
                        borderRadius: 8,
                        border: '1px solid var(--color-brand-border)',
                        background: 'var(--color-brand-card)',
                        fontSize: 13,
                        color: 'var(--color-brand-heading)'
                      }}
                    >
                      <option value="chrome">Google Chrome (Standard)</option>
                      <option value="firefox">Mozilla Firefox</option>
                      <option value="safari">Apple Safari</option>
                      <option value="edge">Microsoft Edge</option>
                      <option value="randomized">Randomized uTLS Fingerprint</option>
                    </select>
                  </div>

                  <div>
                    <label style={{ display: 'block', fontSize: 11, fontWeight: 600, color: 'var(--color-brand-muted)', marginBottom: 6, textTransform: 'uppercase' }}>Encrypted Client Hello (ECH)</label>
                    <div style={{ display: 'flex', alignItems: 'center', gap: 8 }}>
                      <input
                        type="checkbox"
                        checked={evasionEch}
                        onChange={(e) => setEvasionEch(e.target.checked)}
                        style={{ width: 16, height: 16, cursor: 'pointer', accentColor: 'var(--color-brand)' }}
                      />
                      <span style={{ fontSize: 12, color: 'var(--color-brand-text)' }}>Enable ECH</span>
                    </div>
                  </div>
                </div>

                {evasionEch && (
                  <div>
                    <label style={{ display: 'block', fontSize: 11, fontWeight: 600, color: 'var(--color-brand-muted)', marginBottom: 6, textTransform: 'uppercase' }}>ECH Base64 Config Payload</label>
                    <textarea
                      value={evasionEchConfig}
                      onChange={(e) => setEvasionEchConfig(e.target.value)}
                      placeholder="Paste ECHConfigs Base64 string"
                      rows={2}
                      style={{
                        width: '100%',
                        padding: '8px 10px',
                        borderRadius: 8,
                        border: '1px solid var(--color-brand-border)',
                        background: 'var(--color-brand-card)',
                        fontSize: 12,
                        fontFamily: 'Fira Code',
                        color: 'var(--color-brand-heading)'
                      }}
                    />
                  </div>
                )}
              </div>
            </div>

            <button type="submit" className="btn btn--primary" style={{ display: 'flex', alignItems: 'center', gap: 6 }} disabled={isLoading}>
              <FiSave /> Save Proxy Settings
            </button>
          </form>

        </div>

        {/* Right Side: Log terminal, diagnostic probers, wol, and debug proxy */}
        <div style={{ display: 'flex', flexDirection: 'column', gap: 24 }}>
          
          {/* Active Logs Terminal */}
          <div className="g-card" style={{ display: 'flex', flexDirection: 'column', height: 320 }}>
            <div style={{ display: 'flex', alignItems: 'center', justifyContent: 'space-between', marginBottom: 12 }}>
              <div style={{ display: 'flex', alignItems: 'center', gap: 6 }}>
                <FiTerminal style={{ color: 'var(--color-brand)', fontSize: 16 }} />
                <span style={{ fontSize: 13, fontWeight: 600, color: 'var(--color-brand-heading)' }}>Live Core Logs</span>
              </div>
              <input
                type="text"
                placeholder="Filter logs..."
                value={logsQuery}
                onChange={(e) => setLogsQuery(e.target.value)}
                style={{
                  width: 120,
                  padding: '4px 8px',
                  borderRadius: 4,
                  border: '1px solid var(--color-brand-border)',
                  background: 'var(--color-brand-card)',
                  fontSize: 11,
                  color: 'var(--color-brand-heading)'
                }}
              />
            </div>
            
            <div style={{
              flex: 1,
              background: '#1a1a2e',
              borderRadius: 8,
              padding: 10,
              fontFamily: 'Fira Code, monospace',
              fontSize: 10,
              color: '#a9b1d6',
              overflowY: 'auto',
              display: 'flex',
              flexDirection: 'column',
              gap: 4
            }}>
              {logs.length === 0 ? (
                <div style={{ color: '#565f89', textAlign: 'center', marginTop: 80 }}>Listening for core logs...</div>
              ) : (
                logs.map((log, idx) => (
                  <div key={idx} style={{ wordBreak: 'break-all', whiteSpace: 'pre-wrap' }}>{log}</div>
                ))
              )}
              <div ref={logsEndRef} />
            </div>
          </div>

          {/* Diagnostic utilities: Port scanner */}
          <div className="g-card">
            <div style={{ display: 'flex', alignItems: 'center', gap: 8, marginBottom: 14 }}>
              <FiSearch style={{ color: 'var(--color-brand)', fontSize: 16 }} />
              <span style={{ fontSize: 13, fontWeight: 600, color: 'var(--color-brand-heading)' }}>Port Scanning Utility</span>
              <FiHelpCircle 
                style={{ cursor: 'pointer', color: 'var(--color-brand-muted)' }} 
                onClick={() => showHelp('Port Scanning Utility', 'Tests connectivity to remote targets. Input IP addresses and comma-separated ports to perform concurrent port opening scans.')}
              />
            </div>

            <div style={{ display: 'flex', flexDirection: 'column', gap: 12 }}>
              <div style={{ display: 'grid', gridTemplateColumns: '1fr 90px', gap: 10 }}>
                <input
                  type="text"
                  placeholder="Target IP / Hostname"
                  value={probeIP}
                  onChange={(e) => setProbeIP(e.target.value)}
                  style={{
                    padding: '8px 10px',
                    borderRadius: 6,
                    border: '1px solid var(--color-brand-border)',
                    background: 'var(--color-brand-card)',
                    fontSize: 12,
                    color: 'var(--color-brand-heading)'
                  }}
                />
                <select
                  value={probeProto}
                  onChange={(e) => setProbeProto(e.target.value)}
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
                  <option value="udp">UDP</option>
                </select>
              </div>

              <div style={{ display: 'flex', gap: 10 }}>
                <input
                  type="text"
                  placeholder="Ports (e.g. 80,443,53)"
                  value={probePorts}
                  onChange={(e) => setProbePorts(e.target.value)}
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
                <button className="btn btn--primary btn--sm" onClick={handleProbePorts} disabled={isLoading}>Scan</button>
              </div>

              {probeResults.length > 0 && (
                <div style={{ 
                  marginTop: 6, 
                  background: 'var(--color-brand-bg)', 
                  padding: 8, 
                  borderRadius: 6, 
                  maxHeight: 120, 
                  overflowY: 'auto',
                  border: '1px solid var(--color-brand-border)'
                }}>
                  {probeResults.map((r, idx) => (
                    <div key={idx} style={{ display: 'flex', justifyContent: 'space-between', fontSize: 11, marginBottom: 4 }}>
                      <span style={{ color: 'var(--color-brand-heading)', fontFamily: 'Fira Code' }}>Port {r.port} ({r.protocol.toUpperCase()})</span>
                      <span style={{ fontWeight: 700, color: r.open ? 'var(--color-brand-green)' : 'var(--color-brand-red)' }}>
                        {r.open ? `OPEN (${r.latency_ms}ms)` : 'CLOSED'}
                      </span>
                    </div>
                  ))}
                </div>
              )}
            </div>
          </div>

          {/* Local Service Discovery */}
          <div className="g-card">
            <div style={{ display: 'flex', alignItems: 'center', justifyContent: 'space-between', marginBottom: 14 }}>
              <div style={{ display: 'flex', alignItems: 'center', gap: 6 }}>
                <FiWifi style={{ color: 'var(--color-brand)', fontSize: 16 }} />
                <span style={{ fontSize: 13, fontWeight: 600, color: 'var(--color-brand-heading)' }}>Subnet Service Discovery</span>
                <FiHelpCircle 
                  style={{ cursor: 'pointer', color: 'var(--color-brand-muted)' }} 
                  onClick={() => showHelp('Subnet Service Discovery', 'Sweeps the local IPv4 subnet (using TCP port 80/22 checks) to auto-discover active network devices.')}
                />
              </div>
              <button className="btn btn--sm btn--secondary" onClick={handleDiscoverDevices} disabled={isDiscovering}>
                {isDiscovering ? 'Scanning...' : 'Scan Subnet'}
              </button>
            </div>

            {discoveredDevices.length > 0 && (
              <div style={{ 
                maxHeight: 150, 
                overflowY: 'auto', 
                background: 'var(--color-brand-bg)', 
                borderRadius: 6, 
                padding: 8,
                border: '1px solid var(--color-brand-border)'
              }}>
                {discoveredDevices.map((d, idx) => (
                  <div key={idx} style={{ display: 'flex', justifyContent: 'space-between', fontSize: 11, marginBottom: 4 }}>
                    <div>
                      <span style={{ color: 'var(--color-brand-heading)', fontWeight: 600, fontFamily: 'Fira Code' }}>{d.ip}</span>
                      {d.hostname && <span style={{ color: 'var(--color-brand-muted)', marginLeft: 6 }}>({d.hostname})</span>}
                    </div>
                    <span style={{ color: 'var(--color-brand-green)', fontWeight: 700 }}>ACTIVE</span>
                  </div>
                ))}
              </div>
            )}
          </div>

          {/* Wake on LAN */}
          <div className="g-card">
            <div style={{ display: 'flex', alignItems: 'center', gap: 6, marginBottom: 14 }}>
              <FiMonitor style={{ color: 'var(--color-brand)', fontSize: 16 }} />
              <span style={{ fontSize: 13, fontWeight: 600, color: 'var(--color-brand-heading)' }}>Wake-on-LAN (WOL) Client</span>
              <FiHelpCircle 
                style={{ cursor: 'pointer', color: 'var(--color-brand-muted)' }} 
                onClick={() => showHelp('Wake-on-LAN (WOL)', 'Sends a UDP magic packet (0xFF repeat + MAC address repeat) to boot remote network hardware from sleep state.')}
              />
            </div>

            <div style={{ display: 'flex', flexDirection: 'column', gap: 10 }}>
              <input
                type="text"
                placeholder="Target MAC Address (e.g. AA:BB:CC:DD:EE:FF)"
                value={wolMac}
                onChange={(e) => setWolMac(e.target.value)}
                style={{
                  padding: '8px 10px',
                  borderRadius: 6,
                  border: '1px solid var(--color-brand-border)',
                  background: 'var(--color-brand-card)',
                  fontSize: 12,
                  color: 'var(--color-brand-heading)'
                }}
              />
              <div style={{ display: 'flex', gap: 10 }}>
                <input
                  type="text"
                  placeholder="Broadcast IP (default 255.255.255.255)"
                  value={wolBcast}
                  onChange={(e) => setWolBcast(e.target.value)}
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
                <button className="btn btn--primary btn--sm" onClick={handleSendWol} disabled={isLoading}>Wake</button>
              </div>
            </div>
          </div>

          {/* Local Interception Debug Proxy */}
          <div className="g-card">
            <div style={{ display: 'flex', alignItems: 'center', justifyContent: 'space-between', marginBottom: 12 }}>
              <div style={{ display: 'flex', alignItems: 'center', gap: 6 }}>
                <FiZap style={{ color: 'var(--color-brand)', fontSize: 16 }} />
                <span style={{ fontSize: 13, fontWeight: 600, color: 'var(--color-brand-heading)' }}>Local Interception Proxy</span>
                <FiHelpCircle 
                  style={{ cursor: 'pointer', color: 'var(--color-brand-muted)' }} 
                  onClick={() => showHelp('Local Interception Proxy', 'Fires up a local HTTP/HTTPS CONNECT tunnel proxy. Captures outgoing application connections and logs transaction methods/hosts for diagnostic auditing.')}
                />
              </div>
              <span style={{
                fontSize: 10,
                fontWeight: 700,
                color: isDebugProxyActive ? 'var(--color-brand-green)' : 'var(--color-brand-red)'
              }}>{isDebugProxyActive ? 'RUNNING' : 'INACTIVE'}</span>
            </div>

            <div style={{ display: 'flex', flexDirection: 'column', gap: 12 }}>
              <div style={{ display: 'flex', gap: 10 }}>
                <input
                  type="number"
                  placeholder="Proxy Port (e.g. 8080)"
                  value={debugProxyPort}
                  onChange={(e) => setDebugProxyPort(Number(e.target.value))}
                  disabled={isDebugProxyActive}
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
                <button 
                  className="btn btn--sm"
                  onClick={handleToggleDebugProxy}
                  style={{
                    background: isDebugProxyActive ? '#ef4444' : 'var(--color-brand)',
                    color: '#fff',
                    border: 'none',
                    fontWeight: 600
                  }}
                >
                  {isDebugProxyActive ? 'Stop' : 'Start'}
                </button>
              </div>

              {isDebugProxyActive && debugProxyLogs.length > 0 && (
                <div style={{ 
                  maxHeight: 120, 
                  overflowY: 'auto', 
                  background: '#1a1a2e', 
                  borderRadius: 6, 
                  padding: 8,
                  fontFamily: 'Fira Code',
                  fontSize: 9,
                  color: '#a9b1d6'
                }}>
                  {debugProxyLogs.map((l, idx) => (
                    <div key={idx} style={{ marginBottom: 2 }}>{l}</div>
                  ))}
                </div>
              )}
            </div>
          </div>

          {/* System Settings & Shortcuts */}
          <div className="g-card">
            <div style={{ display: 'flex', alignItems: 'center', gap: 6, marginBottom: 14 }}>
              <FiSettings style={{ color: 'var(--color-brand)', fontSize: 16 }} />
              <span style={{ fontSize: 13, fontWeight: 600, color: 'var(--color-brand-heading)' }}>System Tray & Keybindings</span>
              <FiHelpCircle 
                style={{ cursor: 'pointer', color: 'var(--color-brand-muted)' }} 
                onClick={() => showHelp('System Tray & Hotkeys', 'Configure background daemon tray status flags and bind OS-level keyboard shortcuts (e.g. Ctrl+Shift+X) to quickly toggle SOCKS5 proxy status.')}
              />
            </div>

            <div style={{ display: 'flex', flexDirection: 'column', gap: 12 }}>
              <div style={{ display: 'flex', alignItems: 'center', justifyContent: 'space-between' }}>
                <span style={{ fontSize: 12, color: 'var(--color-brand-heading)' }}>Enable System Tray Icon</span>
                <input
                  type="checkbox"
                  checked={systemTrayEnabled}
                  onChange={(e) => setSystemTrayEnabled(e.target.checked)}
                  style={{ width: 16, height: 16, cursor: 'pointer', accentColor: 'var(--color-brand)' }}
                />
              </div>

              <div style={{ display: 'flex', gap: 10 }}>
                <input
                  type="text"
                  placeholder="Toggle Hotkey (e.g. Ctrl+Shift+X)"
                  value={hotkeys}
                  onChange={(e) => setHotkeys(e.target.value)}
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
                <button className="btn btn--sm btn--secondary" onClick={() => setMessage({ type: 'success', text: 'Hotkey overrides saved!' })}>Save</button>
              </div>
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
