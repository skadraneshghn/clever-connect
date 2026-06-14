import React, { useState, useEffect, useRef } from 'react';
import { createPortal } from 'react-dom';
import { 
  FiGlobe, FiCloud, FiServer, FiCompass, 
  FiCopy, FiRefreshCw, FiCheck, FiInfo, FiLoader 
} from 'react-icons/fi';
import { useGeoStore } from '../../store/geoStore';
import type { IPGeoInfo } from '../../store/geoStore';

interface IPResolveBadgeProps {
  ip: string;
  style?: React.CSSProperties;
}

export const IPResolveBadge: React.FC<IPResolveBadgeProps> = ({ ip: rawIp, style }) => {
  const ip = (rawIp || '').trim();
  const resolveIPs = useGeoStore(state => state.resolveIPs);
  const geoInfo = useGeoStore(state => state.geoCache[ip]);
  const isPending = useGeoStore(state => state.pendingResolutions[ip]);

  const [isHovered, setIsHovered] = useState(false);
  const [popupVisible, setPopupVisible] = useState(false);
  const [copied, setCopied] = useState(false);
  const [isRefreshing, setIsRefreshing] = useState(false);
  
  const badgeRef = useRef<HTMLSpanElement>(null);
  const [popupPosition, setPopupPosition] = useState({ top: 0, left: 0 });
  const hoverTimeoutRef = useRef<any>(null);

  // Resolve IP details on mount
  useEffect(() => {
    if (ip) {
      resolveIPs([ip]);
    }
  }, [ip, resolveIPs]);

  // Clean up timeout on unmount
  useEffect(() => {
    return () => {
      if (hoverTimeoutRef.current) {
        clearTimeout(hoverTimeoutRef.current);
      }
    };
  }, []);

  // Update popup visibility when hovered
  useEffect(() => {
    if (isHovered) {
      setPopupVisible(true);
    } else {
      setPopupVisible(false);
    }
  }, [isHovered]);

  const [isMouseOver, setIsMouseOver] = useState(false);

  useEffect(() => {
    const handleKeyDown = (e: KeyboardEvent) => {
      if (isMouseOver && e.key === 'Alt') {
        setIsHovered(true);
      }
    };
    const handleKeyUp = (e: KeyboardEvent) => {
      if (e.key === 'Alt') {
        setIsHovered(false);
      }
    };

    window.addEventListener('keydown', handleKeyDown);
    window.addEventListener('keyup', handleKeyUp);

    return () => {
      window.removeEventListener('keydown', handleKeyDown);
      window.removeEventListener('keyup', handleKeyUp);
    };
  }, [isMouseOver]);

  const updatePosition = () => {
    if (badgeRef.current) {
      const rect = badgeRef.current.getBoundingClientRect();
      const scrollY = window.scrollY || document.documentElement.scrollTop;
      const scrollX = window.scrollX || document.documentElement.scrollLeft;
      setPopupPosition({
        top: rect.top + scrollY,
        left: rect.left + scrollX + rect.width / 2,
      });
    }
  };

  // Recalculate popup positions on state changes or scrolls
  useEffect(() => {
    if (isHovered) {
      updatePosition();
      window.addEventListener('scroll', updatePosition, { passive: true });
      window.addEventListener('resize', updatePosition, { passive: true });
    }
    return () => {
      window.removeEventListener('scroll', updatePosition);
      window.removeEventListener('resize', updatePosition);
    };
  }, [isHovered, ip]);

  const startHideTimeout = () => {
    if (hoverTimeoutRef.current) {
      clearTimeout(hoverTimeoutRef.current);
    }
    hoverTimeoutRef.current = setTimeout(() => {
      setIsHovered(false);
      setPopupVisible(false);
    }, 250); // 250ms buffer to easily bridge any mouse gaps
  };

  const clearHideTimeout = (e?: React.MouseEvent) => {
    if (hoverTimeoutRef.current) {
      clearTimeout(hoverTimeoutRef.current);
      hoverTimeoutRef.current = null;
    }
    setIsHovered(true);
  };

  if (!ip) return <span>-</span>;

  // Render country flag emoji
  const getFlagEmoji = (countryCode: string) => {
    if (!countryCode || countryCode === 'ZZ') return '🌐';
    if (countryCode === 'PV') return '🏠';
    const codePoints = countryCode
      .toUpperCase()
      .split('')
      .map(char =>  127397 + char.charCodeAt(0));
    try {
      return String.fromCodePoint(...codePoints);
    } catch (e) {
      return '🌐';
    }
  };

  const handleCopy = (e: React.MouseEvent) => {
    e.stopPropagation();
    navigator.clipboard.writeText(ip);
    setCopied(true);
    setTimeout(() => setCopied(false), 2000);
  };

  const handleForceRefresh = async (e: React.MouseEvent) => {
    e.stopPropagation();
    setIsRefreshing(true);
    try {
      await resolveIPs([ip], true);
    } catch (err) {
      console.error(err);
    } finally {
      setIsRefreshing(false);
    }
  };

  return (
    <>
      <span
        ref={badgeRef}
        onMouseEnter={(e) => {
          setIsMouseOver(true);
          if (e.altKey) {
            clearHideTimeout(e);
          }
        }}
        onMouseLeave={() => {
          setIsMouseOver(false);
          startHideTimeout();
        }}
        style={{
          display: 'inline-flex',
          alignItems: 'center',
          gap: 6,
          padding: '2px 8px',
          borderRadius: 6,
          background: 'rgba(255, 255, 255, 0.04)',
          border: '1px solid rgba(255, 255, 255, 0.08)',
          fontSize: 12,
          fontFamily: 'monospace',
          color: 'var(--color-brand-heading)',
          cursor: 'help',
          userSelect: 'none',
          transition: 'all 0.2s ease',
          ...style
        }}
        title="Hold Alt + Hover to inspect IP Geolocation & CDN details"
      >
        {/* Flag / Country Indicator */}
        {isPending ? (
          <FiLoader size={12} className="animate-spin" style={{ color: 'var(--color-brand)' }} />
        ) : geoInfo ? (
          <span style={{ fontSize: 13, marginRight: 2 }} role="img" aria-label={geoInfo.country_name}>
            {getFlagEmoji(geoInfo.country_code)}
          </span>
        ) : (
          <FiGlobe size={12} style={{ color: 'var(--color-brand-muted)', opacity: 0.6 }} />
        )}

        {/* IP Text */}
        <span style={{ fontWeight: 500 }}>{ip}</span>

        {/* CDN Badge (flashing icon if CDN) */}
        {!isPending && geoInfo && geoInfo.is_cdn && (
          <span 
            style={{
              display: 'inline-flex',
              alignItems: 'center',
              padding: '1px 4px',
              borderRadius: 4,
              background: 'rgba(59, 130, 246, 0.15)',
              border: '1px solid rgba(59, 130, 246, 0.3)',
              color: '#60a5fa',
              fontSize: 9,
              fontWeight: 700,
              marginLeft: 2,
              letterSpacing: '0.5px'
            }}
            title={`CDN Provider: ${geoInfo.cdn_provider || 'Generic'}`}
          >
            <FiCloud size={9} style={{ marginRight: 2 }} />
            CDN
          </span>
        )}
      </span>

      {/* Amazing Glassmorphism Info Popup */}
      {popupVisible && createPortal(
        <div
          className="ip-resolve-popup"
          style={{
            position: 'absolute',
            top: popupPosition.top - 8,
            left: popupPosition.left,
            transform: 'translateX(-50%) translateY(-100%)',
            width: 320,
            backdropFilter: 'blur(16px)',
            WebkitBackdropFilter: 'blur(16px)',
            borderRadius: 12,
            padding: 16,
            zIndex: 99999,
            fontFamily: 'var(--font-sans), system-ui, sans-serif',
            pointerEvents: 'auto',
            animation: 'fadeInUp 0.15s ease-out'
          }}
          onMouseEnter={() => clearHideTimeout()}
          onMouseLeave={startHideTimeout}
        >
          <style>{`
            .ip-resolve-popup {
              background: rgba(255, 255, 255, 0.95);
              border: 1px solid rgba(0, 0, 0, 0.08);
              box-shadow: 0 20px 40px rgba(0, 0, 0, 0.06), inset 0 1px 1px rgba(255, 255, 255, 0.8);
              color: var(--color-brand-text);
            }
            body.dark-theme .ip-resolve-popup {
              background: rgba(21, 21, 30, 0.92);
              border: 1px solid rgba(255, 255, 255, 0.12);
              box-shadow: 0 20px 40px rgba(0, 0, 0, 0.5), inset 0 1px 1px rgba(255, 255, 255, 0.08);
              color: var(--color-brand-text);
            }
            .ip-resolve-helper {
              background: var(--color-brand-card);
              border: 1px solid var(--color-brand-border);
              color: var(--color-brand-text);
              box-shadow: 0 4px 12px rgba(0,0,0,0.06);
            }
            body.dark-theme .ip-resolve-helper {
              box-shadow: 0 4px 12px rgba(0,0,0,0.25);
            }
            @keyframes fadeInUp {
              from { opacity: 0; transform: translateX(-50%) translateY(-95%) scale(0.96); }
              to { opacity: 1; transform: translateX(-50%) translateY(-100%) scale(1); }
            }
          `}</style>

          {/* Header */}
          <div style={{ display: 'flex', justifyContent: 'space-between', alignItems: 'center', marginBottom: 12 }}>
            <span style={{ fontSize: 9, fontWeight: 800, color: 'var(--color-brand)', letterSpacing: '1px', textTransform: 'uppercase' }}>
              IP Real-time Intelligence
            </span>
            <div style={{ display: 'flex', gap: 8 }}>
              <button
                onClick={handleCopy}
                style={{
                  background: 'none',
                  border: 'none',
                  color: copied ? 'var(--color-brand-green)' : 'var(--color-brand-muted)',
                  cursor: 'pointer',
                  padding: 2,
                  display: 'flex',
                  alignItems: 'center',
                  transition: 'color 0.2s'
                }}
                title="Copy IP Address"
              >
                {copied ? <FiCheck size={14} /> : <FiCopy size={14} />}
              </button>
              <button
                onClick={handleForceRefresh}
                disabled={isRefreshing || isPending}
                style={{
                  background: 'none',
                  border: 'none',
                  color: 'var(--color-brand-muted)',
                  cursor: 'pointer',
                  padding: 2,
                  display: 'flex',
                  alignItems: 'center',
                  transition: 'color 0.2s'
                }}
                title="Force Refresh Data"
              >
                <FiRefreshCw size={14} className={isRefreshing ? 'animate-spin' : ''} />
              </button>
            </div>
          </div>

          {/* IP Display */}
          <div style={{ fontSize: 16, fontWeight: 700, fontFamily: 'monospace', color: 'var(--color-brand-heading)', marginBottom: 14, wordBreak: 'break-all', display: 'flex', alignItems: 'center', gap: 6 }}>
            {ip}
          </div>

          {/* Geolocation Details List */}
          <div style={{ display: 'flex', flexDirection: 'column', gap: 10, borderTop: '1px solid var(--color-brand-border)', paddingTop: 12 }}>
            
            {/* Country */}
            <div style={{ display: 'flex', justifyContent: 'space-between', alignItems: 'center' }}>
              <span style={{ fontSize: 11, color: 'var(--color-brand-text)', display: 'flex', alignItems: 'center', gap: 6 }}>
                <FiGlobe size={12} /> Country
              </span>
              <span style={{ fontSize: 12, fontWeight: 600, color: 'var(--color-brand-heading)' }}>
                {geoInfo ? (
                  <>
                    <span style={{ marginRight: 6 }}>{getFlagEmoji(geoInfo.country_code)}</span>
                    {geoInfo.country_name || 'Unknown'}
                    {geoInfo.country_code ? ` (${geoInfo.country_code})` : ''}
                  </>
                ) : (
                  'Unresolved'
                )}
              </span>
            </div>

            {/* City */}
            <div style={{ display: 'flex', justifyContent: 'space-between', alignItems: 'center' }}>
              <span style={{ fontSize: 11, color: 'var(--color-brand-text)', display: 'flex', alignItems: 'center', gap: 6 }}>
                <FiCompass size={12} /> City / Region
              </span>
              <span style={{ fontSize: 12, fontWeight: 600, color: 'var(--color-brand-heading)' }}>
                {geoInfo?.city || 'Unknown'}
              </span>
            </div>

            {/* ISP / AS */}
            <div style={{ display: 'flex', justifyContent: 'space-between', alignItems: 'flex-start', gap: 12 }}>
              <span style={{ fontSize: 11, color: 'var(--color-brand-text)', display: 'flex', alignItems: 'center', gap: 6, flexShrink: 0 }}>
                <FiServer size={12} /> ISP / ASN
              </span>
              <span style={{ fontSize: 12, fontWeight: 600, color: 'var(--color-brand-heading)', textAlign: 'right', wordBreak: 'break-word', maxWidth: 180 }}>
                {geoInfo?.isp || 'Unknown'}
              </span>
            </div>

            {/* CDN Status */}
            <div style={{ display: 'flex', justifyContent: 'space-between', alignItems: 'center' }}>
              <span style={{ fontSize: 11, color: 'var(--color-brand-text)', display: 'flex', alignItems: 'center', gap: 6 }}>
                <FiCloud size={12} /> CDN Protection
              </span>
              {geoInfo?.is_cdn ? (
                <span style={{ display: 'inline-flex', alignItems: 'center', fontSize: 12, fontWeight: 600, color: 'var(--color-brand-blue)' }}>
                  <span style={{ width: 6, height: 6, borderRadius: '50%', background: 'var(--color-brand-blue)', marginRight: 6, display: 'inline-block' }} />
                  Yes ({geoInfo.cdn_provider || 'Generic'})
                </span>
              ) : (
                <span style={{ fontSize: 12, fontWeight: 600, color: 'var(--color-brand-muted)' }}>
                  No
                </span>
              )}
            </div>

            {/* Coordinates */}
            {geoInfo && (geoInfo.latitude !== 0 || geoInfo.longitude !== 0) && (
              <div style={{ display: 'flex', justifyContent: 'space-between', alignItems: 'center' }}>
                <span style={{ fontSize: 11, color: 'var(--color-brand-text)', display: 'flex', alignItems: 'center', gap: 6 }}>
                  📍 Coordinates
                </span>
                <span style={{ fontSize: 11, fontFamily: 'monospace', color: 'var(--color-brand-text)' }}>
                  {geoInfo.latitude.toFixed(4)}, {geoInfo.longitude.toFixed(4)}
                </span>
              </div>
            )}
            
            {/* Cache Age Info */}
            {geoInfo?.last_updated && (
              <div style={{ fontSize: 9, color: 'var(--color-brand-muted)', textAlign: 'right', marginTop: 4 }}>
                Last updated: {new Date(geoInfo.last_updated).toLocaleDateString()}
              </div>
            )}

            {/* Deep IP Lookup */}
            <button
              onClick={(e) => {
                e.stopPropagation();
                const pathname = window.location.pathname;
                const base = pathname.endsWith('/') ? pathname : pathname.substring(0, pathname.lastIndexOf('/') + 1);
                const targetUrl = `${window.location.origin}${base}ip-domain-checker?target=${encodeURIComponent(ip)}`;
                window.open(targetUrl, '_blank');
              }}
              style={{
                background: 'var(--color-brand)',
                border: 'none',
                borderRadius: 6,
                color: 'white',
                padding: '6px 12px',
                fontSize: 11,
                fontWeight: 600,
                cursor: 'pointer',
                marginTop: 8,
                width: '100%',
                display: 'flex',
                alignItems: 'center',
                justifyContent: 'center',
                gap: 6
              }}
            >
              <FiGlobe size={12} /> Deep IP Lookup & Map
            </button>

            {/* Trigger resolve if not found */}
            {!isPending && !geoInfo && (
              <button
                onClick={handleForceRefresh}
                style={{
                  background: 'rgba(255, 255, 255, 0.08)',
                  border: '1px solid rgba(255, 255, 255, 0.12)',
                  borderRadius: 6,
                  color: 'var(--color-brand-heading)',
                  padding: '6px 12px',
                  fontSize: 11,
                  fontWeight: 600,
                  cursor: 'pointer',
                  marginTop: 6,
                  width: '100%',
                  display: 'flex',
                  alignItems: 'center',
                  justifyContent: 'center',
                  gap: 6
                }}
              >
                <FiRefreshCw size={12} /> Fetch Geolocation
              </button>
            )}
          </div>
        </div>,
        document.body
      )}
    </>
  );
};
