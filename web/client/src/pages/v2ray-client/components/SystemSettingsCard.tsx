import React from 'react';
import { FiSettings, FiHelpCircle } from 'react-icons/fi';

interface SystemSettingsCardProps {
  hotkeys: string;
  setHotkeys: (hotkeys: string) => void;
  systemTrayEnabled: boolean;
  setSystemTrayEnabled: (enabled: boolean) => void;
  setMessage: (msg: { type: 'success' | 'error'; text: string } | null) => void;
  showHelp: (title: string, text: string) => void;
}

export const SystemSettingsCard: React.FC<SystemSettingsCardProps> = ({
  hotkeys,
  setHotkeys,
  systemTrayEnabled,
  setSystemTrayEnabled,
  setMessage,
  showHelp,
}) => {
  return (
    <div className="g-card">
      <div style={{ display: 'flex', alignItems: 'center', gap: 6, marginBottom: 14 }}>
        <FiSettings style={{ color: 'var(--color-brand)', fontSize: 16 }} />
        <span style={{ fontSize: 13, fontWeight: 600, color: 'var(--color-brand-heading)' }}>
          System Tray & Keybindings
        </span>
        <FiHelpCircle
          style={{ cursor: 'pointer', color: 'var(--color-brand-muted)' }}
          onClick={() =>
            showHelp(
              'System Tray & Hotkeys',
              'Configure background daemon tray status flags and bind OS-level keyboard shortcuts (e.g. Ctrl+Shift+X) to quickly toggle SOCKS5 proxy status.'
            )
          }
        />
      </div>

      <div style={{ display: 'flex', flexDirection: 'column', gap: 12 }}>
        <div style={{ display: 'flex', alignItems: 'center', justifyContent: 'space-between' }}>
          <span style={{ fontSize: 12, color: 'var(--color-brand-heading)' }}>Enable System Tray Icon</span>
          <input
            type="checkbox"
            checked={systemTrayEnabled}
            onChange={(e) => setSystemTrayEnabled(e.target.checked)}
            style={{ width: 16, height: 16, cursor: 'pointer', accentColor: 'var(--color-brand)' }}
          />
        </div>

        <div style={{ display: 'flex', gap: 10 }}>
          <input
            type="text"
            placeholder="Toggle Hotkey (e.g. Ctrl+Shift+X)"
            value={hotkeys}
            onChange={(e) => setHotkeys(e.target.value)}
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
          <button
            className="btn btn--sm btn--secondary"
            onClick={() => setMessage({ type: 'success', text: 'Hotkey overrides saved!' })}
          >
            Save
          </button>
        </div>
      </div>
    </div>
  );
};

export default SystemSettingsCard;
