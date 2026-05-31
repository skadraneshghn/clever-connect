import React, { Suspense, useEffect, lazy } from 'react';
import { createBrowserRouter, RouterProvider, Navigate, Outlet, useLocation, useNavigate } from 'react-router-dom';
import { useAuthStore } from './store/authStore';
import { PanelLayout } from './components/templates/PanelLayout';

// Lazy-loaded pages
const LoginPage = lazy(() => import('./pages/LoginPage').then(m => ({ default: m.LoginPage })));
const DashboardPage = lazy(() => import('./pages/DashboardPage').then(m => ({ default: m.DashboardPage })));
const SettingsPage = lazy(() => import('./pages/SettingsPage').then(m => ({ default: m.SettingsPage })));
const LogsPage = lazy(() => import('./pages/LogsPage').then(m => ({ default: m.LogsPage })));

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

const router = createBrowserRouter([
  { path: '/login', element: <LoginGuard /> },
  {
    path: '/',
    element: <ProtectedLayout />,
    children: [
      { index: true, element: <Navigate to="/dashboard" replace /> },
      { path: 'dashboard', element: <DashboardPage /> },
      { path: 'settings', element: <SettingsPage /> },
      { path: 'fw-logs', element: <LogsPage /> },
    ],
  },
  { path: '*', element: <Navigate to="/dashboard" replace /> },
]);

export default function App() {
  return <RouterProvider router={router} />;
}
