import React from 'react';
import { FiSearch, FiHelpCircle } from 'react-icons/fi';

interface PortScannerCardProps {
  isLoading: boolean;
  probeIP: string;
  setProbeIP: (ip: string) => void;
  probePorts: string;
  setProbePorts: (ports: string) => void;
  probeProto: string;
  setProbeProto: (proto: string) => void;
  probeResults: any[];
  handleProbePorts: () => void;
  showHelp: (title: string, text: string) => void;
}

export const PortScannerCard: React.FC<PortScannerCardProps> = ({
  isLoading,
  probeIP,
  setProbeIP,
  probePorts,
  setProbePorts,
  probeProto,
  setProbeProto,
  probeResults,
  handleProbePorts,
  showHelp,
}) => {
  return (
    <div className="g-card">
      <div style={{ display: 'flex', alignItems: 'center', gap: 8, marginBottom: 14 }}>
        <FiSearch style={{ color: 'var(--color-brand)', fontSize: 16 }} />
        <span style={{ fontSize: 13, fontWeight: 600, color: 'var(--color-brand-heading)' }}>Port Scanning Utility</span>
        <FiHelpCircle
          style={{ cursor: 'pointer', color: 'var(--color-brand-muted)' }}
          onClick={() =>
            showHelp(
              'Port Scanning Utility',
              'Tests connectivity to remote targets. Input IP addresses and comma-separated ports to perform concurrent port opening scans.'
            )
          }
        />
      </div>

      <div style={{ display: 'flex', flexDirection: 'column', gap: 12 }}>
        <div style={{ display: 'grid', gridTemplateColumns: '1fr 90px', gap: 10 }}>
          <input
            type="text"
            placeholder="Target IP / Hostname"
            value={probeIP}
            onChange={(e) => setProbeIP(e.target.value)}
            style={{
              padding: '8px 10px',
              borderRadius: 6,
              border: '1px solid var(--color-brand-border)',
              background: 'var(--color-brand-card)',
              fontSize: 12,
              color: 'var(--color-brand-heading)',
            }}
          />
          <select
            value={probeProto}
            onChange={(e) => setProbeProto(e.target.value)}
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
            <option value="udp">UDP</option>
          </select>
        </div>

        <div style={{ display: 'flex', gap: 10 }}>
          <input
            type="text"
            placeholder="Ports (e.g. 80,443,53)"
            value={probePorts}
            onChange={(e) => setProbePorts(e.target.value)}
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
          <button className="btn btn--primary btn--sm" onClick={handleProbePorts} disabled={isLoading}>
            Scan
          </button>
        </div>

        {probeResults.length > 0 && (
          <div
            style={{
              marginTop: 6,
              background: 'var(--color-brand-bg)',
              padding: 8,
              borderRadius: 6,
              maxHeight: 120,
              overflowY: 'auto',
              border: '1px solid var(--color-brand-border)',
            }}
          >
            {probeResults.map((r, idx) => (
              <div key={idx} style={{ display: 'flex', justifyContent: 'space-between', fontSize: 11, marginBottom: 4 }}>
                <span style={{ color: 'var(--color-brand-heading)', fontFamily: 'Fira Code' }}>
                  Port {r.port} ({r.protocol.toUpperCase()})
                </span>
                <span style={{ fontWeight: 700, color: r.open ? 'var(--color-brand-green)' : 'var(--color-brand-red)' }}>
                  {r.open ? `OPEN (${r.latency_ms}ms)` : 'CLOSED'}
                </span>
              </div>
            ))}
          </div>
        )}
      </div>
    </div>
  );
};

export default PortScannerCard;
