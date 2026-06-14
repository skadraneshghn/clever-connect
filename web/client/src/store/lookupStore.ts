import { create } from 'zustand';

export interface IPGeoInfo {
  ip: string;
  country: string;
  country_code: string;
  city: string;
  asn: string;
  isp: string;
  latitude: number;
  longitude: number;
  proxy_status: string; // VPN/Tor/DCH/Clean
  is_proxy: boolean;
  provider: string;
  raw_json: string;
  updated_at: string;
}

export interface DomainWhoisInfo {
  domain_name: string;
  registrar: string;
  creation_date: string;
  expiry_date: string;
  name_servers: string[];
  raw_json: string;
  updated_at: string;
}

export interface LookupResponse {
  target: string;
  type: 'ip' | 'domain';
  resolved_ip?: string;
  geo?: IPGeoInfo;
  whois?: DomainWhoisInfo;
  source: 'cache' | 'api';
  quota_error: boolean;
  error_msg?: string;
}

export interface ApiKeysConfig {
  ID?: number;
  ip2location_key: string;
  ip_api_key: string;
  ip_geolocation_key: string;
  ip_whois_key: string;
  find_ip_key: string;
  enable_ip2location: boolean;
  enable_ip_api: boolean;
  enable_ip_geolocation: boolean;
  enable_ip_whois: boolean;
  enable_find_ip: boolean;
}

interface LookupStore {
  activeTarget: string;
  lookupResult: LookupResponse | null;
  isLoading: boolean;
  errorAlert: string | null;
  apiConfig: ApiKeysConfig | null;

  // Bulk scan states
  bulkProgress: { resolved: number; total: number };
  bulkResults: IPGeoInfo[];
  isBulkLoading: boolean;
  bulkSocket: WebSocket | null;

  fetchApiConfig: () => Promise<void>;
  saveApiConfig: (cfg: ApiKeysConfig) => Promise<boolean>;
  testConnection: (service: string, key: string) => Promise<{ valid: boolean; error?: string }>;
  performLookup: (target: string, type?: string) => Promise<void>;
  startBulkLookup: (ips: string[]) => void;
  stopBulkLookup: () => void;
  resetLookup: () => void;
}

export const useLookupStore = create<LookupStore>((set, get) => ({
  activeTarget: '',
  lookupResult: null,
  isLoading: false,
  errorAlert: null,
  apiConfig: null,

  bulkProgress: { resolved: 0, total: 0 },
  bulkResults: [],
  isBulkLoading: false,
  bulkSocket: null,

  fetchApiConfig: async () => {
    const token = localStorage.getItem('cc_client_token') || '';
    try {
      const res = await fetch('/api/settings/apikeys', {
        headers: {
          'Authorization': `Bearer ${token}`,
        },
      });
      if (res.ok) {
        const data = await res.json();
        set({ apiConfig: data });
      }
    } catch (err) {
      console.error('Failed to fetch api keys configuration', err);
    }
  },

  saveApiConfig: async (cfg: ApiKeysConfig) => {
    const token = localStorage.getItem('cc_client_token') || '';
    try {
      const res = await fetch('/api/settings/apikeys', {
        method: 'POST',
        headers: {
          'Content-Type': 'application/json',
          'Authorization': `Bearer ${token}`,
        },
        body: JSON.stringify(cfg),
      });
      if (res.ok) {
        set({ apiConfig: cfg });
        return true;
      }
    } catch (err) {
      console.error('Failed to save api keys config', err);
    }
    return false;
  },

  testConnection: async (service: string, key: string) => {
    const token = localStorage.getItem('cc_client_token') || '';
    try {
      const res = await fetch('/api/settings/test-key', {
        method: 'POST',
        headers: {
          'Content-Type': 'application/json',
          'Authorization': `Bearer ${token}`,
        },
        body: JSON.stringify({ service, key }),
      });
      if (res.ok) {
        return await res.json();
      }
    } catch (err: any) {
      return { valid: false, error: err.message };
    }
    return { valid: false, error: 'Network request failed' };
  },

  performLookup: async (target: string, type: string = 'auto') => {
    set({ isLoading: true, errorAlert: null, lookupResult: null, activeTarget: target });
    const token = localStorage.getItem('cc_client_token') || '';
    try {
      const res = await fetch('/api/network/lookup', {
        method: 'POST',
        headers: {
          'Content-Type': 'application/json',
          'Authorization': `Bearer ${token}`,
        },
        body: JSON.stringify({ target, type }),
      });
      
      if (res.ok) {
        const data: LookupResponse = await res.json();
        if (data.quota_error) {
          set({
            errorAlert: 'Quota limit exceeded or no API key set. Please configure settings.',
            lookupResult: data,
          });
        } else if (data.error_msg) {
          set({ errorAlert: data.error_msg, lookupResult: data });
        } else {
          set({ lookupResult: data });
        }
      } else {
        const errData = await res.json().catch(() => ({}));
        set({ errorAlert: errData.error || 'Failed to scan target. Check connection or settings.' });
      }
    } catch (err: any) {
      set({ errorAlert: err.message || 'An unexpected error occurred.' });
    } finally {
      set({ isLoading: false });
    }
  },

  startBulkLookup: (ips: string[]) => {
    const { bulkSocket } = get();
    if (bulkSocket) {
      bulkSocket.close();
    }

    const token = localStorage.getItem('cc_client_token') || '';
    const protocol = window.location.protocol === 'https:' ? 'wss:' : 'ws:';
    const wsUrl = `${protocol}//${window.location.host}/ws?token=${token}`;

    set({
      isBulkLoading: true,
      bulkProgress: { resolved: 0, total: ips.length },
      bulkResults: [],
      errorAlert: null,
    });

    const socket = new WebSocket(wsUrl);

    socket.onopen = () => {
      // Send bulk lookup command once connection is open
      const payload = {
        type: 'bulk_lookup',
        data: { ips },
      };
      socket.send(JSON.stringify(payload));
    };

    socket.onmessage = (event) => {
      try {
        const msg = JSON.parse(event.data);
        if (msg.type === 'BULK_PROGRESS' && msg.event === 'BULK_PROGRESS') {
          const newBatch: IPGeoInfo[] = msg.data || [];
          set((state) => ({
            bulkProgress: { resolved: msg.resolved, total: msg.total },
            bulkResults: [...state.bulkResults, ...newBatch],
            isBulkLoading: msg.resolved < msg.total,
          }));
        }
      } catch (err) {
        console.error('Error parsing bulk socket message', err);
      }
    };

    socket.onerror = (err) => {
      console.error('Bulk WS error', err);
      set({ errorAlert: 'WebSocket connection failed.', isBulkLoading: false });
    };

    socket.onclose = () => {
      set({ isBulkLoading: false, bulkSocket: null });
    };

    set({ bulkSocket: socket });
  },

  stopBulkLookup: () => {
    const { bulkSocket } = get();
    if (bulkSocket) {
      bulkSocket.close();
    }
    set({ isBulkLoading: false, bulkSocket: null });
  },

  resetLookup: () => {
    set({
      activeTarget: '',
      lookupResult: null,
      isLoading: false,
      errorAlert: null,
      bulkProgress: { resolved: 0, total: 0 },
      bulkResults: [],
      isBulkLoading: false,
    });
  },
}));
