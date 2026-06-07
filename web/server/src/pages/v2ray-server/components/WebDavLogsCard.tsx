import React from 'react';
import { FiTerminal, FiShare2 } from 'react-icons/fi';

export const WebDavLogsCard: React.FC = () => {
  return (
    <div className="g-card">
      <div style={{ display: 'flex', alignItems: 'center', gap: 6, marginBottom: 12 }}>
        <FiTerminal style={{ color: 'var(--color-brand)', fontSize: 16 }} />
        <span style={{ fontSize: 13, fontWeight: 600, color: 'var(--color-brand-heading)' }}>
          WebDAV log access
        </span>
      </div>
      <p style={{ fontSize: 12, color: 'var(--color-brand-text)', lineHeight: 1.4, margin: '0 0 12px' }}>
        Mount or browse the server's WebDAV logs storage block directly using your local file explorer.
      </p>
      <a
        href="/api/v2ray/webdav/"
        target="_blank"
        rel="noopener noreferrer"
        className="btn btn--sm btn--secondary"
        style={{ display: 'inline-flex', alignItems: 'center', gap: 6 }}
      >
        <FiShare2 /> Browse Logs Folder
      </a>
    </div>
  );
};

export default WebDavLogsCard;
