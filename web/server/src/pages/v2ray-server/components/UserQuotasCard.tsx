import React from 'react';
import { FiUsers, FiHelpCircle } from 'react-icons/fi';

interface UserQuotasCardProps {
  isLoading: boolean;
  users: any[];
  inbounds: any[];
  userName: string;
  setUserName: (name: string) => void;
  userUUID: string;
  setUserUUID: (uuid: string) => void;
  userInboundId: number | string;
  setUserInboundId: React.Dispatch<React.SetStateAction<number | string>>;
  userLimitGB: number;
  setUserLimitGB: React.Dispatch<React.SetStateAction<number>>;
  handleAddUser: (e: React.FormEvent) => void;
  showHelp: (title: string, text: string) => void;
}

export const UserQuotasCard: React.FC<UserQuotasCardProps> = ({
  isLoading,
  users,
  inbounds,
  userName,
  setUserName,
  userUUID,
  setUserUUID,
  userInboundId,
  setUserInboundId,
  userLimitGB,
  setUserLimitGB,
  handleAddUser,
  showHelp,
}) => {
  return (
    <div className="g-card">
      <div style={{ display: 'flex', alignItems: 'center', gap: 8, marginBottom: 16 }}>
        <FiUsers style={{ color: 'var(--color-brand)', fontSize: 18 }} />
        <span style={{ fontSize: 15, fontWeight: 600, color: 'var(--color-brand-heading)' }}>
          User Quota Auditing
        </span>
        <FiHelpCircle
          style={{ cursor: 'pointer', color: 'var(--color-brand-muted)' }}
          onClick={() =>
            showHelp(
              'User Quota Auditing',
              'Manages client proxy profiles and data metrics. Shows realtime upload/download volumes. Users crossing limits are automatically blocked by the scheduler.'
            )
          }
        />
      </div>

      {/* Users list */}
      <div style={{ overflowX: 'auto', border: '1px solid var(--color-brand-border)', borderRadius: 8, marginBottom: 20 }}>
        <table style={{ width: '100%', borderCollapse: 'collapse', fontSize: 11, textAlign: 'left' }}>
          <thead>
            <tr style={{ background: 'var(--color-brand-bg)', borderBottom: '1px solid var(--color-brand-border)' }}>
              <th style={{ padding: '8px 10px', color: 'var(--color-brand-heading)' }}>Username</th>
              <th style={{ padding: '8px 10px', color: 'var(--color-brand-heading)' }}>UUID / Token</th>
              <th style={{ padding: '8px 10px', color: 'var(--color-brand-heading)' }}>Traffic Used</th>
              <th style={{ padding: '8px 10px', color: 'var(--color-brand-heading)' }}>Quota Limit</th>
            </tr>
          </thead>
          <tbody>
            {users.length === 0 ? (
              <tr>
                <td colSpan={4} style={{ padding: 14, textAlign: 'center', color: 'var(--color-brand-muted)' }}>
                  No user accounts active.
                </td>
              </tr>
            ) : (
              users.map((u) => (
                <tr key={u.ID} style={{ borderBottom: '1px solid var(--color-brand-border)' }}>
                  <td style={{ padding: '8px 10px', fontWeight: 600, color: 'var(--color-brand-heading)' }}>{u.name}</td>
                  <td style={{ padding: '8px 10px', fontFamily: 'Fira Code', fontSize: 10 }}>{u.uuid}</td>
                  <td style={{ padding: '8px 10px' }}>
                    {((u.used_upload + u.used_download) / (1024 * 1024 * 1024)).toFixed(2)} GB
                  </td>
                  <td style={{ padding: '8px 10px' }}>
                    {u.traffic_limit > 0
                      ? `${(u.traffic_limit / (1024 * 1024 * 1024)).toFixed(0)} GB`
                      : 'Unlimited'}
                  </td>
                </tr>
              ))
            )}
          </tbody>
        </table>
      </div>

      {/* Form: Add User */}
      <form
        onSubmit={handleAddUser}
        style={{
          borderTop: '1px solid var(--color-brand-border)',
          paddingTop: 16,
          display: 'flex',
          flexDirection: 'column',
          gap: 12,
        }}
      >
        <span style={{ fontSize: 12, fontWeight: 700, color: 'var(--color-brand-heading)' }}>Create Client Account</span>

        <div style={{ display: 'grid', gridTemplateColumns: '1fr 1fr', gap: 12 }}>
          <input
            type="text"
            placeholder="Client Profile Name"
            value={userName}
            onChange={(e) => setUserName(e.target.value)}
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
            placeholder="UUID / Pass (Auto-generated if empty)"
            value={userUUID}
            onChange={(e) => setUserUUID(e.target.value)}
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

        <div style={{ display: 'grid', gridTemplateColumns: '1fr 1fr', gap: 12 }}>
          <select
            value={userInboundId}
            onChange={(e) => setUserInboundId(e.target.value)}
            style={{
              padding: '8px 10px',
              borderRadius: 6,
              border: '1px solid var(--color-brand-border)',
              background: 'var(--color-brand-card)',
              fontSize: 12,
              color: 'var(--color-brand-heading)',
            }}
            required
          >
            <option value="">Choose target Inbound</option>
            {inbounds.map((inb) => (
              <option key={inb.ID} value={inb.ID}>
                {inb.tag} (Port: {inb.port})
              </option>
            ))}
          </select>
          <input
            type="number"
            placeholder="Quota Limit (GB) - 0 for Unlim"
            value={userLimitGB}
            onChange={(e) => setUserLimitGB(Number(e.target.value))}
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

        <button type="submit" className="btn btn--sm btn--primary" disabled={isLoading}>
          Create User
        </button>
      </form>
    </div>
  );
};

export default UserQuotasCard;
