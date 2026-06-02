import React, { useState, useEffect } from 'react';
import { FiSliders, FiCpu, FiGlobe, FiKey, FiPlay, FiSquare, FiSave, FiRefreshCw, FiEye, FiEyeOff, FiHelpCircle, FiTerminal } from 'react-icons/fi';

export const EhcoClientPage: React.FC = () => {
  const [localPort, setLocalPort] = useState('1080');
  const [remoteURL, setRemoteURL] = useState('');
  const [authToken, setAuthToken] = useState('');
  const [isRunning, setIsRunning] = useState(false);
  const [showToken, setShowToken] = useState(false);
  const [isLoading, setIsLoading] = useState(false);
  const [message, setMessage] = useState<{ type: 'success' | 'error', text: string } | null>(null);

  // Fetch current configs
  const fetchConfig = async () => {
    setIsLoading(true);
    try {
      const token = localStorage.getItem('cc_client_token') || '';
      const response = await fetch('/api/ehco/config', {
        headers: {
          'Authorization': `Bearer ${token}`
        }
      });
      if (response.ok) {
        const data = await response.json();
        if (data.config) {
          setLocalPort(data.config.local_port || '1080');
          setRemoteURL(data.config.remote_url || '');
          setAuthToken(data.config.auth_token || '');
        }
        setIsRunning(data.is_running || false);
      }
    } catch (err) {
      console.error('Failed to fetch ehco client configs', err);
    } finally {
      setIsLoading(false);
    }
  };

  useEffect(() => {
    fetchConfig();
  }, []);

  const handleSave = async (e: React.FormEvent) => {
    e.preventDefault();
    setIsLoading(true);
    setMessage(null);
    try {
      const token = localStorage.getItem('cc_client_token') || '';
      const response = await fetch('/api/ehco/config', {
        method: 'POST',
        headers: {
          'Content-Type': 'application/json',
          'Authorization': `Bearer ${token}`
        },
        body: JSON.stringify({
          local_port: localPort,
          remote_url: remoteURL,
          auth_token: authToken
        })
      });

      if (response.ok) {
        const data = await response.json();
        setMessage({ type: 'success', text: 'Local configurations saved!' });
        if (data.config) {
          setLocalPort(data.config.local_port || '1080');
          setRemoteURL(data.config.remote_url || '');
          setAuthToken(data.config.auth_token || '');
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
      const token = localStorage.getItem('cc_client_token') || '';
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
          text: isRunning ? 'Client SOCKS5 tunnel stopped.' : 'Client SOCKS5 tunnel started successfully!'
        });
      } else {
        const data = await response.json();
        setMessage({ type: 'error', text: data.error || 'Operation failed.' });
      }
    } catch (err: any) {
      setMessage({ type: 'error', text: err.message || 'Failed to communicate with local engine.' });
    } finally {
      setIsLoading(false);
    }
  };

  return (
    <div>
      {/* Title */}
      <div style={{ display: 'flex', justifyContent: 'space-between', alignItems: 'center', marginBottom: 24 }}>
        <div>
          <h1 style={{ fontSize: 22, fontWeight: 700, color: 'var(--color-brand-heading)', margin: 0 }}>Ehco Local Tunnel</h1>
          <p style={{ fontSize: 12, color: 'var(--color-brand-text)', margin: '4px 0 0' }}>
            Configure and launch SOCKS5 tunnel to Clever Cloud using high-performance WebSocket proxy.
          </p>
        </div>
        <button className="btn btn--sm" onClick={fetchConfig} disabled={isLoading}>
          <FiRefreshCw className={isLoading ? 'spin-animation' : ''} style={{ marginRight: 6 }} /> Refresh
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
        
        {/* Left Column - Configurations & Guide */}
        <div style={{ display: 'flex', flexDirection: 'column', gap: 20 }}>
          
          <form onSubmit={handleSave} className="g-card">
            <div style={{ display: 'flex', alignItems: 'center', gap: 8, marginBottom: 18 }}>
              <FiSliders style={{ color: 'var(--color-brand)', fontSize: 18 }} />
              <span style={{ fontSize: 15, fontWeight: 600, color: 'var(--color-brand-heading)' }}>Client SOCKS5 settings</span>
            </div>

            <div style={{ display: 'grid', gridTemplateColumns: '130px 1fr', gap: 16, marginBottom: 16 }}>
              <div>
                <label style={{ display: 'block', fontSize: 11, fontWeight: 600, color: 'var(--color-brand-muted)', marginBottom: 6, textTransform: 'uppercase' }}>Local Port</label>
                <input
                  type="text"
                  value={localPort}
                  onChange={(e) => setLocalPort(e.target.value)}
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
                <label style={{ display: 'block', fontSize: 11, fontWeight: 600, color: 'var(--color-brand-muted)', marginBottom: 6, textTransform: 'uppercase' }}>Remote Clever Server URL</label>
                <input
                  type="text"
                  value={remoteURL}
                  onChange={(e) => setRemoteURL(e.target.value)}
                  placeholder="e.g. wss://your-app.cleverapps.io/tunnel"
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

            <div style={{ marginBottom: 20 }}>
              <label style={{ display: 'block', fontSize: 11, fontWeight: 600, color: 'var(--color-brand-muted)', marginBottom: 6, textTransform: 'uppercase' }}>Auth Token</label>
              <div style={{ position: 'relative' }}>
                <input
                  type={showToken ? 'text' : 'password'}
                  value={authToken}
                  onChange={(e) => setAuthToken(e.target.value)}
                  placeholder="Paste server security token"
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
            </div>

            <button type="submit" className="btn btn--primary" style={{ display: 'flex', alignItems: 'center', gap: 6 }} disabled={isLoading}>
              <FiSave /> Save Settings
            </button>
          </form>

          {/* Quick-Start Guide */}
          <div className="g-card">
            <div style={{ display: 'flex', alignItems: 'center', gap: 8, marginBottom: 16 }}>
              <FiHelpCircle style={{ color: 'var(--color-brand)', fontSize: 18 }} />
              <span style={{ fontSize: 15, fontWeight: 600, color: 'var(--color-brand-heading)' }}>SOCKS5 Proxy Configuration Guide</span>
            </div>

            <p style={{ fontSize: 13, color: 'var(--color-brand-text)', lineHeight: 1.5, margin: '0 0 16px' }}>
              Once the local tunnel engine is <strong>RUNNING</strong>, your computer listens on <code>127.0.0.1:{localPort}</code>. 
              Any internet traffic routed to this port is dynamically forwarded through the Clever Cloud server.
            </p>

            <div style={{ display: 'flex', flexDirection: 'column', gap: 14 }}>
              <div style={{ display: 'flex', gap: 12 }}>
                <div style={{
                  width: 24,
                  height: 24,
                  borderRadius: '50%',
                  background: 'var(--color-brand-light)',
                  color: 'var(--color-brand)',
                  display: 'flex',
                  alignItems: 'center',
                  justifyContent: 'center',
                  fontWeight: 700,
                  fontSize: 12,
                  flexShrink: 0
                }}>1</div>
                <div>
                  <h4 style={{ margin: '0 0 4px', fontSize: 13, color: 'var(--color-brand-heading)' }}>Install Proxy SwitchyOmega</h4>
                  <p style={{ margin: 0, fontSize: 11, color: 'var(--color-brand-text)' }}>
                    Install the extension in Chrome, Edge, or Firefox to manage browser proxying seamlessly.
                  </p>
                </div>
              </div>

              <div style={{ display: 'flex', gap: 12 }}>
                <div style={{
                  width: 24,
                  height: 24,
                  borderRadius: '50%',
                  background: 'var(--color-brand-light)',
                  color: 'var(--color-brand)',
                  display: 'flex',
                  alignItems: 'center',
                  justifyContent: 'center',
                  fontWeight: 700,
                  fontSize: 12,
                  flexShrink: 0
                }}>2</div>
                <div>
                  <h4 style={{ margin: '0 0 4px', fontSize: 13, color: 'var(--color-brand-heading)' }}>Add New Proxy Profile</h4>
                  <p style={{ margin: 0, fontSize: 11, color: 'var(--color-brand-text)' }}>
                    Create a profile called <strong>CleverConnect</strong> with Scheme: <code>SOCKS5</code>, Server: <code>127.0.0.1</code>, Port: <code>{localPort}</code>.
                  </p>
                </div>
              </div>

              <div style={{ display: 'flex', gap: 12 }}>
                <div style={{
                  width: 24,
                  height: 24,
                  borderRadius: '50%',
                  background: 'var(--color-brand-light)',
                  color: 'var(--color-brand)',
                  display: 'flex',
                  alignItems: 'center',
                  justifyContent: 'center',
                  fontWeight: 700,
                  fontSize: 12,
                  flexShrink: 0
                }}>3</div>
                <div>
                  <h4 style={{ margin: '0 0 4px', fontSize: 13, color: 'var(--color-brand-heading)' }}>Toggle & Enjoy</h4>
                  <p style={{ margin: 0, fontSize: 11, color: 'var(--color-brand-text)' }}>
                    Switch your browser mode to <strong>CleverConnect</strong>. All website traffic will route securely over Clever Cloud!
                  </p>
                </div>
              </div>
            </div>
          </div>

        </div>

        {/* Right Column - Status Widget */}
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
              Client Engine
            </div>

            <div style={{ display: 'flex', alignItems: 'center', gap: 6, marginTop: 8, marginBottom: 14 }}>
              <span className="live-dot" style={{ background: isRunning ? 'var(--color-brand-green)' : '#ef4444' }} />
              <span style={{ fontSize: 16, fontWeight: 700, color: 'var(--color-brand-heading)' }}>
                {isRunning ? 'ACTIVE' : 'INACTIVE'}
              </span>
            </div>

            <p style={{ fontSize: 11, color: 'var(--color-brand-text)', lineHeight: 1.4, margin: '0 0 16px' }}>
              {isRunning
                ? `Local machine is capturing traffic on port ${localPort} and forwarding it via WSS.`
                : 'Local SOCKS5 port listener is inactive. Click play to establish websocket handshake.'
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
            >
              {isRunning ? (
                <>
                  <FiSquare /> Stop Tunnel
                </>
              ) : (
                <>
                  <FiPlay /> Start Tunnel
                </>
              )}
            </button>
          </div>

          {/* Connection Stats */}
          <div className="g-card">
            <div style={{ display: 'flex', alignItems: 'center', gap: 6, marginBottom: 12 }}>
              <FiTerminal style={{ color: 'var(--color-brand)', fontSize: 16 }} />
              <span style={{ fontSize: 13, fontWeight: 600, color: 'var(--color-brand-heading)' }}>Active Socket</span>
            </div>
            
            <div style={{ display: 'flex', flexDirection: 'column', gap: 10, fontSize: 11 }}>
              <div>
                <div style={{ color: 'var(--color-brand-muted)', marginBottom: 2 }}>Local Gateway:</div>
                <div style={{ fontWeight: 600, color: 'var(--color-brand-heading)', fontFamily: 'Fira Code' }}>
                  127.0.0.1:{localPort}
                </div>
              </div>
              
              <div>
                <div style={{ color: 'var(--color-brand-muted)', marginBottom: 2 }}>Transport:</div>
                <div style={{ fontWeight: 600, color: 'var(--color-brand-heading)' }}>
                  ws/wss encrypted handshake
                </div>
              </div>
            </div>
          </div>
        </div>

      </div>
    </div>
  );
};
