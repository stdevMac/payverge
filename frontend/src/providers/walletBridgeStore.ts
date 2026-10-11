// Wallet state handed from the operator wallet stack to HybridAuthProvider.
//
// HybridAuthProvider sits in the root layout and therefore runs on diner
// routes too. It used to call wagmi hooks directly, which forced a root-level
// WagmiProvider and shipped wagmi + viem to every guest page. Now WagmiProvider
// mounts only in operator layouts (src/context -> DynamicProvider), where
// <WalletBridge /> publishes the wagmi account into this store. This module
// must stay free of wagmi/viem imports (see guest-route-import-graph.test.ts).
//
// `available` is false wherever no WagmiProvider is mounted (diner routes,
// before hydration). Consumers must treat that as "wallet state unknown",
// never as "wallet disconnected", so a wallet session is not cleared just
// because the operator opened a guest page.

import { create } from "zustand";

type WalletSignMessage = (args: { message: string }) => Promise<string>;

export type WalletBridgeState = {
  available: boolean;
  address: `0x${string}` | undefined;
  isConnected: boolean;
  chainId: number | undefined;
  signMessageAsync: WalletSignMessage | null;
};

export const WALLET_BRIDGE_UNAVAILABLE: WalletBridgeState = {
  available: false,
  address: undefined,
  isConnected: false,
  chainId: undefined,
  signMessageAsync: null,
};

export const useWalletBridgeStore = create<WalletBridgeState>(
  () => WALLET_BRIDGE_UNAVAILABLE,
);

export function useWalletBridge(): WalletBridgeState {
  return useWalletBridgeStore();
}

export function publishWalletBridge(state: Omit<WalletBridgeState, "available">): void {
  useWalletBridgeStore.setState({ ...state, available: true });
}

export function resetWalletBridge(): void {
  useWalletBridgeStore.setState(WALLET_BRIDGE_UNAVAILABLE);
}
