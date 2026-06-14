import { showGlobalConfirm } from './dialogStore';
import { create } from 'zustand';

export interface VPNNode {
  id: string;
  name: string;
  country: string;
  flag: string;
  ip: string;
  ping: number;
  active: boolean;
  accounts: number;
  scheduledIn: string;
  scheduledOut: string;
  balance: string;
}

interface BandwidthData {
  time: string;
  upload: number;
  download: number;
}

interface DashboardState {
  connectionState: 'disconnected' | 'connecting' | 'connected';
  selectedNode: VPNNode | null;
  nodes: VPNNode[];
  bandwidthHistory: BandwidthData[];
  totalUsage: { upload: number; download: number };
  latency: number;
  wsConnected: boolean;
  logs: string[];
  
  // Real-time gRPC Stats
  activeConns: number;
  totalUplink: number;
  totalDownlink: number;
  trafficHistory: { time: string; upload: number; download: number }[];
  
  // Feature Stats
  schedulerStats: any;
  domainStats: any;

  // Actions
  fetchRealNodes: () => Promise<void>;
  checkClientStatus: () => Promise<void>;
  connectNode: (node: VPNNode) => Promise<void>;
  disconnectNode: () => Promise<void>;
  deleteAllNodes: () => Promise<void>;
  initWebSocket: (token: string) => () => void;
  connectStream: () => void;
  disconnectStream: () => void;
  fetchSchedulerStats: () => Promise<void>;
  fetchDomainStats: () => Promise<void>;
}

const mapConfigToNode = (cfg: any): VPNNode => {
  const nameLower = cfg.name.toLowerCase();
  let flag = '🌐';
  let country = 'Global';
  
  if (nameLower.includes('singapore') || nameLower.includes('sgd') || nameLower.includes('sg')) {
    flag = '🇸🇬';
    country = 'Singapore';
  } else if (nameLower.includes('germany') || nameLower.includes('de') || nameLower.includes('frankfurt') || nameLower.includes('eur')) {
    flag = '🇩🇪';
    country = 'Germany';
  } else if (nameLower.includes('united kingdom') || nameLower.includes('uk') || nameLower.includes('london') || nameLower.includes('gbp') || nameLower.includes('gb')) {
    flag = '🇬🇧';
    country = 'United Kingdom';
  } else if (nameLower.includes('united states') || nameLower.includes('us') || nameLower.includes('usa') || nameLower.includes('usd') || nameLower.includes('new york')) {
    flag = '🇺🇸';
    country = 'United States';
  } else if (nameLower.includes('australia') || nameLower.includes('aud') || nameLower.includes('sydney') || nameLower.includes('au')) {
    flag = '🇦🇺';
    country = 'Australia';
  } else if (nameLower.includes('iran') || nameLower.includes('ir')) {
    flag = '🇮🇷';
    country = 'Iran';
  } else if (nameLower.includes('finland') || nameLower.includes('fi')) {
    flag = '🇫🇮';
    country = 'Finland';
  } else if (nameLower.includes('netherlands') || nameLower.includes('nl')) {
    flag = '🇳🇱';
    country = 'Netherlands';
  } else if (nameLower.includes('france') || nameLower.includes('fr')) {
    flag = '🇫🇷';
    country = 'France';
  }
  
  return {
    id: String(cfg.ID),
    name: cfg.name,
    country: country,
    flag: flag,
    ip: cfg.address,
    ping: cfg.latency_ms > 0 ? Number(cfg.latency_ms) : 0,
    active: cfg.is_active,
    accounts: 1,
    scheduledIn: '',
    scheduledOut: '',
    balance: `${cfg.protocol.toUpperCase()} / ${cfg.network.toUpperCase()}`
  };
};

export const useDashboardStore = create<DashboardState>((set, get) => {
  let ws: WebSocket | null = null;
  let wsStats: WebSocket | null = null;

  return {
    connectionState: 'disconnected',
    selectedNode: null,
    nodes: [],
    bandwidthHistory: Array.from({ length: 30 }, (_, i) => ({
      time: new Date(Date.now() - (30 - i) * 2000).toLocaleTimeString([], { hour: '2-digit', minute: '2-digit', second: '2-digit' }),
      upload: 0,
      download: 0
    })),
    totalUsage: { upload: 0, download: 0 },
    latency: 0,
    wsConnected: false,
    logs: ['[System] CleverConnect Client Ready. Loading nodes and statistics...'],

    // Real-time gRPC Stats
    activeConns: 0,
    totalUplink: 0,
    totalDownlink: 0,
    trafficHistory: [],

    // Feature Stats
    schedulerStats: null,
    domainStats: null,

    fetchRealNodes: async () => {
      const token = localStorage.getItem('cc_client_token') || '';
      try {
        const res = await fetch('/api/v2ray/client/configs?limit=1000', {
          headers: { Authorization: `Bearer ${token}` }
        });
        if (res.ok) {
          const data = await res.json();
          const configs = data.data || [];
          const mapped = configs.map(mapConfigToNode);
          set({ nodes: mapped });
          
          const active = configs.find((c: any) => c.is_active);
          const isRunning = get().connectionState === 'connected';
          if (active && isRunning) {
            set({ selectedNode: mapConfigToNode(active) });
          }
        }
      } catch (err) {
        console.error('Failed to fetch real nodes:', err);
      }
    },

    checkClientStatus: async () => {
      const token = localStorage.getItem('cc_client_token') || '';
      try {
        const res = await fetch('/api/v2ray/client/status', {
          headers: { Authorization: `Bearer ${token}` }
        });
        if (res.ok) {
          const data = await res.json();
          if (data.is_running) {
            const configsRes = await fetch('/api/v2ray/client/configs?limit=1000', {
              headers: { Authorization: `Bearer ${token}` }
            });
            if (configsRes.ok) {
              const configsData = await configsRes.json();
              const configs = configsData.data || [];
              const active = configs.find((c: any) => c.is_active);
              if (active) {
                set({
                  connectionState: 'connected',
                  selectedNode: mapConfigToNode(active)
                });
                get().connectStream();
              } else {
                set({ connectionState: 'connected' });
              }
            }
          } else {
            set({ connectionState: 'disconnected', selectedNode: null });
            get().disconnectStream();
          }
        }
      } catch (err) {}
    },

    connectNode: async (node) => {
      set({ connectionState: 'connecting', selectedNode: node });
      set((state) => ({ logs: [...state.logs.slice(-49), `[System] Activating profile: ${node.name}...`] }));

      const token = localStorage.getItem('cc_client_token') || '';
      try {
        const activeRes = await fetch(`/api/v2ray/client/configs/${node.id}/active`, {
          method: 'POST',
          headers: { 'Authorization': `Bearer ${token}` }
        });
        if (!activeRes.ok) {
          throw new Error('Failed to activate node profile');
        }

        set((state) => ({ logs: [...state.logs.slice(-49), `[System] Bootstrapping V2Ray core...`] }));

        const startRes = await fetch('/api/v2ray/client/start', {
          method: 'POST',
          headers: { 'Authorization': `Bearer ${token}` }
        });
        if (!startRes.ok) {
          const data = await startRes.json();
          throw new Error(data.error || 'Failed to start proxy core');
        }

        set({ connectionState: 'connected', latency: node.ping });
        set((state) => ({
          logs: [
            ...state.logs.slice(-49),
            `[System] V2Ray tunnel successfully established to ${node.name}!`,
            `[System] Telemetry stream initialized.`
          ]
        }));

        get().connectStream();
        await get().fetchRealNodes();
      } catch (err: any) {
        set({ connectionState: 'disconnected', selectedNode: null });
        set((state) => ({
          logs: [...state.logs.slice(-49), `[Error] Connection failed: ${err.message}`]
        }));
      }
    },

    disconnectNode: async () => {
      const token = localStorage.getItem('cc_client_token') || '';
      try {
        set((state) => ({ logs: [...state.logs.slice(-49), '[System] Shutting down V2Ray core...'] }));
        const res = await fetch('/api/v2ray/client/stop', {
          method: 'POST',
          headers: { 'Authorization': `Bearer ${token}` }
        });
        if (!res.ok) {
          throw new Error('Failed to stop core');
        }
        set({ connectionState: 'disconnected', selectedNode: null, latency: 0 });
        set((state) => ({ logs: [...state.logs.slice(-49), '[System] V2Ray proxy core offline.'] }));
        get().disconnectStream();
        await get().fetchRealNodes();
      } catch (err: any) {
        set((state) => ({ logs: [...state.logs.slice(-49), `[Error] Stop core failed: ${err.message}`] }));
      }
    },

    deleteAllNodes: async () => {
      const confirmed = await showGlobalConfirm('Are you sure you want to delete all gateway nodes? This will also remove them from the backend configs!', { title: 'Delete Gateway Nodes', variant: 'warning' });
      if (!confirmed) return;
      
      try {
        const token = localStorage.getItem('cc_client_token') || '';
        await fetch('/api/v2ray/client/configs/all', {
          method: 'DELETE',
          headers: { 'Authorization': `Bearer ${token}` }
        });
        
        set({ nodes: [], selectedNode: null, connectionState: 'disconnected', latency: 0 });
        set((state) => ({ logs: [...state.logs.slice(-49), '[System] All gateway nodes have been purged.'] }));
        get().disconnectStream();
      } catch (err) {
        console.error('Failed to delete nodes:', err);
      }
    },

    connectStream: () => {
      if (wsStats) return;
      
      const protocol = window.location.protocol === 'https:' ? 'wss:' : 'ws:';
      const host = window.location.host;
      const token = localStorage.getItem('cc_client_token') || '';
      
      wsStats = new WebSocket(`${protocol}//${host}/ws/stats?token=${token}`);

      wsStats.onopen = () => {};
      wsStats.onclose = () => {};

      wsStats.onmessage = (event) => {
        try {
          const data = JSON.parse(event.data);
          const now = new Date().toLocaleTimeString('en-US', { hour12: false, hour: '2-digit', minute: '2-digit', second: '2-digit' });

          set((state) => {
            const upSpeedKb = data.uplinkSpeed / 1024;
            const downSpeedKb = data.downlinkSpeed / 1024;

            const newHistory = [...state.trafficHistory, {
              time: now,
              upload: Number(upSpeedKb.toFixed(1)), 
              download: Number(downSpeedKb.toFixed(1)),
            }].slice(-30);

            // Also keep standard bandwidthHistory updated
            const newBandwidthHistory = [...state.bandwidthHistory.slice(1), {
              time: now,
              upload: Number((upSpeedKb / 1024).toFixed(2)), // MB/s
              download: Number((downSpeedKb / 1024).toFixed(2)) // MB/s
            }];

            return {
              activeConns: data.activeConns,
              totalUplink: data.totalUplink,
              totalDownlink: data.totalDownlink,
              trafficHistory: newHistory,
              bandwidthHistory: newBandwidthHistory,
              totalUsage: {
                upload: data.totalUplink / (1024 * 1024), // to MB
                download: data.totalDownlink / (1024 * 1024) // to MB
              }
            };
          });
        } catch (err) {}
      };
    },

    disconnectStream: () => {
      if (wsStats) {
        wsStats.close();
        wsStats = null;
      }
    },

    fetchSchedulerStats: async () => {
      const token = localStorage.getItem('cc_client_token') || '';
      try {
        const res = await fetch('/api/scheduler/stats', {
          headers: { Authorization: `Bearer ${token}` }
        });
        if (res.ok) {
          const stats = await res.json();
          set({ schedulerStats: stats });
        }
      } catch (err) {}
    },

    fetchDomainStats: async () => {
      const token = localStorage.getItem('cc_client_token') || '';
      try {
        const res = await fetch('/api/domains?limit=1', {
          headers: { Authorization: `Bearer ${token}` }
        });
        if (res.ok) {
          const data = await res.json();
          set({ domainStats: data.stats || null });
        }
      } catch (err) {}
    },

    initWebSocket: (token) => {
      const protocol = window.location.protocol === 'https:' ? 'wss:' : 'ws:';
      const wsUrl = `${protocol}//${window.location.host}/ws?token=${token}`;

      const connect = () => {
        try {
          ws = new WebSocket(wsUrl);

          ws.onopen = () => {
            set({ wsConnected: true });
            set((state) => ({ logs: [...state.logs.slice(-49), '[System] Web telemetry channel open.'] }));
          };

          ws.onmessage = (event) => {
            try {
              const data = JSON.parse(event.data);
              if (data.type === 'bandwidth') {
                set((state) => {
                  const isRunning = state.connectionState === 'connected';
                  // Only use mock bandwidth if not running or stats WS not active
                  if (!isRunning) {
                    const newHistory = [
                      ...state.bandwidthHistory.slice(1),
                      {
                        time: new Date().toLocaleTimeString([], { hour: '2-digit', minute: '2-digit', second: '2-digit' }),
                        upload: data.upload,
                        download: data.download
                      }
                    ];
                    return {
                      bandwidthHistory: newHistory,
                      totalUsage: {
                        upload: data.totalUpload,
                        download: data.totalDownload
                      },
                      latency: data.latency || state.latency
                    };
                  }
                  return {};
                });
              } else if (data.type === 'log') {
                set((state) => ({ logs: [...state.logs.slice(-49), data.message] }));
              }
            } catch (err) {
              // Ignore non-json
            }
          };

          ws.onclose = () => {
            set({ wsConnected: false });
            setTimeout(() => {
              connect();
            }, 5000);
          };
        } catch (e) {
          set({ wsConnected: false });
        }
      };

      connect();

      return () => {
        if (ws) {
          ws.close();
          ws = null;
        }
      };
    }
  };
});
