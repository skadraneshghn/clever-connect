import React, { useState, useEffect } from 'react';
import { FiX, FiKey, FiSave } from 'react-icons/fi';

interface SpotifyConfigData {
  client_id: string;
  client_secret: string;
  default_save_path: string;
  default_format: string;
  default_bitrate: string;
  max_concurrent: number;
  embed_metadata: boolean;
  embed_lyrics: boolean;
  overwrite_existing: boolean;
  proxy_url: string;
  file_name_template: string;
}

interface SpotifyConfigModalProps {
  show: boolean;
  onClose: () => void;
  token: string;
  onSaved: () => void;
}

const FORMATS = ['mp3', 'flac', 'opus', 'm4a', 'ogg', 'wav'];
const BITRATES = ['128k', '192k', '256k', '320k', 'auto'];

const Toggle: React.FC<{ label: string; desc: string; value: boolean; onChange: (v: boolean) => void }> = ({ label, desc, value, onChange }) => (
  <div
    style={{
      display: 'flex', alignItems: 'center', justifyContent: 'space-between',
      padding: '10px 14px', borderRadius: 8,
      border: value ? '1px solid rgba(29,185,84,0.3)' : '1px solid var(--color-brand-border)',
      background: value ? 'rgba(29,185,84,0.04)' : 'transparent',
      cursor: 'pointer', transition: 'all 0.2s',
    }}
    onClick={() => onChange(!value)}
  >
    <div style={{ marginRight: 16 }}>
      <div style={{ fontSize: 13, fontWeight: 600, color: 'var(--color-brand-heading)' }}>{label}</div>
      <div style={{ fontSize: 10, color: 'var(--color-brand-muted)', marginTop: 1 }}>{desc}</div>
    </div>
    <div style={{
      width: 36, height: 18, borderRadius: 18,
      background: value ? '#1db954' : '#334155',
      position: 'relative', transition: 'background 0.2s', flexShrink: 0,
    }}>
      <div style={{
        width: 12, height: 12, borderRadius: '50%', background: '#fff',
        position: 'absolute', top: 3,
        left: value ? 21 : 3, transition: 'left 0.2s cubic-bezier(0.4,0,0.2,1)',
      }} />
    </div>
  </div>
);

export const SpotifyConfigModal: React.FC<SpotifyConfigModalProps> = ({ show, onClose, token, onSaved }) => {
  const [cfg, setCfg] = useState<SpotifyConfigData>({
    client_id: '',
    client_secret: '',
    default_save_path: './downloads/spotify/audios',
    default_format: 'mp3',
    default_bitrate: '320k',
    max_concurrent: 3,
    embed_metadata: true,
    embed_lyrics: true,
    overwrite_existing: false,
    proxy_url: '',
    file_name_template: '{artist} - {title}',
  });
  const [saving, setSaving] = useState(false);
  const [showSecret, setShowSecret] = useState(false);

  useEffect(() => {
    if (!show) return;
    fetch('/api/spotify/config', { headers: { 'Authorization': `Bearer ${token}` } })
      .then(r => r.json())
      .then(data => setCfg(prev => ({ ...prev, ...data })))
      .catch(() => {});
  }, [show, token]);

  if (!show) return null;

  const handleSave = async () => {
    setSaving(true);
    try {
      const res = await fetch('/api/spotify/config', {
        method: 'POST',
        headers: { 'Content-Type': 'application/json', 'Authorization': `Bearer ${token}` },
        body: JSON.stringify(cfg),
      });
      if (res.ok) {
        onSaved();
        onClose();
      }
    } catch { /* ignore */ }
    finally { setSaving(false); }
  };

  const inputStyle: React.CSSProperties = {
    width: '100%', padding: '10px 14px', borderRadius: 8,
    border: '1px solid var(--color-brand-border)', background: 'var(--color-brand-bg)',
    outline: 'none', color: 'var(--color-brand-heading)', fontSize: 13,
  };

  return (
    <div style={{ position: 'fixed', top: 0, left: 0, width: '100vw', height: '100vh', background: 'rgba(0,0,0,0.55)', backdropFilter: 'blur(6px)', zIndex: 1000, display: 'flex', alignItems: 'center', justifyContent: 'center' }}>
      <div className="g-card" style={{ width: '100%', maxWidth: 520, maxHeight: '90vh', overflowY: 'auto', display: 'flex', flexDirection: 'column', gap: 16 }}>
        {/* Header */}
        <div style={{ display: 'flex', justifyContent: 'space-between', alignItems: 'center' }}>
          <h2 style={{ fontSize: 18, fontWeight: 700, margin: 0, color: 'var(--color-brand-heading)', display: 'flex', alignItems: 'center', gap: 8 }}>
            <FiKey size={18} color="#1db954" /> Spotify Settings
          </h2>
          <button onClick={onClose} style={{ background: 'none', border: 'none', cursor: 'pointer', color: 'var(--color-brand-text)' }}><FiX size={18} /></button>
        </div>

        {/* API Credentials Section */}
        <div style={{ background: 'rgba(29,185,84,0.04)', border: '1px solid rgba(29,185,84,0.15)', borderRadius: 10, padding: 14, display: 'flex', flexDirection: 'column', gap: 10 }}>
          <div style={{ fontSize: 12, fontWeight: 700, color: '#1db954', display: 'flex', alignItems: 'center', gap: 6 }}>
            <FiKey size={13} /> Spotify API Credentials
          </div>
          <div style={{ fontSize: 10, color: 'var(--color-brand-muted)', lineHeight: 1.4 }}>
            Get your credentials from <a href="https://developer.spotify.com/dashboard" target="_blank" rel="noreferrer" style={{ color: '#1db954' }}>developer.spotify.com/dashboard</a>. Create an app and copy the Client ID & Secret.
          </div>
          <div style={{ display: 'flex', flexDirection: 'column', gap: 4 }}>
            <label style={{ fontSize: 11, fontWeight: 700, color: 'var(--color-brand-muted)' }}>Client ID</label>
            <input type="text" value={cfg.client_id} onChange={e => setCfg({ ...cfg, client_id: e.target.value })} placeholder="e.g. a1b2c3d4e5f6..." style={inputStyle} />
          </div>
          <div style={{ display: 'flex', flexDirection: 'column', gap: 4 }}>
            <label style={{ fontSize: 11, fontWeight: 700, color: 'var(--color-brand-muted)' }}>Client Secret</label>
            <div style={{ position: 'relative' }}>
              <input type={showSecret ? 'text' : 'password'} value={cfg.client_secret} onChange={e => setCfg({ ...cfg, client_secret: e.target.value })} placeholder="••••••••••••" style={{ ...inputStyle, paddingRight: 60 }} />
              <button onClick={() => setShowSecret(!showSecret)} style={{ position: 'absolute', right: 8, top: '50%', transform: 'translateY(-50%)', background: 'none', border: 'none', fontSize: 10, color: '#1db954', cursor: 'pointer', fontWeight: 600 }}>
                {showSecret ? 'HIDE' : 'SHOW'}
              </button>
            </div>
          </div>
        </div>

        {/* Download Defaults */}
        <div style={{ display: 'flex', gap: 12 }}>
          <div style={{ flex: 1, display: 'flex', flexDirection: 'column', gap: 4 }}>
            <label style={{ fontSize: 11, fontWeight: 700, color: 'var(--color-brand-muted)' }}>Default Format</label>
            <select value={cfg.default_format} onChange={e => setCfg({ ...cfg, default_format: e.target.value })} style={inputStyle}>
              {FORMATS.map(f => <option key={f} value={f}>{f.toUpperCase()}</option>)}
            </select>
          </div>
          <div style={{ flex: 1, display: 'flex', flexDirection: 'column', gap: 4 }}>
            <label style={{ fontSize: 11, fontWeight: 700, color: 'var(--color-brand-muted)' }}>Default Bitrate</label>
            <select value={cfg.default_bitrate} onChange={e => setCfg({ ...cfg, default_bitrate: e.target.value })} style={inputStyle}>
              {BITRATES.map(b => <option key={b} value={b}>{b === 'auto' ? 'Auto' : b}</option>)}
            </select>
          </div>
        </div>

        <div style={{ display: 'flex', gap: 12 }}>
          <div style={{ flex: 1, display: 'flex', flexDirection: 'column', gap: 4 }}>
            <label style={{ fontSize: 11, fontWeight: 700, color: 'var(--color-brand-muted)' }}>Default Save Path</label>
            <input type="text" value={cfg.default_save_path} onChange={e => setCfg({ ...cfg, default_save_path: e.target.value })} style={inputStyle} />
          </div>
          <div style={{ width: 100, display: 'flex', flexDirection: 'column', gap: 4 }}>
            <label style={{ fontSize: 11, fontWeight: 700, color: 'var(--color-brand-muted)' }}>Concurrency</label>
            <input type="number" min={1} max={10} value={cfg.max_concurrent} onChange={e => setCfg({ ...cfg, max_concurrent: Number(e.target.value) })} style={inputStyle} />
          </div>
        </div>

        <div style={{ display: 'flex', flexDirection: 'column', gap: 4 }}>
          <label style={{ fontSize: 11, fontWeight: 700, color: 'var(--color-brand-muted)' }}>Filename Template</label>
          <input type="text" value={cfg.file_name_template} onChange={e => setCfg({ ...cfg, file_name_template: e.target.value })} placeholder="{artist} - {title}" style={inputStyle} />
          <span style={{ fontSize: 9, color: 'var(--color-brand-muted)' }}>Available: {'{artist}'}, {'{title}'}, {'{album}'}, {'{track_number}'}</span>
        </div>

        <div style={{ display: 'flex', flexDirection: 'column', gap: 4 }}>
          <label style={{ fontSize: 11, fontWeight: 700, color: 'var(--color-brand-muted)' }}>HTTP Proxy (Optional)</label>
          <input type="text" value={cfg.proxy_url} onChange={e => setCfg({ ...cfg, proxy_url: e.target.value })} placeholder="e.g. http://127.0.0.1:7890" style={inputStyle} />
        </div>

        {/* Toggles */}
        <div style={{ display: 'flex', flexDirection: 'column', gap: 8 }}>
          <Toggle label="Embed Metadata" desc="Inject title, artist, album, and cover art into audio files" value={cfg.embed_metadata} onChange={v => setCfg({ ...cfg, embed_metadata: v })} />
          <Toggle label="Embed Lyrics" desc="Include synchronized lyrics when available" value={cfg.embed_lyrics} onChange={v => setCfg({ ...cfg, embed_lyrics: v })} />
          <Toggle label="Overwrite Existing" desc="Replace files if they already exist at the target path" value={cfg.overwrite_existing} onChange={v => setCfg({ ...cfg, overwrite_existing: v })} />
        </div>

        {/* Actions */}
        <div style={{ display: 'flex', justifyContent: 'end', gap: 12, marginTop: 4 }}>
          <button className="btn" onClick={onClose}>Cancel</button>
          <button className="btn btn--primary" onClick={handleSave} disabled={saving} style={{ background: '#1db954', borderColor: '#1db954', display: 'flex', alignItems: 'center', gap: 6 }}>
            <FiSave size={13} /> {saving ? 'Saving...' : 'Save Settings'}
          </button>
        </div>
      </div>
    </div>
  );
};
