import React, { useEffect } from 'react';
import { useDashboardStore } from '../store/dashboardStore';
import { ConnectionStateCard } from '../components/molecules/ConnectionStateCard';
import { GoalCard } from '../components/molecules/GoalCard';
import { SplineChart } from '../components/atoms/SplineChart';
import { FiDownload, FiUpload, FiCreditCard } from 'react-icons/fi';
import { Card } from '../components/molecules/Card';
import { LogConsoleCard } from '../components/molecules/LogConsoleCard';
import { SystemMonitor } from '../components/molecules/SystemMonitor';

export const DashboardPage: React.FC = () => {
  const { nodes, connectionState, bandwidthHistory, totalUsage, initWebSocket, connectNode } = useDashboardStore();

  useEffect(() => {
    const token = localStorage.getItem('cc_client_token') || 'dummy';
    const close = initWebSocket(token);
    return () => { close(); };
  }, [initWebSocket]);

  const dlGB = (totalUsage.download / 1024).toFixed(2);
  const ulGB = (totalUsage.upload / 1024).toFixed(2);
  const totalGB = ((totalUsage.download + totalUsage.upload) / 1024).toFixed(1);

  return (
    <div>
      {/* Page Title */}
      <div style={{ display: 'flex', justifyContent: 'space-between', alignItems: 'baseline', marginBottom: 20 }}>
        <h1 style={{ fontSize: 22, fontWeight: 700, color: 'var(--color-brand-heading)', margin: 0 }}>My Dashboard</h1>
        <div style={{ display: 'flex', gap: 8, alignItems: 'center' }}>
          <button className="btn btn--sm">Today</button>
          <button className="btn btn--sm">Manage</button>
        </div>
      </div>

      {/* Two-column layout matching Globyn */}
      <div style={{ display: 'grid', gridTemplateColumns: '380px 1fr', gap: 20, alignItems: 'start' }}>
        {/* LEFT COLUMN — Metrics + My Cards */}
        <div style={{ display: 'flex', flexDirection: 'column', gap: 16 }}>
          {/* TRAFFIC RECEIVED */}
          <Card
            variant="single"
            labelIcon={<FiDownload className="label-icon" />}
            labelText="TRAFFIC RECEIVED"
            title="Completed Downloads"
            value={`${dlGB} GB`}
            changeText="18.6%"
            changeDirection="up"
            description={`You received ${dlGB} GB more this period`}
            rightActionButton={<FiDownload />}
          />

          {/* TRAFFIC SENT */}
          <Card
            variant="single"
            labelIcon={<FiUpload className="label-icon" />}
            labelText="TRAFFIC SENT"
            title="Outgoing Uploads"
            value={`${ulGB} GB`}
            changeText="9.3%"
            changeDirection="down"
            description="Upload decreased by 1,460 MB this period"
            rightActionButton={<FiUpload />}
          />

          {/* QUOTA USAGE */}
          <Card
            variant="single"
            labelIcon={<FiCreditCard className="label-icon" />}
            labelText="QUOTA USAGE"
            title="Total Consumption"
            value={`${totalGB} GB`}
            changeText="12.1%"
            changeDirection="up"
            description="Usage increased by 1,020 MB this period"
            rightActionButton={<FiCreditCard />}
          />

          {/* My Cards header */}
          <div style={{ display: 'flex', justifyContent: 'space-between', alignItems: 'baseline', marginTop: 4 }}>
            <div>
              <div style={{ fontSize: 15, fontWeight: 600, color: 'var(--color-brand-heading)' }}>My Tunnels</div>
              <div style={{ fontSize: 12, color: 'var(--color-brand-text)' }}>Manage your tunnels in real time.</div>
            </div>
            <span style={{ fontSize: 12, fontWeight: 500, color: 'var(--color-brand-heading)', cursor: 'pointer' }}>+ Add Tunnel</span>
          </div>

          {/* Virtual Card */}
          <ConnectionStateCard />

          {/* Real-time System Log Preview Monitor */}
          <LogConsoleCard />
        </div>

        {/* RIGHT COLUMN — Goals + Table */}
        <div style={{ display: 'flex', flexDirection: 'column', gap: 20 }}>
          <SystemMonitor />
          {/* Goals Header */}
          <div style={{ display: 'flex', justifyContent: 'space-between', alignItems: 'baseline' }}>
            <div>
              <div style={{ fontSize: 15, fontWeight: 600, color: 'var(--color-brand-heading)' }}>Goals</div>
              <div style={{ fontSize: 12, color: 'var(--color-brand-text)' }}>Monitor your tunnel goals and usage progress.</div>
            </div>
            <span style={{ fontSize: 12, fontWeight: 500, color: 'var(--color-brand-heading)', cursor: 'pointer' }}>+ Add Goals</span>
          </div>

          {/* 2x2 Goal Cards */}
          <div style={{ display: 'grid', gridTemplateColumns: '1fr 1fr', gap: 14 }}>
            <GoalCard tag="EMERGENCY FUND" tagVariant="orange" targetValue="5.0 GB" dueDate="End of Day"
              currentAmount={`${totalGB} GB`} maxAmount="5.0 GB" progressPercent={parseFloat(totalGB) / 5 * 100} />
            <GoalCard tag="TRAVEL FUND" tagVariant="green" targetValue="50.0 GB" dueDate="15 Jun 2026"
              currentAmount={`${totalGB} GB`} maxAmount="50.0 GB" progressPercent={parseFloat(totalGB) / 50 * 100} />
            <GoalCard tag="BANDWIDTH CAP" tagVariant="blue" targetValue="100.0 GB" dueDate="30 Sep 2026"
              currentAmount={`${totalGB} GB`} maxAmount="100.0 GB" progressPercent={parseFloat(totalGB) / 100 * 100} />
            <GoalCard tag="RESERVE POOL" tagVariant="indigo" targetValue="250.0 GB" dueDate="31 Mar 2027"
              currentAmount={`${totalGB} GB`} maxAmount="250.0 GB" progressPercent={parseFloat(totalGB) / 250 * 100} />
          </div>

          {/* Cash Balances / Gateway Nodes */}
          <div className="g-card" style={{ padding: 0 }}>
            <div style={{ padding: '18px 20px 0' }}>
              <div style={{ display: 'flex', justifyContent: 'space-between', alignItems: 'flex-start', marginBottom: 14 }}>
                <div>
                  <div style={{ fontSize: 15, fontWeight: 600, color: 'var(--color-brand-heading)' }}>Gateway Nodes</div>
                  <div style={{ fontSize: 12, color: 'var(--color-brand-text)' }}>Track your multi-region tunnel balances instantly.</div>
                </div>
                <div style={{ display: 'flex', gap: 6 }}>
                  <button className="btn btn--sm">+ Add Node</button>
                  <button className="btn btn--sm">Switch</button>
                  <button className="btn btn--primary btn--sm">Connect</button>
                </div>
              </div>
            </div>
            <div style={{ padding: '0 20px 16px', overflowX: 'auto' }}>
              <table className="g-table">
                <thead>
                  <tr>
                    <th>GATEWAY</th>
                    <th>ACCOUNTS</th>
                    <th>PING IN</th>
                    <th>PING OUT</th>
                    <th style={{ textAlign: 'right' }}>BALANCE</th>
                  </tr>
                </thead>
                <tbody>
                  {nodes.map((node) => (
                    <tr key={node.id} style={{ cursor: 'pointer' }} onClick={() => connectNode(node)}>
                      <td>
                        <div className="flag-cell">
                          <span className="flag">{node.flag}</span>
                          <span className="currency-code">{node.name.split(' - ')[0]}</span>
                          <span className="currency-name">{node.name.split(' - ')[1] || ''}</span>
                        </div>
                      </td>
                      <td>{node.active ? `${node.accounts} active` : 'dormant'}</td>
                      <td className="sched">↙ {node.ping} ms</td>
                      <td className="sched">↗ {Math.round(node.ping * 1.2)} ms</td>
                      <td className="balance-val">{node.balance}</td>
                    </tr>
                  ))}
                </tbody>
              </table>
            </div>
          </div>

          {/* Spline Chart */}
          <SplineChart data={bandwidthHistory} title="Bandwidth Overview" subtitle="Get a real-time overview of your tunnel throughput." />
        </div>
      </div>
    </div>
  );
};
