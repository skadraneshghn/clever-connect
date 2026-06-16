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
  const { updateAccount, addManualAccount, fetchAccounts, isLoading, error, clearError } = useCloudflareStore();

  const [alias, setAlias] = useState('');
  const [formError, setFormError] = useState<string | null>(null);
  const [isOAuthWaiting, setIsOAuthWaiting] = useState(false);
  const [activeTab, setActiveTab] = useState<'oauth' | 'manual'>('oauth');
  const [authType, setAuthType] = useState<'token' | 'key'>('token');
  const [token, setToken] = useState('');
  const [email, setEmail] = useState('');

  useEffect(() => {
    clearError();
    setFormError(null);
    setIsOAuthWaiting(false);
    setActiveTab('oauth');
    setAuthType('token');
    setToken('');
    setEmail('');
    if (accountToEdit) {
      setAlias(accountToEdit.account_name);
    } else {
      setAlias('');
    }
  }, [accountToEdit, show, clearError]);

  // Listen to the backend callback posting "cloudflare_auth_success"
  useEffect(() => {
    if (!show) return;

    const handleOAuthMessage = (event: MessageEvent) => {
      if (event.data === 'cloudflare_auth_success') {
        setIsOAuthWaiting(false);
        fetchAccounts();
        onClose();
      }
    };

    window.addEventListener('message', handleOAuthMessage);
    return () => {
      window.removeEventListener('message', handleOAuthMessage);
    };
  }, [show, fetchAccounts, onClose]);

  if (!show) return null;

  const handleOAuthConnect = () => {
    setFormError(null);
    const cleanAlias = alias.trim();
    if (!cleanAlias) {
      setFormError('Account name is required to start authorization');
      return;
    }

    setIsOAuthWaiting(true);
    const width = 600;
    const height = 700;
    const left = window.screen.width / 2 - width / 2;
    const top = window.screen.height / 2 - height / 2;

    const authUrl = `/api/cloudflare/oauth/login?alias=${encodeURIComponent(cleanAlias)}`;
    window.open(
      authUrl,
      'CloudflareAuth',
      `width=${width},height=${height},top=${top},left=${left},scrollbars=yes,status=yes`
    );
  };

  const handleManualConnect = async (e: React.FormEvent) => {
    e.preventDefault();
    setFormError(null);
    const cleanAlias = alias.trim();
    const cleanToken = token.trim();
    const cleanEmail = email.trim();

    if (!cleanAlias) {
      setFormError('Account name / alias is required');
      return;
    }
    if (!cleanToken) {
      setFormError(authType === 'token' ? 'API Token is required' : 'Global API Key is required');
      return;
    }
    if (authType === 'key' && !cleanEmail) {
      setFormError('Email address is required for Global API Key');
      return;
    }

    try {
      await addManualAccount({
        account_name: cleanAlias,
        auth_type: authType,
        token: cleanToken,
        email: authType === 'key' ? cleanEmail : undefined,
      });
      onClose();
    } catch {
      // Error is handled by store
    }
  };

  const handleEditSubmit = async (e: React.FormEvent) => {
    e.preventDefault();
    setFormError(null);

    const cleanAlias = alias.trim();
    if (!cleanAlias) {
      setFormError('Account name is required');
      return;
    }

    if (!accountToEdit) return;

    try {
      await updateAccount(accountToEdit.id, cleanAlias);
      onClose();
    } catch {
      // Error is handled by store
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
            {accountToEdit ? 'Edit Cloudflare Account' : 'Connect Cloudflare Account'}
          </h2>
          <button onClick={onClose} disabled={isLoading || isOAuthWaiting} style={{ background: 'none', border: 'none', cursor: 'pointer', color: 'var(--color-brand-text)' }}><FiX size={18} /></button>
        </div>

        {/* Tabs */}
        {!accountToEdit && (
          <div style={{ display: 'flex', borderBottom: '1px solid var(--color-brand-border)', marginBottom: 8 }}>
            <button
              type="button"
              onClick={() => setActiveTab('oauth')}
              style={{
                flex: 1,
                padding: '10px 0',
                background: 'none',
                border: 'none',
                borderBottom: activeTab === 'oauth' ? '2px solid var(--color-brand)' : 'none',
                color: activeTab === 'oauth' ? 'var(--color-brand)' : 'var(--color-brand-muted)',
                fontWeight: 600,
                fontSize: 13,
                cursor: 'pointer',
                outline: 'none'
              }}
            >
              OAuth Flow
            </button>
            <button
              type="button"
              onClick={() => setActiveTab('manual')}
              style={{
                flex: 1,
                padding: '10px 0',
                background: 'none',
                border: 'none',
                borderBottom: activeTab === 'manual' ? '2px solid var(--color-brand)' : 'none',
                color: activeTab === 'manual' ? 'var(--color-brand)' : 'var(--color-brand-muted)',
                fontWeight: 600,
                fontSize: 13,
                cursor: 'pointer',
                outline: 'none'
              }}
            >
              Manual API Config
            </button>
          </div>
        )}

        {accountToEdit ? (
          /* EDIT ALIAS FORM */
          <form onSubmit={handleEditSubmit} style={{ display: 'flex', flexDirection: 'column', gap: 14 }}>
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

            {(formError || error) && (
              <span style={{ fontSize: 12, color: 'var(--color-brand-red)', fontWeight: 500 }}>
                {formError || error}
              </span>
            )}

            <div style={{ display: 'flex', justifyContent: 'end', gap: 12, marginTop: 4 }}>
              <button type="button" className="btn" onClick={onClose} disabled={isLoading}>Cancel</button>
              <button type="submit" className="btn btn--primary" disabled={isLoading} style={{ minWidth: 100 }}>
                {isLoading ? <FiLoader className="spin" size={14} /> : <><FiCheck size={14} /> Save</>}
              </button>
            </div>
          </form>
        ) : activeTab === 'oauth' ? (
          /* ADD OAUTH FLOW */
          <div style={{ display: 'flex', flexDirection: 'column', gap: 14 }}>
            <div style={{ display: 'flex', flexDirection: 'column', gap: 4 }}>
              <label style={{ fontSize: 11, fontWeight: 700, color: 'var(--color-brand-muted)', textTransform: 'uppercase' }}>Account Alias Name</label>
              <input
                type="text"
                placeholder="e.g. My Personal Account"
                value={alias}
                onChange={(e) => setAlias(e.target.value)}
                disabled={isOAuthWaiting}
                style={{ width: '100%', padding: '10px 14px', borderRadius: 8, border: '1px solid var(--color-brand-border)', background: 'var(--color-brand-bg)', outline: 'none', color: 'var(--color-brand-heading)', fontSize: 13 }}
              />
            </div>

            <div style={{ background: 'var(--color-brand-light)', border: '1px solid var(--color-brand-border)', borderRadius: 8, padding: '10px 12px', fontSize: 11, color: 'var(--color-brand-text)', display: 'flex', flexDirection: 'column', gap: 6 }}>
              <div style={{ display: 'flex', alignItems: 'center', gap: 6, fontWeight: 600, color: 'var(--color-brand)' }}>
                <FiInfo size={14} />
                <span>OAuth Integration Flow:</span>
              </div>
              <p style={{ margin: 0, lineHeight: 1.4 }}>
                CleverConnect uses secure Cloudflare OAuth2 clients. Click the button below to open a login popup. You will login to Cloudflare and authorize CleverConnect to access your account stats dynamically.
              </p>
            </div>

            {(formError || error) && (
              <span style={{ fontSize: 12, color: 'var(--color-brand-red)', fontWeight: 500 }}>
                {formError || error}
              </span>
            )}

            <div style={{ display: 'flex', justifyContent: 'end', gap: 12, marginTop: 4 }}>
              <button type="button" className="btn" onClick={onClose} disabled={isOAuthWaiting}>Cancel</button>
              <button
                type="button"
                className="btn btn--primary"
                onClick={handleOAuthConnect}
                disabled={isOAuthWaiting}
                style={{ minWidth: 160 }}
              >
                {isOAuthWaiting ? (
                  <>
                    <FiLoader className="spin" size={14} /> Waiting for login...
                  </>
                ) : (
                  'Connect with Cloudflare'
                )}
              </button>
            </div>
          </div>
        ) : (
          /* MANUAL CONNECTION FORM */
          <form onSubmit={handleManualConnect} style={{ display: 'flex', flexDirection: 'column', gap: 14 }}>
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

            <div style={{ display: 'flex', flexDirection: 'column', gap: 6 }}>
              <label style={{ fontSize: 11, fontWeight: 700, color: 'var(--color-brand-muted)', textTransform: 'uppercase' }}>Authentication Type</label>
              <div style={{ display: 'flex', gap: 20 }}>
                <label style={{ display: 'flex', alignItems: 'center', gap: 6, fontSize: 13, color: 'var(--color-brand-heading)', cursor: 'pointer' }}>
                  <input
                    type="radio"
                    name="authType"
                    checked={authType === 'token'}
                    onChange={() => setAuthType('token')}
                    disabled={isLoading}
                    style={{ accentColor: 'var(--color-brand)' }}
                  />
                  API Token (Recommended)
                </label>
                <label style={{ display: 'flex', alignItems: 'center', gap: 6, fontSize: 13, color: 'var(--color-brand-heading)', cursor: 'pointer' }}>
                  <input
                    type="radio"
                    name="authType"
                    checked={authType === 'key'}
                    onChange={() => setAuthType('key')}
                    disabled={isLoading}
                    style={{ accentColor: 'var(--color-brand)' }}
                  />
                  Global API Key
                </label>
              </div>
            </div>

            {authType === 'key' && (
              <div style={{ display: 'flex', flexDirection: 'column', gap: 4 }}>
                <label style={{ fontSize: 11, fontWeight: 700, color: 'var(--color-brand-muted)', textTransform: 'uppercase' }}>Cloudflare Email</label>
                <input
                  type="email"
                  placeholder="e.g. user@example.com"
                  value={email}
                  onChange={(e) => setEmail(e.target.value)}
                  disabled={isLoading}
                  style={{ width: '100%', padding: '10px 14px', borderRadius: 8, border: '1px solid var(--color-brand-border)', background: 'var(--color-brand-bg)', outline: 'none', color: 'var(--color-brand-heading)', fontSize: 13 }}
                />
              </div>
            )}

            <div style={{ display: 'flex', flexDirection: 'column', gap: 4 }}>
              <label style={{ fontSize: 11, fontWeight: 700, color: 'var(--color-brand-muted)', textTransform: 'uppercase' }}>
                {authType === 'token' ? 'API Token' : 'Global API Key'}
              </label>
              <input
                type="password"
                placeholder={authType === 'token' ? "Enter your scoped Cloudflare API Token" : "Enter your Global API Key"}
                value={token}
                onChange={(e) => setToken(e.target.value)}
                disabled={isLoading}
                style={{ width: '100%', padding: '10px 14px', borderRadius: 8, border: '1px solid var(--color-brand-border)', background: 'var(--color-brand-bg)', outline: 'none', color: 'var(--color-brand-heading)', fontSize: 13 }}
              />
            </div>

            <div style={{ background: 'var(--color-brand-light)', border: '1px solid var(--color-brand-border)', borderRadius: 8, padding: '10px 12px', fontSize: 11, color: 'var(--color-brand-text)', display: 'flex', flexDirection: 'column', gap: 6 }}>
              <div style={{ display: 'flex', alignItems: 'center', gap: 6, fontWeight: 600, color: 'var(--color-brand)' }}>
                <FiInfo size={14} />
                <span>Manual Token Validation:</span>
              </div>
              <p style={{ margin: 0, lineHeight: 1.4 }}>
                {authType === 'token' 
                  ? "Make sure your API Token has 'Account: Read', 'Workers Scripts: Edit', and 'Zone: Read' permissions."
                  : "Using a Global API Key grants full access. It requires your Cloudflare account email address to authenticate."}
              </p>
            </div>

            {(formError || error) && (
              <span style={{ fontSize: 12, color: 'var(--color-brand-red)', fontWeight: 500 }}>
                {formError || error}
              </span>
            )}

            <div style={{ display: 'flex', justifyContent: 'end', gap: 12, marginTop: 4 }}>
              <button type="button" className="btn" onClick={onClose} disabled={isLoading}>Cancel</button>
              <button type="submit" className="btn btn--primary" disabled={isLoading} style={{ minWidth: 120 }}>
                {isLoading ? <FiLoader className="spin" size={14} /> : <><FiCheck size={14} /> Connect</>}
              </button>
            </div>
          </form>
        )}
      </div>
    </div>
  );
};
