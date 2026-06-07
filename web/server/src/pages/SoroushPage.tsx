import React, { useState, useEffect } from 'react';
import { FiPlay, FiSquare, FiSave, FiRefreshCw, FiPlus, FiTrash2, FiCpu, FiShield, FiUsers, FiSettings, FiSend, FiCheck, FiEye, FiEyeOff, FiActivity, FiDownloadCloud, FiCopy } from 'react-icons/fi';
import { showGlobalConfirm } from '../store/dialogStore';

interface SoroushAccount {
  id: number;
  phone_number: string;
  name: string;
  display_name: string;
  role: string;
  status: string;
  is_server_node: boolean;
  soroush_user_id: number;
}

interface TunnelConfig {
  group_chat_id: number;
  group_access_hash: number;
  call_id: number;
  call_access_hash: string;
  server_identity: string;
  psk: string;
  livekit_url: string;
  fallback_livekit_token: string;
  socks_port: number;
  is_active: boolean;
  engine_mode: string;
  max_workers: number;
  load_balance_algo: string;
}

const authHeaders = () => ({
  'Authorization': `Bearer ${localStorage.getItem('cc_server_token') || ''}`,
  'Content-Type': 'application/json',
});

export const SoroushPage: React.FC = () => {
  const [accounts, setAccounts] = useState<SoroushAccount[]>([]);
  const [config, setConfig] = useState<TunnelConfig | null>(null);
  const [isRunning, setIsRunning] = useState(false);
  const [engineStatus, setEngineStatus] = useState<any>(null);
  const [isLoading, setIsLoading] = useState(false);
  const [message, setMessage] = useState<{ type: 'success' | 'error'; text: string } | null>(null);
  const [showPSK, setShowPSK] = useState(false);
  const [showSyncToken, setShowSyncToken] = useState(false);

  // Group Call target settings
  const [groups, setGroups] = useState<any[]>([]);
  const [isFetchingGroups, setIsFetchingGroups] = useState(false);

  // Add account form
  const [newPhone, setNewPhone] = useState('');
  const [newName, setNewName] = useState('');
  const [newRole, setNewRole] = useState('worker');

  // OTP flow
  const [otpAccountId, setOtpAccountId] = useState<number | null>(null);
  const [otpCode, setOtpCode] = useState('');
  const [phoneCodeHash, setPhoneCodeHash] = useState<number[] | null>(null);

  // Config form state
  const [cfgGroupChatId, setCfgGroupChatId] = useState(0);
  const [cfgGroupAccessHash, setCfgGroupAccessHash] = useState(0);
  const [cfgCallId, setCfgCallId] = useState(0);
  const [cfgCallAccessHash, setCfgCallAccessHash] = useState('');
  const [cfgServerIdentity, setCfgServerIdentity] = useState('');
  const [cfgPSK, setCfgPSK] = useState('');
  const [cfgLiveKitURL, setCfgLiveKitURL] = useState('');
  const [cfgFallbackLiveKitToken, setCfgFallbackLiveKitToken] = useState('');
  const [cfgSocksPort, setCfgSocksPort] = useState(4046);
  const [cfgMaxWorkers, setCfgMaxWorkers] = useState(5);
  const [cfgLoadBalanceAlgo, setCfgLoadBalanceAlgo] = useState('least-latency');

  const flash = (type: 'success' | 'error', text: string) => {
    setMessage({ type, text });
    setTimeout(() => setMessage(null), 5000);
  };

  const fetchAccounts = async () => {
    try {
      const r = await fetch('/api/soroush/accounts', { headers: authHeaders() });
      if (r.ok) { const d = await r.json(); setAccounts(d.accounts || []); }
    } catch {}
  };

  const fetchGroups = async () => {
    setIsFetchingGroups(true);
    try {
      const r = await fetch('/api/soroush/groups', { headers: authHeaders() });
      if (r.ok) {
        const d = await r.json();
        setGroups(d || []);
      }
    } catch (err) {
      console.error("Failed to fetch groups", err);
    } finally {
      setIsFetchingGroups(false);
    }
  };

  const fetchConfig = async () => {
    try {
      const r = await fetch('/api/soroush/config', { headers: authHeaders() });
      if (r.ok) {
        const d = await r.json();
        const c = d.config;
        setConfig(c);
        setIsRunning(d.is_running || false);
        if (c) {
          setCfgGroupChatId(c.group_chat_id || 0);
          setCfgGroupAccessHash(c.group_access_hash || 0);
          setCfgCallId(c.call_id || 0);
          setCfgCallAccessHash(c.call_access_hash || '');
          setCfgServerIdentity(c.server_identity || '');
          setCfgPSK(c.psk || '');
          setCfgLiveKitURL(c.livekit_url || '');
          setCfgFallbackLiveKitToken(c.fallback_livekit_token || '');
          setCfgSocksPort(c.socks_port || 4046);
          setCfgMaxWorkers(c.max_workers || 5);
          setCfgLoadBalanceAlgo(c.load_balance_algo || 'least-latency');
        }
      }
    } catch {}
  };

  const fetchEngineStatus = async () => {
    try {
      const r = await fetch('/api/soroush/engine/status', { headers: authHeaders() });
      if (r.ok) { const d = await r.json(); setEngineStatus(d.status); }
    } catch {}
  };

  const refreshAll = async () => {
    setIsLoading(true);
    await fetchAccounts();
    await Promise.all([fetchConfig(), fetchEngineStatus()]);
    setIsLoading(false);
  };

  useEffect(() => {
    refreshAll();
  }, []);

  useEffect(() => {
    if (accounts.some(a => a.status === 'verified')) {
      fetchGroups();
    } else {
      setGroups([]);
    }
  }, [accounts]);

  const addAccount = async (e: React.FormEvent) => {
    e.preventDefault();
    if (!newPhone) return;
    try {
      const r = await fetch('/api/soroush/accounts', {
        method: 'POST', headers: authHeaders(),
        body: JSON.stringify({ phone_number: newPhone, name: newName, role: newRole }),
      });
      if (r.ok) { flash('success', 'Account added'); setNewPhone(''); setNewName(''); fetchAccounts(); }
      else { const d = await r.json(); flash('error', d.error); }
    } catch (e: any) { flash('error', e.message); }
  };

  const deleteAccount = async (id: number) => {
    if (!(await showGlobalConfirm('Delete this account?', { title: 'Delete Account', variant: 'warning' }))) return;
    try {
      const r = await fetch(`/api/soroush/accounts/${id}`, { method: 'DELETE', headers: authHeaders() });
      if (r.ok) { flash('success', 'Account deleted'); fetchAccounts(); }
    } catch {}
  };

  const sendCode = async (id: number) => {
    setIsLoading(true);
    try {
      const r = await fetch(`/api/soroush/accounts/${id}/send-code`, { method: 'POST', headers: authHeaders() });
      const d = await r.json();
      if (r.ok) { setOtpAccountId(id); setPhoneCodeHash(d.phone_code_hash); flash('success', 'Verification code sent to Soroush'); }
      else flash('error', d.error);
    } catch (e: any) { flash('error', e.message); }
    finally { setIsLoading(false); }
  };

  const verifyCode = async () => {
    if (!otpAccountId || !otpCode || !phoneCodeHash) return;
    setIsLoading(true);
    try {
      const r = await fetch(`/api/soroush/accounts/${otpAccountId}/verify`, {
        method: 'POST', headers: authHeaders(),
        body: JSON.stringify({ code: otpCode, phone_code_hash: phoneCodeHash }),
      });
      const d = await r.json();
      if (r.ok) { flash('success', `Verified as ${d.name}`); setOtpAccountId(null); setOtpCode(''); fetchAccounts(); }
      else flash('error', d.error);
    } catch (e: any) { flash('error', e.message); }
    finally { setIsLoading(false); }
  };

  const saveConfig = async (e: React.FormEvent) => {
    e.preventDefault();
    setIsLoading(true);
    try {
      const r = await fetch('/api/soroush/config', {
        method: 'PUT', headers: authHeaders(),
        body: JSON.stringify({
          group_chat_id: cfgGroupChatId,
          group_access_hash: cfgGroupAccessHash,
          call_id: cfgCallId,
          call_access_hash: cfgCallAccessHash,
          server_identity: cfgServerIdentity,
          psk: cfgPSK,
          livekit_url: cfgLiveKitURL,
          fallback_livekit_token: cfgFallbackLiveKitToken,
          socks_port: cfgSocksPort,
          max_workers: cfgMaxWorkers,
          load_balance_algo: cfgLoadBalanceAlgo,
        }),
      });
      if (r.ok) { flash('success', 'Configuration saved'); fetchConfig(); }
      else { const d = await r.json(); flash('error', d.error); }
    } catch (e: any) { flash('error', e.message); }
    finally { setIsLoading(false); }
  };

  const toggleEngine = async () => {
    setIsLoading(true);
    const endpoint = isRunning ? '/api/soroush/engine/stop' : '/api/soroush/engine/start';
    try {
      const r = await fetch(endpoint, { method: 'POST', headers: authHeaders() });
      const d = await r.json();
      if (r.ok) { setIsRunning(d.is_running); flash('success', isRunning ? 'Engine stopped' : 'Engine started'); fetchEngineStatus(); }
      else flash('error', d.error);
    } catch (e: any) { flash('error', e.message); }
    finally { setIsLoading(false); }
  };

  const statusColor = (s: string) => {
    if (s === 'verified' || s === 'tunnel_active') return '#10b981';
    if (s === 'pending_verification') return '#f59e0b';
    if (s === 'error') return '#ef4444';
    return 'var(--color-brand-muted)';
  };

  const labelStyle: React.CSSProperties = { display: 'block', fontSize: 11, fontWeight: 600, color: 'var(--color-brand-muted)', marginBottom: 6, textTransform: 'uppercase' };
  const inputStyle: React.CSSProperties = { width: '100%', padding: '10px 12px', borderRadius: 8, border: '1px solid var(--color-brand-border)', background: 'var(--color-brand-card)', color: 'var(--color-brand-heading)', fontSize: 13 };

  return (
    <div>
      {/* Header */}
      <div style={{ display: 'flex', justifyContent: 'space-between', alignItems: 'center', marginBottom: 24 }}>
        <div>
          <h1 style={{ fontSize: 22, fontWeight: 700, color: 'var(--color-brand-heading)', margin: 0 }}>Soroush P2P Swarm Tunnel</h1>
          <p style={{ fontSize: 12, color: 'var(--color-brand-text)', margin: '4px 0 0' }}>
            Route traffic through an indestructible P2P WebRTC DataChannel swarm. Operates in parallel with Ehco.
          </p>
        </div>
        <button className="btn btn--sm" onClick={refreshAll} disabled={isLoading}>
          <FiRefreshCw className={isLoading ? 'spin-animation' : ''} style={{ marginRight: 6 }} /> Sync
        </button>
      </div>

      {message && (
        <div style={{ padding: '12px 16px', borderRadius: 10, marginBottom: 20, fontSize: 13, fontWeight: 500,
          background: message.type === 'success' ? 'var(--color-brand-light)' : '#fee2e2',
          border: message.type === 'success' ? '1px solid var(--color-brand-border)' : '1px solid #fca5a5',
          color: message.type === 'success' ? 'var(--color-brand)' : '#b91c1c' }}>
          {message.text}
        </div>
      )}

      <div style={{ display: 'grid', gridTemplateColumns: '1fr 340px', gap: 24, alignItems: 'start' }}>
        {/* Left Column */}
        <div style={{ display: 'flex', flexDirection: 'column', gap: 20 }}>

          {/* Server Sync Credentials Card */}
          <div className="g-card" style={{ background: 'linear-gradient(135deg, var(--color-brand-card), var(--color-brand-light))', border: '1px solid var(--color-brand)' }}>
            <div style={{ display: 'flex', alignItems: 'center', gap: 8, marginBottom: 12 }}>
              <FiDownloadCloud style={{ color: 'var(--color-brand)', fontSize: 18 }} />
              <span style={{ fontSize: 15, fontWeight: 600, color: 'var(--color-brand-heading)' }}>Server Sync Credentials</span>
            </div>
            <p style={{ fontSize: 11, color: 'var(--color-brand-text)', lineHeight: 1.5, margin: '0 0 14px' }}>
              Copy these parameters and paste them into your <strong>client panel</strong> during the open internet window to automatically sync the tunnel configuration (Group ID, PSK, LiveKit URL).
            </p>
            <div style={{ display: 'flex', flexDirection: 'column', gap: 10 }}>
              <div>
                <label style={labelStyle}>Server Sync URL</label>
                <div style={{ display: 'flex', gap: 8 }}>
                  <input type="text" readOnly value={window.location.origin} style={{ ...inputStyle, fontFamily: 'Fira Code' }} onClick={(e) => (e.target as HTMLInputElement).select()} />
                  <button type="button" className="btn btn--sm" onClick={() => { navigator.clipboard.writeText(window.location.origin); flash('success', 'URL copied!'); }}><FiCopy /></button>
                </div>
              </div>
              <div>
                <label style={labelStyle}>Auth Token</label>
                <div style={{ display: 'flex', gap: 8 }}>
                  <div style={{ position: 'relative', flex: 1 }}>
                    <input type={showSyncToken ? 'text' : 'password'} readOnly value={localStorage.getItem('cc_server_token') || ''} style={{ ...inputStyle, paddingRight: 40, fontFamily: 'Fira Code' }} onClick={(e) => (e.target as HTMLInputElement).select()} />
                    <button type="button" onClick={() => setShowSyncToken(!showSyncToken)} style={{ position: 'absolute', right: 12, top: '50%', transform: 'translateY(-50%)', background: 'none', border: 'none', cursor: 'pointer', color: 'var(--color-brand-muted)' }}>
                      {showSyncToken ? <FiEyeOff size={16} /> : <FiEye size={16} />}
                    </button>
                  </div>
                  <button type="button" className="btn btn--sm" onClick={() => { navigator.clipboard.writeText(localStorage.getItem('cc_server_token') || ''); flash('success', 'Token copied!'); }}><FiCopy /></button>
                </div>
              </div>
            </div>
          </div>

          {/* Accounts Card */}
          <div className="g-card">
            <div style={{ display: 'flex', alignItems: 'center', justifyContent: 'space-between', marginBottom: 16 }}>
              <div style={{ display: 'flex', alignItems: 'center', gap: 8 }}>
                <FiUsers style={{ color: 'var(--color-brand)', fontSize: 18 }} />
                <span style={{ fontSize: 15, fontWeight: 600, color: 'var(--color-brand-heading)' }}>Soroush Accounts</span>
              </div>
              <span style={{ fontSize: 11, color: 'var(--color-brand-muted)' }}>{accounts.length} registered</span>
            </div>

            {/* Account List */}
            {accounts.length > 0 && (
              <div style={{ display: 'flex', flexDirection: 'column', gap: 8, marginBottom: 16 }}>
                {accounts.map(a => (
                  <div key={a.id} style={{ display: 'flex', alignItems: 'center', justifyContent: 'space-between', padding: '10px 14px', borderRadius: 8, border: '1px solid var(--color-brand-border)', background: 'var(--color-brand-bg)' }}>
                    <div style={{ flex: 1 }}>
                      <div style={{ fontSize: 13, fontWeight: 600, color: 'var(--color-brand-heading)' }}>{a.display_name || a.name || a.phone_number}</div>
                      <div style={{ fontSize: 11, color: 'var(--color-brand-text)', marginTop: 2 }}>{a.phone_number} · {a.role}</div>
                    </div>
                    <div style={{ display: 'flex', alignItems: 'center', gap: 10 }}>
                      <span style={{ fontSize: 10, fontWeight: 700, textTransform: 'uppercase', color: statusColor(a.status), letterSpacing: 0.5 }}>● {a.status}</span>
                      {a.status !== 'verified' && (
                        <button className="btn btn--sm" onClick={() => sendCode(a.id)} disabled={isLoading} title="Send OTP">
                          <FiSend size={12} />
                        </button>
                      )}
                      <button className="btn btn--sm" onClick={() => deleteAccount(a.id)} style={{ color: '#ef4444' }} title="Delete"><FiTrash2 size={12} /></button>
                    </div>
                  </div>
                ))}
              </div>
            )}

            {/* OTP Input */}
            {otpAccountId && (
              <div style={{ display: 'flex', gap: 8, marginBottom: 16, padding: 12, borderRadius: 8, background: 'var(--color-brand-light)', border: '1px solid var(--color-brand-border)' }}>
                <input type="text" placeholder="Enter Soroush OTP code" value={otpCode} onChange={e => setOtpCode(e.target.value)} style={{ ...inputStyle, flex: 1 }} />
                <button className="btn btn--primary" onClick={verifyCode} disabled={isLoading || !otpCode}><FiCheck size={14} /> Verify</button>
              </div>
            )}

            {/* Add Account Form */}
            <form onSubmit={addAccount} style={{ display: 'flex', gap: 8, alignItems: 'end' }}>
              <div style={{ flex: 1 }}>
                <label style={labelStyle}>Phone Number</label>
                <input type="text" placeholder="+98..." value={newPhone} onChange={e => setNewPhone(e.target.value)} style={inputStyle} required />
              </div>
              <div style={{ flex: 1 }}>
                <label style={labelStyle}>Name (optional)</label>
                <input type="text" placeholder="worker-1" value={newName} onChange={e => setNewName(e.target.value)} style={inputStyle} />
              </div>
              <div style={{ width: 120 }}>
                <label style={labelStyle}>Role</label>
                <select value={newRole} onChange={e => setNewRole(e.target.value)} style={inputStyle}>
                  <option value="worker">Worker</option>
                  <option value="host">Host</option>
                </select>
              </div>
              <button type="submit" className="btn btn--primary" style={{ height: 38 }} disabled={isLoading}><FiPlus size={14} /></button>
            </form>
          </div>

          {/* Configuration Card */}
          <form onSubmit={saveConfig}>
            <div className="g-card">
              <div style={{ display: 'flex', alignItems: 'center', gap: 8, marginBottom: 18 }}>
                <FiSettings style={{ color: 'var(--color-brand)', fontSize: 18 }} />
                <span style={{ fontSize: 15, fontWeight: 600, color: 'var(--color-brand-heading)' }}>Tunnel Configuration</span>
              </div>

              <div style={{ display: 'grid', gridTemplateColumns: '1fr 1fr', gap: 16, marginBottom: 16 }}>
                <div style={{ gridColumn: 'span 2' }}>
                  <label style={labelStyle}>Select Target Group (Auto-fills ID & Access Hash)</label>
                  <select
                    value={groups.some(g => g.id === cfgGroupChatId) ? cfgGroupChatId : ''}
                    onChange={e => {
                      const selectedId = Number(e.target.value);
                      const g = groups.find(x => x.id === selectedId);
                      if (g) {
                        setCfgGroupChatId(g.id);
                        setCfgGroupAccessHash(g.accessHash);
                      }
                    }}
                    style={inputStyle}
                    disabled={isFetchingGroups || groups.length === 0}
                  >
                    <option value="">{isFetchingGroups ? 'Fetching groups...' : groups.length === 0 ? 'No groups available (add/verify a Soroush account first)' : 'Select Soroush group...'}</option>
                    {groups.map(g => (
                      <option key={g.id} value={g.id}>{g.title} (ID: {g.id})</option>
                    ))}
                  </select>
                </div>

                <div>
                  <label style={labelStyle}>Group Chat ID</label>
                  <input type="number" value={cfgGroupChatId} onChange={e => setCfgGroupChatId(Number(e.target.value))} style={inputStyle} required />
                  <span style={{ fontSize: 10, color: 'var(--color-brand-text)', marginTop: 4, display: 'block' }}>Target Soroush group for LiveKit room creation.</span>
                </div>
                <div>
                  <label style={labelStyle}>Group Access Hash</label>
                  <input type="number" value={cfgGroupAccessHash} onChange={e => setCfgGroupAccessHash(Number(e.target.value))} style={inputStyle} required />
                </div>
              </div>

              <div style={{ display: 'grid', gridTemplateColumns: '1fr 1fr', gap: 16, marginBottom: 16 }}>
                <div>
                  <label style={labelStyle}>Static Call ID (Bypass)</label>
                  <input type="number" value={cfgCallId || ''} onChange={e => setCfgCallId(Number(e.target.value))} style={inputStyle} />
                  <span style={{ fontSize: 10, color: 'var(--color-brand-text)', marginTop: 4, display: 'block' }}>Optional bypass to target an active meeting.</span>
                </div>
                <div>
                  <label style={labelStyle}>Static Call Access Hash</label>
                  <input type="text" value={cfgCallAccessHash} onChange={e => setCfgCallAccessHash(e.target.value)} style={inputStyle} />
                </div>
              </div>

              <div style={{ display: 'grid', gridTemplateColumns: '1fr 1fr', gap: 16, marginBottom: 16 }}>
                <div>
                  <label style={labelStyle}>Server Soroush ID</label>
                  <input type="text" placeholder="e.g. 64698297" value={cfgServerIdentity} onChange={e => setCfgServerIdentity(e.target.value)} style={inputStyle} required />
                  <span style={{ fontSize: 10, color: 'var(--color-brand-text)', marginTop: 4, display: 'block' }}>The Soroush UserID of the Server Queen.</span>
                </div>
                <div>
                  <label style={labelStyle}>LiveKit URL</label>
                  <input type="text" placeholder="wss://k.splus.ir" value={cfgLiveKitURL} onChange={e => setCfgLiveKitURL(e.target.value)} style={inputStyle} />
                </div>
              </div>

              <div style={{ marginBottom: 16 }}>
                <label style={labelStyle}>Fallback LiveKit Token</label>
                <textarea placeholder="eyJ..." value={cfgFallbackLiveKitToken} onChange={e => setCfgFallbackLiveKitToken(e.target.value)} style={{ ...inputStyle, fontFamily: 'Fira Code', minHeight: 80 }} />
                <span style={{ fontSize: 10, color: 'var(--color-brand-text)', marginTop: 4, display: 'block' }}>Manual fallback LiveKit JWT access token used if dynamic resolution fails.</span>
              </div>

              <div style={{ marginBottom: 16 }}>
                <label style={labelStyle}>Pre-Shared Key (PSK)</label>
                <div style={{ display: 'flex', gap: 8 }}>
                  <div style={{ position: 'relative', flex: 1 }}>
                    <input type={showPSK ? 'text' : 'password'} value={cfgPSK} onChange={e => setCfgPSK(e.target.value)} style={{ ...inputStyle, paddingRight: 40, fontFamily: 'Fira Code' }} required />
                    <button type="button" onClick={() => setShowPSK(!showPSK)} style={{ position: 'absolute', right: 12, top: '50%', transform: 'translateY(-50%)', background: 'none', border: 'none', cursor: 'pointer', color: 'var(--color-brand-muted)' }}>
                      {showPSK ? <FiEyeOff size={16} /> : <FiEye size={16} />}
                    </button>
                  </div>
                </div>
                <span style={{ fontSize: 10, color: 'var(--color-brand-text)', marginTop: 4, display: 'block' }}>In-band DataChannel authentication key. Must match on both server and client.</span>
              </div>

              <div style={{ display: 'grid', gridTemplateColumns: '1fr 1fr 1fr', gap: 16, marginBottom: 16 }}>
                <div>
                  <label style={labelStyle}>SOCKS5 Port</label>
                  <input type="number" value={cfgSocksPort} onChange={e => setCfgSocksPort(Number(e.target.value))} style={inputStyle} required />
                </div>
                <div>
                  <label style={labelStyle}>Max Workers</label>
                  <input type="number" value={cfgMaxWorkers} onChange={e => setCfgMaxWorkers(Number(e.target.value))} style={inputStyle} required />
                </div>
                <div>
                  <label style={labelStyle}>Load Balance</label>
                  <select value={cfgLoadBalanceAlgo} onChange={e => setCfgLoadBalanceAlgo(e.target.value)} style={inputStyle}>
                    <option value="least-latency">Least Latency</option>
                    <option value="round-robin">Round Robin</option>
                  </select>
                </div>
              </div>

              <button type="submit" className="btn btn--primary" style={{ display: 'flex', alignItems: 'center', gap: 6 }} disabled={isLoading}>
                <FiSave /> Save Configuration
              </button>
            </div>
          </form>
        </div>

        {/* Right Column: Engine Status */}
        <div style={{ display: 'flex', flexDirection: 'column', gap: 20 }}>
          {/* Engine Widget */}
          <div className="g-card" style={{ display: 'flex', flexDirection: 'column', alignItems: 'center', textAlign: 'center', padding: '24px 16px' }}>
            <div style={{ width: 54, height: 54, borderRadius: '50%', background: isRunning ? 'var(--color-brand-light)' : 'var(--color-brand-bg)', display: 'flex', alignItems: 'center', justifyContent: 'center', color: isRunning ? 'var(--color-brand)' : 'var(--color-brand-muted)', marginBottom: 14, border: '1px solid var(--color-brand-border)' }}>
              <FiCpu size={24} />
            </div>
            <div style={{ fontSize: 12, fontWeight: 700, color: 'var(--color-brand-muted)', textTransform: 'uppercase', letterSpacing: 1 }}>Hive Swarm Engine</div>
            <div style={{ display: 'flex', alignItems: 'center', gap: 6, marginTop: 8, marginBottom: 14 }}>
              <span className="live-dot" style={{ background: isRunning ? '#10b981' : '#ef4444' }} />
              <span style={{ fontSize: 16, fontWeight: 700, color: 'var(--color-brand-heading)' }}>{isRunning ? 'RUNNING' : 'STOPPED'}</span>
            </div>
            <p style={{ fontSize: 11, color: 'var(--color-brand-text)', lineHeight: 1.4, margin: '0 0 16px' }}>
              {isRunning ? 'P2P WebRTC swarm is active. Traffic is flowing through direct peer-to-peer DataChannels.' : 'Engine is offline. Start to route traffic through Soroush WebRTC swarm infrastructure.'}
            </p>
            <button type="button" onClick={toggleEngine} disabled={isLoading}
              style={{ width: '100%', display: 'flex', alignItems: 'center', justifyContent: 'center', gap: 8, background: isRunning ? '#ef4444' : 'var(--color-brand)', color: '#fff', border: 'none', padding: '10px 16px', borderRadius: 8, fontWeight: 600, fontSize: 13, cursor: 'pointer' }}>
              {isRunning ? <><FiSquare /> Stop Engine</> : <><FiPlay /> Start Engine</>}
            </button>
          </div>

          {/* Live Stats */}
          <div className="g-card">
            <div style={{ display: 'flex', alignItems: 'center', gap: 6, marginBottom: 12 }}>
              <FiActivity style={{ color: 'var(--color-brand)', fontSize: 16 }} />
              <span style={{ fontSize: 13, fontWeight: 600, color: 'var(--color-brand-heading)' }}>Live Statistics</span>
            </div>
            <div style={{ display: 'flex', flexDirection: 'column', gap: 10, fontSize: 12 }}>
              {[
                ['Uptime', engineStatus?.uptime || '—'],
                ['Total Swarm Connections', engineStatus?.total_streams ?? '—'],
                ['Bytes Relayed', engineStatus?.bytes_relayed ? `${(engineStatus.bytes_relayed / 1024 / 1024).toFixed(1)} MB` : '—'],
                ['Verified Accounts', accounts.filter(a => a.status === 'verified').length],
              ].map(([label, val]) => (
                <div key={String(label)} style={{ display: 'flex', justifyContent: 'space-between' }}>
                  <span style={{ color: 'var(--color-brand-muted)' }}>{label}</span>
                  <span style={{ fontWeight: 600, color: 'var(--color-brand-heading)', fontFamily: 'Fira Code' }}>{String(val)}</span>
                </div>
              ))}
            </div>
          </div>

          {/* Security Info */}
          <div className="g-card">
            <div style={{ display: 'flex', alignItems: 'center', gap: 6, marginBottom: 12 }}>
              <FiShield style={{ color: 'var(--color-brand)', fontSize: 16 }} />
              <span style={{ fontSize: 13, fontWeight: 600, color: 'var(--color-brand-heading)' }}>Security Profile</span>
            </div>
            <div style={{ display: 'flex', flexDirection: 'column', gap: 8, fontSize: 11 }}>
              <div><span style={{ color: 'var(--color-brand-muted)' }}>ICE Policy:</span> <strong style={{ color: 'var(--color-brand-heading)' }}>host, srflx, relay</strong></div>
              <div><span style={{ color: 'var(--color-brand-muted)' }}>Signaling:</span> <strong style={{ color: 'var(--color-brand-heading)' }}>LiveKit SFU Room</strong></div>
              <div><span style={{ color: 'var(--color-brand-muted)' }}>Encryption:</span> <strong style={{ color: 'var(--color-brand-heading)' }}>Zero-Trust DTLS-SRTP</strong></div>
              <div><span style={{ color: 'var(--color-brand-muted)' }}>Protocol:</span> <strong style={{ color: 'var(--color-brand-heading)' }}>SCTP over WebRTC</strong></div>
            </div>
          </div>
        </div>
      </div>
    </div>
  );
};
