import React, { useState, useEffect } from 'react';
import { FiSliders, FiSun, FiMoon, FiMonitor, FiType, FiEye, FiCheck, FiGlobe } from 'react-icons/fi';
import { Card } from '../components/molecules/Card';
import { useAuthStore } from '../store/authStore';
import { useLookupStore } from '../store/lookupStore';
import type { ApiKeysConfig } from '../store/lookupStore';

const fonts = [
  { id: 'inter', name: 'Inter', description: 'Clean modern sans-serif, standard interface optimization.', cssClass: 'font-inter' },
  { id: 'outfit', name: 'Outfit', description: 'Elegant rounded geometric style, premium brand feel.', cssClass: 'font-outfit' },
  { id: 'firacode', name: 'Fira Code', description: 'Developer-oriented monospace, perfect for logs and metrics.', cssClass: 'font-firacode' }
];

const themes = [
  { id: 'light', name: 'Light Mode', description: 'Warm beige canvas with elegant rounded borders.', icon: FiSun },
  { id: 'dark', name: 'Dark Mode', description: 'Premium rich dark slate, highly readable and friendly on eyes.', icon: FiMoon },
  { id: 'system', name: 'System Preference', description: 'Automatically matches your native OS setting.', icon: FiMonitor }
];

export const SettingsPage: React.FC = () => {
  const { token } = useAuthStore();
  const { apiConfig, fetchApiConfig, saveApiConfig, testConnection } = useLookupStore();
  const [selectedFont, setSelectedFont] = useState(() => localStorage.getItem('cc_font') || 'inter');
  const [selectedTheme, setSelectedTheme] = useState(() => localStorage.getItem('cc_theme') || 'light');
  const [faviconFile, setFaviconFile] = useState<File | null>(null);
  const [uploadingFavicon, setUploadingFavicon] = useState(false);
  const [faviconMsg, setFaviconMsg] = useState('');

  const [localApiKeys, setLocalApiKeys] = useState<ApiKeysConfig | null>(null);
  const [testStatus, setTestStatus] = useState<Record<string, { loading: boolean; valid?: boolean; error?: string }>>({});
  const [saveStatus, setSaveStatus] = useState('');

  useEffect(() => {
    fetchApiConfig();
  }, [fetchApiConfig]);

  useEffect(() => {
    if (apiConfig) {
      setLocalApiKeys({ ...apiConfig });
    }
  }, [apiConfig]);

  const handleSaveApiKeys = async () => {
    if (!localApiKeys) return;
    setSaveStatus('Saving...');
    const success = await saveApiConfig(localApiKeys);
    if (success) {
      setSaveStatus('✅ Saved successfully!');
      setTimeout(() => setSaveStatus(''), 3000);
    } else {
      setSaveStatus('❌ Failed to save configurations.');
    }
  };

  const handleTestKey = async (service: string, key: string) => {
    setTestStatus(prev => ({ ...prev, [service]: { loading: true } }));
    const result = await testConnection(service, key);
    setTestStatus(prev => ({
      ...prev,
      [service]: {
        loading: false,
        valid: result.valid,
        error: result.error
      }
    }));
  };

  const renderTestStatus = (status: { loading: boolean; valid?: boolean; error?: string }) => {
    if (status.loading) return null;
    if (status.valid) {
      return <div style={{ fontSize: 11, color: '#10b981', marginTop: 4, fontWeight: 600 }}>✅ Connection Success! API key is valid.</div>;
    }
    return <div style={{ fontSize: 11, color: '#ef4444', marginTop: 4, fontWeight: 500 }}>❌ Connection Failed: {status.error}</div>;
  };

  const handleFaviconUpload = async () => {
    if (!faviconFile) return;
    setUploadingFavicon(true);
    setFaviconMsg('');
    try {
      const formData = new FormData();
      formData.append('favicon', faviconFile);
      const res = await fetch('/api/settings/favicon', {
        method: 'POST',
        headers: { 'Authorization': `Bearer ${token}` },
        body: formData,
      });
      const data = await res.json();
      if (res.ok) {
        setFaviconMsg('✅ Favicon updated successfully! Applied instantly.');
        setFaviconFile(null);
        const fileInput = document.getElementById('favicon-input') as HTMLInputElement;
        if (fileInput) fileInput.value = '';

        // Update favicon links dynamically in DOM
        const links = document.querySelectorAll("link[rel~='icon']");
        links.forEach((link: any) => {
          link.href = '/favicon.png?t=' + Date.now();
        });
        if (links.length === 0) {
          const newLink = document.createElement('link');
          newLink.rel = 'icon';
          newLink.type = 'image/png';
          newLink.href = '/favicon.png?t=' + Date.now();
          document.head.appendChild(newLink);
        }

        // Update sidebar logo images dynamically
        const logoImgs = document.querySelectorAll(".sidebar__logo-icon img");
        logoImgs.forEach((img: any) => {
          img.src = '/favicon.png?t=' + Date.now();
        });
      } else {
        setFaviconMsg(`❌ ${data.error}`);
      }
    } catch (e: any) {
      setFaviconMsg('❌ ' + e.message);
    }
    setUploadingFavicon(false);
  };

  // Apply Font
  useEffect(() => {
    localStorage.setItem('cc_font', selectedFont);
    document.body.classList.remove('font-inter', 'font-outfit', 'font-firacode');
    document.body.classList.add(`font-${selectedFont}`);
  }, [selectedFont]);

  // Apply Theme
  useEffect(() => {
    localStorage.setItem('cc_theme', selectedTheme);
    
    const applyThemeMode = (isDark: boolean) => {
      if (isDark) {
        document.body.classList.add('dark-theme');
      } else {
        document.body.classList.remove('dark-theme');
      }
    };

    if (selectedTheme === 'system') {
      const mediaQuery = window.matchMedia('(prefers-color-scheme: dark)');
      applyThemeMode(mediaQuery.matches);

      const handler = (e: MediaQueryListEvent) => applyThemeMode(e.matches);
      mediaQuery.addEventListener('change', handler);
      return () => mediaQuery.removeEventListener('change', handler);
    } else {
      applyThemeMode(selectedTheme === 'dark');
    }
  }, [selectedTheme]);

  return (
    <div>
      {/* Title */}
      <div style={{ marginBottom: 24 }}>
        <h1 style={{ fontSize: 22, fontWeight: 700, color: 'var(--color-brand-heading)', margin: 0 }}>System Settings</h1>
        <p style={{ fontSize: 12, color: 'var(--color-brand-text)', margin: '4px 0 0' }}>Configure appearance, font faces, and offline themes for CleverConnect.</p>
      </div>

      <div style={{ display: 'grid', gridTemplateColumns: '1fr 340px', gap: 24, alignItems: 'start' }}>
        {/* Left Column - Configurations */}
        <div style={{ display: 'flex', flexDirection: 'column', gap: 20 }}>
          
          {/* Theme Selector Card */}
          <div className="g-card">
            <div style={{ display: 'flex', alignItems: 'center', gap: 8, marginBottom: 16 }}>
              <FiSun style={{ color: 'var(--color-brand)', fontSize: 18 }} />
              <span style={{ fontSize: 15, fontWeight: 600, color: 'var(--color-brand-heading)' }}>System Appearance</span>
            </div>
            
            <div style={{ display: 'grid', gridTemplateColumns: 'repeat(3, 1fr)', gap: 12 }}>
              {themes.map((t) => {
                const Icon = t.icon;
                const isSelected = selectedTheme === t.id;
                return (
                  <div
                    key={t.id}
                    onClick={() => setSelectedTheme(t.id)}
                    style={{
                      border: isSelected ? '2px solid var(--color-brand)' : '1px solid var(--color-brand-border)',
                      borderRadius: 12,
                      padding: 16,
                      background: 'var(--color-brand-card)',
                      cursor: 'pointer',
                      transition: 'transform 0.15s, box-shadow 0.15s',
                      textAlign: 'center',
                      display: 'flex',
                      flexDirection: 'column',
                      alignItems: 'center',
                      gap: 8,
                      boxShadow: isSelected ? '0 4px 12px rgba(255, 107, 44, 0.1)' : 'none'
                    }}
                    className="theme-selector-item"
                  >
                    <Icon size={20} style={{ color: isSelected ? 'var(--color-brand)' : 'var(--color-brand-muted)' }} />
                    <div style={{ fontSize: 13, fontWeight: 600, color: 'var(--color-brand-heading)' }}>{t.name}</div>
                    <div style={{ fontSize: 10, color: 'var(--color-brand-text)', lineHeight: 1.3 }}>{t.description}</div>
                  </div>
                );
              })}
            </div>
          </div>

          {/* Favicon Upload Card */}
          <div className="g-card animate-slide-in">
            <div style={{ display: 'flex', alignItems: 'center', gap: 8, marginBottom: 16 }}>
              <FiSliders style={{ color: 'var(--color-brand)', fontSize: 18 }} />
              <span style={{ fontSize: 15, fontWeight: 600, color: 'var(--color-brand-heading)' }}>Website Favicon</span>
            </div>
            <p style={{ fontSize: 11, color: 'var(--color-brand-text)', marginBottom: 14 }}>
              Upload any image format to set it as the website's favicon. It will automatically be converted to a premium high-quality icon.
            </p>
            <div style={{ display: 'flex', gap: 10, alignItems: 'center' }}>
              <input
                id="favicon-input"
                type="file"
                accept="image/*"
                onChange={e => setFaviconFile(e.target.files?.[0] || null)}
                style={{
                  flex: 1,
                  fontSize: 12,
                  color: 'var(--color-brand-text)',
                  padding: '8px 12px',
                  borderRadius: 8,
                  border: '1px solid var(--color-brand-border)',
                  background: 'var(--color-brand-bg)'
                }}
              />
              <button
                className="btn btn--sm btn--primary"
                onClick={handleFaviconUpload}
                disabled={uploadingFavicon || !faviconFile}
                style={{ height: 38, whiteSpace: 'nowrap' }}
              >
                {uploadingFavicon ? 'Uploading...' : 'Upload Icon'}
              </button>
            </div>
            {faviconMsg && (
              <div style={{ marginTop: 10, fontSize: 12, color: faviconMsg.startsWith('✅') ? '#10b981' : '#ef4444' }}>
                {faviconMsg}
              </div>
            )}
          </div>

          {/* API Keys Settings Card */}
          {localApiKeys && (
            <div className="g-card animate-slide-in">
              <div style={{ display: 'flex', alignItems: 'center', gap: 8, marginBottom: 16 }}>
                <FiGlobe style={{ color: 'var(--color-brand)', fontSize: 18 }} />
                <span style={{ fontSize: 15, fontWeight: 600, color: 'var(--color-brand-heading)' }}>IP & Domain Intelligence APIs</span>
              </div>
              <p style={{ fontSize: 11, color: 'var(--color-brand-text)', marginBottom: 16 }}>
                Configure API keys and toggles for third-party intelligence services. Toggled active endpoints will be queried concurrently.
              </p>

              <div style={{ display: 'flex', flexDirection: 'column', gap: 16 }}>
                {/* IP2Location */}
                <div style={{ borderBottom: '1px solid var(--color-brand-border)', paddingBottom: 16 }}>
                  <div style={{ display: 'flex', justifyContent: 'space-between', alignItems: 'center', marginBottom: 8 }}>
                    <span style={{ fontSize: 12, fontWeight: 600, color: 'var(--color-brand-heading)' }}>IP2Location.io (Primary / WHOIS)</span>
                    <label style={{ display: 'flex', alignItems: 'center', gap: 6, cursor: 'pointer' }}>
                      <input
                        type="checkbox"
                        checked={localApiKeys.enable_ip2location}
                        onChange={e => setLocalApiKeys({ ...localApiKeys, enable_ip2location: e.target.checked })}
                        style={{ accentColor: 'var(--color-brand)' }}
                      />
                      <span style={{ fontSize: 11, color: 'var(--color-brand-text)' }}>Enable</span>
                    </label>
                  </div>
                  <div style={{ display: 'flex', gap: 10 }}>
                    <input
                      type="password"
                      placeholder="Enter IP2Location.io API Key"
                      value={localApiKeys.ip2location_key || ''}
                      onChange={e => setLocalApiKeys({ ...localApiKeys, ip2location_key: e.target.value })}
                      style={{
                        flex: 1,
                        fontSize: 12,
                        padding: '8px 12px',
                        borderRadius: 8,
                        border: '1px solid var(--color-brand-border)',
                        background: 'var(--color-brand-bg)',
                        color: 'var(--color-brand-heading)'
                      }}
                    />
                    <button
                      className="btn btn--sm"
                      onClick={() => handleTestKey('ip2location', localApiKeys.ip2location_key)}
                      disabled={testStatus['ip2location']?.loading || !localApiKeys.ip2location_key}
                      style={{ height: 38 }}
                    >
                      {testStatus['ip2location']?.loading ? 'Testing...' : 'Test'}
                    </button>
                  </div>
                  {testStatus['ip2location'] && renderTestStatus(testStatus['ip2location'])}
                </div>

                {/* IP-API */}
                <div style={{ borderBottom: '1px solid var(--color-brand-border)', paddingBottom: 16 }}>
                  <div style={{ display: 'flex', justifyContent: 'space-between', alignItems: 'center', marginBottom: 8 }}>
                    <span style={{ fontSize: 12, fontWeight: 600, color: 'var(--color-brand-heading)' }}>IP-API.com (Pro Key / Free default)</span>
                    <label style={{ display: 'flex', alignItems: 'center', gap: 6, cursor: 'pointer' }}>
                      <input
                        type="checkbox"
                        checked={localApiKeys.enable_ip_api}
                        onChange={e => setLocalApiKeys({ ...localApiKeys, enable_ip_api: e.target.checked })}
                        style={{ accentColor: 'var(--color-brand)' }}
                      />
                      <span style={{ fontSize: 11, color: 'var(--color-brand-text)' }}>Enable</span>
                    </label>
                  </div>
                  <div style={{ display: 'flex', gap: 10 }}>
                    <input
                      type="password"
                      placeholder="Enter IP-API.com Key (Optional for Free)"
                      value={localApiKeys.ip_api_key || ''}
                      onChange={e => setLocalApiKeys({ ...localApiKeys, ip_api_key: e.target.value })}
                      style={{
                        flex: 1,
                        fontSize: 12,
                        padding: '8px 12px',
                        borderRadius: 8,
                        border: '1px solid var(--color-brand-border)',
                        background: 'var(--color-brand-bg)',
                        color: 'var(--color-brand-heading)'
                      }}
                    />
                    <button
                      className="btn btn--sm"
                      onClick={() => handleTestKey('ip_api', localApiKeys.ip_api_key)}
                      disabled={testStatus['ip_api']?.loading || !localApiKeys.ip_api_key}
                      style={{ height: 38 }}
                    >
                      {testStatus['ip_api']?.loading ? 'Testing...' : 'Test'}
                    </button>
                  </div>
                  {testStatus['ip_api'] && renderTestStatus(testStatus['ip_api'])}
                </div>

                {/* IPGeolocation.io */}
                <div style={{ borderBottom: '1px solid var(--color-brand-border)', paddingBottom: 16 }}>
                  <div style={{ display: 'flex', justifyContent: 'space-between', alignItems: 'center', marginBottom: 8 }}>
                    <span style={{ fontSize: 12, fontWeight: 600, color: 'var(--color-brand-heading)' }}>IPGeolocation.io</span>
                    <label style={{ display: 'flex', alignItems: 'center', gap: 6, cursor: 'pointer' }}>
                      <input
                        type="checkbox"
                        checked={localApiKeys.enable_ip_geolocation}
                        onChange={e => setLocalApiKeys({ ...localApiKeys, enable_ip_geolocation: e.target.checked })}
                        style={{ accentColor: 'var(--color-brand)' }}
                      />
                      <span style={{ fontSize: 11, color: 'var(--color-brand-text)' }}>Enable</span>
                    </label>
                  </div>
                  <div style={{ display: 'flex', gap: 10 }}>
                    <input
                      type="password"
                      placeholder="Enter IPGeolocation.io API Key"
                      value={localApiKeys.ip_geolocation_key || ''}
                      onChange={e => setLocalApiKeys({ ...localApiKeys, ip_geolocation_key: e.target.value })}
                      style={{
                        flex: 1,
                        fontSize: 12,
                        padding: '8px 12px',
                        borderRadius: 8,
                        border: '1px solid var(--color-brand-border)',
                        background: 'var(--color-brand-bg)',
                        color: 'var(--color-brand-heading)'
                      }}
                    />
                    <button
                      className="btn btn--sm"
                      onClick={() => handleTestKey('ipgeolocation', localApiKeys.ip_geolocation_key)}
                      disabled={testStatus['ipgeolocation']?.loading || !localApiKeys.ip_geolocation_key}
                      style={{ height: 38 }}
                    >
                      {testStatus['ipgeolocation']?.loading ? 'Testing...' : 'Test'}
                    </button>
                  </div>
                  {testStatus['ipgeolocation'] && renderTestStatus(testStatus['ipgeolocation'])}
                </div>

                {/* IPWhois.io */}
                <div style={{ borderBottom: '1px solid var(--color-brand-border)', paddingBottom: 16 }}>
                  <div style={{ display: 'flex', justifyContent: 'space-between', alignItems: 'center', marginBottom: 8 }}>
                    <span style={{ fontSize: 12, fontWeight: 600, color: 'var(--color-brand-heading)' }}>IPWhois.io / IPWhois.app</span>
                    <label style={{ display: 'flex', alignItems: 'center', gap: 6, cursor: 'pointer' }}>
                      <input
                        type="checkbox"
                        checked={localApiKeys.enable_ip_whois}
                        onChange={e => setLocalApiKeys({ ...localApiKeys, enable_ip_whois: e.target.checked })}
                        style={{ accentColor: 'var(--color-brand)' }}
                      />
                      <span style={{ fontSize: 11, color: 'var(--color-brand-text)' }}>Enable</span>
                    </label>
                  </div>
                  <div style={{ display: 'flex', gap: 10 }}>
                    <input
                      type="password"
                      placeholder="Enter IPWhois.pro Key (Optional for Free)"
                      value={localApiKeys.ip_whois_key || ''}
                      onChange={e => setLocalApiKeys({ ...localApiKeys, ip_whois_key: e.target.value })}
                      style={{
                        flex: 1,
                        fontSize: 12,
                        padding: '8px 12px',
                        borderRadius: 8,
                        border: '1px solid var(--color-brand-border)',
                        background: 'var(--color-brand-bg)',
                        color: 'var(--color-brand-heading)'
                      }}
                    />
                    <button
                      className="btn btn--sm"
                      onClick={() => handleTestKey('ipwhois', localApiKeys.ip_whois_key)}
                      disabled={testStatus['ipwhois']?.loading || !localApiKeys.ip_whois_key}
                      style={{ height: 38 }}
                    >
                      {testStatus['ipwhois']?.loading ? 'Testing...' : 'Test'}
                    </button>
                  </div>
                  {testStatus['ipwhois'] && renderTestStatus(testStatus['ipwhois'])}
                </div>

                {/* FindIP.net */}
                <div>
                  <div style={{ display: 'flex', justifyContent: 'space-between', alignItems: 'center', marginBottom: 8 }}>
                    <span style={{ fontSize: 12, fontWeight: 600, color: 'var(--color-brand-heading)' }}>FindIP.net</span>
                    <label style={{ display: 'flex', alignItems: 'center', gap: 6, cursor: 'pointer' }}>
                      <input
                        type="checkbox"
                        checked={localApiKeys.enable_find_ip}
                        onChange={e => setLocalApiKeys({ ...localApiKeys, enable_find_ip: e.target.checked })}
                        style={{ accentColor: 'var(--color-brand)' }}
                      />
                      <span style={{ fontSize: 11, color: 'var(--color-brand-text)' }}>Enable</span>
                    </label>
                  </div>
                  <div style={{ display: 'flex', gap: 10 }}>
                    <input
                      type="password"
                      placeholder="Enter FindIP.net API Token"
                      value={localApiKeys.find_ip_key || ''}
                      onChange={e => setLocalApiKeys({ ...localApiKeys, find_ip_key: e.target.value })}
                      style={{
                        flex: 1,
                        fontSize: 12,
                        padding: '8px 12px',
                        borderRadius: 8,
                        border: '1px solid var(--color-brand-border)',
                        background: 'var(--color-brand-bg)',
                        color: 'var(--color-brand-heading)'
                      }}
                    />
                    <button
                      className="btn btn--sm"
                      onClick={() => handleTestKey('findip', localApiKeys.find_ip_key)}
                      disabled={testStatus['findip']?.loading || !localApiKeys.find_ip_key}
                      style={{ height: 38 }}
                    >
                      {testStatus['findip']?.loading ? 'Testing...' : 'Test'}
                    </button>
                  </div>
                  {testStatus['findip'] && renderTestStatus(testStatus['findip'])}
                </div>
              </div>

              <div style={{ marginTop: 20, display: 'flex', gap: 10, alignItems: 'center' }}>
                <button
                  className="btn btn--sm btn--primary"
                  onClick={handleSaveApiKeys}
                  style={{ height: 38, whiteSpace: 'nowrap' }}
                >
                  Save API Configurations
                </button>
                {saveStatus && (
                  <span style={{ fontSize: 12, color: saveStatus.startsWith('✅') ? '#10b981' : '#ef4444' }}>
                    {saveStatus}
                  </span>
                )}
              </div>
            </div>
          )}

          {/* Font Selector Card */}
          <div className="g-card">
            <div style={{ display: 'flex', alignItems: 'center', gap: 8, marginBottom: 16 }}>
              <FiType style={{ color: 'var(--color-brand)', fontSize: 18 }} />
              <span style={{ fontSize: 15, fontWeight: 600, color: 'var(--color-brand-heading)' }}>Typography & Font Faces</span>
            </div>

            <div style={{ display: 'flex', flexDirection: 'column', gap: 10 }}>
              {fonts.map((f) => {
                const isSelected = selectedFont === f.id;
                return (
                  <div
                    key={f.id}
                    onClick={() => setSelectedFont(f.id)}
                    style={{
                      display: 'flex',
                      alignItems: 'center',
                      justifyContent: 'space-between',
                      border: isSelected ? '1px solid var(--color-brand)' : '1px solid var(--color-brand-border)',
                      borderRadius: 10,
                      padding: '12px 16px',
                      background: isSelected ? 'var(--color-brand-light)' : 'var(--color-brand-card)',
                      cursor: 'pointer',
                      transition: 'background 0.2s, border-color 0.2s'
                    }}
                  >
                    <div>
                      <span style={{ fontSize: 14, fontWeight: 600, color: 'var(--color-brand-heading)', fontFamily: f.id === 'firacode' ? 'Fira Code' : f.id === 'outfit' ? 'Outfit' : 'Inter' }}>
                        {f.name}
                      </span>
                      <div style={{ fontSize: 11, color: 'var(--color-brand-text)', marginTop: 2 }}>{f.description}</div>
                    </div>
                    {isSelected && <FiCheck style={{ color: 'var(--color-brand)', fontSize: 16 }} />}
                  </div>
                );
              })}
            </div>
          </div>

        </div>

        {/* Right Column - Live Preview */}
        <div>
          <div className="g-card" style={{ position: 'sticky', top: 20 }}>
            <div style={{ display: 'flex', alignItems: 'center', gap: 6, marginBottom: 14 }}>
              <FiEye style={{ color: 'var(--color-brand)', fontSize: 16 }} />
              <span style={{ fontSize: 14, fontWeight: 600, color: 'var(--color-brand-heading)' }}>Interactive Preview</span>
            </div>

            <p style={{ fontSize: 11, color: 'var(--color-brand-text)', marginBottom: 12 }}>
              See how components and letters appear immediately on your panel layout canvas.
            </p>

            <div style={{ background: 'var(--color-brand-bg)', borderRadius: 10, padding: 16, border: '1px solid var(--color-brand-border)' }}>
              <div style={{ display: 'flex', justifyContent: 'space-between', alignItems: 'center', marginBottom: 10 }}>
                <span style={{ fontSize: 9, fontWeight: 700, textTransform: 'uppercase', letterSpacing: 1, color: 'var(--color-brand-muted)' }}>PREVIEW BADGE</span>
                <span className="live-dot" style={{ display: 'inline-block', width: 6, height: 6, borderRadius: '50%', background: 'var(--color-brand-green)' }} />
              </div>
              <div style={{ fontSize: 20, fontWeight: 700, color: 'var(--color-brand-heading)', marginBottom: 6 }}>
                The quick brown fox jumps over the lazy dog.
              </div>
              <div style={{ fontSize: 12, color: 'var(--color-brand-text)', lineHeight: 1.4 }}>
                This card uses the locally served <strong>{fonts.find(f => f.id === selectedFont)?.name}</strong> font family inside a theme-responsive layout.
              </div>

              <div style={{ display: 'flex', gap: 8, marginTop: 14 }}>
                <button className="btn btn--sm btn--primary">Primary Action</button>
                <button className="btn btn--sm">Cancel</button>
              </div>
            </div>
          </div>
        </div>
      </div>
    </div>
  );
};
