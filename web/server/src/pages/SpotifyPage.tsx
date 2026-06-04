import React, { useState, useEffect, useMemo } from 'react';
import { useAuthStore } from '../store/authStore';
import { useJobsStore } from '../store/jobsStore';
import { SpotifyJobCard } from '../components/molecules/SpotifyJobCard';
import { SpotifyAddModal } from '../components/molecules/SpotifyAddModal';
import { SpotifyConfigModal } from '../components/molecules/SpotifyConfigModal';
import { FiPlus, FiSettings, FiMusic, FiCheckCircle, FiAlertCircle, FiLoader, FiTrash2 } from 'react-icons/fi';

interface SpotifyConfig {
  default_save_path: string;
  default_format: string;
  default_bitrate: string;
  max_concurrent: number;
  client_id: string;
  client_secret: string;
}

export const SpotifyPage: React.FC = () => {
  const { token } = useAuthStore();
  const jobs = useJobsStore(state => state.spotifyJobs);
  const initWebSocket = useJobsStore(state => state.initWebSocket);
  const sendAction = useJobsStore(state => state.sendAction);

  const [showAddModal, setShowAddModal] = useState(false);
  const [showConfigModal, setShowConfigModal] = useState(false);
  const [filter, setFilter] = useState<'all' | 'active' | 'completed' | 'error'>('all');

  const [config, setConfig] = useState<SpotifyConfig>({
    default_save_path: './downloads/spotify/audios',
    default_format: 'mp3',
    default_bitrate: '320k',
    max_concurrent: 3,
    client_id: '',
    client_secret: '',
  });

  const fetchConfig = async () => {
    try {
      const res = await fetch('/api/spotify/config', {
        headers: { 'Authorization': `Bearer ${token}` },
      });
      if (res.ok) {
        const data = await res.json();
        setConfig(prev => ({ ...prev, ...data }));
      }
    } catch { /* ignore */ }
  };

  useEffect(() => {
    if (token) {
      const close = initWebSocket(token);
      fetchConfig();
      return () => close();
    }
  }, [token, initWebSocket]);

  const handleCancel = (id: string) => sendAction({ action: 'cancel_spotify', job_id: id });
  const handleDelete = (id: string, deleteFiles: boolean) => sendAction({ action: 'delete_spotify', job_id: id, delete_files: deleteFiles });
  const handleRetry = (id: string) => sendAction({ action: 'retry_spotify', job_id: id });

  // Filter jobs
  const filteredJobs = useMemo(() => {
    switch (filter) {
      case 'active':
        return jobs.filter(j => ['pending', 'fetching_meta', 'matching', 'downloading', 'converting', 'tagging'].includes(j.status));
      case 'completed':
        return jobs.filter(j => j.status === 'completed');
      case 'error':
        return jobs.filter(j => j.status === 'error');
      default:
        return jobs;
    }
  }, [jobs, filter]);

  // Stats
  const stats = useMemo(() => ({
    total: jobs.length,
    active: jobs.filter(j => ['downloading', 'converting', 'tagging', 'matching', 'fetching_meta', 'pending'].includes(j.status)).length,
    completed: jobs.filter(j => j.status === 'completed').length,
    errors: jobs.filter(j => j.status === 'error').length,
  }), [jobs]);

  // Check if API credentials are configured
  const hasCredentials = config.client_id && config.client_secret;

  return (
    <div>
      {/* Header */}
      <div style={{ display: 'flex', justifyContent: 'space-between', alignItems: 'flex-start', marginBottom: 24 }}>
        <div>
          <h1 style={{ fontSize: 22, fontWeight: 700, color: 'var(--color-brand-heading)', margin: 0, display: 'flex', alignItems: 'center', gap: 10 }}>
            <div style={{ width: 32, height: 32, borderRadius: 8, background: 'linear-gradient(135deg, #1db954, #191414)', display: 'flex', alignItems: 'center', justifyContent: 'center' }}>
              <FiMusic size={16} color="#fff" />
            </div>
            Spotify Downloader
          </h1>
          <p style={{ fontSize: 12, color: 'var(--color-brand-text)', margin: '6px 0 0 42px', lineHeight: 1.4 }}>
            Download Spotify tracks and albums in any format with automatic metadata and cover art embedding.
          </p>
        </div>
        <div style={{ display: 'flex', gap: 10, flexShrink: 0 }}>
          <button
            className="btn btn--primary"
            onClick={() => hasCredentials ? setShowAddModal(true) : setShowConfigModal(true)}
            style={{ display: 'flex', alignItems: 'center', gap: 6, background: '#1db954', borderColor: '#1db954' }}
          >
            <FiPlus /> {hasCredentials ? 'Add Track' : 'Configure API'}
          </button>
          <button className="btn" onClick={() => setShowConfigModal(true)} style={{ display: 'flex', alignItems: 'center', gap: 6 }}>
            <FiSettings /> Settings
          </button>
        </div>
      </div>

      {/* API Not Configured Warning */}
      {!hasCredentials && (
        <div className="g-card" style={{ marginBottom: 20, padding: '16px 20px', border: '1px solid rgba(245,158,11,0.3)', background: 'rgba(245,158,11,0.04)' }}>
          <div style={{ display: 'flex', alignItems: 'center', gap: 10 }}>
            <FiAlertCircle size={20} color="#f59e0b" />
            <div>
              <div style={{ fontSize: 14, fontWeight: 700, color: 'var(--color-brand-heading)' }}>API Credentials Required</div>
              <div style={{ fontSize: 12, color: 'var(--color-brand-muted)', marginTop: 2 }}>
                Add your Spotify Client ID and Secret in{' '}
                <span onClick={() => setShowConfigModal(true)} style={{ color: '#1db954', cursor: 'pointer', fontWeight: 600, textDecoration: 'underline' }}>Settings</span>
                {' '}to start downloading. Get free credentials from{' '}
                <a href="https://developer.spotify.com/dashboard" target="_blank" rel="noreferrer" style={{ color: '#1db954' }}>developer.spotify.com</a>.
              </div>
            </div>
          </div>
        </div>
      )}

      {/* Stats & Filter Bar */}
      {jobs.length > 0 && (
        <div style={{ display: 'flex', justifyContent: 'space-between', alignItems: 'center', marginBottom: 16, flexWrap: 'wrap', gap: 10 }}>
          {/* Mini Stats */}
          <div style={{ display: 'flex', gap: 16 }}>
            {[
              { label: 'Active', value: stats.active, icon: FiLoader, color: '#3b82f6' },
              { label: 'Done', value: stats.completed, icon: FiCheckCircle, color: '#22c55e' },
              { label: 'Errors', value: stats.errors, icon: FiAlertCircle, color: '#ef4444' },
            ].map(s => (
              <div key={s.label} style={{ display: 'flex', alignItems: 'center', gap: 6, fontSize: 12, color: 'var(--color-brand-muted)' }}>
                <s.icon size={13} color={s.color} />
                <span style={{ fontWeight: 600 }}>{s.value}</span>
                <span>{s.label}</span>
              </div>
            ))}
          </div>

          {/* Filter Tabs */}
          <div style={{ display: 'flex', gap: 4, background: 'var(--color-brand-bg)', borderRadius: 8, padding: 3, border: '1px solid var(--color-brand-border)' }}>
            {(['all', 'active', 'completed', 'error'] as const).map(f => (
              <button
                key={f}
                onClick={() => setFilter(f)}
                style={{
                  padding: '5px 12px', borderRadius: 6, border: 'none', cursor: 'pointer',
                  fontSize: 11, fontWeight: 600, textTransform: 'capitalize',
                  background: filter === f ? 'var(--color-brand-card)' : 'transparent',
                  color: filter === f ? 'var(--color-brand-heading)' : 'var(--color-brand-muted)',
                  boxShadow: filter === f ? '0 1px 3px rgba(0,0,0,0.08)' : 'none',
                  transition: 'all 0.15s',
                }}
              >
                {f} {f === 'all' ? `(${stats.total})` : ''}
              </button>
            ))}
          </div>
        </div>
      )}

      {/* Jobs List */}
      <div style={{ display: 'flex', flexDirection: 'column', gap: 12 }}>
        {filteredJobs.length === 0 ? (
          <div className="g-card" style={{ padding: '60px 20px', textAlign: 'center', color: 'var(--color-brand-muted)' }}>
            <div style={{ width: 56, height: 56, borderRadius: 14, background: 'linear-gradient(135deg, #1db954, #191414)', display: 'flex', alignItems: 'center', justifyContent: 'center', margin: '0 auto 16px' }}>
              <FiMusic size={26} color="#fff" />
            </div>
            <div style={{ fontSize: 16, fontWeight: 700, color: 'var(--color-brand-heading)' }}>
              {filter !== 'all' ? `No ${filter} downloads` : 'No Spotify Downloads'}
            </div>
            <p style={{ fontSize: 12, maxWidth: 340, margin: '8px auto 0', lineHeight: 1.5 }}>
              {hasCredentials
                ? 'Paste a Spotify track or album link to download high-quality audio with full metadata.'
                : 'Configure your Spotify API credentials in Settings to get started.'
              }
            </p>
          </div>
        ) : (
          filteredJobs.map(job => (
            <SpotifyJobCard
              key={job.id}
              job={job}
              token={token || ''}
              onCancel={handleCancel}
              onDelete={handleDelete}
              onRetry={handleRetry}
            />
          ))
        )}
      </div>

      {/* Modals */}
      <SpotifyAddModal
        show={showAddModal}
        onClose={() => setShowAddModal(false)}
        token={token || ''}
        defaultSaveDir={config.default_save_path}
        defaultFormat={config.default_format}
        defaultBitrate={config.default_bitrate}
      />

      <SpotifyConfigModal
        show={showConfigModal}
        onClose={() => setShowConfigModal(false)}
        token={token || ''}
        onSaved={fetchConfig}
      />
    </div>
  );
};
