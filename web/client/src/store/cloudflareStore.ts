import { create } from 'zustand';

export interface CloudflareAccount {
  id: number;
  CreatedAt?: string;
  UpdatedAt?: string;
  account_name: string;
  account_id: string;
  api_token: string;
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

interface CloudflareStore {
  accounts: CloudflareAccount[];
  stats: Record<number, CloudflareStats>;
  isLoading: boolean;
  error: string | null;
  
  fetchAccounts: () => Promise<void>;
  addAccount: (name: string, token: string) => Promise<void>;
  updateAccount: (id: number, name: string, token?: string) => Promise<void>;
  deleteAccount: (id: number) => Promise<void>;
  fetchStats: (id: number) => Promise<void>;
  clearError: () => void;
}

const getToken = () => localStorage.getItem('cc_server_token') || localStorage.getItem('cc_client_token') || '';

export const useCloudflareStore = create<CloudflareStore>((set, get) => ({
  accounts: [],
  stats: {},
  isLoading: false,
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

  addAccount: async (name, token) => {
    set({ isLoading: true, error: null });
    try {
      const response = await fetch('/api/cloudflare/accounts', {
        method: 'POST',
        headers: {
          'Content-Type': 'application/json',
          'Authorization': `Bearer ${getToken()}`,
        },
        body: JSON.stringify({ account_name: name, api_token: token }),
      });
      if (response.ok) {
        await get().fetchAccounts();
      } else {
        const err = await response.json();
        set({ error: err.error || 'Failed to add Cloudflare token' });
        throw new Error(err.error || 'Failed to add Cloudflare token');
      }
    } catch (e: any) {
      if (!get().error) {
        set({ error: e.message || 'Network error while adding account' });
      }
      throw e;
    } finally {
      set({ isLoading: false });
    }
  },

  updateAccount: async (id, name, token) => {
    set({ isLoading: true, error: null });
    try {
      const response = await fetch(`/api/cloudflare/accounts/${id}`, {
        method: 'PUT',
        headers: {
          'Content-Type': 'application/json',
          'Authorization': `Bearer ${getToken()}`,
        },
        body: JSON.stringify({ account_name: name, api_token: token || undefined }),
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
}));
