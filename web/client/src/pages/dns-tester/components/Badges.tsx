import React from 'react';
import { FiCheck } from 'react-icons/fi';

// Protocol Badges
export const ProtocolBadge = ({ protocol }: { protocol: string }) => {
  const styles: Record<string, { bg: string; color: string; label: string }> = {
    udp: { bg: 'rgba(59, 130, 246, 0.08)', color: 'var(--color-brand-blue)', label: 'UDP' },
    tcp: { bg: 'rgba(99, 102, 241, 0.08)', color: 'var(--color-brand-indigo)', label: 'TCP' },
    dot: { bg: 'rgba(139, 92, 246, 0.08)', color: '#8b5cf6', label: 'DoT' },
    doh: { bg: 'rgba(236, 72, 153, 0.08)', color: '#ec4899', label: 'DoH' },
    doq: { bg: 'rgba(6, 182, 212, 0.08)', color: '#06b6d4', label: 'DoQ' },
  };

  const meta = styles[protocol.toLowerCase()] || { bg: 'var(--color-brand-bg)', color: 'var(--color-brand-text)', label: protocol.toUpperCase() };

  return (
    <span style={{ 
      padding: '2px 8px', 
      borderRadius: 6, 
      background: meta.bg, 
      color: meta.color, 
      fontSize: 10, 
      fontWeight: 700,
      letterSpacing: '0.3px',
      textTransform: 'uppercase'
    }}>
      {meta.label}
    </span>
  );
};

// Censorship diagnostics badge
export const CensorshipBadge = ({ censored, hijacked }: { censored: boolean; hijacked: boolean }) => {
  if (censored) {
    return <span style={{ padding: '2px 6px', borderRadius: 4, background: 'rgba(239, 68, 68, 0.08)', color: 'var(--color-brand-red)', fontSize: 10, fontWeight: 700 }}>Censored</span>;
  }
  if (hijacked) {
    return <span style={{ padding: '2px 6px', borderRadius: 4, background: '#fffbeb', color: '#b45309', fontSize: 10, fontWeight: 700 }}>Hijacked</span>;
  }
  return <span style={{ padding: '2px 6px', borderRadius: 4, background: '#eefbf3', color: '#15803d', fontSize: 10, fontWeight: 700 }}>Clean</span>;
};

// DNSSEC verification badge
export const DNSSECBadge = ({ valid }: { valid: boolean }) => {
  if (valid) {
    return <span style={{ padding: '2px 6px', borderRadius: 4, background: '#eefbf3', color: '#15803d', fontSize: 10, fontWeight: 700, display: 'inline-flex', alignItems: 'center', gap: 2 }}><FiCheck size={10} /> Yes</span>;
  }
  return <span style={{ padding: '2px 6px', borderRadius: 4, background: 'var(--color-brand-bg)', color: 'var(--color-brand-text)', fontSize: 10, fontWeight: 500 }}>No</span>;
};

// Clever Score Visualizer
export const CleverScoreBadge = ({ score }: { score: number }) => {
  let color = 'var(--color-brand-red)';
  let bg = 'rgba(239, 68, 68, 0.08)';
  if (score >= 80) {
    color = '#15803d';
    bg = '#eefbf3';
  } else if (score >= 50) {
    color = '#b45309';
    bg = '#fffbeb';
  }

  return (
    <span style={{ 
      padding: '4px 8px', 
      borderRadius: 6, 
      background: bg, 
      color: color, 
      fontSize: 11, 
      fontWeight: 800,
      fontFamily: 'monospace'
    }}>
      {score > 0 ? score.toFixed(1) : '-'}
    </span>
  );
};
