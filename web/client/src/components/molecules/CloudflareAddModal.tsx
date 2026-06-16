import React, { useState, useEffect } from 'react';
import { FiX, FiCheck, FiLoader, FiGlobe, FiInfo } from 'react-icons/fi';
import { useCloudflareStore } from '../../store/cloudflareStore';
import type { CloudflareAccount } from '../../store/cloudflareStore';

interface CloudflareAddModalProps {
  show: boolean;
  onClose: () => void;
  accountToEdit?: CloudflareAccount | null;
}

export const CloudflareAddModal: React.FC<CloudflareAddModalProps> = ({ show, onClose, accountToEdit }) => {
  const { addAccount, updateAccount, isLoading, error, clearError } = useCloudflareStore();

  const [alias, setAlias] = useState('');
  const [token, setToken] = useState('');
  const [formError, setFormError] = useState<string | null>(null);

  useEffect(() => {
    clearError();
    setFormError(null);
    if (accountToEdit) {
      setAlias(accountToEdit.account_name);
      setToken(''); // Leave token blank when editing unless they want to update it
    } else {
      setAlias('');
      setToken('');
    }
  }, [accountToEdit, show, clearError]);

  if (!show) return null;

  const handleSubmit = async (e: React.FormEvent) => {
    e.preventDefault();
    setFormError(null);

    const cleanAlias = alias.trim();
    const cleanToken = token.trim();

    if (!cleanAlias) {
      setFormError('Account name is required');
      return;
    }

    if (!accountToEdit && !cleanToken) {
      setFormError('API Token is required');
      return;
    }

    try {
      if (accountToEdit) {
        await updateAccount(accountToEdit.id, cleanAlias, cleanToken || undefined);
      } else {
        await addAccount(cleanAlias, cleanToken);
      }
      onClose();
    } catch {
      // Error is stored and shown via useCloudflareStore.error
    }
  };

  return (
    <div style={{ position: 'fixed', top: 0, left: 0, width: '100vw', height: '100vh', background: 'rgba(0,0,0,0.55)', backdropFilter: 'blur(6px)', zIndex: 1000, display: 'flex', alignItems: 'center', justifyContent: 'center' }}>
      <div className="g-card" style={{ width: '100%', maxWidth: 480, maxHeight: '90vh', overflowY: 'auto', display: 'flex', flexDirection: 'column', gap: 16 }}>
        {/* Header */}
        <div style={{ display: 'flex', justifyContent: 'space-between', alignItems: 'center' }}>
          <h2 style={{ fontSize: 18, fontWeight: 700, margin: 0, color: 'var(--color-brand-heading)', display: 'flex', alignItems: 'center', gap: 8 }}>
            <div style={{ width: 28, height: 28, borderRadius: 6, background: 'linear-gradient(135deg, var(--color-brand), #ff9f43)', display: 'flex', alignItems: 'center', justifyContent: 'center' }}>
              <FiGlobe size={14} color="#fff" />
            </div>
            {accountToEdit ? 'Edit Cloudflare Account' : 'Add Cloudflare Account'}
          </h2>
          <button onClick={onClose} style={{ background: 'none', border: 'none', cursor: 'pointer', color: 'var(--color-brand-text)' }}><FiX size={18} /></button>
        </div>

        <form onSubmit={handleSubmit} style={{ display: 'flex', flexDirection: 'column', gap: 14 }}>
          {/* Account Name */}
          <div style={{ display: 'flex', flexDirection: 'column', gap: 4 }}>
            <label style={{ fontSize: 11, fontWeight: 700, color: 'var(--color-brand-muted)', textTransform: 'uppercase' }}>Account Alias Name</label>
            <input
              type="text"
              placeholder="e.g. My Personal Account"
              value={alias}
              onChange={(e) => setAlias(e.target.value)}
              disabled={isLoading}
              style={{ width: '100%', padding: '10px 14px', borderRadius: 8, border: '1px solid var(--color-brand-border)', background: 'var(--color-brand-bg)', outline: 'none', color: 'var(--color-brand-heading)', fontSize: 13 }}
            />
          </div>

          {/* API Token */}
          <div style={{ display: 'flex', flexDirection: 'column', gap: 4 }}>
            <label style={{ fontSize: 11, fontWeight: 700, color: 'var(--color-brand-muted)', textTransform: 'uppercase' }}>
              {accountToEdit ? 'New API Token (Leave blank to keep current)' : 'API Token'}
            </label>
            <input
              type="password"
              placeholder={accountToEdit ? '••••••••••••••••••••••••' : 'Cloudflare API Token'}
              value={token}
              onChange={(e) => setToken(e.target.value)}
              disabled={isLoading}
              style={{ width: '100%', padding: '10px 14px', borderRadius: 8, border: '1px solid var(--color-brand-border)', background: 'var(--color-brand-bg)', outline: 'none', color: 'var(--color-brand-heading)', fontSize: 13, fontFamily: 'monospace' }}
            />
          </div>

          {/* Instructions Box */}
          <div style={{ background: 'var(--color-brand-light)', border: '1px solid var(--color-brand-border)', borderRadius: 8, padding: '10px 12px', fontSize: 11, color: 'var(--color-brand-text)', display: 'flex', flexDirection: 'column', gap: 6 }}>
            <div style={{ display: 'flex', alignItems: 'center', gap: 6, fontWeight: 600, color: 'var(--color-brand)' }}>
              <FiInfo size={14} />
              <span>Token Requirements:</span>
            </div>
            <ul style={{ margin: 0, paddingLeft: 16, display: 'flex', flexDirection: 'column', gap: 3 }}>
              <li><strong>Account:</strong> Memberships (Read)</li>
              <li><strong>Zone:</strong> Zone (Read), Analytics (Read)</li>
              <li><strong>Workers:</strong> Workers Scripts (Read)</li>
            </ul>
            <a
              href="https://dash.cloudflare.com/profile/api-tokens"
              target="_blank"
              rel="noopener noreferrer"
              style={{ color: 'var(--color-brand)', fontWeight: 600, textDecoration: 'none', marginTop: 4, display: 'inline-block' }}
            >
              Create API Token on Cloudflare Dashboard &rarr;
            </a>
          </div>

          {/* Errors display */}
          {(formError || error) && (
            <span style={{ fontSize: 12, color: 'var(--color-brand-red)', fontWeight: 500 }}>
              {formError || error}
            </span>
          )}

          {/* Actions */}
          <div style={{ display: 'flex', justifyContent: 'end', gap: 12, marginTop: 4 }}>
            <button type="button" className="btn" onClick={onClose} disabled={isLoading}>Cancel</button>
            <button type="submit" className="btn btn--primary" disabled={isLoading} style={{ minWidth: 100 }}>
              {isLoading ? (
                <>
                  <FiLoader className="spin" size={14} /> Verifying...
                </>
              ) : (
                <>
                  <FiCheck size={14} /> Save
                </>
              )}
            </button>
          </div>
        </form>
      </div>
    </div>
  );
};
