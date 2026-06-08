import React from 'react';
import { render, screen, fireEvent, waitFor } from '@testing-library/react';
import { vi, describe, it, expect } from 'vitest';
import { EditConfigModal } from '../components/EditConfigModal';

// Mock react-icons to avoid parsing errors
vi.mock('react-icons/fi', () => {
  return {
    FiX: () => <span>FiX</span>,
    FiSave: () => <span>FiSave</span>,
    FiCode: () => <span>FiCode</span>,
    FiLayers: () => <span>FiLayers</span>,
    FiActivity: () => <span>FiActivity</span>,
  };
});

describe('EditConfigModal', () => {
  const mockProfile = {
    ID: 42,
    name: 'Test Profile',
    protocol: 'vless',
    address: 'test.example.com',
    port: 443,
    uuid: '123-456-789',
    network: 'ws',
    mux_enabled: true,
    tls_settings: JSON.stringify({
      security: 'tls',
      sni: 'test.example.com',
      path: '/mypath',
      host: 'test.example.com',
      flow: 'xtls-rprx-vision',
    }),
  };

  it('does not render when isOpen is false', () => {
    const { container } = render(
      <EditConfigModal
        isOpen={false}
        profile={mockProfile}
        onClose={vi.fn()}
        onSave={vi.fn()}
        isLoading={false}
      />
    );
    expect(container.firstChild).toBeNull();
  });

  it('renders and populates form fields correctly when open', () => {
    render(
      <EditConfigModal
        isOpen={true}
        profile={mockProfile}
        onClose={vi.fn()}
        onSave={vi.fn()}
        isLoading={false}
      />
    );

    expect(screen.getByText(/Edit V2Ray Configuration Profile/)).toBeInTheDocument();
    expect(screen.getByLabelText('Profile Name')).toHaveValue('Test Profile');
    expect(screen.getByLabelText('Server Address / Host / IP')).toHaveValue('test.example.com');
    expect(screen.getByLabelText('Port')).toHaveValue(443);
    expect(screen.getByLabelText('UUID / User Identifier')).toHaveValue('123-456-789');
    expect(screen.getByLabelText('Transport Network')).toHaveValue('ws');
    expect(screen.getByLabelText('Protocol')).toHaveValue('vless');
  });

  it('switches to raw JSON editor and supports bidirectional synchronization', () => {
    render(
      <EditConfigModal
        isOpen={true}
        profile={mockProfile}
        onClose={vi.fn()}
        onSave={vi.fn()}
        isLoading={false}
      />
    );

    // Click JSON tab
    const jsonTabBtn = screen.getByRole('button', { name: /Advanced JSON Editor/i });
    fireEvent.click(jsonTabBtn);

    // Textarea should contain compiled json
    const textarea = screen.getByPlaceholderText('{}');
    expect(textarea).toBeInTheDocument();
    
    const parsedText = JSON.parse(textarea.textContent || '{}');
    expect(parsedText.security).toBe('tls');
    expect(parsedText.flow).toBe('xtls-rprx-vision');

    // Switch back to Form tab
    const formTabBtn = screen.getByRole('button', { name: /Dynamic Form View/i });
    fireEvent.click(formTabBtn);
    expect(screen.getByLabelText('Profile Name')).toBeInTheDocument();
  });

  it('triggers onSave callback with updated profile data', async () => {
    const onSave = vi.fn();
    render(
      <EditConfigModal
        isOpen={true}
        profile={mockProfile}
        onClose={vi.fn()}
        onSave={onSave}
        isLoading={false}
      />
    );

    const nameInput = screen.getByLabelText('Profile Name');
    fireEvent.change(nameInput, { target: { value: 'Updated Profile Name' } });

    const saveBtn = screen.getByRole('button', { name: /Save Configuration/i });
    fireEvent.click(saveBtn);

    await waitFor(() => {
      expect(onSave).toHaveBeenCalledTimes(1);
      const passedProfile = onSave.mock.calls[0][0];
      expect(passedProfile.name).toBe('Updated Profile Name');
      expect(passedProfile.ID).toBe(42);
    });
  });

  it('triggers onClose when close button clicked', () => {
    const onClose = vi.fn();
    render(
      <EditConfigModal
        isOpen={true}
        profile={mockProfile}
        onClose={onClose}
        onSave={vi.fn()}
        isLoading={false}
      />
    );

    const closeBtn = screen.getByRole('button', { name: /FiX/i });
    fireEvent.click(closeBtn);
    expect(onClose).toHaveBeenCalledTimes(1);
  });
});
