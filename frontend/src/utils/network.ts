import { getNetworkId } from "@/config/network";

const BASE_TESTNET = 84532;
const BASE_MAINNET = 8453;
export const CHAIN_ID = getNetworkId();

/**
 * Returns the explorer URL for a given chain ID and address
 * @param chainId The chain ID
 * @param address The address to view in the explorer
 * @returns The explorer URL
 */
export const getExplorerUrl = (chainId: number, address: string) => {
  // Base Sepolia
  if (chainId === BASE_TESTNET) {
    return `https://sepolia.basescan.org/address/${address}`;
  }
  // Base Mainnet
  if (chainId === BASE_MAINNET) {
    return `https://basescan.org/address/${address}`;
  }
  return `https://sepolia.basescan.org/address/${address}`;
};

/**
 * Returns the transaction explorer URL for a given chain ID and transaction hash
 * @param chainId The chain ID
 * @param txHash The transaction hash to view in the explorer
 * @returns The explorer URL
 */
export const getExplorerTxUrl = (chainId: number, txHash: string) => {
  // Base Sepolia
  if (chainId === BASE_TESTNET) {
    return `https://sepolia.basescan.org/tx/${txHash}`;
  }
  // Base Mainnet
  if (chainId === BASE_MAINNET) {
    return `https://basescan.org/tx/${txHash}`;
  }
  return `https://sepolia.basescan.org/tx/${txHash}`;
};
