import React, { useEffect, useState, useRef } from 'react';
import { useLogStore, type LogMessage } from '../store/logStore';
import { useAuthStore } from '../store/authStore';
import { FiDownload, FiTrash2, FiPlay, FiPause, FiSearch, FiChevronDown, FiChevronRight, FiGrid, FiActivity } from 'react-icons/fi';

export const LogsPage: React.FC = () => {
  const { logs, isPaused, isConnected, connectLogs, clearLogs, togglePause } = useLogStore();
  const { token } = useAuthStore();

  const [search, setSearch] = useState('');
  const [levelFilter, setLevelFilter] = useState<string>('ALL');
  const [compFilter, setCompFilter] = useState<string>('ALL');
  const [expandedId, setExpandedId] = useState<number | null>(null);
  const [autoscroll, setAutoscroll] = useState(true);

  const logsEndRef = useRef<HTMLDivElement>(null);
  const logContainerRef = useRef<HTMLDivElement>(null);

  // Initialize logs stream WebSocket connection
  useEffect(() => {
    if (token) {
      const close = connectLogs(token);
      return () => { close(); };
    }
  }, [connectLogs, token]);

  // Handle Autoscroll
  useEffect(() => {
    if (autoscroll && logsEndRef.current) {
      logsEndRef.current.scrollIntoView({ behavior: 'smooth' });
    }
  }, [logs, autoscroll]);

  // Filter logs locally
  const filteredLogs = logs.filter((log) => {
    const matchesSearch = log.message.toLowerCase().includes(search.toLowerCase()) || 
                          log.component.toLowerCase().includes(search.toLowerCase()) ||
                          log.raw.toLowerCase().includes(search.toLowerCase());
    
    const matchesLevel = levelFilter === 'ALL' || log.level.toUpperCase() === levelFilter;
    const matchesComp = compFilter === 'ALL' || log.component.toUpperCase() === compFilter.toUpperCase();

    return matchesSearch && matchesLevel && matchesComp;
  });

  // Extract all unique components from logs dynamically to populate filter list
  const uniqueComponents = ['ALL', ...Array.from(new Set(logs.map(l => l.component.toUpperCase())))];

  // Colors mapping for levels
  const getLevelStyles = (level: string) => {
    const lvl = level.toUpperCase();
    if (lvl === 'DEBUG') return { color: '#888ea8', bg: 'rgba(136, 142, 168, 0.12)', border: 'rgba(136, 142, 168, 0.2)' };
    if (lvl === 'INFO') return { color: '#22c55e', bg: 'rgba(34, 197, 94, 0.12)', border: 'rgba(34, 197, 94, 0.2)' };
    if (lvl === 'WARN') return { color: '#eab308', bg: 'rgba(234, 179, 8, 0.12)', border: 'rgba(234, 179, 8, 0.2)' };
    return { color: '#ef4444', bg: 'rgba(239, 68, 68, 0.12)', border: 'rgba(239, 68, 68, 0.2)' }; // ERROR / FATAL
  };

  const handleDownload = () => {
    if (!token) return;
    window.open(`/api/logs/download?token=${encodeURIComponent(token)}`);
  };

  return (
    <div style={{ display: 'flex', flexDirection: 'column', height: 'calc(100vh - 120px)', gap: 16 }}>
      {/* Header toolbar */}
      <div className="g-card" style={{ padding: '14px 20px', display: 'flex', flexWrap: 'wrap', gap: 14, alignItems: 'center', justifyContent: 'space-between' }}>
        <div style={{ display: 'flex', alignItems: 'center', gap: 10 }}>
          <div className="live-dot" style={{ background: isConnected ? (isPaused ? '#eab308' : '#22c55e') : '#ef4444', width: 8, height: 8 }} />
          <div>
            <h1 style={{ fontSize: 16, fontWeight: 700, color: 'var(--color-brand-heading)', margin: 0, display: 'flex', alignItems: 'center', gap: 8 }}>
              System Logs & Diagnostics
            </h1>
            <div style={{ fontSize: 11, color: 'var(--color-brand-text)' }}>
              {isConnected ? (isPaused ? 'Telemetry paused' : 'Active WebSocket Stream') : 'Offline'}
            </div>
          </div>
        </div>

        {/* Filters and Search toolbar */}
        <div style={{ display: 'flex', flexWrap: 'wrap', gap: 10, alignItems: 'center' }}>
          {/* Search */}
          <div style={{ position: 'relative', width: 180 }}>
            <FiSearch style={{ position: 'absolute', left: 10, top: '50%', transform: 'translateY(-50%)', color: 'var(--color-brand-muted)', fontSize: 13 }} />
            <input
              type="text"
              placeholder="Search logs..."
              value={search}
              onChange={(e) => setSearch(e.target.value)}
              style={{
                width: '100%',
                padding: '6px 10px 6px 28px',
                fontSize: 12,
                borderRadius: 6,
                border: '1px solid var(--color-brand-border)',
                background: 'var(--color-brand-bg)',
                color: 'var(--color-brand-heading)'
              }}
            />
          </div>

          {/* Level Filter */}
          <select
            value={levelFilter}
            onChange={(e) => setLevelFilter(e.target.value)}
            style={{
              padding: '6px 10px',
              fontSize: 12,
              borderRadius: 6,
              border: '1px solid var(--color-brand-border)',
              background: 'var(--color-brand-bg)',
              color: 'var(--color-brand-heading)'
            }}
          >
            <option value="ALL">All Levels</option>
            <option value="DEBUG">DEBUG</option>
            <option value="INFO">INFO</option>
            <option value="WARN">WARN</option>
            <option value="ERROR">ERROR</option>
          </select>

          {/* Component Filter */}
          <select
            value={compFilter}
            onChange={(e) => setCompFilter(e.target.value)}
            style={{
              padding: '6px 10px',
              fontSize: 12,
              borderRadius: 6,
              border: '1px solid var(--color-brand-border)',
              background: 'var(--color-brand-bg)',
              color: 'var(--color-brand-heading)'
            }}
          >
            {uniqueComponents.map(c => (
              <option key={c} value={c}>{c === 'ALL' ? 'All Components' : c}</option>
            ))}
          </select>

          {/* Stream Actions */}
          <button className="btn btn--sm" onClick={togglePause} style={{ display: 'flex', alignItems: 'center', gap: 6 }}>
            {isPaused ? <FiPlay size={13} /> : <FiPause size={13} />}
            {isPaused ? 'Resume' : 'Pause'}
          </button>

          <button className="btn btn--sm" onClick={clearLogs} title="Clear Screen">
            <FiTrash2 size={13} />
          </button>

          <button className="btn btn--primary btn--sm" onClick={handleDownload} style={{ display: 'flex', alignItems: 'center', gap: 6 }}>
            <FiDownload size={13} />
            Download today.log
          </button>
        </div>
      </div>

      {/* Terminal Board Container */}
      <div 
        ref={logContainerRef}
        style={{
          flex: 1,
          background: 'var(--color-brand-bg)',
          borderRadius: 12,
          border: '1px solid var(--color-brand-border)',
          overflowY: 'auto',
          boxShadow: 'inset 0 2px 8px rgba(0,0,0,0.06)',
          display: 'flex',
          flexDirection: 'column'
        }}
      >
        {/* Terminal Header */}
        <div style={{
          padding: '8px 16px',
          borderBottom: '1px solid var(--color-brand-border)',
          display: 'flex',
          justifyContent: 'space-between',
          alignItems: 'center',
          fontSize: 11,
          fontWeight: 600,
          color: 'var(--color-brand-text)',
          textTransform: 'uppercase',
          letterSpacing: 0.5
        }}>
          <div style={{ display: 'flex', gap: 6, alignItems: 'center' }}>
            <span style={{ display: 'inline-block', width: 6, height: 6, borderRadius: '50%', background: '#ef4444' }} />
            <span style={{ display: 'inline-block', width: 6, height: 6, borderRadius: '50%', background: '#eab308' }} />
            <span style={{ display: 'inline-block', width: 6, height: 6, borderRadius: '50%', background: '#22c55e' }} />
            <span style={{ marginLeft: 6 }}>CleverConnect Terminal Console</span>
          </div>
          <label style={{ display: 'flex', alignItems: 'center', gap: 6, cursor: 'pointer', textTransform: 'none' }}>
            <input type="checkbox" checked={autoscroll} onChange={(e) => setAutoscroll(e.target.checked)} style={{ cursor: 'pointer' }} />
            Autoscroll
          </label>
        </div>

        {/* Logs Stream Rows */}
        <div style={{ padding: '12px 16px', display: 'flex', flexDirection: 'column', gap: 4, fontFamily: 'monospace', fontSize: 12 }}>
          {filteredLogs.length === 0 ? (
            <div style={{ padding: '40px 0', textAlign: 'center', color: 'var(--color-brand-muted)' }}>
              No log entries match active filters or search terms.
            </div>
          ) : (
            filteredLogs.map((log, index) => {
              const styles = getLevelStyles(log.level);
              const isExpanded = expandedId === index;
              return (
                <div 
                  key={index}
                  style={{
                    display: 'flex',
                    flexDirection: 'column',
                    borderBottom: '1px solid rgba(0,0,0,0.01)',
                    background: isExpanded ? 'rgba(0,0,0,0.02)' : 'transparent',
                    borderRadius: 4
                  }}
                >
                  {/* Single Line Header */}
                  <div 
                    onClick={() => setExpandedId(isExpanded ? null : index)}
                    style={{
                      display: 'flex',
                      alignItems: 'center',
                      padding: '4px 6px',
                      cursor: 'pointer',
                      borderRadius: 4,
                      transition: 'background 0.12s',
                      userSelect: 'text'
                    }}
                    className="log-row-hover"
                  >
                    <span style={{ color: 'var(--color-brand-muted)', marginRight: 10, flexShrink: 0 }}>
                      {isExpanded ? <FiChevronDown size={12} /> : <FiChevronRight size={12} />}
                    </span>

                    {/* Timestamp */}
                    <span style={{ color: 'var(--color-brand-text)', marginRight: 12, flexShrink: 0, width: 84 }}>
                      {log.timestamp}
                    </span>

                    {/* Level badge */}
                    <span style={{
                      color: styles.color,
                      background: styles.bg,
                      border: `1px solid ${styles.border}`,
                      borderRadius: 4,
                      padding: '1px 5px',
                      fontSize: 10,
                      fontWeight: 600,
                      marginRight: 12,
                      width: 50,
                      textAlign: 'center',
                      flexShrink: 0
                    }}>
                      {log.level.toUpperCase()}
                    </span>

                    {/* Component */}
                    <span style={{
                      color: 'var(--color-brand-heading)',
                      fontWeight: 600,
                      marginRight: 12,
                      width: 72,
                      flexShrink: 0
                    }}>
                      [{log.component.toUpperCase()}]
                    </span>

                    {/* Message */}
                    <span style={{
                      color: log.level.toUpperCase() === 'ERROR' ? '#ef4444' : 'var(--color-brand-heading)',
                      flex: 1,
                      wordBreak: 'break-all',
                      paddingRight: 16
                    }}>
                      {log.message}
                    </span>

                    {/* Caller Info */}
                    {log.caller && (
                      <span style={{
                        color: 'var(--color-brand-muted)',
                        fontSize: 11,
                        flexShrink: 0,
                        marginLeft: 'auto'
                      }}>
                        @ {log.caller}
                      </span>
                    )}
                  </div>

                  {/* Expanded JSON details */}
                  {isExpanded && (
                    <div style={{
                      marginLeft: 28,
                      marginRight: 16,
                      marginBottom: 10,
                      padding: 12,
                      borderRadius: 8,
                      background: 'var(--color-brand-bg)',
                      border: '1px solid var(--color-brand-border)',
                      fontSize: 11
                    }}>
                      <div style={{ display: 'flex', flexDirection: 'column', gap: 6 }}>
                        <div>
                          <strong style={{ color: 'var(--color-brand-text)' }}>Timestamp:</strong> <span style={{ color: 'var(--color-brand-heading)' }}>{log.timestamp}</span>
                        </div>
                        <div>
                          <strong style={{ color: 'var(--color-brand-text)' }}>Caller Trace:</strong> <span style={{ color: 'var(--color-brand-heading)' }}>{log.caller || 'None'}</span>
                        </div>
                        {log.fields && Object.keys(log.fields).length > 0 && (
                          <div>
                            <strong style={{ color: 'var(--color-brand-text)' }}>Structured Fields:</strong>
                            <pre style={{
                              margin: '6px 0 0',
                              padding: 8,
                              background: 'var(--color-brand-card)',
                              border: '1px solid var(--color-brand-border)',
                              borderRadius: 6,
                              color: 'var(--color-brand-heading)',
                              overflowX: 'auto'
                            }}>
                              {JSON.stringify(log.fields, null, 2)}
                            </pre>
                          </div>
                        )}
                        <div>
                          <strong style={{ color: 'var(--color-brand-text)' }}>Raw Standard String:</strong>
                          <pre style={{
                            margin: '6px 0 0',
                            padding: 8,
                            background: 'var(--color-brand-card)',
                            border: '1px solid var(--color-brand-border)',
                            borderRadius: 6,
                            color: 'var(--color-brand-heading)',
                            whiteSpace: 'pre-wrap',
                            wordBreak: 'break-all'
                          }}>
                            {log.raw}
                          </pre>
                        </div>
                      </div>
                    </div>
                  )}
                </div>
              );
            })
          )}
          <div ref={logsEndRef} />
        </div>
      </div>
    </div>
  );
};
