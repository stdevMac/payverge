// LI.FI SDK Configuration for Payverge
// Enables cross-chain payments with any token, settling to USDC on Base

import { getPublicConfig } from "@/config/publicConfig";
import {
  createClient,
  getChains,
  ChainId,
  ChainType,
  type SDKClient,
} from "@lifi/sdk";
import { EthereumProvider } from "@lifi/sdk-provider-ethereum";
import type { WalletClient } from "viem";

// Base chain ID where we settle payments
export const SETTLEMENT_CHAIN_ID = ChainId.BAS; // Base mainnet = 8453
 // Base Sepolia

// USDC addresses on different chains. The map lives outside this module so
// callers that only need the contract addresses do not import @lifi/sdk.
export { USDC_ADDRESSES } from "@/constants/usdc";

// Store for wallet client and switch chain function
// These are set dynamically when the payment modal opens
let currentWalletClient: WalletClient | undefined = undefined;
let currentSwitchChain:
  | ((chainId: number) => Promise<WalletClient | undefined>)
  | undefined = undefined;

// LI.FI v4 client instance (replaces the former global createConfig singleton)
let lifiClient: SDKClient | undefined;

// Set the wallet client for LI.FI SDK to use
export const setLiFiWalletClient = (
  walletClient: WalletClient | undefined,
  switchChain?: (chainId: number) => Promise<WalletClient | undefined>,
) => {
  currentWalletClient = walletClient;
  currentSwitchChain = switchChain;
};

export const getLiFiClient = (): SDKClient => {
  if (!lifiClient) {
    throw new Error("LI.FI SDK not initialized. Call initializeLiFi() first.");
  }
  return lifiClient;
};

// Initialize LI.FI SDK with Ethereum provider
// This must be called once at app startup
export const initializeLiFi = (): SDKClient => {
  lifiClient = createClient({
    // LI.FI partner id; runtime-configurable for self-hosted deployments.
    integrator: getPublicConfig().lifiIntegrator,
    // The SDK's built-in chain preload has historically leaked unhandled
    // rejections when the chain-list request fails (audit G-04). We disable
    // it and run the identical preload below with the failure handled.
    preloadChains: false,
    providers: [
      EthereumProvider({
        getWalletClient: async () => {
          if (!currentWalletClient) {
            throw new Error("Wallet not connected");
          }
          return currentWalletClient;
        },
        switchChain: async (chainId: number) => {
          if (currentSwitchChain) {
            return currentSwitchChain(chainId);
          }
          if (!currentWalletClient) {
            throw new Error("Wallet not connected");
          }
          return currentWalletClient;
        },
      }),
    ],
  });

  // Handled mirror of the SDK's own preload (same chain types). On success
  // the chain/RPC config is populated exactly as preloadChains:true would;
  // on failure we log and leave chains lazy — route discovery hits the API
  // directly and surfaces its own retryable errors, and route execution
  // reports "chain not found" through the existing error paths instead of
  // an uncaught exception at page load.
  void getChains(lifiClient, {
    chainTypes: [ChainType.EVM, ChainType.SVM, ChainType.UTXO, ChainType.MVM],
  })
    .then((chains) => {
      lifiClient?.setChains(chains);
    })
    .catch((err: unknown) => {
      console.warn(
        "LI.FI chain preload failed — cross-chain execution will resolve chains on demand:",
        err,
      );
    });

  return lifiClient;
};

// Chain names for display
export const CHAIN_NAMES: Record<number, string> = {
  [ChainId.ETH]: "Ethereum",
  [ChainId.POL]: "Polygon",
  [ChainId.ARB]: "Arbitrum",
  [ChainId.OPT]: "Optimism",
  [ChainId.BAS]: "Base",
  [ChainId.AVA]: "Avalanche",
  [ChainId.BSC]: "BNB Chain",
  84532: "Base Sepolia",
  11155111: "Sepolia",
};

// Get settlement chain based on environment
// NOTE: LI.FI only supports mainnet chains, so we always use Base mainnet for cross-chain payments
export const getSettlementChainId = (_isTestnet: boolean = false): number => {
  // LI.FI doesn't support testnets - always use Base mainnet for cross-chain swaps
  return SETTLEMENT_CHAIN_ID; // Base mainnet = 8453
};
