import React from 'react';

interface SettingsAccordionProps {
  showSettings: boolean;
  concurrencyLimit: number;
  setConcurrencyLimit: (val: number) => void;
  timeoutMs: number;
  setTimeoutMs: (val: number) => void;
  attempts: number;
  setAttempts: (val: number) => void;
  referenceDomain: string;
  setReferenceDomain: (val: string) => void;
  dnsClass: string;
  setDnsClass: (val: string) => void;
  queryGenerator: string;
  setQueryGenerator: (val: string) => void;
  domainSource: string;
  setDomainSource: (val: string) => void;
  customDomains: string;
  setCustomDomains: (val: string) => void;
  wordlistURL: string;
  setWordlistURL: (val: string) => void;
  queryTypes: string[];
  setQueryTypes: (types: string[]) => void;
  selectedProtocols: string[];
  onProtocolCheckbox: (proto: string) => void;
  expectResponse: string;
  setExpectResponse: (val: string) => void;
}

export const SettingsAccordion: React.FC<SettingsAccordionProps> = ({
  showSettings,
  concurrencyLimit,
  setConcurrencyLimit,
  timeoutMs,
  setTimeoutMs,
  attempts,
  setAttempts,
  referenceDomain,
  setReferenceDomain,
  dnsClass,
  setDnsClass,
  queryGenerator,
  setQueryGenerator,
  domainSource,
  setDomainSource,
  customDomains,
  setCustomDomains,
  wordlistURL,
  setWordlistURL,
  queryTypes,
  setQueryTypes,
  selectedProtocols,
  onProtocolCheckbox,
  expectResponse,
  setExpectResponse,
}) => {
  if (!showSettings) return null;

  return (
    <div className="animate-fade-in" style={{
      background: 'var(--color-brand-bg)',
      border: '1px solid var(--color-brand-border)',
      borderRadius: 8,
      padding: 16,
      display: 'grid',
      gridTemplateColumns: 'repeat(auto-fit, minmax(200px, 1fr))',
      gap: 16,
      marginTop: 10
    }}>
      <div style={{ display: 'flex', flexDirection: 'column', gap: 4 }}>
        <label style={{ fontSize: 10, fontWeight: 700, color: 'var(--color-brand-text)' }}>CONCURRENCY LIMIT</label>
        <input 
          type="number" 
          value={concurrencyLimit} 
          onChange={(e) => setConcurrencyLimit(Math.max(1, parseInt(e.target.value) || 1))}
          style={{ padding: '6px 10px', borderRadius: 6, border: '1px solid var(--color-brand-border)', background: 'var(--color-brand-card)', color: 'var(--color-brand-heading)', fontSize: 12 }}
        />
      </div>
      <div style={{ display: 'flex', flexDirection: 'column', gap: 4 }}>
        <label style={{ fontSize: 10, fontWeight: 700, color: 'var(--color-brand-text)' }}>TIMEOUT (MS)</label>
        <input 
          type="number" 
          value={timeoutMs} 
          onChange={(e) => setTimeoutMs(Math.max(100, parseInt(e.target.value) || 100))}
          style={{ padding: '6px 10px', borderRadius: 6, border: '1px solid var(--color-brand-border)', background: 'var(--color-brand-card)', color: 'var(--color-brand-heading)', fontSize: 12 }}
        />
      </div>
      <div style={{ display: 'flex', flexDirection: 'column', gap: 4 }}>
        <label style={{ fontSize: 10, fontWeight: 700, color: 'var(--color-brand-text)' }}>TEST ATTEMPTS</label>
        <input 
          type="number" 
          value={attempts} 
          onChange={(e) => setAttempts(Math.max(1, parseInt(e.target.value) || 1))}
          style={{ padding: '6px 10px', borderRadius: 6, border: '1px solid var(--color-brand-border)', background: 'var(--color-brand-card)', color: 'var(--color-brand-heading)', fontSize: 12 }}
        />
      </div>
      <div style={{ display: 'flex', flexDirection: 'column', gap: 4 }}>
        <label style={{ fontSize: 10, fontWeight: 700, color: 'var(--color-brand-text)' }}>REFERENCE DOMAIN</label>
        <input 
          type="text" 
          value={referenceDomain} 
          onChange={(e) => setReferenceDomain(e.target.value.trim() || 'google.com')}
          style={{ padding: '6px 10px', borderRadius: 6, border: '1px solid var(--color-brand-border)', background: 'var(--color-brand-card)', color: 'var(--color-brand-heading)', fontSize: 12 }}
        />
      </div>

      <div style={{ display: 'flex', flexDirection: 'column', gap: 4 }}>
        <label style={{ fontSize: 10, fontWeight: 700, color: 'var(--color-brand-text)' }}>DNS CLASS</label>
        <select
          value={dnsClass}
          onChange={(e) => setDnsClass(e.target.value)}
          style={{ padding: '6px 10px', borderRadius: 6, border: '1px solid var(--color-brand-border)', background: 'var(--color-brand-card)', color: 'var(--color-brand-heading)', fontSize: 12 }}
        >
          <option value="IN">Internet (IN)</option>
          <option value="CH">Chaos (CH)</option>
          <option value="ANY">Any (ANY)</option>
        </select>
      </div>

      <div style={{ display: 'flex', flexDirection: 'column', gap: 4 }}>
        <label style={{ fontSize: 10, fontWeight: 700, color: 'var(--color-brand-text)' }}>QUERY STRATEGY</label>
        <select
          value={queryGenerator}
          onChange={(e) => setQueryGenerator(e.target.value)}
          style={{ padding: '6px 10px', borderRadius: 6, border: '1px solid var(--color-brand-border)', background: 'var(--color-brand-card)', color: 'var(--color-brand-heading)', fontSize: 12 }}
        >
          <option value="random">Random Subdomain</option>
          <option value="sequential">Sequential Sequence</option>
          <option value="static">Static Reference Domain</option>
        </select>
      </div>

      <div style={{ display: 'flex', flexDirection: 'column', gap: 4 }}>
        <label style={{ fontSize: 10, fontWeight: 700, color: 'var(--color-brand-text)' }}>DOMAIN WORDLIST SOURCE</label>
        <select
          value={domainSource}
          onChange={(e) => setDomainSource(e.target.value)}
          style={{ padding: '6px 10px', borderRadius: 6, border: '1px solid var(--color-brand-border)', background: 'var(--color-brand-card)', color: 'var(--color-brand-heading)', fontSize: 12 }}
        >
          <option value="default">Default Reference Domain</option>
          <option value="custom">Custom Domains List</option>
          <option value="url">Remote Wordlist URL</option>
        </select>
      </div>

      {domainSource === 'custom' && (
        <div style={{ gridColumn: '1 / -1', display: 'flex', flexDirection: 'column', gap: 4, borderTop: '1px solid var(--color-brand-border)', paddingTop: 14 }}>
          <label style={{ fontSize: 10, fontWeight: 700, color: 'var(--color-brand-text)' }}>CUSTOM TARGET DOMAINS (ONE PER LINE OR COMMA-SEPARATED)</label>
          <textarea
            value={customDomains}
            onChange={(e) => setCustomDomains(e.target.value)}
            placeholder="e.g.&#10;google.com&#10;apple.com&#10;github.com"
            rows={3}
            style={{ padding: '8px 12px', borderRadius: 6, border: '1px solid var(--color-brand-border)', background: 'var(--color-brand-card)', color: 'var(--color-brand-heading)', fontSize: 12, fontFamily: 'monospace', resize: 'vertical' }}
          />
        </div>
      )}

      {domainSource === 'url' && (
        <div style={{ gridColumn: '1 / -1', display: 'flex', flexDirection: 'column', gap: 4, borderTop: '1px solid var(--color-brand-border)', paddingTop: 14 }}>
          <label style={{ fontSize: 10, fontWeight: 700, color: 'var(--color-brand-text)' }}>REMOTE WORDLIST URL</label>
          <input
            type="url"
            value={wordlistURL}
            onChange={(e) => setWordlistURL(e.target.value)}
            placeholder="https://example.com/domains.txt"
            style={{ padding: '8px 12px', borderRadius: 6, border: '1px solid var(--color-brand-border)', background: 'var(--color-brand-card)', color: 'var(--color-brand-heading)', fontSize: 12 }}
          />
        </div>
      )}

      <div style={{ gridColumn: '1 / -1', display: 'flex', flexDirection: 'column', gap: 8, borderTop: '1px solid var(--color-brand-border)', paddingTop: 14 }}>
        <span style={{ fontSize: 10, fontWeight: 700, color: 'var(--color-brand-text)' }}>QUERY RECORD TYPES</span>
        <div style={{ display: 'flex', flexWrap: 'wrap', gap: 8 }}>
          {['A', 'AAAA', 'MX', 'TXT', 'CNAME', 'NS', 'SOA', 'PTR', 'HTTPS'].map(t => {
            const active = queryTypes.includes(t);
            return (
              <button
                key={t}
                type="button"
                onClick={() => {
                  if (active) {
                    if (queryTypes.length > 1) {
                      setQueryTypes(queryTypes.filter(x => x !== t));
                    }
                  } else {
                    setQueryTypes([...queryTypes, t]);
                  }
                }}
                style={{
                  padding: '4px 10px',
                  borderRadius: 6,
                  border: '1px solid var(--color-brand-border)',
                  background: active ? 'var(--color-brand-light)' : 'var(--color-brand-card)',
                  color: active ? 'var(--color-brand)' : 'var(--color-brand-text)',
                  fontSize: 10,
                  fontWeight: 700,
                  cursor: 'pointer',
                  transition: 'all 0.15s ease'
                }}
              >
                {t}
              </button>
            );
          })}
        </div>
      </div>

      <div style={{ gridColumn: '1 / -1', display: 'flex', flexDirection: 'column', gap: 8, borderTop: '1px solid var(--color-brand-border)', paddingTop: 14 }}>
        <span style={{ fontSize: 10, fontWeight: 700, color: 'var(--color-brand-text)' }}>SELECT DIAGNOSTIC PROTOCOLS</span>
        <div style={{ display: 'flex', gap: 16 }}>
          {['udp', 'tcp', 'dot', 'doh', 'doq'].map(p => (
            <label key={p} style={{ display: 'flex', alignItems: 'center', gap: 6, fontSize: 12, fontWeight: 700, color: 'var(--color-brand-heading)', textTransform: 'uppercase', cursor: 'pointer' }}>
              <input
                type="checkbox"
                checked={selectedProtocols.includes(p)}
                onChange={() => onProtocolCheckbox(p)}
                style={{ accentColor: 'var(--color-brand)', cursor: 'pointer' }}
              />
              {p}
            </label>
          ))}
        </div>
      </div>

      <div style={{ gridColumn: '1 / -1', display: 'flex', flexDirection: 'column', gap: 4, borderTop: '1px solid var(--color-brand-border)', paddingTop: 14 }}>
        <label style={{ fontSize: 10, fontWeight: 700, color: 'var(--color-brand-text)' }}>EXPECTED RESPONSE SUBSTRING (OPTIONAL - FOR DETECTING CENSORSHIP/DNS SPOOFING)</label>
        <input 
          type="text" 
          id="expect_response"
          value={expectResponse} 
          onChange={(e) => setExpectResponse(e.target.value)}
          placeholder="e.g. 142.250 or google.com (Leave empty to skip substring match checks)"
          style={{ padding: '8px 12px', borderRadius: 6, border: '1px solid var(--color-brand-border)', background: 'var(--color-brand-card)', color: 'var(--color-brand-heading)', fontSize: 12 }}
        />
      </div>
    </div>
  );
};
