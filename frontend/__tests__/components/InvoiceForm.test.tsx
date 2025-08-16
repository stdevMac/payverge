import React from 'react';
import { render, screen, fireEvent, waitFor } from '@testing-library/react';
import { QueryClient, QueryClientProvider } from '@tanstack/react-query';
import { WagmiProvider } from 'wagmi';
import { RainbowKitProvider } from '@rainbow-me/rainbowkit';
import '@testing-library/jest-dom';

import InvoiceForm from '@/components/InvoiceForm';
import { config } from '@/lib/wagmi';

// Mock wagmi hooks
jest.mock('wagmi', () => ({
  ...jest.requireActual('wagmi'),
  useAccount: () => ({
    address: '0x742d35Cc6634C0532925a3b8D0d0E0b4C3d5d1B1',
    isConnected: true,
  }),
  useChainId: () => 1,
}));

// Mock API calls
jest.mock('@/lib/api', () => ({
  createInvoice: jest.fn(),
}));

const createWrapper = () => {
  const queryClient = new QueryClient({
    defaultOptions: {
      queries: { retry: false },
      mutations: { retry: false },
    },
  });

  return ({ children }: { children: React.ReactNode }) => (
    <WagmiProvider config={config}>
      <QueryClientProvider client={queryClient}>
        <RainbowKitProvider>
          {children}
        </RainbowKitProvider>
      </QueryClientProvider>
    </WagmiProvider>
  );
};

describe('InvoiceForm', () => {
  const mockOnSuccess = jest.fn();

  beforeEach(() => {
    jest.clearAllMocks();
  });

  it('renders form fields correctly', () => {
    render(<InvoiceForm onSuccess={mockOnSuccess} />, { wrapper: createWrapper() });

    expect(screen.getByLabelText(/title/i)).toBeInTheDocument();
    expect(screen.getByLabelText(/description/i)).toBeInTheDocument();
    expect(screen.getByLabelText(/amount/i)).toBeInTheDocument();
    expect(screen.getByLabelText(/payer email/i)).toBeInTheDocument();
    expect(screen.getByLabelText(/due date/i)).toBeInTheDocument();
    expect(screen.getByRole('button', { name: /create invoice/i })).toBeInTheDocument();
  });

  it('validates required fields', async () => {
    render(<InvoiceForm onSuccess={mockOnSuccess} />, { wrapper: createWrapper() });

    const submitButton = screen.getByRole('button', { name: /create invoice/i });
    fireEvent.click(submitButton);

    await waitFor(() => {
      expect(screen.getByText(/title is required/i)).toBeInTheDocument();
      expect(screen.getByText(/amount is required/i)).toBeInTheDocument();
    });
  });

  it('validates amount format', async () => {
    render(<InvoiceForm onSuccess={mockOnSuccess} />, { wrapper: createWrapper() });

    const amountInput = screen.getByLabelText(/amount/i);
    const submitButton = screen.getByRole('button', { name: /create invoice/i });

    // Test invalid amount
    fireEvent.change(amountInput, { target: { value: 'invalid' } });
    fireEvent.click(submitButton);

    await waitFor(() => {
      expect(screen.getByText(/invalid amount format/i)).toBeInTheDocument();
    });

    // Test negative amount
    fireEvent.change(amountInput, { target: { value: '-100' } });
    fireEvent.click(submitButton);

    await waitFor(() => {
      expect(screen.getByText(/amount must be positive/i)).toBeInTheDocument();
    });

    // Test zero amount
    fireEvent.change(amountInput, { target: { value: '0' } });
    fireEvent.click(submitButton);

    await waitFor(() => {
      expect(screen.getByText(/amount must be greater than zero/i)).toBeInTheDocument();
    });
  });

  it('validates email format', async () => {
    render(<InvoiceForm onSuccess={mockOnSuccess} />, { wrapper: createWrapper() });

    const emailInput = screen.getByLabelText(/payer email/i);
    const submitButton = screen.getByRole('button', { name: /create invoice/i });

    fireEvent.change(emailInput, { target: { value: 'invalid-email' } });
    fireEvent.click(submitButton);

    await waitFor(() => {
      expect(screen.getByText(/invalid email format/i)).toBeInTheDocument();
    });
  });

  it('validates title length', async () => {
    render(<InvoiceForm onSuccess={mockOnSuccess} />, { wrapper: createWrapper() });

    const titleInput = screen.getByLabelText(/title/i);
    const submitButton = screen.getByRole('button', { name: /create invoice/i });

    // Test title too long
    fireEvent.change(titleInput, { target: { value: 'a'.repeat(201) } });
    fireEvent.click(submitButton);

    await waitFor(() => {
      expect(screen.getByText(/title must be less than 200 characters/i)).toBeInTheDocument();
    });
  });

  it('submits form with valid data', async () => {
    const { createInvoice } = require('@/lib/api');
    createInvoice.mockResolvedValue({
      id: '1',
      payment_url: 'http://localhost:3000/pay/1',
    });

    render(<InvoiceForm onSuccess={mockOnSuccess} />, { wrapper: createWrapper() });

    // Fill form with valid data
    fireEvent.change(screen.getByLabelText(/title/i), {
      target: { value: 'Test Invoice' },
    });
    fireEvent.change(screen.getByLabelText(/description/i), {
      target: { value: 'Test Description' },
    });
    fireEvent.change(screen.getByLabelText(/amount/i), {
      target: { value: '100.50' },
    });
    fireEvent.change(screen.getByLabelText(/payer email/i), {
      target: { value: 'payer@example.com' },
    });

    const submitButton = screen.getByRole('button', { name: /create invoice/i });
    fireEvent.click(submitButton);

    await waitFor(() => {
      expect(createInvoice).toHaveBeenCalledWith({
        title: 'Test Invoice',
        description: 'Test Description',
        amount: '100.50',
        currency: 'USDC',
        creator: '0x742d35Cc6634C0532925a3b8D0d0E0b4C3d5d1B1',
        payerEmail: 'payer@example.com',
        dueDate: expect.any(String),
      });
      expect(mockOnSuccess).toHaveBeenCalledWith({
        id: '1',
        payment_url: 'http://localhost:3000/pay/1',
      });
    });
  });

  it('handles API errors', async () => {
    const { createInvoice } = require('@/lib/api');
    createInvoice.mockRejectedValue(new Error('API Error'));

    render(<InvoiceForm onSuccess={mockOnSuccess} />, { wrapper: createWrapper() });

    // Fill form with valid data
    fireEvent.change(screen.getByLabelText(/title/i), {
      target: { value: 'Test Invoice' },
    });
    fireEvent.change(screen.getByLabelText(/amount/i), {
      target: { value: '100' },
    });

    const submitButton = screen.getByRole('button', { name: /create invoice/i });
    fireEvent.click(submitButton);

    await waitFor(() => {
      expect(screen.getByText(/failed to create invoice/i)).toBeInTheDocument();
    });
  });

  it('disables form when wallet not connected', () => {
    // Mock disconnected wallet
    jest.mocked(require('wagmi').useAccount).mockReturnValue({
      address: undefined,
      isConnected: false,
    });

    render(<InvoiceForm onSuccess={mockOnSuccess} />, { wrapper: createWrapper() });

    const submitButton = screen.getByRole('button', { name: /create invoice/i });
    expect(submitButton).toBeDisabled();
    expect(screen.getByText(/connect wallet to create invoices/i)).toBeInTheDocument();
  });

  it('shows loading state during submission', async () => {
    const { createInvoice } = require('@/lib/api');
    createInvoice.mockImplementation(() => new Promise(resolve => setTimeout(resolve, 1000)));

    render(<InvoiceForm onSuccess={mockOnSuccess} />, { wrapper: createWrapper() });

    // Fill form
    fireEvent.change(screen.getByLabelText(/title/i), {
      target: { value: 'Test Invoice' },
    });
    fireEvent.change(screen.getByLabelText(/amount/i), {
      target: { value: '100' },
    });

    const submitButton = screen.getByRole('button', { name: /create invoice/i });
    fireEvent.click(submitButton);

    expect(screen.getByText(/creating invoice/i)).toBeInTheDocument();
    expect(submitButton).toBeDisabled();
  });
});
