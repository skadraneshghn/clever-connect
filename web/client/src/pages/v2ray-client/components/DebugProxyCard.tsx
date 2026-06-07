import React from 'react';
import { FiZap, FiHelpCircle } from 'react-icons/fi';

interface DebugProxyCardProps {
  isLoading: boolean;
  debugProxyPort: number;
  setDebugProxyPort: (port: number) => void;
  isDebugProxyActive: boolean;
  debugProxyLogs: string[];
  handleToggleDebugProxy: () => void;
  showHelp: (title: string, text: string) => void;
}

export const DebugProxyCard: React.FC<DebugProxyCardProps> = ({
  isLoading,
  debugProxyPort,
  setDebugProxyPort,
  isDebugProxyActive,
  debugProxyLogs,
  handleToggleDebugProxy,
  showHelp,
}) => {
  return (
    <div className="g-card">
      <div style={{ display: 'flex', alignItems: 'center', justifyContent: 'space-between', marginBottom: 12 }}>
        <div style={{ display: 'flex', alignItems: 'center', gap: 6 }}>
          <FiZap style={{ color: 'var(--color-brand)', fontSize: 16 }} />
          <span style={{ fontSize: 13, fontWeight: 600, color: 'var(--color-brand-heading)' }}>
            Local Interception Proxy
          </span>
          <FiHelpCircle
            style={{ cursor: 'pointer', color: 'var(--color-brand-muted)' }}
            onClick={() =>
              showHelp(
                'Local Interception Proxy',
                'Fires up a local HTTP/HTTPS CONNECT tunnel proxy. Captures outgoing application connections and logs transaction methods/hosts for diagnostic auditing.'
              )
            }
          />
        </div>
        <span
          style={{
            fontSize: 10,
            fontWeight: 700,
            color: isDebugProxyActive ? 'var(--color-brand-green)' : 'var(--color-brand-red)',
          }}
        >
          {isDebugProxyActive ? 'RUNNING' : 'INACTIVE'}
        </span>
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
              color: 'var(--color-brand-heading)',
            }}
          />
          <button
            className="btn btn--sm"
            onClick={handleToggleDebugProxy}
            style={{
              background: isDebugProxyActive ? '#ef4444' : 'var(--color-brand)',
              color: '#fff',
              border: 'none',
              fontWeight: 600,
            }}
          >
            {isDebugProxyActive ? 'Stop' : 'Start'}
          </button>
        </div>

        {isDebugProxyActive && debugProxyLogs.length > 0 && (
          <div
            style={{
              maxHeight: 120,
              overflowY: 'auto',
              background: '#1a1a2e',
              borderRadius: 6,
              padding: 8,
              fontFamily: 'Fira Code',
              fontSize: 9,
              color: '#a9b1d6',
            }}
          >
            {debugProxyLogs.map((l, idx) => (
              <div key={idx} style={{ marginBottom: 2 }}>
                {l}
              </div>
            ))}
          </div>
        )}
      </div>
    </div>
  );
};

export default DebugProxyCard;
