import React from 'react';
import { FiGrid, FiLayers, FiGlobe, FiRepeat, FiShield, FiTrendingUp, FiUsers, FiSettings, FiBook, FiHelpCircle, FiLogOut, FiSearch, FiZap, FiMenu } from 'react-icons/fi';

interface SidebarProps {
  activeTab: string;
  setActiveTab: (tab: string) => void;
  isCollapsed: boolean;
  setIsCollapsed: (collapsed: boolean) => void;
}

const navItems = [
  { id: 'dashboard', label: 'Dashboard', icon: FiGrid },
  { id: 'nodes', label: 'VPN Nodes', icon: FiGlobe },
  { id: 'connections', label: 'Connections', icon: FiLayers },
  { id: 'transfers', label: 'Transfers', icon: FiRepeat },
  {
    id: 'firewall', label: 'Firewall', icon: FiShield,
    children: [
      { id: 'fw-rules', label: 'Rules' },
      { id: 'fw-logs', label: 'Traffic Logs' },
    ]
  },
  { id: 'analytics', label: 'Analytics', icon: FiTrendingUp },
  { id: 'team', label: 'Team', icon: FiUsers },
  { id: 'settings', label: 'Settings', icon: FiSettings },
];

export const Sidebar: React.FC<SidebarProps> = ({ activeTab, setActiveTab, isCollapsed, setIsCollapsed }) => {
  const username = localStorage.getItem('cc_client_username') || 'salman';

  return (
    <aside className={`sidebar ${isCollapsed ? 'sidebar--collapsed' : ''}`}>
      {/* Header */}
      <div className="sidebar__header">
        <div className="sidebar__logo-icon">C</div>
        {!isCollapsed && <span className="sidebar__logo-text">CleverConnect<sup>®</sup></span>}
        <button className="sidebar__toggle" onClick={() => setIsCollapsed(!isCollapsed)}>
          <FiMenu size={14} />
        </button>
      </div>

      {/* Search */}
      <div className="sidebar__search">
        <div className="sidebar__search-wrap">
          <FiSearch className="search-icon" />
          {!isCollapsed && (
            <>
              <input type="text" placeholder="Search" />
              <span className="search-shortcut">⌘F</span>
            </>
          )}
        </div>
      </div>

      {/* Navigation */}
      <nav className="sidebar__nav">
        {navItems.map((item) => (
          <React.Fragment key={item.id}>
            <div
              className={`sidebar__item ${activeTab === item.id || (item.children && item.children.some(c => c.id === activeTab)) ? 'sidebar__item--active' : ''}`}
              onClick={() => {
                if (item.children && item.children.length > 0) {
                  // Navigate to the first child by default on click
                  setActiveTab(item.children[0].id);
                } else {
                  setActiveTab(item.id);
                }
              }}
              title={isCollapsed ? item.label : undefined}
            >
              <item.icon className="nav-icon" />
              {!isCollapsed && <span>{item.label}</span>}
            </div>
            {!isCollapsed && item.children && item.children.length > 0 && (
              <div className="sidebar__submenu">
                {item.children.map((child) => (
                  <div
                    key={child.id}
                    className={`sidebar__submenu-item ${activeTab === child.id ? 'sidebar__submenu-item--active' : ''}`}
                    onClick={() => setActiveTab(child.id)}
                  >
                    <span>{child.label}</span>
                  </div>
                ))}
              </div>
            )}
          </React.Fragment>
        ))}
      </nav>

      <div className="sidebar__divider" />

      {/* Bottom */}
      <div className="sidebar__bottom">
        <div className="sidebar__item" onClick={() => setActiveTab('live')} title={isCollapsed ? 'Live' : undefined}>
          <span className="live-dot" />
          {!isCollapsed && <span>Live</span>}
        </div>
        <div className="sidebar__item" onClick={() => setActiveTab('docs')} title={isCollapsed ? 'Documentation' : undefined}>
          <FiBook className="nav-icon" />
          {!isCollapsed && <span>Documentation</span>}
        </div>
        <div className="sidebar__item" onClick={() => setActiveTab('support')} title={isCollapsed ? 'Help & Support' : undefined}>
          <FiHelpCircle className="nav-icon" />
          {!isCollapsed && <span>Help & Support</span>}
        </div>
      </div>

      {!isCollapsed && (
        <>
          {/* Plan card */}
          <div className="sidebar__plan">
            <div className="sidebar__plan-label">Your Starter Plan</div>
            <div className="sidebar__plan-title">CleverConnect Free Trial</div>
            <div className="sidebar__plan-desc">Unlimited protocols, speed capped at 100Mbps.</div>
          </div>

          <button className="sidebar__upgrade" onClick={() => alert('Upgrade to premium')}>
            <FiZap style={{ marginRight: 6, verticalAlign: 'middle' }} />
            Upgrade Plan
          </button>

          <div className="sidebar__footer-text">Access on Mobile</div>
        </>
      )}

      <div style={{ padding: isCollapsed ? '16px 0' : '0 16px 16px', display: 'flex', flexDirection: isCollapsed ? 'column' : 'row', gap: 10, justifyContent: 'space-between', alignItems: 'center', fontSize: 11, color: 'var(--color-brand-text)' }}>
        {!isCollapsed && (
          <div>
            <div style={{ fontSize: 9, textTransform: 'uppercase', letterSpacing: 1, color: 'var(--color-brand-muted)', marginBottom: 2 }}>Logged in as</div>
            <div style={{ fontWeight: 600, color: 'var(--color-brand-heading)' }}>{username}</div>
          </div>
        )}
        <FiLogOut style={{ cursor: 'pointer', fontSize: 15 }} title="Logout" onClick={() => { localStorage.clear(); window.location.reload(); }} />
      </div>
    </aside>
  );
};
