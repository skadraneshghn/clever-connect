import React from 'react';
import { FiSettings } from 'react-icons/fi';

interface WebhookNotifierCardProps {
  webhookURL: string;
  setWebhookURL: (url: string) => void;
  webhookSecret: string;
  setWebhookSecret: (secret: string) => void;
  handleSaveWebhook: () => void;
}

export const WebhookNotifierCard: React.FC<WebhookNotifierCardProps> = ({
  webhookURL,
  setWebhookURL,
  webhookSecret,
  setWebhookSecret,
  handleSaveWebhook,
}) => {
  return (
    <div className="g-card">
      <div style={{ display: 'flex', alignItems: 'center', gap: 6, marginBottom: 14 }}>
        <FiSettings style={{ color: 'var(--color-brand)', fontSize: 16 }} />
        <span style={{ fontSize: 13, fontWeight: 600, color: 'var(--color-brand-heading)' }}>
          Realtime Webhook notifier
        </span>
      </div>

      <div style={{ display: 'flex', flexDirection: 'column', gap: 10 }}>
        <input
          type="text"
          placeholder="Webhook URL (HMAC-SHA256 signed)"
          value={webhookURL}
          onChange={(e) => setWebhookURL(e.target.value)}
          style={{
            padding: '8px 10px',
            borderRadius: 6,
            border: '1px solid var(--color-brand-border)',
            background: 'var(--color-brand-card)',
            fontSize: 12,
            color: 'var(--color-brand-heading)',
          }}
        />
        <input
          type="password"
          placeholder="HMAC Secret Key"
          value={webhookSecret}
          onChange={(e) => setWebhookSecret(e.target.value)}
          style={{
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
          onClick={handleSaveWebhook}
        >
          Save webhook config
        </button>
      </div>
    </div>
  );
};

export default WebhookNotifierCard;
