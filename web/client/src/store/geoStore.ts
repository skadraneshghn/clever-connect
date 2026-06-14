import { create } from 'zustand';

export interface IPGeoInfo {
  ip: string;
  country_code: string;
  country_name: string;
  city: string;
  isp: string;
  cdn_provider: string;
  latitude: number;
  longitude: number;
  is_cdn: boolean;
  last_updated: string;
}

interface GeoStore {
  geoCache: Record<string, IPGeoInfo>;
  pendingResolutions: Record<string, boolean>;
  resolveIPs: (ips: string[], force?: boolean) => Promise<void>;
  updateGeoInfo: (info: IPGeoInfo) => void;
}

export const useGeoStore = create<GeoStore>((set, get) => ({
  geoCache: {},
  pendingResolutions: {},

  updateGeoInfo: (info) => {
    set((state) => ({
      geoCache: {
        ...state.geoCache,
        [info.ip]: info,
      },
    }));
  },

  resolveIPs: async (ips, force = false) => {
    const { geoCache, pendingResolutions } = get();
    const token = localStorage.getItem('cc_client_token') || '';

    // Filter out invalid/empty IPs, and IPs already in cache (unless forced) or currently pending
    const ipsToFetch = ips.filter(
      (ip) => {
        if (!ip) return false;
        const clean = ip.trim();
        if (!clean) return false;
        // Basic check for IP format (contains dot or colon)
        if (!clean.includes('.') && !clean.includes(':')) return false;
        return (force || !geoCache[clean]) && !pendingResolutions[clean];
      }
    ).map(ip => ip.trim());

    if (ipsToFetch.length === 0) return;

    // Mark as pending
    set((state) => {
      const nextPending = { ...state.pendingResolutions };
      ipsToFetch.forEach((ip) => {
        nextPending[ip] = true;
      });
      return { pendingResolutions: nextPending };
    });

    try {
      const response = await fetch('/api/geo/resolve', {
        method: 'POST',
        headers: {
          'Content-Type': 'application/json',
          'Authorization': `Bearer ${token}`,
        },
        body: JSON.stringify({ ips: ipsToFetch, force }),
      });

      if (response.ok) {
        const data = await response.json();
        const results = data.results || [];
        
        set((state) => {
          const nextCache = { ...state.geoCache };
          const nextPending = { ...state.pendingResolutions };

          results.forEach((res: IPGeoInfo) => {
            if (res && res.ip) {
              nextCache[res.ip] = res;
            }
          });

          ipsToFetch.forEach((ip) => {
            delete nextPending[ip];
          });

          return { geoCache: nextCache, pendingResolutions: nextPending };
        });
      } else {
        throw new Error('Failed to resolve IPs on backend');
      }
    } catch (err) {
      console.error('Failed to resolve IPs:', err);
      // Clean pending status on error
      set((state) => {
        const nextPending = { ...state.pendingResolutions };
        ipsToFetch.forEach((ip) => {
          delete nextPending[ip];
        });
        return { pendingResolutions: nextPending };
      });
    }
  },
}));
