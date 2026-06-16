import React, { useEffect, useState } from 'react';
import { FiPlus, FiGlobe, FiCloudLightning, FiAlertTriangle, FiLoader } from 'react-icons/fi';
import { useCloudflareStore } from '../store/cloudflareStore';
import type { CloudflareAccount } from '../store/cloudflareStore';
import { CloudflareStatsCard } from '../components/molecules/CloudflareStatsCard';
import { CloudflareAddModal } from '../components/molecules/CloudflareAddModal';
import { WorkerDeployWizard } from '../components/molecules/WorkerDeployWizard';

export const CloudflarePage: React.FC = () => {
  const { 
    accounts, 
    deployments, 
    fetchAccounts, 
    deleteAccount, 
    fetchDeployments, 
    deleteDeployment, 
    checkDeploymentHealth, 
    isLoading, 
    error 
  } = useCloudflareStore();

  const [showModal, setShowModal] = useState(false);
  const [showDeployWizard, setShowDeployWizard] = useState(false);
  const [editingAccount, setEditingAccount] = useState<CloudflareAccount | null>(null);
  const [checkingIds, setCheckingIds] = useState<number[]>([]);

  useEffect(() => {
    fetchAccounts();
    fetchDeployments();
  }, [fetchAccounts, fetchDeployments]);

  const handleEdit = (account: CloudflareAccount) => {
    setEditingAccount(account);
    setShowModal(true);
  };

  const handleDelete = async (id: number) => {
    if (window.confirm('Are you sure you want to remove this Cloudflare account?')) {
      await deleteAccount(id);
    }
  };

  const handleDeleteDeployment = async (id: number) => {
    if (window.confirm('Are you sure you want to delete this worker deployment record?')) {
      await deleteDeployment(id);
    }
  };

  const handleCheckHealth = async (id: number) => {
    setCheckingIds(prev => [...prev, id]);
    try {
      await checkDeploymentHealth(id);
    } finally {
      setCheckingIds(prev => prev.filter(x => x !== id));
    }
  };

  return (
    <div style={{ display: 'flex', flexDirection: 'column', gap: 20 }}>
      {/* Header Row */}
      <div style={{ display: 'flex', justifyContent: 'space-between', alignItems: 'center' }}>
        <div>
          <h1 style={{ fontSize: 22, fontWeight: 700, margin: 0, color: 'var(--color-brand-heading)' }}>
            Cloudflare Accounts
          </h1>
          <p style={{ fontSize: 13, color: 'var(--color-brand-text)', margin: '4px 0 0' }}>
            Monitor CDN bandwidth caching, zones status, and Workers scripts deployment in real-time.
          </p>
        </div>
        <div style={{ display: 'flex', gap: 12 }}>
          <button
            className="btn"
            style={{ display: 'flex', alignItems: 'center', gap: 6 }}
            onClick={() => setShowDeployWizard(true)}
            disabled={accounts.length === 0}
          >
            <FiCloudLightning size={14} /> Deploy Worker
          </button>
          <button
            className="btn btn--primary"
            onClick={() => {
              setEditingAccount(null);
              setShowModal(true);
            }}
          >
            <FiPlus size={14} /> Add Account
          </button>
        </div>
      </div>

      {/* Error Alert */}
      {error && (
        <div style={{ background: '#fef2f2', border: '1px solid #fecaca', color: '#b91c1c', padding: '12px 16px', borderRadius: 8, fontSize: 13, display: 'flex', alignItems: 'center', gap: 8 }}>
          <FiAlertTriangle size={16} />
          <span>{error}</span>
        </div>
      )}

      {/* Main Grid */}
      {isLoading && accounts.length === 0 ? (
        <div style={{ display: 'flex', flexDirection: 'column', alignItems: 'center', justifyContent: 'center', minHeight: 200, gap: 10 }}>
          <div className="spin" style={{ width: 28, height: 28, border: '3px solid var(--color-brand-border)', borderTopColor: 'var(--color-brand)', borderRadius: '50%' }} />
          <span style={{ fontSize: 13, color: 'var(--color-brand-muted)' }}>Loading accounts...</span>
        </div>
      ) : accounts.length === 0 ? (
        /* Empty State */
        <div className="g-card" style={{ display: 'flex', flexDirection: 'column', alignItems: 'center', justifyContent: 'center', padding: '48px 24px', textAlign: 'center', gap: 16 }}>
          <div style={{ width: 64, height: 64, borderRadius: 16, background: 'var(--color-brand-light)', display: 'flex', alignItems: 'center', justifyContent: 'center', color: 'var(--color-brand)' }}>
            <FiCloudLightning size={32} />
          </div>
          <div>
            <h3 style={{ fontSize: 16, fontWeight: 700, color: 'var(--color-brand-heading)', margin: 0 }}>
              No Cloudflare Accounts Connected
            </h3>
            <p style={{ fontSize: 13, color: 'var(--color-brand-text)', maxWidth: 400, margin: '8px auto 0' }}>
              Connect your Cloudflare account using secure OAuth login to fetch caching analytics, active domains, and Workers scripts deployed on the edge.
            </p>
          </div>
          <button
            className="btn btn--primary"
            onClick={() => {
              setEditingAccount(null);
              setShowModal(true);
            }}
          >
            <FiPlus size={14} /> Add Cloudflare Account
          </button>
        </div>
      ) : (
        /* Accounts Grid */
        <div style={{ display: 'grid', gridTemplateColumns: 'repeat(auto-fill, minmax(320px, 1fr))', gap: 20 }}>
          {accounts.map((acc) => (
            <CloudflareStatsCard
              key={acc.id}
              account={acc}
              onEdit={() => handleEdit(acc)}
              onDelete={() => handleDelete(acc.id)}
            />
          ))}
        </div>
      )}

      {/* Worker Deployments Section */}
      {accounts.length > 0 && (
        <div style={{ marginTop: 10, display: 'flex', flexDirection: 'column', gap: 14 }}>
          <div>
            <h2 style={{ fontSize: 18, fontWeight: 700, margin: 0, color: 'var(--color-brand-heading)' }}>
              Worker Deployments
            </h2>
            <p style={{ fontSize: 13, color: 'var(--color-brand-text)', margin: '4px 0 0' }}>
              Manage automated Cloudflare Edge routing workers, check custom domains, and run diagnostics checks.
            </p>
          </div>

          {deployments.length === 0 ? (
            <div className="g-card" style={{ padding: 24, textAlign: 'center', color: 'var(--color-brand-muted)', fontSize: 13 }}>
              No worker deployments created yet. Click "Deploy Worker" to launch a new instance.
            </div>
          ) : (
            <div className="g-card" style={{ padding: 0, overflow: 'hidden' }}>
              <div style={{ overflowX: 'auto' }}>
                <table style={{ width: '100%', borderCollapse: 'collapse', fontSize: 13, textAlign: 'left', minWidth: 600 }}>
                  <thead>
                    <tr style={{ borderBottom: '1px solid var(--color-brand-border)', background: 'var(--color-brand-light)' }}>
                      <th style={{ padding: '12px 16px', fontWeight: 600, color: 'var(--color-brand-heading)' }}>Script Name</th>
                      <th style={{ padding: '12px 16px', fontWeight: 600, color: 'var(--color-brand-heading)' }}>Endpoints</th>
                      <th style={{ padding: '12px 16px', fontWeight: 600, color: 'var(--color-brand-heading)' }}>Status</th>
                      <th style={{ padding: '12px 16px', fontWeight: 600, color: 'var(--color-brand-heading)', textAlign: 'right' }}>Actions</th>
                    </tr>
                  </thead>
                  <tbody>
                    {deployments.map(dep => {
                      const checking = checkingIds.includes(dep.id);
                      return (
                        <tr key={dep.id} style={{ borderBottom: '1px solid var(--color-brand-border)' }}>
                          <td style={{ padding: '12px 16px', fontWeight: 600, color: 'var(--color-brand-heading)' }}>
                            {dep.script_name}
                          </td>
                          <td style={{ padding: '12px 16px', color: 'var(--color-brand-text)' }}>
                            <div style={{ display: 'flex', flexDirection: 'column', gap: 4 }}>
                              <a href={dep.default_url} target="_blank" rel="noreferrer" style={{ color: 'var(--color-brand)', textDecoration: 'none' }}>
                                {dep.default_url}
                              </a>
                              {dep.custom_domain && (
                                <a href={`https://${dep.custom_domain}`} target="_blank" rel="noreferrer" style={{ color: 'var(--color-brand)', textDecoration: 'none', fontWeight: 500 }}>
                                  https://{dep.custom_domain}
                                </a>
                              )}
                            </div>
                          </td>
                          <td style={{ padding: '12px 16px' }}>
                            <div style={{ display: 'flex', alignItems: 'center', gap: 6 }}>
                              <span style={{ 
                                fontSize: 10, 
                                fontWeight: 700, 
                                padding: '2px 6px', 
                                borderRadius: 10, 
                                color: '#fff', 
                                background: dep.health_status === 'healthy' ? '#22c55e' : dep.health_status === 'unhealthy' ? 'var(--color-brand-red)' : 'var(--color-brand-muted)',
                                textTransform: 'uppercase'
                              }}>
                                {dep.health_status}
                              </span>
                              {dep.message && (
                                <span style={{ fontSize: 11, color: 'var(--color-brand-muted)' }}>
                                  {dep.message}
                                </span>
                              )}
                            </div>
                          </td>
                          <td style={{ padding: '12px 16px', textAlign: 'right' }}>
                            <div style={{ display: 'flex', justifyContent: 'end', gap: 8 }}>
                              <button 
                                className="btn" 
                                style={{ padding: '4px 8px', fontSize: 11, display: 'flex', alignItems: 'center', gap: 4 }}
                                onClick={() => handleCheckHealth(dep.id)}
                                disabled={checking}
                              >
                                {checking && <FiLoader className="spin" size={12} />}
                                Check Health
                              </button>
                              <button 
                                className="btn" 
                                style={{ padding: '4px 8px', fontSize: 11, color: 'var(--color-brand-red)' }}
                                onClick={() => handleDeleteDeployment(dep.id)}
                              >
                                Delete
                              </button>
                            </div>
                          </td>
                        </tr>
                      );
                    })}
                  </tbody>
                </table>
              </div>
            </div>
          )}
        </div>
      )}

      {/* Modals */}
      <CloudflareAddModal
        show={showModal}
        onClose={() => setShowModal(false)}
        accountToEdit={editingAccount}
      />

      <WorkerDeployWizard
        show={showDeployWizard}
        onClose={() => setShowDeployWizard(false)}
      />
    </div>
  );
};
