"use client";

import React from "react";
import { DynamicProvider } from "@/providers/DynamicProvider";

// DynamicProvider wraps WagmiProvider + WalletBridge (SSR-safe) and reuses the
// root AppQueryProvider's QueryClient. Mounted by the (shop) layout only. Previously this file dynamic-imported it with ssr:false AND
// gated rendering on a `mounted` useState, which dropped children from SSR
// entirely — the server HTML body was essentially empty, so first paint
// waited on the full client bundle.
export default function Web3ModalProvider({
  children,
}: {
  children: React.ReactNode;
}) {
  return <DynamicProvider>{children}</DynamicProvider>;
}
