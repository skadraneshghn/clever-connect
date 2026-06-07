import React from 'react';
import { FiServer, FiHelpCircle, FiRefreshCw } from 'react-icons/fi';

interface ClusterControlsCardProps {
  isSyncing: boolean;
  clusterNodes: any[];
  handleSyncCluster: () => void;
  showHelp: (title: string, text: string) => void;
}

export const ClusterControlsCard: React.FC<ClusterControlsCardProps> = ({
  isSyncing,
  clusterNodes,
  handleSyncCluster,
  showHelp,
}) => {
  return (
    <div className="g-card" style={{ display: 'flex', flexDirection: 'column', gap: 14 }}>
      <div style={{ display: 'flex', alignItems: 'center', justifyContent: 'space-between' }}>
        <div style={{ display: 'flex', alignItems: 'center', gap: 8 }}>
          <FiServer style={{ color: 'var(--color-brand)', fontSize: 18 }} />
          <span style={{ fontSize: 15, fontWeight: 700, color: 'var(--color-brand-heading)' }}>
            Cluster Orchestration Node
          </span>
          <FiHelpCircle
            style={{ cursor: 'pointer', color: 'var(--color-brand-muted)' }}
            onClick={() =>
              showHelp(
                'Cluster Orchestration Node',
                'Aggregates active remote proxy inbounds across edge locations, checks latency metrics, and handles automatic routing map updates.'
              )
            }
          />
        </div>
        <button
          className="btn btn--sm btn--primary"
          onClick={handleSyncCluster}
          disabled={isSyncing}
          style={{ display: 'flex', alignItems: 'center', gap: 6 }}
        >
          <FiRefreshCw className={isSyncing ? 'spin-animation' : ''} />
          {isSyncing ? 'Syncing...' : 'Sync Cluster'}
        </button>
      </div>

      <div
        style={{
          background: 'var(--color-brand-bg)',
          borderRadius: 8,
          padding: '10px 14px',
          fontSize: 12,
          display: 'flex',
          justifyContent: 'space-between',
          color: 'var(--color-brand-heading)',
          border: '1px solid var(--color-brand-border)',
        }}
      >
        <div>
          <strong>Total Managed Edges:</strong>{' '}
          <span style={{ color: 'var(--color-brand)', fontWeight: 700 }}>{clusterNodes.length}</span>
        </div>
        <div>
          <strong>Status:</strong>{' '}
          <span style={{ color: 'var(--color-brand-green)', fontWeight: 700 }}>HEALTHY</span>
        </div>
        <div>
          <strong>Sync Loop:</strong> <span style={{ fontFamily: 'monospace' }}>Every 30s</span>
        </div>
      </div>
    </div>
  );
};

export default ClusterControlsCard;
