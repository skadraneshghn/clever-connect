import React, { useState, useEffect } from 'react';
import { FiSliders, FiSun, FiMoon, FiMonitor, FiType, FiEye, FiEyeOff, FiCheck, FiLock, FiClipboard, FiInfo } from 'react-icons/fi';
import { Card } from '../components/molecules/Card';
import { useAuthStore } from '../store/authStore';

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
  const [selectedFont, setSelectedFont] = useState(() => localStorage.getItem('cc_font') || 'inter');
  const [selectedTheme, setSelectedTheme] = useState(() => localStorage.getItem('cc_theme') || 'light');
  const [showToken, setShowToken] = useState(false);
  const [faviconFile, setFaviconFile] = useState<File | null>(null);
  const [uploadingFavicon, setUploadingFavicon] = useState(false);
  const [faviconMsg, setFaviconMsg] = useState('');

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
      } else {
        setFaviconMsg(`❌ ${data.error}`);
      }
    } catch (e: any) {
      setFaviconMsg('❌ ' + e.message);
    }
    setUploadingFavicon(false);
  };

  // Decode JWT Payload helper
  const decodeJWT = (t: string | null) => {
    if (!t) return null;
    try {
      const base64Url = t.split('.')[1];
      const base64 = base64Url.replace(/-/g, '+').replace(/_/g, '/');
      const jsonPayload = decodeURIComponent(window.atob(base64).split('').map(function(c) {
          return '%' + ('00' + c.charCodeAt(0).toString(16)).slice(-2);
      }).join(''));
      return JSON.parse(jsonPayload);
    } catch (e) {
      return null;
    }
  };

  const jwtInfo = decodeJWT(token);

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

          {/* Security & JWT Session Info Card */}
          <div className="g-card animate-slide-in">
            <div style={{ display: 'flex', alignItems: 'center', gap: 8, marginBottom: 16 }}>
              <FiLock style={{ color: 'var(--color-brand)', fontSize: 18 }} />
              <span style={{ fontSize: 15, fontWeight: 600, color: 'var(--color-brand-heading)' }}>Security & Session Keys (JWT)</span>
            </div>

            <div style={{ display: 'flex', flexDirection: 'column', gap: 14 }}>
              {/* Active JWT Access Token view */}
              <div style={{ display: 'flex', flexDirection: 'column', gap: 6 }}>
                <div style={{ display: 'flex', justifyContent: 'space-between', alignItems: 'center' }}>
                  <label style={{ fontSize: 11, fontWeight: 700, color: 'var(--color-brand-muted)', textTransform: 'uppercase', letterSpacing: '0.05em' }}>
                    Active JWT Authentication Token
                  </label>
                  <div style={{ display: 'flex', gap: 8 }}>
                    <button 
                      onClick={() => setShowToken(prev => !prev)} 
                      style={{ background: 'none', border: 'none', cursor: 'pointer', color: 'var(--color-brand)', display: 'flex', alignItems: 'center', gap: 4, fontSize: 11 }}
                    >
                      {showToken ? <FiEyeOff size={13} /> : <FiEye size={13} />} {showToken ? 'Hide' : 'Show'}
                    </button>
                    <button 
                      onClick={() => { token && navigator.clipboard.writeText(token); alert('JWT Token copied to clipboard!'); }}
                      style={{ background: 'none', border: 'none', cursor: 'pointer', color: 'var(--color-brand)', display: 'flex', alignItems: 'center', gap: 4, fontSize: 11 }}
                      disabled={!token}
                    >
                      <FiClipboard size={13} /> Copy Key
                    </button>
                  </div>
                </div>
                
                <textarea 
                  readOnly 
                  value={token || 'No active JWT token available'}
                  style={{
                    width: '100%',
                    height: 80,
                    fontSize: 11,
                    fontFamily: 'monospace',
                    padding: 10,
                    borderRadius: 8,
                    border: '1px solid var(--color-brand-border)',
                    background: 'var(--color-brand-bg)',
                    color: showToken ? 'var(--color-brand-heading)' : 'var(--color-brand-muted)',
                    resize: 'none',
                    outline: 'none',
                    filter: showToken ? 'none' : 'blur(4px)',
                    transition: 'all 0.2s ease'
                  }}
                />
              </div>

              {/* JWT Claims Metadata Breakdown */}
              <div style={{ background: 'rgba(99,102,241,0.02)', borderRadius: 10, padding: 14, border: '1px solid var(--color-brand-border)' }}>
                <div style={{ display: 'flex', alignItems: 'center', gap: 6, marginBottom: 10 }}>
                  <FiInfo size={14} style={{ color: 'var(--color-brand)' }} />
                  <span style={{ fontSize: 12, fontWeight: 700, color: 'var(--color-brand-heading)' }}>Token Signature Analysis</span>
                </div>
                
                <table style={{ width: '100%', fontSize: 12, borderCollapse: 'collapse' }}>
                  <tbody>
                    <tr style={{ borderBottom: '1px solid var(--color-brand-border)' }}>
                      <td style={{ padding: '6px 0', color: 'var(--color-brand-text)', fontWeight: 600 }}>Active Role</td>
                      <td style={{ padding: '6px 0', textAlign: 'right', color: 'var(--color-brand-heading)', fontWeight: 700, textTransform: 'uppercase' }}>
                        {jwtInfo?.role || 'administrator'}
                      </td>
                    </tr>
                    <tr style={{ borderBottom: '1px solid var(--color-brand-border)' }}>
                      <td style={{ padding: '6px 0', color: 'var(--color-brand-text)', fontWeight: 600 }}>Subject (Username)</td>
                      <td style={{ padding: '6px 0', textAlign: 'right', color: 'var(--color-brand-heading)', fontFamily: 'monospace' }}>
                        {jwtInfo?.username || 'salman'}
                      </td>
                    </tr>
                    <tr style={{ borderBottom: '1px solid var(--color-brand-border)' }}>
                      <td style={{ padding: '6px 0', color: 'var(--color-brand-text)', fontWeight: 600 }}>Issued Timestamp</td>
                      <td style={{ padding: '6px 0', textAlign: 'right', color: 'var(--color-brand-heading)' }}>
                        {jwtInfo?.iat ? new Date(jwtInfo.iat * 1000).toLocaleString() : 'N/A'}
                      </td>
                    </tr>
                    <tr style={{ borderBottom: '1px solid var(--color-brand-border)' }}>
                      <td style={{ padding: '6px 0', color: 'var(--color-brand-text)', fontWeight: 600 }}>Token Expiration</td>
                      <td style={{ padding: '6px 0', textAlign: 'right', color: '#10b981', fontWeight: 600 }}>
                        {jwtInfo?.exp ? new Date(jwtInfo.exp * 1000).toLocaleString() : 'Never'}
                      </td>
                    </tr>
                    <tr>
                      <td style={{ padding: '6px 0', color: 'var(--color-brand-text)', fontWeight: 600 }}>Encryption Algorithm</td>
                      <td style={{ padding: '6px 0', textAlign: 'right', color: 'var(--color-brand-muted)', fontFamily: 'monospace' }}>
                        HMAC-SHA256 (JWT Header HS256)
                      </td>
                    </tr>
                  </tbody>
                </table>
              </div>
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
