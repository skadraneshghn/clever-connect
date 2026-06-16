import React, { useMemo, useRef } from 'react';
import { useVirtualizer } from '@tanstack/react-virtual';
import {
  FiList, FiSearch, FiClipboard, FiFileText, FiDownload
} from 'react-icons/fi';
import { IPResolveBadge } from '../../../components/atoms/IPResolveBadge';

export interface ScannedCandidate {
  ip: string;
  port: number;
  protocol: string;
  latencyMs: number;
  speedMbps: number;
  packetLoss: number;
  cdnProvider: string;
  popLocation: string;
  status: 'healthy' | 'failed' | 'in_flight';
  time: string;
}

interface CandidateTableProps {
  candidates: ScannedCandidate[];
  searchQuery: string;
  setSearchQuery: (v: string) => void;
  sortBy: 'latency' | 'speed' | 'time';
  setSortBy: (v: 'latency' | 'speed' | 'time') => void;
  onCopyAll: () => void;
  onDownloadTxt: () => void;
  onDownloadCsv: () => void;
  onCopyItem: (ip: string, port: number) => void;
}

export const CandidateTable: React.FC<CandidateTableProps> = ({
  candidates,
  searchQuery,
  setSearchQuery,
  sortBy,
  setSortBy,
  onCopyAll,
  onDownloadTxt,
  onDownloadCsv,
  onCopyItem,
}) => {
  const parentRef = useRef<HTMLDivElement>(null);

  // Filtering and sorting logic for candidates (memoized for performance)
  const filteredCandidates = useMemo(() => {
    return candidates
      .filter((c) => {
        const q = searchQuery.toLowerCase().trim();
        return c.ip.toLowerCase().includes(q) || c.port.toString().includes(q);
      })
      .sort((a, b) => {
        if (sortBy === 'latency') {
          return a.latencyMs - b.latencyMs;
        } else if (sortBy === 'speed') {
          return b.speedMbps - a.speedMbps;
        } else if (sortBy === 'time') {
          // Put newer ones first if they're timestamps, but let's do alphabetical/simple sort
          return b.time.localeCompare(a.time);
        }
        return 0;
      });
  }, [candidates, searchQuery, sortBy]);

  // Set up the virtualizer for performance rendering
  const virtualizer = useVirtualizer({
    count: filteredCandidates.length,
    getScrollElement: () => parentRef.current,
    estimateSize: () => 38,
    overscan: 10,
  });

  return (
    <div className="g-card" style={{ padding: 20, display: 'flex', flexDirection: 'column', minHeight: 320, maxHeight: 420 }}>
      <div style={{ display: 'flex', justifyContent: 'space-between', alignItems: 'center', marginBottom: 16, gap: 16, flexWrap: 'wrap' }}>
        <div style={{ display: 'flex', alignItems: 'center', gap: 6 }}>
          <FiList style={{ color: 'var(--color-brand)', fontSize: 16 }} />
          <span style={{ fontSize: 14, fontWeight: 600, color: 'var(--color-brand-heading)' }}>
            Discovered Proxy Candidates ({filteredCandidates.length})
          </span>
        </div>

        <div style={{ display: 'flex', alignItems: 'center', gap: 8, flexWrap: 'wrap' }}>
          <div style={{ position: 'relative', width: 140 }}>
            <FiSearch style={{ position: 'absolute', left: 8, top: 8, color: 'var(--color-brand-muted)', fontSize: 12 }} />
            <input
              type="text"
              value={searchQuery}
              onChange={(e) => setSearchQuery(e.target.value)}
              placeholder="Search..."
              style={{
                width: '100%',
                padding: '5px 10px 5px 26px',
                borderRadius: 6,
                border: '1px solid var(--color-brand-border)',
                background: 'var(--color-brand-bg)',
                color: 'var(--color-brand-heading)',
                fontSize: 11,
                outline: 'none'
              }}
            />
          </div>

          <select
            value={sortBy}
            onChange={(e: any) => setSortBy(e.target.value)}
            style={{
              padding: '5px 8px',
              borderRadius: 6,
              border: '1px solid var(--color-brand-border)',
              background: 'var(--color-brand-bg)',
              color: 'var(--color-brand-heading)',
              fontSize: 11,
              outline: 'none'
            }}
          >
            <option value="latency">Latency</option>
            <option value="speed">Speed</option>
            <option value="time">Time Added</option>
          </select>

          <button
            className="btn btn--sm btn--secondary"
            onClick={onCopyAll}
            title="Copy verified hosts (shortcut: C)"
          >
            <FiClipboard /> Copy
          </button>

          <button
            className="btn btn--sm btn--secondary"
            onClick={onDownloadTxt}
            title="Export healthy results to TXT file"
          >
            <FiFileText /> TXT
          </button>

          <button
            className="btn btn--sm btn--secondary"
            onClick={onDownloadCsv}
            title="Export healthy results to CSV file"
          >
            <FiDownload /> CSV
          </button>
        </div>
      </div>

      {/* Candidates Table (Virtualized Scrollable Box) */}
      <div ref={parentRef} style={{ flex: 1, overflowY: 'auto', border: '1px solid var(--color-brand-border)', borderRadius: 8 }}>
        <table style={{ width: '100%', borderCollapse: 'collapse', fontSize: 12, textAlign: 'left' }}>
          <thead style={{ position: 'sticky', top: 0, zIndex: 1, background: 'var(--color-brand-bg)', boxShadow: 'inset 0 -1px 0 var(--color-brand-border)' }}>
            <tr style={{ borderBottom: '1px solid var(--color-brand-border)' }}>
              <th style={{ padding: '8px 6px', color: 'var(--color-brand-muted)', fontWeight: 600 }}>Endpoint IP</th>
              <th style={{ padding: '8px 6px', color: 'var(--color-brand-muted)', fontWeight: 600 }}>Port</th>
              <th style={{ padding: '8px 6px', color: 'var(--color-brand-muted)', fontWeight: 600 }}>CDN Provider / POP</th>
              <th style={{ padding: '8px 6px', color: 'var(--color-brand-muted)', fontWeight: 600 }}>Latency</th>
              <th style={{ padding: '8px 6px', color: 'var(--color-brand-muted)', fontWeight: 600 }}>Packet Loss</th>
              <th style={{ padding: '8px 6px', color: 'var(--color-brand-muted)', fontWeight: 600 }}>Speed</th>
              <th style={{ padding: '8px 6px', color: 'var(--color-brand-muted)', fontWeight: 600 }}>Status</th>
              <th style={{ padding: '8px 6px', color: 'var(--color-brand-muted)', fontWeight: 600, textAlign: 'right' }}>Actions</th>
            </tr>
          </thead>
          <tbody>
            {virtualizer.getVirtualItems()[0]?.start > 0 && (
              <tr style={{ height: virtualizer.getVirtualItems()[0].start }} />
            )}
            
            {virtualizer.getVirtualItems().map((virtualRow) => {
              const c = filteredCandidates[virtualRow.index];
              return (
                <tr 
                  key={virtualRow.index} 
                  style={{ 
                    height: virtualRow.size, 
                    borderBottom: '1px solid var(--color-brand-border)', 
                    verticalAlign: 'middle' 
                  }}
                  className="hover:bg-[var(--color-brand-bg)]"
                >
                  <td style={{ padding: '8px 6px', fontWeight: 600, color: 'var(--color-brand-heading)' }}>
                    <IPResolveBadge ip={c.ip} />
                  </td>
                  <td style={{ padding: '8px 6px', color: 'var(--color-brand-heading)' }}>{c.port}</td>
                  <td style={{ padding: '8px 6px', color: 'var(--color-brand-heading)' }}>
                    {c.cdnProvider ? (
                      <div style={{ display: 'flex', alignItems: 'center', gap: 6 }}>
                        <span style={{ fontSize: 11, fontWeight: 700, color: 'var(--color-brand-indigo)' }}>{c.cdnProvider}</span>
                        {c.popLocation && (
                          <span style={{ fontSize: 9, fontWeight: 800, padding: '1px 4px', background: 'var(--color-brand-light)', border: '1px solid var(--color-brand-border)', borderRadius: 4, color: 'var(--color-brand)' }}>
                            {c.popLocation}
                          </span>
                        )}
                      </div>
                    ) : (
                      <span style={{ color: 'var(--color-brand-muted)', fontSize: 11 }}>-</span>
                    )}
                  </td>
                  <td style={{ padding: '8px 6px', fontWeight: 600, color: c.latencyMs > 0 ? 'var(--color-brand-green)' : 'var(--color-brand-red)' }}>
                    {c.latencyMs > 0 ? `${c.latencyMs} ms` : '-'}
                  </td>
                  <td style={{ padding: '8px 6px' }}>
                    {c.status === 'in_flight' ? (
                      <span style={{ color: 'var(--color-brand-muted)', fontSize: 11 }}>Testing...</span>
                    ) : (
                      <div style={{ display: 'flex', alignItems: 'center', gap: 6 }}>
                        <div style={{ flex: 1, minWidth: 40, height: 6, background: 'var(--color-brand-bg)', borderRadius: 3, overflow: 'hidden', border: '1px solid var(--color-brand-border)' }}>
                          <div style={{ width: `${c.packetLoss}%`, height: '100%', background: c.packetLoss > 50 ? 'var(--color-brand-red)' : c.packetLoss > 0 ? '#f59e0b' : 'var(--color-brand-green)' }} />
                        </div>
                        <span style={{ fontSize: 10, fontWeight: 600, color: c.packetLoss > 0 ? 'var(--color-brand-red)' : 'var(--color-brand-text)' }}>
                          {c.packetLoss}%
                        </span>
                      </div>
                    )}
                  </td>
                  <td style={{ padding: '8px 6px', color: 'var(--color-brand-blue)', fontWeight: 600 }}>
                    {c.speedMbps > 0 ? `${c.speedMbps.toFixed(2)} MB/s` : '-'}
                  </td>
                  <td style={{ padding: '8px 6px' }}>
                    <span style={{
                      fontSize: 9,
                      fontWeight: 600,
                      padding: '3px 8px',
                      borderRadius: 12,
                      background: c.status === 'healthy' ? 'rgba(34,197,94,0.1)' : c.status === 'failed' ? 'rgba(239,68,68,0.1)' : 'rgba(59,130,246,0.1)',
                      color: c.status === 'healthy' ? 'var(--color-brand-green)' : c.status === 'failed' ? 'var(--color-brand-red)' : 'var(--color-brand-blue)',
                    }}>
                      {c.status.toUpperCase()}
                    </span>
                  </td>
                  <td style={{ padding: '8px 6px', textAlign: 'right' }}>
                    <button
                      className="btn btn--xs btn--secondary"
                      onClick={() => onCopyItem(c.ip, c.port)}
                      style={{ padding: '3px 6px', fontSize: 10 }}
                    >
                      Copy IP
                    </button>
                  </td>
                </tr>
              );
            })}

            {virtualizer.getVirtualItems().length > 0 && (
              <tr
                style={{
                  height:
                    virtualizer.getTotalSize() -
                    virtualizer.getVirtualItems()[virtualizer.getVirtualItems().length - 1].end,
                }}
              />
            )}

            {filteredCandidates.length === 0 && (
              <tr>
                <td colSpan={8} style={{ padding: 30, textAlign: 'center', color: 'var(--color-brand-muted)' }}>
                  <FiFileText size={20} style={{ marginBottom: 8, opacity: 0.3, display: 'inline-block' }} />
                  <div>No candidates found.</div>
                </td>
              </tr>
            )}
          </tbody>
        </table>
      </div>
    </div>
  );
};
