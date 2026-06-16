import React, { useEffect } from 'react';
import { FiEdit, FiTrash2, FiRefreshCw, FiGlobe, FiDatabase, FiCpu, FiTrendingUp } from 'react-icons/fi';
import { useCloudflareStore } from '../../store/cloudflareStore';
import type { CloudflareAccount, CloudflareStats } from '../../store/cloudflareStore';

interface CloudflareStatsCardProps {
  account: CloudflareAccount;
  onEdit: () => void;
  onDelete: () => void;
}

const formatBytes = (bytes: number) => {
  if (!bytes || bytes === 0) return '0 MB';
  const k = 1024;
  const sizes = ['Bytes', 'KB', 'MB', 'GB', 'TB'];
  const i = Math.floor(Math.log(bytes) / Math.log(k));
  return parseFloat((bytes / Math.pow(k, i)).toFixed(1)) + ' ' + sizes[i];
};

const formatNumber = (num: number) => {
  if (!num || num === 0) return '0';
  if (num >= 1000000) return (num / 1000000).toFixed(1) + 'M';
  if (num >= 1000) return (num / 1000).toFixed(1) + 'K';
  return num;
};

export const CloudflareStatsCard: React.FC<CloudflareStatsCardProps> = ({ account, onEdit, onDelete }) => {
  const { stats, fetchStats } = useCloudflareStore();
  const accountStats: CloudflareStats | undefined = stats[account.id];

  useEffect(() => {
    fetchStats(account.id);
  }, [account.id, fetchStats]);

  const cacheHitRate = accountStats && accountStats.total_bandwidth > 0
    ? (accountStats.cached_bandwidth / accountStats.total_bandwidth) * 100
    : 0;

  const requestCacheRate = accountStats && accountStats.total_requests > 0
    ? (accountStats.cached_requests / accountStats.total_requests) * 100
    : 0;

  return (
    <div className="g-card" style={{ display: 'flex', flexDirection: 'column', gap: 16 }}>
      {/* Title block */}
      <div style={{ display: 'flex', justifyContent: 'space-between', alignItems: 'start' }}>
        <div>
          <div style={{ display: 'flex', alignItems: 'center', gap: 8 }}>
            <span style={{ fontSize: 15, fontWeight: 700, color: 'var(--color-brand-heading)' }}>
              {account.account_name}
            </span>
            <span className={`badge ${account.status === 'active' ? 'badge--success' : 'badge--error'}`} style={{ fontSize: 10, padding: '2px 8px', borderRadius: 6, background: account.status === 'active' ? '#eefbf3' : '#fef2f2', color: account.status === 'active' ? '#15803d' : '#b91c1c', fontWeight: 600 }}>
              {account.status === 'active' ? 'Connected' : 'Token Error'}
            </span>
          </div>
          <div style={{ fontSize: 11, fontFamily: 'monospace', color: 'var(--color-brand-muted)', marginTop: 4 }}>
            ID: {account.account_id}
          </div>
        </div>
        
        {/* Buttons */}
        <div style={{ display: 'flex', gap: 6 }}>
          <button className="g-card__icon-btn" title="Refresh metrics" onClick={() => fetchStats(account.id)} style={{ border: '1px solid var(--color-brand-border)', borderRadius: 6, width: 28, height: 28, display: 'flex', alignItems: 'center', justifyContent: 'center', cursor: 'pointer', background: 'var(--color-brand-card)' }}>
            <FiRefreshCw size={12} />
          </button>
          <button className="g-card__icon-btn" title="Edit account" onClick={onEdit} style={{ border: '1px solid var(--color-brand-border)', borderRadius: 6, width: 28, height: 28, display: 'flex', alignItems: 'center', justifyContent: 'center', cursor: 'pointer', background: 'var(--color-brand-card)' }}>
            <FiEdit size={12} />
          </button>
          <button className="g-card__icon-btn" title="Delete account" onClick={onDelete} style={{ border: '1px solid var(--color-brand-border)', borderRadius: 6, width: 28, height: 28, display: 'flex', alignItems: 'center', justifyContent: 'center', cursor: 'pointer', background: 'var(--color-brand-card)', color: 'var(--color-brand-red)' }}>
            <FiTrash2 size={12} />
          </button>
        </div>
      </div>

      {/* Grid statistics */}
      {!accountStats ? (
        <div style={{ display: 'flex', alignItems: 'center', justifyContent: 'center', height: 120, color: 'var(--color-brand-muted)', fontSize: 12, gap: 8 }}>
          <FiRefreshCw className="spin" size={14} /> Loading metrics...
        </div>
      ) : (
        <div style={{ display: 'flex', flexDirection: 'column', gap: 14 }}>
          {/* Main Grid */}
          <div style={{ display: 'grid', gridTemplateColumns: '1fr 1fr', gap: 10 }}>
            {/* Zones Count */}
            <div style={{ background: 'var(--color-brand-bg)', borderRadius: 8, padding: '10px 12px', border: '1px solid var(--color-brand-border)' }}>
              <div style={{ display: 'flex', alignItems: 'center', gap: 6, fontSize: 11, color: 'var(--color-brand-muted)', fontWeight: 600 }}>
                <FiGlobe size={12} /> ZONES
              </div>
              <div style={{ fontSize: 20, fontWeight: 700, color: 'var(--color-brand-heading)', marginTop: 4 }}>
                {accountStats.total_zones}
              </div>
              <div style={{ fontSize: 10, color: 'var(--color-brand-text)', marginTop: 2 }}>
                {accountStats.active_zones} Active • {accountStats.pending_zones} Pending
              </div>
            </div>

            {/* Workers Count */}
            <div style={{ background: 'var(--color-brand-bg)', borderRadius: 8, padding: '10px 12px', border: '1px solid var(--color-brand-border)' }}>
              <div style={{ display: 'flex', alignItems: 'center', gap: 6, fontSize: 11, color: 'var(--color-brand-muted)', fontWeight: 600 }}>
                <FiCpu size={12} /> WORKERS
              </div>
              <div style={{ fontSize: 20, fontWeight: 700, color: 'var(--color-brand-heading)', marginTop: 4 }}>
                {accountStats.worker_scripts}
              </div>
              <div style={{ fontSize: 10, color: 'var(--color-brand-text)', marginTop: 2 }}>
                Deployed scripts count
              </div>
            </div>
          </div>

          {/* Caching analytics */}
          <div style={{ borderTop: '1px solid var(--color-brand-border)', paddingTop: 12, display: 'flex', flexDirection: 'column', gap: 12 }}>
            
            {/* Bandwidth bar */}
            <div>
              <div style={{ display: 'flex', justifyContent: 'space-between', fontSize: 11, fontWeight: 600, color: 'var(--color-brand-muted)', marginBottom: 4 }}>
                <div style={{ display: 'flex', alignItems: 'center', gap: 4 }}>
                  <FiDatabase size={11} /> BANDWIDTH CACHED (30D)
                </div>
                <span style={{ color: 'var(--color-brand)' }}>{cacheHitRate.toFixed(1)}%</span>
              </div>
              <div style={{ height: 6, background: 'var(--color-brand-bg)', borderRadius: 3, overflow: 'hidden' }}>
                <div style={{ height: '100%', background: 'linear-gradient(90deg, var(--color-brand), #ff9f43)', width: `${cacheHitRate}%`, borderRadius: 3 }} />
              </div>
              <div style={{ display: 'flex', justifyContent: 'space-between', fontSize: 10, color: 'var(--color-brand-text)', marginTop: 4 }}>
                <span>Cached: {formatBytes(accountStats.cached_bandwidth)}</span>
                <span>Total: {formatBytes(accountStats.total_bandwidth)}</span>
              </div>
            </div>

            {/* Requests bar */}
            <div>
              <div style={{ display: 'flex', justifyContent: 'space-between', fontSize: 11, fontWeight: 600, color: 'var(--color-brand-muted)', marginBottom: 4 }}>
                <div style={{ display: 'flex', alignItems: 'center', gap: 4 }}>
                  <FiTrendingUp size={11} /> REQUESTS CACHED (30D)
                </div>
                <span style={{ color: 'var(--color-brand)' }}>{requestCacheRate.toFixed(1)}%</span>
              </div>
              <div style={{ height: 6, background: 'var(--color-brand-bg)', borderRadius: 3, overflow: 'hidden' }}>
                <div style={{ height: '100%', background: 'linear-gradient(90deg, var(--color-brand), #ff9f43)', width: `${requestCacheRate}%`, borderRadius: 3 }} />
              </div>
              <div style={{ display: 'flex', justifyContent: 'space-between', fontSize: 10, color: 'var(--color-brand-text)', marginTop: 4 }}>
                <span>Cached: {formatNumber(accountStats.cached_requests)}</span>
                <span>Total: {formatNumber(accountStats.total_requests)}</span>
              </div>
            </div>

          </div>
        </div>
      )}
    </div>
  );
};
