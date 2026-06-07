import React from 'react';
import { FiServer, FiHelpCircle, FiX } from 'react-icons/fi';

interface EdgeNodesCardProps {
  isLoading: boolean;
  nodes: any[];
  nodeName: string;
  setNodeName: (name: string) => void;
  nodeIP: string;
  setNodeIP: (ip: string) => void;
  nodeSSHPort: number;
  setNodeSSHPort: (port: number) => void;
  nodeSSHCreds: string;
  setNodeSSHCreds: (creds: string) => void;
  selectedNodeIdForProvision: number | null;
  setSelectedNodeIdForProvision: (id: number | null) => void;
  provisionPassword: string;
  setProvisionPassword: (password: string) => void;
  handleAddNode: (e: React.FormEvent) => void;
  handleProvision: (e: React.FormEvent) => void;
  showHelp: (title: string, text: string) => void;
}

export const EdgeNodesCard: React.FC<EdgeNodesCardProps> = ({
  isLoading,
  nodes,
  nodeName,
  setNodeName,
  nodeIP,
  setNodeIP,
  nodeSSHPort,
  setNodeSSHPort,
  nodeSSHCreds,
  setNodeSSHCreds,
  selectedNodeIdForProvision,
  setSelectedNodeIdForProvision,
  provisionPassword,
  setProvisionPassword,
  handleAddNode,
  handleProvision,
  showHelp,
}) => {
  return (
    <>
      <div className="g-card">
        <div style={{ display: 'flex', alignItems: 'center', gap: 8, marginBottom: 16 }}>
          <FiServer style={{ color: 'var(--color-brand)', fontSize: 18 }} />
          <span style={{ fontSize: 15, fontWeight: 600, color: 'var(--color-brand-heading)' }}>
            Remote VPS Edge Nodes
          </span>
          <FiHelpCircle
            style={{ cursor: 'pointer', color: 'var(--color-brand-muted)' }}
            onClick={() =>
              showHelp(
                'Remote VPS Nodes',
                'Registers remote virtual private servers to configure them as proxy nodes. Use automated SSH provisioning to auto-install Xray core binaries and open necessary firewalls.'
              )
            }
          />
        </div>

        {/* List Nodes */}
        <div
          style={{
            display: 'grid',
            gridTemplateColumns: 'repeat(auto-fill, minmax(220px, 1fr))',
            gap: 14,
            marginBottom: 20,
          }}
        >
          {nodes.length === 0 ? (
            <div
              style={{
                gridColumn: '1/-1',
                padding: '16px 0',
                textAlign: 'center',
                color: 'var(--color-brand-muted)',
                fontSize: 12,
              }}
            >
              No remote nodes registered.
            </div>
          ) : (
            nodes.map((n) => (
              <div
                key={n.ID}
                style={{
                  border: '1px solid var(--color-brand-border)',
                  borderRadius: 8,
                  padding: 12,
                  background: 'var(--color-brand-bg)',
                }}
              >
                <div style={{ display: 'flex', justifyContent: 'space-between', alignItems: 'center', marginBottom: 6 }}>
                  <span style={{ fontWeight: 700, fontSize: 13, color: 'var(--color-brand-heading)' }}>{n.name}</span>
                  <span
                    style={{
                      fontSize: 9,
                      padding: '2px 6px',
                      borderRadius: 4,
                      fontWeight: 700,
                      background: n.status === 'online' ? 'var(--color-brand-light)' : '#fee2e2',
                      color: n.status === 'online' ? 'var(--color-brand-green)' : 'var(--color-brand-red)',
                    }}
                  >
                    {n.status.toUpperCase()}
                  </span>
                </div>
                <div
                  style={{
                    fontSize: 11,
                    color: 'var(--color-brand-text)',
                    fontFamily: 'Fira Code',
                    marginBottom: 8,
                  }}
                >
                  {n.ip}:{n.ssh_port}
                </div>

                <button
                  className="btn btn--xs btn--secondary"
                  style={{ width: '100%', fontSize: 10 }}
                  onClick={() => setSelectedNodeIdForProvision(n.ID)}
                >
                  Trigger SSH Provisioning
                </button>
              </div>
            ))
          )}
        </div>

        {/* Form: Add Node */}
        <form
          onSubmit={handleAddNode}
          style={{
            borderTop: '1px solid var(--color-brand-border)',
            paddingTop: 16,
            display: 'flex',
            flexDirection: 'column',
            gap: 12,
          }}
        >
          <span style={{ fontSize: 12, fontWeight: 700, color: 'var(--color-brand-heading)' }}>Register VPS Node</span>

          <div style={{ display: 'grid', gridTemplateColumns: '1fr 1fr', gap: 12 }}>
            <input
              type="text"
              placeholder="Node Name"
              value={nodeName}
              onChange={(e) => setNodeName(e.target.value)}
              style={{
                padding: '8px 10px',
                borderRadius: 6,
                border: '1px solid var(--color-brand-border)',
                background: 'var(--color-brand-card)',
                fontSize: 12,
                color: 'var(--color-brand-heading)',
              }}
              required
            />
            <input
              type="text"
              placeholder="VPS IP Address"
              value={nodeIP}
              onChange={(e) => setNodeIP(e.target.value)}
              style={{
                padding: '8px 10px',
                borderRadius: 6,
                border: '1px solid var(--color-brand-border)',
                background: 'var(--color-brand-card)',
                fontSize: 12,
                color: 'var(--color-brand-heading)',
              }}
              required
            />
          </div>

          <div style={{ display: 'grid', gridTemplateColumns: '80px 1fr', gap: 12 }}>
            <input
              type="number"
              placeholder="SSH Port"
              value={nodeSSHPort}
              onChange={(e) => setNodeSSHPort(Number(e.target.value))}
              style={{
                padding: '8px 10px',
                borderRadius: 6,
                border: '1px solid var(--color-brand-border)',
                background: 'var(--color-brand-card)',
                fontSize: 12,
                color: 'var(--color-brand-heading)',
              }}
              required
            />
            <input
              type="password"
              placeholder="SSH Private Key / Password"
              value={nodeSSHCreds}
              onChange={(e) => setNodeSSHCreds(e.target.value)}
              style={{
                padding: '8px 10px',
                borderRadius: 6,
                border: '1px solid var(--color-brand-border)',
                background: 'var(--color-brand-card)',
                fontSize: 12,
                color: 'var(--color-brand-heading)',
              }}
            />
          </div>

          <button type="submit" className="btn btn--sm btn--primary" disabled={isLoading}>
            Add Node
          </button>
        </form>
      </div>

      {/* SSH Provision Dialog */}
      {selectedNodeIdForProvision && (
        <form
          onSubmit={handleProvision}
          className="g-card"
          style={{ border: '2px solid var(--color-brand)', background: 'var(--color-brand-light)' }}
        >
          <div style={{ display: 'flex', justifyContent: 'space-between', alignItems: 'center', marginBottom: 12 }}>
            <span style={{ fontSize: 13, fontWeight: 700, color: 'var(--color-brand-heading)' }}>
              SSH Root Credentials Auth
            </span>
            <FiX style={{ cursor: 'pointer' }} onClick={() => setSelectedNodeIdForProvision(null)} />
          </div>
          <p style={{ fontSize: 11, color: 'var(--color-brand-text)', margin: '0 0 12px' }}>
            Enter root password for VPS node execution. The daemon will connect over SSH to install the Xray compiler.
          </p>
          <div style={{ display: 'flex', gap: 10 }}>
            <input
              type="password"
              placeholder="Root Password"
              value={provisionPassword}
              onChange={(e) => setProvisionPassword(e.target.value)}
              style={{
                flex: 1,
                padding: '8px 10px',
                borderRadius: 6,
                border: '1px solid var(--color-brand-border)',
                background: 'var(--color-brand-card)',
                fontSize: 12,
                color: 'var(--color-brand-heading)',
              }}
              required
            />
            <button type="submit" className="btn btn--sm btn--primary" disabled={isLoading}>
              Execute Deploy
            </button>
          </div>
        </form>
      )}
    </>
  );
};

export default EdgeNodesCard;
