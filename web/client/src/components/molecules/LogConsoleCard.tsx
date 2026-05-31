import React, { useEffect } from 'react';
import { useLogStore } from '../../store/logStore';
import { useAuthStore } from '../../store/authStore';
import { useNavigate } from 'react-router-dom';
import { FiTerminal } from 'react-icons/fi';

export const LogConsoleCard: React.FC = () => {
  const { logs, connectLogs, isConnected } = useLogStore();
  const { token } = useAuthStore();
  const navigate = useNavigate();

  useEffect(() => {
    if (token) {
      const close = connectLogs(token);
      return () => { close(); };
    }
  }, [connectLogs, token]);

  // Keep only the last 5 logs for this tiny monitoring preview
  const recentLogs = logs.slice(-5);

  const getLevelColor = (level: string) => {
    const lvl = level.toUpperCase();
    if (lvl === 'DEBUG') return '#888ea8';
    if (lvl === 'INFO') return '#22c55e';
    if (lvl === 'WARN') return '#eab308';
    return '#ef4444'; // ERROR/FATAL
  };

  return (
    <div className="g-card" style={{ padding: '18px 20px', display: 'flex', flexDirection: 'column', gap: 12 }}>
      {/* Header */}
      <div style={{ display: 'flex', justifyContent: 'space-between', alignItems: 'center' }}>
        <div style={{ display: 'flex', alignItems: 'center', gap: 8 }}>
          <FiTerminal style={{ color: 'var(--color-brand-muted)', fontSize: 16 }} />
          <div>
            <div style={{ fontSize: 14, fontWeight: 600, color: 'var(--color-brand-heading)' }}>System Console Monitor</div>
            <div style={{ fontSize: 11, color: 'var(--color-brand-text)', display: 'flex', alignItems: 'center', gap: 6 }}>
              <span className="live-dot" style={{ width: 6, height: 6, background: isConnected ? '#22c55e' : '#ef4444' }} />
              {isConnected ? 'Streaming Live' : 'Offline'}
            </div>
          </div>
        </div>
        <span 
          onClick={() => navigate('/fw-logs')}
          style={{ fontSize: 11, fontWeight: 600, color: 'var(--color-brand)', cursor: 'pointer', display: 'flex', alignItems: 'center', gap: 4 }}
        >
          View Diagnostics ↗
        </span>
      </div>

      {/* Sleek Monospace Terminal Preview */}
      <div 
        style={{
          background: 'rgba(0,0,0,0.02)',
          borderRadius: 8,
          border: '1px solid var(--color-brand-border)',
          padding: '10px 14px',
          fontFamily: 'monospace',
          fontSize: 11,
          display: 'flex',
          flexDirection: 'column',
          gap: 6,
          minHeight: 116,
          justifyContent: recentLogs.length === 0 ? 'center' : 'flex-start'
        }}
      >
        {recentLogs.length === 0 ? (
          <div style={{ textAlign: 'center', color: 'var(--color-brand-muted)' }}>
            Awaiting log events...
          </div>
        ) : (
          recentLogs.map((log, i) => (
            <div key={i} style={{ display: 'flex', gap: 8, whiteSpace: 'nowrap', overflow: 'hidden', textOverflow: 'ellipsis' }}>
              <span style={{ color: 'var(--color-brand-muted)', flexShrink: 0 }}>{log.timestamp}</span>
              <span style={{ color: getLevelColor(log.level), fontWeight: 600, flexShrink: 0, width: 44 }}>
                [{log.level.substring(0, 4)}]
              </span>
              <span style={{ color: 'var(--color-brand-text)', flexShrink: 0, fontWeight: 600 }}>
                {log.component}:
              </span>
              <span style={{ color: 'var(--color-brand-heading)', overflow: 'hidden', textOverflow: 'ellipsis' }} title={log.message}>
                {log.message}
              </span>
            </div>
          ))
        )}
      </div>
    </div>
  );
};
