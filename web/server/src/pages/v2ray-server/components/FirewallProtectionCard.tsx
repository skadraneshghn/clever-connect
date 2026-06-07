import React from 'react';
import { FiLock } from 'react-icons/fi';

interface FirewallProtectionCardProps {
  blockIP: string;
  setBlockIP: (ip: string) => void;
  firewallIPs: string[];
  handleBlockIP: (e: React.FormEvent) => void;
}

export const FirewallProtectionCard: React.FC<FirewallProtectionCardProps> = ({
  blockIP,
  setBlockIP,
  firewallIPs,
  handleBlockIP,
}) => {
  return (
    <div className="g-card">
      <div style={{ display: 'flex', alignItems: 'center', gap: 6, marginBottom: 14 }}>
        <FiLock style={{ color: 'var(--color-brand)', fontSize: 16 }} />
        <span style={{ fontSize: 13, fontWeight: 600, color: 'var(--color-brand-heading)' }}>
          Fail2ban port protection
        </span>
      </div>

      <form onSubmit={handleBlockIP} style={{ display: 'flex', gap: 10, marginBottom: 12 }}>
        <input
          type="text"
          placeholder="Block Malicious IP"
          value={blockIP}
          onChange={(e) => setBlockIP(e.target.value)}
          style={{
            flex: 1,
            padding: '8px 10px',
            borderRadius: 6,
            border: '1px solid var(--color-brand-border)',
            background: 'var(--color-brand-card)',
            fontSize: 12,
            color: 'var(--color-brand-heading)',
          }}
        />
        <button type="submit" className="btn btn--primary btn--sm">
          Block
        </button>
      </form>

      {firewallIPs.length > 0 && (
        <div
          style={{
            background: 'var(--color-brand-bg)',
            borderRadius: 6,
            padding: 8,
            fontSize: 11,
            border: '1px solid var(--color-brand-border)',
          }}
        >
          <div style={{ fontWeight: 700, color: 'var(--color-brand-heading)', marginBottom: 4 }}>
            Blocked IPs:
          </div>
          {firewallIPs.map((ip, idx) => (
            <div key={idx} style={{ color: 'var(--color-brand-red)', fontFamily: 'Fira Code' }}>
              {ip} (Blocked via kernel iptables)
            </div>
          ))}
        </div>
      )}
    </div>
  );
};

export default FirewallProtectionCard;
