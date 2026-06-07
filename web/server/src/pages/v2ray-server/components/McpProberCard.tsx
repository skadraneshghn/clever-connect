import React from 'react';
import { FiZap } from 'react-icons/fi';

interface McpProberCardProps {
  isLoading: boolean;
  mcpMethod: string;
  setMcpMethod: (method: string) => void;
  mcpResult: any;
  handleTestMCP: () => void;
}

export const McpProberCard: React.FC<McpProberCardProps> = ({
  isLoading,
  mcpMethod,
  setMcpMethod,
  mcpResult,
  handleTestMCP,
}) => {
  return (
    <div className="g-card">
      <div style={{ display: 'flex', alignItems: 'center', gap: 6, marginBottom: 14 }}>
        <FiZap style={{ color: 'var(--color-brand)', fontSize: 16 }} />
        <span style={{ fontSize: 13, fontWeight: 600, color: 'var(--color-brand-heading)' }}>
          MCP RPC diagnostic prober
        </span>
      </div>

      <div style={{ display: 'flex', gap: 10, marginBottom: 12 }}>
        <select
          value={mcpMethod}
          onChange={(e) => setMcpMethod(e.target.value)}
          style={{
            flex: 1,
            padding: '8px 10px',
            borderRadius: 6,
            border: '1px solid var(--color-brand-border)',
            background: 'var(--color-brand-card)',
            fontSize: 12,
            color: 'var(--color-brand-heading)',
          }}
        >
          <option value="system.status">system.status</option>
          <option value="node.list">node.list</option>
          <option value="user.audit">user.audit</option>
        </select>
        <button
          className="btn btn--secondary btn--sm"
          onClick={handleTestMCP}
          disabled={isLoading}
        >
          Invoke
        </button>
      </div>

      {mcpResult && (
        <pre
          style={{
            margin: 0,
            background: '#1a1a2e',
            color: '#a9b1d6',
            borderRadius: 6,
            padding: 10,
            fontSize: 10,
            fontFamily: 'Fira Code',
            overflowX: 'auto',
          }}
        >
          {JSON.stringify(mcpResult.result, null, 2)}
        </pre>
      )}
    </div>
  );
};

export default McpProberCard;
