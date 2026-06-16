import { create } from 'zustand';

export interface CloudflareAccount {
  id: number;
  CreatedAt?: string;
  UpdatedAt?: string;
  account_name: string;
  account_id: string;
  access_token: string;
  refresh_token: string;
  token_expiry: string;
  email?: string;
  status: string; // "active" | "error"
}

export interface CloudflareStats {
  total_zones: number;
  active_zones: number;
  pending_zones: number;
  worker_scripts: number;
  total_bandwidth: number;
  cached_bandwidth: number;
  total_requests: number;
  cached_requests: number;
}

export interface CloudflareZone {
  id: string;
  name: string;
  status: string;
}

export interface CloudflareWorkerDeployment {
  id: number;
  CreatedAt?: string;
  UpdatedAt?: string;
  account_id: string;
  script_name: string;
  local_path: string;
  default_url: string;
  custom_domain: string;
  zone_id: string;
  health_status: string; // "healthy" | "unhealthy" | "error" | "unknown"
  message?: string;
}

interface CloudflareStore {
  accounts: CloudflareAccount[];
  stats: Record<number, CloudflareStats>;
  zones: CloudflareZone[];
  deployments: CloudflareWorkerDeployment[];
  isLoading: boolean;
  isDeploying: boolean;
  error: string | null;
  
  fetchAccounts: () => Promise<void>;
  updateAccount: (id: number, name: string) => Promise<void>;
  deleteAccount: (id: number) => Promise<void>;
  fetchStats: (id: number) => Promise<void>;
  fetchZones: (accountId: string) => Promise<void>;
  fetchDeployments: () => Promise<void>;
  deployWorker: (payload: { account_id: string; script_name: string; custom_domain?: string; zone_id?: string }) => Promise<void>;
  deleteDeployment: (id: number) => Promise<void>;
  checkDeploymentHealth: (id: number) => Promise<void>;
  addManualAccount: (payload: { account_name: string; auth_type: 'token' | 'key'; token: string; email?: string }) => Promise<void>;
  clearError: () => void;
}

const getToken = () => localStorage.getItem('cc_server_token') || localStorage.getItem('cc_client_token') || '';

export const useCloudflareStore = create<CloudflareStore>((set, get) => ({
  accounts: [],
  stats: {},
  zones: [],
  deployments: [],
  isLoading: false,
  isDeploying: false,
  error: null,

  clearError: () => set({ error: null }),

  fetchAccounts: async () => {
    set({ isLoading: true, error: null });
    try {
      const response = await fetch('/api/cloudflare/accounts', {
        headers: { 'Authorization': `Bearer ${getToken()}` },
      });
      if (response.ok) {
        const data = await response.json();
        set({ accounts: data || [] });
      } else {
        const err = await response.json();
        set({ error: err.error || 'Failed to fetch Cloudflare accounts' });
      }
    } catch {
      set({ error: 'Network error while fetching accounts' });
    } finally {
      set({ isLoading: false });
    }
  },

  updateAccount: async (id, name) => {
    set({ isLoading: true, error: null });
    try {
      const response = await fetch(`/api/cloudflare/accounts/${id}`, {
        method: 'PUT',
        headers: {
          'Content-Type': 'application/json',
          'Authorization': `Bearer ${getToken()}`,
        },
        body: JSON.stringify({ account_name: name }),
      });
      if (response.ok) {
        await get().fetchAccounts();
      } else {
        const err = await response.json();
        set({ error: err.error || 'Failed to update Cloudflare account' });
        throw new Error(err.error || 'Failed to update Cloudflare account');
      }
    } catch (e: any) {
      if (!get().error) {
        set({ error: e.message || 'Network error while updating account' });
      }
      throw e;
    } finally {
      set({ isLoading: false });
    }
  },

  deleteAccount: async (id) => {
    set({ isLoading: true, error: null });
    try {
      const response = await fetch(`/api/cloudflare/accounts/${id}`, {
        method: 'DELETE',
        headers: { 'Authorization': `Bearer ${getToken()}` },
      });
      if (response.ok) {
        set((state) => {
          const nextStats = { ...state.stats };
          delete nextStats[id];
          return {
            accounts: state.accounts.filter(a => a.id !== id),
            stats: nextStats,
          };
        });
      } else {
        const err = await response.json();
        set({ error: err.error || 'Failed to delete account' });
      }
    } catch {
      set({ error: 'Network error while deleting account' });
    } finally {
      set({ isLoading: false });
    }
  },

  fetchStats: async (id) => {
    try {
      const response = await fetch(`/api/cloudflare/accounts/${id}/stats`, {
        headers: { 'Authorization': `Bearer ${getToken()}` },
      });
      if (response.ok) {
        const data = await response.json();
        set((state) => ({
          stats: {
            ...state.stats,
            [id]: data,
          },
        }));
      } else {
        // Mark account state as error locally if API returns Bad Gateway/Unauthorized
        set((state) => ({
          accounts: state.accounts.map(acc => acc.id === id ? { ...acc, status: 'error' } : acc),
        }));
      }
    } catch {
      // network/fetch failed
    }
  },

  fetchZones: async (accountId: string) => {
    set({ isLoading: true, error: null });
    try {
      const response = await fetch(`/api/cloudflare/zones?account_id=${accountId}`, {
        headers: { 'Authorization': `Bearer ${getToken()}` },
      });
      if (response.ok) {
        const data = await response.json();
        set({ zones: data || [] });
      } else {
        const err = await response.json();
        set({ error: err.error || 'Failed to fetch zones' });
      }
    } catch {
      set({ error: 'Network error while fetching zones' });
    } finally {
      set({ isLoading: false });
    }
  },

  fetchDeployments: async () => {
    set({ isLoading: true, error: null });
    try {
      const response = await fetch('/api/cloudflare/workers/deployments', {
        headers: { 'Authorization': `Bearer ${getToken()}` },
      });
      if (response.ok) {
        const data = await response.json();
        set({ deployments: data || [] });
      } else {
        const err = await response.json();
        set({ error: err.error || 'Failed to fetch deployments' });
      }
    } catch {
      set({ error: 'Network error while fetching deployments' });
    } finally {
      set({ isLoading: false });
    }
  },

  deployWorker: async (payload) => {
    set({ isDeploying: true, error: null });
    try {
      const response = await fetch('/api/cloudflare/workers/deploy', {
        method: 'POST',
        headers: {
          'Content-Type': 'application/json',
          'Authorization': `Bearer ${getToken()}`,
        },
        body: JSON.stringify(payload),
      });
      if (response.ok) {
        const newDeployment = await response.json();
        set((state) => ({
          deployments: [newDeployment, ...state.deployments]
        }));
      } else {
        const err = await response.json();
        set({ error: err.error || 'Failed to deploy worker' });
        throw new Error(err.error || 'Failed to deploy worker');
      }
    } catch (e: any) {
      if (!get().error) {
        set({ error: e.message || 'Network error during worker deployment' });
      }
      throw e;
    } finally {
      set({ isDeploying: false });
    }
  },

  deleteDeployment: async (id) => {
    set({ isLoading: true, error: null });
    try {
      const response = await fetch(`/api/cloudflare/workers/deployments/${id}`, {
        method: 'DELETE',
        headers: { 'Authorization': `Bearer ${getToken()}` },
      });
      if (response.ok) {
        set((state) => ({
          deployments: state.deployments.filter(d => d.id !== id),
        }));
      } else {
        const err = await response.json();
        set({ error: err.error || 'Failed to delete deployment' });
      }
    } catch {
      set({ error: 'Network error while deleting deployment' });
    } finally {
      set({ isLoading: false });
    }
  },

  checkDeploymentHealth: async (id) => {
    try {
      const response = await fetch(`/api/cloudflare/workers/deployments/${id}/check-health`, {
        method: 'POST',
        headers: { 'Authorization': `Bearer ${getToken()}` },
      });
      if (response.ok) {
        const updated = await response.json();
        set((state) => ({
          deployments: state.deployments.map(d => d.id === id ? updated : d),
        }));
      }
    } catch {
      // ignore network errors
    }
  },

  addManualAccount: async (payload) => {
    set({ isLoading: true, error: null });
    try {
      const response = await fetch('/api/cloudflare/accounts', {
        method: 'POST',
        headers: {
          'Content-Type': 'application/json',
          'Authorization': `Bearer ${getToken()}`,
        },
        body: JSON.stringify(payload),
      });
      if (response.ok) {
        await get().fetchAccounts();
      } else {
        const err = await response.json();
        set({ error: err.error || 'Failed to add Cloudflare account' });
        throw new Error(err.error || 'Failed to add Cloudflare account');
      }
    } catch (e: any) {
      if (!get().error) {
        set({ error: e.message || 'Network error while adding manual account' });
      }
      throw e;
    } finally {
      set({ isLoading: false });
    }
  },
}));
