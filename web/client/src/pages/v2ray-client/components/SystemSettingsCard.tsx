import React from 'react';
import { FiSettings, FiHelpCircle, FiCode } from 'react-icons/fi';
import { AdvancedCoreConfigModal } from './AdvancedCoreConfigModal';

interface SystemSettingsCardProps {
  hotkeys: string;
  setHotkeys: (hotkeys: string) => void;
  systemTrayEnabled: boolean;
  setSystemTrayEnabled: (enabled: boolean) => void;
  dpiBypassEnabled: boolean;
  setDpiBypassEnabled: (enabled: boolean) => void;
  dpiBypassArgs: string;
  setDpiBypassArgs: (args: string) => void;
  setMessage: (msg: { type: 'success' | 'error'; text: string } | null) => void;
  showHelp: (title: string, text: string) => void;
  onSave: () => void;
  selectedCore: string;
}

export const SystemSettingsCard: React.FC<SystemSettingsCardProps> = ({
  hotkeys,
  setHotkeys,
  systemTrayEnabled,
  setSystemTrayEnabled,
  dpiBypassEnabled,
  setDpiBypassEnabled,
  dpiBypassArgs,
  setDpiBypassArgs,
  setMessage,
  showHelp,
  onSave,
  selectedCore,
}) => {
  const [showCoreModal, setShowCoreModal] = React.useState(false);

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
            onClick={onSave}
          >
            Save
          </button>
        </div>

        {/* DPI Bypass settings */}
        <hr style={{ border: 'none', borderTop: '1px solid var(--color-brand-border)', margin: '4px 0' }} />
        
        <div style={{ display: 'flex', alignItems: 'center', justifyContent: 'space-between' }}>
          <span style={{ fontSize: 12, color: 'var(--color-brand-heading)' }}>
            <strong>Raw Socket DPI Evasion (Requires Root)</strong>
          </span>
          <input
            type="checkbox"
            checked={dpiBypassEnabled}
            onChange={(e) => setDpiBypassEnabled(e.target.checked)}
            style={{ width: 16, height: 16, cursor: 'pointer', accentColor: 'var(--color-brand)' }}
          />
        </div>

        {dpiBypassEnabled && (
          <div style={{ display: 'flex', flexDirection: 'column', gap: 6 }}>
            <span style={{ fontSize: 11, color: 'var(--color-brand-muted)' }}>
              Enter ByeByeDPI style CLI arguments for TCP desync (e.g. <code style={{background: 'var(--color-brand-light)', padding: '2px 4px', borderRadius: 4}}>-d1+s -t 5</code>)
            </span>
            <div style={{ display: 'flex', gap: 10 }}>
              <input
                type="text"
                placeholder="-d1+s -t 5"
                value={dpiBypassArgs}
                onChange={(e) => setDpiBypassArgs(e.target.value)}
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
                className="btn btn--sm btn--primary"
                onClick={onSave}
              >
                Save Params
              </button>
            </div>
          </div>
        )}

        {/* Core Config Button */}
        <hr style={{ border: 'none', borderTop: '1px solid var(--color-brand-border)', margin: '4px 0' }} />
        <div style={{ display: 'flex', alignItems: 'center', justifyContent: 'space-between' }}>
          <span style={{ fontSize: 12, color: 'var(--color-brand-heading)' }}>
            <strong>Advanced Core Configuration</strong>
          </span>
          <button
            onClick={() => setShowCoreModal(true)}
            className="btn btn--sm btn--secondary"
            style={{ display: 'flex', alignItems: 'center', gap: 6 }}
          >
            <FiCode /> Edit Template
          </button>
        </div>

      </div>

      {showCoreModal && (
        <AdvancedCoreConfigModal
          selectedCore={selectedCore}
          onClose={() => setShowCoreModal(false)}
        />
      )}
    </div>
  );
};

export default SystemSettingsCard;
