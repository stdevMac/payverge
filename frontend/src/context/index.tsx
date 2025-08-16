"use client";

import React, { useEffect, useState } from "react";
import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import { State, WagmiProvider } from "wagmi";
import { config, metadata, siweConfig } from "@/config";
import { createAppKit } from "@reown/appkit/react";
import { getNetwork } from "@/config/network";

// Setup queryClient
const queryClient = new QueryClient();
const network = getNetwork();
const chainKey = `eip155:${network.id}`;

createAppKit({
  adapters: [config],
  networks: [network],
  metadata: metadata,
  // Setting tokens to null to hide ETH values
  tokens: {
    [chainKey]: {
      address: process.env.NEXT_PUBLIC_USDC_ADDRESS!,
    }
  },
  siweConfig: siweConfig(),
  debug: false,
  defaultNetwork: network,
  projectId: process.env.NEXT_PUBLIC_PROJECT_ID!,
  features: {
    analytics: true,
    email: true, // default to true
    socials: ['x', 'google', 'apple'], // default value
    emailShowWallets: true // default to true
  },
});

export default function Web3ModalProvider({
  children,
  initialState,
}: {
  children: React.ReactNode;
  initialState?: State;
}) {
  const [mounted, setMounted] = useState(false);

  useEffect(() => {
    setMounted(true);
  }, []);

  if (!mounted) {
    return null;
  }

  return (
    <WagmiProvider config={config.wagmiConfig} initialState={initialState}>
      <QueryClientProvider client={queryClient}>{children}</QueryClientProvider>
    </WagmiProvider>
  );
}
