import React, { useState, useEffect, useRef } from 'react';
import { 
  FiRefreshCw, FiPlay, FiSquare, FiDownload, FiUpload, FiSettings, 
  FiSearch, FiClipboard, FiAlertCircle, FiCheckCircle, FiFileText, FiList,
  FiTerminal, FiTrash2, FiGlobe, FiSliders, FiActivity, FiPlus, FiChevronDown, FiChevronUp,
  FiPlusCircle, FiXCircle
} from 'react-icons/fi';
import { useAuthStore } from '../store/authStore';
import { IPResolveBadge } from '../components/atoms/IPResolveBadge';
import { useGeoStore } from '../store/geoStore';

interface ScannedCandidate {
  ip: string;
  port: number;
  protocol: string;
  latencyMs: number;
  speedMbps: number;
  packetLoss: number;
  cdnProvider: string;
  popLocation: string;
  status: 'healthy' | 'failed' | 'in_flight';
  time: string;
}

interface ScannerSource {
  id: number;
  name: string;
  url: string;
  type: 'cidr' | 'proxyip' | 'domain';
  is_enabled: boolean;
}

const COMMON_PORTS = [
  { value: 443, label: '443 (HTTPS)' },
  { value: 2053, label: '2053' },
  { value: 2083, label: '2083' },
  { value: 2087, label: '2087' },
  { value: 2096, label: '2096' },
  { value: 8443, label: '8443' },
  { value: 80, label: '80 (HTTP)' },
  { value: 2052, label: '2052' },
  { value: 2082, label: '2082' },
  { value: 2086, label: '2086' },
  { value: 2095, label: '2095' },
  { value: 8080, label: '8080' }
];

const ToggleSwitch: React.FC<{ checked: boolean; onChange: () => void }> = ({ checked, onChange }) => {
  return (
    <button
      type="button"
      onClick={onChange}
      className={`relative inline-flex h-5 w-9 shrink-0 cursor-pointer rounded-full border-2 border-transparent transition-colors duration-200 ease-in-out focus:outline-none ${
        checked ? 'bg-[var(--color-brand)]' : 'bg-zinc-700'
      }`}
    >
      <span
        className={`pointer-events-none inline-block h-4 w-4 transform rounded-full bg-white shadow ring-0 transition duration-200 ease-in-out ${
          checked ? 'translate-x-4' : 'translate-x-0'
        }`}
      />
    </button>
  );
};

export const NetworkToolsPage: React.FC = () => {
  const { token } = useAuthStore();
  const [ws, setWs] = useState<WebSocket | null>(null);
  const [wsConnected, setWsConnected] = useState(false);
  const [isScanning, setIsScanning] = useState(false);

  // Scanner state metrics
  const [stats, setStats] = useState({
    tested: 0,
    healthy: 0,
    failed: 0,
    in_flight: 0,
    total_targets: 0,
    remaining_sec: 0,
    phase: '',
  });

  // Candidates list
  const [candidates, setCandidates] = useState<ScannedCandidate[]>([]);
  const [searchQuery, setSearchQuery] = useState('');
  const [sortBy, setSortBy] = useState<'latency' | 'speed' | 'time'>('latency');

  // Logs state
  const [scannerLogs, setScannerLogs] = useState<string[]>([]);
  const [logsFilter, setLogsFilter] = useState('');

  // Port Selection state
  const [selectedPortsList, setSelectedPortsList] = useState<number[]>([443, 2053, 2083, 2087, 2096, 8443]);
  const [customPorts, setCustomPorts] = useState('');

  // Advanced Input states
  const [rawConfigLink, setRawConfigLink] = useState('');
  const [targetCidrs, setTargetCidrs] = useState('108.162.192.0/18\n103.21.244.0/22');
  const [concurrencyLimit, setConcurrencyLimit] = useState(100);
  const [maxRateLimit, setMaxRateLimit] = useState(0);
  const [networkTimeoutMs, setNetworkTimeoutMs] = useState(3000);
  const [probeAttempts, setProbeAttempts] = useState(1);
  const [targetMode, setTargetMode] = useState<'ws' | 'tls' | 'http'>('ws');
  const [targetSni, setTargetSni] = useState('speed.cloudflare.com');
  const [websocketHost, setWebsocketHost] = useState('speed.cloudflare.com');
  const [websocketPath, setWebsocketPath] = useState('/__down');
  const [requireWs, setRequireWs] = useState(true);
  const [enableNeighbors, setEnableNeighbors] = useState(true);
  const [topLimit, setTopLimit] = useState(20);
  const [totalTargetCount, setTotalTargetCount] = useState(0);

  // Sources state
  const [sources, setSources] = useState<ScannerSource[]>([]);
  const [showAddSourceInline, setShowAddSourceInline] = useState(false);
  const [newSourceName, setNewSourceName] = useState('');
  const [newSourceUrl, setNewSourceUrl] = useState('');
  const [newSourceType, setNewSourceType] = useState<'cidr' | 'proxyip' | 'domain'>('cidr');
  const [selectedCDNs, setSelectedCDNs] = useState<string[]>([]);

  // Collapsible configuration panels
  const [showAdvancedSettings, setShowAdvancedSettings] = useState(false);

  // Drag and drop state
  const [isDragging, setIsDragging] = useState(false);
  const [message, setMessage] = useState<{ type: 'success' | 'error' | 'info'; text: string } | null>(null);

  const dragCounter = useRef(0);
  const logsContainerRef = useRef<HTMLDivElement | null>(null);

  // Auto scroll logic for logs terminal
  useEffect(() => {
    if (logsContainerRef.current) {
      logsContainerRef.current.scrollTop = logsContainerRef.current.scrollHeight;
    }
  }, [scannerLogs]);

  // Parse connection link logic
  const handleParseLink = () => {
    if (!rawConfigLink.trim()) {
      setMessage({ type: 'error', text: 'Connection link is empty!' });
      return;
    }

    try {
      let cleaned = rawConfigLink.trim();
      const urlObj = new URL(cleaned);
      const protocol = urlObj.protocol.replace(':', '');
      
      if (protocol !== 'vless' && protocol !== 'trojan') {
        setMessage({ type: 'error', text: `Unsupported protocol scheme: ${protocol}` });
        return;
      }

      const params = new URLSearchParams(urlObj.search);
      const host = urlObj.hostname;
      const port = Number(urlObj.port) || (protocol === 'trojan' ? 443 : 80);

      // Populate form state fields
      setTargetSni(params.get('sni') || host);
      setWebsocketHost(params.get('host') || host);
      setWebsocketPath(params.get('path') || '/');
      setRequireWs(params.get('type') === 'ws');
      setTargetMode(params.get('type') === 'ws' ? 'ws' : (params.get('security') === 'tls' ? 'tls' : 'http'));

      setMessage({ type: 'success', text: `Parsed outbound configuration link: ${protocol.toUpperCase()}://${host}:${port}` });
    } catch (err: any) {
      setMessage({ type: 'error', text: `Failed parsing configuration link: ${err.message}` });
    }
  };

  // Connect WebSocket channel
  useEffect(() => {
    const protocol = window.location.protocol === 'https:' ? 'wss:' : 'ws:';
    const activeToken = token || localStorage.getItem('cc_client_token') || '';
    const wsUrl = `${protocol}//${window.location.host}/ws?token=${activeToken}`;

    const socket = new WebSocket(wsUrl);

    socket.onopen = () => {
      setWsConnected(true);
      socket.send(JSON.stringify({ type: 'scanner:telemetry', data: {} }));
    };

    socket.onmessage = (event) => {
      try {
        const msg = JSON.parse(event.data);
        if (msg.type === 'GEO_RESOLVED' && msg.data) {
          useGeoStore.getState().updateGeoInfo(msg.data);
        }
        if (msg.type === 'scanner:telemetry') {
          if (msg.stats) {
            setStats({
              tested: msg.stats.tested || 0,
              healthy: msg.stats.healthy || 0,
              failed: msg.stats.failed || 0,
              in_flight: msg.stats.in_flight || 0,
              total_targets: msg.stats.total_targets || 0,
              remaining_sec: msg.stats.remaining_sec || 0,
              phase: msg.stats.phase || '',
            });
            setIsScanning(
              (msg.stats.in_flight || 0) > 0 ||
              msg.event === 'scanner.progress' ||
              (msg.stats.phase && msg.stats.phase !== '' && msg.event !== 'scanner.finished')
            );
          }

          if (msg.event === 'scanner.log' && msg.data) {
            setScannerLogs((prev) => [...prev, String(msg.data)]);
          }

          if (msg.event === 'scanner.error' && msg.data) {
            setMessage({ type: 'error', text: String(msg.data) });
            setScannerLogs((prev) => [...prev, `[ERROR] ${msg.data}`]);
          }

          if (msg.event === 'scanner.candidate' && msg.data) {
            const c = msg.data;
            setCandidates((prev) => {
              const filtered = prev.filter((item) => item.ip !== c.ip || item.port !== c.port);
              return [
                {
                  ip: c.ip,
                  port: c.port,
                  protocol: c.protocol || 'ws',
                  latencyMs: c.latency_ms || c.latencyMs || 0,
                  speedMbps: c.download_speed_mbps || c.speed_mbps || c.speedMbps || 0.0,
                  packetLoss: c.packet_loss !== undefined ? c.packet_loss : 0,
                  cdnProvider: c.cdn_provider || '',
                  popLocation: c.pop_location || '',
                  status: (c.latency_ms || c.latencyMs) > 0 ? (c.packet_loss === 100 ? 'failed' : 'healthy') : 'failed',
                  time: new Date().toLocaleTimeString(),
                },
                ...filtered,
              ];
            });
          }

          if (msg.event === 'scanner.finished') {
            setIsScanning(false);
            setMessage({ type: 'success', text: 'Scanning operations completed.' });
            fetchSavedConfigs();
          }
        }
      } catch (err) {
        // Suppress json parsing issues
      }
    };

    socket.onclose = () => {
      setWsConnected(false);
      setIsScanning(false);
    };

    setWs(socket);

    return () => {
      socket.close();
    };
  }, [token]);

  // Load existing discovered configs
  const fetchSavedConfigs = async () => {
    try {
      const activeToken = token || localStorage.getItem('cc_client_token') || '';
      const response = await fetch('/api/v2ray/client/configs', {
        headers: { 'Authorization': `Bearer ${activeToken}` }
      });
      if (response.ok) {
        const list = await response.json();
        const parsed: ScannedCandidate[] = (list.data || list)
          .filter((item: any) => item.name && item.name.startsWith('Discovered-'))
          .map((item: any) => ({
            ip: item.address,
            port: item.port,
            protocol: item.network || 'tcp',
            latencyMs: item.latency_ms || item.latency || 0,
            speedMbps: item.download_speed_mbps || item.speed_mbps || item.speed || 0.0,
            packetLoss: item.packet_loss || 0,
            cdnProvider: item.cdn_provider || '',
            popLocation: item.pop_location || '',
            status: item.latency_ms > 0 ? (item.packet_loss === 100 ? 'failed' : 'healthy') : 'failed',
            time: 'Saved',
          }));
        setCandidates(parsed);
      }
    } catch (err) {
      console.error('Failed to load configs:', err);
    }
  };

  const downloadTxt = () => {
    const healthyList = candidates
      .filter((c) => c.status === 'healthy')
      .map((c) => `${c.ip}:${c.port}`)
      .join('\n');
    if (!healthyList) {
      setMessage({ type: 'info', text: 'No verified candidates available to download.' });
      return;
    }
    const blob = new Blob([healthyList], { type: 'text/plain;charset=utf-8' });
    const url = URL.createObjectURL(blob);
    const link = document.createElement('a');
    link.href = url;
    link.download = 'healthy_ips.txt';
    document.body.appendChild(link);
    link.click();
    document.body.removeChild(link);
    URL.revokeObjectURL(url);
    setMessage({ type: 'success', text: 'Downloaded healthy_ips.txt successfully.' });
  };

  const downloadCsv = () => {
    const healthy = candidates.filter((c) => c.status === 'healthy');
    if (healthy.length === 0) {
      setMessage({ type: 'info', text: 'No verified candidates available to download.' });
      return;
    }
    const headers = ['IP', 'Port', 'Protocol', 'Latency(ms)', 'Speed(Mbps)'];
    const rows = healthy.map((c) => [
      c.ip,
      c.port,
      c.protocol,
      c.latencyMs,
      c.speedMbps.toFixed(2)
    ]);
    const content = [headers.join(','), ...rows.map((r) => r.join(','))].join('\n');
    const blob = new Blob([content], { type: 'text/csv;charset=utf-8' });
    const url = URL.createObjectURL(blob);
    const link = document.createElement('a');
    link.href = url;
    link.download = 'healthy_ips.csv';
    document.body.appendChild(link);
    link.click();
    document.body.removeChild(link);
    URL.revokeObjectURL(url);
    setMessage({ type: 'success', text: 'Downloaded healthy_ips.csv successfully.' });
  };

  // Fetch Sources
  const fetchSources = async () => {
    try {
      const activeToken = token || localStorage.getItem('cc_client_token') || '';
      const res = await fetch('/api/v2ray/scanner/sources', {
        headers: {
          'Authorization': `Bearer ${activeToken}`
        }
      });
      if (res.ok) {
        const data = await res.json();
        setSources(data);
      }
    } catch (err) {
      console.error('Failed to fetch scanner sources', err);
    }
  };

  const handleToggleSource = async (src: ScannerSource) => {
    try {
      const activeToken = token || localStorage.getItem('cc_client_token') || '';
      const res = await fetch(`/api/v2ray/scanner/sources/${src.id}`, {
        method: 'PUT',
        headers: {
          'Content-Type': 'application/json',
          'Authorization': `Bearer ${activeToken}`
        },
        body: JSON.stringify({
          ...src,
          is_enabled: !src.is_enabled
        })
      });
      if (res.ok) {
        fetchSources();
      }
    } catch (err) {
      console.error(err);
    }
  };

  const handleAddSource = async (name: string, url: string, type: 'cidr' | 'proxyip' | 'domain') => {
    try {
      const activeToken = token || localStorage.getItem('cc_client_token') || '';
      const res = await fetch('/api/v2ray/scanner/sources', {
        method: 'POST',
        headers: {
          'Content-Type': 'application/json',
          'Authorization': `Bearer ${activeToken}`
        },
        body: JSON.stringify({
          name,
          url,
          type,
          is_enabled: true
        })
      });
      if (res.ok) {
        fetchSources();
        setMessage({ type: 'success', text: `Added new source: ${name}` });
      }
    } catch (err) {
      console.error(err);
    }
  };

  const handleDeleteSource = async (id: number) => {
    if (!confirm('Are you sure you want to delete this source?')) return;
    try {
      const activeToken = token || localStorage.getItem('cc_client_token') || '';
      const res = await fetch(`/api/v2ray/scanner/sources/${id}`, {
        method: 'DELETE',
        headers: {
          'Authorization': `Bearer ${activeToken}`
        }
      });
      if (res.ok) {
        fetchSources();
        setMessage({ type: 'success', text: 'Source deleted.' });
      }
    } catch (err) {
      console.error(err);
    }
  };

  const handleResetSources = async () => {
    try {
      const activeToken = token || localStorage.getItem('cc_client_token') || '';
      await Promise.all(
        sources.map((src) => {
          const shouldBeEnabled = src.name === 'Cloudflare Official';
          if (src.is_enabled !== shouldBeEnabled) {
            return fetch(`/api/v2ray/scanner/sources/${src.id}`, {
              method: 'PUT',
              headers: {
                'Content-Type': 'application/json',
                'Authorization': `Bearer ${activeToken}`
              },
              body: JSON.stringify({
                ...src,
                is_enabled: shouldBeEnabled
              })
            });
          }
          return Promise.resolve();
        })
      );
      fetchSources();
      setMessage({ type: 'success', text: 'Reset IP sources default selection.' });
    } catch (err) {
      console.error(err);
    }
  };

  const handleTogglePort = (port: number) => {
    if (selectedPortsList.includes(port)) {
      setSelectedPortsList(selectedPortsList.filter((p) => p !== port));
    } else {
      setSelectedPortsList([...selectedPortsList, port]);
    }
  };

  const handleSelectAllPorts = () => {
    setSelectedPortsList(COMMON_PORTS.map((p) => p.value));
  };

  const handleClearPorts = () => {
    setSelectedPortsList([]);
  };

  const getSelectedPorts = () => {
    const custom = customPorts
      .split(',')
      .map((p) => parseInt(p.trim()))
      .filter((p) => !isNaN(p));
    return Array.from(new Set([...selectedPortsList, ...custom]));
  };

  // Keyboard shortcut listener (c key to copy results)
  useEffect(() => {
    const handleKeyDown = (e: KeyboardEvent) => {
      const tag = document.activeElement?.tagName.toLowerCase();
      if (tag === 'input' || tag === 'textarea') {
        return;
      }

      if (e.key === 'c' || e.key === 'C') {
        const healthyList = candidates
          .filter((c) => c.status === 'healthy')
          .map((c) => `${c.ip}:${c.port} (latency: ${c.latencyMs}ms, speed: ${c.speedMbps.toFixed(2)} Mbps)`)
          .join('\n');

        if (healthyList) {
          navigator.clipboard.writeText(healthyList);
          setMessage({ type: 'success', text: 'Copied verified IPs list to clipboard!' });
        } else {
          setMessage({ type: 'info', text: 'No verified candidates available to copy.' });
        }
      }
    };

    window.addEventListener('keydown', handleKeyDown);
    return () => {
      window.removeEventListener('keydown', handleKeyDown);
    };
  }, [candidates]);

  // Start scanning routine
  const handleStartScan = (retry = false, scanDiscoveredOnly = false) => {
    if (!wsConnected || !ws) {
      setMessage({ type: 'error', text: 'WebSocket connection not ready!' });
      return;
    }

    let msg = 'Launching network scan sweep...';
    if (retry) {
      msg = 'Retrying last configuration scan sweep...';
    } else if (scanDiscoveredOnly) {
      msg = 'Rerunning scan on saved healthy discovered list...';
    }

    setMessage({ type: 'info', text: msg });
    setIsScanning(true);
    setCandidates([]);
    setScannerLogs([]); // Clear logs
    
    const ports = getSelectedPorts();

    const cidrs = targetCidrs
      .split('\n')
      .map((c) => c.trim())
      .filter((c) => c.length > 0);

    const payload = {
      type: 'scanner:start',
      data: {
        target_cidrs: cidrs,
        target_cdns: selectedCDNs,
        selected_ports: ports,
        concurrency_limit: Number(concurrencyLimit),
        max_rate_limit: Number(maxRateLimit),
        network_timeout_ms: Number(networkTimeoutMs),
        probe_attempts: Number(probeAttempts),
        target_mode: targetMode,
        target_sni: targetSni,
        websocket_host: websocketHost,
        websocket_path: websocketPath,
        require_ws: requireWs,
        enable_neighbors: enableNeighbors,
        top_limit: Number(topLimit),
        total_target_count: Number(totalTargetCount),
        retry: retry,
        scan_discovered_only: scanDiscoveredOnly,
      },
    };

    ws.send(JSON.stringify(payload));
  };

  const fetchScannerConfig = async () => {
    try {
      const activeToken = token || localStorage.getItem('cc_client_token') || '';
      const res = await fetch('/api/v2ray/scanner/config', {
        headers: { 'Authorization': `Bearer ${activeToken}` }
      });
      if (res.ok) {
        const data = await res.json();
        if (data.ports) {
          const common = COMMON_PORTS.map(p => p.value).filter(p => data.ports.includes(p));
          setSelectedPortsList(common);
          const custom = data.ports.filter((p: number) => !COMMON_PORTS.map(cp => cp.value).includes(p));
          setCustomPorts(custom.join(', '));
        }
        if (data.target_cidrs) {
          setTargetCidrs(data.target_cidrs.join('\n'));
        }
        if (data.target_cdns) {
          setSelectedCDNs(data.target_cdns);
        }
        if (data.concurrency_limit) {
          setConcurrencyLimit(data.concurrency_limit);
        }
        if (data.max_rate_limit !== undefined) {
          setMaxRateLimit(data.max_rate_limit);
        }
        if (data.network_timeout_sec) {
          setNetworkTimeoutMs(data.network_timeout_sec * 1000);
        }
        if (data.probe_attempts) {
          setProbeAttempts(data.probe_attempts);
        }
        if (data.target_mode) {
          setTargetMode(data.target_mode);
        }
        if (data.target_sni) {
          setTargetSni(data.target_sni);
        }
        if (data.websocket_host) {
          setWebsocketHost(data.websocket_host);
        }
        if (data.websocket_path) {
          setWebsocketPath(data.websocket_path);
        }
        if (data.require_ws !== undefined) {
          setRequireWs(data.require_ws);
        }
        if (data.enable_neighbors !== undefined) {
          setEnableNeighbors(data.enable_neighbors);
        }
        if (data.top_limit) {
          setTopLimit(data.top_limit);
        }
        if (data.total_target_count !== undefined) {
          setTotalTargetCount(data.total_target_count);
        }
      }
    } catch (err) {
      console.error('Failed to fetch scanner config', err);
    }
  };

  const handleResetSettings = async () => {
    if (!confirm('Are you sure you want to reset all scanner settings to default values?')) {
      return;
    }
    try {
      const activeToken = token || localStorage.getItem('cc_client_token') || '';
      const res = await fetch('/api/v2ray/scanner/config/reset', {
        method: 'POST',
        headers: { 'Authorization': `Bearer ${activeToken}` }
      });
      if (res.ok) {
        setMessage({ type: 'success', text: 'Scanner settings successfully reset to default.' });
        fetchScannerConfig();
      } else {
        setMessage({ type: 'error', text: 'Failed to reset settings.' });
      }
    } catch (err) {
      console.error('Failed to reset scanner settings', err);
    }
  };

  const handleCleanupDiscoveredHealthy = async () => {
    if (!confirm('Are you sure you want to delete ALL scanner-discovered healthy results completely from the database?')) {
      return;
    }
    try {
      const activeToken = token || localStorage.getItem('cc_client_token') || '';
      const res = await fetch('/api/v2ray/client/configs/discovered', {
        method: 'DELETE',
        headers: {
          'Authorization': `Bearer ${activeToken}`
        }
      });
      if (res.ok) {
        const data = await res.json();
        setMessage({ type: 'success', text: `Cleaned up ${data.count || 0} discovered nodes.` });
        fetchSavedConfigs();
      } else {
        setMessage({ type: 'error', text: 'Failed to delete discovered nodes.' });
      }
    } catch (err) {
      console.error(err);
      setMessage({ type: 'error', text: 'Network error during healthy candidate deletion.' });
    }
  };

  const handleCleanupDiscovered = async () => {
    if (!confirm('Are you sure you want to clean up all failed candidates from the database?')) {
      return;
    }
    try {
      const activeToken = token || localStorage.getItem('cc_client_token') || '';
      const res = await fetch('/api/v2ray/client/configs/failed', {
        method: 'DELETE',
        headers: {
          'Authorization': `Bearer ${activeToken}`
        }
      });
      if (res.ok) {
        const data = await res.json();
        setMessage({ type: 'success', text: `Cleaned up ${data.count || 0} failed candidates.` });
        fetchSavedConfigs();
      } else {
        setMessage({ type: 'error', text: 'Failed to clean up configs.' });
      }
    } catch (err) {
      console.error(err);
      setMessage({ type: 'error', text: 'Network error during cleanup.' });
    }
  };

  const handleStopScan = () => {
    if (!ws) return;
    ws.send(JSON.stringify({ type: 'scanner:stop', data: {} }));
    setIsScanning(false);
    setMessage({ type: 'info', text: 'Scan canceled by user.' });
  };

  // Drag and drop target files processing
  const handleDragEnter = (e: React.DragEvent) => {
    e.preventDefault();
    e.stopPropagation();
    dragCounter.current++;
    if (e.dataTransfer.items && e.dataTransfer.items.length > 0) {
      setIsDragging(true);
    }
  };

  const handleDragLeave = (e: React.DragEvent) => {
    e.preventDefault();
    e.stopPropagation();
    dragCounter.current--;
    if (dragCounter.current === 0) {
      setIsDragging(false);
    }
  };

  const handleDragOver = (e: React.DragEvent) => {
    e.preventDefault();
    e.stopPropagation();
  };

  const handleDrop = (e: React.DragEvent) => {
    e.preventDefault();
    e.stopPropagation();
    setIsDragging(false);
    dragCounter.current = 0;

    const files = e.dataTransfer.files;
    if (files && files.length > 0) {
      const file = files[0];
      const reader = new FileReader();
      reader.onload = (event) => {
        const text = event.target?.result as string;
        if (text) {
          const lines = text
            .split('\n')
            .map((line) => line.trim())
            .filter((line) => line.length > 0 && !line.startsWith('#'));
          
          setTargetCidrs(lines.join('\n'));
          setMessage({ type: 'success', text: `Imported ${lines.length} network targets from ${file.name}` });
        }
      };
      reader.readAsText(file);
    }
  };

  // Load resources on mount
  useEffect(() => {
    fetchSavedConfigs();
    fetchSources();
    fetchScannerConfig();
  }, []);

  const handleAddSourceSubmit = (e: React.FormEvent) => {
    e.preventDefault();
    if (!newSourceName || !newSourceUrl) return;
    handleAddSource(newSourceName, newSourceUrl, newSourceType);
    setNewSourceName('');
    setNewSourceUrl('');
    setShowAddSourceInline(false);
  };

  // Filtering and sorting logic for candidates
  const filteredCandidates = candidates
    .filter((c) => {
      const q = searchQuery.toLowerCase().trim();
      return c.ip.toLowerCase().includes(q) || c.port.toString().includes(q);
    })
    .sort((a, b) => {
      if (sortBy === 'latency') {
        return a.latencyMs - b.latencyMs;
      } else if (sortBy === 'speed') {
        return b.speedMbps - a.speedMbps;
      }
      return 0;
    });

  const formatRemainingTime = (seconds: number) => {
    if (seconds <= 0) return '0s';
    const m = Math.floor(seconds / 60);
    const s = seconds % 60;
    if (m > 0) return `${m}m ${s}s`;
    return `${s}s`;
  };

  const formatCount = (count: number) => {
    if (count >= 1000) {
      return `${(count / 1000).toFixed(1)}K`;
    }
    return count.toString();
  };

  // Filtering logs
  const filteredLogs = scannerLogs.filter((log) => {
    return log.toLowerCase().includes(logsFilter.toLowerCase());
  });

  const enabledSourcesCount = sources.filter((s) => s.is_enabled).length;

  return (
    <div className="page-container animate-fade-in" style={{ padding: '4px 0', fontFamily: 'var(--font-sans)' }}>
      
      {/* Styles for premium visuals & animations */}
      <style>{`
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
        @keyframes pulse-dot {
          0% { transform: scale(0.9); box-shadow: 0 0 0 0 rgba(255, 107, 44, 0.6); }
          70% { transform: scale(1.1); box-shadow: 0 0 0 8px rgba(255, 107, 44, 0); }
          100% { transform: scale(0.9); box-shadow: 0 0 0 0 rgba(255, 107, 44, 0); }
        }
        .animate-pulse-dot {
          animation: pulse-dot 1.8s infinite ease-in-out;
        }
      `}</style>

      {/* Header section */}
      <div style={{ display: 'flex', justifyContent: 'space-between', alignItems: 'center', marginBottom: 20 }}>
        <div>
          <h1 style={{ fontSize: 22, fontWeight: 700, color: 'var(--color-brand-heading)', margin: 0 }}>
            V2Ray Network Scanner
          </h1>
          <p style={{ fontSize: 12, color: 'var(--color-brand-text)', margin: '4px 0 0' }}>
            High-velocity deep-packet inspection (DPI) bypass verification engine. Emulates TLS and WebSocket handshakes.
          </p>
        </div>
        
        <div style={{ display: 'flex', alignItems: 'center', gap: 12 }}>
          <div style={{ display: 'flex', alignItems: 'center', gap: 8, padding: '6px 14px', background: 'var(--color-brand-light)', border: '1px solid var(--color-brand-border)', borderRadius: 20 }}>
            <span style={{ display: 'flex', width: 8, height: 8, position: 'relative' }}>
              <span style={{ position: 'absolute', width: '100%', height: '100%', borderRadius: '50%', backgroundColor: wsConnected ? 'var(--color-brand-green)' : 'var(--color-brand-red)', opacity: 0.7, animation: 'ping 1s cubic-bezier(0, 0, 0.2, 1) infinite' }} />
              <span style={{ position: 'relative', width: '100%', height: '100%', borderRadius: '50%', backgroundColor: wsConnected ? 'var(--color-brand-green)' : 'var(--color-brand-red)' }} />
            </span>
            <span style={{ fontSize: 12, fontWeight: 600, color: 'var(--color-brand-heading)' }}>
              {wsConnected ? 'Telemetry Connected' : 'Offline'}
            </span>
          </div>
          <button className="btn btn--sm btn--secondary" onClick={fetchSavedConfigs}>
            <FiRefreshCw style={{ marginRight: 6 }} /> Refresh List
          </button>
        </div>
      </div>

      {message && (
        <div style={{
          padding: '12px 18px',
          borderRadius: 10,
          marginBottom: 20,
          fontSize: 13,
          fontWeight: 500,
          display: 'flex',
          alignItems: 'center',
          gap: 10,
          background: message.type === 'success' ? 'var(--color-brand-light)' : 'rgba(239, 68, 68, 0.08)',
          border: '1px solid var(--color-brand-border)',
          color: message.type === 'success' ? 'var(--color-brand)' : '#ef4444'
        }}>
          {message.type === 'success' ? <FiCheckCircle size={16} /> : <FiAlertCircle size={16} />}
          <span>{message.text}</span>
        </div>
      )}

      {/* Dynamic Scan Status Bar */}
      {isScanning && (
        <div className="g-card" style={{ padding: '16px 20px', marginBottom: 20, background: 'linear-gradient(135deg, rgba(255, 107, 44, 0.05), rgba(59, 130, 246, 0.03))', borderColor: 'rgba(255, 107, 44, 0.15)' }}>
          <div style={{ display: 'flex', justifyContent: 'space-between', alignItems: 'center', marginBottom: 10 }}>
            <div style={{ display: 'flex', alignItems: 'center', gap: 10 }}>
              <span className="animate-pulse-dot" style={{ display: 'inline-block', width: 8, height: 8, borderRadius: '50%', background: 'var(--color-brand)' }}></span>
              <span style={{ fontSize: 13, fontWeight: 600, color: 'var(--color-brand-heading)', textTransform: 'uppercase', letterSpacing: '0.5px' }}>
                {stats.phase || 'Scanning Targets'}
              </span>
            </div>
            <span style={{ fontSize: 12, fontWeight: 700, color: 'var(--color-brand-heading)' }}>
              {stats.tested} / {stats.total_targets} ({stats.total_targets > 0 ? Math.min(100, Math.round((stats.tested / stats.total_targets) * 100)) : 0}%)
            </span>
          </div>

          <div style={{ width: '100%', height: 8, background: 'var(--color-brand-bg)', borderRadius: 4, overflow: 'hidden', marginBottom: 10, border: '1px solid var(--color-brand-border)' }}>
            <div style={{
              width: `${stats.total_targets > 0 ? Math.min(100, Math.round((stats.tested / stats.total_targets) * 100)) : 0}%`,
              height: '100%',
              background: 'linear-gradient(90deg, var(--color-brand) 0%, var(--color-brand-blue) 100%)',
              transition: 'width 0.4s ease-out',
              borderRadius: 4
            }}></div>
          </div>

          <div style={{ display: 'flex', justifyContent: 'space-between', fontSize: 11, color: 'var(--color-brand-text)', fontWeight: 500 }}>
            <span>Active Worker Connections: <strong style={{ color: 'var(--color-brand-blue)' }}>{stats.in_flight}</strong></span>
            {stats.remaining_sec > 0 && (
              <span>Estimated Time Remaining (ETA): <strong style={{ color: 'var(--color-brand-heading)' }}>{formatRemainingTime(stats.remaining_sec)}</strong></span>
            )}
          </div>
        </div>
      )}

      {/* Main Grid Layout */}
      <div style={{ display: 'grid', gridTemplateColumns: 'repeat(12, minmax(0, 1fr))', gap: '20px' }}>
        
        {/* Left Column: Config Panel (span 5) */}
        <div className="col-span-12 lg:col-span-5" style={{ display: 'flex', flexDirection: 'column', gap: 20 }}>
          
          {/* Card: Target CDN Filtering */}
          <div className="g-card" style={{ padding: 20 }}>
            <h3 style={{ fontSize: 11, fontWeight: 700, color: 'var(--color-brand-muted)', textTransform: 'uppercase', letterSpacing: '1px', margin: 0, marginBottom: 14 }}>
              TARGET CDN REGISTRY FILTERING
            </h3>
            <p style={{ fontSize: 11, color: 'var(--color-brand-text)', marginBottom: 12 }}>
              Select target CDNs to sweep their official offline IP registry ranges. If selected, other sources are bypassed.
            </p>
            <div style={{ display: 'grid', gridTemplateColumns: '1fr 1fr', gap: '8px 12px' }}>
              {[
                { id: 'cloudflare', name: 'Cloudflare' },
                { id: 'cloudfront', name: 'AWS CloudFront' },
                { id: 'fastly', name: 'Fastly' },
                { id: 'bunny', name: 'Bunny CDN' },
                { id: 'cdn77', name: 'CDN77' },
                { id: 'gcore', name: 'Gcore' },
                { id: 'akamai', name: 'Akamai' },
                { id: 'google', name: 'Google Cloud CDN' },
                { id: 'azure', name: 'Microsoft Azure' }
              ].map((cdn) => {
                const isChecked = selectedCDNs.includes(cdn.name);
                return (
                  <label
                    key={cdn.id}
                    style={{
                      display: 'flex',
                      alignItems: 'center',
                      gap: 8,
                      padding: '6px 10px',
                      borderRadius: 6,
                      border: isChecked ? '1px solid var(--color-brand)' : '1px solid var(--color-brand-border)',
                      background: isChecked ? 'var(--color-brand-light)' : 'var(--color-brand-card)',
                      cursor: 'pointer',
                      userSelect: 'none',
                      transition: 'all 0.15s ease'
                    }}
                  >
                    <input
                      type="checkbox"
                      checked={isChecked}
                      onChange={() => {
                        if (isChecked) {
                          setSelectedCDNs(selectedCDNs.filter((c) => c !== cdn.name));
                        } else {
                          setSelectedCDNs([...selectedCDNs, cdn.name]);
                        }
                      }}
                      style={{ accentColor: 'var(--color-brand)', cursor: 'pointer' }}
                    />
                    <span style={{ fontSize: 11, fontWeight: 600, color: 'var(--color-brand-heading)' }}>
                      {cdn.name}
                    </span>
                  </label>
                );
              })}
            </div>
            {selectedCDNs.length > 0 && (
              <div style={{ marginTop: 12, display: 'flex', justifyContent: 'flex-end' }}>
                <button
                  onClick={() => setSelectedCDNs([])}
                  style={{ background: 'none', border: 'none', color: 'var(--color-brand-red)', fontSize: 10, fontWeight: 600, cursor: 'pointer' }}
                >
                  Clear CDN Filters
                </button>
              </div>
            )}
          </div>

          {/* Card 1: Port Configuration */}
          <div className="g-card" style={{ padding: 20 }}>
            <div style={{ display: 'flex', justifyContent: 'space-between', alignItems: 'center', marginBottom: 14 }}>
              <h3 style={{ fontSize: 11, fontWeight: 700, color: 'var(--color-brand-muted)', textTransform: 'uppercase', letterSpacing: '1px', margin: 0 }}>
                PORT CONFIGURATION
              </h3>
              <div style={{ display: 'flex', gap: 10, fontSize: 11 }}>
                <button onClick={handleSelectAllPorts} style={{ background: 'none', border: 'none', color: 'var(--color-brand)', fontWeight: 600, cursor: 'pointer' }}>
                  All
                </button>
                <span style={{ color: 'var(--color-brand-border)' }}>|</span>
                <button onClick={handleClearPorts} style={{ background: 'none', border: 'none', color: 'var(--color-brand-text)', fontWeight: 500, cursor: 'pointer' }}>
                  Clear
                </button>
              </div>
            </div>

            {/* Ports Grid layout */}
            <div style={{ display: 'grid', gridTemplateColumns: 'repeat(3, minmax(0, 1fr))', gap: 8, marginBottom: 12 }}>
              {COMMON_PORTS.map((port) => {
                const isChecked = selectedPortsList.includes(port.value);
                return (
                  <label
                    key={port.value}
                    style={{
                      display: 'flex',
                      alignItems: 'center',
                      gap: 8,
                      padding: '8px 10px',
                      borderRadius: 8,
                      border: isChecked ? '1px solid var(--color-brand)' : '1px solid var(--color-brand-border)',
                      background: isChecked ? 'var(--color-brand-light)' : 'var(--color-brand-card)',
                      cursor: 'pointer',
                      userSelect: 'none',
                      transition: 'all 0.15s ease'
                    }}
                  >
                    <input
                      type="checkbox"
                      checked={isChecked}
                      onChange={() => handleTogglePort(port.value)}
                      style={{
                        accentColor: 'var(--color-brand)',
                        cursor: 'pointer'
                      }}
                    />
                    <span style={{ fontSize: 11, fontWeight: 600, color: 'var(--color-brand-heading)' }}>
                      {port.label}
                    </span>
                  </label>
                );
              })}
            </div>

            {/* Custom additional ports */}
            <div>
              <label style={{ display: 'block', fontSize: 10, fontWeight: 700, color: 'var(--color-brand-text)', marginBottom: 4, textTransform: 'uppercase' }}>
                Additional Ports (comma-separated)
              </label>
              <input
                type="text"
                value={customPorts}
                onChange={(e) => setCustomPorts(e.target.value)}
                placeholder="e.g. 8880, 2082"
                style={{
                  width: '100%',
                  padding: '7px 10px',
                  borderRadius: 6,
                  border: '1px solid var(--color-brand-border)',
                  background: 'var(--color-brand-bg)',
                  color: 'var(--color-brand-heading)',
                  fontSize: 11,
                  outline: 'none'
                }}
              />
            </div>
          </div>

          {/* Card 2: IP Sources Panel */}
          <div className="g-card" style={{ padding: 20 }}>
            <div style={{ display: 'flex', justifyContent: 'space-between', alignItems: 'center', marginBottom: 14 }}>
              <h3 style={{ fontSize: 11, fontWeight: 700, color: 'var(--color-brand-muted)', textTransform: 'uppercase', letterSpacing: '1px', margin: 0 }}>
                IP SOURCES ({enabledSourcesCount} ENABLED)
              </h3>
              <div style={{ display: 'flex', gap: 10, fontSize: 11 }}>
                <button
                  onClick={() => setShowAddSourceInline(!showAddSourceInline)}
                  style={{ background: 'none', border: 'none', color: 'var(--color-brand)', fontWeight: 600, cursor: 'pointer', display: 'flex', alignItems: 'center', gap: 4 }}
                >
                  <FiPlus size={12} /> Add Source
                </button>
                <span style={{ color: 'var(--color-brand-border)' }}>|</span>
                <button onClick={handleResetSources} style={{ background: 'none', border: 'none', color: 'var(--color-brand-text)', fontWeight: 500, cursor: 'pointer' }}>
                  Reset
                </button>
              </div>
            </div>

            {/* Inline Add Source Form */}
            {showAddSourceInline && (
              <form onSubmit={handleAddSourceSubmit} style={{ marginBottom: 14, padding: 12, borderRadius: 8, background: 'var(--color-brand-bg)', border: '1px solid var(--color-brand-border)', display: 'flex', flexDirection: 'column', gap: 8 }}>
                <div style={{ display: 'grid', gridTemplateColumns: '1fr 1fr', gap: 8 }}>
                  <input
                    type="text"
                    placeholder="Source Name"
                    value={newSourceName}
                    onChange={(e) => setNewSourceName(e.target.value)}
                    style={{ padding: '6px 8px', borderRadius: 6, border: '1px solid var(--color-brand-border)', background: 'var(--color-brand-card)', color: 'var(--color-brand-heading)', fontSize: 11, outline: 'none' }}
                    required
                  />
                  <select
                    value={newSourceType}
                    onChange={(e: any) => setNewSourceType(e.target.value)}
                    style={{ padding: '6px 8px', borderRadius: 6, border: '1px solid var(--color-brand-border)', background: 'var(--color-brand-card)', color: 'var(--color-brand-heading)', fontSize: 11, outline: 'none' }}
                  >
                    <option value="cidr">CIDR Ranges</option>
                    <option value="proxyip">Proxy IP list</option>
                    <option value="domain">Domain list</option>
                  </select>
                </div>
                <input
                  type="url"
                  placeholder="https://example.com/ips.txt"
                  value={newSourceUrl}
                  onChange={(e) => setNewSourceUrl(e.target.value)}
                  style={{ width: '100%', padding: '6px 8px', borderRadius: 6, border: '1px solid var(--color-brand-border)', background: 'var(--color-brand-card)', color: 'var(--color-brand-heading)', fontSize: 11, outline: 'none' }}
                  required
                />
                <div style={{ display: 'flex', justifyContent: 'flex-end', gap: 8 }}>
                  <button type="button" onClick={() => setShowAddSourceInline(false)} className="btn btn--sm btn--secondary" style={{ padding: '4px 10px' }}>
                    Cancel
                  </button>
                  <button type="submit" className="btn btn--sm btn--primary" style={{ padding: '4px 10px' }}>
                    Add
                  </button>
                </div>
              </form>
            )}

            {/* Sources List */}
            <div style={{ display: 'flex', flexDirection: 'column', gap: 8, maxHeight: 280, overflowY: 'auto', paddingRight: 4 }}>
              {sources.length === 0 ? (
                <div style={{ padding: '20px 0', textAlign: 'center', color: 'var(--color-brand-muted)', fontSize: 11 }}>
                  No scanner sources seeded.
                </div>
              ) : (
                sources.map((src) => {
                  const isCidr = src.type === 'cidr';
                  const isProxy = src.type === 'proxyip';
                  
                  return (
                    <div
                      key={src.id}
                      style={{
                        display: 'flex',
                        alignItems: 'center',
                        justifyContent: 'space-between',
                        padding: '10px 12px',
                        borderRadius: 8,
                        background: 'var(--color-brand-card)',
                        border: '1px solid var(--color-brand-border)',
                        transition: 'all 0.15s ease'
                      }}
                    >
                      <div style={{ display: 'flex', alignItems: 'center', gap: 10, flex: 1, minWidth: 0 }}>
                        <div style={{ display: 'flex', alignItems: 'center', justifyContent: 'center', width: 28, height: 28, borderRadius: 6, background: 'var(--color-brand-bg)', border: '1px solid var(--color-brand-border)', flexShrink: 0 }}>
                          <FiGlobe size={13} style={{ color: isCidr ? 'var(--color-brand-indigo)' : isProxy ? 'var(--color-brand-green)' : 'var(--color-brand-blue)' }} />
                        </div>
                        <div style={{ display: 'flex', flexDirection: 'column', minWidth: 0 }}>
                          <div style={{ display: 'flex', alignItems: 'center', gap: 6 }}>
                            <span style={{
                              fontSize: 8,
                              fontWeight: 700,
                              padding: '1px 4px',
                              borderRadius: 4,
                              background: isCidr ? 'rgba(99, 102, 241, 0.08)' : isProxy ? 'rgba(34, 197, 94, 0.08)' : 'rgba(59, 130, 246, 0.08)',
                              color: isCidr ? 'var(--color-brand-indigo)' : isProxy ? 'var(--color-brand-green)' : 'var(--color-brand-blue)',
                              textTransform: 'uppercase',
                              border: '1px solid transparent'
                            }}>
                              {src.type}
                            </span>
                            <span style={{ fontSize: 12, fontWeight: 600, color: 'var(--color-brand-heading)', overflow: 'hidden', textOverflow: 'ellipsis', whiteSpace: 'nowrap' }}>
                              {src.name}
                            </span>
                          </div>
                          <span style={{ fontSize: 10, color: 'var(--color-brand-muted)', overflow: 'hidden', textOverflow: 'ellipsis', whiteSpace: 'nowrap', marginTop: 1 }}>
                            {src.url}
                          </span>
                        </div>
                      </div>

                      <div style={{ display: 'flex', alignItems: 'center', gap: 10, marginLeft: 10 }}>
                        <ToggleSwitch
                          checked={src.is_enabled}
                          onChange={() => handleToggleSource(src)}
                        />
                        <button
                          onClick={() => handleDeleteSource(src.id)}
                          style={{ background: 'none', border: 'none', color: 'var(--color-brand-red)', cursor: 'pointer', padding: 4, display: 'flex', alignItems: 'center' }}
                          title="Delete source"
                        >
                          <FiTrash2 size={13} />
                        </button>
                      </div>
                    </div>
                  );
                })
              )}
            </div>
          </div>

          {/* Card 3: Advanced tuning & controls wrapper */}
          <div className="g-card" style={{ padding: 20 }}>
            <button
              onClick={() => setShowAdvancedSettings(!showAdvancedSettings)}
              style={{
                display: 'flex',
                width: '100%',
                justifyContent: 'space-between',
                alignItems: 'center',
                background: 'none',
                border: 'none',
                cursor: 'pointer',
                padding: 0,
                outline: 'none'
              }}
            >
              <span style={{ fontSize: 11, fontWeight: 700, color: 'var(--color-brand-muted)', textTransform: 'uppercase', letterSpacing: '1px' }}>
                Advanced Sweep Parameters
              </span>
              {showAdvancedSettings ? <FiChevronUp size={16} /> : <FiChevronDown size={16} />}
            </button>

            {showAdvancedSettings && (
              <div style={{ display: 'flex', flexDirection: 'column', gap: 12, marginTop: 14 }}>
                
                {/* Drag and Drop Zone */}
                <div 
                  onDragEnter={handleDragEnter}
                  onDragOver={handleDragOver}
                  onDragLeave={handleDragLeave}
                  onDrop={handleDrop}
                  style={{
                    border: isDragging ? '2px dashed var(--color-brand)' : '1px dashed var(--color-brand-border)',
                    background: isDragging ? 'var(--color-brand-light)' : 'rgba(255,255,255,0.01)',
                    borderRadius: 8,
                    padding: 12,
                    textAlign: 'center',
                    cursor: 'pointer',
                    transition: 'all 0.15s ease'
                  }}
                >
                  <FiUpload size={16} style={{ color: 'var(--color-brand-muted)', marginBottom: 4 }} />
                  <div style={{ fontSize: 10, fontWeight: 600, color: 'var(--color-brand-heading)' }}>
                    Drag & Drop Custom CIDR Text File
                  </div>
                </div>

                {/* Connection Parser input */}
                <div>
                  <label style={{ display: 'block', fontSize: 10, fontWeight: 700, color: 'var(--color-brand-text)', marginBottom: 4, textTransform: 'uppercase' }}>
                    Emulation Link Parser
                  </label>
                  <div style={{ display: 'flex', gap: 8 }}>
                    <input
                      type="text"
                      value={rawConfigLink}
                      onChange={(e) => setRawConfigLink(e.target.value)}
                      placeholder="Paste vless:// or trojan:// outbound link..."
                      style={{ flex: 1, padding: '6px 10px', borderRadius: 6, border: '1px solid var(--color-brand-border)', background: 'var(--color-brand-bg)', color: 'var(--color-brand-heading)', fontSize: 11, outline: 'none' }}
                    />
                    <button type="button" className="btn btn--primary btn--sm" onClick={handleParseLink}>
                      Parse
                    </button>
                  </div>
                </div>

                {/* CIDRs textarea backup */}
                <div>
                  <label style={{ display: 'block', fontSize: 10, fontWeight: 700, color: 'var(--color-brand-text)', marginBottom: 4, textTransform: 'uppercase' }}>
                    Custom CIDRs (Fallback if DB empty)
                  </label>
                  <textarea
                    value={targetCidrs}
                    onChange={(e) => setTargetCidrs(e.target.value)}
                    rows={2}
                    style={{ width: '100%', padding: '6px 10px', borderRadius: 6, border: '1px solid var(--color-brand-border)', background: 'var(--color-brand-bg)', fontSize: 11, color: 'var(--color-brand-heading)', resize: 'none', outline: 'none' }}
                  />
                </div>

                {/* Settings Grid */}
                <div style={{ display: 'grid', gridTemplateColumns: '1fr 1fr', gap: 10 }}>
                  <div>
                    <label style={{ display: 'block', fontSize: 10, fontWeight: 700, color: 'var(--color-brand-text)', marginBottom: 4, textTransform: 'uppercase' }}>Concurrency</label>
                    <input
                      type="number"
                      value={concurrencyLimit}
                      onChange={(e) => setConcurrencyLimit(Number(e.target.value))}
                      style={{ width: '100%', padding: '6px 8px', borderRadius: 6, border: '1px solid var(--color-brand-border)', background: 'var(--color-brand-bg)', color: 'var(--color-brand-heading)', fontSize: 11, outline: 'none' }}
                    />
                  </div>
                  <div>
                    <label style={{ display: 'block', fontSize: 10, fontWeight: 700, color: 'var(--color-brand-text)', marginBottom: 4, textTransform: 'uppercase' }}>Timeout (ms)</label>
                    <input
                      type="number"
                      value={networkTimeoutMs}
                      onChange={(e) => setNetworkTimeoutMs(Number(e.target.value))}
                      style={{ width: '100%', padding: '6px 8px', borderRadius: 6, border: '1px solid var(--color-brand-border)', background: 'var(--color-brand-bg)', color: 'var(--color-brand-heading)', fontSize: 11, outline: 'none' }}
                    />
                  </div>
                </div>

                <div style={{ display: 'grid', gridTemplateColumns: '1fr 1fr', gap: 10 }}>
                  <div>
                    <label style={{ display: 'block', fontSize: 10, fontWeight: 700, color: 'var(--color-brand-text)', marginBottom: 4, textTransform: 'uppercase' }}>Attempts</label>
                    <input
                      type="number"
                      value={probeAttempts}
                      onChange={(e) => setProbeAttempts(Number(e.target.value))}
                      style={{ width: '100%', padding: '6px 8px', borderRadius: 6, border: '1px solid var(--color-brand-border)', background: 'var(--color-brand-bg)', color: 'var(--color-brand-heading)', fontSize: 11, outline: 'none' }}
                    />
                  </div>
                  <div>
                    <label style={{ display: 'block', fontSize: 10, fontWeight: 700, color: 'var(--color-brand-text)', marginBottom: 4, textTransform: 'uppercase' }}>Top Save Limit</label>
                    <input
                      type="number"
                      value={topLimit}
                      onChange={(e) => setTopLimit(Number(e.target.value))}
                      style={{ width: '100%', padding: '6px 8px', borderRadius: 6, border: '1px solid var(--color-brand-border)', background: 'var(--color-brand-bg)', color: 'var(--color-brand-heading)', fontSize: 11, outline: 'none' }}
                    />
                  </div>
                </div>

                <div>
                  <label style={{ display: 'block', fontSize: 10, fontWeight: 700, color: 'var(--color-brand-text)', marginBottom: 4, textTransform: 'uppercase' }}>Target Mode</label>
                  <select
                    value={targetMode}
                    onChange={(e: any) => setTargetMode(e.target.value)}
                    style={{ width: '100%', padding: '6px 8px', borderRadius: 6, border: '1px solid var(--color-brand-border)', background: 'var(--color-brand-bg)', color: 'var(--color-brand-heading)', fontSize: 11, outline: 'none' }}
                  >
                    <option value="ws">WebSocket (WS)</option>
                    <option value="tls">Direct TLS</option>
                    <option value="http">HTTP Direct</option>
                  </select>
                </div>

                <div>
                  <label style={{ display: 'block', fontSize: 10, fontWeight: 700, color: 'var(--color-brand-text)', marginBottom: 4, textTransform: 'uppercase' }}>SNI Hostname Fingerprint</label>
                  <input
                    type="text"
                    value={targetSni}
                    onChange={(e) => setTargetSni(e.target.value)}
                    style={{ width: '100%', padding: '6px 8px', borderRadius: 6, border: '1px solid var(--color-brand-border)', background: 'var(--color-brand-bg)', color: 'var(--color-brand-heading)', fontSize: 11, outline: 'none' }}
                  />
                </div>

                {targetMode === 'ws' && (
                  <div style={{ display: 'flex', flexDirection: 'column', gap: 10, borderLeft: '2px solid var(--color-brand)', paddingLeft: 10 }}>
                    <div>
                      <label style={{ display: 'block', fontSize: 10, fontWeight: 700, color: 'var(--color-brand-text)', marginBottom: 4, textTransform: 'uppercase' }}>WS Host Header</label>
                      <input
                        type="text"
                        value={websocketHost}
                        onChange={(e) => setWebsocketHost(e.target.value)}
                        style={{ width: '100%', padding: '6px 8px', borderRadius: 6, border: '1px solid var(--color-brand-border)', background: 'var(--color-brand-bg)', color: 'var(--color-brand-heading)', fontSize: 11, outline: 'none' }}
                      />
                    </div>
                    <div>
                      <label style={{ display: 'block', fontSize: 10, fontWeight: 700, color: 'var(--color-brand-text)', marginBottom: 4, textTransform: 'uppercase' }}>WS Query Path</label>
                      <input
                        type="text"
                        value={websocketPath}
                        onChange={(e) => setWebsocketPath(e.target.value)}
                        style={{ width: '100%', padding: '6px 8px', borderRadius: 6, border: '1px solid var(--color-brand-border)', background: 'var(--color-brand-bg)', color: 'var(--color-brand-heading)', fontSize: 11, outline: 'none' }}
                      />
                    </div>
                  </div>
                )}

                <div style={{ display: 'flex', flexDirection: 'column', gap: 8, marginTop: 4 }}>
                  <div style={{ display: 'flex', justifyContent: 'space-between', alignItems: 'center' }}>
                    <div>
                      <span style={{ fontSize: 11, fontWeight: 600, color: 'var(--color-brand-heading)' }}>Require WS Compatibility</span>
                    </div>
                    <input
                      type="checkbox"
                      checked={requireWs}
                      onChange={(e) => setRequireWs(e.target.checked)}
                      style={{ width: 14, height: 14, accentColor: 'var(--color-brand)' }}
                    />
                  </div>
                  <div style={{ display: 'flex', justifyContent: 'space-between', alignItems: 'center' }}>
                    <div>
                      <span style={{ fontSize: 11, fontWeight: 600, color: 'var(--color-brand-heading)' }}>Auto-Discover Subnet Neighbors</span>
                    </div>
                    <input
                      type="checkbox"
                      checked={enableNeighbors}
                      onChange={(e) => setEnableNeighbors(e.target.checked)}
                      style={{ width: 14, height: 14, accentColor: 'var(--color-brand)' }}
                    />
                  </div>
                </div>

                <div style={{ display: 'grid', gridTemplateColumns: '1fr 1fr', gap: 10 }}>
                  <div>
                    <label style={{ display: 'block', fontSize: 10, fontWeight: 700, color: 'var(--color-brand-text)', marginBottom: 4, textTransform: 'uppercase' }}>Targets Cap</label>
                    <input
                      type="number"
                      value={totalTargetCount}
                      onChange={(e) => setTotalTargetCount(Number(e.target.value))}
                      style={{ width: '100%', padding: '6px 8px', borderRadius: 6, border: '1px solid var(--color-brand-border)', background: 'var(--color-brand-bg)', color: 'var(--color-brand-heading)', fontSize: 11, outline: 'none' }}
                    />
                  </div>
                  <div>
                    <label style={{ display: 'block', fontSize: 10, fontWeight: 700, color: 'var(--color-brand-text)', marginBottom: 4, textTransform: 'uppercase' }}>Rate Limit (0=No)</label>
                    <input
                      type="number"
                      value={maxRateLimit}
                      onChange={(e) => setMaxRateLimit(Number(e.target.value))}
                      style={{ width: '100%', padding: '6px 8px', borderRadius: 6, border: '1px solid var(--color-brand-border)', background: 'var(--color-brand-bg)', color: 'var(--color-brand-heading)', fontSize: 11, outline: 'none' }}
                    />
                  </div>
                </div>

              </div>
            )}
          </div>

          {/* Card 4: Action sweeps control buttons */}
          <div className="g-card" style={{ padding: 20 }}>
            <h3 style={{ fontSize: 11, fontWeight: 700, color: 'var(--color-brand-muted)', textTransform: 'uppercase', letterSpacing: '1px', marginBottom: 12, marginTop: 0 }}>
              SCAN CONTROL CENTER
            </h3>
            <div style={{ display: 'flex', flexDirection: 'column', gap: 8 }}>
              {isScanning ? (
                <button className="btn" onClick={handleStopScan} style={{ width: '100%', justifyContent: 'center', background: 'var(--color-brand-red)', borderColor: 'var(--color-brand-red)', color: '#fff', padding: '10px' }}>
                  <FiSquare style={{ marginRight: 6 }} /> Stop Sweep Operations
                </button>
              ) : (
                <>
                  <div style={{ display: 'flex', gap: 8 }}>
                    <button className="btn btn--primary" onClick={() => handleStartScan(false)} style={{ flex: 2, justifyContent: 'center', padding: '10px' }}>
                      <FiPlay style={{ marginRight: 6 }} /> Start Sweep
                    </button>
                    <button className="btn btn--secondary" onClick={() => handleStartScan(true)} title="Rerun last configuration scan parameters" style={{ flex: 1, justifyContent: 'center', padding: '10px' }}>
                      Retry Last
                    </button>
                  </div>
                  <div style={{ display: 'flex', gap: 8 }}>
                    <button 
                      className="btn btn--secondary" 
                      onClick={() => handleStartScan(false, true)} 
                      style={{ flex: 1, justifyContent: 'center' }}
                      title="Rescan previously verified healthy nodes"
                    >
                      <FiRefreshCw style={{ marginRight: 6 }} /> Rescan Healthy
                    </button>
                    <button 
                      className="btn btn--secondary" 
                      onClick={handleCleanupDiscovered} 
                      style={{ flex: 1, justifyContent: 'center', color: '#ef4444', borderColor: 'rgba(239, 68, 68, 0.2)' }}
                      title="Delete all failed nodes from database"
                    >
                      <FiTrash2 style={{ marginRight: 6 }} /> Clean Failed
                    </button>
                  </div>
                  <div style={{ display: 'flex', gap: 8 }}>
                    <button 
                      className="btn btn--secondary" 
                      onClick={handleCleanupDiscoveredHealthy} 
                      style={{ flex: 1, justifyContent: 'center', color: '#ef4444', borderColor: 'rgba(239, 68, 68, 0.2)' }}
                      title="Delete ALL scanner-discovered healthy results completely from database"
                    >
                      <FiTrash2 style={{ marginRight: 6 }} /> Delete Healthy
                    </button>
                    <button 
                      className="btn btn--secondary" 
                      onClick={handleResetSettings} 
                      style={{ flex: 1, justifyContent: 'center' }}
                      title="Reset all scanner settings to default values"
                    >
                      <FiRefreshCw style={{ marginRight: 6 }} /> Reset Settings
                    </button>
                  </div>
                </>
              )}
            </div>
          </div>
        </div>

        {/* Right Column: Visual Telemetry + Output Table + Logs (span 7) */}
        <div className="col-span-12 lg:col-span-7" style={{ display: 'flex', flexDirection: 'column', gap: 20 }}>
          
          {/* Card 1: Radar status & metrics */}
          <div className="g-card" style={{ padding: 20 }}>
            <div style={{ display: 'flex', flexDirection: 'row', alignItems: 'center', gap: 24, flexWrap: 'wrap' }}>
              
              <div style={{ display: 'flex', width: '100%', flexWrap: 'wrap', alignItems: 'center', justifyContent: 'space-around', gap: 20 }}>
                {/* Sonar Radar Graphic */}
                <div style={{ position: 'relative', width: 120, height: 120, borderRadius: '50%', border: '1px solid rgba(255, 107, 44, 0.25)', background: 'radial-gradient(circle, rgba(255, 107, 44, 0.05) 0%, rgba(0,0,0,0) 70%)', overflow: 'hidden', flexShrink: 0 }}>
                  <div style={{ position: 'absolute', inset: 0, borderRadius: '50%', border: '1px solid rgba(255, 107, 44, 0.15)', transform: 'scale(0.66)' }} />
                  <div style={{ position: 'absolute', inset: 0, borderRadius: '50%', border: '1px solid rgba(255, 107, 44, 0.1)', transform: 'scale(0.33)' }} />
                  <div style={{ position: 'absolute', width: '100%', height: '1px', background: 'rgba(255, 107, 44, 0.12)', top: '50%', left: 0 }} />
                  <div style={{ position: 'absolute', height: '100%', width: '1px', background: 'rgba(255, 107, 44, 0.12)', left: '50%', top: 0 }} />
                  
                  {/* Blinking center spot */}
                  <div style={{ position: 'absolute', width: 6, height: 6, borderRadius: '50%', background: 'var(--color-brand)', boxShadow: '0 0 10px var(--color-brand)', left: '50%', top: '50%', transform: 'translate(-50%, -50%)', zIndex: 5 }} />
                  
                  {/* Sweep ray */}
                  <div 
                    className={`clip-radar ${isScanning ? 'animate-radar-sweep' : 'opacity-20'}`}
                    style={{
                      position: 'absolute',
                      width: '50%',
                      height: '50%',
                      top: 0,
                      left: '50%',
                      transformOrigin: 'bottom left',
                      background: 'linear-gradient(to right, rgba(255, 107, 44, 0.4) 0%, rgba(255, 107, 44, 0) 100%)',
                      clipPath: 'polygon(0 100%, 100% 100%, 100% 0)'
                    }}
                  />
                </div>

                {/* Metrics */}
                <div style={{ flex: 1, minWidth: 160 }}>
                  <div style={{ fontSize: 10, fontWeight: 700, color: 'var(--color-brand-muted)', textTransform: 'uppercase', letterSpacing: '1px', marginBottom: 10 }}>
                    {isScanning ? `${stats.phase || 'SCANNING IN PROGRESS...'}` : 'SCAN COMPLETE'}
                  </div>
                  <div style={{ display: 'grid', gridTemplateColumns: '1fr 1fr', gap: 16 }}>
                    <div>
                      <span style={{ display: 'block', fontSize: 10, color: 'var(--color-brand-text)', fontWeight: 600, textTransform: 'uppercase' }}>Scanned</span>
                      <strong style={{ fontSize: 22, fontWeight: 700, color: 'var(--color-brand-heading)' }}>
                        {formatCount(stats.tested)}
                      </strong>
                    </div>
                    <div>
                      <span style={{ display: 'block', fontSize: 10, color: 'var(--color-brand-green)', fontWeight: 600, textTransform: 'uppercase' }}>Alive</span>
                      <strong style={{ fontSize: 22, fontWeight: 700, color: 'var(--color-brand-green)' }}>
                        {formatCount(stats.healthy)}
                      </strong>
                    </div>
                    <div>
                      <span style={{ display: 'block', fontSize: 10, color: 'var(--color-brand-red)', fontWeight: 600, textTransform: 'uppercase' }}>Dead</span>
                      <strong style={{ fontSize: 22, fontWeight: 700, color: 'var(--color-brand-red)' }}>
                        {formatCount(stats.failed)}
                      </strong>
                    </div>
                    <div>
                      <span style={{ display: 'block', fontSize: 10, color: 'var(--color-brand-blue)', fontWeight: 600, textTransform: 'uppercase' }}>Verifying</span>
                      <strong style={{ fontSize: 22, fontWeight: 700, color: 'var(--color-brand-blue)' }}>
                        {stats.in_flight}
                      </strong>
                    </div>
                  </div>
                </div>
              </div>

            </div>
          </div>

          {/* Card: CDN Benchmarking Chart */}
          {candidates.filter(c => c.status === 'healthy' && c.cdnProvider).length > 0 && (
            <div className="g-card" style={{ padding: 20 }}>
              <h3 style={{ fontSize: 11, fontWeight: 700, color: 'var(--color-brand-muted)', textTransform: 'uppercase', letterSpacing: '1px', margin: 0, marginBottom: 14 }}>
                CDN PERFORMANCE BENCHMARKING
              </h3>
              <div style={{ display: 'flex', flexDirection: 'column', gap: 12 }}>
                {(() => {
                  const groups: { [key: string]: { count: number; totalLat: number; totalSpeed: number } } = {};
                  candidates.forEach(c => {
                    if (c.status === 'healthy' && c.cdnProvider) {
                      if (!groups[c.cdnProvider]) {
                        groups[c.cdnProvider] = { count: 0, totalLat: 0, totalSpeed: 0 };
                      }
                      groups[c.cdnProvider].count++;
                      groups[c.cdnProvider].totalLat += c.latencyMs;
                      groups[c.cdnProvider].totalSpeed += c.speedMbps;
                    }
                  });

                  const data = Object.keys(groups).map(name => ({
                    name,
                    count: groups[name].count,
                    avgLat: groups[name].totalLat / groups[name].count,
                    avgSpeed: groups[name].totalSpeed / groups[name].count
                  })).sort((a, b) => b.avgSpeed - a.avgSpeed);

                  const maxSpeed = Math.max(...data.map(d => d.avgSpeed), 1);
                  const maxLat = Math.max(...data.map(d => d.avgLat), 1);

                  return data.map((d, index) => (
                    <div key={d.name} style={{ display: 'flex', flexDirection: 'column', gap: 6, padding: '10px 14px', borderRadius: 8, background: 'var(--color-brand-card)', border: '1px solid var(--color-brand-border)' }}>
                      <div style={{ display: 'flex', justifyContent: 'space-between', alignItems: 'center' }}>
                        <span style={{ fontSize: 12, fontWeight: 700, color: 'var(--color-brand-heading)' }}>
                          {index + 1}. {d.name} <span style={{ fontSize: 10, color: 'var(--color-brand-text)', fontWeight: 500 }}>({d.count} nodes)</span>
                        </span>
                        <div style={{ display: 'flex', gap: 12, fontSize: 11, fontWeight: 600 }}>
                          <span style={{ color: 'var(--color-brand)' }}>{d.avgSpeed.toFixed(2)} MB/s</span>
                          <span style={{ color: 'var(--color-brand-muted)' }}>|</span>
                          <span style={{ color: 'var(--color-brand-green)' }}>{Math.round(d.avgLat)} ms</span>
                        </div>
                      </div>
                      
                      <div style={{ display: 'flex', alignItems: 'center', gap: 10 }}>
                        <span style={{ width: 45, fontSize: 9, fontWeight: 600, color: 'var(--color-brand-text)' }}>SPEED:</span>
                        <div style={{ flex: 1, height: 8, background: 'var(--color-brand-bg)', borderRadius: 4, overflow: 'hidden', border: '1px solid var(--color-brand-border)' }}>
                          <div style={{ width: `${(d.avgSpeed / maxSpeed) * 100}%`, height: '100%', background: 'linear-gradient(90deg, var(--color-brand) 0%, var(--color-brand-blue) 100%)', borderRadius: 4 }} />
                        </div>
                      </div>

                      <div style={{ display: 'flex', alignItems: 'center', gap: 10 }}>
                        <span style={{ width: 45, fontSize: 9, fontWeight: 600, color: 'var(--color-brand-text)' }}>PING:</span>
                        <div style={{ flex: 1, height: 8, background: 'var(--color-brand-bg)', borderRadius: 4, overflow: 'hidden', border: '1px solid var(--color-brand-border)' }}>
                          <div style={{ width: `${(d.avgLat / maxLat) * 100}%`, height: '100%', background: 'linear-gradient(90deg, #10b981 0%, #f59e0b 100%)', borderRadius: 4 }} />
                        </div>
                      </div>
                    </div>
                  ));
                })()}
              </div>
            </div>
          )}

          {/* Card 2: Discovered Candidates Table */}
          <div className="g-card" style={{ padding: 20, display: 'flex', flexDirection: 'column', minHeight: 320, maxHeight: 420 }}>
            <div style={{ display: 'flex', justifyContent: 'space-between', alignItems: 'center', marginBottom: 16, gap: 16, flexWrap: 'wrap' }}>
              <div style={{ display: 'flex', alignItems: 'center', gap: 6 }}>
                <FiList style={{ color: 'var(--color-brand)', fontSize: 16 }} />
                <span style={{ fontSize: 14, fontWeight: 600, color: 'var(--color-brand-heading)' }}>
                  Discovered Proxy Candidates ({filteredCandidates.length})
                </span>
              </div>
              
              <div style={{ display: 'flex', alignItems: 'center', gap: 8, flexWrap: 'wrap' }}>
                <div style={{ position: 'relative', width: 140 }}>
                  <FiSearch style={{ position: 'absolute', left: 8, top: 8, color: 'var(--color-brand-muted)', fontSize: 12 }} />
                  <input
                    type="text"
                    value={searchQuery}
                    onChange={(e) => setSearchQuery(e.target.value)}
                    placeholder="Search..."
                    style={{
                      width: '100%',
                      padding: '5px 10px 5px 26px',
                      borderRadius: 6,
                      border: '1px solid var(--color-brand-border)',
                      background: 'var(--color-brand-bg)',
                      color: 'var(--color-brand-heading)',
                      fontSize: 11,
                      outline: 'none'
                    }}
                  />
                </div>

                <select
                  value={sortBy}
                  onChange={(e: any) => setSortBy(e.target.value)}
                  style={{
                    padding: '5px 8px',
                    borderRadius: 6,
                    border: '1px solid var(--color-brand-border)',
                    background: 'var(--color-brand-bg)',
                    color: 'var(--color-brand-heading)',
                    fontSize: 11,
                    outline: 'none'
                  }}
                >
                  <option value="latency">Latency</option>
                  <option value="speed">Speed</option>
                  <option value="time">Time Added</option>
                </select>

                <button 
                  className="btn btn--sm btn--secondary" 
                  onClick={() => {
                    const healthyList = candidates
                      .filter((c) => c.status === 'healthy')
                      .map((c) => `${c.ip}:${c.port}`)
                      .join('\n');
                    if (healthyList) {
                      navigator.clipboard.writeText(healthyList);
                      setMessage({ type: 'success', text: 'Copied all verified IP:port endpoints to clipboard.' });
                    } else {
                      setMessage({ type: 'info', text: 'No verified candidates available to copy.' });
                    }
                  }}
                  title="Copy verified hosts (shortcut: C)"
                >
                  <FiClipboard /> Copy
                </button>

                <button 
                  className="btn btn--sm btn--secondary" 
                  onClick={downloadTxt}
                  title="Export healthy results to TXT file"
                >
                  <FiFileText /> TXT
                </button>

                <button 
                  className="btn btn--sm btn--secondary" 
                  onClick={downloadCsv}
                  title="Export healthy results to CSV file"
                >
                  <FiDownload /> CSV
                </button>
              </div>
            </div>

            {/* Candidates Table */}
            <div style={{ flex: 1, overflowY: 'auto' }}>
              <table style={{ width: '100%', borderCollapse: 'collapse', fontSize: 12, textAlign: 'left' }}>
                <thead>
                  <tr style={{ borderBottom: '1px solid var(--color-brand-border)' }}>
                    <th style={{ padding: '8px 6px', color: 'var(--color-brand-muted)', fontWeight: 600 }}>Endpoint IP</th>
                    <th style={{ padding: '8px 6px', color: 'var(--color-brand-muted)', fontWeight: 600 }}>Port</th>
                    <th style={{ padding: '8px 6px', color: 'var(--color-brand-muted)', fontWeight: 600 }}>CDN Provider / POP</th>
                    <th style={{ padding: '8px 6px', color: 'var(--color-brand-muted)', fontWeight: 600 }}>Latency</th>
                    <th style={{ padding: '8px 6px', color: 'var(--color-brand-muted)', fontWeight: 600 }}>Packet Loss</th>
                    <th style={{ padding: '8px 6px', color: 'var(--color-brand-muted)', fontWeight: 600 }}>Speed</th>
                    <th style={{ padding: '8px 6px', color: 'var(--color-brand-muted)', fontWeight: 600 }}>Status</th>
                    <th style={{ padding: '8px 6px', color: 'var(--color-brand-muted)', fontWeight: 600, textAlign: 'right' }}>Actions</th>
                  </tr>
                </thead>
                <tbody>
                  {filteredCandidates.length === 0 ? (
                    <tr>
                      <td colSpan={8} style={{ padding: 30, textAlign: 'center', color: 'var(--color-brand-muted)' }}>
                        <FiFileText size={20} style={{ marginBottom: 8, opacity: 0.3, display: 'inline-block' }} />
                        <div>No candidates found.</div>
                      </td>
                    </tr>
                  ) : (
                    filteredCandidates.map((c, idx) => (
                      <tr key={idx} style={{ borderBottom: '1px solid var(--color-brand-border)', verticalAlign: 'middle' }}>
                        <td style={{ padding: '8px 6px', fontWeight: 600, color: 'var(--color-brand-heading)' }}>
                          <IPResolveBadge ip={c.ip} />
                        </td>
                        <td style={{ padding: '8px 6px', color: 'var(--color-brand-heading)' }}>{c.port}</td>
                        <td style={{ padding: '8px 6px', color: 'var(--color-brand-heading)' }}>
                          {c.cdnProvider ? (
                            <div style={{ display: 'flex', alignItems: 'center', gap: 6 }}>
                              <span style={{ fontSize: 11, fontWeight: 700, color: 'var(--color-brand-indigo)' }}>{c.cdnProvider}</span>
                              {c.popLocation && (
                                <span style={{ fontSize: 9, fontWeight: 800, padding: '1px 4px', background: 'var(--color-brand-light)', border: '1px solid var(--color-brand-border)', borderRadius: 4, color: 'var(--color-brand)' }}>
                                  {c.popLocation}
                                </span>
                              )}
                            </div>
                          ) : (
                            <span style={{ color: 'var(--color-brand-muted)', fontSize: 11 }}>-</span>
                          )}
                        </td>
                        <td style={{ padding: '8px 6px', fontWeight: 600, color: c.latencyMs > 0 ? 'var(--color-brand-green)' : 'var(--color-brand-red)' }}>
                          {c.latencyMs > 0 ? `${c.latencyMs} ms` : '-'}
                        </td>
                        <td style={{ padding: '8px 6px' }}>
                          {c.status === 'in_flight' ? (
                            <span style={{ color: 'var(--color-brand-muted)', fontSize: 11 }}>Testing...</span>
                          ) : (
                            <div style={{ display: 'flex', alignItems: 'center', gap: 6 }}>
                              <div style={{ flex: 1, minWidth: 40, height: 6, background: 'var(--color-brand-bg)', borderRadius: 3, overflow: 'hidden', border: '1px solid var(--color-brand-border)' }}>
                                <div style={{ width: `${c.packetLoss}%`, height: '100%', background: c.packetLoss > 50 ? 'var(--color-brand-red)' : c.packetLoss > 0 ? '#f59e0b' : 'var(--color-brand-green)' }} />
                              </div>
                              <span style={{ fontSize: 10, fontWeight: 600, color: c.packetLoss > 0 ? 'var(--color-brand-red)' : 'var(--color-brand-text)' }}>
                                {c.packetLoss}%
                              </span>
                            </div>
                          )}
                        </td>
                        <td style={{ padding: '8px 6px', color: 'var(--color-brand-blue)', fontWeight: 600 }}>
                          {c.speedMbps > 0 ? `${c.speedMbps.toFixed(2)} MB/s` : '-'}
                        </td>
                        <td style={{ padding: '8px 6px' }}>
                          <span style={{
                            fontSize: 9,
                            fontWeight: 600,
                            padding: '3px 8px',
                            borderRadius: 12,
                            background: c.status === 'healthy' ? 'rgba(34,197,94,0.1)' : c.status === 'failed' ? 'rgba(239,68,68,0.1)' : 'rgba(59,130,246,0.1)',
                            color: c.status === 'healthy' ? 'var(--color-brand-green)' : c.status === 'failed' ? 'var(--color-brand-red)' : 'var(--color-brand-blue)',
                          }}>
                            {c.status.toUpperCase()}
                          </span>
                        </td>
                        <td style={{ padding: '8px 6px', textAlign: 'right' }}>
                          <button
                            className="btn btn--xs btn--secondary"
                            onClick={() => {
                              navigator.clipboard.writeText(`${c.ip}:${c.port}`);
                              setMessage({ type: 'success', text: `Copied ${c.ip}:${c.port} to clipboard.` });
                            }}
                            style={{ padding: '3px 6px', fontSize: 10 }}
                          >
                            Copy IP
                          </button>
                        </td>
                      </tr>
                    ))
                  )}
                </tbody>
              </table>
            </div>
          </div>

          {/* Card 3: Monospace Diagnostic Logs */}
          <div className="g-card" style={{ padding: 20, display: 'flex', flexDirection: 'column', height: 280 }}>
            <div style={{ display: 'flex', alignItems: 'center', justifyContent: 'space-between', marginBottom: 12 }}>
              <div style={{ display: 'flex', alignItems: 'center', gap: 6 }}>
                <FiTerminal style={{ color: 'var(--color-brand)', fontSize: 16 }} />
                <span style={{ fontSize: 13, fontWeight: 600, color: 'var(--color-brand-heading)' }}>
                  Live Diagnostic Scanner Logs
                </span>
              </div>
              <div style={{ display: 'flex', alignItems: 'center', gap: 8 }}>
                <input
                  type="text"
                  placeholder="Filter logs..."
                  value={logsFilter}
                  onChange={(e) => setLogsFilter(e.target.value)}
                  style={{
                    width: 120,
                    padding: '4px 8px',
                    borderRadius: 6,
                    border: '1px solid var(--color-brand-border)',
                    background: 'var(--color-brand-bg)',
                    fontSize: 11,
                    color: 'var(--color-brand-heading)',
                    outline: 'none'
                  }}
                />
                <button 
                  className="btn btn--sm btn--secondary" 
                  onClick={() => setScannerLogs([])}
                  title="Clear logs"
                  style={{ display: 'flex', alignItems: 'center', gap: 4 }}
                >
                  <FiTrash2 size={12} /> Clear
                </button>
              </div>
            </div>

            {/* Monospace terminal body */}
            <div
              ref={logsContainerRef}
              style={{
                flex: 1,
                background: 'var(--color-brand-bg)',
                border: '1px solid var(--color-brand-border)',
                borderRadius: 8,
                padding: 12,
                fontFamily: 'Fira Code, Courier, monospace',
                fontSize: 11,
                color: 'var(--color-brand-text)',
                overflowY: 'auto',
                display: 'flex',
                flexDirection: 'column',
                gap: 4
              }}
            >
              {filteredLogs.length === 0 ? (
                <div style={{ color: 'var(--color-brand-muted)', textAlign: 'center', marginTop: 70 }}>
                  No diagnostic logs. Click "Start Sweep" to stream live.
                </div>
              ) : (
                filteredLogs.map((log, idx) => {
                  let color = 'var(--color-brand-text)';
                  if (log.includes('[ERROR]') || log.includes('Critical:') || log.includes('Failed candidate:')) {
                    color = 'var(--color-brand-red)';
                  } else if (log.includes('Healthy candidate:') || log.includes('Success') || log.includes('clean node')) {
                    color = 'var(--color-brand-green)';
                  } else if (log.includes('Initiating') || log.includes('Parameters:')) {
                    color = 'var(--color-brand)';
                  }
                  
                  return (
                    <div key={idx} style={{ wordBreak: 'break-all', whiteSpace: 'pre-wrap', color }}>
                      {log}
                    </div>
                  );
                })
              )}
            </div>
          </div>

        </div>

      </div>

    </div>
  );
};
