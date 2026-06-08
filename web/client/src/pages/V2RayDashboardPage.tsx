import React, { useEffect } from 'react';
import { useDashboardStore } from '../store/dashboardStore';
import { SplineChart } from '../components/atoms/SplineChart';
import { FiActivity, FiServer, FiCpu, FiCloud, FiClock, FiClipboard, FiCalendar, FiPieChart, FiBarChart2, FiUsers, FiHardDrive, FiDownload, FiUpload } from 'react-icons/fi';

const formatBytes = (bytes: number) => {
  if (bytes === 0 || isNaN(bytes)) return '0 B';
  const k = 1024;
  const sizes = ['B', 'KB', 'MB', 'GB', 'TB'];
  const i = Math.floor(Math.log(bytes) / Math.log(k));
  return parseFloat((bytes / Math.pow(k, i)).toFixed(2)) + ' ' + sizes[i];
};

export const V2RayDashboardPage: React.FC = () => {
  const { 
    connectStream, 
    disconnectStream, 
    trafficHistory, 
    activeConns, 
    totalDownlink, 
    totalUplink,
    wsConnected 
  } = useDashboardStore();

  useEffect(() => {
    connectStream();
    return () => disconnectStream();
  }, [connectStream, disconnectStream]);

  const currentSpeed = trafficHistory.length > 0 
    ? trafficHistory[trafficHistory.length - 1] 
    : { upload: 0, download: 0 };

  const totalData = totalDownlink + totalUplink;

  // Mini sparkline component
  const MiniSparkline = ({ data, color }: { data: number[], color: string }) => {
    if (!data || data.length < 2) return null;
    const max = Math.max(...data, 10);
    const min = 0;
    const w = 100, h = 30;
    
    const pts = data.map((d, i) => {
      const x = (i / (data.length - 1)) * w;
      const y = h - ((d - min) / (max - min)) * h;
      return `${x},${y}`;
    }).join(' L ');

    return (
      <svg width="100%" height="40" viewBox="0 0 100 40" preserveAspectRatio="none" style={{ position: 'absolute', bottom: 0, left: 0, right: 0, opacity: 0.2 }}>
        <path d={`M 0,${30} L ` + pts + ` L 100,${30}`} fill="none" stroke={color} strokeWidth="2" vectorEffect="non-scaling-stroke" />
        <path d={`M 0,${30} L ` + pts + ` L 100,40 L 0,40 Z`} fill={color} opacity="0.1" />
      </svg>
    );
  };

  const uploadHistory = trafficHistory.map(t => t.upload);
  const downloadHistory = trafficHistory.map(t => t.download);

  // Styled Section Header
  const SectionHeader = ({ title }: { title: string }) => (
    <h3 style={{ fontSize: 15, fontWeight: 600, color: 'var(--color-brand-muted, #94a3b8)', marginBottom: 12, marginTop: 32 }}>
      {title}
    </h3>
  );

  // Styled Card Component
  const StatCard = ({ title, value, sub, subValue, icon: Icon, iconColor, children }: any) => (
    <div style={{ 
      background: 'var(--color-brand-card, #111827)', 
      border: '1px solid var(--color-brand-border, rgba(255,255,255,0.05))', 
      borderRadius: 12, 
      padding: '20px 24px', 
      position: 'relative',
      overflow: 'hidden',
      display: 'flex',
      flexDirection: 'column',
      justifyContent: 'space-between',
      minHeight: 110
    }}>
      <div style={{ display: 'flex', justifyContent: 'space-between', alignItems: 'flex-start', zIndex: 1 }}>
        <div style={{ display: 'flex', alignItems: 'center', gap: 12 }}>
          <div style={{ 
            width: 36, height: 36, borderRadius: '50%', 
            background: `rgba(${iconColor}, 0.1)`, 
            border: `1px solid rgba(${iconColor}, 0.2)`,
            display: 'flex', alignItems: 'center', justifyContent: 'center',
            color: `rgb(${iconColor})`
          }}>
            <Icon size={16} />
          </div>
          <span style={{ fontSize: 13, color: 'var(--color-brand-muted, #94a3b8)', fontWeight: 500 }}>{title}</span>
        </div>
      </div>
      <div style={{ marginTop: 16, zIndex: 1 }}>
        <div style={{ display: 'flex', alignItems: 'baseline', gap: 8 }}>
          <span style={{ fontSize: 20, fontWeight: 700, color: 'var(--color-brand-heading, #fff)' }}>{value}</span>
          {subValue && <span style={{ fontSize: 13, color: 'var(--color-brand-muted, #94a3b8)' }}>/ {subValue}</span>}
        </div>
        {sub && (
          <div style={{ fontSize: 11, marginTop: 4, display: 'flex', alignItems: 'center', gap: 4 }}>
            <span style={{ color: sub.startsWith('-') ? '#ef4444' : '#22c55e' }}>
              {sub.startsWith('-') ? '▼' : '▲'} {sub}
            </span>
            <span style={{ color: '#64748b' }}>vs yesterday</span>
          </div>
        )}
      </div>
      {children}
    </div>
  );

  return (
    <div style={{ padding: '32px 40px', background: 'var(--color-brand-bg, #0f111a)', minHeight: '100vh', fontFamily: '"Inter", sans-serif' }}>
      
      <div style={{ display: 'flex', justifyContent: 'space-between', alignItems: 'center', marginBottom: 8 }}>
        <div>
          <h1 style={{ fontSize: 24, fontWeight: 'bold', color: 'var(--color-brand-heading, #fff)', margin: 0, letterSpacing: '-0.5px' }}>V2Ray Dashboard</h1>
        </div>
        <div style={{ display: 'flex', alignItems: 'center', gap: 8, padding: '6px 12px', background: 'rgba(255,255,255,0.03)', border: '1px solid rgba(255,255,255,0.05)', borderRadius: 20 }}>
          <span style={{ display: 'flex', width: 8, height: 8, position: 'relative' }}>
            <span style={{ position: 'absolute', width: '100%', height: '100%', borderRadius: '50%', backgroundColor: wsConnected ? '#4ade80' : '#f87171', opacity: 0.7, animation: 'ping 1s cubic-bezier(0, 0, 0.2, 1) infinite' }}></span>
            <span style={{ position: 'relative', width: '100%', height: '100%', borderRadius: '50%', backgroundColor: wsConnected ? '#22c55e' : '#ef4444' }}></span>
          </span>
          <span style={{ fontSize: 12, fontWeight: 500, color: 'var(--color-brand-heading, #fff)' }}>
            {wsConnected ? 'Core Live' : 'Offline'}
          </span>
        </div>
      </div>

      <SectionHeader title="Real-time Speeds" />
      <div style={{ display: 'grid', gridTemplateColumns: 'repeat(auto-fit, minmax(240px, 1fr))', gap: 16, marginBottom: 32 }}>
        <StatCard title="Download Speed" value={currentSpeed.download.toFixed(1) + " KB/s"} icon={FiDownload} iconColor="16, 185, 129">
          <MiniSparkline data={downloadHistory} color="#10b981" />
        </StatCard>
        <StatCard title="Upload Speed" value={currentSpeed.upload.toFixed(1) + " KB/s"} icon={FiUpload} iconColor="59, 130, 246">
          <MiniSparkline data={uploadHistory} color="#3b82f6" />
        </StatCard>
        <StatCard title="Peak Download" value={Math.max(0, ...downloadHistory).toFixed(1) + " KB/s"} icon={FiActivity} iconColor="245, 158, 11" />
        <StatCard title="Peak Upload" value={Math.max(0, ...uploadHistory).toFixed(1) + " KB/s"} icon={FiActivity} iconColor="239, 68, 68" />
      </div>

      <SectionHeader title="System & Bandwidth" />
      <div style={{ display: 'grid', gridTemplateColumns: 'repeat(auto-fit, minmax(300px, 1fr))', gap: 16, marginBottom: 32 }}>
        <StatCard title="Total Traffic" value={formatBytes(totalData)} icon={FiBarChart2} iconColor="16, 185, 129" />
        <StatCard title="Total Download" value={formatBytes(totalDownlink)} icon={FiDownload} iconColor="59, 130, 246" />
        <StatCard title="Total Upload" value={formatBytes(totalUplink)} icon={FiUpload} iconColor="245, 158, 11" />
      </div>

      <div style={{ background: 'var(--color-brand-card, #111827)', border: '1px solid var(--color-brand-border, rgba(255,255,255,0.05))', borderRadius: 12, padding: 24, height: 400 }}>
        <SplineChart 
          data={trafficHistory} 
          title="Throughput History"
          subtitle="Real-time Core Performance Metrics (KB/s)"
        />
      </div>
    </div>
  );
};
