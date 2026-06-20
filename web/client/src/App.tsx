import React, { Suspense, useEffect, lazy, useState } from 'react';
import { createBrowserRouter, RouterProvider, Navigate, Outlet, useLocation, useNavigate } from 'react-router-dom';
import { useAuthStore } from './store/authStore';
import { PanelLayout } from './components/templates/PanelLayout';
import { GlobalDialog } from './components/molecules/GlobalDialog';


// Lazy-loaded pages
const LoginPage = lazy(() => import('./pages/LoginPage').then(m => ({ default: m.LoginPage })));
const DashboardPage = lazy(() => import('./pages/DashboardPage').then(m => ({ default: m.DashboardPage })));
const SettingsPage = lazy(() => import('./pages/SettingsPage').then(m => ({ default: m.SettingsPage })));
const LogsPage = lazy(() => import('./pages/LogsPage').then(m => ({ default: m.LogsPage })));
const EhcoClientPage = lazy(() => import('./pages/EhcoClientPage').then(m => ({ default: m.EhcoClientPage })));
const FilesPage = lazy(() => import('./pages/FilesPage').then(m => ({ default: m.FilesPage })));
const LeechPage = lazy(() => import('./pages/LeechPage').then(m => ({ default: m.LeechPage })));
const PlayerPage = lazy(() => import('./pages/PlayerPage').then(m => ({ default: m.PlayerPage })));
const TorrentPage = lazy(() => import('./pages/TorrentPage').then(m => ({ default: m.TorrentPage })));
const YouTubePage = lazy(() => import('./pages/YouTubePage').then(m => ({ default: m.YouTubePage })));
const SpotifyPage = lazy(() => import('./pages/SpotifyPage').then(m => ({ default: m.SpotifyPage })));
const JobSchedulerPage = lazy(() => import('./pages/JobSchedulerPage').then(m => ({ default: m.JobSchedulerPage })));
const TelegramSettingsPage = lazy(() => import('./pages/TelegramSettingsPage').then(m => ({ default: m.TelegramSettingsPage })));
const SoroushPage = lazy(() => import('./pages/SoroushPage').then(m => ({ default: m.SoroushPage })));
const V2RayDashboardPage = lazy(() => import('./pages/V2RayDashboardPage').then(m => ({ default: m.V2RayDashboardPage })));
const V2RayClientPage = lazy(() => import('./pages/V2RayClientPage').then(m => ({ default: m.V2RayClientPage })));
const V2RayCorePage = lazy(() => import('./pages/V2RayCorePage').then(m => ({ default: m.V2RayCorePage })));
const V2RayRoutingPage = lazy(() => import('./pages/V2RayRoutingPage').then(m => ({ default: m.V2RayRoutingPage })));
const V2RayScannerPage = lazy(() => import('./pages/v2ray-scanner/V2RayScannerPage').then(m => ({ default: m.V2RayScannerPage })));
const DomainCheckerPage = lazy(() => import('./pages/domain-checker/DomainCheckerPage').then(m => ({ default: m.DomainCheckerPage })));
const IPDomainCheckerPage = lazy(() => import('./pages/ip-domain-checker/IPDomainCheckerPage').then(m => ({ default: m.IPDomainCheckerPage })));
const DNSTesterPage = lazy(() => import('./pages/dns-tester/DNSTesterPage').then(m => ({ default: m.DNSTesterPage })));
const V2RayMultipathPage = lazy(() => import('./pages/V2RayMultipathPage').then(m => ({ default: m.V2RayMultipathPage })));
const CloudflarePage = lazy(() => import('./pages/CloudflarePage').then(m => ({ default: m.CloudflarePage })));
const WarpPage = lazy(() => import('./pages/WarpPage').then(m => ({ default: m.WarpPage })));
const TrustTunnelPage = lazy(() => import('./pages/TrustTunnelPage').then(m => ({ default: m.TrustTunnelPage })));

// Loading spinner
const PageLoader = () => (
  <div style={{ display: 'flex', alignItems: 'center', justifyContent: 'center', height: '100vh', background: 'var(--color-brand-bg)' }}>
    <div style={{ width: 32, height: 32, border: '3px solid var(--color-brand-border)', borderTopColor: 'var(--color-brand)', borderRadius: '50%', animation: 'spin .6s linear infinite' }} />
    <style>{`@keyframes spin { to { transform: rotate(360deg); } }`}</style>
  </div>
);

// Protected wrapper synchronizing URL path with sidebar highlight
const ProtectedLayout: React.FC = () => {
  const { isAuthenticated, initialize } = useAuthStore();
  const location = useLocation();
  const navigate = useNavigate();

  useEffect(() => { initialize(); }, [initialize]);

  if (!isAuthenticated) return <Navigate to="/login" replace />;

  const currentTab = location.pathname.split('/')[1] || 'dashboard';

  const crumbs: Record<string, string[]> = {
    dashboard: ['Wallets', 'Balance'],
    nodes: ['VPN Nodes', 'Servers'],
    connections: ['Connections', 'Active'],
    settings: ['Preferences', 'Settings'],
    'fw-logs': ['System Logs', 'Diagnostics'],
    'ehco-tunnel': ['Protocol', 'Ehco'],
    files: ['Storage', 'Files Explorer'],
    leech: ['Storage', 'Remote Leech Manager'],
    torrent: ['Storage', 'Torrent Client'],
    youtube: ['Storage', 'YouTube Downloader'],
    spotify: ['Storage', 'Spotify Downloader'],
    scheduler: ['System', 'Job Scheduler'],
    'telegram-settings': ['Settings', 'Telegram Bot'],
    'soroush-tunnel': ['Protocol', 'Soroush WebRTC Tunnel'],
    trusttunnel: ['Protocol', 'TrustTunnel Stealth'],
    'v2ray-dashboard': ['V2Ray', 'Realtime Dashboard'],
    'v2ray-nodes': ['V2Ray', 'Nodes Manager'],
    'v2ray-core': ['V2Ray', 'Core Configuration'],
    'v2ray-routing': ['V2Ray', 'Routing Rules'],
    'v2ray-scanner': ['Network Tools', 'Scanner Engine'],
    'domain-checker': ['Network Tools', 'Domain Checker'],
    'ip-domain-checker': ['Network Tools', 'IP & Domain Checker'],
    'dns-tester': ['Network Tools', 'DNS Tester'],
    'v2ray-multipath': ['V2Ray', 'Multipath Engine'],
    cloudflare: ['Network Tools', 'Cloudflare'],
    'v2ray-warp': ['V2Ray', 'WARP+ Engine'],
  };

  // Inject user local preferences (Font and Theme) on initial bootstrap
  useEffect(() => {
    const savedFont = localStorage.getItem('cc_font') || 'inter';
    document.body.classList.add(`font-${savedFont}`);

    const savedTheme = localStorage.getItem('cc_theme') || 'light';
    const applyThemeMode = (isDark: boolean) => {
      if (isDark) {
        document.body.classList.add('dark-theme');
      } else {
        document.body.classList.remove('dark-theme');
      }
    };

    if (savedTheme === 'system') {
      const isDarkOS = window.matchMedia('(prefers-color-scheme: dark)').matches;
      applyThemeMode(isDarkOS);
    } else {
      applyThemeMode(savedTheme === 'dark');
    }
  }, []);

  return (
    <PanelLayout activeTab={currentTab} setActiveTab={(tab) => navigate(`/${tab}`)} breadcrumbs={crumbs[currentTab] || ['Dashboard']}>
      <Suspense fallback={<PageLoader />}>
        <Outlet />
      </Suspense>
    </PanelLayout>
  );
};

// Auth guard for login
const LoginGuard: React.FC = () => {
  const { isAuthenticated, initialize } = useAuthStore();
  useEffect(() => { initialize(); }, [initialize]);
  if (isAuthenticated) return <Navigate to="/dashboard" replace />;
  return <Suspense fallback={<PageLoader />}><LoginPage /></Suspense>;
};

const PlayerGuard: React.FC = () => {
  const { isAuthenticated, initialize } = useAuthStore();
  useEffect(() => { initialize(); }, [initialize]);
  if (!isAuthenticated) return <Navigate to="/login" replace />;
  return <Suspense fallback={<PageLoader />}><PlayerPage /></Suspense>;
};

const router = createBrowserRouter([
  { path: '/login', element: <LoginGuard /> },
  { path: '/player', element: <PlayerGuard /> },
  {
    path: '/',
    element: <ProtectedLayout />,
    children: [
      { index: true, element: <Navigate to="/dashboard" replace /> },
      { path: 'dashboard', element: <DashboardPage /> },
      { path: 'settings', element: <SettingsPage /> },
      { path: 'fw-logs', element: <LogsPage /> },
      { path: 'ehco-tunnel', element: <EhcoClientPage /> },
      { path: 'files', element: <FilesPage /> },
      { path: 'leech', element: <LeechPage /> },
      { path: 'torrent', element: <TorrentPage /> },
      { path: 'youtube', element: <YouTubePage /> },
      { path: 'spotify', element: <SpotifyPage /> },
      { path: 'scheduler', element: <JobSchedulerPage /> },
      { path: 'telegram-settings', element: <TelegramSettingsPage /> },
      { path: 'soroush-tunnel', element: <SoroushPage /> },
      { path: 'trusttunnel', element: <TrustTunnelPage /> },
      { path: 'v2ray-dashboard', element: <V2RayDashboardPage /> },
      { path: 'v2ray-nodes', element: <V2RayClientPage /> },
      { path: 'v2ray-core', element: <V2RayCorePage /> },
      { path: 'v2ray-routing', element: <V2RayRoutingPage /> },
      { path: 'v2ray-scanner', element: <V2RayScannerPage /> },
      { path: 'domain-checker', element: <DomainCheckerPage /> },
      { path: 'ip-domain-checker', element: <IPDomainCheckerPage /> },
      { path: 'dns-tester', element: <DNSTesterPage /> },
      { path: 'v2ray-multipath', element: <V2RayMultipathPage /> },
      { path: 'cloudflare', element: <CloudflarePage /> },
      { path: 'v2ray-warp', element: <WarpPage /> },
    ],
  },
  { path: '*', element: <Navigate to="/dashboard" replace /> },
]);

const TopLoadingBar: React.FC = () => {
  const [active, setActive] = useState(false);
  const [progress, setProgress] = useState(0);
  const [visible, setVisible] = useState(false);

  useEffect(() => {
    const handleLoadingChange = (e: Event) => {
      const customEvent = e as CustomEvent<{ active: boolean; count: number }>;
      setActive(customEvent.detail.active);
    };

    window.addEventListener('api-loading-change', handleLoadingChange);
    return () => {
      window.removeEventListener('api-loading-change', handleLoadingChange);
    };
  }, []);

  useEffect(() => {
    let interval: any;
    if (active) {
      setVisible(true);
      setProgress(10);
      interval = setInterval(() => {
        setProgress(prev => {
          if (prev >= 90) return prev;
          const remaining = 90 - prev;
          const step = Math.max(1, Math.floor(remaining * 0.15));
          return prev + step;
        });
      }, 200);
    } else {
      setProgress(100);
      const timer = setTimeout(() => {
        setVisible(false);
        setProgress(0);
      }, 400);
      return () => {
        clearTimeout(timer);
      };
    }

    return () => {
      if (interval) clearInterval(interval);
    };
  }, [active]);

  if (!visible) return null;

  return (
    <>
      <style>{`
        @keyframes loading-glow {
          0% { background-position: 0% 50%; }
          50% { background-position: 100% 50%; }
          100% { background-position: 0% 50%; }
        }
        @keyframes loading-pulse {
          0% { box-shadow: 0 0 8px rgba(255, 107, 44, 0.8), 0 0 4px rgba(255, 107, 44, 0.5); }
          50% { box-shadow: 0 0 16px rgba(255, 107, 44, 1), 0 0 8px rgba(255, 107, 44, 0.8); }
          100% { box-shadow: 0 0 8px rgba(255, 107, 44, 0.8), 0 0 4px rgba(255, 107, 44, 0.5); }
        }
      `}</style>
      <div
        style={{
          position: 'fixed',
          top: 0,
          left: 0,
          width: `${progress}%`,
          height: '3px',
          background: 'linear-gradient(90deg, #ff6b2c, #ff9e7d, #ff6b2c, #ffa88a, #ff6b2c)',
          backgroundSize: '200% 100%',
          animation: 'loading-glow 1.5s linear infinite, loading-pulse 1.5s ease-in-out infinite',
          zIndex: 999999,
          transition: progress === 100 ? 'width 0.3s ease-out, opacity 0.3s ease-out' : 'width 0.2s ease-out',
          opacity: progress === 100 ? 0 : 1,
        }}
      />
    </>
  );
};

export default function App() {
  return (
    <>
      <TopLoadingBar />
      <RouterProvider router={router} />
      <GlobalDialog />
    </>
  );
}
