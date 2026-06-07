import React from 'react';

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
        <p style={{ margin: 0, fontSize: 13, color: 'var(--color-brand-text)', lineHeight: 1.5 }}>
          {text}
        </p>
      </div>
    </div>
  );
};

export default HelpModal;
