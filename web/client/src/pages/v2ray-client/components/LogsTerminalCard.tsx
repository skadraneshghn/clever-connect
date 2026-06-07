import React from 'react';
import { FiTerminal } from 'react-icons/fi';

interface LogsTerminalCardProps {
  logs: string[];
  logsQuery: string;
  setLogsQuery: (query: string) => void;
  logsContainerRef: React.RefObject<HTMLDivElement | null>;
}

export const LogsTerminalCard: React.FC<LogsTerminalCardProps> = ({
  logs,
  logsQuery,
  setLogsQuery,
  logsContainerRef,
}) => {
  return (
    <div className="g-card" style={{ display: 'flex', flexDirection: 'column', height: 320 }}>
      <div style={{ display: 'flex', alignItems: 'center', justifyContent: 'space-between', marginBottom: 12 }}>
        <div style={{ display: 'flex', alignItems: 'center', gap: 6 }}>
          <FiTerminal style={{ color: 'var(--color-brand)', fontSize: 16 }} />
          <span style={{ fontSize: 13, fontWeight: 600, color: 'var(--color-brand-heading)' }}>Live Core Logs</span>
        </div>
        <input
          type="text"
          placeholder="Filter logs..."
          value={logsQuery}
          onChange={(e) => setLogsQuery(e.target.value)}
          style={{
            width: 120,
            padding: '4px 8px',
            borderRadius: 4,
            border: '1px solid var(--color-brand-border)',
            background: 'var(--color-brand-card)',
            fontSize: 11,
            color: 'var(--color-brand-heading)',
          }}
        />
      </div>

      <div
        ref={logsContainerRef}
        style={{
          flex: 1,
          background: '#1a1a2e',
          borderRadius: 8,
          padding: 10,
          fontFamily: 'Fira Code, monospace',
          fontSize: 10,
          color: '#a9b1d6',
          overflowY: 'auto',
          display: 'flex',
          flexDirection: 'column',
          gap: 4,
        }}
      >
        {logs.length === 0 ? (
          <div style={{ color: '#565f89', textAlign: 'center', marginTop: 80 }}>Listening for core logs...</div>
        ) : (
          logs.map((log, idx) => (
            <div key={idx} style={{ wordBreak: 'break-all', whiteSpace: 'pre-wrap' }}>
              {log}
            </div>
          ))
        )}
      </div>
    </div>
  );
};

export default LogsTerminalCard;
