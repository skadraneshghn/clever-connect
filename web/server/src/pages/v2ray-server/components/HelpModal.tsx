import React from 'react';
import { FiX } from 'react-icons/fi';

interface HelpModalProps {
  title: string | null;
  text: string | null;
  onClose: () => void;
}

export const HelpModal: React.FC<HelpModalProps> = ({ title, text, onClose }) => {
  if (!title) return null;

  return (
    <div
      style={{
        position: 'fixed',
        top: 0,
        left: 0,
        width: '100%',
        height: '100%',
        background: 'rgba(0,0,0,0.5)',
        display: 'flex',
        alignItems: 'center',
        justifyContent: 'center',
        zIndex: 9999,
      }}
      onClick={onClose}
    >
      <div
        style={{
          background: 'var(--color-brand-card)',
          padding: 24,
          borderRadius: 12,
          width: 440,
          maxWidth: '90%',
          boxShadow: '0 10px 25px rgba(0,0,0,0.1)',
        }}
        onClick={(e) => e.stopPropagation()}
      >
        <div
          style={{
            display: 'flex',
            justifyContent: 'space-between',
            alignItems: 'center',
            marginBottom: 14,
            borderBottom: '1px solid var(--color-brand-border)',
            paddingBottom: 10,
          }}
        >
          <h3 style={{ margin: 0, fontSize: 16, fontWeight: 700, color: 'var(--color-brand-heading)' }}>
            {title}
          </h3>
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
          >
            <FiX size={18} />
          </button>
        </div>
        <p style={{ margin: 0, fontSize: 13, color: 'var(--color-brand-text)', lineHeight: 1.5 }}>
          {text}
        </p>
      </div>
    </div>
  );
};

export default HelpModal;
