import React from 'react';
import { render, screen, fireEvent, waitFor } from '@testing-library/react';
import { QueryClient, QueryClientProvider } from '@tanstack/react-query';
import { WagmiProvider } from 'wagmi';
import { RainbowKitProvider } from '@rainbow-me/rainbowkit';
import '@testing-library/jest-dom';

import PaymentPage from '@/components/PaymentPage';
import { config } from '@/lib/wagmi';

// Mock wagmi hooks
jest.mock('wagmi', () => ({
  ...jest.requireActual('wagmi'),
  useAccount: () => ({
    address: '0x853e46Dd7645B0632936a4b9D0d0F0c5D4e6e2C2',
    isConnected: true,
  }),
  useChainId: () => 1,
  useWriteContract: () => ({
    writeContract: jest.fn(),
    isPending: false,
  }),
  useWaitForTransactionReceipt: () => ({
    isLoading: false,
    isSuccess: false,
  }),
}));

// Mock API calls
jest.mock('@/lib/api', () => ({
  getInvoice: jest.fn(),
}));

const mockInvoice = {
  id: '1',
  title: 'Test Invoice',
  description: 'Test Description',
  amount: '100.00',
  currency: 'USDC',
  creator: '0x742d35Cc6634C0532925a3b8D0d0E0b4C3d5d1B1',
  status: 'pending',
  createdAt: '2024-01-01T00:00:00Z',
  dueDate: '2024-01-02T00:00:00Z',
  payerEmail: 'payer@example.com',
};

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

describe('PaymentPage', () => {
  beforeEach(() => {
    jest.clearAllMocks();
    const { getInvoice } = require('@/lib/api');
    getInvoice.mockResolvedValue(mockInvoice);
  });

  it('renders invoice details correctly', async () => {
    render(<PaymentPage invoiceId="1" />, { wrapper: createWrapper() });

    await waitFor(() => {
      expect(screen.getByText('Test Invoice')).toBeInTheDocument();
      expect(screen.getByText('Test Description')).toBeInTheDocument();
      expect(screen.getByText('100.00 USDC')).toBeInTheDocument();
      expect(screen.getByText(/0x742d35...d5d1B1/)).toBeInTheDocument();
    });
  });

  it('shows loading state initially', () => {
    const { getInvoice } = require('@/lib/api');
    getInvoice.mockImplementation(() => new Promise(() => {})); // Never resolves

    render(<PaymentPage invoiceId="1" />, { wrapper: createWrapper() });

    expect(screen.getByText(/loading invoice/i)).toBeInTheDocument();
  });

  it('handles invoice not found', async () => {
    const { getInvoice } = require('@/lib/api');
    getInvoice.mockRejectedValue(new Error('Invoice not found'));

    render(<PaymentPage invoiceId="1" />, { wrapper: createWrapper() });

    await waitFor(() => {
      expect(screen.getByText(/invoice not found/i)).toBeInTheDocument();
    });
  });

  it('shows already paid status', async () => {
    const { getInvoice } = require('@/lib/api');
    getInvoice.mockResolvedValue({
      ...mockInvoice,
      status: 'paid',
      paidBy: '0x853e46Dd7645B0632936a4b9D0d0F0c5D4e6e2C2',
      paidAt: '2024-01-01T12:00:00Z',
    });

    render(<PaymentPage invoiceId="1" />, { wrapper: createWrapper() });

    await waitFor(() => {
      expect(screen.getByText(/invoice paid/i)).toBeInTheDocument();
      expect(screen.getByText(/paid by/i)).toBeInTheDocument();
    });
  });

  it('shows cancelled status', async () => {
    const { getInvoice } = require('@/lib/api');
    getInvoice.mockResolvedValue({
      ...mockInvoice,
      status: 'cancelled',
    });

    render(<PaymentPage invoiceId="1" />, { wrapper: createWrapper() });

    await waitFor(() => {
      expect(screen.getByText(/invoice cancelled/i)).toBeInTheDocument();
    });
  });

  it('shows overdue status', async () => {
    const { getInvoice } = require('@/lib/api');
    getInvoice.mockResolvedValue({
      ...mockInvoice,
      dueDate: '2023-01-01T00:00:00Z', // Past date
    });

    render(<PaymentPage invoiceId="1" />, { wrapper: createWrapper() });

    await waitFor(() => {
      expect(screen.getByText(/overdue/i)).toBeInTheDocument();
    });
  });

  it('enables payment when wallet connected', async () => {
    render(<PaymentPage invoiceId="1" />, { wrapper: createWrapper() });

    await waitFor(() => {
      const payButton = screen.getByRole('button', { name: /pay invoice/i });
      expect(payButton).not.toBeDisabled();
    });
  });

  it('disables payment when wallet not connected', async () => {
    jest.mocked(require('wagmi').useAccount).mockReturnValue({
      address: undefined,
      isConnected: false,
    });

    render(<PaymentPage invoiceId="1" />, { wrapper: createWrapper() });

    await waitFor(() => {
      expect(screen.getByText(/connect wallet to pay/i)).toBeInTheDocument();
    });
  });

  it('handles payment flow', async () => {
    const mockWriteContract = jest.fn();
    jest.mocked(require('wagmi').useWriteContract).mockReturnValue({
      writeContract: mockWriteContract,
      isPending: false,
    });

    render(<PaymentPage invoiceId="1" />, { wrapper: createWrapper() });

    await waitFor(() => {
      const payButton = screen.getByRole('button', { name: /pay invoice/i });
      fireEvent.click(payButton);
    });

    expect(mockWriteContract).toHaveBeenCalledWith({
      address: expect.any(String),
      abi: expect.any(Array),
      functionName: 'payInvoice',
      args: [1, expect.any(BigInt)],
    });
  });

  it('shows payment pending state', async () => {
    jest.mocked(require('wagmi').useWriteContract).mockReturnValue({
      writeContract: jest.fn(),
      isPending: true,
    });

    render(<PaymentPage invoiceId="1" />, { wrapper: createWrapper() });

    await waitFor(() => {
      expect(screen.getByText(/processing payment/i)).toBeInTheDocument();
      const payButton = screen.getByRole('button', { name: /processing/i });
      expect(payButton).toBeDisabled();
    });
  });

  it('shows payment success', async () => {
    jest.mocked(require('wagmi').useWaitForTransactionReceipt).mockReturnValue({
      isLoading: false,
      isSuccess: true,
    });

    render(<PaymentPage invoiceId="1" />, { wrapper: createWrapper() });

    await waitFor(() => {
      expect(screen.getByText(/payment successful/i)).toBeInTheDocument();
    });
  });

  it('validates network compatibility', async () => {
    jest.mocked(require('wagmi').useChainId).mockReturnValue(999); // Unsupported network

    render(<PaymentPage invoiceId="1" />, { wrapper: createWrapper() });

    await waitFor(() => {
      expect(screen.getByText(/unsupported network/i)).toBeInTheDocument();
    });
  });

  it('shows QR code for mobile payments', async () => {
    render(<PaymentPage invoiceId="1" />, { wrapper: createWrapper() });

    await waitFor(() => {
      const qrButton = screen.getByRole('button', { name: /show qr code/i });
      fireEvent.click(qrButton);
      expect(screen.getByText(/scan with mobile wallet/i)).toBeInTheDocument();
    });
  });

  it('copies payment link', async () => {
    // Mock clipboard API
    Object.assign(navigator, {
      clipboard: {
        writeText: jest.fn(),
      },
    });

    render(<PaymentPage invoiceId="1" />, { wrapper: createWrapper() });

    await waitFor(() => {
      const copyButton = screen.getByRole('button', { name: /copy link/i });
      fireEvent.click(copyButton);
      expect(navigator.clipboard.writeText).toHaveBeenCalledWith(
        expect.stringContaining('/pay/1')
      );
    });
  });
});
