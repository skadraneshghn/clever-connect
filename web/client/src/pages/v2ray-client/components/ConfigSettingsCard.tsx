import React from 'react';
import { FiSliders, FiHelpCircle, FiLock, FiSave } from 'react-icons/fi';

interface ConfigSettingsCardProps {
  isLoading: boolean;
  selectedCore: string;
  setSelectedCore: (core: string) => void;
  socksPort: number;
  setSocksPort: (port: number) => void;
  httpPort: number;
  setHttpPort: (port: number) => void;
  dnsServer: string;
  setDnsServer: (dns: string) => void;
  routingPreset: string;
  setRoutingPreset: (preset: string) => void;
  customRouting: string;
  setCustomRouting: (routing: string) => void;
  evasionFingerprint: string;
  setEvasionFingerprint: (fingerprint: string) => void;
  evasionFragment: boolean;
  setEvasionFragment: (fragment: boolean) => void;
  fragmentMode: string;
  setFragmentMode: (mode: string) => void;
  fragmentLength: string;
  setFragmentLength: (len: string) => void;
  fragmentInterval: string;
  setFragmentInterval: (interval: string) => void;
  evasionEch: boolean;
  setEvasionEch: (ech: boolean) => void;
  evasionEchConfig: string;
  setEvasionEchConfig: (config: string) => void;
  evasionTcpBrutal: boolean;
  setEvasionTcpBrutal: (brutal: boolean) => void;
  muxEnabled: boolean;
  setMuxEnabled: (mux: boolean) => void;
  handleSaveSettings: (e: React.FormEvent) => void;
  showHelp: (title: string, text: string) => void;
}

export const ConfigSettingsCard: React.FC<ConfigSettingsCardProps> = ({
  isLoading,
  selectedCore,
  setSelectedCore,
  socksPort,
  setSocksPort,
  httpPort,
  setHttpPort,
  dnsServer,
  setDnsServer,
  routingPreset,
  setRoutingPreset,
  customRouting,
  setCustomRouting,
  evasionFingerprint,
  setEvasionFingerprint,
  evasionFragment,
  setEvasionFragment,
  fragmentMode,
  setFragmentMode,
  fragmentLength,
  setFragmentLength,
  fragmentInterval,
  setFragmentInterval,
  evasionEch,
  setEvasionEch,
  evasionEchConfig,
  setEvasionEchConfig,
  evasionTcpBrutal,
  setEvasionTcpBrutal,
  muxEnabled,
  setMuxEnabled,
  handleSaveSettings,
  showHelp,
}) => {
  return (
    <form onSubmit={handleSaveSettings} className="g-card" style={{ display: 'flex', flexDirection: 'column', gap: 18 }}>
      <div style={{ display: 'flex', alignItems: 'center', gap: 8 }}>
        <FiSliders style={{ color: 'var(--color-brand)', fontSize: 18 }} />
        <span style={{ fontSize: 15, fontWeight: 600, color: 'var(--color-brand-heading)' }}>Proxy Configurations</span>
        <FiHelpCircle
          style={{ cursor: 'pointer', color: 'var(--color-brand-muted)' }}
          onClick={() =>
            showHelp(
              'Proxy Configurations',
              'Configure inbound local ports for network application mapping. Set custom DNS resolving servers to avoid ISP redirection leaks.'
            )
          }
        />
      </div>

      <div style={{ display: 'grid', gridTemplateColumns: '1fr', gap: 16 }}>
        <div>
          <label
            style={{
              display: 'block',
              fontSize: 11,
              fontWeight: 600,
              color: 'var(--color-brand-muted)',
              marginBottom: 6,
              textTransform: 'uppercase',
            }}
          >
            Default Core
          </label>
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
              color: 'var(--color-brand-heading)',
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
          <label
            style={{
              display: 'block',
              fontSize: 11,
              fontWeight: 600,
              color: 'var(--color-brand-muted)',
              marginBottom: 6,
              textTransform: 'uppercase',
            }}
          >
            SOCKS5 Port
          </label>
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
              color: 'var(--color-brand-heading)',
            }}
            required
          />
        </div>

        <div>
          <label
            style={{
              display: 'block',
              fontSize: 11,
              fontWeight: 600,
              color: 'var(--color-brand-muted)',
              marginBottom: 6,
              textTransform: 'uppercase',
            }}
          >
            HTTP Port
          </label>
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
              color: 'var(--color-brand-heading)',
            }}
            required
          />
        </div>
      </div>

      <div style={{ display: 'grid', gridTemplateColumns: '1fr 1fr', gap: 16 }}>
        <div>
          <label
            style={{
              display: 'block',
              fontSize: 11,
              fontWeight: 600,
              color: 'var(--color-brand-muted)',
              marginBottom: 6,
              textTransform: 'uppercase',
            }}
          >
            DNS Resolver Server
          </label>
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
              color: 'var(--color-brand-heading)',
            }}
            required
          />
        </div>

        <div>
          <label
            style={{
              display: 'block',
              fontSize: 11,
              fontWeight: 600,
              color: 'var(--color-brand-muted)',
              marginBottom: 6,
              textTransform: 'uppercase',
            }}
          >
            Routing Preset
          </label>
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
              color: 'var(--color-brand-heading)',
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
          <label
            style={{
              display: 'block',
              fontSize: 11,
              fontWeight: 600,
              color: 'var(--color-brand-muted)',
              marginBottom: 6,
              textTransform: 'uppercase',
            }}
          >
            Custom Routing Rules (JSON array)
          </label>
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
              color: 'var(--color-brand-heading)',
            }}
          />
        </div>
      )}

      {/* DPI Evasion & Security */}
      <div style={{ borderTop: '1px solid var(--color-brand-border)', paddingTop: 16 }}>
        <div style={{ display: 'flex', alignItems: 'center', gap: 6, marginBottom: 14 }}>
          <FiLock style={{ color: 'var(--color-brand)', fontSize: 16 }} />
          <span style={{ fontSize: 13, fontWeight: 600, color: 'var(--color-brand-heading)' }}>
            DPI Evasion & Security Hardening
          </span>
          <FiHelpCircle
            style={{ cursor: 'pointer', color: 'var(--color-brand-muted)' }}
            onClick={() =>
              showHelp(
                'DPI Evasion & Security Hardening',
                'TLS fragmentation splits ClientHello records to slip past SNI deep packet filters. Browser fingerprints masquerade TLS signatures as standard web browsers (Chrome/Safari). ECH hides hostnames, and TCP Brutal speeds up output on congested lossy paths.'
              )
            }
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
              <div
                style={{
                  padding: 12,
                  background: 'var(--color-brand-bg)',
                  border: '1px solid var(--color-brand-border)',
                  borderRadius: 8,
                  display: 'flex',
                  flexDirection: 'column',
                  gap: 12,
                  marginTop: 4,
                  marginLeft: 8,
                }}
              >
                <div>
                  <label
                    style={{
                      display: 'block',
                      fontSize: 11,
                      fontWeight: 600,
                      color: 'var(--color-brand-muted)',
                      marginBottom: 4,
                      textTransform: 'uppercase',
                    }}
                  >
                    Fragment Mode
                  </label>
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
                      color: 'var(--color-brand-heading)',
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
                      <label
                        style={{
                          display: 'block',
                          fontSize: 10,
                          fontWeight: 600,
                          color: 'var(--color-brand-muted)',
                          marginBottom: 4,
                          textTransform: 'uppercase',
                        }}
                      >
                        Length Range
                      </label>
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
                          color: 'var(--color-brand-heading)',
                        }}
                      />
                    </div>
                    <div>
                      <label
                        style={{
                          display: 'block',
                          fontSize: 10,
                          fontWeight: 600,
                          color: 'var(--color-brand-muted)',
                          marginBottom: 4,
                          textTransform: 'uppercase',
                        }}
                      >
                        Interval (ms)
                      </label>
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
                          color: 'var(--color-brand-heading)',
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
              <label
                style={{
                  display: 'block',
                  fontSize: 11,
                  fontWeight: 600,
                  color: 'var(--color-brand-muted)',
                  marginBottom: 6,
                  textTransform: 'uppercase',
                }}
              >
                uTLS Browser Fingerprint
              </label>
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
                  color: 'var(--color-brand-heading)',
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
              <label
                style={{
                  display: 'block',
                  fontSize: 11,
                  fontWeight: 600,
                  color: 'var(--color-brand-muted)',
                  marginBottom: 6,
                  textTransform: 'uppercase',
                }}
              >
                Encrypted Client Hello (ECH)
              </label>
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
              <label
                style={{
                  display: 'block',
                  fontSize: 11,
                  fontWeight: 600,
                  color: 'var(--color-brand-muted)',
                  marginBottom: 6,
                  textTransform: 'uppercase',
                }}
              >
                ECH Base64 Config Payload
              </label>
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
                  color: 'var(--color-brand-heading)',
                }}
              />
            </div>
          )}
        </div>
      </div>

      <button
        type="submit"
        className="btn btn--primary"
        style={{ display: 'flex', alignItems: 'center', gap: 6 }}
        disabled={isLoading}
      >
        <FiSave /> Save Proxy Settings
      </button>
    </form>
  );
};

export default ConfigSettingsCard;
