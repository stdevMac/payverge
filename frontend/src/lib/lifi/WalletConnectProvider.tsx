"use client";

import React, {
  createContext,
  useContext,
  useState,
  useCallback,
  useEffect,
  useRef,
} from "react";
import {
  createConfig,
  http,
  WagmiProvider,
  injected,
  useAccount,
  useConnect,
  useDisconnect,
  useChainId,
  useSwitchChain,
  useWalletClient,
} from "wagmi";
import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import { errMessage } from "@/utils/apiError";
import type { WalletClient } from "viem";
import { executeRoute, Route, RouteExtended } from "@lifi/sdk";
import {
  setLiFiWalletClient,
  initializeLiFi,
  getLiFiClient,
  SETTLEMENT_CHAIN_ID,
} from "./config";
import {
  mainnet,
  polygon,
  arbitrum,
  optimism,
  base,
  baseSepolia,
  avalanche,
  bsc,
} from "@/config/chains";

interface PaymentProgress {
  status: "idle" | "executing" | "success" | "error";
  currentStep: number;
  totalSteps: number;
  txHash?: string;
  error?: string;
}

function settlementChainTxHash(route: RouteExtended): string {
  let settlementHash = "";
  let receivingHash = "";
  let fallbackHash = "";

  route.steps.forEach((step) => {
    // LI.FI v4: Execution.process → Execution.actions
    step.execution?.actions.forEach((action) => {
      if (!action.txHash) {
        return;
      }

      fallbackHash = action.txHash;
      if (action.type === "RECEIVING_CHAIN") {
        receivingHash = action.txHash;
      }
      if (action.chainId === SETTLEMENT_CHAIN_ID) {
        settlementHash = action.txHash;
      }
    });
  });

  return settlementHash || receivingHash || fallbackHash;
}

// Supported chains for cross-chain payments
const supportedChains = [
  mainnet,
  polygon,
  arbitrum,
  optimism,
  base,
  baseSepolia,
  avalanche,
  bsc,
] as const;

// Create a separate wagmi config for cross-chain payments (isolated from main app's Dynamic Labs)
const crossChainWagmiConfig = createConfig({
  chains: supportedChains,
  connectors: [
    injected({
      shimDisconnect: true,
    }),
  ],
  transports: {
    [mainnet.id]: http(),
    [polygon.id]: http(),
    [arbitrum.id]: http(),
    [optimism.id]: http(),
    [base.id]: http(),
    [baseSepolia.id]: http(),
    [avalanche.id]: http(),
    [bsc.id]: http(),
  },
});

// Create a separate query client for cross-chain payments
const crossChainQueryClient = new QueryClient({
  defaultOptions: {
    queries: {
      staleTime: 60_000,
      refetchOnWindowFocus: false,
    },
  },
});

// Context for cross-chain wallet state
interface CrossChainWalletContextType {
  address: `0x${string}` | undefined;
  isConnected: boolean;
  chainId: number | undefined;
  isConnecting: boolean;
  walletClient: WalletClient | undefined;
  connectInjected: () => void;
  connectWalletConnect: () => void;
  disconnect: () => void;
  switchChain: (chainId: number) => Promise<void>;
  hasWalletConnect: boolean;
  // Payment execution
  paymentProgress: PaymentProgress;
  executePayment: (
    route: Route,
    onSuccess?: (txHash: string, route: RouteExtended) => void,
    onError?: (error: string) => void,
  ) => Promise<void>;
  resetPayment: () => void;
}

const CrossChainWalletContext =
  createContext<CrossChainWalletContextType | null>(null);

// Initialize LI.FI SDK once
let lifiInitialized = false;

// Inner component that uses wagmi hooks and syncs with LI.FI SDK
function CrossChainWalletInner({ children }: { children: React.ReactNode }) {
  const { address, isConnected } = useAccount();
  const chainId = useChainId();
  const { connect, connectors, isPending } = useConnect();
  const { disconnect: wagmiDisconnect } = useDisconnect();
  const { switchChainAsync } = useSwitchChain();
  const { data: walletClient } = useWalletClient();

  // Refs to keep latest values for LI.FI SDK callbacks
  const walletClientRef = useRef(walletClient);
  const switchChainAsyncRef = useRef(switchChainAsync);

  // Update refs when values change
  useEffect(() => {
    walletClientRef.current = walletClient;
    switchChainAsyncRef.current = switchChainAsync;
  }, [walletClient, switchChainAsync]);

  // Initialize LI.FI SDK and sync wallet client
  useEffect(() => {
    if (!lifiInitialized) {
      initializeLiFi();
      lifiInitialized = true;
    }

    // Update LI.FI SDK with current wallet client
    if (walletClient) {
      setLiFiWalletClient(walletClient, async (targetChainId: number) => {
        if (switchChainAsyncRef.current) {
          await switchChainAsyncRef.current({ chainId: targetChainId });
          return walletClientRef.current;
        }
        return walletClientRef.current;
      });
    } else {
      setLiFiWalletClient(undefined);
    }
  }, [walletClient]);

  const connectInjected = useCallback(() => {
    const injectedConnector = connectors.find((c) => c.id === "injected");
    if (injectedConnector) {
      connect({ connector: injectedConnector });
    }
  }, [connectors, connect]);

  const connectWalletConnect = useCallback(() => {
    // WalletConnect intentionally disabled for faster and more stable builds.
  }, []);

  const disconnect = useCallback(() => {
    wagmiDisconnect();
    setLiFiWalletClient(undefined);
  }, [wagmiDisconnect]);

  const switchChain = useCallback(
    async (targetChainId: number) => {
      if (switchChainAsync) {
        await switchChainAsync({ chainId: targetChainId });
      }
    },
    [switchChainAsync],
  );

  const hasWalletConnect = connectors.some((c) => c.id === "walletConnect");

  // Payment execution state
  const [paymentProgress, setPaymentProgress] = useState<PaymentProgress>({
    status: "idle",
    currentStep: 0,
    totalSteps: 0,
  });

  const resetPayment = useCallback(() => {
    setPaymentProgress({
      status: "idle",
      currentStep: 0,
      totalSteps: 0,
    });
  }, []);

  const executePayment = useCallback(
    async (
      route: Route,
      onSuccess?: (txHash: string, executedRoute: RouteExtended) => void,
      onError?: (error: string) => void,
    ) => {
      if (!address || !walletClient) {
        const error = "Wallet not connected";
        setPaymentProgress({
          status: "error",
          currentStep: 0,
          totalSteps: 0,
          error,
        });
        onError?.(error);
        return;
      }

      setPaymentProgress({
        status: "executing",
        currentStep: 0,
        totalSteps: route.steps.length,
      });

      try {
        const executedRoute = await executeRoute(getLiFiClient(), route, {
          updateRouteHook: (updatedRoute) => {
            let currentStep = 0;
            let lastTxHash: string | undefined;

            updatedRoute.steps.forEach((step, index) => {
              if (step.execution) {
                currentStep = index + 1;
                const lastAction =
                  step.execution.actions[step.execution.actions.length - 1];
                if (lastAction?.txHash) {
                  lastTxHash = lastAction.txHash;
                }
              }
            });

            setPaymentProgress({
              status: "executing",
              currentStep,
              totalSteps: updatedRoute.steps.length,
              txHash: lastTxHash,
            });
          },
        });

        // Prefer the destination-chain settlement transaction. The backend
        // verifies the USDC transfer on Base, so sending a source-chain bridge
        // tx hash would make a successful LI.FI payment look unverified.
        const finalTxHash = settlementChainTxHash(executedRoute);

        setPaymentProgress({
          status: "success",
          currentStep: route.steps.length,
          totalSteps: route.steps.length,
          txHash: finalTxHash,
        });

        onSuccess?.(finalTxHash, executedRoute);
      } catch (err: unknown) {
        console.error("Payment execution failed:", err);
        const errorMessage = errMessage(err) || "Payment failed";
        setPaymentProgress({
          status: "error",
          currentStep: 0,
          totalSteps: 0,
          error: errorMessage,
        });
        onError?.(errorMessage);
      }
    },
    [address, walletClient],
  );

  const value: CrossChainWalletContextType = {
    address,
    isConnected,
    chainId,
    isConnecting: isPending,
    walletClient,
    connectInjected,
    connectWalletConnect,
    disconnect,
    switchChain,
    hasWalletConnect,
    paymentProgress,
    executePayment,
    resetPayment,
  };

  return (
    <CrossChainWalletContext.Provider value={value}>
      {children}
    </CrossChainWalletContext.Provider>
  );
}

// Provider component - wraps children with isolated wagmi provider
export function CrossChainWalletProvider({
  children,
}: {
  children: React.ReactNode;
}) {
  return (
    <WagmiProvider config={crossChainWagmiConfig}>
      <QueryClientProvider client={crossChainQueryClient}>
        <CrossChainWalletInner>{children}</CrossChainWalletInner>
      </QueryClientProvider>
    </WagmiProvider>
  );
}

// Hook to use cross-chain wallet
export function useCrossChainWallet() {
  const context = useContext(CrossChainWalletContext);
  if (!context) {
    throw new Error(
      "useCrossChainWallet must be used within CrossChainWalletProvider",
    );
  }
  return context;
}
