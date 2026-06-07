import React from 'react';
import { FiActivity, FiHelpCircle } from 'react-icons/fi';

interface TrafficAuditsCardProps {
  trafficLogs: any[];
  showHelp: (title: string, text: string) => void;
}

export const TrafficAuditsCard: React.FC<TrafficAuditsCardProps> = ({
  trafficLogs,
  showHelp,
}) => {
  return (
    <div className="g-card">
      <div style={{ display: 'flex', alignItems: 'center', gap: 6, marginBottom: 14 }}>
        <FiActivity style={{ color: 'var(--color-brand)', fontSize: 18 }} />
        <span style={{ fontSize: 15, fontWeight: 600, color: 'var(--color-brand-heading)' }}>
          User Traffic Audits
        </span>
        <FiHelpCircle
          style={{ cursor: 'pointer', color: 'var(--color-brand-muted)' }}
          onClick={() =>
            showHelp(
              'User Traffic Audits',
              'Displays recent data consumption log entries and metrics tracked by the accounting parser.'
            )
          }
        />
      </div>

      <div style={{ overflowX: 'auto', border: '1px solid var(--color-brand-border)', borderRadius: 8 }}>
        <table style={{ width: '100%', borderCollapse: 'collapse', fontSize: 11, textAlign: 'left' }}>
          <thead>
            <tr style={{ background: 'var(--color-brand-bg)', borderBottom: '1px solid var(--color-brand-border)' }}>
              <th style={{ padding: '8px 10px', color: 'var(--color-brand-heading)' }}>User / UUID</th>
              <th style={{ padding: '8px 10px', color: 'var(--color-brand-heading)' }}>Uploaded</th>
              <th style={{ padding: '8px 10px', color: 'var(--color-brand-heading)' }}>Downloaded</th>
              <th style={{ padding: '8px 10px', color: 'var(--color-brand-heading)' }}>Timestamp</th>
            </tr>
          </thead>
          <tbody>
            {trafficLogs.length === 0 ? (
              <tr>
                <td colSpan={4} style={{ padding: 14, textAlign: 'center', color: 'var(--color-brand-muted)' }}>
                  No traffic events logged.
                </td>
              </tr>
            ) : (
              trafficLogs.slice(0, 15).map((log, idx) => (
                <tr key={idx} style={{ borderBottom: '1px solid var(--color-brand-border)' }}>
                  <td style={{ padding: '8px 10px', fontWeight: 600, color: 'var(--color-brand-heading)' }}>
                    {log.name || log.uuid || 'Client'}
                  </td>
                  <td style={{ padding: '8px 10px' }}>{(log.upload / (1024 * 1024)).toFixed(2)} MB</td>
                  <td style={{ padding: '8px 10px' }}>{(log.download / (1024 * 1024)).toFixed(2)} MB</td>
                  <td style={{ padding: '8px 10px', color: 'var(--color-brand-muted)' }}>
                    {new Date(log.CreatedAt || log.UpdatedAt || Date.now()).toLocaleTimeString()}
                  </td>
                </tr>
              ))
            )}
          </tbody>
        </table>
      </div>
    </div>
  );
};

export default TrafficAuditsCard;
