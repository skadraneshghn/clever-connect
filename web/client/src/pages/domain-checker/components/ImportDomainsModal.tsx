import React from 'react';
import { FiUploadCloud } from 'react-icons/fi';

interface ImportDomainsModalProps {
  isOpen: boolean;
  onClose: () => void;
  categories: string[];
  importCategory: string;
  setImportCategory: (cat: string) => void;
  isCreatingNewCatInImport: boolean;
  setIsCreatingNewCatInImport: (val: boolean) => void;
  customImportCat: string;
  setCustomImportCat: (cat: string) => void;
  addMethod: 'text' | 'file';
  setAddMethod: (m: 'text' | 'file') => void;
  rawTextImport: string;
  setRawTextImport: (t: string) => void;
  fileDomains: string[];
  fileParsedCount: number;
  fileInputRef: React.RefObject<HTMLInputElement | null>;
  handleFileChange: (e: React.ChangeEvent<HTMLInputElement>) => void;
  handleImportSubmit: () => void;
  isImporting: boolean;
}

export const ImportDomainsModal: React.FC<ImportDomainsModalProps> = ({
  isOpen,
  onClose,
  categories,
  importCategory,
  setImportCategory,
  isCreatingNewCatInImport,
  setIsCreatingNewCatInImport,
  customImportCat,
  setCustomImportCat,
  addMethod,
  setAddMethod,
  rawTextImport,
  setRawTextImport,
  fileDomains,
  fileParsedCount,
  fileInputRef,
  handleFileChange,
  handleImportSubmit,
  isImporting,
}) => {
  if (!isOpen) return null;

  return (
    <div
      style={{
        position: 'fixed',
        top: 0, left: 0, width: '100%', height: '100%',
        background: 'rgba(0,0,0,0.5)',
        display: 'flex', alignItems: 'center', justifyContent: 'center',
        zIndex: 999,
      }}
      onClick={onClose}
    >
      <div
        style={{
          background: 'var(--color-brand-card)',
          padding: 24,
          borderRadius: 12,
          width: 500,
          maxWidth: '90%',
          boxShadow: '0 10px 25px rgba(0,0,0,0.15)',
          display: 'flex',
          flexDirection: 'column',
          gap: 16
        }}
        onClick={(e) => e.stopPropagation()}
      >
        <h3 style={{ fontSize: 16, fontWeight: 700, color: 'var(--color-brand-heading)', margin: 0 }}>
          Bulk Domain Importer
        </h3>

        {isImporting ? (
          <div style={{
            display: 'flex',
            flexDirection: 'column',
            alignItems: 'center',
            justifyContent: 'center',
            padding: '40px 20px',
            gap: 14,
            background: 'var(--color-brand-bg)',
            border: '1px solid var(--color-brand-border)',
            borderRadius: 8,
            margin: '10px 0'
          }}>
            <div style={{
              width: 32,
              height: 32,
              border: '3px solid var(--color-brand-border)',
              borderTop: '3px solid var(--color-brand)',
              borderRadius: '50%',
              animation: 'spin 1s linear infinite',
            }} />
            <style>{`
              @keyframes spin {
                0% { transform: rotate(0deg); }
                100% { transform: rotate(360deg); }
              }
            `}</style>
            <div style={{ display: 'flex', flexDirection: 'column', alignItems: 'center', gap: 4 }}>
              <span style={{ fontSize: 13, color: 'var(--color-brand-heading)', fontWeight: 600 }}>
                Importing {addMethod === 'text' ? rawTextImport.split('\n').map(s => s.trim()).filter(Boolean).length : fileDomains.length} domains...
              </span>
              <span style={{ fontSize: 11, color: 'var(--color-brand-muted)' }}>
                Writing to Pebble database & updating categories
              </span>
            </div>
          </div>
        ) : (
          <>
            {/* Category Select / Creation */}
            <div style={{ display: 'flex', flexDirection: 'column', gap: 6 }}>
              <label style={{ fontSize: 11, fontWeight: 600, color: 'var(--color-brand-text)' }}>Target Category</label>

              {!isCreatingNewCatInImport ? (
                <div style={{ display: 'flex', gap: 10 }}>
                  <select
                    value={importCategory}
                    onChange={(e) => setImportCategory(e.target.value)}
                    style={{
                      flex: 1,
                      padding: '8px 10px',
                      borderRadius: 8,
                      border: '1px solid var(--color-brand-border)',
                      background: 'var(--color-brand-bg)',
                      fontSize: 13,
                      color: 'var(--color-brand-heading)'
                    }}
                  >
                    {categories.map(cat => (
                      <option key={cat} value={cat}>{cat}</option>
                    ))}
                  </select>
                  <button
                    type="button"
                    className="btn btn--secondary"
                    onClick={() => setIsCreatingNewCatInImport(true)}
                  >
                    New
                  </button>
                </div>
              ) : (
                <div style={{ display: 'flex', gap: 10 }}>
                  <input
                    type="text"
                    placeholder="Enter category name..."
                    value={customImportCat}
                    onChange={(e) => setCustomImportCat(e.target.value)}
                    style={{
                      flex: 1,
                      padding: '8px 10px',
                      borderRadius: 8,
                      border: '1px solid var(--color-brand-border)',
                      background: 'var(--color-brand-bg)',
                      fontSize: 13,
                      color: 'var(--color-brand-heading)'
                    }}
                  />
                  <button
                    type="button"
                    className="btn btn--secondary"
                    onClick={() => setIsCreatingNewCatInImport(false)}
                  >
                    Select Existing
                  </button>
                </div>
              )}
            </div>

            {/* Selector: Text or File */}
            <div style={{ display: 'flex', borderBottom: '1px solid var(--color-brand-border)' }}>
              <button
                onClick={() => setAddMethod('text')}
                style={{
                  flex: 1, padding: '8px 0', border: 'none', background: 'none',
                  fontSize: 12, fontWeight: 600, cursor: 'pointer',
                  borderBottom: addMethod === 'text' ? '2px solid var(--color-brand)' : 'none',
                  color: addMethod === 'text' ? 'var(--color-brand)' : 'var(--color-brand-text)'
                }}
              >
                Raw Text List
              </button>
              <button
                onClick={() => setAddMethod('file')}
                style={{
                  flex: 1, padding: '8px 0', border: 'none', background: 'none',
                  fontSize: 12, fontWeight: 600, cursor: 'pointer',
                  borderBottom: addMethod === 'file' ? '2px solid var(--color-brand)' : 'none',
                  color: addMethod === 'file' ? 'var(--color-brand)' : 'var(--color-brand-text)'
                }}
              >
                Upload TXT / CSV File
              </button>
            </div>

            {/* Input area */}
            {addMethod === 'text' ? (
              <textarea
                value={rawTextImport}
                onChange={(e) => setRawTextImport(e.target.value)}
                placeholder="Paste domains (one per line, e.g. google.com)..."
                style={{
                  width: '100%', height: 180, padding: 12, borderRadius: 8,
                  border: '1px solid var(--color-brand-border)',
                  background: 'var(--color-brand-bg)',
                  fontSize: 13, color: 'var(--color-brand-heading)',
                  resize: 'none', outline: 'none'
                }}
              />
            ) : (
              <div
                onClick={() => fileInputRef.current?.click()}
                style={{
                  height: 180, border: '2px dashed var(--color-brand-border)',
                  borderRadius: 8, display: 'flex', flexDirection: 'column',
                  alignItems: 'center', justifyContent: 'center', gap: 10,
                  cursor: 'pointer', background: 'var(--color-brand-bg)'
                }}
              >
                <FiUploadCloud size={32} style={{ color: 'var(--color-brand)' }} />
                <span style={{ fontSize: 12, color: 'var(--color-brand-text)', fontWeight: 500 }}>
                  Click to select TXT or CSV domain list file
                </span>
                {fileParsedCount > 0 && (
                  <span style={{ fontSize: 11, color: 'var(--color-brand-green)', fontWeight: 700 }}>
                    Successfully parsed {fileParsedCount} domains!
                  </span>
                )}
                <input
                  type="file"
                  accept=".txt,.csv"
                  ref={fileInputRef}
                  onChange={handleFileChange}
                  style={{ display: 'none' }}
                />
              </div>
            )}
          </>
        )}

        {/* Actions */}
        <div style={{ display: 'flex', justifyContent: 'flex-end', gap: 10 }}>
          <button
            type="button"
            className="btn btn--secondary btn--sm"
            onClick={onClose}
            disabled={isImporting}
            style={{ opacity: isImporting ? 0.6 : 1, cursor: isImporting ? 'not-allowed' : 'pointer' }}
          >
            Cancel
          </button>
          <button
            type="button"
            className="btn btn--primary btn--sm"
            onClick={handleImportSubmit}
            disabled={isImporting}
            style={{ opacity: isImporting ? 0.6 : 1, cursor: isImporting ? 'not-allowed' : 'pointer' }}
          >
            {isImporting ? 'Importing...' : 'Import Domains'}
          </button>
        </div>
      </div>
    </div>
  );
};
