import { create } from 'zustand';

export interface LogMessage {
  timestamp: string;
  level: string;
  component: string;
  message: string;
  caller: string;
  fields?: Record<string, any>;
  raw: string;
}

interface LogState {
  logs: LogMessage[];
  isPaused: boolean;
  isConnected: boolean;
  ws: WebSocket | null;
  connectLogs: (token: string) => () => void;
  clearLogs: () => void;
  togglePause: () => void;
}

export const useLogStore = create<LogState>((set, get) => ({
  logs: [],
  isPaused: false,
  isConnected: false,
  ws: null,

  connectLogs: (token) => {
    // If socket is already open, do not re-create
    if (get().ws) {
      return () => {};
    }

    const protocol = window.location.protocol === 'https:' ? 'wss:' : 'ws:';
    const host = window.location.host;
    const wsUrl = `${protocol}//${host}/ws/logs?token=${token}`;

    const socket = new WebSocket(wsUrl);

    socket.onopen = () => {
      set({ isConnected: true, ws: socket });
    };

    socket.onmessage = (event) => {
      if (get().isPaused) return;

      try {
        const msg: LogMessage = JSON.parse(event.data);
        set((state) => {
          // Cap at 200 logs to optimize DOM rendering performance
          const newLogs = [...state.logs, msg];
          if (newLogs.length > 200) {
            newLogs.shift();
          }
          return { logs: newLogs };
        });
      } catch (err) {
        console.error('Failed to parse incoming WS log:', err);
      }
    };

    socket.onclose = () => {
      set({ isConnected: false, ws: null });
    };

    socket.onerror = () => {
      set({ isConnected: false });
    };

    return () => {
      socket.close();
      set({ isConnected: false, ws: null });
    };
  },

  clearLogs: () => {
    set({ logs: [] });
  },

  togglePause: () => {
    set((state) => ({ isPaused: !state.isPaused }));
  },
}));
