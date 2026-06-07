import React, { useState, useEffect, useRef, lazy, Suspense } from 'react';
import { FiRefreshCw } from 'react-icons/fi';

// Skeleton fallbacks
import {
  CardSkeleton,
  LogsTerminalSkeleton,
  SubscriptionsSkeleton,
  ConfigSettingsSkeleton,
} from './v2ray-client/components/Skeletons';

// Lazy-loaded components
const HelpModal = lazy(() => import('./v2ray-client/components/HelpModal'));
const ClipboardModal = lazy(() => import('./v2ray-client/components/ClipboardModal'));
const EngineStatusCard = lazy(() => import('./v2ray-client/components/EngineStatusCard'));
const SubscriptionsCard = lazy(() => import('./v2ray-client/components/SubscriptionsCard'));
const CdnScannerCard = lazy(() => import('./v2ray-client/components/CdnScannerCard'));
const ConfigSettingsCard = lazy(() => import('./v2ray-client/components/ConfigSettingsCard'));
const LogsTerminalCard = lazy(() => import('./v2ray-client/components/LogsTerminalCard'));
const PortScannerCard = lazy(() => import('./v2ray-client/components/PortScannerCard'));
const DeviceDiscoveryCard = lazy(() => import('./v2ray-client/components/DeviceDiscoveryCard'));
const WakeOnLanCard = lazy(() => import('./v2ray-client/components/WakeOnLanCard'));
const DebugProxyCard = lazy(() => import('./v2ray-client/components/DebugProxyCard'));
const SystemSettingsCard = lazy(() => import('./v2ray-client/components/SystemSettingsCard'));

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
  const [message, setMessage] = useState<{ type: 'success' | 'error'; text: string } | null>(null);

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

  const logsContainerRef = useRef<HTMLDivElement | null>(null);

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
        headers: { Authorization: `Bearer ${token}` },
      });
      if (sResp.ok) {
        const data = await sResp.json();
        setSelectedCore(data.v2ray_core || 'xray');
        setFragmentMode(data.fragment_mode || 'default');
        setFragmentPackets(data.fragment_packets || 'tlshello');
        setFragmentLength(data.fragment_length || '100-200');
        setFragmentInterval(data.fragment_interval || '10-20');
        setSocksPort(data.socks_port ? Number(data.socks_port) : 10808);
        setHttpPort(data.http_port ? Number(data.http_port) : 10809);
        setMuxEnabled(data.mux_enabled === 'true');
        setDnsServer(data.dns_server || '8.8.8.8');
        setRoutingPreset(data.routing_preset || 'bypass_domestic');
        setCustomRouting(data.custom_routing || '');
        setEvasionFingerprint(data.evasion_fingerprint || 'chrome');
        setEvasionFragment(data.evasion_fragment === 'true');
        setEvasionEch(data.evasion_ech === 'true');
        setEvasionEchConfig(data.evasion_ech_config || '');
        setEvasionTcpBrutal(data.evasion_tcp_brutal === 'true');
      }

      // Fetch core status
      const stResp = await fetch('/api/v2ray/client/status', {
        headers: { Authorization: `Bearer ${token}` },
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
        headers: { Authorization: `Bearer ${token}` },
      });
      if (pResp.ok) {
        const pList = await pResp.json();
        const data = pList.data || [];
        setTotalProfiles(pList.total || 0);

        if (reset) {
          setProfiles(data);
          setPageOffset(offset);
        } else {
          setProfiles((prev) => [...prev, ...data]);
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
        headers: { Authorization: `Bearer ${token}` },
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
    if (logsContainerRef.current) {
      logsContainerRef.current.scrollTop = logsContainerRef.current.scrollHeight;
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
          Authorization: `Bearer ${token}`,
        },
        body: JSON.stringify({
          v2ray_core: String(selectedCore),
          fragment_mode: String(fragmentMode),
          fragment_packets: String(fragmentPackets),
          fragment_length: String(fragmentLength),
          fragment_interval: String(fragmentInterval),
          socks_port: String(socksPort),
          http_port: String(httpPort),
          mux_enabled: String(muxEnabled),
          dns_server: String(dnsServer),
          routing_preset: String(routingPreset),
          custom_routing: String(customRouting),
          evasion_fingerprint: String(evasionFingerprint),
          evasion_fragment: String(evasionFragment),
          evasion_ech: String(evasionEch),
          evasion_ech_config: String(evasionEchConfig),
          evasion_tcp_brutal: String(evasionTcpBrutal),
        }),
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
        headers: { Authorization: `Bearer ${token}` },
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
        headers: { Authorization: `Bearer ${token}` },
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
          Authorization: `Bearer ${token}`,
        },
        body: JSON.stringify({ url: subUrl }),
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
          Authorization: `Bearer ${token}`,
        },
        body: JSON.stringify({ uri: manualUri }),
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
              port,
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
            Authorization: `Bearer ${token}`,
          },
          body: JSON.stringify({ uris: batch }),
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
    if (
      p.protocol === 'vless' ||
      p.protocol === 'vmess' ||
      p.protocol === 'trojan' ||
      p.protocol === 'shadowsocks' ||
      p.protocol === 'ss'
    ) {
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
        headers: { Authorization: `Bearer ${token}` },
        body: formData,
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
          Authorization: `Bearer ${token}`,
        },
        body: JSON.stringify({ ids: selectedProfileIds }),
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
    const targetProfile = profiles.find((p) => p.ID === activeProfileId) || profiles[0];
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
          Authorization: `Bearer ${token}`,
        },
        body: JSON.stringify({
          uri: link,
          ranges: cdnRanges
            .split('\n')
            .map((r) => r.trim())
            .filter(Boolean),
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
          base_port: 20000,
        }),
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
        headers: { Authorization: `Bearer ${token}` },
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
        headers: { Authorization: `Bearer ${token}` },
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
          Authorization: `Bearer ${token}`,
        },
        body: JSON.stringify({ size_bytes: 10000000 }),
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
        headers: { Authorization: `Bearer ${token}` },
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
        headers: { Authorization: `Bearer ${token}` },
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
    if (
      !window.confirm('Are you sure you want to delete ALL outbound configurations? This action cannot be undone!')
    )
      return;
    setIsLoading(true);
    try {
      const token = localStorage.getItem('cc_client_token') || '';
      const res = await fetch('/api/v2ray/client/configs/all', {
        method: 'DELETE',
        headers: { Authorization: `Bearer ${token}` },
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
        headers: { Authorization: `Bearer ${token}` },
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
      const portsArr = probePorts
        .split(',')
        .map((p) => Number(p.trim()))
        .filter((p) => !isNaN(p));
      const res = await fetch('/api/v2ray/client/probe-ports', {
        method: 'POST',
        headers: {
          'Content-Type': 'application/json',
          Authorization: `Bearer ${token}`,
        },
        body: JSON.stringify({
          ip: probeIP,
          ports: portsArr,
          protocol: probeProto,
        }),
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
          Authorization: `Bearer ${token}`,
        },
        body: JSON.stringify({
          mac: wolMac,
          broadcast_ip: wolBcast,
        }),
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
        headers: { Authorization: `Bearer ${token}` },
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
      const body = isDebugProxyActive ? undefined : JSON.stringify({ port: Number(debugProxyPort) });

      const res = await fetch(endpoint, {
        method: 'POST',
        headers: {
          'Content-Type': 'application/json',
          Authorization: `Bearer ${token}`,
        },
        body: body,
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
        headers: { Authorization: `Bearer ${token}` },
      });
      if (res.ok) {
        const data = await res.json();
        setDebugProxyLogs(data || []);
      }
    } catch (err) {
      console.error(err);
    }
  };

  return (
    <div>
      {/* Title */}
      <div style={{ display: 'flex', justifyContent: 'space-between', alignItems: 'center', marginBottom: 24 }}>
        <div>
          <h1 style={{ fontSize: 22, fontWeight: 700, color: 'var(--color-brand-heading)', margin: 0 }}>
            V2Ray / Xray manager
          </h1>
          <p style={{ fontSize: 12, color: 'var(--color-brand-text)', margin: '4px 0 0' }}>
            Zero-TUN Censorship Evasion panel supporting VLESS, VMess, Reality, and dynamic routing settings.
          </p>
        </div>
        <button className="btn btn--sm" onClick={loadSettings} disabled={isLoading}>
          <FiRefreshCw className={isLoading ? 'spin-animation' : ''} style={{ marginRight: 6 }} /> Refresh State
        </button>
      </div>

      {message && (
        <div
          style={{
            padding: '12px 16px',
            borderRadius: 10,
            marginBottom: 20,
            fontSize: 13,
            fontWeight: 500,
            background: message.type === 'success' ? 'var(--color-brand-light)' : '#fee2e2',
            border: message.type === 'success' ? '1px solid var(--color-brand-border)' : '1px solid #fca5a5',
            color: message.type === 'success' ? 'var(--color-brand)' : '#b91c1c',
          }}
        >
          {message.text}
        </div>
      )}

      {/* Grid Layout */}
      <div style={{ display: 'grid', gridTemplateColumns: '1fr 360px', gap: 24, alignItems: 'start' }}>
        {/* Left Side: Outbounds, Configurations & Evasion */}
        <div style={{ display: 'flex', flexDirection: 'column', gap: 24 }}>
          {/* Active Engine controls */}
          <Suspense fallback={<CardSkeleton height={100} title="Core Supervisor" />}>
            <EngineStatusCard
              isRunning={isRunning}
              isLoading={isLoading}
              socksPort={socksPort}
              httpPort={httpPort}
              speedTestActive={speedTestActive}
              speedTestBreakdown={speedTestBreakdown}
              handleRunSpeedTest={handleRunSpeedTest}
              handleStartCore={handleStartCore}
              handleStopCore={handleStopCore}
              showHelp={showHelp}
            />
          </Suspense>

          {/* Subscriptions & Profiles */}
          <Suspense fallback={<SubscriptionsSkeleton />}>
            <SubscriptionsCard
              isLoading={isLoading}
              subUrl={subUrl}
              setSubUrl={setSubUrl}
              manualUri={manualUri}
              setManualUri={setManualUri}
              profiles={profiles}
              totalProfiles={totalProfiles}
              activeProfileId={activeProfileId}
              selectedProfileIds={selectedProfileIds}
              setSelectedProfileIds={setSelectedProfileIds}
              handleTestLatency={handleTestLatency}
              handleExportPDF={handleExportPDF}
              handleImportSub={handleImportSub}
              handleManualImport={handleManualImport}
              handleDeleteAllNodes={handleDeleteAllNodes}
              handleQRImport={handleQRImport}
              qrFileInputRef={qrFileInputRef}
              fetchProfiles={fetchProfiles}
              pageOffset={pageOffset}
              handleSelectProfile={handleSelectProfile}
              handleDeleteProfile={handleDeleteProfile}
              showHelp={showHelp}
              openClipboardModal={() => setIsClipboardModalOpen(true)}
            />
          </Suspense>

          {/* CDN IP Scanner & Optimizer */}
          <Suspense fallback={<CardSkeleton height={250} title="CDN Scanner" />}>
            <CdnScannerCard
              isLoading={isLoading}
              cdnRanges={cdnRanges}
              setCdnRanges={setCdnRanges}
              cdnScannerActive={cdnScannerActive}
              cdnScanStatus={cdnScanStatus}
              handleStartCDNScan={handleStartCDNScan}
              handleStopCDNScan={handleStopCDNScan}
              showHelp={showHelp}
            />
          </Suspense>

          {/* Configuration Form */}
          <Suspense fallback={<ConfigSettingsSkeleton />}>
            <ConfigSettingsCard
              isLoading={isLoading}
              selectedCore={selectedCore}
              setSelectedCore={setSelectedCore}
              socksPort={socksPort}
              setSocksPort={setSocksPort}
              httpPort={httpPort}
              setHttpPort={setHttpPort}
              dnsServer={dnsServer}
              setDnsServer={setDnsServer}
              routingPreset={routingPreset}
              setRoutingPreset={setRoutingPreset}
              customRouting={customRouting}
              setCustomRouting={setCustomRouting}
              evasionFingerprint={evasionFingerprint}
              setEvasionFingerprint={setEvasionFingerprint}
              evasionFragment={evasionFragment}
              setEvasionFragment={setEvasionFragment}
              fragmentMode={fragmentMode}
              setFragmentMode={setFragmentMode}
              fragmentLength={fragmentLength}
              setFragmentLength={setFragmentLength}
              fragmentInterval={fragmentInterval}
              setFragmentInterval={setFragmentInterval}
              evasionEch={evasionEch}
              setEvasionEch={setEvasionEch}
              evasionEchConfig={evasionEchConfig}
              setEvasionEchConfig={setEvasionEchConfig}
              evasionTcpBrutal={evasionTcpBrutal}
              setEvasionTcpBrutal={setEvasionTcpBrutal}
              muxEnabled={muxEnabled}
              setMuxEnabled={setMuxEnabled}
              handleSaveSettings={handleSaveSettings}
              showHelp={showHelp}
            />
          </Suspense>
        </div>

        {/* Right Side: Log terminal, diagnostic probers, wol, and debug proxy */}
        <div style={{ display: 'flex', flexDirection: 'column', gap: 24 }}>
          {/* Active Logs Terminal */}
          <Suspense fallback={<LogsTerminalSkeleton />}>
            <LogsTerminalCard
              logs={logs}
              logsQuery={logsQuery}
              setLogsQuery={setLogsQuery}
              logsContainerRef={logsContainerRef}
            />
          </Suspense>

          {/* Diagnostic utilities: Port scanner */}
          <Suspense fallback={<CardSkeleton height={150} title="Port Scanner" />}>
            <PortScannerCard
              isLoading={isLoading}
              probeIP={probeIP}
              setProbeIP={setProbeIP}
              probePorts={probePorts}
              setProbePorts={setProbePorts}
              probeProto={probeProto}
              setProbeProto={setProbeProto}
              probeResults={probeResults}
              handleProbePorts={handleProbePorts}
              showHelp={showHelp}
            />
          </Suspense>

          {/* Local Service Discovery */}
          <Suspense fallback={<CardSkeleton height={120} title="Subnet Discovery" />}>
            <DeviceDiscoveryCard
              isDiscovering={isDiscovering}
              discoveredDevices={discoveredDevices}
              handleDiscoverDevices={handleDiscoverDevices}
              showHelp={showHelp}
            />
          </Suspense>

          {/* Wake on LAN */}
          <Suspense fallback={<CardSkeleton height={120} title="Wake-on-LAN" />}>
            <WakeOnLanCard
              isLoading={isLoading}
              wolMac={wolMac}
              setWolMac={setWolMac}
              wolBcast={wolBcast}
              setWolBcast={setWolBcast}
              handleSendWol={handleSendWol}
              showHelp={showHelp}
            />
          </Suspense>

          {/* Local Interception Debug Proxy */}
          <Suspense fallback={<CardSkeleton height={120} title="Interception Proxy" />}>
            <DebugProxyCard
              isLoading={isLoading}
              debugProxyPort={debugProxyPort}
              setDebugProxyPort={setDebugProxyPort}
              isDebugProxyActive={isDebugProxyActive}
              debugProxyLogs={debugProxyLogs}
              handleToggleDebugProxy={handleToggleDebugProxy}
              showHelp={showHelp}
            />
          </Suspense>

          {/* System Settings & Shortcuts */}
          <Suspense fallback={<CardSkeleton height={120} title="Keybindings" />}>
            <SystemSettingsCard
              hotkeys={hotkeys}
              setHotkeys={setHotkeys}
              systemTrayEnabled={systemTrayEnabled}
              setSystemTrayEnabled={setSystemTrayEnabled}
              setMessage={setMessage}
              showHelp={showHelp}
            />
          </Suspense>
        </div>
      </div>

      {/* Help Modal Popup Dialog */}
      <Suspense fallback={null}>
        <HelpModal title={helpTitle} text={helpText} onClose={() => { setHelpTitle(null); setHelpText(null); }} />
      </Suspense>

      {/* Clipboard Mass Import Modal */}
      <Suspense fallback={null}>
        <ClipboardModal
          isOpen={isClipboardModalOpen}
          onClose={() => setIsClipboardModalOpen(false)}
          isImportingBulk={isImportingBulk}
          isParsing={isParsing}
          parseProgress={parseProgress}
          clipboardCount={clipboardCount}
          clipboardPage={clipboardPage}
          clipboardSearch={clipboardSearch}
          setClipboardPage={setClipboardPage}
          setClipboardSearch={setClipboardSearch}
          processPastedTextChunked={processPastedTextChunked}
          handleImportBulk={handleImportBulk}
          parsedConfigsRef={parsedConfigsRef}
          deselectedSetRef={deselectedSetRef}
          clipboardUpdateTrigger={clipboardUpdateTrigger}
          setClipboardUpdateTrigger={setClipboardUpdateTrigger}
          setClipboardCount={setClipboardCount}
        />
      </Suspense>
    </div>
  );
};
