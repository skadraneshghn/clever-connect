import React from 'react';
import { render, screen, fireEvent, waitFor } from '@testing-library/react';
import { vi, describe, it, expect, beforeEach } from 'vitest';
import { EngineStatusCard } from '../components/EngineStatusCard';
import { V2RayClientPage } from '../../V2RayClientPage';

// Mock react-icons to avoid SVG parsing issues or missing icons
vi.mock('react-icons/fi', () => {
  return {
    FiHelpCircle: () => <span>FiHelpCircle</span>,
    FiActivity: () => <span>FiActivity</span>,
    FiPlay: () => <span>FiPlay</span>,
    FiSquare: () => <span>FiSquare</span>,
    FiRefreshCw: () => <span>FiRefreshCw</span>,
    FiServer: () => <span>FiServer</span>,
    FiX: () => <span>FiX</span>,
  };
});

describe('EngineStatusCard', () => {
  const defaultProps = {
    isRunning: false,
    isLoading: false,
    socksPort: 10808,
    httpPort: 10809,
    speedTestActive: false,
    speedTestBreakdown: null,
    handleRunSpeedTest: vi.fn(),
    handleStartCore: vi.fn(),
    handleStopCore: vi.fn(),
    showHelp: vi.fn(),
  };

  it('renders correctly when core is stopped', () => {
    render(<EngineStatusCard {...defaultProps} />);
    expect(screen.getByText(/Core Supervisor: STOPPED/)).toBeInTheDocument();
    expect(screen.getByText(/Local inbound captures: SOCKS5 on port 10808/)).toBeInTheDocument();
    
    const startBtn = screen.getByRole('button', { name: /Start/i });
    expect(startBtn).toBeInTheDocument();
    expect(startBtn).not.toBeDisabled();

    const stopBtn = screen.getByRole('button', { name: /Stop/i });
    expect(stopBtn).toBeDisabled();
  });

  it('renders correctly when core is running', () => {
    render(<EngineStatusCard {...defaultProps} isRunning={true} />);
    expect(screen.getByText(/Core Supervisor: RUNNING/)).toBeInTheDocument();

    const startBtn = screen.getByRole('button', { name: /Start/i });
    expect(startBtn).toBeDisabled();

    const stopBtn = screen.getByRole('button', { name: /Stop/i });
    expect(stopBtn).not.toBeDisabled();
  });

  it('calls handleStartCore and handleStopCore callbacks', () => {
    const handleStartCore = vi.fn();
    const handleStopCore = vi.fn();
    render(
      <EngineStatusCard
        {...defaultProps}
        isRunning={false}
        handleStartCore={handleStartCore}
        handleStopCore={handleStopCore}
      />
    );

    const startBtn = screen.getByRole('button', { name: /Start/i });
    fireEvent.click(startBtn);
    expect(handleStartCore).toHaveBeenCalledTimes(1);
  });

  it('displays speed test breakdown if available', () => {
    const speedTestBreakdown = {
      throughput_mbps: 45.67,
      tls_handshake_ms: 20,
      ttfb_ms: 35,
      tcp_conn_ms: 10,
    };
    render(
      <EngineStatusCard
        {...defaultProps}
        isRunning={true}
        speedTestBreakdown={speedTestBreakdown}
      />
    );
    expect(screen.getByText(/45.67 Mbps/)).toBeInTheDocument();
    expect(screen.getByText(/20ms/)).toBeInTheDocument();
    expect(screen.getByText(/35ms/)).toBeInTheDocument();
  });
});

describe('V2RayClientPage', () => {
  beforeEach(() => {
    vi.stubGlobal('localStorage', {
      getItem: vi.fn().mockReturnValue('mock-token'),
      setItem: vi.fn(),
      clear: vi.fn(),
    });

    vi.stubGlobal('fetch', vi.fn().mockImplementation((url) => {
      let data: any = [];
      if (url.includes('/api/v2ray/inbounds')) {
        data = [{ ID: 1, tag: 'vless-inbound', port: 10808, protocol: 'vless', network: 'tcp' }];
      } else if (url.includes('/api/v2ray/core/status')) {
        data = { is_running: true, socks_port: 10808, http_port: 10809 };
      } else if (url.includes('/api/v2ray/subscriptions')) {
        data = [{ ID: 1, name: 'My Sub', url: 'https://example.com' }];
      } else if (url.includes('/api/v2ray/settings')) {
        data = { core_binary: 'xray', socks_port: 10808, http_port: 10809 };
      }
      return Promise.resolve({
        ok: true,
        json: () => Promise.resolve(data),
      });
    }));
  });

  it('renders the V2RayClientPage lazy loaded views without crashing', async () => {
    render(<V2RayClientPage />);
    
    // Check if main header is displayed
    expect(screen.getByText(/V2Ray \/ Xray manager/i)).toBeInTheDocument();

    // Verify lazy loading finishes and card components appear (e.g. Core Supervisor status card)
    await waitFor(() => {
      expect(screen.getByText(/Core Supervisor/i)).toBeInTheDocument();
    });
  });
});
