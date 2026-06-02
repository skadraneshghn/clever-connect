import React, { useState, useEffect } from 'react';
import { FiSliders, FiCpu, FiGlobe, FiKey, FiPlay, FiSquare, FiSave, FiRefreshCw, FiEye, FiEyeOff } from 'react-icons/fi';

export const EhcoServerPage: React.FC = () => {
  const [listenPort, setListenPort] = useState('3001');
  const [authToken, setAuthToken] = useState('');
  const [targetMode, setTargetMode] = useState('direct');
  const [targetHost, setTargetHost] = useState('127.0.0.1:80');
  
  // NEW CTO CONFIGS
  const [enableMux, setEnableMux] = useState(true);
  const [keepAlive, setKeepAlive] = useState(15);
  const [showAdvanced, setShowAdvanced] = useState(false);

  const [isRunning, setIsRunning] = useState(false);
  const [showToken, setShowToken] = useState(false);
  const [isLoading, setIsLoading] = useState(false);
  const [message, setMessage] = useState<{ type: 'success' | 'error', text: string } | null>(null);

  // Fetch current configs
  const fetchConfig = async () => {
    setIsLoading(true);
    try {
      const token = localStorage.getItem('cc_server_token') || '';
      const response = await fetch('/api/ehco/config', {
        headers: {
          'Authorization': `Bearer ${token}`
        }
      });
      if (response.ok) {
        const data = await response.json();
        if (data.config) {
          setListenPort(data.config.listen_port || '3001');
          setAuthToken(data.config.auth_token || '');
          setTargetMode(data.config.target_mode || 'direct');
          setTargetHost(data.config.target_host || '127.0.0.1:80');
          setEnableMux(data.config.enable_mux !== undefined ? data.config.enable_mux : true);
          setKeepAlive(data.config.keep_alive || 15);
        }
        setIsRunning(data.is_running || false);
      }
    } catch (err) {
      console.error('Failed to fetch ehco server configs', err);
    } finally {
      setIsLoading(false);
    }
  };

  useEffect(() => {
    fetchConfig();
  }, []);

  const generateToken = () => {
    const chars = 'abcdef0123456789';
    let result = '';
    for (let i = 0; i < 32; i++) {
      result += chars.charAt(Math.floor(Math.random() * chars.length));
    }
    setAuthToken(result);
  };

  const handleSave = async (e: React.FormEvent) => {
    e.preventDefault();
    setIsLoading(true);
    setMessage(null);
    try {
      const token = localStorage.getItem('cc_server_token') || '';
      const response = await fetch('/api/ehco/config', {
        method: 'POST',
        headers: {
          'Content-Type': 'application/json',
          'Authorization': `Bearer ${token}`
        },
        body: JSON.stringify({
          listen_port: listenPort,
          auth_token: authToken,
          target_mode: targetMode,
          target_host: targetHost,
          enable_mux: enableMux,
          keep_alive: Number(keepAlive)
        })
      });

      if (response.ok) {
        const data = await response.json();
        setMessage({ type: 'success', text: 'Configurations saved successfully!' });
        // Update states
        if (data.config) {
          setListenPort(data.config.listen_port || '3001');
          setAuthToken(data.config.auth_token || '');
          setTargetMode(data.config.target_mode || 'direct');
          setTargetHost(data.config.target_host || '127.0.0.1:80');
          setEnableMux(data.config.enable_mux !== undefined ? data.config.enable_mux : true);
          setKeepAlive(data.config.keep_alive || 15);
        }
      } else {
        const data = await response.json();
        setMessage({ type: 'error', text: data.error || 'Failed to save configurations.' });
      }
    } catch (err: any) {
      setMessage({ type: 'error', text: err.message || 'An unexpected error occurred.' });
    } finally {
      setIsLoading(false);
    }
  };

  const handleToggleEngine = async () => {
    setIsLoading(true);
    setMessage(null);
    try {
      const token = localStorage.getItem('cc_server_token') || '';
      const endpoint = isRunning ? '/api/ehco/stop' : '/api/ehco/start';
      const response = await fetch(endpoint, {
        method: 'POST',
        headers: {
          'Authorization': `Bearer ${token}`
        }
      });

      if (response.ok) {
        const data = await response.json();
        setIsRunning(data.is_running);
        setMessage({
          type: 'success',
          text: isRunning ? 'Ehco tunnel engine stopped successfully.' : 'Ehco tunnel engine started successfully!'
        });
      } else {
        const data = await response.json();
        setMessage({ type: 'error', text: data.error || 'Operation failed.' });
      }
    } catch (err: any) {
      setMessage({ type: 'error', text: err.message || 'Failed to communicate with engine.' });
    } finally {
      setIsLoading(false);
    }
  };

  return (
    <div>
      {/* Title */}
      <div style={{ display: 'flex', justifyContent: 'space-between', alignItems: 'center', marginBottom: 24 }}>
        <div>
          <h1 style={{ fontSize: 22, fontWeight: 700, color: 'var(--color-brand-heading)', margin: 0 }}>Ehco Tunnel Engine</h1>
          <p style={{ fontSize: 12, color: 'var(--color-brand-text)', margin: '4px 0 0' }}>
            Multiplex and relay incoming WebSocket traffic on Clever Cloud using embedded Ehco Core.
          </p>
        </div>
        <button className="btn btn--sm" onClick={fetchConfig} disabled={isLoading}>
          <FiRefreshCw className={isLoading ? 'spin-animation' : ''} style={{ marginRight: 6 }} /> Sync Status
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
        
        {/* Left Column: Form Settings */}
        <form onSubmit={handleSave} style={{ display: 'flex', flexDirection: 'column', gap: 20 }}>
          <div className="g-card">
            <div style={{ display: 'flex', alignItems: 'center', gap: 8, marginBottom: 18 }}>
              <FiSliders style={{ color: 'var(--color-brand)', fontSize: 18 }} />
              <span style={{ fontSize: 15, fontWeight: 600, color: 'var(--color-brand-heading)' }}>Inbound Connection settings</span>
            </div>

            <div style={{ display: 'grid', gridTemplateColumns: '1fr 1fr', gap: 16, marginBottom: 16 }}>
              <div>
                <label style={{ display: 'block', fontSize: 11, fontWeight: 600, color: 'var(--color-brand-muted)', marginBottom: 6, textTransform: 'uppercase' }}>Listen Port</label>
                <input
                  type="text"
                  value={listenPort}
                  onChange={(e) => setListenPort(e.target.value)}
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
                <span style={{ fontSize: 10, color: 'var(--color-brand-text)', marginTop: 4, display: 'block' }}>
                  Must match internal port 3001 exposed to Nginx.
                </span>
              </div>

              <div>
                <label style={{ display: 'block', fontSize: 11, fontWeight: 600, color: 'var(--color-brand-muted)', marginBottom: 6, textTransform: 'uppercase' }}>WebSocket Path</label>
                <input
                  type="text"
                  value="/tunnel"
                  disabled
                  style={{
                    width: '100%',
                    padding: '10px 12px',
                    borderRadius: 8,
                    border: '1px solid var(--color-brand-border)',
                    background: 'var(--color-brand-bg)',
                    color: 'var(--color-brand-text)',
                    fontSize: 13,
                    cursor: 'not-allowed'
                  }}
                />
                <span style={{ fontSize: 10, color: 'var(--color-brand-text)', marginTop: 4, display: 'block' }}>
                  Multiplexed path handled automatically.
                </span>
              </div>
            </div>

            <div style={{ marginBottom: 8 }}>
              <label style={{ display: 'block', fontSize: 11, fontWeight: 600, color: 'var(--color-brand-muted)', marginBottom: 6, textTransform: 'uppercase' }}>Auth Token (Secure Path Extension)</label>
              <div style={{ display: 'flex', gap: 8 }}>
                <div style={{ position: 'relative', flex: 1 }}>
                  <input
                    type={showToken ? 'text' : 'password'}
                    value={authToken}
                    onChange={(e) => setAuthToken(e.target.value)}
                    placeholder="Enter security token or generate one"
                    style={{
                      width: '100%',
                      padding: '10px 40px 10px 12px',
                      borderRadius: 8,
                      border: '1px solid var(--color-brand-border)',
                      background: 'var(--color-brand-card)',
                      color: 'var(--color-brand-heading)',
                      fontSize: 13,
                      fontFamily: 'Fira Code'
                    }}
                    required
                  />
                  <button
                    type="button"
                    onClick={() => setShowToken(!showToken)}
                    style={{
                      position: 'absolute',
                      right: 12,
                      top: '50%',
                      transform: 'translateY(-50%)',
                      background: 'none',
                      border: 'none',
                      cursor: 'pointer',
                      color: 'var(--color-brand-muted)',
                      display: 'flex',
                      alignItems: 'center'
                    }}
                  >
                    {showToken ? <FiEyeOff size={16} /> : <FiEye size={16} />}
                  </button>
                </div>
                <button type="button" className="btn" onClick={generateToken}>Generate</button>
              </div>
              <span style={{ fontSize: 10, color: 'var(--color-brand-text)', marginTop: 4, display: 'block' }}>
                Secure WebSocket endpoint will listen at <code>/tunnel/{authToken || '<token>'}</code>. Copy this token to Client settings.
              </span>
            </div>
          </div>

          <div className="g-card">
            <div style={{ display: 'flex', alignItems: 'center', gap: 8, marginBottom: 18 }}>
              <FiGlobe style={{ color: 'var(--color-brand)', fontSize: 18 }} />
              <span style={{ fontSize: 15, fontWeight: 600, color: 'var(--color-brand-heading)' }}>Outbound Target Settings</span>
            </div>

            <div style={{ display: 'grid', gridTemplateColumns: '1fr 1fr', gap: 16, marginBottom: 8 }}>
              <div>
                <label style={{ display: 'block', fontSize: 11, fontWeight: 600, color: 'var(--color-brand-muted)', marginBottom: 6, textTransform: 'uppercase' }}>Target Mode</label>
                <select
                  value={targetMode}
                  onChange={(e) => setTargetMode(e.target.value)}
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
                  <option value="direct">Direct Internet</option>
                  <option value="xray">Xray Core (Internal Redirect)</option>
                </select>
              </div>

              <div>
                <label style={{ display: 'block', fontSize: 11, fontWeight: 600, color: 'var(--color-brand-muted)', marginBottom: 6, textTransform: 'uppercase' }}>Outbound Host</label>
                <input
                  type="text"
                  value={targetHost}
                  onChange={(e) => setTargetHost(e.target.value)}
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
          </div>

          {/* Advanced CTO Settings Accordion */}
          <div className="g-card" style={{ padding: '16px 20px' }}>
            <div 
              style={{ display: 'flex', justifyContent: 'space-between', alignItems: 'center', cursor: 'pointer' }}
              onClick={() => setShowAdvanced(!showAdvanced)}
            >
              <div style={{ display: 'flex', alignItems: 'center', gap: 8 }}>
                <FiSliders style={{ color: 'var(--color-brand)', fontSize: 16 }} />
                <span style={{ fontSize: 13, fontWeight: 600, color: 'var(--color-brand-heading)' }}>
                  Advanced DPI Bypass Settings
                </span>
              </div>
              <span style={{ fontSize: 12, color: 'var(--color-brand)', fontWeight: 600 }}>
                {showAdvanced ? 'Hide ▲' : 'Show Advanced Settings ▼'}
              </span>
            </div>

            {showAdvanced && (
              <div style={{ marginTop: 16, display: 'flex', flexDirection: 'column', gap: 14, borderTop: '1px solid var(--color-brand-border)', paddingTop: 16 }}>
                
                {/* Enable Mux */}
                <div style={{ display: 'flex', alignItems: 'center', justifyContent: 'space-between' }}>
                  <div>
                    <label style={{ display: 'block', fontSize: 12, fontWeight: 600, color: 'var(--color-brand-heading)', margin: 0 }}>
                      Enable TCP Multiplexing (Mux)
                    </label>
                    <span style={{ fontSize: 10, color: 'var(--color-brand-text)' }}>
                      Funnels all TCP streams through a single persistent WebSocket to reduce handshake latency.
                    </span>
                  </div>
                  <input
                    type="checkbox"
                    checked={enableMux}
                    onChange={(e) => setEnableMux(e.target.checked)}
                    style={{ width: 18, height: 18, cursor: 'pointer', accentColor: 'var(--color-brand)' }}
                  />
                </div>

                {/* Keep-Alive Interval */}
                <div>
                  <label style={{ display: 'block', fontSize: 11, fontWeight: 600, color: 'var(--color-brand-muted)', marginBottom: 6, textTransform: 'uppercase' }}>
                    Keep-Alive Ping (Seconds)
                  </label>
                  <input
                    type="number"
                    value={keepAlive}
                    onChange={(e) => setKeepAlive(Number(e.target.value))}
                    min="5"
                    max="300"
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
                  <span style={{ fontSize: 10, color: 'var(--color-brand-text)', marginTop: 4, display: 'block' }}>
                    Prevents Iranian ISPs from silently dropping idle proxy sockets. Recommended: 15s.
                  </span>
                </div>

              </div>
            )}
          </div>

          <div style={{ display: 'flex', gap: 12 }}>
            <button type="submit" className="btn btn--primary" style={{ display: 'flex', alignItems: 'center', gap: 6 }} disabled={isLoading}>
              <FiSave /> Save Settings
            </button>
          </div>
        </form>

        {/* Right Column: Engine Status Widget */}
        <div style={{ display: 'flex', flexDirection: 'column', gap: 20 }}>
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
              Engine Status
            </div>

            <div style={{ display: 'flex', alignItems: 'center', gap: 6, marginTop: 8, marginBottom: 14 }}>
              <span className="live-dot" style={{ background: isRunning ? '#10b981' : '#ef4444' }} />
              <span style={{ fontSize: 16, fontWeight: 700, color: 'var(--color-brand-heading)' }}>
                {isRunning ? 'RUNNING' : 'STOPPED'}
              </span>
            </div>

            <p style={{ fontSize: 11, color: 'var(--color-brand-text)', lineHeight: 1.4, margin: '0 0 16px' }}>
              {isRunning
                ? `Ehco engine is active and serving WebSocket connections internally on port ${listenPort}.`
                : 'Tunnel engine is inactive. Click below to boot the Go relay loop.'
              }
            </p>

            <button
              onClick={handleToggleEngine}
              disabled={isLoading}
              style={{
                width: '100%',
                display: 'flex',
                alignItems: 'center',
                justifyContent: 'center',
                gap: 8,
                background: isRunning ? '#ef4444' : 'var(--color-brand)',
                color: '#fff',
                border: 'none',
                padding: '10px 16px',
                borderRadius: 8,
                fontWeight: 600,
                fontSize: 13,
                cursor: 'pointer',
                transition: 'opacity 0.2s'
              }}
              className="action-button-engine"
            >
              {isRunning ? (
                <>
                  <FiSquare /> Stop Engine
                </>
              ) : (
                <>
                  <FiPlay /> Start Engine
                </>
              )}
            </button>
          </div>

          {/* Quick Stats */}
          <div className="g-card">
            <div style={{ display: 'flex', alignItems: 'center', gap: 6, marginBottom: 12 }}>
              <FiKey style={{ color: 'var(--color-brand)', fontSize: 16 }} />
              <span style={{ fontSize: 13, fontWeight: 600, color: 'var(--color-brand-heading)' }}>Endpoint Details</span>
            </div>
            
            <div style={{ display: 'flex', flexDirection: 'column', gap: 10, fontSize: 11 }}>
              <div>
                <div style={{ color: 'var(--color-brand-muted)', marginBottom: 2 }}>Secure Server Path:</div>
                <div style={{ wordBreak: 'break-all', fontWeight: 600, color: 'var(--color-brand-heading)', fontFamily: 'Fira Code' }}>
                  /tunnel/{authToken || '<token>'}
                </div>
              </div>
              
              <div>
                <div style={{ color: 'var(--color-brand-muted)', marginBottom: 2 }}>Protocol:</div>
                <div style={{ fontWeight: 600, color: 'var(--color-brand-heading)' }}>
                  Secure WebSocket (WSS) via Nginx
                </div>
              </div>
            </div>
          </div>
        </div>

      </div>
    </div>
  );
};
