import React from 'react';
import { FiCpu, FiPlay, FiSquare } from 'react-icons/fi';

interface ActiveCoreControlCardProps {
  isRunning: boolean;
  isLoading: boolean;
  handleToggleCore: (action: 'start' | 'stop') => void;
}

export const ActiveCoreControlCard: React.FC<ActiveCoreControlCardProps> = ({
  isRunning,
  isLoading,
  handleToggleCore,
}) => {
  return (
    <div
      className="g-card"
      style={{
        display: 'flex',
        flexDirection: 'column',
        alignItems: 'center',
        textAlign: 'center',
        padding: '24px 16px',
      }}
    >
      <div
        style={{
          width: 54,
          height: 54,
          borderRadius: '50%',
          background: isRunning ? 'var(--color-brand-light)' : 'var(--color-brand-bg)',
          display: 'flex',
          alignItems: 'center',
          justifyContent: 'center',
          color: isRunning ? 'var(--color-brand)' : 'var(--color-brand-muted)',
          marginBottom: 14,
          border: '1px solid var(--color-brand-border)',
        }}
      >
        <FiCpu size={24} />
      </div>

      <div style={{ fontSize: 12, fontWeight: 700, color: 'var(--color-brand-muted)', textTransform: 'uppercase', letterSpacing: 1 }}>
        Server core engine
      </div>

      <div style={{ display: 'flex', alignItems: 'center', gap: 6, marginTop: 8, marginBottom: 14 }}>
        <span className="live-dot" style={{ background: isRunning ? '#10b981' : '#ef4444' }} />
        <span style={{ fontSize: 16, fontWeight: 700, color: 'var(--color-brand-heading)' }}>
          {isRunning ? 'ACTIVE' : 'INACTIVE'}
        </span>
      </div>

      <div style={{ display: 'flex', gap: 10, width: '100%' }}>
        <button
          onClick={() => handleToggleCore('start')}
          className="btn btn--primary"
          style={{ flex: 1, display: 'flex', alignItems: 'center', justifyContent: 'center', gap: 6 }}
          disabled={isRunning || isLoading}
        >
          <FiPlay /> Start
        </button>
        <button
          onClick={() => handleToggleCore('stop')}
          className="btn btn--secondary"
          style={{
            flex: 1,
            display: 'flex',
            alignItems: 'center',
            justifyContent: 'center',
            gap: 6,
            borderColor: '#ef4444',
            color: '#ef4444',
          }}
          disabled={!isRunning || isLoading}
        >
          <FiSquare /> Stop
        </button>
      </div>
    </div>
  );
};

export default ActiveCoreControlCard;
