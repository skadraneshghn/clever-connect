import React from 'react';
import { FiMonitor, FiHelpCircle } from 'react-icons/fi';

interface WakeOnLanCardProps {
  isLoading: boolean;
  wolMac: string;
  setWolMac: (mac: string) => void;
  wolBcast: string;
  setWolBcast: (bcast: string) => void;
  handleSendWol: () => void;
  showHelp: (title: string, text: string) => void;
}

export const WakeOnLanCard: React.FC<WakeOnLanCardProps> = ({
  isLoading,
  wolMac,
  setWolMac,
  wolBcast,
  setWolBcast,
  handleSendWol,
  showHelp,
}) => {
  return (
    <div className="g-card">
      <div style={{ display: 'flex', alignItems: 'center', gap: 6, marginBottom: 14 }}>
        <FiMonitor style={{ color: 'var(--color-brand)', fontSize: 16 }} />
        <span style={{ fontSize: 13, fontWeight: 600, color: 'var(--color-brand-heading)' }}>Wake-on-LAN (WOL) Client</span>
        <FiHelpCircle
          style={{ cursor: 'pointer', color: 'var(--color-brand-muted)' }}
          onClick={() =>
            showHelp(
              'Wake-on-LAN (WOL)',
              'Sends a UDP magic packet (0xFF repeat + MAC address repeat) to boot remote network hardware from sleep state.'
            )
          }
        />
      </div>

      <div style={{ display: 'flex', flexDirection: 'column', gap: 10 }}>
        <input
          type="text"
          placeholder="Target MAC Address (e.g. AA:BB:CC:DD:EE:FF)"
          value={wolMac}
          onChange={(e) => setWolMac(e.target.value)}
          style={{
            padding: '8px 10px',
            borderRadius: 6,
            border: '1px solid var(--color-brand-border)',
            background: 'var(--color-brand-card)',
            fontSize: 12,
            color: 'var(--color-brand-heading)',
          }}
        />
        <div style={{ display: 'flex', gap: 10 }}>
          <input
            type="text"
            placeholder="Broadcast IP (default 255.255.255.255)"
            value={wolBcast}
            onChange={(e) => setWolBcast(e.target.value)}
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
          <button className="btn btn--primary btn--sm" onClick={handleSendWol} disabled={isLoading}>
            Wake
          </button>
        </div>
      </div>
    </div>
  );
};

export default WakeOnLanCard;
