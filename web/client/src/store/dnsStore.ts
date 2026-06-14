import { create } from 'zustand';

export interface DNSResolver {
  id: number;
  ip: string;
  protocol: string;
  provider_name: string;
  category: string;
  support_udp: boolean;
  support_tcp: boolean;
  support_dot: boolean;
  support_doh: boolean;
  support_doq: boolean;
  is_custom: boolean;
  
  // Real-time test telemetry fields
  latency_ms: number;
  jitter_ms: number;
  success_rate: number;
  packet_loss: number;
  censored: boolean;
  nxdomain_hijacked: boolean;
  dnssec_valid: boolean;
  dns_rebinding_vuln?: boolean;
  query_type?: string;
  dns_class?: string;
  domain?: string;
  clever_score: number;
  completed_at: string;
  error_message: string;
  is_testing: boolean;

  // Resolved domain IP geodata & expectation matching
  resolved_ip?: string;
  country_code?: string;
  country_name?: string;
  city?: string;
  isp?: string;
  is_cdn?: boolean;
  cdn_provider?: string;
  expected_match?: boolean;
  asn?: string;
  censorship_status?: string;
}

export interface DNSJobStats {
  total_resolvers: number;
  processed_resolvers: number;
  successful_resolvers: number;
  failed_resolvers: number;
  in_flight_resolvers: number;
  is_active: boolean;
  elapsed_ms: number;
}

export interface DNSBulkProgress {
  total: number;
  processed: number;
  added: number;
  duplicates: number;
  active: boolean;
}

interface DNSStore {
  resolvers: Record<string, DNSResolver>;
  resolverKeys: string[];
  jobStats: DNSJobStats | null;
  isSweeping: boolean;
  appliedResolver: string;
  isLoading: boolean;
  bulkProgress: DNSBulkProgress | null;

  setLoading: (loading: boolean) => void;
  setResolvers: (list: any[]) => void;
  updateResolver: (key: string, data: Partial<DNSResolver>) => void;
  setJobStats: (stats: DNSJobStats | null) => void;
  setSweeping: (sweeping: boolean) => void;
  setAppliedResolver: (key: string) => void;
  clearResults: () => void;
  setBulkProgress: (progress: DNSBulkProgress | null) => void;
}

// Generate unique composite key
export const getResolverKey = (ip: string, protocol: string) => `${ip}:${protocol}`;

export const useDNSStore = create<DNSStore>((set) => ({
  resolvers: {},
  resolverKeys: [],
  jobStats: null,
  isSweeping: false,
  appliedResolver: '',
  isLoading: false,
  bulkProgress: null,

  setLoading: (loading) => set({ isLoading: loading }),

  setResolvers: (list) => {
    const map: Record<string, DNSResolver> = {};
    const keys: string[] = [];
    list.forEach((r: any) => {
      const k = getResolverKey(r.ip, r.protocol);
      map[k] = {
        ...r,
        latency_ms: r.latency_ms || 0,
        jitter_ms: r.jitter_ms || 0,
        success_rate: r.success_rate_pct !== undefined ? r.success_rate_pct / 100 : (r.success_rate || 0),
        packet_loss: r.packet_loss_pct !== undefined ? r.packet_loss_pct : (r.packet_loss || 0),
        clever_score: r.clever_score || 0,
        censored: r.censorship_status === 'manipulated' || r.censorship_status === 'sinkhole',
        nxdomain_hijacked: r.censorship_status === 'hijacked',
        dnssec_valid: r.dnssec_override === false,
        dns_rebinding_vuln: r.dns_rebinding_vuln || false,
        resolved_ip: r.resolved_ip,
        country_code: r.country_code,
        country_name: r.country_name,
        city: r.city,
        isp: r.isp || r.ISP,
        is_cdn: r.is_cdn,
        cdn_provider: r.cdn_provider,
        expected_match: r.expected_match !== false, // defaults to true
        asn: r.asn || r.ASN,
        censorship_status: r.censorship_status,
      };
      keys.push(k);
    });
    set({ resolvers: map, resolverKeys: keys });
  },

  updateResolver: (key, data) => {
    set((state) => {
      const resolver = state.resolvers[key];
      if (!resolver) {
        // If not present (e.g. swept resolver not in DB), dynamically add it
        const [ip, protocol] = key.split(':');
        const newResolver: DNSResolver = {
          id: 0,
          ip: ip || '',
          protocol: protocol || 'udp',
          provider_name: data.provider_name || 'Dynamic Resolver',
          category: data.category || 'public',
          support_udp: protocol === 'udp',
          support_tcp: protocol === 'tcp',
          support_dot: protocol === 'dot',
          support_doh: protocol === 'doh',
          support_doq: protocol === 'doq',
          is_custom: false,
          latency_ms: 0,
          jitter_ms: 0,
          success_rate: 0,
          packet_loss: 0,
          censored: false,
          nxdomain_hijacked: false,
          dnssec_valid: false,
          dns_rebinding_vuln: false,
          query_type: '',
          dns_class: '',
          domain: '',
          clever_score: 0,
          completed_at: '',
          error_message: '',
          is_testing: false,
          ...data,
        };
        return {
          resolvers: { ...state.resolvers, [key]: newResolver },
          resolverKeys: state.resolverKeys.includes(key) ? state.resolverKeys : [...state.resolverKeys, key],
        };
      }

      return {
        resolvers: {
          ...state.resolvers,
          [key]: { ...resolver, ...data },
        },
      };
    });
  },

  setJobStats: (stats) => set({ jobStats: stats, isSweeping: stats ? stats.is_active : false }),
  
  setSweeping: (sweeping) => set({ isSweeping: sweeping }),
  
  setAppliedResolver: (resolver) => set({ appliedResolver: resolver }),

  clearResults: () => {
    set((state) => {
      const cleared: Record<string, DNSResolver> = {};
      state.resolverKeys.forEach((k) => {
        cleared[k] = {
          ...state.resolvers[k],
          latency_ms: 0,
          jitter_ms: 0,
          success_rate: 0,
          packet_loss: 0,
          censored: false,
          nxdomain_hijacked: false,
          dnssec_valid: false,
          clever_score: 0,
          completed_at: '',
          error_message: '',
          is_testing: false,
        };
      });
      return { resolvers: cleared, jobStats: null, isSweeping: false };
    });
  },

  setBulkProgress: (progress) => set({ bulkProgress: progress }),
}));
