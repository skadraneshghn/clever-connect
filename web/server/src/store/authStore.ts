import { create } from 'zustand';

interface AuthState {
  token: string | null;
  username: string | null;
  isAuthenticated: boolean;
  isLoading: boolean;
  error: string | null;
  login: (username: string, password: string) => Promise<boolean>;
  logout: () => void;
  initialize: () => void;
}

const getInitialAuth = () => {
  if (typeof window === 'undefined') return { token: null, username: null, isAuthenticated: false };
  const token = localStorage.getItem('cc_server_token');
  const username = localStorage.getItem('cc_server_username');
  return {
    token,
    username,
    isAuthenticated: !!(token && username)
  };
};

const initialAuth = getInitialAuth();

export const useAuthStore = create<AuthState>((set) => ({
  token: initialAuth.token,
  username: initialAuth.username,
  isAuthenticated: initialAuth.isAuthenticated,
  isLoading: false,
  error: null,

  initialize: () => {
    const token = localStorage.getItem('cc_server_token');
    const username = localStorage.getItem('cc_server_username');
    if (token && username) {
      set({ token, username, isAuthenticated: true });
    }
  },

  login: async (username, password) => {
    set({ isLoading: true, error: null });
    try {
      const response = await fetch('/api/auth/login', {
        method: 'POST',
        headers: {
          'Content-Type': 'application/json',
        },
        body: JSON.stringify({ username, password }),
      });

      if (!response.ok) {
        const data = await response.json();
        throw new Error(data.error || 'Authentication failed');
      }

      const data = await response.json();
      localStorage.setItem('cc_server_token', data.token);
      localStorage.setItem('cc_server_username', username);

      set({
        token: data.token,
        username: username,
        isAuthenticated: true,
        isLoading: false,
      });
      return true;
    } catch (err: any) {
      set({ error: err.message, isLoading: false });
      return false;
    }
  },

  logout: () => {
    localStorage.removeItem('cc_server_token');
    localStorage.removeItem('cc_server_username');
    set({ token: null, username: null, isAuthenticated: false, error: null });
  },
}));
