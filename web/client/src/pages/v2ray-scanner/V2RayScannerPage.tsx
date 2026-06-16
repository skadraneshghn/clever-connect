import React, { useState, useEffect, useRef } from 'react';
import {
  FiRefreshCw, FiPlay, FiSquare, FiCheckCircle, FiAlertCircle,
  FiGlobe, FiTrash2, FiActivity
} from 'react-icons/fi';
import { useAuthStore } from '../../store/authStore';
import { useGeoStore } from '../../store/geoStore';
import { ScannerConfig } from './components/ScannerConfig';
import { TelemetryLogs } from './components/TelemetryLogs';
import { CandidateTable } from './components/CandidateTable';
import type { ScannerSource } from './components/ScannerConfig';
import type { ScannedCandidate } from './components/CandidateTable';

export const V2RayScannerPage: React.FC = () => {
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

  // Candidates list & buffer for high performance batching
  const [candidates, setCandidates] = useState<ScannedCandidate[]>([]);
  const candidatesBufferRef = useRef<Record<string, ScannedCandidate>>({});
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

  // Flush buffer to candidates state periodically to prevent browser freezing
  useEffect(() => {
    const interval = setInterval(() => {
      const buffer = candidatesBufferRef.current;
      const keys = Object.keys(buffer);
      if (keys.length === 0) return;

      // Clear the buffer
      candidatesBufferRef.current = {};

      setCandidates((prev) => {
        const map = new Map<string, ScannedCandidate>();
        
        // Load previous items (if there's a match, it will be overwritten by the buffer)
        prev.forEach(item => map.set(`${item.ip}:${item.port}`, item));

        // Merge buffer updates
        keys.forEach((k) => {
          map.set(k, buffer[k]);
        });

        // Convert map back to list.
        // To retain chronological sorting for 'time added', we should keep the same array ordering style
        // by putting new candidates at the start. So let's extract new buffer elements vs modified ones.
        const mergedList = Array.from(map.values());
        
        return mergedList;
      });
    }, 250);

    return () => clearInterval(interval);
  }, []);

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
            const newCand: ScannedCandidate = {
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
            };
            candidatesBufferRef.current[`${c.ip}:${c.port}`] = newCand;
          }

          if (msg.event === 'scanner.finished') {
            setIsScanning(false);
            setMessage({ type: 'success', text: 'Scanning operations completed.' });
            fetchSavedConfigs(true);
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
  const fetchSavedConfigs = async (merge: boolean | any = false) => {
    try {
      const shouldMerge = merge === true;
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
        if (shouldMerge) {
          setCandidates((prev) => {
            const map = new Map<string, ScannedCandidate>();
            prev.forEach(item => map.set(`${item.ip}:${item.port}`, item));
            parsed.forEach(dbCand => {
              const key = `${dbCand.ip}:${dbCand.port}`;
              const existing = map.get(key);
              if (existing) {
                map.set(key, {
                  ...existing,
                  ...dbCand,
                  time: 'Saved',
                });
              } else {
                map.set(key, dbCand);
              }
            });
            return Array.from(map.values());
          });
        } else {
          setCandidates(parsed);
        }
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
    setSelectedPortsList([443, 2053, 2083, 2087, 2096, 8443, 80, 2052, 2082, 2086, 2095, 8080]);
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
        copyAllHealthy();
      }
    };

    window.addEventListener('keydown', handleKeyDown);
    return () => {
      window.removeEventListener('keydown', handleKeyDown);
    };
  }, [candidates]);

  const copyAllHealthy = () => {
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
  };

  const handleCopyItem = (ip: string, port: number) => {
    navigator.clipboard.writeText(`${ip}:${port}`);
    setMessage({ type: 'success', text: `Copied ${ip}:${port} to clipboard.` });
  };

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
          const commonPorts = [443, 2053, 2083, 2087, 2096, 8443, 80, 2052, 2082, 2086, 2095, 8080];
          const common = commonPorts.filter(p => data.ports.includes(p));
          setSelectedPortsList(common);
          const custom = data.ports.filter((p: number) => !commonPorts.includes(p));
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

  // Memoized CDN performance metrics
  const cdnData = React.useMemo(() => {
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

    return Object.keys(groups).map(name => ({
      name,
      count: groups[name].count,
      avgLat: groups[name].totalLat / groups[name].count,
      avgSpeed: groups[name].totalSpeed / groups[name].count
    })).sort((a, b) => b.avgSpeed - a.avgSpeed);
  }, [candidates]);

  const maxSpeed = React.useMemo(() => Math.max(...cdnData.map(d => d.avgSpeed), 1), [cdnData]);
  const maxLat = React.useMemo(() => Math.max(...cdnData.map(d => d.avgLat), 1), [cdnData]);

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
          <ScannerConfig
            selectedCDNs={selectedCDNs}
            setSelectedCDNs={setSelectedCDNs}
            selectedPortsList={selectedPortsList}
            setSelectedPortsList={setSelectedPortsList}
            customPorts={customPorts}
            setCustomPorts={setCustomPorts}
            sources={sources}
            showAddSourceInline={showAddSourceInline}
            setShowAddSourceInline={setShowAddSourceInline}
            newSourceName={newSourceName}
            setNewSourceName={setNewSourceName}
            newSourceUrl={newSourceUrl}
            setNewSourceUrl={setNewSourceUrl}
            newSourceType={newSourceType}
            setNewSourceType={setNewSourceType}
            showAdvancedSettings={showAdvancedSettings}
            setShowAdvancedSettings={setShowAdvancedSettings}
            isDragging={isDragging}
            rawConfigLink={rawConfigLink}
            setRawConfigLink={setRawConfigLink}
            targetCidrs={targetCidrs}
            setTargetCidrs={setTargetCidrs}
            concurrencyLimit={concurrencyLimit}
            setConcurrencyLimit={setConcurrencyLimit}
            maxRateLimit={maxRateLimit}
            setMaxRateLimit={setMaxRateLimit}
            networkTimeoutMs={networkTimeoutMs}
            setNetworkTimeoutMs={setNetworkTimeoutMs}
            probeAttempts={probeAttempts}
            setProbeAttempts={setProbeAttempts}
            targetMode={targetMode}
            setTargetMode={setTargetMode}
            targetSni={targetSni}
            setTargetSni={setTargetSni}
            websocketHost={websocketHost}
            setWebsocketHost={setWebsocketHost}
            websocketPath={websocketPath}
            setWebsocketPath={setWebsocketPath}
            requireWs={requireWs}
            setRequireWs={setRequireWs}
            enableNeighbors={enableNeighbors}
            setEnableNeighbors={setEnableNeighbors}
            topLimit={topLimit}
            setTopLimit={setTopLimit}
            totalTargetCount={totalTargetCount}
            setTotalTargetCount={setTotalTargetCount}
            handleParseLink={handleParseLink}
            handleToggleSource={handleToggleSource}
            handleDeleteSource={handleDeleteSource}
            handleResetSources={handleResetSources}
            handleAddSourceSubmit={handleAddSourceSubmit}
            handleSelectAllPorts={handleSelectAllPorts}
            handleClearPorts={handleClearPorts}
            handleTogglePort={handleTogglePort}
            handleDragEnter={handleDragEnter}
            handleDragOver={handleDragOver}
            handleDragLeave={handleDragLeave}
            handleDrop={handleDrop}
          />

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
          {cdnData.length > 0 && (
            <div className="g-card" style={{ padding: 20 }}>
              <h3 style={{ fontSize: 11, fontWeight: 700, color: 'var(--color-brand-muted)', textTransform: 'uppercase', letterSpacing: '1px', margin: 0, marginBottom: 14 }}>
                CDN PERFORMANCE BENCHMARKING
              </h3>
              <div style={{ display: 'flex', flexDirection: 'column', gap: 12 }}>
                {cdnData.map((d, index) => (
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
                ))}
              </div>
            </div>
          )}

          {/* Virtualized Candidates list/table component */}
          <CandidateTable
            candidates={candidates}
            searchQuery={searchQuery}
            setSearchQuery={setSearchQuery}
            sortBy={sortBy}
            setSortBy={setSortBy}
            onCopyAll={copyAllHealthy}
            onDownloadTxt={downloadTxt}
            onDownloadCsv={downloadCsv}
            onCopyItem={handleCopyItem}
          />

          {/* Diagnostics scanner logs Body */}
          <TelemetryLogs
            scannerLogs={scannerLogs}
            logsFilter={logsFilter}
            setLogsFilter={setLogsFilter}
            setScannerLogs={setScannerLogs}
          />
        </div>
      </div>
    </div>
  );
};
