import React from 'react';
import type { SpotifyJob } from '../../store/jobsStore';
import { FiTrash2, FiDownload, FiRefreshCw, FiXCircle, FiMusic, FiExternalLink } from 'react-icons/fi';
import { showGlobalConfirm } from '../../store/dialogStore';


interface SpotifyJobCardProps {
  job: SpotifyJob;
  token: string;
  onCancel: (id: string) => void;
  onDelete: (id: string, deleteFiles: boolean) => void;
  onRetry: (id: string) => void;
}

const statusColors: Record<string, { bg: string; fg: string }> = {
  pending:       { bg: 'rgba(107,114,128,0.10)', fg: '#6b7280' },
  fetching_meta: { bg: 'rgba(168,85,247,0.10)',  fg: '#a855f7' },
  matching:      { bg: 'rgba(245,158,11,0.10)',   fg: '#f59e0b' },
  downloading:   { bg: 'rgba(59,130,246,0.10)',   fg: '#3b82f6' },
  converting:    { bg: 'rgba(236,72,153,0.10)',   fg: '#ec4899' },
  tagging:       { bg: 'rgba(16,185,129,0.10)',   fg: '#10b981' },
  completed:     { bg: 'rgba(34,197,94,0.10)',    fg: '#22c55e' },
  error:         { bg: 'rgba(239,68,68,0.10)',    fg: '#ef4444' },
};

const statusLabels: Record<string, string> = {
  pending: 'Queued',
  fetching_meta: 'Fetching Metadata',
  matching: 'Matching on YouTube',
  downloading: 'Downloading',
  converting: 'Converting Audio',
  tagging: 'Embedding Tags',
  completed: 'Completed',
  error: 'Error',
};

const formatDuration = (ms: number) => {
  const totalSec = Math.floor(ms / 1000);
  const min = Math.floor(totalSec / 60);
  const sec = totalSec % 60;
  return `${min}:${sec.toString().padStart(2, '0')}`;
};

const formatBytes = (bytes: number) => {
  if (!bytes || bytes === 0) return '—';
  const k = 1024;
  const sizes = ['B', 'KB', 'MB', 'GB'];
  const i = Math.floor(Math.log(bytes) / Math.log(k));
  return parseFloat((bytes / Math.pow(k, i)).toFixed(1)) + ' ' + sizes[i];
};

export const SpotifyJobCard: React.FC<SpotifyJobCardProps> = ({ job, token, onCancel, onDelete, onRetry }) => {
  const [isHovered, setIsHovered] = React.useState(false);
  const [dismissed, setDismissed] = React.useState(false);

  const sc = statusColors[job.status] || statusColors.pending;
  const isActive = ['downloading', 'converting', 'tagging', 'matching', 'fetching_meta'].includes(job.status);
  const isCompleted = job.status === 'completed';
  const isError = job.status === 'error';

  const fileMissing = isCompleted && job.file_exists === false;
  const showOverlay = fileMissing && isHovered && !dismissed;

  // Progress bar gradient based on pipeline phase
  const getProgressGradient = () => {
    switch (job.status) {
      case 'matching':      return 'linear-gradient(90deg, #f59e0b, #fbbf24)';
      case 'downloading':   return 'linear-gradient(90deg, #3b82f6, #60a5fa)';
      case 'converting':    return 'linear-gradient(90deg, #ec4899, #f472b6)';
      case 'tagging':       return 'linear-gradient(90deg, #10b981, #34d399)';
      case 'completed':     return 'linear-gradient(90deg, #22c55e, #4ade80)';
      default:              return 'linear-gradient(90deg, #6b7280, #9ca3af)';
    }
  };

  return (
    <div 
      className="g-card" 
      onMouseEnter={() => setIsHovered(true)}
      onMouseLeave={() => {
        setIsHovered(false);
        setDismissed(false);
      }}
      style={{ 
        display: 'flex', 
        gap: 14, 
        alignItems: 'flex-start', 
        transition: 'all 0.2s',
        position: 'relative',
        ...(fileMissing ? {
          filter: 'grayscale(1) contrast(1.1) brightness(0.8)',
          border: '1px dashed #4b5563',
          background: '#121212',
        } : {})
      }}
    >
      {showOverlay && (
        <div className="missing-file-overlay" style={{
          position: 'absolute',
          top: 0,
          left: 0,
          right: 0,
          bottom: 0,
          background: 'rgba(0, 0, 0, 0.85)',
          zIndex: 50,
          display: 'flex',
          alignItems: 'center',
          justifyContent: 'center',
          gap: 12,
          borderRadius: 8,
        }}>
          <div style={{ display: 'flex', flexDirection: 'column', alignItems: 'center', gap: 10 }}>
            <div style={{ color: '#ef4444', fontSize: 13, fontWeight: 'bold' }}>
              File is missing physically on disk
            </div>
            <div style={{ display: 'flex', gap: 12 }}>
              <button 
                className="btn btn--primary" 
                onClick={(e) => {
                  e.stopPropagation();
                  onDelete(job.id, false);
                }}
                style={{ 
                  background: '#dc2626', 
                  borderColor: '#dc2626',
                  color: '#fff',
                  padding: '6px 12px',
                  fontSize: 12,
                  fontWeight: 'bold',
                  display: 'flex',
                  alignItems: 'center',
                  gap: 6
                }}
              >
                <FiTrash2 size={14} /> Delete Database Record
              </button>
              <button 
                className="btn" 
                onClick={(e) => {
                  e.stopPropagation();
                  setDismissed(true);
                }}
                style={{ 
                  background: '#374151',
                  borderColor: '#4b5563',
                  color: '#fff',
                  padding: '6px 12px',
                  fontSize: 12
                }}
              >
                Cancel / View Info
              </button>
            </div>
          </div>
        </div>
      )}
      {/* Cover Art */}
      <div style={{ position: 'relative', width: 64, height: 64, borderRadius: 8, overflow: 'hidden', flexShrink: 0, background: 'var(--color-brand-bg)' }}>
        {job.cover_url ? (
          <img src={job.cover_url} alt={job.title} style={{ width: '100%', height: '100%', objectFit: 'cover' }} />
        ) : (
          <div style={{ width: '100%', height: '100%', display: 'flex', alignItems: 'center', justifyContent: 'center', background: 'linear-gradient(135deg, #1db954 0%, #191414 100%)' }}>
            <FiMusic size={24} color="#fff" />
          </div>
        )}
        {job.explicit && (
          <div style={{ position: 'absolute', bottom: 3, left: 3, background: 'rgba(0,0,0,0.7)', color: '#fff', fontSize: 8, fontWeight: 800, padding: '1px 4px', borderRadius: 3, letterSpacing: 0.5 }}>E</div>
        )}
      </div>

      {/* Content */}
      <div style={{ flex: 1, minWidth: 0, display: 'flex', flexDirection: 'column', gap: 6 }}>
        {/* Header row */}
        <div style={{ display: 'flex', justifyContent: 'space-between', alignItems: 'flex-start', gap: 12 }}>
          <div style={{ minWidth: 0, flex: 1 }}>
            <div style={{ display: 'flex', alignItems: 'center', gap: 8, flexWrap: 'wrap' }}>
              <span style={{ fontSize: 14, fontWeight: 700, color: 'var(--color-brand-heading)', whiteSpace: 'nowrap', overflow: 'hidden', textOverflow: 'ellipsis', maxWidth: 300 }}>
                {job.title || job.filename}
              </span>
              <span style={{
                fontSize: 9, fontWeight: 700, padding: '2px 8px', borderRadius: 40,
                textTransform: 'uppercase', letterSpacing: 0.5,
                background: sc.bg, color: sc.fg,
              }}>
                {statusLabels[job.status] || job.status}
              </span>
              <span style={{
                fontSize: 9, fontWeight: 600, padding: '2px 6px', borderRadius: 4,
                background: 'rgba(29,185,84,0.1)', color: '#1db954',
                textTransform: 'uppercase',
              }}>
                {job.format} • {job.bitrate}
              </span>
            </div>
            <div style={{ display: 'flex', alignItems: 'center', gap: 10, marginTop: 3, fontSize: 11, color: 'var(--color-brand-muted)', flexWrap: 'wrap' }}>
              <span>{job.artist}</span>
              <span style={{ opacity: 0.4 }}>•</span>
              <span>{job.album}</span>
              <span style={{ opacity: 0.4 }}>•</span>
              <span>{formatDuration(job.duration_ms)}</span>
              {job.total_bytes > 0 && (
                <>
                  <span style={{ opacity: 0.4 }}>•</span>
                  <span>{formatBytes(job.total_bytes)}</span>
                </>
              )}
            </div>
          </div>

          {/* Actions */}
          <div style={{ display: 'flex', gap: 6, flexShrink: 0 }}>
            {isActive && (
              <button className="btn btn--sm" onClick={() => onCancel(job.id)} title="Cancel" style={{ display: 'flex', alignItems: 'center', gap: 4 }}>
                <FiXCircle size={12} /> Cancel
              </button>
            )}
            {isError && (
              <button className="btn btn--sm" onClick={() => onRetry(job.id)} title="Retry" style={{ display: 'flex', alignItems: 'center', gap: 4, color: '#f59e0b', borderColor: '#f59e0b' }}>
                <FiRefreshCw size={12} /> Retry
              </button>
            )}
            {isCompleted && (
              <a
                href={`/api/files/stream?path=${encodeURIComponent(`${job.save_directory}/${job.filename}`)}&download=true&token=${encodeURIComponent(token || '')}`}
                download
                className="btn btn--sm btn--primary"
                style={{ display: 'inline-flex', alignItems: 'center', gap: 4, textDecoration: 'none', height: 30 }}
              >
                <FiDownload size={12} /> Download
              </a>
            )}
            <button
              className="btn btn--sm"
              onClick={async () => { if (await showGlobalConfirm('Remove this job?', { title: 'Remove Job', variant: 'warning' })) onDelete(job.id, isCompleted); }}
              title="Delete"
              style={{ color: '#ef4444', borderColor: 'rgba(239,68,68,0.3)' }}
            >
              <FiTrash2 size={12} />
            </button>
          </div>
        </div>

        {/* Progress bar */}
        {(isActive || isCompleted) && (
          <div style={{ display: 'flex', flexDirection: 'column', gap: 3 }}>
            <div style={{ display: 'flex', justifyContent: 'space-between', fontSize: 10, color: 'var(--color-brand-muted)' }}>
              <span>{job.progress.toFixed(1)}%</span>
              {job.speed > 0 && <span style={{ color: sc.fg, fontWeight: 600 }}>{job.speed.toFixed(1)} MB/s</span>}
            </div>
            <div style={{ height: 5, background: 'rgba(0,0,0,0.08)', borderRadius: 3, overflow: 'hidden' }}>
              <div style={{
                width: `${Math.min(job.progress, 100)}%`,
                height: '100%',
                background: getProgressGradient(),
                borderRadius: 3,
                transition: 'width 0.4s ease',
              }} />
            </div>
          </div>
        )}

        {/* Error message */}
        {job.error_message && (
          <div style={{ background: 'rgba(239,68,68,0.05)', borderLeft: '3px solid #ef4444', padding: '5px 10px', borderRadius: 4, fontSize: 11, color: '#ef4444', marginTop: 2 }}>
            {job.error_message}
          </div>
        )}

        {/* YouTube link if matched */}
        {job.youtube_url && (
          <a href={job.youtube_url} target="_blank" rel="noreferrer" style={{ fontSize: 10, color: 'var(--color-brand-muted)', display: 'flex', alignItems: 'center', gap: 4, textDecoration: 'none', opacity: 0.6 }}>
            <FiExternalLink size={10} /> Matched: {job.youtube_url.substring(0, 50)}...
          </a>
        )}
      </div>
    </div>
  );
};
