import React, { useState, useEffect } from 'react';
import { FiX, FiFolder, FiChevronLeft, FiFolderPlus, FiCheck, FiLoader, FiMusic, FiDisc, FiInfo } from 'react-icons/fi';

interface TrackMeta {
  id: string;
  title: string;
  artist: string;
  artists: string[];
  album: string;
  album_artist: string;
  cover_url: string;
  release_date: string;
  track_number: number;
  total_tracks: number;
  disc_number: number;
  duration_ms: number;
  isrc: string;
  genre: string;
  explicit: boolean;
  popularity: number;
  spotify_url: string;
}

interface AlbumMeta {
  id: string;
  name: string;
  artist: string;
  cover_url: string;
  release_date: string;
  total_tracks: number;
  tracks: TrackMeta[];
}

interface SpotifyAddModalProps {
  show: boolean;
  onClose: () => void;
  token: string;
  defaultSaveDir: string;
  defaultFormat: string;
  defaultBitrate: string;
}

const formatDuration = (ms: number) => {
  const totalSec = Math.floor(ms / 1000);
  const min = Math.floor(totalSec / 60);
  const sec = totalSec % 60;
  return `${min}:${sec.toString().padStart(2, '0')}`;
};

const FORMATS = ['mp3', 'flac', 'opus', 'm4a', 'ogg', 'wav'];
const BITRATES = ['128k', '192k', '256k', '320k', 'auto'];

export const SpotifyAddModal: React.FC<SpotifyAddModalProps> = ({ show, onClose, token, defaultSaveDir, defaultFormat, defaultBitrate }) => {
  const [url, setUrl] = useState('');
  const [isFetching, setIsFetching] = useState(false);
  const [fetchError, setFetchError] = useState('');
  const [linkType, setLinkType] = useState<'track' | 'album' | null>(null);
  const [trackData, setTrackData] = useState<TrackMeta | null>(null);
  const [albumData, setAlbumData] = useState<AlbumMeta | null>(null);
  const [format, setFormat] = useState(defaultFormat || 'mp3');
  const [bitrate, setBitrate] = useState(defaultBitrate || '320k');
  const [saveDir, setSaveDir] = useState(defaultSaveDir || 'downloads/spotify/audios');
  const [isAdding, setIsAdding] = useState(false);

  // Folder picker state
  const [showFolderPicker, setShowFolderPicker] = useState(false);
  const [currentPath, setCurrentPath] = useState('/');
  const [directories, setDirectories] = useState<string[]>([]);
  const [newFolderName, setNewFolderName] = useState('');
  const [showNewFolder, setShowNewFolder] = useState(false);

  useEffect(() => {
    setFormat(defaultFormat || 'mp3');
    setBitrate(defaultBitrate || '320k');
    setSaveDir(defaultSaveDir || 'downloads/spotify/audios');
  }, [defaultFormat, defaultBitrate, defaultSaveDir]);

  if (!show) return null;

  // Auto-fetch when Spotify link detected
  const handleUrlChange = async (val: string) => {
    setUrl(val);
    setFetchError('');
    setTrackData(null);
    setAlbumData(null);
    setLinkType(null);

    const trimmed = val.trim();
    if (!trimmed.includes('open.spotify.com/')) return;

    setIsFetching(true);
    try {
      const res = await fetch('/api/spotify/info', {
        method: 'POST',
        headers: { 'Content-Type': 'application/json', 'Authorization': `Bearer ${token}` },
        body: JSON.stringify({ url: trimmed }),
      });
      if (res.ok) {
        const json = await res.json();
        if (json.type === 'track') {
          setLinkType('track');
          setTrackData(json.data);
        } else if (json.type === 'album') {
          setLinkType('album');
          setAlbumData(json.data);
        }
      } else {
        const err = await res.json();
        setFetchError(err.details || err.error || 'Failed to fetch Spotify info');
      }
    } catch {
      setFetchError('Network error while fetching Spotify info');
    } finally {
      setIsFetching(false);
    }
  };

  const handleSubmit = async () => {
    if (!linkType) return;
    setIsAdding(true);
    try {
      const body: any = { url: url.trim(), format, bitrate, save_directory: saveDir, type: linkType };
      if (linkType === 'track' && trackData) body.track = trackData;
      if (linkType === 'album' && albumData) body.album = albumData;

      const res = await fetch('/api/spotify/add', {
        method: 'POST',
        headers: { 'Content-Type': 'application/json', 'Authorization': `Bearer ${token}` },
        body: JSON.stringify(body),
      });
      if (res.ok) {
        setUrl('');
        setTrackData(null);
        setAlbumData(null);
        setLinkType(null);
        onClose();
      } else {
        const err = await res.json();
        setFetchError(err.details || err.error || 'Failed to add download');
      }
    } catch {
      setFetchError('Network error');
    } finally {
      setIsAdding(false);
    }
  };

  // Folder picker helpers
  const fetchDirs = async (path: string) => {
    try {
      const res = await fetch(`/api/files/list?path=${encodeURIComponent(path)}`, {
        headers: { 'Authorization': `Bearer ${token}` },
      });
      if (res.ok) {
        const data = await res.json();
        const dirs = (data.files || []).filter((f: any) => f.is_dir).map((f: any) => f.name);
        setDirectories(dirs);
        setCurrentPath(data.current_path || '/');
      }
    } catch { /* ignore */ }
  };

  const createDir = async () => {
    if (!newFolderName.trim()) return;
    await fetch('/api/files/create-folder', {
      method: 'POST',
      headers: { 'Content-Type': 'application/json', 'Authorization': `Bearer ${token}` },
      body: JSON.stringify({ parent_path: currentPath, folder_name: newFolderName }),
    });
    setNewFolderName('');
    setShowNewFolder(false);
    fetchDirs(currentPath);
  };

  return (
    <div style={{ position: 'fixed', top: 0, left: 0, width: '100vw', height: '100vh', background: 'rgba(0,0,0,0.55)', backdropFilter: 'blur(6px)', zIndex: 1000, display: 'flex', alignItems: 'center', justifyContent: 'center' }}>
      <div className="g-card" style={{ width: '100%', maxWidth: 540, maxHeight: '90vh', overflowY: 'auto', display: 'flex', flexDirection: 'column', gap: 16 }}>
        {/* Header */}
        <div style={{ display: 'flex', justifyContent: 'space-between', alignItems: 'center' }}>
          <h2 style={{ fontSize: 18, fontWeight: 700, margin: 0, color: 'var(--color-brand-heading)', display: 'flex', alignItems: 'center', gap: 8 }}>
            <div style={{ width: 28, height: 28, borderRadius: 6, background: 'linear-gradient(135deg, #1db954, #191414)', display: 'flex', alignItems: 'center', justifyContent: 'center' }}>
              <FiMusic size={14} color="#fff" />
            </div>
            Add Spotify Download
          </h2>
          <button onClick={onClose} style={{ background: 'none', border: 'none', cursor: 'pointer', color: 'var(--color-brand-text)' }}><FiX size={18} /></button>
        </div>

        {/* URL Input */}
        <div style={{ display: 'flex', flexDirection: 'column', gap: 4 }}>
          <label style={{ fontSize: 11, fontWeight: 700, color: 'var(--color-brand-muted)' }}>Spotify Track or Album Link</label>
          <div style={{ position: 'relative' }}>
            <input
              type="text"
              placeholder="https://open.spotify.com/track/... or /album/..."
              value={url}
              onChange={(e) => handleUrlChange(e.target.value)}
              disabled={isAdding}
              style={{ width: '100%', padding: '10px 14px', paddingRight: isFetching ? 40 : 14, borderRadius: 8, border: '1px solid var(--color-brand-border)', background: 'var(--color-brand-bg)', outline: 'none', color: 'var(--color-brand-heading)', fontSize: 13, fontFamily: 'monospace' }}
            />
            {isFetching && <FiLoader className="spin" style={{ position: 'absolute', right: 14, top: 12, color: '#1db954' }} />}
          </div>
          {fetchError && <span style={{ fontSize: 11, color: '#ef4444' }}>{fetchError}</span>}
        </div>

        {/* Track Preview */}
        {linkType === 'track' && trackData && (
          <div style={{ display: 'flex', gap: 12, background: 'var(--color-brand-bg)', padding: 12, borderRadius: 10, border: '1px solid var(--color-brand-border)' }}>
            {trackData.cover_url && <img src={trackData.cover_url} style={{ width: 64, height: 64, objectFit: 'cover', borderRadius: 6 }} alt="" />}
            <div style={{ minWidth: 0, flex: 1 }}>
              <div style={{ fontSize: 14, fontWeight: 700, color: 'var(--color-brand-heading)', overflow: 'hidden', textOverflow: 'ellipsis', whiteSpace: 'nowrap' }}>{trackData.title}</div>
              <div style={{ fontSize: 11, color: 'var(--color-brand-muted)', marginTop: 2 }}>{trackData.artist} • {trackData.album}</div>
              <div style={{ display: 'flex', gap: 8, marginTop: 4, fontSize: 10, color: 'var(--color-brand-muted)' }}>
                <span>{formatDuration(trackData.duration_ms)}</span>
                <span>•</span>
                <span>{trackData.release_date}</span>
                {trackData.explicit && <span style={{ color: '#ef4444', fontWeight: 700 }}>Explicit</span>}
              </div>
            </div>
          </div>
        )}

        {/* Album Preview */}
        {linkType === 'album' && albumData && (
          <div style={{ background: 'var(--color-brand-bg)', padding: 12, borderRadius: 10, border: '1px solid var(--color-brand-border)' }}>
            <div style={{ display: 'flex', gap: 12, marginBottom: 10 }}>
              {albumData.cover_url && <img src={albumData.cover_url} style={{ width: 72, height: 72, objectFit: 'cover', borderRadius: 6 }} alt="" />}
              <div style={{ minWidth: 0 }}>
                <div style={{ fontSize: 14, fontWeight: 700, color: 'var(--color-brand-heading)' }}>{albumData.name}</div>
                <div style={{ fontSize: 11, color: 'var(--color-brand-muted)', marginTop: 2 }}>{albumData.artist} • {albumData.release_date}</div>
                <div style={{ display: 'flex', alignItems: 'center', gap: 4, marginTop: 6 }}>
                  <FiDisc size={12} color="#1db954" />
                  <span style={{ fontSize: 12, fontWeight: 600, color: '#1db954' }}>{albumData.total_tracks} tracks</span>
                </div>
              </div>
            </div>
            {albumData.tracks && albumData.tracks.length > 0 && (
              <div style={{ maxHeight: 160, overflowY: 'auto', borderTop: '1px solid var(--color-brand-border)', paddingTop: 8 }}>
                {albumData.tracks.map((t, i) => (
                  <div key={t.id || i} style={{ display: 'flex', alignItems: 'center', gap: 8, padding: '4px 0', fontSize: 11, color: 'var(--color-brand-text)' }}>
                    <span style={{ width: 20, textAlign: 'right', color: 'var(--color-brand-muted)', fontWeight: 600 }}>{t.track_number}</span>
                    <span style={{ flex: 1, overflow: 'hidden', textOverflow: 'ellipsis', whiteSpace: 'nowrap' }}>{t.title}</span>
                    <span style={{ color: 'var(--color-brand-muted)', flexShrink: 0 }}>{formatDuration(t.duration_ms)}</span>
                  </div>
                ))}
              </div>
            )}
          </div>
        )}

        {/* Format & Bitrate */}
        {linkType && (
          <div style={{ display: 'flex', gap: 12 }}>
            <div style={{ flex: 1, display: 'flex', flexDirection: 'column', gap: 4 }}>
              <label style={{ fontSize: 11, fontWeight: 700, color: 'var(--color-brand-muted)' }}>Audio Format</label>
              <select
                value={format}
                onChange={(e) => setFormat(e.target.value)}
                style={{ padding: '10px 12px', borderRadius: 8, border: '1px solid var(--color-brand-border)', background: 'var(--color-brand-bg)', color: 'var(--color-brand-heading)', fontSize: 13 }}
              >
                {FORMATS.map(f => <option key={f} value={f}>{f.toUpperCase()}</option>)}
              </select>
            </div>
            <div style={{ flex: 1, display: 'flex', flexDirection: 'column', gap: 4 }}>
              <label style={{ fontSize: 11, fontWeight: 700, color: 'var(--color-brand-muted)' }}>Bitrate</label>
              <select
                value={bitrate}
                onChange={(e) => setBitrate(e.target.value)}
                disabled={format === 'flac' || format === 'wav'}
                style={{ padding: '10px 12px', borderRadius: 8, border: '1px solid var(--color-brand-border)', background: 'var(--color-brand-bg)', color: 'var(--color-brand-heading)', fontSize: 13 }}
              >
                {BITRATES.map(b => <option key={b} value={b}>{b === 'auto' ? 'Auto (Original)' : b}</option>)}
              </select>
            </div>
          </div>
        )}

        {/* Save Directory */}
        {linkType && (
          <div style={{ display: 'flex', flexDirection: 'column', gap: 4 }}>
            <label style={{ fontSize: 11, fontWeight: 700, color: 'var(--color-brand-muted)' }}>Save Directory</label>
            <div style={{ display: 'flex', gap: 8 }}>
              <input type="text" readOnly value={saveDir} style={{ flex: 1, padding: '10px 14px', borderRadius: 8, border: '1px solid var(--color-brand-border)', background: 'var(--color-brand-card)', color: 'var(--color-brand-heading)', outline: 'none' }} />
              <button className="btn" onClick={() => { fetchDirs(saveDir || currentPath); setShowFolderPicker(true); }} style={{ display: 'flex', alignItems: 'center' }}><FiFolder size={16} /></button>
            </div>
          </div>
        )}

        {/* Info Note */}
        {linkType && (
          <div style={{ background: 'rgba(29,185,84,0.06)', border: '1px solid rgba(29,185,84,0.15)', borderRadius: 8, padding: '8px 12px', fontSize: 11, color: '#1db954', display: 'flex', alignItems: 'center', gap: 6 }}>
            <FiInfo size={14} />
            {linkType === 'album'
              ? `All ${albumData?.total_tracks || 0} tracks will be downloaded in parallel as ${format.toUpperCase()} at ${bitrate} into an album subfolder.`
              : `Track will be downloaded as ${format.toUpperCase()} at ${bitrate} with embedded metadata and cover art.`
            }
          </div>
        )}

        {/* Actions */}
        <div style={{ display: 'flex', justifyContent: 'end', gap: 12, marginTop: 4 }}>
          <button className="btn" onClick={onClose} disabled={isAdding}>Cancel</button>
          <button className="btn btn--primary" onClick={handleSubmit} disabled={!linkType || isAdding} style={{ background: linkType ? '#1db954' : undefined, borderColor: linkType ? '#1db954' : undefined }}>
            {isAdding ? 'Adding...' : linkType === 'album' ? `Download ${albumData?.total_tracks || 0} Tracks` : 'Start Download'}
          </button>
        </div>
      </div>

      {/* Folder Picker Sub-modal */}
      {showFolderPicker && (
        <div style={{ position: 'fixed', top: 0, left: 0, width: '100vw', height: '100vh', background: 'rgba(0,0,0,0.4)', zIndex: 1001, display: 'flex', alignItems: 'center', justifyContent: 'center' }}>
          <div className="g-card" style={{ width: '100%', maxWidth: 500, height: 420, display: 'flex', flexDirection: 'column' }}>
            <div style={{ display: 'flex', justifyContent: 'space-between', alignItems: 'center', marginBottom: 12 }}>
              <div>
                <h3 style={{ fontSize: 15, fontWeight: 700, margin: 0, color: 'var(--color-brand-heading)' }}>Select Folder</h3>
                <div style={{ fontSize: 10, color: 'var(--color-brand-muted)', fontFamily: 'monospace', marginTop: 2 }}>{currentPath}</div>
              </div>
              <button onClick={() => setShowFolderPicker(false)} style={{ background: 'none', border: 'none', cursor: 'pointer', color: 'var(--color-brand-text)' }}><FiX size={16} /></button>
            </div>

            <div style={{ display: 'flex', gap: 8, marginBottom: 10 }}>
              <button className="btn btn--sm" onClick={() => { const p = currentPath.split('/'); p.pop(); fetchDirs(p.join('/') || '/'); }} disabled={currentPath === '/'} style={{ display: 'flex', alignItems: 'center', gap: 4 }}><FiChevronLeft /> Up</button>
              <button className="btn btn--sm" onClick={() => setShowNewFolder(true)} style={{ display: 'flex', alignItems: 'center', gap: 4 }}><FiFolderPlus /> New</button>
            </div>

            {showNewFolder && (
              <div style={{ display: 'flex', gap: 6, marginBottom: 8, padding: 6, background: 'var(--color-brand-bg)', borderRadius: 6, border: '1px solid var(--color-brand-border)' }}>
                <input type="text" placeholder="Folder name..." value={newFolderName} onChange={(e) => setNewFolderName(e.target.value)} style={{ flex: 1, padding: '4px 8px', fontSize: 12, borderRadius: 4, border: '1px solid var(--color-brand-border)', background: 'var(--color-brand-card)', color: 'var(--color-brand-heading)', outline: 'none' }} />
                <button className="btn btn--sm btn--primary" onClick={createDir}><FiCheck size={12} /></button>
                <button className="btn btn--sm" onClick={() => setShowNewFolder(false)}><FiX size={12} /></button>
              </div>
            )}

            <div style={{ flex: 1, border: '1px solid var(--color-brand-border)', borderRadius: 8, background: 'var(--color-brand-bg)', overflowY: 'auto', padding: 8 }}>
              {directories.length === 0 ? (
                <div style={{ display: 'flex', alignItems: 'center', justifyContent: 'center', height: '100%', color: 'var(--color-brand-muted)', fontSize: 12 }}>No subdirectories</div>
              ) : (
                <div style={{ display: 'grid', gridTemplateColumns: '1fr 1fr', gap: 6 }}>
                  {directories.map(dir => (
                    <div key={dir} onClick={() => fetchDirs(currentPath === '/' ? `/${dir}` : `${currentPath}/${dir}`)} style={{ display: 'flex', alignItems: 'center', gap: 6, padding: 8, background: 'var(--color-brand-card)', borderRadius: 6, border: '1px solid var(--color-brand-border)', cursor: 'pointer', fontSize: 12, color: 'var(--color-brand-heading)' }}>
                      <FiFolder style={{ color: '#1db954', flexShrink: 0 }} />{dir}
                    </div>
                  ))}
                </div>
              )}
            </div>

            <div style={{ display: 'flex', justifyContent: 'end', gap: 10, marginTop: 10 }}>
              <button className="btn" onClick={() => setShowFolderPicker(false)}>Cancel</button>
              <button className="btn btn--primary" onClick={() => { setSaveDir(currentPath); setShowFolderPicker(false); }}>Select</button>
            </div>
          </div>
        </div>
      )}
    </div>
  );
};
