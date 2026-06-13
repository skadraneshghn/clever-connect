import { create } from 'zustand';

export interface Domain {
  id: string;
  domain_name: string;
  category: string;
  status: 'pending' | 'checking' | 'online' | 'offline' | 'timeout' | 'nxdomain';
  ip_addresses: string;
  http_status: number;
  latency_ms: number;
  tls_status: boolean;
  tls_expiry_days: number;
  last_checked_at: string;
}

interface DomainStore {
  domains: Record<string, Domain>;
  domainIds: string[];
  isLoading: boolean;
  setLoading: (loading: boolean) => void;
  setDomains: (domains: Domain[]) => void;
  appendDomains: (domains: Domain[]) => void;
  updateDomain: (domain: Domain) => void;
  removeDomain: (id: string) => void;
  removeDomainsBulk: (ids: string[]) => void;
  clearDomains: () => void;
}

export const useDomainStore = create<DomainStore>((set) => ({
  domains: {},
  domainIds: [],
  isLoading: false,

  setLoading: (loading) => set({ isLoading: loading }),

  setDomains: (domains) => {
    const domainMap: Record<string, Domain> = {};
    const ids: string[] = [];
    domains.forEach((d) => {
      domainMap[d.id] = d;
      ids.push(d.id);
    });
    set({ domains: domainMap, domainIds: ids });
  },

  appendDomains: (newDomains) => {
    set((state) => {
      const updatedDomains = { ...state.domains };
      const updatedIds = [...state.domainIds];
      newDomains.forEach((d) => {
        if (!updatedDomains[d.id]) {
          updatedIds.push(d.id);
        }
        updatedDomains[d.id] = d;
      });
      return { domains: updatedDomains, domainIds: updatedIds };
    });
  },

  updateDomain: (domain) => {
    set((state) => ({
      domains: {
        ...state.domains,
        [domain.id]: {
          ...state.domains[domain.id],
          ...domain,
        },
      },
    }));
  },

  removeDomain: (id) => {
    set((state) => {
      const nextDomains = { ...state.domains };
      delete nextDomains[id];
      const nextIds = state.domainIds.filter(x => x !== id);
      return { domains: nextDomains, domainIds: nextIds };
    });
  },

  removeDomainsBulk: (ids) => {
    set((state) => {
      const nextDomains = { ...state.domains };
      const idSet = new Set(ids);
      ids.forEach(id => {
        delete nextDomains[id];
      });
      const nextIds = state.domainIds.filter(x => !idSet.has(x));
      return { domains: nextDomains, domainIds: nextIds };
    });
  },

  clearDomains: () => set({ domains: {}, domainIds: [] }),
}));
