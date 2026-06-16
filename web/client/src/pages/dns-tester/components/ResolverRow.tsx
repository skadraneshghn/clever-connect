import React from 'react';
import { 
  FiAlertTriangle, FiCheckCircle, FiPlay, 
  FiActivity, FiDatabase, FiTrash2 
} from 'react-icons/fi';
import { useDNSStore } from '../../../store/dnsStore';
import { IPResolveBadge } from '../../../components/atoms/IPResolveBadge';
import { 
  ProtocolBadge, CensorshipBadge, 
  DNSSECBadge, CleverScoreBadge 
} from './Badges';

interface ResolverRowProps { 
  resolverKey: string; 
  style: React.CSSProperties;
  onDeleteSingle: (key: string) => void;
  onApplyResolver: (key: string) => void;
  isActiveSystem: boolean;
  onOpenTrace: (key: string) => void;
  onOpenAXFR: (key: string) => void;
  onOpenAdvancedTest: (key: string) => void;
  isSelected: boolean;
  onToggleSelect: (key: string) => void;
}

export const ResolverRow = React.memo(({ 
  resolverKey, 
  style, 
  onDeleteSingle,
  onApplyResolver,
  isActiveSystem,
  onOpenTrace,
  onOpenAXFR,
  onOpenAdvancedTest,
  isSelected,
  onToggleSelect,
}: ResolverRowProps) => {
  const resolver = useDNSStore(state => state.resolvers[resolverKey]);

  if (!resolver) return null;

  const isTesting = resolver.is_testing;
  
  const rowStyle: React.CSSProperties = {
    ...style,
    borderBottom: '1px solid var(--color-brand-border)',
    background: isActiveSystem ? 'rgba(255, 107, 44, 0.04)' : 'none',
    transition: 'background-color 0.2s ease',
  };

  const getLatencyColor = (ms: number) => {
    if (ms <= 0) return 'var(--color-brand-muted)';
    if (ms < 50) return 'var(--color-brand-green)';
    if (ms < 150) return '#f59e0b';
    return 'var(--color-brand-red)';
  };

  return (
    <tr
      className={isTesting ? 'pulse-testing' : ''}
      style={rowStyle}
    >
      <td style={{ padding: '10px 12px', textAlign: 'center', width: 40 }}>
        <input 
          type="checkbox" 
          checked={isSelected}
          onChange={() => onToggleSelect(resolverKey)}
          style={{ accentColor: 'var(--color-brand)', cursor: 'pointer' }}
        />
      </td>
      <td style={{ padding: '10px 12px', fontWeight: 600, color: 'var(--color-brand-heading)' }}>
        <IPResolveBadge ip={resolver.ip} />
      </td>
      <td style={{ padding: '10px 12px', color: 'var(--color-brand-heading)', fontWeight: 500 }}>
        <div>{resolver.provider_name}</div>
        {resolver.query_type && (
          <div style={{ fontSize: 9, color: 'var(--color-brand-muted)', marginTop: 2, display: 'flex', gap: 4, alignItems: 'center' }}>
            <span style={{ background: 'var(--color-brand-bg)', padding: '1px 4px', borderRadius: 3, fontWeight: 700 }}>{resolver.query_type}</span>
            <span style={{ background: 'var(--color-brand-bg)', padding: '1px 4px', borderRadius: 3, fontWeight: 700 }}>{resolver.dns_class || 'IN'}</span>
            <span style={{ overflow: 'hidden', textOverflow: 'ellipsis', whiteSpace: 'nowrap', maxWidth: 100 }} title={resolver.domain}>{resolver.domain}</span>
          </div>
        )}
        {resolver.resolved_ip && (
          <div style={{ fontSize: 9, color: 'var(--color-brand-muted)', marginTop: 4, display: 'flex', gap: 6, flexWrap: 'wrap', alignItems: 'center' }}>
            <span style={{ display: 'inline-flex', alignItems: 'center', gap: 4 }}>
              Resolved: <IPResolveBadge ip={resolver.resolved_ip} style={{ fontSize: 9, padding: '1px 4px', height: 16 }} />
            </span>
            {resolver.expected_match !== undefined && (
              <span style={{ 
                background: resolver.expected_match ? 'rgba(34, 197, 94, 0.1)' : 'rgba(239, 68, 68, 0.1)', 
                color: resolver.expected_match ? '#16a34a' : '#dc2626', 
                padding: '1px 4px', 
                borderRadius: 3, 
                fontWeight: 700 
              }}>
                {resolver.expected_match ? "Expect Match" : "Expect Mismatch"}
              </span>
            )}
          </div>
        )}
        {resolver.error_message && (
          <div style={{ marginTop: 4 }}>
            <span style={{ 
              background: 'rgba(239, 68, 68, 0.08)', 
              color: 'var(--color-brand-red)', 
              padding: '2px 6px', 
              borderRadius: 4, 
              fontWeight: 600,
              fontSize: 10,
              display: 'inline-flex',
              alignItems: 'center',
              gap: 4
            }} title={resolver.error_message}>
              <FiAlertTriangle size={10} /> Error: {resolver.error_message}
            </span>
          </div>
        )}
      </td>
      <td style={{ padding: '10px 12px' }}>
        <ProtocolBadge protocol={resolver.protocol} />
      </td>
      <td style={{ padding: '10px 12px', textAlign: 'center', fontWeight: 700, color: getLatencyColor(resolver.latency_ms) }}>
        {resolver.latency_ms > 0 ? `${resolver.latency_ms.toFixed(1)}ms` : '-'}
      </td>
      <td style={{ padding: '10px 12px', textAlign: 'center', color: 'var(--color-brand-text)' }}>
        {resolver.jitter_ms > 0 ? `${resolver.jitter_ms.toFixed(1)}ms` : '-'}
      </td>
      <td style={{ padding: '10px 12px', textAlign: 'center', color: resolver.success_rate > 0 ? 'var(--color-brand-green)' : 'var(--color-brand-text)' }}>
        {resolver.success_rate > 0 ? `${(resolver.success_rate * 100).toFixed(0)}%` : '-'}
      </td>
      <td style={{ padding: '10px 12px', textAlign: 'center' }}>
        {resolver.latency_ms > 0 ? (
          <CensorshipBadge censored={resolver.censored} hijacked={resolver.nxdomain_hijacked} />
        ) : (
          '-'
        )}
      </td>
      <td style={{ padding: '10px 12px', textAlign: 'center' }}>
        {resolver.latency_ms > 0 ? (
          <DNSSECBadge valid={resolver.dnssec_valid} />
        ) : (
          '-'
        )}
      </td>
      <td style={{ padding: '10px 12px', textAlign: 'center' }}>
        {resolver.latency_ms > 0 ? (
          resolver.dns_rebinding_vuln ? (
            <span style={{ padding: '2px 6px', borderRadius: 4, background: 'rgba(239, 68, 68, 0.08)', color: 'var(--color-brand-red)', fontSize: 10, fontWeight: 700 }}>Vulnerable</span>
          ) : (
            <span style={{ padding: '2px 6px', borderRadius: 4, background: '#eefbf3', color: '#15803d', fontSize: 10, fontWeight: 700 }}>Secure</span>
          )
        ) : (
          '-'
        )}
      </td>
      <td style={{ padding: '10px 12px', textAlign: 'center' }}>
        <CleverScoreBadge score={resolver.clever_score} />
      </td>
      <td style={{ padding: '10px 12px', textAlign: 'center' }}>
        <div style={{ display: 'flex', alignItems: 'center', justifyContent: 'center', gap: 6 }}>
          {isActiveSystem ? (
            <span style={{ fontSize: 11, fontWeight: 700, color: 'var(--color-brand)', display: 'inline-flex', alignItems: 'center', gap: 4 }}>
              <FiCheckCircle /> Active
            </span>
          ) : (
            <button
              onClick={() => onApplyResolver(resolverKey)}
              disabled={resolver.latency_ms <= 0}
              className="btn btn-primary"
              style={{ 
                padding: '4px 8px', 
                fontSize: 10, 
                fontWeight: 700, 
                borderRadius: 6,
                background: resolver.latency_ms > 0 ? 'var(--color-brand)' : 'var(--color-brand-muted)',
                cursor: resolver.latency_ms > 0 ? 'pointer' : 'not-allowed'
              }}
              title="Apply as system DNS resolver"
            >
              Apply
            </button>
          )}

          <button
            onClick={() => onOpenAdvancedTest(resolverKey)}
            style={{ 
              padding: '4px 8px', 
              fontSize: 10, 
              fontWeight: 700, 
              borderRadius: 6,
              cursor: 'pointer',
              display: 'flex',
              alignItems: 'center',
              gap: 2,
              background: 'rgba(59, 130, 246, 0.08)',
              border: '1px solid rgba(59, 130, 246, 0.2)',
              color: 'var(--color-brand)'
            }}
            title="Advanced Single DNS Test"
          >
            <FiPlay size={10} /> Test
          </button>

          <button
            onClick={() => onOpenTrace(resolverKey)}
            disabled={resolver.latency_ms <= 0}
            style={{ 
              padding: '4px 8px', 
              fontSize: 10, 
              fontWeight: 700, 
              borderRadius: 6,
              cursor: resolver.latency_ms > 0 ? 'pointer' : 'not-allowed',
              display: 'flex',
              alignItems: 'center',
              gap: 2,
              background: 'none',
              border: '1px solid var(--color-brand-border)',
              color: 'var(--color-brand-text)'
            }}
            title="Trace DNS Delegation Path"
          >
            <FiActivity size={10} /> Trace
          </button>

          <button
            onClick={() => onOpenAXFR(resolverKey)}
            disabled={resolver.latency_ms <= 0}
            style={{ 
              padding: '4px 8px', 
              fontSize: 10, 
              fontWeight: 700, 
              borderRadius: 6,
              cursor: resolver.latency_ms > 0 ? 'pointer' : 'not-allowed',
              display: 'flex',
              alignItems: 'center',
              gap: 2,
              background: 'none',
              border: '1px solid var(--color-brand-border)',
              color: 'var(--color-brand-text)'
            }}
            title="AXFR Zone Transfer Audit"
          >
            <FiDatabase size={10} /> AXFR
          </button>

          <button
            onClick={() => onDeleteSingle(resolverKey)}
            style={{ background: 'none', border: 'none', cursor: 'pointer', color: 'var(--color-brand-red)', display: 'inline-flex', alignSelf: 'center' }}
            title="Delete Resolver"
          >
            <FiTrash2 size={12} />
          </button>
        </div>
      </td>
    </tr>
  );
});

ResolverRow.displayName = 'ResolverRow';
