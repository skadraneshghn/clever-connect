import React, { useEffect } from 'react';
import { useServerStore } from '../store/dashboardStore';
import { GoalCard } from '../components/molecules/GoalCard';
import { SplineChart } from '../components/atoms/SplineChart';
import { FiUsers, FiCpu, FiShield, FiTerminal } from 'react-icons/fi';
import { Card } from '../components/molecules/Card';
import { LogConsoleCard } from '../components/molecules/LogConsoleCard';
import { SystemMonitor } from '../components/molecules/SystemMonitor';

export const DashboardPage: React.FC = () => {
  const { cpu, memory, disk, activeConnectionsCount, clients, bandwidthHistory, totalBandwidth, logs, initWebSocket, disconnectClient, blockClient } = useServerStore();

  useEffect(() => {
    const token = localStorage.getItem('cc_server_token') || 'dummy';
    const close = initWebSocket(token);
    return () => { close(); };
  }, [initWebSocket]);

  return (
    <div>
      {/* Title */}
      <div style={{ display: 'flex', justifyContent: 'space-between', alignItems: 'baseline', marginBottom: 20 }}>
        <h1 style={{ fontSize: 22, fontWeight: 700, color: 'var(--color-brand-heading)', margin: 0 }}>Dashboard</h1>
        <div style={{ display: 'flex', gap: 8 }}>
          <button className="btn btn--sm">Today</button>
          <button className="btn btn--primary btn--sm">Export Data</button>
        </div>
      </div>

      {/* Top 3 summary cards utilizing the split-column reusable Card component */}
      <div className="summary-cards">
        <Card
          variant="split"
          labelIcon={<FiUsers size={13} />}
          labelText="ACTIVE TUNNELS"
          columns={[
            { subLabel: 'Connected', value: String(activeConnectionsCount), subValue: 'clients', changeText: 'vs. last period', changeDirection: 'up' },
            { subLabel: 'Protocols', value: '4', subValue: 'active', changeText: 'vs. 3 last period', changeDirection: 'up' }
          ]}
        />
        <Card
          variant="split"
          labelIcon={<FiShield size={13} />}
          labelText="BANDWIDTH USAGE"
          columns={[
            { subLabel: 'Download', value: (totalBandwidth.download).toFixed(0), subValue: 'GB', changeText: 'vs. last period', changeDirection: 'up' },
            { subLabel: 'Upload', value: (totalBandwidth.upload).toFixed(0), subValue: 'GB', changeText: 'vs. last period', changeDirection: 'up' }
          ]}
        />
        <Card
          variant="split"
          labelIcon={<FiCpu size={13} />}
          labelText="SYSTEM LOAD"
          columns={[
            { subLabel: 'CPU', value: String(cpu), subValue: '%', changeText: 'nominal', changeDirection: 'up' },
            { subLabel: 'Memory', value: String(memory), subValue: '%', changeText: 'healthy', changeDirection: 'up' }
          ]}
        />
      </div>

      {/* Two-column */}
      <div style={{ display: 'grid', gridTemplateColumns: '1fr 380px', gap: 20, alignItems: 'start' }}>
        {/* Left */}
        <div style={{ display: 'flex', flexDirection: 'column', gap: 18 }}>
          {/* Wallet-like aggregate card */}
          <div className="g-card">
            <div style={{ fontSize: 12, color: 'var(--color-brand-text)', marginBottom: 4 }}>Total Server Bandwidth</div>
            <div style={{ display: 'flex', alignItems: 'baseline', gap: 8, marginBottom: 4 }}>
              <span style={{ fontSize: 28, fontWeight: 700, color: 'var(--color-brand-heading)' }}>{(totalBandwidth.download / 1024).toFixed(1)} TB</span>
              <span style={{ fontSize: 12, color: 'var(--color-brand-muted)' }}>consumed • Updated now</span>
            </div>
            <div style={{ display: 'grid', gridTemplateColumns: 'repeat(4, 1fr)', gap: 8, marginTop: 12, borderTop: '1px solid var(--color-brand-border)', paddingTop: 12 }}>
              {[{ icon: '📡', label: 'Connect' }, { icon: '⚡', label: 'Restart' }, { icon: '🔄', label: 'Rotate' }, { icon: '🏦', label: 'Export' }].map((a) => (
                <button key={a.label} className="vcard__action"><span className="action-icon">{a.icon}</span>{a.label}</button>
              ))}
            </div>
          </div>

          {/* Goals */}
          <div>
            <div style={{ display: 'flex', justifyContent: 'space-between', alignItems: 'baseline', marginBottom: 12 }}>
              <span style={{ fontSize: 15, fontWeight: 600, color: 'var(--color-brand-heading)' }}>Resource Goals</span>
              <span style={{ fontSize: 12, color: 'var(--color-brand)', cursor: 'pointer' }}>✏ Edit</span>
            </div>
            <div style={{ display: 'grid', gridTemplateColumns: '1fr 1fr', gap: 14 }}>
              <GoalCard tag="CPU LIMIT" tagVariant="orange" targetValue="90%" dueDate="Alert Threshold" currentAmount={`${cpu}%`} maxAmount="100%" progressPercent={cpu} />
              <GoalCard tag="RAM BUFFER" tagVariant="green" targetValue="16 GB" dueDate="ECC Buffered" currentAmount={`${(memory * 0.16).toFixed(1)} GB`} maxAmount="16.0 GB" progressPercent={memory} />
            </div>
          </div>

          {/* Chart */}
          <SplineChart data={bandwidthHistory} title="Throughput Overview" subtitle="Get a real-time overview of server bandwidth." />
        </div>

        {/* Right */}
        <div style={{ display: 'flex', flexDirection: 'column', gap: 18 }}>
          <SystemMonitor />
          {/* Connected clients */}
          <div className="g-card" style={{ padding: 0 }}>
            <div style={{ padding: '18px 20px 0' }}>
              <div style={{ display: 'flex', justifyContent: 'space-between', alignItems: 'flex-start', marginBottom: 14 }}>
                <div>
                  <div style={{ fontSize: 15, fontWeight: 600, color: 'var(--color-brand-heading)' }}>Active Clients</div>
                  <div style={{ fontSize: 12, color: 'var(--color-brand-text)' }}>Track connected users across your nodes.</div>
                </div>
                <button className="btn btn--sm">Download report</button>
              </div>
              <div style={{ fontSize: 12, fontWeight: 600, color: 'var(--color-brand-heading)', marginBottom: 8 }}>Top Connected Users</div>
            </div>
            <div style={{ padding: '0 20px 16px' }}>
              {clients.map((c, i) => (
                <div key={c.id} style={{ display: 'flex', alignItems: 'center', justifyContent: 'space-between', padding: '10px 0', borderBottom: i < clients.length - 1 ? '1px solid var(--color-brand-border)' : 'none' }}>
                  <div style={{ display: 'flex', alignItems: 'center', gap: 10 }}>
                    <span style={{ fontSize: 13, fontWeight: 600, color: 'var(--color-brand-muted)', width: 16 }}>#{i + 1}</span>
                    <span style={{ fontSize: 18 }}>{c.flag}</span>
                    <div>
                      <div style={{ fontSize: 13, fontWeight: 600, color: 'var(--color-brand-heading)' }}>{c.username}</div>
                      <div style={{ fontSize: 10, color: 'var(--color-brand-muted)' }}>{c.protocol}</div>
                    </div>
                  </div>
                  <div style={{ textAlign: 'right' }}>
                    <div style={{ fontSize: 13, fontWeight: 600, color: 'var(--color-brand-heading)' }}>↘{c.downloadSpeed} MB/s</div>
                    <div style={{ display: 'flex', gap: 4, marginTop: 4 }}>
                      <button onClick={() => disconnectClient(c.id)} style={{ fontSize: 9, padding: '2px 6px', border: '1px solid var(--color-brand-border)', borderRadius: 4, background: '#fff', cursor: 'pointer' }}>Kick</button>
                      <button onClick={() => blockClient(c.id)} style={{ fontSize: 9, padding: '2px 6px', border: '1px solid var(--color-brand-border)', borderRadius: 4, background: '#fff', cursor: 'pointer' }}>Block</button>
                    </div>
                  </div>
                </div>
              ))}
            </div>
          </div>

          {/* Assets Allocation */}
          <div className="g-card">
            <div style={{ display: 'flex', justifyContent: 'space-between', alignItems: 'baseline', marginBottom: 8 }}>
              <span style={{ fontSize: 15, fontWeight: 600, color: 'var(--color-brand-heading)' }}>Assets Allocation</span>
              <button className="btn btn--sm">Manage</button>
            </div>
            <div className="alloc-bar">
              <div className="alloc-bar__seg" style={{ width: '18%', background: '#ff6b2c' }} />
              <div className="alloc-bar__seg" style={{ width: '34%', background: '#22c55e' }} />
              <div className="alloc-bar__seg" style={{ width: '17%', background: '#3b82f6' }} />
              <div className="alloc-bar__seg" style={{ width: '31%', background: '#6366f1' }} />
            </div>
            <div className="alloc-legend">
              <div className="alloc-legend__item"><div className="alloc-legend__dot" style={{ background: '#ff6b2c' }} /> CPU 18%</div>
              <div className="alloc-legend__item"><div className="alloc-legend__dot" style={{ background: '#22c55e' }} /> RAM 34%</div>
              <div className="alloc-legend__item"><div className="alloc-legend__dot" style={{ background: '#3b82f6' }} /> Disk 17%</div>
              <div className="alloc-legend__item"><div className="alloc-legend__dot" style={{ background: '#6366f1' }} /> Reserves 31%</div>
            </div>
            <table className="g-table" style={{ marginTop: 14 }}>
              <thead><tr><th>NAME</th><th>CATEGORY</th><th style={{ textAlign: 'right' }}>VALUE</th></tr></thead>
              <tbody>
                <tr><td>▸ CPU Cores · 4</td><td>Compute</td><td className="balance-val">{cpu}%</td></tr>
                <tr><td>▸ Memory · 16 GB</td><td>Buffer</td><td className="balance-val">{memory}%</td></tr>
                <tr><td>▸ Disk · 500 GB</td><td>Storage</td><td className="balance-val">{disk}%</td></tr>
                <tr><td>▸ Connections · {activeConnectionsCount}</td><td>Network</td><td className="balance-val">{activeConnectionsCount}</td></tr>
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
