import React from 'react';
import { FiSliders, FiHelpCircle } from 'react-icons/fi';

interface InboundEndpointsCardProps {
  isLoading: boolean;
  inbounds: any[];
  inboundTag: string;
  setInboundTag: (tag: string) => void;
  inboundProtocol: string;
  setInboundProtocol: (proto: string) => void;
  inboundPort: number;
  setInboundPort: (port: number) => void;
  inboundNetwork: string;
  setInboundNetwork: (network: string) => void;
  inboundTlsMode: string;
  setInboundTlsMode: (tlsMode: string) => void;
  inboundSNI: string;
  setInboundSNI: (sni: string) => void;
  inboundFallback: string;
  setInboundFallback: (fallback: string) => void;
  handleAddInbound: (e: React.FormEvent) => void;
  showHelp: (title: string, text: string) => void;
}

export const InboundEndpointsCard: React.FC<InboundEndpointsCardProps> = ({
  isLoading,
  inbounds,
  inboundTag,
  setInboundTag,
  inboundProtocol,
  setInboundProtocol,
  inboundPort,
  setInboundPort,
  inboundNetwork,
  setInboundNetwork,
  inboundTlsMode,
  setInboundTlsMode,
  inboundSNI,
  setInboundSNI,
  inboundFallback,
  setInboundFallback,
  handleAddInbound,
  showHelp,
}) => {
  return (
    <div className="g-card">
      <div style={{ display: 'flex', alignItems: 'center', gap: 8, marginBottom: 16 }}>
        <FiSliders style={{ color: 'var(--color-brand)', fontSize: 18 }} />
        <span style={{ fontSize: 15, fontWeight: 600, color: 'var(--color-brand-heading)' }}>
          Server Inbound Endpoints
        </span>
        <FiHelpCircle
          style={{ cursor: 'pointer', color: 'var(--color-brand-muted)' }}
          onClick={() =>
            showHelp(
              'Server Inbound Endpoints',
              'Configures proxy listening rules on specific ports. Reality TLS mode intercepts TLS handshakes using target SNIs (like yahoo.com) to completely disguise the proxy from active probes.'
            )
          }
        />
      </div>

      {/* List Inbounds */}
      <div style={{ overflowX: 'auto', border: '1px solid var(--color-brand-border)', borderRadius: 8, marginBottom: 20 }}>
        <table style={{ width: '100%', borderCollapse: 'collapse', fontSize: 11, textAlign: 'left' }}>
          <thead>
            <tr style={{ background: 'var(--color-brand-bg)', borderBottom: '1px solid var(--color-brand-border)' }}>
              <th style={{ padding: '8px 10px', color: 'var(--color-brand-heading)' }}>Tag</th>
              <th style={{ padding: '8px 10px', color: 'var(--color-brand-heading)' }}>Protocol</th>
              <th style={{ padding: '8px 10px', color: 'var(--color-brand-heading)' }}>Port</th>
              <th style={{ padding: '8px 10px', color: 'var(--color-brand-heading)' }}>Transport</th>
              <th style={{ padding: '8px 10px', color: 'var(--color-brand-heading)' }}>TLS Mode</th>
              <th style={{ padding: '8px 10px', color: 'var(--color-brand-heading)' }}>Fallback</th>
            </tr>
          </thead>
          <tbody>
            {inbounds.length === 0 ? (
              <tr>
                <td colSpan={6} style={{ padding: 14, textAlign: 'center', color: 'var(--color-brand-muted)' }}>
                  No inbound rules registered.
                </td>
              </tr>
            ) : (
              inbounds.map((inb) => (
                <tr key={inb.ID} style={{ borderBottom: '1px solid var(--color-brand-border)' }}>
                  <td style={{ padding: '8px 10px', fontWeight: 600, color: 'var(--color-brand-heading)' }}>{inb.tag}</td>
                  <td style={{ padding: '8px 10px', textTransform: 'uppercase', color: 'var(--color-brand)' }}>
                    {inb.protocol}
                  </td>
                  <td style={{ padding: '8px 10px' }}>{inb.port}</td>
                  <td style={{ padding: '8px 10px' }}>{inb.network}</td>
                  <td style={{ padding: '8px 10px', textTransform: 'uppercase' }}>{inb.tls_mode}</td>
                  <td style={{ padding: '8px 10px', color: 'var(--color-brand-text)' }}>{inb.fallback_dest}</td>
                </tr>
              ))
            )}
          </tbody>
        </table>
      </div>

      {/* Form: Add Inbound */}
      <form
        onSubmit={handleAddInbound}
        style={{
          borderTop: '1px solid var(--color-brand-border)',
          paddingTop: 16,
          display: 'flex',
          flexDirection: 'column',
          gap: 12,
        }}
      >
        <span style={{ fontSize: 12, fontWeight: 700, color: 'var(--color-brand-heading)' }}>Deploy New Inbound</span>

        <div style={{ display: 'grid', gridTemplateColumns: '1fr 1fr', gap: 12 }}>
          <input
            type="text"
            placeholder="Inbound Tag (e.g. vless-reality-443)"
            value={inboundTag}
            onChange={(e) => setInboundTag(e.target.value)}
            style={{
              padding: '8px 10px',
              borderRadius: 6,
              border: '1px solid var(--color-brand-border)',
              background: 'var(--color-brand-card)',
              fontSize: 12,
              color: 'var(--color-brand-heading)',
            }}
            required
          />
          <select
            value={inboundProtocol}
            onChange={(e) => setInboundProtocol(e.target.value)}
            style={{
              padding: '8px 10px',
              borderRadius: 6,
              border: '1px solid var(--color-brand-border)',
              background: 'var(--color-brand-card)',
              fontSize: 12,
              color: 'var(--color-brand-heading)',
            }}
          >
            <option value="vless">VLESS</option>
            <option value="vmess">VMess</option>
            <option value="trojan">Trojan</option>
            <option value="shadowsocks">Shadowsocks</option>
          </select>
        </div>

        <div style={{ display: 'grid', gridTemplateColumns: '80px 1fr 1fr', gap: 12 }}>
          <input
            type="number"
            placeholder="Port"
            value={inboundPort}
            onChange={(e) => setInboundPort(Number(e.target.value))}
            style={{
              padding: '8px 10px',
              borderRadius: 6,
              border: '1px solid var(--color-brand-border)',
              background: 'var(--color-brand-card)',
              fontSize: 12,
              color: 'var(--color-brand-heading)',
            }}
            required
          />
          <select
            value={inboundNetwork}
            onChange={(e) => setInboundNetwork(e.target.value)}
            style={{
              padding: '8px 10px',
              borderRadius: 6,
              border: '1px solid var(--color-brand-border)',
              background: 'var(--color-brand-card)',
              fontSize: 12,
              color: 'var(--color-brand-heading)',
            }}
          >
            <option value="tcp">TCP</option>
            <option value="ws">WebSocket</option>
            <option value="grpc">gRPC</option>
          </select>
          <select
            value={inboundTlsMode}
            onChange={(e) => setInboundTlsMode(e.target.value)}
            style={{
              padding: '8px 10px',
              borderRadius: 6,
              border: '1px solid var(--color-brand-border)',
              background: 'var(--color-brand-card)',
              fontSize: 12,
              color: 'var(--color-brand-heading)',
            }}
          >
            <option value="reality">Reality</option>
            <option value="tls">Standard TLS</option>
            <option value="none">None</option>
          </select>
        </div>

        <div style={{ display: 'grid', gridTemplateColumns: '1fr 1fr', gap: 12 }}>
          <input
            type="text"
            placeholder="Target SNI (Reality)"
            value={inboundSNI}
            onChange={(e) => setInboundSNI(e.target.value)}
            style={{
              padding: '8px 10px',
              borderRadius: 6,
              border: '1px solid var(--color-brand-border)',
              background: 'var(--color-brand-card)',
              fontSize: 12,
              color: 'var(--color-brand-heading)',
            }}
          />
          <input
            type="text"
            placeholder="Fallback Destination"
            value={inboundFallback}
            onChange={(e) => setInboundFallback(e.target.value)}
            style={{
              padding: '8px 10px',
              borderRadius: 6,
              border: '1px solid var(--color-brand-border)',
              background: 'var(--color-brand-card)',
              fontSize: 12,
              color: 'var(--color-brand-heading)',
            }}
          />
        </div>

        <button type="submit" className="btn btn--sm btn--primary" disabled={isLoading}>
          Deploy Inbound
        </button>
      </form>
    </div>
  );
};

export default InboundEndpointsCard;
