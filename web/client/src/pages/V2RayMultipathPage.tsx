import React, { useEffect, useCallback, useState } from 'react';
import { useMultipathStore, type ArteryStatus } from '../store/multipathStore';
import { FiCheckCircle, FiAlertTriangle, FiXCircle, FiActivity, FiPlayCircle, FiRefreshCw } from 'react-icons/fi';

/* ────────────────────────────────────────────────────────────────────────── */
/*  Helpers                                                                  */
/* ────────────────────────────────────────────────────────────────────────── */
function formatSpeed(bps: number): string {
  if (bps === 0) return '0 B/s';
  if (bps < 1024) return `${bps} B/s`;
  if (bps < 1024 * 1024) return `${(bps / 1024).toFixed(1)} KB/s`;
  return `${(bps / (1024 * 1024)).toFixed(2)} MB/s`;
}

function formatBytes(bytes: number): string {
  if (bytes === 0) return '0 B';
  if (bytes < 1024) return `${bytes} B`;
  if (bytes < 1024 * 1024) return `${(bytes / 1024).toFixed(1)} KB`;
  if (bytes < 1024 * 1024 * 1024) return `${(bytes / (1024 * 1024)).toFixed(2)} MB`;
  return `${(bytes / (1024 * 1024 * 1024)).toFixed(2)} GB`;
}

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
/*  DIAGNOSTICS PANEL                                                        */
/* ────────────────────────────────────────────────────────────────────────── */
const DiagnosticsPanel: React.FC<{
  results: any[] | null;
  loading: boolean;
  onRun: () => void;
}> = ({ results, loading, onRun }) => {
  const [revealIndex, setRevealIndex] = useState(-1);
  const [isAnimating, setIsAnimating] = useState(false);

  useEffect(() => {
    if (loading) {
      setRevealIndex(-1);
      setIsAnimating(true);
    }
  }, [loading]);

  useEffect(() => {
    if (!loading && results && results.length > 0 && isAnimating) {
      setRevealIndex(0);
      const timer = setInterval(() => {
        setRevealIndex((prev) => {
          if (prev >= results.length - 1) {
            clearInterval(timer);
            setIsAnimating(false);
            return results.length;
          }
          return prev + 1;
        });
      }, 900);
      return () => clearInterval(timer);
    }
  }, [loading, results, isAnimating]);

  const handleStart = () => {
    onRun();
  };

  const getOverallStatus = () => {
    if (!results || isAnimating || revealIndex < results.length) return 'pending';
    let hasError = false;
    let hasWarning = false;
    for (const r of results) {
      if (r.status === 'error') hasError = true;
      if (r.status === 'warning') hasWarning = true;
    }
    if (hasError) return 'error';
    if (hasWarning) return 'warning';
    return 'success';
  };

  const overall = getOverallStatus();

  return (
    <div style={{
      background: 'var(--color-brand-card)',
      borderRadius: 14,
      border: '1px solid var(--color-brand-border)',
      padding: '20px 24px',
      display: 'flex',
      flexDirection: 'column',
      gap: 16,
      boxShadow: '0 4px 20px rgba(0,0,0,0.15)',
      position: 'relative',
      overflow: 'hidden'
    }}>
      <style>{`
        @keyframes pulse-indigo {
          0%, 100% { opacity: 0.3; transform: scale(1); }
          50% { opacity: 0.8; transform: scale(1.15); }
        }
        @keyframes rotate-slow {
          0% { transform: rotate(0deg); }
          100% { transform: rotate(360deg); }
        }
        @keyframes slide-in-step {
          0% { opacity: 0; transform: translateY(12px); }
          100% { opacity: 1; transform: translateY(0); }
        }
        .animate-step {
          animation: slide-in-step 0.4s cubic-bezier(0.16, 1, 0.3, 1) forwards;
        }
        .animate-spin-slow {
          animation: rotate-slow 2s linear infinite;
        }
        .animate-pulse-glow {
          animation: pulse-indigo 1.5s ease-in-out infinite;
        }
      `}</style>

      <div style={{ display: 'flex', justifyContent: 'space-between', alignItems: 'center' }}>
        <div>
          <div style={{ fontSize: 11, textTransform: 'uppercase', letterSpacing: 1.5, color: 'var(--color-brand-muted)', fontWeight: 600 }}>
            🔍 Diagnostics Observatory
          </div>
          <div style={{ fontSize: 13, color: 'var(--color-brand-text)', marginTop: 2 }}>
            Run real-time multi-layered verification tests
          </div>
        </div>
        <button
          onClick={handleStart}
          disabled={loading || isAnimating}
          style={{
            padding: '8px 16px',
            borderRadius: 8,
            border: 'none',
            background: loading || isAnimating ? 'var(--color-brand-border)' : 'linear-gradient(135deg, var(--color-brand), #4f46e5)',
            color: '#fff',
            fontSize: 12,
            fontWeight: 700,
            cursor: loading || isAnimating ? 'not-allowed' : 'pointer',
            display: 'flex',
            alignItems: 'center',
            gap: 6,
            boxShadow: loading || isAnimating ? 'none' : '0 4px 12px rgba(99,102,241,0.2)',
            transition: 'all 0.2s ease',
          }}
        >
          {loading ? (
            <>
              <FiRefreshCw className="animate-spin-slow" size={14} />
              Running API...
            </>
          ) : isAnimating ? (
            <>
              <FiActivity className="animate-pulse-glow" size={14} />
              Evaluating...
            </>
          ) : (
            <>
              <FiPlayCircle size={14} />
              Run Health Checks
            </>
          )}
        </button>
      </div>

      {(loading || results) && (
        <div style={{ width: '100%', height: 4, background: 'var(--color-brand-bg)', borderRadius: 2, overflow: 'hidden', position: 'relative' }}>
          <div style={{
            height: '100%',
            background: overall === 'error' ? '#ef4444' : overall === 'warning' ? '#f59e0b' : '#10b981',
            width: loading ? '30%' : `${((results ? Math.min(revealIndex + 1, results.length) : 0) / (results?.length || 1)) * 100}%`,
            transition: loading ? 'width 2s ease-in-out infinite' : 'width 0.4s ease-out',
            borderRadius: 2
          }} />
        </div>
      )}

      {results && (
        <div style={{ display: 'flex', flexDirection: 'column', gap: 10 }}>
          {results.map((step, i) => {
            const isRevealed = i < revealIndex;
            const isCurrent = i === revealIndex;

            if (!isRevealed && !isCurrent) {
              return (
                <div key={step.name} style={{ display: 'flex', alignItems: 'center', gap: 12, padding: '10px 14px', borderRadius: 8, border: '1px solid transparent', opacity: 0.35 }}>
                  <div style={{ width: 20, height: 20, borderRadius: '50%', background: '#374151', display: 'flex', alignItems: 'center', justifyContent: 'center' }}>
                    <span style={{ width: 6, height: 6, borderRadius: '50%', background: '#9ca3af' }} />
                  </div>
                  <div>
                    <div style={{ fontSize: 13, fontWeight: 600, color: 'var(--color-brand-muted)' }}>{step.name}</div>
                    <div style={{ fontSize: 10, color: 'var(--color-brand-muted)' }}>Pending analysis...</div>
                  </div>
                </div>
              );
            }

            let stepColor = 'var(--color-brand-muted)';
            let statusIcon = <FiActivity className="animate-spin-slow" style={{ color: 'var(--color-brand)' }} size={16} />;
            let bg = 'rgba(255, 255, 255, 0.02)';
            let border = '1px solid var(--color-brand-border)';

            if (isRevealed) {
              if (step.status === 'success') {
                stepColor = '#10b981';
                statusIcon = <FiCheckCircle style={{ color: '#10b981' }} size={16} />;
                bg = 'rgba(16, 185, 129, 0.04)';
                border = '1px solid rgba(16, 185, 129, 0.15)';
              } else if (step.status === 'warning') {
                stepColor = '#f59e0b';
                statusIcon = <FiAlertTriangle style={{ color: '#f59e0b' }} size={16} />;
                bg = 'rgba(245, 158, 11, 0.04)';
                border = '1px solid rgba(245, 158, 11, 0.15)';
              } else if (step.status === 'error') {
                stepColor = '#ef4444';
                statusIcon = <FiXCircle style={{ color: '#ef4444' }} size={16} />;
                bg = 'rgba(239, 68, 68, 0.04)';
                border = '1px solid rgba(239, 68, 68, 0.15)';
              }
            } else if (isCurrent) {
              bg = 'rgba(99, 102, 241, 0.04)';
              border = '1px solid rgba(99, 102, 241, 0.25)';
            }

            return (
              <div
                key={step.name}
                className="animate-step"
                style={{
                  display: 'flex',
                  flexDirection: 'column',
                  gap: 6,
                  padding: '12px 16px',
                  borderRadius: 10,
                  background: bg,
                  border: border,
                  transition: 'all 0.3s ease',
                }}
              >
                <div style={{ display: 'flex', alignItems: 'center', justifyContent: 'space-between', gap: 12 }}>
                  <div style={{ display: 'flex', alignItems: 'center', gap: 10 }}>
                    {statusIcon}
                    <div>
                      <div style={{ fontSize: 13, fontWeight: 700, color: 'var(--color-brand-heading)' }}>{step.name}</div>
                      <div style={{ fontSize: 11, color: 'var(--color-brand-text)' }}>{step.description}</div>
                    </div>
                  </div>
                  {isRevealed && (
                    <span style={{
                      fontSize: 10,
                      fontWeight: 700,
                      textTransform: 'uppercase',
                      padding: '2px 8px',
                      borderRadius: 12,
                      background: step.status === 'success' ? 'rgba(16,185,129,0.1)' : step.status === 'warning' ? 'rgba(245,158,11,0.1)' : 'rgba(239,68,68,0.1)',
                      color: stepColor,
                    }}>
                      {step.status}
                    </span>
                  )}
                  {isCurrent && (
                    <span className="animate-pulse-glow" style={{
                      fontSize: 10,
                      fontWeight: 700,
                      textTransform: 'uppercase',
                      padding: '2px 8px',
                      borderRadius: 12,
                      background: 'rgba(99,102,241,0.1)',
                      color: 'var(--color-brand)',
                    }}>
                      Checking...
                    </span>
                  )}
                </div>

                {isRevealed && (step.details || step.error_message) && (
                  <div style={{
                    marginTop: 4,
                    padding: '8px 12px',
                    borderRadius: 6,
                    background: 'var(--color-brand-bg)',
                    fontSize: 11,
                    fontFamily: 'monospace',
                    color: step.status === 'error' ? '#ef4444' : 'var(--color-brand-muted)',
                    borderLeft: `3px solid ${stepColor}`,
                    whiteSpace: 'pre-wrap',
                    wordBreak: 'break-word',
                  }}>
                    {step.status === 'error' ? step.error_message : step.details}
                  </div>
                )}
              </div>
            );
          })}
        </div>
      )}

      {results && !isAnimating && revealIndex >= results.length && (
        <div style={{
          padding: '14px 18px',
          borderRadius: 10,
          background: overall === 'error' ? 'rgba(239,68,68,0.06)' : overall === 'warning' ? 'rgba(245,158,11,0.06)' : 'rgba(16,185,129,0.06)',
          border: `1.5px solid ${overall === 'error' ? '#ef4444' : overall === 'warning' ? '#f59e0b' : '#10b981'}`,
          display: 'flex',
          flexDirection: 'column',
          gap: 4,
          alignItems: 'center',
          textAlign: 'center',
          marginTop: 10,
          animation: 'slide-in-step 0.5s ease forwards'
        }}>
          <div style={{ display: 'flex', alignItems: 'center', gap: 6 }}>
            <span style={{ fontSize: 18 }}>
              {overall === 'error' ? '❌' : overall === 'warning' ? '⚠️' : '✅'}
            </span>
            <span style={{
              fontWeight: 800,
              fontSize: 14,
              letterSpacing: 0.5,
              color: overall === 'error' ? '#ef4444' : overall === 'warning' ? '#f59e0b' : '#10b981'
            }}>
              {overall === 'error'
                ? 'CRITICAL FAULT DETECTED'
                : overall === 'warning'
                  ? 'SUBOPTIMAL PERFORMANCE WARNING'
                  : 'ALL SYSTEMS FULLY FUNCTIONAL'}
            </span>
          </div>
          <div style={{ fontSize: 11, color: 'var(--color-brand-muted)' }}>
            Scan completed successfully. Diagnostics verified local configs, network boundaries, and tunnel aggregation layers.
          </div>
        </div>
      )}
    </div>
  );
};

/* ────────────────────────────────────────────────────────────────────────── */
/*  MAIN PAGE                                                               */
/* ────────────────────────────────────────────────────────────────────────── */
export const V2RayMultipathPage: React.FC = () => {
  const {
    status, config, loading, error,
    fetchConfig, fetchStatus, startEngine, stopEngine, connectTelemetry, saveConfig,
    diagnoseResults, diagnoseLoading, runDiagnostics,
  } = useMultipathStore();

  // Local config form states
  const [mode, setMode] = useState('selector');
  const [stripingMode, setStripingMode] = useState('auto');
  const [maxArteries, setMaxArteries] = useState(5);
  const [minArteries, setMinArteries] = useState(2);
  const [socksPort, setSocksPort] = useState(10646);
  const [httpPort, setHttpPort] = useState(10545);
  const [combinerUrl, setCombinerUrl] = useState('');
  const [originId, setOriginId] = useState('');
  const [pskHex, setPskHex] = useState('');
  const [frameSize, setFrameSize] = useState(4096);
  const [evalWindowMs, setEvalWindowMs] = useState(5000);
  const [demoteRttX, setDemoteRttX] = useState(1.5);
  const [promoteRttX, setPromoteRttX] = useState(1.2);
  const [lossDemotePct, setLossDemotePct] = useState(5.0);
  const [cooldownSec, setCooldownSec] = useState(30);
  const [errorBudget, setErrorBudget] = useState(5);

  const [showAdvanced, setShowAdvanced] = useState(false);

  // Bootstrap
  useEffect(() => {
    fetchConfig();
    fetchStatus();
  }, [fetchConfig, fetchStatus]);

  // Sync database config to local state on initial load
  useEffect(() => {
    if (config) {
      setMode(config.mode || 'selector');
      setStripingMode(config.striping_mode || 'auto');
      setMaxArteries(config.max_arteries || 5);
      setMinArteries(config.min_arteries || 2);
      setSocksPort(config.socks_port || 10646);
      setHttpPort(config.http_port || 10545);
      setCombinerUrl(config.combiner_url || '');
      setOriginId(config.origin_id || '');
      setPskHex(config.psk_hex || '');
      setFrameSize(config.frame_size || 4096);
      setEvalWindowMs(config.eval_window_ms || 5000);
      setDemoteRttX(config.demote_rtt_x || 1.5);
      setPromoteRttX(config.promote_rtt_x || 1.2);
      setLossDemotePct(config.loss_demote_pct || 5.0);
      setCooldownSec(config.cooldown_sec || 30);
      setErrorBudget(config.error_budget || 5);
    }
  }, [config]);

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
      // Auto-save form values before starting
      await saveConfig({
        is_active: true,
        mode,
        striping_mode: stripingMode,
        max_arteries: Number(maxArteries),
        min_arteries: Number(minArteries),
        socks_port: Number(socksPort),
        http_port: Number(httpPort),
        combiner_url: combinerUrl,
        origin_id: originId,
        psk_hex: pskHex,
        frame_size: Number(frameSize),
        eval_window_ms: Number(evalWindowMs),
        demote_rtt_x: Number(demoteRttX),
        promote_rtt_x: Number(promoteRttX),
        loss_demote_pct: Number(lossDemotePct),
        cooldown_sec: Number(cooldownSec),
        error_budget: Number(errorBudget),
      });
      await startEngine();
    }
    await fetchStatus();
  }, [
    isRunning, mode, stripingMode, maxArteries, minArteries, socksPort, httpPort,
    combinerUrl, originId, pskHex, frameSize, evalWindowMs, demoteRttX, promoteRttX,
    lossDemotePct, cooldownSec, errorBudget, saveConfig, startEngine, fetchStatus, stopEngine
  ]);

  const handleSave = async (e: React.FormEvent) => {
    e.preventDefault();
    await saveConfig({
      is_active: config?.is_active ?? false,
      mode,
      striping_mode: stripingMode,
      max_arteries: Number(maxArteries),
      min_arteries: Number(minArteries),
      socks_port: Number(socksPort),
      http_port: Number(httpPort),
      combiner_url: combinerUrl,
      origin_id: originId,
      psk_hex: pskHex,
      frame_size: Number(frameSize),
      eval_window_ms: Number(evalWindowMs),
      demote_rtt_x: Number(demoteRttX),
      promote_rtt_x: Number(promoteRttX),
      loss_demote_pct: Number(lossDemotePct),
      cooldown_sec: Number(cooldownSec),
      error_budget: Number(errorBudget),
    });
  };

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

        {/* Stats strip — row 1: engine info */}
        {isRunning && (
          <div style={{
            display: 'grid', gridTemplateColumns: 'repeat(4, 1fr)', gap: 16,
            marginTop: 24, padding: '16px 0', borderTop: '1px solid rgba(255,255,255,0.08)',
            position: 'relative', zIndex: 1,
          }}>
            <Stat label="Mode" value={config?.mode === 'bonding' ? 'Bonding' : 'Selector'} color="#ff6b2c" />
            <Stat label="Active Arteries" value={status?.active_count || 0} color="#10b981" />
            <Stat label="Pool Size" value={status?.total_pool || 0} color="#6366f1" />
            <Stat label="Active Conns" value={status?.active_conns || 0} color="#f59e0b" />
          </div>
        )}
        {/* Stats strip — row 2: live traffic */}
        {isRunning && (
          <div style={{
            display: 'grid', gridTemplateColumns: 'repeat(4, 1fr)', gap: 16,
            marginTop: 8, padding: '14px 0', borderTop: '1px solid rgba(255,255,255,0.06)',
            position: 'relative', zIndex: 1,
          }}>
            <div style={{ textAlign: 'center' }}>
              <div style={{ fontSize: 10, color: '#9b9bab', textTransform: 'uppercase', letterSpacing: 1, marginBottom: 4 }}>↑ Upload Speed</div>
              <div style={{ fontSize: 18, fontWeight: 700, color: '#10b981', fontVariantNumeric: 'tabular-nums' }}>
                {formatSpeed(status?.uplink_bps || 0)}
              </div>
              <div style={{ fontSize: 10, color: '#9b9bab', marginTop: 2 }}>Total: {formatBytes(status?.bytes_tx || 0)}</div>
            </div>
            <div style={{ textAlign: 'center' }}>
              <div style={{ fontSize: 10, color: '#9b9bab', textTransform: 'uppercase', letterSpacing: 1, marginBottom: 4 }}>↓ Download Speed</div>
              <div style={{ fontSize: 18, fontWeight: 700, color: '#6366f1', fontVariantNumeric: 'tabular-nums' }}>
                {formatSpeed(status?.downlink_bps || 0)}
              </div>
              <div style={{ fontSize: 10, color: '#9b9bab', marginTop: 2 }}>Total: {formatBytes(status?.bytes_rx || 0)}</div>
            </div>
            <div style={{ textAlign: 'center' }}>
              <div style={{ fontSize: 10, color: '#9b9bab', textTransform: 'uppercase', letterSpacing: 1, marginBottom: 4 }}>Total Uploaded</div>
              <div style={{ fontSize: 18, fontWeight: 700, color: '#f59e0b', fontVariantNumeric: 'tabular-nums' }}>
                {formatBytes(status?.bytes_tx || 0)}
              </div>
            </div>
            <div style={{ textAlign: 'center' }}>
              <div style={{ fontSize: 10, color: '#9b9bab', textTransform: 'uppercase', letterSpacing: 1, marginBottom: 4 }}>Total Downloaded</div>
              <div style={{ fontSize: 18, fontWeight: 700, color: '#a78bfa', fontVariantNumeric: 'tabular-nums' }}>
                {formatBytes(status?.bytes_rx || 0)}
              </div>
            </div>
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
          <div style={{ display: 'flex', flexDirection: 'column', gap: 20 }}>
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

            <DiagnosticsPanel
              results={diagnoseResults}
              loading={diagnoseLoading}
              onRun={() => runDiagnostics(config || undefined)}
            />
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

      {/* ── Offline View ─────────────────────────────────────────────── */}
      {!isRunning && (
        <div style={{ display: 'grid', gridTemplateColumns: '1.2fr 1fr', gap: 20, alignItems: 'start' }}>
          {/* Configuration Form Card */}
          <div style={{ display: 'flex', flexDirection: 'column', gap: 20 }}>
            <div style={{
              background: 'var(--color-brand-card)', borderRadius: 14,
              border: '1px solid var(--color-brand-border)', padding: '20px 24px',
              display: 'flex', flexDirection: 'column', gap: 16
            }}>
            <div style={{ fontSize: 11, textTransform: 'uppercase', letterSpacing: 1.5, color: 'var(--color-brand-muted)', fontWeight: 600 }}>
              ⚙️ Engine Settings
            </div>
            <form onSubmit={handleSave} style={{ display: 'flex', flexDirection: 'column', gap: 16 }}>
              {/* Mode Selection Cards */}
              <div>
                <label style={{ fontSize: 11, fontWeight: 600, color: 'var(--color-brand-muted)', textTransform: 'uppercase', display: 'block', marginBottom: 8 }}>
                  Engine Mode
                </label>
                <div style={{ display: 'grid', gridTemplateColumns: '1fr 1fr', gap: 12 }}>
                  <div
                    onClick={() => {
                      setMode('selector');
                      useMultipathStore.setState({ diagnoseResults: null });
                    }}
                    style={{
                      padding: '16px', borderRadius: 10, cursor: 'pointer',
                      background: mode === 'selector' ? 'rgba(16,185,129,0.08)' : 'var(--color-brand-bg)',
                      border: `1.5px solid ${mode === 'selector' ? '#10b981' : 'var(--color-brand-border)'}`,
                      transition: 'all 0.2s ease',
                      boxShadow: mode === 'selector' ? '0 4px 12px rgba(16,185,129,0.1)' : 'none',
                    }}
                  >
                    <div style={{ fontWeight: 700, fontSize: 13, color: 'var(--color-brand-heading)' }}>Mode A — Selector</div>
                    <div style={{ fontSize: 10, color: 'var(--color-brand-muted)', marginTop: 4 }}>Failover / Smart Selection</div>
                  </div>
                  <div
                    onClick={() => {
                      setMode('bonding');
                      useMultipathStore.setState({ diagnoseResults: null });
                    }}
                    style={{
                      padding: '16px', borderRadius: 10, cursor: 'pointer',
                      background: mode === 'bonding' ? 'rgba(99,102,241,0.08)' : 'var(--color-brand-bg)',
                      border: `1.5px solid ${mode === 'bonding' ? '#6366f1' : 'var(--color-brand-border)'}`,
                      transition: 'all 0.2s ease',
                      boxShadow: mode === 'bonding' ? '0 4px 12px rgba(99,102,241,0.1)' : 'none',
                    }}
                  >
                    <div style={{ fontWeight: 700, fontSize: 13, color: 'var(--color-brand-heading)' }}>Mode B — True Bonding</div>
                    <div style={{ fontSize: 10, color: 'var(--color-brand-muted)', marginTop: 4 }}>Multipath Tunnel Aggregation</div>
                  </div>
                </div>
              </div>

              {/* Mode B specific fields */}
              {mode === 'bonding' && (
                <div style={{ display: 'flex', flexDirection: 'column', gap: 12, padding: 14, borderRadius: 10, background: 'rgba(99,102,241,0.04)', border: '1px dashed rgba(99,102,241,0.2)' }}>
                  <div style={{ display: 'flex', flexDirection: 'column', gap: 4 }}>
                    <label style={{ fontSize: 11, fontWeight: 600, color: 'var(--color-brand-heading)' }}>Combiner URL *</label>
                    <input
                      type="text"
                      placeholder="e.g. ws://your-server-ip:10545/api/bonding/combine"
                      value={combinerUrl}
                      onChange={(e) => setCombinerUrl(e.target.value)}
                      required={mode === 'bonding'}
                      style={{ width: '100%', padding: '8px 12px', borderRadius: 8, border: '1px solid var(--color-brand-border)', background: 'var(--color-brand-bg)', fontSize: 13, color: 'var(--color-brand-heading)' }}
                    />
                  </div>
                  <div style={{ display: 'grid', gridTemplateColumns: '1fr 1fr', gap: 12 }}>
                    <div style={{ display: 'flex', flexDirection: 'column', gap: 4 }}>
                      <label style={{ fontSize: 11, fontWeight: 600, color: 'var(--color-brand-heading)' }}>Pre-Shared Key (Hex)</label>
                      <input
                        type="text"
                        placeholder="Optional hex token"
                        value={pskHex}
                        onChange={(e) => setPskHex(e.target.value)}
                        style={{ width: '100%', padding: '8px 12px', borderRadius: 8, border: '1px solid var(--color-brand-border)', background: 'var(--color-brand-bg)', fontSize: 13, color: 'var(--color-brand-heading)' }}
                      />
                    </div>
                    <div style={{ display: 'flex', flexDirection: 'column', gap: 4 }}>
                      <label style={{ fontSize: 11, fontWeight: 600, color: 'var(--color-brand-heading)' }}>Client Origin ID</label>
                      <input
                        type="text"
                        placeholder="Optional client ID"
                        value={originId}
                        onChange={(e) => setOriginId(e.target.value)}
                        style={{ width: '100%', padding: '8px 12px', borderRadius: 8, border: '1px solid var(--color-brand-border)', background: 'var(--color-brand-bg)', fontSize: 13, color: 'var(--color-brand-heading)' }}
                      />
                    </div>
                  </div>
                </div>
              )}

              {/* General connection fields */}
              <div style={{ display: 'grid', gridTemplateColumns: 'repeat(3, 1fr)', gap: 12 }}>
                <div style={{ display: 'flex', flexDirection: 'column', gap: 4 }}>
                  <label style={{ fontSize: 11, fontWeight: 600, color: 'var(--color-brand-heading)' }}>Max Arteries</label>
                  <input
                    type="number"
                    min={1}
                    value={maxArteries}
                    onChange={(e) => setMaxArteries(Number(e.target.value))}
                    style={{ width: '100%', padding: '8px 12px', borderRadius: 8, border: '1px solid var(--color-brand-border)', background: 'var(--color-brand-bg)', fontSize: 13, color: 'var(--color-brand-heading)' }}
                  />
                </div>
                <div style={{ display: 'flex', flexDirection: 'column', gap: 4 }}>
                  <label style={{ fontSize: 11, fontWeight: 600, color: 'var(--color-brand-heading)' }}>SOCKS Port</label>
                  <input
                    type="number"
                    value={socksPort}
                    onChange={(e) => setSocksPort(Number(e.target.value))}
                    style={{ width: '100%', padding: '8px 12px', borderRadius: 8, border: '1px solid var(--color-brand-border)', background: 'var(--color-brand-bg)', fontSize: 13, color: 'var(--color-brand-heading)' }}
                  />
                </div>
                <div style={{ display: 'flex', flexDirection: 'column', gap: 4 }}>
                  <label style={{ fontSize: 11, fontWeight: 600, color: 'var(--color-brand-heading)' }}>HTTP Port</label>
                  <input
                    type="number"
                    value={httpPort}
                    onChange={(e) => setHttpPort(Number(e.target.value))}
                    style={{ width: '100%', padding: '8px 12px', borderRadius: 8, border: '1px solid var(--color-brand-border)', background: 'var(--color-brand-bg)', fontSize: 13, color: 'var(--color-brand-heading)' }}
                  />
                </div>
              </div>

              {/* Advanced toggle */}
              <div
                onClick={() => setShowAdvanced(!showAdvanced)}
                style={{ cursor: 'pointer', fontSize: 12, color: 'var(--color-brand)', fontWeight: 600, userSelect: 'none', display: 'flex', alignItems: 'center', gap: 4 }}
              >
                {showAdvanced ? '▼ Hide Advanced Parameters' : '▶ Show Advanced Parameters'}
              </div>

              {showAdvanced && (
                <div style={{ display: 'grid', gridTemplateColumns: '1fr 1fr', gap: 12, padding: 12, borderRadius: 8, background: 'var(--color-brand-bg)', border: '1px solid var(--color-brand-border)' }}>
                  <div style={{ display: 'flex', flexDirection: 'column', gap: 4 }}>
                    <label style={{ fontSize: 11, fontWeight: 600, color: 'var(--color-brand-heading)' }}>Min Arteries</label>
                    <input
                      type="number"
                      min={1}
                      value={minArteries}
                      onChange={(e) => setMinArteries(Number(e.target.value))}
                      style={{ padding: '6px 10px', borderRadius: 6, border: '1px solid var(--color-brand-border)', background: 'var(--color-brand-card)', fontSize: 12, color: 'var(--color-brand-heading)' }}
                    />
                  </div>
                  <div style={{ display: 'flex', flexDirection: 'column', gap: 4 }}>
                    <label style={{ fontSize: 11, fontWeight: 600, color: 'var(--color-brand-heading)' }}>Frame Size (Bytes)</label>
                    <input
                      type="number"
                      value={frameSize}
                      onChange={(e) => setFrameSize(Number(e.target.value))}
                      style={{ padding: '6px 10px', borderRadius: 6, border: '1px solid var(--color-brand-border)', background: 'var(--color-brand-card)', fontSize: 12, color: 'var(--color-brand-heading)' }}
                    />
                  </div>
                  <div style={{ display: 'flex', flexDirection: 'column', gap: 4 }}>
                    <label style={{ fontSize: 11, fontWeight: 600, color: 'var(--color-brand-heading)' }}>Eval Window (ms)</label>
                    <input
                      type="number"
                      value={evalWindowMs}
                      onChange={(e) => setEvalWindowMs(Number(e.target.value))}
                      style={{ padding: '6px 10px', borderRadius: 6, border: '1px solid var(--color-brand-border)', background: 'var(--color-brand-card)', fontSize: 12, color: 'var(--color-brand-heading)' }}
                    />
                  </div>
                  <div style={{ display: 'flex', flexDirection: 'column', gap: 4 }}>
                    <label style={{ fontSize: 11, fontWeight: 600, color: 'var(--color-brand-heading)' }}>Loss Demote %</label>
                    <input
                      type="number"
                      value={lossDemotePct}
                      onChange={(e) => setLossDemotePct(Number(e.target.value))}
                      style={{ padding: '6px 10px', borderRadius: 6, border: '1px solid var(--color-brand-border)', background: 'var(--color-brand-card)', fontSize: 12, color: 'var(--color-brand-heading)' }}
                    />
                  </div>
                  <div style={{ display: 'flex', flexDirection: 'column', gap: 4 }}>
                    <label style={{ fontSize: 11, fontWeight: 600, color: 'var(--color-brand-heading)' }}>Demote RTT Factor</label>
                    <input
                      type="number"
                      step="0.1"
                      value={demoteRttX}
                      onChange={(e) => setDemoteRttX(Number(e.target.value))}
                      style={{ padding: '6px 10px', borderRadius: 6, border: '1px solid var(--color-brand-border)', background: 'var(--color-brand-card)', fontSize: 12, color: 'var(--color-brand-heading)' }}
                    />
                  </div>
                  <div style={{ display: 'flex', flexDirection: 'column', gap: 4 }}>
                    <label style={{ fontSize: 11, fontWeight: 600, color: 'var(--color-brand-heading)' }}>Promote RTT Factor</label>
                    <input
                      type="number"
                      step="0.1"
                      value={promoteRttX}
                      onChange={(e) => setPromoteRttX(Number(e.target.value))}
                      style={{ padding: '6px 10px', borderRadius: 6, border: '1px solid var(--color-brand-border)', background: 'var(--color-brand-card)', fontSize: 12, color: 'var(--color-brand-heading)' }}
                    />
                  </div>
                  <div style={{ display: 'flex', flexDirection: 'column', gap: 4 }}>
                    <label style={{ fontSize: 11, fontWeight: 600, color: 'var(--color-brand-heading)' }}>Cooldown (Sec)</label>
                    <input
                      type="number"
                      value={cooldownSec}
                      onChange={(e) => setCooldownSec(Number(e.target.value))}
                      style={{ padding: '6px 10px', borderRadius: 6, border: '1px solid var(--color-brand-border)', background: 'var(--color-brand-card)', fontSize: 12, color: 'var(--color-brand-heading)' }}
                    />
                  </div>
                  <div style={{ display: 'flex', flexDirection: 'column', gap: 4 }}>
                    <label style={{ fontSize: 11, fontWeight: 600, color: 'var(--color-brand-heading)' }}>Error Budget</label>
                    <input
                      type="number"
                      value={errorBudget}
                      onChange={(e) => setErrorBudget(Number(e.target.value))}
                      style={{ padding: '6px 10px', borderRadius: 6, border: '1px solid var(--color-brand-border)', background: 'var(--color-brand-card)', fontSize: 12, color: 'var(--color-brand-heading)' }}
                    />
                  </div>
                  <div style={{ display: 'flex', flexDirection: 'column', gap: 4, gridColumn: 'span 2' }}>
                    <label style={{ fontSize: 11, fontWeight: 600, color: 'var(--color-brand-heading)' }}>Striping Mode</label>
                    <select
                      value={stripingMode}
                      onChange={(e) => setStripingMode(e.target.value)}
                      style={{ padding: '6px 10px', borderRadius: 6, border: '1px solid var(--color-brand-border)', background: 'var(--color-brand-card)', fontSize: 12, color: 'var(--color-brand-heading)', width: '100%' }}
                    >
                      <option value="auto">Auto (Best Effort)</option>
                      <option value="roundrobin">Round-Robin</option>
                      <option value="redundant">Redundant (Duplicate Packets)</option>
                    </select>
                  </div>
                </div>
              )}

              <button
                type="submit"
                className="btn btn--secondary"
                style={{ width: '100%', padding: '10px', fontWeight: 600, fontSize: 13 }}
              >
                Save Settings Configuration
              </button>
            </form>
          </div>

          <DiagnosticsPanel
            results={diagnoseResults}
            loading={diagnoseLoading}
            onRun={() => runDiagnostics({
              mode,
              striping_mode: stripingMode,
              max_arteries: maxArteries,
              min_arteries: minArteries,
              socks_port: socksPort,
              http_port: httpPort,
              combiner_url: combinerUrl,
              origin_id: originId,
              psk_hex: pskHex,
              frame_size: frameSize,
              eval_window_ms: evalWindowMs,
              demote_rtt_x: demoteRttX,
              promote_rtt_x: promoteRttX,
              loss_demote_pct: lossDemotePct,
              cooldown_sec: cooldownSec,
              error_budget: errorBudget,
              is_active: config?.is_active ?? false,
            })}
          />
        </div>

          {/* Right Column: Explanatory Details */}
          <div style={{ display: 'flex', flexDirection: 'column', gap: 20 }}>
            {/* Architecture Card */}
            <div style={{
              background: 'var(--color-brand-card)', borderRadius: 14,
              border: '1px solid var(--color-brand-border)', padding: '20px 24px',
            }}>
              <div style={{ fontSize: 11, textTransform: 'uppercase', letterSpacing: 1.5, color: 'var(--color-brand-muted)', marginBottom: 16, fontWeight: 600 }}>
                🏗 Architecture &amp; Exit IP Explained
              </div>
              <div style={{ display: 'flex', flexDirection: 'column', gap: 16 }}>
                {/* Mode A explain */}
                <div style={{
                  padding: '14px', borderRadius: 8,
                  background: mode === 'selector' ? 'rgba(16,185,129,0.08)' : 'rgba(255,255,255,0.02)',
                  border: `1px solid ${mode === 'selector' ? 'rgba(16,185,129,0.3)' : 'var(--color-brand-border)'}`,
                }}>
                  <div style={{ display: 'flex', alignItems: 'center', gap: 8, marginBottom: 8 }}>
                    <span style={{ fontSize: 16 }}>⚡</span>
                    <div>
                      <div style={{ fontWeight: 700, fontSize: 13, color: 'var(--color-brand-heading)' }}>Mode A — Selector/Failover</div>
                      <div style={{ fontSize: 9, color: '#10b981', fontWeight: 600 }}>{mode === 'selector' ? '● SELECTED' : '○ Inactive'}</div>
                    </div>
                  </div>
                  <div style={{ fontSize: 12, color: 'var(--color-brand-text)', lineHeight: 1.6 }}>
                    Traffic route: <strong>You → Best V2Ray Node → Internet</strong><br />
                    <span style={{ color: '#f59e0b' }}>⚠ Exit IP = V2Ray node IP</span> (not your Clever Cloud server).<br />
                    Best for simple failover, lowest latency selection, and zero-server changes.
                  </div>
                </div>

                {/* Mode B explain */}
                <div style={{
                  padding: '14px', borderRadius: 8,
                  background: mode === 'bonding' ? 'rgba(99,102,241,0.08)' : 'rgba(255,255,255,0.02)',
                  border: `1px solid ${mode === 'bonding' ? 'rgba(99,102,241,0.3)' : 'var(--color-brand-border)'}`,
                }}>
                  <div style={{ display: 'flex', alignItems: 'center', gap: 8, marginBottom: 8 }}>
                    <span style={{ fontSize: 16 }}>🔗</span>
                    <div>
                      <div style={{ fontWeight: 700, fontSize: 13, color: 'var(--color-brand-heading)' }}>Mode B — True Bonding</div>
                      <div style={{ fontSize: 9, color: '#6366f1', fontWeight: 600 }}>{mode === 'bonding' ? '● SELECTED' : '○ Inactive'}</div>
                    </div>
                  </div>
                  <div style={{ fontSize: 12, color: 'var(--color-brand-text)', lineHeight: 1.6 }}>
                    Traffic route: <strong>You → V2Ray Nodes → Clever Cloud Server → Internet</strong><br />
                    <span style={{ color: '#10b981' }}>✓ Exit IP = Clever Cloud server IP</span>.<br />
                    Requires configuring the Combiner URL and running a server combiner.
                  </div>
                </div>
              </div>
            </div>

            {/* Offline state details */}
            <div style={{
              background: 'var(--color-brand-card)', borderRadius: 14,
              border: '1px solid var(--color-brand-border)', padding: '20px 24px',
              textAlign: 'center'
            }}>
              <div style={{ fontSize: 32, marginBottom: 12 }}>🔌</div>
              <h3 style={{ fontSize: 15, fontWeight: 700, color: 'var(--color-brand-heading)', margin: '0 0 6px' }}>
                Engine Offline
              </h3>
              <p style={{ fontSize: 12, color: 'var(--color-brand-muted)', margin: 0, lineHeight: 1.5 }}>
                Configure settings and click <strong>Start Engine</strong> in the header to activate the tunnel. Requires at least 2 healthy nodes in the scanner pool to build arteries.
              </p>
            </div>
          </div>
        </div>
      )}
    </div>
  );
};

