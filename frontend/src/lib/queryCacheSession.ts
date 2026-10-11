"use client";

import { useEffect } from "react";
import type { QueryClient } from "@tanstack/react-query";
import { apiCache } from "@/utils/cache";

/**
 * Clear every React Query entry when the session ends (logout) or one
 * principal replaces another (staff/owner switch on a shared device), so no
 * cached response of the previous user is rendered to the next one.
 */
export function useClearQueryCacheOnSessionChange(queryClient: QueryClient): void {
  useEffect(
    () => apiCache.onSessionChange(() => queryClient.clear()),
    [queryClient],
  );
}
