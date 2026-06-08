import React, { useState, useEffect } from 'react';
import { FiX, FiSave, FiCode, FiList, FiInfo, FiPlus, FiTrash2, FiSettings, FiActivity, FiGlobe } from 'react-icons/fi';

interface AdvancedCoreConfigModalProps {
  onClose: () => void;
  selectedCore: string;
}

const defaultV2rayConfig = {
  "log": { "access": "", "error": "", "loglevel": "warning" },
  "api": { "tag": "api", "services": ["HandlerService", "LoggerService", "StatsService"] },
  "dns": {
    "hosts": { "localhost": "127.0.0.1" },
    "servers": [ "8.8.8.8", "1.1.1.1", { "address": "223.5.5.5", "port": 53, "domains": ["geosite:cn"] } ],
    "clientIp": "0.0.0.0", "tag": "dns"
  },
  "stats": {},
  "policy": {
    "levels": { "0": { "handshake": 4, "connIdle": 300, "uplinkOnly": 2, "downlinkOnly": 5, "bufferSize": 10240, "statsUserUplink": true, "statsUserDownlink": true } },
    "system": { "statsInboundUplink": true, "statsInboundDownlink": true, "statsOutboundUplink": true, "statsOutboundDownlink": true }
  },
  "routing": {
    "domainStrategy": "IPIfNonMatch", "domainMatcher": "mph",
    "rules": [
      { "type": "field", "inboundTag": ["socks-in"], "outboundTag": "proxy" },
      { "type": "field", "domain": ["geosite:cn"], "outboundTag": "direct" },
      { "type": "field", "ip": ["geoip:private", "geoip:cn"], "outboundTag": "direct" },
      { "type": "field", "protocol": ["bittorrent"], "outboundTag": "block" }
    ],
    "balancers": [ { "tag": "proxy-balancer", "selector": ["proxy"] } ]
  },
  "reverse": { "bridges": [], "portals": [] },
  "inbounds": [
    {
      "tag": "socks-in", "listen": "127.0.0.1", "port": 1080, "protocol": "socks",
      "settings": { "auth": "noauth", "udp": true, "ip": "127.0.0.1" },
      "streamSettings": { "network": "tcp", "security": "none" },
      "sniffing": { "enabled": true, "destOverride": ["http", "tls"] },
      "allocate": { "strategy": "always", "refresh": 5, "concurrency": 3 }
    }
  ],
  "outbounds": [
    {
      "tag": "proxy", "protocol": "vmess",
      "settings": { "vnext": [ { "address": "example.com", "port": 443, "users": [ { "id": "00000000-0000-0000-0000-000000000000", "alterId": 0, "security": "auto", "email": "user@example.com" } ] } ] },
      "sendThrough": "0.0.0.0",
      "streamSettings": { "network": "ws", "security": "tls", "tlsSettings": { "serverName": "example.com", "allowInsecure": false, "alpn": ["http/1.1", "h2"] }, "wsSettings": { "path": "/ws", "headers": { "Host": "example.com" } }, "sockopt": { "mark": 0, "tcpFastOpen": false, "tproxy": "off" } },
      "proxySettings": { "tag": "another-proxy" }, "mux": { "enabled": false, "concurrency": 8 }
    },
    { "tag": "direct", "protocol": "freedom", "settings": {} },
    { "tag": "block", "protocol": "blackhole", "settings": {} }
  ],
  "transport": { "tcpSettings": {}, "kcpSettings": {}, "wsSettings": {}, "httpSettings": {}, "dsSettings": {}, "quicSettings": {} }
};

const defaultXrayConfig = {
  "log": { "access": "", "error": "", "loglevel": "warning", "dnsLog": false },
  "api": { "tag": "api", "services": ["HandlerService", "LoggerService", "StatsService", "RoutingService"] },
  "stats": {}, "metrics": { "tag": "metrics" },
  "fakedns": [ { "ipPool": "198.18.0.0/15", "poolSize": 65535 } ],
  "dns": {
    "tag": "dns", "hosts": { "localhost": "127.0.0.1" },
    "servers": [ "8.8.8.8", { "address": "https://1.1.1.1/dns-query", "domains": ["geosite:geolocation-!cn"], "expectIPs": ["geoip:!cn"], "skipFallback": false }, { "address": "223.5.5.5", "domains": ["geosite:cn"] }, "fakedns" ],
    "queryStrategy": "UseIP", "disableCache": false, "disableFallback": false, "disableFallbackIfMatch": false
  },
  "policy": {
    "levels": { "0": { "handshake": 4, "connIdle": 300, "uplinkOnly": 2, "downlinkOnly": 5, "bufferSize": 10240, "statsUserUplink": true, "statsUserDownlink": true } },
    "system": { "statsInboundUplink": true, "statsInboundDownlink": true, "statsOutboundUplink": true, "statsOutboundDownlink": true }
  },
  "routing": {
    "domainStrategy": "IPIfNonMatch", "domainMatcher": "mph",
    "rules": [
      { "type": "field", "inboundTag": ["socks-in"], "outboundTag": "proxy" },
      { "type": "field", "network": "udp", "port": "443", "outboundTag": "block" },
      { "type": "field", "domain": ["geosite:cn"], "outboundTag": "direct" },
      { "type": "field", "ip": ["geoip:private", "geoip:cn"], "outboundTag": "direct" },
      { "type": "field", "protocol": ["bittorrent"], "outboundTag": "block" }
    ],
    "balancers": [ { "tag": "proxy-balancer", "selector": ["proxy"], "strategy": { "type": "random" } } ]
  },
  "observatory": { "subjectSelector": ["proxy"], "probeUrl": "https://www.google.com/generate_204", "probeInterval": "10s" },
  "burstObservatory": { "subjectSelector": ["proxy"], "pingConfig": { "destination": "https://www.google.com/generate_204" } },
  "reverse": { "bridges": [], "portals": [] },
  "inbounds": [
    {
      "tag": "socks-in", "listen": "127.0.0.1", "port": 1080, "protocol": "socks",
      "settings": { "auth": "noauth", "udp": true, "ip": "127.0.0.1" },
      "sniffing": { "enabled": true, "destOverride": ["http", "tls", "quic", "fakedns"], "routeOnly": false },
      "streamSettings": { "network": "tcp", "security": "none" }
    }
  ],
  "outbounds": [
    {
      "tag": "proxy", "protocol": "vless",
      "settings": { "vnext": [ { "address": "server.example.com", "port": 443, "users": [ { "id": "00000000-0000-0000-0000-000000000000", "encryption": "none", "flow": "xtls-rprx-vision", "level": 0, "email": "user@example.com" } ] } ] },
      "streamSettings": {
        "network": "tcp", "security": "reality",
        "realitySettings": { "serverName": "www.microsoft.com", "fingerprint": "chrome", "publicKey": "PUBLIC_KEY", "shortId": "0123456789abcdef", "spiderX": "/" },
        "tlsSettings": { "serverName": "example.com", "allowInsecure": false, "fingerprint": "chrome", "alpn": ["h2", "http/1.1"] },
        "tcpSettings": {}, "wsSettings": { "path": "/ws", "headers": { "Host": "example.com" } },
        "grpcSettings": { "serviceName": "grpc" }, "httpupgradeSettings": { "host": "example.com", "path": "/upgrade" }, "xhttpSettings": { "host": "example.com", "path": "/xhttp", "mode": "auto" },
        "sockopt": { "tcpFastOpen": true, "tcpNoDelay": true, "mark": 0, "domainStrategy": "UseIP", "dialerProxy": "", "tcpKeepAliveInterval": 15 }
      },
      "mux": { "enabled": false, "concurrency": 8, "xudpConcurrency": 16, "xudpProxyUDP443": "reject" },
      "proxySettings": { "tag": "another-hop", "transportLayer": false }
    },
    { "tag": "wireguard", "protocol": "wireguard", "settings": { "secretKey": "PRIVATE_KEY", "address": ["172.16.0.2/32"], "peers": [ { "publicKey": "PEER_PUBLIC_KEY", "endpoint": "1.2.3.4:51820" } ], "mtu": 1420 } },
    { "tag": "direct", "protocol": "freedom", "settings": {} },
    { "tag": "dns-out", "protocol": "dns", "settings": {} },
    { "tag": "block", "protocol": "blackhole", "settings": {} }
  ]
};

const defaultSingBoxConfig = {
  "log": { "level": "info", "timestamp": true, "output": "sing-box.log" },
  "dns": {
    "servers": [ { "tag": "local", "address": "local" }, { "tag": "cloudflare", "address": "https://1.1.1.1/dns-query", "detour": "proxy" } ],
    "rules": [ { "outbound": "direct", "server": "local" } ],
    "final": "cloudflare", "strategy": "prefer_ipv4", "disable_cache": false, "independent_cache": true
  },
  "ntp": { "enabled": true, "server": "time.cloudflare.com", "server_port": 123, "interval": "30m", "detour": "direct" },
  "certificate": { "store": "system" },
  "certificate_providers": [ { "type": "local", "tag": "cert-provider", "certificate_path": "cert.pem", "key_path": "key.pem" } ],
  "http_clients": [ { "tag": "default-http", "tls": { "enabled": true, "server_name": "example.com" }, "connect_timeout": "10s", "idle_timeout": "30s", "keep_alive_period": "15s" } ],
  "endpoints": [ { "type": "wireguard", "tag": "wg-endpoint", "address": ["10.0.0.2/32"], "private_key": "PRIVATE_KEY", "peers": [ { "address": "1.2.3.4", "port": 51820, "public_key": "PUBLIC_KEY" } ] } ],
  "inbounds": [
    { "type": "mixed", "tag": "mixed-in", "listen": "127.0.0.1", "listen_port": 2080, "users": [ { "username": "user", "password": "pass" } ], "tcp_fast_open": true, "tcp_multi_path": false, "sniff": true, "sniff_override_destination": false },
    { "type": "tun", "tag": "tun-in", "interface_name": "singtun0", "address": ["172.19.0.1/30", "fdfe:dcba:9876::1/126"], "mtu": 1500, "auto_route": true, "strict_route": true, "stack": "system", "endpoint_independent_nat": true, "sniff": true }
  ],
  "outbounds": [
    { "type": "vless", "tag": "proxy", "server": "server.example.com", "server_port": 443, "uuid": "00000000-0000-0000-0000-000000000000", "flow": "xtls-rprx-vision", "tls": { "enabled": true, "server_name": "www.cloudflare.com", "utls": { "enabled": true, "fingerprint": "chrome" }, "reality": { "enabled": true, "public_key": "PUBLIC_KEY", "short_id": "0123456789abcdef" } }, "multiplex": { "enabled": true, "protocol": "smux", "max_connections": 8, "min_streams": 4, "max_streams": 32 }, "transport": { "type": "grpc", "service_name": "grpc", "idle_timeout": "15s", "ping_timeout": "15s" }, "detour": "" },
    { "type": "selector", "tag": "selector", "outbounds": ["proxy", "direct"], "default": "proxy" },
    { "type": "urltest", "tag": "auto", "outbounds": ["proxy"], "url": "https://www.gstatic.com/generate_204", "interval": "3m", "tolerance": 50 },
    { "type": "dns", "tag": "dns-out" }, { "type": "direct", "tag": "direct" }, { "type": "block", "tag": "block" }
  ],
  "route": {
    "rules": [ { "protocol": "dns", "outbound": "dns-out" }, { "ip_is_private": true, "outbound": "direct" }, { "rule_set": ["geosite-cn"], "outbound": "direct" } ],
    "rule_set": [ { "tag": "geosite-cn", "type": "remote", "format": "binary", "url": "https://example.com/geosite-cn.srs", "download_detour": "direct" } ],
    "final": "selector", "auto_detect_interface": true, "override_android_vpn": true, "default_domain_resolver": "local"
  },
  "services": [ { "type": "clash-api", "external_controller": "127.0.0.1:9090", "secret": "password" } ],
  "experimental": { "cache_file": { "enabled": true, "path": "cache.db", "store_fakeip": true }, "clash_api": { "external_controller": "127.0.0.1:9090", "secret": "password" } }
};

export const AdvancedCoreConfigModal: React.FC<AdvancedCoreConfigModalProps> = ({ onClose, selectedCore }) => {
  const [viewMode, setViewMode] = useState<'ui' | 'json'>('ui');
  const [jsonText, setJsonText] = useState('');
  const [configObj, setConfigObj] = useState<any>({});
  const [isLoading, setIsLoading] = useState(true);
  const [saveStatus, setSaveStatus] = useState<'idle' | 'saving' | 'success' | 'error'>('idle');
  const [errorMsg, setErrorMsg] = useState('');

  // UI state derived from configObj
  const [dnsServersText, setDnsServersText] = useState('');

  useEffect(() => {
    fetchTemplate();
  }, [selectedCore]);

  const fetchTemplate = async () => {
    try {
      const token = localStorage.getItem('cc_client_token') || '';
      const res = await fetch(`/api/v2ray/client/core-template?core=${selectedCore}`, {
        headers: { Authorization: `Bearer ${token}` }
      });
      if (res.ok) {
        const data = await res.json();
        if (data.template) {
          setJsonText(data.template);
          const parsed = JSON.parse(data.template);
          setConfigObj(parsed);
          syncToUIState(parsed);
        } else {
          // Load default
          let def: any = defaultXrayConfig;
          if (selectedCore === 'v2ray') def = defaultV2rayConfig;
          if (selectedCore === 'sing-box') def = defaultSingBoxConfig;
          const str = JSON.stringify(def, null, 2);
          setJsonText(str);
          setConfigObj(def);
          syncToUIState(def);
        }
      }
    } catch (err) {
      console.error(err);
    } finally {
      setIsLoading(false);
    }
  };

  const syncToUIState = (cfg: any) => {
    if (selectedCore === 'sing-box') {
      if (cfg.dns && Array.isArray(cfg.dns.servers)) {
        setDnsServersText(cfg.dns.servers.map((s: any) => s.address || '').join('\n'));
      }
    } else {
      if (cfg.dns && Array.isArray(cfg.dns.servers)) {
        setDnsServersText(cfg.dns.servers.map((s: any) => typeof s === 'string' ? s : (s.address || '')).join('\n'));
      }
    }
  };

  const syncFromUIState = (cfg: any) => {
    const copy = { ...cfg };
    const servers = dnsServersText.split('\n').map(s => s.trim()).filter(s => s);
    
    if (selectedCore === 'sing-box') {
      if (!copy.dns) copy.dns = {};
      // Preserve complex DNS configs, just append/overwrite the basic list simply for this UI
      // In a real perfect mapping, we'd manage the tags. For now, we update addresses of existing or push new ones.
      if (!copy.dns.servers) copy.dns.servers = [];
      const newServers = servers.map((addr, i) => {
        return copy.dns.servers[i] ? { ...copy.dns.servers[i], address: addr } : { tag: `dns-${i}`, address: addr };
      });
      copy.dns.servers = newServers;
    } else {
      if (!copy.dns) copy.dns = {};
      if (!copy.dns.servers) copy.dns.servers = [];
      const newServers = servers.map((addr, i) => {
        const existing = copy.dns.servers[i];
        if (typeof existing === 'object' && existing !== null) {
          return { ...existing, address: addr };
        }
        return addr;
      });
      copy.dns.servers = newServers;
    }
    return copy;
  };

  const handleSave = async () => {
    setSaveStatus('saving');
    setErrorMsg('');
    let payloadStr = jsonText;

    if (viewMode === 'ui') {
      const finalConfig = syncFromUIState(configObj);
      payloadStr = JSON.stringify(finalConfig, null, 2);
      setConfigObj(finalConfig);
      setJsonText(payloadStr);
    } else {
      try {
        const parsed = JSON.parse(jsonText);
        setConfigObj(parsed);
        syncToUIState(parsed);
      } catch (err: any) {
        setSaveStatus('error');
        setErrorMsg('Invalid JSON format: ' + err.message);
        return;
      }
    }

    try {
      const token = localStorage.getItem('cc_client_token') || '';
      const res = await fetch('/api/v2ray/client/core-template', {
        method: 'POST',
        headers: {
          'Content-Type': 'application/json',
          Authorization: `Bearer ${token}`
        },
        body: JSON.stringify({
          core: selectedCore,
          template: payloadStr
        })
      });

      if (res.ok) {
        setSaveStatus('success');
        setTimeout(() => setSaveStatus('idle'), 3000);
      } else {
        const data = await res.json();
        setSaveStatus('error');
        setErrorMsg(data.error || 'Failed to save template');
      }
    } catch (err: any) {
      setSaveStatus('error');
      setErrorMsg(err.message);
    }
  };

  const updateNested = (path: string[], value: any) => {
    setConfigObj((prev: any) => {
      const copy = JSON.parse(JSON.stringify(prev)); // deep clone
      let curr = copy;
      for (let i = 0; i < path.length - 1; i++) {
        if (curr[path[i]] === undefined) curr[path[i]] = {};
        curr = curr[path[i]];
      }
      curr[path[path.length - 1]] = value;
      return copy;
    });
  };

  const getNested = (path: string[]) => {
    let curr = configObj;
    for (let p of path) {
      curr = curr?.[p];
    }
    return curr;
  };

  const removeArrayItem = (path: string[], index: number) => {
    setConfigObj((prev: any) => {
      const copy = JSON.parse(JSON.stringify(prev));
      let curr = copy;
      for (let i = 0; i < path.length - 1; i++) {
        if (curr[path[i]] === undefined) return copy;
        curr = curr[path[i]];
      }
      if (Array.isArray(curr[path[path.length - 1]])) {
        curr[path[path.length - 1]].splice(index, 1);
      }
      return copy;
    });
  };

  const addArrayItem = (path: string[], item: any) => {
    setConfigObj((prev: any) => {
      const copy = JSON.parse(JSON.stringify(prev));
      let curr = copy;
      for (let i = 0; i < path.length - 1; i++) {
        if (curr[path[i]] === undefined) curr[path[i]] = {};
        curr = curr[path[i]];
      }
      if (!Array.isArray(curr[path[path.length - 1]])) {
        curr[path[path.length - 1]] = [];
      }
      curr[path[path.length - 1]].push(item);
      return copy;
    });
  };

  // UI Helpers
  const inbounds = configObj.inbounds || [];
  const outbounds = configObj.outbounds || [];

  return (
    <div className="modal-overlay" style={{ zIndex: 9999, backgroundColor: 'rgba(0,0,0,0.85)', backdropFilter: 'blur(8px)' }}>
      <div className="modal-content" style={{ width: '95%', maxWidth: 1400, height: '95vh', display: 'flex', flexDirection: 'column', background: 'var(--color-brand-bg)', borderRadius: 12, overflow: 'hidden', border: '1px solid var(--color-brand-border)' }}>
        
        {/* Header matching user image mockup */}
        <div style={{ padding: '24px 32px', borderBottom: '1px solid var(--color-brand-border)', display: 'flex', justifyContent: 'space-between', alignItems: 'flex-start', background: 'var(--color-brand-card)' }}>
          <div>
            <div style={{ display: 'flex', alignItems: 'center', gap: 8, color: 'var(--color-brand-muted)', fontSize: 13, marginBottom: 8 }}>
              <FiGlobe /> Home / {selectedCore.toUpperCase()} / Core Configuration
            </div>
            <h2 style={{ fontSize: 24, fontWeight: 800, color: 'var(--color-brand-heading)', margin: '0 0 8px 0', letterSpacing: '-0.5px' }}>
              {selectedCore.toUpperCase()} Core Configurator
            </h2>
            <span style={{ fontSize: 14, color: 'var(--color-brand-muted)' }}>
              Modify daemon log levels, buffer boundaries, and write/overwrite raw client configuration files directly.
            </span>
            
            <div style={{ display: 'flex', gap: 12, marginTop: 24 }}>
              <button
                onClick={() => setViewMode('ui')}
                style={{ padding: '10px 20px', borderRadius: 8, border: 'none', background: viewMode === 'ui' ? '#f97316' : 'transparent', color: viewMode === 'ui' ? '#fff' : 'var(--color-brand-heading)', cursor: 'pointer', display: 'flex', alignItems: 'center', gap: 8, fontSize: 14, fontWeight: 700, transition: 'all 0.2s' }}
              >
                GUI Configurator
              </button>
              <button
                onClick={() => {
                  if (viewMode === 'ui') {
                    setJsonText(JSON.stringify(syncFromUIState(configObj), null, 2));
                  }
                  setViewMode('json');
                }}
                style={{ padding: '10px 20px', borderRadius: 8, border: 'none', background: viewMode === 'json' ? '#f97316' : 'transparent', color: viewMode === 'json' ? '#fff' : 'var(--color-brand-heading)', cursor: 'pointer', display: 'flex', alignItems: 'center', gap: 8, fontSize: 14, fontWeight: 700, transition: 'all 0.2s' }}
              >
                Raw JSON Config
              </button>
            </div>
          </div>

          <button onClick={onClose} style={{ background: 'var(--color-brand-surface)', padding: 10, borderRadius: 8, border: '1px solid var(--color-brand-border)', cursor: 'pointer', color: 'var(--color-brand-heading)', display: 'flex', alignItems: 'center', gap: 8, fontWeight: 600 }}>
            <FiX size={18} /> Close Panel
          </button>
        </div>

        {/* Body Layout */}
        <div style={{ flex: 1, overflowY: 'hidden', display: 'flex' }}>
          {isLoading ? (
            <div style={{ flex: 1, display: 'flex', alignItems: 'center', justifyContent: 'center', color: 'var(--color-brand-muted)' }}>Loading Engine Configuration...</div>
          ) : viewMode === 'json' ? (
            <div style={{ flex: 1, padding: 24, display: 'flex', flexDirection: 'column' }}>
              <div style={{ display: 'flex', justifyContent: 'space-between', alignItems: 'center', marginBottom: 12 }}>
                <span style={{ fontSize: 13, color: 'var(--color-brand-muted)', display: 'flex', alignItems: 'center', gap: 6 }}><FiCode /> Raw JSON Editor</span>
              </div>
              <textarea
                value={jsonText}
                onChange={e => setJsonText(e.target.value)}
                style={{ flex: 1, width: '100%', padding: 20, borderRadius: 12, border: '1px solid var(--color-brand-border)', background: '#0d1117', color: '#c9d1d9', fontFamily: '"JetBrains Mono", monospace', fontSize: 14, resize: 'none', lineHeight: 1.5 }}
                spellCheck={false}
              />
              <div style={{ marginTop: 24, display: 'flex', justifyContent: 'flex-end', alignItems: 'center', gap: 16 }}>
                {saveStatus === 'success' && <span style={{ color: '#10b981', fontSize: 14, fontWeight: 600 }}>JSON Template saved successfully!</span>}
                {saveStatus === 'error' && <span style={{ color: '#ef4444', fontSize: 14, fontWeight: 600 }}>{errorMsg}</span>}
                <button onClick={handleSave} disabled={saveStatus === 'saving'} style={{ background: '#f97316', color: '#fff', border: 'none', padding: '12px 24px', borderRadius: 8, fontWeight: 700, fontSize: 14, display: 'flex', alignItems: 'center', gap: 8, cursor: 'pointer' }}>
                  <FiSave /> {saveStatus === 'saving' ? 'Saving...' : 'Save JSON Settings'}
                </button>
              </div>
            </div>
          ) : (
            <div style={{ flex: 1, display: 'flex', overflow: 'hidden' }}>
              
              {/* Left Column: Main Editor */}
              <div style={{ flex: 1, padding: 32, overflowY: 'auto', borderRight: '1px solid var(--color-brand-border)' }}>
                <div style={{ display: 'flex', alignItems: 'center', gap: 8, marginBottom: 24, color: 'var(--color-brand-heading)', fontWeight: 700, fontSize: 16 }}>
                  <FiSettings style={{ color: '#f97316' }} /> GUI Core Settings
                </div>

                {/* Inbounds */}
                <div style={{ marginBottom: 32 }}>
                  <label style={{ fontSize: 13, fontWeight: 700, color: 'var(--color-brand-heading)', marginBottom: 12, display: 'block', textTransform: 'uppercase', letterSpacing: '0.5px' }}>Inbound Interfaces</label>
                  <div style={{ display: 'flex', flexDirection: 'column', gap: 12 }}>
                    {inbounds.map((inb: any, idx: number) => (
                      <div key={idx} style={{ display: 'flex', gap: 12, alignItems: 'center', background: 'var(--color-brand-card)', padding: '12px 16px', borderRadius: 8, border: '1px solid var(--color-brand-border)' }}>
                        <input
                          type="text"
                          value={selectedCore === 'sing-box' ? inb.listen_port : inb.port}
                          onChange={e => updateNested(['inbounds', idx.toString(), selectedCore === 'sing-box' ? 'listen_port' : 'port'], parseInt(e.target.value) || 0)}
                          placeholder="Port"
                          style={{ width: 100, padding: '8px 12px', borderRadius: 6, border: '1px solid var(--color-brand-border)', background: 'var(--color-brand-bg)', color: 'var(--color-brand-heading)' }}
                        />
                        <select
                          value={selectedCore === 'sing-box' ? inb.type : inb.protocol}
                          onChange={e => updateNested(['inbounds', idx.toString(), selectedCore === 'sing-box' ? 'type' : 'protocol'], e.target.value)}
                          style={{ width: 120, padding: '8px 12px', borderRadius: 6, border: '1px solid var(--color-brand-border)', background: 'var(--color-brand-bg)', color: 'var(--color-brand-heading)' }}
                        >
                          <option value="socks">socks</option>
                          <option value="http">http</option>
                          <option value="mixed">mixed</option>
                          <option value="tun">tun</option>
                        </select>
                        <input
                          type="text"
                          value={inb.listen || ''}
                          onChange={e => updateNested(['inbounds', idx.toString(), 'listen'], e.target.value)}
                          placeholder="Listen IP (e.g. 127.0.0.1)"
                          style={{ flex: 1, padding: '8px 12px', borderRadius: 6, border: '1px solid var(--color-brand-border)', background: 'var(--color-brand-bg)', color: 'var(--color-brand-heading)' }}
                        />
                        <input
                          type="text"
                          value={inb.tag || ''}
                          onChange={e => updateNested(['inbounds', idx.toString(), 'tag'], e.target.value)}
                          placeholder="Tag"
                          style={{ width: 150, padding: '8px 12px', borderRadius: 6, border: '1px solid var(--color-brand-border)', background: 'var(--color-brand-bg)', color: 'var(--color-brand-heading)' }}
                        />
                        <button onClick={() => removeArrayItem(['inbounds'], idx)} style={{ padding: '8px 16px', borderRadius: 6, border: '1px solid rgba(239, 68, 68, 0.3)', background: 'rgba(239, 68, 68, 0.1)', color: '#ef4444', cursor: 'pointer', fontWeight: 600 }}>Remove</button>
                      </div>
                    ))}
                    <button onClick={() => addArrayItem(['inbounds'], selectedCore === 'sing-box' ? { type: 'socks', tag: 'new-in', listen: '127.0.0.1', listen_port: 1080 } : { protocol: 'socks', tag: 'new-in', listen: '127.0.0.1', port: 1080 })} style={{ alignSelf: 'flex-start', padding: '8px 16px', borderRadius: 6, border: '1px dashed var(--color-brand-border)', background: 'transparent', color: 'var(--color-brand-muted)', cursor: 'pointer', display: 'flex', alignItems: 'center', gap: 6, fontWeight: 600 }}>
                      <FiPlus /> Add Inbound
                    </button>
                  </div>
                </div>

                {/* Outbounds */}
                <div style={{ marginBottom: 32 }}>
                  <label style={{ fontSize: 13, fontWeight: 700, color: 'var(--color-brand-heading)', marginBottom: 12, display: 'block', textTransform: 'uppercase', letterSpacing: '0.5px' }}>Outbound Configurations</label>
                  <div style={{ display: 'flex', flexDirection: 'column', gap: 12 }}>
                    {outbounds.map((out: any, idx: number) => (
                      <div key={idx} style={{ display: 'flex', gap: 12, alignItems: 'center', background: 'var(--color-brand-card)', padding: '12px 16px', borderRadius: 8, border: '1px solid var(--color-brand-border)' }}>
                        <select
                          value={selectedCore === 'sing-box' ? out.type : out.protocol}
                          onChange={e => updateNested(['outbounds', idx.toString(), selectedCore === 'sing-box' ? 'type' : 'protocol'], e.target.value)}
                          style={{ width: 180, padding: '8px 12px', borderRadius: 6, border: '1px solid var(--color-brand-border)', background: 'var(--color-brand-bg)', color: 'var(--color-brand-heading)' }}
                        >
                          <option value="freedom">Freedom / Direct</option>
                          <option value="direct">Direct</option>
                          <option value="blackhole">Blackhole / Block</option>
                          <option value="block">Block</option>
                          <option value="vless">VLESS</option>
                          <option value="vmess">VMess</option>
                          <option value="wireguard">WireGuard</option>
                          <option value="dns">DNS</option>
                        </select>
                        <input
                          type="text"
                          value={out.tag || ''}
                          onChange={e => updateNested(['outbounds', idx.toString(), 'tag'], e.target.value)}
                          placeholder="Tag"
                          style={{ flex: 1, padding: '8px 12px', borderRadius: 6, border: '1px solid var(--color-brand-border)', background: 'var(--color-brand-bg)', color: 'var(--color-brand-heading)' }}
                        />
                        <button onClick={() => removeArrayItem(['outbounds'], idx)} style={{ padding: '8px 16px', borderRadius: 6, border: '1px solid rgba(239, 68, 68, 0.3)', background: 'rgba(239, 68, 68, 0.1)', color: '#ef4444', cursor: 'pointer', fontWeight: 600 }}>Remove</button>
                      </div>
                    ))}
                    <button onClick={() => addArrayItem(['outbounds'], selectedCore === 'sing-box' ? { type: 'direct', tag: 'direct' } : { protocol: 'freedom', tag: 'direct' })} style={{ alignSelf: 'flex-start', padding: '8px 16px', borderRadius: 6, border: '1px dashed var(--color-brand-border)', background: 'transparent', color: 'var(--color-brand-muted)', cursor: 'pointer', display: 'flex', alignItems: 'center', gap: 6, fontWeight: 600 }}>
                      <FiPlus /> Add Outbound
                    </button>
                  </div>
                </div>

                {/* DNS & Routing Grid */}
                <div style={{ display: 'grid', gridTemplateColumns: '1fr 1fr', gap: 32, marginBottom: 32 }}>
                  <div>
                    <label style={{ fontSize: 13, fontWeight: 700, color: 'var(--color-brand-heading)', marginBottom: 12, display: 'block', textTransform: 'uppercase', letterSpacing: '0.5px' }}>DNS Servers (One Per Line)</label>
                    <textarea
                      value={dnsServersText}
                      onChange={e => setDnsServersText(e.target.value)}
                      placeholder="8.8.8.8\n1.1.1.1\nlocalhost"
                      style={{ width: '100%', height: 120, padding: 16, borderRadius: 8, border: '1px solid var(--color-brand-border)', background: 'var(--color-brand-card)', color: 'var(--color-brand-heading)', resize: 'vertical', fontFamily: 'monospace' }}
                    />
                  </div>
                  <div>
                    <label style={{ fontSize: 13, fontWeight: 700, color: 'var(--color-brand-heading)', marginBottom: 12, display: 'block', textTransform: 'uppercase', letterSpacing: '0.5px' }}>{selectedCore === 'sing-box' ? 'DNS Strategy' : 'Domain Strategy'}</label>
                    <select
                      value={selectedCore === 'sing-box' ? getNested(['dns', 'strategy']) : getNested(['routing', 'domainStrategy'])}
                      onChange={e => updateNested(selectedCore === 'sing-box' ? ['dns', 'strategy'] : ['routing', 'domainStrategy'], e.target.value)}
                      style={{ width: '100%', padding: '12px 16px', borderRadius: 8, border: '1px solid var(--color-brand-border)', background: 'var(--color-brand-card)', color: 'var(--color-brand-heading)' }}
                    >
                      {selectedCore === 'sing-box' ? (
                        <>
                          <option value="prefer_ipv4">prefer_ipv4</option>
                          <option value="prefer_ipv6">prefer_ipv6</option>
                          <option value="ipv4_only">ipv4_only</option>
                          <option value="ipv6_only">ipv6_only</option>
                        </>
                      ) : (
                        <>
                          <option value="AsIs">AsIs</option>
                          <option value="IPIfNonMatch">IPIfNonMatch</option>
                          <option value="IPOnDemand">IPOnDemand</option>
                        </>
                      )}
                    </select>
                  </div>
                </div>

                {/* Log Paths */}
                <div style={{ display: 'grid', gridTemplateColumns: '1fr 1fr', gap: 32 }}>
                  <div>
                    <label style={{ fontSize: 13, fontWeight: 700, color: 'var(--color-brand-heading)', marginBottom: 12, display: 'block', textTransform: 'uppercase', letterSpacing: '0.5px' }}>Access Log Path</label>
                    <input
                      type="text"
                      value={getNested(selectedCore === 'sing-box' ? ['log', 'output'] : ['log', 'access']) || ''}
                      onChange={e => updateNested(selectedCore === 'sing-box' ? ['log', 'output'] : ['log', 'access'], e.target.value)}
                      placeholder="e.g. /var/log/xray/access.log"
                      style={{ width: '100%', padding: '12px 16px', borderRadius: 8, border: '1px solid var(--color-brand-border)', background: 'var(--color-brand-card)', color: 'var(--color-brand-heading)' }}
                    />
                  </div>
                  <div>
                    <label style={{ fontSize: 13, fontWeight: 700, color: 'var(--color-brand-heading)', marginBottom: 12, display: 'block', textTransform: 'uppercase', letterSpacing: '0.5px' }}>Error Log Path</label>
                    <input
                      type="text"
                      value={getNested(selectedCore === 'sing-box' ? ['log', 'output'] : ['log', 'error']) || ''}
                      onChange={e => updateNested(selectedCore === 'sing-box' ? ['log', 'output'] : ['log', 'error'], e.target.value)}
                      placeholder="e.g. /var/log/xray/error.log"
                      style={{ width: '100%', padding: '12px 16px', borderRadius: 8, border: '1px solid var(--color-brand-border)', background: 'var(--color-brand-card)', color: 'var(--color-brand-heading)' }}
                    />
                  </div>
                </div>

              </div>

              {/* Right Column: Core Parameters Sidebar */}
              <div style={{ width: 350, padding: 32, background: 'var(--color-brand-card)', display: 'flex', flexDirection: 'column' }}>
                <div style={{ display: 'flex', alignItems: 'center', gap: 8, marginBottom: 32, color: 'var(--color-brand-heading)', fontWeight: 700, fontSize: 16 }}>
                  <FiActivity style={{ color: '#f97316' }} /> Core Parameters
                </div>

                <div style={{ display: 'flex', flexDirection: 'column', gap: 24, flex: 1 }}>
                  <div>
                    <label style={{ fontSize: 12, fontWeight: 700, color: 'var(--color-brand-muted)', marginBottom: 8, display: 'block', textTransform: 'uppercase', letterSpacing: '0.5px' }}>Core Log Verbosity</label>
                    <select
                      value={getNested(selectedCore === 'sing-box' ? ['log', 'level'] : ['log', 'loglevel']) || 'warning'}
                      onChange={e => updateNested(selectedCore === 'sing-box' ? ['log', 'level'] : ['log', 'loglevel'], e.target.value)}
                      style={{ width: '100%', padding: '10px 14px', borderRadius: 8, border: '1px solid var(--color-brand-border)', background: 'var(--color-brand-bg)', color: 'var(--color-brand-heading)' }}
                    >
                      <option value="debug">Debug (All Info)</option>
                      <option value="info">Info</option>
                      <option value="warning">Warning (Issues only)</option>
                      <option value="error">Error</option>
                      <option value="none">None</option>
                    </select>
                  </div>

                  {selectedCore !== 'sing-box' && (
                    <>
                      <div>
                        <label style={{ fontSize: 12, fontWeight: 700, color: 'var(--color-brand-muted)', marginBottom: 8, display: 'block', textTransform: 'uppercase', letterSpacing: '0.5px' }}>Max Connections Limit</label>
                        <input
                          type="number"
                          value={getNested(['policy', 'levels', '0', 'bufferSize']) || ''}
                          onChange={e => updateNested(['policy', 'levels', '0', 'bufferSize'], parseInt(e.target.value))}
                          placeholder="10240"
                          style={{ width: '100%', padding: '10px 14px', borderRadius: 8, border: '1px solid var(--color-brand-border)', background: 'var(--color-brand-bg)', color: 'var(--color-brand-heading)' }}
                        />
                      </div>
                      <div>
                        <label style={{ fontSize: 12, fontWeight: 700, color: 'var(--color-brand-muted)', marginBottom: 8, display: 'block', textTransform: 'uppercase', letterSpacing: '0.5px' }}>Connection Idle Timeout (s)</label>
                        <input
                          type="number"
                          value={getNested(['policy', 'levels', '0', 'connIdle']) || ''}
                          onChange={e => updateNested(['policy', 'levels', '0', 'connIdle'], parseInt(e.target.value))}
                          placeholder="300"
                          style={{ width: '100%', padding: '10px 14px', borderRadius: 8, border: '1px solid var(--color-brand-border)', background: 'var(--color-brand-bg)', color: 'var(--color-brand-heading)' }}
                        />
                      </div>
                    </>
                  )}

                  {selectedCore === 'sing-box' && (
                    <div>
                      <label style={{ fontSize: 12, fontWeight: 700, color: 'var(--color-brand-muted)', marginBottom: 8, display: 'block', textTransform: 'uppercase', letterSpacing: '0.5px' }}>Cache Enabled</label>
                      <select
                        value={getNested(['experimental', 'cache_file', 'enabled']) ? 'true' : 'false'}
                        onChange={e => updateNested(['experimental', 'cache_file', 'enabled'], e.target.value === 'true')}
                        style={{ width: '100%', padding: '10px 14px', borderRadius: 8, border: '1px solid var(--color-brand-border)', background: 'var(--color-brand-bg)', color: 'var(--color-brand-heading)' }}
                      >
                        <option value="true">Enabled</option>
                        <option value="false">Disabled</option>
                      </select>
                    </div>
                  )}

                </div>

                <div style={{ marginTop: 'auto', paddingTop: 24 }}>
                  <div style={{ display: 'flex', justifyContent: 'center', marginBottom: 16 }}>
                    {saveStatus === 'success' && <span style={{ color: '#10b981', fontSize: 13, fontWeight: 600 }}>Saved successfully!</span>}
                    {saveStatus === 'error' && <span style={{ color: '#ef4444', fontSize: 13, fontWeight: 600 }}>{errorMsg}</span>}
                  </div>
                  <button
                    onClick={handleSave}
                    disabled={saveStatus === 'saving'}
                    style={{ width: '100%', padding: '14px', borderRadius: 8, border: 'none', background: '#f97316', color: '#fff', fontSize: 15, fontWeight: 700, cursor: 'pointer', display: 'flex', alignItems: 'center', justifyContent: 'center', gap: 8, transition: 'all 0.2s', boxShadow: '0 4px 12px rgba(249, 115, 22, 0.3)' }}
                  >
                    <FiSave size={18} /> {saveStatus === 'saving' ? 'Saving...' : 'Save Core Settings'}
                  </button>
                </div>
              </div>

            </div>
          )}
        </div>
      </div>
    </div>
  );
};
