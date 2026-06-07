import React from 'react';
import { render, screen, fireEvent, waitFor } from '@testing-library/react';
import { vi, describe, it, expect, beforeEach } from 'vitest';
import { ActiveCoreControlCard } from '../components/ActiveCoreControlCard';
import { V2RayServerPage } from '../../V2RayServerPage';

// Mock react-icons to avoid issues with SVG imports
vi.mock('react-icons/fi', () => {
  return {
    FiHelpCircle: () => <span>FiHelpCircle</span>,
    FiActivity: () => <span>FiActivity</span>,
    FiPlay: () => <span>FiPlay</span>,
    FiSquare: () => <span>FiSquare</span>,
    FiRefreshCw: () => <span>FiRefreshCw</span>,
    FiServer: () => <span>FiServer</span>,
    FiX: () => <span>FiX</span>,
    FiSliders: () => <span>FiSliders</span>,
    FiUsers: () => <span>FiUsers</span>,
    FiTerminal: () => <span>FiTerminal</span>,
    FiLock: () => <span>FiLock</span>,
    FiZap: () => <span>FiZap</span>,
    FiSettings: () => <span>FiSettings</span>,
    FiShare2: () => <span>FiShare2</span>,
    FiCpu: () => <span>FiCpu</span>,
  };
});

describe('ActiveCoreControlCard', () => {
  const defaultProps = {
    isRunning: false,
    isLoading: false,
    handleToggleCore: vi.fn(),
  };

  it('renders status inactive when isRunning is false', () => {
    render(<ActiveCoreControlCard {...defaultProps} />);
    expect(screen.getByText('INACTIVE')).toBeInTheDocument();
    
    const startBtn = screen.getByRole('button', { name: /Start/i });
    expect(startBtn).not.toBeDisabled();
    
    const stopBtn = screen.getByRole('button', { name: /Stop/i });
    expect(stopBtn).toBeDisabled();
  });

  it('renders status active when isRunning is true', () => {
    render(<ActiveCoreControlCard {...defaultProps} isRunning={true} />);
    expect(screen.getByText('ACTIVE')).toBeInTheDocument();
    
    const startBtn = screen.getByRole('button', { name: /Start/i });
    expect(startBtn).toBeDisabled();
    
    const stopBtn = screen.getByRole('button', { name: /Stop/i });
    expect(stopBtn).not.toBeDisabled();
  });

  it('triggers handleToggleCore callback on click', () => {
    const handleToggleCore = vi.fn();
    render(<ActiveCoreControlCard {...defaultProps} handleToggleCore={handleToggleCore} />);
    
    const startBtn = screen.getByRole('button', { name: /Start/i });
    fireEvent.click(startBtn);
    expect(handleToggleCore).toHaveBeenCalledWith('start');
  });
});

describe('V2RayServerPage', () => {
  beforeEach(() => {
    vi.spyOn(console, 'error').mockImplementation((msg) => {
      console.log('Spied Console Error:', msg);
    });

    vi.stubGlobal('localStorage', {
      getItem: vi.fn().mockReturnValue('mock-server-token'),
      setItem: vi.fn(),
      clear: vi.fn(),
    });

    vi.stubGlobal('fetch', vi.fn().mockImplementation((url) => {
      let data: any = [];
      if (url.includes('/api/v2ray/nodes')) {
        data = [{ ID: 1, name: 'SG-Edge-01', ip: '128.199.100.12', ssh_port: 22, status: 'online' }];
      } else if (url.includes('/api/v2ray/inbounds')) {
        data = [{ ID: 1, tag: 'vless-reality-443', port: 443, protocol: 'vless', network: 'tcp', tls_mode: 'reality' }];
      } else if (url.includes('/api/v2ray/users')) {
        data = [{ ID: 1, name: 'user01', uuid: 'some-uuid', used_upload: 1024, used_download: 2048, traffic_limit: 0 }];
      } else if (url.includes('/api/v2ray/traffic/logs')) {
        data = [{ name: 'user01', upload: 512, download: 1024 }];
      } else if (url.includes('/api/v2ray/core/status')) {
        data = { is_running: true };
      }
      return Promise.resolve({
        ok: true,
        json: () => Promise.resolve(data),
      });
    }));
  });

  it('renders server panel layout and lazy components', async () => {
    render(<V2RayServerPage />);

    expect(screen.getByText(/V2Ray \/ Xray Server panel/i)).toBeInTheDocument();

    await waitFor(() => {
      expect(screen.getByText(/Remote VPS Edge Nodes/i)).toBeInTheDocument();
      expect(screen.getByText(/SG-Edge-01/i)).toBeInTheDocument();
      expect(screen.getAllByText(/vless-reality-443/i).length).toBeGreaterThan(0);
    });
  });
});
