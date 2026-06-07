import React, { useRef, useEffect } from 'react';
import { useVirtualizer } from '@tanstack/react-virtual';
import { FiDownloadCloud, FiHelpCircle, FiActivity, FiTrash2 } from 'react-icons/fi';

interface SubscriptionsCardProps {
  isLoading: boolean;
  subUrl: string;
  setSubUrl: (url: string) => void;
  manualUri: string;
  setManualUri: (uri: string) => void;
  profiles: any[];
  totalProfiles: number;
  activeProfileId: number | null;
  selectedProfileIds: number[];
  setSelectedProfileIds: (ids: number[]) => void;
  handleTestLatency: () => void;
  handleExportPDF: () => void;
  handleImportSub: () => void;
  handleManualImport: () => void;
  handleDeleteAllNodes: () => void;
  handleQRImport: (e: React.ChangeEvent<HTMLInputElement>) => void;
  qrFileInputRef: React.RefObject<HTMLInputElement | null>;
  fetchProfiles: (offset: number, reset?: boolean) => void;
  pageOffset: number;
  handleSelectProfile: (id: number) => void;
  handleDeleteProfile: (id: number) => void;
  showHelp: (title: string, text: string) => void;
  openClipboardModal: () => void;
}

export const SubscriptionsCard: React.FC<SubscriptionsCardProps> = ({
  isLoading,
  subUrl,
  setSubUrl,
  manualUri,
  setManualUri,
  profiles,
  totalProfiles,
  activeProfileId,
  selectedProfileIds,
  setSelectedProfileIds,
  handleTestLatency,
  handleExportPDF,
  handleImportSub,
  handleManualImport,
  handleDeleteAllNodes,
  handleQRImport,
  qrFileInputRef,
  fetchProfiles,
  pageOffset,
  handleSelectProfile,
  handleDeleteProfile,
  showHelp,
  openClipboardModal,
}) => {
  const PAGE_LIMIT = 50;
  const parentRef = useRef<HTMLDivElement>(null);

  const rowVirtualizer = useVirtualizer({
    count: profiles.length < totalProfiles ? profiles.length + 1 : profiles.length,
    getScrollElement: () => parentRef.current,
    estimateSize: () => 48,
    overscan: 5,
  });

  useEffect(() => {
    const virtualItems = rowVirtualizer.getVirtualItems();
    if (!virtualItems.length) return;
    const lastItem = virtualItems[virtualItems.length - 1];

    if (
      lastItem.index >= profiles.length - 1 &&
      profiles.length < totalProfiles &&
      !isLoading
    ) {
      fetchProfiles(pageOffset + PAGE_LIMIT);
    }
  }, [
    rowVirtualizer.getVirtualItems(),
    profiles.length,
    totalProfiles,
    isLoading,
    pageOffset,
    fetchProfiles,
  ]);

  const getLatencyColor = (ms: number) => {
    if (ms <= 0) return 'var(--color-brand-muted)';
    if (ms < 100) return 'var(--color-brand-green)';
    if (ms < 300) return '#f59e0b';
    return 'var(--color-brand-red)';
  };

  return (
    <div className="g-card">
      <div style={{ display: 'flex', alignItems: 'center', justifyContent: 'space-between', marginBottom: 16 }}>
        <div style={{ display: 'flex', alignItems: 'center', gap: 8 }}>
          <FiDownloadCloud style={{ color: 'var(--color-brand)', fontSize: 18 }} />
          <span style={{ fontSize: 15, fontWeight: 600, color: 'var(--color-brand-heading)' }}>Subscriptions & Profiles</span>
          <FiHelpCircle
            style={{ cursor: 'pointer', color: 'var(--color-brand-muted)' }}
            onClick={() =>
              showHelp(
                'Subscriptions & Profiles',
                'Sync and import remote proxy configuration files. Add URL links to sync periodically, or paste raw URI configurations directly. Use QR image uploads or batch export profiles as PDF files.'
              )
            }
          />
        </div>
        <div style={{ display: 'flex', gap: 8 }}>
          <button className="btn btn--sm btn--secondary" onClick={handleTestLatency} disabled={isLoading}>
            <FiActivity style={{ marginRight: 6 }} /> Test Latencies
          </button>
          <button
            className="btn btn--sm btn--primary"
            onClick={handleExportPDF}
            disabled={isLoading || selectedProfileIds.length === 0}
          >
            Export Selected PDF ({selectedProfileIds.length})
          </button>
        </div>
      </div>

      {/* Input URL */}
      <div style={{ display: 'flex', gap: 10, marginBottom: 12 }}>
        <input
          type="text"
          placeholder="Subscription Link (HTTP/S Base64)"
          value={subUrl}
          onChange={(e) => setSubUrl(e.target.value)}
          style={{
            flex: 1,
            padding: '8px 12px',
            borderRadius: 8,
            border: '1px solid var(--color-brand-border)',
            background: 'var(--color-brand-card)',
            fontSize: 13,
            color: 'var(--color-brand-heading)',
          }}
        />
        <button className="btn btn--primary" onClick={handleImportSub} disabled={isLoading}>
          Import
        </button>
      </div>

      {/* Manual import & QR Upload */}
      <div style={{ display: 'flex', flexDirection: 'column', gap: 12, marginBottom: 20 }}>
        <div style={{ display: 'flex', gap: 10 }}>
          <input
            type="text"
            placeholder="Manual Config URI (vmess://, vless://, trojan://, ss://)"
            value={manualUri}
            onChange={(e) => setManualUri(e.target.value)}
            style={{
              flex: 1,
              padding: '8px 12px',
              borderRadius: 8,
              border: '1px solid var(--color-brand-border)',
              background: 'var(--color-brand-card)',
              fontSize: 13,
              color: 'var(--color-brand-heading)',
            }}
          />
          <button className="btn btn--secondary" onClick={handleManualImport} disabled={isLoading}>
            Import URI
          </button>
          <button
            className="btn"
            type="button"
            onClick={openClipboardModal}
            style={{ background: 'var(--color-brand)', color: '#fff', border: 'none', display: 'flex', alignItems: 'center' }}
          >
            Clipboard Import
          </button>
          <button
            className="btn"
            type="button"
            onClick={handleDeleteAllNodes}
            disabled={isLoading || profiles.length === 0}
            style={{ background: '#dc3545', color: '#fff', border: 'none', display: 'flex', alignItems: 'center' }}
          >
            Delete All Nodes
          </button>
        </div>
        <div
          style={{
            display: 'flex',
            alignItems: 'center',
            gap: 10,
            background: 'var(--color-brand-bg)',
            padding: '10px 12px',
            borderRadius: 8,
            border: '1px solid var(--color-brand-border)',
          }}
        >
          <span style={{ fontSize: 12, fontWeight: 600, color: 'var(--color-brand-heading)' }}>
            Import Config via QR Code Image:
          </span>
          <input
            type="file"
            accept="image/*"
            ref={qrFileInputRef}
            onChange={handleQRImport}
            style={{ fontSize: 12, color: 'var(--color-brand-text)' }}
            disabled={isLoading}
          />
        </div>
      </div>

      {/* Table of Profiles */}
      <div ref={parentRef} style={{ maxHeight: 600, overflow: 'auto', border: '1px solid var(--color-brand-border)', borderRadius: 8 }}>
        <table style={{ width: '100%', borderCollapse: 'collapse', fontSize: 12, textAlign: 'left' }}>
          <thead style={{ position: 'sticky', top: 0, zIndex: 1, background: 'var(--color-brand-bg)' }}>
            <tr style={{ borderBottom: '1px solid var(--color-brand-border)' }}>
              <th style={{ padding: '10px 12px', color: 'var(--color-brand-heading)', width: 50 }}>Active</th>
              <th style={{ padding: '10px 12px', color: 'var(--color-brand-heading)', width: 50 }}>Select</th>
              <th style={{ padding: '10px 12px', color: 'var(--color-brand-heading)' }}>Name</th>
              <th style={{ padding: '10px 12px', color: 'var(--color-brand-heading)' }}>Protocol</th>
              <th style={{ padding: '10px 12px', color: 'var(--color-brand-heading)' }}>Address</th>
              <th style={{ padding: '10px 12px', color: 'var(--color-brand-heading)' }}>Ping</th>
              <th style={{ padding: '10px 12px', color: 'var(--color-brand-heading)', textAlign: 'center' }}>Actions</th>
            </tr>
          </thead>
          <tbody>
            {profiles.length === 0 ? (
              <tr>
                <td colSpan={7} style={{ padding: 20, textAlign: 'center', color: 'var(--color-brand-muted)' }}>
                  No profiles imported. Add subscription URL or paste configs.
                </td>
              </tr>
            ) : (
              <>
                {rowVirtualizer.getVirtualItems()[0]?.start > 0 && (
                  <tr>
                    <td colSpan={7} style={{ height: rowVirtualizer.getVirtualItems()[0].start }} />
                  </tr>
                )}
                {rowVirtualizer.getVirtualItems().map((virtualRow) => {
                  const p = profiles[virtualRow.index];
                  if (!p) return null;
                  return (
                    <tr
                      key={virtualRow.key}
                      style={{
                        height: virtualRow.size,
                        borderBottom: '1px solid var(--color-brand-border)',
                        background: p.ID === activeProfileId ? 'var(--color-brand-light)' : 'none',
                      }}
                    >
                      <td style={{ padding: '10px 12px' }}>
                        <input
                          type="radio"
                          name="active_profile"
                          checked={p.ID === activeProfileId}
                          onChange={() => handleSelectProfile(p.ID)}
                          style={{ cursor: 'pointer', accentColor: 'var(--color-brand)' }}
                        />
                      </td>
                      <td style={{ padding: '10px 12px' }}>
                        <input
                          type="checkbox"
                          checked={selectedProfileIds.includes(p.ID)}
                          onChange={(e) => {
                            if (e.target.checked) {
                              setSelectedProfileIds([...selectedProfileIds, p.ID]);
                            } else {
                              setSelectedProfileIds(selectedProfileIds.filter((id) => id !== p.ID));
                            }
                          }}
                          style={{ cursor: 'pointer', accentColor: 'var(--color-brand)' }}
                        />
                      </td>
                      <td style={{ padding: '10px 12px', fontWeight: 600, color: 'var(--color-brand-heading)' }}>
                        {p.name}
                      </td>
                      <td style={{ padding: '10px 12px', textTransform: 'uppercase' }}>
                        <span
                          style={{
                            padding: '2px 6px',
                            borderRadius: 4,
                            background: '#e0f2fe',
                            color: '#0369a1',
                            fontSize: 10,
                            fontWeight: 700,
                          }}
                        >
                          {p.protocol}
                        </span>
                      </td>
                      <td style={{ padding: '10px 12px', color: 'var(--color-brand-text)' }}>
                        {p.address}:{p.port}
                      </td>
                      <td style={{ padding: '10px 12px' }}>
                        <span
                          style={{
                            fontWeight: 700,
                            color: getLatencyColor(p.latency_ms),
                          }}
                        >
                          {p.latency_ms > 0 ? `${p.latency_ms}ms` : 'N/A'}
                        </span>
                      </td>
                      <td style={{ padding: '10px 12px', textAlign: 'center' }}>
                        <button
                          onClick={() => handleDeleteProfile(p.ID)}
                          style={{ background: 'none', border: 'none', cursor: 'pointer', color: 'var(--color-brand-red)' }}
                        >
                          <FiTrash2 size={14} />
                        </button>
                      </td>
                    </tr>
                  );
                })}
                {rowVirtualizer.getVirtualItems().length > 0 && (
                  <tr>
                    <td
                      colSpan={7}
                      style={{
                        height:
                          rowVirtualizer.getTotalSize() -
                          rowVirtualizer.getVirtualItems()[rowVirtualizer.getVirtualItems().length - 1].end,
                      }}
                    />
                  </tr>
                )}
                {isLoading && profiles.length < totalProfiles && (
                  <tr>
                    <td colSpan={7} style={{ padding: 10, textAlign: 'center', color: 'var(--color-brand-muted)' }}>
                      Loading more...
                    </td>
                  </tr>
                )}
              </>
            )}
          </tbody>
        </table>
      </div>
    </div>
  );
};

export default SubscriptionsCard;
