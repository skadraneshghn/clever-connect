import React, { useEffect, useCallback, useState } from 'react';
import { useCombinerStore, type ArteryStats } from '../store/combinerStore';
import { FiCpu, FiPlay, FiStopCircle, FiCheck, FiCopy, FiRefreshCw, FiAlertCircle, FiCheckCircle, FiAlertTriangle, FiXCircle, FiActivity, FiPlayCircle } from 'react-icons/fi';

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

const Stat: React.FC<{ label: string; value: string | number; unit?: string; color?: string }> = ({ label, value, unit, color }) => (
  <div style={{ textAlign: 'center' }}>
    <div style={{ fontSize: 10, color: '#9b9bab', textTransform: 'uppercase', letterSpacing: 1, marginBottom: 2 }}>{label}</div>
    <div style={{ fontSize: 20, fontWeight: 700, color: color || '#fff', fontVariantNumeric: 'tabular-nums' }}>
      {value}<span style={{ fontSize: 11, fontWeight: 400, color: '#9b9bab', marginLeft: 2 }}>{unit}</span>
    </div>
  </div>
);

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
          <div style={{ fontSize: 11, textTransform: 'uppercase', letterSpacing: 1.5, color: '#9b9bab', fontWeight: 600 }}>
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

            let stepColor = '#9b9bab';
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
                    color: step.status === 'error' ? '#ef4444' : '#9b9bab',
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
          <div style={{ fontSize: 11, color: '#9b9bab' }}>
            Scan completed successfully. Diagnostics verified local configs, network boundaries, and tunnel egress layers.
          </div>
        </div>
      )}
    </div>
  );
};

export const BondingCombinerPage: React.FC = () => {
  const {
    status, config, loading, error,
    fetchConfig, fetchStatus, startCombiner, stopCombiner, saveConfig,
    diagnoseResults, diagnoseLoading, runDiagnostics,
  } = useCombinerStore();

  const [originId, setOriginId] = useState('default');
  const [pskHex, setPskHex] = useState('');
  const [copiedText, setCopiedText] = useState<string | null>(null);

  // Bootstrap configuration
  useEffect(() => {
    fetchConfig();
    fetchStatus();
  }, [fetchConfig, fetchStatus]);

  // Sync config backend state to local state
  useEffect(() => {
    if (config) {
      setOriginId(config.origin_id || 'default');
      setPskHex(config.psk_hex || '');
    }
  }, [config]);

  // Telemetry status polling when active
  useEffect(() => {
    let interval: any = null;
    if (status?.running) {
      interval = setInterval(() => {
        fetchStatus();
      }, 3000);
    } else {
      fetchStatus();
    }
    return () => {
      if (interval) clearInterval(interval);
    };
  }, [status?.running, fetchStatus]);

  const isRunning = status?.running || false;

  const generatePSK = () => {
    const chars = '0123456789abcdef';
    let psk = '';
    for (let i = 0; i < 64; i++) {
      psk += chars[Math.floor(Math.random() * chars.length)];
    }
    setPskHex(psk);
  };

  const handleSave = async (e: React.FormEvent) => {
    e.preventDefault();
    await saveConfig({
      is_active: config?.is_active ?? false,
      mode: 'combiner',
      origin_id: originId,
      psk_hex: pskHex,
    });
  };

  const handleToggle = useCallback(async () => {
    if (isRunning) {
      await stopCombiner();
    } else {
      // Auto-save form values before starting
      await saveConfig({
        is_active: true,
        mode: 'combiner',
        origin_id: originId,
        psk_hex: pskHex,
      });
      await startCombiner();
    }
  }, [isRunning, originId, pskHex, startCombiner, stopCombiner, saveConfig]);

  const copyToClipboard = (text: string, label: string) => {
    navigator.clipboard.writeText(text);
    setCopiedText(label);
    setTimeout(() => setCopiedText(null), 2000);
  };

  // Derive client configuration values
  const serverHost = window.location.hostname;
  const clientCombinerUrl = `ws://${serverHost}/ws/bonding/combiner`;

  return (
    <div style={{ display: 'flex', flexDirection: 'column', gap: 20, padding: 4 }}>
      {/* Hero Header */}
      <div style={{
        background: 'linear-gradient(135deg, #1f1c2c 0%, #302b63 50%, #24243e 100%)',
        borderRadius: 16, padding: '28px 32px', color: '#fff', position: 'relative', overflow: 'hidden',
      }}>
        {/* Mesh Background */}
        <div style={{
          position: 'absolute', inset: 0, opacity: 0.08,
          backgroundImage: 'radial-gradient(circle at 10% 30%, #6366f1 0%, transparent 40%), radial-gradient(circle at 90% 70%, #ff6b2c 0%, transparent 40%)',
        }} />

        <div style={{ display: 'flex', justifyContent: 'space-between', alignItems: 'center', position: 'relative', zIndex: 1 }}>
          <div>
            <div style={{ fontSize: 10, textTransform: 'uppercase', letterSpacing: 2, color: '#9b9bab', marginBottom: 6 }}>
              Multipath Aggregation Server
            </div>
            <h1 style={{ fontSize: 24, fontWeight: 800, margin: 0, letterSpacing: -0.5 }}>
              🔗 Multipath Combiner
            </h1>
            <p style={{ fontSize: 12, color: '#9b9bab', marginTop: 6, maxWidth: 500 }}>
              The server-side reassembler terminates the WebSocket tunnels and re-sequences split client packets for secure delivery to the internet.
            </p>
          </div>

          {/* Master Toggle */}
          <button
            onClick={handleToggle}
            disabled={loading}
            style={{
              padding: '12px 28px', borderRadius: 12, border: 'none', cursor: loading ? 'not-allowed' : 'pointer',
              fontSize: 13, fontWeight: 700, letterSpacing: 0.5, transition: 'all 0.3s ease',
              background: isRunning
                ? 'linear-gradient(135deg, #ef4444, #dc2626)'
                : 'linear-gradient(135deg, #6366f1, #4f46e5)',
              color: '#fff',
              boxShadow: isRunning ? '0 4px 20px rgba(239,68,68,0.3)' : '0 4px 20px rgba(99,102,241,0.3)',
              opacity: loading ? 0.6 : 1,
            }}
          >
            {loading ? '...' : isRunning ? '■  Stop Combiner' : '▶  Start Combiner'}
          </button>
        </div>

        {/* Stats strip — row 1: engine info */}
        {isRunning && (
          <div style={{
            display: 'grid', gridTemplateColumns: 'repeat(4, 1fr)', gap: 16,
            marginTop: 24, padding: '16px 0', borderTop: '1px solid rgba(255,255,255,0.08)',
            position: 'relative', zIndex: 1,
          }}>
            <Stat label="Status" value="RUNNING" color="#10b981" />
            <Stat label="Client Origin" value={status?.origin_id || 'default'} color="#ff6b2c" />
            <Stat label="Connected Arteries" value={status?.artery_count || 0} color="#6366f1" />
            <Stat label="Active Streams" value={status?.active_streams || 0} color="#f59e0b" />
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
              <div style={{ fontSize: 10, color: '#9b9bab', textTransform: 'uppercase', letterSpacing: 1, marginBottom: 4 }}>↑ Upload Speed (Tx)</div>
              <div style={{ fontSize: 18, fontWeight: 700, color: '#10b981', fontVariantNumeric: 'tabular-nums' }}>
                {formatSpeed(status?.tx_bps || 0)}
              </div>
              <div style={{ fontSize: 10, color: '#9b9bab', marginTop: 2 }}>Total: {formatBytes(status?.bytes_tx || 0)}</div>
            </div>
            <div style={{ textAlign: 'center' }}>
              <div style={{ fontSize: 10, color: '#9b9bab', textTransform: 'uppercase', letterSpacing: 1, marginBottom: 4 }}>↓ Download Speed (Rx)</div>
              <div style={{ fontSize: 18, fontWeight: 700, color: '#6366f1', fontVariantNumeric: 'tabular-nums' }}>
                {formatSpeed(status?.rx_bps || 0)}
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

      {/* Grid Layout */}
      <div style={{ display: 'grid', gridTemplateColumns: isRunning && (status?.artery_stats?.length ?? 0) > 0 ? '1fr 1fr' : '1.2fr 1fr', gap: 20, alignItems: 'start' }}>
        
        {/* Combiner Settings */}
        <div style={{ display: 'flex', flexDirection: 'column', gap: 20 }}>
          <div style={{
            background: 'var(--color-brand-card)', borderRadius: 14,
            border: '1px solid var(--color-brand-border)', padding: '20px 24px',
            display: 'flex', flexDirection: 'column', gap: 16
          }}>
          <div style={{ fontSize: 11, textTransform: 'uppercase', letterSpacing: 1.5, color: 'var(--color-brand-muted)', fontWeight: 600 }}>
            ⚙️ Combiner Settings
          </div>
          <form onSubmit={handleSave} style={{ display: 'flex', flexDirection: 'column', gap: 16 }}>
            <div style={{ display: 'flex', flexDirection: 'column', gap: 4 }}>
              <label style={{ fontSize: 11, fontWeight: 600, color: 'var(--color-brand-heading)' }}>Client Origin ID</label>
              <input
                type="text"
                placeholder="default"
                value={originId}
                onChange={(e) => {
                  setOriginId(e.target.value);
                  useCombinerStore.setState({ diagnoseResults: null });
                }}
                required
                disabled={isRunning}
                style={{ width: '100%', padding: '8px 12px', borderRadius: 8, border: '1px solid var(--color-brand-border)', background: 'var(--color-brand-bg)', fontSize: 13, color: 'var(--color-brand-heading)' }}
              />
              <span style={{ fontSize: 10, color: 'var(--color-brand-muted)' }}>
                Unique identifier of the client matching this connection.
              </span>
            </div>

            <div style={{ display: 'flex', flexDirection: 'column', gap: 4 }}>
              <label style={{ fontSize: 11, fontWeight: 600, color: 'var(--color-brand-heading)', display: 'flex', justifyContent: 'space-between' }}>
                <span>Pre-Shared Key (Hex)</span>
                {!isRunning && (
                  <button
                    type="button"
                    onClick={() => {
                      generatePSK();
                      useCombinerStore.setState({ diagnoseResults: null });
                    }}
                    style={{ background: 'none', border: 'none', color: 'var(--color-brand)', cursor: 'pointer', fontSize: 11, fontWeight: 600, display: 'flex', alignItems: 'center', gap: 4 }}
                  >
                    <FiRefreshCw size={10} /> Generate Secure Key
                  </button>
                )}
              </label>
              <input
                type="text"
                placeholder="Optional 32-byte hexadecimal key (64 characters)"
                value={pskHex}
                onChange={(e) => {
                  setPskHex(e.target.value);
                  useCombinerStore.setState({ diagnoseResults: null });
                }}
                disabled={isRunning}
                style={{ width: '100%', padding: '8px 12px', borderRadius: 8, border: '1px solid var(--color-brand-border)', background: 'var(--color-brand-bg)', fontSize: 13, color: 'var(--color-brand-heading)', fontFamily: 'monospace' }}
              />
              <span style={{ fontSize: 10, color: 'var(--color-brand-muted)' }}>
                Ensures only clients holding this key can bind tunnels to your combiner. Leave blank to disable authentication.
              </span>
            </div>

            {!isRunning && (
              <button
                type="submit"
                className="btn btn--secondary"
                style={{ width: '100%', padding: '10px', fontWeight: 600, fontSize: 13 }}
              >
                Save Config Settings
              </button>
            )}
          </form>
        </div>

        <DiagnosticsPanel
          results={diagnoseResults}
          loading={diagnoseLoading}
          onRun={() => runDiagnostics({
            is_active: config?.is_active ?? false,
            mode: 'combiner',
            origin_id: originId,
            psk_hex: pskHex,
          })}
        />
      </div>

        {/* Copy Connection Parameters */}
        <div style={{
          background: 'var(--color-brand-card)', borderRadius: 14,
          border: '1px solid var(--color-brand-border)', padding: '20px 24px',
          display: 'flex', flexDirection: 'column', gap: 16
        }}>
          <div style={{ fontSize: 11, textTransform: 'uppercase', letterSpacing: 1.5, color: 'var(--color-brand-muted)', fontWeight: 600 }}>
            📋 Client Configuration Values
          </div>
          <div style={{ fontSize: 12, color: 'var(--color-brand-text)', lineHeight: 1.6 }}>
            Copy and paste these exact values into the <strong>Multipath Engine settings</strong> on your local Client Panel:
          </div>

          <div style={{ display: 'flex', flexDirection: 'column', gap: 12 }}>
            {/* Combiner URL Box */}
            <div style={{ background: 'var(--color-brand-bg)', border: '1px solid var(--color-brand-border)', borderRadius: 8, padding: 12 }}>
              <div style={{ display: 'flex', justifyContent: 'space-between', alignItems: 'center', marginBottom: 4 }}>
                <span style={{ fontSize: 10, fontWeight: 700, color: 'var(--color-brand)' }}>COMBINER URL</span>
                <button
                  onClick={() => copyToClipboard(clientCombinerUrl, 'url')}
                  style={{ background: 'none', border: 'none', cursor: 'pointer', color: 'var(--color-brand-text)', display: 'flex', alignItems: 'center', gap: 4, fontSize: 10 }}
                >
                  {copiedText === 'url' ? <FiCheck color="#10b981" /> : <FiCopy />} Copy
                </button>
              </div>
              <div style={{ fontSize: 12, fontFamily: 'monospace', color: 'var(--color-brand-heading)', wordBreak: 'break-all' }}>
                {clientCombinerUrl}
              </div>
            </div>

            {/* Client Origin ID Box */}
            <div style={{ background: 'var(--color-brand-bg)', border: '1px solid var(--color-brand-border)', borderRadius: 8, padding: 12 }}>
              <div style={{ display: 'flex', justifyContent: 'space-between', alignItems: 'center', marginBottom: 4 }}>
                <span style={{ fontSize: 10, fontWeight: 700, color: 'var(--color-brand)' }}>CLIENT ORIGIN ID</span>
                <button
                  onClick={() => copyToClipboard(originId, 'origin')}
                  style={{ background: 'none', border: 'none', cursor: 'pointer', color: 'var(--color-brand-text)', display: 'flex', alignItems: 'center', gap: 4, fontSize: 10 }}
                >
                  {copiedText === 'origin' ? <FiCheck color="#10b981" /> : <FiCopy />} Copy
                </button>
              </div>
              <div style={{ fontSize: 12, fontFamily: 'monospace', color: 'var(--color-brand-heading)' }}>
                {originId}
              </div>
            </div>

            {/* PSK Hex Box */}
            <div style={{ background: 'var(--color-brand-bg)', border: '1px solid var(--color-brand-border)', borderRadius: 8, padding: 12 }}>
              <div style={{ display: 'flex', justifyContent: 'space-between', alignItems: 'center', marginBottom: 4 }}>
                <span style={{ fontSize: 10, fontWeight: 700, color: 'var(--color-brand)' }}>PRE-SHARED KEY (HEX)</span>
                <button
                  onClick={() => copyToClipboard(pskHex, 'psk')}
                  style={{ background: 'none', border: 'none', cursor: 'pointer', color: 'var(--color-brand-text)', display: 'flex', alignItems: 'center', gap: 4, fontSize: 10 }}
                >
                  {copiedText === 'psk' ? <FiCheck color="#10b981" /> : <FiCopy />} Copy
                </button>
              </div>
              <div style={{ fontSize: 12, fontFamily: 'monospace', color: 'var(--color-brand-heading)', wordBreak: 'break-all', minHeight: 18 }}>
                {pskHex || '—'}
              </div>
            </div>
          </div>
        </div>
      </div>

      {/* Connected Client Tunnels (Arteries) */}
      {isRunning && (
        <div style={{
          background: 'var(--color-brand-card)', borderRadius: 14,
          border: '1px solid var(--color-brand-border)', overflow: 'hidden',
        }}>
          <div style={{ padding: '16px 24px', borderBottom: '1px solid var(--color-brand-border)', display: 'flex', justifyContent: 'space-between', alignItems: 'center' }}>
            <div>
              <div style={{ fontSize: 11, textTransform: 'uppercase', letterSpacing: 1.5, color: 'var(--color-brand-muted)', fontWeight: 600 }}>
                Connected Client Arteries
              </div>
              <div style={{ fontSize: 13, color: 'var(--color-brand-text)', marginTop: 2 }}>
                Currently active WebSocket tunnels forwarding split packets from the client.
              </div>
            </div>
            <div style={{
              padding: '4px 12px', borderRadius: 20, fontSize: 11, fontWeight: 600,
              background: 'rgba(99,102,241,0.1)', color: '#6366f1',
            }}>
              {status?.artery_count || 0} Tunnels Active
            </div>
          </div>

          {(status?.artery_stats?.length ?? 0) === 0 ? (
            <div style={{ padding: '40px', textAlign: 'center', color: 'var(--color-brand-muted)' }}>
              <FiAlertCircle size={28} style={{ marginBottom: 8 }} />
              <div>No client connections active yet.</div>
              <div style={{ fontSize: 11, marginTop: 4 }}> Tunnels will appear here once the local Multipath Engine starts.</div>
            </div>
          ) : (
            <div style={{ overflowX: 'auto' }}>
              <table style={{ width: '100%', borderCollapse: 'collapse' }}>
                <thead>
                  <tr style={{ borderBottom: '1px solid var(--color-brand-border)', background: 'var(--color-brand-bg)' }}>
                    {['Artery ID', 'Last Ping Received', 'Last Pong Transmitted', 'Status'].map(h => (
                      <th key={h} style={{
                        padding: '10px 16px', fontSize: 10, fontWeight: 600, textTransform: 'uppercase',
                        letterSpacing: 1, color: 'var(--color-brand-muted)', textAlign: 'left',
                      }}>{h}</th>
                    ))}
                  </tr>
                </thead>
                <tbody>
                  {status?.artery_stats?.map((a: ArteryStats) => (
                    <tr key={a.artery_id} style={{ borderBottom: '1px solid var(--color-brand-border)' }}>
                      <td style={{ padding: '12px 16px', fontWeight: 600, fontSize: 13, color: 'var(--color-brand-heading)' }}>
                        {a.artery_id}
                      </td>
                      <td style={{ padding: '12px 16px', fontSize: 12, fontFamily: 'monospace' }}>
                        {new Date(a.last_ping).toLocaleTimeString()}
                      </td>
                      <td style={{ padding: '12px 16px', fontSize: 12, fontFamily: 'monospace' }}>
                        {new Date(a.last_pong).toLocaleTimeString()}
                      </td>
                      <td style={{ padding: '12px 16px' }}>
                        <span style={{
                          display: 'inline-flex', alignItems: 'center', gap: 4,
                          padding: '2px 10px', borderRadius: 20, fontSize: 10, fontWeight: 600,
                          background: 'rgba(16,185,129,0.1)', color: '#10b981',
                        }}>
                          CONNECTED
                        </span>
                      </td>
                    </tr>
                  ))}
                </tbody>
              </table>
            </div>
          )}
        </div>
      )}
    </div>
  );
};
