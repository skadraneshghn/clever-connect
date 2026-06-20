import { create } from 'zustand';

// ─────────────────────────────────────────────────────────────────────────────
// Type Definitions
// ─────────────────────────────────────────────────────────────────────────────

export type TransportMode = 'masque' | 'masque_h2' | 'wireguard';

export type TunnelStatus =
  | 'dormant'     // engine stopped
  | 'sweeping'    // endpoint scan in progress
  | 'linking'     // engine starting / handshaking
  | 'active'      // running + trace validated
  | 'restricted'; // running but trace failed

export interface WarpGlobalConfig {
  transport_mode: TransportMode;
  target_sni: string;
  socks_port: number;
  http_port: number;
  is_active: boolean;
  last_trace_ok: boolean;
  updated_at?: string;
  active_account?: WarpAccountSummary;
}

export interface WarpAccountSummary {
  id: number;
  account_type: string;
  total_quota: number;
  used_quota: number;
  is_functional: boolean;
  device_id: string;
}

export interface WarpAccount {
  ID: number;
  CreatedAt?: string;
  UpdatedAt?: string;
  license_key?: string;
  device_id: string;
  token: string;
  private_key: string;
  public_key: string;
  client_id: string;
  account_type: string;
  total_quota: number;
  used_quota: number;
  is_functional: boolean;
}

export interface WarpScanResult {
  ip_address: string;
  port: number;
  latency_ms: number;
  packet_loss: number;
  throughput_bps: number;
  supported_alpns: string[];
  last_scanned: string;
  is_restricted: boolean;
  fail_count: number;    // connection failures since last scan (penalises score)
  last_failed: string;   // RFC3339 timestamp of last failure
  score: number;         // computed ranking score (higher = better)
}

export interface ScanProgress {
  is_running: boolean;
  total_targets: number;
  scanned: number;
  passed: number;
  failed: number;
  progress: number;
  udp_blocked?: boolean; // true when ISP appears to block UDP/QUIC
}

export interface ScanEvent {
  index: number;
  time: string;
  ip: string;
  port: number;
  status: 'pass' | 'tcp_ok' | 'fail';
  latency_ms?: number;
  note?: string;
}

export interface EngineStatus {
  state: string;
  transport_mode: string;
  socks_port: number;
  http_port: number;
  active_endpoint: string;
  account_type: string;
  last_trace_ok: boolean;
  uptime: string;
  tcp_fallback?: boolean;
}

export interface NetworkMetricPoint {
  time: string;
  latencyMs: number;
}

// ─────────────────────────────────────────────────────────────────────────────
// Store Interface
// ─────────────────────────────────────────────────────────────────────────────

interface WarpStore {
  // ── Config State ──
  config: WarpGlobalConfig | null;
  isConfigDirty: boolean;

  // ── Account Fleet Pool ──
  accounts: WarpAccount[];
  activeAccountId: number;

  // ── Engine Telemetry ──
  engineStatus: EngineStatus | null;
  tunnelStatus: TunnelStatus;
  scanResults: WarpScanResult[];
  scanProgress: ScanProgress | null;
  scanEvents: ScanEvent[];
  scanEventCursor: number;

  // ── Network Metrics History (sliding window, 30 entries) ──
  networkMetrics: NetworkMetricPoint[];

  // ── UI State ──
  globalLoading: boolean;
  scanLoading: boolean;
  error: string | null;
  licenseError: string | null;

  // ── Actions ──
  fetchGlobalConfig: () => Promise<void>;
  commitEngineTuning: (patch: Partial<WarpGlobalConfig>) => Promise<void>;
  fetchAccounts: () => Promise<void>;
  provisionNewLicense: (licenseKey: string) => Promise<void>;
  deleteAccount: (id: number) => Promise<void>;
  toggleTunnelLifecycle: () => Promise<void>;
  activateAccount: (id: number) => Promise<void>;
  initiateManualEdgeScan: (workers?: number, timeoutMs?: number) => Promise<void>;
  stopEdgeScan: () => Promise<void>;
  fetchScanEvents: () => Promise<void>;
  fetchScanResults: (mode?: string) => Promise<void>;
  fetchStatus: () => Promise<void>;
  clearError: () => void;
}

// ─────────────────────────────────────────────────────────────────────────────
// Helpers
// ─────────────────────────────────────────────────────────────────────────────

const getToken = () =>
  localStorage.getItem('cc_client_token') || localStorage.getItem('cc_server_token') || '';

const apiHeaders = () => ({
  Authorization: `Bearer ${getToken()}`,
  'Content-Type': 'application/json',
});

function deriveTunnelStatus(
  engineStatus: EngineStatus | null,
  scanProgress: ScanProgress | null
): TunnelStatus {
  if (scanProgress?.is_running) return 'sweeping';
  if (!engineStatus || engineStatus.state === 'stopped') return 'dormant';
  if (engineStatus.state === 'starting') return 'linking';
  if (engineStatus.state === 'running') {
    return engineStatus.last_trace_ok ? 'active' : 'restricted';
  }
  return 'dormant';
}

// ─────────────────────────────────────────────────────────────────────────────
// Store
// ─────────────────────────────────────────────────────────────────────────────

export const useWarpStore = create<WarpStore>((set, get) => ({
  config: null,
  isConfigDirty: false,
  accounts: [],
  activeAccountId: 0,
  engineStatus: null,
  tunnelStatus: 'dormant',
  scanResults: [],
  scanProgress: null,
  scanEvents: [],
  scanEventCursor: 0,
  networkMetrics: [],
  globalLoading: false,
  scanLoading: false,
  error: null,
  licenseError: null,

  clearError: () => set({ error: null, licenseError: null }),

  fetchGlobalConfig: async () => {
    set({ globalLoading: true, error: null });
    try {
      const res = await fetch('/api/v2ray/warp/config', { headers: apiHeaders() });
      if (res.ok) {
        const data: WarpGlobalConfig = await res.json();
        set({
          config: data,
          activeAccountId: data.active_account?.id || 0,
          isConfigDirty: false,
        });
      } else {
        const err = await res.json();
        set({ error: err.error || 'Failed to fetch WARP configuration' });
      }
    } catch {
      set({ error: 'Network error while fetching WARP configuration' });
    } finally {
      set({ globalLoading: false });
    }
  },

  commitEngineTuning: async (patch) => {
    set({ globalLoading: true, error: null });
    try {
      const res = await fetch('/api/v2ray/warp/config', {
        method: 'POST',
        headers: apiHeaders(),
        body: JSON.stringify(patch),
      });
      if (res.ok) {
        set({ isConfigDirty: false });
        await get().fetchGlobalConfig();
      } else {
        const err = await res.json();
        set({ error: err.error || 'Failed to save WARP configuration' });
      }
    } catch {
      set({ error: 'Network error while saving configuration' });
    } finally {
      set({ globalLoading: false });
    }
  },

  fetchAccounts: async () => {
    try {
      const res = await fetch('/api/v2ray/warp/accounts', { headers: apiHeaders() });
      if (res.ok) {
        const data: WarpAccount[] = await res.json();
        set({ accounts: data || [] });
      }
    } catch {
      // silent
    }
  },

  provisionNewLicense: async (licenseKey) => {
    set({ globalLoading: true, licenseError: null });
    try {
      const res = await fetch('/api/v2ray/warp/accounts', {
        method: 'POST',
        headers: apiHeaders(),
        body: JSON.stringify({ license_key: licenseKey || '' }),
      });
      if (res.ok) {
        await get().fetchAccounts();
        await get().fetchGlobalConfig();
      } else {
        const err = await res.json();
        set({ licenseError: err.error || 'Cloudflare API rejected registration' });
        throw new Error(err.error);
      }
    } catch (e: any) {
      if (!get().licenseError) set({ licenseError: e.message });
      throw e;
    } finally {
      set({ globalLoading: false });
    }
  },

  deleteAccount: async (id) => {
    set({ globalLoading: true, error: null });
    try {
      const res = await fetch(`/api/v2ray/warp/accounts/${id}`, {
        method: 'DELETE',
        headers: apiHeaders(),
      });
      if (res.ok) {
        set((state) => ({ accounts: state.accounts.filter((a) => a.ID !== id) }));
        await get().fetchGlobalConfig();
      } else {
        const err = await res.json();
        set({ error: err.error || 'Failed to delete account' });
      }
    } catch {
      set({ error: 'Network error while deleting account' });
    } finally {
      set({ globalLoading: false });
    }
  },

  activateAccount: async (id) => {
    set({ globalLoading: true, error: null });
    try {
      const res = await fetch(`/api/v2ray/warp/accounts/${id}/activate`, {
        method: 'POST',
        headers: apiHeaders(),
      });
      if (res.ok) {
        await get().fetchGlobalConfig();
        await get().fetchAccounts();
      } else {
        const err = await res.json();
        set({ error: err.error || 'Failed to activate account' });
      }
    } catch {
      set({ error: 'Network error while activating account' });
    } finally {
      set({ globalLoading: false });
    }
  },

  toggleTunnelLifecycle: async () => {
    const { engineStatus } = get();
    const isRunning = engineStatus?.state === 'running';
    const isStarting = engineStatus?.state === 'starting';
    const endpoint = (isRunning || isStarting) ? '/api/v2ray/warp/tunnel/stop' : '/api/v2ray/warp/tunnel/start';

    set({ globalLoading: true, error: null, tunnelStatus: (isRunning || isStarting) ? 'dormant' : 'linking' });
    try {
      const res = await fetch(endpoint, {
        method: 'POST',
        headers: apiHeaders(),
      });
      if (res.ok) {
        await get().fetchStatus();
        await get().fetchGlobalConfig();
      } else {
        const err = await res.json();
        // Reset to dormant so the button doesn't get stuck in "Processing..."
        set({ error: err.error || 'Failed to toggle tunnel lifecycle', tunnelStatus: 'dormant' });
      }
    } catch {
      set({ error: 'Network error during tunnel toggle', tunnelStatus: 'dormant' });
    } finally {
      set({ globalLoading: false });
    }
  },

  initiateManualEdgeScan: async (workers?: number, timeoutMs?: number) => {
    set({ scanLoading: true, error: null, scanResults: [], scanProgress: null, scanEvents: [], scanEventCursor: 0 });
    try {
      const body: Record<string, number> = {};
      if (workers && workers > 0) body.workers = workers;
      if (timeoutMs && timeoutMs > 0) body.timeout_ms = timeoutMs;

      const res = await fetch('/api/v2ray/warp/scan', {
        method: 'POST',
        headers: apiHeaders(),
        body: JSON.stringify(body),
      });
      if (res.ok) {
        set((state) => ({
          tunnelStatus: state.tunnelStatus === 'dormant' ? 'sweeping' : state.tunnelStatus,
        }));
        await get().fetchScanResults();
      } else {
        const err = await res.json();
        set({ error: err.error || 'Failed to initiate edge scan' });
      }
    } catch {
      set({ error: 'Network error during edge scan' });
    } finally {
      set({ scanLoading: false });
    }
  },

  stopEdgeScan: async () => {
    try {
      await fetch('/api/v2ray/warp/scan/stop', {
        method: 'POST',
        headers: apiHeaders(),
      });
      set((state) => ({
        scanProgress: state.scanProgress ? { ...state.scanProgress, is_running: false } : null,
        tunnelStatus: state.tunnelStatus === 'sweeping' ? 'dormant' : state.tunnelStatus,
      }));
    } catch { /* silent */ }
  },

  fetchScanEvents: async () => {
    const cursor = useWarpStore.getState().scanEventCursor;
    try {
      const res = await fetch(`/api/v2ray/warp/scan/events?since=${cursor}`, {
        headers: apiHeaders(),
      });
      if (!res.ok) return;
      const data = await res.json();
      if (data.events && data.events.length > 0) {
        set((state) => ({
          scanEvents: [...state.scanEvents, ...data.events].slice(-500), // keep last 500
          scanEventCursor: data.last_index,
        }));
      }
    } catch { /* silent */ }
  },

  fetchScanResults: async (mode = 'masque') => {
    try {
      const res = await fetch(`/api/v2ray/warp/scan/results?mode=${mode}`, {
        headers: apiHeaders(),
      });
      if (res.ok) {
        const data = await res.json();
        const progress: ScanProgress | null = data.progress || null;
        set({
          scanResults: data.results || [],
          scanProgress: progress,
          tunnelStatus: deriveTunnelStatus(get().engineStatus, progress),
        });
      }
    } catch {
      // silent
    }
  },

  fetchStatus: async () => {
    try {
      const res = await fetch('/api/v2ray/warp/status', { headers: apiHeaders() });
      if (res.ok) {
        const data = await res.json();
        const engineStatus: EngineStatus = data.engine;
        const scanProgress: ScanProgress | null = data.scan_progress || null;
        set({
          engineStatus,
          scanProgress,
          tunnelStatus: deriveTunnelStatus(engineStatus, scanProgress),
        });
      }
    } catch {
      // silent
    }
  },
}));
