import { parseAbi } from 'viem';

// Contract ABI for the InvoiceGenerator contract
export const INVOICE_CONTRACT_ABI = parseAbi([
  'function createInvoice(uint256 amount, string calldata metadataURI) external returns (uint256)',
  'function payInvoice(uint256 invoiceId, uint256 amount) external',
  'function getInvoice(uint256 invoiceId) external view returns (uint256 id, address creator, uint256 amount, string metadataURI, uint256 amountPaid, bool isActive, uint256 createdAt)',
  'function isInvoiceFullyPaid(uint256 invoiceId) external view returns (bool)',
  'function getPaymentStatus(uint256 invoiceId) external view returns (string)',
  'function cancelInvoice(uint256 invoiceId) external',
  'event InvoiceCreated(uint256 indexed invoiceId, address indexed creator, uint256 amount, string metadataURI, uint256 timestamp)',
  'event InvoicePaid(uint256 indexed invoiceId, address indexed payer, uint256 amount, uint256 platformFee, uint256 creatorAmount, uint256 timestamp)',
  'event InvoiceCancelled(uint256 indexed invoiceId, address indexed creator, uint256 timestamp)',
]);

// USDC Contract ABI (ERC-20)
export const USDC_CONTRACT_ABI = parseAbi([
  'function approve(address spender, uint256 amount) external returns (bool)',
  'function allowance(address owner, address spender) external view returns (uint256)',
  'function balanceOf(address account) external view returns (uint256)',
  'function transfer(address to, uint256 amount) external returns (bool)',
  'function transferFrom(address from, address to, uint256 amount) external returns (bool)',
  'function decimals() external view returns (uint8)',
  'function symbol() external view returns (string)',
  'function name() external view returns (string)',
]);

// Contract addresses (these would be set after deployment)
export const CONTRACT_ADDRESSES = {
  // Mainnet
  1: {
    INVOICE_GENERATOR: process.env.NEXT_PUBLIC_INVOICE_CONTRACT_MAINNET || '',
    USDC: '0xA0b86a33E6441b8a0E9e8F5C5B0f5B0f5B0f5B0f', // Mainnet USDC
  },
  // Polygon
  137: {
    INVOICE_GENERATOR: process.env.NEXT_PUBLIC_INVOICE_CONTRACT_POLYGON || '',
    USDC: '0x2791Bca1f2de4661ED88A30C99A7a9449Aa84174', // Polygon USDC
  },
  // Sepolia (testnet)
  11155111: {
    INVOICE_GENERATOR: process.env.NEXT_PUBLIC_INVOICE_CONTRACT_SEPOLIA || '',
    USDC: '0x1c7D4B196Cb0C7B01d743Fbc6116a902379C7238', // Sepolia USDC (example)
  },
} as const;

// Validate contract addresses before use
export function validateContractAddress(address: string): boolean {
  return address !== '' && address !== '0x0000000000000000000000000000000000000000' && /^0x[a-fA-F0-9]{40}$/.test(address);
}

export type SupportedChainId = keyof typeof CONTRACT_ADDRESSES;

export function getContractAddresses(chainId: number) {
  return CONTRACT_ADDRESSES[chainId as SupportedChainId] || CONTRACT_ADDRESSES[1];
}

// Helper functions for USDC amounts
export function formatUSDC(amount: bigint): string {
  return (Number(amount) / 1000000).toFixed(2);
}

export function parseUSDC(amount: string): bigint {
  return BigInt(Math.floor(parseFloat(amount) * 1000000));
}

// Contract interaction hooks would go here
export const CONTRACT_CONFIG = {
  PLATFORM_FEE_BPS: 100, // 1%
  USDC_DECIMALS: 6,
};
