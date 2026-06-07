import React from 'react';
import { FiWifi, FiHelpCircle } from 'react-icons/fi';

interface CdnScannerCardProps {
  isLoading: boolean;
  cdnRanges: string;
  setCdnRanges: (ranges: string) => void;
  cdnScannerActive: boolean;
  cdnScanStatus: any;
  handleStartCDNScan: () => void;
  handleStopCDNScan: () => void;
  showHelp: (title: string, text: string) => void;
}

export const CdnScannerCard: React.FC<CdnScannerCardProps> = ({
  isLoading,
  cdnRanges,
  setCdnRanges,
  cdnScannerActive,
  cdnScanStatus,
  handleStartCDNScan,
  handleStopCDNScan,
  showHelp,
}) => {
  return (
    <div className="g-card" style={{ display: 'flex', flexDirection: 'column', gap: 16 }}>
      <div style={{ display: 'flex', alignItems: 'center', justifyContent: 'space-between' }}>
        <div style={{ display: 'flex', alignItems: 'center', gap: 8 }}>
          <FiWifi style={{ color: 'var(--color-brand)', fontSize: 18 }} />
          <span style={{ fontSize: 15, fontWeight: 600, color: 'var(--color-brand-heading)' }}>
            CDN IP Auto-Scanner & Optimizer
          </span>
          <FiHelpCircle
            style={{ cursor: 'pointer', color: 'var(--color-brand-muted)' }}
            onClick={() =>
              showHelp(
                'CDN IP Scanner',
                'Performs massively parallel port scans and throughput speed tests on CDN edge ranges (e.g. Cloudflare) to auto-discover clean, high-performance IP addresses and inject them as working configurations.'
              )
            }
          />
        </div>
        <div style={{ display: 'flex', gap: 8 }}>
          <button
            className="btn btn--sm btn--primary"
            onClick={handleStartCDNScan}
            disabled={isLoading || cdnScannerActive}
          >
            Start Scan
          </button>
          <button
            className="btn btn--sm btn--secondary"
            onClick={handleStopCDNScan}
            style={{ borderColor: '#ef4444', color: '#ef4444' }}
            disabled={!cdnScannerActive}
          >
            Stop Scan
          </button>
        </div>
      </div>

      <div>
        <label
          style={{
            display: 'block',
            fontSize: 11,
            fontWeight: 600,
            color: 'var(--color-brand-muted)',
            marginBottom: 6,
            textTransform: 'uppercase',
          }}
        >
          Target IP CIDR Ranges (One per line)
        </label>
        <textarea
          value={cdnRanges}
          onChange={(e) => setCdnRanges(e.target.value)}
          placeholder="104.16.0.0/16&#10;172.64.0.0/13"
          rows={3}
          style={{
            width: '100%',
            padding: '8px 12px',
            borderRadius: 8,
            border: '1px solid var(--color-brand-border)',
            background: 'var(--color-brand-card)',
            fontSize: 12,
            fontFamily: 'monospace',
            color: 'var(--color-brand-heading)',
          }}
        />
      </div>

      {cdnScanStatus && (
        <div
          style={{
            background: 'var(--color-brand-bg)',
            border: '1px solid var(--color-brand-border)',
            borderRadius: 8,
            padding: 12,
            fontSize: 12,
          }}
        >
          <div style={{ display: 'grid', gridTemplateColumns: '1fr 1fr', gap: 10, marginBottom: 10 }}>
            <div>
              <strong>State:</strong>{' '}
              <span style={{ color: 'var(--color-brand)', fontWeight: 700 }}>
                {cdnScanStatus.is_running ? 'SCANNING' : 'IDLE / FINISHED'}
              </span>
            </div>
            <div>
              <strong>Scanned Count:</strong> {cdnScanStatus.scanned_count || 0}
            </div>
            <div>
              <strong>Live IPs Found:</strong> {cdnScanStatus.live_count || 0}
            </div>
            <div>
              <strong>Active Workers:</strong> {cdnScanStatus.workers_active || 0}
            </div>
          </div>

          {cdnScanStatus.top_results && cdnScanStatus.top_results.length > 0 && (
            <div>
              <span
                style={{
                  fontSize: 11,
                  fontWeight: 700,
                  color: 'var(--color-brand-heading)',
                  display: 'block',
                  marginBottom: 6,
                }}
              >
                Top Discovered CDN IPs
              </span>
              <div
                style={{
                  maxHeight: 120,
                  overflowY: 'auto',
                  border: '1px solid var(--color-brand-border)',
                  borderRadius: 6,
                }}
              >
                <table style={{ width: '100%', borderCollapse: 'collapse', fontSize: 11 }}>
                  <thead>
                    <tr style={{ background: 'var(--color-brand-bg)', borderBottom: '1px solid var(--color-brand-border)' }}>
                      <th style={{ padding: '6px 8px', textAlign: 'left' }}>IP Address</th>
                      <th style={{ padding: '6px 8px', textAlign: 'left' }}>Ping (RTT)</th>
                      <th style={{ padding: '6px 8px', textAlign: 'left' }}>Speed</th>
                    </tr>
                  </thead>
                  <tbody>
                    {cdnScanStatus.top_results.map((res: any, idx: number) => (
                      <tr key={idx} style={{ borderBottom: '1px solid var(--color-brand-border)' }}>
                        <td style={{ padding: '6px 8px', fontFamily: 'monospace' }}>{res.ip}</td>
                        <td style={{ padding: '6px 8px', color: 'var(--color-brand-green)', fontWeight: 700 }}>
                          {res.ping_ms}ms
                        </td>
                        <td style={{ padding: '6px 8px' }}>{res.speed_mbps.toFixed(2)} Mbps</td>
                      </tr>
                    ))}
                  </tbody>
                </table>
              </div>
            </div>
          )}
        </div>
      )}
    </div>
  );
};

export default CdnScannerCard;
