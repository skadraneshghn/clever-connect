import { create } from 'zustand';

export interface TorrentJob {
  info_hash: string;
  name: string;
  magnet_uri: string;
  torrent_path: string;
  save_directory: string;
  status: 'downloading' | 'paused' | 'completed' | 'seeding' | 'error';
  total_bytes: number;
  downloaded: number;
  uploaded: number;
  progress: number;
  download_speed: number;
  upload_speed: number;
  peers: number;
  error_message: string;
  created_at: string;
  updated_at: string;
}

export interface LeechJob {
  id: string;
  url: string;
  filename: string;
  save_directory: string;
  total_bytes: number;
  downloaded: number;
  status: 'pending' | 'downloading' | 'paused' | 'completed' | 'error';
  progress: number;
  speed: number;
  threads: number;
  error_message: string;
  created_at: string;
  updated_at: string;
}

interface JobsState {
  torrents: TorrentJob[];
  leechJobs: LeechJob[];
  wsConnected: boolean;
  initWebSocket: (token: string) => () => void;
  sendAction: (cmd: { action: string; info_hash?: string; job_id?: string; delete_files?: boolean }) => void;
}

export const useJobsStore = create<JobsState>((set, get) => {
  let ws: WebSocket | null = null;
  let reconnectTimeout: any = null;

  return {
    torrents: [],
    leechJobs: [],
    wsConnected: false,

    initWebSocket: (token) => {
      const protocol = window.location.protocol === 'https:' ? 'wss:' : 'ws:';
      const wsUrl = `${protocol}//${window.location.host}/ws/jobs?token=${token}`;

      const connect = () => {
        try {
          ws = new WebSocket(wsUrl);

          ws.onopen = () => {
            set({ wsConnected: true });
          };

          ws.onmessage = (event) => {
            try {
              const data = JSON.parse(event.data);
              set({
                torrents: data.torrents || [],
                leechJobs: data.leechJobs || []
              });
            } catch (err) {
              console.error('Failed to parse jobs WS data', err);
            }
          };

          ws.onclose = () => {
            set({ wsConnected: false });
            // Reconnect after 3 seconds
            reconnectTimeout = setTimeout(connect, 3000);
          };

          ws.onerror = () => {
            set({ wsConnected: false });
          };
        } catch (e) {
          set({ wsConnected: false });
        }
      };

      connect();

      return () => {
        if (reconnectTimeout) clearTimeout(reconnectTimeout);
        if (ws) {
          ws.onclose = null;
          ws.close();
          ws = null;
        }
      };
    },

    sendAction: (cmd) => {
      if (ws && ws.readyState === WebSocket.OPEN) {
        ws.send(JSON.stringify(cmd));
      } else {
        console.warn('Jobs WebSocket is not connected. Action ignored.', cmd);
      }
    }
  };
});
