"use client";

import { useEffect } from "react";
import { useAccount, useChainId, useSignMessage } from "wagmi";
import { publishWalletBridge, resetWalletBridge } from "./walletBridgeStore";

/**
 * Mirrors the wagmi account into the wallet bridge store so the root-level
 * HybridAuthProvider can drive wallet sign-in without importing wagmi.
 * Render inside WagmiProvider (DynamicProvider does this).
 *
 * While wagmi is (re)connecting after a mount, `isConnected` is false even
 * for a wallet that is about to come back. The bridge stays "unavailable"
 * through that window so the auth provider does not read it as a logout.
 */
export function WalletBridge(): null {
  const { address, isConnected, status } = useAccount();
  const chainId = useChainId();
  const { signMessageAsync } = useSignMessage();
  const settling = status === "reconnecting" || status === "connecting";

  useEffect(() => {
    if (settling) {
      resetWalletBridge();
      return;
    }
    publishWalletBridge({ address, isConnected, chainId, signMessageAsync });
  }, [settling, address, isConnected, chainId, signMessageAsync]);

  useEffect(() => resetWalletBridge, []);

  return null;
}
