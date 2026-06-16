import React, { useState } from 'react';
import { FiDatabase, FiSearch, FiCopy } from 'react-icons/fi';

interface ComprehensiveProfileCardProps {
  result: any;
}

export const ComprehensiveProfileCard: React.FC<ComprehensiveProfileCardProps> = ({ result }) => {
  const [searchTerm, setSearchTerm] = useState('');
  const [activeTab, setActiveTab] = useState('all');

  const rawGeo = result?.geo?.raw_json ? JSON.parse(result.geo.raw_json) : {};
  const rawWhois = result?.whois?.raw_json ? JSON.parse(result.whois.raw_json) : {};

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

export default ComprehensiveProfileCard;
