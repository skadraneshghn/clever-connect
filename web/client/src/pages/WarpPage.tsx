import React, { useEffect, useState, useCallback, useRef } from 'react';
import {
  FiWifi, FiShield, FiActivity, FiZap, FiRefreshCw, FiPlus, FiTrash2,
  FiCheck, FiAlertTriangle, FiX, FiClock, FiServer, FiSettings,
  FiTarget, FiUser, FiRadio,
} from 'react-icons/fi';
import { useWarpStore, type TunnelStatus, type WarpScanResult, type ScanEvent } from '../store/warpStore';

// ─────────────────────────────────────────────────────────────────────────────
// Global keyframe styles
// ─────────────────────────────────────────────────────────────────────────────
const WARP_STYLES = `
  @keyframes warp-pulse-ring {
    0% { transform: scale(1); opacity: 0.6; }
    100% { transform: scale(2.8); opacity: 0; }
  }
  @keyframes warp-slide-in {
    0% { opacity: 0; transform: translateY(10px); }
    100% { opacity: 1; transform: translateY(0); }
  }
  @keyframes warp-spin {
    to { transform: rotate(360deg); }
  }
  @keyframes warp-shimmer {
    0% { background-position: -400% 0; }
    100% { background-position: 400% 0; }
  }
  @keyframes warp-glow-pulse {
    0%, 100% { box-shadow: 0 0 12px rgba(16,185,129,0.4), 0 0 4px rgba(16,185,129,0.2); }
    50% { box-shadow: 0 0 28px rgba(16,185,129,0.7), 0 0 12px rgba(16,185,129,0.4); }
  }
  @keyframes warp-restricted-pulse {
    0%, 100% { box-shadow: 0 0 12px rgba(239,68,68,0.4); }
    50% { box-shadow: 0 0 28px rgba(239,68,68,0.7); }
  }
  @keyframes warp-float {
    0%, 100% { transform: translateY(0px); }
    50% { transform: translateY(-4px); }
  }
  .warp-slide-in { animation: warp-slide-in 0.4s cubic-bezier(0.16,1,0.3,1) forwards; }
  .warp-spin { animation: warp-spin 1s linear infinite; }
  .warp-shimmer-bg {
    background: linear-gradient(90deg, transparent 0%, rgba(255,255,255,0.04) 40%, rgba(255,255,255,0.08) 50%, rgba(255,255,255,0.04) 60%, transparent 100%);
    background-size: 400% 100%;
    animation: warp-shimmer 2s linear infinite;
  }
`;

// ─────────────────────────────────────────────────────────────────────────────
// Helpers
// ─────────────────────────────────────────────────────────────────────────────
function formatBytes(bytes: number): string {
  if (!bytes || bytes === 0) return '0 B';
  if (bytes < 1024) return `${bytes} B`;
  if (bytes < 1024 ** 2) return `${(bytes / 1024).toFixed(1)} KB`;
  if (bytes < 1024 ** 3) return `${(bytes / 1024 ** 2).toFixed(2)} MB`;
  return `${(bytes / 1024 ** 3).toFixed(2)} GB`;
}

function formatQuotaPct(used: number, total: number): number {
  if (!total) return 0;
  return Math.min(100, Math.round((used / total) * 100));
}

// ─────────────────────────────────────────────────────────────────────────────
// Status colour tokens
// ─────────────────────────────────────────────────────────────────────────────
const STATUS_CFG: Record<TunnelStatus, {
  label: string;
  color: string;
  bg: string;
  border: string;
  icon: React.ReactNode;
  heroGradient: string;
}> = {
  dormant: {
    label: 'DORMANT',
    color: '#94a3b8',
    bg: 'rgba(148,163,184,0.08)',
    border: 'rgba(148,163,184,0.2)',
    icon: <FiShield size={14} />,
    heroGradient: 'linear-gradient(135deg, #0f172a 0%, #1e293b 100%)',
  },
  sweeping: {
    label: 'SCANNING',
    color: '#f59e0b',
    bg: 'rgba(245,158,11,0.08)',
    border: 'rgba(245,158,11,0.25)',
    icon: <FiTarget size={14} />,
    heroGradient: 'linear-gradient(135deg, #1c1400 0%, #2a1f00 50%, #1c1400 100%)',
  },
  linking: {
    label: 'LINKING',
    color: '#6366f1',
    bg: 'rgba(99,102,241,0.08)',
    border: 'rgba(99,102,241,0.25)',
    icon: <FiRadio size={14} />,
    heroGradient: 'linear-gradient(135deg, #0c0d1e 0%, #16183a 50%, #0c0d1e 100%)',
  },
  active: {
    label: 'ACTIVE',
    color: '#10b981',
    bg: 'rgba(16,185,129,0.08)',
    border: 'rgba(16,185,129,0.25)',
    icon: <FiCheck size={14} />,
    heroGradient: 'linear-gradient(135deg, #001a0f 0%, #002d1a 50%, #001a0f 100%)',
  },
  restricted: {
    label: 'RESTRICTED',
    color: '#ef4444',
    bg: 'rgba(239,68,68,0.08)',
    border: 'rgba(239,68,68,0.25)',
    icon: <FiAlertTriangle size={14} />,
    heroGradient: 'linear-gradient(135deg, #1a0000 0%, #2d0000 50%, #1a0000 100%)',
  },
};

// ─────────────────────────────────────────────────────────────────────────────
// Atom: PulseDot
// ─────────────────────────────────────────────────────────────────────────────
const PulseDot: React.FC<{ color: string; active?: boolean; size?: number }> = ({
  color, active = false, size = 8,
}) => (
  <span style={{ position: 'relative', display: 'inline-flex', width: size, height: size }}>
    {active && (
      <span style={{
        position: 'absolute', inset: 0, borderRadius: '50%', backgroundColor: color,
        animation: 'warp-pulse-ring 1.6s ease-out infinite', opacity: 0.6,
      }} />
    )}
    <span style={{ position: 'relative', width: size, height: size, borderRadius: '50%', backgroundColor: color }} />
  </span>
);

// ─────────────────────────────────────────────────────────────────────────────
// Atom: StatCell
// ─────────────────────────────────────────────────────────────────────────────
const StatCell: React.FC<{ label: string; value: React.ReactNode; unit?: string; color?: string }> = ({
  label, value, unit, color,
}) => (
  <div style={{ textAlign: 'center' }}>
    <div style={{ fontSize: 9, textTransform: 'uppercase', letterSpacing: 1.5, color: 'rgba(255,255,255,0.4)', marginBottom: 4 }}>
      {label}
    </div>
    <div style={{ fontSize: 20, fontWeight: 700, color: color || '#fff', fontVariantNumeric: 'tabular-nums', lineHeight: 1 }}>
      {value}
      {unit && <span style={{ fontSize: 10, fontWeight: 400, color: 'rgba(255,255,255,0.4)', marginLeft: 3 }}>{unit}</span>}
    </div>
  </div>
);

// ─────────────────────────────────────────────────────────────────────────────
// Atom: SectionCard
// ─────────────────────────────────────────────────────────────────────────────
const SectionCard: React.FC<{ title: string; subtitle?: string; action?: React.ReactNode; children: React.ReactNode }> = ({
  title, subtitle, action, children,
}) => (
  <div style={{
    background: 'var(--color-brand-card)',
    borderRadius: 14,
    border: '1px solid var(--color-brand-border)',
    overflow: 'hidden',
    boxShadow: '0 4px 24px rgba(0,0,0,0.12)',
  }}>
    <div style={{
      padding: '16px 20px',
      borderBottom: '1px solid var(--color-brand-border)',
      display: 'flex',
      justifyContent: 'space-between',
      alignItems: 'center',
    }}>
      <div>
        <div style={{ fontSize: 10, fontWeight: 700, textTransform: 'uppercase', letterSpacing: 1.5, color: 'var(--color-brand-muted)' }}>
          {title}
        </div>
        {subtitle && <div style={{ fontSize: 12, color: 'var(--color-brand-text)', marginTop: 2 }}>{subtitle}</div>}
      </div>
      {action}
    </div>
    <div style={{ padding: 20 }}>{children}</div>
  </div>
);

// ─────────────────────────────────────────────────────────────────────────────
// Atom: StyledInput / StyledSelect
// ─────────────────────────────────────────────────────────────────────────────
const inputStyle: React.CSSProperties = {
  width: '100%',
  padding: '9px 12px',
  borderRadius: 8,
  border: '1px solid var(--color-brand-border)',
  background: 'var(--color-brand-bg)',
  color: 'var(--color-brand-heading)',
  fontSize: 13,
  outline: 'none',
  transition: 'border-color 0.2s ease',
  boxSizing: 'border-box',
};

const labelStyle: React.CSSProperties = {
  fontSize: 10,
  fontWeight: 600,
  textTransform: 'uppercase',
  letterSpacing: 1,
  color: 'var(--color-brand-muted)',
  marginBottom: 6,
  display: 'block',
};

// ─────────────────────────────────────────────────────────────────────────────
// Molecule: Mini Throughput Sparkline (SVG-based)
// ─────────────────────────────────────────────────────────────────────────────
const ThroughputSparkline: React.FC<{
  data: Array<{ latencyMs: number }>;
  color?: string;
  width?: number;
  height?: number;
}> = ({ data, color = '#10b981', width = 200, height = 50 }) => {
  if (!data.length) {
    return (
      <div style={{ width, height, display: 'flex', alignItems: 'center', justifyContent: 'center', color: 'var(--color-brand-muted)', fontSize: 11 }}>
        No data yet
      </div>
    );
  }

  const values = data.map((d) => d.latencyMs);
  const min = Math.min(...values);
  const max = Math.max(...values) || 1;
  const range = max - min || 1;

  const points = values.map((v, i) => {
    const x = (i / Math.max(values.length - 1, 1)) * width;
    const y = height - ((v - min) / range) * (height - 10) - 5;
    return `${x},${y}`;
  }).join(' ');

  const areaPoints = `0,${height} ${points} ${width},${height}`;

  return (
    <svg width={width} height={height} style={{ overflow: 'visible' }}>
      <defs>
        <linearGradient id="warp-spark-grad" x1="0" y1="0" x2="0" y2="1">
          <stop offset="0%" stopColor={color} stopOpacity={0.3} />
          <stop offset="100%" stopColor={color} stopOpacity={0.02} />
        </linearGradient>
      </defs>
      <polygon points={areaPoints} fill="url(#warp-spark-grad)" />
      <polyline
        points={points}
        fill="none"
        stroke={color}
        strokeWidth={1.5}
        strokeLinejoin="round"
        strokeLinecap="round"
      />
      {values.length > 0 && (() => {
        const last = values[values.length - 1];
        const lx = width;
        const ly = height - ((last - min) / range) * (height - 10) - 5;
        return <circle cx={lx} cy={ly} r={3} fill={color} />;
      })()}
    </svg>
  );
};

// ─────────────────────────────────────────────────────────────────────────────
// Molecule: Quota Allocation Meter
// ─────────────────────────────────────────────────────────────────────────────
const QuotaMeter: React.FC<{ used: number; total: number }> = ({ used, total }) => {
  const pct = formatQuotaPct(used, total);
  const color = pct > 80 ? '#f59e0b' : pct > 60 ? '#6366f1' : '#10b981';

  return (
    <div style={{ display: 'flex', flexDirection: 'column', gap: 8 }}>
      <div style={{ display: 'flex', justifyContent: 'space-between', alignItems: 'baseline' }}>
        <span style={{ fontSize: 11, color: 'var(--color-brand-muted)' }}>Quota Used</span>
        <span style={{ fontSize: 13, fontWeight: 700, color }}>
          {pct}%
        </span>
      </div>
      <div style={{ height: 6, borderRadius: 3, background: 'var(--color-brand-bg)', overflow: 'hidden' }}>
        <div style={{
          height: '100%',
          width: `${pct}%`,
          borderRadius: 3,
          background: pct > 80
            ? 'linear-gradient(90deg, #f59e0b, #ef4444)'
            : pct > 60
              ? 'linear-gradient(90deg, #6366f1, #8b5cf6)'
              : 'linear-gradient(90deg, #10b981, #34d399)',
          transition: 'width 0.6s ease-out',
        }} />
      </div>
      <div style={{ display: 'flex', justifyContent: 'space-between', fontSize: 10, color: 'var(--color-brand-muted)' }}>
        <span>{formatBytes(used)} used</span>
        <span>{total > 0 ? formatBytes(total) : '∞'} total</span>
      </div>
    </div>
  );
};

// ─────────────────────────────────────────────────────────────────────────────
// Molecule: Scan Result Row
// ─────────────────────────────────────────────────────────────────────────────
const ScanResultRow: React.FC<{ result: WarpScanResult; rank: number }> = ({ result, rank }) => {
  const latColor = result.latency_ms < 100 ? '#10b981' : result.latency_ms < 300 ? '#f59e0b' : '#ef4444';
  const score = result.score ?? 0;
  const scoreColor = score > 800 ? '#10b981' : score > 500 ? '#f59e0b' : '#ef4444';

  return (
    <div style={{
      display: 'grid',
      gridTemplateColumns: '28px 1fr 70px 70px 100px 50px auto',
      alignItems: 'center',
      padding: '10px 16px',
      borderBottom: '1px solid var(--color-brand-border)',
      fontSize: 12,
      gap: 8,
      opacity: result.is_restricted && !result.fail_count ? 0.7 : 1,
      transition: 'background 0.15s ease',
    }}>
      <span style={{ fontSize: 10, color: 'var(--color-brand-muted)', fontWeight: 700 }}>#{rank}</span>
      <span style={{ fontFamily: 'monospace', color: 'var(--color-brand-heading)', fontWeight: 600 }}>
        {result.ip_address}:{result.port}
      </span>
      <span style={{ color: latColor, fontWeight: 700, fontVariantNumeric: 'tabular-nums' }}>
        {result.latency_ms.toFixed(0)}<span style={{ fontSize: 9, color: 'var(--color-brand-muted)', marginLeft: 2 }}>ms</span>
      </span>
      <span style={{ color: scoreColor, fontWeight: 700, fontVariantNumeric: 'tabular-nums' }}>
        {score.toFixed(0)}<span style={{ fontSize: 9, color: 'var(--color-brand-muted)', marginLeft: 2 }}>pts</span>
      </span>
      <span style={{ color: 'var(--color-brand-muted)', fontSize: 10 }}>
        {result.supported_alpns?.join(', ') || '—'}
      </span>
      <span style={{ fontVariantNumeric: 'tabular-nums', fontSize: 11 }}>
        {(result.fail_count ?? 0) > 0
          ? <span style={{ fontSize: 9, fontWeight: 700, padding: '1px 6px', borderRadius: 8, background: 'rgba(239,68,68,0.12)', color: '#ef4444' }}>✗{result.fail_count}</span>
          : <span style={{ fontSize: 9, color: 'var(--color-brand-muted)' }}>0</span>
        }
      </span>
      {result.is_restricted
        ? <span style={{ fontSize: 9, fontWeight: 700, padding: '2px 8px', borderRadius: 10, background: 'rgba(245,158,11,0.1)', color: '#f59e0b' }}>TCP only</span>
        : <span style={{ fontSize: 9, fontWeight: 700, padding: '2px 8px', borderRadius: 10, background: 'rgba(16,185,129,0.1)', color: '#10b981' }}>QUIC+TCP</span>
      }
    </div>
  );
};

// ─────────────────────────────────────────────────────────────────────────────
// Organism A: Instrumentation Cockpit
// ─────────────────────────────────────────────────────────────────────────────
const InstrumentationCockpit: React.FC = () => {
  const { tunnelStatus, engineStatus, config, networkMetrics, scanProgress } = useWarpStore();
  const sc = STATUS_CFG[tunnelStatus];

  return (
    <div style={{ display: 'grid', gridTemplateColumns: 'repeat(3, 1fr)', gap: 16 }}>
      {/* Card 1: Uplink Topology */}
      <SectionCard title="Uplink Topology" subtitle="Active protocol & edge node">
        <div style={{ display: 'flex', flexDirection: 'column', gap: 12 }}>
          <div style={{ display: 'flex', alignItems: 'center', gap: 10 }}>
            <PulseDot
              color={sc.color}
              active={tunnelStatus === 'active' || tunnelStatus === 'linking'}
              size={10}
            />
            <span style={{
              fontSize: 12, fontWeight: 700, letterSpacing: 0.5,
              color: sc.color, textTransform: 'uppercase',
            }}>
              {sc.label}
            </span>
          </div>

          <div style={{ padding: '10px 14px', borderRadius: 10, background: sc.bg, border: `1px solid ${sc.border}` }}>
            <div style={{ fontSize: 10, color: 'var(--color-brand-muted)', marginBottom: 4 }}>Transport</div>
            <div style={{ fontSize: 15, fontWeight: 700, color: 'var(--color-brand-heading)', textTransform: 'uppercase' }}>
              {engineStatus?.transport_mode || config?.transport_mode || 'MASQUE'}
            </div>
          </div>

          {engineStatus?.active_endpoint && (
            <div style={{ padding: '10px 14px', borderRadius: 10, background: 'var(--color-brand-bg)' }}>
              <div style={{ fontSize: 10, color: 'var(--color-brand-muted)', marginBottom: 4 }}>Active Edge Node</div>
              <div style={{
                fontSize: 12, fontWeight: 600, color: '#6366f1',
                fontFamily: 'monospace', wordBreak: 'break-all',
              }}>
                {engineStatus.active_endpoint}
              </div>
            </div>
          )}

          {engineStatus?.uptime && (
            <div style={{ display: 'flex', alignItems: 'center', gap: 6, fontSize: 11, color: 'var(--color-brand-muted)' }}>
              <FiClock size={11} />
              Uptime: <span style={{ color: 'var(--color-brand-text)', fontWeight: 600 }}>{engineStatus.uptime}</span>
            </div>
          )}

          {tunnelStatus === 'sweeping' && scanProgress && (
            <div style={{ marginTop: 4 }}>
              <div style={{ display: 'flex', justifyContent: 'space-between', fontSize: 10, color: 'var(--color-brand-muted)', marginBottom: 6 }}>
                <span>Scanning edge nodes…</span>
                <span>{scanProgress.scanned}/{scanProgress.total_targets}</span>
              </div>
              <div style={{ height: 4, borderRadius: 2, background: 'var(--color-brand-bg)', overflow: 'hidden' }}>
                <div style={{
                  height: '100%',
                  width: `${scanProgress.progress}%`,
                  background: 'linear-gradient(90deg, #f59e0b, #fbbf24)',
                  transition: 'width 0.4s ease-out',
                  borderRadius: 2,
                }} />
              </div>
            </div>
          )}
        </div>
      </SectionCard>

      {/* Card 2: Allocation Meter */}
      <SectionCard title="Quota Allocation" subtitle="Data pool consumption tracker">
        {config?.active_account ? (
          <div style={{ display: 'flex', flexDirection: 'column', gap: 16 }}>
            <div style={{ display: 'flex', alignItems: 'center', gap: 10 }}>
              <div style={{
                width: 36, height: 36, borderRadius: 10,
                background: config.active_account.account_type === 'warp_plus'
                  ? 'linear-gradient(135deg, #f59e0b, #d97706)'
                  : 'linear-gradient(135deg, #6366f1, #4f46e5)',
                display: 'flex', alignItems: 'center', justifyContent: 'center',
              }}>
                <FiZap size={16} color="#fff" />
              </div>
              <div>
                <div style={{ fontSize: 13, fontWeight: 700, color: 'var(--color-brand-heading)' }}>
                  {config.active_account.account_type === 'warp_plus' ? 'WARP+' : 'WARP Free'}
                </div>
                <div style={{ fontSize: 10, color: 'var(--color-brand-muted)', fontFamily: 'monospace' }}>
                  {config.active_account.device_id.substring(0, 16)}…
                </div>
              </div>
            </div>

            <QuotaMeter
              used={config.active_account.used_quota}
              total={config.active_account.total_quota}
            />

            <div style={{
              display: 'flex', alignItems: 'center', gap: 6, fontSize: 11,
              padding: '8px 12px', borderRadius: 8,
              background: config.active_account.is_functional ? 'rgba(16,185,129,0.08)' : 'rgba(239,68,68,0.08)',
              color: config.active_account.is_functional ? '#10b981' : '#ef4444',
              border: `1px solid ${config.active_account.is_functional ? 'rgba(16,185,129,0.2)' : 'rgba(239,68,68,0.2)'}`,
            }}>
              {config.active_account.is_functional ? <FiCheck size={11} /> : <FiX size={11} />}
              <span style={{ fontWeight: 600 }}>
                {config.active_account.is_functional ? 'Account Functional' : 'Account Invalid'}
              </span>
            </div>
          </div>
        ) : (
          <div style={{ textAlign: 'center', padding: '20px 0', color: 'var(--color-brand-muted)', fontSize: 12 }}>
            <FiUser size={28} style={{ marginBottom: 8, opacity: 0.4 }} />
            <div>No active account</div>
            <div style={{ fontSize: 10, marginTop: 4 }}>Register an account in Fleet Pool below</div>
          </div>
        )}
      </SectionCard>

      {/* Card 3: Throughput Curve */}
      <SectionCard title="Latency Curve" subtitle="Historical RTT measurements">
        {networkMetrics.length > 1 ? (
          <div style={{ display: 'flex', flexDirection: 'column', gap: 12 }}>
            <div style={{ display: 'flex', justifyContent: 'space-between', alignItems: 'baseline' }}>
              <span style={{ fontSize: 10, color: 'var(--color-brand-muted)' }}>Live RTT</span>
              <span style={{ fontSize: 18, fontWeight: 700, color: '#10b981', fontVariantNumeric: 'tabular-nums' }}>
                {networkMetrics[networkMetrics.length - 1]?.latencyMs.toFixed(0)}
                <span style={{ fontSize: 10, color: 'var(--color-brand-muted)', marginLeft: 2 }}>ms</span>
              </span>
            </div>
            <ThroughputSparkline data={networkMetrics} color="#10b981" width={220} height={60} />
          </div>
        ) : (
          <div style={{ height: 80, display: 'flex', flexDirection: 'column', alignItems: 'center', justifyContent: 'center', gap: 8 }}>
            <FiActivity size={24} style={{ color: 'var(--color-brand-muted)', opacity: 0.5 }} />
            <div style={{ fontSize: 11, color: 'var(--color-brand-muted)' }}>
              {tunnelStatus === 'active' ? 'Collecting metrics…' : 'Start tunnel to see live metrics'}
            </div>
          </div>
        )}
      </SectionCard>
    </div>
  );
};

// ─────────────────────────────────────────────────────────────────────────────
// Organism B: Core Configuration Grid
// ─────────────────────────────────────────────────────────────────────────────
const CoreConfigGrid: React.FC = () => {
  const { config, commitEngineTuning, globalLoading } = useWarpStore();

  const [transportMode, setTransportMode] = useState<'masque' | 'masque_h2' | 'wireguard'>('masque');
  const [targetSni, setTargetSni] = useState('consumer-masque.cloudflareclient.com');
  const [socksPort, setSocksPort] = useState(10880);
  const [httpPort, setHttpPort] = useState(10881);
  const [isDirty, setIsDirty] = useState(false);

  useEffect(() => {
    if (config) {
      setTransportMode((config.transport_mode as 'masque' | 'masque_h2' | 'wireguard') || 'masque');
      setTargetSni(config.target_sni || 'consumer-masque.cloudflareclient.com');
      setSocksPort(config.socks_port || 10880);
      setHttpPort(config.http_port || 10881);
      setIsDirty(false);
    }
  }, [config]);

  const markDirty = () => setIsDirty(true);

  const socksPortConflict = socksPort === httpPort;
  const portInvalid = (p: number) => p < 1024 || p > 65535;

  const handleSave = async (e: React.FormEvent) => {
    e.preventDefault();
    await commitEngineTuning({ transport_mode: transportMode, target_sni: targetSni, socks_port: socksPort, http_port: httpPort });
    setIsDirty(false);
  };

  return (
    <SectionCard
      title="Engine Tuning"
      subtitle="Transport protocol, SNI mask, and socket allocation"
      action={
        isDirty && (
          <span style={{
            fontSize: 10, fontWeight: 700, padding: '3px 10px', borderRadius: 12,
            background: 'rgba(245,158,11,0.1)', color: '#f59e0b',
            border: '1px solid rgba(245,158,11,0.2)',
          }}>
            UNSAVED CHANGES
          </span>
        )
      }
    >
      <form onSubmit={handleSave} style={{ display: 'flex', flexDirection: 'column', gap: 18 }}>
        {/* Transport Mode Select */}
        <div>
          <label style={labelStyle}>Transport Mode</label>
          <div style={{ display: 'grid', gridTemplateColumns: '1fr 1fr 1fr', gap: 10 }}>
            {([
              { id: 'masque',    label: 'MASQUE',     sub: 'HTTP/3 over QUIC (UDP)', badge: null },
              { id: 'masque_h2', label: 'MASQUE TCP',  sub: 'HTTP/2 over TLS (UDP blocked)', badge: '🔒 UDP Bypass' },
              { id: 'wireguard', label: 'WireGuard',   sub: 'gVisor userspace netstack', badge: null },
            ] as const).map(({ id, label, sub, badge }) => (
              <label key={id} style={{
                display: 'flex', alignItems: 'flex-start', gap: 10,
                padding: '12px 14px', borderRadius: 10, cursor: 'pointer',
                border: `1.5px solid ${transportMode === id ? 'var(--color-brand)' : 'var(--color-brand-border)'}`,
                background: transportMode === id ? 'rgba(255,107,44,0.06)' : 'var(--color-brand-bg)',
                transition: 'all 0.2s ease',
              }}>
                <input
                  type="radio"
                  name="transportMode"
                  value={id}
                  checked={transportMode === id}
                  onChange={() => { setTransportMode(id); markDirty(); }}
                  style={{ display: 'none' }}
                />
                <div style={{
                  width: 16, height: 16, borderRadius: '50%', flexShrink: 0, marginTop: 2,
                  border: `2px solid ${transportMode === id ? 'var(--color-brand)' : 'var(--color-brand-border)'}`,
                  background: transportMode === id ? 'var(--color-brand)' : 'transparent',
                  display: 'flex', alignItems: 'center', justifyContent: 'center',
                  transition: 'all 0.2s ease',
                }}>
                  {transportMode === id && <div style={{ width: 5, height: 5, borderRadius: '50%', background: '#fff' }} />}
                </div>
                <div>
                  <div style={{ fontSize: 12, fontWeight: 700, color: 'var(--color-brand-heading)', textTransform: 'uppercase' }}>
                    {label}
                  </div>
                  <div style={{ fontSize: 10, color: 'var(--color-brand-muted)', marginTop: 1 }}>{sub}</div>
                  {badge && (
                    <div style={{
                      marginTop: 5, display: 'inline-block',
                      fontSize: 9, fontWeight: 700, padding: '2px 7px', borderRadius: 20,
                      background: 'rgba(16,185,129,0.1)', color: '#10b981',
                      border: '1px solid rgba(16,185,129,0.2)',
                    }}>{badge}</div>
                  )}
                </div>
              </label>
            ))}
          </div>
        </div>

        {/* SNI Input (MASQUE/H2 modes only) */}
        {(transportMode === 'masque' || transportMode === 'masque_h2') && (
          <div>
            <label style={labelStyle}>Target SNI Mask</label>
            <input
              style={{
                ...inputStyle,
                borderColor: targetSni.includes(' ') ? '#ef4444' : 'var(--color-brand-border)',
              }}
              value={targetSni}
              placeholder="consumer-masque.cloudflareclient.com"
              onChange={(e) => { setTargetSni(e.target.value.replace(/\s/g, '')); markDirty(); }}
            />
            <div style={{ fontSize: 10, color: 'var(--color-brand-muted)', marginTop: 4 }}>
              The SNI hostname injected into TLS ClientHello for edge masking
            </div>
          </div>
        )}

        {/* Socket Allocation Pair */}
        <div style={{ display: 'grid', gridTemplateColumns: '1fr 1fr', gap: 14 }}>
          <div>
            <label style={labelStyle}>SOCKS5 Port</label>
            <input
              type="number"
              style={{
                ...inputStyle,
                borderColor: portInvalid(socksPort) || socksPortConflict ? '#ef4444' : 'var(--color-brand-border)',
              }}
              value={socksPort}
              min={1024} max={65535}
              onChange={(e) => { setSocksPort(Number(e.target.value)); markDirty(); }}
            />
            {portInvalid(socksPort) && (
              <div style={{ fontSize: 10, color: '#ef4444', marginTop: 4 }}>Must be between 1024–65535</div>
            )}
          </div>
          <div>
            <label style={labelStyle}>HTTP Proxy Port</label>
            <input
              type="number"
              style={{
                ...inputStyle,
                borderColor: portInvalid(httpPort) || socksPortConflict ? '#ef4444' : 'var(--color-brand-border)',
              }}
              value={httpPort}
              min={1024} max={65535}
              onChange={(e) => { setHttpPort(Number(e.target.value)); markDirty(); }}
            />
          </div>
        </div>
        {socksPortConflict && (
          <div style={{ fontSize: 11, color: '#ef4444', display: 'flex', alignItems: 'center', gap: 6 }}>
            <FiAlertTriangle size={12} />
            SOCKS5 and HTTP ports cannot be identical
          </div>
        )}

        <button
          type="submit"
          disabled={globalLoading || !isDirty || socksPortConflict || portInvalid(socksPort) || portInvalid(httpPort)}
          style={{
            padding: '10px 20px', borderRadius: 9, border: 'none', fontWeight: 700,
            fontSize: 12, cursor: (!isDirty || globalLoading) ? 'not-allowed' : 'pointer',
            background: (!isDirty || globalLoading)
              ? 'var(--color-brand-border)'
              : 'linear-gradient(135deg, var(--color-brand), #e55a1e)',
            color: '#fff', transition: 'all 0.2s ease',
            alignSelf: 'flex-start',
            boxShadow: isDirty ? '0 4px 12px rgba(255,107,44,0.25)' : 'none',
          }}
        >
          {globalLoading ? 'Applying…' : 'Apply Configuration'}
        </button>
      </form>
    </SectionCard>
  );
};

// ─────────────────────────────────────────────────────────────────────────────
// Organism C: Master Execution Panel
// ─────────────────────────────────────────────────────────────────────────────
const MasterExecutionPanel: React.FC = () => {
  const { tunnelStatus, engineStatus, globalLoading, error, toggleTunnelLifecycle, fetchStatus, clearError } = useWarpStore();
  const sc = STATUS_CFG[tunnelStatus];
  const isRunning = engineStatus?.state === 'running';
  const isStarting = engineStatus?.state === 'starting';
  // Only disable the toggle button while the HTTP request is in-flight (globalLoading)
  // NOT based on tunnelStatus — that was causing the "stuck" bug
  const isTransitioning = globalLoading;
  const isTCPFallback = engineStatus?.tcp_fallback === true;

  // Refresh status periodically while running
  useEffect(() => {
    if (!isRunning && !isStarting) return;
    const id = setInterval(() => fetchStatus(), 8000);
    return () => clearInterval(id);
  }, [isRunning, isStarting, fetchStatus]);

  const buttonStyle = (active: boolean): React.CSSProperties => ({
    padding: '14px 36px',
    borderRadius: 12,
    border: active ? '2px solid rgba(239,68,68,0.6)' : '2px solid rgba(16,185,129,0.4)',
    cursor: isTransitioning ? 'not-allowed' : 'pointer',
    fontSize: 14, fontWeight: 800, letterSpacing: 0.5,
    transition: 'all 0.3s ease',
    background: active
      ? 'linear-gradient(135deg, #dc2626, #b91c1c)'
      : 'linear-gradient(135deg, #059669, #047857)',
    color: '#fff',
    opacity: isTransitioning ? 0.65 : 1,
    animation: isRunning ? 'warp-glow-pulse 2.5s ease-in-out infinite' : 'none',
    display: 'flex', alignItems: 'center', gap: 10,
  });

  return (
    <div style={{
      borderRadius: 16,
      border: `1.5px solid ${sc.border}`,
      background: sc.heroGradient,
      overflow: 'hidden',
      position: 'relative',
    }}>
      {/* Background mesh */}
      <div style={{
        position: 'absolute', inset: 0, opacity: 0.07,
        backgroundImage: `radial-gradient(circle at 20% 50%, ${sc.color} 0%, transparent 50%), radial-gradient(circle at 80% 50%, #6366f1 0%, transparent 50%)`,
      }} />

      {/* Shimmer overlay during transitions */}
      {isTransitioning && (
        <div className="warp-shimmer-bg" style={{ position: 'absolute', inset: 0, zIndex: 0 }} />
      )}

      <div style={{ position: 'relative', zIndex: 1, padding: '28px 32px' }}>
        <div style={{ display: 'flex', justifyContent: 'space-between', alignItems: 'center', flexWrap: 'wrap', gap: 20 }}>
          <div>
            <div style={{ fontSize: 10, textTransform: 'uppercase', letterSpacing: 2, color: 'rgba(255,255,255,0.4)', marginBottom: 8 }}>
              WARP+ Core Engine
            </div>
            <h1 style={{ fontSize: 26, fontWeight: 800, margin: 0, color: '#fff', letterSpacing: -0.5, display: 'flex', alignItems: 'center', gap: 10 }}>
              <span style={{ animation: isRunning ? 'warp-float 3s ease-in-out infinite' : 'none' }}>🛡</span>
              Cloudflare WARP+
            </h1>
            <p style={{ fontSize: 12, color: 'rgba(255,255,255,0.5)', marginTop: 8, maxWidth: 420, lineHeight: 1.6 }}>
              {tunnelStatus === 'dormant' && 'Engine offline. Register an account and run a scan before initiating.'}
              {tunnelStatus === 'sweeping' && 'Sweeping Cloudflare edge nodes for optimal connection endpoints…'}
              {tunnelStatus === 'linking' && (isTCPFallback
                ? 'Establishing TCP/TLS tunnel (UDP is blocked by your ISP)…'
                : 'Establishing QUIC session and validating captive portal trace…')}
              {tunnelStatus === 'active' && (isTCPFallback
                ? 'Tunnel active via TCP/TLS fallback (QUIC/UDP is ISP-blocked).'
                : 'Tunnel active. Traffic is flowing through Cloudflare WARP edge network.')}
              {tunnelStatus === 'restricted' && 'Handshake succeeded but trace validation failed. Endpoint may be blocked.'}
            </p>
          </div>

          <div style={{ display: 'flex', flexDirection: 'column', alignItems: 'flex-end', gap: 12 }}>
            {/* Status badge */}
            <div style={{ display: 'flex', alignItems: 'center', gap: 8 }}>
              {isTCPFallback && isRunning && (
                <span style={{
                  fontSize: 9, fontWeight: 700, padding: '3px 8px', borderRadius: 10,
                  background: 'rgba(245,158,11,0.15)', color: '#f59e0b',
                  border: '1px solid rgba(245,158,11,0.3)',
                }}>TCP FALLBACK</span>
              )}
              <div style={{
                display: 'flex', alignItems: 'center', gap: 6,
                padding: '6px 14px', borderRadius: 20,
                background: sc.bg, border: `1px solid ${sc.border}`,
                fontSize: 11, fontWeight: 700, color: sc.color,
              }}>
                <PulseDot color={sc.color} active={tunnelStatus === 'active' || tunnelStatus === 'linking'} />
                {sc.label}
              </div>
            </div>

            {/* Main toggle — never permanently disabled, even on error */}
            <button
              id="warp-tunnel-toggle"
              onClick={() => { clearError(); toggleTunnelLifecycle(); }}
              disabled={isTransitioning}
              style={buttonStyle(isRunning || isStarting)}
            >
              {isTransitioning ? (
                <><FiRefreshCw size={16} className="warp-spin" /> Processing…</>
              ) : (isRunning || isStarting) ? (
                <><FiX size={16} /> Kill Tunnel Process</>
              ) : (
                <><FiWifi size={16} /> Initiate Tunnel Link</>
              )}
            </button>
          </div>
        </div>

        {/* Live stats strip */}
        {isRunning && (
          <div style={{
            display: 'grid', gridTemplateColumns: 'repeat(6, 1fr)', gap: 16,
            marginTop: 24, paddingTop: 20, borderTop: '1px solid rgba(255,255,255,0.07)',
          }}>
            <StatCell label="Transport" value={engineStatus?.transport_mode?.toUpperCase() || '—'} color={isTCPFallback ? '#f59e0b' : '#10b981'} />
            <StatCell label="Endpoint" value={engineStatus?.active_endpoint || '—'} color="#6366f1" />
            <StatCell label="SOCKS Port" value={engineStatus?.socks_port || '—'} color="#6366f1" />
            <StatCell label="HTTP Port" value={engineStatus?.http_port || '—'} color="#6366f1" />
            <StatCell label="Account" value={engineStatus?.account_type || '—'} color="#10b981" />
            <StatCell label="Trace" value={engineStatus?.last_trace_ok ? '✓ VALID' : '✗ FAILED'} color={engineStatus?.last_trace_ok ? '#10b981' : '#ef4444'} />
          </div>
        )}

        {/* Error banner */}
        {error && (
          <div className="warp-slide-in" style={{
            marginTop: 16, padding: '10px 14px', borderRadius: 9,
            background: 'rgba(239,68,68,0.12)', border: '1px solid rgba(239,68,68,0.3)',
            color: '#fca5a5', fontSize: 12, display: 'flex', alignItems: 'center', gap: 8,
          }}>
            <FiAlertTriangle size={13} />
            <span style={{ flex: 1 }}>{error}</span>
            <button onClick={clearError} style={{ background: 'none', border: 'none', color: '#fca5a5', cursor: 'pointer', padding: 0 }}>
              <FiX size={13} />
            </button>
          </div>
        )}
      </div>
    </div>
  );
};

// ─────────────────────────────────────────────────────────────────────────────
// Organism D: Fleet Pool Inventory
// ─────────────────────────────────────────────────────────────────────────────
const FleetPoolInventory: React.FC = () => {
  const { accounts, globalLoading, licenseError, provisionNewLicense, deleteAccount, activateAccount, config } = useWarpStore();
  const [licenseKey, setLicenseKey] = useState('');
  const [showForm, setShowForm] = useState(false);
  const [confirmDelete, setConfirmDelete] = useState<number | null>(null);

  const handleRegister = async () => {
    try {
      await provisionNewLicense(licenseKey);
      setLicenseKey('');
      setShowForm(false);
    } catch {
      // error shown from store
    }
  };

  return (
    <SectionCard
      title="Fleet Pool Inventory"
      subtitle="Registered WARP account profiles"
      action={
        <button
          id="warp-add-account"
          onClick={() => setShowForm(!showForm)}
          style={{
            display: 'flex', alignItems: 'center', gap: 6,
            padding: '7px 14px', borderRadius: 8, border: 'none',
            background: 'linear-gradient(135deg, var(--color-brand), #e55a1e)',
            color: '#fff', fontSize: 11, fontWeight: 700, cursor: 'pointer',
            boxShadow: '0 2px 8px rgba(255,107,44,0.25)',
          }}
        >
          <FiPlus size={13} />
          Register Account
        </button>
      }
    >
      {/* Register form */}
      {showForm && (
        <div className="warp-slide-in" style={{
          marginBottom: 16, padding: '16px', borderRadius: 10,
          background: 'var(--color-brand-bg)', border: '1px solid var(--color-brand-border)',
        }}>
          <div style={{ fontSize: 12, fontWeight: 700, color: 'var(--color-brand-heading)', marginBottom: 12 }}>
            Register New WARP Account
          </div>
          <div style={{ display: 'flex', gap: 10, alignItems: 'flex-start' }}>
            <div style={{ flex: 1 }}>
              <label style={labelStyle}>WARP+ License Key (optional)</label>
              <input
                style={{
                  ...inputStyle,
                  borderColor: licenseError ? '#ef4444' : 'var(--color-brand-border)',
                }}
                placeholder="xxxx-xxxx-xxxx-xxxx-xxxx (26 chars, or leave empty for free)"
                value={licenseKey}
                maxLength={26}
                onChange={(e) => setLicenseKey(e.target.value)}
              />
              {licenseError && (
                <div style={{ fontSize: 11, color: '#ef4444', marginTop: 5, display: 'flex', alignItems: 'center', gap: 5 }}>
                  <FiAlertTriangle size={11} /> {licenseError}
                </div>
              )}
              <div style={{ fontSize: 10, color: 'var(--color-brand-muted)', marginTop: 5 }}>
                Leave blank to register a free WARP account. The API call uses uTLS Chrome fingerprinting.
              </div>
            </div>
            <div style={{ display: 'flex', gap: 8, paddingTop: 22 }}>
              <button
                onClick={handleRegister}
                disabled={globalLoading || (licenseKey.length > 0 && !/^[A-Za-z0-9]{8}-[A-Za-z0-9]{8}-[A-Za-z0-9]{8}$/.test(licenseKey))}
                style={{
                  padding: '9px 16px', borderRadius: 8, border: 'none', fontWeight: 700,
                  fontSize: 12, cursor: globalLoading ? 'not-allowed' : 'pointer',
                  background: 'linear-gradient(135deg, #10b981, #059669)',
                  color: '#fff', whiteSpace: 'nowrap',
                  opacity: globalLoading ? 0.6 : 1,
                }}
              >
                {globalLoading ? 'Registering…' : 'Register'}
              </button>
              <button
                onClick={() => { setShowForm(false); setLicenseKey(''); }}
                style={{
                  padding: '9px 14px', borderRadius: 8, border: '1px solid var(--color-brand-border)',
                  background: 'transparent', color: 'var(--color-brand-text)', fontSize: 12, cursor: 'pointer',
                }}
              >
                Cancel
              </button>
            </div>
          </div>
        </div>
      )}

      {/* Account list */}
      {accounts.length === 0 ? (
        <div style={{ textAlign: 'center', padding: '32px 0', color: 'var(--color-brand-muted)' }}>
          <FiServer size={32} style={{ marginBottom: 10, opacity: 0.35 }} />
          <div style={{ fontSize: 13, fontWeight: 600 }}>No accounts registered</div>
          <div style={{ fontSize: 11, marginTop: 4 }}>Click "Register Account" to provision a new WARP device</div>
        </div>
      ) : (
        <div style={{ display: 'flex', flexDirection: 'column', gap: 10 }}>
          {accounts.map((acc) => {
            const isActive = config?.active_account?.id === acc.ID;
            const pct = formatQuotaPct(acc.used_quota, acc.total_quota);

            return (
              <div key={acc.ID} className="warp-slide-in" style={{
                padding: '14px 16px',
                borderRadius: 10,
                border: `1.5px solid ${isActive ? 'rgba(255,107,44,0.45)' : 'var(--color-brand-border)'}`,
                background: isActive ? 'rgba(255,107,44,0.05)' : 'var(--color-brand-bg)',
                display: 'grid',
                gridTemplateColumns: 'auto 1fr auto auto auto auto',
                alignItems: 'center',
                gap: 14,
                transition: 'border-color 0.25s ease, background 0.25s ease',
              }}>
                {/* Account type icon */}
                <div style={{
                  width: 36, height: 36, borderRadius: 9,
                  background: acc.account_type === 'warp_plus'
                    ? 'linear-gradient(135deg, #f59e0b, #d97706)'
                    : 'linear-gradient(135deg, #6366f1, #4f46e5)',
                  display: 'flex', alignItems: 'center', justifyContent: 'center',
                  flexShrink: 0,
                }}>
                  <FiZap size={16} color="#fff" />
                </div>

                {/* Identity & badges */}
                <div>
                  <div style={{ display: 'flex', alignItems: 'center', gap: 7, flexWrap: 'wrap' }}>
                    <span style={{ fontSize: 12, fontWeight: 700, color: 'var(--color-brand-heading)' }}>
                      {acc.account_type === 'warp_plus' ? 'WARP+' : 'WARP Free'}
                    </span>
                    {isActive && (
                      <span style={{
                        fontSize: 9, fontWeight: 700, padding: '2px 8px', borderRadius: 10,
                        background: 'rgba(255,107,44,0.15)', color: 'var(--color-brand)',
                        border: '1px solid rgba(255,107,44,0.3)',
                      }}>● ACTIVE</span>
                    )}
                    {!acc.is_functional && (
                      <span style={{
                        fontSize: 9, fontWeight: 700, padding: '2px 7px', borderRadius: 10,
                        background: 'rgba(239,68,68,0.1)', color: '#ef4444',
                        border: '1px solid rgba(239,68,68,0.2)',
                      }}>INVALID</span>
                    )}
                  </div>
                  <div style={{ fontSize: 10, color: 'var(--color-brand-muted)', fontFamily: 'monospace', marginTop: 2 }}>
                    {acc.device_id?.substring(0, 20)}…
                  </div>
                </div>

                {/* Quota mini-bar */}
                <div style={{ minWidth: 90 }}>
                  <div style={{ display: 'flex', justifyContent: 'space-between', fontSize: 9, color: 'var(--color-brand-muted)', marginBottom: 4 }}>
                    <span>Quota</span><span>{pct}%</span>
                  </div>
                  <div style={{ height: 4, borderRadius: 2, background: 'var(--color-brand-border)', overflow: 'hidden', width: 90 }}>
                    <div style={{
                      height: '100%', width: `${pct}%`, borderRadius: 2,
                      background: pct > 80 ? '#f59e0b' : '#10b981',
                      transition: 'width 0.4s ease',
                    }} />
                  </div>
                </div>

                {/* Token snippet */}
                <div style={{ fontSize: 10, fontFamily: 'monospace', color: 'var(--color-brand-muted)', maxWidth: 110, overflow: 'hidden', textOverflow: 'ellipsis', whiteSpace: 'nowrap' }}>
                  {acc.token?.substring(0, 18)}…
                </div>

                {/* Activate button */}
                {isActive ? (
                  <div style={{
                    padding: '5px 10px', borderRadius: 7, fontSize: 10, fontWeight: 700,
                    color: 'var(--color-brand)', border: '1px solid rgba(255,107,44,0.3)',
                    background: 'rgba(255,107,44,0.06)',
                  }}>
                    Active
                  </div>
                ) : (
                  <button
                    id={`warp-activate-${acc.ID}`}
                    onClick={() => activateAccount(acc.ID)}
                    disabled={globalLoading || !acc.is_functional}
                    title={!acc.is_functional ? 'Account is invalid' : 'Set as active account'}
                    style={{
                      padding: '5px 12px', borderRadius: 7, border: '1px solid rgba(16,185,129,0.35)',
                      background: 'rgba(16,185,129,0.08)', color: '#10b981',
                      fontSize: 10, fontWeight: 700, cursor: (!acc.is_functional || globalLoading) ? 'not-allowed' : 'pointer',
                      whiteSpace: 'nowrap', transition: 'all 0.2s ease',
                      opacity: (!acc.is_functional || globalLoading) ? 0.45 : 1,
                      display: 'flex', alignItems: 'center', gap: 5,
                    }}
                  >
                    <FiCheck size={10} /> Set Active
                  </button>
                )}

                {/* Delete */}
                {confirmDelete === acc.ID ? (
                  <div style={{ display: 'flex', gap: 5 }}>
                    <button
                      onClick={() => { deleteAccount(acc.ID); setConfirmDelete(null); }}
                      style={{
                        padding: '5px 9px', borderRadius: 7, border: 'none',
                        background: '#ef4444', color: '#fff', fontSize: 10, fontWeight: 700, cursor: 'pointer',
                      }}
                    >
                      Confirm
                    </button>
                    <button
                      onClick={() => setConfirmDelete(null)}
                      style={{
                        padding: '5px 9px', borderRadius: 7, border: '1px solid var(--color-brand-border)',
                        background: 'transparent', color: 'var(--color-brand-text)', fontSize: 10, cursor: 'pointer',
                      }}
                    >
                      Cancel
                    </button>
                  </div>
                ) : (
                  <button
                    onClick={() => setConfirmDelete(acc.ID)}
                    style={{
                      padding: '7px 8px', borderRadius: 8, border: '1px solid var(--color-brand-border)',
                      background: 'transparent', color: 'var(--color-brand-muted)', cursor: 'pointer',
                      display: 'flex', alignItems: 'center',
                      transition: 'all 0.15s ease',
                    }}
                    title="Delete account"
                  >
                    <FiTrash2 size={13} />
                  </button>
                )}
              </div>
            );
          })}
        </div>
      )}
    </SectionCard>
  );
};

// ─────────────────────────────────────────────────────────────────────────────
// Organism: Scan Results Table
// ─────────────────────────────────────────────────────────────────────────────
const ScanResultsTable: React.FC = () => {
  const {
    scanResults, scanProgress, fetchScanResults, tunnelStatus,
    scanEvents, fetchScanEvents, initiateManualEdgeScan, stopEdgeScan, scanLoading,
  } = useWarpStore();

  const [mode, setMode] = useState<'masque' | 'masque_h2' | 'wireguard'>('masque');
  const [workers, setWorkers] = useState(0); // 0 = auto (CPU×4)
  const [timeoutMs, setTimeoutMs] = useState(2000);
  const eventLogRef = useRef<HTMLDivElement>(null);

  const isScanning = tunnelStatus === 'sweeping' || (scanProgress?.is_running ?? false);

  useEffect(() => {
    fetchScanResults(mode);
  }, [mode, fetchScanResults]);

  // Poll scan results + events while scanning
  useEffect(() => {
    if (!isScanning) return;
    const resultsId = setInterval(() => fetchScanResults(mode), 3000);
    const eventsId  = setInterval(() => fetchScanEvents(), 800);
    return () => { clearInterval(resultsId); clearInterval(eventsId); };
  }, [isScanning, mode, fetchScanResults, fetchScanEvents]);

  // Auto-scroll event log to bottom
  useEffect(() => {
    if (eventLogRef.current) {
      eventLogRef.current.scrollTop = eventLogRef.current.scrollHeight;
    }
  }, [scanEvents]);

  const handleStartScan = () => initiateManualEdgeScan(workers || undefined, timeoutMs);

  const eventColor = (status: ScanEvent['status']) => {
    if (status === 'pass')   return '#10b981';
    if (status === 'tcp_ok') return '#f59e0b';
    return '#6b7280';
  };
  const eventIcon = (status: ScanEvent['status']) => {
    if (status === 'pass')   return '✓';
    if (status === 'tcp_ok') return '⚠';
    return '✗';
  };

  return (
    <div style={{ display: 'flex', flexDirection: 'column', gap: 16 }}>

      {/* ── Scan Configuration Card ─────────────────────── */}
      <SectionCard title="Edge Node Scanner" subtitle="Parallel endpoint discovery across Cloudflare WARP CIDRs">

        {/* Config row */}
        <div style={{ display: 'flex', gap: 12, alignItems: 'flex-end', flexWrap: 'wrap', marginBottom: 16 }}>
          <div style={{ flex: '0 0 auto' }}>
            <label style={labelStyle}>Workers (0 = auto CPU×4)</label>
            <input
              type="number" min={0} max={500}
              style={{ ...inputStyle, width: 140 }}
              value={workers}
              onChange={(e) => setWorkers(Number(e.target.value))}
            />
          </div>
          <div style={{ flex: '0 0 auto' }}>
            <label style={labelStyle}>Timeout (ms)</label>
            <input
              type="number" min={500} max={10000} step={500}
              style={{ ...inputStyle, width: 120 }}
              value={timeoutMs}
              onChange={(e) => setTimeoutMs(Number(e.target.value))}
            />
          </div>
          <div style={{ flex: '0 0 auto' }}>
            <label style={labelStyle}>Mode</label>
            <div style={{ display: 'flex', gap: 6 }}>
              {([
                { id: 'masque',    label: 'MASQUE' },
                { id: 'masque_h2', label: 'MASQUE TCP' },
                { id: 'wireguard', label: 'WireGuard' },
              ] as const).map(({ id, label }) => (
                <button key={id} onClick={() => setMode(id)} style={{
                  padding: '7px 12px', borderRadius: 7,
                  border: `1px solid ${mode === id ? 'var(--color-brand)' : 'var(--color-brand-border)'}`,
                  background: mode === id ? 'rgba(255,107,44,0.08)' : 'transparent',
                  color: mode === id ? 'var(--color-brand)' : 'var(--color-brand-muted)',
                  fontSize: 10, fontWeight: 700, textTransform: 'uppercase', cursor: 'pointer',
                }}>{label}</button>
              ))}
            </div>
          </div>

          {/* Start / Stop */}
          <div style={{ marginLeft: 'auto', display: 'flex', gap: 8, alignItems: 'flex-end' }}>
            {isScanning ? (
              <button id="warp-scan-stop" onClick={() => stopEdgeScan()} style={{
                padding: '8px 18px', borderRadius: 8, border: '1px solid rgba(239,68,68,0.4)',
                background: 'rgba(239,68,68,0.1)', color: '#ef4444', fontSize: 11, fontWeight: 700,
                cursor: 'pointer', display: 'flex', alignItems: 'center', gap: 7,
              }}>
                <FiX size={12} /> Stop Scan
              </button>
            ) : (
              <button id="warp-scan-trigger" onClick={handleStartScan} disabled={scanLoading} style={{
                padding: '8px 20px', borderRadius: 8, border: 'none',
                background: 'linear-gradient(135deg, var(--color-brand), #e55a1e)',
                color: '#fff', fontSize: 11, fontWeight: 700,
                cursor: scanLoading ? 'not-allowed' : 'pointer',
                display: 'flex', alignItems: 'center', gap: 7, opacity: scanLoading ? 0.6 : 1,
                boxShadow: '0 3px 10px rgba(255,107,44,0.25)',
              }}>
                {scanLoading ? <FiRefreshCw size={12} className="warp-spin" /> : <FiTarget size={12} />}
                {scanLoading ? 'Starting…' : 'Start Scan'}
              </button>
            )}
            <button onClick={() => fetchScanResults(mode)} style={{
              padding: '8px 10px', borderRadius: 8, border: '1px solid var(--color-brand-border)',
              background: 'transparent', cursor: 'pointer', display: 'flex', alignItems: 'center',
              color: 'var(--color-brand-muted)',
            }}>
              <FiRefreshCw size={12} />
            </button>
          </div>
        </div>

        {/* Progress bar */}
        {isScanning && scanProgress && (
          <div style={{ marginBottom: 14 }}>
            <div style={{ display: 'flex', justifyContent: 'space-between', fontSize: 10, color: 'var(--color-brand-muted)', marginBottom: 5 }}>
              <span>
                Scanning… {scanProgress.passed} passed / {scanProgress.failed} failed
                {scanProgress.udp_blocked && (
                  <span style={{ marginLeft: 10, color: '#f59e0b', fontWeight: 700 }}>
                    ⚠ UDP appears blocked by ISP — TCP probes only
                  </span>
                )}
              </span>
              <span>{scanProgress.scanned}/{scanProgress.total_targets} ({scanProgress.progress.toFixed(1)}%)</span>
            </div>
            <div style={{ height: 5, borderRadius: 3, background: 'var(--color-brand-bg)', overflow: 'hidden' }}>
              <div style={{
                height: '100%', borderRadius: 3,
                width: `${scanProgress.progress}%`,
                background: 'linear-gradient(90deg, #f59e0b, #f97316)',
                transition: 'width 0.4s ease-out',
              }} />
            </div>
          </div>
        )}

        {/* Real-time event log */}
        {(isScanning || scanEvents.length > 0) && (
          <div style={{ marginBottom: 12 }}>
            <div style={{ fontSize: 10, fontWeight: 700, textTransform: 'uppercase', letterSpacing: 1, color: 'var(--color-brand-muted)', marginBottom: 6 }}>
              Live Probe Log
            </div>
            <div
              ref={eventLogRef}
              style={{
                height: 220, overflowY: 'auto', overflowX: 'hidden',
                background: '#0a0a0f', borderRadius: 8, border: '1px solid var(--color-brand-border)',
                padding: '8px 12px', fontFamily: 'monospace', fontSize: 11,
              }}
            >
              {scanEvents.length === 0 ? (
                <div style={{ color: '#6b7280' }}>Waiting for probe results…</div>
              ) : (
                scanEvents.map((ev) => (
                  <div key={ev.index} style={{ display: 'flex', gap: 10, lineHeight: 1.7, whiteSpace: 'nowrap' }}>
                    <span style={{ color: '#4b5563', flexShrink: 0 }}>{ev.time}</span>
                    <span style={{ color: '#6366f1', flexShrink: 0, minWidth: 120 }}>{ev.ip}:{ev.port}</span>
                    <span style={{ color: eventColor(ev.status), fontWeight: 700, flexShrink: 0, minWidth: 10 }}>
                      {eventIcon(ev.status)}
                    </span>
                    <span style={{ color: eventColor(ev.status), flexShrink: 0, minWidth: 70 }}>
                      {ev.status === 'pass' ? 'PASS' : ev.status === 'tcp_ok' ? 'TCP ONLY' : 'FAIL'}
                    </span>
                    {ev.latency_ms ? (
                      <span style={{ color: '#9ca3af', flexShrink: 0, minWidth: 60 }}>
                        {ev.latency_ms.toFixed(0)}ms
                      </span>
                    ) : <span style={{ minWidth: 60 }} />}
                    <span style={{ color: '#6b7280', overflow: 'hidden', textOverflow: 'ellipsis' }}>
                      {ev.note}
                    </span>
                  </div>
                ))
              )}
            </div>
          </div>
        )}

        {/* Results table */}
        {scanResults.length > 0 && (
          <>
            <div style={{
              display: 'grid',
              gridTemplateColumns: '28px 1fr 70px 70px 100px 50px auto',
              padding: '8px 16px',
              fontSize: 9, fontWeight: 700, textTransform: 'uppercase', letterSpacing: 1,
              color: 'var(--color-brand-muted)',
              borderBottom: '1px solid var(--color-brand-border)',
              gap: 8,
            }}>
              <span>#</span>
              <span>Endpoint</span>
              <span>Latency</span>
              <span>Score</span>
              <span>ALPN</span>
              <span>Fails</span>
              <span>Status</span>
            </div>
            <div style={{ maxHeight: 280, overflowY: 'auto' }}>
              {scanResults.map((r, i) => (
                <ScanResultRow key={`${r.ip_address}:${r.port}`} result={r} rank={i + 1} />
              ))}
            </div>
          </>
        )}

        {scanResults.length === 0 && !isScanning && (
          <div style={{ textAlign: 'center', padding: '28px 0', color: 'var(--color-brand-muted)' }}>
            <FiTarget size={28} style={{ marginBottom: 8, opacity: 0.35 }} />
            <div style={{ fontSize: 13, fontWeight: 600 }}>No endpoints scanned yet</div>
            <div style={{ fontSize: 11, marginTop: 4 }}>Configure workers &amp; timeout above, then click Start Scan</div>
          </div>
        )}
      </SectionCard>
    </div>
  );
};

// ─────────────────────────────────────────────────────────────────────────────
// Main Page
// ─────────────────────────────────────────────────────────────────────────────
export const WarpPage: React.FC = () => {
  const { fetchGlobalConfig, fetchAccounts, fetchStatus } = useWarpStore();
  const initRef = useRef(false);

  useEffect(() => {
    if (initRef.current) return;
    initRef.current = true;
    fetchGlobalConfig();
    fetchAccounts();
    fetchStatus();
  }, [fetchGlobalConfig, fetchAccounts, fetchStatus]);

  return (
    <div style={{ display: 'flex', flexDirection: 'column', gap: 20, padding: 4 }}>
      <style>{WARP_STYLES}</style>

      {/* ── Organism C: Master Execution Panel ─── */}
      <MasterExecutionPanel />

      {/* ── Organism A: Instrumentation Cockpit ── */}
      <InstrumentationCockpit />

      {/* ── Two-column grid: Config + Fleet ────── */}
      <div style={{ display: 'grid', gridTemplateColumns: '1fr 1fr', gap: 20 }}>
        <CoreConfigGrid />
        <FleetPoolInventory />
      </div>

      {/* ── Scan Results Table ───────────────────  */}
      <ScanResultsTable />
    </div>
  );
};
