import React, { useState, useEffect, useRef } from 'react';
import { useDialogStore } from '../../store/dialogStore';
import { FiCheckCircle, FiXCircle, FiAlertTriangle, FiInfo, FiHelpCircle } from 'react-icons/fi';

export const GlobalDialog: React.FC = () => {
  const { currentDialog, close } = useDialogStore();
  const [inputValue, setInputValue] = useState('');
  const inputRef = useRef<HTMLInputElement>(null);
  const confirmButtonRef = useRef<HTMLButtonElement>(null);

  useEffect(() => {
    if (currentDialog) {
      setInputValue(currentDialog.defaultValue || '');
      // Auto focus logic after the modal transitions in
      const timer = setTimeout(() => {
        if (currentDialog.type === 'prompt') {
          inputRef.current?.focus();
          inputRef.current?.select();
        } else {
          confirmButtonRef.current?.focus();
        }
      }, 50);
      return () => clearTimeout(timer);
    }
  }, [currentDialog]);

  useEffect(() => {
    if (!currentDialog) return;

    const handleKeyDown = (e: KeyboardEvent) => {
      if (e.key === 'Escape') {
        e.preventDefault();
        if (currentDialog.type === 'confirm') {
          close(false);
        } else if (currentDialog.type === 'prompt') {
          close(null);
        } else {
          close(undefined);
        }
      }
    };

    window.addEventListener('keydown', handleKeyDown);
    return () => window.removeEventListener('keydown', handleKeyDown);
  }, [currentDialog, close]);

  if (!currentDialog) return null;

  const { type, variant, title, message, placeholder } = currentDialog;

  const handleSubmit = (e: React.FormEvent) => {
    e.preventDefault();
    if (type === 'prompt') {
      close(inputValue);
    } else if (type === 'confirm') {
      close(true);
    } else {
      close(undefined);
    }
  };

  const getIcon = () => {
    switch (variant) {
      case 'success':
        return <FiCheckCircle />;
      case 'error':
        return <FiXCircle />;
      case 'warning':
        return <FiAlertTriangle />;
      case 'info':
        return <FiInfo />;
      case 'question':
      default:
        return <FiHelpCircle />;
    }
  };

  const getConfirmButtonClass = () => {
    if (variant === 'error' || variant === 'warning') {
      return 'g-dialog-btn g-dialog-btn--confirm g-dialog-btn--confirm--danger';
    }
    if (variant === 'success') {
      return 'g-dialog-btn g-dialog-btn--confirm g-dialog-btn--confirm--success';
    }
    return 'g-dialog-btn g-dialog-btn--confirm';
  };

  return (
    <div className="g-dialog-overlay" onClick={() => {
      // Allow dismissing alerts by clicking backdrop, but not prompts or confirms
      if (type === 'alert') {
        close(undefined);
      }
    }}>
      <div className="g-dialog-box" onClick={(e) => e.stopPropagation()}>
        <div className="g-dialog-header">
          <div className={`g-dialog-icon-wrap g-dialog-icon-wrap--${variant}`}>
            {getIcon()}
          </div>
          <div className="g-dialog-title-area">
            <h3 className="g-dialog-title">{title}</h3>
          </div>
        </div>

        <div className="g-dialog-message-container">
          <p className="g-dialog-message">{message}</p>
        </div>

        <form onSubmit={handleSubmit} style={{ display: 'flex', flexDirection: 'column', gap: '20px', margin: 0 }}>
          {type === 'prompt' && (
            <input
              ref={inputRef}
              type="text"
              className="g-dialog-input"
              value={inputValue}
              onChange={(e) => setInputValue(e.target.value)}
              placeholder={placeholder}
            />
          )}

          <div className="g-dialog-actions">
            {(type === 'confirm' || type === 'prompt') && (
              <button
                type="button"
                className="g-dialog-btn g-dialog-btn--cancel"
                onClick={() => {
                  if (type === 'confirm') close(false);
                  else close(null);
                }}
              >
                Cancel
              </button>
            )}
            <button
              ref={confirmButtonRef}
              type="submit"
              className={getConfirmButtonClass()}
            >
              {type === 'confirm' ? 'Confirm' : 'OK'}
            </button>
          </div>
        </form>
      </div>
    </div>
  );
};
