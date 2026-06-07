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
  balance: string; // traffic limit / allowance remaining
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
  connectNode: (node: VPNNode) => Promise<void>;
  disconnectNode: () => Promise<void>;
  deleteAllNodes: () => Promise<void>;
  initWebSocket: (token: string) => () => void;
}

export const useDashboardStore = create<DashboardState>((set, get) => {
  let ws: WebSocket | null = null;
  let mockInterval: any = null;

  return {
    connectionState: 'disconnected',
    selectedNode: null,
    nodes: [
      { id: '1', name: 'SGD - Singapore Premium', country: 'Singapore', flag: '🇸🇬', ip: '139.59.241.12', ping: 42, active: true, accounts: 2, scheduledIn: '3.2 GB', scheduledOut: '1.1 GB', balance: '18.4 GB' },
      { id: '2', name: 'EUR - Frankfurt HighSpeed', country: 'Germany', flag: '🇩🇪', ip: '46.101.200.89', ping: 124, active: true, accounts: 1, scheduledIn: '5.8 GB', scheduledOut: '2.4 GB', balance: '21.9 GB' },
      { id: '3', name: 'GBP - London Secure', country: 'United Kingdom', flag: '🇬🇧', ip: '178.62.90.104', ping: 145, active: false, accounts: 1, scheduledIn: '2.1 GB', scheduledOut: '780 MB', balance: '12.9 GB' },
      { id: '4', name: 'USD - New York Fiber', country: 'United States', flag: '🇺🇸', ip: '104.248.50.31', ping: 180, active: true, accounts: 3, scheduledIn: '5.6 GB', scheduledOut: '2.4 GB', balance: '24.8 GB' },
      { id: '5', name: 'AUD - Sydney Edge', country: 'Australia', flag: '🇦🇺', ip: '13.211.45.160', ping: 220, active: false, accounts: 1, scheduledIn: '1.9 GB', scheduledOut: '650 MB', balance: '8.1 GB' }
    ],
    bandwidthHistory: Array.from({ length: 30 }, (_, i) => ({
      time: new Date(Date.now() - (30 - i) * 2000).toLocaleTimeString([], { hour: '2-digit', minute: '2-digit', second: '2-digit' }),
      upload: 0,
      download: 0
    })),
    totalUsage: { upload: 2450, download: 8120 }, // in MB
    latency: 0,
    wsConnected: false,
    logs: ['[System] Client ready. Select a node to establish connection.'],

    connectNode: async (node) => {
      set({ connectionState: 'connecting', selectedNode: node });
      set((state) => ({ logs: [...state.logs.slice(-49), `[System] Resolving IP ${node.ip}...`, `[System] Handshaking via TLS...`] }));

      await new Promise((resolve) => setTimeout(resolve, 1500));

      set({ connectionState: 'connected', latency: node.ping });
      set((state) => ({
        logs: [
          ...state.logs.slice(-49),
          `[System] Connection established successfully to ${node.name}!`,
          `[System] Tunned via Secure TLS Protocol. MTU 1420.`
        ]
      }));

      // Setup mock data updates if WS is not active
      if (!get().wsConnected) {
        if (mockInterval) clearInterval(mockInterval);
        mockInterval = setInterval(() => {
          const downloadSpeed = Math.floor(Math.random() * 80) + 10; // MB/s
          const uploadSpeed = Math.floor(Math.random() * 20) + 2;

          set((state) => {
            const newHistory = [
              ...state.bandwidthHistory.slice(1),
              {
                time: new Date().toLocaleTimeString([], { hour: '2-digit', minute: '2-digit', second: '2-digit' }),
                upload: uploadSpeed,
                download: downloadSpeed
              }
            ];

            return {
              bandwidthHistory: newHistory,
              totalUsage: {
                upload: state.totalUsage.upload + uploadSpeed / 10,
                download: state.totalUsage.download + downloadSpeed / 10
              },
              latency: node.ping + Math.floor(Math.random() * 10) - 5
            };
          });
        }, 2000);
      }
    },

    disconnectNode: async () => {
      if (mockInterval) {
        clearInterval(mockInterval);
        mockInterval = null;
      }
      const nodeName = get().selectedNode?.name || 'Node';
      set({ connectionState: 'disconnected', selectedNode: null, latency: 0 });
      set((state) => ({
        logs: [...state.logs.slice(-49), `[System] Connection closed to ${nodeName}.`],
        bandwidthHistory: state.bandwidthHistory.map((h) => ({ ...h, upload: 0, download: 0 }))
      }));
    },

    deleteAllNodes: async () => {
      if (!window.confirm('Are you sure you want to delete all gateway nodes? This will also remove them from the backend configs!')) return;
      
      try {
        const token = localStorage.getItem('cc_client_token') || '';
        await fetch('/api/v2ray/client/configs/all', {
          method: 'DELETE',
          headers: { 'Authorization': `Bearer ${token}` }
        });
        
        set({ nodes: [], selectedNode: null, connectionState: 'disconnected', latency: 0 });
        set((state) => ({ logs: [...state.logs.slice(-49), '[System] All gateway nodes have been purged.'] }));
        if (mockInterval) {
          clearInterval(mockInterval);
          mockInterval = null;
        }
      } catch (err) {
        console.error('Failed to delete nodes:', err);
      }
    },

    initWebSocket: (token) => {
      const protocol = window.location.protocol === 'https:' ? 'wss:' : 'ws:';
      const wsUrl = `${protocol}//${window.location.host}/ws?token=${token}`;

      const connect = () => {
        try {
          ws = new WebSocket(wsUrl);

          ws.onopen = () => {
            set({ wsConnected: true });
            set((state) => ({ logs: [...state.logs.slice(-49), '[System] WebSocket channel established. Real-time updates enabled.'] }));
          };

          ws.onmessage = (event) => {
            try {
              const data = JSON.parse(event.data);
              if (data.type === 'bandwidth') {
                set((state) => {
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
            set((state) => ({ logs: [...state.logs.slice(-49), '[Warning] WebSocket closed. Running on autonomous mock fallback.'] }));
            // Retry connection after 5 seconds
            setTimeout(() => {
              if (get().connectionState === 'connected') {
                connect();
              }
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
        if (mockInterval) {
          clearInterval(mockInterval);
          mockInterval = null;
        }
      };
    }
  };
});
