import React, { useEffect } from 'react';
import { useServerStore } from '../store/dashboardStore';
import { GoalCard } from '../components/molecules/GoalCard';
import { SplineChart } from '../components/atoms/SplineChart';
import { FiUsers, FiCpu, FiShield, FiTerminal } from 'react-icons/fi';
import { Card } from '../components/molecules/Card';
import { LogConsoleCard } from '../components/molecules/LogConsoleCard';
import { SystemMonitor } from '../components/molecules/SystemMonitor';

export const DashboardPage: React.FC = () => {
  const { 
    cpu, memory, disk, activeConnectionsCount, clients, bandwidthHistory, totalBandwidth, logs, initWebSocket, disconnectClient, blockClient,
    mem_total_gb, mem_used_gb, disk_total_gb, disk_used_gb, uptime_seconds, app_mem_mb, go_version, os_runtime,
    active_leeches, active_torrents, active_scheds,
    os_platform, os_kernel, swap_total_gb, swap_used_gb, swap_percent
  } = useServerStore();

  useEffect(() => {
    const token = localStorage.getItem('cc_server_token') || 'dummy';
    const close = initWebSocket(token);
    return () => { close(); };
  }, [initWebSocket]);

  const formatUptime = (sec: number) => {
    const d = Math.floor(sec / (3600*24));
    const h = Math.floor((sec % (3600*24)) / 3600);
    const m = Math.floor((sec % 3600) / 60);
    const parts = [];
    if (d > 0) parts.push(`${d}d`);
    if (h > 0) parts.push(`${h}h`);
    if (m > 0 || parts.length === 0) parts.push(`${m}m`);
    return parts.join(' ');
  };

  return (
    <div>
      {/* Title */}
      <div style={{ display: 'flex', justifyContent: 'space-between', alignItems: 'baseline', marginBottom: 20 }}>
        <h1 style={{ fontSize: 22, fontWeight: 700, color: 'var(--color-brand-heading)', margin: 0 }}>Dashboard</h1>
        <div style={{ display: 'flex', gap: 8 }}>
          <button className="btn btn--primary btn--sm">System Telemetry Online</button>
        </div>
      </div>

      {/* Top 3 summary cards utilizing the split-column reusable Card component */}
      <div className="summary-cards">
        <Card
          variant="split"
          labelIcon={<FiUsers size={13} />}
          labelText="ACTIVE TUNNELS"
          columns={[
            { subLabel: 'Connected', value: String(activeConnectionsCount), subValue: 'clients', changeText: 'live connection', changeDirection: 'up' },
            { subLabel: 'Protocols', value: '4', subValue: 'active', changeText: 'available', changeDirection: 'up' }
          ]}
        />
        <Card
          variant="split"
          labelIcon={<FiShield size={13} />}
          labelText="BANDWIDTH USAGE"
          columns={[
            { subLabel: 'Download', value: (totalBandwidth.download).toFixed(0), subValue: 'GB', changeText: 'total download', changeDirection: 'up' },
            { subLabel: 'Upload', value: (totalBandwidth.upload).toFixed(0), subValue: 'GB', changeText: 'total upload', changeDirection: 'up' }
          ]}
        />
        <Card
          variant="split"
          labelIcon={<FiCpu size={13} />}
          labelText="SYSTEM LOAD"
          columns={[
            { subLabel: 'CPU', value: String(cpu), subValue: '%', changeText: 'utilization', changeDirection: 'up' },
            { subLabel: 'Memory', value: String(memory), subValue: '%', changeText: 'ram consumption', changeDirection: 'up' }
          ]}
        />
      </div>

      {/* Two-column */}
      <div style={{ display: 'grid', gridTemplateColumns: '1fr 380px', gap: 20, alignItems: 'start' }}>
        {/* Left */}
        <div style={{ display: 'flex', flexDirection: 'column', gap: 18 }}>
          {/* Real Bandwidth Consumption display */}
          <div className="g-card">
            <div style={{ fontSize: 12, color: 'var(--color-brand-text)', marginBottom: 4 }}>Total Server Throughput</div>
            <div style={{ display: 'flex', alignItems: 'baseline', gap: 8, marginBottom: 4 }}>
              <span style={{ fontSize: 28, fontWeight: 700, color: 'var(--color-brand-heading)' }}>{((totalBandwidth.download + totalBandwidth.upload) / 1024).toFixed(2)} TB</span>
              <span style={{ fontSize: 12, color: 'var(--color-brand-muted)' }}>consumed • Updated via WebSocket</span>
            </div>
          </div>

          {/* Running Background Services Status */}
          <div className="g-card">
            <div style={{ fontSize: 15, fontWeight: 600, color: 'var(--color-brand-heading)', marginBottom: 12 }}>
              Background Managers & Downloader
            </div>
            <div style={{ display: 'grid', gridTemplateColumns: 'repeat(3, 1fr)', gap: 14 }}>
              <div style={{ border: '1px solid var(--color-brand-border)', borderRadius: 8, padding: 12, background: 'var(--color-brand-bg)' }}>
                <div style={{ fontSize: 10, fontWeight: 700, color: 'var(--color-brand-muted)', textTransform: 'uppercase' }}>Torrent Engine</div>
                <div style={{ fontSize: 18, fontWeight: 700, color: 'var(--color-brand-heading)', marginTop: 4 }}>{active_torrents}</div>
                <div style={{ fontSize: 11, color: 'var(--color-brand-text)', marginTop: 2 }}>active downloads</div>
              </div>
              <div style={{ border: '1px solid var(--color-brand-border)', borderRadius: 8, padding: 12, background: 'var(--color-brand-bg)' }}>
                <div style={{ fontSize: 10, fontWeight: 700, color: 'var(--color-brand-muted)', textTransform: 'uppercase' }}>Remote Leech</div>
                <div style={{ fontSize: 18, fontWeight: 700, color: 'var(--color-brand-heading)', marginTop: 4 }}>{active_leeches}</div>
                <div style={{ fontSize: 11, color: 'var(--color-brand-text)', marginTop: 2 }}>downloading files</div>
              </div>
              <div style={{ border: '1px solid var(--color-brand-border)', borderRadius: 8, padding: 12, background: 'var(--color-brand-bg)' }}>
                <div style={{ fontSize: 10, fontWeight: 700, color: 'var(--color-brand-muted)', textTransform: 'uppercase' }}>Job Scheduler</div>
                <div style={{ fontSize: 18, fontWeight: 700, color: 'var(--color-brand-heading)', marginTop: 4 }}>{active_scheds}</div>
                <div style={{ fontSize: 11, color: 'var(--color-brand-text)', marginTop: 2 }}>running tasks</div>
              </div>
            </div>
          </div>

          {/* Chart */}
          <SplineChart data={bandwidthHistory} title="Throughput Overview" subtitle="Get a real-time overview of server bandwidth." />
        </div>

        {/* Right */}
        <div style={{ display: 'flex', flexDirection: 'column', gap: 18 }}>
          <SystemMonitor />
          

          {/* System Specs & Allocation */}
          <div className="g-card">
            <div style={{ display: 'flex', justifyContent: 'space-between', alignItems: 'baseline', marginBottom: 8 }}>
              <span style={{ fontSize: 15, fontWeight: 600, color: 'var(--color-brand-heading)' }}>System Specifications</span>
            </div>
            
            <table className="g-table" style={{ marginTop: 8 }}>
              <thead><tr><th>NAME</th><th>METRIC</th><th style={{ textAlign: 'right' }}>VALUE</th></tr></thead>
              <tbody>
                <tr><td>Uptime</td><td>Duration</td><td className="balance-val" style={{ fontFamily: 'monospace' }}>{formatUptime(uptime_seconds)}</td></tr>
                <tr><td>App Process Memory</td><td>Allocated</td><td className="balance-val" style={{ fontFamily: 'monospace' }}>{app_mem_mb.toFixed(1)} MB</td></tr>
                <tr><td>Go Compiler</td><td>Runtime</td><td className="balance-val" style={{ fontFamily: 'monospace' }}>{go_version}</td></tr>
                <tr><td>OS Type</td><td>Platform</td><td className="balance-val" style={{ fontFamily: 'monospace' }}>{os_platform || os_runtime}</td></tr>
                {os_kernel && <tr><td>Kernel Version</td><td>Release</td><td className="balance-val" style={{ fontFamily: 'monospace', fontSize: 10 }}>{os_kernel}</td></tr>}
                <tr><td>System RAM</td><td>{mem_used_gb.toFixed(1)} / {mem_total_gb.toFixed(0)} GB</td><td className="balance-val">{memory}%</td></tr>
                {swap_total_gb > 0 && <tr><td>Swap Memory</td><td>{swap_used_gb.toFixed(1)} / {swap_total_gb.toFixed(0)} GB</td><td className="balance-val">{swap_percent.toFixed(0)}%</td></tr>}
                <tr><td>System Storage</td><td>{disk_used_gb.toFixed(1)} / {disk_total_gb.toFixed(0)} GB</td><td className="balance-val">{disk}%</td></tr>
              </tbody>
            </table>
          </div>

          {/* Real-time System Log Preview Monitor */}
          <LogConsoleCard />
        </div>
      </div>
    </div>
  );
};
