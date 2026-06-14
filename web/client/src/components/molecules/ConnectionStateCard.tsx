import React from 'react';
import { FiWifi, FiSend, FiCornerDownLeft, FiRepeat } from 'react-icons/fi';
import { useDashboardStore } from '../../store/dashboardStore';
import { IPResolveBadge } from '../atoms/IPResolveBadge';

export const ConnectionStateCard: React.FC = () => {
  const { connectionState, selectedNode, totalUsage, nodes, connectNode, disconnectNode } = useDashboardStore();
  const isConnected = connectionState === 'connected';

  const handleQuickConnect = () => {
    if (nodes.length === 0) return;
    const target = selectedNode || nodes.find(n => n.active) || nodes[0];
    if (target) {
      connectNode(target);
    }
  };

  const handleSwitch = () => {
    if (nodes.length <= 1) return;
    const currentIndex = nodes.findIndex(n => n.id === selectedNode?.id);
    const nextIndex = (currentIndex + 1) % nodes.length;
    connectNode(nodes[nextIndex]);
  };

  const activeProtocol = selectedNode 
    ? (selectedNode.balance.includes('/') ? selectedNode.balance.split('/')[0].trim() : selectedNode.balance)
    : 'NONE';

  const formattedUsage = (totalUsage.download + totalUsage.upload).toFixed(1);

  return (
    <div className="vcard">
      <div className="vcard__chip-row">
        <FiWifi className="chip-wifi" style={{ color: isConnected ? 'var(--color-brand)' : 'var(--color-brand-muted)' }} />
        <div className="chip-dots"><span /><span /></div>
      </div>

      <div className="vcard__label">Active Session Usage</div>
      <div className="vcard__balance">
        {isConnected ? `${formattedUsage} MB` : '0.0 MB'}
        <span 
          className="vcard__badge" 
          style={{ 
            background: isConnected ? '#eefbf3' : 'rgba(239, 68, 68, 0.1)', 
            color: isConnected ? '#15803d' : '#ef4444' 
          }}
        >
          {connectionState === 'connecting' ? 'Connecting' : isConnected ? 'Connected' : 'Offline'}
        </span>
      </div>

      <div className="vcard__detail">
        <span className="vcard__detail-label">Tunnel Gateway</span>
        <span className="vcard__detail-value" style={{ maxWidth: '60%', overflow: 'hidden', textOverflow: 'ellipsis', whiteSpace: 'nowrap' }}>
          {selectedNode ? <IPResolveBadge ip={selectedNode.ip} /> : '•••• •••• •••• ••••'}
        </span>
      </div>
      <div className="vcard__detail">
        <span className="vcard__detail-label">Active Protocol</span>
        <span className="vcard__detail-value">{isConnected ? activeProtocol : 'NONE'}</span>
      </div>
      <div className="vcard__detail">
        <span className="vcard__detail-label">Ping Latency</span>
        <span className="vcard__detail-value">{selectedNode && selectedNode.ping > 0 ? `${selectedNode.ping} ms` : '– – –'}</span>
      </div>

      <div className="vcard__actions">
        <button 
          className="vcard__action" 
          onClick={handleQuickConnect} 
          disabled={connectionState === 'connecting' || nodes.length === 0}
          style={{ opacity: nodes.length === 0 ? 0.5 : 1 }}
        >
          <FiSend className="action-icon" />
          Quick Connect
        </button>
        <button 
          className="vcard__action" 
          onClick={disconnectNode} 
          disabled={!isConnected}
          style={{ opacity: !isConnected ? 0.5 : 1 }}
        >
          <FiCornerDownLeft className="action-icon" />
          Disconnect
        </button>
        <button 
          className="vcard__action" 
          onClick={handleSwitch} 
          disabled={nodes.length <= 1}
          style={{ opacity: nodes.length <= 1 ? 0.5 : 1 }}
        >
          <FiRepeat className="action-icon" />
          Switch
        </button>
      </div>
    </div>
  );
};

