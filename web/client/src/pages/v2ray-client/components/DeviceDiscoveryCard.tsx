import React from 'react';
import { FiWifi, FiHelpCircle } from 'react-icons/fi';

interface DeviceDiscoveryCardProps {
  isDiscovering: boolean;
  discoveredDevices: any[];
  handleDiscoverDevices: () => void;
  showHelp: (title: string, text: string) => void;
}

export const DeviceDiscoveryCard: React.FC<DeviceDiscoveryCardProps> = ({
  isDiscovering,
  discoveredDevices,
  handleDiscoverDevices,
  showHelp,
}) => {
  return (
    <div className="g-card">
      <div style={{ display: 'flex', alignItems: 'center', justifyContent: 'space-between', marginBottom: 14 }}>
        <div style={{ display: 'flex', alignItems: 'center', gap: 6 }}>
          <FiWifi style={{ color: 'var(--color-brand)', fontSize: 16 }} />
          <span style={{ fontSize: 13, fontWeight: 600, color: 'var(--color-brand-heading)' }}>
            Subnet Service Discovery
          </span>
          <FiHelpCircle
            style={{ cursor: 'pointer', color: 'var(--color-brand-muted)' }}
            onClick={() =>
              showHelp(
                'Subnet Service Discovery',
                'Sweeps the local IPv4 subnet (using TCP port 80/22 checks) to auto-discover active network devices.'
              )
            }
          />
        </div>
        <button className="btn btn--sm btn--secondary" onClick={handleDiscoverDevices} disabled={isDiscovering}>
          {isDiscovering ? 'Scanning...' : 'Scan Subnet'}
        </button>
      </div>

      {discoveredDevices.length > 0 && (
        <div
          style={{
            maxHeight: 150,
            overflowY: 'auto',
            background: 'var(--color-brand-bg)',
            borderRadius: 6,
            padding: 8,
            border: '1px solid var(--color-brand-border)',
          }}
        >
          {discoveredDevices.map((d, idx) => (
            <div key={idx} style={{ display: 'flex', justifyContent: 'space-between', fontSize: 11, marginBottom: 4 }}>
              <div>
                <span style={{ color: 'var(--color-brand-heading)', fontWeight: 600, fontFamily: 'Fira Code' }}>
                  {d.ip}
                </span>
                {d.hostname && <span style={{ color: 'var(--color-brand-muted)', marginLeft: 6 }}>({d.hostname})</span>}
              </div>
              <span style={{ color: 'var(--color-brand-green)', fontWeight: 700 }}>ACTIVE</span>
            </div>
          ))}
        </div>
      )}
    </div>
  );
};

export default DeviceDiscoveryCard;
