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

export const ClusterSkeleton: React.FC = () => (
  <CardSkeleton height={140} title="Cluster Orchestration" />
);

export const EdgeNodesSkeleton: React.FC = () => (
  <CardSkeleton height={350} title="Edge Nodes" />
);

export const InboundsSkeleton: React.FC = () => (
  <CardSkeleton height={380} title="Inbounds" />
);

export const UsersSkeleton: React.FC = () => (
  <CardSkeleton height={300} title="User Quotas" />
);
