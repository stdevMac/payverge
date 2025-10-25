import { axiosInstance } from './index';

export interface FaucetResponse {
  status: string;
  eth_tx_hash?: string;
  usdc_tx_hash?: string;
  to_address?: string;
  eth_sent?: string;
  usdc_sent?: string;
  eth_balance?: string;
  usdc_balance?: string;
  error?: string;
  message?: string;
}

export interface FaucetTopUpRequest {
  address: string;
}

class FaucetAPI {
  /**
   * Check if an address is eligible for testnet faucet
   */
  async checkEligibility(address: string): Promise<FaucetResponse> {
    const response = await axiosInstance.get<FaucetResponse>(
      `/inside/testnet-faucet/check/${address}`
    );
    return response.data;
  }

  /**
   * Request testnet tokens for an address
   */
  async requestTokens(request: FaucetTopUpRequest): Promise<FaucetResponse> {
    const response = await axiosInstance.post<FaucetResponse>(
      `/inside/testnet-faucet/topup`,
      request
    );
    return response.data;
  }

  /**
   * Check if current network is a supported testnet
   */
  isTestnet(chainId: number): boolean {
    // Base Sepolia = 84532, Ethereum Sepolia = 11155111
    return chainId === 84532 || chainId === 11155111;
  }

  /**
   * Get testnet network name
   */
  getTestnetName(chainId: number): string {
    switch (chainId) {
      case 84532:
        return 'Base Sepolia';
      case 11155111:
        return 'Ethereum Sepolia';
      default:
        return 'Unknown Testnet';
    }
  }

  /**
   * Get block explorer URL for a transaction
   */
  getExplorerUrl(chainId: number, txHash: string): string {
    switch (chainId) {
      case 84532:
        return `https://sepolia.basescan.org/tx/${txHash}`;
      case 11155111:
        return `https://sepolia.etherscan.io/tx/${txHash}`;
      default:
        return '#';
    }
  }

  /**
   * Format token amount for display
   */
  formatTokenAmount(amount: string, decimals: number = 18, precision: number = 4): string {
    try {
      const value = BigInt(amount);
      const divisor = BigInt(10 ** decimals);
      const result = Number(value) / Number(divisor);
      return result.toFixed(precision);
    } catch {
      return '0';
    }
  }

  /**
   * Get faucet limits and information
   */
  getFaucetInfo() {
    return {
      ethAmount: '0.001',
      usdcAmount: '1000',
      cooldownHours: 24,
      supportedNetworks: [
        { chainId: 84532, name: 'Base Sepolia', explorer: 'https://sepolia.basescan.org' },
        { chainId: 11155111, name: 'Ethereum Sepolia', explorer: 'https://sepolia.etherscan.io' },
      ],
    };
  }
}

export const faucetAPI = new FaucetAPI();
