import { create } from 'zustand';

export interface ArteryStats {
  artery_id: string;
  last_ping: string;
  last_pong: string;
}

export interface CombinerStatus {
  running: boolean;
  origin_id: string;
  artery_count: number;
  artery_stats: ArteryStats[];
  active_streams: number;
  bytes_rx: number;
  bytes_tx: number;
  rx_bps: number;
  tx_bps: number;
}

export interface CombinerConfig {
  id?: number;
  is_active: boolean;
  mode: string;
  origin_id: string;
  psk_hex: string;
}

export interface DiagnosticStep {
  name: string;
  description: string;
  status: 'success' | 'warning' | 'error' | 'pending';
  error_message?: string;
  details?: string;
}

interface CombinerState {
  status: CombinerStatus | null;
  config: CombinerConfig | null;
  loading: boolean;
  error: string | null;
  diagnoseResults: DiagnosticStep[] | null;
  diagnoseLoading: boolean;

  fetchConfig: () => Promise<void>;
  saveConfig: (cfg: CombinerConfig) => Promise<void>;
  fetchStatus: () => Promise<void>;
  startCombiner: () => Promise<void>;
  stopCombiner: () => Promise<void>;
  runDiagnostics: (cfg?: CombinerConfig) => Promise<void>;
}

const getToken = () => localStorage.getItem('cc_server_token') || '';
const apiHeaders = () => ({
  'Authorization': `Bearer ${getToken()}`,
  'Content-Type': 'application/json',
});

export const useCombinerStore = create<CombinerState>((set, get) => ({
  status: null,
  config: null,
  loading: false,
  error: null,
  diagnoseResults: null,
  diagnoseLoading: false,

  fetchConfig: async () => {
    try {
      const res = await fetch('/api/bonding/combiner/config', { headers: apiHeaders() });
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
      const res = await fetch('/api/bonding/combiner/config', {
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
      const res = await fetch('/api/bonding/combiner/status', { headers: apiHeaders() });
      if (res.ok) {
        const status = await res.json();
        set({ status });
      }
    } catch (err: any) {
      set({ error: err.message });
    }
  },

  startCombiner: async () => {
    set({ loading: true, error: null });
    try {
      const res = await fetch('/api/bonding/combiner/start', {
        method: 'POST',
        headers: apiHeaders(),
      });
      if (res.ok) {
        set({ loading: false });
        await get().fetchStatus();
      } else {
        const data = await res.json();
        set({ error: data.error || 'Failed to start combiner', loading: false });
      }
    } catch (err: any) {
      set({ error: err.message, loading: false });
    }
  },

  stopCombiner: async () => {
    set({ loading: true, error: null });
    try {
      const res = await fetch('/api/bonding/combiner/stop', {
        method: 'POST',
        headers: apiHeaders(),
      });
      if (res.ok) {
        set({ loading: false });
        await get().fetchStatus();
      } else {
        const data = await res.json();
        set({ error: data.error || 'Failed to stop combiner', loading: false });
      }
    } catch (err: any) {
      set({ error: err.message, loading: false });
    }
  },

  runDiagnostics: async (cfg) => {
    set({ diagnoseLoading: true, error: null });
    try {
      const method = cfg ? 'POST' : 'GET';
      const body = cfg ? JSON.stringify(cfg) : undefined;
      const res = await fetch('/api/bonding/combiner/diagnose', {
        method,
        headers: apiHeaders(),
        body,
      });
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
}));
