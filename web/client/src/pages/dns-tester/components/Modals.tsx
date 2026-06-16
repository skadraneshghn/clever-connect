import React from 'react';
import { 
  FiGlobe, FiX, FiCheckCircle, FiDownload, 
  FiRefreshCw, FiPlay, FiActivity, FiDatabase,
  FiAlertTriangle, FiInfo
} from 'react-icons/fi';
import type { DNSResolver } from '../../../store/dnsStore';

// 1. Fetch Public DNS Modal
interface FetchPublicModalProps {
  show: boolean;
  onClose: () => void;
  isFetchingPublic: boolean;
  fetchPublicResult: any;
  selectedPublicSource: string;
  setSelectedPublicSource: (src: string) => void;
  onFetch: () => void;
}

export const FetchPublicModal: React.FC<FetchPublicModalProps> = ({
  show,
  onClose,
  isFetchingPublic,
  fetchPublicResult,
  selectedPublicSource,
  setSelectedPublicSource,
  onFetch,
}) => {
  if (!show) return null;
  return (
    <div
      style={{
        position: 'fixed',
        top: 0, left: 0, width: '100%', height: '100%',
        background: 'rgba(0,0,0,0.6)',
        backdropFilter: 'blur(4px)',
        display: 'flex', alignItems: 'center', justifyContent: 'center',
        zIndex: 999,
      }}
      onClick={() => {
        if (!isFetchingPublic) onClose();
      }}
    >
      <div
        style={{
          background: 'var(--color-brand-card)',
          padding: 28,
          borderRadius: 16,
          width: 500,
          maxWidth: '90%',
          boxShadow: '0 20px 25px -5px rgba(0, 0, 0, 0.3), 0 10px 10px -5px rgba(0, 0, 0, 0.2)',
          border: '1px solid var(--color-brand-border)',
          display: 'flex',
          flexDirection: 'column',
          gap: 16
        }}
        onClick={(e) => e.stopPropagation()}
      >
        <div style={{ display: 'flex', justifyContent: 'space-between', alignItems: 'center' }}>
          <h3 style={{ margin: 0, color: 'var(--color-brand-heading)', display: 'flex', alignItems: 'center', gap: 8 }}>
            <FiGlobe style={{ color: 'var(--color-brand)' }} /> Load Public DNS from Internet
          </h3>
          {!isFetchingPublic && (
            <button
              onClick={onClose}
              style={{ background: 'none', border: 'none', cursor: 'pointer', color: 'var(--color-brand-text)' }}
            >
              <FiX size={18} />
            </button>
          )}
        </div>

        <p style={{ fontSize: 13, color: 'var(--color-brand-text)', margin: '0 0 8px 0', lineHeight: '1.5' }}>
          Import thousands of active, verified public nameservers from remote repositories. Duplicate IP addresses and protocols will automatically be filtered out to keep your list clean.
        </p>

        {fetchPublicResult ? (
          <div style={{
            background: 'rgba(16, 185, 129, 0.08)',
            border: '1px solid rgba(16, 185, 129, 0.3)',
            borderRadius: 12,
            padding: 16,
            display: 'flex',
            flexDirection: 'column',
            gap: 8,
            textAlign: 'center'
          }}>
            <FiCheckCircle size={32} style={{ color: 'var(--color-brand-green)', margin: '0 auto 4px auto' }} />
            <h4 style={{ margin: 0, color: 'var(--color-brand-heading)' }}>Import Successful!</h4>
            <div style={{ fontSize: 13, color: 'var(--color-brand-text)' }}>
              Successfully scanned and processed <strong>{fetchPublicResult.total_found}</strong> potential resolvers.
            </div>
            <div style={{ fontSize: 15, fontWeight: 'bold', color: 'var(--color-brand-green)' }}>
              + {fetchPublicResult.added_count} New DNS Resolvers Added
            </div>
            <p style={{ fontSize: 11, color: 'var(--color-brand-text)', margin: '4px 0 0 0' }}>
              All duplicate or existing resolvers were ignored.
            </p>
            <button
              className="btn btn-primary"
              onClick={onClose}
              style={{ marginTop: 12, alignSelf: 'center', padding: '6px 20px', borderRadius: 8 }}
            >
              Done
            </button>
          </div>
        ) : (
          <>
            <div style={{ display: 'flex', flexDirection: 'column', gap: 12 }}>
              <label style={{ fontSize: 12, fontWeight: 700, color: 'var(--color-brand-heading)' }}>
                Select Resolver Data Source:
              </label>

              <div style={{ display: 'flex', flexDirection: 'column', gap: 8 }}>
                <div
                  onClick={() => !isFetchingPublic && setSelectedPublicSource('curated')}
                  style={{
                    padding: 12,
                    borderRadius: 10,
                    border: `1px solid ${selectedPublicSource === 'curated' ? 'var(--color-brand)' : 'var(--color-brand-border)'}`,
                    background: selectedPublicSource === 'curated' ? 'rgba(59, 130, 246, 0.05)' : 'var(--color-brand-bg)',
                    cursor: isFetchingPublic ? 'not-allowed' : 'pointer',
                    transition: 'all 0.2s ease'
                  }}
                >
                  <div style={{ display: 'flex', alignItems: 'center', gap: 8, marginBottom: 4 }}>
                    <input
                      type="radio"
                      checked={selectedPublicSource === 'curated'}
                      readOnly
                      disabled={isFetchingPublic}
                    />
                    <strong style={{ fontSize: 13, color: 'var(--color-brand-heading)' }}>Curated Anycast DNS (Highly Recommended)</strong>
                  </div>
                  <span style={{ fontSize: 11, color: 'var(--color-brand-text)', paddingLeft: 20, display: 'block' }}>
                    Google DNS, Cloudflare, Quad9, AdGuard, OpenDNS, CleanBrowsing, DNS.SB, ControlD, Level3, Comodo, etc. (High stability & low latency).
                  </span>
                </div>

                <div
                  onClick={() => !isFetchingPublic && setSelectedPublicSource('bls')}
                  style={{
                    padding: 12,
                    borderRadius: 10,
                    border: `1px solid ${selectedPublicSource === 'bls' ? 'var(--color-brand)' : 'var(--color-brand-border)'}`,
                    background: selectedPublicSource === 'bls' ? 'rgba(59, 130, 246, 0.05)' : 'var(--color-brand-bg)',
                    cursor: isFetchingPublic ? 'not-allowed' : 'pointer',
                    transition: 'all 0.2s ease'
                  }}
                >
                  <div style={{ display: 'flex', alignItems: 'center', gap: 8, marginBottom: 4 }}>
                    <input
                      type="radio"
                      checked={selectedPublicSource === 'bls'}
                      readOnly
                      disabled={isFetchingPublic}
                    />
                    <strong style={{ fontSize: 13, color: 'var(--color-brand-heading)' }}>BLS Global Verified List (6,000+ DNS)</strong>
                  </div>
                  <span style={{ fontSize: 11, color: 'var(--color-brand-text)', paddingLeft: 20, display: 'block' }}>
                    Weekly verified public DNS server list compiled from various active nameservers worldwide.
                  </span>
                </div>

                <div
                  onClick={() => !isFetchingPublic && setSelectedPublicSource('trickest')}
                  style={{
                    padding: 12,
                    borderRadius: 10,
                    border: `1px solid ${selectedPublicSource === 'trickest' ? 'var(--color-brand)' : 'var(--color-brand-border)'}`,
                    background: selectedPublicSource === 'trickest' ? 'rgba(59, 130, 246, 0.05)' : 'var(--color-brand-bg)',
                    cursor: isFetchingPublic ? 'not-allowed' : 'pointer',
                    transition: 'all 0.2s ease'
                  }}
                >
                  <div style={{ display: 'flex', alignItems: 'center', gap: 8, marginBottom: 4 }}>
                    <input
                      type="radio"
                      checked={selectedPublicSource === 'trickest'}
                      readOnly
                      disabled={isFetchingPublic}
                    />
                    <strong style={{ fontSize: 13, color: 'var(--color-brand-heading)' }}>Trickest Verified Resolvers</strong>
                  </div>
                  <span style={{ fontSize: 11, color: 'var(--color-brand-text)', paddingLeft: 20, display: 'block' }}>
                    Exhaustive multi-source public DNS server compilation validated using active validation algorithms.
                  </span>
                </div>
              </div>
            </div>

            <div style={{ display: 'flex', gap: 10, justifyContent: 'flex-end', marginTop: 12 }}>
              <button
                onClick={onClose}
                disabled={isFetchingPublic}
                style={{
                  padding: '8px 16px',
                  borderRadius: 8,
                  border: '1px solid var(--color-brand-border)',
                  background: 'var(--color-brand-bg)',
                  color: 'var(--color-brand-heading)',
                  cursor: isFetchingPublic ? 'not-allowed' : 'pointer'
                }}
              >
                Cancel
              </button>
              <button
                onClick={onFetch}
                disabled={isFetchingPublic}
                className="btn btn-primary"
                style={{
                  padding: '8px 20px',
                  borderRadius: 8,
                  display: 'flex',
                  alignItems: 'center',
                  gap: 8,
                  cursor: isFetchingPublic ? 'wait' : 'pointer'
                }}
              >
                {isFetchingPublic ? (
                  <>
                    <FiRefreshCw className="animate-spin" /> Fetching and Importing...
                  </>
                ) : (
                  <>
                    <FiDownload /> Fetch & Import
                  </>
                )}
              </button>
            </div>
          </>
        )}
      </div>
    </div>
  );
};

// 2. Add Custom Resolver Modal
interface AddResolverModalProps {
  show: boolean;
  onClose: () => void;
  modalTab: 'single' | 'bulk';
  setModalTab: (tab: 'single' | 'bulk') => void;
  customIP: string;
  setCustomIP: (ip: string) => void;
  customProvider: string;
  setCustomProvider: (p: string) => void;
  customProtocol: string;
  setCustomProtocol: (proto: string) => void;
  customCategory: string;
  setCustomCategory: (cat: string) => void;
  handleAddCustomResolver: () => void;
  bulkText: string;
  setBulkText: (t: string) => void;
  bulkFile: File | null;
  setBulkFile: (f: File | null) => void;
  handleBulkImport: () => void;
  isImporting: boolean;
}

export const AddResolverModal: React.FC<AddResolverModalProps> = ({
  show,
  onClose,
  modalTab,
  setModalTab,
  customIP,
  setCustomIP,
  customProvider,
  setCustomProvider,
  customProtocol,
  setCustomProtocol,
  customCategory,
  setCustomCategory,
  handleAddCustomResolver,
  bulkText,
  setBulkText,
  bulkFile,
  setBulkFile,
  handleBulkImport,
  isImporting,
}) => {
  if (!show) return null;
  return (
    <div
      style={{
        position: 'fixed',
        top: 0, left: 0, width: '100%', height: '100%',
        background: 'rgba(0,0,0,0.5)',
        display: 'flex', alignItems: 'center', justifyContent: 'center',
        zIndex: 999,
      }}
      onClick={onClose}
    >
      <div
        style={{
          background: 'var(--color-brand-card)',
          padding: 24,
          borderRadius: 12,
          width: 460,
          maxWidth: '90%',
          boxShadow: '0 10px 25px rgba(0,0,0,0.15)',
          display: 'flex',
          flexDirection: 'column',
          gap: 14
        }}
        onClick={(e) => e.stopPropagation()}
      >
        <h3 style={{ fontSize: 16, fontWeight: 700, color: 'var(--color-brand-heading)', margin: 0 }}>
          Add DNS Resolvers
        </h3>

        <div style={{ display: 'flex', borderBottom: '1px solid var(--color-brand-border)', marginBottom: 4 }}>
          <button
            onClick={() => setModalTab('single')}
            style={{
              flex: 1,
              padding: '8px 16px',
              background: 'none',
              border: 'none',
              borderBottom: modalTab === 'single' ? '2px solid var(--color-brand)' : '2px solid transparent',
              color: modalTab === 'single' ? 'var(--color-brand-heading)' : 'var(--color-brand-text)',
              fontWeight: 700,
              fontSize: 12,
              cursor: 'pointer',
              transition: 'all 0.2s ease'
            }}
          >
            Single DNS
          </button>
          <button
            onClick={() => setModalTab('bulk')}
            style={{
              flex: 1,
              padding: '8px 16px',
              background: 'none',
              border: 'none',
              borderBottom: modalTab === 'bulk' ? '2px solid var(--color-brand)' : '2px solid transparent',
              color: modalTab === 'bulk' ? 'var(--color-brand-heading)' : 'var(--color-brand-text)',
              fontWeight: 700,
              fontSize: 12,
              cursor: 'pointer',
              transition: 'all 0.2s ease'
            }}
          >
            Bulk Import
          </button>
        </div>

        {modalTab === 'single' ? (
          <>
            <div style={{ display: 'flex', flexDirection: 'column', gap: 4 }}>
              <label style={{ fontSize: 10, fontWeight: 700, color: 'var(--color-brand-text)' }}>RESOLVER IP/URL</label>
              <input 
                type="text" 
                placeholder="e.g. 1.1.1.1 or 8.8.8.8"
                value={customIP} 
                onChange={(e) => setCustomIP(e.target.value)}
                style={{ padding: '8px 12px', borderRadius: 8, border: '1px solid var(--color-brand-border)', background: 'var(--color-brand-bg)', color: 'var(--color-brand-heading)', fontSize: 12 }}
              />
            </div>

            <div style={{ display: 'flex', flexDirection: 'column', gap: 4 }}>
              <label style={{ fontSize: 10, fontWeight: 700, color: 'var(--color-brand-text)' }}>PROVIDER NAME</label>
              <input 
                type="text" 
                placeholder="e.g. Cloudflare, Google"
                value={customProvider} 
                onChange={(e) => setCustomProvider(e.target.value)}
                style={{ padding: '8px 12px', borderRadius: 8, border: '1px solid var(--color-brand-border)', background: 'var(--color-brand-bg)', color: 'var(--color-brand-heading)', fontSize: 12 }}
              />
            </div>

            <div style={{ display: 'grid', gridTemplateColumns: '1fr 1fr', gap: 12 }}>
              <div style={{ display: 'flex', flexDirection: 'column', gap: 4 }}>
                <label style={{ fontSize: 10, fontWeight: 700, color: 'var(--color-brand-text)' }}>PROTOCOL</label>
                <select
                  value={customProtocol}
                  onChange={(e) => setCustomProtocol(e.target.value)}
                  style={{ padding: '8px 12px', borderRadius: 8, border: '1px solid var(--color-brand-border)', background: 'var(--color-brand-bg)', color: 'var(--color-brand-heading)', fontSize: 12 }}
                >
                  <option value="udp">Plain UDP</option>
                  <option value="tcp">Plain TCP</option>
                  <option value="dot">DoT (TLS)</option>
                  <option value="doh">DoH (HTTPS)</option>
                  <option value="doq">DoQ (QUIC)</option>
                </select>
              </div>

              <div style={{ display: 'flex', flexDirection: 'column', gap: 4 }}>
                <label style={{ fontSize: 10, fontWeight: 700, color: 'var(--color-brand-text)' }}>CATEGORY</label>
                <select
                  value={customCategory}
                  onChange={(e) => setCustomCategory(e.target.value)}
                  style={{ padding: '8px 12px', borderRadius: 8, border: '1px solid var(--color-brand-border)', background: 'var(--color-brand-bg)', color: 'var(--color-brand-heading)', fontSize: 12 }}
                >
                  <option value="custom">Custom (Standard)</option>
                  <option value="public">Public Core</option>
                  <option value="security">Ad/Security filtering</option>
                </select>
              </div>
            </div>

            <div style={{ display: 'flex', gap: 10, marginTop: 10 }}>
              <button 
                onClick={onClose}
                className="btn" 
                style={{ flex: 1, padding: '10px', borderRadius: 8, border: '1px solid var(--color-brand-border)', background: 'var(--color-brand-card)', color: 'var(--color-brand-heading)', cursor: 'pointer', fontSize: 12, fontWeight: 700 }}
              >
                Cancel
              </button>
              <button 
                onClick={handleAddCustomResolver}
                className="btn btn-primary" 
                style={{ flex: 1, padding: '10px', borderRadius: 8, cursor: 'pointer', fontSize: 12, fontWeight: 700 }}
              >
                Save Resolver
              </button>
            </div>
          </>
        ) : (
          <>
            <div style={{ fontSize: 11, color: 'var(--color-brand-text)', lineHeight: 1.4, background: 'var(--color-brand-bg)', padding: '8px 12px', borderRadius: 8, border: '1px solid var(--color-brand-border)' }}>
              <strong>Format:</strong> One per line (e.g. <code>8.8.8.8, Google, udp</code> or <code>1.1.1.1</code>). If protocol is omitted, we will automatically probe and detect it. Duplicate IPs will be skipped.
            </div>

            <div style={{ display: 'flex', flexDirection: 'column', gap: 4 }}>
              <label style={{ fontSize: 10, fontWeight: 700, color: 'var(--color-brand-text)' }}>DNS LIST (PASTE TEXT)</label>
              <textarea
                placeholder="e.g.&#10;8.8.8.8, Google DNS, udp&#10;1.1.1.1, Cloudflare&#10;https://dns.google/dns-query"
                value={bulkText}
                onChange={(e) => {
                  setBulkText(e.target.value);
                  setBulkFile(null);
                }}
                rows={5}
                style={{ padding: '8px 12px', borderRadius: 8, border: '1px solid var(--color-brand-border)', background: 'var(--color-brand-bg)', color: 'var(--color-brand-heading)', fontSize: 12, fontFamily: 'monospace', resize: 'vertical' }}
              />
            </div>

            <div style={{ display: 'flex', flexDirection: 'column', gap: 4 }}>
              <label style={{ fontSize: 10, fontWeight: 700, color: 'var(--color-brand-text)' }}>OR UPLOAD FILE (.TXT / .CSV)</label>
              <input
                type="file"
                accept=".txt,.csv"
                onChange={(e) => {
                  if (e.target.files && e.target.files[0]) {
                    setBulkFile(e.target.files[0]);
                    setBulkText('');
                  }
                }}
                style={{ padding: '6px 10px', borderRadius: 8, border: '1px solid var(--color-brand-border)', background: 'var(--color-brand-bg)', color: 'var(--color-brand-heading)', fontSize: 12 }}
              />
              {bulkFile && (
                <span style={{ fontSize: 11, color: 'var(--color-brand-green)', fontWeight: 600 }}>
                  Selected: {bulkFile.name} ({(bulkFile.size / 1024).toFixed(1)} KB)
                </span>
              )}
            </div>

            <div style={{ display: 'flex', gap: 10, marginTop: 10 }}>
              <button 
                onClick={onClose}
                className="btn" 
                style={{ flex: 1, padding: '10px', borderRadius: 8, border: '1px solid var(--color-brand-border)', background: 'var(--color-brand-card)', color: 'var(--color-brand-heading)', cursor: 'pointer', fontSize: 12, fontWeight: 700 }}
              >
                Cancel
              </button>
              <button 
                onClick={handleBulkImport}
                disabled={isImporting}
                className="btn btn-primary" 
                style={{ flex: 1, padding: '10px', borderRadius: 8, cursor: 'pointer', fontSize: 12, fontWeight: 700, opacity: isImporting ? 0.7 : 1 }}
              >
                {isImporting ? "Submitting..." : "Import Resolvers"}
              </button>
            </div>
          </>
        )}
      </div>
    </div>
  );
};

// 3. Trace Modal
interface TraceModalProps {
  show: boolean;
  onClose: () => void;
  resolver: DNSResolver | undefined;
  traceDomain: string;
  setTraceDomain: (d: string) => void;
  isTracing: boolean;
  traceSteps: any[];
  onRunTrace: (ip: string, domain: string) => void;
}

export const TraceModal: React.FC<TraceModalProps> = ({
  show,
  onClose,
  resolver,
  traceDomain,
  setTraceDomain,
  isTracing,
  traceSteps,
  onRunTrace,
}) => {
  if (!show || !resolver) return null;
  return (
    <div
      style={{
        position: 'fixed',
        top: 0, left: 0, width: '100%', height: '100%',
        background: 'rgba(0,0,0,0.6)',
        display: 'flex', alignItems: 'center', justifyContent: 'center',
        zIndex: 999,
      }}
      onClick={onClose}
    >
      <div
        style={{
          background: 'var(--color-brand-card)',
          padding: 24,
          borderRadius: 12,
          width: 580,
          maxWidth: '95%',
          maxHeight: '85vh',
          overflowY: 'auto',
          boxShadow: '0 10px 25px rgba(0,0,0,0.15)',
          display: 'flex',
          flexDirection: 'column',
          gap: 16
        }}
        onClick={(e) => e.stopPropagation()}
      >
        <div style={{ display: 'flex', justifyContent: 'space-between', alignItems: 'center' }}>
          <h3 style={{ fontSize: 16, fontWeight: 700, color: 'var(--color-brand-heading)', margin: 0 }}>
            Iterative DNS Path Trace Diagnostics
          </h3>
          <span style={{ fontSize: 10, padding: '2px 6px', borderRadius: 4, background: 'var(--color-brand-bg)', color: 'var(--color-brand-text)' }}>
            Target: {resolver.ip}
          </span>
        </div>

        <div style={{ display: 'flex', gap: 10, alignItems: 'flex-end' }}>
          <div style={{ flex: 1, display: 'flex', flexDirection: 'column', gap: 4 }}>
            <label style={{ fontSize: 10, fontWeight: 700, color: 'var(--color-brand-text)' }}>DOMAIN TO TRACE</label>
            <input 
              type="text" 
              value={traceDomain} 
              onChange={(e) => setTraceDomain(e.target.value.trim())}
              placeholder="e.g. google.com"
              style={{ padding: '8px 12px', borderRadius: 8, border: '1px solid var(--color-brand-border)', background: 'var(--color-brand-bg)', color: 'var(--color-brand-heading)', fontSize: 12 }}
            />
          </div>
          <button
            onClick={() => onRunTrace(resolver.ip, traceDomain)}
            disabled={isTracing || !traceDomain}
            className="btn btn-primary"
            style={{ padding: '10px 16px', borderRadius: 8, fontSize: 12, fontWeight: 700, cursor: 'pointer', height: 36, display: 'flex', alignItems: 'center', gap: 6 }}
          >
            {isTracing ? <FiRefreshCw className="animate-spin" /> : <FiPlay />}
            {isTracing ? "Tracing..." : "Run Trace"}
          </button>
        </div>

        <div style={{ border: '1px solid var(--color-brand-border)', borderRadius: 8, background: 'var(--color-brand-bg)', padding: 12, minHeight: 200, maxHeight: 350, overflowY: 'auto', display: 'flex', flexDirection: 'column', gap: 12 }}>
          {traceSteps.length === 0 ? (
            <div style={{ display: 'flex', flexDirection: 'column', alignItems: 'center', justifyContent: 'center', height: '100%', minHeight: 180, color: 'var(--color-brand-muted)', gap: 8, textAlign: 'center' }}>
              <FiActivity size={24} />
              <span style={{ fontSize: 12 }}>
                {isTracing ? "Querying nameservers iteratively starting from Root..." : "Click Run Trace to query recursive paths from root DNS authorities down."}
              </span>
            </div>
          ) : (
            <div style={{ display: 'flex', flexDirection: 'column', gap: 14 }}>
              {traceSteps.map((step, idx) => (
                <div key={idx} style={{ display: 'flex', gap: 12, alignItems: 'flex-start', position: 'relative' }}>
                  {idx < traceSteps.length - 1 && (
                    <div style={{ position: 'absolute', left: 14, top: 28, bottom: -22, width: 2, background: 'var(--color-brand-border)' }} />
                  )}
                  
                  <div style={{ 
                    width: 30, height: 30, borderRadius: '50%', 
                    background: 'var(--color-brand)', color: '#fff', 
                    display: 'flex', alignItems: 'center', justifyContent: 'center', 
                    fontSize: 12, fontWeight: 700, flexShrink: 0 
                  }}>
                    {step.hop}
                  </div>

                  <div style={{ 
                    flex: 1, padding: '10px 14px', borderRadius: 8, 
                    background: 'var(--color-brand-card)', border: '1px solid var(--color-brand-border)'
                  }}>
                    <div style={{ display: 'flex', justifyContent: 'space-between', flexWrap: 'wrap', gap: 8 }}>
                      <span style={{ fontWeight: 700, color: 'var(--color-brand-heading)', fontSize: 12 }}>{step.server_name || step.name}</span>
                      <span style={{ 
                        fontSize: 10, padding: '2px 6px', borderRadius: 4, 
                        background: step.rcode === 'NOERROR' ? 'rgba(34, 197, 94, 0.1)' : 'rgba(239, 68, 68, 0.1)', 
                        color: step.rcode === 'NOERROR' ? '#16a34a' : '#dc2626', 
                        fontWeight: 700 
                      }}>
                        {step.rcode}
                      </span>
                    </div>
                    <div style={{ display: 'flex', gap: 14, fontSize: 11, color: 'var(--color-brand-text)', marginTop: 4 }}>
                      <span>IP: <strong>{step.server_ip || step.ip}</strong></span>
                      <span>Latency: <strong style={{ color: 'var(--color-brand-green)' }}>{Number(step.latency_ms !== undefined ? step.latency_ms : (step.rtt_ms || 0)).toFixed(1)}ms</strong></span>
                    </div>
                    {step.delegated_to && (
                      <div style={{ fontSize: 10, color: 'var(--color-brand-muted)', borderTop: '1px solid var(--color-brand-border)', marginTop: 6, paddingTop: 6 }}>
                        Delegated Zone to: <strong style={{ color: 'var(--color-brand-heading)' }}>{step.delegated_to}</strong>
                      </div>
                    )}
                  </div>
                </div>
              ))}
            </div>
          )}
        </div>

        <div style={{ display: 'flex', justifyContent: 'flex-end', marginTop: 4 }}>
          <button 
            onClick={onClose}
            className="btn" 
            style={{ padding: '8px 16px', borderRadius: 8, border: '1px solid var(--color-brand-border)', background: 'var(--color-brand-card)', color: 'var(--color-brand-heading)', cursor: 'pointer', fontSize: 12, fontWeight: 700 }}
          >
            Close Dialog
          </button>
        </div>
      </div>
    </div>
  );
};

// 4. AXFR Zone Transfer Modal
interface AXFRModalProps {
  show: boolean;
  onClose: () => void;
  resolver: DNSResolver | undefined;
  axfrDomain: string;
  setAxfrDomain: (d: string) => void;
  isTestingAXFR: boolean;
  axfrResult: any;
  onRunAXFR: (ip: string, domain: string) => void;
}

export const AXFRModal: React.FC<AXFRModalProps> = ({
  show,
  onClose,
  resolver,
  axfrDomain,
  setAxfrDomain,
  isTestingAXFR,
  axfrResult,
  onRunAXFR,
}) => {
  if (!show || !resolver) return null;
  return (
    <div
      style={{
        position: 'fixed',
        top: 0, left: 0, width: '100%', height: '100%',
        background: 'rgba(0,0,0,0.6)',
        display: 'flex', alignItems: 'center', justifyContent: 'center',
        zIndex: 999,
      }}
      onClick={onClose}
    >
      <div
        style={{
          background: 'var(--color-brand-card)',
          padding: 24,
          borderRadius: 12,
          width: 580,
          maxWidth: '95%',
          maxHeight: '85vh',
          overflowY: 'auto',
          boxShadow: '0 10px 25px rgba(0,0,0,0.15)',
          display: 'flex',
          flexDirection: 'column',
          gap: 16
        }}
        onClick={(e) => e.stopPropagation()}
      >
        <div style={{ display: 'flex', justifyContent: 'space-between', alignItems: 'center' }}>
          <h3 style={{ fontSize: 16, fontWeight: 700, color: 'var(--color-brand-heading)', margin: 0 }}>
            DNS AXFR Zone Transfer Auditor
          </h3>
          <span style={{ fontSize: 10, padding: '2px 6px', borderRadius: 4, background: 'var(--color-brand-bg)', color: 'var(--color-brand-text)' }}>
            Target: {resolver.ip}
          </span>
        </div>

        <div style={{ display: 'flex', gap: 10, alignItems: 'flex-end' }}>
          <div style={{ flex: 1, display: 'flex', flexDirection: 'column', gap: 4 }}>
            <label style={{ fontSize: 10, fontWeight: 700, color: 'var(--color-brand-text)' }}>DOMAIN ZONE TO TEST</label>
            <input 
              type="text" 
              value={axfrDomain} 
              onChange={(e) => setAxfrDomain(e.target.value.trim())}
              placeholder="e.g. zonetransfer.me"
              style={{ padding: '8px 12px', borderRadius: 8, border: '1px solid var(--color-brand-border)', background: 'var(--color-brand-bg)', color: 'var(--color-brand-heading)', fontSize: 12 }}
            />
          </div>
          <button
            onClick={() => onRunAXFR(resolver.ip, axfrDomain)}
            disabled={isTestingAXFR || !axfrDomain}
            className="btn btn-primary"
            style={{ padding: '10px 16px', borderRadius: 8, fontSize: 12, fontWeight: 700, cursor: 'pointer', height: 36, display: 'flex', alignItems: 'center', gap: 6 }}
          >
            {isTestingAXFR ? <FiRefreshCw className="animate-spin" /> : <FiPlay />}
            {isTestingAXFR ? "Auditing..." : "Audit Zone"}
          </button>
        </div>

        <div style={{ minHeight: 200, maxHeight: 350, overflowY: 'auto' }}>
          {isTestingAXFR ? (
            <div style={{ display: 'flex', flexDirection: 'column', alignItems: 'center', justifyContent: 'center', height: '100%', minHeight: 180, color: 'var(--color-brand-muted)', gap: 8 }}>
              <FiRefreshCw className="animate-spin" size={24} />
              <span style={{ fontSize: 12 }}>Contacting nameservers and requesting full zone transfer (AXFR)...</span>
            </div>
          ) : axfrResult ? (
            <div style={{ display: 'flex', flexDirection: 'column', gap: 12 }}>
              <div style={{ 
                padding: 14, borderRadius: 8, 
                border: '1px solid ' + (axfrResult.allowed ? '#ef4444' : '#22c55e'),
                background: axfrResult.allowed ? 'rgba(239, 68, 68, 0.08)' : 'rgba(34, 197, 94, 0.08)',
                color: axfrResult.allowed ? '#ef4444' : '#22c55e',
                display: 'flex', alignItems: 'center', gap: 10
              }}>
                <FiInfo size={18} />
                <div>
                  <div style={{ fontWeight: 700, fontSize: 13 }}>
                    {axfrResult.allowed ? "VULNERABILITY DETECTED: AXFR Zone Transfer Allowed!" : "SECURE: Zone Transfer Request Rejected"}
                  </div>
                  <div style={{ fontSize: 11, opacity: 0.85, marginTop: 2 }}>
                    {axfrResult.allowed 
                      ? `Leaked ${axfrResult.records_count} resource records from the nameserver.` 
                      : `The resolver or authoritative server refused the replication request.`
                    }
                  </div>
                </div>
              </div>

              {axfrResult.message && (
                <div style={{ fontSize: 11, padding: 10, borderRadius: 6, background: 'var(--color-brand-bg)', border: '1px solid var(--color-brand-border)', color: 'var(--color-brand-text)', fontStyle: 'italic' }}>
                  Server Response: {axfrResult.message}
                </div>
              )}

              {axfrResult.allowed && axfrResult.records && axfrResult.records.length > 0 && (
                <div style={{ display: 'flex', flexDirection: 'column', gap: 4 }}>
                  <span style={{ fontSize: 10, fontWeight: 700, color: 'var(--color-brand-text)' }}>LEAKED RESOURCE RECORDS:</span>
                  <div style={{ 
                    fontFamily: 'monospace', fontSize: 11, padding: 12, borderRadius: 8, 
                    background: 'var(--color-brand-bg)', border: '1px solid var(--color-brand-border)', 
                    color: 'var(--color-brand-heading)', maxHeight: 200, overflowY: 'auto', whiteSpace: 'pre-wrap'
                  }}>
                    {axfrResult.records.join('\n')}
                  </div>
                </div>
              )}
            </div>
          ) : (
            <div style={{ display: 'flex', flexDirection: 'column', alignItems: 'center', justifyContent: 'center', height: '100%', minHeight: 180, color: 'var(--color-brand-muted)', gap: 8, textAlign: 'center' }}>
              <FiDatabase size={24} />
              <span style={{ fontSize: 12 }}>
                Click Audit Zone to audit zone transfer settings on this resolver.
              </span>
            </div>
          )}
        </div>

        <div style={{ display: 'flex', justifyContent: 'flex-end', marginTop: 4 }}>
          <button 
            onClick={onClose}
            className="btn" 
            style={{ padding: '8px 16px', borderRadius: 8, border: '1px solid var(--color-brand-border)', background: 'var(--color-brand-card)', color: 'var(--color-brand-heading)', cursor: 'pointer', fontSize: 12, fontWeight: 700 }}
          >
            Close Dialog
          </button>
        </div>
      </div>
    </div>
  );
};

// 5. Advanced Single DNS Test Modal
interface AdvancedTestModalProps {
  show: boolean;
  onClose: () => void;
  resolver: DNSResolver | undefined;
  advDomain: string;
  setAdvDomain: (d: string) => void;
  advQueryType: string;
  setAdvQueryType: (t: string) => void;
  advDNSClass: string;
  setAdvDNSClass: (c: string) => void;
  advTimeout: number;
  setAdvTimeout: (t: number) => void;
  advAttempts: number;
  setAdvAttempts: (a: number) => void;
  advExpectResponse: string;
  setAdvExpectResponse: (r: string) => void;
  advCacheBusting: boolean;
  setAdvCacheBusting: (b: boolean) => void;
  onExecute: () => void;
  isTestingSingle: boolean;
  singleTestError: string | null;
  singleTestResult: any;
}

export const AdvancedTestModal: React.FC<AdvancedTestModalProps> = ({
  show,
  onClose,
  resolver,
  advDomain,
  setAdvDomain,
  advQueryType,
  setAdvQueryType,
  advDNSClass,
  setAdvDNSClass,
  advTimeout,
  setAdvTimeout,
  advAttempts,
  setAdvAttempts,
  advExpectResponse,
  setAdvExpectResponse,
  advCacheBusting,
  setAdvCacheBusting,
  onExecute,
  isTestingSingle,
  singleTestError,
  singleTestResult,
}) => {
  if (!show || !resolver) return null;
  return (
    <div
      style={{
        position: 'fixed',
        top: 0, left: 0, width: '100%', height: '100%',
        background: 'rgba(0,0,0,0.6)',
        display: 'flex', alignItems: 'center', justifyContent: 'center',
        zIndex: 999,
      }}
      onClick={onClose}
    >
      <div
        style={{
          background: 'var(--color-brand-card)',
          padding: 24,
          borderRadius: 12,
          width: 720,
          maxWidth: '95%',
          maxHeight: '90vh',
          overflowY: 'auto',
          boxShadow: '0 10px 25px rgba(0,0,0,0.15)',
          display: 'flex',
          flexDirection: 'column',
          gap: 16
        }}
        onClick={(e) => e.stopPropagation()}
      >
        <div style={{ display: 'flex', justifyContent: 'space-between', alignItems: 'center', borderBottom: '1px solid var(--color-brand-border)', paddingBottom: 12 }}>
          <div>
            <h3 style={{ fontSize: 16, fontWeight: 700, color: 'var(--color-brand-heading)', margin: 0 }}>
              Advanced Single DNS Resolver Test
            </h3>
            <div style={{ fontSize: 11, color: 'var(--color-brand-muted)', marginTop: 4 }}>
              Target Resolver: <strong style={{ color: 'var(--color-brand-heading)' }}>{resolver.ip}</strong> ({resolver.protocol.toUpperCase()})
            </div>
          </div>
          <button 
            onClick={onClose}
            style={{ background: 'none', border: 'none', color: 'var(--color-brand-muted)', cursor: 'pointer' }}
          >
            <FiX size={18} />
          </button>
        </div>

        <div style={{ display: 'grid', gridTemplateColumns: 'repeat(auto-fit, minmax(180px, 1fr))', gap: 12, background: 'var(--color-brand-bg)', padding: 14, borderRadius: 8, border: '1px solid var(--color-brand-border)' }}>
          <div style={{ display: 'flex', flexDirection: 'column', gap: 4 }}>
            <label style={{ fontSize: 10, fontWeight: 700, color: 'var(--color-brand-text)' }}>QUERY DOMAIN</label>
            <input 
              type="text" 
              value={advDomain}
              onChange={(e) => setAdvDomain(e.target.value)}
              style={{ padding: '6px 10px', borderRadius: 6, border: '1px solid var(--color-brand-border)', background: 'var(--color-brand-card)', color: 'var(--color-brand-heading)', fontSize: 11 }}
            />
          </div>

          <div style={{ display: 'flex', flexDirection: 'column', gap: 4 }}>
            <label style={{ fontSize: 10, fontWeight: 700, color: 'var(--color-brand-text)' }}>QUERY TYPE</label>
            <select 
              value={advQueryType}
              onChange={(e) => setAdvQueryType(e.target.value)}
              style={{ padding: '6px 10px', borderRadius: 6, border: '1px solid var(--color-brand-border)', background: 'var(--color-brand-card)', color: 'var(--color-brand-heading)', fontSize: 11 }}
            >
              {['A', 'AAAA', 'MX', 'TXT', 'NS', 'CNAME', 'SOA', 'SRV', 'CAA'].map(t => (
                <option key={t} value={t}>{t}</option>
              ))}
            </select>
          </div>

          <div style={{ display: 'flex', flexDirection: 'column', gap: 4 }}>
            <label style={{ fontSize: 10, fontWeight: 700, color: 'var(--color-brand-text)' }}>DNS CLASS</label>
            <select 
              value={advDNSClass}
              onChange={(e) => setAdvDNSClass(e.target.value)}
              style={{ padding: '6px 10px', borderRadius: 6, border: '1px solid var(--color-brand-border)', background: 'var(--color-brand-card)', color: 'var(--color-brand-heading)', fontSize: 11 }}
            >
              {['IN', 'CH', 'HS'].map(c => (
                <option key={c} value={c}>{c}</option>
              ))}
            </select>
          </div>

          <div style={{ display: 'flex', flexDirection: 'column', gap: 4 }}>
            <label style={{ fontSize: 10, fontWeight: 700, color: 'var(--color-brand-text)' }}>TIMEOUT (MS)</label>
            <input 
              type="number" 
              value={advTimeout}
              onChange={(e) => setAdvTimeout(Number(e.target.value))}
              style={{ padding: '6px 10px', borderRadius: 6, border: '1px solid var(--color-brand-border)', background: 'var(--color-brand-card)', color: 'var(--color-brand-heading)', fontSize: 11 }}
            />
          </div>

          <div style={{ display: 'flex', flexDirection: 'column', gap: 4 }}>
            <label style={{ fontSize: 10, fontWeight: 700, color: 'var(--color-brand-text)' }}>ATTEMPTS</label>
            <input 
              type="number" 
              value={advAttempts}
              onChange={(e) => setAdvAttempts(Number(e.target.value))}
              style={{ padding: '6px 10px', borderRadius: 6, border: '1px solid var(--color-brand-border)', background: 'var(--color-brand-card)', color: 'var(--color-brand-heading)', fontSize: 11 }}
            />
          </div>

          <div style={{ display: 'flex', flexDirection: 'column', gap: 4 }}>
            <label style={{ fontSize: 10, fontWeight: 700, color: 'var(--color-brand-text)' }}>EXPECT RESPONSE (OPTIONAL)</label>
            <input 
              type="text" 
              value={advExpectResponse}
              onChange={(e) => setAdvExpectResponse(e.target.value)}
              placeholder="e.g. 1.1.1.1 or cloudflare"
              style={{ padding: '6px 10px', borderRadius: 6, border: '1px solid var(--color-brand-border)', background: 'var(--color-brand-card)', color: 'var(--color-brand-heading)', fontSize: 11 }}
            />
          </div>

          <div style={{ display: 'flex', alignItems: 'center', gap: 8, gridColumn: '1 / -1', marginTop: 4 }}>
            <input 
              type="checkbox" 
              id="advCacheBusting"
              checked={advCacheBusting}
              onChange={(e) => setAdvCacheBusting(e.target.checked)}
              style={{ accentColor: 'var(--color-brand)', cursor: 'pointer' }}
            />
            <label htmlFor="advCacheBusting" style={{ fontSize: 11, fontWeight: 600, color: 'var(--color-brand-heading)', cursor: 'pointer' }}>
              Enable Cache Busting (adds a random sub-domain prefix to bypass middlebox caching)
            </label>
          </div>
        </div>

        <div style={{ display: 'flex', justifyContent: 'flex-end' }}>
          <button
            onClick={onExecute}
            disabled={isTestingSingle}
            className="btn btn-primary"
            style={{ padding: '8px 16px', borderRadius: 8, fontSize: 12, fontWeight: 700, cursor: 'pointer', display: 'flex', alignItems: 'center', gap: 6 }}
          >
            {isTestingSingle ? <FiRefreshCw className="animate-spin" /> : <FiPlay />}
            {isTestingSingle ? "Running Test Query..." : "Execute Test"}
          </button>
        </div>

        <div style={{ display: 'flex', flexDirection: 'column', gap: 12 }}>
          {isTestingSingle ? (
            <div style={{ display: 'flex', flexDirection: 'column', alignItems: 'center', justifyContent: 'center', minHeight: 200, border: '1px dashed var(--color-brand-border)', borderRadius: 8, color: 'var(--color-brand-muted)', gap: 8 }}>
              <FiRefreshCw className="animate-spin" size={24} />
              <span style={{ fontSize: 12 }}>Sending query pack and listening for response...</span>
            </div>
          ) : singleTestError ? (
            <div style={{ display: 'flex', gap: 10, padding: 12, borderRadius: 8, background: 'rgba(239, 68, 68, 0.08)', border: '1px solid #ef4444', color: '#ef4444', alignItems: 'center', fontSize: 12 }}>
              <FiAlertTriangle size={16} />
              <span>{singleTestError}</span>
            </div>
          ) : singleTestResult ? (
            <div style={{ display: 'flex', flexDirection: 'column', gap: 12 }}>
              <div style={{ display: 'grid', gridTemplateColumns: 'repeat(auto-fit, minmax(130px, 1fr))', gap: 10 }}>
                <div style={{ background: 'var(--color-brand-bg)', border: '1px solid var(--color-brand-border)', padding: '10px 14px', borderRadius: 8, textAlign: 'center' }}>
                  <div style={{ fontSize: 9, color: 'var(--color-brand-muted)', fontWeight: 700 }}>CLEVER SCORE</div>
                  <div style={{ fontSize: 20, fontWeight: 800, color: 'var(--color-brand)', marginTop: 4 }}>{singleTestResult.clever_score}/100</div>
                </div>
                <div style={{ background: 'var(--color-brand-bg)', border: '1px solid var(--color-brand-border)', padding: '10px 14px', borderRadius: 8, textAlign: 'center' }}>
                  <div style={{ fontSize: 9, color: 'var(--color-brand-muted)', fontWeight: 700 }}>RTT LATENCY</div>
                  <div style={{ fontSize: 20, fontWeight: 800, color: 'var(--color-brand-heading)', marginTop: 4 }}>{singleTestResult.latency_ms.toFixed(1)} ms</div>
                </div>
                <div style={{ background: 'var(--color-brand-bg)', border: '1px solid var(--color-brand-border)', padding: '10px 14px', borderRadius: 8, textAlign: 'center' }}>
                  <div style={{ fontSize: 9, color: 'var(--color-brand-muted)', fontWeight: 700 }}>SUCCESS RATE</div>
                  <div style={{ fontSize: 20, fontWeight: 800, color: singleTestResult.success_rate > 50 ? 'var(--color-brand-green)' : 'var(--color-brand-red)', marginTop: 4 }}>{singleTestResult.success_rate.toFixed(0)}%</div>
                </div>
                <div style={{ background: 'var(--color-brand-bg)', border: '1px solid var(--color-brand-border)', padding: '10px 14px', borderRadius: 8, textAlign: 'center' }}>
                  <div style={{ fontSize: 9, color: 'var(--color-brand-muted)', fontWeight: 700 }}>DNSSEC</div>
                  <div style={{ fontSize: 20, fontWeight: 800, color: singleTestResult.dnssec ? 'var(--color-brand-green)' : 'var(--color-brand-muted)', marginTop: 4 }}>
                    {singleTestResult.dnssec ? "SUPPORTED" : "NO"}
                  </div>
                </div>
              </div>

              {singleTestResult.resolved_ip && (
                <div style={{ display: 'flex', flexDirection: 'column', gap: 6, background: 'var(--color-brand-bg)', border: '1px solid var(--color-brand-border)', padding: 12, borderRadius: 8 }}>
                  <span style={{ fontSize: 10, fontWeight: 700, color: 'var(--color-brand-text)' }}>RESOLVED IP GEOLOCATION INTELLIGENCE</span>
                  <div style={{ display: 'flex', flexWrap: 'wrap', gap: 14, fontSize: 11, color: 'var(--color-brand-heading)' }}>
                    <span>IP: <strong>{singleTestResult.resolved_ip}</strong></span>
                    {singleTestResult.geoip?.country_name && (
                      <span style={{ display: 'inline-flex', alignItems: 'center', gap: 4 }}>
                        Country: <strong>{singleTestResult.geoip.country_name}</strong>
                      </span>
                    )}
                    {singleTestResult.geoip?.city && (
                      <span>City: <strong>{singleTestResult.geoip.city}</strong></span>
                    )}
                    {singleTestResult.geoip?.isp && (
                      <span>ISP: <strong>{singleTestResult.geoip.isp}</strong></span>
                    )}
                    {singleTestResult.geoip?.is_cdn && (
                      <span style={{ background: 'rgba(249, 115, 22, 0.1)', color: '#ea580c', padding: '1px 4px', borderRadius: 3, fontWeight: 700 }}>
                        CDN: {singleTestResult.geoip.cdn_provider}
                      </span>
                    )}
                    {singleTestResult.rebinding && (
                      <span style={{ background: 'rgba(239, 68, 68, 0.1)', color: '#dc2626', padding: '1px 4px', borderRadius: 3, fontWeight: 700 }}>
                        DNS Rebinding Vulnerability Detected!
                      </span>
                    )}
                  </div>
                </div>
              )}

              {singleTestResult.answers && singleTestResult.answers.length > 0 && (
                <div style={{ display: 'flex', flexDirection: 'column', gap: 4 }}>
                  <span style={{ fontSize: 10, fontWeight: 700, color: 'var(--color-brand-text)' }}>ANSWER RECORDS ({singleTestResult.answers.length})</span>
                  <div style={{ maxHeight: 180, overflowY: 'auto', border: '1px solid var(--color-brand-border)', borderRadius: 6 }}>
                    <table style={{ width: '100%', borderCollapse: 'collapse', fontSize: 11 }}>
                      <thead>
                        <tr style={{ background: 'var(--color-brand-bg)', borderBottom: '1px solid var(--color-brand-border)', textAlign: 'left' }}>
                          <th style={{ padding: '6px 8px' }}>Name</th>
                          <th style={{ padding: '6px 8px' }}>Type</th>
                          <th style={{ padding: '6px 8px' }}>TTL</th>
                          <th style={{ padding: '6px 8px' }}>Data</th>
                        </tr>
                      </thead>
                      <tbody>
                        {singleTestResult.answers.map((ans: any, idx: number) => (
                          <tr key={idx} style={{ borderBottom: '1px solid var(--color-brand-border)' }}>
                            <td style={{ padding: '6px 8px', color: 'var(--color-brand-text)' }}>{ans.name}</td>
                            <td style={{ padding: '6px 8px', fontWeight: 700, color: 'var(--color-brand-heading)' }}>{ans.type}</td>
                            <td style={{ padding: '6px 8px', color: 'var(--color-brand-muted)' }}>{ans.ttl}</td>
                            <td style={{ padding: '6px 8px', color: 'var(--color-brand-heading)', fontFamily: 'monospace' }}>{ans.data}</td>
                          </tr>
                        ))}
                      </tbody>
                    </table>
                  </div>
                </div>
              )}

              {singleTestResult.raw_response && (
                <div style={{ display: 'flex', flexDirection: 'column', gap: 4 }}>
                  <span style={{ fontSize: 10, fontWeight: 700, color: 'var(--color-brand-text)' }}>RAW DNS EXCHANGE MESSAGE (MIEKGDNS PACKET FORMAT)</span>
                  <pre style={{ 
                    margin: 0, padding: 12, borderRadius: 6, 
                    background: 'var(--color-brand-bg)', border: '1px solid var(--color-brand-border)', 
                    color: 'var(--color-brand-heading)', fontSize: 10, fontFamily: 'monospace',
                    maxHeight: 180, overflowY: 'auto', whiteSpace: 'pre-wrap'
                  }}>
                    {singleTestResult.raw_response}
                  </pre>
                </div>
              )}
            </div>
          ) : (
            <div style={{ display: 'flex', flexDirection: 'column', alignItems: 'center', justifyContent: 'center', minHeight: 180, color: 'var(--color-brand-muted)', gap: 8, textAlign: 'center', border: '1px dashed var(--color-brand-border)', borderRadius: 8 }}>
              <FiActivity size={24} />
              <span style={{ fontSize: 12 }}>
                Click Execute Test to send custom query payloads to the resolver.
              </span>
            </div>
          )}
        </div>

        <div style={{ display: 'flex', justifyContent: 'flex-end', borderTop: '1px solid var(--color-brand-border)', paddingTop: 12, marginTop: 4 }}>
          <button 
            onClick={onClose}
            className="btn" 
            style={{ padding: '8px 16px', borderRadius: 8, border: '1px solid var(--color-brand-border)', background: 'var(--color-brand-card)', color: 'var(--color-brand-heading)', cursor: 'pointer', fontSize: 12, fontWeight: 700 }}
          >
            Close Panel
          </button>
        </div>
      </div>
    </div>
  );
};
