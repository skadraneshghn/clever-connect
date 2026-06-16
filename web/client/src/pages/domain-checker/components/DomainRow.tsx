import React from 'react';
import { FiPlay, FiTrash2 } from 'react-icons/fi';
import { useDomainStore } from '../../../store/domainStore';

interface DomainRowProps {
  domainId: string;
  onCheckSingle: (id: string) => void;
  onDeleteSingle: (id: string) => void;
  isChecking: boolean;
  isSelected: boolean;
  onToggleSelect: (id: string) => void;
}

export const DomainRow = React.memo(({
  domainId,
  onCheckSingle,
  onDeleteSingle,
  isChecking,
  isSelected,
  onToggleSelect,
}: DomainRowProps) => {
  const domain = useDomainStore(state => state.domains[domainId]);

  if (!domain) return null;

  return (
    <tr
      className={domain.status === 'checking' ? 'pulse-testing' : ''}
      style={{
        borderBottom: '1px solid var(--color-brand-border)',
        verticalAlign: 'middle',
        background: domain.status === 'checking' ? 'rgba(255, 107, 44, 0.04)' : 'none',
        transition: 'background-color 0.2s ease',
      }}
    >
      <td style={{ padding: '10px 12px', textAlign: 'center', width: 40 }}>
        <input
          type="checkbox"
          checked={isSelected}
          onChange={() => onToggleSelect(domainId)}
          style={{ accentColor: 'var(--color-brand)', cursor: 'pointer' }}
        />
      </td>
      <td style={{ padding: '10px 12px', fontWeight: 600, color: 'var(--color-brand-heading)', wordBreak: 'break-all' }}>
        {domain.domain_name}
      </td>
      <td style={{ padding: '10px 12px', textAlign: 'center' }}>
        <span style={{
          padding: '2px 6px',
          borderRadius: 4,
          background:
            domain.status === 'online' ? '#eefbf3' :
            domain.status === 'checking' ? 'rgba(59, 130, 246, 0.08)' :
            domain.status === 'pending' ? 'var(--color-brand-bg)' :
            'rgba(239, 68, 68, 0.08)',
          color:
            domain.status === 'online' ? '#15803d' :
            domain.status === 'checking' ? 'var(--color-brand-blue)' :
            domain.status === 'pending' ? 'var(--color-brand-text)' :
            'var(--color-brand-red)',
          fontSize: 10,
          fontWeight: 700,
          textTransform: 'uppercase'
        }}>
          {domain.status}
        </span>
      </td>
      <td style={{ padding: '10px 12px', color: 'var(--color-brand-text)', fontSize: 11, fontFamily: 'monospace' }}>
        {domain.ip_addresses || '-'}
      </td>
      <td style={{ padding: '10px 12px', textAlign: 'center', fontWeight: 700, color: domain.latency_ms > 0 ? (domain.latency_ms < 150 ? 'var(--color-brand-green)' : '#f59e0b') : 'var(--color-brand-muted)' }}>
        {domain.latency_ms > 0 ? `${domain.latency_ms}ms` : '-'}
      </td>
      <td style={{ padding: '10px 12px', textAlign: 'center' }}>
        {domain.status !== 'pending' && domain.status !== 'checking' && (
          <span style={{
            padding: '2px 6px',
            borderRadius: 4,
            background: domain.tls_status ? '#eefbf3' : 'rgba(239, 68, 68, 0.08)',
            color: domain.tls_status ? '#15803d' : 'var(--color-brand-red)',
            fontSize: 10,
            fontWeight: 700,
          }}>
            {domain.tls_status ? (domain.tls_expiry_days > 0 ? `${domain.tls_expiry_days}d` : 'Valid') : 'Invalid'}
          </span>
        )}
      </td>
      <td style={{ padding: '10px 12px', textAlign: 'center', fontFamily: 'monospace', color: domain.http_status === 200 ? 'var(--color-brand-green)' : 'var(--color-brand-text)' }}>
        {domain.http_status || '-'}
      </td>
      <td style={{ padding: '10px 12px', textAlign: 'center' }}>
        <div style={{ display: 'flex', alignItems: 'center', justifyContent: 'center', gap: 10 }}>
          <button
            onClick={() => onCheckSingle(domainId)}
            disabled={isChecking}
            style={{ background: 'none', border: 'none', cursor: 'pointer', color: 'var(--color-brand)' }}
            title="Check Domain"
          >
            <FiPlay size={12} />
          </button>
          <button
            onClick={() => onDeleteSingle(domainId)}
            style={{ background: 'none', border: 'none', cursor: 'pointer', color: 'var(--color-brand-red)' }}
            title="Delete Domain"
          >
            <FiTrash2 size={12} />
          </button>
        </div>
      </td>
    </tr>
  );
});

DomainRow.displayName = 'DomainRow';
