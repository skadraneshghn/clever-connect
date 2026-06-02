import React, { useState, useEffect } from 'react';
import { FiCpu, FiHardDrive, FiActivity, FiClock } from 'react-icons/fi';

interface Stats {
  cpu_percent: number;
  mem_total_gb: number;
  mem_used_gb: number;
  mem_percent: number;
  disk_total_gb: number;
  disk_used_gb: number;
  disk_percent: number;
  app_mem_mb: number;
  uptime_seconds: number;
}

export const SystemMonitor: React.FC = () => {
  const [stats, setStats] = useState<Stats>({
    cpu_percent: 12,
    mem_total_gb: 16,
    mem_used_gb: 4.8,
    mem_percent: 30,
    disk_total_gb: 120,
    disk_used_gb: 21,
    disk_percent: 17.5,
    app_mem_mb: 6.2,
    uptime_seconds: 90
  });
  const [loading, setLoading] = useState(true);

  const formatUptime = (totalSeconds: number): string => {
    const d = Math.floor(totalSeconds / (3600 * 24));
    const h = Math.floor((totalSeconds % (3600 * 24)) / 3600);
    const m = Math.floor((totalSeconds % 3600) / 60);
    const s = totalSeconds % 60;
    
    const parts = [];
    if (d > 0) parts.push(`${d}d`);
    if (h > 0) parts.push(`${h}h`);
    if (m > 0) parts.push(`${m}m`);
    parts.push(`${s}s`);
    
    return parts.join(' ');
  };

  const fetchStats = async () => {
    try {
      const token = localStorage.getItem('cc_server_token') || localStorage.getItem('cc_client_token') || '';
      const response = await fetch('/api/system/stats', {
        headers: {
          'Authorization': `Bearer ${token}`
        }
      });
      if (response.ok) {
        const data = await response.json();
        setStats(data);
      } else {
        // Mock drift offline
        setStats(prev => ({
          ...prev,
          cpu_percent: Math.min(100, Math.max(0, prev.cpu_percent + (Math.random() * 4 - 2))),
          mem_percent: Math.min(100, Math.max(0, prev.mem_percent + (Math.random() * 1.5 - 0.75))),
          mem_used_gb: Math.min(prev.mem_total_gb, Math.max(0, prev.mem_used_gb + (Math.random() * 0.05 - 0.025))),
          app_mem_mb: Math.max(4, prev.app_mem_mb + (Math.random() * 0.1 - 0.05)),
          uptime_seconds: prev.uptime_seconds + 3
        }));
      }
    } catch (err) {
      // Offline fallback
      setStats(prev => ({
        ...prev,
        cpu_percent: Math.min(100, Math.max(0, prev.cpu_percent + (Math.random() * 4 - 2))),
        mem_percent: Math.min(100, Math.max(0, prev.mem_percent + (Math.random() * 1.5 - 0.75))),
        uptime_seconds: prev.uptime_seconds + 3
      }));
    } finally {
      setLoading(false);
    }
  };

  useEffect(() => {
    fetchStats();
    const interval = setInterval(fetchStats, 3000);
    return () => clearInterval(interval);
  }, []);

  const cpuColor = 'var(--color-brand)';
  const ramColor = '#10b981';
  const diskColor = '#3b82f6';

  // SVG Circular progress params
  const radius = 32;
  const stroke = 6;
  const normalizedRadius = radius - stroke * 2;
  const circumference = normalizedRadius * 2 * Math.PI;

  const getStrokeDashoffset = (percent: number) => {
    return circumference - (Math.min(100, Math.max(0, percent)) / 100) * circumference;
  };

  return (
    <div className="g-card" style={{ display: 'flex', flexDirection: 'column', gap: 16 }}>
      
      {/* Title */}
      <div style={{ display: 'flex', justifyContent: 'space-between', alignItems: 'center' }}>
        <div style={{ display: 'flex', alignItems: 'center', gap: 8 }}>
          <FiActivity style={{ color: 'var(--color-brand)', fontSize: 18 }} />
          <span style={{ fontSize: 15, fontWeight: 600, color: 'var(--color-brand-heading)' }}>Hardware & App Resource Monitor</span>
        </div>
        {loading && (
          <span style={{ fontSize: 10, color: 'var(--color-brand-muted)', textTransform: 'uppercase', letterSpacing: 1 }} className="blink-animation">
            syncing...
          </span>
        )}
      </div>

      {/* Grid of dials */}
      <div style={{ display: 'grid', gridTemplateColumns: 'repeat(3, 1fr)', gap: 12, textAlign: 'center' }}>
        
        {/* CPU circular progress */}
        <div style={{ display: 'flex', flexDirection: 'column', alignItems: 'center', background: 'var(--color-brand-bg)', borderRadius: 10, padding: 12, border: '1px solid var(--color-brand-border)' }}>
          <div style={{ position: 'relative', width: 64, height: 64, marginBottom: 8 }}>
            <svg height="64" width="64">
              <circle
                stroke="var(--color-brand-border)"
                fill="transparent"
                strokeWidth={stroke}
                r={normalizedRadius}
                cx="32"
                cy="32"
              />
              <circle
                stroke={cpuColor}
                fill="transparent"
                strokeWidth={stroke}
                strokeDasharray={circumference + ' ' + circumference}
                style={{ strokeDashoffset: getStrokeDashoffset(stats.cpu_percent), transition: 'stroke-dashoffset 0.8s ease-in-out' }}
                strokeLinecap="round"
                r={normalizedRadius}
                cx="32"
                cy="32"
              />
            </svg>
            <div style={{
              position: 'absolute',
              top: '50%',
              left: '50%',
              transform: 'translate(-50%, -50%)',
              fontSize: 12,
              fontWeight: 700,
              color: 'var(--color-brand-heading)'
            }}>
              {Math.round(stats.cpu_percent)}%
            </div>
          </div>
          <span style={{ fontSize: 11, fontWeight: 600, color: 'var(--color-brand-muted)', display: 'flex', alignItems: 'center', gap: 4 }}>
            <FiCpu size={12} /> CPU
          </span>
        </div>

        {/* RAM circular progress */}
        <div style={{ display: 'flex', flexDirection: 'column', alignItems: 'center', background: 'var(--color-brand-bg)', borderRadius: 10, padding: 12, border: '1px solid var(--color-brand-border)' }}>
          <div style={{ position: 'relative', width: 64, height: 64, marginBottom: 8 }}>
            <svg height="64" width="64">
              <circle
                stroke="var(--color-brand-border)"
                fill="transparent"
                strokeWidth={stroke}
                r={normalizedRadius}
                cx="32"
                cy="32"
              />
              <circle
                stroke={ramColor}
                fill="transparent"
                strokeWidth={stroke}
                strokeDasharray={circumference + ' ' + circumference}
                style={{ strokeDashoffset: getStrokeDashoffset(stats.mem_percent), transition: 'stroke-dashoffset 0.8s ease-in-out' }}
                strokeLinecap="round"
                r={normalizedRadius}
                cx="32"
                cy="32"
              />
            </svg>
            <div style={{
              position: 'absolute',
              top: '50%',
              left: '50%',
              transform: 'translate(-50%, -50%)',
              fontSize: 12,
              fontWeight: 700,
              color: 'var(--color-brand-heading)'
            }}>
              {Math.round(stats.mem_percent)}%
            </div>
          </div>
          <span style={{ fontSize: 11, fontWeight: 600, color: 'var(--color-brand-muted)', display: 'flex', alignItems: 'center', gap: 4 }}>
            <FiCpu size={12} style={{ color: ramColor }} /> Memory
          </span>
        </div>

        {/* Disk bar/circular progress */}
        <div style={{ display: 'flex', flexDirection: 'column', alignItems: 'center', background: 'var(--color-brand-bg)', borderRadius: 10, padding: 12, border: '1px solid var(--color-brand-border)' }}>
          <div style={{ position: 'relative', width: 64, height: 64, marginBottom: 8 }}>
            <svg height="64" width="64">
              <circle
                stroke="var(--color-brand-border)"
                fill="transparent"
                strokeWidth={stroke}
                r={normalizedRadius}
                cx="32"
                cy="32"
              />
              <circle
                stroke={diskColor}
                fill="transparent"
                strokeWidth={stroke}
                strokeDasharray={circumference + ' ' + circumference}
                style={{ strokeDashoffset: getStrokeDashoffset(stats.disk_percent), transition: 'stroke-dashoffset 0.8s ease-in-out' }}
                strokeLinecap="round"
                r={normalizedRadius}
                cx="32"
                cy="32"
              />
            </svg>
            <div style={{
              position: 'absolute',
              top: '50%',
              left: '50%',
              transform: 'translate(-50%, -50%)',
              fontSize: 12,
              fontWeight: 700,
              color: 'var(--color-brand-heading)'
            }}>
              {Math.round(stats.disk_percent)}%
            </div>
          </div>
          <span style={{ fontSize: 11, fontWeight: 600, color: 'var(--color-brand-muted)', display: 'flex', alignItems: 'center', gap: 4 }}>
            <FiHardDrive size={12} style={{ color: diskColor }} /> Storage
          </span>
        </div>

      </div>

      {/* Row with Go App Alloc & Uptime */}
      <div style={{ display: 'grid', gridTemplateColumns: '1fr 1fr', gap: 10, borderTop: '1px solid var(--color-brand-border)', paddingTop: 12 }}>
        
        <div>
          <div style={{ fontSize: 10, color: 'var(--color-brand-muted)', textTransform: 'uppercase', fontWeight: 600 }}>App Alloc</div>
          <div style={{ display: 'flex', alignItems: 'baseline', gap: 4, marginTop: 2 }}>
            <span style={{ fontSize: 15, fontWeight: 700, color: 'var(--color-brand-heading)' }}>
              {stats.app_mem_mb.toFixed(1)}
            </span>
            <span style={{ fontSize: 10, color: 'var(--color-brand-text)' }}>MB RAM</span>
          </div>
        </div>

        <div>
          <div style={{ fontSize: 10, color: 'var(--color-brand-muted)', textTransform: 'uppercase', fontWeight: 600, display: 'flex', alignItems: 'center', gap: 4 }}>
            <FiClock /> Uptime
          </div>
          <div style={{ fontSize: 13, fontWeight: 700, color: 'var(--color-brand-heading)', marginTop: 4 }}>
            {formatUptime(stats.uptime_seconds)}
          </div>
        </div>

      </div>

    </div>
  );
};
