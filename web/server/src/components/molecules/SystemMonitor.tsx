import React, { useState } from 'react';
import { useServerStore } from '../../store/dashboardStore';
import { FiCpu, FiHardDrive, FiActivity, FiClock, FiActivity as FiNet, FiThermometer, FiChevronDown, FiChevronUp } from 'react-icons/fi';

export const SystemMonitor: React.FC = () => {
  const state = useServerStore();
  const [showCores, setShowCores] = useState(false);

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

  const formatSpeed = (bytesPerSec: number): string => {
    if (!bytesPerSec || bytesPerSec === 0 || isNaN(bytesPerSec)) return '0 B/s';
    const k = 1024;
    const sizes = ['B/s', 'KB/s', 'MB/s', 'GB/s'];
    const i = Math.floor(Math.log(bytesPerSec) / Math.log(k));
    const clampedIndex = Math.min(i, sizes.length - 1);
    return parseFloat((bytesPerSec / Math.pow(k, clampedIndex)).toFixed(1)) + ' ' + sizes[clampedIndex];
  };

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

  // Safe accessors
  const cpuPercent = state.cpu ?? 0;
  const cpuCores = state.cpu_cores_percent ?? [];
  const cpuMhz = state.cpu_mhz ?? 0;
  const cpuTemp = state.cpu_temp ?? 0;
  const memPercent = state.memory ?? 0;
  const memUsed = state.mem_used_gb ?? 0;
  const memTotal = state.mem_total_gb ?? 0;
  const memFree = state.mem_free_gb ?? 0;
  const swapPercent = state.swap_percent ?? 0;
  const swapUsed = state.swap_used_gb ?? 0;
  const swapTotal = state.swap_total_gb ?? 0;
  const diskPercent = state.disk ?? 0;
  const diskUsed = state.disk_used_gb ?? 0;
  const diskTotal = state.disk_total_gb ?? 0;
  const diskFree = state.disk_free_gb ?? 0;
  const diskReadSpeed = state.disk_read_bytes_sec ?? 0;
  const diskWriteSpeed = state.disk_write_bytes_sec ?? 0;
  const netRecvSpeed = state.net_recv_bytes_sec ?? 0;
  const netSentSpeed = state.net_sent_bytes_sec ?? 0;

  return (
    <div className="g-card" style={{ display: 'flex', flexDirection: 'column', gap: 16 }}>
      
      {/* Title */}
      <div style={{ display: 'flex', justifyContent: 'space-between', alignItems: 'center' }}>
        <div style={{ display: 'flex', alignItems: 'center', gap: 8 }}>
          <FiActivity style={{ color: 'var(--color-brand)', fontSize: 18 }} />
          <span style={{ fontSize: 15, fontWeight: 600, color: 'var(--color-brand-heading)' }}>Observability Diagnostics</span>
        </div>
        {state.wsConnected && (
          <span style={{ fontSize: 9, color: 'var(--color-brand-success)', textTransform: 'uppercase', letterSpacing: 1, fontWeight: 700 }} className="blink-animation">
            ● live
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
                style={{ strokeDashoffset: getStrokeDashoffset(cpuPercent), transition: 'stroke-dashoffset 0.8s ease-in-out' }}
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
              {Math.round(cpuPercent)}%
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
                style={{ strokeDashoffset: getStrokeDashoffset(memPercent), transition: 'stroke-dashoffset 0.8s ease-in-out' }}
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
              {Math.round(memPercent)}%
            </div>
          </div>
          <span style={{ fontSize: 11, fontWeight: 600, color: 'var(--color-brand-muted)', display: 'flex', alignItems: 'center', gap: 4 }}>
            <FiCpu size={12} style={{ color: ramColor }} /> RAM
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
                style={{ strokeDashoffset: getStrokeDashoffset(diskPercent), transition: 'stroke-dashoffset 0.8s ease-in-out' }}
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
              {Math.round(diskPercent)}%
            </div>
          </div>
          <span style={{ fontSize: 11, fontWeight: 600, color: 'var(--color-brand-muted)', display: 'flex', alignItems: 'center', gap: 4 }}>
            <FiHardDrive size={12} style={{ color: diskColor }} /> Disk
          </span>
        </div>

      </div>

      {/* Hardware Temp & Freq info */}
      <div style={{ display: 'flex', justifyContent: 'space-between', padding: '8px 12px', background: 'var(--color-brand-bg)', border: '1px dashed var(--color-brand-border)', borderRadius: 8, fontSize: 12 }}>
        <span style={{ display: 'flex', alignItems: 'center', gap: 4, color: 'var(--color-brand-text)' }}>
          <FiThermometer style={{ color: '#ef4444' }} /> CPU Temp: <strong>{cpuTemp > 0 ? `${cpuTemp.toFixed(1)}°C` : 'N/A'}</strong>
        </span>
        <span style={{ display: 'flex', alignItems: 'center', gap: 4, color: 'var(--color-brand-text)' }}>
          <FiCpu style={{ color: 'var(--color-brand)' }} /> Freq: <strong>{cpuMhz > 0 ? `${(cpuMhz / 1000).toFixed(2)} GHz` : 'N/A'}</strong>
        </span>
      </div>

      {/* Collapsible CPU Cores list */}
      {cpuCores.length > 0 && (
        <div style={{ borderTop: '1px solid var(--color-brand-border)', paddingTop: 10 }}>
          <div 
            onClick={() => setShowCores(!showCores)} 
            style={{ display: 'flex', justifyContent: 'space-between', alignItems: 'center', cursor: 'pointer', fontSize: 12, color: 'var(--color-brand-heading)', fontWeight: 600 }}
          >
            <span>Per-Core CPU Load ({cpuCores.length} cores)</span>
            {showCores ? <FiChevronUp size={14} /> : <FiChevronDown size={14} />}
          </div>
          
          {showCores && (
            <div style={{ display: 'flex', flexDirection: 'column', gap: 6, marginTop: 10 }}>
              {cpuCores.map((percent, index) => (
                <div key={index} style={{ display: 'flex', alignItems: 'center', gap: 10, fontSize: 11 }}>
                  <span style={{ width: 45, color: 'var(--color-brand-muted)', fontWeight: 500 }}>Core {index}</span>
                  <div style={{ flex: 1, height: 6, background: 'var(--color-brand-border)', borderRadius: 3, overflow: 'hidden' }}>
                    <div style={{ width: `${percent}%`, height: '100%', background: cpuColor, transition: 'width 0.4s ease' }} />
                  </div>
                  <span style={{ width: 32, textAlign: 'right', color: 'var(--color-brand-heading)', fontWeight: 700 }}>{Math.round(percent)}%</span>
                </div>
              ))}
            </div>
          )}
        </div>
      )}

      {/* Observability Details section */}
      <div style={{ display: 'flex', flexDirection: 'column', gap: 10, borderTop: '1px solid var(--color-brand-border)', paddingTop: 12, fontSize: 12 }}>
        
        {/* RAM detailed specs */}
        <div style={{ display: 'flex', justifyContent: 'space-between', alignItems: 'center' }}>
          <span style={{ color: 'var(--color-brand-muted)' }}>Memory (Used/Free/Total)</span>
          <span style={{ color: 'var(--color-brand-heading)', fontWeight: 600 }}>
            {memUsed.toFixed(1)} GB / {memFree.toFixed(1)} GB / {memTotal.toFixed(1)} GB
          </span>
        </div>

        {/* SWAP detailed specs */}
        {swapTotal > 0 && (
          <div style={{ display: 'flex', justifyContent: 'space-between', alignItems: 'center' }}>
            <span style={{ color: 'var(--color-brand-muted)' }}>Swap Space ({Math.round(swapPercent)}% used)</span>
            <span style={{ color: 'var(--color-brand-heading)', fontWeight: 600 }}>
              {swapUsed.toFixed(1)} GB / {swapTotal.toFixed(1)} GB
            </span>
          </div>
        )}

        {/* Disk read/write throughput */}
        <div style={{ display: 'flex', justifyContent: 'space-between', alignItems: 'center' }}>
          <span style={{ color: 'var(--color-brand-muted)' }}>Disk IO Read / Write</span>
          <span style={{ color: 'var(--color-brand-heading)', fontWeight: 600 }}>
            {formatSpeed(diskReadSpeed)} / {formatSpeed(diskWriteSpeed)}
          </span>
        </div>

        {/* Network speed throughput */}
        <div style={{ display: 'flex', justifyContent: 'space-between', alignItems: 'center' }}>
          <span style={{ color: 'var(--color-brand-muted)' }}>Network Ingress / Egress</span>
          <span style={{ fontWeight: 600, color: 'var(--color-brand)' }}>
            ↓ {formatSpeed(netRecvSpeed)} / ↑ {formatSpeed(netSentSpeed)}
          </span>
        </div>

      </div>

      {/* Row with Go App Alloc & Uptime */}
      <div style={{ display: 'grid', gridTemplateColumns: '1fr 1fr', gap: 10, borderTop: '1px solid var(--color-brand-border)', paddingTop: 12 }}>
        
        <div>
          <div style={{ fontSize: 10, color: 'var(--color-brand-muted)', textTransform: 'uppercase', fontWeight: 600 }}>App Memory</div>
          <div style={{ display: 'flex', alignItems: 'baseline', gap: 4, marginTop: 2 }}>
            <span style={{ fontSize: 15, fontWeight: 700, color: 'var(--color-brand-heading)' }}>
              {state.app_mem_mb ? state.app_mem_mb.toFixed(1) : '0.0'}
            </span>
            <span style={{ fontSize: 10, color: 'var(--color-brand-text)' }}>MB RAM</span>
          </div>
        </div>

        <div>
          <div style={{ fontSize: 10, color: 'var(--color-brand-muted)', textTransform: 'uppercase', fontWeight: 600, display: 'flex', alignItems: 'center', gap: 4 }}>
            <FiClock /> Uptime
          </div>
          <div style={{ fontSize: 13, fontWeight: 700, color: 'var(--color-brand-heading)', marginTop: 4 }}>
            {formatUptime(state.uptime_seconds)}
          </div>
        </div>

      </div>

    </div>
  );
};
