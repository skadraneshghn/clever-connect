import React from 'react';
import { FiX, FiRefreshCw, FiPlus } from 'react-icons/fi';

interface ClipboardModalProps {
  isOpen: boolean;
  onClose: () => void;
  isImportingBulk: boolean;
  isParsing: boolean;
  parseProgress: number;
  clipboardCount: number;
  clipboardPage: number;
  clipboardSearch: string;
  setClipboardPage: (page: number) => void;
  setClipboardSearch: (search: string) => void;
  processPastedTextChunked: (text: string) => void;
  handleImportBulk: () => void;
  parsedConfigsRef: React.MutableRefObject<any[]>;
  deselectedSetRef: React.MutableRefObject<Set<number>>;
  clipboardUpdateTrigger: number;
  setClipboardUpdateTrigger: React.Dispatch<React.SetStateAction<number>>;
  setClipboardCount: (count: number) => void;
}

export const ClipboardModal: React.FC<ClipboardModalProps> = ({
  isOpen,
  onClose,
  isImportingBulk,
  isParsing,
  parseProgress,
  clipboardCount,
  clipboardPage,
  clipboardSearch,
  setClipboardPage,
  setClipboardSearch,
  processPastedTextChunked,
  handleImportBulk,
  parsedConfigsRef,
  deselectedSetRef,
  clipboardUpdateTrigger,
  setClipboardUpdateTrigger,
  setClipboardCount,
}) => {
  if (!isOpen) return null;

  const filtered = parsedConfigsRef.current.filter((c) => {
    if (!clipboardSearch) return true;
    const query = clipboardSearch.toLowerCase();
    return (
      c.name.toLowerCase().includes(query) ||
      c.host.toLowerCase().includes(query) ||
      c.protocol.includes(query)
    );
  });

  const PAGE_SIZE = 100;
  const slice = filtered.slice(clipboardPage * PAGE_SIZE, (clipboardPage + 1) * PAGE_SIZE);

  return (
    <div
      style={{
        position: 'fixed',
        top: 0,
        left: 0,
        width: '100%',
        height: '100%',
        background: 'rgba(0,0,0,0.6)',
        display: 'flex',
        alignItems: 'center',
        justifyContent: 'center',
        zIndex: 9999,
      }}
    >
      <div
        style={{
          background: 'var(--color-brand-card)',
          padding: 24,
          borderRadius: 16,
          width: 900,
          maxWidth: '95%',
          maxHeight: '90vh',
          boxShadow: '0 20px 40px rgba(0,0,0,0.25)',
          display: 'flex',
          flexDirection: 'column',
          gap: 16,
          overflow: 'hidden',
        }}
      >
        {/* Header */}
        <div
          style={{
            display: 'flex',
            justifyContent: 'space-between',
            alignItems: 'center',
            borderBottom: '1px solid var(--color-brand-border)',
            paddingBottom: 12,
          }}
        >
          <div>
            <h3 style={{ margin: 0, fontSize: 16, fontWeight: 700, color: 'var(--color-brand-heading)' }}>
              Clipboard Mass Config Importer
            </h3>
            <span style={{ fontSize: 11, color: 'var(--color-brand-text)' }}>
              Highly optimized node parser designed for extreme config list scale.
            </span>
          </div>
          <button
            onClick={onClose}
            style={{
              background: 'none',
              border: 'none',
              cursor: 'pointer',
              color: 'var(--color-brand-muted)',
              display: 'flex',
              alignItems: 'center',
            }}
            disabled={isImportingBulk}
          >
            <FiX size={20} />
          </button>
        </div>

        {/* Paste Drop Zone / Progress Bar / List view */}
        {isParsing ? (
          <div
            style={{
              padding: '60px 20px',
              textAlign: 'center',
              display: 'flex',
              flexDirection: 'column',
              alignItems: 'center',
              gap: 12,
            }}
          >
            <FiRefreshCw className="spin-animation" size={36} style={{ color: 'var(--color-brand)' }} />
            <div style={{ fontSize: 14, fontWeight: 600, color: 'var(--color-brand-heading)' }}>
              Parsing Payload... {parseProgress}%
            </div>
            <div
              style={{
                width: '80%',
                maxWidth: 400,
                background: 'var(--color-brand-border)',
                height: 8,
                borderRadius: 4,
                overflow: 'hidden',
              }}
            >
              <div
                style={{
                  width: `${parseProgress}%`,
                  background: 'var(--color-brand)',
                  height: '100%',
                  transition: 'width 0.1s linear',
                }}
              />
            </div>
          </div>
        ) : clipboardCount === 0 ? (
          <div
            style={{
              border: '2px dashed var(--color-brand-border)',
              borderRadius: 12,
              padding: 60,
              textAlign: 'center',
              background: 'var(--color-brand-bg)',
              position: 'relative',
            }}
          >
            <FiPlus size={40} style={{ color: 'var(--color-brand)', marginBottom: 12 }} />
            <p style={{ margin: 0, fontSize: 14, fontWeight: 600, color: 'var(--color-brand-heading)' }}>
              Click inside this box and press Ctrl + V to paste configs
            </p>
            <p style={{ margin: '4px 0 0', fontSize: 11, color: 'var(--color-brand-text)' }}>
              Accepts base64 subscription blobs, multi-line URIs, or JSON configuration lists.
            </p>
            <textarea
              autoFocus
              value=""
              onChange={() => {}}
              onPaste={(e) => {
                const text = e.clipboardData.getData('text');
                processPastedTextChunked(text);
              }}
              style={{
                position: 'absolute',
                top: 0,
                left: 0,
                width: '100%',
                height: '100%',
                opacity: 0,
                cursor: 'pointer',
              }}
            />
          </div>
        ) : (
          // Parsed view
          <div style={{ display: 'flex', flexDirection: 'column', gap: 12, overflow: 'hidden', flex: 1 }}>
            {/* Search & Bulk selection tools */}
            <div style={{ display: 'flex', gap: 12, alignItems: 'center' }}>
              <input
                type="text"
                placeholder="Search parsed configs..."
                value={clipboardSearch}
                onChange={(e) => {
                  setClipboardSearch(e.target.value);
                  setClipboardPage(0);
                }}
                style={{
                  flex: 1,
                  padding: '8px 12px',
                  borderRadius: 8,
                  border: '1px solid var(--color-brand-border)',
                  background: 'var(--color-brand-card)',
                  fontSize: 12,
                  color: 'var(--color-brand-heading)',
                }}
              />
              <div style={{ display: 'flex', gap: 6 }}>
                <button
                  type="button"
                  className="btn btn--xs btn--secondary"
                  onClick={() => {
                    filtered.forEach((c) => {
                      const idx = parsedConfigsRef.current.indexOf(c);
                      if (idx !== -1) deselectedSetRef.current.delete(idx);
                    });
                    setClipboardUpdateTrigger((prev) => prev + 1);
                  }}
                >
                  Select Search Results
                </button>
                <button
                  type="button"
                  className="btn btn--xs btn--secondary"
                  onClick={() => {
                    filtered.forEach((c) => {
                      const idx = parsedConfigsRef.current.indexOf(c);
                      if (idx !== -1) deselectedSetRef.current.add(idx);
                    });
                    setClipboardUpdateTrigger((prev) => prev + 1);
                  }}
                >
                  Deselect Search Results
                </button>
                <button
                  type="button"
                  className="btn btn--xs btn--secondary"
                  onClick={() => {
                    deselectedSetRef.current.clear();
                    setClipboardUpdateTrigger((prev) => prev + 1);
                  }}
                >
                  Select All
                </button>
                <button
                  type="button"
                  className="btn btn--xs btn--secondary"
                  onClick={() => {
                    parsedConfigsRef.current.forEach((_, idx) => deselectedSetRef.current.add(idx));
                    setClipboardUpdateTrigger((prev) => prev + 1);
                  }}
                >
                  Deselect All
                </button>
              </div>
            </div>

            {/* Status bar */}
            <div style={{ fontSize: 11, color: 'var(--color-brand-text)', display: 'flex', justifyContent: 'space-between' }}>
              <span>
                Total Parsed: <strong>{clipboardCount}</strong> configs
              </span>
              <span>
                Selected: <strong>{clipboardCount - deselectedSetRef.current.size}</strong> / {clipboardCount}
              </span>
            </div>

            {/* Config Table with Virtual Viewport slicing */}
            <div style={{ flex: 1, overflowY: 'auto', border: '1px solid var(--color-brand-border)', borderRadius: 8 }}>
              <table style={{ width: '100%', borderCollapse: 'collapse', fontSize: 11, textAlign: 'left' }}>
                <thead>
                  <tr
                    style={{
                      background: 'var(--color-brand-bg)',
                      borderBottom: '1px solid var(--color-brand-border)',
                      position: 'sticky',
                      top: 0,
                      zIndex: 10,
                    }}
                  >
                    <th style={{ padding: '8px 12px', color: 'var(--color-brand-heading)', width: 40 }}>Sel</th>
                    <th style={{ padding: '8px 12px', color: 'var(--color-brand-heading)', width: 180 }}>Name</th>
                    <th style={{ padding: '8px 12px', color: 'var(--color-brand-heading)', width: 70 }}>Protocol</th>
                    <th style={{ padding: '8px 12px', color: 'var(--color-brand-heading)' }}>Address</th>
                    <th style={{ padding: '8px 12px', color: 'var(--color-brand-heading)', width: 60 }}>Port</th>
                  </tr>
                </thead>
                <tbody>
                  {slice.length === 0 ? (
                    <tr>
                      <td colSpan={5} style={{ padding: 20, textAlign: 'center', color: 'var(--color-brand-muted)' }}>
                        No configurations match search filters.
                      </td>
                    </tr>
                  ) : (
                    slice.map((c) => {
                      const origIdx = parsedConfigsRef.current.indexOf(c);
                      const isSelected = !deselectedSetRef.current.has(origIdx);

                      return (
                        <tr
                          key={origIdx}
                          style={{
                            borderBottom: '1px solid var(--color-brand-border)',
                            background: isSelected ? 'var(--color-brand-light)' : 'none',
                          }}
                        >
                          <td style={{ padding: '8px 12px' }}>
                            <input
                              type="checkbox"
                              checked={isSelected}
                              onChange={() => {
                                if (deselectedSetRef.current.has(origIdx)) {
                                  deselectedSetRef.current.delete(origIdx);
                                } else {
                                  deselectedSetRef.current.add(origIdx);
                                }
                                setClipboardUpdateTrigger((prev) => prev + 1);
                              }}
                              style={{ cursor: 'pointer', accentColor: 'var(--color-brand)' }}
                            />
                          </td>
                          <td
                            style={{
                              padding: '8px 12px',
                              fontWeight: 600,
                              color: 'var(--color-brand-heading)',
                              whiteSpace: 'nowrap',
                              overflow: 'hidden',
                              textOverflow: 'ellipsis',
                              maxWidth: 180,
                            }}
                          >
                            {c.name}
                          </td>
                          <td style={{ padding: '8px 12px', textTransform: 'uppercase', color: 'var(--color-brand)' }}>
                            {c.protocol}
                          </td>
                          <td style={{ padding: '8px 12px', fontFamily: 'monospace' }}>{c.host}</td>
                          <td style={{ padding: '8px 12px' }}>{c.port}</td>
                        </tr>
                      );
                    })
                  )}
                </tbody>
              </table>
            </div>

            {/* Footer Controls */}
            <div
              style={{
                display: 'flex',
                justifyContent: 'space-between',
                alignItems: 'center',
                borderTop: '1px solid var(--color-brand-border)',
                paddingBottom: 4,
                paddingTop: 12,
              }}
            >
              {/* Pagination */}
              <div style={{ display: 'flex', gap: 6, alignItems: 'center' }}>
                <button
                  type="button"
                  className="btn btn--xs btn--secondary"
                  disabled={clipboardPage === 0}
                  onClick={() => setClipboardPage(clipboardPage - 1)}
                >
                  Prev
                </button>
                <span style={{ fontSize: 11, color: 'var(--color-brand-text)' }}>
                  Page <strong>{clipboardPage + 1}</strong> of{' '}
                  <strong>{Math.ceil(filtered.length / 100) || 1}</strong>
                </span>
                <button
                  type="button"
                  className="btn btn--xs btn--secondary"
                  disabled={(clipboardPage + 1) * 100 >= filtered.length}
                  onClick={() => setClipboardPage(clipboardPage + 1)}
                >
                  Next
                </button>
              </div>

              {/* Actions */}
              <div style={{ display: 'flex', gap: 10 }}>
                <button
                  type="button"
                  className="btn btn--sm btn--secondary"
                  onClick={() => {
                    parsedConfigsRef.current = [];
                    deselectedSetRef.current.clear();
                    setClipboardCount(0);
                    setClipboardPage(0);
                  }}
                  disabled={isImportingBulk}
                >
                  Reset / Clear
                </button>
                <button
                  type="button"
                  className="btn btn--sm btn--primary"
                  onClick={handleImportBulk}
                  disabled={isImportingBulk || clipboardCount - deselectedSetRef.current.size === 0}
                  style={{ background: 'var(--color-brand-green)', borderColor: 'var(--color-brand-green)' }}
                >
                  {isImportingBulk ? 'Importing...' : `Import Selected (${clipboardCount - deselectedSetRef.current.size})`}
                </button>
              </div>
            </div>
          </div>
        )}
      </div>
    </div>
  );
};

export default ClipboardModal;
