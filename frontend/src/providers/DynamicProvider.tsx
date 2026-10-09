"use client";

import React, { ReactNode, useCallback, useMemo, useState } from "react";
import {
  WagmiProvider,
  createConfig,
  http,
  injected,
  useAccount,
  useConnect,
  useDisconnect,
} from "wagmi";
import { base, baseSepolia } from "@/config/chains";
import { getRpcUrlFor } from "@/config/publicConfig";
import { WalletBridge } from "./WalletBridge";

// Built lazily on first render, not at module evaluation: the module can be
// evaluated before the root layout's runtime config script has set
// window.__PAYVERGE_ENV__, which would pin the build-time RPC URLs.
function createWagmiConfig() {
  return createConfig({
    chains: [base, baseSepolia],
    connectors: [
      injected({
        shimDisconnect: true,
      }),
    ],
    multiInjectedProviderDiscovery: false,
    transports: {
      [base.id]: http(getRpcUrlFor("base")),
      [baseSepolia.id]: http(getRpcUrlFor("baseSepolia")),
    },
  });
}

let browserWagmiConfig: ReturnType<typeof createWagmiConfig> | null = null;

/** One config per browser page; per call on the server (env is per-process). */
export function getWagmiConfig(): ReturnType<typeof createWagmiConfig> {
  if (typeof window === "undefined") return createWagmiConfig();
  browserWagmiConfig ??= createWagmiConfig();
  return browserWagmiConfig;
}

/** Test seam: drop the cached browser config. */
export function __resetWagmiConfigForTests(): void {
  browserWagmiConfig = null;
}

interface DynamicProviderProps {
  children: ReactNode;
}

interface DynamicCompatibleUser {
  verifiedCredentials?: Array<{ address: string }>;
}

function shortenAddress(address: string) {
  if (!address) return "";
  return `${address.slice(0, 6)}...${address.slice(-4)}`;
}

/**
 * Operator wallet provider (formerly the Dynamic SDK wrapper). Requires an
 * ancestor QueryClientProvider (AppQueryProvider in the root layout).
 */
export function DynamicProvider({ children }: DynamicProviderProps) {
  // wagmi reuses the QueryClient from the root AppQueryProvider. This
  // provider mounts only in operator layouts, so diner routes never load
  // wagmi/viem; <WalletBridge /> hands the account to HybridAuthProvider.
  const [wagmiConfig] = useState(getWagmiConfig);
  return (
    <WagmiProvider config={wagmiConfig}>
      <WalletBridge />
      {children}
    </WagmiProvider>
  );
}

/**
 * Compatibility hook for modules that previously used Dynamic context.
 */
export function useDynamicContext() {
  const { address } = useAccount();
  const { connectors, connect } = useConnect();
  const { disconnect } = useDisconnect();

  const setShowAuthFlow = useCallback(
    (open: boolean) => {
      if (!open || address) {
        return;
      }

      const preferredConnector = connectors.find((connector) => connector.name !== "Injected") || connectors[0];
      if (!preferredConnector) {
        if (process.env.NODE_ENV !== "production") {
          console.warn("No wallet connectors are available.");
        }
        return;
      }

      connect({ connector: preferredConnector });
    },
    [address, connect, connectors],
  );

  const user = useMemo<DynamicCompatibleUser | null>(() => {
    if (!address) return null;
    return {
      verifiedCredentials: [{ address }],
    };
  }, [address]);

  return {
    setShowAuthFlow,
    user,
    isPending: false,
    disconnect,
  };
}


export function DynamicWidget() {
  const { address } = useAccount();
  const { connectors, connect } = useConnect();
  const { disconnect } = useDisconnect();

  const handleClick = useCallback(() => {
    if (address) {
      disconnect();
      return;
    }

    const preferredConnector = connectors.find((connector) => connector.name !== "Injected") || connectors[0];
    if (preferredConnector) {
      connect({ connector: preferredConnector });
    }
  }, [address, connect, connectors, disconnect]);

  return (
    <button
      type="button"
      onClick={handleClick}
      className="px-4 py-2 rounded-lg bg-primary text-primary-foreground text-sm hover:opacity-90 transition-opacity disabled:opacity-60"
    >
      {address ? shortenAddress(address) : "Connect Wallet"}
    </button>
  );
}
