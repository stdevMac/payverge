import { QueryClient } from "@tanstack/react-query";

// Sane app-wide defaults: without these React Query uses staleTime:0 +
// refetchOnWindowFocus:true, so default-config queries refetch on every mount
// and tab refocus on a money dashboard that already polls + has SSE. Per-hook
// overrides (e.g. useBusinessAccess, useBusinessPageData) still win. (P-2)
//
// M1 — React Query is now the SINGLE retry owner. The axios interceptor no
// longer retries GETs (defaults `_maxRetries` to 0), so this `retry: 1` is the
// authoritative, online/focus-aware retry budget: 2 total attempts per query,
// not the previous 8-16 (RQ retries × axios's 3 exp-backoff retries) that made
// `isError` lag 30-60s during an outage. Keep this small and bounded.
export function createAppQueryClient(): QueryClient {
  return new QueryClient({
    defaultOptions: {
      queries: {
        staleTime: 30_000,
        refetchOnWindowFocus: false,
        retry: 1,
      },
    },
  });
}
