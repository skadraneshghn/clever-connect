import React, { useState, useEffect, useRef } from 'react';
import { 
  FiSearch, FiGlobe, FiDatabase, FiLayers, FiShield, FiAlertTriangle, 
  FiMapPin, FiCpu, FiExternalLink, FiServer, FiCopy, FiCheck, FiSettings,
  FiPlay, FiSquare, FiList, FiTrendingUp, FiActivity, FiArrowRight
} from 'react-icons/fi';
import { MapContainer, TileLayer, Marker, Popup, useMap } from 'react-leaflet';
import L from 'leaflet';
import 'leaflet/dist/leaflet.css';
import { useLookupStore } from '../store/lookupStore';
import type { IPGeoInfo } from '../store/lookupStore';

// Recenter map helper component
const ChangeMapView: React.FC<{ center: [number, number] }> = ({ center }) => {
  const map = useMap();
  useEffect(() => {
    map.setView(center, 9); // zoom level 9 is a sweet spot
  }, [center, map]);
  return null;
};

// Create a premium custom dot marker icon for Leaflet
const customMarkerIcon = L.divIcon({
  className: 'custom-leaflet-marker',
  html: `<div style="
    background-color: #ff6b2c;
    width: 16px;
    height: 16px;
    border-radius: 50%;
    border: 3px solid #ffffff;
    box-shadow: 0 0 12px rgba(255, 107, 44, 0.7);
    animation: marker-pulse 1.8s infinite ease-in-out;
  "></div>
  <style>
    @keyframes marker-pulse {
      0% { transform: scale(1); box-shadow: 0 0 4px rgba(255,107,44,0.5); }
      50% { transform: scale(1.25); box-shadow: 0 0 16px rgba(255,107,44,0.9); }
      100% { transform: scale(1); box-shadow: 0 0 4px rgba(255,107,44,0.5); }
    }
  </style>`,
  iconSize: [16, 16],
  iconAnchor: [8, 8]
});

interface ComprehensiveProfileCardProps {
  result: any;
}

const ComprehensiveProfileCard: React.FC<ComprehensiveProfileCardProps> = ({ result }) => {
  const [searchTerm, setSearchTerm] = useState('');
  const [activeTab, setActiveTab] = useState('all');

  const rawGeo = result.geo?.raw_json ? JSON.parse(result.geo.raw_json) : {};
  const rawWhois = result.whois?.raw_json ? JSON.parse(result.whois.raw_json) : {};

  // Flatten helper to make nested structures flat for search and display
  const flattenObject = (obj: any, prefix = ''): Record<string, string> => {
    let flattened: Record<string, string> = {};
    if (!obj) return flattened;
    
    Object.entries(obj).forEach(([key, value]) => {
      if (key === 'raw_json' || key === 'error') return;

      const fullKey = prefix ? `${prefix}_${key}` : key;
      if (value === null || value === undefined) return;

      if (typeof value === 'object' && !Array.isArray(value)) {
        Object.assign(flattened, flattenObject(value, fullKey));
      } else {
        flattened[fullKey] = Array.isArray(value) ? value.join(', ') : String(value);
      }
    });

    return flattened;
  };

  const geoFlat = flattenObject(rawGeo);
  const whoisFlat = flattenObject(rawWhois);
  
  // Combine all parameters
  const allParams = { ...geoFlat, ...whoisFlat };

  // Helper to format keys beautifully
  const formatKeyName = (key: string): string => {
    return key
      .split('_')
      .map(word => word.charAt(0).toUpperCase() + word.slice(1))
      .join(' ');
  };

  // Grouping/Categorization rules
  const categories: Record<string, { label: string; icon: string; keys: string[] }> = {
    all: { label: 'All Parameters', icon: '📋', keys: [] },
    geo: { label: 'Geography & Location', icon: '📍', keys: ['country', 'city', 'region', 'state', 'lat', 'lon', 'coord', 'zip', 'postal', 'continent', 'district', 'area', 'elevation'] },
    net: { label: 'Network & ASN', icon: '⚡', keys: ['asn', 'isp', 'org', 'net', 'speed', 'connection', 'carrier', 'mcc', 'mnc', 'mobile', 'ip', 'domain', 'provider', 'nameserver', 'dns'] },
    sec: { label: 'Security & Threats', icon: '🛡️', keys: ['proxy', 'vpn', 'tor', 'bot', 'threat', 'security', 'anonymous', 'crawler', 'scanner', 'usage_type'] },
    time: { label: 'Timezone & Locale', icon: '🕒', keys: ['time', 'date', 'zone', 'current', 'offset', 'dst', 'currency', 'lang', 'symbol', 'locale'] },
    other: { label: 'Metadata & Registry', icon: '⚙️', keys: [] },
  };

  const getCategoryForKey = (key: string): string => {
    const k = key.toLowerCase();
    if (categories.geo.keys.some(kw => k.includes(kw))) return 'geo';
    if (categories.net.keys.some(kw => k.includes(kw))) return 'net';
    if (categories.sec.keys.some(kw => k.includes(kw))) return 'sec';
    if (categories.time.keys.some(kw => k.includes(kw))) return 'time';
    return 'other';
  };

  const categorizedData: Record<string, Array<{ key: string; label: string; value: string; category: string }>> = {
    all: [],
    geo: [],
    net: [],
    sec: [],
    time: [],
    other: [],
  };

  Object.entries(allParams).forEach(([key, value]) => {
    const label = formatKeyName(key);
    const category = getCategoryForKey(key);
    const entry = { key, label, value, category };
    categorizedData[category].push(entry);
    categorizedData.all.push(entry);
  });

  const filterEntries = (entries: typeof categorizedData.all) => {
    if (!searchTerm.trim()) return entries;
    const term = searchTerm.toLowerCase();
    return entries.filter(e => e.key.toLowerCase().includes(term) || e.label.toLowerCase().includes(term) || e.value.toLowerCase().includes(term));
  };

  const activeEntries = filterEntries(categorizedData[activeTab]);

  return (
    <div className="bg-[var(--color-brand-card)] border border-[var(--color-brand-border)] rounded-2xl p-6 shadow-sm flex flex-col gap-6 mt-6">
      <div className="flex flex-col sm:flex-row sm:items-center justify-between gap-4 border-b border-[var(--color-brand-border)] pb-4">
        <div>
          <h3 className="font-bold text-base text-[var(--color-brand-heading)] flex items-center gap-2">
            <FiDatabase className="text-[var(--color-brand)]" /> Comprehensive Parameter Registry
          </h3>
          <p className="text-xs text-[var(--color-brand-text)] mt-1">
            Displaying every available telemetry parameter, routing metadata, and registration detail returned by the lookup services.
          </p>
        </div>
        
        {/* Search Bar */}
        <div className="relative w-full sm:w-72">
          <span className="absolute inset-y-0 left-0 pl-3 flex items-center pointer-events-none text-[var(--color-brand-text)]">
            <FiSearch size={14} />
          </span>
          <input
            type="text"
            placeholder="Search parameters or values..."
            value={searchTerm}
            onChange={(e) => setSearchTerm(e.target.value)}
            className="w-full pl-9 pr-3 py-1.5 bg-[var(--color-brand-bg)] border border-[var(--color-brand-border)] rounded-lg text-xs focus:outline-none focus:ring-1 focus:ring-[var(--color-brand)] text-[var(--color-brand-heading)]"
          />
        </div>
      </div>

      {/* Tabs */}
      <div className="flex flex-wrap gap-2 border-b border-[var(--color-brand-border)] pb-2 overflow-x-auto">
        {Object.entries(categories).map(([catId, cat]) => {
          const count = filterEntries(categorizedData[catId]).length;
          const isActive = activeTab === catId;
          return (
            <button
              key={catId}
              onClick={() => setActiveTab(catId)}
              className={`flex items-center gap-1.5 px-3 py-2 text-xs font-semibold rounded-lg transition-all border whitespace-nowrap ${
                isActive
                  ? 'bg-[var(--color-brand)] text-white border-[var(--color-brand)] shadow-sm'
                  : 'bg-[var(--color-brand-bg)] text-[var(--color-brand-text)] hover:text-[var(--color-brand-heading)] border-[var(--color-brand-border)]'
              }`}
            >
              <span>{cat.icon}</span>
              <span>{cat.label}</span>
              <span className={`text-[10px] px-1.5 py-0.5 rounded-full ${isActive ? 'bg-white/20 text-white' : 'bg-[var(--color-brand-border)] text-[var(--color-brand-muted)]'}`}>
                {count}
              </span>
            </button>
          );
        })}
      </div>

      {/* Param Grid / Table */}
      {activeEntries.length > 0 ? (
        <div className="grid grid-cols-1 md:grid-cols-2 lg:grid-cols-3 gap-4">
          {activeEntries.map((item, idx) => {
            const isSecAlert = item.category === 'sec' && (item.value.toLowerCase() === 'true' || item.value.toLowerCase() === 'vpn' || item.value.toLowerCase() === 'tor');
            const isClean = item.value.toLowerCase() === 'false' || item.value.toLowerCase() === 'clean';
            
            return (
              <div 
                key={idx} 
                className="bg-[var(--color-brand-bg)] border border-[var(--color-brand-border)] p-4 rounded-xl flex flex-col justify-between hover:border-[var(--color-brand)] transition-all group relative overflow-hidden"
              >
                <div>
                  <div className="flex items-center justify-between gap-2 mb-1">
                    <span className="text-[10px] uppercase font-bold tracking-wider text-[var(--color-brand-muted)] truncate max-w-[80%]">
                      {item.key}
                    </span>
                    <button
                      onClick={() => {
                        navigator.clipboard.writeText(item.value);
                      }}
                      className="text-[10px] text-[var(--color-brand-muted)] hover:text-[var(--color-brand)] opacity-0 group-hover:opacity-100 transition-opacity"
                      title="Copy Value"
                    >
                      <FiCopy size={12} />
                    </button>
                  </div>
                  <h4 className="text-xs font-bold text-[var(--color-brand-heading)] mb-2">
                    {item.label}
                  </h4>
                </div>

                <div className="mt-1">
                  {isSecAlert ? (
                    <span className="inline-flex items-center gap-1 px-2.5 py-1 rounded-full text-[10px] font-bold bg-red-50 dark:bg-red-950/20 text-red-700 dark:text-red-400 border border-red-200 dark:border-red-900/40">
                      ⚠️ {item.value}
                    </span>
                  ) : isClean ? (
                    <span className="inline-flex items-center gap-1 px-2.5 py-1 rounded-full text-[10px] font-bold bg-green-50 dark:bg-green-950/20 text-green-700 dark:text-green-400 border border-green-200 dark:border-green-900/40">
                      ✓ {item.value}
                    </span>
                  ) : (
                    <span className="text-xs font-mono font-bold text-[var(--color-brand-text)] bg-[var(--color-brand-card)] px-2.5 py-1 rounded border border-[var(--color-brand-border)] inline-block break-all max-w-full">
                      {item.value}
                    </span>
                  )}
                </div>
              </div>
            );
          })}
        </div>
      ) : (
        <div className="text-center py-12 text-xs text-[var(--color-brand-muted)] bg-[var(--color-brand-bg)] rounded-xl border border-dashed border-[var(--color-brand-border)]">
          No parameters match the search criteria.
        </div>
      )}
    </div>
  );
};

export const IPDomainCheckerPage: React.FC = () => {
  const {
    activeTarget,
    lookupResult,
    isLoading,
    errorAlert,
    apiConfig,
    bulkProgress,
    bulkResults,
    isBulkLoading,
    fetchApiConfig,
    performLookup,
    startBulkLookup,
    stopBulkLookup,
    resetLookup
  } = useLookupStore();

  const [activeTab, setActiveTab] = useState<'single' | 'bulk'>('single');
  const [singleInput, setSingleInput] = useState('');
  const [bulkInput, setBulkInput] = useState('');
  const [copiedRaw, setCopiedRaw] = useState(false);
  const [copiedWhois, setCopiedWhois] = useState(false);
  const [isTestLoading, setIsTestLoading] = useState(false);

  useEffect(() => {
    fetchApiConfig();
  }, [fetchApiConfig]);

  useEffect(() => {
    const hashPart = window.location.hash.split('?')[1] || '';
    const searchPart = window.location.search || '';
    const params = new URLSearchParams(hashPart || searchPart);
    const targetParam = params.get('target');
    if (targetParam) {
      const decodedTarget = decodeURIComponent(targetParam).trim();
      if (decodedTarget) {
        setSingleInput(decodedTarget);
        setActiveTab('single');
        performLookup(decodedTarget);
      }
    }
  }, [performLookup]);

  // Handle single lookup search submit
  const handleSearch = (e: React.FormEvent) => {
    e.preventDefault();
    const target = singleInput.trim();
    if (!target) return;
    performLookup(target);
  };

  // Trigger loading single target from bulk table row click
  const loadSingleFromBulk = (ip: string) => {
    setSingleInput(ip);
    setActiveTab('single');
    performLookup(ip);
  };

  // Start bulk WebSocket scan
  const handleStartBulk = () => {
    const ips = bulkInput
      .split('\n')
      .map(s => s.trim())
      .filter(s => {
        // Simple IP validator regex (IPv4 / IPv6 basics)
        return s.length > 0 && !s.includes(' ') && (s.includes('.') || s.includes(':'));
      });
    
    if (ips.length === 0) return;
    startBulkLookup(ips);
  };

  // Helper: Copy string to clipboard
  const copyText = (text: string, setCopied: (v: boolean) => void) => {
    navigator.clipboard.writeText(text);
    setCopied(true);
    setTimeout(() => setCopied(false), 2000);
  };

  const currentTheme = localStorage.getItem('cc_theme') || 'light';
  // CartoDB Tile Layers are highly premium looking
  const tileUrl = currentTheme === 'dark'
    ? 'https://{s}.basemaps.cartocdn.com/dark_all/{z}/{x}/{y}{r}.png'
    : 'https://{s}.basemaps.cartocdn.com/light_all/{z}/{x}/{y}{r}.png';

  const mapAttribution = '&copy; <a href="https://www.openstreetmap.org/copyright">OpenStreetMap</a> &copy; <a href="https://carto.com/attributions">CARTO</a>';

  // Get current active location coordinates
  const lat = lookupResult?.geo?.latitude || 0;
  const lng = lookupResult?.geo?.longitude || 0;
  const hasCoordinates = lat !== 0 || lng !== 0;

  // Render Proxy status badge with styled colors
  const getProxyStatusInfo = (status: string) => {
    const norm = (status || '').toUpperCase();
    if (norm === 'CLEAN') return { bg: '#eefbf3', text: '#15803d', border: '#bbf7d0', label: 'Clean Connection' };
    if (norm === 'VPN') return { bg: '#fef3c7', text: '#d97706', border: '#fde68a', label: 'VPN Detected' };
    if (norm === 'TOR') return { bg: '#fef2f2', text: '#dc2626', border: '#fca5a5', label: 'Tor Exit Node' };
    if (norm === 'DCH' || norm === 'HOSTING') return { bg: '#eff6ff', text: '#2563eb', border: '#bfdbfe', label: 'DataCenter / Hosting' };
    return { bg: '#f4f4f5', text: '#71717a', border: '#e4e4e7', label: status || 'Unknown Security' };
  };

  const proxyInfo = getProxyStatusInfo(lookupResult?.geo?.proxy_status || 'Clean');

  // Check if API keys are configured (mainly ip2location key as it is primary)
  const hasPrimaryApiKey = apiConfig?.ip2location_key && apiConfig.ip2location_key.trim() !== '';

  return (
    <div className="flex flex-col gap-6 p-4 max-w-7xl mx-auto font-sans min-h-[calc(100vh-100px)]">
      {/* Page Title & Tab Selector */}
      <div className="flex flex-col md:flex-row md:items-center justify-between gap-4 border-b border-[var(--color-brand-border)] pb-4">
        <div>
          <h1 className="text-2xl font-bold text-[var(--color-brand-heading)] flex items-center gap-2">
            <FiGlobe className="text-[var(--color-brand)] animate-pulse" /> IP & Domain Intelligence
          </h1>
          <p className="text-xs text-[var(--color-brand-text)] mt-1">
            Resolve IP geolocation, proxy signals, network ASN ownership, and WHOIS domain registrations.
          </p>
        </div>

        {/* Tab Controls */}
        <div className="flex p-1 bg-[var(--color-brand-border)] rounded-lg self-start">
          <button
            onClick={() => setActiveTab('single')}
            className={`flex items-center gap-2 px-4 py-2 text-xs font-semibold rounded-md transition-all ${
              activeTab === 'single'
                ? 'bg-[var(--color-brand-card)] text-[var(--color-brand)] shadow-sm'
                : 'text-[var(--color-brand-text)] hover:text-[var(--color-brand-heading)]'
            }`}
          >
            <FiSearch size={14} /> Single Lookup
          </button>
          <button
            onClick={() => setActiveTab('bulk')}
            className={`flex items-center gap-2 px-4 py-2 text-xs font-semibold rounded-md transition-all ${
              activeTab === 'bulk'
                ? 'bg-[var(--color-brand-card)] text-[var(--color-brand)] shadow-sm'
                : 'text-[var(--color-brand-text)] hover:text-[var(--color-brand-heading)]'
            }`}
          >
            <FiList size={14} /> Bulk Resolver
          </button>
        </div>
      </div>

      {/* API Key Missing Alert */}
      {!hasPrimaryApiKey && (
        <div className="bg-orange-50 dark:bg-amber-950/20 border border-orange-200 dark:border-amber-900/50 rounded-xl p-4 flex flex-col sm:flex-row items-start sm:items-center justify-between gap-3 animate-pulse">
          <div className="flex items-center gap-3">
            <div className="bg-orange-100 dark:bg-amber-900/40 p-2 rounded-lg text-orange-600 dark:text-amber-500">
              <FiAlertTriangle size={18} />
            </div>
            <div>
              <h4 className="text-sm font-semibold text-orange-800 dark:text-amber-300">No IP2Location.io API Key configured</h4>
              <p className="text-xs text-orange-700 dark:text-amber-400/80 mt-0.5">
                Some advanced lookup queries, domain WHOIS records, or proxy flags require a valid API token.
              </p>
            </div>
          </div>
          <button 
            onClick={() => window.location.hash = '#/settings'}
            className="flex items-center gap-1.5 px-3 py-1.5 bg-orange-600 hover:bg-orange-700 text-white rounded-lg text-xs font-medium transition-all"
          >
            <FiSettings size={13} /> Configure API Keys
          </button>
        </div>
      )}

      {/* SINGLE LOOKUP TAB */}
      {activeTab === 'single' && (
        <div className="flex flex-col gap-6">
          {/* Search Card */}
          <div className="bg-[var(--color-brand-card)] border border-[var(--color-brand-border)] rounded-2xl p-6 shadow-sm">
            <form onSubmit={handleSearch} className="flex flex-col sm:flex-row gap-3">
              <div className="relative flex-1">
                <div className="absolute inset-y-0 left-0 pl-3 flex items-center pointer-events-none text-[var(--color-brand-text)]">
                  <FiGlobe size={18} />
                </div>
                <input
                  type="text"
                  placeholder="Enter IP address (e.g. 8.8.8.8) or domain (e.g. google.com)"
                  value={singleInput}
                  onChange={(e) => setSingleInput(e.target.value)}
                  className="w-full pl-10 pr-4 py-3 bg-[var(--color-brand-bg)] border border-[var(--color-brand-border)] rounded-xl text-sm focus:outline-none focus:ring-2 focus:ring-[var(--color-brand)] focus:border-transparent text-[var(--color-brand-heading)] placeholder-[var(--color-brand-muted)]"
                />
              </div>
              <button
                type="submit"
                disabled={isLoading || !singleInput.trim()}
                className="px-6 py-3 bg-[var(--color-brand)] hover:bg-[var(--color-brand-hover)] disabled:bg-orange-300 text-white font-medium text-sm rounded-xl transition-all flex items-center justify-center gap-2 shadow-sm"
              >
                {isLoading ? (
                  <>
                    <div className="w-4 h-4 border-2 border-white border-t-transparent rounded-full animate-spin" />
                    Querying APIs...
                  </>
                ) : (
                  <>
                    <FiSearch size={16} /> Scan Target
                  </>
                )}
              </button>
            </form>
          </div>

          {/* Error Alert Box */}
          {errorAlert && (
            <div className="bg-red-50 dark:bg-red-950/20 border border-red-200 dark:border-red-900/50 rounded-xl p-4 flex gap-3 text-red-800 dark:text-red-300">
              <FiAlertTriangle size={18} className="flex-shrink-0 mt-0.5" />
              <div>
                <h4 className="text-sm font-semibold">Lookup Request Failed</h4>
                <p className="text-xs mt-0.5 opacity-90">{errorAlert}</p>
              </div>
            </div>
          )}

          {/* Result Telemetry Layout */}
          {lookupResult && (
            <div className="flex flex-col gap-6">
              <div className="grid grid-cols-1 lg:grid-cols-12 gap-6 items-start animate-fade-in">
              {/* Left Column - Details & WHOIS */}
              <div className="lg:col-span-7 flex flex-col gap-6">
                
                {/* Geolocation Details Card */}
                {lookupResult.geo && (
                  <div className="bg-[var(--color-brand-card)] border border-[var(--color-brand-border)] rounded-2xl p-6 shadow-sm">
                    <div className="flex items-center justify-between border-b border-[var(--color-brand-border)] pb-4 mb-4">
                      <h3 className="font-bold text-sm text-[var(--color-brand-heading)] flex items-center gap-2">
                        <FiMapPin className="text-[var(--color-brand)]" /> Geolocation Telemetry
                      </h3>
                      <span className="text-[10px] uppercase font-bold text-[var(--color-brand-muted)] bg-[var(--color-brand-bg)] px-2.5 py-1 rounded-full">
                        API Source: {lookupResult.geo.provider}
                      </span>
                    </div>

                    <div className="grid grid-cols-1 sm:grid-cols-2 gap-4">
                      <div className="flex items-center gap-3 bg-[var(--color-brand-bg)] p-3 rounded-xl">
                        <span className="text-2xl" role="img" aria-label="flag">
                          {lookupResult.geo.country_code
                            ? lookupResult.geo.country_code.toUpperCase().replace(/./g, char => String.fromCodePoint(char.charCodeAt(0) + 127397))
                            : '🏳️'}
                        </span>
                        <div>
                          <div className="text-[10px] text-[var(--color-brand-muted)] font-semibold uppercase">Country</div>
                          <div className="text-xs font-bold text-[var(--color-brand-heading)]">
                            {lookupResult.geo.country} ({lookupResult.geo.country_code})
                          </div>
                        </div>
                      </div>

                      <div className="flex items-center gap-3 bg-[var(--color-brand-bg)] p-3 rounded-xl">
                        <div className="p-2 bg-blue-50 dark:bg-blue-950/20 text-blue-600 rounded-lg text-lg">
                          <FiServer size={18} />
                        </div>
                        <div>
                          <div className="text-[10px] text-[var(--color-brand-muted)] font-semibold uppercase">City / Region</div>
                          <div className="text-xs font-bold text-[var(--color-brand-heading)] truncate max-w-[180px]">
                            {lookupResult.geo.city || 'Unknown City'}
                          </div>
                        </div>
                      </div>

                      <div className="flex items-center gap-3 bg-[var(--color-brand-bg)] p-3 rounded-xl col-span-1 sm:col-span-2">
                        <div className="p-2 bg-indigo-50 dark:bg-indigo-950/20 text-indigo-600 rounded-lg text-lg">
                          <FiCpu size={18} />
                        </div>
                        <div className="flex-1 min-w-0">
                          <div className="text-[10px] text-[var(--color-brand-muted)] font-semibold uppercase">ISP & ASN</div>
                          <div className="text-xs font-bold text-[var(--color-brand-heading)] truncate">
                            {lookupResult.geo.isp || 'Unknown Provider'} ({lookupResult.geo.asn || 'No ASN'})
                          </div>
                        </div>
                      </div>

                      <div className="flex items-center gap-3 bg-[var(--color-brand-bg)] p-3 rounded-xl">
                        <div className="p-2 bg-purple-50 dark:bg-purple-950/20 text-purple-600 rounded-lg text-lg">
                          <FiGlobe size={18} />
                        </div>
                        <div>
                          <div className="text-[10px] text-[var(--color-brand-muted)] font-semibold uppercase">Coordinates</div>
                          <div className="text-xs font-bold text-[var(--color-brand-heading)]">
                            {lookupResult.geo.latitude.toFixed(4)}, {lookupResult.geo.longitude.toFixed(4)}
                          </div>
                        </div>
                      </div>

                      {/* Proxy Security Card Status */}
                      <div 
                        style={{
                          backgroundColor: proxyInfo.bg,
                          color: proxyInfo.text,
                          borderColor: proxyInfo.border
                        }}
                        className="flex items-center gap-3 p-3 rounded-xl border"
                      >
                        <div className="p-2 rounded-lg bg-white/70 dark:bg-black/20 text-lg">
                          <FiShield size={18} />
                        </div>
                        <div>
                          <div className="text-[10px] opacity-75 font-semibold uppercase">Proxy / Security</div>
                          <div className="text-xs font-bold">{proxyInfo.label}</div>
                        </div>
                      </div>
                    </div>
                  </div>
                )}

                {/* Domain WHOIS Metadata Card */}
                {lookupResult.type === 'domain' && (
                  <div className="bg-[var(--color-brand-card)] border border-[var(--color-brand-border)] rounded-2xl p-6 shadow-sm">
                    <div className="flex items-center justify-between border-b border-[var(--color-brand-border)] pb-4 mb-4">
                      <h3 className="font-bold text-sm text-[var(--color-brand-heading)] flex items-center gap-2">
                        <FiDatabase className="text-[var(--color-brand)]" /> Domain WHOIS Information
                      </h3>
                      {lookupResult.resolved_ip && (
                        <span className="text-[10px] font-bold text-[var(--color-brand-text)] bg-[var(--color-brand-bg)] px-2.5 py-1 rounded-full">
                          Resolved IP: {lookupResult.resolved_ip}
                        </span>
                      )}
                    </div>

                    {lookupResult.whois ? (
                      <div className="flex flex-col gap-4">
                        <div className="grid grid-cols-1 sm:grid-cols-2 gap-4">
                          <div className="bg-[var(--color-brand-bg)] p-3 rounded-xl">
                            <div className="text-[10px] text-[var(--color-brand-muted)] font-semibold uppercase">Registrar</div>
                            <div className="text-xs font-bold text-[var(--color-brand-heading)] truncate">
                              {lookupResult.whois.registrar || 'Unknown Registrar'}
                            </div>
                          </div>

                          <div className="bg-[var(--color-brand-bg)] p-3 rounded-xl">
                            <div className="text-[10px] text-[var(--color-brand-muted)] font-semibold uppercase">Creation Date</div>
                            <div className="text-xs font-bold text-[var(--color-brand-heading)]">
                              {lookupResult.whois.creation_date || 'N/A'}
                            </div>
                          </div>

                          <div className="bg-[var(--color-brand-bg)] p-3 rounded-xl">
                            <div className="text-[10px] text-[var(--color-brand-muted)] font-semibold uppercase">Expiration Date</div>
                            <div className="text-xs font-bold text-[var(--color-brand-heading)]">
                              {lookupResult.whois.expiry_date || 'N/A'}
                            </div>
                          </div>

                          <div className="bg-[var(--color-brand-bg)] p-3 rounded-xl">
                            <div className="text-[10px] text-[var(--color-brand-muted)] font-semibold uppercase">Cache Status</div>
                            <div className="text-xs font-bold text-[var(--color-brand-heading)] flex items-center gap-1">
                              {lookupResult.source === 'cache' ? 'Cached Local' : 'Fresh Request API'}
                            </div>
                          </div>
                        </div>

                        {/* Name Servers */}
                        {lookupResult.whois.name_servers && lookupResult.whois.name_servers.length > 0 && (
                          <div className="bg-[var(--color-brand-bg)] p-3 rounded-xl">
                            <div className="text-[10px] text-[var(--color-brand-muted)] font-semibold uppercase mb-1">Nameservers</div>
                            <div className="flex flex-wrap gap-1.5">
                              {lookupResult.whois.name_servers.map((ns, idx) => (
                                <span key={idx} className="bg-[var(--color-brand-card)] border border-[var(--color-brand-border)] text-[var(--color-brand-heading)] text-[10px] px-2 py-0.5 rounded font-mono">
                                  {ns}
                                </span>
                              ))}
                            </div>
                          </div>
                        )}

                        {/* WHOIS RAW payload */}
                        <div className="mt-2">
                          <div className="flex justify-between items-center mb-1.5">
                            <div className="text-[10px] text-[var(--color-brand-muted)] font-semibold uppercase">Raw WHOIS JSON Payload</div>
                            <button
                              onClick={() => copyText(lookupResult.whois?.raw_json || '', setCopiedWhois)}
                              className="text-[10px] text-[var(--color-brand)] hover:underline flex items-center gap-1 font-semibold"
                            >
                              {copiedWhois ? <FiCheck /> : <FiCopy />} {copiedWhois ? 'Copied' : 'Copy JSON'}
                            </button>
                          </div>
                          <pre className="text-[10px] font-mono bg-[var(--color-brand-bg)] p-3 rounded-xl overflow-x-auto max-h-[160px] text-[var(--color-brand-text)] border border-[var(--color-brand-border)]">
                            {JSON.stringify(JSON.parse(lookupResult.whois.raw_json), null, 2)}
                          </pre>
                        </div>
                      </div>
                    ) : (
                      <div className="text-center py-6 text-xs text-[var(--color-brand-muted)]">
                        No WHOIS record details found. (Make sure you set your IP2Location key in settings)
                      </div>
                    )}
                  </div>
                )}
              </div>

              {/* Right Column - Map & Raw JSON */}
              <div className="lg:col-span-5 flex flex-col gap-6">
                
                {/* Map Card */}
                <div className="bg-[var(--color-brand-card)] border border-[var(--color-brand-border)] rounded-2xl p-6 shadow-sm overflow-hidden flex flex-col">
                  <div className="flex items-center justify-between border-b border-[var(--color-brand-border)] pb-4 mb-4">
                    <h3 className="font-bold text-sm text-[var(--color-brand-heading)] flex items-center gap-2">
                      <FiLayers className="text-[var(--color-brand)]" /> Interactive Map Location
                    </h3>
                  </div>

                  {hasCoordinates ? (
                    <div className="w-full h-[280px] rounded-xl overflow-hidden border border-[var(--color-brand-border)] relative z-10">
                      <MapContainer
                        center={[lat, lng]}
                        zoom={9}
                        scrollWheelZoom={true}
                        style={{ height: '100%', width: '100%' }}
                      >
                        <TileLayer
                          url={tileUrl}
                          attribution={mapAttribution}
                        />
                        <Marker position={[lat, lng]} icon={customMarkerIcon}>
                          <Popup>
                            <div className="text-xs font-sans">
                              <p className="font-bold text-[var(--color-brand-heading)]">{lookupResult.geo?.city || 'IP Location'}</p>
                              <p className="text-[var(--color-brand-text)] mt-0.5">{lookupResult.geo?.isp}</p>
                              <p className="text-[var(--color-brand-muted)] font-mono text-[10px] mt-0.5">{lat.toFixed(5)}, {lng.toFixed(5)}</p>
                            </div>
                          </Popup>
                        </Marker>
                        <ChangeMapView center={[lat, lng]} />
                      </MapContainer>
                    </div>
                  ) : (
                    <div className="w-full h-[280px] bg-[var(--color-brand-bg)] border border-[var(--color-brand-border)] rounded-xl flex flex-col items-center justify-center text-center p-6 gap-3">
                      <FiLayers size={32} className="text-[var(--color-brand-muted)] animate-bounce" />
                      <div className="text-xs font-bold text-[var(--color-brand-heading)]">Coordinates Unavailable</div>
                      <p className="text-[10px] text-[var(--color-brand-text)] max-w-[200px]">
                        No latitude or longitude details resolved for this target.
                      </p>
                    </div>
                  )}
                </div>

                {/* Geo Raw Payload Card */}
                {lookupResult.geo && (
                  <div className="bg-[var(--color-brand-card)] border border-[var(--color-brand-border)] rounded-2xl p-6 shadow-sm">
                    <div className="flex justify-between items-center border-b border-[var(--color-brand-border)] pb-4 mb-4">
                      <h3 className="font-bold text-sm text-[var(--color-brand-heading)] flex items-center gap-2">
                        <FiCpu className="text-[var(--color-brand)]" /> API Response Payload
                      </h3>
                      <button
                        onClick={() => copyText(lookupResult.geo?.raw_json || '', setCopiedRaw)}
                        className="text-[10px] text-[var(--color-brand)] hover:underline flex items-center gap-1 font-semibold"
                      >
                        {copiedRaw ? <FiCheck /> : <FiCopy />} {copiedRaw ? 'Copied' : 'Copy JSON'}
                      </button>
                    </div>
                    <pre className="text-[10px] font-mono bg-[var(--color-brand-bg)] p-3 rounded-xl overflow-x-auto max-h-[140px] text-[var(--color-brand-text)] border border-[var(--color-brand-border)]">
                      {JSON.stringify(JSON.parse(lookupResult.geo.raw_json), null, 2)}
                    </pre>
                  </div>
                )}
              </div>
            </div>
            
            {/* Comprehensive Intelligence Profile */}
            <ComprehensiveProfileCard result={lookupResult} />
          </div>
        )}

          {/* Empty State when no results resolved yet */}
          {!lookupResult && !isLoading && (
            <div className="bg-[var(--color-brand-card)] border border-[var(--color-brand-border)] rounded-2xl p-12 text-center flex flex-col items-center justify-center gap-4 min-h-[300px]">
              <div className="w-16 h-16 rounded-full bg-[var(--color-brand-light)] text-[var(--color-brand)] flex items-center justify-center animate-bounce shadow-inner">
                <FiGlobe size={28} />
              </div>
              <div>
                <h3 className="text-base font-bold text-[var(--color-brand-heading)]">Awaiting IP/Domain Scan</h3>
                <p className="text-xs text-[var(--color-brand-text)] max-w-sm mt-1 mx-auto leading-relaxed">
                  Enter an IP address or a website domain to query real-time routing metrics, geographical boundaries, and DNS credentials.
                </p>
              </div>
            </div>
          )}
        </div>
      )}

      {/* BULK RESOLVER TAB */}
      {activeTab === 'bulk' && (
        <div className="grid grid-cols-1 lg:grid-cols-12 gap-6 items-start">
          {/* Input Panel Card */}
          <div className="lg:col-span-4 bg-[var(--color-brand-card)] border border-[var(--color-brand-border)] rounded-2xl p-6 shadow-sm flex flex-col gap-4">
            <h3 className="font-bold text-sm text-[var(--color-brand-heading)] flex items-center gap-2">
              <FiList className="text-[var(--color-brand)]" /> Bulk IP Queue Input
            </h3>
            <p className="text-[11px] text-[var(--color-brand-text)]">
              Input a list of IPv4 or IPv6 targets (one IP per line). The WebSocket runner streams results back instantly.
            </p>

            <textarea
              rows={8}
              value={bulkInput}
              onChange={(e) => setBulkInput(e.target.value)}
              placeholder="8.8.8.8&#10;1.1.1.1&#10;104.244.42.1&#10;185.112.33.4"
              disabled={isBulkLoading}
              className="w-full p-3 bg-[var(--color-brand-bg)] border border-[var(--color-brand-border)] rounded-xl text-xs font-mono focus:outline-none focus:ring-2 focus:ring-[var(--color-brand)] focus:border-transparent text-[var(--color-brand-heading)] placeholder-[var(--color-brand-muted)]"
            />

            <div className="flex gap-2">
              {isBulkLoading ? (
                <button
                  onClick={stopBulkLookup}
                  className="flex-1 py-2.5 bg-red-600 hover:bg-red-700 text-white font-medium text-xs rounded-xl transition-all flex items-center justify-center gap-2 shadow-sm"
                >
                  <FiSquare size={13} /> Stop Scanner
                </button>
              ) : (
                <button
                  onClick={handleStartBulk}
                  disabled={!bulkInput.trim()}
                  className="flex-1 py-2.5 bg-[var(--color-brand)] hover:bg-[var(--color-brand-hover)] disabled:bg-orange-300 text-white font-medium text-xs rounded-xl transition-all flex items-center justify-center gap-2 shadow-sm"
                >
                  <FiPlay size={13} /> Execute Scan
                </button>
              )}
            </div>
          </div>

          {/* Table / Results Stream Card */}
          <div className="lg:col-span-8 bg-[var(--color-brand-card)] border border-[var(--color-brand-border)] rounded-2xl p-6 shadow-sm flex flex-col gap-6">
            {/* Header + Stats */}
            <div className="flex flex-col sm:flex-row sm:items-center justify-between gap-4 border-b border-[var(--color-brand-border)] pb-4">
              <div>
                <h3 className="font-bold text-sm text-[var(--color-brand-heading)] flex items-center gap-2">
                  <FiActivity className="text-[var(--color-brand)]" /> Scanner Stream Output
                </h3>
              </div>

              {/* Progress and Counters */}
              <div className="flex items-center gap-4 text-xs">
                <div className="flex items-center gap-1 text-[var(--color-brand-text)] font-semibold">
                  <FiTrendingUp size={13} className="text-green-500" /> Resolved: {bulkProgress.resolved} / {bulkProgress.total}
                </div>

                {isBulkLoading && (
                  <span className="flex h-2.5 w-2.5 relative">
                    <span className="animate-ping absolute inline-flex h-full w-full rounded-full bg-green-400 opacity-75"></span>
                    <span className="relative inline-flex rounded-full h-2.5 w-2.5 bg-green-500"></span>
                  </span>
                )}
              </div>
            </div>

            {/* Progress Bar */}
            {bulkProgress.total > 0 && (
              <div className="w-full bg-[var(--color-brand-bg)] h-2 rounded-full overflow-hidden border border-[var(--color-brand-border)]">
                <div 
                  className="bg-[var(--color-brand)] h-full transition-all duration-300"
                  style={{ width: `${(bulkProgress.resolved / bulkProgress.total) * 100}%` }}
                />
              </div>
            )}

            {/* Result Table */}
            <div className="overflow-x-auto">
              <table className="w-full text-left text-xs border-collapse">
                <thead>
                  <tr className="border-b border-[var(--color-brand-border)] text-[var(--color-brand-muted)] uppercase tracking-wider font-bold">
                    <th className="py-2.5 pb-2.5 font-bold">IP Address</th>
                    <th className="py-2.5 pb-2.5 font-bold">Country</th>
                    <th className="py-2.5 pb-2.5 font-bold">ISP / Network</th>
                    <th className="py-2.5 pb-2.5 font-bold text-center">Proxy Status</th>
                    <th className="py-2.5 pb-2.5 font-bold text-right">Actions</th>
                  </tr>
                </thead>
                <tbody className="divide-y divide-[var(--color-brand-border)]">
                  {bulkResults.map((item, idx) => {
                    const statusInfo = getProxyStatusInfo(item.proxy_status);
                    return (
                      <tr key={idx} className="hover:bg-[var(--color-brand-bg)] transition-all">
                        <td className="py-3 font-mono font-bold text-[var(--color-brand-heading)]">
                          {item.ip}
                        </td>
                        <td className="py-3 text-[var(--color-brand-heading)] font-semibold flex items-center gap-1.5">
                          <span className="text-base">
                            {item.country_code
                              ? item.country_code.toUpperCase().replace(/./g, char => String.fromCodePoint(char.charCodeAt(0) + 127397))
                              : '🏳️'}
                          </span>
                          {item.country}
                        </td>
                        <td className="py-3 text-[var(--color-brand-text)] max-w-[160px] truncate">
                          {item.isp} ({item.asn})
                        </td>
                        <td className="py-3 text-center">
                          <span 
                            style={{ backgroundColor: statusInfo.bg, color: statusInfo.text, borderColor: statusInfo.border }}
                            className="px-2 py-0.5 rounded border text-[9px] font-bold inline-block"
                          >
                            {statusInfo.label}
                          </span>
                        </td>
                        <td className="py-3 text-right">
                          <button
                            onClick={() => loadSingleFromBulk(item.ip)}
                            className="text-[var(--color-brand)] hover:text-[var(--color-brand-hover)] font-bold flex items-center gap-1 justify-end ml-auto"
                          >
                            Map Details <FiArrowRight size={12} />
                          </button>
                        </td>
                      </tr>
                    );
                  })}

                  {bulkResults.length === 0 && (
                    <tr>
                      <td colSpan={5} className="py-8 text-center text-[var(--color-brand-muted)]">
                        No resolved results yet. Enter queue list and execute scan to start streaming.
                      </td>
                    </tr>
                  )}
                </tbody>
              </table>
            </div>
          </div>
        </div>
      )}
    </div>
  );
};
