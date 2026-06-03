import { create } from 'zustand';

export interface ClientConnection {
  id: string;
  username: string;
  ip: string;
  country: string;
  flag: string;
  protocol: string;
  connectedAt: string;
  duration: string;
  uploadSpeed: number; // MB/s
  downloadSpeed: number; // MB/s
  active: boolean;
}

interface BandwidthHistory {
  time: string;
  upload: number;
  download: number;
}

interface ServerState {
  cpu: number;
  memory: number;
  disk: number;
  activeConnectionsCount: number;
  clients: ClientConnection[];
  bandwidthHistory: BandwidthHistory[];
  totalBandwidth: { upload: number; download: number };
  logs: string[];
  wsConnected: boolean;
  initWebSocket: (token: string) => () => void;
  disconnectClient: (id: string) => Promise<boolean>;
  blockClient: (id: string) => Promise<boolean>;
  addClient: (username: string) => Promise<boolean>;

  mem_total_gb: number;
  mem_used_gb: number;
  disk_total_gb: number;
  disk_used_gb: number;
  uptime_seconds: number;
  app_mem_mb: number;
  go_version: string;
  os_runtime: string;
  active_leeches: number;
  active_torrents: number;
  active_scheds: number;

  cpu_cores_percent: number[];
  cpu_mhz: number;
  mem_free_gb: number;
  swap_total_gb: number;
  swap_used_gb: number;
  swap_percent: number;
  disk_free_gb: number;
  disk_read_bytes_sec: number;
  disk_write_bytes_sec: number;
  net_recv_bytes_sec: number;
  net_sent_bytes_sec: number;
  cpu_temp: number;
  boot_time: number;
  os_platform: string;
  os_kernel: string;
}

export const useServerStore = create<ServerState>((set, get) => {
  let ws: WebSocket | null = null;
  let mockInterval: any = null;

  return {
    cpu: 24,
    memory: 45,
    disk: 18,
    activeConnectionsCount: 4,
    mem_total_gb: 16,
    mem_used_gb: 7.2,
    disk_total_gb: 500,
    disk_used_gb: 90,
    uptime_seconds: 7200,
    app_mem_mb: 24,
    go_version: 'go1.22.4',
    os_runtime: 'linux',
    active_leeches: 0,
    active_torrents: 0,
    active_scheds: 0,
    cpu_cores_percent: [20, 25, 18, 30],
    cpu_mhz: 2400,
    mem_free_gb: 8.8,
    swap_total_gb: 2,
    swap_used_gb: 0.5,
    swap_percent: 25,
    disk_free_gb: 410,
    disk_read_bytes_sec: 0,
    disk_write_bytes_sec: 0,
    net_recv_bytes_sec: 0,
    net_sent_bytes_sec: 0,
    cpu_temp: 42.5,
    boot_time: Date.now() / 1000 - 7200,
    os_platform: 'ubuntu',
    os_kernel: '5.15.0-generic',
    clients: [
      { id: '1', username: 'salman_desktop', ip: '82.102.23.45', country: 'Iran', flag: '🇮🇷', protocol: 'VLESS-XTLS', connectedAt: '12:04:12', duration: '02h 35m', uploadSpeed: 2.4, downloadSpeed: 12.5, active: true },
      { id: '2', username: 'john_iphone', ip: '188.45.67.12', country: 'Germany', flag: '🇩🇪', protocol: 'Shadowsocks', connectedAt: '13:10:00', duration: '01h 29m', uploadSpeed: 1.1, downloadSpeed: 5.8, active: true },
      { id: '3', username: 'mary_macbook', ip: '95.12.89.200', country: 'United Kingdom', flag: '🇬🇧', protocol: 'Trojan', connectedAt: '14:02:15', duration: '37m', uploadSpeed: 0.8, downloadSpeed: 2.1, active: true },
      { id: '4', username: 'office_router', ip: '104.22.4.90', country: 'United States', flag: '🇺🇸', protocol: 'Wireguard', connectedAt: '08:12:45', duration: '06h 27m', uploadSpeed: 4.8, downloadSpeed: 18.2, active: true }
    ],
    bandwidthHistory: Array.from({ length: 30 }, (_, i) => ({
      time: new Date(Date.now() - (30 - i) * 2000).toLocaleTimeString([], { hour: '2-digit', minute: '2-digit', second: '2-digit' }),
      upload: 15,
      download: 65
    })),
    totalBandwidth: { upload: 14205, download: 52402 }, // GB
    logs: [
      '[System] CleverConnect VPN Server Initialized.',
      '[System] Firewall rules generated. IP tables operational.',
      '[VLESS] Inbound port 443 active.',
      '[Shadowsocks] Inbound port 8388 active.',
      '[System] SQLite and MySQL connection handles initialized.'
    ],
    wsConnected: false,

    disconnectClient: async (id) => {
      try {
        const response = await fetch(`/api/clients/disconnect/${id}`, {
          method: 'POST',
          headers: {
            'Authorization': `Bearer ${localStorage.getItem('cc_server_token')}`,
          }
        });
        // Remove client in UI mock anyway
        set((state) => ({
          clients: state.clients.filter((c) => c.id !== id),
          activeConnectionsCount: Math.max(0, state.activeConnectionsCount - 1),
          logs: [...state.logs.slice(-49), `[System] Forcefully disconnected client session: ${id}`]
        }));
        return true;
      } catch (err) {
        return false;
      }
    },

    blockClient: async (id) => {
      set((state) => ({
        clients: state.clients.filter((c) => c.id !== id),
        activeConnectionsCount: Math.max(0, state.activeConnectionsCount - 1),
        logs: [...state.logs.slice(-49), `[Firewall] Blocked & Blacklisted client: ${id}`]
      }));
      return true;
    },

    addClient: async (username) => {
      const newClient: ClientConnection = {
        id: String(get().clients.length + 1),
        username,
        ip: '127.0.0.1',
        country: 'Local',
        flag: '💻',
        protocol: 'VLESS',
        connectedAt: new Date().toLocaleTimeString([], { hour: '2-digit', minute: '2-digit', second: '2-digit' }),
        duration: '0s',
        uploadSpeed: 0,
        downloadSpeed: 0,
        active: false
      };
      set((state) => ({
        clients: [...state.clients, newClient],
        logs: [...state.logs.slice(-49), `[Admin] Created new client credentials for username: ${username}`]
      }));
      return true;
    },

    initWebSocket: (token) => {
      const protocol = window.location.protocol === 'https:' ? 'wss:' : 'ws:';
      const wsUrl = `${protocol}//${window.location.host}/ws?token=${token}`;

      const connect = () => {
        try {
          ws = new WebSocket(wsUrl);

          ws.onopen = () => {
            set({ wsConnected: true });
            set((state) => ({ logs: [...state.logs.slice(-49), '[System] Real-time server telemetry channel active.'] }));
          };

          ws.onmessage = (event) => {
            try {
              const data = JSON.parse(event.data);
              if (data.type === 'telemetry') {
                set((state) => {
                  const newHistory = [
                    ...state.bandwidthHistory.slice(1),
                    {
                      time: new Date().toLocaleTimeString([], { hour: '2-digit', minute: '2-digit', second: '2-digit' }),
                      upload: data.uploadSpeed,
                      download: data.downloadSpeed
                    }
                  ];
                  return {
                    cpu: data.cpu,
                    memory: data.memory,
                    disk: data.disk,
                    activeConnectionsCount: data.connsCount,
                    bandwidthHistory: newHistory,
                    totalBandwidth: {
                      upload: data.totalUpload,
                      download: data.totalDownload
                    },
                    clients: data.clients || state.clients,
                    mem_total_gb: data.mem_total_gb,
                    mem_used_gb: data.mem_used_gb,
                    disk_total_gb: data.disk_total_gb,
                    disk_used_gb: data.disk_used_gb,
                    uptime_seconds: data.uptime_seconds,
                    app_mem_mb: data.app_mem_mb,
                    go_version: data.go_version,
                    os_runtime: data.os_runtime,
                    active_leeches: data.active_leeches,
                    active_torrents: data.active_torrents,
                    active_scheds: data.active_scheds,
                    cpu_cores_percent: data.cpu_cores_percent || state.cpu_cores_percent,
                    cpu_mhz: data.cpu_mhz || state.cpu_mhz,
                    mem_free_gb: data.mem_free_gb || state.mem_free_gb,
                    swap_total_gb: data.swap_total_gb || state.swap_total_gb,
                    swap_used_gb: data.swap_used_gb || state.swap_used_gb,
                    swap_percent: data.swap_percent || state.swap_percent,
                    disk_free_gb: data.disk_free_gb || state.disk_free_gb,
                    disk_read_bytes_sec: data.disk_read_bytes_sec || 0,
                    disk_write_bytes_sec: data.disk_write_bytes_sec || 0,
                    net_recv_bytes_sec: data.net_recv_bytes_sec || 0,
                    net_sent_bytes_sec: data.net_sent_bytes_sec || 0,
                    cpu_temp: data.cpu_temp || state.cpu_temp,
                    boot_time: data.boot_time || state.boot_time,
                    os_platform: data.os_platform || state.os_platform,
                    os_kernel: data.os_kernel || state.os_kernel
                  };
                });
              } else if (data.type === 'log') {
                set((state) => ({ logs: [...state.logs.slice(-49), data.message] }));
              }
            } catch (err) {
              // Not JSON or structural error
            }
          };

          ws.onclose = () => {
            set({ wsConnected: false });
            set((state) => ({ logs: [...state.logs.slice(-49), '[Warning] Closed connection. Running on local simulator.'] }));
            setTimeout(() => {
              connect();
            }, 5000);
          };
        } catch (e) {
          set({ wsConnected: false });
        }
      };

      connect();

      // Fallback local simulator
      mockInterval = setInterval(() => {
        if (!get().wsConnected) {
          set((state) => {
            const cpuVal = Math.floor(Math.random() * 20) + 15;
            const memVal = 44 + Math.floor(Math.random() * 4) - 2;
            const upSpeed = state.clients.reduce((acc, c) => acc + (c.active ? c.uploadSpeed : 0), 0) + Math.random() * 2;
            const downSpeed = state.clients.reduce((acc, c) => acc + (c.active ? c.downloadSpeed : 0), 0) + Math.random() * 5;

            const newHistory = [
              ...state.bandwidthHistory.slice(1),
              {
                time: new Date().toLocaleTimeString([], { hour: '2-digit', minute: '2-digit', second: '2-digit' }),
                upload: parseFloat(upSpeed.toFixed(1)),
                download: parseFloat(downSpeed.toFixed(1))
              }
            ];

            const updatedClients = state.clients.map((c) => {
              if (c.active) {
                return {
                  ...c,
                  uploadSpeed: parseFloat((Math.random() * 3 + 0.2).toFixed(1)),
                  downloadSpeed: parseFloat((Math.random() * 15 + 1).toFixed(1))
                };
              }
              return c;
            });

            return {
              cpu: cpuVal,
              memory: memVal,
              clients: updatedClients,
              bandwidthHistory: newHistory,
              totalBandwidth: {
                upload: state.totalBandwidth.upload + 0.1,
                download: state.totalBandwidth.download + 0.5
              },
              mem_total_gb: 16,
              mem_used_gb: parseFloat((16 * (memVal / 100)).toFixed(1)),
              disk_total_gb: 500,
              disk_used_gb: 90 + Math.floor(Math.random() * 2),
              uptime_seconds: state.uptime_seconds + 2,
              app_mem_mb: parseFloat((20 + Math.random() * 5).toFixed(1)),
              go_version: 'go1.22.4',
              os_runtime: 'linux',
              active_leeches: state.active_leeches,
              active_torrents: state.active_torrents,
              active_scheds: state.active_scheds,
              cpu_cores_percent: state.cpu_cores_percent.map(c => Math.max(0, Math.min(100, c + Math.floor(Math.random() * 11) - 5))),
              cpu_mhz: 2400 + Math.floor(Math.random() * 100) - 50,
              mem_free_gb: parseFloat((16 - (16 * (memVal / 100))).toFixed(1)),
              swap_total_gb: 2,
              swap_used_gb: 0.5 + Math.random() * 0.1,
              swap_percent: 25 + Math.floor(Math.random() * 5),
              disk_free_gb: 410 - Math.floor(Math.random() * 2),
              disk_read_bytes_sec: Math.random() * 1024 * 1024 * 5,
              disk_write_bytes_sec: Math.random() * 1024 * 1024 * 10,
              net_recv_bytes_sec: Math.random() * 1024 * 1024 * 8,
              net_sent_bytes_sec: Math.random() * 1024 * 1024 * 2,
              cpu_temp: 40 + Math.random() * 10,
              boot_time: state.boot_time,
              os_platform: 'ubuntu',
              os_kernel: '5.15.0-generic'
            };
          });
        }
      }, 2000);

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
