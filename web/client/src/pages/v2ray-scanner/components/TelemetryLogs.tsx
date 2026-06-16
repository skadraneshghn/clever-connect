import React, { useEffect, useRef } from 'react';
import { FiTerminal, FiTrash2 } from 'react-icons/fi';

interface TelemetryLogsProps {
  scannerLogs: string[];
  logsFilter: string;
  setLogsFilter: (v: string) => void;
  setScannerLogs: React.Dispatch<React.SetStateAction<string[]>>;
}

export const TelemetryLogs: React.FC<TelemetryLogsProps> = ({
  scannerLogs,
  logsFilter,
  setLogsFilter,
  setScannerLogs,
}) => {
  const logsContainerRef = useRef<HTMLDivElement | null>(null);

  // Auto scroll logic for logs terminal
  useEffect(() => {
    if (logsContainerRef.current) {
      logsContainerRef.current.scrollTop = logsContainerRef.current.scrollHeight;
    }
  }, [scannerLogs]);

  // Filtering logs
  const filteredLogs = scannerLogs.filter((log) => {
    return log.toLowerCase().includes(logsFilter.toLowerCase());
  });

  return (
    <div className="g-card" style={{ padding: 20, display: 'flex', flexDirection: 'column', height: 280 }}>
      <div style={{ display: 'flex', alignItems: 'center', justifyContent: 'space-between', marginBottom: 12 }}>
        <div style={{ display: 'flex', alignItems: 'center', gap: 6 }}>
          <FiTerminal style={{ color: 'var(--color-brand)', fontSize: 16 }} />
          <span style={{ fontSize: 13, fontWeight: 600, color: 'var(--color-brand-heading)' }}>
            Live Diagnostic Scanner Logs
          </span>
        </div>
        <div style={{ display: 'flex', alignItems: 'center', gap: 8 }}>
          <input
            type="text"
            placeholder="Filter logs..."
            value={logsFilter}
            onChange={(e) => setLogsFilter(e.target.value)}
            style={{
              width: 120,
              padding: '4px 8px',
              borderRadius: 6,
              border: '1px solid var(--color-brand-border)',
              background: 'var(--color-brand-bg)',
              fontSize: 11,
              color: 'var(--color-brand-heading)',
              outline: 'none'
            }}
          />
          <button
            className="btn btn--sm btn--secondary"
            onClick={() => setScannerLogs([])}
            title="Clear logs"
            style={{ display: 'flex', alignItems: 'center', gap: 4 }}
          >
            <FiTrash2 size={12} /> Clear
          </button>
        </div>
      </div>

      {/* Monospace terminal body */}
      <div
        ref={logsContainerRef}
        style={{
          flex: 1,
          background: 'var(--color-brand-bg)',
          border: '1px solid var(--color-brand-border)',
          borderRadius: 8,
          padding: 12,
          fontFamily: 'Fira Code, Courier, monospace',
          fontSize: 11,
          color: 'var(--color-brand-text)',
          overflowY: 'auto',
          display: 'flex',
          flexDirection: 'column',
          gap: 4
        }}
      >
        {filteredLogs.length === 0 ? (
          <div style={{ color: 'var(--color-brand-muted)', textAlign: 'center', marginTop: 70 }}>
            No diagnostic logs. Click "Start Sweep" to stream live.
          </div>
        ) : (
          filteredLogs.map((log, idx) => {
            let color = 'var(--color-brand-text)';
            if (log.includes('[ERROR]') || log.includes('Critical:') || log.includes('Failed candidate:')) {
              color = 'var(--color-brand-red)';
            } else if (log.includes('Healthy candidate:') || log.includes('Success') || log.includes('clean node')) {
              color = 'var(--color-brand-green)';
            } else if (log.includes('Initiating') || log.includes('Parameters:')) {
              color = 'var(--color-brand)';
            }

            return (
              <div key={idx} style={{ wordBreak: 'break-all', whiteSpace: 'pre-wrap', color }}>
                {log}
              </div>
            );
          })
        )}
      </div>
    </div>
  );
};
