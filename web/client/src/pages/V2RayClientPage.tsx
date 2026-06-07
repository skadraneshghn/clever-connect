import React, { useState, useEffect, useRef } from 'react';
import { 
  FiSliders, FiCpu, FiGlobe, FiKey, FiPlay, FiSquare, FiSave, FiRefreshCw, 
  FiEye, FiEyeOff, FiHelpCircle, FiTerminal, FiDownloadCloud, FiPlus, 
  FiTrash2, FiActivity, FiSearch, FiZap, FiWifi, FiMonitor, FiSettings, 
  FiAlertCircle, FiLock, FiLogOut, FiCheck, FiX
} from 'react-icons/fi';import { useVirtualizer } from '@tanstack/react-virtual';

export const V2RayClientPage: React.FC = () => {
  // Help modal state
  const [helpTitle, setHelpTitle] = useState<string | null>(null);
  const [helpText, setHelpText] = useState<string | null>(null);

  const showHelp = (title: string, text: string) => {
    setHelpTitle(title);
    setHelpText(text);
  };

  // State definitions
  const [isRunning, setIsRunning] = useState(false);
  const [isLoading, setIsLoading] = useState(false);
  const [logs, setLogs] = useState<string[]>([]);
  const [logsQuery, setLogsQuery] = useState('');
  const [message, setMessage] = useState<{ type: 'success' | 'error', text: string } | null>(null);

  // Settings
  const [selectedCore, setSelectedCore] = useState('xray');
  const [socksPort, setSocksPort] = useState(10808);
  const [httpPort, setHttpPort] = useState(10809);
  const [muxEnabled, setMuxEnabled] = useState(true);
  const [dnsServer, setDnsServer] = useState('8.8.8.8');
  const [routingPreset, setRoutingPreset] = useState('bypass_domestic');
  const [customRouting, setCustomRouting] = useState('');

  // Evasion settings
  const [evasionFingerprint, setEvasionFingerprint] = useState('chrome');
  const [evasionFragment, setEvasionFragment] = useState(true);
  const [fragmentMode, setFragmentMode] = useState('default');
  const [fragmentPackets, setFragmentPackets] = useState('tlshello');
  const [fragmentLength, setFragmentLength] = useState('100-200');
  const [fragmentInterval, setFragmentInterval] = useState('10-20');
  const [evasionEch, setEvasionEch] = useState(false);
  const [evasionEchConfig, setEvasionEchConfig] = useState('');
  const [evasionTcpBrutal, setEvasionTcpBrutal] = useState(false);

  // Subscriptions & Profiles (Infinite Scroll / Windowing)
  const [subUrl, setSubUrl] = useState('');
  const [profiles, setProfiles] = useState<any[]>([]);
  const [totalProfiles, setTotalProfiles] = useState(0);
  const [pageOffset, setPageOffset] = useState(0);
  const PAGE_LIMIT = 50;
  const [activeProfileId, setActiveProfileId] = useState<number | null>(null);
  const [manualUri, setManualUri] = useState('');

  // Port prober state
  const [probeIP, setProbeIP] = useState('8.8.8.8');
  const [probePorts, setProbePorts] = useState('53,80,443');
  const [probeProto, setProbeProto] = useState('tcp');
  const [probeResults, setProbeResults] = useState<any[]>([]);

  // Wake on LAN state
  const [wolMac, setWolMac] = useState('');
  const [wolBcast, setWolBcast] = useState('255.255.255.255');

  // Local device discovery
  const [discoveredDevices, setDiscoveredDevices] = useState<any[]>([]);
  const [isDiscovering, setIsDiscovering] = useState(false);

  // Debug interception proxy state
  const [debugProxyPort, setDebugProxyPort] = useState(8080);
  const [isDebugProxyActive, setIsDebugProxyActive] = useState(false);
  const [debugProxyLogs, setDebugProxyLogs] = useState<string[]>([]);

  // Hotkeys & System tray
  const [hotkeys, setHotkeys] = useState('Ctrl+Shift+X');
  const [systemTrayEnabled, setSystemTrayEnabled] = useState(true);

  const qrFileInputRef = useRef<HTMLInputElement | null>(null);
  const [selectedProfileIds, setSelectedProfileIds] = useState<number[]>([]);
  const [cdnRanges, setCdnRanges] = useState('104.16.0.0/16');
  const [cdnScannerActive, setCdnScannerActive] = useState(false);
  const [cdnScanStatus, setCdnScanStatus] = useState<any>(null);
  const [speedTestActive, setSpeedTestActive] = useState(false);
  const [speedTestBreakdown, setSpeedTestBreakdown] = useState<any>(null);

  const logsEndRef = useRef<HTMLDivElement | null>(null);

  // Clipboard Mass Import States
  const [isClipboardModalOpen, setIsClipboardModalOpen] = useState(false);
  const [clipboardCount, setClipboardCount] = useState(0);
  const [clipboardPage, setClipboardPage] = useState(0);
  const [clipboardSearch, setClipboardSearch] = useState('');
  const [clipboardUpdateTrigger, setClipboardUpdateTrigger] = useState(0);
  const [isImportingBulk, setIsImportingBulk] = useState(false);
  const [isParsing, setIsParsing] = useState(false);
  const [parseProgress, setParseProgress] = useState(0);

  // Refs for zero-render high performance
  const parsedConfigsRef = useRef<any[]>([]);
  const deselectedSetRef = useRef<Set<number>>(new Set());

  // Load configs
  const loadSettings = async () => {
    setIsLoading(true);
    try {
      const token = localStorage.getItem('cc_client_token') || '';
      // Fetch core settings
      const sResp = await fetch('/api/v2ray/client/settings', {
        headers: { 'Authorization': `Bearer ${token}` }
      });
      if (sResp.ok) {
        const data = await sResp.json();
        setSelectedCore(data.v2ray_core || 'xray');
        setFragmentMode(data.fragment_mode || 'default');
        setFragmentPackets(data.fragment_packets || 'tlshello');
        setFragmentLength(data.fragment_length || '100-200');
        setFragmentInterval(data.fragment_interval || '10-20');
        setSocksPort(data.socks_port || 10808);
        setHttpPort(data.http_port || 10809);
        setMuxEnabled(data.mux_enabled);
        setDnsServer(data.dns_server || '8.8.8.8');
        setRoutingPreset(data.routing_preset || 'bypass_domestic');
        setCustomRouting(data.custom_routing || '');
        setEvasionFingerprint(data.evasion_fingerprint || 'chrome');
        setEvasionFragment(data.evasion_fragment);
        setEvasionEch(data.evasion_ech);
        setEvasionEchConfig(data.evasion_ech_config || '');
        setEvasionTcpBrutal(data.evasion_tcp_brutal);
      }

      // Fetch core status
      const stResp = await fetch('/api/v2ray/client/status', {
        headers: { 'Authorization': `Bearer ${token}` }
      });
      if (stResp.ok) {
        const statusData = await stResp.json();
        setIsRunning(statusData.is_running);
      }

      // Fetch first page of profiles
      fetchProfiles(0, true);

      // Fetch logs
      fetchLogs();
    } catch (err) {
      console.error(err);
    } finally {
      setIsLoading(false);
    }
  };

  const fetchProfiles = async (offset: number, reset: boolean = false) => {
    try {
      const token = localStorage.getItem('cc_client_token') || '';
      const pResp = await fetch(`/api/v2ray/client/configs?offset=${offset}&limit=${PAGE_LIMIT}`, {
        headers: { 'Authorization': `Bearer ${token}` }
      });
      if (pResp.ok) {
        const pList = await pResp.json();
        const data = pList.data || [];
        setTotalProfiles(pList.total || 0);
        
        if (reset) {
          setProfiles(data);
          setPageOffset(offset);
        } else {
          setProfiles(prev => [...prev, ...data]);
          setPageOffset(offset);
        }

        const active = data.find((p: any) => p.is_active);
        if (active) setActiveProfileId(active.ID);
      }
    } catch (err) {
      console.error('Failed to fetch profiles', err);
    }
  };

  const fetchLogs = async () => {
    try {
      const token = localStorage.getItem('cc_client_token') || '';
      const response = await fetch(`/api/v2ray/client/logs?q=${encodeURIComponent(logsQuery)}`, {
        headers: { 'Authorization': `Bearer ${token}` }
      });
      if (response.ok) {
        const data = await response.json();
        setLogs(data || []);
      }
    } catch (err) {
      console.error(err);
    }
  };

  useEffect(() => {
    loadSettings();
    const ticker = setInterval(() => {
      fetchLogs();
      if (isDebugProxyActive) fetchProxyLogs();
      if (cdnScannerActive) fetchCDNScanStatus();
    }, 4000);
    return () => clearInterval(ticker);
  }, [logsQuery, isDebugProxyActive, cdnScannerActive]);

  useEffect(() => {
    if (logsEndRef.current) {
      logsEndRef.current.scrollIntoView({ behavior: 'smooth' });
    }
  }, [logs]);

  // Actions
  const handleSaveSettings = async (e: React.FormEvent) => {
    e.preventDefault();
    setIsLoading(true);
    setMessage(null);
    try {
      const token = localStorage.getItem('cc_client_token') || '';
      const res = await fetch('/api/v2ray/client/settings', {
        method: 'POST',
        headers: {
          'Content-Type': 'application/json',
          'Authorization': `Bearer ${token}`
        },
        body: JSON.stringify({
          v2ray_core: selectedCore,
          fragment_mode: fragmentMode,
          fragment_packets: fragmentPackets,
          fragment_length: fragmentLength,
          fragment_interval: fragmentInterval,
          socks_port: Number(socksPort),
          http_port: Number(httpPort),
          mux_enabled: muxEnabled,
          dns_server: dnsServer,
          routing_preset: routingPreset,
          custom_routing: customRouting,
          evasion_fingerprint: evasionFingerprint,
          evasion_fragment: evasionFragment,
          evasion_ech: evasionEch,
          evasion_ech_config: evasionEchConfig,
          evasion_tcp_brutal: evasionTcpBrutal
        })
      });
      if (res.ok) {
        setMessage({ type: 'success', text: 'V2Ray client settings updated successfully!' });
        loadSettings();
      } else {
        const data = await res.json();
        setMessage({ type: 'error', text: data.error || 'Failed to update settings.' });
      }
    } catch (err: any) {
      setMessage({ type: 'error', text: err.message });
    } finally {
      setIsLoading(false);
    }
  };

  const handleStartCore = async () => {
    setIsLoading(true);
    setMessage(null);
    try {
      const token = localStorage.getItem('cc_client_token') || '';
      const res = await fetch('/api/v2ray/client/start', {
        method: 'POST',
        headers: { 'Authorization': `Bearer ${token}` }
      });
      if (res.ok) {
        setIsRunning(true);
        setMessage({ type: 'success', text: 'V2Ray Client engine started successfully!' });
      } else {
        const data = await res.json();
        setMessage({ type: 'error', text: data.error || 'Failed to start proxy core.' });
      }
    } catch (err: any) {
      setMessage({ type: 'error', text: err.message });
    } finally {
      setIsLoading(false);
    }
  };

  const handleStopCore = async () => {
    setIsLoading(true);
    setMessage(null);
    try {
      const token = localStorage.getItem('cc_client_token') || '';
      const res = await fetch('/api/v2ray/client/stop', {
        method: 'POST',
        headers: { 'Authorization': `Bearer ${token}` }
      });
      if (res.ok) {
        setIsRunning(false);
        setMessage({ type: 'success', text: 'V2Ray Client engine stopped.' });
      } else {
        const data = await res.json();
        setMessage({ type: 'error', text: data.error || 'Failed to stop proxy core.' });
      }
    } catch (err: any) {
      setMessage({ type: 'error', text: err.message });
    } finally {
      setIsLoading(false);
    }
  };

  const handleImportSub = async () => {
    if (!subUrl) return;
    setIsLoading(true);
    setMessage(null);
    try {
      const token = localStorage.getItem('cc_client_token') || '';
      const res = await fetch('/api/v2ray/client/subscriptions', {
        method: 'POST',
        headers: {
          'Content-Type': 'application/json',
          'Authorization': `Bearer ${token}`
        },
        body: JSON.stringify({ url: subUrl })
      });
      if (res.ok) {
        setSubUrl('');
        setMessage({ type: 'success', text: 'Subscription sync trigger added! Reloading profiles...' });
        loadSettings();
      } else {
        const data = await res.json();
        setMessage({ type: 'error', text: data.error || 'Failed to import subscription.' });
      }
    } catch (err: any) {
      setMessage({ type: 'error', text: err.message });
    } finally {
      setIsLoading(false);
    }
  };

  const handleManualImport = async () => {
    if (!manualUri) return;
    setIsLoading(true);
    setMessage(null);
    try {
      const token = localStorage.getItem('cc_client_token') || '';
      const res = await fetch('/api/v2ray/client/configs/import-manual', {
        method: 'POST',
        headers: {
          'Content-Type': 'application/json',
          'Authorization': `Bearer ${token}`
        },
        body: JSON.stringify({ uri: manualUri })
      });
      if (res.ok) {
        setManualUri('');
        setMessage({ type: 'success', text: 'Manual profile imported successfully!' });
        loadSettings();
      } else {
        const data = await res.json();
        setMessage({ type: 'error', text: data.error || 'Import failed.' });
      }
    } catch (err: any) {
      setMessage({ type: 'error', text: err.message });
    } finally {
      setIsLoading(false);
    }
  };

  const processPastedTextChunked = (text: string) => {
    setIsParsing(true);
    setParseProgress(0);
    parsedConfigsRef.current = [];
    deselectedSetRef.current = new Set();
    
    if (!text || !text.trim()) {
      setClipboardCount(0);
      setClipboardPage(0);
      setIsParsing(false);
      return;
    }

    const lines = text.split(/\r?\n/);
    const totalLines = lines.length;
    let index = 0;
    const chunkSize = 15000;
    const parsed: any[] = [];

    const parseNextChunk = () => {
      const limit = Math.min(index + chunkSize, totalLines);
      for (let i = index; i < limit; i++) {
        const line = lines[i].trim();
        if (!line) continue;
        
        const match = line.match(/^([a-zA-Z0-9]+):\/\/(.*)$/);
        if (match) {
          const proto = match[1].toLowerCase();
          if (['vmess', 'vless', 'trojan', 'ss', 'shadowsocks'].includes(proto)) {
            const rest = match[2];
            let name = 'Node ' + (parsed.length + 1);
            let mainPart = rest;
            const hashIdx = rest.indexOf('#');
            if (hashIdx !== -1) {
              mainPart = rest.substring(0, hashIdx);
              try {
                name = decodeURIComponent(rest.substring(hashIdx + 1));
              } catch (_) {
                name = rest.substring(hashIdx + 1);
              }
            }
            
            let host = '';
            let port = '443';
            const atIdx = mainPart.indexOf('@');
            if (atIdx !== -1) {
              const serverPart = mainPart.substring(atIdx + 1).split('?')[0];
              const parts = serverPart.split(':');
              host = parts[0];
              port = parts[1] || '443';
            } else if (proto === 'vmess') {
              try {
                const decoded = atob(mainPart);
                const obj = JSON.parse(decoded);
                host = obj.add || '';
                port = obj.port || '443';
                if (obj.ps) name = obj.ps;
              } catch (_) {}
            }
            
            parsed.push({
              raw: line,
              protocol: proto,
              name,
              host: host || 'Dynamic Host',
              port
            });
          }
        }
      }

      index = limit;
      setParseProgress(Math.round((index / totalLines) * 100));

      if (index < totalLines) {
        setTimeout(parseNextChunk, 0);
      } else {
        parsedConfigsRef.current = parsed;
        setClipboardCount(parsed.length);
        setClipboardPage(0);
        setIsParsing(false);
      }
    };

    parseNextChunk();
  };

  const handleImportBulk = async () => {
    const selectedUris: string[] = [];
    parsedConfigsRef.current.forEach((c, idx) => {
      if (!deselectedSetRef.current.has(idx)) {
        selectedUris.push(c.raw);
      }
    });

    if (selectedUris.length === 0) {
      alert('Please select at least one configuration to import.');
      return;
    }

    setIsImportingBulk(true);
    let importedCount = 0;
    const batchSize = 2500;
    const token = localStorage.getItem('cc_client_token') || '';
    
    try {
      for (let i = 0; i < selectedUris.length; i += batchSize) {
        const batch = selectedUris.slice(i, i + batchSize);
        const res = await fetch('/api/v2ray/client/configs/import-bulk', {
          method: 'POST',
          headers: {
            'Content-Type': 'application/json',
            'Authorization': `Bearer ${token}`
          },
          body: JSON.stringify({ uris: batch })
        });
        if (res.ok) {
          const data = await res.json();
          importedCount += data.count || 0;
        }
      }
      setMessage({ type: 'success', text: `Successfully imported ${importedCount} profiles from clipboard!` });
      setIsClipboardModalOpen(false);
      loadSettings();
    } catch (err: any) {
      setMessage({ type: 'error', text: 'Bulk import failed: ' + err.message });
    } finally {
      setIsImportingBulk(false);
    }
  };

  const BuildProxyLink = (p: any) => {
    if (p.protocol === 'vless' || p.protocol === 'vmess' || p.protocol === 'trojan' || p.protocol === 'shadowsocks' || p.protocol === 'ss') {
      const proto = p.protocol === 'shadowsocks' ? 'ss' : p.protocol;
      let url = `${proto}://${p.uuid || ''}@${p.address}:${p.port}`;
      const params: string[] = [];
      if (p.network) params.push(`type=${p.network}`);
      if (p.tls_mode) params.push(`security=${p.tls_mode}`);
      if (p.sni) params.push(`sni=${p.sni}`);
      if (p.path) params.push(`path=${p.path}`);
      if (params.length > 0) {
        url += `?${params.join('&')}`;
      }
      if (p.name) {
        url += `#${encodeURIComponent(p.name)}`;
      }
      return url;
    }
    return '';
  };

  const handleQRImport = async (e: React.ChangeEvent<HTMLInputElement>) => {
    const file = e.target.files?.[0];
    if (!file) return;
    setIsLoading(true);
    setMessage(null);
    try {
      const token = localStorage.getItem('cc_client_token') || '';
      const formData = new FormData();
      formData.append('file', file);
      const res = await fetch('/api/v2ray/client/configs/import-qr', {
        method: 'POST',
        headers: { 'Authorization': `Bearer ${token}` },
        body: formData
      });
      if (res.ok) {
        setMessage({ type: 'success', text: 'QR code imported successfully!' });
        loadSettings();
      } else {
        const data = await res.json();
        setMessage({ type: 'error', text: data.error || 'Failed to decode QR code.' });
      }
    } catch (err: any) {
      setMessage({ type: 'error', text: err.message });
    } finally {
      setIsLoading(false);
      if (qrFileInputRef.current) qrFileInputRef.current.value = '';
    }
  };

  const handleExportPDF = async () => {
    if (selectedProfileIds.length === 0) {
      setMessage({ type: 'error', text: 'Please select at least one profile to export.' });
      return;
    }
    setIsLoading(true);
    try {
      const token = localStorage.getItem('cc_client_token') || '';
      const res = await fetch('/api/v2ray/client/export-pdf', {
        method: 'POST',
        headers: {
          'Content-Type': 'application/json',
          'Authorization': `Bearer ${token}`
        },
        body: JSON.stringify({ ids: selectedProfileIds })
      });
      if (res.ok) {
        const blob = await res.blob();
        const url = window.URL.createObjectURL(blob);
        const a = document.createElement('a');
        a.href = url;
        a.download = 'clever_configs_export.pdf';
        document.body.appendChild(a);
        a.click();
        a.remove();
        setMessage({ type: 'success', text: 'PDF export downloaded successfully!' });
      } else {
        const err = await res.json();
        setMessage({ type: 'error', text: err.error || 'Failed to export PDF.' });
      }
    } catch (err: any) {
      setMessage({ type: 'error', text: err.message });
    } finally {
      setIsLoading(false);
    }
  };

  const handleStartCDNScan = async () => {
    if (profiles.length === 0) {
      setMessage({ type: 'error', text: 'Please import a template profile first.' });
      return;
    }
    const targetProfile = profiles.find(p => p.ID === activeProfileId) || profiles[0];
    const link = BuildProxyLink(targetProfile);
    if (!link) {
      setMessage({ type: 'error', text: 'Could not construct valid config link from active profile.' });
      return;
    }

    setIsLoading(true);
    try {
      const token = localStorage.getItem('cc_client_token') || '';
      const res = await fetch('/api/v2ray/client/scan-cdn', {
        method: 'POST',
        headers: {
          'Content-Type': 'application/json',
          'Authorization': `Bearer ${token}`
        },
        body: JSON.stringify({
          uri: link,
          ranges: cdnRanges.split('\n').map(r => r.trim()).filter(Boolean),
          per_range_limit: 10,
          max_scan_cap: 100,
          ports: [443],
          top_for_speed: 5,
          final_count: 3,
          download_bytes: 1000000,
          upload_bytes: 500000,
          ping_timeout_sec: 2,
          speed_timeout_sec: 10,
          ping_concurrency: 20,
          speed_conc: 2,
          base_port: 20000
        })
      });
      if (res.ok) {
        setCdnScannerActive(true);
        setMessage({ type: 'success', text: 'CDN IP Scan initialized!' });
      } else {
        const err = await res.json();
        setMessage({ type: 'error', text: err.error || 'Scanner failed to start.' });
      }
    } catch (err: any) {
      setMessage({ type: 'error', text: err.message });
    } finally {
      setIsLoading(false);
    }
  };

  const handleStopCDNScan = async () => {
    setIsLoading(true);
    try {
      const token = localStorage.getItem('cc_client_token') || '';
      const res = await fetch('/api/v2ray/client/scan-cdn/stop', {
        method: 'POST',
        headers: { 'Authorization': `Bearer ${token}` }
      });
      if (res.ok) {
        setCdnScannerActive(false);
        setMessage({ type: 'success', text: 'CDN IP scanner stopped by user request.' });
      }
    } catch (err) {
      console.error(err);
    } finally {
      setIsLoading(false);
    }
  };

  const fetchCDNScanStatus = async () => {
    try {
      const token = localStorage.getItem('cc_client_token') || '';
      const res = await fetch('/api/v2ray/client/scan-cdn/status', {
        headers: { 'Authorization': `Bearer ${token}` }
      });
      if (res.ok) {
        const status = await res.json();
        setCdnScanStatus(status);
        if (status && !status.is_running) {
          setCdnScannerActive(false);
        }
      }
    } catch (err) {
      console.error(err);
    }
  };

  const handleRunSpeedTest = async () => {
    if (!isRunning) {
      setMessage({ type: 'error', text: 'V2Ray client proxy is not running. Please connect first.' });
      return;
    }
    setSpeedTestActive(true);
    setSpeedTestBreakdown(null);
    try {
      const token = localStorage.getItem('cc_client_token') || '';
      const res = await fetch('/api/v2ray/client/speed-test', {
        method: 'POST',
        headers: {
          'Content-Type': 'application/json',
          'Authorization': `Bearer ${token}`
        },
        body: JSON.stringify({ size_bytes: 10000000 })
      });
      if (res.ok) {
        const data = await res.json();
        setSpeedTestBreakdown(data);
      } else {
        const err = await res.json();
        setMessage({ type: 'error', text: err.error || 'Speed test failed.' });
      }
    } catch (err: any) {
      setMessage({ type: 'error', text: err.message });
    } finally {
      setSpeedTestActive(false);
    }
  };

  const handleSelectProfile = async (id: number) => {
    setIsLoading(true);
    try {
      const token = localStorage.getItem('cc_client_token') || '';
      const res = await fetch(`/api/v2ray/client/configs/${id}/active`, {
        method: 'POST',
        headers: { 'Authorization': `Bearer ${token}` }
      });
      if (res.ok) {
        setActiveProfileId(id);
        setMessage({ type: 'success', text: 'Active outbound profile modified!' });
        loadSettings();
      }
    } catch (err) {
      console.error(err);
    } finally {
      setIsLoading(false);
    }
  };

  const handleDeleteProfile = async (id: number) => {
    if (!window.confirm('Delete this outbound configuration?')) return;
    setIsLoading(true);
    try {
      const token = localStorage.getItem('cc_client_token') || '';
      const res = await fetch(`/api/v2ray/client/configs/${id}`, {
        method: 'DELETE',
        headers: { 'Authorization': `Bearer ${token}` }
      });
      if (res.ok) {
        setMessage({ type: 'success', text: 'Outbound profile deleted.' });
        loadSettings();
      }
    } catch (err) {
      console.error(err);
    } finally {
      setIsLoading(false);
    }
  };

  const handleDeleteAllNodes = async () => {
    if (!window.confirm('Are you sure you want to delete ALL outbound configurations? This action cannot be undone!')) return;
    setIsLoading(true);
    try {
      const token = localStorage.getItem('cc_client_token') || '';
      const res = await fetch('/api/v2ray/client/configs/all', {
        method: 'DELETE',
        headers: { 'Authorization': `Bearer ${token}` }
      });
      if (res.ok) {
        setMessage({ type: 'success', text: 'All outbound profiles have been deleted.' });
        loadSettings();
      } else {
        const data = await res.json();
        setMessage({ type: 'error', text: data.error || 'Failed to delete nodes.' });
      }
    } catch (err) {
      console.error(err);
      setMessage({ type: 'error', text: 'An unexpected error occurred while deleting nodes.' });
    } finally {
      setIsLoading(false);
    }
  };

  const handleTestLatency = async () => {
    setIsLoading(true);
    setMessage({ type: 'success', text: 'Running parallel RTT latency test sweep...' });
    try {
      const token = localStorage.getItem('cc_client_token') || '';
      const res = await fetch('/api/v2ray/client/test-mass', {
        method: 'POST',
        headers: { 'Authorization': `Bearer ${token}` }
      });
      if (res.ok) {
        loadSettings();
      }
    } catch (err) {
      console.error(err);
    } finally {
      setIsLoading(false);
    }
  };

  // Diagnostic actions
  const handleProbePorts = async () => {
    if (!probeIP || !probePorts) return;
    setIsLoading(true);
    setProbeResults([]);
    try {
      const token = localStorage.getItem('cc_client_token') || '';
      const portsArr = probePorts.split(',').map(p => Number(p.trim())).filter(p => !isNaN(p));
      const res = await fetch('/api/v2ray/client/probe-ports', {
        method: 'POST',
        headers: {
          'Content-Type': 'application/json',
          'Authorization': `Bearer ${token}`
        },
        body: JSON.stringify({
          ip: probeIP,
          ports: portsArr,
          protocol: probeProto
        })
      });
      if (res.ok) {
        const data = await res.json();
        setProbeResults(data);
      }
    } catch (err) {
      console.error(err);
    } finally {
      setIsLoading(false);
    }
  };

  const handleSendWol = async () => {
    if (!wolMac) return;
    setIsLoading(true);
    setMessage(null);
    try {
      const token = localStorage.getItem('cc_client_token') || '';
      const res = await fetch('/api/v2ray/client/wol', {
        method: 'POST',
        headers: {
          'Content-Type': 'application/json',
          'Authorization': `Bearer ${token}`
        },
        body: JSON.stringify({
          mac: wolMac,
          broadcast_ip: wolBcast
        })
      });
      if (res.ok) {
        setMessage({ type: 'success', text: 'Magic WOL broadcast packet dispatched!' });
      } else {
        const data = await res.json();
        setMessage({ type: 'error', text: data.error || 'Failed to dispatch packet.' });
      }
    } catch (err: any) {
      setMessage({ type: 'error', text: err.message });
    } finally {
      setIsLoading(false);
    }
  };

  const handleDiscoverDevices = async () => {
    setIsDiscovering(true);
    setDiscoveredDevices([]);
    try {
      const token = localStorage.getItem('cc_client_token') || '';
      const res = await fetch('/api/v2ray/client/discover', {
        headers: { 'Authorization': `Bearer ${token}` }
      });
      if (res.ok) {
        const data = await res.json();
        setDiscoveredDevices(data || []);
      }
    } catch (err) {
      console.error(err);
    } finally {
      setIsDiscovering(false);
    }
  };

  // Interception proxy
  const handleToggleDebugProxy = async () => {
    setIsLoading(true);
    try {
      const token = localStorage.getItem('cc_client_token') || '';
      const endpoint = isDebugProxyActive 
        ? '/api/v2ray/client/debug-proxy/stop'
        : '/api/v2ray/client/debug-proxy/start';
      const body = isDebugProxyActive 
        ? undefined 
        : JSON.stringify({ port: Number(debugProxyPort) });

      const res = await fetch(endpoint, {
        method: 'POST',
        headers: {
          'Content-Type': 'application/json',
          'Authorization': `Bearer ${token}`
        },
        body: body
      });

      if (res.ok) {
        setIsDebugProxyActive(!isDebugProxyActive);
        setDebugProxyLogs([]);
      }
    } catch (err) {
      console.error(err);
    } finally {
      setIsLoading(false);
    }
  };

  const fetchProxyLogs = async () => {
    try {
      const token = localStorage.getItem('cc_client_token') || '';
      const res = await fetch('/api/v2ray/client/debug-proxy/logs', {
        headers: { 'Authorization': `Bearer ${token}` }
      });
      if (res.ok) {
        const data = await res.json();
        setDebugProxyLogs(data || []);
      }
    } catch (err) {
      console.error(err);
    }
  };

  const getLatencyColor = (ms: number) => {
    if (ms <= 0) return 'var(--color-brand-muted)';
    if (ms < 100) return 'var(--color-brand-green)';
    if (ms < 300) return '#f59e0b';
    return 'var(--color-brand-red)';
  };

  // Virtualized Infinite Scroll Setup
  const parentRef = useRef<HTMLDivElement>(null);
  
  const rowVirtualizer = useVirtualizer({
    count: profiles.length < totalProfiles ? profiles.length + 1 : profiles.length,
    getScrollElement: () => parentRef.current,
    estimateSize: () => 48,
    overscan: 5,
  });

  useEffect(() => {
    const virtualItems = rowVirtualizer.getVirtualItems();
    if (!virtualItems.length) return;
    const lastItem = virtualItems[virtualItems.length - 1];
    
    if (
      lastItem.index >= profiles.length - 1 &&
      profiles.length < totalProfiles &&
      !isLoading
    ) {
      fetchProfiles(pageOffset + PAGE_LIMIT);
    }
  }, [
    rowVirtualizer.getVirtualItems(),
    profiles.length,
    totalProfiles,
    isLoading,
    pageOffset,
  ]);

  return (
    <div>
      {/* Title */}
      <div style={{ display: 'flex', justifyContent: 'space-between', alignItems: 'center', marginBottom: 24 }}>
        <div>
          <h1 style={{ fontSize: 22, fontWeight: 700, color: 'var(--color-brand-heading)', margin: 0 }}>V2Ray / Xray manager</h1>
          <p style={{ fontSize: 12, color: 'var(--color-brand-text)', margin: '4px 0 0' }}>
            Zero-TUN Censorship Evasion panel supporting VLESS, VMess, Reality, and dynamic routing settings.
          </p>
        </div>
        <button className="btn btn--sm" onClick={loadSettings} disabled={isLoading}>
          <FiRefreshCw className={isLoading ? 'spin-animation' : ''} style={{ marginRight: 6 }} /> Refresh State
        </button>
      </div>

      {message && (
        <div style={{
          padding: '12px 16px',
          borderRadius: 10,
          marginBottom: 20,
          fontSize: 13,
          fontWeight: 500,
          background: message.type === 'success' ? 'var(--color-brand-light)' : '#fee2e2',
          border: message.type === 'success' ? '1px solid var(--color-brand-border)' : '1px solid #fca5a5',
          color: message.type === 'success' ? 'var(--color-brand)' : '#b91c1c'
        }}>
          {message.text}
        </div>
      )}

      {/* Grid Layout */}
      <div style={{ display: 'grid', gridTemplateColumns: '1fr 360px', gap: 24, alignItems: 'start' }}>
        
        {/* Left Side: Outbounds, Configurations & Evasion */}
        <div style={{ display: 'flex', flexDirection: 'column', gap: 24 }}>
          
           {/* Active Engine controls */}
          <div className="g-card" style={{ display: 'flex', flexDirection: 'column', gap: 14 }}>
            <div style={{ display: 'flex', alignItems: 'center', justifyContent: 'space-between' }}>
              <div>
                <div style={{ display: 'flex', alignItems: 'center', gap: 8 }}>
                  <span className="live-dot" style={{ background: isRunning ? '#10b981' : '#ef4444' }} />
                  <span style={{ fontSize: 15, fontWeight: 700, color: 'var(--color-brand-heading)' }}>
                    Core Supervisor: {isRunning ? 'RUNNING' : 'STOPPED'}
                  </span>
                  <FiHelpCircle 
                    style={{ cursor: 'pointer', color: 'var(--color-brand-muted)' }} 
                    onClick={() => showHelp('V2Ray Core Supervisor', 'Manages the local Xray daemon lifecycle in the background. On start, compiles database rules into an optimized JSON config, ensures SOCKS5 and HTTP inbound listeners bind safely without port conflicts, and launches Xray.')}
                  />
                </div>
                <span style={{ fontSize: 11, color: 'var(--color-brand-text)', display: 'block', marginTop: 4 }}>
                  Local inbound captures: SOCKS5 on port {socksPort} & HTTP on port {httpPort}.
                </span>
              </div>

              <div style={{ display: 'flex', gap: 10, alignItems: 'center' }}>
                {isRunning && (
                  <button 
                    onClick={handleRunSpeedTest}
                    className="btn btn--sm btn--secondary"
                    style={{ borderColor: 'var(--color-brand)', color: 'var(--color-brand)', display: 'flex', alignItems: 'center', gap: 4 }}
                    disabled={speedTestActive}
                  >
                    <FiActivity size={13} className={speedTestActive ? 'spin-animation' : ''} /> {speedTestActive ? 'Testing speed...' : 'Run Speed Test'}
                  </button>
                )}
                <button 
                  onClick={handleStartCore} 
                  className="btn btn--primary" 
                  style={{ display: 'flex', alignItems: 'center', gap: 6, background: isRunning ? '#a3a3a3' : undefined }}
                  disabled={isRunning || isLoading}
                >
                  <FiPlay /> Start
                </button>
                <button 
                  onClick={handleStopCore} 
                  className="btn btn--secondary" 
                  style={{ display: 'flex', alignItems: 'center', gap: 6, borderColor: '#ef4444', color: '#ef4444' }}
                  disabled={!isRunning || isLoading}
                >
                  <FiSquare /> Stop
                </button>
              </div>
            </div>

            {speedTestBreakdown && (
              <div style={{
                background: 'var(--color-brand-light)',
                border: '1px solid var(--color-brand-border)',
                borderRadius: 8,
                padding: '10px 14px',
                fontSize: 12,
                display: 'flex',
                justifyContent: 'space-between',
                color: 'var(--color-brand-heading)'
              }}>
                <div><strong>Throughput Speed:</strong> <span style={{ color: 'var(--color-brand)', fontWeight: 700 }}>{(speedTestBreakdown.throughput_mbps).toFixed(2)} Mbps</span></div>
                <div><strong>TLS Handshake:</strong> <span style={{ fontFamily: 'monospace' }}>{speedTestBreakdown.tls_handshake_ms}ms</span></div>
                <div><strong>TTFB (First Byte):</strong> <span style={{ fontFamily: 'monospace' }}>{speedTestBreakdown.ttfb_ms}ms</span></div>
                <div><strong>TCP Conn:</strong> <span style={{ fontFamily: 'monospace' }}>{speedTestBreakdown.tcp_conn_ms}ms</span></div>
              </div>
            )}
          </div>

          {/* Subscriptions & Profiles */}
          <div className="g-card">
            <div style={{ display: 'flex', alignItems: 'center', justifyContent: 'space-between', marginBottom: 16 }}>
              <div style={{ display: 'flex', alignItems: 'center', gap: 8 }}>
                <FiDownloadCloud style={{ color: 'var(--color-brand)', fontSize: 18 }} />
                <span style={{ fontSize: 15, fontWeight: 600, color: 'var(--color-brand-heading)' }}>Subscriptions & Profiles</span>
                <FiHelpCircle 
                  style={{ cursor: 'pointer', color: 'var(--color-brand-muted)' }} 
                  onClick={() => showHelp('Subscriptions & Profiles', 'Sync and import remote proxy configuration files. Add URL links to sync periodically, or paste raw URI configurations directly. Use QR image uploads or batch export profiles as PDF files.')}
                />
              </div>
              <div style={{ display: 'flex', gap: 8 }}>
                <button className="btn btn--sm btn--secondary" onClick={handleTestLatency} disabled={isLoading}>
                  <FiActivity style={{ marginRight: 6 }} /> Test Latencies
                </button>
                <button className="btn btn--sm btn--primary" onClick={handleExportPDF} disabled={isLoading || selectedProfileIds.length === 0}>
                  Export Selected PDF ({selectedProfileIds.length})
                </button>
              </div>
            </div>

            {/* Input URL */}
            <div style={{ display: 'flex', gap: 10, marginBottom: 12 }}>
              <input
                type="text"
                placeholder="Subscription Link (HTTP/S Base64)"
                value={subUrl}
                onChange={(e) => setSubUrl(e.target.value)}
                style={{
                  flex: 1,
                  padding: '8px 12px',
                  borderRadius: 8,
                  border: '1px solid var(--color-brand-border)',
                  background: 'var(--color-brand-card)',
                  fontSize: 13,
                  color: 'var(--color-brand-heading)'
                }}
              />
              <button className="btn btn--primary" onClick={handleImportSub} disabled={isLoading}>Import</button>
            </div>

            {/* Manual import & QR Upload */}
            <div style={{ display: 'flex', flexDirection: 'column', gap: 12, marginBottom: 20 }}>
              <div style={{ display: 'flex', gap: 10 }}>
                <input
                  type="text"
                  placeholder="Manual Config URI (vmess://, vless://, trojan://, ss://)"
                  value={manualUri}
                  onChange={(e) => setManualUri(e.target.value)}
                  style={{
                    flex: 1,
                    padding: '8px 12px',
                    borderRadius: 8,
                    border: '1px solid var(--color-brand-border)',
                    background: 'var(--color-brand-card)',
                    fontSize: 13,
                    color: 'var(--color-brand-heading)'
                  }}
                />
                <button className="btn btn--secondary" onClick={handleManualImport} disabled={isLoading}>Import URI</button>
                <button 
                  className="btn" 
                  type="button" 
                  onClick={() => {
                    parsedConfigsRef.current = [];
                    deselectedSetRef.current = new Set();
                    setClipboardCount(0);
                    setClipboardPage(0);
                    setClipboardSearch('');
                    setIsClipboardModalOpen(true);
                  }}
                  style={{ background: 'var(--color-brand)', color: '#fff', border: 'none', display: 'flex', alignItems: 'center' }}
                >
                  Clipboard Import
                </button>
                <button 
                  className="btn" 
                  type="button" 
                  onClick={handleDeleteAllNodes}
                  disabled={isLoading || profiles.length === 0}
                  style={{ background: '#dc3545', color: '#fff', border: 'none', display: 'flex', alignItems: 'center' }}
                >
                  Delete All Nodes
                </button>
              </div>
              <div style={{ display: 'flex', alignItems: 'center', gap: 10, background: 'var(--color-brand-bg)', padding: '10px 12px', borderRadius: 8, border: '1px solid var(--color-brand-border)' }}>
                <span style={{ fontSize: 12, fontWeight: 600, color: 'var(--color-brand-heading)' }}>Import Config via QR Code Image:</span>
                <input
                  type="file"
                  accept="image/*"
                  ref={qrFileInputRef}
                  onChange={handleQRImport}
                  style={{ fontSize: 12, color: 'var(--color-brand-text)' }}
                  disabled={isLoading}
                />
              </div>
            </div>

            {/* Table of Profiles */}
            <div ref={parentRef} style={{ maxHeight: 600, overflow: 'auto', border: '1px solid var(--color-brand-border)', borderRadius: 8 }}>
              <table style={{ width: '100%', borderCollapse: 'collapse', fontSize: 12, textAlign: 'left' }}>
                <thead style={{ position: 'sticky', top: 0, zIndex: 1, background: 'var(--color-brand-bg)' }}>
                  <tr style={{ borderBottom: '1px solid var(--color-brand-border)' }}>
                    <th style={{ padding: '10px 12px', color: 'var(--color-brand-heading)', width: 50 }}>Active</th>
                    <th style={{ padding: '10px 12px', color: 'var(--color-brand-heading)', width: 50 }}>Select</th>
                    <th style={{ padding: '10px 12px', color: 'var(--color-brand-heading)' }}>Name</th>
                    <th style={{ padding: '10px 12px', color: 'var(--color-brand-heading)' }}>Protocol</th>
                    <th style={{ padding: '10px 12px', color: 'var(--color-brand-heading)' }}>Address</th>
                    <th style={{ padding: '10px 12px', color: 'var(--color-brand-heading)' }}>Ping</th>
                    <th style={{ padding: '10px 12px', color: 'var(--color-brand-heading)', textAlign: 'center' }}>Actions</th>
                  </tr>
                </thead>
                <tbody>
                  {profiles.length === 0 ? (
                    <tr>
                      <td colSpan={7} style={{ padding: 20, textAlign: 'center', color: 'var(--color-brand-muted)' }}>
                        No profiles imported. Add subscription URL or paste configs.
                      </td>
                    </tr>
                  ) : (
                    <>
                      {rowVirtualizer.getVirtualItems()[0]?.start > 0 && (
                        <tr><td colSpan={7} style={{ height: rowVirtualizer.getVirtualItems()[0].start }} /></tr>
                      )}
                      {rowVirtualizer.getVirtualItems().map((virtualRow) => {
                        const p = profiles[virtualRow.index];
                        if (!p) return null;
                        return (
                          <tr key={virtualRow.key} style={{ height: virtualRow.size, borderBottom: '1px solid var(--color-brand-border)', background: p.ID === activeProfileId ? 'var(--color-brand-light)' : 'none' }}>
                            <td style={{ padding: '10px 12px' }}>
                              <input
                                type="radio"
                                name="active_profile"
                                checked={p.ID === activeProfileId}
                                onChange={() => handleSelectProfile(p.ID)}
                                style={{ cursor: 'pointer', accentColor: 'var(--color-brand)' }}
                              />
                            </td>
                            <td style={{ padding: '10px 12px' }}>
                              <input
                                type="checkbox"
                                checked={selectedProfileIds.includes(p.ID)}
                                onChange={(e) => {
                                  if (e.target.checked) {
                                    setSelectedProfileIds([...selectedProfileIds, p.ID]);
                                  } else {
                                    setSelectedProfileIds(selectedProfileIds.filter(id => id !== p.ID));
                                  }
                                }}
                                style={{ cursor: 'pointer', accentColor: 'var(--color-brand)' }}
                              />
                            </td>
                            <td style={{ padding: '10px 12px', fontWeight: 600, color: 'var(--color-brand-heading)' }}>{p.name}</td>
                            <td style={{ padding: '10px 12px', textTransform: 'uppercase' }}>
                              <span style={{
                                padding: '2px 6px',
                                borderRadius: 4,
                                background: '#e0f2fe',
                                color: '#0369a1',
                                fontSize: 10,
                                fontWeight: 700
                              }}>{p.protocol}</span>
                            </td>
                            <td style={{ padding: '10px 12px', color: 'var(--color-brand-text)' }}>{p.address}:{p.port}</td>
                            <td style={{ padding: '10px 12px' }}>
                              <span style={{
                                fontWeight: 700,
                                color: getLatencyColor(p.latency_ms)
                              }}>{p.latency_ms > 0 ? `${p.latency_ms}ms` : 'N/A'}</span>
                            </td>
                            <td style={{ padding: '10px 12px', textAlign: 'center' }}>
                              <button 
                                onClick={() => handleDeleteProfile(p.ID)} 
                                style={{ background: 'none', border: 'none', cursor: 'pointer', color: 'var(--color-brand-red)' }}
                              >
                                <FiTrash2 size={14} />
                              </button>
                            </td>
                          </tr>
                        );
                      })}
                      {rowVirtualizer.getVirtualItems().length > 0 && (
                        <tr><td colSpan={7} style={{ height: rowVirtualizer.getTotalSize() - rowVirtualizer.getVirtualItems()[rowVirtualizer.getVirtualItems().length - 1].end }} /></tr>
                      )}
                      {isLoading && profiles.length < totalProfiles && (
                        <tr><td colSpan={7} style={{ padding: 10, textAlign: 'center', color: 'var(--color-brand-muted)' }}>Loading more...</td></tr>
                      )}
                    </>
                  )}
                </tbody>
              </table>
            </div>
          </div>

          {/* CDN IP Scanner & Optimizer */}
          <div className="g-card" style={{ display: 'flex', flexDirection: 'column', gap: 16 }}>
            <div style={{ display: 'flex', alignItems: 'center', justifyContent: 'space-between' }}>
              <div style={{ display: 'flex', alignItems: 'center', gap: 8 }}>
                <FiWifi style={{ color: 'var(--color-brand)', fontSize: 18 }} />
                <span style={{ fontSize: 15, fontWeight: 600, color: 'var(--color-brand-heading)' }}>CDN IP Auto-Scanner & Optimizer</span>
                <FiHelpCircle 
                  style={{ cursor: 'pointer', color: 'var(--color-brand-muted)' }} 
                  onClick={() => showHelp('CDN IP Scanner', 'Performs massively parallel port scans and throughput speed tests on CDN edge ranges (e.g. Cloudflare) to auto-discover clean, high-performance IP addresses and inject them as working configurations.')}
                />
              </div>
              <div style={{ display: 'flex', gap: 8 }}>
                <button 
                  className="btn btn--sm btn--primary" 
                  onClick={handleStartCDNScan}
                  disabled={isLoading || cdnScannerActive}
                >
                  Start Scan
                </button>
                <button 
                  className="btn btn--sm btn--secondary" 
                  onClick={handleStopCDNScan}
                  style={{ borderColor: '#ef4444', color: '#ef4444' }}
                  disabled={!cdnScannerActive}
                >
                  Stop Scan
                </button>
              </div>
            </div>

            <div>
              <label style={{ display: 'block', fontSize: 11, fontWeight: 600, color: 'var(--color-brand-muted)', marginBottom: 6, textTransform: 'uppercase' }}>Target IP CIDR Ranges (One per line)</label>
              <textarea
                value={cdnRanges}
                onChange={(e) => setCdnRanges(e.target.value)}
                placeholder="104.16.0.0/16&#10;172.64.0.0/13"
                rows={3}
                style={{
                  width: '100%',
                  padding: '8px 12px',
                  borderRadius: 8,
                  border: '1px solid var(--color-brand-border)',
                  background: 'var(--color-brand-card)',
                  fontSize: 12,
                  fontFamily: 'monospace',
                  color: 'var(--color-brand-heading)'
                }}
              />
            </div>

            {cdnScanStatus && (
              <div style={{
                background: 'var(--color-brand-bg)',
                border: '1px solid var(--color-brand-border)',
                borderRadius: 8,
                padding: 12,
                fontSize: 12
              }}>
                <div style={{ display: 'grid', gridTemplateColumns: '1fr 1fr', gap: 10, marginBottom: 10 }}>
                  <div><strong>State:</strong> <span style={{ color: 'var(--color-brand)', fontWeight: 700 }}>{cdnScanStatus.is_running ? 'SCANNING' : 'IDLE / FINISHED'}</span></div>
                  <div><strong>Scanned Count:</strong> {cdnScanStatus.scanned_count || 0}</div>
                  <div><strong>Live IPs Found:</strong> {cdnScanStatus.live_count || 0}</div>
                  <div><strong>Active Workers:</strong> {cdnScanStatus.workers_active || 0}</div>
                </div>

                {cdnScanStatus.top_results && cdnScanStatus.top_results.length > 0 && (
                  <div>
                    <span style={{ fontSize: 11, fontWeight: 700, color: 'var(--color-brand-heading)', display: 'block', marginBottom: 6 }}>Top Discovered CDN IPs</span>
                    <div style={{ maxHeight: 120, overflowY: 'auto', border: '1px solid var(--color-brand-border)', borderRadius: 6 }}>
                      <table style={{ width: '100%', borderCollapse: 'collapse', fontSize: 11 }}>
                        <thead>
                          <tr style={{ background: 'var(--color-brand-bg)', borderBottom: '1px solid var(--color-brand-border)' }}>
                            <th style={{ padding: '6px 8px', textAlign: 'left' }}>IP Address</th>
                            <th style={{ padding: '6px 8px', textAlign: 'left' }}>Ping (RTT)</th>
                            <th style={{ padding: '6px 8px', textAlign: 'left' }}>Speed</th>
                          </tr>
                        </thead>
                        <tbody>
                          {cdnScanStatus.top_results.map((res: any, idx: number) => (
                            <tr key={idx} style={{ borderBottom: '1px solid var(--color-brand-border)' }}>
                              <td style={{ padding: '6px 8px', fontFamily: 'monospace' }}>{res.ip}</td>
                              <td style={{ padding: '6px 8px', color: 'var(--color-brand-green)', fontWeight: 700 }}>{res.ping_ms}ms</td>
                              <td style={{ padding: '6px 8px' }}>{(res.speed_mbps).toFixed(2)} Mbps</td>
                            </tr>
                          ))}
                        </tbody>
                      </table>
                    </div>
                  </div>
                )}
              </div>
            )}
          </div>

          {/* Configuration Form */}
          <form onSubmit={handleSaveSettings} className="g-card" style={{ display: 'flex', flexDirection: 'column', gap: 18 }}>
            <div style={{ display: 'flex', alignItems: 'center', gap: 8 }}>
              <FiSliders style={{ color: 'var(--color-brand)', fontSize: 18 }} />
              <span style={{ fontSize: 15, fontWeight: 600, color: 'var(--color-brand-heading)' }}>Proxy Configurations</span>
              <FiHelpCircle 
                style={{ cursor: 'pointer', color: 'var(--color-brand-muted)' }} 
                onClick={() => showHelp('Proxy Configurations', 'Configure inbound local ports for network application mapping. Set custom DNS resolving servers to avoid ISP redirection leaks.')}
              />
            </div>

            <div style={{ display: 'grid', gridTemplateColumns: '1fr', gap: 16 }}>
              <div>
                <label style={{ display: 'block', fontSize: 11, fontWeight: 600, color: 'var(--color-brand-muted)', marginBottom: 6, textTransform: 'uppercase' }}>Default Core</label>
                <select
                  value={selectedCore}
                  onChange={(e) => setSelectedCore(e.target.value)}
                  style={{
                    width: '100%',
                    padding: '8px 12px',
                    borderRadius: 8,
                    border: '1px solid var(--color-brand-border)',
                    background: 'var(--color-brand-card)',
                    fontSize: 13,
                    color: 'var(--color-brand-heading)'
                  }}
                >
                  <option value="xray">Xray (Highly Recommended, supports Reality & XTLS)</option>
                  <option value="v2ray">V2Ray (Standard core, strips XTLS/Reality/Brutal)</option>
                  <option value="sing-box">Sing-Box (Next-gen core, supports urltest & DNS routes)</option>
                </select>
              </div>
            </div>

            <div style={{ display: 'grid', gridTemplateColumns: '1fr 1fr', gap: 16 }}>
              <div>
                <label style={{ display: 'block', fontSize: 11, fontWeight: 600, color: 'var(--color-brand-muted)', marginBottom: 6, textTransform: 'uppercase' }}>SOCKS5 Port</label>
                <input
                  type="number"
                  value={socksPort}
                  onChange={(e) => setSocksPort(Number(e.target.value))}
                  style={{
                    width: '100%',
                    padding: '8px 12px',
                    borderRadius: 8,
                    border: '1px solid var(--color-brand-border)',
                    background: 'var(--color-brand-card)',
                    fontSize: 13,
                    color: 'var(--color-brand-heading)'
                  }}
                  required
                />
              </div>

              <div>
                <label style={{ display: 'block', fontSize: 11, fontWeight: 600, color: 'var(--color-brand-muted)', marginBottom: 6, textTransform: 'uppercase' }}>HTTP Port</label>
                <input
                  type="number"
                  value={httpPort}
                  onChange={(e) => setHttpPort(Number(e.target.value))}
                  style={{
                    width: '100%',
                    padding: '8px 12px',
                    borderRadius: 8,
                    border: '1px solid var(--color-brand-border)',
                    background: 'var(--color-brand-card)',
                    fontSize: 13,
                    color: 'var(--color-brand-heading)'
                  }}
                  required
                />
              </div>
            </div>

            <div style={{ display: 'grid', gridTemplateColumns: '1fr 1fr', gap: 16 }}>
              <div>
                <label style={{ display: 'block', fontSize: 11, fontWeight: 600, color: 'var(--color-brand-muted)', marginBottom: 6, textTransform: 'uppercase' }}>DNS Resolver Server</label>
                <input
                  type="text"
                  value={dnsServer}
                  onChange={(e) => setDnsServer(e.target.value)}
                  style={{
                    width: '100%',
                    padding: '8px 12px',
                    borderRadius: 8,
                    border: '1px solid var(--color-brand-border)',
                    background: 'var(--color-brand-card)',
                    fontSize: 13,
                    color: 'var(--color-brand-heading)'
                  }}
                  required
                />
              </div>

              <div>
                <label style={{ display: 'block', fontSize: 11, fontWeight: 600, color: 'var(--color-brand-muted)', marginBottom: 6, textTransform: 'uppercase' }}>Routing Preset</label>
                <select
                  value={routingPreset}
                  onChange={(e) => setRoutingPreset(e.target.value)}
                  style={{
                    width: '100%',
                    padding: '8px 12px',
                    borderRadius: 8,
                    border: '1px solid var(--color-brand-border)',
                    background: 'var(--color-brand-card)',
                    fontSize: 13,
                    color: 'var(--color-brand-heading)'
                  }}
                >
                  <option value="global">Global (Route All traffic through Proxy)</option>
                  <option value="bypass_domestic">Bypass Iran (GeoIP:IR + Geosite:IR go Direct)</option>
                  <option value="block_ads">Block Ads (Reject Ad hosts, others proxy)</option>
                  <option value="custom">Custom (Compile User Custom Rules)</option>
                </select>
              </div>
            </div>

            {routingPreset === 'custom' && (
              <div>
                <label style={{ display: 'block', fontSize: 11, fontWeight: 600, color: 'var(--color-brand-muted)', marginBottom: 6, textTransform: 'uppercase' }}>Custom Routing Rules (JSON array)</label>
                <textarea
                  value={customRouting}
                  onChange={(e) => setCustomRouting(e.target.value)}
                  placeholder='[{"type": "field", "outboundTag": "direct", "domain": ["geosite:ir"]}]'
                  rows={4}
                  style={{
                    width: '100%',
                    padding: '10px 12px',
                    borderRadius: 8,
                    border: '1px solid var(--color-brand-border)',
                    background: 'var(--color-brand-card)',
                    fontSize: 12,
                    fontFamily: 'Fira Code',
                    color: 'var(--color-brand-heading)'
                  }}
                />
              </div>
            )}

            {/* DPI Evasion & Security */}
            <div style={{ borderTop: '1px solid var(--color-brand-border)', paddingTop: 16 }}>
              <div style={{ display: 'flex', alignItems: 'center', gap: 6, marginBottom: 14 }}>
                <FiLock style={{ color: 'var(--color-brand)', fontSize: 16 }} />
                <span style={{ fontSize: 13, fontWeight: 600, color: 'var(--color-brand-heading)' }}>DPI Evasion & Security Hardening</span>
                <FiHelpCircle 
                  style={{ cursor: 'pointer', color: 'var(--color-brand-muted)' }} 
                  onClick={() => showHelp('DPI Evasion & Security Hardening', 'TLS fragmentation splits ClientHello records to slip past SNI deep packet filters. Browser fingerprints masquerade TLS signatures as standard web browsers (Chrome/Safari). ECH hides hostnames, and TCP Brutal speeds up output on congested lossy paths.')}
                />
              </div>

              <div style={{ display: 'flex', flexDirection: 'column', gap: 12 }}>
                <div style={{ display: 'flex', flexDirection: 'column', gap: 8 }}>
                  <div style={{ display: 'flex', alignItems: 'center', justifyContent: 'space-between' }}>
                    <div>
                      <label style={{ fontSize: 12, fontWeight: 600, color: 'var(--color-brand-heading)', margin: 0 }}>
                        TLS Record-Layer Fragmentation
                      </label>
                      <span style={{ fontSize: 9, color: 'var(--color-brand-text)', display: 'block' }}>
                        Splits client handshake packets into segments to slip past SNI deep packet filters.
                      </span>
                    </div>
                    <input
                      type="checkbox"
                      checked={evasionFragment}
                      onChange={(e) => setEvasionFragment(e.target.checked)}
                      style={{ width: 16, height: 16, cursor: 'pointer', accentColor: 'var(--color-brand)' }}
                    />
                  </div>

                  {evasionFragment && (
                    <div style={{
                      padding: 12,
                      background: 'var(--color-brand-bg)',
                      border: '1px solid var(--color-brand-border)',
                      borderRadius: 8,
                      display: 'flex',
                      flexDirection: 'column',
                      gap: 12,
                      marginTop: 4,
                      marginLeft: 8
                    }}>
                      <div>
                        <label style={{ display: 'block', fontSize: 11, fontWeight: 600, color: 'var(--color-brand-muted)', marginBottom: 4, textTransform: 'uppercase' }}>Fragment Mode</label>
                        <select
                          value={fragmentMode}
                          onChange={(e) => setFragmentMode(e.target.value)}
                          style={{
                            width: '100%',
                            padding: '6px 10px',
                            borderRadius: 6,
                            border: '1px solid var(--color-brand-border)',
                            background: 'var(--color-brand-card)',
                            fontSize: 12,
                            color: 'var(--color-brand-heading)'
                          }}
                        >
                          <option value="default">Default Mode (embedded/custom values)</option>
                          <option value="domain">SNI / Domain Mode (splits early to destroy SNI signatures)</option>
                          <option value="random">Random Mode (micro-chunks at irregular random intervals)</option>
                        </select>
                      </div>

                      {fragmentMode === 'default' && (
                        <div style={{ display: 'grid', gridTemplateColumns: '1fr 1fr', gap: 10 }}>
                          <div>
                            <label style={{ display: 'block', fontSize: 10, fontWeight: 600, color: 'var(--color-brand-muted)', marginBottom: 4, textTransform: 'uppercase' }}>Length Range</label>
                            <input
                              type="text"
                              value={fragmentLength}
                              onChange={(e) => setFragmentLength(e.target.value)}
                              placeholder="e.g. 100-200"
                              style={{
                                width: '100%',
                                padding: '6px 10px',
                                borderRadius: 6,
                                border: '1px solid var(--color-brand-border)',
                                background: 'var(--color-brand-card)',
                                fontSize: 12,
                                color: 'var(--color-brand-heading)'
                              }}
                            />
                          </div>
                          <div>
                            <label style={{ display: 'block', fontSize: 10, fontWeight: 600, color: 'var(--color-brand-muted)', marginBottom: 4, textTransform: 'uppercase' }}>Interval (ms)</label>
                            <input
                              type="text"
                              value={fragmentInterval}
                              onChange={(e) => setFragmentInterval(e.target.value)}
                              placeholder="e.g. 10-20"
                              style={{
                                width: '100%',
                                padding: '6px 10px',
                                borderRadius: 6,
                                border: '1px solid var(--color-brand-border)',
                                background: 'var(--color-brand-card)',
                                fontSize: 12,
                                color: 'var(--color-brand-heading)'
                              }}
                            />
                          </div>
                        </div>
                      )}
                    </div>
                  )}
                </div>

                <div style={{ display: 'flex', alignItems: 'center', justifyContent: 'space-between' }}>
                  <div>
                    <label style={{ fontSize: 12, fontWeight: 600, color: 'var(--color-brand-heading)', margin: 0 }}>
                      TCP Brutal Congestion Control
                    </label>
                    <span style={{ fontSize: 9, color: 'var(--color-brand-text)', display: 'block' }}>
                      Enforces aggressive window scaling and packet loss compensation over direct outbounds.
                    </span>
                  </div>
                  <input
                    type="checkbox"
                    checked={evasionTcpBrutal}
                    onChange={(e) => setEvasionTcpBrutal(e.target.checked)}
                    style={{ width: 16, height: 16, cursor: 'pointer', accentColor: 'var(--color-brand)' }}
                  />
                </div>

                <div style={{ display: 'flex', alignItems: 'center', justifyContent: 'space-between' }}>
                  <div>
                    <label style={{ fontSize: 12, fontWeight: 600, color: 'var(--color-brand-heading)', margin: 0 }}>
                      Enable Connection Multiplexing (Mux)
                    </label>
                    <span style={{ fontSize: 9, color: 'var(--color-brand-text)', display: 'block' }}>
                      Reuses TCP socket handlers to prevent firewall port tracking blocks.
                    </span>
                  </div>
                  <input
                    type="checkbox"
                    checked={muxEnabled}
                    onChange={(e) => setMuxEnabled(e.target.checked)}
                    style={{ width: 16, height: 16, cursor: 'pointer', accentColor: 'var(--color-brand)' }}
                  />
                </div>

                <div style={{ display: 'grid', gridTemplateColumns: '1fr 1fr', gap: 16, marginTop: 4 }}>
                  <div>
                    <label style={{ display: 'block', fontSize: 11, fontWeight: 600, color: 'var(--color-brand-muted)', marginBottom: 6, textTransform: 'uppercase' }}>uTLS Browser Fingerprint</label>
                    <select
                      value={evasionFingerprint}
                      onChange={(e) => setEvasionFingerprint(e.target.value)}
                      style={{
                        width: '100%',
                        padding: '8px 12px',
                        borderRadius: 8,
                        border: '1px solid var(--color-brand-border)',
                        background: 'var(--color-brand-card)',
                        fontSize: 13,
                        color: 'var(--color-brand-heading)'
                      }}
                    >
                      <option value="chrome">Google Chrome (Standard)</option>
                      <option value="firefox">Mozilla Firefox</option>
                      <option value="safari">Apple Safari</option>
                      <option value="edge">Microsoft Edge</option>
                      <option value="randomized">Randomized uTLS Fingerprint</option>
                    </select>
                  </div>

                  <div>
                    <label style={{ display: 'block', fontSize: 11, fontWeight: 600, color: 'var(--color-brand-muted)', marginBottom: 6, textTransform: 'uppercase' }}>Encrypted Client Hello (ECH)</label>
                    <div style={{ display: 'flex', alignItems: 'center', gap: 8 }}>
                      <input
                        type="checkbox"
                        checked={evasionEch}
                        onChange={(e) => setEvasionEch(e.target.checked)}
                        style={{ width: 16, height: 16, cursor: 'pointer', accentColor: 'var(--color-brand)' }}
                      />
                      <span style={{ fontSize: 12, color: 'var(--color-brand-text)' }}>Enable ECH</span>
                    </div>
                  </div>
                </div>

                {evasionEch && (
                  <div>
                    <label style={{ display: 'block', fontSize: 11, fontWeight: 600, color: 'var(--color-brand-muted)', marginBottom: 6, textTransform: 'uppercase' }}>ECH Base64 Config Payload</label>
                    <textarea
                      value={evasionEchConfig}
                      onChange={(e) => setEvasionEchConfig(e.target.value)}
                      placeholder="Paste ECHConfigs Base64 string"
                      rows={2}
                      style={{
                        width: '100%',
                        padding: '8px 10px',
                        borderRadius: 8,
                        border: '1px solid var(--color-brand-border)',
                        background: 'var(--color-brand-card)',
                        fontSize: 12,
                        fontFamily: 'Fira Code',
                        color: 'var(--color-brand-heading)'
                      }}
                    />
                  </div>
                )}
              </div>
            </div>

            <button type="submit" className="btn btn--primary" style={{ display: 'flex', alignItems: 'center', gap: 6 }} disabled={isLoading}>
              <FiSave /> Save Proxy Settings
            </button>
          </form>

        </div>

        {/* Right Side: Log terminal, diagnostic probers, wol, and debug proxy */}
        <div style={{ display: 'flex', flexDirection: 'column', gap: 24 }}>
          
          {/* Active Logs Terminal */}
          <div className="g-card" style={{ display: 'flex', flexDirection: 'column', height: 320 }}>
            <div style={{ display: 'flex', alignItems: 'center', justifyContent: 'space-between', marginBottom: 12 }}>
              <div style={{ display: 'flex', alignItems: 'center', gap: 6 }}>
                <FiTerminal style={{ color: 'var(--color-brand)', fontSize: 16 }} />
                <span style={{ fontSize: 13, fontWeight: 600, color: 'var(--color-brand-heading)' }}>Live Core Logs</span>
              </div>
              <input
                type="text"
                placeholder="Filter logs..."
                value={logsQuery}
                onChange={(e) => setLogsQuery(e.target.value)}
                style={{
                  width: 120,
                  padding: '4px 8px',
                  borderRadius: 4,
                  border: '1px solid var(--color-brand-border)',
                  background: 'var(--color-brand-card)',
                  fontSize: 11,
                  color: 'var(--color-brand-heading)'
                }}
              />
            </div>
            
            <div style={{
              flex: 1,
              background: '#1a1a2e',
              borderRadius: 8,
              padding: 10,
              fontFamily: 'Fira Code, monospace',
              fontSize: 10,
              color: '#a9b1d6',
              overflowY: 'auto',
              display: 'flex',
              flexDirection: 'column',
              gap: 4
            }}>
              {logs.length === 0 ? (
                <div style={{ color: '#565f89', textAlign: 'center', marginTop: 80 }}>Listening for core logs...</div>
              ) : (
                logs.map((log, idx) => (
                  <div key={idx} style={{ wordBreak: 'break-all', whiteSpace: 'pre-wrap' }}>{log}</div>
                ))
              )}
              <div ref={logsEndRef} />
            </div>
          </div>

          {/* Diagnostic utilities: Port scanner */}
          <div className="g-card">
            <div style={{ display: 'flex', alignItems: 'center', gap: 8, marginBottom: 14 }}>
              <FiSearch style={{ color: 'var(--color-brand)', fontSize: 16 }} />
              <span style={{ fontSize: 13, fontWeight: 600, color: 'var(--color-brand-heading)' }}>Port Scanning Utility</span>
              <FiHelpCircle 
                style={{ cursor: 'pointer', color: 'var(--color-brand-muted)' }} 
                onClick={() => showHelp('Port Scanning Utility', 'Tests connectivity to remote targets. Input IP addresses and comma-separated ports to perform concurrent port opening scans.')}
              />
            </div>

            <div style={{ display: 'flex', flexDirection: 'column', gap: 12 }}>
              <div style={{ display: 'grid', gridTemplateColumns: '1fr 90px', gap: 10 }}>
                <input
                  type="text"
                  placeholder="Target IP / Hostname"
                  value={probeIP}
                  onChange={(e) => setProbeIP(e.target.value)}
                  style={{
                    padding: '8px 10px',
                    borderRadius: 6,
                    border: '1px solid var(--color-brand-border)',
                    background: 'var(--color-brand-card)',
                    fontSize: 12,
                    color: 'var(--color-brand-heading)'
                  }}
                />
                <select
                  value={probeProto}
                  onChange={(e) => setProbeProto(e.target.value)}
                  style={{
                    padding: '8px 10px',
                    borderRadius: 6,
                    border: '1px solid var(--color-brand-border)',
                    background: 'var(--color-brand-card)',
                    fontSize: 12,
                    color: 'var(--color-brand-heading)'
                  }}
                >
                  <option value="tcp">TCP</option>
                  <option value="udp">UDP</option>
                </select>
              </div>

              <div style={{ display: 'flex', gap: 10 }}>
                <input
                  type="text"
                  placeholder="Ports (e.g. 80,443,53)"
                  value={probePorts}
                  onChange={(e) => setProbePorts(e.target.value)}
                  style={{
                    flex: 1,
                    padding: '8px 10px',
                    borderRadius: 6,
                    border: '1px solid var(--color-brand-border)',
                    background: 'var(--color-brand-card)',
                    fontSize: 12,
                    color: 'var(--color-brand-heading)'
                  }}
                />
                <button className="btn btn--primary btn--sm" onClick={handleProbePorts} disabled={isLoading}>Scan</button>
              </div>

              {probeResults.length > 0 && (
                <div style={{ 
                  marginTop: 6, 
                  background: 'var(--color-brand-bg)', 
                  padding: 8, 
                  borderRadius: 6, 
                  maxHeight: 120, 
                  overflowY: 'auto',
                  border: '1px solid var(--color-brand-border)'
                }}>
                  {probeResults.map((r, idx) => (
                    <div key={idx} style={{ display: 'flex', justifyContent: 'space-between', fontSize: 11, marginBottom: 4 }}>
                      <span style={{ color: 'var(--color-brand-heading)', fontFamily: 'Fira Code' }}>Port {r.port} ({r.protocol.toUpperCase()})</span>
                      <span style={{ fontWeight: 700, color: r.open ? 'var(--color-brand-green)' : 'var(--color-brand-red)' }}>
                        {r.open ? `OPEN (${r.latency_ms}ms)` : 'CLOSED'}
                      </span>
                    </div>
                  ))}
                </div>
              )}
            </div>
          </div>

          {/* Local Service Discovery */}
          <div className="g-card">
            <div style={{ display: 'flex', alignItems: 'center', justifyContent: 'space-between', marginBottom: 14 }}>
              <div style={{ display: 'flex', alignItems: 'center', gap: 6 }}>
                <FiWifi style={{ color: 'var(--color-brand)', fontSize: 16 }} />
                <span style={{ fontSize: 13, fontWeight: 600, color: 'var(--color-brand-heading)' }}>Subnet Service Discovery</span>
                <FiHelpCircle 
                  style={{ cursor: 'pointer', color: 'var(--color-brand-muted)' }} 
                  onClick={() => showHelp('Subnet Service Discovery', 'Sweeps the local IPv4 subnet (using TCP port 80/22 checks) to auto-discover active network devices.')}
                />
              </div>
              <button className="btn btn--sm btn--secondary" onClick={handleDiscoverDevices} disabled={isDiscovering}>
                {isDiscovering ? 'Scanning...' : 'Scan Subnet'}
              </button>
            </div>

            {discoveredDevices.length > 0 && (
              <div style={{ 
                maxHeight: 150, 
                overflowY: 'auto', 
                background: 'var(--color-brand-bg)', 
                borderRadius: 6, 
                padding: 8,
                border: '1px solid var(--color-brand-border)'
              }}>
                {discoveredDevices.map((d, idx) => (
                  <div key={idx} style={{ display: 'flex', justifyContent: 'space-between', fontSize: 11, marginBottom: 4 }}>
                    <div>
                      <span style={{ color: 'var(--color-brand-heading)', fontWeight: 600, fontFamily: 'Fira Code' }}>{d.ip}</span>
                      {d.hostname && <span style={{ color: 'var(--color-brand-muted)', marginLeft: 6 }}>({d.hostname})</span>}
                    </div>
                    <span style={{ color: 'var(--color-brand-green)', fontWeight: 700 }}>ACTIVE</span>
                  </div>
                ))}
              </div>
            )}
          </div>

          {/* Wake on LAN */}
          <div className="g-card">
            <div style={{ display: 'flex', alignItems: 'center', gap: 6, marginBottom: 14 }}>
              <FiMonitor style={{ color: 'var(--color-brand)', fontSize: 16 }} />
              <span style={{ fontSize: 13, fontWeight: 600, color: 'var(--color-brand-heading)' }}>Wake-on-LAN (WOL) Client</span>
              <FiHelpCircle 
                style={{ cursor: 'pointer', color: 'var(--color-brand-muted)' }} 
                onClick={() => showHelp('Wake-on-LAN (WOL)', 'Sends a UDP magic packet (0xFF repeat + MAC address repeat) to boot remote network hardware from sleep state.')}
              />
            </div>

            <div style={{ display: 'flex', flexDirection: 'column', gap: 10 }}>
              <input
                type="text"
                placeholder="Target MAC Address (e.g. AA:BB:CC:DD:EE:FF)"
                value={wolMac}
                onChange={(e) => setWolMac(e.target.value)}
                style={{
                  padding: '8px 10px',
                  borderRadius: 6,
                  border: '1px solid var(--color-brand-border)',
                  background: 'var(--color-brand-card)',
                  fontSize: 12,
                  color: 'var(--color-brand-heading)'
                }}
              />
              <div style={{ display: 'flex', gap: 10 }}>
                <input
                  type="text"
                  placeholder="Broadcast IP (default 255.255.255.255)"
                  value={wolBcast}
                  onChange={(e) => setWolBcast(e.target.value)}
                  style={{
                    flex: 1,
                    padding: '8px 10px',
                    borderRadius: 6,
                    border: '1px solid var(--color-brand-border)',
                    background: 'var(--color-brand-card)',
                    fontSize: 12,
                    color: 'var(--color-brand-heading)'
                  }}
                />
                <button className="btn btn--primary btn--sm" onClick={handleSendWol} disabled={isLoading}>Wake</button>
              </div>
            </div>
          </div>

          {/* Local Interception Debug Proxy */}
          <div className="g-card">
            <div style={{ display: 'flex', alignItems: 'center', justifyContent: 'space-between', marginBottom: 12 }}>
              <div style={{ display: 'flex', alignItems: 'center', gap: 6 }}>
                <FiZap style={{ color: 'var(--color-brand)', fontSize: 16 }} />
                <span style={{ fontSize: 13, fontWeight: 600, color: 'var(--color-brand-heading)' }}>Local Interception Proxy</span>
                <FiHelpCircle 
                  style={{ cursor: 'pointer', color: 'var(--color-brand-muted)' }} 
                  onClick={() => showHelp('Local Interception Proxy', 'Fires up a local HTTP/HTTPS CONNECT tunnel proxy. Captures outgoing application connections and logs transaction methods/hosts for diagnostic auditing.')}
                />
              </div>
              <span style={{
                fontSize: 10,
                fontWeight: 700,
                color: isDebugProxyActive ? 'var(--color-brand-green)' : 'var(--color-brand-red)'
              }}>{isDebugProxyActive ? 'RUNNING' : 'INACTIVE'}</span>
            </div>

            <div style={{ display: 'flex', flexDirection: 'column', gap: 12 }}>
              <div style={{ display: 'flex', gap: 10 }}>
                <input
                  type="number"
                  placeholder="Proxy Port (e.g. 8080)"
                  value={debugProxyPort}
                  onChange={(e) => setDebugProxyPort(Number(e.target.value))}
                  disabled={isDebugProxyActive}
                  style={{
                    flex: 1,
                    padding: '8px 10px',
                    borderRadius: 6,
                    border: '1px solid var(--color-brand-border)',
                    background: 'var(--color-brand-card)',
                    fontSize: 12,
                    color: 'var(--color-brand-heading)'
                  }}
                />
                <button 
                  className="btn btn--sm"
                  onClick={handleToggleDebugProxy}
                  style={{
                    background: isDebugProxyActive ? '#ef4444' : 'var(--color-brand)',
                    color: '#fff',
                    border: 'none',
                    fontWeight: 600
                  }}
                >
                  {isDebugProxyActive ? 'Stop' : 'Start'}
                </button>
              </div>

              {isDebugProxyActive && debugProxyLogs.length > 0 && (
                <div style={{ 
                  maxHeight: 120, 
                  overflowY: 'auto', 
                  background: '#1a1a2e', 
                  borderRadius: 6, 
                  padding: 8,
                  fontFamily: 'Fira Code',
                  fontSize: 9,
                  color: '#a9b1d6'
                }}>
                  {debugProxyLogs.map((l, idx) => (
                    <div key={idx} style={{ marginBottom: 2 }}>{l}</div>
                  ))}
                </div>
              )}
            </div>
          </div>

          {/* System Settings & Shortcuts */}
          <div className="g-card">
            <div style={{ display: 'flex', alignItems: 'center', gap: 6, marginBottom: 14 }}>
              <FiSettings style={{ color: 'var(--color-brand)', fontSize: 16 }} />
              <span style={{ fontSize: 13, fontWeight: 600, color: 'var(--color-brand-heading)' }}>System Tray & Keybindings</span>
              <FiHelpCircle 
                style={{ cursor: 'pointer', color: 'var(--color-brand-muted)' }} 
                onClick={() => showHelp('System Tray & Hotkeys', 'Configure background daemon tray status flags and bind OS-level keyboard shortcuts (e.g. Ctrl+Shift+X) to quickly toggle SOCKS5 proxy status.')}
              />
            </div>

            <div style={{ display: 'flex', flexDirection: 'column', gap: 12 }}>
              <div style={{ display: 'flex', alignItems: 'center', justifyContent: 'space-between' }}>
                <span style={{ fontSize: 12, color: 'var(--color-brand-heading)' }}>Enable System Tray Icon</span>
                <input
                  type="checkbox"
                  checked={systemTrayEnabled}
                  onChange={(e) => setSystemTrayEnabled(e.target.checked)}
                  style={{ width: 16, height: 16, cursor: 'pointer', accentColor: 'var(--color-brand)' }}
                />
              </div>

              <div style={{ display: 'flex', gap: 10 }}>
                <input
                  type="text"
                  placeholder="Toggle Hotkey (e.g. Ctrl+Shift+X)"
                  value={hotkeys}
                  onChange={(e) => setHotkeys(e.target.value)}
                  style={{
                    flex: 1,
                    padding: '8px 10px',
                    borderRadius: 6,
                    border: '1px solid var(--color-brand-border)',
                    background: 'var(--color-brand-card)',
                    fontSize: 12,
                    color: 'var(--color-brand-heading)'
                  }}
                />
                <button className="btn btn--sm btn--secondary" onClick={() => setMessage({ type: 'success', text: 'Hotkey overrides saved!' })}>Save</button>
              </div>
            </div>
          </div>

        </div>

      </div>

      {/* Help Modal Popup Dialog */}
      {helpTitle && (
        <div style={{
          position: 'fixed',
          top: 0,
          left: 0,
          width: '100%',
          height: '100%',
          background: 'rgba(0,0,0,0.5)',
          display: 'flex',
          alignItems: 'center',
          justifyContent: 'center',
          zIndex: 9999
        }}>
          <div style={{
            background: 'var(--color-brand-card)',
            padding: 24,
            borderRadius: 12,
            width: 440,
            maxWidth: '90%',
            boxShadow: '0 10px 25px rgba(0,0,0,0.1)'
          }}>
            <p style={{ margin: 0, fontSize: 13, color: 'var(--color-brand-text)', lineHeight: 1.5 }}>{helpText}</p>
          </div>
        </div>
      )}

      {/* Clipboard Mass Import Modal */}
      {isClipboardModalOpen && (
        <div style={{
          position: 'fixed',
          top: 0, left: 0, width: '100%', height: '100%',
          background: 'rgba(0,0,0,0.6)',
          display: 'flex', alignItems: 'center', justifyContent: 'center', zIndex: 9999
        }}>
          <div style={{
            background: 'var(--color-brand-card)',
            padding: 24, borderRadius: 16, width: 900, maxWidth: '95%', maxHeight: '90vh',
            boxShadow: '0 20px 40px rgba(0,0,0,0.25)',
            display: 'flex', flexDirection: 'column', gap: 16, overflow: 'hidden'
          }}>
            {/* Header */}
            <div style={{ display: 'flex', justifyContent: 'space-between', alignItems: 'center', borderBottom: '1px solid var(--color-brand-border)', paddingBottom: 12 }}>
              <div>
                <h3 style={{ margin: 0, fontSize: 16, fontWeight: 700, color: 'var(--color-brand-heading)' }}>Clipboard Mass Config Importer</h3>
                <span style={{ fontSize: 11, color: 'var(--color-brand-text)' }}>Highly optimized node parser designed for extreme config list scale.</span>
              </div>
              <button 
                onClick={() => setIsClipboardModalOpen(false)}
                style={{ background: 'none', border: 'none', cursor: 'pointer', color: 'var(--color-brand-muted)', display: 'flex', alignItems: 'center' }}
                disabled={isImportingBulk}
              >
                <FiX size={20} />
              </button>
            </div>

            {/* Paste Drop Zone / Progress Bar / List view */}
            {isParsing ? (
              <div style={{ padding: '60px 20px', textAlign: 'center', display: 'flex', flexDirection: 'column', alignItems: 'center', gap: 12 }}>
                <FiRefreshCw className="spin-animation" size={36} style={{ color: 'var(--color-brand)' }} />
                <div style={{ fontSize: 14, fontWeight: 600, color: 'var(--color-brand-heading)' }}>Parsing Payload... {parseProgress}%</div>
                <div style={{ width: '80%', maxWidth: 400, background: 'var(--color-brand-border)', height: 8, borderRadius: 4, overflow: 'hidden' }}>
                  <div style={{ width: `${parseProgress}%`, background: 'var(--color-brand)', height: '100%', transition: 'width 0.1s linear' }} />
                </div>
              </div>
            ) : clipboardCount === 0 ? (
              <div 
                style={{
                  border: '2px dashed var(--color-brand-border)',
                  borderRadius: 12,
                  padding: 60,
                  textAlign: 'center',
                  background: 'var(--color-brand-bg)',
                  position: 'relative'
                }}
              >
                <FiPlus size={40} style={{ color: 'var(--color-brand)', marginBottom: 12 }} />
                <p style={{ margin: 0, fontSize: 14, fontWeight: 600, color: 'var(--color-brand-heading)' }}>
                  Click inside this box and press Ctrl + V to paste configs
                </p>
                <p style={{ margin: '4px 0 0', fontSize: 11, color: 'var(--color-brand-text)' }}>
                  Accepts base64 subscription blobs, multi-line URIs, or JSON configuration lists.
                </p>
                <textarea
                  autoFocus
                  value=""
                  onChange={() => {}}
                  onPaste={(e) => {
                    const text = e.clipboardData.getData('text');
                    processPastedTextChunked(text);
                  }}
                  style={{
                    position: 'absolute',
                    top: 0, left: 0, width: '100%', height: '100%',
                    opacity: 0, cursor: 'pointer'
                  }}
                />
              </div>
            ) : (
              // Parsed view
              <div style={{ display: 'flex', flexDirection: 'column', gap: 12, overflow: 'hidden', flex: 1 }}>
                
                {/* Search & Bulk selection tools */}
                <div style={{ display: 'flex', gap: 12, alignItems: 'center' }}>
                  <input
                    type="text"
                    placeholder="Search parsed configs..."
                    value={clipboardSearch}
                    onChange={(e) => {
                      setClipboardSearch(e.target.value);
                      setClipboardPage(0);
                    }}
                    style={{
                      flex: 1,
                      padding: '8px 12px',
                      borderRadius: 8,
                      border: '1px solid var(--color-brand-border)',
                      background: 'var(--color-brand-card)',
                      fontSize: 12,
                      color: 'var(--color-brand-heading)'
                    }}
                  />
                  <div style={{ display: 'flex', gap: 6 }}>
                    <button 
                      type="button" 
                      className="btn btn--xs btn--secondary"
                      onClick={() => {
                        const filtered = parsedConfigsRef.current.filter(c => {
                          const query = clipboardSearch.toLowerCase();
                          return c.name.toLowerCase().includes(query) || c.host.toLowerCase().includes(query) || c.protocol.includes(query);
                        });
                        filtered.forEach(c => {
                          const idx = parsedConfigsRef.current.indexOf(c);
                          if (idx !== -1) deselectedSetRef.current.delete(idx);
                        });
                        setClipboardUpdateTrigger(prev => prev + 1);
                      }}
                    >
                      Select Search Results
                    </button>
                    <button 
                      type="button" 
                      className="btn btn--xs btn--secondary"
                      onClick={() => {
                        const filtered = parsedConfigsRef.current.filter(c => {
                          const query = clipboardSearch.toLowerCase();
                          return c.name.toLowerCase().includes(query) || c.host.toLowerCase().includes(query) || c.protocol.includes(query);
                        });
                        filtered.forEach(c => {
                          const idx = parsedConfigsRef.current.indexOf(c);
                          if (idx !== -1) deselectedSetRef.current.add(idx);
                        });
                        setClipboardUpdateTrigger(prev => prev + 1);
                      }}
                    >
                      Deselect Search Results
                    </button>
                    <button 
                      type="button" 
                      className="btn btn--xs btn--secondary"
                      onClick={() => {
                        deselectedSetRef.current.clear();
                        setClipboardUpdateTrigger(prev => prev + 1);
                      }}
                    >
                      Select All
                    </button>
                    <button 
                      type="button" 
                      className="btn btn--xs btn--secondary"
                      onClick={() => {
                        parsedConfigsRef.current.forEach((_, idx) => deselectedSetRef.current.add(idx));
                        setClipboardUpdateTrigger(prev => prev + 1);
                      }}
                    >
                      Deselect All
                    </button>
                  </div>
                </div>

                {/* Status bar */}
                <div style={{ fontSize: 11, color: 'var(--color-brand-text)', display: 'flex', justifyContent: 'space-between' }}>
                  <span>Total Parsed: <strong>{clipboardCount}</strong> configs</span>
                  <span>
                    Selected: <strong>{clipboardCount - deselectedSetRef.current.size}</strong> / {clipboardCount}
                  </span>
                </div>

                {/* Config Table with Virtual Viewport slicing */}
                <div style={{ flex: 1, overflowY: 'auto', border: '1px solid var(--color-brand-border)', borderRadius: 8 }}>
                  <table style={{ width: '100%', borderCollapse: 'collapse', fontSize: 11, textAlign: 'left' }}>
                    <thead>
                      <tr style={{ background: 'var(--color-brand-bg)', borderBottom: '1px solid var(--color-brand-border)', position: 'sticky', top: 0, zIndex: 10 }}>
                        <th style={{ padding: '8px 12px', color: 'var(--color-brand-heading)', width: 40 }}>Sel</th>
                        <th style={{ padding: '8px 12px', color: 'var(--color-brand-heading)', width: 180 }}>Name</th>
                        <th style={{ padding: '8px 12px', color: 'var(--color-brand-heading)', width: 70 }}>Protocol</th>
                        <th style={{ padding: '8px 12px', color: 'var(--color-brand-heading)' }}>Address</th>
                        <th style={{ padding: '8px 12px', color: 'var(--color-brand-heading)', width: 60 }}>Port</th>
                      </tr>
                    </thead>
                    <tbody>
                      {(() => {
                        const filtered = parsedConfigsRef.current.filter(c => {
                          if (!clipboardSearch) return true;
                          const query = clipboardSearch.toLowerCase();
                          return c.name.toLowerCase().includes(query) || c.host.toLowerCase().includes(query) || c.protocol.includes(query);
                        });
                        const PAGE_SIZE = 100;
                        const slice = filtered.slice(clipboardPage * PAGE_SIZE, (clipboardPage + 1) * PAGE_SIZE);
                        
                        if (slice.length === 0) {
                          return (
                            <tr>
                              <td colSpan={5} style={{ padding: 20, textAlign: 'center', color: 'var(--color-brand-muted)' }}>No configurations match search filters.</td>
                            </tr>
                          );
                        }

                        return slice.map((c, rowIdx) => {
                          const origIdx = parsedConfigsRef.current.indexOf(c);
                          const isSelected = !deselectedSetRef.current.has(origIdx);
                          
                          return (
                            <tr key={origIdx} style={{ borderBottom: '1px solid var(--color-brand-border)', background: isSelected ? 'var(--color-brand-light)' : 'none' }}>
                              <td style={{ padding: '8px 12px' }}>
                                <input
                                  type="checkbox"
                                  checked={isSelected}
                                  onChange={() => {
                                    if (deselectedSetRef.current.has(origIdx)) {
                                      deselectedSetRef.current.delete(origIdx);
                                    } else {
                                      deselectedSetRef.current.add(origIdx);
                                    }
                                    setClipboardUpdateTrigger(prev => prev + 1);
                                  }}
                                  style={{ cursor: 'pointer', accentColor: 'var(--color-brand)' }}
                                />
                              </td>
                              <td style={{ padding: '8px 12px', fontWeight: 600, color: 'var(--color-brand-heading)', whiteSpace: 'nowrap', overflow: 'hidden', textOverflow: 'ellipsis', maxWidth: 180 }}>{c.name}</td>
                              <td style={{ padding: '8px 12px', textTransform: 'uppercase', color: 'var(--color-brand)' }}>{c.protocol}</td>
                              <td style={{ padding: '8px 12px', fontFamily: 'monospace' }}>{c.host}</td>
                              <td style={{ padding: '8px 12px' }}>{c.port}</td>
                            </tr>
                          );
                        });
                      })()}
                    </tbody>
                  </table>
                </div>

                {/* Footer Controls */}
                <div style={{ display: 'flex', justifyContent: 'space-between', alignItems: 'center', borderTop: '1px solid var(--color-brand-border)', paddingTop: 12 }}>
                  {/* Pagination */}
                  <div style={{ display: 'flex', gap: 6, alignItems: 'center' }}>
                    <button
                      type="button"
                      className="btn btn--xs btn--secondary"
                      disabled={clipboardPage === 0}
                      onClick={() => setClipboardPage(prev => prev - 1)}
                    >
                      Prev
                    </button>
                    <span style={{ fontSize: 11, color: 'var(--color-brand-text)' }}>
                      Page <strong>{clipboardPage + 1}</strong> of <strong>{Math.ceil(parsedConfigsRef.current.filter(c => {
                        if (!clipboardSearch) return true;
                        const query = clipboardSearch.toLowerCase();
                        return c.name.toLowerCase().includes(query) || c.host.toLowerCase().includes(query) || c.protocol.includes(query);
                      }).length / 100) || 1}</strong>
                    </span>
                    <button
                      type="button"
                      className="btn btn--xs btn--secondary"
                      disabled={(clipboardPage + 1) * 100 >= parsedConfigsRef.current.filter(c => {
                        if (!clipboardSearch) return true;
                        const query = clipboardSearch.toLowerCase();
                        return c.name.toLowerCase().includes(query) || c.host.toLowerCase().includes(query) || c.protocol.includes(query);
                      }).length}
                      onClick={() => setClipboardPage(prev => prev + 1)}
                    >
                      Next
                    </button>
                  </div>

                  {/* Actions */}
                  <div style={{ display: 'flex', gap: 10 }}>
                    <button
                      type="button"
                      className="btn btn--sm btn--secondary"
                      onClick={() => {
                        parsedConfigsRef.current = [];
                        deselectedSetRef.current = new Set();
                        setClipboardCount(0);
                        setClipboardPage(0);
                      }}
                      disabled={isImportingBulk}
                    >
                      Reset / Clear
                    </button>
                    <button
                      type="button"
                      className="btn btn--sm btn--primary"
                      onClick={handleImportBulk}
                      disabled={isImportingBulk || (clipboardCount - deselectedSetRef.current.size === 0)}
                      style={{ background: 'var(--color-brand-green)', borderColor: 'var(--color-brand-green)' }}
                    >
                      {isImportingBulk ? 'Importing...' : `Import Selected (${clipboardCount - deselectedSetRef.current.size})`}
                    </button>
                  </div>
                </div>

              </div>
            )}
          </div>
        </div>
      )}

    </div>
  );
};
