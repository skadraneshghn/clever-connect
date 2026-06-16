import React, { Suspense, lazy } from 'react';
import { 
  FiGlobe, FiSearch, FiAlertTriangle, FiMapPin, FiServer, 
  FiCpu, FiShield, FiDatabase, FiCheck, FiCopy 
} from 'react-icons/fi';
import { MapSkeleton } from './Skeletons';

const MapCard = lazy(() => import('./MapCard'));

interface SingleLookupProps {
  singleInput: string;
  setSingleInput: (v: string) => void;
  isLoading: boolean;
  errorAlert: string | null;
  lookupResult: any;
  performLookup: (target: string) => void;
  copyText: (text: string, setCopied: (v: boolean) => void) => void;
  copiedRaw: boolean;
  setCopiedRaw: (v: boolean) => void;
  copiedWhois: boolean;
  setCopiedWhois: (v: boolean) => void;
  proxyInfo: {
    bg: string;
    text: string;
    border: string;
    label: string;
  };
  tileUrl: string;
  mapAttribution: string;
  lat: number;
  lng: number;
  hasCoordinates: boolean;
}

export const SingleLookup: React.FC<SingleLookupProps> = ({
  singleInput,
  setSingleInput,
  isLoading,
  errorAlert,
  lookupResult,
  performLookup,
  copyText,
  copiedRaw,
  setCopiedRaw,
  copiedWhois,
  setCopiedWhois,
  proxyInfo,
  tileUrl,
  mapAttribution,
  lat,
  lng,
  hasCoordinates,
}) => {
  const handleSearch = (e: React.FormEvent) => {
    e.preventDefault();
    const target = singleInput.trim();
    if (!target) return;
    performLookup(target);
  };

  return (
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
                          ? lookupResult.geo.country_code.toUpperCase().replace(/./g, (char: string) => String.fromCodePoint(char.charCodeAt(0) + 127397))
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
                            {lookupResult.whois.name_servers.map((ns: string, idx: number) => (
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
                            type="button"
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
              
              {/* Lazy-Loaded Interactive Map with Skeleton Fallback */}
              <Suspense fallback={<MapSkeleton />}>
                <MapCard 
                  lat={lat} 
                  lng={lng} 
                  hasCoordinates={hasCoordinates} 
                  tileUrl={tileUrl} 
                  mapAttribution={mapAttribution} 
                  lookupResult={lookupResult} 
                />
              </Suspense>

              {/* Geo Raw Payload Card */}
              {lookupResult.geo && (
                <div className="bg-[var(--color-brand-card)] border border-[var(--color-brand-border)] rounded-2xl p-6 shadow-sm">
                  <div className="flex justify-between items-center border-b border-[var(--color-brand-border)] pb-4 mb-4">
                    <h3 className="font-bold text-sm text-[var(--color-brand-heading)] flex items-center gap-2">
                      <FiCpu className="text-[var(--color-brand)]" /> API Response Payload
                    </h3>
                    <button
                      type="button"
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
  );
};
