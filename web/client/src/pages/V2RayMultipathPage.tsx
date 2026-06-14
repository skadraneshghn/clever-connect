import React, { useEffect, useCallback } from 'react';
import { useMultipathStore, ArteryStatus } from '../store/multipathStore';

/* ────────────────────────────────────────────────────────────────────────── */
/*  State → visual mapping                                                  */
/* ────────────────────────────────────────────────────────────────────────── */
const stateColor: Record<string, string> = {
  active:      '#10b981',
  shadow:      '#6366f1',
  probation:   '#f59e0b',
  dead:        '#ef4444',
  quarantined: '#9b9bab',
};

const stateBg: Record<string, string> = {
  active:      'rgba(16,185,129,0.12)',
  shadow:      'rgba(99,102,241,0.12)',
  probation:   'rgba(245,158,11,0.12)',
  dead:        'rgba(239,68,68,0.12)',
  quarantined: 'rgba(155,155,171,0.12)',
};

/* ────────────────────────────────────────────────────────────────────────── */
/*  Pulse dot                                                               */
/* ────────────────────────────────────────────────────────────────────────── */
const PulseDot: React.FC<{ color: string; active?: boolean }> = ({ color, active = true }) => (
  <span style={{ position: 'relative', display: 'inline-flex', width: 8, height: 8 }}>
    {active && (
      <span style={{
        position: 'absolute', inset: 0, borderRadius: '50%', backgroundColor: color,
        animation: 'pulse-ring 1.5s ease-in-out infinite', opacity: 0.4,
      }} />
    )}
    <span style={{
      position: 'relative', width: 8, height: 8, borderRadius: '50%',
      backgroundColor: color,
    }} />
  </span>
);

/* ────────────────────────────────────────────────────────────────────────── */
/*  Stat cell helper                                                        */
/* ────────────────────────────────────────────────────────────────────────── */
const Stat: React.FC<{ label: string; value: string | number; unit?: string; color?: string }> = ({ label, value, unit, color }) => (
  <div style={{ textAlign: 'center' }}>
    <div style={{ fontSize: 10, color: 'var(--color-brand-muted)', textTransform: 'uppercase', letterSpacing: 1, marginBottom: 2 }}>{label}</div>
    <div style={{ fontSize: 20, fontWeight: 700, color: color || 'var(--color-brand-heading)', fontVariantNumeric: 'tabular-nums' }}>
      {value}<span style={{ fontSize: 11, fontWeight: 400, color: 'var(--color-brand-muted)', marginLeft: 2 }}>{unit}</span>
    </div>
  </div>
);

/* ────────────────────────────────────────────────────────────────────────── */
/*  Topology node SVG                                                       */
/* ────────────────────────────────────────────────────────────────────────── */
const TopologyVisualizer: React.FC<{ arteries: ArteryStatus[] }> = ({ arteries }) => {
  const w = 500, h = Math.max(180, arteries.length * 50 + 60);
  const clientX = 70, serverX = w - 70;
  const clientY = h / 2, serverY = h / 2;

  return (
    <svg viewBox={`0 0 ${w} ${h}`} style={{ width: '100%', maxWidth: 500, height: 'auto' }}>
      <defs>
        <filter id="glow-active"><feGaussianBlur stdDeviation="3" result="g" /><feMerge><feMergeNode in="g" /><feMergeNode in="SourceGraphic" /></feMerge></filter>
        <filter id="glow-dead"><feGaussianBlur stdDeviation="2" result="g" /><feMerge><feMergeNode in="g" /><feMergeNode in="SourceGraphic" /></feMerge></filter>
        {/* animated particle */}
        <circle id="particle" r="3" fill="#10b981" />
      </defs>

      {/* Client node */}
      <circle cx={clientX} cy={clientY} r={22} fill="var(--color-brand-card)" stroke="var(--color-brand)" strokeWidth={2.5} filter="url(#glow-active)" />
      <text x={clientX} y={clientY + 1} textAnchor="middle" dominantBaseline="middle" fill="var(--color-brand)" fontSize={9} fontWeight={700}>LOCAL</text>

      {/* Server node */}
      <circle cx={serverX} cy={serverY} r={22} fill="var(--color-brand-card)" stroke="#6366f1" strokeWidth={2.5} filter="url(#glow-active)" />
      <text x={serverX} y={serverY + 1} textAnchor="middle" dominantBaseline="middle" fill="#6366f1" fontSize={8} fontWeight={700}>SERVER</text>

      {/* Artery lines */}
      {arteries.map((a, i) => {
        const y = 30 + ((h - 60) / Math.max(arteries.length - 1, 1)) * i;
        const color = stateColor[a.state] || '#9b9bab';
        const isDead = a.state === 'dead' || a.state === 'quarantined';
        const isShadow = a.state === 'shadow';

        return (
          <g key={a.tag}>
            <path
              d={`M ${clientX + 24} ${clientY} Q ${w / 2} ${y}, ${serverX - 24} ${serverY}`}
              fill="none"
              stroke={color}
              strokeWidth={isDead ? 1 : isShadow ? 1.5 : 2.5}
              strokeDasharray={isDead ? '4 4' : isShadow ? '6 3' : 'none'}
              opacity={isDead ? 0.4 : isShadow ? 0.6 : 1}
              filter={!isDead ? 'url(#glow-active)' : undefined}
            />
            {/* Label */}
            <rect x={w / 2 - 32} y={y - 9} width={64} height={18} rx={9} fill={stateBg[a.state] || '#f5f5f2'} stroke={color} strokeWidth={0.8} />
            <text x={w / 2} y={y + 1} textAnchor="middle" dominantBaseline="middle" fontSize={8} fontWeight={600} fill={color}>
              {a.tag}
            </text>
            {/* Animated particles for active paths */}
            {!isDead && !isShadow && (
              <circle r={3} fill={color} opacity={0.8}>
                <animateMotion
                  dur={`${1.5 + i * 0.3}s`}
                  repeatCount="indefinite"
                  path={`M ${clientX + 24} ${clientY} Q ${w / 2} ${y}, ${serverX - 24} ${serverY}`}
                />
              </circle>
            )}
          </g>
        );
      })}
    </svg>
  );
};

/* ────────────────────────────────────────────────────────────────────────── */
/*  Artery row                                                              */
/* ────────────────────────────────────────────────────────────────────────── */
const ArteryRow: React.FC<{ artery: ArteryStatus }> = ({ artery: a }) => {
  const color = stateColor[a.state] || '#9b9bab';
  return (
    <tr style={{ borderBottom: '1px solid var(--color-brand-border)' }}>
      <td style={{ padding: '10px 12px', display: 'flex', alignItems: 'center', gap: 8 }}>
        <PulseDot color={color} active={a.state === 'active'} />
        <span style={{ fontWeight: 600, fontSize: 13, color: 'var(--color-brand-heading)' }}>{a.tag}</span>
      </td>
      <td style={{ padding: '10px 12px', fontSize: 12, color: 'var(--color-brand-text)' }}>
        {a.node_name || '—'}
      </td>
      <td style={{ padding: '10px 12px', fontSize: 12, fontFamily: 'monospace' }}>
        {a.address}:{a.port}
      </td>
      <td style={{ padding: '10px 12px' }}>
        <span style={{
          display: 'inline-flex', alignItems: 'center', gap: 4,
          padding: '2px 10px', borderRadius: 20, fontSize: 10, fontWeight: 600,
          textTransform: 'uppercase', letterSpacing: 0.5,
          background: stateBg[a.state], color,
        }}>
          <PulseDot color={color} active={a.state === 'active' || a.state === 'probation'} />
          {a.state}
        </span>
      </td>
      <td style={{ padding: '10px 12px', fontSize: 13, fontVariantNumeric: 'tabular-nums', fontWeight: 500 }}>
        {a.srtt_ms > 0 ? `${a.srtt_ms.toFixed(0)}` : '—'}<span style={{ fontSize: 10, color: 'var(--color-brand-muted)' }}> ms</span>
      </td>
      <td style={{ padding: '10px 12px', fontSize: 13, fontVariantNumeric: 'tabular-nums', color: a.loss_pct > 3 ? '#ef4444' : 'var(--color-brand-text)' }}>
        {a.loss_pct.toFixed(1)}%
      </td>
      <td style={{ padding: '10px 12px', fontSize: 13, fontVariantNumeric: 'tabular-nums' }}>
        {a.win_rate.toFixed(0)}%
      </td>
      <td style={{ padding: '10px 12px', fontSize: 13, fontVariantNumeric: 'tabular-nums' }}>
        {a.error_count}
      </td>
    </tr>
  );
};

/* ────────────────────────────────────────────────────────────────────────── */
/*  MAIN PAGE                                                               */
/* ────────────────────────────────────────────────────────────────────────── */
export const V2RayMultipathPage: React.FC = () => {
  const {
    status, config, loading, error,
    fetchConfig, fetchStatus, startEngine, stopEngine, connectTelemetry, saveConfig,
  } = useMultipathStore();

  // Bootstrap
  useEffect(() => {
    fetchConfig();
    fetchStatus();
  }, [fetchConfig, fetchStatus]);

  // Connect telemetry when engine is running
  useEffect(() => {
    if (status?.state === 'running') {
      const cleanup = connectTelemetry();
      return cleanup;
    }
  }, [status?.state, connectTelemetry]);

  const isRunning = status?.state === 'running';
  const arteries = status?.arteries || [];

  const handleToggle = useCallback(async () => {
    if (isRunning) {
      await stopEngine();
    } else {
      // Auto-save defaults if no config exists
      if (!config?.id) {
        await saveConfig({
          is_active: true, mode: 'selector', striping_mode: 'auto',
          max_arteries: 5, min_arteries: 2, combiner_url: '', origin_id: '',
          psk_hex: '', frame_size: 4096, socks_port: 10646, http_port: 10545,
          eval_window_ms: 5000, demote_rtt_x: 1.5, promote_rtt_x: 1.2,
          loss_demote_pct: 5.0, cooldown_sec: 30, error_budget: 5,
        });
      }
      await startEngine();
    }
    await fetchStatus();
  }, [isRunning, config, saveConfig, startEngine, stopEngine, fetchStatus]);

  return (
    <div style={{ display: 'flex', flexDirection: 'column', gap: 20, padding: 4 }}>
      {/* Pulse keyframes */}
      <style>{`
        @keyframes pulse-ring { 0% { transform: scale(1); opacity: 0.4; } 100% { transform: scale(2.5); opacity: 0; } }
        @keyframes shimmer { 0% { background-position: -200% 0; } 100% { background-position: 200% 0; } }
      `}</style>

      {/* ── Hero Card ───────────────────────────────────────────────── */}
      <div style={{
        background: 'linear-gradient(135deg, #1a1a2e 0%, #16213e 50%, #0f3460 100%)',
        borderRadius: 16, padding: '28px 32px', color: '#fff', position: 'relative', overflow: 'hidden',
      }}>
        {/* Decorative mesh */}
        <div style={{
          position: 'absolute', inset: 0, opacity: 0.06,
          backgroundImage: 'radial-gradient(circle at 20% 50%, #ff6b2c 0%, transparent 50%), radial-gradient(circle at 80% 50%, #6366f1 0%, transparent 50%)',
        }} />

        <div style={{ display: 'flex', justifyContent: 'space-between', alignItems: 'center', position: 'relative', zIndex: 1 }}>
          <div>
            <div style={{ fontSize: 10, textTransform: 'uppercase', letterSpacing: 2, color: '#9b9bab', marginBottom: 6 }}>
              Dynamic Multipath Bonding
            </div>
            <h1 style={{ fontSize: 24, fontWeight: 800, margin: 0, letterSpacing: -0.5 }}>
              ⚡ Multipath Engine
            </h1>
            <p style={{ fontSize: 12, color: '#9b9bab', marginTop: 6, maxWidth: 400 }}>
              Intelligent multi-line selector with automatic failover, health monitoring, and anti-oscillation protection.
            </p>
          </div>

          {/* Master Toggle */}
          <button
            onClick={handleToggle}
            disabled={loading}
            style={{
              padding: '12px 32px', borderRadius: 12, border: 'none', cursor: loading ? 'not-allowed' : 'pointer',
              fontSize: 14, fontWeight: 700, letterSpacing: 0.5, transition: 'all 0.3s ease',
              background: isRunning
                ? 'linear-gradient(135deg, #ef4444, #dc2626)'
                : 'linear-gradient(135deg, #10b981, #059669)',
              color: '#fff',
              boxShadow: isRunning ? '0 4px 20px rgba(239,68,68,0.4)' : '0 4px 20px rgba(16,185,129,0.4)',
              opacity: loading ? 0.6 : 1,
            }}
          >
            {loading ? '...' : isRunning ? '■  Stop Engine' : '▶  Start Engine'}
          </button>
        </div>

        {/* Stats strip */}
        {isRunning && (
          <div style={{
            display: 'grid', gridTemplateColumns: 'repeat(4, 1fr)', gap: 16,
            marginTop: 24, padding: '16px 0', borderTop: '1px solid rgba(255,255,255,0.08)',
            position: 'relative', zIndex: 1,
          }}>
            <Stat label="Mode" value={config?.mode === 'bonding' ? 'Bonding' : 'Selector'} color="#ff6b2c" />
            <Stat label="Active Arteries" value={status?.active_count || 0} color="#10b981" />
            <Stat label="Pool Size" value={status?.total_pool || 0} color="#6366f1" />
            <Stat label="Engine State" value={status?.state || '—'} color="#f59e0b" />
          </div>
        )}
      </div>

      {error && (
        <div style={{
          padding: '12px 16px', borderRadius: 10,
          background: 'rgba(239,68,68,0.08)', border: '1px solid rgba(239,68,68,0.2)',
          color: '#ef4444', fontSize: 13,
        }}>
          ⚠ {error}
        </div>
      )}

      {/* ── Topology + Config Grid ──────────────────────────────────── */}
      {isRunning && arteries.length > 0 && (
        <div style={{ display: 'grid', gridTemplateColumns: '1fr 1fr', gap: 20 }}>
          {/* Topology Visualizer */}
          <div style={{
            background: 'var(--color-brand-card)', borderRadius: 14,
            border: '1px solid var(--color-brand-border)', padding: '20px 24px',
          }}>
            <div style={{ fontSize: 11, textTransform: 'uppercase', letterSpacing: 1.5, color: 'var(--color-brand-muted)', marginBottom: 16, fontWeight: 600 }}>
              Live Topology
            </div>
            <TopologyVisualizer arteries={arteries} />
          </div>

          {/* Config Summary */}
          <div style={{
            background: 'var(--color-brand-card)', borderRadius: 14,
            border: '1px solid var(--color-brand-border)', padding: '20px 24px',
          }}>
            <div style={{ fontSize: 11, textTransform: 'uppercase', letterSpacing: 1.5, color: 'var(--color-brand-muted)', marginBottom: 16, fontWeight: 600 }}>
              Engine Configuration
            </div>
            <div style={{ display: 'grid', gridTemplateColumns: '1fr 1fr', gap: 12 }}>
              {[
                ['Mode', config?.mode || 'selector'],
                ['Striping', config?.striping_mode || 'auto'],
                ['Max Arteries', config?.max_arteries || 5],
                ['Min Arteries', config?.min_arteries || 2],
                ['SOCKS Port', config?.socks_port || 10646],
                ['HTTP Port', config?.http_port || 10545],
                ['Eval Window', `${config?.eval_window_ms || 5000}ms`],
                ['Demote RTT×', config?.demote_rtt_x || 1.5],
                ['Promote RTT×', config?.promote_rtt_x || 1.2],
                ['Loss Demote %', `${config?.loss_demote_pct || 5}%`],
                ['Cooldown', `${config?.cooldown_sec || 30}s`],
                ['Error Budget', config?.error_budget || 5],
              ].map(([label, value]) => (
                <div key={String(label)} style={{ padding: '8px 12px', borderRadius: 8, background: 'var(--color-brand-bg)' }}>
                  <div style={{ fontSize: 9, textTransform: 'uppercase', letterSpacing: 1, color: 'var(--color-brand-muted)' }}>{label}</div>
                  <div style={{ fontSize: 14, fontWeight: 600, color: 'var(--color-brand-heading)', marginTop: 2 }}>{String(value)}</div>
                </div>
              ))}
            </div>
          </div>
        </div>
      )}

      {/* ── Arteries Table ──────────────────────────────────────────── */}
      {isRunning && arteries.length > 0 && (
        <div style={{
          background: 'var(--color-brand-card)', borderRadius: 14,
          border: '1px solid var(--color-brand-border)', overflow: 'hidden',
        }}>
          <div style={{ padding: '16px 24px', borderBottom: '1px solid var(--color-brand-border)', display: 'flex', justifyContent: 'space-between', alignItems: 'center' }}>
            <div>
              <div style={{ fontSize: 11, textTransform: 'uppercase', letterSpacing: 1.5, color: 'var(--color-brand-muted)', fontWeight: 600 }}>
                Active Arteries
              </div>
              <div style={{ fontSize: 13, color: 'var(--color-brand-text)', marginTop: 2 }}>
                Real-time performance metrics for each bonding line
              </div>
            </div>
            <div style={{
              padding: '4px 12px', borderRadius: 20, fontSize: 11, fontWeight: 600,
              background: 'rgba(16,185,129,0.1)', color: '#10b981',
            }}>
              {arteries.filter(a => a.state === 'active').length} / {arteries.length} active
            </div>
          </div>

          <div style={{ overflowX: 'auto' }}>
            <table style={{ width: '100%', borderCollapse: 'collapse' }}>
              <thead>
                <tr style={{ borderBottom: '1px solid var(--color-brand-border)', background: 'var(--color-brand-bg)' }}>
                  {['Tag', 'Node', 'Endpoint', 'State', 'RTT', 'Loss', 'Win Rate', 'Errors'].map(h => (
                    <th key={h} style={{
                      padding: '8px 12px', fontSize: 10, fontWeight: 600, textTransform: 'uppercase',
                      letterSpacing: 1, color: 'var(--color-brand-muted)', textAlign: 'left',
                    }}>{h}</th>
                  ))}
                </tr>
              </thead>
              <tbody>
                {arteries.map(a => <ArteryRow key={a.tag} artery={a} />)}
              </tbody>
            </table>
          </div>
        </div>
      )}

      {/* ── Empty State ─────────────────────────────────────────────── */}
      {!isRunning && (
        <div style={{
          background: 'var(--color-brand-card)', borderRadius: 14,
          border: '1px solid var(--color-brand-border)', padding: '60px 40px',
          textAlign: 'center',
        }}>
          <div style={{ fontSize: 48, marginBottom: 16 }}>🔌</div>
          <h2 style={{ fontSize: 18, fontWeight: 700, color: 'var(--color-brand-heading)', margin: '0 0 8px' }}>
            Engine Offline
          </h2>
          <p style={{ fontSize: 13, color: 'var(--color-brand-muted)', maxWidth: 400, margin: '0 auto', lineHeight: 1.6 }}>
            The Multipath Engine monitors multiple proxy lines simultaneously and automatically switches
            to the fastest available path. Click <strong>Start Engine</strong> to begin.
          </p>
          <p style={{ fontSize: 11, color: 'var(--color-brand-muted)', marginTop: 12 }}>
            Requires at least 2 healthy nodes in the scanner pool.
          </p>
        </div>
      )}
    </div>
  );
};
