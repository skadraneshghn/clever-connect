import { StrictMode } from 'react'
import { createRoot } from 'react-dom/client'
import './index.css'
import './styles/globyn.scss'
import App from './App.tsx'

let activeRequests = 0;
const notifyLoadingChange = () => {
  const event = new CustomEvent('api-loading-change', {
    detail: { active: activeRequests > 0, count: activeRequests }
  });
  window.dispatchEvent(event);
};

// Global fetch interceptor to handle 401 Unauthorized (expired/invalid tokens)
const originalFetch = window.fetch;
window.fetch = async (input, init) => {
  const url = typeof input === 'string' ? input : (input as any)?.url || '';
  const isApi = url.includes('/api/') || url.startsWith('api/');

  if (isApi) {
    activeRequests++;
    notifyLoadingChange();
  }

  try {
    const response = await originalFetch(input, init);
    if (response.status === 401) {
      console.warn("Token expired. Redirecting to login...");
      localStorage.removeItem('cc_client_token');
      localStorage.removeItem('cc_client_username');
      // Prevent redirect loop if already on login page
      if (!window.location.pathname.endsWith('/login')) {
        window.location.href = '/login';
      }
    }
    return response;
  } finally {
    if (isApi) {
      activeRequests--;
      if (activeRequests < 0) activeRequests = 0;
      notifyLoadingChange();
    }
  }
};

createRoot(document.getElementById('root')!).render(
  <StrictMode>
    <App />
  </StrictMode>,
)
