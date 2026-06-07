import React, { useState, useEffect, lazy, Suspense } from 'react';
import { FiRefreshCw } from 'react-icons/fi';

// Skeleton loaders
import {
  CardSkeleton,
  EdgeNodesSkeleton,
  InboundsSkeleton,
  UsersSkeleton,
} from './v2ray-server/components/Skeletons';

// Lazy-loaded components
const HelpModal = lazy(() => import('./v2ray-server/components/HelpModal'));
const EdgeNodesCard = lazy(() => import('./v2ray-server/components/EdgeNodesCard'));
const InboundEndpointsCard = lazy(() => import('./v2ray-server/components/InboundEndpointsCard'));
const UserQuotasCard = lazy(() => import('./v2ray-server/components/UserQuotasCard'));
const TrafficAuditsCard = lazy(() => import('./v2ray-server/components/TrafficAuditsCard'));
const ActiveCoreControlCard = lazy(() => import('./v2ray-server/components/ActiveCoreControlCard'));
const WebDavLogsCard = lazy(() => import('./v2ray-server/components/WebDavLogsCard'));
const FirewallProtectionCard = lazy(() => import('./v2ray-server/components/FirewallProtectionCard'));
const McpProberCard = lazy(() => import('./v2ray-server/components/McpProberCard'));
const WebhookNotifierCard = lazy(() => import('./v2ray-server/components/WebhookNotifierCard'));

export const V2RayServerPage: React.FC = () => {
  // Help Modal popup state
  const [helpTitle, setHelpTitle] = useState<string | null>(null);
  const [helpText, setHelpText] = useState<string | null>(null);

  const showHelp = (title: string, text: string) => {
    setHelpTitle(title);
    setHelpText(text);
  };

  // State definitions
  const [isRunning, setIsRunning] = useState(false);
  const [isLoading, setIsLoading] = useState(false);
  const [message, setMessage] = useState<{ type: 'success' | 'error'; text: string } | null>(null);

  // Lists
  const [nodes, setNodes] = useState<any[]>([]);
  const [inbounds, setInbounds] = useState<any[]>([]);
  const [users, setUsers] = useState<any[]>([]);
  const [firewallIPs, setFirewallIPs] = useState<string[]>([]);
  const [trafficLogs, setTrafficLogs] = useState<any[]>([]);

  // Forms: Nodes
  const [nodeName, setNodeName] = useState('');
  const [nodeIP, setNodeIP] = useState('');
  const [nodeSSHPort, setNodeSSHPort] = useState(22);
  const [nodeSSHCreds, setNodeSSHCreds] = useState('');
  const [provisionPassword, setProvisionPassword] = useState('');
  const [selectedNodeIdForProvision, setSelectedNodeIdForProvision] = useState<number | null>(null);

  // Forms: Inbounds
  const [inboundTag, setInboundTag] = useState('');
  const [inboundProtocol, setInboundProtocol] = useState('vless');
  const [inboundPort, setInboundPort] = useState(443);
  const [inboundNetwork, setInboundNetwork] = useState('tcp');
  const [inboundTlsMode, setInboundTlsMode] = useState('reality');
  const [inboundSNI, setInboundSNI] = useState('yahoo.com');
  const [inboundFallback, setInboundFallback] = useState('127.0.0.1:80');

  // Forms: Users
  const [userName, setUserName] = useState('');
  const [userUUID, setUserUUID] = useState('');
  const [userInboundId, setUserInboundId] = useState<number | string>('');
  const [userLimitGB, setUserLimitGB] = useState(100);

  // Forms: Firewall & Webhook
  const [blockIP, setBlockIP] = useState('');
  const [webhookURL, setWebhookURL] = useState('');
  const [webhookSecret, setWebhookSecret] = useState('');

  // MCP test client
  const [mcpMethod, setMcpMethod] = useState('system.status');
  const [mcpResult, setMcpResult] = useState<any>(null);

  // Load everything
  const loadData = async () => {
    setIsLoading(true);
    try {
      const token = localStorage.getItem('cc_server_token') || localStorage.getItem('cc_client_token') || '';

      // Nodes
      const nResp = await fetch('/api/v2ray/nodes', {
        headers: { Authorization: `Bearer ${token}` },
      });
      if (nResp.ok) setNodes((await nResp.json()) || []);

      // Inbounds
      const iResp = await fetch('/api/v2ray/inbounds', {
        headers: { Authorization: `Bearer ${token}` },
      });
      if (iResp.ok) {
        const ins = (await iResp.json()) || [];
        setInbounds(ins);
        if (ins.length > 0 && !userInboundId) setUserInboundId(ins[0].ID);
      }

      // Users
      const uResp = await fetch('/api/v2ray/users', {
        headers: { Authorization: `Bearer ${token}` },
      });
      if (uResp.ok) setUsers((await uResp.json()) || []);

      // Traffic logs
      const tResp = await fetch('/api/v2ray/traffic/logs', {
        headers: { Authorization: `Bearer ${token}` },
      });
      if (tResp.ok) setTrafficLogs((await tResp.json()) || []);

      // Server core status
      const sResp = await fetch('/api/v2ray/core/status', {
        headers: { Authorization: `Bearer ${token}` },
      });
      if (sResp.ok) {
        const data = await sResp.json();
        setIsRunning(data.is_running);
      }
    } catch (err) {
      console.error(err);
    } finally {
      setIsLoading(false);
    }
  };

  useEffect(() => {
    loadData();
  }, []);

  // Actions: Core
  const handleToggleCore = async (action: 'start' | 'stop') => {
    setIsLoading(true);
    setMessage(null);
    try {
      const token = localStorage.getItem('cc_server_token') || localStorage.getItem('cc_client_token') || '';
      const res = await fetch(`/api/v2ray/core/${action}`, {
        method: 'POST',
        headers: { Authorization: `Bearer ${token}` },
      });
      if (res.ok) {
        setIsRunning(action === 'start');
        setMessage({
          type: 'success',
          text: `Core process ${action === 'start' ? 'started' : 'stopped'} successfully!`,
        });
      } else {
        const data = await res.json();
        setMessage({ type: 'error', text: data.error || 'Failed to toggle core.' });
      }
    } catch (err: any) {
      setMessage({ type: 'error', text: err.message });
    } finally {
      setIsLoading(false);
    }
  };

  // Actions: Nodes
  const handleAddNode = async (e: React.FormEvent) => {
    e.preventDefault();
    setIsLoading(true);
    setMessage(null);
    try {
      const token = localStorage.getItem('cc_server_token') || localStorage.getItem('cc_client_token') || '';
      const res = await fetch('/api/v2ray/nodes', {
        method: 'POST',
        headers: {
          'Content-Type': 'application/json',
          Authorization: `Bearer ${token}`,
        },
        body: JSON.stringify({
          name: nodeName,
          ip: nodeIP,
          ssh_port: Number(nodeSSHPort),
          ssh_credentials: nodeSSHCreds,
        }),
      });
      if (res.ok) {
        setNodeName('');
        setNodeIP('');
        setNodeSSHCreds('');
        setMessage({ type: 'success', text: 'New remote VPS node registered!' });
        loadData();
      }
    } catch (err: any) {
      setMessage({ type: 'error', text: err.message });
    } finally {
      setIsLoading(false);
    }
  };

  const handleProvision = async (e: React.FormEvent) => {
    e.preventDefault();
    if (!selectedNodeIdForProvision || !provisionPassword) return;
    setIsLoading(true);
    setMessage({ type: 'success', text: 'Connecting to remote host and executing install tools...' });
    try {
      const token = localStorage.getItem('cc_server_token') || localStorage.getItem('cc_client_token') || '';
      const res = await fetch(`/api/v2ray/nodes/${selectedNodeIdForProvision}/provision`, {
        method: 'POST',
        headers: {
          'Content-Type': 'application/json',
          Authorization: `Bearer ${token}`,
        },
        body: JSON.stringify({ password: provisionPassword }),
      });
      if (res.ok) {
        setProvisionPassword('');
        setSelectedNodeIdForProvision(null);
        setMessage({
          type: 'success',
          text: 'SSH provisioning task started in background. Monitor the status below.',
        });
        loadData();
      }
    } catch (err: any) {
      setMessage({ type: 'error', text: err.message });
    } finally {
      setIsLoading(false);
    }
  };

  // Actions: Inbounds
  const handleAddInbound = async (e: React.FormEvent) => {
    e.preventDefault();
    setIsLoading(true);
    setMessage(null);
    try {
      const token = localStorage.getItem('cc_server_token') || localStorage.getItem('cc_client_token') || '';
      const res = await fetch('/api/v2ray/inbounds', {
        method: 'POST',
        headers: {
          'Content-Type': 'application/json',
          Authorization: `Bearer ${token}`,
        },
        body: JSON.stringify({
          tag: inboundTag,
          protocol: inboundProtocol,
          port: Number(inboundPort),
          network: inboundNetwork,
          tls_mode: inboundTlsMode,
          sni: inboundSNI,
          fallback_dest: inboundFallback,
        }),
      });
      if (res.ok) {
        setInboundTag('');
        setMessage({ type: 'success', text: 'Inbound endpoint rule deployed!' });
        loadData();
      }
    } catch (err: any) {
      setMessage({ type: 'error', text: err.message });
    } finally {
      setIsLoading(false);
    }
  };

  // Actions: Users
  const handleAddUser = async (e: React.FormEvent) => {
    e.preventDefault();
    setIsLoading(true);
    setMessage(null);
    try {
      const token = localStorage.getItem('cc_server_token') || localStorage.getItem('cc_client_token') || '';
      const res = await fetch('/api/v2ray/users', {
        method: 'POST',
        headers: {
          'Content-Type': 'application/json',
          Authorization: `Bearer ${token}`,
        },
        body: JSON.stringify({
          name: userName,
          uuid: userUUID || undefined,
          inbound_id: Number(userInboundId),
          traffic_limit: Number(userLimitGB) * 1024 * 1024 * 1024, // GB to Bytes
        }),
      });
      if (res.ok) {
        setUserName('');
        setUserUUID('');
        setMessage({ type: 'success', text: 'Proxy user account created successfully!' });
        loadData();
      }
    } catch (err: any) {
      setMessage({ type: 'error', text: err.message });
    } finally {
      setIsLoading(false);
    }
  };

  // Actions: Firewall & Webhook
  const handleBlockIP = async (e: React.FormEvent) => {
    e.preventDefault();
    if (!blockIP) return;
    setIsLoading(true);
    try {
      const token = localStorage.getItem('cc_server_token') || localStorage.getItem('cc_client_token') || '';
      const res = await fetch('/api/v2ray/firewall/block', {
        method: 'POST',
        headers: {
          'Content-Type': 'application/json',
          Authorization: `Bearer ${token}`,
        },
        body: JSON.stringify({ ip: blockIP }),
      });
      if (res.ok) {
        setFirewallIPs([...firewallIPs, blockIP]);
        setBlockIP('');
        setMessage({ type: 'success', text: 'IP blocked in kernel firewall.' });
      }
    } catch (err: any) {
      setMessage({ type: 'error', text: err.message });
    } finally {
      setIsLoading(false);
    }
  };

  // Actions: MCP RPC
  const handleTestMCP = async () => {
    setIsLoading(true);
    setMcpResult(null);
    try {
      const token = localStorage.getItem('cc_server_token') || localStorage.getItem('cc_client_token') || '';
      const res = await fetch('/api/v2ray/mcp', {
        method: 'POST',
        headers: {
          'Content-Type': 'application/json',
          Authorization: `Bearer ${token}`,
        },
        body: JSON.stringify({
          jsonrpc: '2.0',
          method: mcpMethod,
          params: {},
          id: 1,
        }),
      });
      if (res.ok) {
        setMcpResult(await res.json());
      }
    } catch (err) {
      console.error(err);
    } finally {
      setIsLoading(false);
    }
  };

  return (
    <div>
      {/* Title */}
      <div style={{ display: 'flex', justifyContent: 'space-between', alignItems: 'center', marginBottom: 24 }}>
        <div>
          <h1 style={{ fontSize: 22, fontWeight: 700, color: 'var(--color-brand-heading)', margin: 0 }}>
            V2Ray / Xray Server panel
          </h1>
          <p style={{ fontSize: 12, color: 'var(--color-brand-text)', margin: '4px 0 0' }}>
            Centralized orchestration dashboard to deploy edge nodes, configure inbounds, and audit user quotas.
          </p>
        </div>
        <button className="btn btn--sm" onClick={loadData} disabled={isLoading}>
          <FiRefreshCw className={isLoading ? 'spin-animation' : ''} style={{ marginRight: 6 }} /> Sync Cluster
        </button>
      </div>

      {message && (
        <div
          style={{
            padding: '12px 16px',
            borderRadius: 10,
            marginBottom: 20,
            fontSize: 13,
            fontWeight: 500,
            background: message.type === 'success' ? 'var(--color-brand-light)' : '#fee2e2',
            border: message.type === 'success' ? '1px solid var(--color-brand-border)' : '1px solid #fca5a5',
            color: message.type === 'success' ? 'var(--color-brand)' : '#b91c1c',
          }}
        >
          {message.text}
        </div>
      )}

      {/* Grid structure */}
      <div style={{ display: 'grid', gridTemplateColumns: '1fr 360px', gap: 24, alignItems: 'start' }}>
        {/* Left column: Nodes & Provisioning, inbounds, users */}
        <div style={{ display: 'flex', flexDirection: 'column', gap: 24 }}>
          {/* Remote VPS Edge Nodes */}
          <Suspense fallback={<EdgeNodesSkeleton />}>
            <EdgeNodesCard
              isLoading={isLoading}
              nodes={nodes}
              nodeName={nodeName}
              setNodeName={setNodeName}
              nodeIP={nodeIP}
              setNodeIP={setNodeIP}
              nodeSSHPort={nodeSSHPort}
              setNodeSSHPort={setNodeSSHPort}
              nodeSSHCreds={nodeSSHCreds}
              setNodeSSHCreds={setNodeSSHCreds}
              selectedNodeIdForProvision={selectedNodeIdForProvision}
              setSelectedNodeIdForProvision={setSelectedNodeIdForProvision}
              provisionPassword={provisionPassword}
              setProvisionPassword={setProvisionPassword}
              handleAddNode={handleAddNode}
              handleProvision={handleProvision}
              showHelp={showHelp}
            />
          </Suspense>

          {/* Inbound Endpoint configuration */}
          <Suspense fallback={<InboundsSkeleton />}>
            <InboundEndpointsCard
              isLoading={isLoading}
              inbounds={inbounds}
              inboundTag={inboundTag}
              setInboundTag={setInboundTag}
              inboundProtocol={inboundProtocol}
              setInboundProtocol={setInboundProtocol}
              inboundPort={inboundPort}
              setInboundPort={setInboundPort}
              inboundNetwork={inboundNetwork}
              setInboundNetwork={setInboundNetwork}
              inboundTlsMode={inboundTlsMode}
              setInboundTlsMode={setInboundTlsMode}
              inboundSNI={inboundSNI}
              setInboundSNI={setInboundSNI}
              inboundFallback={inboundFallback}
              setInboundFallback={setInboundFallback}
              handleAddInbound={handleAddInbound}
              showHelp={showHelp}
            />
          </Suspense>

          {/* User Management & Quotas */}
          <Suspense fallback={<UsersSkeleton />}>
            <UserQuotasCard
              isLoading={isLoading}
              users={users}
              inbounds={inbounds}
              userName={userName}
              setUserName={setUserName}
              userUUID={userUUID}
              setUserUUID={setUserUUID}
              userInboundId={userInboundId}
              setUserInboundId={setUserInboundId}
              userLimitGB={userLimitGB}
              setUserLimitGB={setUserLimitGB}
              handleAddUser={handleAddUser}
              showHelp={showHelp}
            />
          </Suspense>

          {/* User Traffic Audits */}
          <Suspense fallback={<CardSkeleton height={180} title="User Traffic Audits" />}>
            <TrafficAuditsCard trafficLogs={trafficLogs} showHelp={showHelp} />
          </Suspense>
        </div>

        {/* Right column: Core Status, Fail2ban, WebDAV logs link, Webhooks & MCP test client */}
        <div style={{ display: 'flex', flexDirection: 'column', gap: 24 }}>
          {/* Active core control */}
          <Suspense fallback={<CardSkeleton height={140} title="Server core engine" />}>
            <ActiveCoreControlCard
              isRunning={isRunning}
              isLoading={isLoading}
              handleToggleCore={handleToggleCore}
            />
          </Suspense>

          {/* WebDAV Logs Explorer */}
          <Suspense fallback={<CardSkeleton height={100} title="WebDAV log access" />}>
            <WebDavLogsCard />
          </Suspense>

          {/* Fail2ban dynamic blocks */}
          <Suspense fallback={<CardSkeleton height={120} title="Fail2ban port protection" />}>
            <FirewallProtectionCard
              blockIP={blockIP}
              setBlockIP={setBlockIP}
              firewallIPs={firewallIPs}
              handleBlockIP={handleBlockIP}
            />
          </Suspense>

          {/* MCP diagnostic client */}
          <Suspense fallback={<CardSkeleton height={120} title="MCP RPC diagnostic prober" />}>
            <McpProberCard
              isLoading={isLoading}
              mcpMethod={mcpMethod}
              setMcpMethod={setMcpMethod}
              mcpResult={mcpResult}
              handleTestMCP={handleTestMCP}
            />
          </Suspense>

          {/* Webhooks config */}
          <Suspense fallback={<CardSkeleton height={150} title="Realtime Webhook notifier" />}>
            <WebhookNotifierCard
              webhookURL={webhookURL}
              setWebhookURL={setWebhookURL}
              webhookSecret={webhookSecret}
              setWebhookSecret={setWebhookSecret}
              handleSaveWebhook={() => setMessage({ type: 'success', text: 'Webhook listener registered!' })}
            />
          </Suspense>
        </div>
      </div>

      {/* Help Modal Popup Dialog */}
      <Suspense fallback={null}>
        <HelpModal title={helpTitle} text={helpText} onClose={() => { setHelpTitle(null); setHelpText(null); }} />
      </Suspense>
    </div>
  );
};
