import React from 'react';
import { FiHelpCircle, FiActivity, FiPlay, FiSquare } from 'react-icons/fi';

interface EngineStatusCardProps {
  isRunning: boolean;
  isLoading: boolean;
  socksPort: number;
  httpPort: number;
  speedTestActive: boolean;
  speedTestBreakdown: any;
  handleRunSpeedTest: () => void;
  handleStartCore: () => void;
  handleStopCore: () => void;
  showHelp: (title: string, text: string) => void;
}

export const EngineStatusCard: React.FC<EngineStatusCardProps> = ({
  isRunning,
  isLoading,
  socksPort,
  httpPort,
  speedTestActive,
  speedTestBreakdown,
  handleRunSpeedTest,
  handleStartCore,
  handleStopCore,
  showHelp,
}) => {
  return (
    <div className="g-card" style={{ display: 'flex', flexDirection: 'column', gap: 14 }}>
      <div style={{ display: 'flex', alignItems: 'center', justifyContent: 'space-between' }}>
        <div>
          <div style={{ display: 'flex', alignItems: 'center', gap: 8 }}>
            <span className="live-dot" style={{ background: isRunning ? '#10b981' : '#ef4444' }} />
            <span style={{ fontSize: 15, fontWeight: 700, color: 'var(--color-brand-heading)' }}>
              Core Supervisor: {isRunning ? 'RUNNING' : 'STOPPED'}
            </span>
            <FiHelpCircle
              style={{ cursor: 'pointer', color: 'var(--color-brand-muted)' }}
              onClick={() =>
                showHelp(
                  'V2Ray Core Supervisor',
                  'Manages the local Xray daemon lifecycle in the background. On start, compiles database rules into an optimized JSON config, ensures SOCKS5 and HTTP inbound listeners bind safely without port conflicts, and launches Xray.'
                )
              }
            />
          </div>
          <span style={{ fontSize: 11, color: 'var(--color-brand-text)', display: 'block', marginTop: 4 }}>
            Local inbound captures: SOCKS5 on port {socksPort} & HTTP on port {httpPort}.
          </span>
        </div>

        <div style={{ display: 'flex', gap: 10, alignItems: 'center' }}>
          {isRunning && (
            <button
              onClick={handleRunSpeedTest}
              className="btn btn--sm btn--secondary"
              style={{
                borderColor: 'var(--color-brand)',
                color: 'var(--color-brand)',
                display: 'flex',
                alignItems: 'center',
                gap: 4,
              }}
              disabled={speedTestActive}
            >
              <FiActivity size={13} className={speedTestActive ? 'spin-animation' : ''} />{' '}
              {speedTestActive ? 'Testing speed...' : 'Run Speed Test'}
            </button>
          )}
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

      {speedTestBreakdown && (
        <div
          style={{
            background: 'var(--color-brand-light)',
            border: '1px solid var(--color-brand-border)',
            borderRadius: 8,
            padding: '10px 14px',
            fontSize: 12,
            display: 'flex',
            justifyContent: 'space-between',
            color: 'var(--color-brand-heading)',
          }}
        >
          <div>
            <strong>Throughput Speed:</strong>{' '}
            <span style={{ color: 'var(--color-brand)', fontWeight: 700 }}>
              {speedTestBreakdown.throughput_mbps.toFixed(2)} Mbps
            </span>
          </div>
          <div>
            <strong>TLS Handshake:</strong>{' '}
            <span style={{ fontFamily: 'monospace' }}>{speedTestBreakdown.tls_handshake_ms}ms</span>
          </div>
          <div>
            <strong>TTFB (First Byte):</strong>{' '}
            <span style={{ fontFamily: 'monospace' }}>{speedTestBreakdown.ttfb_ms}ms</span>
          </div>
          <div>
            <strong>TCP Conn:</strong>{' '}
            <span style={{ fontFamily: 'monospace' }}>{speedTestBreakdown.tcp_conn_ms}ms</span>
          </div>
        </div>
      )}
    </div>
  );
};

export default EngineStatusCard;
