import React from 'react';

interface StatsDashboardProps {
  dbStats: {
    total: number;
    online: number;
    offline: number;
    checking: number;
    ssl_valid: number;
  };
  isChecking: boolean;
  isCheckingAll: boolean;
  checkedAllCount: number;
  totalAllToCheck: number;
}

export const StatsDashboard: React.FC<StatsDashboardProps> = ({
  dbStats,
  isChecking,
  isCheckingAll,
  checkedAllCount,
  totalAllToCheck,
}) => {
  return (
    <div className="g-card" style={{ padding: 20 }}>
      <div style={{ display: 'flex', flexDirection: 'row', alignItems: 'center', gap: 24, flexWrap: 'wrap' }}>
        <div style={{ display: 'flex', width: '100%', flexWrap: 'wrap', alignItems: 'center', justifyContent: 'space-around', gap: 20 }}>
          
          {/* Sonar Radar Graphic */}
          <div style={{ position: 'relative', width: 100, height: 100, borderRadius: '50%', border: '1px solid rgba(255, 107, 44, 0.25)', background: 'radial-gradient(circle, rgba(255, 107, 44, 0.05) 0%, rgba(0,0,0,0) 70%)', overflow: 'hidden', flexShrink: 0 }}>
            <div style={{ position: 'absolute', inset: 0, borderRadius: '50%', border: '1px solid rgba(255, 107, 44, 0.15)', transform: 'scale(0.66)' }} />
            <div style={{ position: 'absolute', inset: 0, borderRadius: '50%', border: '1px solid rgba(255, 107, 44, 0.15)', transform: 'scale(0.33)' }} />
            <div style={{ position: 'absolute', width: '100%', height: '1px', background: 'rgba(255, 107, 44, 0.12)', top: '50%', left: 0 }} />
            <div style={{ position: 'absolute', height: '100%', width: '1px', background: 'rgba(255, 107, 44, 0.12)', left: '50%', top: 0 }} />

            {/* Blinking center spot */}
            <div style={{ position: 'absolute', width: 6, height: 6, borderRadius: '50%', background: 'var(--color-brand)', boxShadow: '0 0 10px var(--color-brand)', left: '50%', top: '50%', transform: 'translate(-50%, -50%)', zIndex: 5 }} />

            {/* Sweep ray */}
            <div
              className={`clip-radar ${isChecking ? 'animate-radar-sweep' : 'opacity-20'}`}
              style={{
                position: 'absolute',
                width: '50%',
                height: '50%',
                top: 0,
                left: '50%',
                transformOrigin: 'bottom left',
                background: 'linear-gradient(to right, rgba(255, 107, 44, 0.4) 0%, rgba(255, 107, 44, 0) 100%)',
                clipPath: 'polygon(0 100%, 100% 100%, 100% 0)'
              }}
            />
          </div>

          {/* Metrics */}
          <div style={{ flex: 1, minWidth: 160 }}>
            <div style={{ fontSize: 10, fontWeight: 700, color: 'var(--color-brand-muted)', textTransform: 'uppercase', letterSpacing: '1px', marginBottom: 10 }}>
              {isChecking ? 'DOMAIN PROBING IN PROGRESS...' : 'TELEMETRY SCAN IDLE'}
            </div>
            <div style={{ display: 'grid', gridTemplateColumns: 'repeat(5, minmax(0, 1fr))', gap: 16 }}>
              <div>
                <span style={{ display: 'block', fontSize: 10, color: 'var(--color-brand-text)', fontWeight: 600, textTransform: 'uppercase' }}>Total</span>
                <strong style={{ fontSize: 22, fontWeight: 700, color: 'var(--color-brand-heading)' }}>
                  {dbStats.total}
                </strong>
              </div>
              <div>
                <span style={{ display: 'block', fontSize: 10, color: 'var(--color-brand-green)', fontWeight: 600, textTransform: 'uppercase' }}>Online</span>
                <strong style={{ fontSize: 22, fontWeight: 700, color: 'var(--color-brand-green)' }}>
                  {dbStats.online}
                </strong>
              </div>
              <div>
                <span style={{ display: 'block', fontSize: 10, color: 'var(--color-brand-red)', fontWeight: 600, textTransform: 'uppercase' }}>Offline</span>
                <strong style={{ fontSize: 22, fontWeight: 700, color: 'var(--color-brand-red)' }}>
                  {dbStats.offline}
                </strong>
              </div>
              <div>
                <span style={{ display: 'block', fontSize: 10, color: 'var(--color-brand-blue)', fontWeight: 600, textTransform: 'uppercase' }}>Checking</span>
                <strong style={{ fontSize: 22, fontWeight: 700, color: 'var(--color-brand-blue)' }}>
                  {dbStats.checking}
                </strong>
              </div>
              <div>
                <span style={{ display: 'block', fontSize: 10, color: 'var(--color-brand-indigo)', fontWeight: 600, textTransform: 'uppercase' }}>SSL Valid</span>
                <strong style={{ fontSize: 22, fontWeight: 700, color: 'var(--color-brand-indigo)' }}>
                  {dbStats.ssl_valid}
                </strong>
              </div>
            </div>
          </div>
        </div>

        {isCheckingAll && (
          <div style={{ width: '100%', marginTop: 16, borderTop: '1px dashed var(--color-brand-border)', paddingTop: 16 }}>
            <div style={{ display: 'flex', justifyContent: 'space-between', alignItems: 'center', fontSize: 11, color: 'var(--color-brand-text)', marginBottom: 6 }}>
              <span style={{ fontWeight: 600, display: 'flex', alignItems: 'center', gap: 6 }}>
                <span className="shimmer-text" style={{ color: 'var(--color-brand)' }}>●</span> Bulk Probing: {checkedAllCount} / {totalAllToCheck} Domains Checked
              </span>
              <span style={{ fontWeight: 700, color: 'var(--color-brand)' }}>{Math.round((checkedAllCount / (totalAllToCheck || 1)) * 100)}%</span>
            </div>
            <div style={{ width: '100%', height: 6, background: 'var(--color-brand-bg)', borderRadius: 3, overflow: 'hidden' }}>
              <div style={{
                width: `${Math.min(100, Math.round((checkedAllCount / (totalAllToCheck || 1)) * 100))}%`,
                height: '100%',
                background: 'linear-gradient(to right, var(--color-brand-light), var(--color-brand))',
                borderRadius: 3,
                transition: 'width 0.3s ease-out'
              }} />
            </div>
          </div>
        )}

      </div>
    </div>
  );
};
