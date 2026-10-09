"use client";

import React, { ReactNode, useState } from "react";
import { QueryClientProvider } from "@tanstack/react-query";
import { createAppQueryClient } from "@/lib/createAppQueryClient";
import { useClearQueryCacheOnSessionChange } from "@/lib/queryCacheSession";

/**
 * Root React Query provider. It used to live inside DynamicProvider next to
 * WagmiProvider, which meant every route (diner pages included) mounted and
 * downloaded wagmi just to get a QueryClient. wagmi now mounts only in the
 * operator layouts and reuses this client from the nearest ancestor.
 */
export function AppQueryProvider({ children }: { children: ReactNode }) {
  // One client per provider mount, never a module singleton: a module-level
  // client is shared by every request rendered in the same server process.
  const [queryClient] = useState(() => createAppQueryClient());
  useClearQueryCacheOnSessionChange(queryClient);
  return <QueryClientProvider client={queryClient}>{children}</QueryClientProvider>;
}
