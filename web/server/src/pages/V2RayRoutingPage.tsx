import React, { useState, useEffect } from 'react';
import { FiGlobe, FiPlus, FiTrash2, FiSave, FiRefreshCw, FiHelpCircle, FiCheck, FiX, FiShield, FiAlertTriangle } from 'react-icons/fi';

export const V2RayRoutingPage: React.FC = () => {
  const [helpTitle, setHelpTitle] = useState<string | null>(null);
  const [helpText, setHelpText] = useState<string | null>(null);

  const showHelp = (title: string, text: string) => {
    setHelpTitle(title);
    setHelpText(text);
  };

  const [isLoading, setIsLoading] = useState(false);
  const [message, setMessage] = useState<{ type: 'success' | 'error', text: string } | null>(null);

  // Lists
  const [rules, setRules] = useState<any[]>([]);
  const [bannedIPs, setBannedIPs] = useState<any[]>([]);

  // Form: Routing Rule
  const [ruleTag, setRuleTag] = useState('');
  const [outboundTag, setOutboundTag] = useState('proxy');
  const [ruleDomains, setRuleDomains] = useState('');
  const [ruleIPs, setRuleIPs] = useState('');

  // Form: Firewall Block
  const [blockIP, setBlockIP] = useState('');

  const loadData = async () => {
    setIsLoading(true);
    try {
      const token = localStorage.getItem('cc_server_token') || localStorage.getItem('cc_client_token') || '';
      
      // Fetch server routing rules
      const rResp = await fetch('/api/v2ray/routing', {
        headers: { 'Authorization': `Bearer ${token}` }
      });
      if (rResp.ok) setRules(await rResp.json() || []);

      // Fetch firewall block logs or settings (Fail2ban simulation)
      const sResp = await fetch('/api/v2ray/client/settings', {
        headers: { 'Authorization': `Bearer ${token}` }
      });
      if (sResp.ok) {
        const data = await sResp.json();
        if (data.firewall_blocked_ips) {
          setBannedIPs(data.firewall_blocked_ips.split(',').filter(Boolean));
        }
      }
    } catch (err) {
      console.error(err);
    } finally {
      setIsLoading(false);
    }
  };

  const handleAddRule = async (e: React.FormEvent) => {
    e.preventDefault();
    setIsLoading(true);
    setMessage(null);
    try {
      const token = localStorage.getItem('cc_server_token') || localStorage.getItem('cc_client_token') || '';
      const domainsArr = ruleDomains.split(',').map(d => d.trim()).filter(Boolean);
      const ipsArr = ruleIPs.split(',').map(ip => ip.trim()).filter(Boolean);

      const res = await fetch('/api/v2ray/routing', {
        method: 'POST',
        headers: {
          'Content-Type': 'application/json',
          'Authorization': `Bearer ${token}`
        },
        body: JSON.stringify({
          tag: ruleTag,
          outbound_tag: outboundTag,
          domains: domainsArr,
          ips: ipsArr
        })
      });

      if (res.ok) {
        setRuleTag('');
        setRuleDomains('');
        setRuleIPs('');
        setMessage({ type: 'success', text: 'Server routing rule deployed successfully!' });
        loadData();
      } else {
        const errData = await res.json();
        setMessage({ type: 'error', text: errData.error || 'Failed to create routing rule.' });
      }
    } catch (err: any) {
      setMessage({ type: 'error', text: err.message });
    } finally {
      setIsLoading(false);
    }
  };

  const handleDeleteRule = async (id: number) => {
    if (!window.confirm('Delete this routing rule?')) return;
    setIsLoading(true);
    try {
      const token = localStorage.getItem('cc_server_token') || localStorage.getItem('cc_client_token') || '';
      const res = await fetch(`/api/v2ray/routing/${id}`, {
        method: 'DELETE',
        headers: { 'Authorization': `Bearer ${token}` }
      });
      if (res.ok) {
        setMessage({ type: 'success', text: 'Routing rule removed.' });
        loadData();
      }
    } catch (err) {
      console.error(err);
    } finally {
      setIsLoading(false);
    }
  };

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
        const updatedBans = [...bannedIPs, blockIP];
        setBannedIPs(updatedBans);
        
        // Save to flat settings to persist list
        await fetch('/api/v2ray/client/settings', {
          method: 'POST',
          headers: {
            'Content-Type': 'application/json',
            'Authorization': `Bearer ${token}`
          },
          body: JSON.stringify({
            firewall_blocked_ips: updatedBans.join(',')
          })
        });

        setBlockIP('');
        setMessage({ type: 'success', text: `IP address ${blockIP} banned in firewall kernel.` });
      }
    } catch (err: any) {
      setMessage({ type: 'error', text: err.message });
    } finally {
      setIsLoading(false);
    }
  };

  const handleUnblockIP = async (ip: string) => {
    const token = localStorage.getItem('cc_server_token') || localStorage.getItem('cc_client_token') || '';
    const updated = bannedIPs.filter(b => b !== ip);
    setBannedIPs(updated);
    try {
      await fetch('/api/v2ray/client/settings', {
        method: 'POST',
        headers: {
          'Content-Type': 'application/json',
          'Authorization': `Bearer ${token}`
        },
        body: JSON.stringify({
          firewall_blocked_ips: updated.join(',')
        })
      });
      setMessage({ type: 'success', text: `IP address ${ip} unblocked.` });
    } catch (err) {
      console.error(err);
    }
  };

  useEffect(() => {
    loadData();
  }, []);

  return (
    <div>
      <div style={{ display: 'flex', justifyContent: 'space-between', alignItems: 'center', marginBottom: 24 }}>
        <div>
          <h1 style={{ fontSize: 22, fontWeight: 700, color: 'var(--color-brand-heading)', margin: 0 }}>V2Ray Server Routing & Firewall</h1>
          <p style={{ fontSize: 12, color: 'var(--color-brand-text)', margin: '4px 0 0' }}>
            Orchestrate server-side routing rule arrays and manage kernel-level Fail2ban firewall blocks.
          </p>
        </div>
        <button className="btn btn--sm" onClick={loadData} disabled={isLoading}>
          <FiRefreshCw className={isLoading ? 'spin-animation' : ''} style={{ marginRight: 6 }} /> Sync Routing
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

      <div style={{ display: 'grid', gridTemplateColumns: '1fr 340px', gap: 24, alignItems: 'start' }}>
        
        {/* Left Side: Server Routing Rules List & Add Form */}
        <div style={{ display: 'flex', flexDirection: 'column', gap: 24 }}>
          
          {/* Rules list */}
          <div className="g-card">
            <div style={{ display: 'flex', alignItems: 'center', gap: 8, marginBottom: 16 }}>
              <FiGlobe style={{ color: 'var(--color-brand)', fontSize: 18 }} />
              <span style={{ fontSize: 15, fontWeight: 600, color: 'var(--color-brand-heading)' }}>Server-Side Routing Rules</span>
              <FiHelpCircle 
                style={{ cursor: 'pointer', color: 'var(--color-brand-muted)' }} 
                onClick={() => showHelp('Server Routing Rules', 'Manages server-side traffic intercept. Allows dropping, proxying, or routing connections to specific destinations based on domain names or IP block patterns.')}
              />
            </div>

            <div style={{ overflowX: 'auto', border: '1px solid var(--color-brand-border)', borderRadius: 8 }}>
              <table style={{ width: '100%', borderCollapse: 'collapse', fontSize: 11, textAlign: 'left' }}>
                <thead>
                  <tr style={{ background: 'var(--color-brand-bg)', borderBottom: '1px solid var(--color-brand-border)' }}>
                    <th style={{ padding: '8px 10px', color: 'var(--color-brand-heading)' }}>Tag</th>
                    <th style={{ padding: '8px 10px', color: 'var(--color-brand-heading)' }}>Outbound Tag</th>
                    <th style={{ padding: '8px 10px', color: 'var(--color-brand-heading)' }}>Domains</th>
                    <th style={{ padding: '8px 10px', color: 'var(--color-brand-heading)' }}>IPs</th>
                    <th style={{ padding: '8px 10px', color: 'var(--color-brand-heading)', textAlign: 'center' }}>Remove</th>
                  </tr>
                </thead>
                <tbody>
                  {rules.length === 0 ? (
                    <tr>
                      <td colSpan={5} style={{ padding: 14, textAlign: 'center', color: 'var(--color-brand-muted)' }}>No routing rules active on server.</td>
                    </tr>
                  ) : (
                    rules.map((r) => (
                      <tr key={r.ID} style={{ borderBottom: '1px solid var(--color-brand-border)' }}>
                        <td style={{ padding: '8px 10px', fontWeight: 600, color: 'var(--color-brand-heading)' }}>{r.tag}</td>
                        <td style={{ padding: '8px 10px', textTransform: 'uppercase', color: 'var(--color-brand)' }}>{r.outbound_tag}</td>
                        <td style={{ padding: '8px 10px', color: 'var(--color-brand-text)', maxWidth: 120, overflow: 'hidden', textOverflow: 'ellipsis', whiteSpace: 'nowrap' }}>
                          {r.domains ? JSON.parse(r.domains).join(', ') : 'None'}
                        </td>
                        <td style={{ padding: '8px 10px', color: 'var(--color-brand-text)', maxWidth: 120, overflow: 'hidden', textOverflow: 'ellipsis', whiteSpace: 'nowrap' }}>
                          {r.ips ? JSON.parse(r.ips).join(', ') : 'None'}
                        </td>
                        <td style={{ padding: '8px 10px', textAlign: 'center' }}>
                          <button onClick={() => handleDeleteRule(r.ID)} style={{ background: 'none', border: 'none', cursor: 'pointer', color: 'var(--color-brand-red)' }}>
                            <FiTrash2 size={13} />
                          </button>
                        </td>
                      </tr>
                    ))
                  )}
                </tbody>
              </table>
            </div>
          </div>

          {/* Add form */}
          <form onSubmit={handleAddRule} className="g-card" style={{ display: 'flex', flexDirection: 'column', gap: 14 }}>
            <span style={{ fontSize: 13, fontWeight: 700, color: 'var(--color-brand-heading)' }}>Create Server Routing Rule</span>
            
            <div style={{ display: 'grid', gridTemplateColumns: '1fr 1fr', gap: 12 }}>
              <div>
                <label style={{ display: 'block', fontSize: 10, fontWeight: 600, color: 'var(--color-brand-muted)', marginBottom: 4 }}>RULE TAG</label>
                <input
                  type="text"
                  placeholder="e.g. block-domestic"
                  value={ruleTag}
                  onChange={(e) => setRuleTag(e.target.value)}
                  style={{
                    width: '100%',
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

              <div>
                <label style={{ display: 'block', fontSize: 10, fontWeight: 600, color: 'var(--color-brand-muted)', marginBottom: 4 }}>OUTBOUND TAG</label>
                <select
                  value={outboundTag}
                  onChange={(e) => setOutboundTag(e.target.value)}
                  style={{
                    width: '100%',
                    padding: '8px 10px',
                    borderRadius: 6,
                    border: '1px solid var(--color-brand-border)',
                    background: 'var(--color-brand-card)',
                    fontSize: 12,
                    color: 'var(--color-brand-heading)'
                  }}
                >
                  <option value="proxy">Proxy (Route via outbound server)</option>
                  <option value="direct">Direct (Freedom routing bypass)</option>
                  <option value="block">Block (Drop packets immediately)</option>
                </select>
              </div>
            </div>

            <div>
              <label style={{ display: 'block', fontSize: 10, fontWeight: 600, color: 'var(--color-brand-muted)', marginBottom: 4 }}>TARGET DOMAINS (Comma separated)</label>
              <input
                type="text"
                placeholder="e.g. domain:ir, geosite:cn"
                value={ruleDomains}
                onChange={(e) => setRuleDomains(e.target.value)}
                style={{
                  width: '100%',
                  padding: '8px 10px',
                  borderRadius: 6,
                  border: '1px solid var(--color-brand-border)',
                  background: 'var(--color-brand-card)',
                  fontSize: 12,
                  color: 'var(--color-brand-heading)'
                }}
              />
            </div>

            <div>
              <label style={{ display: 'block', fontSize: 10, fontWeight: 600, color: 'var(--color-brand-muted)', marginBottom: 4 }}>TARGET IPS (Comma separated)</label>
              <input
                type="text"
                placeholder="e.g. geoip:ir, 1.1.1.1/32"
                value={ruleIPs}
                onChange={(e) => setRuleIPs(e.target.value)}
                style={{
                  width: '100%',
                  padding: '8px 10px',
                  borderRadius: 6,
                  border: '1px solid var(--color-brand-border)',
                  background: 'var(--color-brand-card)',
                  fontSize: 12,
                  color: 'var(--color-brand-heading)'
                }}
              />
            </div>

            <button type="submit" className="btn btn--primary" style={{ display: 'flex', alignItems: 'center', justifyContent: 'center', gap: 6, marginTop: 4 }}>
              <FiPlus /> Deploy Routing Rule
            </button>
          </form>

        </div>

        {/* Right Side: Fail2ban firewall blocks */}
        <div style={{ display: 'flex', flexDirection: 'column', gap: 24 }}>
          
          <div className="g-card" style={{ display: 'flex', flexDirection: 'column', gap: 14 }}>
            <div style={{ display: 'flex', alignItems: 'center', gap: 8 }}>
              <FiShield style={{ color: 'var(--color-brand)', fontSize: 18 }} />
              <span style={{ fontSize: 15, fontWeight: 600, color: 'var(--color-brand-heading)' }}>Fail2ban IPS Firewalls</span>
              <FiHelpCircle 
                style={{ cursor: 'pointer', color: 'var(--color-brand-muted)' }} 
                onClick={() => showHelp('Fail2ban Firewall', 'Intercepts scan packets using automated system rules. Allows manual listing and dropping of malicious IP blocks directly via kernel-level netfilter rules.')}
              />
            </div>

            <form onSubmit={handleBlockIP} style={{ display: 'flex', gap: 10 }}>
              <input
                type="text"
                placeholder="Manual Block IP"
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
                required
              />
              <button type="submit" className="btn btn--primary btn--sm">Block</button>
            </form>

            <div style={{ borderTop: '1px solid var(--color-brand-border)', paddingTop: 14 }}>
              <span style={{ fontSize: 11, fontWeight: 700, color: 'var(--color-brand-heading)', display: 'block', marginBottom: 8 }}>Banned IP List</span>
              {bannedIPs.length === 0 ? (
                <div style={{ fontSize: 11, color: 'var(--color-brand-muted)', padding: '10px 0', textAlign: 'center' }}>No IP addresses currently blocked.</div>
              ) : (
                <div style={{ display: 'flex', flexDirection: 'column', gap: 6 }}>
                  {bannedIPs.map((ip, idx) => (
                    <div key={idx} style={{ display: 'flex', justifyContent: 'space-between', alignItems: 'center', background: 'var(--color-brand-bg)', padding: '6px 10px', borderRadius: 6, border: '1px solid var(--color-brand-border)' }}>
                      <span style={{ fontFamily: 'Fira Code', fontSize: 11, color: 'var(--color-brand-red)' }}>{ip}</span>
                      <button onClick={() => handleUnblockIP(ip)} style={{ background: 'none', border: 'none', cursor: 'pointer', color: 'var(--color-brand-text)', fontSize: 11 }}>Unban</button>
                    </div>
                  ))}
                </div>
              )}
            </div>
          </div>

        </div>

      </div>

      {/* Help Modal */}
      {helpTitle && (
        <div style={{
          position: 'fixed',
          top: 0, left: 0, width: '100%', height: '100%',
          background: 'rgba(0,0,0,0.5)',
          display: 'flex', alignItems: 'center', justifyContent: 'center', zIndex: 9999
        }}>
          <div style={{
            background: 'var(--color-brand-card)',
            padding: 24, borderRadius: 12, width: 400, maxWidth: '90%',
            boxShadow: '0 10px 25px rgba(0,0,0,0.1)'
          }}>
            <div style={{ display: 'flex', justifyContent: 'space-between', alignItems: 'center', marginBottom: 14, borderBottom: '1px solid var(--color-brand-border)', paddingBottom: 10 }}>
              <h3 style={{ margin: 0, fontSize: 16, fontWeight: 700, color: 'var(--color-brand-heading)' }}>{helpTitle}</h3>
              <button onClick={() => { setHelpTitle(null); setHelpText(null); }} style={{ background: 'none', border: 'none', cursor: 'pointer', color: 'var(--color-brand-muted)', display: 'flex', alignItems: 'center' }}>
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
