// Contract configuration for PayvergePayments
import { Address } from 'viem';

export interface ContractConfig {
  address: Address;
  chainId: number;
  usdcAddress: Address;
  platformTreasury: Address;
}

// Contract addresses by chain ID
export const CONTRACT_ADDRESSES: Record<number, ContractConfig> = {
  // Ethereum Mainnet
  1: {
    address: process.env.NEXT_PUBLIC_PAYVERGE_CONTRACT_ADDRESS as Address || '0x',
    chainId: 1,
    usdcAddress: '0xA0b86a33E6441E6C673b3b4b7B4b3b4b7B4b3b4b' as Address, // USDC on mainnet
    platformTreasury: process.env.NEXT_PUBLIC_PLATFORM_TREASURY as Address || '0x',
  },
  // Sepolia Testnet
  11155111: {
    address: process.env.NEXT_PUBLIC_PAYVERGE_CONTRACT_ADDRESS as Address || '0x',
    chainId: 11155111,
    usdcAddress: '0x1c7D4B196Cb0C7B01d743Fbc6116a902379C7238' as Address, // USDC on Sepolia
    platformTreasury: process.env.NEXT_PUBLIC_PLATFORM_TREASURY as Address || '0x',
  },
  // Polygon
  137: {
    address: process.env.NEXT_PUBLIC_PAYVERGE_CONTRACT_ADDRESS as Address || '0x',
    chainId: 137,
    usdcAddress: '0x2791Bca1f2de4661ED88A30C99A7a9449Aa84174' as Address, // USDC on Polygon
    platformTreasury: process.env.NEXT_PUBLIC_PLATFORM_TREASURY as Address || '0x',
  },
  // Base
  8453: {
    address: process.env.NEXT_PUBLIC_PAYVERGE_CONTRACT_ADDRESS as Address || '0x',
    chainId: 8453,
    usdcAddress: '0x833589fCD6eDb6E08f4c7C32D4f71b54bdA02913' as Address, // USDC on Base
    platformTreasury: process.env.NEXT_PUBLIC_PLATFORM_TREASURY as Address || '0x',
  },
};

export const getContractConfig = (chainId: number): ContractConfig => {
  const config = CONTRACT_ADDRESSES[chainId];
  if (!config) {
    throw new Error(`Unsupported chain ID: ${chainId}`);
  }
  return config;
};

// Contract constants
export const CONTRACT_CONSTANTS = {
  FEE_DENOMINATOR: BigInt(10000), // 100.00% = 10000
  MAX_PLATFORM_FEE: BigInt(1000), // 10.00% = 1000
  MAX_TIP_PERCENTAGE: BigInt(10000), // 100.00% = 10000
  MIN_PAYMENT_AMOUNT: BigInt(1000000), // 1 USDC (6 decimals)
  MAX_PAYMENT_AMOUNT: BigInt(1000000000000), // 1M USDC (6 decimals)
} as const;
