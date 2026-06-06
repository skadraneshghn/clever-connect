import React, { useState, useEffect } from 'react';
import { FiSliders, FiCpu, FiGlobe, FiSave, FiRefreshCw, FiHelpCircle, FiFileText, FiCheck, FiX } from 'react-icons/fi';

export const V2RayCorePage: React.FC = () => {
  const [helpTitle, setHelpTitle] = useState<string | null>(null);
  const [helpText, setHelpText] = useState<string | null>(null);

  const showHelp = (title: string, text: string) => {
    setHelpTitle(title);
    setHelpText(text);
  };

  const [isLoading, setIsLoading] = useState(false);
  const [message, setMessage] = useState<{ type: 'success' | 'error', text: string } | null>(null);

  // Settings
  const [rawConfigJson, setRawConfigJson] = useState('{\n  "log": {\n    "loglevel": "warning"\n  }\n}');
  const [geofilesUpdate, setGeofilesUpdate] = useState(true);
  const [loggingLevel, setLoggingLevel] = useState('warning');
  const [maxConnections, setMaxConnections] = useState(1024);
  const [bufferSize, setBufferSize] = useState(512);

  // Tab State
  const [activeTab, setActiveTab] = useState<'gui' | 'raw'>('gui');

  // GUI States
  const [guiAccessLog, setGuiAccessLog] = useState('');
  const [guiErrorLog, setGuiErrorLog] = useState('');
  const [guiDnsServers, setGuiDnsServers] = useState('8.8.8.8\n1.1.1.1');
  const [guiDomainStrategy, setGuiDomainStrategy] = useState('IPIfNonMatch');
  const [guiInbounds, setGuiInbounds] = useState<any[]>([
    { port: 10808, protocol: 'socks', listen: '127.0.0.1', tag: 'socks-in' }
  ]);
  const [guiOutbounds, setGuiOutbounds] = useState<any[]>([
    { protocol: 'freedom', tag: 'direct', address: '', port: '', uuid: '' }
  ]);

  const compileGuiToJSON = (
    logLvl = loggingLevel,
    accLog = guiAccessLog,
    errLog = guiErrorLog,
    dnsSrvs = guiDnsServers,
    domStrat = guiDomainStrategy,
    inb = guiInbounds,
    outb = guiOutbounds
  ) => {
    try {
      let base: any = {};
      try {
        base = JSON.parse(rawConfigJson);
      } catch (_) {
        base = {};
      }
      base.log = {
        loglevel: logLvl,
        access: accLog || undefined,
        error: errLog || undefined
      };
      base.dns = {
        servers: dnsSrvs.split('\n').map(s => s.trim()).filter(Boolean)
      };
      base.routing = {
        domainStrategy: domStrat,
        rules: base.routing?.rules || []
      };
      base.inbounds = inb.map(i => ({
        port: Number(i.port) || 10808,
        protocol: i.protocol,
        listen: i.listen || '127.0.0.1',
        tag: i.tag || undefined,
        settings: i.protocol === 'socks' ? { auth: 'noauth', udp: true } : {}
      }));
      base.outbounds = outb.map(o => {
        const ob: any = {
          protocol: o.protocol,
          tag: o.tag
        };
        if (o.protocol !== 'freedom' && o.protocol !== 'blackhole') {
          ob.settings = {
            servers: [
              {
                address: o.address || '127.0.0.1',
                port: Number(o.port) || 443,
                users: o.uuid ? [{ id: o.uuid, security: 'auto' }] : undefined
              }
            ]
          };
        } else {
          ob.settings = {};
        }
        return ob;
      });
      const jsonStr = JSON.stringify(base, null, 2);
      setRawConfigJson(jsonStr);
      return jsonStr;
    } catch (e) {
      console.error(e);
      return rawConfigJson;
    }
  };

  const syncRawToGui = (jsonStr: string) => {
    try {
      const parsed = JSON.parse(jsonStr);
      if (parsed.log) {
        if (parsed.log.loglevel) setLoggingLevel(parsed.log.loglevel);
        if (parsed.log.access) setGuiAccessLog(parsed.log.access);
        if (parsed.log.error) setGuiErrorLog(parsed.log.error);
      }
      if (parsed.dns && Array.isArray(parsed.dns.servers)) {
        setGuiDnsServers(parsed.dns.servers.join('\n'));
      }
      if (parsed.routing && parsed.routing.domainStrategy) {
        setGuiDomainStrategy(parsed.routing.domainStrategy);
      }
      if (Array.isArray(parsed.inbounds)) {
        setGuiInbounds(parsed.inbounds.map((ib: any) => ({
          port: ib.port || 10808,
          protocol: ib.protocol || 'socks',
          listen: ib.listen || '127.0.0.1',
          tag: ib.tag || ''
        })));
      }
      if (Array.isArray(parsed.outbounds)) {
        setGuiOutbounds(parsed.outbounds.map((ob: any) => {
          const serverInfo = ob.settings?.servers?.[0];
          const userInfo = serverInfo?.users?.[0];
          return {
            protocol: ob.protocol || 'freedom',
            tag: ob.tag || 'direct',
            address: serverInfo?.address || '',
            port: serverInfo?.port || '',
            uuid: userInfo?.id || userInfo?.uuid || ''
          };
        }));
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
        if (data.raw_config_json) {
          setRawConfigJson(data.raw_config_json);
          syncRawToGui(data.raw_config_json);
        }
        setGeofilesUpdate(data.geofiles_auto_update === 'true');
        setLoggingLevel(data.logging_level || 'warning');
        setMaxConnections(Number(data.max_connections) || 1024);
        setBufferSize(Number(data.buffer_size) || 512);
      }
    } catch (err) {
      console.error(err);
    } finally {
      setIsLoading(false);
    }
  };

  const handleSave = async (e: React.FormEvent) => {
    e.preventDefault();
    setIsLoading(true);
    setMessage(null);
    try {
      let finalJson = rawConfigJson;
      if (activeTab === 'gui') {
        finalJson = compileGuiToJSON();
      }

      // Validate JSON first
      try {
        JSON.parse(finalJson);
      } catch (err: any) {
        setMessage({ type: 'error', text: `Invalid config JSON format: ${err.message}` });
        setIsLoading(false);
        return;
      }

      const token = localStorage.getItem('cc_client_token') || '';
      const res = await fetch('/api/v2ray/client/settings', {
        method: 'POST',
        headers: {
          'Content-Type': 'application/json',
          'Authorization': `Bearer ${token}`
        },
        body: JSON.stringify({
          raw_config_json: finalJson,
          geofiles_auto_update: String(geofilesUpdate),
          logging_level: loggingLevel,
          max_connections: String(maxConnections),
          buffer_size: String(bufferSize)
        })
      });
      if (res.ok) {
        setMessage({ type: 'success', text: 'Core configuration settings updated successfully!' });
      } else {
        const errData = await res.json();
        setMessage({ type: 'error', text: errData.error || 'Failed to save core settings.' });
      }
    } catch (err: any) {
      setMessage({ type: 'error', text: err.message });
    } finally {
      setIsLoading(false);
    }
  };

  const handleFetchActiveGenerated = async () => {
    setIsLoading(true);
    try {
      const defaultJson = {
        log: { loglevel: loggingLevel },
        inbounds: [
          {
            port: 10808,
            protocol: "socks",
            settings: { auth: "noauth", udp: true }
          }
        ],
        outbounds: [
          {
            protocol: "freedom",
            settings: {}
          }
        ]
      };
      const jsonStr = JSON.stringify(defaultJson, null, 2);
      setRawConfigJson(jsonStr);
      syncRawToGui(jsonStr);
    } catch (err) {
      console.error(err);
    } finally {
      setIsLoading(false);
    }
  };

  useEffect(() => {
    loadSettings();
  }, []);

  return (
    <div>
      <div style={{ display: 'flex', justifyContent: 'space-between', alignItems: 'center', marginBottom: 20 }}>
        <div>
          <h1 style={{ fontSize: 22, fontWeight: 700, color: 'var(--color-brand-heading)', margin: 0 }}>V2Ray / Xray Core Configurator</h1>
          <p style={{ fontSize: 12, color: 'var(--color-brand-text)', margin: '4px 0 0' }}>
            Modify daemon log levels, buffer boundaries, and write/overwrite raw client configuration files directly.
          </p>
        </div>
        <button className="btn btn--sm" onClick={loadSettings} disabled={isLoading}>
          <FiRefreshCw className={isLoading ? 'spin-animation' : ''} style={{ marginRight: 6 }} /> Reload Settings
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

      {/* Two Tab Navigation */}
      <div style={{ display: 'flex', gap: 12, marginBottom: 20, borderBottom: '1px solid var(--color-brand-border)', paddingBottom: 10 }}>
        <button
          type="button"
          onClick={() => {
            syncRawToGui(rawConfigJson);
            setActiveTab('gui');
          }}
          style={{
            padding: '8px 16px',
            borderRadius: 6,
            background: activeTab === 'gui' ? 'var(--color-brand)' : 'none',
            color: activeTab === 'gui' ? '#fff' : 'var(--color-brand-text)',
            border: 'none',
            cursor: 'pointer',
            fontWeight: 600,
            fontSize: 13
          }}
        >
          GUI Configurator
        </button>
        <button
          type="button"
          onClick={() => {
            compileGuiToJSON();
            setActiveTab('raw');
          }}
          style={{
            padding: '8px 16px',
            borderRadius: 6,
            background: activeTab === 'raw' ? 'var(--color-brand)' : 'none',
            color: activeTab === 'raw' ? '#fff' : 'var(--color-brand-text)',
            border: 'none',
            cursor: 'pointer',
            fontWeight: 600,
            fontSize: 13
          }}
        >
          Raw JSON Config
        </button>
      </div>

      <div style={{ display: 'grid', gridTemplateColumns: '1fr 340px', gap: 24 }}>
        
        {activeTab === 'raw' ? (
          /* Tab 1: Monospace JSON Editor card */
          <div className="g-card" style={{ display: 'flex', flexDirection: 'column' }}>
            <div style={{ display: 'flex', alignItems: 'center', justifyContent: 'space-between', marginBottom: 16 }}>
              <div style={{ display: 'flex', alignItems: 'center', gap: 8 }}>
                <FiFileText style={{ color: 'var(--color-brand)', fontSize: 18 }} />
                <span style={{ fontSize: 15, fontWeight: 600, color: 'var(--color-brand-heading)' }}>Raw Config JSON Editor</span>
                <FiHelpCircle 
                  style={{ cursor: 'pointer', color: 'var(--color-brand-muted)' }} 
                  onClick={() => showHelp('Raw JSON Editor', 'Write standard Xray config JSON blocks to customize routing details, add multiple inbounds, or map advanced transport parameters. Validates JSON content structural soundness prior to save operations.')}
                />
              </div>
              <button className="btn btn--xs btn--secondary" onClick={handleFetchActiveGenerated}>
                Load Default Structure
              </button>
            </div>

            <textarea
              value={rawConfigJson}
              onChange={(e) => setRawConfigJson(e.target.value)}
              rows={22}
              style={{
                width: '100%',
                padding: 14,
                borderRadius: 8,
                border: '1px solid var(--color-brand-border)',
                background: '#1a1a2e',
                color: '#a9b1d6',
                fontFamily: 'Fira Code, monospace',
                fontSize: 12,
                lineHeight: 1.5,
                resize: 'vertical'
              }}
            />
          </div>
        ) : (
          /* Tab 2: GUI Options Configurator */
          <div className="g-card" style={{ display: 'flex', flexDirection: 'column', gap: 20 }}>
            <div style={{ display: 'flex', alignItems: 'center', gap: 8, marginBottom: 10 }}>
              <FiSliders style={{ color: 'var(--color-brand)', fontSize: 18 }} />
              <span style={{ fontSize: 15, fontWeight: 600, color: 'var(--color-brand-heading)' }}>GUI Core Settings</span>
              <FiHelpCircle 
                style={{ cursor: 'pointer', color: 'var(--color-brand-muted)' }} 
                onClick={() => showHelp('GUI Editor', 'Configure the V2Ray core using simple forms. Changes will be compiled to raw JSON dynamically before saving.')}
              />
            </div>

            {/* Inbounds section */}
            <div style={{ borderBottom: '1px solid var(--color-brand-border)', paddingBottom: 16 }}>
              <h3 style={{ fontSize: 13, fontWeight: 700, color: 'var(--color-brand-heading)', marginBottom: 12 }}>Inbound Interfaces</h3>
              {guiInbounds.map((ib, idx) => (
                <div key={idx} style={{ display: 'flex', gap: 8, alignItems: 'center', marginBottom: 8 }}>
                  <input
                    type="number"
                    value={ib.port}
                    onChange={(e) => {
                      const copy = [...guiInbounds];
                      copy[idx].port = Number(e.target.value);
                      setGuiInbounds(copy);
                      compileGuiToJSON(loggingLevel, guiAccessLog, guiErrorLog, guiDnsServers, guiDomainStrategy, copy, guiOutbounds);
                    }}
                    placeholder="Port"
                    style={{ width: 80, padding: '6px 10px', borderRadius: 6, border: '1px solid var(--color-brand-border)', background: 'var(--color-brand-card)', color: 'var(--color-brand-heading)', fontSize: 12 }}
                  />
                  <select
                    value={ib.protocol}
                    onChange={(e) => {
                      const copy = [...guiInbounds];
                      copy[idx].protocol = e.target.value;
                      setGuiInbounds(copy);
                      compileGuiToJSON(loggingLevel, guiAccessLog, guiErrorLog, guiDnsServers, guiDomainStrategy, copy, guiOutbounds);
                    }}
                    style={{ width: 90, padding: '6px 10px', borderRadius: 6, border: '1px solid var(--color-brand-border)', background: 'var(--color-brand-card)', color: 'var(--color-brand-heading)', fontSize: 12 }}
                  >
                    <option value="socks">Socks</option>
                    <option value="http">HTTP</option>
                    <option value="vmess">VMess</option>
                    <option value="vless">VLess</option>
                    <option value="trojan">Trojan</option>
                  </select>
                  <input
                    type="text"
                    value={ib.listen}
                    onChange={(e) => {
                      const copy = [...guiInbounds];
                      copy[idx].listen = e.target.value;
                      setGuiInbounds(copy);
                      compileGuiToJSON(loggingLevel, guiAccessLog, guiErrorLog, guiDnsServers, guiDomainStrategy, copy, guiOutbounds);
                    }}
                    placeholder="Listen IP"
                    style={{ flex: 1, padding: '6px 10px', borderRadius: 6, border: '1px solid var(--color-brand-border)', background: 'var(--color-brand-card)', color: 'var(--color-brand-heading)', fontSize: 12 }}
                  />
                  <input
                    type="text"
                    value={ib.tag}
                    onChange={(e) => {
                      const copy = [...guiInbounds];
                      copy[idx].tag = e.target.value;
                      setGuiInbounds(copy);
                      compileGuiToJSON(loggingLevel, guiAccessLog, guiErrorLog, guiDnsServers, guiDomainStrategy, copy, guiOutbounds);
                    }}
                    placeholder="Tag"
                    style={{ flex: 1, padding: '6px 10px', borderRadius: 6, border: '1px solid var(--color-brand-border)', background: 'var(--color-brand-card)', color: 'var(--color-brand-heading)', fontSize: 12 }}
                  />
                  <button
                    type="button"
                    className="btn btn--xs btn--secondary"
                    onClick={() => {
                      const copy = guiInbounds.filter((_, i) => i !== idx);
                      setGuiInbounds(copy);
                      compileGuiToJSON(loggingLevel, guiAccessLog, guiErrorLog, guiDnsServers, guiDomainStrategy, copy, guiOutbounds);
                    }}
                    style={{ borderColor: 'var(--color-brand-red)', color: 'var(--color-brand-red)' }}
                  >
                    Remove
                  </button>
                </div>
              ))}
              <button
                type="button"
                className="btn btn--xs btn--secondary"
                onClick={() => {
                  const copy = [...guiInbounds, { port: 10808 + guiInbounds.length, protocol: 'socks', listen: '127.0.0.1', tag: `inbound-${guiInbounds.length}` }];
                  setGuiInbounds(copy);
                  compileGuiToJSON(loggingLevel, guiAccessLog, guiErrorLog, guiDnsServers, guiDomainStrategy, copy, guiOutbounds);
                }}
              >
                + Add Inbound
              </button>
            </div>

            {/* Outbounds section */}
            <div style={{ borderBottom: '1px solid var(--color-brand-border)', paddingBottom: 16 }}>
              <h3 style={{ fontSize: 13, fontWeight: 700, color: 'var(--color-brand-heading)', marginBottom: 12 }}>Outbound Configurations</h3>
              {guiOutbounds.map((ob, idx) => (
                <div key={idx} style={{ display: 'flex', flexDirection: 'column', gap: 8, padding: 10, background: 'var(--color-brand-bg)', borderRadius: 8, border: '1px solid var(--color-brand-border)', marginBottom: 10 }}>
                  <div style={{ display: 'flex', gap: 10, alignItems: 'center' }}>
                    <select
                      value={ob.protocol}
                      onChange={(e) => {
                        const copy = [...guiOutbounds];
                        copy[idx].protocol = e.target.value;
                        setGuiOutbounds(copy);
                        compileGuiToJSON(loggingLevel, guiAccessLog, guiErrorLog, guiDnsServers, guiDomainStrategy, guiInbounds, copy);
                      }}
                      style={{ width: 110, padding: 6, borderRadius: 6, border: '1px solid var(--color-brand-border)', background: 'var(--color-brand-card)', color: 'var(--color-brand-heading)', fontSize: 12 }}
                    >
                      <option value="freedom">Freedom (Direct)</option>
                      <option value="blackhole">Blackhole (Block)</option>
                      <option value="vmess">VMess</option>
                      <option value="vless">VLess</option>
                      <option value="trojan">Trojan</option>
                      <option value="shadowsocks">Shadowsocks</option>
                    </select>
                    <input
                      type="text"
                      value={ob.tag}
                      onChange={(e) => {
                        const copy = [...guiOutbounds];
                        copy[idx].tag = e.target.value;
                        setGuiOutbounds(copy);
                        compileGuiToJSON(loggingLevel, guiAccessLog, guiErrorLog, guiDnsServers, guiDomainStrategy, guiInbounds, copy);
                      }}
                      placeholder="Tag (e.g. direct / proxy)"
                      style={{ flex: 1, padding: 6, borderRadius: 6, border: '1px solid var(--color-brand-border)', background: 'var(--color-brand-card)', color: 'var(--color-brand-heading)', fontSize: 12 }}
                    />
                    <button
                      type="button"
                      className="btn btn--xs btn--secondary"
                      onClick={() => {
                        const copy = guiOutbounds.filter((_, i) => i !== idx);
                        setGuiOutbounds(copy);
                        compileGuiToJSON(loggingLevel, guiAccessLog, guiErrorLog, guiDnsServers, guiDomainStrategy, guiInbounds, copy);
                      }}
                      style={{ borderColor: 'var(--color-brand-red)', color: 'var(--color-brand-red)' }}
                    >
                      Remove
                    </button>
                  </div>

                  {ob.protocol !== 'freedom' && ob.protocol !== 'blackhole' && (
                    <div style={{ display: 'grid', gridTemplateColumns: '2fr 1fr 2fr', gap: 8 }}>
                      <input
                        type="text"
                        value={ob.address || ''}
                        onChange={(e) => {
                          const copy = [...guiOutbounds];
                          copy[idx].address = e.target.value;
                          setGuiOutbounds(copy);
                          compileGuiToJSON(loggingLevel, guiAccessLog, guiErrorLog, guiDnsServers, guiDomainStrategy, guiInbounds, copy);
                        }}
                        placeholder="Server Address"
                        style={{ padding: 6, borderRadius: 6, border: '1px solid var(--color-brand-border)', background: 'var(--color-brand-card)', color: 'var(--color-brand-heading)', fontSize: 12 }}
                      />
                      <input
                        type="number"
                        value={ob.port || ''}
                        onChange={(e) => {
                          const copy = [...guiOutbounds];
                          copy[idx].port = e.target.value;
                          setGuiOutbounds(copy);
                          compileGuiToJSON(loggingLevel, guiAccessLog, guiErrorLog, guiDnsServers, guiDomainStrategy, guiInbounds, copy);
                        }}
                        placeholder="Port"
                        style={{ padding: 6, borderRadius: 6, border: '1px solid var(--color-brand-border)', background: 'var(--color-brand-card)', color: 'var(--color-brand-heading)', fontSize: 12 }}
                      />
                      <input
                        type="text"
                        value={ob.uuid || ''}
                        onChange={(e) => {
                          const copy = [...guiOutbounds];
                          copy[idx].uuid = e.target.value;
                          setGuiOutbounds(copy);
                          compileGuiToJSON(loggingLevel, guiAccessLog, guiErrorLog, guiDnsServers, guiDomainStrategy, guiInbounds, copy);
                        }}
                        placeholder="UUID / Password"
                        style={{ padding: 6, borderRadius: 6, border: '1px solid var(--color-brand-border)', background: 'var(--color-brand-card)', color: 'var(--color-brand-heading)', fontSize: 12 }}
                      />
                    </div>
                  )}
                </div>
              ))}
              <button
                type="button"
                className="btn btn--xs btn--secondary"
                onClick={() => {
                  const copy = [...guiOutbounds, { protocol: 'freedom', tag: `outbound-${guiOutbounds.length}`, address: '', port: '', uuid: '' }];
                  setGuiOutbounds(copy);
                  compileGuiToJSON(loggingLevel, guiAccessLog, guiErrorLog, guiDnsServers, guiDomainStrategy, guiInbounds, copy);
                }}
              >
                + Add Outbound
              </button>
            </div>

            {/* DNS list and Domain strategy */}
            <div style={{ display: 'grid', gridTemplateColumns: '1fr 1fr', gap: 16 }}>
              <div>
                <label style={{ display: 'block', fontSize: 11, fontWeight: 600, color: 'var(--color-brand-muted)', marginBottom: 6, textTransform: 'uppercase' }}>DNS Servers (One per line)</label>
                <textarea
                  value={guiDnsServers}
                  onChange={(e) => {
                    setGuiDnsServers(e.target.value);
                    compileGuiToJSON(loggingLevel, guiAccessLog, guiErrorLog, e.target.value, guiDomainStrategy, guiInbounds, guiOutbounds);
                  }}
                  rows={3}
                  style={{
                    width: '100%',
                    padding: '8px 12px',
                    borderRadius: 8,
                    border: '1px solid var(--color-brand-border)',
                    background: 'var(--color-brand-card)',
                    fontSize: 12,
                    color: 'var(--color-brand-heading)'
                  }}
                />
              </div>

              <div>
                <label style={{ display: 'block', fontSize: 11, fontWeight: 600, color: 'var(--color-brand-muted)', marginBottom: 6, textTransform: 'uppercase' }}>Domain Strategy</label>
                <select
                  value={guiDomainStrategy}
                  onChange={(e) => {
                    setGuiDomainStrategy(e.target.value);
                    compileGuiToJSON(loggingLevel, guiAccessLog, guiErrorLog, guiDnsServers, e.target.value, guiInbounds, guiOutbounds);
                  }}
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
                  <option value="AsIs">AsIs</option>
                  <option value="IPIfNonMatch">IPIfNonMatch</option>
                  <option value="IPOnDemand">IPOnDemand</option>
                </select>
              </div>
            </div>

            {/* Advanced Log Paths */}
            <div style={{ display: 'grid', gridTemplateColumns: '1fr 1fr', gap: 16 }}>
              <div>
                <label style={{ display: 'block', fontSize: 11, fontWeight: 600, color: 'var(--color-brand-muted)', marginBottom: 6, textTransform: 'uppercase' }}>Access Log Path</label>
                <input
                  type="text"
                  value={guiAccessLog}
                  onChange={(e) => {
                    setGuiAccessLog(e.target.value);
                    compileGuiToJSON(loggingLevel, e.target.value, guiErrorLog, guiDnsServers, guiDomainStrategy, guiInbounds, guiOutbounds);
                  }}
                  placeholder="e.g. /var/log/xray/access.log"
                  style={{
                    width: '100%',
                    padding: '8px 12px',
                    borderRadius: 8,
                    border: '1px solid var(--color-brand-border)',
                    background: 'var(--color-brand-card)',
                    fontSize: 13,
                    color: 'var(--color-brand-heading)'
                  }}
                />
              </div>

              <div>
                <label style={{ display: 'block', fontSize: 11, fontWeight: 600, color: 'var(--color-brand-muted)', marginBottom: 6, textTransform: 'uppercase' }}>Error Log Path</label>
                <input
                  type="text"
                  value={guiErrorLog}
                  onChange={(e) => {
                    setGuiErrorLog(e.target.value);
                    compileGuiToJSON(loggingLevel, guiAccessLog, e.target.value, guiDnsServers, guiDomainStrategy, guiInbounds, guiOutbounds);
                  }}
                  placeholder="e.g. /var/log/xray/error.log"
                  style={{
                    width: '100%',
                    padding: '8px 12px',
                    borderRadius: 8,
                    border: '1px solid var(--color-brand-border)',
                    background: 'var(--color-brand-card)',
                    fontSize: 13,
                    color: 'var(--color-brand-heading)'
                  }}
                />
              </div>
            </div>
          </div>
        )}

        {/* Core parameters sidebar card */}
        <form onSubmit={handleSave} className="g-card" style={{ display: 'flex', flexDirection: 'column', gap: 18 }}>
          <div style={{ display: 'flex', alignItems: 'center', gap: 8 }}>
            <FiSliders style={{ color: 'var(--color-brand)', fontSize: 18 }} />
            <span style={{ fontSize: 15, fontWeight: 600, color: 'var(--color-brand-heading)' }}>Core Parameters</span>
          </div>

          <div>
            <label style={{ display: 'block', fontSize: 11, fontWeight: 600, color: 'var(--color-brand-muted)', marginBottom: 6, textTransform: 'uppercase' }}>
              Core Log Verbosity
            </label>
            <select
              value={loggingLevel}
              onChange={(e) => {
                setLoggingLevel(e.target.value);
                compileGuiToJSON(e.target.value, guiAccessLog, guiErrorLog, guiDnsServers, guiDomainStrategy, guiInbounds, guiOutbounds);
              }}
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
              <option value="debug">Debug (All events)</option>
              <option value="info">Info (Default audit)</option>
              <option value="warning">Warning (Issues only)</option>
              <option value="error">Error (Only faults)</option>
            </select>
          </div>

          <div>
            <label style={{ display: 'block', fontSize: 11, fontWeight: 600, color: 'var(--color-brand-muted)', marginBottom: 6, textTransform: 'uppercase' }}>
              Max Connections Limit
            </label>
            <input
              type="number"
              value={maxConnections}
              onChange={(e) => setMaxConnections(Number(e.target.value))}
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
            <label style={{ display: 'block', fontSize: 11, fontWeight: 600, color: 'var(--color-brand-muted)', marginBottom: 6, textTransform: 'uppercase' }}>
              Stream Buffer Size (KB)
            </label>
            <input
              type="number"
              value={bufferSize}
              onChange={(e) => setBufferSize(Number(e.target.value))}
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

          <div style={{ display: 'flex', alignItems: 'center', justifyContent: 'space-between', borderTop: '1px solid var(--color-brand-border)', paddingTop: 16 }}>
            <div>
              <span style={{ fontSize: 13, fontWeight: 600, color: 'var(--color-brand-heading)' }}>Auto-Update GeoFiles</span>
              <p style={{ margin: 0, fontSize: 9, color: 'var(--color-brand-text)' }}>
                Download latest IP database files on startup.
              </p>
            </div>
            <input
              type="checkbox"
              checked={geofilesUpdate}
              onChange={(e) => setGeofilesUpdate(e.target.checked)}
              style={{ width: 16, height: 16, cursor: 'pointer', accentColor: 'var(--color-brand)' }}
            />
          </div>

          <button type="submit" className="btn btn--primary" style={{ display: 'flex', alignItems: 'center', justifyContent: 'center', gap: 6, marginTop: 10 }} disabled={isLoading}>
            <FiSave /> Save Core Settings
          </button>
        </form>
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
