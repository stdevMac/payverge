import { render, screen, fireEvent, waitFor } from '@testing-library/react'
import { describe, it, expect, vi, beforeEach } from 'vitest'
import { WalletConnection } from '../WalletConnection'

// Mock the Web3 wallet hooks
vi.mock('@reown/appkit/react', () => ({
  useAppKit: () => ({
    open: vi.fn(),
  }),
  useAppKitAccount: () => ({
    address: '0x742d35Cc6635C0532925a3b8D400E4C3f2c0C1c1',
    isConnected: true,
  }),
  useDisconnect: () => ({
    disconnect: vi.fn(),
  }),
}))

describe('WalletConnection', () => {
  beforeEach(() => {
    vi.clearAllMocks()
  })

  it('renders connect button when wallet not connected', () => {
    // Mock disconnected state
    vi.mocked(useAppKitAccount).mockReturnValue({
      address: undefined,
      isConnected: false,
    })

    render(<WalletConnection />)
    
    expect(screen.getByText(/connect wallet/i)).toBeInTheDocument()
  })

  it('shows wallet address when connected', () => {
    render(<WalletConnection />)
    
    expect(screen.getByText(/0x742d35/)).toBeInTheDocument()
  })

  it('opens wallet modal when connect button clicked', async () => {
    const mockOpen = vi.fn()
    vi.mocked(useAppKit).mockReturnValue({ open: mockOpen })
    
    vi.mocked(useAppKitAccount).mockReturnValue({
      address: undefined,
      isConnected: false,
    })

    render(<WalletConnection />)
    
    const connectButton = screen.getByText(/connect wallet/i)
    fireEvent.click(connectButton)
    
    expect(mockOpen).toHaveBeenCalled()
  })

  it('handles disconnect functionality', async () => {
    const mockDisconnect = vi.fn()
    vi.mocked(useDisconnect).mockReturnValue({ disconnect: mockDisconnect })

    render(<WalletConnection />)
    
    const disconnectButton = screen.getByText(/disconnect/i)
    fireEvent.click(disconnectButton)
    
    await waitFor(() => {
      expect(mockDisconnect).toHaveBeenCalled()
    })
  })
})
