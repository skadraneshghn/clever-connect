import React from 'react';

export const CardSkeleton: React.FC<{ height?: number; title?: string }> = ({ height = 150, title }) => (
  <div
    className="g-card animate-pulse"
    style={{
      height,
      display: 'flex',
      flexDirection: 'column',
      gap: 12,
      background: 'var(--color-brand-bg)',
      border: '1px solid var(--color-brand-border)',
      borderRadius: 12,
      padding: 16,
    }}
  >
    <div style={{ display: 'flex', alignItems: 'center', gap: 8 }}>
      <div style={{ width: 18, height: 18, borderRadius: '50%', background: 'rgba(0,0,0,0.06)' }} />
      <div style={{ width: 120, height: 14, borderRadius: 4, background: 'rgba(0,0,0,0.06)' }}>
        {title && <span style={{ opacity: 0 }}>{title}</span>}
      </div>
    </div>
    <div style={{ flex: 1, display: 'flex', flexDirection: 'column', gap: 8, justifyContent: 'center' }}>
      <div style={{ width: '100%', height: 10, borderRadius: 4, background: 'rgba(0,0,0,0.06)' }} />
      <div style={{ width: '90%', height: 10, borderRadius: 4, background: 'rgba(0,0,0,0.06)' }} />
      <div style={{ width: '60%', height: 10, borderRadius: 4, background: 'rgba(0,0,0,0.06)' }} />
    </div>
  </div>
);

export const LogsTerminalSkeleton: React.FC = () => (
  <CardSkeleton height={320} title="Live Core Logs" />
);

export const SubscriptionsSkeleton: React.FC = () => (
  <CardSkeleton height={450} title="Subscriptions & Profiles" />
);

export const ConfigSettingsSkeleton: React.FC = () => (
  <CardSkeleton height={500} title="Proxy Configurations" />
);
