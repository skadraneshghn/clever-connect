import React, { useState, useEffect, useRef, useCallback } from 'react';
import { useVirtualizer } from '@tanstack/react-virtual';
import { 
  FiSearch, FiGlobe, FiPlus, FiDownload, FiFolderPlus, FiFilter,
  FiTrash2, FiFolder, FiFileText, FiChevronDown, FiChevronUp,
  FiPlay, FiSquare, FiList, FiTrendingUp, FiActivity, FiArrowRight
} from 'react-icons/fi';
import { useDomainStore } from '../../store/domainStore';
import { useAuthStore } from '../../store/authStore';
import { useGeoStore } from '../../store/geoStore';

import { DomainRow } from './components/DomainRow';
import { SidebarCategories } from './components/SidebarCategories';
import { StatsDashboard } from './components/StatsDashboard';
import { ImportDomainsModal } from './components/ImportDomainsModal';

// Sort icon helper
const SortIcon = ({ column, sortBy, sortOrder }: { column: string; sortBy: string; sortOrder: 'asc' | 'desc' }) => {
  if (sortBy !== column) return <FiChevronDown style={{ opacity: 0.3 }} />;
  return sortOrder === 'asc' ? <FiChevronUp style={{ color: 'var(--color-brand)' }} /> : <FiChevronDown style={{ color: 'var(--color-brand)' }} />;
};

export const DomainCheckerPage: React.FC = () => {
  const { token } = useAuthStore();
  const { 
    domains, 
    domainIds, 
    setDomains, 
    appendDomains, 
    updateDomain,
    updateDomainsBulk
  } = useDomainStore();

  const [ws, setWs] = useState<WebSocket | null>(null);

  // Filtering & Pagination States
  const [search, setSearch] = useState('');
  const [selectedCategory, setSelectedCategory] = useState('ALL');
  const [statusFilter, setStatusFilter] = useState('');
  const [tlsFilter, setTlsFilter] = useState('');
  const [httpStatusFilter, setHttpStatusFilter] = useState('');
  const [showAdvancedFilters, setShowAdvancedFilters] = useState(false);

  const [categories, setCategories] = useState<string[]>(['ALL']);
  const [selectedIds, setSelectedIds] = useState<Set<string>>(new Set());

  const [sortBy, setSortBy] = useState<string>('created_at');
  const [sortOrder, setSortOrder] = useState<'asc' | 'desc'>('desc');

  const [page, setPage] = useState(0);
  const [hasMore, setHasMore] = useState(true);
  const [isFetching, setIsFetching] = useState(false);

  // Modals States
  const [isAddModalOpen, setIsAddModalOpen] = useState(false);
  const [isNewCatModalOpen, setIsNewCatModalOpen] = useState(false);
  const [newCatName, setNewCatName] = useState('');

  // Add Modal Internal States
  const [addMethod, setAddMethod] = useState<'text' | 'file'>('text');
  const [rawTextImport, setRawTextImport] = useState('');
  const [importCategory, setImportCategory] = useState('ALL');
  const [isCreatingNewCatInImport, setIsCreatingNewCatInImport] = useState(false);
  const [customImportCat, setCustomImportCat] = useState('');
  const [fileDomains, setFileDomains] = useState<string[]>([]);
  const [fileParsedCount, setFileParsedCount] = useState(0);
  const [isImporting, setIsImporting] = useState(false);
  const fileInputRef = useRef<HTMLInputElement>(null);

  // Runner Options States
  const [runnerThreads, setRunnerThreads] = useState(50);
  const [runnerTimeout, setRunnerTimeout] = useState(3);

  // Bulk Progress Tracking States
  const [isCheckingAll, setIsCheckingAll] = useState(false);
  const [checkedAllCount, setCheckedAllCount] = useState(0);
  const [totalAllToCheck, setTotalAllToCheck] = useState(0);

  const parentRef = useRef<HTMLDivElement>(null);

  // Real database-wide category stats
  const [dbStats, setDbStats] = useState({
    total: 0,
    online: 0,
    offline: 0,
    checking: 0,
    ssl_valid: 0
  });

  const isChecking = dbStats.checking > 0;

  const fetchCategories = async () => {
    try {
      const activeToken = token || localStorage.getItem('cc_client_token') || '';
      const res = await fetch('/api/domains/categories', {
        headers: { 'Authorization': `Bearer ${activeToken}` }
      });
      if (res.ok) {
        const data = await res.json();
        setCategories(data.categories || ['ALL']);
      }
    } catch (e) {
      console.error(e);
    }
  };

  const fetchDomains = async (pageNum = 0, reset = false) => {
    if (isFetching || (!hasMore && !reset)) return;
    setIsFetching(true);
    try {
      const activeToken = token || localStorage.getItem('cc_client_token') || '';
      const limit = 100;

      let url = `/api/domains?limit=${limit}&offset=${pageNum * limit}&sortBy=${sortBy}&sortOrder=${sortOrder}`;
      if (selectedCategory && selectedCategory !== 'ALL') {
        url += `&category=${encodeURIComponent(selectedCategory)}`;
      }
      if (search) {
        url += `&search=${encodeURIComponent(search)}`;
      }
      if (statusFilter) {
        url += `&status=${encodeURIComponent(statusFilter)}`;
      }
      if (tlsFilter) {
        url += `&tlsFilter=${encodeURIComponent(tlsFilter)}`;
      }
      if (httpStatusFilter) {
        url += `&httpStatus=${encodeURIComponent(httpStatusFilter)}`;
      }

      const res = await fetch(url, {
        headers: { 'Authorization': `Bearer ${activeToken}` }
      });
      if (res.ok) {
        const data = await res.json();
        const incoming = data.domains || [];
        if (reset) {
          setDomains(incoming);
        } else {
          appendDomains(incoming);
        }
        setHasMore(incoming.length === limit);
        setPage(pageNum);

        if (data.stats) {
          setDbStats(data.stats);
        }
      }
    } catch (e) {
      console.error('Failed to fetch domains', e);
    } finally {
      setIsFetching(false);
    }
  };

  useEffect(() => {
    fetchDomains(0, true);
    fetchCategories();
  }, [sortBy, sortOrder, selectedCategory, search, statusFilter, tlsFilter, httpStatusFilter]);

  useEffect(() => {
    const protocol = window.location.protocol === 'https:' ? 'wss:' : 'ws:';
    const activeToken = token || localStorage.getItem('cc_client_token') || '';
    const wsUrl = `${protocol}//${window.location.host}/ws?token=${activeToken}`;

    const socket = new WebSocket(wsUrl);

    socket.onopen = () => console.log('Domain checker WS connected');

    socket.onmessage = (event) => {
      try {
        const msg = JSON.parse(event.data);
        if (msg.type === 'DOMAIN_CHECK_RESULT' && msg.data) {
          const oldDomain = useDomainStore.getState().domains[msg.data.id];
          updateDomain(msg.data);

          if (msg.data.status !== 'checking' && msg.data.status !== 'pending') {
            setCheckedAllCount(prev => prev + 1);
          }

          setDbStats(prev => {
            const next = { ...prev };
            if (oldDomain) {
              if (oldDomain.status === 'online') next.online = Math.max(0, next.online - 1);
              else if (['offline', 'timeout', 'nxdomain'].includes(oldDomain.status)) next.offline = Math.max(0, next.offline - 1);
              else if (oldDomain.status === 'checking') next.checking = Math.max(0, next.checking - 1);

              if (oldDomain.tls_status) next.ssl_valid = Math.max(0, next.ssl_valid - 1);
            } else {
              next.checking = Math.max(0, next.checking - 1);
            }

            if (msg.data.status === 'online') next.online++;
            else if (['offline', 'timeout', 'nxdomain'].includes(msg.data.status)) next.offline++;
            else if (msg.data.status === 'checking') next.checking++;

            if (msg.data.tls_status) next.ssl_valid++;

            return next;
          });
        }
        if (msg.type === 'GEO_RESOLVED' && msg.data) {
          useGeoStore.getState().updateGeoInfo(msg.data);
        }
      } catch (err) {
        // ignore
      }
    };

    setWs(socket);

    return () => socket.close();
  }, [token]);

  useEffect(() => {
    if (isCheckingAll && totalAllToCheck > 0 && checkedAllCount >= totalAllToCheck) {
      const timer = setTimeout(() => {
        setIsCheckingAll(false);
        setCheckedAllCount(0);
        setTotalAllToCheck(0);
      }, 5000);
      return () => clearTimeout(timer);
    }
  }, [checkedAllCount, totalAllToCheck, isCheckingAll]);

  const virtualizer = useVirtualizer({
    count: domainIds.length,
    getScrollElement: () => parentRef.current,
    estimateSize: () => 40,
    overscan: 10,
  });

  // Infinite scroll
  useEffect(() => {
    const [lastItem] = [...virtualizer.getVirtualItems()].reverse();
    if (!lastItem) return;

    if (lastItem.index >= domainIds.length - 1 && hasMore && !isFetching) {
      fetchDomains(page + 1, false);
    }
  }, [virtualizer.getVirtualItems(), hasMore, isFetching, domainIds.length, page]);

  const toggleSelectAll = () => {
    if (selectedIds.size === domainIds.length) {
      setSelectedIds(new Set());
    } else {
      setSelectedIds(new Set(domainIds));
    }
  };

  const toggleSelect = useCallback((id: string) => {
    setSelectedIds(prev => {
      const next = new Set(prev);
      if (next.has(id)) next.delete(id);
      else next.add(id);
      return next;
    });
  }, []);

  const handleFileChange = (e: React.ChangeEvent<HTMLInputElement>) => {
    const file = e.target.files?.[0];
    if (!file) return;

    const reader = new FileReader();
    reader.onload = (event) => {
      const text = event.target?.result as string;
      if (!text) return;

      let parsedDomains: string[] = [];
      if (file.name.endsWith('.csv')) {
        const lines = text.split(/\r?\n/);
        if (lines.length > 0) {
          const headers = lines[0].split(',').map(h => h.trim().toLowerCase());
          let domainColIndex = headers.findIndex(h => h.includes('domain') || h.includes('host') || h.includes('url'));
          if (domainColIndex === -1) domainColIndex = 0;

          for (let i = 1; i < lines.length; i++) {
            const row = lines[i].split(',');
            if (row.length > domainColIndex) {
              const domainVal = row[domainColIndex].replace(/["']/g, '').trim();
              if (domainVal) parsedDomains.push(domainVal);
            }
          }
        }
      } else {
        parsedDomains = text.split(/\r?\n/).map(line => line.trim()).filter(Boolean);
      }

      setFileParsedCount(parsedDomains.length);
      setFileDomains(parsedDomains);
    };
    reader.readAsText(file);
  };

  const handleImportSubmit = async () => {
    let list: string[] = [];
    if (addMethod === 'text') {
      list = rawTextImport.split('\n').map(s => s.trim()).filter(Boolean);
    } else {
      list = fileDomains;
    }

    if (!list.length) return;

    const targetCategory = isCreatingNewCatInImport ? customImportCat.trim() : importCategory;
    if (!targetCategory) return;

    setIsImporting(true);
    try {
      const activeToken = token || localStorage.getItem('cc_client_token') || '';
      const res = await fetch('/api/domains', {
        method: 'POST',
        headers: {
          'Authorization': `Bearer ${activeToken}`,
          'Content-Type': 'application/json'
        },
        body: JSON.stringify({
          domains: list,
          category: targetCategory
        })
      });
      if (res.ok) {
        setRawTextImport('');
        setFileDomains([]);
        setFileParsedCount(0);
        setIsAddModalOpen(false);
        setIsCreatingNewCatInImport(false);
        setCustomImportCat('');
        fetchCategories();
        fetchDomains(0, true);
      }
    } catch (err) {
      console.error('Domain import error:', err);
    } finally {
      setIsImporting(false);
    }
  };

  const handleCheckSingle = useCallback(async (id: string) => {
    const domain = domains[id];
    if (!domain) return;
    updateDomain({ ...domain, status: 'checking' });

    const activeToken = token || localStorage.getItem('cc_client_token') || '';
    await fetch(`/api/domains/check/${id}`, {
      method: 'POST',
      headers: { 'Authorization': `Bearer ${activeToken}` }
    });
  }, [domains, updateDomain, token]);

  const handleCheckSelected = async () => {
    if (selectedIds.size === 0) return;
    const activeToken = token || localStorage.getItem('cc_client_token') || '';

    const idsArray = Array.from(selectedIds);
    setCheckedAllCount(0);
    setTotalAllToCheck(idsArray.length);
    setIsCheckingAll(true);

    // Visually set checking state (HIGH PERFORMANCE BATCH UPDATE)
    const updates: Record<string, Partial<any>> = {};
    idsArray.forEach(id => {
      updates[id] = { status: 'checking' };
    });
    updateDomainsBulk(updates);

    await fetch('/api/domains/check/bulk', {
      method: 'POST',
      headers: {
        'Authorization': `Bearer ${activeToken}`,
        'Content-Type': 'application/json'
      },
      body: JSON.stringify({
        ids: idsArray,
        threads: runnerThreads,
        timeout: runnerTimeout
      })
    });

    setSelectedIds(new Set());
  };

  const handleCheckCategory = async () => {
    const activeToken = token || localStorage.getItem('cc_client_token') || '';

    setCheckedAllCount(0);
    setTotalAllToCheck(0);
    setIsCheckingAll(true);

    // Visually set stats to checking state instantly
    setDbStats(prev => ({
      total: prev.total,
      online: 0,
      offline: 0,
      checking: prev.total,
      ssl_valid: 0
    }));

    const res = await fetch('/api/domains/check/bulk', {
      method: 'POST',
      headers: {
        'Authorization': `Bearer ${activeToken}`,
        'Content-Type': 'application/json'
      },
      body: JSON.stringify({
        ids: [],
        category: selectedCategory,
        threads: runnerThreads,
        timeout: runnerTimeout
      })
    });

    if (res.ok) {
      const data = await res.json();
      if (data && data.count) {
        setTotalAllToCheck(data.count);
      }
    }
  };

  const handleDeleteSingle = useCallback(async (id: string) => {
    if (!window.confirm("Are you sure you want to delete this domain?")) return;

    const activeToken = token || localStorage.getItem('cc_client_token') || '';
    const res = await fetch(`/api/domains/${id}`, {
      method: 'DELETE',
      headers: { 'Authorization': `Bearer ${activeToken}` }
    });
    if (res.ok) {
      useDomainStore.getState().removeDomain(id);
    }
  }, [token]);

  const handleDeleteSelected = async () => {
    if (selectedIds.size === 0) return;
    if (!window.confirm(`Are you sure you want to delete ${selectedIds.size} selected domains?`)) return;

    const idsArray = Array.from(selectedIds);
    const activeToken = token || localStorage.getItem('cc_client_token') || '';
    const res = await fetch('/api/domains/delete', {
      method: 'POST',
      headers: {
        'Authorization': `Bearer ${activeToken}`,
        'Content-Type': 'application/json'
      },
      body: JSON.stringify({ ids: idsArray })
    });
    if (res.ok) {
      useDomainStore.getState().removeDomainsBulk(idsArray);
      setSelectedIds(new Set());
    }
  };

  const handleDeleteCategory = async () => {
    if (!window.confirm(`Are you sure you want to delete all domains in category "${selectedCategory}"?`)) return;

    const activeToken = token || localStorage.getItem('cc_client_token') || '';
    const res = await fetch('/api/domains/delete', {
      method: 'POST',
      headers: {
        'Authorization': `Bearer ${activeToken}`,
        'Content-Type': 'application/json'
      },
      body: JSON.stringify({
        category: selectedCategory,
        all: true
      })
    });
    if (res.ok) {
      fetchCategories();
      fetchDomains(0, true);
      setSelectedIds(new Set());
    }
  };

  const handleCreateCategory = () => {
    if (!newCatName.trim()) return;
    const name = newCatName.trim();
    if (!categories.includes(name)) {
      setCategories([...categories, name]);
    }
    setSelectedCategory(name);
    setNewCatName('');
    setIsNewCatModalOpen(false);
  };

  // Export functions
  const exportToCSV = () => {
    const list = domainIds.map(id => domains[id]).filter(Boolean);
    const headers = ['Domain Name', 'Status', 'Category', 'IP Addresses', 'HTTP Status', 'Latency (ms)', 'TLS Valid', 'TLS Expiry (Days)'];
    const rows = list.map(d => [
      d.domain_name,
      d.status,
      d.category,
      d.ip_addresses || '',
      d.http_status || '',
      d.latency_ms || '',
      d.tls_status ? 'YES' : 'NO',
      d.tls_expiry_days || ''
    ]);

    const csvContent = "data:text/csv;charset=utf-8,"
      + [headers.join(','), ...rows.map(e => e.map(val => `"${val}"`).join(","))].join("\n");

    const encodedUri = encodeURI(csvContent);
    const link = document.createElement("a");
    link.setAttribute("href", encodedUri);
    link.setAttribute("download", `domains_${selectedCategory.toLowerCase()}.csv`);
    document.body.appendChild(link);
    link.click();
    document.body.removeChild(link);
  };

  const exportToTXT = () => {
    const list = domainIds.map(id => domains[id]).filter(Boolean);
    const textContent = list.map(d => d.domain_name).join('\n');

    const element = document.createElement("a");
    const file = new Blob([textContent], { type: 'text/plain' });
    element.href = URL.createObjectURL(file);
    element.download = `domains_${selectedCategory.toLowerCase()}.txt`;
    document.body.appendChild(element);
    element.click();
    document.body.removeChild(element);
  };

  const toggleSort = (column: string) => {
    if (sortBy === column) {
      setSortOrder(sortOrder === 'asc' ? 'desc' : 'asc');
    } else {
      setSortBy(column);
      setSortOrder('desc');
    }
  };

  return (
    <div className="page-container animate-fade-in" style={{ padding: '4px 0', fontFamily: 'var(--font-sans)', display: 'flex', gap: 20, minHeight: 'calc(100vh - 120px)' }}>
      <style>{`
        @keyframes pulse-row {
          0% { background-color: rgba(255, 107, 44, 0.01); }
          50% { background-color: rgba(255, 107, 44, 0.08); }
          100% { background-color: rgba(255, 107, 44, 0.01); }
        }
        .pulse-testing {
          animation: pulse-row 1.8s infinite ease-in-out;
        }
        .shimmer-text {
          background: linear-gradient(90deg, #ff6b2c 0%, #3b82f6 50%, #ff6b2c 100%);
          background-size: 200% auto;
          color: transparent;
          background-clip: text;
          -webkit-background-clip: text;
          animation: shine 1.5s linear infinite;
        }
        @keyframes shine {
          to { background-position: 200% center; }
        }
        @keyframes radar-sweep {
          from {
            transform: rotate(0deg);
          }
          to {
            transform: rotate(360deg);
          }
        }
        .animate-radar-sweep {
          animation: radar-sweep 2.5s linear infinite;
        }
        .clip-radar {
          clip-path: polygon(0 100%, 100% 100%, 100% 0);
        }
      `}</style>

      {/* LEFT SIDEBAR: Categories list */}
      <SidebarCategories
        categories={categories}
        selectedCategory={selectedCategory}
        setSelectedCategory={setSelectedCategory}
        onAddCategoryClick={() => setIsNewCatModalOpen(true)}
      />

      {/* RIGHT CONTENT: Toolbar, Filters & Table */}
      <div style={{ flex: 1, display: 'flex', flexDirection: 'column', gap: 20, minWidth: 0 }}>

        {/* Header toolbar */}
        <div style={{ display: 'flex', justifyContent: 'space-between', alignItems: 'center', gap: 16 }}>
          <div>
            <h1 style={{ fontSize: 22, fontWeight: 700, color: 'var(--color-brand-heading)', margin: 0 }}>
              Domain Orchestrator
            </h1>
            <p style={{ fontSize: 12, color: 'var(--color-brand-text)', margin: '4px 0 0' }}>
              Real-time TLS/DNS/TCP network telemetry and health verification
            </p>
          </div>

          <div style={{ display: 'flex', alignItems: 'center', gap: 10 }}>
            {/* Runner Config panel */}
            <div style={{ display: 'flex', alignItems: 'center', gap: 8, background: 'var(--color-brand-card)', border: '1px solid var(--color-brand-border)', padding: '5px 10px', borderRadius: 8 }}>
              <span style={{ fontSize: 11, fontWeight: 700, color: 'var(--color-brand-text)', textTransform: 'uppercase', letterSpacing: '0.5px' }}>Runner Options</span>

              <div style={{ display: 'flex', alignItems: 'center', gap: 4, borderLeft: '1px solid var(--color-brand-border)', paddingLeft: 8 }}>
                <span style={{ fontSize: 10, color: 'var(--color-brand-muted)', fontWeight: 600 }}>THREADS:</span>
                <input
                  type="number"
                  min="1"
                  max="1000"
                  value={runnerThreads}
                  onChange={(e) => setRunnerThreads(Math.max(1, parseInt(e.target.value) || 50))}
                  style={{
                    width: 50,
                    padding: '2px 4px',
                    borderRadius: 4,
                    border: '1px solid var(--color-brand-border)',
                    background: 'var(--color-brand-bg)',
                    color: 'var(--color-brand-heading)',
                    fontSize: 11,
                    fontWeight: 700,
                    textAlign: 'center'
                  }}
                />
              </div>

              <div style={{ display: 'flex', alignItems: 'center', gap: 4, borderLeft: '1px solid var(--color-brand-border)', paddingLeft: 8 }}>
                <span style={{ fontSize: 10, color: 'var(--color-brand-muted)', fontWeight: 600 }}>TIMEOUT:</span>
                <select
                  value={runnerTimeout}
                  onChange={(e) => setRunnerTimeout(parseInt(e.target.value) || 3)}
                  style={{
                    padding: '2px 4px',
                    borderRadius: 4,
                    border: '1px solid var(--color-brand-border)',
                    background: 'var(--color-brand-bg)',
                    color: 'var(--color-brand-heading)',
                    fontSize: 11,
                    fontWeight: 700
                  }}
                >
                  <option value="1">1s</option>
                  <option value="2">2s</option>
                  <option value="3">3s</option>
                  <option value="5">5s</option>
                  <option value="10">10s</option>
                </select>
              </div>
            </div>

            <button
              className="btn btn--secondary btn--sm"
              onClick={() => setIsAddModalOpen(true)}
              style={{ display: 'flex', alignItems: 'center', gap: 6 }}
            >
              <FiPlus /> Add Domains
            </button>

            <button
              className="btn btn--primary btn--sm"
              onClick={handleCheckCategory}
              style={{ display: 'flex', alignItems: 'center', gap: 6 }}
            >
              <FiPlay /> Test Category ({selectedCategory})
            </button>

            <div style={{ position: 'relative', display: 'inline-block' }}>
              <button
                className="btn btn--secondary btn--sm"
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
                style={{ display: 'flex', alignItems: 'center', gap: 6 }}
              >
                <FiDownload /> Export
              </button>
              <div
                style={{
                  display: 'none',
                  position: 'absolute',
                  right: 0,
                  top: '100%',
                  marginTop: 4,
                  background: 'var(--color-brand-card)',
                  border: '1px solid var(--color-brand-border)',
                  borderRadius: 8,
                  boxShadow: '0 4px 12px rgba(0,0,0,0.1)',
                  zIndex: 10,
                  width: 140
                }}
              >
                <button
                  onClick={exportToTXT}
                  style={{ display: 'block', width: '100%', padding: '8px 12px', border: 'none', background: 'none', textAlign: 'left', fontSize: 12, cursor: 'pointer', color: 'var(--color-brand-text)' }}
                >
                  Export TXT List
                </button>
                <button
                  onClick={exportToCSV}
                  style={{ display: 'block', width: '100%', padding: '8px 12px', border: 'none', background: 'none', textAlign: 'left', fontSize: 12, cursor: 'pointer', color: 'var(--color-brand-text)', borderTop: '1px solid var(--color-brand-border)' }}
                >
                  Export CSV File
                </button>
              </div>
            </div>
          </div>
        </div>

        {/* Telemetry Card */}
        <StatsDashboard
          dbStats={dbStats}
          isChecking={isChecking}
          isCheckingAll={isCheckingAll}
          checkedAllCount={checkedAllCount}
          totalAllToCheck={totalAllToCheck}
        />

        {/* Filter bar card */}
        <div className="g-card" style={{ padding: 16, display: 'flex', flexDirection: 'column', gap: 12 }}>
          <div style={{ display: 'flex', justifyContent: 'space-between', alignItems: 'center', gap: 16 }}>
            <div style={{ position: 'relative', width: 300 }}>
              <FiSearch style={{ position: 'absolute', left: 10, top: '50%', transform: 'translateY(-50%)', color: 'var(--color-brand-muted)', fontSize: 13 }} />
              <input
                type="text"
                placeholder="Search domain or IP address..."
                value={search}
                onChange={(e) => setSearch(e.target.value)}
                style={{
                  width: '100%',
                  padding: '8px 10px 8px 32px',
                  borderRadius: 8,
                  border: '1px solid var(--color-brand-border)',
                  background: 'var(--color-brand-bg)',
                  fontSize: 13,
                  color: 'var(--color-brand-heading)',
                  outline: 'none'
                }}
              />
            </div>

            <div style={{ display: 'flex', gap: 10, alignItems: 'center' }}>
              <button
                onClick={() => setShowAdvancedFilters(!showAdvancedFilters)}
                className={`btn btn--sm ${showAdvancedFilters ? 'btn--primary' : 'btn--secondary'}`}
                style={{ display: 'flex', alignItems: 'center', gap: 6 }}
              >
                <FiFilter /> Filters
              </button>

              {selectedIds.size > 0 ? (
                <>
                  <button
                    onClick={handleCheckSelected}
                    className="btn btn--primary btn--sm"
                    style={{ display: 'flex', alignItems: 'center', gap: 6 }}
                  >
                    <FiPlay /> Check Selected ({selectedIds.size})
                  </button>
                  <button
                    onClick={handleDeleteSelected}
                    className="btn btn--secondary btn--sm"
                    style={{ display: 'flex', alignItems: 'center', gap: 6, color: 'var(--color-brand-red)', borderColor: 'rgba(239, 68, 68, 0.2)' }}
                  >
                    <FiTrash2 /> Delete Selected ({selectedIds.size})
                  </button>
                </>
              ) : (
                <button
                  onClick={handleDeleteCategory}
                  className="btn btn--secondary btn--sm"
                  style={{ display: 'flex', alignItems: 'center', gap: 6, color: 'var(--color-brand-red)', borderColor: 'rgba(239, 68, 68, 0.2)' }}
                >
                  <FiTrash2 /> Delete Category
                </button>
              )}
            </div>
          </div>

          {/* Advanced Filters Panel */}
          {showAdvancedFilters && (
            <div style={{
              display: 'grid',
              gridTemplateColumns: 'repeat(auto-fit, minmax(180px, 1fr))',
              gap: 12,
              paddingTop: 12,
              borderTop: '1px solid var(--color-brand-border)',
              animation: 'fadeIn 0.2s ease-out'
            }}>
              {/* Status Filter */}
              <div style={{ display: 'flex', flexDirection: 'column', gap: 4 }}>
                <label style={{ fontSize: 11, fontWeight: 600, color: 'var(--color-brand-muted)' }}>Ping Status</label>
                <select
                  value={statusFilter}
                  onChange={(e) => setStatusFilter(e.target.value)}
                  style={{
                    padding: '6px 10px',
                    borderRadius: 6,
                    border: '1px solid var(--color-brand-border)',
                    background: 'var(--color-brand-bg)',
                    fontSize: 12,
                    color: 'var(--color-brand-heading)'
                  }}
                >
                  <option value="">All Statuses</option>
                  <option value="online">Online</option>
                  <option value="offline">Offline</option>
                  <option value="timeout">Timeout</option>
                  <option value="nxdomain">NXDomain</option>
                  <option value="pending">Pending</option>
                  <option value="checking">Checking</option>
                </select>
              </div>

              {/* TLS Status Filter */}
              <div style={{ display: 'flex', flexDirection: 'column', gap: 4 }}>
                <label style={{ fontSize: 11, fontWeight: 600, color: 'var(--color-brand-muted)' }}>SSL/TLS Validity</label>
                <select
                  value={tlsFilter}
                  onChange={(e) => setTlsFilter(e.target.value)}
                  style={{
                    padding: '6px 10px',
                    borderRadius: 6,
                    border: '1px solid var(--color-brand-border)',
                    background: 'var(--color-brand-bg)',
                    fontSize: 12,
                    color: 'var(--color-brand-heading)'
                  }}
                >
                  <option value="">All SSL States</option>
                  <option value="valid">Valid Certificate</option>
                  <option value="invalid">Invalid Certificate</option>
                  <option value="expired">Expired Certificate</option>
                </select>
              </div>

              {/* HTTP Status Code Filter */}
              <div style={{ display: 'flex', flexDirection: 'column', gap: 4 }}>
                <label style={{ fontSize: 11, fontWeight: 600, color: 'var(--color-brand-muted)' }}>HTTP Status Filter</label>
                <select
                  value={httpStatusFilter}
                  onChange={(e) => setHttpStatusFilter(e.target.value)}
                  style={{
                    padding: '6px 10px',
                    borderRadius: 6,
                    border: '1px solid var(--color-brand-border)',
                    background: 'var(--color-brand-bg)',
                    fontSize: 12,
                    color: 'var(--color-brand-heading)'
                  }}
                >
                  <option value="">All HTTP Codes</option>
                  <option value="200">200 OK</option>
                  <option value="301">301 Moved</option>
                  <option value="302">302 Found</option>
                  <option value="400">400 Bad Request</option>
                  <option value="403">403 Forbidden</option>
                  <option value="404">404 Not Found</option>
                  <option value="500">500 Server Error</option>
                  <option value="502">502 Bad Gateway</option>
                </select>
              </div>
            </div>
          )}
        </div>

        {/* Table container Card */}
        <div className="g-card" style={{ padding: 0, overflow: 'hidden' }}>
          <div ref={parentRef} style={{ maxHeight: 600, overflow: 'auto', border: '1px solid var(--color-brand-border)', borderRadius: 8 }}>
            <table style={{ width: '100%', borderCollapse: 'collapse', fontSize: 12, textAlign: 'left' }}>
              <thead style={{ position: 'sticky', top: 0, zIndex: 1, background: 'var(--color-brand-bg)' }}>
                <tr style={{ borderBottom: '1px solid var(--color-brand-border)' }}>
                  <th style={{ padding: '10px 12px', color: 'var(--color-brand-heading)', width: 50, textAlign: 'center' }}>
                    <input
                      type="checkbox"
                      style={{ cursor: 'pointer', accentColor: 'var(--color-brand)', transform: 'scale(1.1)' }}
                      checked={domainIds.length > 0 && domainIds.every(id => selectedIds.has(id))}
                      onChange={toggleSelectAll}
                    />
                  </th>
                  <th style={{ padding: '10px 12px', color: 'var(--color-brand-heading)', cursor: 'pointer' }} onClick={() => toggleSort('domain_name')}>
                    <div style={{ display: 'flex', alignItems: 'center', gap: 4 }}>
                      Domain <SortIcon column="domain_name" sortBy={sortBy} sortOrder={sortOrder} />
                    </div>
                  </th>
                  <th style={{ padding: '10px 12px', color: 'var(--color-brand-heading)', width: 120, cursor: 'pointer' }} onClick={() => toggleSort('status')}>
                    <div style={{ display: 'flex', alignItems: 'center', gap: 4 }}>
                      Status <SortIcon column="status" sortBy={sortBy} sortOrder={sortOrder} />
                    </div>
                  </th>
                  <th style={{ padding: '10px 12px', color: 'var(--color-brand-heading)' }}>IP Addresses</th>
                  <th style={{ padding: '10px 12px', color: 'var(--color-brand-heading)', width: 100, textAlign: 'center', cursor: 'pointer' }} onClick={() => toggleSort('latency_ms')}>
                    <div style={{ display: 'flex', alignItems: 'center', justifyContent: 'center', gap: 4 }}>
                      Latency <SortIcon column="latency_ms" sortBy={sortBy} sortOrder={sortOrder} />
                    </div>
                  </th>
                  <th style={{ padding: '10px 12px', color: 'var(--color-brand-heading)', width: 120, textAlign: 'center', cursor: 'pointer' }} onClick={() => toggleSort('tls_expiry_days')}>
                    <div style={{ display: 'flex', alignItems: 'center', justifyContent: 'center', gap: 4 }}>
                      SSL/TLS <SortIcon column="tls_expiry_days" sortBy={sortBy} sortOrder={sortOrder} />
                    </div>
                  </th>
                  <th style={{ padding: '10px 12px', color: 'var(--color-brand-heading)', width: 90, textAlign: 'center', cursor: 'pointer' }} onClick={() => toggleSort('http_status')}>
                    <div style={{ display: 'flex', alignItems: 'center', justifyContent: 'center', gap: 4 }}>
                      HTTP <SortIcon column="http_status" sortBy={sortBy} sortOrder={sortOrder} />
                    </div>
                  </th>
                  <th style={{ padding: '10px 12px', color: 'var(--color-brand-heading)', width: 80, textAlign: 'center' }}>Actions</th>
                </tr>
              </thead>
              <tbody>
                {virtualizer.getVirtualItems()[0]?.start > 0 && (
                  <tr style={{ height: virtualizer.getVirtualItems()[0].start }} />
                )}
                {virtualizer.getVirtualItems().map((virtualRow) => {
                  const id = domainIds[virtualRow.index];
                  return (
                    <DomainRow
                      key={id}
                      domainId={id}
                      onCheckSingle={handleCheckSingle}
                      onDeleteSingle={handleDeleteSingle}
                      isChecking={isChecking}
                      isSelected={selectedIds.has(id)}
                      onToggleSelect={toggleSelect}
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

                {domainIds.length === 0 && (
                  <tr>
                    <td colSpan={8} style={{ padding: 20, textAlign: 'center', color: 'var(--color-brand-muted)' }}>
                      No domains added in this filter context. Click "Add Domains" to import.
                    </td>
                  </tr>
                )}
              </tbody>
            </table>
          </div>
        </div>
      </div>

      {/* BULK IMPORT MODAL */}
      <ImportDomainsModal
        isOpen={isAddModalOpen}
        onClose={() => !isImporting && setIsAddModalOpen(false)}
        categories={categories}
        importCategory={importCategory}
        setImportCategory={setImportCategory}
        isCreatingNewCatInImport={isCreatingNewCatInImport}
        setIsCreatingNewCatInImport={setIsCreatingNewCatInImport}
        customImportCat={customImportCat}
        setCustomImportCat={setCustomImportCat}
        addMethod={addMethod}
        setAddMethod={setAddMethod}
        rawTextImport={rawTextImport}
        setRawTextImport={setRawTextImport}
        fileDomains={fileDomains}
        fileParsedCount={fileParsedCount}
        fileInputRef={fileInputRef}
        handleFileChange={handleFileChange}
        handleImportSubmit={handleImportSubmit}
        isImporting={isImporting}
      />

      {/* CREATE CATEGORY MODAL */}
      {isNewCatModalOpen && (
        <div
          style={{
            position: 'fixed',
            top: 0, left: 0, width: '100%', height: '100%',
            background: 'rgba(0,0,0,0.5)',
            display: 'flex', alignItems: 'center', justifyContent: 'center',
            zIndex: 999,
          }}
          onClick={() => setIsNewCatModalOpen(false)}
        >
          <div
            style={{
              background: 'var(--color-brand-card)',
              padding: 24,
              borderRadius: 12,
              width: 360,
              maxWidth: '90%',
              boxShadow: '0 10px 25px rgba(0,0,0,0.15)',
              display: 'flex',
              flexDirection: 'column',
              gap: 16
            }}
            onClick={(e) => e.stopPropagation()}
          >
            <h3 style={{ fontSize: 14, fontWeight: 700, color: 'var(--color-brand-heading)', margin: 0 }}>
              Create New Category
            </h3>
            <input
              type="text"
              placeholder="Category name..."
              value={newCatName}
              onChange={(e) => setNewCatName(e.target.value)}
              style={{
                width: '100%',
                padding: '8px 10px',
                borderRadius: 8,
                border: '1px solid var(--color-brand-border)',
                background: 'var(--color-brand-bg)',
                fontSize: 13,
                color: 'var(--color-brand-heading)',
                outline: 'none'
              }}
            />
            <div style={{ display: 'flex', justifyContent: 'flex-end', gap: 10 }}>
              <button
                type="button"
                className="btn btn--secondary btn--sm"
                onClick={() => setIsNewCatModalOpen(false)}
              >
                Cancel
              </button>
              <button
                type="button"
                className="btn btn--primary btn--sm"
                onClick={handleCreateCategory}
              >
                Create
              </button>
            </div>
          </div>
        </div>
      )}
    </div>
  );
};
