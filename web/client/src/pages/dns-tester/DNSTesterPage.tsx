import React, { useEffect, useState, useRef, useMemo, useCallback } from 'react';
import { useVirtualizer } from '@tanstack/react-virtual';
import { 
  FiSearch, FiPlay, FiCheckCircle, FiClock, FiGlobe, FiPlus, 
  FiRefreshCw, FiServer, FiActivity, FiTrendingUp, 
  FiDatabase, FiStopCircle, FiCheck, FiInfo, FiAlertTriangle,
  FiChevronDown, FiChevronUp, FiFilter, FiX, FiCopy, FiDownload, FiSettings
} from 'react-icons/fi';
import { useDNSStore, getResolverKey } from '../../store/dnsStore';
import type { DNSResolver } from '../../store/dnsStore';
import { useAuthStore } from '../../store/authStore';
import { IPResolveBadge } from '../../components/atoms/IPResolveBadge';

import { ResolverRow } from './components/ResolverRow';
import { SettingsAccordion } from './components/SettingsAccordion';
import { 
  FetchPublicModal, AddResolverModal, 
  TraceModal, AXFRModal, AdvancedTestModal 
} from './components/Modals';

// Sort icon helper
const SortIcon = ({ column, sortBy, sortOrder }: { column: string; sortBy: string; sortOrder: 'asc' | 'desc' }) => {
  if (sortBy !== column) return <FiChevronDown style={{ opacity: 0.3 }} />;
  return sortOrder === 'asc' ? <FiChevronUp style={{ color: 'var(--color-brand)' }} /> : <FiChevronDown style={{ color: 'var(--color-brand)' }} />;
};

export const DNSTesterPage: React.FC = () => {
  const { token } = useAuthStore();
  const { 
    resolvers, 
    resolverKeys, 
    jobStats, 
    isSweeping, 
    appliedResolver,
    updateResolver,
    updateResolversBulk,
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
  const [selectedKeys, setSelectedKeys] = useState<Set<string>>(new Set());

  // Alerts
  const [toastMessage, setToastMessage] = useState<string | null>(null);
  const [toastType, setToastType] = useState<'success' | 'error' | 'info'>('success');

  // Accordion parameters
  const [showSettings, setShowSettings] = useState(false);
  const [showAddModal, setShowAddModal] = useState(false);
  const [modalTab, setModalTab] = useState<'single' | 'bulk'>('single');
  const [showAdvancedFilters, setShowAdvancedFilters] = useState(false);

  // Advanced Inline Filters
  const [filterMinLatency, setFilterMinLatency] = useState('');
  const [filterMaxLatency, setFilterMaxLatency] = useState('');
  const [filterMinSuccessRate, setFilterMinSuccessRate] = useState('');
  const [filterCensorship, setFilterCensorship] = useState('ALL');
  const [filterDNSSEC, setFilterDNSSEC] = useState('ALL');
  const [filterRebinding, setFilterRebinding] = useState('ALL');
  const [filterISP, setFilterISP] = useState('');
  const [filterCountry, setFilterCountry] = useState('');
  const [filterCDN, setFilterCDN] = useState('ALL');

  // Custom Form fields
  const [customIP, setCustomIP] = useState('');
  const [customProvider, setCustomProvider] = useState('');
  const [customProtocol, setCustomProtocol] = useState('udp');
  const [customCategory, setCustomCategory] = useState('custom');

  // Sweep Config parameters
  const [concurrencyLimit, setConcurrencyLimit] = useState(10);
  const [qpsLimit] = useState(50); // Locked by backend usually
  const [timeoutMs, setTimeoutMs] = useState(2000);
  const [attempts, setAttempts] = useState(2);
  const [cacheBusting, setCacheBusting] = useState(true);
  const [referenceDomain, setReferenceDomain] = useState('google.com');
  const [selectedProtocols, setSelectedProtocols] = useState<string[]>(['udp', 'tcp', 'dot', 'doh']);
  const [queryTypes, setQueryTypes] = useState<string[]>(['A']);
  const [dnsClass, setDnsClass] = useState('IN');
  const [queryGenerator, setQueryGenerator] = useState('random');
  const [domainSource, setDomainSource] = useState('default');
  const [customDomains, setCustomDomains] = useState('');
  const [wordlistURL, setWordlistURL] = useState('');
  const [expectResponse, setExpectResponse] = useState('');

  // Trace variables
  const [showTraceModal, setShowTraceModal] = useState(false);
  const [traceResolverKey, setTraceResolverKey] = useState<string | null>(null);
  const [traceDomain, setTraceDomain] = useState('google.com');
  const [traceSteps, setTraceSteps] = useState<any[]>([]);
  const [isTracing, setIsTracing] = useState(false);

  // AXFR variables
  const [showAXFRModal, setShowAXFRModal] = useState(false);
  const [axfrResolverKey, setAxfrResolverKey] = useState<string | null>(null);
  const [axfrDomain, setAxfrDomain] = useState('zonetransfer.me');
  const [axfrResult, setAxfrResult] = useState<any>(null);
  const [isTestingAXFR, setIsTestingAXFR] = useState(false);

  // Advanced query test variables
  const [showAdvancedTestModal, setShowAdvancedTestModal] = useState(false);
  const [advancedTestResolverKey, setAdvancedTestResolverKey] = useState<string | null>(null);
  const [advDomain, setAdvDomain] = useState('google.com');
  const [advQueryType, setAdvQueryType] = useState('A');
  const [advDNSClass, setAdvDNSClass] = useState('IN');
  const [advTimeout, setAdvTimeout] = useState(2000);
  const [advAttempts, setAdvAttempts] = useState(2);
  const [advCacheBusting, setAdvCacheBusting] = useState(true);
  const [advExpectResponse, setAdvExpectResponse] = useState('');
  const [isTestingSingle, setIsTestingSingle] = useState(false);
  const [singleTestResult, setSingleTestResult] = useState<any>(null);
  const [singleTestError, setSingleTestError] = useState<string | null>(null);

  // Public DNS Grabber variables
  const [showFetchPublicModal, setShowFetchPublicModal] = useState(false);
  const [selectedPublicSource, setSelectedPublicSource] = useState('curated');
  const [isFetchingPublic, setIsFetchingPublic] = useState(false);
  const [fetchPublicResult, setFetchPublicResult] = useState<any>(null);

  // Bulk File uploading
  const [bulkText, setBulkText] = useState('');
  const [bulkFile, setBulkFile] = useState<File | null>(null);
  const [isImporting, setIsImporting] = useState(false);

  const parentRef = useRef<HTMLDivElement>(null);

  const triggerToast = (msg: string, type: 'success' | 'error' | 'info' = 'success') => {
    setToastMessage(msg);
    setToastType(type);
    setTimeout(() => { setToastMessage(null); }, 4000);
  };

  // 2. Fetch initial active resolver and configs
  const fetchResolversList = async () => {
    try {
      const activeToken = token || localStorage.getItem('cc_client_token') || '';
      const res = await fetch('/api/dns/resolvers', {
        headers: { 'Authorization': `Bearer ${activeToken}` }
      });
      if (res.ok) {
        const list = await res.json();
        useDNSStore.getState().setResolvers(list.data || list);
      }
    } catch (e) {
      console.error("Failed to load resolvers list", e);
    }
  };

  const fetchTesterConfig = async () => {
    try {
      const activeToken = token || localStorage.getItem('cc_client_token') || '';
      const res = await fetch('/api/dns/config', {
        headers: { 'Authorization': `Bearer ${activeToken}` }
      });
      if (res.ok) {
        const data = await res.json();
        if (data.resolver_applied) {
          setAppliedResolver(data.resolver_applied);
        }
        if (data.concurrency_limit) setConcurrencyLimit(data.concurrency_limit);
        if (data.timeout_ms) setTimeoutMs(data.timeout_ms);
        if (data.attempts) setAttempts(data.attempts);
        if (data.reference_domain) setReferenceDomain(data.reference_domain);
        if (data.selected_protocols) setSelectedProtocols(data.selected_protocols);
        if (data.query_types) setQueryTypes(data.query_types);
        if (data.dns_class) setDnsClass(data.dns_class);
        if (data.query_generator) setQueryGenerator(data.query_generator);
        if (data.domain_source) setDomainSource(data.domain_source);
        if (data.custom_domains) setCustomDomains(data.custom_domains);
        if (data.wordlist_url) setWordlistURL(data.wordlist_url);
        if (data.expect_response !== undefined) setExpectResponse(data.expect_response || '');
      }
    } catch (e) {
      console.error("Failed to fetch dns configurations", e);
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
            const updates: Record<string, Partial<DNSResolver>> = {};
            candidates.forEach((res: any) => {
              const key = getResolverKey(res.ip, res.protocol);
              updates[key] = {
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
              };
            });
            updateResolversBulk(updates);
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

    // Visually set testing state (HIGH PERFORMANCE BATCH UPDATE)
    const updates: Record<string, Partial<DNSResolver>> = {};
    resolverKeys.forEach(k => {
      const r = resolvers[k];
      if (r && selectedProtocols.includes(r.protocol)) {
        updates[k] = { is_testing: true };
      }
    });
    updateResolversBulk(updates);

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

      const searchMatch = r.ip.includes(search) || r.provider_name.toLowerCase().includes(search.toLowerCase());
      if (!searchMatch) return false;

      if (protocolFilter !== 'ALL' && r.protocol.toLowerCase() !== protocolFilter.toLowerCase()) {
        return false;
      }

      if (categoryFilter !== 'ALL' && r.category.toLowerCase() !== categoryFilter.toLowerCase()) {
        return false;
      }

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

  // Helper to check if resolver is the applied resolver
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

  const activeTraceResolver = traceResolverKey ? resolvers[traceResolverKey] : undefined;
  const activeAXFRResolver = axfrResolverKey ? resolvers[axfrResolverKey] : undefined;
  const activeAdvancedTestResolver = advancedTestResolverKey ? resolvers[advancedTestResolverKey] : undefined;

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
            <FiInfo /> Applied settings override Xray/Sing-box core configs.
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

            {/* Config & Advanced filters toggle buttons */}
            <button
              onClick={() => setShowSettings(!showSettings)}
              className="btn btn--secondary"
              style={{
                display: 'flex',
                alignItems: 'center',
                gap: 6,
                padding: '8px 12px',
                borderRadius: 8,
                fontSize: 12,
                border: `1px solid ${showSettings ? 'var(--color-brand)' : 'var(--color-brand-border)'}`,
                background: showSettings ? 'rgba(255, 107, 44, 0.08)' : 'var(--color-brand-bg)',
                color: showSettings ? 'var(--color-brand)' : 'var(--color-brand-heading)',
                cursor: 'pointer',
                fontWeight: showSettings ? 600 : 400,
                transition: 'all 0.15s ease'
              }}
            >
              <FiSettings /> {showSettings ? "Hide Options" : "Test Options"}
            </button>

            <button
              onClick={() => setShowAdvancedFilters(!showAdvancedFilters)}
              className="btn"
              style={{
                display: 'flex',
                alignItems: 'center',
                gap: 6,
                padding: '8px 12px',
                borderRadius: 8,
                fontSize: 12,
                cursor: 'pointer',
                border: `1px solid ${showAdvancedFilters ? 'var(--color-brand)' : 'var(--color-brand-border)'}`,
                background: showAdvancedFilters ? 'rgba(59, 130, 246, 0.08)' : 'var(--color-brand-bg)',
                color: showAdvancedFilters ? 'var(--color-brand)' : 'var(--color-brand-heading)',
                outline: 'none',
                fontWeight: showAdvancedFilters ? 600 : 400,
                transition: 'all 0.15s ease'
              }}
            >
              <FiFilter /> {showAdvancedFilters ? "Hide Filters" : "Advanced Filters"}
            </button>

          </div>

          {/* Action Toolbar */}
          <div style={{ display: 'flex', alignItems: 'center', gap: 8 }}>
            
            <button
              className="btn btn--secondary"
              onClick={copyHealthyToClipboard}
              style={{ display: 'flex', alignItems: 'center', gap: 6, padding: '8px 12px', borderRadius: 8, fontSize: 12 }}
              title="Copy all healthy resolver IPs to clipboard"
            >
              <FiCopy /> Copy Healthy
            </button>

            <div style={{ position: 'relative' }}>
              <button
                className="btn btn--secondary"
                onClick={(e) => {
                  const menu = e.currentTarget.nextElementSibling as HTMLElement;
                  if (menu) menu.style.display = menu.style.display === 'block' ? 'none' : 'block';
                }}
                onBlur={(e) => {
                  const menu = e.currentTarget.nextElementSibling as HTMLElement;
                  setTimeout(() => {
                    if (menu) menu.style.display = 'none';
                  }, 200);
                }}
                style={{ display: 'flex', alignItems: 'center', gap: 6, padding: '8px 12px', borderRadius: 8, fontSize: 12 }}
              >
                <FiDownload /> Export
              </button>
              <div style={{ display: 'none', position: 'absolute', right: 0, top: '100%', marginTop: 4, background: 'var(--color-brand-card)', border: '1px solid var(--color-brand-border)', borderRadius: 8, boxShadow: '0 4px 12px rgba(0,0,0,0.1)', zIndex: 10, width: 140 }}>
                <button
                  onClick={() => exportResolvers('json')}
                  style={{ display: 'block', width: '100%', padding: '8px 12px', border: 'none', background: 'none', textAlign: 'left', fontSize: 12, cursor: 'pointer', color: 'var(--color-brand-text)' }}
                >
                  Export JSON
                </button>
                <button
                  onClick={() => exportResolvers('csv')}
                  style={{ display: 'block', width: '100%', padding: '8px 12px', border: 'none', background: 'none', textAlign: 'left', fontSize: 12, cursor: 'pointer', color: 'var(--color-brand-text)', borderTop: '1px solid var(--color-brand-border)' }}
                >
                  Export CSV
                </button>
              </div>
            </div>

            {selectedKeys.size > 0 && (
              <button
                className="btn"
                onClick={deleteSelectedResolvers}
                style={{ background: 'var(--color-brand-red)', color: '#fff', border: 'none', display: 'flex', alignItems: 'center', gap: 6, padding: '8px 12px', borderRadius: 8, fontSize: 12, cursor: 'pointer' }}
              >
                Delete Selected ({selectedKeys.size})
              </button>
            )}

            <button
              className="btn btn--secondary"
              onClick={() => {
                setFetchPublicResult(null);
                setShowFetchPublicModal(true);
              }}
              style={{ display: 'flex', alignItems: 'center', gap: 6, padding: '8px 12px', borderRadius: 8, fontSize: 12 }}
            >
              <FiGlobe /> Import Public DNS
            </button>

            <button
              className="btn"
              onClick={() => setShowAddModal(true)}
              style={{ border: '1px solid var(--color-brand-border)', background: 'var(--color-brand-card)', color: 'var(--color-brand-heading)', display: 'flex', alignItems: 'center', gap: 6, padding: '8px 12px', borderRadius: 8, fontSize: 12, cursor: 'pointer' }}
            >
              <FiPlus /> Add Resolver
            </button>
          </div>
        </div>

        {/* Sweep settings (Accordion) */}
        <SettingsAccordion
          showSettings={showSettings}
          concurrencyLimit={concurrencyLimit}
          setConcurrencyLimit={setConcurrencyLimit}
          timeoutMs={timeoutMs}
          setTimeoutMs={setTimeoutMs}
          attempts={attempts}
          setAttempts={setAttempts}
          referenceDomain={referenceDomain}
          setReferenceDomain={setReferenceDomain}
          dnsClass={dnsClass}
          setDnsClass={setDnsClass}
          queryGenerator={queryGenerator}
          setQueryGenerator={setQueryGenerator}
          domainSource={domainSource}
          setDomainSource={setDomainSource}
          customDomains={customDomains}
          setCustomDomains={setCustomDomains}
          wordlistURL={wordlistURL}
          setWordlistURL={setWordlistURL}
          queryTypes={queryTypes}
          setQueryTypes={setQueryTypes}
          selectedProtocols={selectedProtocols}
          onProtocolCheckbox={handleProtocolCheckbox}
          expectResponse={expectResponse}
          setExpectResponse={setExpectResponse}
        />

        {/* Advanced Filters (Accordion) */}
        {showAdvancedFilters && (
          <div className="animate-fade-in" style={{
            background: 'var(--color-brand-bg)',
            border: '1px solid var(--color-brand-border)',
            borderRadius: 8,
            padding: 16,
            display: 'grid',
            gridTemplateColumns: 'repeat(auto-fit, minmax(180px, 1fr))',
            gap: 12,
            marginTop: 10
          }}>
            <div style={{ display: 'flex', flexDirection: 'column', gap: 4 }}>
              <label style={{ fontSize: 9, fontWeight: 700, color: 'var(--color-brand-text)' }}>MIN LATENCY (MS)</label>
              <input type="number" placeholder="e.g. 10" value={filterMinLatency} onChange={(e) => setFilterMinLatency(e.target.value)} style={{ padding: '6px 10px', borderRadius: 6, border: '1px solid var(--color-brand-border)', background: 'var(--color-brand-card)', color: 'var(--color-brand-heading)', fontSize: 11 }} />
            </div>
            <div style={{ display: 'flex', flexDirection: 'column', gap: 4 }}>
              <label style={{ fontSize: 9, fontWeight: 700, color: 'var(--color-brand-text)' }}>MAX LATENCY (MS)</label>
              <input type="number" placeholder="e.g. 150" value={filterMaxLatency} onChange={(e) => setFilterMaxLatency(e.target.value)} style={{ padding: '6px 10px', borderRadius: 6, border: '1px solid var(--color-brand-border)', background: 'var(--color-brand-card)', color: 'var(--color-brand-heading)', fontSize: 11 }} />
            </div>
            <div style={{ display: 'flex', flexDirection: 'column', gap: 4 }}>
              <label style={{ fontSize: 9, fontWeight: 700, color: 'var(--color-brand-text)' }}>MIN SUCCESS RATE (%)</label>
              <input type="number" placeholder="e.g. 90" value={filterMinSuccessRate} onChange={(e) => setFilterMinSuccessRate(e.target.value)} style={{ padding: '6px 10px', borderRadius: 6, border: '1px solid var(--color-brand-border)', background: 'var(--color-brand-card)', color: 'var(--color-brand-heading)', fontSize: 11 }} />
            </div>

            <div style={{ display: 'flex', flexDirection: 'column', gap: 4 }}>
              <label style={{ fontSize: 9, fontWeight: 700, color: 'var(--color-brand-text)' }}>CENSORSHIP STATUS</label>
              <select value={filterCensorship} onChange={(e) => setFilterCensorship(e.target.value)} style={{ padding: '6px 10px', borderRadius: 6, border: '1px solid var(--color-brand-border)', background: 'var(--color-brand-card)', color: 'var(--color-brand-heading)', fontSize: 11 }}>
                <option value="ALL">All States</option>
                <option value="clean">Clean Only</option>
                <option value="manipulated">Manipulated / Censored</option>
                <option value="sinkhole">Sinkholed</option>
                <option value="hijacked">NXDomain Hijacked</option>
              </select>
            </div>

            <div style={{ display: 'flex', flexDirection: 'column', gap: 4 }}>
              <label style={{ fontSize: 9, fontWeight: 700, color: 'var(--color-brand-text)' }}>DNSSEC CAPABILITY</label>
              <select value={filterDNSSEC} onChange={(e) => setFilterDNSSEC(e.target.value)} style={{ padding: '6px 10px', borderRadius: 6, border: '1px solid var(--color-brand-border)', background: 'var(--color-brand-card)', color: 'var(--color-brand-heading)', fontSize: 11 }}>
                <option value="ALL">All Resolvers</option>
                <option value="valid">DNSSEC Validated</option>
                <option value="none">No Validation Support</option>
              </select>
            </div>

            <div style={{ display: 'flex', flexDirection: 'column', gap: 4 }}>
              <label style={{ fontSize: 9, fontWeight: 700, color: 'var(--color-brand-text)' }}>DNS REBINDING VULN</label>
              <select value={filterRebinding} onChange={(e) => setFilterRebinding(e.target.value)} style={{ padding: '6px 10px', borderRadius: 6, border: '1px solid var(--color-brand-border)', background: 'var(--color-brand-card)', color: 'var(--color-brand-heading)', fontSize: 11 }}>
                <option value="ALL">All</option>
                <option value="secure">Secure Only</option>
                <option value="vulnerable">Vulnerable Only</option>
              </select>
            </div>

            <div style={{ display: 'flex', flexDirection: 'column', gap: 4 }}>
              <label style={{ fontSize: 9, fontWeight: 700, color: 'var(--color-brand-text)' }}>FILTER ISP</label>
              <input type="text" placeholder="e.g. Telecom" value={filterISP} onChange={(e) => setFilterISP(e.target.value)} style={{ padding: '6px 10px', borderRadius: 6, border: '1px solid var(--color-brand-border)', background: 'var(--color-brand-card)', color: 'var(--color-brand-heading)', fontSize: 11 }} />
            </div>

            <div style={{ display: 'flex', flexDirection: 'column', gap: 4 }}>
              <label style={{ fontSize: 9, fontWeight: 700, color: 'var(--color-brand-text)' }}>FILTER COUNTRY</label>
              <input type="text" placeholder="e.g. Germany" value={filterCountry} onChange={(e) => setFilterCountry(e.target.value)} style={{ padding: '6px 10px', borderRadius: 6, border: '1px solid var(--color-brand-border)', background: 'var(--color-brand-card)', color: 'var(--color-brand-heading)', fontSize: 11 }} />
            </div>

            <div style={{ display: 'flex', flexDirection: 'column', gap: 4 }}>
              <label style={{ fontSize: 9, fontWeight: 700, color: 'var(--color-brand-text)' }}>CDN PROVIDER</label>
              <select value={filterCDN} onChange={(e) => setFilterCDN(e.target.value)} style={{ padding: '6px 10px', borderRadius: 6, border: '1px solid var(--color-brand-border)', background: 'var(--color-brand-card)', color: 'var(--color-brand-heading)', fontSize: 11 }}>
                <option value="ALL">All IP Blocks</option>
                <option value="cdn">CDN Registered IPs</option>
                <option value="nocdn">Standard ISP Blocks</option>
              </select>
            </div>
          </div>
        )}

        {/* Dynamic scroll list and table container */}
        <div 
          ref={parentRef}
          style={{
            maxHeight: 520,
            overflowY: 'auto',
            border: '1px solid var(--color-brand-border)',
            borderRadius: 8,
            background: 'var(--color-brand-card)',
            position: 'relative'
          }}
        >
          <table style={{ width: '100%', borderCollapse: 'collapse', textAlign: 'left', fontSize: 12 }}>
            <thead style={{ position: 'sticky', top: 0, background: 'var(--color-brand-bg)', zIndex: 10, boxShadow: 'inset 0 -1px 0 var(--color-brand-border)' }}>
              <tr>
                <th style={{ padding: '12px 10px', width: 40, textAlign: 'center' }}>
                  <input 
                    type="checkbox" 
                    checked={isAllSelected}
                    onChange={handleToggleSelectAll}
                    style={{ accentColor: 'var(--color-brand)', cursor: 'pointer' }}
                  />
                </th>
                <th onClick={() => toggleSort('ip')} style={{ padding: '12px 10px', cursor: 'pointer', fontWeight: 700, color: 'var(--color-brand-muted)' }}>
                  IP ADDRESS <SortIcon column="ip" sortBy={sortBy} sortOrder={sortOrder} />
                </th>
                <th onClick={() => toggleSort('provider_name')} style={{ padding: '12px 10px', cursor: 'pointer', fontWeight: 700, color: 'var(--color-brand-muted)' }}>
                  RESOLVER / DETAILS <SortIcon column="provider_name" sortBy={sortBy} sortOrder={sortOrder} />
                </th>
                <th onClick={() => toggleSort('protocol')} style={{ padding: '12px 10px', cursor: 'pointer', fontWeight: 700, color: 'var(--color-brand-muted)', width: 90 }}>
                  PROTOCOL <SortIcon column="protocol" sortBy={sortBy} sortOrder={sortOrder} />
                </th>
                <th onClick={() => toggleSort('latency_ms')} style={{ padding: '12px 10px', cursor: 'pointer', textAlign: 'center', fontWeight: 700, color: 'var(--color-brand-muted)', width: 85 }}>
                  LATENCY <SortIcon column="latency_ms" sortBy={sortBy} sortOrder={sortOrder} />
                </th>
                <th onClick={() => toggleSort('jitter_ms')} style={{ padding: '12px 10px', cursor: 'pointer', textAlign: 'center', fontWeight: 700, color: 'var(--color-brand-muted)', width: 80 }}>
                  JITTER <SortIcon column="jitter_ms" sortBy={sortBy} sortOrder={sortOrder} />
                </th>
                <th onClick={() => toggleSort('success_rate')} style={{ padding: '12px 10px', cursor: 'pointer', textAlign: 'center', fontWeight: 700, color: 'var(--color-brand-muted)', width: 80 }}>
                  SUCCESS <SortIcon column="success_rate" sortBy={sortBy} sortOrder={sortOrder} />
                </th>
                <th style={{ padding: '12px 10px', textAlign: 'center', fontWeight: 700, color: 'var(--color-brand-muted)', width: 85 }}>CENSOR</th>
                <th style={{ padding: '12px 10px', textAlign: 'center', fontWeight: 700, color: 'var(--color-brand-muted)', width: 80 }}>DNSSEC</th>
                <th style={{ padding: '12px 10px', textAlign: 'center', fontWeight: 700, color: 'var(--color-brand-muted)', width: 85 }}>REBIND</th>
                <th onClick={() => toggleSort('clever_score')} style={{ padding: '12px 10px', cursor: 'pointer', textAlign: 'center', fontWeight: 700, color: 'var(--color-brand-muted)', width: 85 }}>
                  CLEVER <SortIcon column="clever_score" sortBy={sortBy} sortOrder={sortOrder} />
                </th>
                <th style={{ padding: '12px 10px', textAlign: 'center', width: 230 }}>ACTIONS</th>
              </tr>
            </thead>
            <tbody>
              {virtualizer.getVirtualItems()[0]?.start > 0 && (
                <tr style={{ height: virtualizer.getVirtualItems()[0].start }} />
              )}
              {virtualizer.getVirtualItems().map((virtualRow) => {
                const key = filteredKeys[virtualRow.index];
                const resolver = resolvers[key];
                const isApplied = resolver ? checkIsApplied(resolver) : false;
                
                return (
                  <ResolverRow
                    key={key}
                    resolverKey={key}
                    style={{ height: virtualRow.size }}
                    onDeleteSingle={handleDeleteResolver}
                    onApplyResolver={handleApplyResolver}
                    isActiveSystem={isApplied}
                    onOpenTrace={(k) => {
                      setTraceResolverKey(k);
                      setTraceSteps([]);
                      setShowTraceModal(true);
                    }}
                    onOpenAXFR={(k) => {
                      setAxfrResolverKey(k);
                      setAxfrResult(null);
                      setShowAXFRModal(true);
                    }}
                    onOpenAdvancedTest={(k) => {
                      setAdvancedTestResolverKey(k);
                      setSingleTestResult(null);
                      setSingleTestError(null);
                      setShowAdvancedTestModal(true);
                    }}
                    isSelected={selectedKeys.has(key)}
                    onToggleSelect={handleToggleSelect}
                  />
                );
              })}
              {virtualizer.getVirtualItems().length > 0 && (
                <tr
                  style={{
                    height:
                      virtualizer.getTotalSize() -
                      virtualizer.getVirtualItems()[virtualizer.getVirtualItems().length - 1].end,
                  }}
                />
              )}

              {filteredKeys.length === 0 && (
                <tr>
                  <td colSpan={12} style={{ padding: 40, textAlign: 'center', color: 'var(--color-brand-muted)', fontSize: 13 }}>
                    No resolvers match the current search filters. Try launching a sweep.
                  </td>
                </tr>
              )}
            </tbody>
          </table>
        </div>
      </div>

      {/* FETCH PUBLIC DNS DIALOG */}
      <FetchPublicModal
        show={showFetchPublicModal}
        onClose={() => { if (!isFetchingPublic) setShowFetchPublicModal(false); }}
        isFetchingPublic={isFetchingPublic}
        fetchPublicResult={fetchPublicResult}
        selectedPublicSource={selectedPublicSource}
        setSelectedPublicSource={setSelectedPublicSource}
        onFetch={handleFetchPublicDNS}
      />

      {/* ADD CUSTOM RESOLVER DIALOG */}
      <AddResolverModal
        show={showAddModal}
        onClose={() => setShowAddModal(false)}
        modalTab={modalTab}
        setModalTab={setModalTab}
        customIP={customIP}
        setCustomIP={setCustomIP}
        customProvider={customProvider}
        setCustomProvider={setCustomProvider}
        customProtocol={customProtocol}
        setCustomProtocol={setCustomProtocol}
        customCategory={customCategory}
        setCustomCategory={setCustomCategory}
        handleAddCustomResolver={handleAddCustomResolver}
        bulkText={bulkText}
        setBulkText={setBulkText}
        bulkFile={bulkFile}
        setBulkFile={setBulkFile}
        handleBulkImport={handleBulkImport}
        isImporting={isImporting}
      />

      {/* DNS PATH DELEGATION TRACE DIALOG */}
      <TraceModal
        show={showTraceModal}
        onClose={() => setShowTraceModal(false)}
        resolver={activeTraceResolver}
        traceDomain={traceDomain}
        setTraceDomain={setTraceDomain}
        isTracing={isTracing}
        traceSteps={traceSteps}
        onRunTrace={handleRunTrace}
      />

      {/* DNS ZONE TRANSFER AUDITOR (AXFR) */}
      <AXFRModal
        show={showAXFRModal}
        onClose={() => setShowAXFRModal(false)}
        resolver={activeAXFRResolver}
        axfrDomain={axfrDomain}
        setAxfrDomain={setAxfrDomain}
        isTestingAXFR={isTestingAXFR}
        axfrResult={axfrResult}
        onRunAXFR={handleRunAXFR}
      />

      {/* ADVANCED SINGLE TEST PANEL */}
      <AdvancedTestModal
        show={showAdvancedTestModal}
        onClose={() => setShowAdvancedTestModal(false)}
        resolver={activeAdvancedTestResolver}
        advDomain={advDomain}
        setAdvDomain={setAdvDomain}
        advQueryType={advQueryType}
        setAdvQueryType={setAdvQueryType}
        advDNSClass={advDNSClass}
        setAdvDNSClass={setAdvDNSClass}
        advTimeout={advTimeout}
        setAdvTimeout={setAdvTimeout}
        advAttempts={advAttempts}
        setAdvAttempts={setAdvAttempts}
        advExpectResponse={advExpectResponse}
        setAdvExpectResponse={setAdvExpectResponse}
        advCacheBusting={advCacheBusting}
        setAdvCacheBusting={setAdvCacheBusting}
        onExecute={handleRunSingleTest}
        isTestingSingle={isTestingSingle}
        singleTestError={singleTestError}
        singleTestResult={singleTestResult}
      />
      
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
