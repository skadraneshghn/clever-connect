import React from 'react';
import {
  FiGlobe, FiTrash2, FiPlus, FiChevronUp, FiChevronDown, FiUpload
} from 'react-icons/fi';

export interface ScannerSource {
  id: number;
  name: string;
  url: string;
  type: 'cidr' | 'proxyip' | 'domain';
  is_enabled: boolean;
}

export const COMMON_PORTS = [
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

export const ToggleSwitch: React.FC<{ checked: boolean; onChange: () => void }> = ({ checked, onChange }) => {
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

interface ScannerConfigProps {
  selectedCDNs: string[];
  setSelectedCDNs: React.Dispatch<React.SetStateAction<string[]>>;
  selectedPortsList: number[];
  setSelectedPortsList: React.Dispatch<React.SetStateAction<number[]>>;
  customPorts: string;
  setCustomPorts: (v: string) => void;
  sources: ScannerSource[];
  showAddSourceInline: boolean;
  setShowAddSourceInline: (v: boolean) => void;
  newSourceName: string;
  setNewSourceName: (v: string) => void;
  newSourceUrl: string;
  setNewSourceUrl: (v: string) => void;
  newSourceType: 'cidr' | 'proxyip' | 'domain';
  setNewSourceType: (v: 'cidr' | 'proxyip' | 'domain') => void;
  showAdvancedSettings: boolean;
  setShowAdvancedSettings: (v: boolean) => void;
  isDragging: boolean;
  rawConfigLink: string;
  setRawConfigLink: (v: string) => void;
  targetCidrs: string;
  setTargetCidrs: (v: string) => void;
  concurrencyLimit: number;
  setConcurrencyLimit: (v: number) => void;
  maxRateLimit: number;
  setMaxRateLimit: (v: number) => void;
  networkTimeoutMs: number;
  setNetworkTimeoutMs: (v: number) => void;
  probeAttempts: number;
  setProbeAttempts: (v: number) => void;
  targetMode: 'ws' | 'tls' | 'http';
  setTargetMode: (v: 'ws' | 'tls' | 'http') => void;
  targetSni: string;
  setTargetSni: (v: string) => void;
  websocketHost: string;
  setWebsocketHost: (v: string) => void;
  websocketPath: string;
  setWebsocketPath: (v: string) => void;
  requireWs: boolean;
  setRequireWs: (v: boolean) => void;
  enableNeighbors: boolean;
  setEnableNeighbors: (v: boolean) => void;
  topLimit: number;
  setTopLimit: (v: number) => void;
  totalTargetCount: number;
  setTotalTargetCount: (v: number) => void;

  handleParseLink: () => void;
  handleToggleSource: (src: ScannerSource) => void;
  handleDeleteSource: (id: number) => void;
  handleResetSources: () => void;
  handleAddSourceSubmit: (e: React.FormEvent) => void;
  handleSelectAllPorts: () => void;
  handleClearPorts: () => void;
  handleTogglePort: (port: number) => void;
  handleDragEnter: (e: React.DragEvent) => void;
  handleDragOver: (e: React.DragEvent) => void;
  handleDragLeave: (e: React.DragEvent) => void;
  handleDrop: (e: React.DragEvent) => void;
}

export const ScannerConfig: React.FC<ScannerConfigProps> = ({
  selectedCDNs,
  setSelectedCDNs,
  selectedPortsList,
  customPorts,
  setCustomPorts,
  sources,
  showAddSourceInline,
  setShowAddSourceInline,
  newSourceName,
  setNewSourceName,
  newSourceUrl,
  setNewSourceUrl,
  newSourceType,
  setNewSourceType,
  showAdvancedSettings,
  setShowAdvancedSettings,
  isDragging,
  rawConfigLink,
  setRawConfigLink,
  targetCidrs,
  setTargetCidrs,
  concurrencyLimit,
  setConcurrencyLimit,
  maxRateLimit,
  setMaxRateLimit,
  networkTimeoutMs,
  setNetworkTimeoutMs,
  probeAttempts,
  setProbeAttempts,
  targetMode,
  setTargetMode,
  targetSni,
  setTargetSni,
  websocketHost,
  setWebsocketHost,
  websocketPath,
  setWebsocketPath,
  requireWs,
  setRequireWs,
  enableNeighbors,
  setEnableNeighbors,
  topLimit,
  setTopLimit,
  totalTargetCount,
  setTotalTargetCount,

  handleParseLink,
  handleToggleSource,
  handleDeleteSource,
  handleResetSources,
  handleAddSourceSubmit,
  handleSelectAllPorts,
  handleClearPorts,
  handleTogglePort,
  handleDragEnter,
  handleDragOver,
  handleDragLeave,
  handleDrop
}) => {
  const enabledSourcesCount = sources.filter((s) => s.is_enabled).length;

  return (
    <div style={{ display: 'flex', flexDirection: 'column', gap: 20 }}>
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
    </div>
  );
};
