import React, { useEffect, useState } from 'react';
import { FiPlus, FiGlobe, FiCloudLightning, FiAlertTriangle } from 'react-icons/fi';
import { useCloudflareStore } from '../store/cloudflareStore';
import type { CloudflareAccount } from '../store/cloudflareStore';
import { CloudflareStatsCard } from '../components/molecules/CloudflareStatsCard';
import { CloudflareAddModal } from '../components/molecules/CloudflareAddModal';

export const CloudflarePage: React.FC = () => {
  const { accounts, fetchAccounts, deleteAccount, isLoading, error } = useCloudflareStore();
  const [showModal, setShowModal] = useState(false);
  const [editingAccount, setEditingAccount] = useState<CloudflareAccount | null>(null);

  useEffect(() => {
    fetchAccounts();
  }, [fetchAccounts]);

  const handleEdit = (account: CloudflareAccount) => {
    setEditingAccount(account);
    setShowModal(true);
  };

  const handleDelete = async (id: number) => {
    if (window.confirm('Are you sure you want to remove this Cloudflare account?')) {
      await deleteAccount(id);
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
              Connect your Cloudflare account using an API Token to fetch caching analytics, active domains, and Workers scripts deployed on the edge.
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

      {/* Modal Dialog */}
      <CloudflareAddModal
        show={showModal}
        onClose={() => setShowModal(false)}
        accountToEdit={editingAccount}
      />
    </div>
  );
};
