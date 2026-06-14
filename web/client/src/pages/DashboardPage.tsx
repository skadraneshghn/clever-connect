import React, { useEffect } from 'react';
import { useNavigate } from 'react-router-dom';
import { useDashboardStore } from '../store/dashboardStore';
import { useJobsStore } from '../store/jobsStore';
import { ConnectionStateCard } from '../components/molecules/ConnectionStateCard';
import { SplineChart } from '../components/atoms/SplineChart';
import { 
  FiDownload, FiUpload, FiActivity, FiGlobe, 
  FiList, FiCpu, FiTerminal, FiChevronRight, FiPlay, FiServer
} from 'react-icons/fi';
import { LogConsoleCard } from '../components/molecules/LogConsoleCard';
import { SystemMonitor } from '../components/molecules/SystemMonitor';

export const DashboardPage: React.FC = () => {
  const navigate = useNavigate();
  const { 
    nodes, 
    connectionState, 
    bandwidthHistory, 
    totalUsage, 
    initWebSocket, 
    connectNode, 
    fetchRealNodes,
    checkClientStatus,
    activeConns,
    totalUplink,
    totalDownlink,
    trafficHistory,
    schedulerStats,
    domainStats,
    fetchSchedulerStats,
    fetchDomainStats
  } = useDashboardStore();

  const { 
    torrents, 
    leechJobs, 
    youtubeJobs, 
    spotifyJobs, 
    initWebSocket: initJobsWebSocket 
  } = useJobsStore();

  useEffect(() => {
    const token = localStorage.getItem('cc_client_token') || 'dummy';
    
    // Initialize WebSockets
    const closeDashboardWS = initWebSocket(token);
    const closeJobsWS = initJobsWebSocket(token);

    // Initial and periodic polling for static stats
    checkClientStatus();
    fetchRealNodes();
    fetchSchedulerStats();
    fetchDomainStats();

    const interval = setInterval(() => {
      fetchSchedulerStats();
      fetchDomainStats();
      fetchRealNodes();
    }, 4000);

    return () => { 
      closeDashboardWS(); 
      closeJobsWS();
      clearInterval(interval);
    };
  }, [initWebSocket, initJobsWebSocket]);

  // Calculations for bandwidth speeds
  const currentSpeed = trafficHistory.length > 0 
    ? trafficHistory[trafficHistory.length - 1] 
    : { download: 0, upload: 0 };

  const dlSpeedFormatted = currentSpeed.download >= 1024 
    ? `${(currentSpeed.download / 1024).toFixed(1)} MB/s` 
    : `${currentSpeed.download.toFixed(1)} KB/s`;

  const ulSpeedFormatted = currentSpeed.upload >= 1024 
    ? `${(currentSpeed.upload / 1024).toFixed(1)} MB/s` 
    : `${currentSpeed.upload.toFixed(1)} KB/s`;

  // Media download stats
  const downloadingTorrents = torrents.filter(t => t.status === 'downloading').length;
  const downloadingLeech = leechJobs.filter(l => l.status === 'downloading').length;
  const downloadingYT = youtubeJobs.filter(y => y.status === 'downloading').length;
  const downloadingSpotify = spotifyJobs.filter(s => s.status === 'downloading').length;

  const totalActiveMedia = downloadingTorrents + downloadingLeech + downloadingYT + downloadingSpotify;
  const totalCompletedMedia = 
    torrents.filter(t => t.status === 'completed' || t.status === 'seeding').length +
    leechJobs.filter(l => l.status === 'completed').length +
    youtubeJobs.filter(y => y.status === 'completed').length +
    spotifyJobs.filter(s => s.status === 'completed').length;

  // Active Connection Info
  const isConnected = connectionState === 'connected';

  return (
    <div style={{ display: 'flex', flexDirection: 'column', gap: 24, paddingBottom: 32 }}>
      
      {/* Top Banner & Header */}
      <div style={{ display: 'flex', justifyContent: 'space-between', alignItems: 'center' }}>
        <div>
          <h1 style={{ fontSize: 24, fontWeight: 700, color: 'var(--color-brand-heading)', margin: 0, letterSpacing: '-0.5px' }}>
            Enterprise Hub
          </h1>
          <div style={{ fontSize: 13, color: 'var(--color-brand-text)', marginTop: 4 }}>
            Real-time telemetry, connection controller, task scheduler, and media downloader stats.
          </div>
        </div>
        <div style={{ display: 'flex', gap: 8 }}>
          <button className="btn btn--sm" onClick={() => fetchRealNodes()}>Sync Gateway Nodes</button>
          <button className="btn btn--primary btn--sm" onClick={() => navigate('/v2ray-nodes')}>Manage Configs</button>
        </div>
      </div>

      {/* Grid of Real Statistics & Telemetry Cards */}
      <div style={{ display: 'grid', gridTemplateColumns: 'repeat(auto-fit, minmax(240px, 1fr))', gap: 16 }}>
        
        {/* VPN/Tunnel Telemetry */}
        <div className="g-card" style={{ display: 'flex', flexDirection: 'column', justifyContent: 'space-between' }}>
          <div>
            <div className="g-card__label">
              <FiServer className="label-icon" /> Tunnel Status
            </div>
            <div style={{ display: 'flex', alignItems: 'center', gap: 8, marginTop: 4 }}>
              <span className="live-dot" style={{ 
                width: 8, 
                height: 8, 
                background: isConnected ? '#22c55e' : '#ef4444',
                boxShadow: isConnected ? '0 0 8px #22c55e' : 'none'
              }} />
              <div style={{ fontSize: 18, fontWeight: 700, color: 'var(--color-brand-heading)' }}>
                {connectionState === 'connecting' ? 'Connecting...' : isConnected ? 'Tunnel Secure' : 'Core Offline'}
              </div>
            </div>
            <div style={{ fontSize: 12, color: 'var(--color-brand-text)', marginTop: 8 }}>
              {isConnected && useDashboardStore.getState().selectedNode ? (
                <>Connected to <strong style={{ color: 'var(--color-brand-heading)' }}>{useDashboardStore.getState().selectedNode?.name}</strong></>
              ) : (
                'System waiting for Quick Connect trigger.'
              )}
            </div>
          </div>
          <div style={{ display: 'flex', justifyContent: 'space-between', alignItems: 'center', marginTop: 16, paddingTop: 12, borderTop: '1px solid var(--color-brand-border)' }}>
            <span style={{ fontSize: 11, fontWeight: 500, color: 'var(--color-brand-muted)' }}>Latent Speed:</span>
            <span style={{ fontSize: 12, fontWeight: 600, color: 'var(--color-brand-heading)' }}>
              {isConnected && useDashboardStore.getState().selectedNode?.ping ? `${useDashboardStore.getState().selectedNode?.ping} ms` : '– – –'}
            </span>
          </div>
        </div>

        {/* Job Scheduler Card */}
        <div 
          className="g-card" 
          style={{ display: 'flex', flexDirection: 'column', justifyContent: 'space-between', cursor: 'pointer' }}
          onClick={() => navigate('/scheduler')}
        >
          <div>
            <div className="g-card__label">
              <FiList className="label-icon" /> Job Scheduler
            </div>
            <div style={{ fontSize: 24, fontWeight: 700, color: 'var(--color-brand-heading)', marginTop: 4 }}>
              {schedulerStats ? schedulerStats.total_jobs : 0} <span style={{ fontSize: 14, fontWeight: 500, color: 'var(--color-brand-text)' }}>Tasks</span>
            </div>
            <div style={{ fontSize: 12, color: 'var(--color-brand-text)', marginTop: 6, display: 'flex', gap: 10 }}>
              <span style={{ color: '#22c55e' }}>● {schedulerStats ? schedulerStats.completed_jobs : 0} Done</span>
              <span style={{ color: '#ef4444' }}>● {schedulerStats ? schedulerStats.failed_jobs : 0} Failed</span>
              <span style={{ color: '#3b82f6' }}>● {schedulerStats ? schedulerStats.running_jobs : 0} Run</span>
            </div>
          </div>
          <div style={{ display: 'flex', justifyContent: 'space-between', alignItems: 'center', marginTop: 16, paddingTop: 12, borderTop: '1px solid var(--color-brand-border)' }}>
            <span style={{ fontSize: 11, fontWeight: 500, color: 'var(--color-brand-muted)', display: 'flex', alignItems: 'center', gap: 4 }}>
              Scheduler Workers
            </span>
            <span style={{ fontSize: 12, fontWeight: 600, color: 'var(--color-brand-heading)' }}>
              {schedulerStats ? `${schedulerStats.active_workers}/${schedulerStats.max_workers}` : '0/0'} Active
            </span>
          </div>
        </div>

        {/* Media Downloader Card */}
        <div 
          className="g-card" 
          style={{ display: 'flex', flexDirection: 'column', justifyContent: 'space-between', cursor: 'pointer' }}
          onClick={() => navigate('/leech')}
        >
          <div>
            <div className="g-card__label">
              <FiDownload className="label-icon" /> Media Engine
            </div>
            <div style={{ fontSize: 24, fontWeight: 700, color: 'var(--color-brand-heading)', marginTop: 4 }}>
              {totalActiveMedia} <span style={{ fontSize: 14, fontWeight: 500, color: 'var(--color-brand-text)' }}>Downloading</span>
            </div>
            <div style={{ fontSize: 12, color: 'var(--color-brand-text)', marginTop: 8 }}>
              Active tasks: Torrent, Spotify, Leech, or YouTube downloads.
            </div>
          </div>
          <div style={{ display: 'flex', justifyContent: 'space-between', alignItems: 'center', marginTop: 16, paddingTop: 12, borderTop: '1px solid var(--color-brand-border)' }}>
            <span style={{ fontSize: 11, fontWeight: 500, color: 'var(--color-brand-muted)' }}>Finished Files:</span>
            <span style={{ fontSize: 12, fontWeight: 600, color: 'var(--color-brand-green)' }}>
              {totalCompletedMedia} Completed
            </span>
          </div>
        </div>

        {/* Domain Intelligence Card */}
        <div 
          className="g-card" 
          style={{ display: 'flex', flexDirection: 'column', justifyContent: 'space-between', cursor: 'pointer' }}
          onClick={() => navigate('/domain-checker')}
        >
          <div>
            <div className="g-card__label">
              <FiGlobe className="label-icon" /> Domain Checker
            </div>
            <div style={{ fontSize: 24, fontWeight: 700, color: 'var(--color-brand-heading)', marginTop: 4 }}>
              {domainStats ? domainStats.total : 0} <span style={{ fontSize: 14, fontWeight: 500, color: 'var(--color-brand-text)' }}>Hosts</span>
            </div>
            <div style={{ fontSize: 12, color: 'var(--color-brand-text)', marginTop: 6, display: 'flex', gap: 10 }}>
              <span style={{ color: '#22c55e' }}>● {domainStats ? domainStats.online : 0} Online</span>
              <span style={{ color: '#ef4444' }}>● {domainStats ? domainStats.offline : 0} Offline</span>
              <span style={{ color: 'var(--color-brand)' }}>● {domainStats ? domainStats.ssl_valid : 0} SSL OK</span>
            </div>
          </div>
          <div style={{ display: 'flex', justifyContent: 'space-between', alignItems: 'center', marginTop: 16, paddingTop: 12, borderTop: '1px solid var(--color-brand-border)' }}>
            <span style={{ fontSize: 11, fontWeight: 500, color: 'var(--color-brand-muted)' }}>Intel Scan Status:</span>
            <span style={{ fontSize: 11, fontWeight: 600, color: 'var(--color-brand-heading)' }}>
              {domainStats && domainStats.checking > 0 ? 'Scanning...' : 'Idle'}
            </span>
          </div>
        </div>

      </div>

      {/* Main Two-column Telemetry Layout */}
      <div style={{ display: 'grid', gridTemplateColumns: 'minmax(0, 1.2fr) minmax(0, 1.8fr)', gap: 24 }}>
        
        {/* Left Side: Real Connection Controller & Live Bandwidth Chart */}
        <div style={{ display: 'flex', flexDirection: 'column', gap: 20 }}>
          
          <div style={{ fontSize: 15, fontWeight: 600, color: 'var(--color-brand-heading)', marginBottom: -4 }}>
            Core Connection Controller
          </div>
          
          {/* Real Connection Controller Card */}
          <ConnectionStateCard />

          {/* Speed Dials */}
          <div style={{ display: 'grid', gridTemplateColumns: '1fr 1fr', gap: 12 }}>
            <div className="g-card" style={{ padding: 14, background: 'var(--color-brand-light)' }}>
              <div style={{ fontSize: 10, fontWeight: 600, color: 'var(--color-brand)', textTransform: 'uppercase', letterSpacing: 0.5 }}>
                Downlink Speed
              </div>
              <div style={{ display: 'flex', alignItems: 'baseline', gap: 4, marginTop: 4 }}>
                <span style={{ fontSize: 20, fontWeight: 700, color: 'var(--color-brand-heading)' }}>
                  {isConnected ? dlSpeedFormatted.split(' ')[0] : '0.0'}
                </span>
                <span style={{ fontSize: 11, color: 'var(--color-brand-text)', fontWeight: 500 }}>
                  {isConnected ? dlSpeedFormatted.split(' ')[1] : 'KB/s'}
                </span>
              </div>
            </div>
            <div className="g-card" style={{ padding: 14, background: 'rgba(34, 197, 94, 0.05)' }}>
              <div style={{ fontSize: 10, fontWeight: 600, color: '#22c55e', textTransform: 'uppercase', letterSpacing: 0.5 }}>
                Uplink Speed
              </div>
              <div style={{ display: 'flex', alignItems: 'baseline', gap: 4, marginTop: 4 }}>
                <span style={{ fontSize: 20, fontWeight: 700, color: 'var(--color-brand-heading)' }}>
                  {isConnected ? ulSpeedFormatted.split(' ')[0] : '0.0'}
                </span>
                <span style={{ fontSize: 11, color: 'var(--color-brand-text)', fontWeight: 500 }}>
                  {isConnected ? ulSpeedFormatted.split(' ')[1] : 'KB/s'}
                </span>
              </div>
            </div>
          </div>

          {/* Bandwidth overview details */}
          <div className="g-card" style={{ display: 'flex', flexDirection: 'column', gap: 10, padding: 16 }}>
            <div style={{ display: 'flex', justifyContent: 'space-between', fontSize: 12 }}>
              <span style={{ color: 'var(--color-brand-text)' }}>Active TCP/UDP Conns</span>
              <strong style={{ color: 'var(--color-brand-heading)' }}>{isConnected ? activeConns : 0} Connections</strong>
            </div>
            <div style={{ display: 'flex', justifyContent: 'space-between', fontSize: 12 }}>
              <span style={{ color: 'var(--color-brand-text)' }}>Total Session Received</span>
              <strong style={{ color: 'var(--color-brand-heading)' }}>
                {isConnected ? `${(totalDownlink / (1024 * 1024)).toFixed(1)} MB` : '0.0 MB'}
              </strong>
            </div>
            <div style={{ display: 'flex', justifyContent: 'space-between', fontSize: 12 }}>
              <span style={{ color: 'var(--color-brand-text)' }}>Total Session Sent</span>
              <strong style={{ color: 'var(--color-brand-heading)' }}>
                {isConnected ? `${(totalUplink / (1024 * 1024)).toFixed(1)} MB` : '0.0 MB'}
              </strong>
            </div>
          </div>

        </div>

        {/* Right Side: Gateway Nodes List & Spline Chart */}
        <div style={{ display: 'flex', flexDirection: 'column', gap: 20 }}>
          
          <div className="g-card" style={{ padding: 0 }}>
            <div style={{ padding: '18px 20px 0' }}>
              <div style={{ display: 'flex', justifyContent: 'space-between', alignItems: 'flex-start', marginBottom: 14 }}>
                <div>
                  <div style={{ fontSize: 15, fontWeight: 600, color: 'var(--color-brand-heading)' }}>VPN Gateway Nodes</div>
                  <div style={{ fontSize: 12, color: 'var(--color-brand-text)', marginTop: 2 }}>
                    List of configured V2Ray proxy nodes in the Pebble key-value store.
                  </div>
                </div>
                <div style={{ display: 'flex', gap: 6 }}>
                  <button className="btn btn--sm" style={{ background: '#dc3545', color: '#fff', border: 'none' }} onClick={() => useDashboardStore.getState().deleteAllNodes()}>
                    Delete All
                  </button>
                </div>
              </div>
            </div>

            <div style={{ padding: '0 20px 16px', overflowX: 'auto', maxHeight: 310, overflowY: 'auto' }}>
              {nodes.length === 0 ? (
                <div style={{ textAlign: 'center', padding: '36px 0', color: 'var(--color-brand-muted)', display: 'flex', flexDirection: 'column', alignItems: 'center', gap: 12 }}>
                  <span style={{ fontSize: 13 }}>No configuration profiles stored in the local Pebble database.</span>
                  <button className="btn btn--primary btn--sm" onClick={() => navigate('/v2ray-nodes')}>Import Node Profiles</button>
                </div>
              ) : (
                <table className="g-table">
                  <thead>
                    <tr>
                      <th>GATEWAY</th>
                      <th>TYPE</th>
                      <th>ADDRESS</th>
                      <th>LATENCY</th>
                      <th style={{ textAlign: 'right' }}>ACTION</th>
                    </tr>
                  </thead>
                  <tbody>
                    {nodes.map((node) => {
                      const isNodeActive = node.id === useDashboardStore.getState().selectedNode?.id;
                      return (
                        <tr 
                          key={node.id} 
                          style={{ 
                            background: isNodeActive && isConnected ? 'var(--color-brand-light)' : 'transparent',
                            transition: 'background 0.2s'
                          }}
                        >
                          <td>
                            <div className="flag-cell">
                              <span className="flag">{node.flag}</span>
                              <span className="currency-code" style={{ fontWeight: 600 }}>{node.name}</span>
                            </div>
                          </td>
                          <td>
                            <span style={{
                              fontSize: 10,
                              fontWeight: 700,
                              background: 'var(--color-brand-border)',
                              padding: '2px 6px',
                              borderRadius: 4,
                              color: 'var(--color-brand-heading)'
                            }}>
                              {node.balance.split('/')[0].trim()}
                            </span>
                          </td>
                          <td style={{ fontFamily: 'monospace', fontSize: 11, color: 'var(--color-brand-muted)' }}>
                            {node.ip}:{node.balance.split('/')[1]?.trim() || ''}
                          </td>
                          <td>
                            <span style={{ 
                              color: node.ping === 0 ? 'var(--color-brand-muted)' : node.ping < 100 ? '#22c55e' : node.ping < 200 ? '#eab308' : '#ef4444',
                              fontWeight: 600
                            }}>
                              {node.ping === 0 ? 'untested' : `${node.ping} ms`}
                            </span>
                          </td>
                          <td style={{ textAlign: 'right' }}>
                            {isNodeActive && isConnected ? (
                              <span style={{ fontSize: 11, color: '#22c55e', fontWeight: 600 }}>Connected</span>
                            ) : (
                              <button 
                                className="btn btn--sm" 
                                style={{ padding: '3px 8px', fontSize: 11 }}
                                onClick={() => connectNode(node)}
                              >
                                Connect
                              </button>
                            )}
                          </td>
                        </tr>
                      );
                    })}
                  </tbody>
                </table>
              )}
            </div>
          </div>

          {/* Throughput Spline Chart */}
          <div className="g-card" style={{ padding: 20 }}>
            <SplineChart 
              data={bandwidthHistory} 
              title="Throughput Monitor" 
              subtitle="Live chart streaming aggregate V2Ray core bandwidth consumption." 
            />
          </div>

        </div>

      </div>

      {/* Bottom Telemetry Section: Hardware Monitor & Logs Terminal */}
      <div style={{ display: 'grid', gridTemplateColumns: 'repeat(auto-fit, minmax(360px, 1fr))', gap: 24 }}>
        <SystemMonitor />
        <LogConsoleCard />
      </div>

    </div>
  );
};
