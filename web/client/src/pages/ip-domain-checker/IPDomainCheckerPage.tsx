import React, { useState, useEffect, Suspense, lazy } from 'react';
import { 
  FiGlobe, FiAlertTriangle, FiSettings, FiSearch, FiList 
} from 'react-icons/fi';
import { useLookupStore } from '../../store/lookupStore';

import { SingleLookup } from './components/SingleLookup';
import { BulkResolver } from './components/BulkResolver';
import { ProfileSkeleton } from './components/Skeletons';

const ComprehensiveProfileCard = lazy(() => import('./components/ComprehensiveProfileCard'));

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
  // CartoDB Tile Layers
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

  // Check if API keys are configured
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
          <SingleLookup
            singleInput={singleInput}
            setSingleInput={setSingleInput}
            isLoading={isLoading}
            errorAlert={errorAlert}
            lookupResult={lookupResult}
            performLookup={performLookup}
            copyText={copyText}
            copiedRaw={copiedRaw}
            setCopiedRaw={setCopiedRaw}
            copiedWhois={copiedWhois}
            setCopiedWhois={setCopiedWhois}
            proxyInfo={proxyInfo}
            tileUrl={tileUrl}
            mapAttribution={mapAttribution}
            lat={lat}
            lng={lng}
            hasCoordinates={hasCoordinates}
          />
          
          {/* Lazy loaded Parameter Details */}
          {lookupResult && (
            <Suspense fallback={<ProfileSkeleton />}>
              <ComprehensiveProfileCard result={lookupResult} />
            </Suspense>
          )}
        </div>
      )}

      {/* BULK RESOLVER TAB */}
      {activeTab === 'bulk' && (
        <BulkResolver
          bulkInput={bulkInput}
          setBulkInput={setBulkInput}
          isBulkLoading={isBulkLoading}
          bulkProgress={bulkProgress}
          bulkResults={bulkResults}
          onStartBulk={handleStartBulk}
          onStopBulk={stopBulkLookup}
          loadSingleFromBulk={loadSingleFromBulk}
          getProxyStatusInfo={getProxyStatusInfo}
        />
      )}
    </div>
  );
};
