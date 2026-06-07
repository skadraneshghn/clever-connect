import { StrictMode } from 'react'
import { createRoot } from 'react-dom/client'
import './index.css'
import './styles/globyn.scss'
import App from './App.tsx'

// Global fetch interceptor to handle 401 Unauthorized (expired/invalid tokens)
const originalFetch = window.fetch;
window.fetch = async (input, init) => {
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
};

createRoot(document.getElementById('root')!).render(
  <StrictMode>
    <App />
  </StrictMode>,
)
