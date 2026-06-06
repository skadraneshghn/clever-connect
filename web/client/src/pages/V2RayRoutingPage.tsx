import React, { useState, useEffect } from 'react';
import { FiGlobe, FiSliders, FiPlus, FiTrash2, FiSave, FiRefreshCw, FiHelpCircle, FiCheck, FiX, FiList, FiArrowUp, FiArrowDown } from 'react-icons/fi';

export const V2RayRoutingPage: React.FC = () => {
  const [helpTitle, setHelpTitle] = useState<string | null>(null);
  const [helpText, setHelpText] = useState<string | null>(null);

  const showHelp = (title: string, text: string) => {
    setHelpTitle(title);
    setHelpText(text);
  };

  const [isLoading, setIsLoading] = useState(false);
  const [message, setMessage] = useState<{ type: 'success' | 'error', text: string } | null>(null);

  // Settings
  const [routingPreset, setRoutingPreset] = useState('bypass_domestic');
  const [customRoutingRules, setCustomRoutingRules] = useState('');
  
  // GUI rules state
  const [guiRules, setGuiRules] = useState<any[]>([
    { outboundTag: 'direct', domain: 'geosite:ir', ip: 'geoip:ir', port: '', network: 'any' }
  ]);

  // Fronting Maps state
  const [frontingMaps, setFrontingMaps] = useState<any[]>([]);
  const [targetDomain, setTargetDomain] = useState('');
  const [frontDomain, setFrontDomain] = useState('');
  const [cdnIP, setCdnIP] = useState('');
  const [frontPort, setFrontPort] = useState(443);
  const [frontSNI, setFrontSNI] = useState('');
  const [frontAlpn, setFrontAlpn] = useState('h2,http/1.1');

  // Convert GUI rules array to custom_routing string
  const compileGuiRulesToJSON = (rulesList: any[]) => {
    const parsedRules = rulesList.map(r => {
      const ruleObj: any = {
        type: "field",
        outboundTag: r.outboundTag || 'proxy'
      };
      if (r.domain && r.domain.trim()) {
        ruleObj.domain = r.domain.split(',').map((d: string) => d.trim()).filter(Boolean);
      }
      if (r.ip && r.ip.trim()) {
        ruleObj.ip = r.ip.split(',').map((ip: string) => ip.trim()).filter(Boolean);
      }
      if (r.port && r.port.trim()) {
        ruleObj.port = r.port.trim();
      }
      if (r.network && r.network !== 'any') {
        ruleObj.network = r.network;
      }
      return ruleObj;
    });
    const jsonStr = JSON.stringify(parsedRules, null, 2);
    setCustomRoutingRules(jsonStr);
    return jsonStr;
  };

  // Convert custom_routing string back to GUI rules array
  const parseJSONToGuiRules = (jsonStr: string) => {
    try {
      const parsed = JSON.parse(jsonStr);
      if (Array.isArray(parsed)) {
        setGuiRules(parsed.map(r => ({
          outboundTag: r.outboundTag || 'proxy',
          domain: Array.isArray(r.domain) ? r.domain.join(', ') : '',
          ip: Array.isArray(r.ip) ? r.ip.join(', ') : '',
          port: r.port || '',
          network: r.network || 'any'
        })));
      }
    } catch (e) {
      console.warn("JSON Parse warn:", e);
    }
  };

  const loadSettings = async () => {
    setIsLoading(true);
    try {
      const token = localStorage.getItem('cc_client_token') || '';
      const response = await fetch('/api/v2ray/client/settings', {
        headers: { 'Authorization': `Bearer ${token}` }
      });
      if (response.ok) {
        const data = await response.json();
        setRoutingPreset(data.routing_preset || 'bypass_domestic');
        setCustomRoutingRules(data.custom_routing || '');
        if (data.custom_routing) {
          parseJSONToGuiRules(data.custom_routing);
        }
        if (data.fronting_maps) {
          try {
            setFrontingMaps(JSON.parse(data.fronting_maps));
          } catch (e) {
            setFrontingMaps([]);
          }
        }
      }
    } catch (err) {
      console.error(err);
    } finally {
      setIsLoading(false);
    }
  };

  const handleSaveRouting = async (e: React.FormEvent) => {
    e.preventDefault();
    setIsLoading(true);
    setMessage(null);
    try {
      let finalCustomRules = customRoutingRules;
      if (routingPreset === 'custom') {
        finalCustomRules = compileGuiRulesToJSON(guiRules);
      }

      if (routingPreset === 'custom' && finalCustomRules) {
        try {
          JSON.parse(finalCustomRules);
        } catch (err: any) {
          setMessage({ type: 'error', text: `Invalid Custom Routing Rules JSON: ${err.message}` });
          setIsLoading(false);
          return;
        }
      }

      const token = localStorage.getItem('cc_client_token') || '';
      const res = await fetch('/api/v2ray/client/settings', {
        method: 'POST',
        headers: {
          'Content-Type': 'application/json',
          'Authorization': `Bearer ${token}`
        },
        body: JSON.stringify({
          routing_preset: routingPreset,
          custom_routing: finalCustomRules,
          fronting_maps: JSON.stringify(frontingMaps)
        })
      });
      if (res.ok) {
        setMessage({ type: 'success', text: 'Client routing settings updated successfully!' });
      } else {
        const errData = await res.json();
        setMessage({ type: 'error', text: errData.error || 'Failed to update routing settings.' });
      }
    } catch (err: any) {
      setMessage({ type: 'error', text: err.message });
    } finally {
      setIsLoading(false);
    }
  };

  const handleAddFrontingMap = (e: React.FormEvent) => {
    e.preventDefault();
    if (!targetDomain || !frontDomain || !cdnIP) return;
    const newMap = {
      target: targetDomain,
      front: frontDomain,
      cdn: cdnIP,
      port: frontPort,
      sni: frontSNI || frontDomain,
      alpn: frontAlpn
    };
    const updated = [...frontingMaps, newMap];
    setFrontingMaps(updated);
    setTargetDomain('');
    setFrontDomain('');
    setCdnIP('');
    setFrontSNI('');
  };

  const handleDeleteFrontingMap = (index: number) => {
    const updated = frontingMaps.filter((_, i) => i !== index);
    setFrontingMaps(updated);
  };

  useEffect(() => {
    loadSettings();
  }, []);

  return (
    <div>
      <div style={{ display: 'flex', justifyContent: 'space-between', alignItems: 'center', marginBottom: 24 }}>
        <div>
          <h1 style={{ fontSize: 22, fontWeight: 700, color: 'var(--color-brand-heading)', margin: 0 }}>V2Ray Client Routing & Domain Fronting</h1>
          <p style={{ fontSize: 12, color: 'var(--color-brand-text)', margin: '4px 0 0' }}>
            Configure client-side request redirection paths, domain fronting maps, and custom bypass lists.
          </p>
        </div>
        <button className="btn btn--sm" onClick={loadSettings} disabled={isLoading}>
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

      <div style={{ display: 'grid', gridTemplateColumns: '1.2fr 1fr', gap: 24, alignItems: 'start' }}>
        
        {/* Left Side: Routing Presets */}
        <form onSubmit={handleSaveRouting} className="g-card" style={{ display: 'flex', flexDirection: 'column', gap: 18 }}>
          <div style={{ display: 'flex', alignItems: 'center', gap: 8 }}>
            <FiList style={{ color: 'var(--color-brand)', fontSize: 18 }} />
            <span style={{ fontSize: 15, fontWeight: 600, color: 'var(--color-brand-heading)' }}>Client Routing Presets</span>
            <FiHelpCircle 
              style={{ cursor: 'pointer', color: 'var(--color-brand-muted)' }} 
              onClick={() => showHelp('Routing Presets', 'Define the client-side proxy rules. Bypass Domestic routes IR-specific IP ranges and sites direct to save tunnel resources. Block Ads drops tracking requests.')}
            />
          </div>

          <div>
            <label style={{ display: 'block', fontSize: 11, fontWeight: 600, color: 'var(--color-brand-muted)', marginBottom: 6, textTransform: 'uppercase' }}>Active Preset</label>
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
              <option value="global">Global (All requests route via Proxy)</option>
              <option value="bypass_domestic">Bypass Iran (IR hosts go Direct)</option>
              <option value="block_ads">Block Ads (Drop ad/tracking scripts)</option>
              <option value="custom">Custom GUI Builder & JSON rules</option>
            </select>
          </div>

          {routingPreset === 'custom' && (
            <div style={{ display: 'flex', flexDirection: 'column', gap: 12 }}>
              <div style={{ display: 'flex', alignItems: 'center', justifyContent: 'space-between' }}>
                <span style={{ fontSize: 13, fontWeight: 700, color: 'var(--color-brand-heading)' }}>GUI Custom Rules Compiler</span>
                <button
                  type="button"
                  className="btn btn--xs btn--secondary"
                  onClick={() => {
                    const copy = [...guiRules, { outboundTag: 'proxy', domain: '', ip: '', port: '', network: 'any' }];
                    setGuiRules(copy);
                    compileGuiRulesToJSON(copy);
                  }}
                >
                  + Add Rule Field
                </button>
              </div>

              {guiRules.map((rule, idx) => (
                <div key={idx} style={{ padding: 12, background: 'var(--color-brand-bg)', borderRadius: 8, border: '1px solid var(--color-brand-border)', display: 'flex', flexDirection: 'column', gap: 8 }}>
                  <div style={{ display: 'flex', justifyItems: 'center', justifyContent: 'space-between', alignItems: 'center' }}>
                    <span style={{ fontSize: 11, fontWeight: 700, color: 'var(--color-brand-heading)' }}>Rule #{idx + 1}</span>
                    <div style={{ display: 'flex', gap: 4 }}>
                      <button
                        type="button"
                        className="btn btn--xs btn--secondary"
                        disabled={idx === 0}
                        onClick={() => {
                          const copy = [...guiRules];
                          const temp = copy[idx];
                          copy[idx] = copy[idx - 1];
                          copy[idx - 1] = temp;
                          setGuiRules(copy);
                          compileGuiRulesToJSON(copy);
                        }}
                      >
                        <FiArrowUp size={10} />
                      </button>
                      <button
                        type="button"
                        className="btn btn--xs btn--secondary"
                        disabled={idx === guiRules.length - 1}
                        onClick={() => {
                          const copy = [...guiRules];
                          const temp = copy[idx];
                          copy[idx] = copy[idx + 1];
                          copy[idx + 1] = temp;
                          setGuiRules(copy);
                          compileGuiRulesToJSON(copy);
                        }}
                      >
                        <FiArrowDown size={10} />
                      </button>
                      <button
                        type="button"
                        className="btn btn--xs btn--secondary"
                        style={{ borderColor: 'var(--color-brand-red)', color: 'var(--color-brand-red)' }}
                        onClick={() => {
                          const copy = guiRules.filter((_, i) => i !== idx);
                          setGuiRules(copy);
                          compileGuiRulesToJSON(copy);
                        }}
                      >
                        <FiTrash2 size={10} />
                      </button>
                    </div>
                  </div>

                  <div style={{ display: 'grid', gridTemplateColumns: '1.2fr 1fr', gap: 8 }}>
                    <div>
                      <label style={{ display: 'block', fontSize: 10, color: 'var(--color-brand-muted)', marginBottom: 2 }}>Outbound Tag</label>
                      <select
                        value={rule.outboundTag}
                        onChange={(e) => {
                          const copy = [...guiRules];
                          copy[idx].outboundTag = e.target.value;
                          setGuiRules(copy);
                          compileGuiRulesToJSON(copy);
                        }}
                        style={{ width: '100%', padding: '4px 8px', borderRadius: 6, border: '1px solid var(--color-brand-border)', background: 'var(--color-brand-card)', color: 'var(--color-brand-heading)', fontSize: 11 }}
                      >
                        <option value="proxy">Proxy (selected node)</option>
                        <option value="direct">Direct (Bypass)</option>
                        <option value="block">Block (Drop)</option>
                      </select>
                    </div>
                    <div>
                      <label style={{ display: 'block', fontSize: 10, color: 'var(--color-brand-muted)', marginBottom: 2 }}>Network Protocol</label>
                      <select
                        value={rule.network}
                        onChange={(e) => {
                          const copy = [...guiRules];
                          copy[idx].network = e.target.value;
                          setGuiRules(copy);
                          compileGuiRulesToJSON(copy);
                        }}
                        style={{ width: '100%', padding: '4px 8px', borderRadius: 6, border: '1px solid var(--color-brand-border)', background: 'var(--color-brand-card)', color: 'var(--color-brand-heading)', fontSize: 11 }}
                      >
                        <option value="any">Any (TCP + UDP)</option>
                        <option value="tcp">TCP</option>
                        <option value="udp">UDP</option>
                      </select>
                    </div>
                  </div>

                  <div>
                    <label style={{ display: 'block', fontSize: 10, color: 'var(--color-brand-muted)', marginBottom: 2 }}>Domains (Comma separated)</label>
                    <input
                      type="text"
                      value={rule.domain}
                      onChange={(e) => {
                        const copy = [...guiRules];
                        copy[idx].domain = e.target.value;
                        setGuiRules(copy);
                        compileGuiRulesToJSON(copy);
                      }}
                      placeholder="e.g. domain:google.com, geosite:google"
                      style={{ width: '100%', padding: '4px 8px', borderRadius: 6, border: '1px solid var(--color-brand-border)', background: 'var(--color-brand-card)', color: 'var(--color-brand-heading)', fontSize: 11 }}
                    />
                  </div>

                  <div>
                    <label style={{ display: 'block', fontSize: 10, color: 'var(--color-brand-muted)', marginBottom: 2 }}>IP Blocks / GeoIP (Comma separated)</label>
                    <input
                      type="text"
                      value={rule.ip}
                      onChange={(e) => {
                        const copy = [...guiRules];
                        copy[idx].ip = e.target.value;
                        setGuiRules(copy);
                        compileGuiRulesToJSON(copy);
                      }}
                      placeholder="e.g. 8.8.8.8, geoip:private"
                      style={{ width: '100%', padding: '4px 8px', borderRadius: 6, border: '1px solid var(--color-brand-border)', background: 'var(--color-brand-card)', color: 'var(--color-brand-heading)', fontSize: 11 }}
                    />
                  </div>

                  <div>
                    <label style={{ display: 'block', fontSize: 10, color: 'var(--color-brand-muted)', marginBottom: 2 }}>Destination Ports</label>
                    <input
                      type="text"
                      value={rule.port}
                      onChange={(e) => {
                        const copy = [...guiRules];
                        copy[idx].port = e.target.value;
                        setGuiRules(copy);
                        compileGuiRulesToJSON(copy);
                      }}
                      placeholder="e.g. 80, 443, 8080-8090"
                      style={{ width: '100%', padding: '4px 8px', borderRadius: 6, border: '1px solid var(--color-brand-border)', background: 'var(--color-brand-card)', color: 'var(--color-brand-heading)', fontSize: 11 }}
                    />
                  </div>
                </div>
              ))}
            </div>
          )}

          <button type="submit" className="btn btn--primary" style={{ display: 'flex', alignItems: 'center', justifyContent: 'center', gap: 6 }}>
            <FiSave /> Save Routing Parameters
          </button>
        </form>
 
        {/* Right Side: Domain Fronting Maps */}
        <div className="g-card" style={{ display: 'flex', flexDirection: 'column', gap: 16 }}>
          <div style={{ display: 'flex', alignItems: 'center', gap: 8 }}>
            <FiGlobe style={{ color: 'var(--color-brand)', fontSize: 18 }} />
            <span style={{ fontSize: 15, fontWeight: 600, color: 'var(--color-brand-heading)' }}>Client Domain Fronting MitM</span>
            <FiHelpCircle 
              style={{ cursor: 'pointer', color: 'var(--color-brand-muted)' }} 
              onClick={() => showHelp('Domain Fronting', 'Intercepts and maps outbound proxy connections. Directs traffic to whitelisted fronting domains (like CDNs) with specific CDN destination IPs to bypass censorship blockades.')}
            />
          </div>

          {/* Form */}
          <form onSubmit={handleAddFrontingMap} style={{ background: 'var(--color-brand-bg)', padding: 12, borderRadius: 8, border: '1px solid var(--color-brand-border)', display: 'flex', flexDirection: 'column', gap: 10 }}>
            <span style={{ fontSize: 12, fontWeight: 700, color: 'var(--color-brand-heading)' }}>Add Fronting Map</span>
            <div style={{ display: 'grid', gridTemplateColumns: '1fr 1fr', gap: 10 }}>
              <input
                type="text"
                placeholder="Target Host (e.g. proxy.com)"
                value={targetDomain}
                onChange={(e) => setTargetDomain(e.target.value)}
                style={{
                  padding: '6px 10px',
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
                placeholder="Front Host (e.g. cdn.com)"
                value={frontDomain}
                onChange={(e) => setFrontDomain(e.target.value)}
                style={{
                  padding: '6px 10px',
                  borderRadius: 6,
                  border: '1px solid var(--color-brand-border)',
                  background: 'var(--color-brand-card)',
                  fontSize: 12,
                  color: 'var(--color-brand-heading)'
                }}
                required
              />
            </div>
            <div style={{ display: 'grid', gridTemplateColumns: '2fr 1fr', gap: 10 }}>
              <input
                type="text"
                placeholder="CDN IP Address"
                value={cdnIP}
                onChange={(e) => setCdnIP(e.target.value)}
                style={{
                  padding: '6px 10px',
                  borderRadius: 6,
                  border: '1px solid var(--color-brand-border)',
                  background: 'var(--color-brand-card)',
                  fontSize: 12,
                  color: 'var(--color-brand-heading)'
                }}
                required
              />
              <input
                type="number"
                placeholder="Port"
                value={frontPort}
                onChange={(e) => setFrontPort(Number(e.target.value))}
                style={{
                  padding: '6px 10px',
                  borderRadius: 6,
                  border: '1px solid var(--color-brand-border)',
                  background: 'var(--color-brand-card)',
                  fontSize: 12,
                  color: 'var(--color-brand-heading)'
                }}
                required
              />
            </div>
            <div style={{ display: 'grid', gridTemplateColumns: '1.5fr 1fr', gap: 10 }}>
              <input
                type="text"
                placeholder="Override SNI (blank for Front Host)"
                value={frontSNI}
                onChange={(e) => setFrontSNI(e.target.value)}
                style={{
                  padding: '6px 10px',
                  borderRadius: 6,
                  border: '1px solid var(--color-brand-border)',
                  background: 'var(--color-brand-card)',
                  fontSize: 12,
                  color: 'var(--color-brand-heading)'
                }}
              />
              <input
                type="text"
                placeholder="ALPN (e.g. h2,http/1.1)"
                value={frontAlpn}
                onChange={(e) => setFrontAlpn(e.target.value)}
                style={{
                  padding: '6px 10px',
                  borderRadius: 6,
                  border: '1px solid var(--color-brand-border)',
                  background: 'var(--color-brand-card)',
                  fontSize: 12,
                  color: 'var(--color-brand-heading)'
                }}
                required
              />
            </div>
            <button type="submit" className="btn btn--secondary btn--sm" style={{ display: 'flex', alignItems: 'center', justifyContent: 'center', gap: 4, width: '100%', marginTop: 4 }}>
              <FiPlus /> Add Map Rule
            </button>
          </form>

          {/* List */}
          <div style={{ overflowX: 'auto', border: '1px solid var(--color-brand-border)', borderRadius: 8 }}>
            <table style={{ width: '100%', borderCollapse: 'collapse', fontSize: 11, textAlign: 'left' }}>
              <thead>
                <tr style={{ background: 'var(--color-brand-bg)', borderBottom: '1px solid var(--color-brand-border)' }}>
                  <th style={{ padding: '8px 10px', color: 'var(--color-brand-heading)' }}>Target Domain</th>
                  <th style={{ padding: '8px 10px', color: 'var(--color-brand-heading)' }}>Front / SNI</th>
                  <th style={{ padding: '8px 10px', color: 'var(--color-brand-heading)' }}>CDN IP:Port</th>
                  <th style={{ padding: '8px 10px', color: 'var(--color-brand-heading)' }}>ALPN</th>
                  <th style={{ padding: '8px 10px', color: 'var(--color-brand-heading)', textAlign: 'center' }}>Remove</th>
                </tr>
              </thead>
              <tbody>
                {frontingMaps.length === 0 ? (
                  <tr>
                    <td colSpan={5} style={{ padding: 12, textAlign: 'center', color: 'var(--color-brand-muted)' }}>No fronting mappings defined.</td>
                  </tr>
                ) : (
                  frontingMaps.map((m, idx) => (
                    <tr key={idx} style={{ borderBottom: '1px solid var(--color-brand-border)' }}>
                      <td style={{ padding: '8px 10px', fontWeight: 600, color: 'var(--color-brand-heading)' }}>{m.target}</td>
                      <td style={{ padding: '8px 10px', color: 'var(--color-brand-heading)' }}>
                        <div>{m.front}</div>
                        {m.sni !== m.front && <div style={{ fontSize: 9, color: 'var(--color-brand-muted)' }}>SNI: {m.sni}</div>}
                      </td>
                      <td style={{ padding: '8px 10px', fontFamily: 'Fira Code' }}>{m.cdn}:{m.port || 443}</td>
                      <td style={{ padding: '8px 10px', color: 'var(--color-brand-text)' }}>{m.alpn}</td>
                      <td style={{ padding: '8px 10px', textAlign: 'center' }}>
                        <button onClick={() => handleDeleteFrontingMap(idx)} style={{ background: 'none', border: 'none', cursor: 'pointer', color: 'var(--color-brand-red)' }}>
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
