import React from 'react';

export const ServerSidePanel: React.FC = () => {
  return (
    <div
      style={{
        padding: 12,
        border: '1px solid var(--color-brand-border)',
        borderRadius: 8,
        background: 'var(--color-brand-bg)',
      }}
    >
      <h4 style={{ margin: 0, color: 'var(--color-brand-heading)' }}>Server Side Management</h4>
      <p style={{ margin: '4px 0 0', fontSize: 11, color: 'var(--color-brand-text)' }}>
        Orchestration controls active.
      </p>
    </div>
  );
};

export default ServerSidePanel;
