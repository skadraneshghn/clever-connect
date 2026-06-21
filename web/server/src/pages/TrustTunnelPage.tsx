import React, { useState, useEffect } from 'react';
import {
  FiSliders,
  FiCpu,
  FiGlobe,
  FiPlay,
  FiSquare,
  FiSave,
  FiRefreshCw,
  FiKey,
  FiPlus,
  FiTrash2,
  FiShare2,
  FiUsers,
  FiChevronDown,
  FiChevronUp,
  FiHelpCircle
} from 'react-icons/fi';
import { useTrustTunnelStore } from '../store/trusttunnelStore';

export const TrustTunnelPage: React.FC = () => {
  const {
    config,
    users,
    rules,
    isRunning,
    isLoading,
    error,
    successMessage,
    fetchConfig,
    saveConfig,
    startEngine,
    stopEngine,
    addUser,
    deleteUser,
    addRule,
    deleteRule,
    exportConnectionToken,
    generateCert,
    clearMessages
  } = useTrustTunnelStore();

  // Local Form state
  const [acmeEmail, setAcmeEmail] = useState('');
  const [listenAddress, setListenAddress] = useState('0.0.0.0:443');
  const [forcedTransport, setForcedTransport] = useState<'http2' | 'http1' | 'quic'>('http2');
  const [authFailureStatusCode, setAuthFailureStatusCode] = useState(404);
  const [clientRandomPrefix, setClientRandomPrefix] = useState('a0b0/f0f0');
  const [h2InitialStreamWindowSize, setH2InitialStreamWindowSize] = useState(131072);
  const [h2InitialConnWindowSize, setH2InitialConnWindowSize] = useState(262144);
  const [tlsHandshakeTimeoutSecs, setTlsHandshakeTimeoutSecs] = useState(4);
  const [activePreset, setActivePreset] = useState('iran-stealth');
  const [tlsCertPath, setTlsCertPath] = useState('');
  const [tlsKeyPath, setTlsKeyPath] = useState('');
  const [serverHostname, setServerHostname] = useState('');

  // User management state
  const [usernameInput, setUsernameInput] = useState('');
  const [passwordInput, setPasswordInput] = useState('');

  // Rule management state
  const [newCidr, setNewCidr] = useState('');
  const [newStrategy, setNewStrategy] = useState('direct-route');
  const [newDesc, setNewDesc] = useState('');

  // UI state
  const [showAdvanced, setShowAdvanced] = useState(false);
  const [exportedToken, setExportedToken] = useState('');
  const [tlsServerCert, setTlsServerCert] = useState('');
  const [certCopied, setCertCopied] = useState(false);

  // Sync store configs with local state
  useEffect(() => {
    fetchConfig();
  }, []);

  useEffect(() => {
    if (config) {
      setListenAddress(config.listen_address || '0.0.0.0:443');
      setForcedTransport(config.forced_transport || 'http2');
      setAuthFailureStatusCode(config.auth_failure_status_code || 404);
      setClientRandomPrefix(config.client_random_prefix || '');
      setH2InitialStreamWindowSize(config.h2_initial_stream_window_size || 131072);
      setH2InitialConnWindowSize(config.h2_initial_conn_window_size || 262144);
      setTlsHandshakeTimeoutSecs(config.tls_handshake_timeout_secs || 4);
      setActivePreset(config.active_preset || 'iran-stealth');
      setTlsCertPath(config.tls_cert_path || '');
      setTlsKeyPath(config.tls_key_path || '');
      setServerHostname(config.server_hostname || '');
      setTlsServerCert(config.tls_server_cert || '');
    }
  }, [config]);

  // Handle Preset application
  const applyPresetLocally = (presetName: string) => {
    setActivePreset(presetName);
    if (presetName === 'iran-stealth') {
      setForcedTransport('http2');
      setAuthFailureStatusCode(407);
      setClientRandomPrefix('a0b0/f0f0');
      setH2InitialStreamWindowSize(131072);
      setH2InitialConnWindowSize(262144);
      setTlsHandshakeTimeoutSecs(4);
    } else if (presetName === 'standard-web') {
      setForcedTransport('http2');
      setAuthFailureStatusCode(405);
      setClientRandomPrefix('');
      setH2InitialStreamWindowSize(65535);
      setH2InitialConnWindowSize(131072);
      setTlsHandshakeTimeoutSecs(10);
    } else if (presetName === 'minimal-cover') {
      setForcedTransport('http1');
      setAuthFailureStatusCode(407);
      setClientRandomPrefix('');
      setH2InitialStreamWindowSize(65535);
      setH2InitialConnWindowSize(131072);
      setTlsHandshakeTimeoutSecs(30);
    }
  };

  const handleSave = async (e: React.FormEvent) => {
    e.preventDefault();
    await saveConfig({
      listen_address: listenAddress,
      forced_transport: forcedTransport,
      auth_failure_status_code: Number(authFailureStatusCode),
      client_random_prefix: clientRandomPrefix,
      h2_initial_stream_window_size: Number(h2InitialStreamWindowSize),
      h2_initial_conn_window_size: Number(h2InitialConnWindowSize),
      tls_handshake_timeout_secs: Number(tlsHandshakeTimeoutSecs),
      active_preset: activePreset,
      tls_cert_path: tlsCertPath,
      tls_key_path: tlsKeyPath,
      server_hostname: serverHostname,
      is_active: config?.is_active || false
    });
  };

  const handleToggleAutoStart = async () => {
    if (!config) return;
    await saveConfig({
      is_active: !config.is_active
    });
  };

  const handleAddUserSubmit = async (e: React.FormEvent) => {
    e.preventDefault();
    if (!usernameInput || !passwordInput) return;
    await addUser(usernameInput, passwordInput);
    setUsernameInput('');
    setPasswordInput('');
  };

  const handleAddRuleSubmit = async (e: React.FormEvent) => {
    e.preventDefault();
    if (!newCidr) return;
    await addRule(newCidr, newStrategy, newDesc);
    setNewCidr('');
    setNewDesc('');
  };

  const handleGenerateCert = async () => {
    if (!serverHostname) return;
    try {
      const data = await generateCert(serverHostname, acmeEmail);
      if (data) {
        setTlsCertPath(data.cert_chain_path);
        setTlsKeyPath(data.private_key_path);
      }
    } catch {
      // error handled by store
    }
  };

  const handleExportToken = async () => {
    try {
      const token = await exportConnectionToken();
      setExportedToken(token);
    } catch {
      // error handled by store
    }
  };

  const copyToClipboard = () => {
    if (!exportedToken) return;
    navigator.clipboard.writeText(exportedToken);
    alert('Token copied to clipboard!');
  };

  const copyCertToClipboard = () => {
    if (!tlsServerCert) return;
    navigator.clipboard.writeText(tlsServerCert);
    setCertCopied(true);
    setTimeout(() => setCertCopied(false), 2000);
  };

  return (
    <div>
      {/* Title */}
      <div style={{ display: 'flex', justifyContent: 'space-between', alignItems: 'center', marginBottom: 24 }}>
        <div>
          <h1 style={{ fontSize: 22, fontWeight: 700, color: 'var(--color-brand-heading)', margin: 0 }}>TrustTunnel Server</h1>
          <p style={{ fontSize: 12, color: 'var(--color-brand-text)', margin: '4px 0 0' }}>
            Stealth Layer-4 Obfuscation VPN Protocol using Chrome Fingerprint simulation.
          </p>
        </div>
        <button className="btn btn--sm" onClick={fetchConfig} disabled={isLoading}>
          <FiRefreshCw className={isLoading ? 'spin-animation' : ''} style={{ marginRight: 6 }} /> Refresh
        </button>
      </div>

      {/* Messages */}
      {error && (
        <div style={{
          padding: '12px 16px',
          borderRadius: 10,
          marginBottom: 20,
          fontSize: 13,
          fontWeight: 500,
          background: '#fee2e2',
          border: '1px solid #fca5a5',
          color: '#b91c1c',
          display: 'flex',
          justifyContent: 'space-between',
          alignItems: 'center'
        }}>
          <span>{error}</span>
          <button onClick={clearMessages} style={{ background: 'none', border: 'none', color: '#b91c1c', cursor: 'pointer', fontWeight: 'bold' }}>X</button>
        </div>
      )}

      {successMessage && (
        <div style={{
          padding: '12px 16px',
          borderRadius: 10,
          marginBottom: 20,
          fontSize: 13,
          fontWeight: 500,
          background: 'var(--color-brand-light)',
          border: '1px solid var(--color-brand-border)',
          color: 'var(--color-brand)',
          display: 'flex',
          justifyContent: 'space-between',
          alignItems: 'center'
        }}>
          <span>{successMessage}</span>
          <button onClick={clearMessages} style={{ background: 'none', border: 'none', color: 'var(--color-brand)', cursor: 'pointer', fontWeight: 'bold' }}>X</button>
        </div>
      )}

      {/* Layout Grid */}
      <div style={{ display: 'grid', gridTemplateColumns: '1fr 340px', gap: 24, alignItems: 'start' }}>

        {/* Left Column (Forms & Rules) */}
        <div style={{ display: 'flex', flexDirection: 'column', gap: 24 }}>

          {/* Preset Selector */}
          <div className="g-card" style={{ padding: 20 }}>
            <div style={{ display: 'flex', alignItems: 'center', gap: 8, marginBottom: 16 }}>
              <FiSliders style={{ color: 'var(--color-brand)', fontSize: 18 }} />
              <span style={{ fontSize: 15, fontWeight: 600, color: 'var(--color-brand-heading)' }}>Select Defenses Preset</span>
            </div>

            <div style={{ display: 'grid', gridTemplateColumns: '1fr 1fr 1fr', gap: 12 }}>
              {/* Iran Stealth Preset */}
              <div
                onClick={() => applyPresetLocally('iran-stealth')}
                style={{
                  border: activePreset === 'iran-stealth' ? '2px solid var(--color-brand)' : '1px solid var(--color-brand-border)',
                  borderRadius: 10,
                  padding: 14,
                  cursor: 'pointer',
                  background: activePreset === 'iran-stealth' ? 'var(--color-brand-light)' : 'var(--color-brand-card)',
                  transition: 'all 0.2s ease'
                }}
              >
                <div style={{ fontWeight: 600, fontSize: 13, color: activePreset === 'iran-stealth' ? 'var(--color-brand)' : 'var(--color-brand-heading)', marginBottom: 4 }}>
                  🇮🇷 Iran Stealth (DPI Defense)
                </div>
                <div style={{ fontSize: 11, color: 'var(--color-brand-muted)', lineHeight: '1.4' }}>
                  HTTP2 + 404 camouflage, randomized prefix masking, Custom TLS flow windows, and Kill-Switch enabled.
                </div>
              </div>

              {/* Standard Web Emulation Preset */}
              <div
                onClick={() => applyPresetLocally('standard-web')}
                style={{
                  border: activePreset === 'standard-web' ? '2px solid var(--color-brand)' : '1px solid var(--color-brand-border)',
                  borderRadius: 10,
                  padding: 14,
                  cursor: 'pointer',
                  background: activePreset === 'standard-web' ? 'var(--color-brand-light)' : 'var(--color-brand-card)',
                  transition: 'all 0.2s ease'
                }}
              >
                <div style={{ fontWeight: 600, fontSize: 13, color: activePreset === 'standard-web' ? 'var(--color-brand)' : 'var(--color-brand-heading)', marginBottom: 4 }}>
                  🌐 Standard Web Emulation
                </div>
                <div style={{ fontSize: 11, color: 'var(--color-brand-muted)', lineHeight: '1.4' }}>
                  HTTP2 with normal window size, standard 401 unauthorized probe behavior, and longer handshakes.
                </div>
              </div>

              {/* Minimal Cover Mask Preset */}
              <div
                onClick={() => applyPresetLocally('minimal-cover')}
                style={{
                  border: activePreset === 'minimal-cover' ? '2px solid var(--color-brand)' : '1px solid var(--color-brand-border)',
                  borderRadius: 10,
                  padding: 14,
                  cursor: 'pointer',
                  background: activePreset === 'minimal-cover' ? 'var(--color-brand-light)' : 'var(--color-brand-card)',
                  transition: 'all 0.2s ease'
                }}
              >
                <div style={{ fontWeight: 600, fontSize: 13, color: activePreset === 'minimal-cover' ? 'var(--color-brand)' : 'var(--color-brand-heading)', marginBottom: 4 }}>
                  🛡️ Minimal Cover Mask
                </div>
                <div style={{ fontSize: 11, color: 'var(--color-brand-muted)', lineHeight: '1.4' }}>
                  HTTP1.1 transport, 407 proxy authentication error spoofing, and broad timeouts.
                </div>
              </div>
            </div>
          </div>

          {/* Server Settings Form */}
          <form onSubmit={handleSave} className="g-card" style={{ padding: 20, display: 'flex', flexDirection: 'column', gap: 16 }}>
            <div style={{ display: 'flex', alignItems: 'center', gap: 8, marginBottom: 4 }}>
              <FiGlobe style={{ color: 'var(--color-brand)', fontSize: 18 }} />
              <span style={{ fontSize: 15, fontWeight: 600, color: 'var(--color-brand-heading)' }}>Server Bind Settings</span>
            </div>

            <div style={{ display: 'grid', gridTemplateColumns: '1fr 2fr', gap: 16 }}>
              <div>
                <label style={{ display: 'block', fontSize: 11, fontWeight: 600, color: 'var(--color-brand-muted)', marginBottom: 6, textTransform: 'uppercase' }}>Listen Address</label>
                <input
                  type="text"
                  value={listenAddress}
                  onChange={(e) => setListenAddress(e.target.value)}
                  placeholder="e.g. 0.0.0.0:443"
                  style={{
                    width: '100%',
                    padding: '10px 12px',
                    borderRadius: 8,
                    border: '1px solid var(--color-brand-border)',
                    background: 'var(--color-brand-card)',
                    color: 'var(--color-brand-heading)',
                    fontSize: 13,
                    fontFamily: 'Fira Code'
                  }}
                  required
                />
              </div>

              <div>
                <label style={{ display: 'block', fontSize: 11, fontWeight: 600, color: 'var(--color-brand-muted)', marginBottom: 6, textTransform: 'uppercase' }}>Public Hostname / SNI Domain Match</label>
                <input
                  type="text"
                  value={serverHostname}
                  onChange={(e) => setServerHostname(e.target.value)}
                  placeholder="e.g. secure.yourdomain.com"
                  style={{
                    width: '100%',
                    padding: '10px 12px',
                    borderRadius: 8,
                    border: '1px solid var(--color-brand-border)',
                    background: 'var(--color-brand-card)',
                    color: 'var(--color-brand-heading)',
                    fontSize: 13
                  }}
                  required
                />
              </div>
            </div>

            <div style={{ display: 'grid', gridTemplateColumns: '1fr 1fr', gap: 16 }}>
              <div>
                <label style={{ display: 'block', fontSize: 11, fontWeight: 600, color: 'var(--color-brand-muted)', marginBottom: 6, textTransform: 'uppercase' }}>TLS Certificate Absolute Path (Optional)</label>
                <input
                  type="text"
                  value={tlsCertPath}
                  onChange={(e) => setTlsCertPath(e.target.value)}
                  placeholder="e.g. /etc/letsencrypt/live/secure/fullchain.pem"
                  style={{
                    width: '100%',
                    padding: '10px 12px',
                    borderRadius: 8,
                    border: '1px solid var(--color-brand-border)',
                    background: 'var(--color-brand-card)',
                    color: 'var(--color-brand-heading)',
                    fontSize: 13,
                    fontFamily: 'Fira Code'
                  }}
                />
              </div>

              <div>
                <label style={{ display: 'block', fontSize: 11, fontWeight: 600, color: 'var(--color-brand-muted)', marginBottom: 6, textTransform: 'uppercase' }}>TLS Private Key Absolute Path (Optional)</label>
                <input
                  type="text"
                  value={tlsKeyPath}
                  onChange={(e) => setTlsKeyPath(e.target.value)}
                  placeholder="e.g. /etc/letsencrypt/live/secure/privkey.pem"
                  style={{
                    width: '100%',
                    padding: '10px 12px',
                    borderRadius: 8,
                    border: '1px solid var(--color-brand-border)',
                    background: 'var(--color-brand-card)',
                    color: 'var(--color-brand-heading)',
                    fontSize: 13,
                    fontFamily: 'Fira Code'
                  }}
                />
              </div>
            </div>

            <p style={{ fontSize: 11, color: 'var(--color-brand-muted)', margin: '0 0 4px', lineHeight: '1.4' }}>
              💡 <b>Tip:</b> If certificate paths are left blank, the VPN engine will automatically generate a fallback self-signed TLS certificate for your public hostname.
            </p>

            {/* Let's Encrypt Cert Generator */}
            <div style={{
              border: '1px dashed var(--color-brand)',
              borderRadius: 10,
              padding: 16,
              background: 'rgba(var(--color-brand-rgb), 0.02)',
              marginTop: 4,
              display: 'flex',
              flexDirection: 'column',
              gap: 12
            }}>
              <div style={{ display: 'flex', alignItems: 'center', gap: 8 }}>
                <span style={{ fontSize: 13, fontWeight: 700, color: 'var(--color-brand-heading)' }}>🛡️ Let's Encrypt Self-Hosted Cert Generator</span>
              </div>
              <p style={{ fontSize: 11, color: 'var(--color-brand-muted)', margin: 0, lineHeight: '1.4' }}>
                Ensure your <b>Public Hostname</b> resolves to this server's public IP. The generator handles validation via port 80 dynamically.
              </p>
              <div style={{ display: 'grid', gridTemplateColumns: '1fr 1fr auto', gap: 12, alignItems: 'end' }}>
                <div>
                  <label style={{ display: 'block', fontSize: 10, fontWeight: 600, color: 'var(--color-brand-muted)', marginBottom: 4, textTransform: 'uppercase' }}>Admin Email (optional)</label>
                  <input
                    type="email"
                    value={acmeEmail}
                    onChange={(e) => setAcmeEmail(e.target.value)}
                    placeholder="e.g. admin@yourdomain.com"
                    style={{
                      width: '100%',
                      padding: '8px 10px',
                      borderRadius: 6,
                      border: '1px solid var(--color-brand-border)',
                      background: 'var(--color-brand-card)',
                      color: 'var(--color-brand-heading)',
                      fontSize: 12
                    }}
                  />
                </div>
                <div>
                  <label style={{ display: 'block', fontSize: 10, fontWeight: 600, color: 'var(--color-brand-muted)', marginBottom: 4, textTransform: 'uppercase' }}>Target Hostname</label>
                  <input
                    type="text"
                    value={serverHostname}
                    onChange={(e) => setServerHostname(e.target.value)}
                    placeholder="e.g. secure.yourdomain.com"
                    style={{
                      width: '100%',
                      padding: '8px 10px',
                      borderRadius: 6,
                      border: '1px solid var(--color-brand-border)',
                      background: 'var(--color-brand-card)',
                      color: 'var(--color-brand-heading)',
                      fontSize: 12
                    }}
                  />
                </div>
                <button
                  type="button"
                  onClick={handleGenerateCert}
                  className="btn btn--sm btn--primary"
                  disabled={isLoading || !serverHostname}
                  style={{ height: 32 }}
                >
                  Generate Cert
                </button>
              </div>
            </div>

            {/* Collapsible Advanced Settings */}
            <div style={{ borderTop: '1px solid var(--color-brand-border)', paddingTop: 16 }}>
              <button
                type="button"
                onClick={() => setShowAdvanced(!showAdvanced)}
                style={{
                  display: 'flex',
                  alignItems: 'center',
                  gap: 6,
                  background: 'none',
                  border: 'none',
                  color: 'var(--color-brand)',
                  cursor: 'pointer',
                  fontWeight: 600,
                  fontSize: 13,
                  padding: 0
                }}
              >
                {showAdvanced ? <FiChevronUp /> : <FiChevronDown />} Advanced Defenses Settings
              </button>

              {showAdvanced && (
                <div style={{ display: 'grid', gridTemplateColumns: '1fr 1fr', gap: 16, marginTop: 16 }}>
                  <div>
                    <label style={{ display: 'block', fontSize: 11, fontWeight: 600, color: 'var(--color-brand-muted)', marginBottom: 6, textTransform: 'uppercase' }}>Forced Transport Layer</label>
                    <select
                      value={forcedTransport}
                      onChange={(e) => setForcedTransport(e.target.value as any)}
                      style={{
                        width: '100%',
                        padding: '10px 12px',
                        borderRadius: 8,
                        border: '1px solid var(--color-brand-border)',
                        background: 'var(--color-brand-card)',
                        color: 'var(--color-brand-heading)',
                        fontSize: 13
                      }}
                    >
                      <option value="http2">HTTP/2 Multiplex (Recommended)</option>
                      <option value="http1">HTTP/1.1 Standard</option>
                      <option value="quic">QUIC / UDP Transport</option>
                    </select>
                  </div>

                  <div>
                    <label style={{ display: 'block', fontSize: 11, fontWeight: 600, color: 'var(--color-brand-muted)', marginBottom: 6, textTransform: 'uppercase' }}>Probe Spoof Status Code</label>
                    <select
                      value={authFailureStatusCode}
                      onChange={(e) => setAuthFailureStatusCode(Number(e.target.value))}
                      style={{
                        width: '100%',
                        padding: '10px 12px',
                        borderRadius: 8,
                        border: '1px solid var(--color-brand-border)',
                        background: 'var(--color-brand-card)',
                        color: 'var(--color-brand-heading)',
                        fontSize: 13
                      }}
                    >
                      <option value={407}>407 Proxy Authentication Required</option>
                      <option value={405}>405 Method Not Allowed</option>
                    </select>
                  </div>

                  <div>
                    <label style={{ display: 'block', fontSize: 11, fontWeight: 600, color: 'var(--color-brand-muted)', marginBottom: 6, textTransform: 'uppercase' }}>Entropy Handshake Prefix</label>
                    <input
                      type="text"
                      value={clientRandomPrefix}
                      onChange={(e) => setClientRandomPrefix(e.target.value)}
                      placeholder="e.g. a0b0/f0f0"
                      style={{
                        width: '100%',
                        padding: '10px 12px',
                        borderRadius: 8,
                        border: '1px solid var(--color-brand-border)',
                        background: 'var(--color-brand-card)',
                        color: 'var(--color-brand-heading)',
                        fontSize: 13,
                        fontFamily: 'Fira Code'
                      }}
                    />
                  </div>

                  <div>
                    <label style={{ display: 'block', fontSize: 11, fontWeight: 600, color: 'var(--color-brand-muted)', marginBottom: 6, textTransform: 'uppercase' }}>TLS Handshake Timeout (seconds)</label>
                    <input
                      type="number"
                      value={tlsHandshakeTimeoutSecs}
                      onChange={(e) => setTlsHandshakeTimeoutSecs(Number(e.target.value))}
                      style={{
                        width: '100%',
                        padding: '10px 12px',
                        borderRadius: 8,
                        border: '1px solid var(--color-brand-border)',
                        background: 'var(--color-brand-card)',
                        color: 'var(--color-brand-heading)',
                        fontSize: 13,
                        fontFamily: 'Fira Code'
                      }}
                    />
                  </div>

                  <div>
                    <label style={{ display: 'block', fontSize: 11, fontWeight: 600, color: 'var(--color-brand-muted)', marginBottom: 6, textTransform: 'uppercase' }}>H2 Stream Initial Window</label>
                    <input
                      type="number"
                      value={h2InitialStreamWindowSize}
                      onChange={(e) => setH2InitialStreamWindowSize(Number(e.target.value))}
                      style={{
                        width: '100%',
                        padding: '10px 12px',
                        borderRadius: 8,
                        border: '1px solid var(--color-brand-border)',
                        background: 'var(--color-brand-card)',
                        color: 'var(--color-brand-heading)',
                        fontSize: 13,
                        fontFamily: 'Fira Code'
                      }}
                    />
                  </div>

                  <div>
                    <label style={{ display: 'block', fontSize: 11, fontWeight: 600, color: 'var(--color-brand-muted)', marginBottom: 6, textTransform: 'uppercase' }}>H2 Conn Initial Window</label>
                    <input
                      type="number"
                      value={h2InitialConnWindowSize}
                      onChange={(e) => setH2InitialConnWindowSize(Number(e.target.value))}
                      style={{
                        width: '100%',
                        padding: '10px 12px',
                        borderRadius: 8,
                        border: '1px solid var(--color-brand-border)',
                        background: 'var(--color-brand-card)',
                        color: 'var(--color-brand-heading)',
                        fontSize: 13,
                        fontFamily: 'Fira Code'
                      }}
                    />
                  </div>
                </div>
              )}
            </div>

            <div style={{ display: 'flex', justifyContent: 'flex-end', marginTop: 8 }}>
              <button type="submit" className="btn btn--primary btn--md" disabled={isLoading}>
                <FiSave style={{ marginRight: 6 }} /> Save Configuration
              </button>
            </div>
          </form>

          {/* User Management Section */}
          <div className="g-card" style={{ padding: 20 }}>
            <div style={{ display: 'flex', alignItems: 'center', gap: 8, marginBottom: 16 }}>
              <FiUsers style={{ color: 'var(--color-brand)', fontSize: 18 }} />
              <span style={{ fontSize: 15, fontWeight: 600, color: 'var(--color-brand-heading)' }}>Proxy Accounts (Clients Auth)</span>
            </div>

            <form onSubmit={handleAddUserSubmit} style={{ display: 'grid', gridTemplateColumns: '1fr 1fr 120px', gap: 12, marginBottom: 18 }}>
              <div>
                <input
                  type="text"
                  placeholder="Username"
                  value={usernameInput}
                  onChange={(e) => setUsernameInput(e.target.value)}
                  style={{
                    width: '100%',
                    padding: '8px 10px',
                    borderRadius: 6,
                    border: '1px solid var(--color-brand-border)',
                    background: 'var(--color-brand-card)',
                    color: 'var(--color-brand-heading)',
                    fontSize: 12
                  }}
                  required
                />
              </div>

              <div>
                <input
                  type="password"
                  placeholder="Password"
                  value={passwordInput}
                  onChange={(e) => setPasswordInput(e.target.value)}
                  style={{
                    width: '100%',
                    padding: '8px 10px',
                    borderRadius: 6,
                    border: '1px solid var(--color-brand-border)',
                    background: 'var(--color-brand-card)',
                    color: 'var(--color-brand-heading)',
                    fontSize: 12
                  }}
                  required
                />
              </div>

              <button type="submit" className="btn btn--sm btn--primary" style={{ height: '100%' }}>
                <FiPlus size={14} style={{ marginRight: 4 }} /> Create User
              </button>
            </form>

            <div className="table-responsive" style={{ border: '1px solid var(--color-brand-border)', borderRadius: 8 }}>
              <table className="table" style={{ margin: 0, fontSize: 12 }}>
                <thead>
                  <tr>
                    <th>Username</th>
                    <th>Status</th>
                    <th>Created At</th>
                    <th style={{ width: 60, textAlign: 'center' }}>Action</th>
                  </tr>
                </thead>
                <tbody>
                  {users.length === 0 ? (
                    <tr>
                      <td colSpan={4} style={{ textAlign: 'center', padding: '16px 8px', color: 'var(--color-brand-muted)' }}>
                        No proxy accounts created. Clients will not be able to authenticate.
                      </td>
                    </tr>
                  ) : (
                    users.map((user) => (
                      <tr key={user.id}>
                        <td style={{ fontWeight: 600, color: 'var(--color-brand-heading)' }}>{user.username}</td>
                        <td>
                          <span style={{
                            padding: '2px 8px',
                            borderRadius: 12,
                            fontSize: 10,
                            fontWeight: 600,
                            background: user.is_active ? '#dcfce7' : '#fee2e2',
                            color: user.is_active ? '#15803d' : '#b91c1c'
                          }}>
                            {user.is_active ? 'Active' : 'Disabled'}
                          </span>
                        </td>
                        <td>{user.created_at || '-'}</td>
                        <td style={{ textAlign: 'center' }}>
                          <button
                            onClick={() => deleteUser(user.id)}
                            style={{
                              background: 'none',
                              border: 'none',
                              color: 'var(--color-brand-muted)',
                              cursor: 'pointer',
                              padding: 4
                            }}
                            onMouseOver={(e) => (e.currentTarget.style.color = '#ef4444')}
                            onMouseOut={(e) => (e.currentTarget.style.color = 'var(--color-brand-muted)')}
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

          {/* Firewall Rules Section */}
          <div className="g-card" style={{ padding: 20 }}>
            <div style={{ display: 'flex', alignItems: 'center', gap: 8, marginBottom: 16 }}>
              <FiCpu style={{ color: 'var(--color-brand)', fontSize: 18 }} />
              <span style={{ fontSize: 15, fontWeight: 600, color: 'var(--color-brand-heading)' }}>Firewall Routing Overrides</span>
            </div>

            <form onSubmit={handleAddRuleSubmit} style={{ display: 'grid', gridTemplateColumns: '1fr 160px 160px 80px', gap: 12, marginBottom: 18 }}>
              <div>
                <input
                  type="text"
                  placeholder="Target IP/CIDR (e.g. 10.0.0.0/8, 185.12.0.0/16)"
                  value={newCidr}
                  onChange={(e) => setNewCidr(e.target.value)}
                  style={{
                    width: '100%',
                    padding: '8px 10px',
                    borderRadius: 6,
                    border: '1px solid var(--color-brand-border)',
                    background: 'var(--color-brand-card)',
                    color: 'var(--color-brand-heading)',
                    fontSize: 12,
                    fontFamily: 'Fira Code'
                  }}
                  required
                />
              </div>

              <div>
                <select
                  value={newStrategy}
                  onChange={(e) => setNewStrategy(e.target.value)}
                  style={{
                    width: '100%',
                    padding: '8px 10px',
                    borderRadius: 6,
                    border: '1px solid var(--color-brand-border)',
                    background: 'var(--color-brand-card)',
                    color: 'var(--color-brand-heading)',
                    fontSize: 12
                  }}
                >
                  <option value="direct-route">Bypass / Direct Route</option>
                  <option value="allow">Allow via Tunnel</option>
                  <option value="deny">Deny Traffic</option>
                </select>
              </div>

              <div>
                <input
                  type="text"
                  placeholder="Description"
                  value={newDesc}
                  onChange={(e) => setNewDesc(e.target.value)}
                  style={{
                    width: '100%',
                    padding: '8px 10px',
                    borderRadius: 6,
                    border: '1px solid var(--color-brand-border)',
                    background: 'var(--color-brand-card)',
                    color: 'var(--color-brand-heading)',
                    fontSize: 12
                  }}
                />
              </div>

              <button type="submit" className="btn btn--sm btn--primary" style={{ height: '100%' }}>
                <FiPlus size={14} style={{ marginRight: 4 }} /> Add
              </button>
            </form>

            <div className="table-responsive" style={{ border: '1px solid var(--color-brand-border)', borderRadius: 8 }}>
              <table className="table" style={{ margin: 0, fontSize: 12 }}>
                <thead>
                  <tr>
                    <th>Target IP Network (CIDR)</th>
                    <th>Strategy</th>
                    <th>Description</th>
                    <th style={{ width: 60, textAlign: 'center' }}>Action</th>
                  </tr>
                </thead>
                <tbody>
                  {rules.length === 0 ? (
                    <tr>
                      <td colSpan={4} style={{ textAlign: 'center', padding: '16px 8px', color: 'var(--color-brand-muted)' }}>
                        No bypass or firewall rules configured.
                      </td>
                    </tr>
                  ) : (
                    rules.map((rule) => (
                      <tr key={rule.id}>
                        <td style={{ fontFamily: 'Fira Code', fontWeight: 500 }}>{rule.target_cidr}</td>
                        <td>
                          <span style={{
                            padding: '2px 8px',
                            borderRadius: 12,
                            fontSize: 10,
                            fontWeight: 600,
                            background: rule.bypass_strategy === 'direct-route' ? 'var(--color-brand-light)' : rule.bypass_strategy === 'allow' ? '#dcfce7' : '#fee2e2',
                            color: rule.bypass_strategy === 'direct-route' ? 'var(--color-brand)' : rule.bypass_strategy === 'allow' ? '#15803d' : '#b91c1c'
                          }}>
                            {rule.bypass_strategy}
                          </span>
                        </td>
                        <td>{rule.description || '-'}</td>
                        <td style={{ textAlign: 'center' }}>
                          <button
                            onClick={() => deleteRule(rule.id)}
                            style={{
                              background: 'none',
                              border: 'none',
                              color: 'var(--color-brand-muted)',
                              cursor: 'pointer',
                              padding: 4
                            }}
                            onMouseOver={(e) => (e.currentTarget.style.color = '#ef4444')}
                            onMouseOut={(e) => (e.currentTarget.style.color = 'var(--color-brand-muted)')}
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

        </div>

        {/* Right Column (Status & Controls & Export) */}
        <div style={{ display: 'flex', flexDirection: 'column', gap: 24 }}>

          {/* Engine Status Card */}
          <div className="g-card" style={{ padding: 20, textAlign: 'center' }}>
            <div style={{ fontSize: 12, fontWeight: 600, color: 'var(--color-brand-muted)', textTransform: 'uppercase', marginBottom: 12 }}>
              TrustTunnel Status
            </div>

            <div style={{
              display: 'inline-flex',
              alignItems: 'center',
              justifyContent: 'center',
              gap: 8,
              padding: '8px 16px',
              borderRadius: 24,
              fontWeight: 700,
              fontSize: 14,
              background: isRunning ? 'var(--color-brand-light)' : '#fee2e2',
              color: isRunning ? 'var(--color-brand)' : '#b91c1c',
              border: isRunning ? '1px solid var(--color-brand-border)' : '1px solid #fca5a5',
              marginBottom: 20
            }}>
              <span style={{
                width: 10,
                height: 10,
                borderRadius: '50%',
                background: isRunning ? 'var(--color-brand)' : '#ef4444',
                boxShadow: isRunning ? '0 0 10px var(--color-brand)' : '0 0 10px #ef4444',
                animation: isRunning ? 'pulse-animation 1.5s infinite' : 'none'
              }}></span>
              {isRunning ? 'DAEMON ACTIVE' : 'DAEMON INACTIVE'}
            </div>

            <div style={{ display: 'flex', flexDirection: 'column', gap: 10 }}>
              {isRunning ? (
                <button className="btn btn--danger btn--md" onClick={stopEngine} disabled={isLoading} style={{ width: '100%' }}>
                  <FiSquare style={{ marginRight: 6 }} /> Stop Server Engine
                </button>
              ) : (
                <button className="btn btn--primary btn--md" onClick={startEngine} disabled={isLoading} style={{ width: '100%' }}>
                  <FiPlay style={{ marginRight: 6 }} /> Start Server Engine
                </button>
              )}

              <button
                type="button"
                onClick={handleToggleAutoStart}
                className={`btn btn--sm ${config?.is_active ? 'btn--primary' : 'btn--outline'}`}
                style={{ width: '100%', marginTop: 6 }}
              >
                Auto-Start on Boot: {config?.is_active ? 'ENABLED' : 'DISABLED'}
              </button>
            </div>
          </div>

          {/* Export Token Card */}
          <div className="g-card" style={{ padding: 20 }}>
            <div style={{ display: 'flex', alignItems: 'center', gap: 8, marginBottom: 12 }}>
              <FiShare2 style={{ color: 'var(--color-brand)', fontSize: 16 }} />
              <span style={{ fontSize: 13, fontWeight: 600, color: 'var(--color-brand-heading)' }}>Export Client Token</span>
            </div>

            <div style={{ display: 'flex', flexDirection: 'column', gap: 10 }}>
              <p style={{ fontSize: 11, color: 'var(--color-brand-muted)', margin: 0, lineHeight: '1.4' }}>
                Generates a secure connection token incorporating current encryption profiles, active presets, TLS parameters, and host IP address.
              </p>
              <button onClick={handleExportToken} className="btn btn--sm btn--primary" style={{ width: '100%' }}>
                Generate Token Link
              </button>

              {exportedToken && (
                <div style={{ marginTop: 10 }}>
                  <textarea
                    readOnly
                    value={exportedToken}
                    style={{
                      width: '100%',
                      height: 80,
                      padding: '8px 10px',
                      borderRadius: 6,
                      border: '1px solid var(--color-brand-border)',
                      background: 'var(--color-brand-card)',
                      color: 'var(--color-brand-heading)',
                      fontSize: 11,
                      resize: 'none',
                      fontFamily: 'Fira Code',
                      marginBottom: 8
                    }}
                  />
                  <button onClick={copyToClipboard} className="btn btn--sm btn--outline" style={{ width: '100%' }}>
                    Copy to Clipboard
                  </button>
                </div>
              )}
            </div>
          </div>

          {/* Server TLS Certificate PEM Override Card */}
          <div className="g-card" style={{ padding: 20 }}>
            <div style={{ display: 'flex', alignItems: 'center', gap: 8, marginBottom: 12 }}>
              <FiKey style={{ color: 'var(--color-brand)', fontSize: 16 }} />
              <span style={{ fontSize: 13, fontWeight: 600, color: 'var(--color-brand-heading)' }}>Server TLS Certificate (PEM)</span>
            </div>

            <div style={{ display: 'flex', flexDirection: 'column', gap: 10 }}>
              <p style={{ fontSize: 11, color: 'var(--color-brand-muted)', margin: 0, lineHeight: '1.4' }}>
                This is the public TLS certificate of this server. Clients can use this PEM block to configure their client-side overrides when verification checks are skipped.
              </p>

              {tlsServerCert ? (
                <div>
                  <textarea
                    readOnly
                    value={tlsServerCert}
                    style={{
                      width: '100%',
                      height: 120,
                      padding: '8px 10px',
                      borderRadius: 6,
                      border: '1px solid var(--color-brand-border)',
                      background: 'var(--color-brand-card)',
                      color: 'var(--color-brand-heading)',
                      fontSize: 10,
                      resize: 'none',
                      fontFamily: 'Fira Code',
                      marginBottom: 8
                    }}
                  />
                  <button onClick={copyCertToClipboard} className="btn btn--sm btn--outline" style={{ width: '100%' }}>
                    {certCopied ? 'Copied!' : 'Copy Certificate to Clipboard'}
                  </button>
                </div>
              ) : (
                <div style={{
                  padding: '12px 14px',
                  borderRadius: 8,
                  background: 'rgba(var(--color-brand-rgb), 0.02)',
                  border: '1px dashed var(--color-brand-border)',
                  fontSize: 11,
                  color: 'var(--color-brand-muted)',
                  textAlign: 'center'
                }}>
                  No certificate loaded. Generate a Let's Encrypt certificate or start the engine to populate the certificate.
                </div>
              )}
            </div>
          </div>

          {/* Active Bind Info */}
          <div className="g-card" style={{ padding: 20 }}>
            <div style={{ display: 'flex', alignItems: 'center', gap: 8, marginBottom: 14 }}>
              <FiGlobe style={{ color: 'var(--color-brand)', fontSize: 16 }} />
              <span style={{ fontSize: 13, fontWeight: 600, color: 'var(--color-brand-heading)' }}>Active Port Mappings</span>
            </div>

            <div style={{ display: 'flex', flexDirection: 'column', gap: 10, fontSize: 12 }}>
              <div style={{ display: 'flex', justifyContent: 'space-between', borderBottom: '1px solid var(--color-brand-border)', paddingBottom: 6 }}>
                <span style={{ color: 'var(--color-brand-muted)' }}>Bind Location:</span>
                <span style={{ fontFamily: 'Fira Code', fontWeight: 600, color: 'var(--color-brand-heading)' }}>{listenAddress}</span>
              </div>
              <div style={{ display: 'flex', justifyContent: 'space-between', borderBottom: '1px solid var(--color-brand-border)', paddingBottom: 6 }}>
                <span style={{ color: 'var(--color-brand-muted)' }}>Transport:</span>
                <span style={{ fontWeight: 600, textTransform: 'uppercase', color: 'var(--color-brand)' }}>{forcedTransport}</span>
              </div>
              <div style={{ display: 'flex', justifyContent: 'space-between', borderBottom: '1px solid var(--color-brand-border)', paddingBottom: 6 }}>
                <span style={{ color: 'var(--color-brand-muted)' }}>Registered Users:</span>
                <span style={{ fontWeight: 600, color: 'var(--color-brand-heading)' }}>{users.length}</span>
              </div>
              <div style={{ display: 'flex', justifyContent: 'space-between' }}>
                <span style={{ color: 'var(--color-brand-muted)' }}>Preset:</span>
                <span style={{ fontWeight: 600, color: 'var(--color-brand-heading)' }}>{activePreset}</span>
              </div>
            </div>
          </div>

          {/* Quick Guide */}
          <div className="g-card" style={{ padding: 20, fontSize: 11, color: 'var(--color-brand-muted)', lineHeight: '1.5' }}>
            <div style={{ display: 'flex', alignItems: 'center', gap: 6, marginBottom: 8, fontWeight: 600, color: 'var(--color-brand-heading)' }}>
              <FiHelpCircle size={14} /> Quick Setup Guide
            </div>
            1. Set the bind interface (e.g. <b>{listenAddress}</b>) and public SNI hostname.<br />
            2. Supply absolute paths to valid TLS certificate and private key files.<br />
            3. Apply a preset like "Iran Stealth" to enforce masking defenses.<br />
            4. Click "Start Server Engine".<br />
            5. Create proxy accounts under "Proxy Accounts" for each client.<br />
            6. Click "Generate Token Link" and share it with your clients so they can import it on their dashboards.
          </div>

        </div>

      </div>
    </div>
  );
};
