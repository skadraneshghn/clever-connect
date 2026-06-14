import React, { useEffect, useState, useRef, useMemo, useCallback } from 'react';
import { useVirtualizer } from '@tanstack/react-virtual';
import { 
  FiSearch, FiPlay, FiCheckCircle, FiXCircle, 
  FiClock, FiGlobe, FiPlus, FiTrash2, FiSettings, 
  FiRefreshCw, FiServer, FiActivity, FiTrendingUp, 
  FiDatabase, FiStopCircle, FiCheck, FiInfo, FiAlertTriangle,
  FiChevronDown, FiChevronUp, FiFilter, FiX, FiCopy, FiDownload
} from 'react-icons/fi';
import { useDNSStore, getResolverKey } from '../store/dnsStore';
import type { DNSResolver, DNSJobStats } from '../store/dnsStore';
import { useAuthStore } from '../store/authStore';
import { IPResolveBadge } from '../components/atoms/IPResolveBadge';

// Protocol Badges
const ProtocolBadge = ({ protocol }: { protocol: string }) => {
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
const CensorshipBadge = ({ censored, hijacked }: { censored: boolean; hijacked: boolean }) => {
  if (censored) {
    return <span style={{ padding: '2px 6px', borderRadius: 4, background: 'rgba(239, 68, 68, 0.08)', color: 'var(--color-brand-red)', fontSize: 10, fontWeight: 700 }}>Censored</span>;
  }
  if (hijacked) {
    return <span style={{ padding: '2px 6px', borderRadius: 4, background: '#fffbeb', color: '#b45309', fontSize: 10, fontWeight: 700 }}>Hijacked</span>;
  }
  return <span style={{ padding: '2px 6px', borderRadius: 4, background: '#eefbf3', color: '#15803d', fontSize: 10, fontWeight: 700 }}>Clean</span>;
};

// DNSSEC verification badge
const DNSSECBadge = ({ valid }: { valid: boolean }) => {
  if (valid) {
    return <span style={{ padding: '2px 6px', borderRadius: 4, background: '#eefbf3', color: '#15803d', fontSize: 10, fontWeight: 700, display: 'inline-flex', alignItems: 'center', gap: 2 }}><FiCheck size={10} /> Yes</span>;
  }
  return <span style={{ padding: '2px 6px', borderRadius: 4, background: 'var(--color-brand-bg)', color: 'var(--color-brand-text)', fontSize: 10, fontWeight: 500 }}>No</span>;
};

// Clever Score Visualizer
const CleverScoreBadge = ({ score }: { score: number }) => {
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

// Table Row Component
const ResolverRow = React.memo(({ 
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
}: { 
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
}) => {
  const resolver = useDNSStore(state => state.resolvers[resolverKey]);

  if (!resolver) return null;

  const isTesting = resolver.is_testing;
  
  let rowStyle: React.CSSProperties = {
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
});;

export const DNSTesterPage: React.FC = () => {
  const { token } = useAuthStore();
  const { 
    resolvers, 
    resolverKeys, 
    jobStats, 
    isSweeping, 
    appliedResolver,
    setResolvers,
    updateResolver,
    setJobStats,
    setSweeping,
    setAppliedResolver,
    clearResults,
    bulkProgress,
    setBulkProgress
  } = useDNSStore();

  const [ws, setWs] = useState<WebSocket | null>(null);
  const [sweepStartTime, setSweepStartTime] = useState<number | null>(null);
  const [elapsedMs, setElapsedMs] = useState<number>(0);

  // Filter & UI variables
  const [search, setSearch] = useState('');
  const [categoryFilter, setCategoryFilter] = useState('ALL');
  const [protocolFilter, setProtocolFilter] = useState('ALL');
  const [sortBy, setSortBy] = useState('clever_score');
  const [sortOrder, setSortOrder] = useState<'asc' | 'desc'>('desc');
  const [showSettings, setShowSettings] = useState(false);
  const [showAddModal, setShowAddModal] = useState(false);

  // Advanced Filters states
  const [showAdvancedFilters, setShowAdvancedFilters] = useState(false);
  const [filterMinLatency, setFilterMinLatency] = useState('');
  const [filterMaxLatency, setFilterMaxLatency] = useState('');
  const [filterMinSuccessRate, setFilterMinSuccessRate] = useState('');
  const [filterCensorship, setFilterCensorship] = useState('ALL');
  const [filterDNSSEC, setFilterDNSSEC] = useState('ALL');
  const [filterRebinding, setFilterRebinding] = useState('ALL');
  const [filterISP, setFilterISP] = useState('');
  const [filterCountry, setFilterCountry] = useState('');
  const [filterCDN, setFilterCDN] = useState('ALL');

  const resetAdvancedFilters = () => {
    setFilterMinLatency('');
    setFilterMaxLatency('');
    setFilterMinSuccessRate('');
    setFilterCensorship('ALL');
    setFilterDNSSEC('ALL');
    setFilterRebinding('ALL');
    setFilterISP('');
    setFilterCountry('');
    setFilterCDN('ALL');
  };

  // Selection states
  const [selectedKeys, setSelectedKeys] = useState<Set<string>>(new Set());
  
  // Custom Resolver creation states
  const [customIP, setCustomIP] = useState('');
  const [customProvider, setCustomProvider] = useState('');
  const [customCategory, setCustomCategory] = useState('custom');
  const [customProtocol, setCustomProtocol] = useState('udp');

  // Bulk import states
  const [modalTab, setModalTab] = useState<'single' | 'bulk'>('single');
  const [bulkText, setBulkText] = useState('');
  const [bulkFile, setBulkFile] = useState<File | null>(null);
  const [isImporting, setIsImporting] = useState(false);

  // Fetch Public DNS States
  const [showFetchPublicModal, setShowFetchPublicModal] = useState(false);
  const [selectedPublicSource, setSelectedPublicSource] = useState<'curated' | 'bls' | 'trickest'>('curated');
  const [isFetchingPublic, setIsFetchingPublic] = useState(false);
  const [fetchPublicResult, setFetchPublicResult] = useState<{ added_count: number; total_found: number } | null>(null);

  // Sweep configuration states
  const [concurrencyLimit, setConcurrencyLimit] = useState(100);
  const [qpsLimit, setQpsLimit] = useState(0);
  const [timeoutMs, setTimeoutMs] = useState(3000);
  const [attempts, setAttempts] = useState(3);
  const [cacheBusting, setCacheBusting] = useState(true);
  const [referenceDomain, setReferenceDomain] = useState('google.com');
  const [selectedProtocols, setSelectedProtocols] = useState<string[]>(['udp', 'tcp', 'dot', 'doh', 'doq']);
  const [queryTypes, setQueryTypes] = useState<string[]>(['A']);
  const [dnsClass, setDnsClass] = useState('IN');
  const [queryGenerator, setQueryGenerator] = useState('random');
  const [domainSource, setDomainSource] = useState('default');
  const [customDomains, setCustomDomains] = useState('');
  const [wordlistURL, setWordlistURL] = useState('');
  const [expectResponse, setExpectResponse] = useState('');

  // Trace Dialog States
  const [showTraceModal, setShowTraceModal] = useState(false);
  const [traceResolverKey, setTraceResolverKey] = useState<string | null>(null);
  const [traceDomain, setTraceDomain] = useState('google.com');
  const [traceSteps, setTraceSteps] = useState<any[]>([]);
  const [isTracing, setIsTracing] = useState(false);

  // AXFR Dialog States
  const [showAXFRModal, setShowAXFRModal] = useState(false);
  const [axfrResolverKey, setAxfrResolverKey] = useState<string | null>(null);
  const [axfrDomain, setAxfrDomain] = useState('google.com');
  const [axfrResult, setAxfrResult] = useState<any | null>(null);
  const [isTestingAXFR, setIsTestingAXFR] = useState(false);

  // Advanced Single DNS Test Dialog States
  const [showAdvancedTestModal, setShowAdvancedTestModal] = useState(false);
  const [advancedTestResolverKey, setAdvancedTestResolverKey] = useState<string | null>(null);
  const [advDomain, setAdvDomain] = useState('google.com');
  const [advQueryType, setAdvQueryType] = useState('A');
  const [advDNSClass, setAdvDNSClass] = useState('IN');
  const [advTimeout, setAdvTimeout] = useState(3000);
  const [advAttempts, setAdvAttempts] = useState(3);
  const [advCacheBusting, setAdvCacheBusting] = useState(true);
  const [advExpectResponse, setAdvExpectResponse] = useState('');
  const [isTestingSingle, setIsTestingSingle] = useState(false);
  const [singleTestResult, setSingleTestResult] = useState<any | null>(null);
  const [singleTestError, setSingleTestError] = useState<string | null>(null);

  // UI Toast Alert Status
  const [toastMessage, setToastMessage] = useState<string | null>(null);
  const [toastType, setToastType] = useState<'success' | 'error' | 'info'>('success');

  const parentRef = useRef<HTMLDivElement>(null);

  // Trigger Toast helper
  const triggerToast = (msg: string, type: 'success' | 'error' | 'info' = 'success') => {
    setToastMessage(msg);
    setToastType(type);
    setTimeout(() => setToastMessage(null), 5000);
  };

  // 1. Fetch resolver list from database
  const fetchResolversList = async () => {
    try {
      const activeToken = token || localStorage.getItem('cc_client_token') || '';
      const res = await fetch('/api/dns/resolvers', {
        headers: { 'Authorization': `Bearer ${activeToken}` }
      });
      if (res.ok) {
        const data = await res.json();
        setResolvers(data || []);
      }
    } catch (e) {
      console.error(e);
      triggerToast("Failed to fetch resolvers list", "error");
    }
  };

  // 2. Fetch configurations and active system settings on load
  const fetchTesterConfig = async () => {
    try {
      const activeToken = token || localStorage.getItem('cc_client_token') || '';
      // Get sweep config
      const resConf = await fetch('/api/dns/config', {
        headers: { 'Authorization': `Bearer ${activeToken}` }
      });
      if (resConf.ok) {
        const conf = await resConf.json();
        setConcurrencyLimit(conf.ConcurrencyLimit || 100);
        setQpsLimit(conf.QPSLimit || 0);
        setTimeoutMs(conf.TimeoutMs || 3000);
        setAttempts(conf.Attempts || 3);
        setCacheBusting(conf.CacheBusting !== false);
        setReferenceDomain(conf.ReferenceDomain || 'google.com');
        if (conf.QueryTypes && conf.QueryTypes.length > 0) {
          setQueryTypes(conf.QueryTypes);
        }
        if (conf.DNSClass) {
          setDnsClass(conf.DNSClass);
        }
        if (conf.QueryGenerator) {
          setQueryGenerator(conf.QueryGenerator);
        }
        if (conf.DomainSource) {
          setDomainSource(conf.DomainSource);
        }
        if (conf.CustomDomains) {
          setCustomDomains(conf.CustomDomains.join('\n'));
        }
        if (conf.WordlistURL) {
          setWordlistURL(conf.WordlistURL);
        }
        if (conf.ExpectResponse) {
          setExpectResponse(conf.ExpectResponse);
        }
      }

      // Get applied DNS url
      const resSetting = await fetch('/api/v2ray/client/settings', {
        headers: { 'Authorization': `Bearer ${activeToken}` }
      });
      if (resSetting.ok) {
        const settings = await resSetting.json();
        if (settings.dns_doh_url) {
          setAppliedResolver(settings.dns_doh_url);
        }
      }
    } catch (e) {
      console.error(e);
    }
  };

  const fetchBulkProgress = async () => {
    try {
      const activeToken = token || localStorage.getItem('cc_client_token') || '';
      const res = await fetch('/api/dns/resolvers/bulk/progress', {
        headers: { 'Authorization': `Bearer ${activeToken}` }
      });
      if (res.ok) {
        const data = await res.json();
        setBulkProgress(data);
      }
    } catch (e) {
      console.error("Failed to fetch bulk progress", e);
    }
  };

  const handleBulkImport = async () => {
    try {
      setIsImporting(true);
      const activeToken = token || localStorage.getItem('cc_client_token') || '';
      const formData = new FormData();
      if (bulkFile) {
        formData.append('file', bulkFile);
      } else {
        if (!bulkText.trim()) {
          triggerToast("Please enter some DNS resolver strings or choose a file to upload.", "error");
          setIsImporting(false);
          return;
        }
        const response = await fetch('/api/dns/resolvers/bulk', {
          method: 'POST',
          headers: {
            'Content-Type': 'application/json',
            'Authorization': `Bearer ${activeToken}`
          },
          body: JSON.stringify({ text: bulkText })
        });
        if (!response.ok) {
          const errData = await response.json();
          throw new Error(errData.error || 'Failed to submit bulk import');
        }
        setShowAddModal(false);
        setBulkText('');
        setBulkFile(null);
        triggerToast("Bulk import started in background", "info");
        setIsImporting(false);
        return;
      }

      const response = await fetch('/api/dns/resolvers/bulk', {
        method: 'POST',
        headers: {
          'Authorization': `Bearer ${activeToken}`
        },
        body: formData
      });

      if (!response.ok) {
        const errData = await response.json();
        throw new Error(errData.error || 'Failed to submit bulk import file');
      }

      setShowAddModal(false);
      setBulkText('');
      setBulkFile(null);
      triggerToast("Bulk import started in background", "info");
    } catch (err: any) {
      triggerToast("Error: " + err.message, "error");
    } finally {
      setIsImporting(false);
    }
  };

  useEffect(() => {
    fetchResolversList();
    fetchTesterConfig();
    fetchBulkProgress();
  }, []);

  // 3. Setup WebSocket connection
  useEffect(() => {
    const protocol = window.location.protocol === 'https:' ? 'wss:' : 'ws:';
    const activeToken = token || localStorage.getItem('cc_client_token') || '';
    const wsUrl = `${protocol}//${window.location.host}/ws?token=${activeToken}`;

    const socket = new WebSocket(wsUrl);

    socket.onopen = () => {
      console.log('DNS Tester WS Connected');
      // Query current sweep stats on boot
      socket.send(JSON.stringify({
        type: 'dns:telemetry',
        data: {}
      }));
    };

    socket.onmessage = (event) => {
      try {
        const msg = JSON.parse(event.data);
        if (msg.type === 'dns:trace_result') {
          setTraceSteps(msg.steps || []);
          setIsTracing(false);
        } else if (msg.type === 'dns:axfr_result') {
          setAxfrResult(msg.result || null);
          setIsTestingAXFR(false);
        } else if (msg.type === 'dns:telemetry') {
          if (msg.stats) {
            const raw = msg.stats;
            const active = raw.phase === "benchmark_sweep";
            setJobStats({
              total_resolvers: raw.total_targets || 0,
              processed_resolvers: raw.tested || 0,
              successful_resolvers: raw.healthy || 0,
              failed_resolvers: raw.failed || 0,
              in_flight_resolvers: raw.in_flight || 0,
              is_active: active,
              elapsed_ms: 0
            });
            if (active && !isSweeping) {
              setSweeping(true);
              if (!sweepStartTime) {
                setSweepStartTime(Date.now());
              }
            }
          }
          
          if (msg.event === 'dns.candidate' && msg.data) {
            const candidates = Array.isArray(msg.data) ? msg.data : [msg.data];
            candidates.forEach((res: any) => {
              const key = getResolverKey(res.ip, res.protocol);
              updateResolver(key, {
                latency_ms: res.latency_ms,
                jitter_ms: res.jitter_ms,
                success_rate: res.success_rate_pct / 100,
                packet_loss: res.packet_loss_pct,
                censored: res.censorship === 'manipulated' || res.censorship === 'sinkhole',
                nxdomain_hijacked: res.censorship === 'hijacked',
                censorship_status: res.censorship,
                dnssec_valid: res.dnssec_valid,
                dns_rebinding_vuln: res.dns_rebinding_vuln,
                query_type: res.query_type,
                dns_class: res.dns_class,
                domain: res.domain,
                clever_score: res.clever_score,
                completed_at: res.checked_at,
                error_message: res.error,
                is_testing: false,
                resolved_ip: res.resolved_ip,
                country_code: res.country_code,
                country_name: res.country_name,
                city: res.city,
                isp: res.isp,
                is_cdn: res.is_cdn,
                cdn_provider: res.cdn_provider,
                expected_match: res.expected_match !== false,
              });
            });
          } else if (msg.event === 'dns.bulk_progress' && msg.data) {
            setBulkProgress(msg.data);
            if (!msg.data.active) {
              fetchResolversList();
            }
          } else if (msg.event === 'dns.started') {
            setSweeping(true);
            setSweepStartTime(Date.now());
          } else if (msg.event === 'dns.stopped' || msg.event === 'dns.finished') {
            setSweeping(false);
            setSweepStartTime(null);
            fetchResolversList(); // refresh scores
            fetchTesterConfig(); // refresh applied
          }
        }
      } catch (err) {
        console.error("WS decode error", err);
      }
    };

    socket.onerror = (e) => {
      console.error("WS error", e);
    };

    setWs(socket);
    return () => socket.close();
  }, [token, isSweeping, sweepStartTime]);

  // Timer Effect for Sweep Duration
  useEffect(() => {
    let timer: any = null;
    if (isSweeping && sweepStartTime) {
      timer = setInterval(() => {
        setElapsedMs(Date.now() - sweepStartTime);
      }, 100);
    } else {
      setElapsedMs(0);
    }
    return () => {
      if (timer) clearInterval(timer);
    };
  }, [isSweeping, sweepStartTime]);

  // 4. Custom resolver lifecycle methods
  const handleAddCustomResolver = async () => {
    if (!customIP.trim()) {
      triggerToast("IP address is required", "error");
      return;
    }
    try {
      const activeToken = token || localStorage.getItem('cc_client_token') || '';
      const payload = {
        ip: customIP.trim(),
        protocol: customProtocol,
        provider_name: customProvider.trim() || 'Custom Resolver',
        category: customCategory,
        is_custom: true,
        support_udp: customProtocol === 'udp',
        support_tcp: customProtocol === 'tcp',
        support_dot: customProtocol === 'dot',
        support_doh: customProtocol === 'doh',
        support_doq: customProtocol === 'doq'
      };

      const res = await fetch('/api/dns/resolvers', {
        method: 'POST',
        headers: {
          'Authorization': `Bearer ${activeToken}`,
          'Content-Type': 'application/json'
        },
        body: JSON.stringify(payload)
      });

      if (res.ok) {
        triggerToast("Custom resolver saved successfully");
        setShowAddModal(false);
        setCustomIP('');
        setCustomProvider('');
        fetchResolversList();
      } else {
        const data = await res.json();
        triggerToast(data.error || "Failed to save resolver", "error");
      }
    } catch (e) {
      console.error(e);
      triggerToast("Error saving custom resolver", "error");
    }
  };

  const handleDeleteResolver = useCallback(async (key: string) => {
    if (!window.confirm("Are you sure you want to delete this resolver?")) return;
    const resolver = resolvers[key];
    if (!resolver) return;

    try {
      const activeToken = token || localStorage.getItem('cc_client_token') || '';
      const res = await fetch(`/api/dns/resolvers/${resolver.id}`, {
        method: 'DELETE',
        headers: { 'Authorization': `Bearer ${activeToken}` }
      });

      if (res.ok) {
        triggerToast("Resolver deleted successfully");
        fetchResolversList();
      }
    } catch (e) {
      console.error(e);
      triggerToast("Error deleting custom resolver", "error");
    }
  }, [resolvers, token]);

  const handleApplyResolver = useCallback(async (key: string) => {
    const resolver = resolvers[key];
    if (!resolver) return;

    try {
      const activeToken = token || localStorage.getItem('cc_client_token') || '';
      const res = await fetch('/api/dns/core/apply', {
        method: 'POST',
        headers: {
          'Authorization': `Bearer ${activeToken}`,
          'Content-Type': 'application/json'
        },
        body: JSON.stringify({
          ip: resolver.ip,
          protocol: resolver.protocol
        })
      });

      if (res.ok) {
        const data = await res.json();
        setAppliedResolver(data.resolver_applied);
        triggerToast(`Applied ${resolver.provider_name} (${resolver.protocol.toUpperCase()}) as system DNS! Core restart: ${data.core_reloaded ? "Success" : "Bypassed"}`);
      } else {
        const data = await res.json();
        triggerToast(data.error || "Failed to apply resolver", "error");
      }
    } catch (e) {
      console.error(e);
      triggerToast("Error applying active resolver", "error");
    }
  }, [resolvers, token]);

  const handleRunSingleTest = useCallback(async () => {
    if (!advancedTestResolverKey) return;
    const resolver = resolvers[advancedTestResolverKey];
    if (!resolver) return;

    setIsTestingSingle(true);
    setSingleTestResult(null);
    setSingleTestError(null);

    try {
      const activeToken = token || localStorage.getItem('cc_client_token') || '';
      const res = await fetch(`/api/dns/resolvers/${resolver.id}/test`, {
        method: 'POST',
        headers: {
          'Authorization': `Bearer ${activeToken}`,
          'Content-Type': 'application/json'
        },
        body: JSON.stringify({
          domain: advDomain,
          query_type: advQueryType,
          dns_class: advDNSClass,
          timeout_ms: advTimeout,
          attempts: advAttempts,
          cache_busting: advCacheBusting,
          expect_response: advExpectResponse
        })
      });

      if (!res.ok) {
        const errData = await res.json();
        throw new Error(errData.error || "Failed to perform test query");
      }

      const data = await res.json();
      setSingleTestResult(data);
      triggerToast("DNS test completed successfully!");
      fetchResolversList();
    } catch (e: any) {
      console.error(e);
      setSingleTestError(e.message || "Failed to run DNS test query");
      triggerToast(e.message || "Failed to run DNS test query", "error");
    } finally {
      setIsTestingSingle(false);
    }
  }, [advancedTestResolverKey, resolvers, token, advDomain, advQueryType, advDNSClass, advTimeout, advAttempts, advCacheBusting, advExpectResponse]);

  const handleRunTrace = (resolverIp: string, domain: string) => {
    if (!ws || ws.readyState !== WebSocket.OPEN) {
      triggerToast("WebSocket connection closed. Try refreshing.", "error");
      return;
    }
    setIsTracing(true);
    setTraceSteps([]);
    ws.send(JSON.stringify({
      type: 'dns:trace',
      data: {
        resolver_ip: resolverIp,
        domain: domain
      }
    }));
  };

  const handleRunAXFR = (resolverIp: string, domain: string) => {
    if (!ws || ws.readyState !== WebSocket.OPEN) {
      triggerToast("WebSocket connection closed. Try refreshing.", "error");
      return;
    }
    setIsTestingAXFR(true);
    setAxfrResult(null);
    ws.send(JSON.stringify({
      type: 'dns:axfr',
      data: {
        resolver_ip: resolverIp,
        domain: domain
      }
    }));
  };

  // 5. Sweep Control commands
  const handleStartSweep = () => {
    if (!ws || ws.readyState !== WebSocket.OPEN) {
      triggerToast("WebSocket connection closed. Try refreshing.", "error");
      return;
    }

    // Prepare custom resolver array list from store
    const list = resolverKeys.map(k => resolvers[k]).filter(Boolean);

    ws.send(JSON.stringify({
      type: 'dns:start',
      data: {
        concurrency_limit: concurrencyLimit,
        qps_limit: qpsLimit,
        timeout_ms: timeoutMs,
        attempts: attempts,
        cache_busting: cacheBusting,
        reference_domain: referenceDomain,
        selected_protocols: selectedProtocols,
        custom_resolvers: list.filter(r => r.is_custom),
        query_types: queryTypes,
        dns_class: dnsClass,
        query_generator: queryGenerator,
        domain_source: domainSource,
        custom_domains: customDomains.split(/[\n,]+/).map(d => d.trim()).filter(Boolean),
        wordlist_url: wordlistURL,
        expect_response: expectResponse,
      }
    }));

    // Visually set testing state
    resolverKeys.forEach(k => {
      const r = resolvers[k];
      if (r && selectedProtocols.includes(r.protocol)) {
        updateResolver(k, { is_testing: true });
      }
    });

    triggerToast("Starting multi-protocol DNS scan sweep...");
  };

  const handleStopSweep = () => {
    if (!ws || ws.readyState !== WebSocket.OPEN) return;
    ws.send(JSON.stringify({
      type: 'dns:stop',
      data: {}
    }));
    triggerToast("Stopping DNS sweep...");
  };

  const handleResetSweep = async () => {
    clearResults();
    triggerToast("Cleaned local DNS metrics. Database records untouched.");
  };

  const handleProtocolCheckbox = (proto: string) => {
    if (selectedProtocols.includes(proto)) {
      setSelectedProtocols(selectedProtocols.filter(p => p !== proto));
    } else {
      setSelectedProtocols([...selectedProtocols, proto]);
    }
  };

  // 6. Filtering and Sorting in memory
  const filteredKeys = useMemo(() => {
    return resolverKeys.filter((key) => {
      const r = resolvers[key];
      if (!r) return false;

      // search filter
      const searchMatch = r.ip.includes(search) || r.provider_name.toLowerCase().includes(search.toLowerCase());
      if (!searchMatch) return false;

      // protocol filter
      if (protocolFilter !== 'ALL' && r.protocol.toLowerCase() !== protocolFilter.toLowerCase()) {
        return false;
      }

      // category filter
      if (categoryFilter !== 'ALL' && r.category.toLowerCase() !== categoryFilter.toLowerCase()) {
        return false;
      }

      // Advanced Filters
      if (filterMinLatency !== '') {
        const val = parseFloat(filterMinLatency);
        if (!isNaN(val) && (r.latency_ms <= 0 || r.latency_ms < val)) return false;
      }
      if (filterMaxLatency !== '') {
        const val = parseFloat(filterMaxLatency);
        if (!isNaN(val) && (r.latency_ms <= 0 || r.latency_ms > val)) return false;
      }
      if (filterMinSuccessRate !== '') {
        const val = parseFloat(filterMinSuccessRate);
        if (!isNaN(val) && (r.success_rate * 100 < val)) return false;
      }
      if (filterCensorship !== 'ALL') {
        const status = (r.censorship_status || '').toLowerCase();
        if (status !== filterCensorship.toLowerCase()) return false;
      }
      if (filterDNSSEC !== 'ALL') {
        const isDnssec = !!r.dnssec_valid;
        const wantDnssec = filterDNSSEC === 'valid';
        if (isDnssec !== wantDnssec) return false;
      }
      if (filterRebinding !== 'ALL') {
        const isVuln = !!r.dns_rebinding_vuln;
        const wantVuln = filterRebinding === 'vulnerable';
        if (isVuln !== wantVuln) return false;
      }
      if (filterISP !== '' && !(r.isp || '').toLowerCase().includes(filterISP.toLowerCase())) {
        return false;
      }
      if (filterCountry !== '' && !(r.country_name || r.country_code || '').toLowerCase().includes(filterCountry.toLowerCase())) {
        return false;
      }
      if (filterCDN !== 'ALL') {
        const isCDN = !!r.is_cdn;
        const wantCDN = filterCDN === 'cdn';
        if (isCDN !== wantCDN) return false;
      }

      return true;
    }).sort((aKey, bKey) => {
      const a = resolvers[aKey];
      const b = resolvers[bKey];
      if (!a || !b) return 0;

      let compareVal = 0;
      switch (sortBy) {
        case 'latency_ms':
          // Sort untested to the bottom
          const aLat = a.latency_ms > 0 ? a.latency_ms : 999999;
          const bLat = b.latency_ms > 0 ? b.latency_ms : 999999;
          compareVal = aLat - bLat;
          break;
        case 'success_rate':
          compareVal = a.success_rate - b.success_rate;
          break;
        case 'jitter_ms':
          const aJit = a.latency_ms > 0 ? a.jitter_ms : 999999;
          const bJit = b.latency_ms > 0 ? b.jitter_ms : 999999;
          compareVal = aJit - bJit;
          break;
        case 'clever_score':
        default:
          compareVal = a.clever_score - b.clever_score;
          break;
      }

      return sortOrder === 'asc' ? compareVal : -compareVal;
    });
  }, [
    resolverKeys, resolvers, search, categoryFilter, protocolFilter, sortBy, sortOrder,
    filterMinLatency, filterMaxLatency, filterMinSuccessRate, filterCensorship,
    filterDNSSEC, filterRebinding, filterISP, filterCountry, filterCDN
  ]);

  const handleToggleSelect = useCallback((key: string) => {
    setSelectedKeys((prev) => {
      const next = new Set(prev);
      if (next.has(key)) {
        next.delete(key);
      } else {
        next.add(key);
      }
      return next;
    });
  }, []);

  const isAllSelected = useMemo(() => {
    if (filteredKeys.length === 0) return false;
    return filteredKeys.every(k => selectedKeys.has(k));
  }, [filteredKeys, selectedKeys]);

  const handleToggleSelectAll = useCallback(() => {
    setSelectedKeys((prev) => {
      const next = new Set<string>();
      if (!isAllSelected) {
        filteredKeys.forEach(k => next.add(k));
      }
      return next;
    });
  }, [filteredKeys, isAllSelected]);

  const copyHealthyToClipboard = () => {
    const targetKeys = selectedKeys.size > 0 ? Array.from(selectedKeys) : filteredKeys;
    const healthyIPs = targetKeys
      .map(k => resolvers[k])
      .filter(r => r && r.latency_ms > 0 && r.packet_loss < 100)
      .map(r => r.ip);

    if (healthyIPs.length === 0) {
      triggerToast("No healthy DNS resolvers found to copy", "info");
      return;
    }

    const textToCopy = Array.from(new Set(healthyIPs)).join('\n');
    navigator.clipboard.writeText(textToCopy)
      .then(() => triggerToast(`Copied ${healthyIPs.length} healthy resolver IPs to clipboard!`, "success"))
      .catch((err) => triggerToast("Failed to copy: " + err.message, "error"));
  };

  const exportResolvers = (format: 'json' | 'csv') => {
    const targetKeys = selectedKeys.size > 0 ? Array.from(selectedKeys) : filteredKeys;
    const items = targetKeys.map(k => resolvers[k]).filter(Boolean);

    if (items.length === 0) {
      triggerToast("No resolvers found to export", "info");
      return;
    }

    let blobContent = '';
    let mimeType = 'application/json';
    let filename = `dns_resolvers_export_${Date.now()}`;

    if (format === 'json') {
      blobContent = JSON.stringify(items, null, 2);
      mimeType = 'application/json';
      filename += '.json';
    } else {
      const headers = ['IP', 'Protocol', 'Provider', 'Category', 'Latency (ms)', 'Jitter (ms)', 'Success Rate (%)', 'Censorship', 'DNSSEC Valid', 'Rebinding Vuln', 'Country', 'ISP'];
      const rows = items.map(r => [
        r.ip,
        r.protocol,
        r.provider_name,
        r.category,
        r.latency_ms > 0 ? r.latency_ms : '',
        r.latency_ms > 0 ? r.jitter_ms : '',
        r.success_rate * 100,
        r.censorship_status || 'unverified',
        r.dnssec_valid ? 'Yes' : 'No',
        r.dns_rebinding_vuln ? 'Yes' : 'No',
        r.country_name || r.country_code || '',
        r.isp || ''
      ]);
      
      const csvContent = [headers.join(','), ...rows.map(row => row.map(val => `"${String(val).replace(/"/g, '""')}"`).join(','))].join('\n');
      blobContent = csvContent;
      mimeType = 'text/csv';
      filename += '.csv';
    }

    const blob = new Blob([blobContent], { type: mimeType });
    const url = URL.createObjectURL(blob);
    const link = document.createElement('a');
    link.href = url;
    link.download = filename;
    document.body.appendChild(link);
    link.click();
    document.body.removeChild(link);
    URL.revokeObjectURL(url);
    triggerToast(`Exported ${items.length} resolvers successfully!`, "success");
  };

  const deleteSelectedResolvers = async () => {
    if (selectedKeys.size === 0) return;

    const selectedResolvers = Array.from(selectedKeys)
      .map(k => resolvers[k])
      .filter(Boolean);

    if (selectedResolvers.length === 0) return;

    if (!window.confirm(`Are you sure you want to delete the ${selectedResolvers.length} selected resolvers?`)) {
      return;
    }

    const idsToDelete = selectedResolvers.map(r => r.id).filter(id => id > 0);

    try {
      if (idsToDelete.length > 0) {
        const activeToken = token || localStorage.getItem('cc_client_token') || '';
        const res = await fetch('/api/dns/resolvers/batch-delete', {
          method: 'POST',
          headers: { 
            'Authorization': `Bearer ${activeToken}`,
            'Content-Type': 'application/json'
          },
          body: JSON.stringify({ ids: idsToDelete })
        });

        if (!res.ok) {
          throw new Error(await res.text());
        }
      }

      triggerToast(`Successfully deleted ${selectedResolvers.length} resolvers!`);
      setSelectedKeys(new Set());
      fetchResolversList();
    } catch (e: any) {
      console.error(e);
      triggerToast("Error deleting selected resolvers: " + e.message, "error");
    }
  };

  const handleFetchPublicDNS = async () => {
    setIsFetchingPublic(true);
    setFetchPublicResult(null);
    try {
      const activeToken = token || localStorage.getItem('cc_client_token') || '';
      const res = await fetch('/api/dns/resolvers/fetch-public', {
        method: 'POST',
        headers: {
          'Authorization': `Bearer ${activeToken}`,
          'Content-Type': 'application/json'
        },
        body: JSON.stringify({ source: selectedPublicSource })
      });

      if (!res.ok) {
        throw new Error(await res.text());
      }

      const data = await res.json();
      setFetchPublicResult(data);
      triggerToast(`Fetched list and added ${data.added_count} new resolvers!`);
      fetchResolversList();
    } catch (e: any) {
      console.error(e);
      triggerToast("Failed to fetch public DNS: " + e.message, "error");
    } finally {
      setIsFetchingPublic(false);
    }
  };

  // Virtualizer setup
  const virtualizer = useVirtualizer({
    count: filteredKeys.length,
    getScrollElement: () => parentRef.current,
    estimateSize: () => 44,
    overscan: 10,
  });

  const toggleSort = (field: string) => {
    if (sortBy === field) {
      setSortOrder(sortOrder === 'asc' ? 'desc' : 'asc');
    } else {
      setSortBy(field);
      setSortOrder(field === 'latency_ms' || field === 'jitter_ms' ? 'asc' : 'desc');
    }
  };

  // Helper to check if DOH URL matches this resolver profile
  const checkIsApplied = (resolver: DNSResolver) => {
    const cleanApplied = appliedResolver.toLowerCase();
    const cleanIP = resolver.ip.toLowerCase();
    
    if (resolver.protocol === 'doh') {
      return cleanApplied.includes(cleanIP) && cleanApplied.includes('dns-query');
    } else if (resolver.protocol === 'dot') {
      return cleanApplied.includes(cleanIP) && cleanApplied.includes('853');
    }
    return cleanApplied === cleanIP;
  };

  const progressPercent = useMemo(() => {
    if (!jobStats || jobStats.total_resolvers === 0) return 0;
    return (jobStats.processed_resolvers / jobStats.total_resolvers) * 100;
  }, [jobStats]);

  return (
    <div className="page-container animate-fade-in" style={{ padding: '8px 0', fontFamily: 'var(--font-sans)', display: 'flex', flexDirection: 'column', gap: 20, minHeight: 'calc(100vh - 120px)' }}>
      
      {/* Toast Alert */}
      {toastMessage && (
        <div style={{
          position: 'fixed',
          top: 20,
          right: 20,
          background: toastType === 'success' ? '#eefbf3' : toastType === 'error' ? 'rgba(239, 68, 68, 0.08)' : 'var(--color-brand-light)',
          color: toastType === 'success' ? '#15803d' : toastType === 'error' ? 'var(--color-brand-red)' : 'var(--color-brand)',
          border: `1px solid ${toastType === 'success' ? '#22c55e' : toastType === 'error' ? 'var(--color-brand-red)' : 'var(--color-brand-border)'}`,
          padding: '12px 20px',
          borderRadius: 8,
          boxShadow: '0 10px 20px rgba(0,0,0,0.05)',
          zIndex: 1000,
          fontSize: 12,
          fontWeight: 700,
          display: 'flex',
          alignItems: 'center',
          gap: 8,
          animation: 'slideIn 0.3s ease'
        }}>
          {toastType === 'success' ? <FiCheckCircle /> : <FiAlertTriangle />}
          <span>{toastMessage}</span>
        </div>
      )}

      {/* Grid of Stats Cards */}
      <div style={{ display: 'grid', gridTemplateColumns: 'repeat(auto-fit, minmax(220px, 1fr))', gap: 16 }}>
        
        {/* Core sweep Control */}
        <div className="g-card" style={{ padding: 20, display: 'flex', flexDirection: 'column', justifyContent: 'space-between', minHeight: 130 }}>
          <div style={{ display: 'flex', justifyContent: 'space-between', alignItems: 'center' }}>
            <span style={{ fontSize: 13, fontWeight: 700, color: 'var(--color-brand-heading)', display: 'flex', alignItems: 'center', gap: 6 }}>
              <FiActivity /> Diagnostic Status
            </span>
            <span style={{ 
              width: 10, height: 10, borderRadius: '50%', 
              background: isSweeping ? 'var(--color-brand-green)' : 'var(--color-brand-muted)',
              boxShadow: isSweeping ? '0 0 10px var(--color-brand-green)' : 'none'
            }} />
          </div>

          <div style={{ margin: '14px 0 0' }}>
            <div style={{ fontSize: 18, fontWeight: 800, color: 'var(--color-brand-heading)' }}>
              {isSweeping ? "Sweeping Networks..." : "Daemon Idle"}
            </div>
            <div style={{ fontSize: 11, color: 'var(--color-brand-text)', marginTop: 4 }}>
              Active test loops validating DNS hijacking/censorship benchmarks.
            </div>
          </div>

          <div style={{ display: 'flex', gap: 10, marginTop: 16 }}>
            {isSweeping ? (
              <button onClick={handleStopSweep} className="btn" style={{ flex: 1, display: 'flex', alignItems: 'center', justifyContent: 'center', gap: 6, background: 'var(--color-brand-red)', color: '#fff', border: 'none', borderRadius: 8, padding: '8px 12px', cursor: 'pointer', fontSize: 12, fontWeight: 700 }}>
                <FiStopCircle /> Stop Sweep
              </button>
            ) : (
              <button onClick={handleStartSweep} className="btn btn-primary" style={{ flex: 1, display: 'flex', alignItems: 'center', justifyContent: 'center', gap: 6, borderRadius: 8, padding: '8px 12px', fontSize: 12, fontWeight: 700 }}>
                <FiPlay /> Start Sweep
              </button>
            )}
            <button onClick={handleResetSweep} className="btn" style={{ border: '1px solid var(--color-brand-border)', background: 'var(--color-brand-card)', color: 'var(--color-brand-heading)', padding: '8px 12px', borderRadius: 8, cursor: 'pointer', fontSize: 12, fontWeight: 700 }} title="Reset Metrics">
              Reset
            </button>
          </div>
        </div>

        {/* Live Job Progress Meter */}
        <div className="g-card" style={{ padding: 20, display: 'flex', flexDirection: 'column', justifyContent: 'space-between', minHeight: 130 }}>
          <div>
            <span style={{ fontSize: 13, fontWeight: 700, color: 'var(--color-brand-heading)', display: 'flex', alignItems: 'center', gap: 6 }}>
              <FiTrendingUp /> Sweep Progress
            </span>
          </div>

          {jobStats ? (
            <div style={{ margin: '10px 0' }}>
              <div style={{ display: 'flex', justifyContent: 'space-between', fontSize: 12, fontWeight: 700, color: 'var(--color-brand-heading)', marginBottom: 6 }}>
                <span>Tested: {jobStats.processed_resolvers}/{jobStats.total_resolvers}</span>
                <span>{progressPercent.toFixed(0)}%</span>
              </div>
              <div style={{ width: '100%', height: 6, background: 'var(--color-brand-bg)', borderRadius: 3, overflow: 'hidden' }}>
                <div style={{ width: `${progressPercent}%`, height: '100%', background: 'linear-gradient(90deg, var(--color-brand) 0%, var(--color-brand-blue) 100%)', borderRadius: 3, transition: 'width 0.2s ease' }} />
              </div>
              <div style={{ display: 'flex', justifyContent: 'space-between', fontSize: 10, color: 'var(--color-brand-text)', marginTop: 8 }}>
                <span style={{ color: 'var(--color-brand-green)', fontWeight: 600 }}>Healthy: {jobStats.successful_resolvers}</span>
                <span style={{ color: 'var(--color-brand-red)', fontWeight: 600 }}>Failed: {jobStats.failed_resolvers}</span>
                <span>In-flight: {jobStats.in_flight_resolvers}</span>
              </div>
            </div>
          ) : (
            <div style={{ textAlign: 'center', color: 'var(--color-brand-muted)', padding: '20px 0', fontSize: 12 }}>
              No active test runs recorded. Click "Start Sweep" to begin.
            </div>
          )}

          <div style={{ fontSize: 10, color: 'var(--color-brand-muted)' }}>
            Elapsed Duration: {isSweeping || elapsedMs > 0 ? `${(elapsedMs / 1000).toFixed(1)}s` : '0.0s'}
          </div>
        </div>

        {/* Applied Settings Card */}
        <div className="g-card" style={{ padding: 20, display: 'flex', flexDirection: 'column', justifyContent: 'space-between', minHeight: 130 }}>
          <div>
            <span style={{ fontSize: 13, fontWeight: 700, color: 'var(--color-brand-heading)', display: 'flex', alignItems: 'center', gap: 6 }}>
              <FiServer /> System Core DNS
            </span>
          </div>

          <div style={{ margin: '10px 0' }}>
            <div style={{ fontSize: 11, textTransform: 'uppercase', letterSpacing: '0.5px', color: 'var(--color-brand-muted)', fontWeight: 600 }}>Active Endpoint</div>
            <div style={{ fontSize: 13, fontWeight: 700, color: 'var(--color-brand-heading)', overflow: 'hidden', textOverflow: 'ellipsis', whiteSpace: 'nowrap', marginTop: 4 }} title={appliedResolver}>
              {appliedResolver || "Not customized (System Default)"}
            </div>
          </div>

          <div style={{ fontSize: 10, color: 'var(--color-brand-muted)', display: 'flex', alignItems: 'center', gap: 4 }}>
            <FiInfo /> Applied settings automatically override Xray/Sing-box core config.
          </div>
        </div>

      </div>

      {/* Bulk Import Progress Banner */}
      {bulkProgress && bulkProgress.active && (
        <div className="g-card" style={{ padding: 20, display: 'flex', flexDirection: 'column', gap: 10, background: 'rgba(59, 130, 246, 0.05)', borderLeft: '4px solid var(--color-brand)', marginTop: 16 }}>
          <div style={{ display: 'flex', justifyContent: 'space-between', alignItems: 'center' }}>
            <span style={{ fontSize: 13, fontWeight: 700, color: 'var(--color-brand-heading)', display: 'flex', alignItems: 'center', gap: 6 }}>
              <FiTrendingUp className="animate-pulse" style={{ color: 'var(--color-brand)' }} /> Importing DNS Resolvers (Auto Probing & GeoIP Enrichment)
            </span>
            <span style={{ fontSize: 12, fontWeight: 700, color: 'var(--color-brand-heading)' }}>
              {bulkProgress.total > 0 ? ((bulkProgress.processed / bulkProgress.total) * 100).toFixed(0) : 0}%
            </span>
          </div>

          <div style={{ width: '100%', height: 6, background: 'var(--color-brand-bg)', borderRadius: 3, overflow: 'hidden' }}>
            <div style={{ 
              width: `${bulkProgress.total > 0 ? (bulkProgress.processed / bulkProgress.total) * 100 : 0}%`, 
              height: '100%', 
              background: 'linear-gradient(90deg, var(--color-brand) 0%, var(--color-brand-blue) 100%)', 
              borderRadius: 3, 
              transition: 'width 0.3s ease' 
            }} />
          </div>

          <div style={{ display: 'flex', flexWrap: 'wrap', justifyContent: 'space-between', fontSize: 11, color: 'var(--color-brand-text)', gap: 8 }}>
            <span>Processed: {bulkProgress.processed} / {bulkProgress.total}</span>
            <span style={{ color: 'var(--color-brand-green)', fontWeight: 600 }}>Added: {bulkProgress.added}</span>
            <span style={{ color: 'var(--color-brand-yellow)', fontWeight: 600 }}>Duplicates Ignored: {bulkProgress.duplicates}</span>
            <span style={{ fontStyle: 'italic' }}>Status: Running background checks...</span>
          </div>
        </div>
      )}

      {/* Main Panel Controls */}
      <div className="g-card" style={{ padding: 16, display: 'flex', flexDirection: 'column', gap: 16 }}>
        <div style={{ display: 'flex', flexWrap: 'wrap', justifyContent: 'space-between', alignItems: 'center', gap: 16 }}>
          
          {/* Filters & Search Toolbar */}
          <div style={{ display: 'flex', flexWrap: 'wrap', alignItems: 'center', gap: 10, flex: 1 }}>
            
            {/* Search Input */}
            <div style={{ position: 'relative', width: 220 }}>
              <FiSearch style={{ position: 'absolute', left: 10, top: 11, color: 'var(--color-brand-muted)' }} size={14} />
              <input 
                type="text"
                placeholder="Search resolvers by IP/Name..."
                value={search}
                onChange={(e) => setSearch(e.target.value)}
                style={{
                  width: '100%',
                  padding: '8px 12px 8px 32px',
                  borderRadius: 8,
                  border: '1px solid var(--color-brand-border)',
                  background: 'var(--color-brand-bg)',
                  color: 'var(--color-brand-heading)',
                  fontSize: 12,
                  outline: 'none',
                  transition: 'border-color 0.15s ease'
                }}
              />
            </div>

            {/* Protocol Selector */}
            <select
              value={protocolFilter}
              onChange={(e) => setProtocolFilter(e.target.value)}
              style={{
                padding: '8px 12px',
                borderRadius: 8,
                border: '1px solid var(--color-brand-border)',
                background: 'var(--color-brand-bg)',
                color: 'var(--color-brand-heading)',
                fontSize: 12,
                cursor: 'pointer'
              }}
            >
              <option value="ALL">All Protocols</option>
              <option value="udp">Plain UDP</option>
              <option value="tcp">Plain TCP</option>
              <option value="dot">DNS over TLS (DoT)</option>
              <option value="doh">DNS over HTTPS (DoH)</option>
              <option value="doq">DNS over QUIC (DoQ)</option>
            </select>

            {/* Category Filter */}
            <select
              value={categoryFilter}
              onChange={(e) => setCategoryFilter(e.target.value)}
              style={{
                padding: '8px 12px',
                borderRadius: 8,
                border: '1px solid var(--color-brand-border)',
                background: 'var(--color-brand-bg)',
                color: 'var(--color-brand-heading)',
                fontSize: 12,
                cursor: 'pointer'
              }}
            >
              <option value="ALL">All Categories</option>
              <option value="public">Public Standard</option>
              <option value="security">Ad/Security filter</option>
              <option value="custom">Custom Resolvers</option>
            </select>

            {/* Config Expand Switch */}
            <button 
              onClick={() => setShowSettings(!showSettings)}
              style={{
                padding: '8px 12px',
                borderRadius: 8,
                border: '1px solid var(--color-brand-border)',
                background: 'var(--color-brand-bg)',
                color: 'var(--color-brand-heading)',
                fontSize: 12,
                cursor: 'pointer',
                display: 'flex',
                alignItems: 'center',
                gap: 6
              }}
            >
              <FiSettings /> {showSettings ? "Hide Options" : "Test Options"}
            </button>

            {/* Advanced Filters Switch */}
            <button 
              onClick={() => setShowAdvancedFilters(!showAdvancedFilters)}
              style={{
                padding: '8px 12px',
                borderRadius: 8,
                border: `1px solid ${showAdvancedFilters ? 'var(--color-brand)' : 'var(--color-brand-border)'}`,
                background: showAdvancedFilters ? 'rgba(59, 130, 246, 0.08)' : 'var(--color-brand-bg)',
                color: showAdvancedFilters ? 'var(--color-brand)' : 'var(--color-brand-heading)',
                fontSize: 12,
                fontWeight: showAdvancedFilters ? 600 : 400,
                cursor: 'pointer',
                display: 'flex',
                alignItems: 'center',
                gap: 6,
                transition: 'all 0.2s ease'
              }}
            >
              <FiFilter /> {showAdvancedFilters ? "Hide Filters" : "Advanced Filters"}
            </button>
          </div>

          {/* Action trigger buttons */}
          <div style={{ display: 'flex', alignItems: 'center', gap: 10 }}>
            {/* Copy / Export Actions */}
            <div style={{ display: 'flex', alignItems: 'center', gap: 6, borderRight: '1px solid var(--color-brand-border)', paddingRight: 10 }}>
              {selectedKeys.size > 0 && (
                <>
                  <button
                    onClick={() => setSelectedKeys(new Set())}
                    style={{
                      padding: '8px 12px',
                      borderRadius: 8,
                      border: '1px solid var(--color-brand-border)',
                      background: 'var(--color-brand-bg)',
                      color: 'var(--color-brand-heading)',
                      fontSize: 12,
                      cursor: 'pointer',
                      display: 'flex',
                      alignItems: 'center',
                      gap: 6
                    }}
                    title="Clear all selections"
                  >
                    <FiX /> Clear ({selectedKeys.size})
                  </button>

                  <button
                    onClick={deleteSelectedResolvers}
                    style={{
                      padding: '8px 12px',
                      borderRadius: 8,
                      border: '1px solid var(--color-brand-red)',
                      background: 'rgba(239, 68, 68, 0.08)',
                      color: 'var(--color-brand-red)',
                      fontSize: 12,
                      cursor: 'pointer',
                      display: 'flex',
                      alignItems: 'center',
                      gap: 6
                    }}
                    title="Delete all selected resolvers"
                  >
                    <FiTrash2 /> Delete ({selectedKeys.size})
                  </button>
                </>
              )}

              <button
                onClick={copyHealthyToClipboard}
                style={{
                  padding: '8px 12px',
                  borderRadius: 8,
                  border: '1px solid var(--color-brand-border)',
                  background: 'var(--color-brand-bg)',
                  color: 'var(--color-brand-heading)',
                  fontSize: 12,
                  cursor: 'pointer',
                  display: 'flex',
                  alignItems: 'center',
                  gap: 6
                }}
                title={selectedKeys.size > 0 ? "Copy healthy selected resolver IPs to clipboard" : "Copy all healthy resolver IPs in list to clipboard"}
              >
                <FiCopy /> {selectedKeys.size > 0 ? "Copy Healthy (Selected)" : "Copy Healthy (All)"}
              </button>

              <select
                onChange={(e) => {
                  if (e.target.value === 'json' || e.target.value === 'csv') {
                    exportResolvers(e.target.value as 'json' | 'csv');
                    e.target.value = ''; // reset selection dropdown
                  }
                }}
                style={{
                  padding: '8px 12px',
                  borderRadius: 8,
                  border: '1px solid var(--color-brand-border)',
                  background: 'var(--color-brand-bg)',
                  color: 'var(--color-brand-heading)',
                  fontSize: 12,
                  cursor: 'pointer',
                  outline: 'none'
                }}
              >
                <option value="">{selectedKeys.size > 0 ? `Export Selected (${selectedKeys.size})` : 'Export All'}</option>
                <option value="json">JSON Format</option>
                <option value="csv">CSV Format</option>
              </select>
            </div>

            <button
              onClick={() => {
                setFetchPublicResult(null);
                setShowFetchPublicModal(true);
              }}
              style={{
                padding: '8px 16px',
                borderRadius: 8,
                fontSize: 12,
                fontWeight: 700,
                display: 'flex',
                alignItems: 'center',
                gap: 6,
                cursor: 'pointer',
                background: 'rgba(59, 130, 246, 0.1)',
                border: '1px solid rgba(59, 130, 246, 0.3)',
                color: 'var(--color-brand)'
              }}
            >
              <FiGlobe /> Load Public DNS
            </button>

            <button
              onClick={() => setShowAddModal(true)}
              className="btn btn-primary animate-fade-in"
              style={{
                padding: '8px 16px',
                borderRadius: 8,
                fontSize: 12,
                fontWeight: 700,
                display: 'flex',
                alignItems: 'center',
                gap: 6,
                cursor: 'pointer'
              }}
            >
              <FiPlus /> Add Resolver
            </button>
          </div>
        </div>

        {/* Sweep settings (Accordion) */}
        {showSettings && (
          <div className="animate-fade-in" style={{
            background: 'var(--color-brand-bg)',
            border: '1px solid var(--color-brand-border)',
            borderRadius: 8,
            padding: 16,
            display: 'grid',
            gridTemplateColumns: 'repeat(auto-fit, minmax(200px, 1fr))',
            gap: 16,
            marginTop: 10
          }}>
            <div style={{ display: 'flex', flexDirection: 'column', gap: 4 }}>
              <label style={{ fontSize: 10, fontWeight: 700, color: 'var(--color-brand-text)' }}>CONCURRENCY LIMIT</label>
              <input 
                type="number" 
                value={concurrencyLimit} 
                onChange={(e) => setConcurrencyLimit(Math.max(1, parseInt(e.target.value) || 1))}
                style={{ padding: '6px 10px', borderRadius: 6, border: '1px solid var(--color-brand-border)', background: 'var(--color-brand-card)', color: 'var(--color-brand-heading)', fontSize: 12 }}
              />
            </div>
            <div style={{ display: 'flex', flexDirection: 'column', gap: 4 }}>
              <label style={{ fontSize: 10, fontWeight: 700, color: 'var(--color-brand-text)' }}>TIMEOUT (MS)</label>
              <input 
                type="number" 
                value={timeoutMs} 
                onChange={(e) => setTimeoutMs(Math.max(100, parseInt(e.target.value) || 100))}
                style={{ padding: '6px 10px', borderRadius: 6, border: '1px solid var(--color-brand-border)', background: 'var(--color-brand-card)', color: 'var(--color-brand-heading)', fontSize: 12 }}
              />
            </div>
            <div style={{ display: 'flex', flexDirection: 'column', gap: 4 }}>
              <label style={{ fontSize: 10, fontWeight: 700, color: 'var(--color-brand-text)' }}>TEST ATTEMPTS</label>
              <input 
                type="number" 
                value={attempts} 
                onChange={(e) => setAttempts(Math.max(1, parseInt(e.target.value) || 1))}
                style={{ padding: '6px 10px', borderRadius: 6, border: '1px solid var(--color-brand-border)', background: 'var(--color-brand-card)', color: 'var(--color-brand-heading)', fontSize: 12 }}
              />
            </div>
            <div style={{ display: 'flex', flexDirection: 'column', gap: 4 }}>
              <label style={{ fontSize: 10, fontWeight: 700, color: 'var(--color-brand-text)' }}>REFERENCE DOMAIN</label>
              <input 
                type="text" 
                value={referenceDomain} 
                onChange={(e) => setReferenceDomain(e.target.value.trim() || 'google.com')}
                style={{ padding: '6px 10px', borderRadius: 6, border: '1px solid var(--color-brand-border)', background: 'var(--color-brand-card)', color: 'var(--color-brand-heading)', fontSize: 12 }}
              />
            </div>

            <div style={{ display: 'flex', flexDirection: 'column', gap: 4 }}>
              <label style={{ fontSize: 10, fontWeight: 700, color: 'var(--color-brand-text)' }}>DNS CLASS</label>
              <select
                value={dnsClass}
                onChange={(e) => setDnsClass(e.target.value)}
                style={{ padding: '6px 10px', borderRadius: 6, border: '1px solid var(--color-brand-border)', background: 'var(--color-brand-card)', color: 'var(--color-brand-heading)', fontSize: 12 }}
              >
                <option value="IN">Internet (IN)</option>
                <option value="CH">Chaos (CH)</option>
                <option value="ANY">Any (ANY)</option>
              </select>
            </div>

            <div style={{ display: 'flex', flexDirection: 'column', gap: 4 }}>
              <label style={{ fontSize: 10, fontWeight: 700, color: 'var(--color-brand-text)' }}>QUERY STRATEGY</label>
              <select
                value={queryGenerator}
                onChange={(e) => setQueryGenerator(e.target.value)}
                style={{ padding: '6px 10px', borderRadius: 6, border: '1px solid var(--color-brand-border)', background: 'var(--color-brand-card)', color: 'var(--color-brand-heading)', fontSize: 12 }}
              >
                <option value="random">Random Subdomain</option>
                <option value="sequential">Sequential Sequence</option>
                <option value="static">Static Reference Domain</option>
              </select>
            </div>

            <div style={{ display: 'flex', flexDirection: 'column', gap: 4 }}>
              <label style={{ fontSize: 10, fontWeight: 700, color: 'var(--color-brand-text)' }}>DOMAIN WORDLIST SOURCE</label>
              <select
                value={domainSource}
                onChange={(e) => setDomainSource(e.target.value)}
                style={{ padding: '6px 10px', borderRadius: 6, border: '1px solid var(--color-brand-border)', background: 'var(--color-brand-card)', color: 'var(--color-brand-heading)', fontSize: 12 }}
              >
                <option value="default">Default Reference Domain</option>
                <option value="custom">Custom Domains List</option>
                <option value="url">Remote Wordlist URL</option>
              </select>
            </div>

            {domainSource === 'custom' && (
              <div style={{ gridColumn: '1 / -1', display: 'flex', flexDirection: 'column', gap: 4, borderTop: '1px solid var(--color-brand-border)', paddingTop: 14 }}>
                <label style={{ fontSize: 10, fontWeight: 700, color: 'var(--color-brand-text)' }}>CUSTOM TARGET DOMAINS (ONE PER LINE OR COMMA-SEPARATED)</label>
                <textarea
                  value={customDomains}
                  onChange={(e) => setCustomDomains(e.target.value)}
                  placeholder="e.g.&#10;google.com&#10;apple.com&#10;github.com"
                  rows={3}
                  style={{ padding: '8px 12px', borderRadius: 6, border: '1px solid var(--color-brand-border)', background: 'var(--color-brand-card)', color: 'var(--color-brand-heading)', fontSize: 12, fontFamily: 'monospace', resize: 'vertical' }}
                />
              </div>
            )}

            {domainSource === 'url' && (
              <div style={{ gridColumn: '1 / -1', display: 'flex', flexDirection: 'column', gap: 4, borderTop: '1px solid var(--color-brand-border)', paddingTop: 14 }}>
                <label style={{ fontSize: 10, fontWeight: 700, color: 'var(--color-brand-text)' }}>REMOTE WORDLIST URL</label>
                <input
                  type="url"
                  value={wordlistURL}
                  onChange={(e) => setWordlistURL(e.target.value)}
                  placeholder="https://example.com/domains.txt"
                  style={{ padding: '8px 12px', borderRadius: 6, border: '1px solid var(--color-brand-border)', background: 'var(--color-brand-card)', color: 'var(--color-brand-heading)', fontSize: 12 }}
                />
              </div>
            )}

            <div style={{ gridColumn: '1 / -1', display: 'flex', flexDirection: 'column', gap: 8, borderTop: '1px solid var(--color-brand-border)', paddingTop: 14 }}>
              <span style={{ fontSize: 10, fontWeight: 700, color: 'var(--color-brand-text)' }}>QUERY RECORD TYPES</span>
              <div style={{ display: 'flex', flexWrap: 'wrap', gap: 8 }}>
                {['A', 'AAAA', 'MX', 'TXT', 'CNAME', 'NS', 'SOA', 'PTR', 'HTTPS'].map(t => {
                  const active = queryTypes.includes(t);
                  return (
                    <button
                      key={t}
                      type="button"
                      onClick={() => {
                        if (active) {
                          if (queryTypes.length > 1) {
                            setQueryTypes(queryTypes.filter(q => q !== t));
                          }
                        } else {
                          setQueryTypes([...queryTypes, t]);
                        }
                      }}
                      style={{
                        padding: '4px 10px',
                        borderRadius: 20,
                        border: '1px solid ' + (active ? 'var(--color-brand)' : 'var(--color-brand-border)'),
                        background: active ? 'var(--color-brand)' : 'var(--color-brand-card)',
                        color: active ? '#fff' : 'var(--color-brand-heading)',
                        fontSize: 10,
                        fontWeight: 700,
                        cursor: 'pointer',
                        transition: 'all 0.15s ease'
                      }}
                    >
                      {t}
                    </button>
                  );
                })}
              </div>
            </div>

            <div style={{ gridColumn: '1 / -1', display: 'flex', flexWrap: 'wrap', gap: 14, borderTop: '1px solid var(--color-brand-border)', paddingTop: 14 }}>
              <div style={{ display: 'flex', alignItems: 'center', gap: 8 }}>
                <input 
                  type="checkbox" 
                  id="cacheBust" 
                  checked={cacheBusting} 
                  onChange={(e) => setCacheBusting(e.target.checked)}
                  style={{ accentColor: 'var(--color-brand)', cursor: 'pointer' }}
                />
                <label htmlFor="cacheBust" style={{ fontSize: 11, fontWeight: 600, color: 'var(--color-brand-heading)', cursor: 'pointer' }}>Cache Busting (Entropy queries)</label>
              </div>

              <div style={{ display: 'flex', gap: 12, borderLeft: '1px solid var(--color-brand-border)', paddingLeft: 14 }}>
                <span style={{ fontSize: 11, fontWeight: 700, color: 'var(--color-brand-text)' }}>PROTOCOLS:</span>
                {['udp', 'tcp', 'dot', 'doh', 'doq'].map(p => (
                  <div key={p} style={{ display: 'flex', alignItems: 'center', gap: 4 }}>
                    <input 
                      type="checkbox" 
                      id={`p-${p}`}
                      checked={selectedProtocols.includes(p)}
                      onChange={() => handleProtocolCheckbox(p)}
                      style={{ accentColor: 'var(--color-brand)', cursor: 'pointer' }}
                    />
                    <label htmlFor={`p-${p}`} style={{ fontSize: 10, fontWeight: 700, textTransform: 'uppercase', color: 'var(--color-brand-heading)', cursor: 'pointer' }}>{p}</label>
                  </div>
                ))}
              </div>
            </div>

            <div style={{ gridColumn: '1 / -1', display: 'flex', flexDirection: 'column', gap: 4, borderTop: '1px solid var(--color-brand-border)', paddingTop: 14 }}>
              <label style={{ fontSize: 10, fontWeight: 700, color: 'var(--color-brand-text)' }}>EXPECT ANSWER PATTERN MATCH (OPTIONAL)</label>
              <input 
                type="text" 
                placeholder="e.g. 1.1.1.1, AS15169, google.com"
                value={expectResponse} 
                onChange={(e) => setExpectResponse(e.target.value)}
                style={{ padding: '8px 12px', borderRadius: 6, border: '1px solid var(--color-brand-border)', background: 'var(--color-brand-card)', color: 'var(--color-brand-heading)', fontSize: 12 }}
              />
            </div>
          </div>
        )}

        {/* Advanced Filters Panel */}
        {showAdvancedFilters && (
          <div className="animate-fade-in" style={{
            background: 'var(--color-brand-bg)',
            border: '1px solid var(--color-brand-border)',
            borderRadius: 8,
            padding: 16,
            display: 'flex',
            flexDirection: 'column',
            gap: 14,
            marginTop: 10
          }}>
            <div style={{ display: 'flex', justifyContent: 'space-between', alignItems: 'center', borderBottom: '1px solid var(--color-brand-border)', paddingBottom: 8 }}>
              <span style={{ fontSize: 13, fontWeight: 700, color: 'var(--color-brand-heading)', display: 'flex', alignItems: 'center', gap: 6 }}>
                <FiFilter style={{ color: 'var(--color-brand)' }} /> Filter Results By Column
              </span>
              <button 
                onClick={resetAdvancedFilters}
                style={{
                  padding: '4px 10px',
                  borderRadius: 6,
                  border: '1px solid var(--color-brand-border)',
                  background: 'var(--color-brand-bg)',
                  color: 'var(--color-brand-text)',
                  fontSize: 11,
                  cursor: 'pointer',
                  display: 'flex',
                  alignItems: 'center',
                  gap: 4
                }}
              >
                <FiX size={12} /> Reset Filters
              </button>
            </div>

            <div style={{
              display: 'grid',
              gridTemplateColumns: 'repeat(auto-fit, minmax(220px, 1fr))',
              gap: 14
            }}>
              {/* Latency Min/Max */}
              <div style={{ display: 'flex', flexDirection: 'column', gap: 4 }}>
                <label style={{ fontSize: 10, fontWeight: 700, color: 'var(--color-brand-text)' }}>LATENCY RANGE (MS)</label>
                <div style={{ display: 'flex', alignItems: 'center', gap: 8 }}>
                  <input 
                    type="number" 
                    placeholder="Min" 
                    value={filterMinLatency} 
                    onChange={(e) => setFilterMinLatency(e.target.value)}
                    style={{ width: '100%', padding: '6px 10px', borderRadius: 6, border: '1px solid var(--color-brand-border)', background: 'var(--color-brand-card)', color: 'var(--color-brand-heading)', fontSize: 12 }}
                  />
                  <span style={{ color: 'var(--color-brand-text)', fontSize: 11 }}>to</span>
                  <input 
                    type="number" 
                    placeholder="Max" 
                    value={filterMaxLatency} 
                    onChange={(e) => setFilterMaxLatency(e.target.value)}
                    style={{ width: '100%', padding: '6px 10px', borderRadius: 6, border: '1px solid var(--color-brand-border)', background: 'var(--color-brand-card)', color: 'var(--color-brand-heading)', fontSize: 12 }}
                  />
                </div>
              </div>

              {/* Success Rate Min */}
              <div style={{ display: 'flex', flexDirection: 'column', gap: 4 }}>
                <label style={{ fontSize: 10, fontWeight: 700, color: 'var(--color-brand-text)' }}>MIN SUCCESS RATE (%)</label>
                <input 
                  type="number" 
                  min="0"
                  max="100"
                  placeholder="e.g. 50" 
                  value={filterMinSuccessRate} 
                  onChange={(e) => setFilterMinSuccessRate(e.target.value)}
                  style={{ padding: '6px 10px', borderRadius: 6, border: '1px solid var(--color-brand-border)', background: 'var(--color-brand-card)', color: 'var(--color-brand-heading)', fontSize: 12 }}
                />
              </div>

              {/* Censorship */}
              <div style={{ display: 'flex', flexDirection: 'column', gap: 4 }}>
                <label style={{ fontSize: 10, fontWeight: 700, color: 'var(--color-brand-text)' }}>CENSORSHIP STATUS</label>
                <select 
                  value={filterCensorship} 
                  onChange={(e) => setFilterCensorship(e.target.value)}
                  style={{ padding: '6px 10px', borderRadius: 6, border: '1px solid var(--color-brand-border)', background: 'var(--color-brand-card)', color: 'var(--color-brand-heading)', fontSize: 12, cursor: 'pointer' }}
                >
                  <option value="ALL">All Statuses</option>
                  <option value="clean">Clean (Unfiltered)</option>
                  <option value="manipulated">Manipulated (DNS Spoofed)</option>
                  <option value="hijacked">Hijacked (NXDOMAIN Redirection)</option>
                  <option value="sinkhole">Sinkhole (Telemetry Blocked)</option>
                  <option value="unverified">Unverified</option>
                </select>
              </div>

              {/* DNSSEC */}
              <div style={{ display: 'flex', flexDirection: 'column', gap: 4 }}>
                <label style={{ fontSize: 10, fontWeight: 700, color: 'var(--color-brand-text)' }}>DNSSEC VALIDATION</label>
                <select 
                  value={filterDNSSEC} 
                  onChange={(e) => setFilterDNSSEC(e.target.value)}
                  style={{ padding: '6px 10px', borderRadius: 6, border: '1px solid var(--color-brand-border)', background: 'var(--color-brand-card)', color: 'var(--color-brand-heading)', fontSize: 12, cursor: 'pointer' }}
                >
                  <option value="ALL">All Profiles</option>
                  <option value="valid">Valid (DNSSEC verified)</option>
                  <option value="stripped">Stripped/Invalid</option>
                </select>
              </div>

              {/* Rebinding */}
              <div style={{ display: 'flex', flexDirection: 'column', gap: 4 }}>
                <label style={{ fontSize: 10, fontWeight: 700, color: 'var(--color-brand-text)' }}>REBINDING SECURITY</label>
                <select 
                  value={filterRebinding} 
                  onChange={(e) => setFilterRebinding(e.target.value)}
                  style={{ padding: '6px 10px', borderRadius: 6, border: '1px solid var(--color-brand-border)', background: 'var(--color-brand-card)', color: 'var(--color-brand-heading)', fontSize: 12, cursor: 'pointer' }}
                >
                  <option value="ALL">All Profiles</option>
                  <option value="secure">Secure (No rebinding vuln)</option>
                  <option value="vulnerable">Vulnerable to Rebinding</option>
                </select>
              </div>

              {/* ISP */}
              <div style={{ display: 'flex', flexDirection: 'column', gap: 4 }}>
                <label style={{ fontSize: 10, fontWeight: 700, color: 'var(--color-brand-text)' }}>ISP / ASN</label>
                <input 
                  type="text" 
                  placeholder="e.g. Telecommunication, AS12880" 
                  value={filterISP} 
                  onChange={(e) => setFilterISP(e.target.value)}
                  style={{ padding: '6px 10px', borderRadius: 6, border: '1px solid var(--color-brand-border)', background: 'var(--color-brand-card)', color: 'var(--color-brand-heading)', fontSize: 12 }}
                />
              </div>

              {/* Country */}
              <div style={{ display: 'flex', flexDirection: 'column', gap: 4 }}>
                <label style={{ fontSize: 10, fontWeight: 700, color: 'var(--color-brand-text)' }}>GEOGRAPHY (COUNTRY/CITY)</label>
                <input 
                  type="text" 
                  placeholder="e.g. Germany, Tehran, IR" 
                  value={filterCountry} 
                  onChange={(e) => setFilterCountry(e.target.value)}
                  style={{ padding: '6px 10px', borderRadius: 6, border: '1px solid var(--color-brand-border)', background: 'var(--color-brand-card)', color: 'var(--color-brand-heading)', fontSize: 12 }}
                />
              </div>

              {/* CDN */}
              <div style={{ display: 'flex', flexDirection: 'column', gap: 4 }}>
                <label style={{ fontSize: 10, fontWeight: 700, color: 'var(--color-brand-text)' }}>CDN PROVIDER</label>
                <select 
                  value={filterCDN} 
                  onChange={(e) => setFilterCDN(e.target.value)}
                  style={{ padding: '6px 10px', borderRadius: 6, border: '1px solid var(--color-brand-border)', background: 'var(--color-brand-card)', color: 'var(--color-brand-heading)', fontSize: 12, cursor: 'pointer' }}
                >
                  <option value="ALL">All Resolvers</option>
                  <option value="cdn">CDN Resolvers Only</option>
                  <option value="non-cdn">Non-CDN Only</option>
                </select>
              </div>
            </div>
          </div>
        )}

        {/* Virtualized Table */}
        <div ref={parentRef} className="table-container" style={{ maxHeight: 560, overflowY: 'auto', border: '1px solid var(--color-brand-border)', borderRadius: 10 }}>
          <table style={{ width: '100%', borderCollapse: 'collapse', fontSize: 12, textAlign: 'left' }}>
            <thead style={{ background: 'var(--color-brand-bg)', position: 'sticky', top: 0, zIndex: 5, borderBottom: '1px solid var(--color-brand-border)' }}>
              <tr>
                <th style={{ padding: '12px', textAlign: 'center', width: 40 }}>
                  <input 
                    type="checkbox" 
                    checked={isAllSelected}
                    onChange={handleToggleSelectAll}
                    style={{ accentColor: 'var(--color-brand)', cursor: 'pointer' }}
                  />
                </th>
                <th style={{ padding: '12px', color: 'var(--color-brand-heading)', fontWeight: 700 }}>RESOLVER IP</th>
                <th style={{ padding: '12px', color: 'var(--color-brand-heading)', fontWeight: 700 }}>PROVIDER</th>
                <th style={{ padding: '12px', color: 'var(--color-brand-heading)', fontWeight: 700 }}>PROTOCOL</th>
                <th onClick={() => toggleSort('latency_ms')} style={{ padding: '12px', textAlign: 'center', color: 'var(--color-brand-heading)', fontWeight: 700, cursor: 'pointer' }}>
                  LATENCY <SortIcon column="latency_ms" sortBy={sortBy} sortOrder={sortOrder} />
                </th>
                <th onClick={() => toggleSort('jitter_ms')} style={{ padding: '12px', textAlign: 'center', color: 'var(--color-brand-heading)', fontWeight: 700, cursor: 'pointer' }}>
                  JITTER <SortIcon column="jitter_ms" sortBy={sortBy} sortOrder={sortOrder} />
                </th>
                <th onClick={() => toggleSort('success_rate')} style={{ padding: '12px', textAlign: 'center', color: 'var(--color-brand-heading)', fontWeight: 700, cursor: 'pointer' }}>
                  HEALTH <SortIcon column="success_rate" sortBy={sortBy} sortOrder={sortOrder} />
                </th>
                <th style={{ padding: '12px', textAlign: 'center', color: 'var(--color-brand-heading)', fontWeight: 700 }}>CENSORSHIP</th>
                <th style={{ padding: '12px', textAlign: 'center', color: 'var(--color-brand-heading)', fontWeight: 700 }}>DNSSEC</th>
                <th style={{ padding: '12px', textAlign: 'center', color: 'var(--color-brand-heading)', fontWeight: 700 }}>REBINDING</th>
                <th onClick={() => toggleSort('clever_score')} style={{ padding: '12px', textAlign: 'center', color: 'var(--color-brand-heading)', fontWeight: 700, cursor: 'pointer' }}>
                  CLEVER SCORE <SortIcon column="clever_score" sortBy={sortBy} sortOrder={sortOrder} />
                </th>
                <th style={{ padding: '12px', textAlign: 'center', color: 'var(--color-brand-heading)', fontWeight: 700 }}>ACTIONS</th>
              </tr>
            </thead>
            <tbody>
              {filteredKeys.length === 0 ? (
                <tr>
                  <td colSpan={12} style={{ padding: 30, textAlign: 'center', color: 'var(--color-brand-muted)' }}>
                    No resolvers match the current search filters. Try launching a sweep.
                  </td>
                </tr>
              ) : (
                <>
                  {virtualizer.getVirtualItems()[0]?.start > 0 && (
                    <tr>
                      <td colSpan={12} style={{ height: virtualizer.getVirtualItems()[0].start }} />
                    </tr>
                  )}
                  {virtualizer.getVirtualItems().map((virtualRow) => {
                    const key = filteredKeys[virtualRow.index];
                    if (!key) return null;
                    const isApplied = resolvers[key] ? checkIsApplied(resolvers[key]) : false;
                    return (
                      <ResolverRow
                        key={key}
                        resolverKey={key}
                        style={{ height: virtualRow.size }}
                        isActiveSystem={isApplied}
                        onDeleteSingle={handleDeleteResolver}
                        onApplyResolver={handleApplyResolver}
                        isSelected={selectedKeys.has(key)}
                        onToggleSelect={handleToggleSelect}
                        onOpenTrace={(k) => {
                          setTraceResolverKey(k);
                          setTraceDomain(resolvers[k]?.domain || 'google.com');
                          setTraceSteps([]);
                          setShowTraceModal(true);
                        }}
                        onOpenAXFR={(k) => {
                          setAxfrResolverKey(k);
                          setAxfrDomain(resolvers[k]?.domain || 'google.com');
                          setAxfrResult(null);
                          setShowAXFRModal(true);
                        }}
                        onOpenAdvancedTest={(k) => {
                          setAdvancedTestResolverKey(k);
                          setShowAdvancedTestModal(true);
                        }}
                      />
                    );
                  })}
                  {virtualizer.getVirtualItems().length > 0 && (
                    <tr>
                      <td
                        colSpan={12}
                        style={{
                          height:
                            virtualizer.getTotalSize() -
                            virtualizer.getVirtualItems()[virtualizer.getVirtualItems().length - 1].end,
                        }}
                      />
                    </tr>
                  )}
                </>
              )}
            </tbody>
          </table>
        </div>
        <div style={{ display: 'flex', justifyContent: 'space-between', fontSize: 11, color: 'var(--color-brand-text)' }}>
          <span>Total matched resolver profiles: {filteredKeys.length}</span>
          <span>WebSocket status: Online</span>
        </div>
      </div>

      {/* FETCH PUBLIC DNS DIALOG */}
      {showFetchPublicModal && (
        <div
          style={{
            position: 'fixed',
            top: 0, left: 0, width: '100%', height: '100%',
            background: 'rgba(0,0,0,0.6)',
            backdropFilter: 'blur(4px)',
            display: 'flex', alignItems: 'center', justifyContent: 'center',
            zIndex: 999,
          }}
          onClick={() => {
            if (!isFetchingPublic) setShowFetchPublicModal(false);
          }}
        >
          <div
            style={{
              background: 'var(--color-brand-card)',
              padding: 28,
              borderRadius: 16,
              width: 500,
              maxWidth: '90%',
              boxShadow: '0 20px 25px -5px rgba(0, 0, 0, 0.3), 0 10px 10px -5px rgba(0, 0, 0, 0.2)',
              border: '1px solid var(--color-brand-border)',
              display: 'flex',
              flexDirection: 'column',
              gap: 16
            }}
            onClick={(e) => e.stopPropagation()}
          >
            <div style={{ display: 'flex', justifyContent: 'space-between', alignItems: 'center' }}>
              <h3 style={{ margin: 0, color: 'var(--color-brand-heading)', display: 'flex', alignItems: 'center', gap: 8 }}>
                <FiGlobe style={{ color: 'var(--color-brand)' }} /> Load Public DNS from Internet
              </h3>
              {!isFetchingPublic && (
                <button
                  onClick={() => setShowFetchPublicModal(false)}
                  style={{ background: 'none', border: 'none', cursor: 'pointer', color: 'var(--color-brand-text)' }}
                >
                  <FiX size={18} />
                </button>
              )}
            </div>

            <p style={{ fontSize: 13, color: 'var(--color-brand-text)', margin: '0 0 8px 0', lineHeight: '1.5' }}>
              Import thousands of active, verified public nameservers from remote repositories. Duplicate IP addresses and protocols will automatically be filtered out to keep your list clean.
            </p>

            {fetchPublicResult ? (
              <div style={{
                background: 'rgba(16, 185, 129, 0.08)',
                border: '1px solid rgba(16, 185, 129, 0.3)',
                borderRadius: 12,
                padding: 16,
                display: 'flex',
                flexDirection: 'column',
                gap: 8,
                textAlign: 'center'
              }}>
                <FiCheckCircle size={32} style={{ color: 'var(--color-brand-green)', margin: '0 auto 4px auto' }} />
                <h4 style={{ margin: 0, color: 'var(--color-brand-heading)' }}>Import Successful!</h4>
                <div style={{ fontSize: 13, color: 'var(--color-brand-text)' }}>
                  Successfully scanned and processed <strong>{fetchPublicResult.total_found}</strong> potential resolvers.
                </div>
                <div style={{ fontSize: 15, fontWeight: 'bold', color: 'var(--color-brand-green)' }}>
                  + {fetchPublicResult.added_count} New DNS Resolvers Added
                </div>
                <p style={{ fontSize: 11, color: 'var(--color-brand-text)', margin: '4px 0 0 0' }}>
                  All duplicate or existing resolvers were ignored.
                </p>
                <button
                  className="btn btn-primary"
                  onClick={() => setShowFetchPublicModal(false)}
                  style={{ marginTop: 12, alignSelf: 'center', padding: '6px 20px', borderRadius: 8 }}
                >
                  Done
                </button>
              </div>
            ) : (
              <>
                <div style={{ display: 'flex', flexDirection: 'column', gap: 12 }}>
                  <label style={{ fontSize: 12, fontWeight: 700, color: 'var(--color-brand-heading)' }}>
                    Select Resolver Data Source:
                  </label>

                  <div style={{ display: 'flex', flexDirection: 'column', gap: 8 }}>
                    {/* Curated list */}
                    <div
                      onClick={() => !isFetchingPublic && setSelectedPublicSource('curated')}
                      style={{
                        padding: 12,
                        borderRadius: 10,
                        border: `1px solid ${selectedPublicSource === 'curated' ? 'var(--color-brand)' : 'var(--color-brand-border)'}`,
                        background: selectedPublicSource === 'curated' ? 'rgba(59, 130, 246, 0.05)' : 'var(--color-brand-bg)',
                        cursor: isFetchingPublic ? 'not-allowed' : 'pointer',
                        transition: 'all 0.2s ease'
                      }}
                    >
                      <div style={{ display: 'flex', alignItems: 'center', gap: 8, marginBottom: 4 }}>
                        <input
                          type="radio"
                          checked={selectedPublicSource === 'curated'}
                          readOnly
                          disabled={isFetchingPublic}
                        />
                        <strong style={{ fontSize: 13, color: 'var(--color-brand-heading)' }}>Curated Anycast DNS (Highly Recommended)</strong>
                      </div>
                      <span style={{ fontSize: 11, color: 'var(--color-brand-text)', paddingLeft: 20, display: 'block' }}>
                        Google DNS, Cloudflare, Quad9, AdGuard, OpenDNS, CleanBrowsing, DNS.SB, ControlD, Level3, Comodo, etc. (High stability & low latency).
                      </span>
                    </div>

                    {/* BLS List */}
                    <div
                      onClick={() => !isFetchingPublic && setSelectedPublicSource('bls')}
                      style={{
                        padding: 12,
                        borderRadius: 10,
                        border: `1px solid ${selectedPublicSource === 'bls' ? 'var(--color-brand)' : 'var(--color-brand-border)'}`,
                        background: selectedPublicSource === 'bls' ? 'rgba(59, 130, 246, 0.05)' : 'var(--color-brand-bg)',
                        cursor: isFetchingPublic ? 'not-allowed' : 'pointer',
                        transition: 'all 0.2s ease'
                      }}
                    >
                      <div style={{ display: 'flex', alignItems: 'center', gap: 8, marginBottom: 4 }}>
                        <input
                          type="radio"
                          checked={selectedPublicSource === 'bls'}
                          readOnly
                          disabled={isFetchingPublic}
                        />
                        <strong style={{ fontSize: 13, color: 'var(--color-brand-heading)' }}>BLS Global Verified List (6,000+ DNS)</strong>
                      </div>
                      <span style={{ fontSize: 11, color: 'var(--color-brand-text)', paddingLeft: 20, display: 'block' }}>
                        Weekly verified public DNS server list compiled from various active nameservers worldwide.
                      </span>
                    </div>

                    {/* Trickest List */}
                    <div
                      onClick={() => !isFetchingPublic && setSelectedPublicSource('trickest')}
                      style={{
                        padding: 12,
                        borderRadius: 10,
                        border: `1px solid ${selectedPublicSource === 'trickest' ? 'var(--color-brand)' : 'var(--color-brand-border)'}`,
                        background: selectedPublicSource === 'trickest' ? 'rgba(59, 130, 246, 0.05)' : 'var(--color-brand-bg)',
                        cursor: isFetchingPublic ? 'not-allowed' : 'pointer',
                        transition: 'all 0.2s ease'
                      }}
                    >
                      <div style={{ display: 'flex', alignItems: 'center', gap: 8, marginBottom: 4 }}>
                        <input
                          type="radio"
                          checked={selectedPublicSource === 'trickest'}
                          readOnly
                          disabled={isFetchingPublic}
                        />
                        <strong style={{ fontSize: 13, color: 'var(--color-brand-heading)' }}>Trickest Verified Resolvers</strong>
                      </div>
                      <span style={{ fontSize: 11, color: 'var(--color-brand-text)', paddingLeft: 20, display: 'block' }}>
                        Exhaustive multi-source public DNS server compilation validated using active validation algorithms.
                      </span>
                    </div>
                  </div>
                </div>

                <div style={{ display: 'flex', gap: 10, justifyContent: 'flex-end', marginTop: 12 }}>
                  <button
                    onClick={() => setShowFetchPublicModal(false)}
                    disabled={isFetchingPublic}
                    style={{
                      padding: '8px 16px',
                      borderRadius: 8,
                      border: '1px solid var(--color-brand-border)',
                      background: 'var(--color-brand-bg)',
                      color: 'var(--color-brand-heading)',
                      cursor: isFetchingPublic ? 'not-allowed' : 'pointer'
                    }}
                  >
                    Cancel
                  </button>
                  <button
                    onClick={handleFetchPublicDNS}
                    disabled={isFetchingPublic}
                    className="btn btn-primary"
                    style={{
                      padding: '8px 20px',
                      borderRadius: 8,
                      display: 'flex',
                      alignItems: 'center',
                      gap: 8,
                      cursor: isFetchingPublic ? 'wait' : 'pointer'
                    }}
                  >
                    {isFetchingPublic ? (
                      <>
                        <FiRefreshCw className="animate-spin" /> Fetching and Importing...
                      </>
                    ) : (
                      <>
                        <FiDownload /> Fetch & Import
                      </>
                    )}
                  </button>
                </div>
              </>
            )}
          </div>
        </div>
      )}

      {/* ADD CUSTOM RESOLVER DIALOG */}
      {showAddModal && (
        <div
          style={{
            position: 'fixed',
            top: 0, left: 0, width: '100%', height: '100%',
            background: 'rgba(0,0,0,0.5)',
            display: 'flex', alignItems: 'center', justifyContent: 'center',
            zIndex: 999,
          }}
          onClick={() => setShowAddModal(false)}
        >
          <div
            style={{
              background: 'var(--color-brand-card)',
              padding: 24,
              borderRadius: 12,
              width: 460,
              maxWidth: '90%',
              boxShadow: '0 10px 25px rgba(0,0,0,0.15)',
              display: 'flex',
              flexDirection: 'column',
              gap: 14
            }}
            onClick={(e) => e.stopPropagation()}
          >
            <h3 style={{ fontSize: 16, fontWeight: 700, color: 'var(--color-brand-heading)', margin: 0 }}>
              Add DNS Resolvers
            </h3>

            <div style={{ display: 'flex', borderBottom: '1px solid var(--color-brand-border)', marginBottom: 4 }}>
              <button
                onClick={() => setModalTab('single')}
                style={{
                  flex: 1,
                  padding: '8px 16px',
                  background: 'none',
                  border: 'none',
                  borderBottom: modalTab === 'single' ? '2px solid var(--color-brand)' : '2px solid transparent',
                  color: modalTab === 'single' ? 'var(--color-brand-heading)' : 'var(--color-brand-text)',
                  fontWeight: 700,
                  fontSize: 12,
                  cursor: 'pointer',
                  transition: 'all 0.2s ease'
                }}
              >
                Single DNS
              </button>
              <button
                onClick={() => setModalTab('bulk')}
                style={{
                  flex: 1,
                  padding: '8px 16px',
                  background: 'none',
                  border: 'none',
                  borderBottom: modalTab === 'bulk' ? '2px solid var(--color-brand)' : '2px solid transparent',
                  color: modalTab === 'bulk' ? 'var(--color-brand-heading)' : 'var(--color-brand-text)',
                  fontWeight: 700,
                  fontSize: 12,
                  cursor: 'pointer',
                  transition: 'all 0.2s ease'
                }}
              >
                Bulk Import
              </button>
            </div>

            {modalTab === 'single' ? (
              <>
                <div style={{ display: 'flex', flexDirection: 'column', gap: 4 }}>
                  <label style={{ fontSize: 10, fontWeight: 700, color: 'var(--color-brand-text)' }}>RESOLVER IP/URL</label>
                  <input 
                    type="text" 
                    placeholder="e.g. 1.1.1.1 or 8.8.8.8"
                    value={customIP} 
                    onChange={(e) => setCustomIP(e.target.value)}
                    style={{ padding: '8px 12px', borderRadius: 8, border: '1px solid var(--color-brand-border)', background: 'var(--color-brand-bg)', color: 'var(--color-brand-heading)', fontSize: 12 }}
                  />
                </div>

                <div style={{ display: 'flex', flexDirection: 'column', gap: 4 }}>
                  <label style={{ fontSize: 10, fontWeight: 700, color: 'var(--color-brand-text)' }}>PROVIDER NAME</label>
                  <input 
                    type="text" 
                    placeholder="e.g. Cloudflare, Google"
                    value={customProvider} 
                    onChange={(e) => setCustomProvider(e.target.value)}
                    style={{ padding: '8px 12px', borderRadius: 8, border: '1px solid var(--color-brand-border)', background: 'var(--color-brand-bg)', color: 'var(--color-brand-heading)', fontSize: 12 }}
                  />
                </div>

                <div style={{ display: 'grid', gridTemplateColumns: '1fr 1fr', gap: 12 }}>
                  <div style={{ display: 'flex', flexDirection: 'column', gap: 4 }}>
                    <label style={{ fontSize: 10, fontWeight: 700, color: 'var(--color-brand-text)' }}>PROTOCOL</label>
                    <select
                      value={customProtocol}
                      onChange={(e) => setCustomProtocol(e.target.value)}
                      style={{ padding: '8px 12px', borderRadius: 8, border: '1px solid var(--color-brand-border)', background: 'var(--color-brand-bg)', color: 'var(--color-brand-heading)', fontSize: 12 }}
                    >
                      <option value="udp">Plain UDP</option>
                      <option value="tcp">Plain TCP</option>
                      <option value="dot">DoT (TLS)</option>
                      <option value="doh">DoH (HTTPS)</option>
                      <option value="doq">DoQ (QUIC)</option>
                    </select>
                  </div>

                  <div style={{ display: 'flex', flexDirection: 'column', gap: 4 }}>
                    <label style={{ fontSize: 10, fontWeight: 700, color: 'var(--color-brand-text)' }}>CATEGORY</label>
                    <select
                      value={customCategory}
                      onChange={(e) => setCustomCategory(e.target.value)}
                      style={{ padding: '8px 12px', borderRadius: 8, border: '1px solid var(--color-brand-border)', background: 'var(--color-brand-bg)', color: 'var(--color-brand-heading)', fontSize: 12 }}
                    >
                      <option value="custom">Custom (Standard)</option>
                      <option value="public">Public Core</option>
                      <option value="security">Ad/Security filtering</option>
                    </select>
                  </div>
                </div>

                <div style={{ display: 'flex', gap: 10, marginTop: 10 }}>
                  <button 
                    onClick={() => setShowAddModal(false)}
                    className="btn" 
                    style={{ flex: 1, padding: '10px', borderRadius: 8, border: '1px solid var(--color-brand-border)', background: 'var(--color-brand-card)', color: 'var(--color-brand-heading)', cursor: 'pointer', fontSize: 12, fontWeight: 700 }}
                  >
                    Cancel
                  </button>
                  <button 
                    onClick={handleAddCustomResolver}
                    className="btn btn-primary" 
                    style={{ flex: 1, padding: '10px', borderRadius: 8, cursor: 'pointer', fontSize: 12, fontWeight: 700 }}
                  >
                    Save Resolver
                  </button>
                </div>
              </>
            ) : (
              <>
                <div style={{ fontSize: 11, color: 'var(--color-brand-text)', lineHeight: 1.4, background: 'var(--color-brand-bg)', padding: '8px 12px', borderRadius: 8, border: '1px solid var(--color-brand-border)' }}>
                  <strong>Format:</strong> One per line (e.g. <code>8.8.8.8, Google, udp</code> or <code>1.1.1.1</code>). If protocol is omitted, we will automatically probe and detect it. Duplicate IPs will be skipped.
                </div>

                <div style={{ display: 'flex', flexDirection: 'column', gap: 4 }}>
                  <label style={{ fontSize: 10, fontWeight: 700, color: 'var(--color-brand-text)' }}>DNS LIST (PASTE TEXT)</label>
                  <textarea
                    placeholder="e.g.&#10;8.8.8.8, Google DNS, udp&#10;1.1.1.1, Cloudflare&#10;https://dns.google/dns-query"
                    value={bulkText}
                    onChange={(e) => {
                      setBulkText(e.target.value);
                      setBulkFile(null);
                    }}
                    rows={5}
                    style={{ padding: '8px 12px', borderRadius: 8, border: '1px solid var(--color-brand-border)', background: 'var(--color-brand-bg)', color: 'var(--color-brand-heading)', fontSize: 12, fontFamily: 'monospace', resize: 'vertical' }}
                  />
                </div>

                <div style={{ display: 'flex', flexDirection: 'column', gap: 4 }}>
                  <label style={{ fontSize: 10, fontWeight: 700, color: 'var(--color-brand-text)' }}>OR UPLOAD FILE (.TXT / .CSV)</label>
                  <input
                    type="file"
                    accept=".txt,.csv"
                    onChange={(e) => {
                      if (e.target.files && e.target.files[0]) {
                        setBulkFile(e.target.files[0]);
                        setBulkText('');
                      }
                    }}
                    style={{ padding: '6px 10px', borderRadius: 8, border: '1px solid var(--color-brand-border)', background: 'var(--color-brand-bg)', color: 'var(--color-brand-heading)', fontSize: 12 }}
                  />
                  {bulkFile && (
                    <span style={{ fontSize: 11, color: 'var(--color-brand-green)', fontWeight: 600 }}>
                      Selected: {bulkFile.name} ({(bulkFile.size / 1024).toFixed(1)} KB)
                    </span>
                  )}
                </div>

                <div style={{ display: 'flex', gap: 10, marginTop: 10 }}>
                  <button 
                    onClick={() => setShowAddModal(false)}
                    className="btn" 
                    style={{ flex: 1, padding: '10px', borderRadius: 8, border: '1px solid var(--color-brand-border)', background: 'var(--color-brand-card)', color: 'var(--color-brand-heading)', cursor: 'pointer', fontSize: 12, fontWeight: 700 }}
                  >
                    Cancel
                  </button>
                  <button 
                    onClick={handleBulkImport}
                    disabled={isImporting}
                    className="btn btn-primary" 
                    style={{ flex: 1, padding: '10px', borderRadius: 8, cursor: 'pointer', fontSize: 12, fontWeight: 700, opacity: isImporting ? 0.7 : 1 }}
                  >
                    {isImporting ? "Submitting..." : "Import Resolvers"}
                  </button>
                </div>
              </>
            )}
          </div>
        </div>
      )}

      {/* DNS PATH DELEGATION TRACE DIALOG */}
      {showTraceModal && traceResolverKey && (
        <div
          style={{
            position: 'fixed',
            top: 0, left: 0, width: '100%', height: '100%',
            background: 'rgba(0,0,0,0.6)',
            display: 'flex', alignItems: 'center', justifyContent: 'center',
            zIndex: 999,
          }}
          onClick={() => setShowTraceModal(false)}
        >
          <div
            style={{
              background: 'var(--color-brand-card)',
              padding: 24,
              borderRadius: 12,
              width: 580,
              maxWidth: '95%',
              maxHeight: '85vh',
              overflowY: 'auto',
              boxShadow: '0 10px 25px rgba(0,0,0,0.15)',
              display: 'flex',
              flexDirection: 'column',
              gap: 16
            }}
            onClick={(e) => e.stopPropagation()}
          >
            <div style={{ display: 'flex', justifyContent: 'space-between', alignItems: 'center' }}>
              <h3 style={{ fontSize: 16, fontWeight: 700, color: 'var(--color-brand-heading)', margin: 0 }}>
                Iterative DNS Path Trace Diagnostics
              </h3>
              <span style={{ fontSize: 10, padding: '2px 6px', borderRadius: 4, background: 'var(--color-brand-bg)', color: 'var(--color-brand-text)' }}>
                Target: {resolvers[traceResolverKey]?.ip}
              </span>
            </div>

            <div style={{ display: 'flex', gap: 10, alignItems: 'flex-end' }}>
              <div style={{ flex: 1, display: 'flex', flexDirection: 'column', gap: 4 }}>
                <label style={{ fontSize: 10, fontWeight: 700, color: 'var(--color-brand-text)' }}>DOMAIN TO TRACE</label>
                <input 
                  type="text" 
                  value={traceDomain} 
                  onChange={(e) => setTraceDomain(e.target.value.trim())}
                  placeholder="e.g. google.com"
                  style={{ padding: '8px 12px', borderRadius: 8, border: '1px solid var(--color-brand-border)', background: 'var(--color-brand-bg)', color: 'var(--color-brand-heading)', fontSize: 12 }}
                />
              </div>
              <button
                onClick={() => handleRunTrace(resolvers[traceResolverKey]?.ip, traceDomain)}
                disabled={isTracing || !traceDomain}
                className="btn btn-primary"
                style={{ padding: '10px 16px', borderRadius: 8, fontSize: 12, fontWeight: 700, cursor: 'pointer', height: 36, display: 'flex', alignItems: 'center', gap: 6 }}
              >
                {isTracing ? <FiRefreshCw className="animate-spin" /> : <FiPlay />}
                {isTracing ? "Tracing..." : "Run Trace"}
              </button>
            </div>

            {/* Trace results */}
            <div style={{ border: '1px solid var(--color-brand-border)', borderRadius: 8, background: 'var(--color-brand-bg)', padding: 12, minHeight: 200, maxHeight: 350, overflowY: 'auto', display: 'flex', flexDirection: 'column', gap: 12 }}>
              {traceSteps.length === 0 ? (
                <div style={{ display: 'flex', flexDirection: 'column', alignItems: 'center', justifyContent: 'center', height: '100%', minHeight: 180, color: 'var(--color-brand-muted)', gap: 8, textAlign: 'center' }}>
                  <FiActivity size={24} />
                  <span style={{ fontSize: 12 }}>
                    {isTracing ? "Querying nameservers iteratively starting from Root..." : "Click Run Trace to query recursive paths from root DNS authorities down."}
                  </span>
                </div>
              ) : (
                <div style={{ display: 'flex', flexDirection: 'column', gap: 14 }}>
                  {traceSteps.map((step, idx) => (
                    <div key={idx} style={{ display: 'flex', gap: 12, alignItems: 'flex-start', position: 'relative' }}>
                      {/* Connection Line */}
                      {idx < traceSteps.length - 1 && (
                        <div style={{ position: 'absolute', left: 14, top: 28, bottom: -22, width: 2, background: 'var(--color-brand-border)' }} />
                      )}
                      
                      {/* Hop Circle */}
                      <div style={{ 
                        width: 30, height: 30, borderRadius: '50%', 
                        background: 'var(--color-brand)', color: '#fff', 
                        display: 'flex', alignItems: 'center', justifyContent: 'center', 
                        fontSize: 12, fontWeight: 700, flexShrink: 0 
                      }}>
                        {step.hop}
                      </div>

                      {/* Content Card */}
                      <div style={{ 
                        flex: 1, padding: '10px 14px', borderRadius: 8, 
                        background: 'var(--color-brand-card)', border: '1px solid var(--color-brand-border)'
                      }}>
                        <div style={{ display: 'flex', justifyContent: 'space-between', flexWrap: 'wrap', gap: 8 }}>
                          <span style={{ fontWeight: 700, color: 'var(--color-brand-heading)', fontSize: 12 }}>{step.server_name || step.name}</span>
                          <span style={{ 
                            fontSize: 10, padding: '2px 6px', borderRadius: 4, 
                            background: step.rcode === 'NOERROR' ? 'rgba(34, 197, 94, 0.1)' : 'rgba(239, 68, 68, 0.1)', 
                            color: step.rcode === 'NOERROR' ? '#16a34a' : '#dc2626', 
                            fontWeight: 700 
                          }}>
                            {step.rcode}
                          </span>
                        </div>
                        <div style={{ display: 'flex', gap: 14, fontSize: 11, color: 'var(--color-brand-text)', marginTop: 4 }}>
                          <span>IP: <strong>{step.server_ip || step.ip}</strong></span>
                          <span>Latency: <strong style={{ color: 'var(--color-brand-green)' }}>{Number(step.latency_ms !== undefined ? step.latency_ms : (step.rtt_ms || 0)).toFixed(1)}ms</strong></span>
                        </div>
                        {step.delegated_to && (
                          <div style={{ fontSize: 10, color: 'var(--color-brand-muted)', borderTop: '1px solid var(--color-brand-border)', marginTop: 6, paddingTop: 6 }}>
                            Delegated Zone to: <strong style={{ color: 'var(--color-brand-heading)' }}>{step.delegated_to}</strong>
                          </div>
                        )}
                      </div>
                    </div>
                  ))}
                </div>
              )}
            </div>

            <div style={{ display: 'flex', justifyContent: 'flex-end', marginTop: 4 }}>
              <button 
                onClick={() => setShowTraceModal(false)}
                className="btn" 
                style={{ padding: '8px 16px', borderRadius: 8, border: '1px solid var(--color-brand-border)', background: 'var(--color-brand-card)', color: 'var(--color-brand-heading)', cursor: 'pointer', fontSize: 12, fontWeight: 700 }}
              >
                Close Dialog
              </button>
            </div>
          </div>
        </div>
      )}

      {/* DNS ZONE TRANSFER AUDITOR (AXFR) */}
      {showAXFRModal && axfrResolverKey && (
        <div
          style={{
            position: 'fixed',
            top: 0, left: 0, width: '100%', height: '100%',
            background: 'rgba(0,0,0,0.6)',
            display: 'flex', alignItems: 'center', justifyContent: 'center',
            zIndex: 999,
          }}
          onClick={() => setShowAXFRModal(false)}
        >
          <div
            style={{
              background: 'var(--color-brand-card)',
              padding: 24,
              borderRadius: 12,
              width: 580,
              maxWidth: '95%',
              maxHeight: '85vh',
              overflowY: 'auto',
              boxShadow: '0 10px 25px rgba(0,0,0,0.15)',
              display: 'flex',
              flexDirection: 'column',
              gap: 16
            }}
            onClick={(e) => e.stopPropagation()}
          >
            <div style={{ display: 'flex', justifyContent: 'space-between', alignItems: 'center' }}>
              <h3 style={{ fontSize: 16, fontWeight: 700, color: 'var(--color-brand-heading)', margin: 0 }}>
                DNS AXFR Zone Transfer Auditor
              </h3>
              <span style={{ fontSize: 10, padding: '2px 6px', borderRadius: 4, background: 'var(--color-brand-bg)', color: 'var(--color-brand-text)' }}>
                Target: {resolvers[axfrResolverKey]?.ip}
              </span>
            </div>

            <div style={{ display: 'flex', gap: 10, alignItems: 'flex-end' }}>
              <div style={{ flex: 1, display: 'flex', flexDirection: 'column', gap: 4 }}>
                <label style={{ fontSize: 10, fontWeight: 700, color: 'var(--color-brand-text)' }}>DOMAIN ZONE TO TEST</label>
                <input 
                  type="text" 
                  value={axfrDomain} 
                  onChange={(e) => setAxfrDomain(e.target.value.trim())}
                  placeholder="e.g. zonetransfer.me"
                  style={{ padding: '8px 12px', borderRadius: 8, border: '1px solid var(--color-brand-border)', background: 'var(--color-brand-bg)', color: 'var(--color-brand-heading)', fontSize: 12 }}
                />
              </div>
              <button
                onClick={() => handleRunAXFR(resolvers[axfrResolverKey]?.ip, axfrDomain)}
                disabled={isTestingAXFR || !axfrDomain}
                className="btn btn-primary"
                style={{ padding: '10px 16px', borderRadius: 8, fontSize: 12, fontWeight: 700, cursor: 'pointer', height: 36, display: 'flex', alignItems: 'center', gap: 6 }}
              >
                {isTestingAXFR ? <FiRefreshCw className="animate-spin" /> : <FiPlay />}
                {isTestingAXFR ? "Auditing..." : "Audit Zone"}
              </button>
            </div>

            {/* AXFR audit outcome */}
            <div style={{ minHeight: 200, maxHeight: 350, overflowY: 'auto' }}>
              {isTestingAXFR ? (
                <div style={{ display: 'flex', flexDirection: 'column', alignItems: 'center', justifyContent: 'center', height: '100%', minHeight: 180, color: 'var(--color-brand-muted)', gap: 8 }}>
                  <FiRefreshCw className="animate-spin" size={24} />
                  <span style={{ fontSize: 12 }}>Contacting nameservers and requesting full zone transfer (AXFR)...</span>
                </div>
              ) : axfrResult ? (
                <div style={{ display: 'flex', flexDirection: 'column', gap: 12 }}>
                  {/* Status Banner */}
                  <div style={{ 
                    padding: 14, borderRadius: 8, 
                    border: '1px solid ' + (axfrResult.allowed ? '#ef4444' : '#22c55e'),
                    background: axfrResult.allowed ? 'rgba(239, 68, 68, 0.08)' : 'rgba(34, 197, 94, 0.08)',
                    color: axfrResult.allowed ? '#ef4444' : '#22c55e',
                    display: 'flex', alignItems: 'center', gap: 10
                  }}>
                    <FiInfo size={18} />
                    <div>
                      <div style={{ fontWeight: 700, fontSize: 13 }}>
                        {axfrResult.allowed ? "VULNERABILITY DETECTED: AXFR Zone Transfer Allowed!" : "SECURE: Zone Transfer Request Rejected"}
                      </div>
                      <div style={{ fontSize: 11, opacity: 0.85, marginTop: 2 }}>
                        {axfrResult.allowed 
                          ? `Leaked ${axfrResult.records_count} resource records from the nameserver.` 
                          : `The resolver or authoritative server refused the replication request.`
                        }
                      </div>
                    </div>
                  </div>

                  {/* Message Detail / Leaked Records List */}
                  {axfrResult.message && (
                    <div style={{ fontSize: 11, padding: 10, borderRadius: 6, background: 'var(--color-brand-bg)', border: '1px solid var(--color-brand-border)', color: 'var(--color-brand-text)', fontStyle: 'italic' }}>
                      Server Response: {axfrResult.message}
                    </div>
                  )}

                  {axfrResult.allowed && axfrResult.records && axfrResult.records.length > 0 && (
                    <div style={{ display: 'flex', flexDirection: 'column', gap: 4 }}>
                      <span style={{ fontSize: 10, fontWeight: 700, color: 'var(--color-brand-text)' }}>LEAKED RESOURCE RECORDS:</span>
                      <div style={{ 
                        fontFamily: 'monospace', fontSize: 11, padding: 12, borderRadius: 8, 
                        background: 'var(--color-brand-bg)', border: '1px solid var(--color-brand-border)', 
                        color: 'var(--color-brand-heading)', maxHeight: 200, overflowY: 'auto', whiteSpace: 'pre-wrap'
                      }}>
                        {axfrResult.records.join('\n')}
                      </div>
                    </div>
                  )}
                </div>
              ) : (
                <div style={{ display: 'flex', flexDirection: 'column', alignItems: 'center', justifyContent: 'center', height: '100%', minHeight: 180, color: 'var(--color-brand-muted)', gap: 8, textAlign: 'center' }}>
                  <FiDatabase size={24} />
                  <span style={{ fontSize: 12 }}>
                    Click Audit Zone to audit zone transfer settings on this resolver.
                  </span>
                </div>
              )}
            </div>

            <div style={{ display: 'flex', justifyContent: 'flex-end', marginTop: 4 }}>
              <button 
                onClick={() => setShowAXFRModal(false)}
                className="btn" 
                style={{ padding: '8px 16px', borderRadius: 8, border: '1px solid var(--color-brand-border)', background: 'var(--color-brand-card)', color: 'var(--color-brand-heading)', cursor: 'pointer', fontSize: 12, fontWeight: 700 }}
              >
                Close Dialog
              </button>
            </div>
          </div>
        </div>
      )}
      {showAdvancedTestModal && advancedTestResolverKey && (
        <div
          style={{
            position: 'fixed',
            top: 0, left: 0, width: '100%', height: '100%',
            background: 'rgba(0,0,0,0.6)',
            display: 'flex', alignItems: 'center', justifyContent: 'center',
            zIndex: 999,
          }}
          onClick={() => setShowAdvancedTestModal(false)}
        >
          <div
            style={{
              background: 'var(--color-brand-card)',
              padding: 24,
              borderRadius: 12,
              width: 720,
              maxWidth: '95%',
              maxHeight: '90vh',
              overflowY: 'auto',
              boxShadow: '0 10px 25px rgba(0,0,0,0.15)',
              display: 'flex',
              flexDirection: 'column',
              gap: 16
            }}
            onClick={(e) => e.stopPropagation()}
          >
            {/* Modal Header */}
            <div style={{ display: 'flex', justifyContent: 'space-between', alignItems: 'center', borderBottom: '1px solid var(--color-brand-border)', paddingBottom: 12 }}>
              <div>
                <h3 style={{ fontSize: 16, fontWeight: 700, color: 'var(--color-brand-heading)', margin: 0 }}>
                  Advanced Single DNS Resolver Test
                </h3>
                <div style={{ fontSize: 11, color: 'var(--color-brand-muted)', marginTop: 4 }}>
                  Target Resolver: <strong style={{ color: 'var(--color-brand-heading)' }}>{resolvers[advancedTestResolverKey]?.ip}</strong> ({resolvers[advancedTestResolverKey]?.protocol.toUpperCase()})
                </div>
              </div>
              <button 
                onClick={() => setShowAdvancedTestModal(false)}
                style={{ background: 'none', border: 'none', color: 'var(--color-brand-muted)', cursor: 'pointer' }}
              >
                <FiX size={18} />
              </button>
            </div>

            {/* Config Fields Grid */}
            <div style={{ display: 'grid', gridTemplateColumns: 'repeat(auto-fit, minmax(180px, 1fr))', gap: 12, background: 'var(--color-brand-bg)', padding: 14, borderRadius: 8, border: '1px solid var(--color-brand-border)' }}>
              {/* Query Domain */}
              <div style={{ display: 'flex', flexDirection: 'column', gap: 4 }}>
                <label style={{ fontSize: 10, fontWeight: 700, color: 'var(--color-brand-text)' }}>QUERY DOMAIN</label>
                <input 
                  type="text" 
                  value={advDomain}
                  onChange={(e) => setAdvDomain(e.target.value)}
                  style={{ padding: '6px 10px', borderRadius: 6, border: '1px solid var(--color-brand-border)', background: 'var(--color-brand-card)', color: 'var(--color-brand-heading)', fontSize: 11 }}
                />
              </div>

              {/* Query Type */}
              <div style={{ display: 'flex', flexDirection: 'column', gap: 4 }}>
                <label style={{ fontSize: 10, fontWeight: 700, color: 'var(--color-brand-text)' }}>QUERY TYPE</label>
                <select 
                  value={advQueryType}
                  onChange={(e) => setAdvQueryType(e.target.value)}
                  style={{ padding: '6px 10px', borderRadius: 6, border: '1px solid var(--color-brand-border)', background: 'var(--color-brand-card)', color: 'var(--color-brand-heading)', fontSize: 11 }}
                >
                  {['A', 'AAAA', 'MX', 'TXT', 'NS', 'CNAME', 'SOA', 'SRV', 'CAA'].map(t => (
                    <option key={t} value={t}>{t}</option>
                  ))}
                </select>
              </div>

              {/* DNS Class */}
              <div style={{ display: 'flex', flexDirection: 'column', gap: 4 }}>
                <label style={{ fontSize: 10, fontWeight: 700, color: 'var(--color-brand-text)' }}>DNS CLASS</label>
                <select 
                  value={advDNSClass}
                  onChange={(e) => setAdvDNSClass(e.target.value)}
                  style={{ padding: '6px 10px', borderRadius: 6, border: '1px solid var(--color-brand-border)', background: 'var(--color-brand-card)', color: 'var(--color-brand-heading)', fontSize: 11 }}
                >
                  {['IN', 'CH', 'HS'].map(c => (
                    <option key={c} value={c}>{c}</option>
                  ))}
                </select>
              </div>

              {/* Timeout */}
              <div style={{ display: 'flex', flexDirection: 'column', gap: 4 }}>
                <label style={{ fontSize: 10, fontWeight: 700, color: 'var(--color-brand-text)' }}>TIMEOUT (MS)</label>
                <input 
                  type="number" 
                  value={advTimeout}
                  onChange={(e) => setAdvTimeout(Number(e.target.value))}
                  style={{ padding: '6px 10px', borderRadius: 6, border: '1px solid var(--color-brand-border)', background: 'var(--color-brand-card)', color: 'var(--color-brand-heading)', fontSize: 11 }}
                />
              </div>

              {/* Attempts */}
              <div style={{ display: 'flex', flexDirection: 'column', gap: 4 }}>
                <label style={{ fontSize: 10, fontWeight: 700, color: 'var(--color-brand-text)' }}>ATTEMPTS</label>
                <input 
                  type="number" 
                  value={advAttempts}
                  onChange={(e) => setAdvAttempts(Number(e.target.value))}
                  style={{ padding: '6px 10px', borderRadius: 6, border: '1px solid var(--color-brand-border)', background: 'var(--color-brand-card)', color: 'var(--color-brand-heading)', fontSize: 11 }}
                />
              </div>

              {/* Expect Response */}
              <div style={{ display: 'flex', flexDirection: 'column', gap: 4 }}>
                <label style={{ fontSize: 10, fontWeight: 700, color: 'var(--color-brand-text)' }}>EXPECT RESPONSE (OPTIONAL)</label>
                <input 
                  type="text" 
                  value={advExpectResponse}
                  onChange={(e) => setAdvExpectResponse(e.target.value)}
                  placeholder="e.g. 1.1.1.1 or cloudflare"
                  style={{ padding: '6px 10px', borderRadius: 6, border: '1px solid var(--color-brand-border)', background: 'var(--color-brand-card)', color: 'var(--color-brand-heading)', fontSize: 11 }}
                />
              </div>

              {/* Cache Busting */}
              <div style={{ display: 'flex', alignItems: 'center', gap: 8, gridColumn: '1 / -1', marginTop: 4 }}>
                <input 
                  type="checkbox" 
                  id="advCacheBusting"
                  checked={advCacheBusting}
                  onChange={(e) => setAdvCacheBusting(e.target.checked)}
                  style={{ accentColor: 'var(--color-brand)', cursor: 'pointer' }}
                />
                <label htmlFor="advCacheBusting" style={{ fontSize: 11, fontWeight: 600, color: 'var(--color-brand-heading)', cursor: 'pointer' }}>
                  Enable Cache Busting (adds a random sub-domain prefix to bypass middlebox caching)
                </label>
              </div>
            </div>

            {/* Run Button */}
            <div style={{ display: 'flex', justifyContent: 'flex-end' }}>
              <button
                onClick={handleRunSingleTest}
                disabled={isTestingSingle}
                className="btn btn-primary"
                style={{ padding: '8px 16px', borderRadius: 8, fontSize: 12, fontWeight: 700, cursor: 'pointer', display: 'flex', alignItems: 'center', gap: 6 }}
              >
                {isTestingSingle ? <FiRefreshCw className="animate-spin" /> : <FiPlay />}
                {isTestingSingle ? "Running Test Query..." : "Execute Test"}
              </button>
            </div>

            {/* Test Results Outcome */}
            <div style={{ display: 'flex', flexDirection: 'column', gap: 12 }}>
              {isTestingSingle ? (
                <div style={{ display: 'flex', flexDirection: 'column', alignItems: 'center', justifyContent: 'center', minHeight: 200, border: '1px dashed var(--color-brand-border)', borderRadius: 8, color: 'var(--color-brand-muted)', gap: 8 }}>
                  <FiRefreshCw className="animate-spin" size={24} />
                  <span style={{ fontSize: 12 }}>Sending query pack and listening for response...</span>
                </div>
              ) : singleTestError ? (
                <div style={{ display: 'flex', gap: 10, padding: 12, borderRadius: 8, background: 'rgba(239, 68, 68, 0.08)', border: '1px solid #ef4444', color: '#ef4444', alignItems: 'center', fontSize: 12 }}>
                  <FiAlertTriangle size={16} />
                  <span>{singleTestError}</span>
                </div>
              ) : singleTestResult ? (
                <div style={{ display: 'flex', flexDirection: 'column', gap: 12 }}>
                  {/* Summary Metric Stats cards */}
                  <div style={{ display: 'grid', gridTemplateColumns: 'repeat(auto-fit, minmax(130px, 1fr))', gap: 10 }}>
                    <div style={{ background: 'var(--color-brand-bg)', border: '1px solid var(--color-brand-border)', padding: '10px 14px', borderRadius: 8, textAlign: 'center' }}>
                      <div style={{ fontSize: 9, color: 'var(--color-brand-muted)', fontWeight: 700 }}>CLEVER SCORE</div>
                      <div style={{ fontSize: 20, fontWeight: 800, color: 'var(--color-brand)', marginTop: 4 }}>{singleTestResult.clever_score}/100</div>
                    </div>
                    <div style={{ background: 'var(--color-brand-bg)', border: '1px solid var(--color-brand-border)', padding: '10px 14px', borderRadius: 8, textAlign: 'center' }}>
                      <div style={{ fontSize: 9, color: 'var(--color-brand-muted)', fontWeight: 700 }}>RTT LATENCY</div>
                      <div style={{ fontSize: 20, fontWeight: 800, color: 'var(--color-brand-heading)', marginTop: 4 }}>{singleTestResult.latency_ms.toFixed(1)} ms</div>
                    </div>
                    <div style={{ background: 'var(--color-brand-bg)', border: '1px solid var(--color-brand-border)', padding: '10px 14px', borderRadius: 8, textAlign: 'center' }}>
                      <div style={{ fontSize: 9, color: 'var(--color-brand-muted)', fontWeight: 700 }}>SUCCESS RATE</div>
                      <div style={{ fontSize: 20, fontWeight: 800, color: singleTestResult.success_rate > 50 ? 'var(--color-brand-green)' : 'var(--color-brand-red)', marginTop: 4 }}>{singleTestResult.success_rate.toFixed(0)}%</div>
                    </div>
                    <div style={{ background: 'var(--color-brand-bg)', border: '1px solid var(--color-brand-border)', padding: '10px 14px', borderRadius: 8, textAlign: 'center' }}>
                      <div style={{ fontSize: 9, color: 'var(--color-brand-muted)', fontWeight: 700 }}>DNSSEC</div>
                      <div style={{ fontSize: 20, fontWeight: 800, color: singleTestResult.dnssec ? 'var(--color-brand-green)' : 'var(--color-brand-muted)', marginTop: 4 }}>
                        {singleTestResult.dnssec ? "SUPPORTED" : "NO"}
                      </div>
                    </div>
                  </div>

                  {/* Geodata information registry */}
                  {singleTestResult.resolved_ip && (
                    <div style={{ display: 'flex', flexDirection: 'column', gap: 6, background: 'var(--color-brand-bg)', border: '1px solid var(--color-brand-border)', padding: 12, borderRadius: 8 }}>
                      <span style={{ fontSize: 10, fontWeight: 700, color: 'var(--color-brand-text)' }}>RESOLVED IP GEOLOCATION INTELLIGENCE</span>
                      <div style={{ display: 'flex', flexWrap: 'wrap', gap: 14, fontSize: 11, color: 'var(--color-brand-heading)' }}>
                        <span>IP: <strong>{singleTestResult.resolved_ip}</strong></span>
                        {singleTestResult.geoip?.country_name && (
                          <span style={{ display: 'inline-flex', alignItems: 'center', gap: 4 }}>
                            Country: <strong>{singleTestResult.geoip.country_name}</strong>
                          </span>
                        )}
                        {singleTestResult.geoip?.city && (
                          <span>City: <strong>{singleTestResult.geoip.city}</strong></span>
                        )}
                        {singleTestResult.geoip?.isp && (
                          <span>ISP: <strong>{singleTestResult.geoip.isp}</strong></span>
                        )}
                        {singleTestResult.geoip?.is_cdn && (
                          <span style={{ background: 'rgba(249, 115, 22, 0.1)', color: '#ea580c', padding: '1px 4px', borderRadius: 3, fontWeight: 700 }}>
                            CDN: {singleTestResult.geoip.cdn_provider}
                          </span>
                        )}
                        {singleTestResult.rebinding && (
                          <span style={{ background: 'rgba(239, 68, 68, 0.1)', color: '#dc2626', padding: '1px 4px', borderRadius: 3, fontWeight: 700 }}>
                            DNS Rebinding Vulnerability Detected!
                          </span>
                        )}
                      </div>
                    </div>
                  )}

                  {/* Answer Records list */}
                  {singleTestResult.answers && singleTestResult.answers.length > 0 && (
                    <div style={{ display: 'flex', flexDirection: 'column', gap: 4 }}>
                      <span style={{ fontSize: 10, fontWeight: 700, color: 'var(--color-brand-text)' }}>ANSWER RECORDS ({singleTestResult.answers.length})</span>
                      <div style={{ maxHeight: 180, overflowY: 'auto', border: '1px solid var(--color-brand-border)', borderRadius: 6 }}>
                        <table style={{ width: '100%', borderCollapse: 'collapse', fontSize: 11 }}>
                          <thead>
                            <tr style={{ background: 'var(--color-brand-bg)', borderBottom: '1px solid var(--color-brand-border)', textAlign: 'left' }}>
                              <th style={{ padding: '6px 8px' }}>Name</th>
                              <th style={{ padding: '6px 8px' }}>Type</th>
                              <th style={{ padding: '6px 8px' }}>TTL</th>
                              <th style={{ padding: '6px 8px' }}>Data</th>
                            </tr>
                          </thead>
                          <tbody>
                            {singleTestResult.answers.map((ans: any, idx: number) => (
                              <tr key={idx} style={{ borderBottom: '1px solid var(--color-brand-border)' }}>
                                <td style={{ padding: '6px 8px', color: 'var(--color-brand-text)' }}>{ans.name}</td>
                                <td style={{ padding: '6px 8px', fontWeight: 700, color: 'var(--color-brand-heading)' }}>{ans.type}</td>
                                <td style={{ padding: '6px 8px', color: 'var(--color-brand-muted)' }}>{ans.ttl}</td>
                                <td style={{ padding: '6px 8px', color: 'var(--color-brand-heading)', fontFamily: 'monospace' }}>{ans.data}</td>
                              </tr>
                            ))}
                          </tbody>
                        </table>
                      </div>
                    </div>
                  )}

                  {/* Raw Response exchange */}
                  {singleTestResult.raw_response && (
                    <div style={{ display: 'flex', flexDirection: 'column', gap: 4 }}>
                      <span style={{ fontSize: 10, fontWeight: 700, color: 'var(--color-brand-text)' }}>RAW DNS EXCHANGE MESSAGE (MIEKGDNS PACKET FORMAT)</span>
                      <pre style={{ 
                        margin: 0, padding: 12, borderRadius: 6, 
                        background: 'var(--color-brand-bg)', border: '1px solid var(--color-brand-border)', 
                        color: 'var(--color-brand-heading)', fontSize: 10, fontFamily: 'monospace',
                        maxHeight: 180, overflowY: 'auto', whiteSpace: 'pre-wrap'
                      }}>
                        {singleTestResult.raw_response}
                      </pre>
                    </div>
                  )}
                </div>
              ) : (
                <div style={{ display: 'flex', flexDirection: 'column', alignItems: 'center', justifyContent: 'center', minHeight: 180, color: 'var(--color-brand-muted)', gap: 8, textAlign: 'center', border: '1px dashed var(--color-brand-border)', borderRadius: 8 }}>
                  <FiActivity size={24} />
                  <span style={{ fontSize: 12 }}>
                    Click Execute Test to send custom query payloads to the resolver.
                  </span>
                </div>
              )}
            </div>

            {/* Modal Footer */}
            <div style={{ display: 'flex', justifyContent: 'flex-end', borderTop: '1px solid var(--color-brand-border)', paddingTop: 12, marginTop: 4 }}>
              <button 
                onClick={() => setShowAdvancedTestModal(false)}
                className="btn" 
                style={{ padding: '8px 16px', borderRadius: 8, border: '1px solid var(--color-brand-border)', background: 'var(--color-brand-card)', color: 'var(--color-brand-heading)', cursor: 'pointer', fontSize: 12, fontWeight: 700 }}
              >
                Close Panel
              </button>
            </div>
          </div>
        </div>
      )}
      
      <style>{`
        @keyframes pulse-row {
          0% { background-color: rgba(255, 107, 44, 0.01); }
          50% { background-color: rgba(255, 107, 44, 0.08); }
          100% { background-color: rgba(255, 107, 44, 0.01); }
        }
        .pulse-testing {
          animation: pulse-row 1.8s infinite ease-in-out;
        }
        @keyframes slideIn {
          from { transform: translateY(-20px); opacity: 0; }
          to { transform: translateY(0); opacity: 1; }
        }
      `}</style>
    </div>
  );
};

// Sort icon helper
const SortIcon = ({ column, sortBy, sortOrder }: { column: string; sortBy: string; sortOrder: 'asc' | 'desc' }) => {
  if (sortBy !== column) return <FiChevronDown style={{ opacity: 0.3 }} />;
  return sortOrder === 'asc' ? <FiChevronUp style={{ color: 'var(--color-brand)' }} /> : <FiChevronDown style={{ color: 'var(--color-brand)' }} />;
};
