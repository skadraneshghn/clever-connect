import { create } from 'zustand';

export interface ArteryStatus {
  tag: string;
  node_name: string;
  address: string;
  port: number;
  state: string;
  srtt_ms: number;
  loss_pct: number;
  win_rate: number;
  throughput_mbps: number;
  error_count: number;
}

export interface EngineStatus {
  state: string;
  mode: string;
  active_count: number;
  total_pool: number;
  arteries: ArteryStatus[];
  last_eval_at: string;
  // Real-time traffic metrics
  bytes_tx: number;
  bytes_rx: number;
  uplink_bps: number;
  downlink_bps: number;
  active_conns: number;
}

export interface BondingConfig {
  id?: number;
  is_active: boolean;
  mode: string;
  striping_mode: string;
  max_arteries: number;
  min_arteries: number;
  combiner_url: string;
  origin_id: string;
  psk_hex: string;
  frame_size: number;
  socks_port: number;
  http_port: number;
  eval_window_ms: number;
  demote_rtt_x: number;
  promote_rtt_x: number;
  loss_demote_pct: number;
  cooldown_sec: number;
  error_budget: number;
}

export interface DiagnosticStep {
  name: string;
  description: string;
  status: 'success' | 'warning' | 'error' | 'pending';
  error_message?: string;
  details?: string;
}

interface MultipathState {
  // Engine status
  status: EngineStatus | null;
  config: BondingConfig | null;
  loading: boolean;
  error: string | null;
  wsConnected: boolean;
  diagnoseResults: DiagnosticStep[] | null;
  diagnoseLoading: boolean;

  // Telemetry history (last 60 data points)
  arteryHistory: Record<string, { time: string; srtt: number; loss: number }[]>;

  // Actions
  fetchConfig: () => Promise<void>;
  saveConfig: (cfg: BondingConfig) => Promise<void>;
  fetchStatus: () => Promise<void>;
  startEngine: () => Promise<void>;
  stopEngine: () => Promise<void>;
  connectTelemetry: () => () => void;
  runDiagnostics: () => Promise<void>;
}

const getToken = () => localStorage.getItem('cc_client_token') || '';
const apiHeaders = () => ({
  'Authorization': `Bearer ${getToken()}`,
  'Content-Type': 'application/json',
});

export const useMultipathStore = create<MultipathState>((set, get) => {
  let ws: WebSocket | null = null;

  return {
    status: null,
    config: null,
    loading: false,
    error: null,
    wsConnected: false,
    diagnoseResults: null,
    diagnoseLoading: false,
    arteryHistory: {},

    fetchConfig: async () => {
      try {
        const res = await fetch('/api/v2ray/bonding/config', { headers: apiHeaders() });
        if (res.ok) {
          const cfg = await res.json();
          set({ config: cfg });
        }
      } catch (err: any) {
        set({ error: err.message });
      }
    },

    saveConfig: async (cfg) => {
      set({ loading: true, error: null });
      try {
        const res = await fetch('/api/v2ray/bonding/config', {
          method: 'POST',
          headers: apiHeaders(),
          body: JSON.stringify(cfg),
        });
        if (res.ok) {
          const data = await res.json();
          set({ config: data.config || cfg, loading: false });
        } else {
          const data = await res.json();
          set({ error: data.error || 'Failed to save config', loading: false });
        }
      } catch (err: any) {
        set({ error: err.message, loading: false });
      }
    },

    fetchStatus: async () => {
      try {
        const res = await fetch('/api/v2ray/bonding/status', { headers: apiHeaders() });
        if (res.ok) {
          const status = await res.json();
          set({ status });
        }
      } catch (err: any) {
        set({ error: err.message });
      }
    },

    startEngine: async () => {
      set({ loading: true, error: null });
      try {
        const res = await fetch('/api/v2ray/bonding/start', {
          method: 'POST',
          headers: apiHeaders(),
        });
        if (res.ok) {
          set({ loading: false });
          await get().fetchStatus();
          get().connectTelemetry();
        } else {
          const data = await res.json();
          set({ error: data.error || 'Failed to start engine', loading: false });
        }
      } catch (err: any) {
        set({ error: err.message, loading: false });
      }
    },

    stopEngine: async () => {
      set({ loading: true, error: null });
      try {
        const res = await fetch('/api/v2ray/bonding/stop', {
          method: 'POST',
          headers: apiHeaders(),
        });
        if (res.ok) {
          set({ loading: false, status: null, arteryHistory: {} });
        } else {
          const data = await res.json();
          set({ error: data.error || 'Failed to stop engine', loading: false });
        }
      } catch (err: any) {
        set({ error: err.message, loading: false });
      }
    },

    connectTelemetry: () => {
      if (ws) {
        ws.close();
        ws = null;
      }

      const protocol = window.location.protocol === 'https:' ? 'wss:' : 'ws:';
      const host = window.location.host;
      const token = getToken();

      ws = new WebSocket(`${protocol}//${host}/ws/v2ray/bonding/telemetry?token=${token}`);

      ws.onopen = () => {
        set({ wsConnected: true });
      };

      ws.onmessage = (event) => {
        try {
          const data: EngineStatus = JSON.parse(event.data);
          const now = new Date().toLocaleTimeString('en-US', {
            hour12: false, hour: '2-digit', minute: '2-digit', second: '2-digit',
          });

          set((state) => {
            const newHistory = { ...state.arteryHistory };
            if (data.arteries) {
              for (const a of data.arteries) {
                if (!newHistory[a.tag]) newHistory[a.tag] = [];
                newHistory[a.tag] = [
                  ...newHistory[a.tag],
                  { time: now, srtt: a.srtt_ms, loss: a.loss_pct },
                ].slice(-60);
              }
            }
            return { status: data, arteryHistory: newHistory };
          });
        } catch {}
      };

      ws.onclose = () => {
        set({ wsConnected: false });
      };

      return () => {
        if (ws) {
          ws.close();
          ws = null;
        }
      };
    },

    runDiagnostics: async () => {
      set({ diagnoseLoading: true, error: null });
      try {
        const res = await fetch('/api/v2ray/bonding/diagnose', { headers: apiHeaders() });
        if (res.ok) {
          const steps = await res.json();
          set({ diagnoseResults: steps, diagnoseLoading: false });
        } else {
          const data = await res.json();
          set({ error: data.error || 'Failed to run diagnostics', diagnoseLoading: false });
        }
      } catch (err: any) {
        set({ error: err.message, diagnoseLoading: false });
      }
    },
  };
});
