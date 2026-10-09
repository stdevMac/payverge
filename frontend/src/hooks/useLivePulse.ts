import { useCallback, useEffect, useRef, useState } from "react";
import { analyticsApi, type DashboardSummary,
  type LiveBill } from "@/api/analytics";
import { usePolling } from "@/hooks/usePolling";

/**
 * useLivePulse — ONE shared 10s poller for the "Today / Live" surface.
 *
 * Before this hook, TodayLivePanel polled getDashboardSummary every 10s and
 * LiveBills separately polled /analytics/live-bills every 10s — two overlapping
 * pollers computing the same live picture per open tab (§3.9). This consolidates
 * them: a single usePolling cycle fetches BOTH endpoints together (hidden-tab
 * pause preserved via usePolling), and both panels read from one source.
 *
 * The initial load sets `loading`; subsequent poll cycles refresh silently so
 * the numbers don't flash a spinner every 10s. `error` carries a raw error for
 * the caller to localize; on a transient blip the previous data stays visible.
 */
export interface LivePulse {
  summary: DashboardSummary | null;
  liveBills: LiveBill[];
  /** True when the server truncated the live-bills list at its LIMIT. */
  capped: boolean;
  lastUpdated: Date;
  loading: boolean;
  error: unknown;
  /** Manual refresh (shows loading like the initial load). */
  refresh: () => Promise<void>;
  isPolling: boolean;
}

export function useLivePulse(
  businessId: string,
  { intervalMs = 10000, enabled = true }: { intervalMs?: number; enabled?: boolean } = {},
): LivePulse {
  const [summary, setSummary] = useState<DashboardSummary | null>(null);
  const [liveBills, setLiveBills] = useState<LiveBill[]>([]);
  const [capped, setCapped] = useState(false);
  const [lastUpdated, setLastUpdated] = useState<Date>(() => new Date());
  const [loading, setLoading] = useState(true);
  const [error, setError] = useState<unknown>(null);

  // Request-id guard: only the newest cycle may write state. usePolling already
  // aborts superseded cycles via the signal, but a settled-but-superseded
  // response could still resolve — this keeps the last write authoritative.
  const genRef = useRef(0);

  const fetchAll = useCallback(
    async (isInitialLoad: boolean, signal?: AbortSignal) => {
      if (!businessId) return;
      const gen = ++genRef.current;
      try {
        if (isInitialLoad) setLoading(true);
        // One cycle, both reads in parallel.
        const [summaryRes, billsRes] = await Promise.all([
          analyticsApi.getDashboardSummary(businessId, signal),
          analyticsApi.getLiveBillsPage(businessId, signal),
        ]);
        if (gen !== genRef.current || signal?.aborted) return;
        setSummary(summaryRes);
        setLiveBills(billsRes.bills);
        setCapped(billsRes.capped);
        setLastUpdated(new Date());
        setError(null);
      } catch (err) {
        if (gen !== genRef.current || signal?.aborted) return;
        // Leave previous data in place; surface the error for the caller.
        setError(err);
      } finally {
        if (gen === genRef.current && isInitialLoad) setLoading(false);
      }
    },
    [businessId],
  );

  // Initial load.
  useEffect(() => {
    if (businessId && enabled) void fetchAll(true);
  }, [businessId, enabled, fetchAll]);

  const silentPoll = useCallback(
    (signal?: AbortSignal) => fetchAll(false, signal),
    [fetchAll],
  );

  const { isPolling } = usePolling({
    callback: silentPoll,
    interval: intervalMs,
    enabled: enabled && !!businessId,
    immediate: false, // initial load handled above
  });

  const refresh = useCallback(() => fetchAll(true), [fetchAll]);

  return { summary, liveBills, capped, lastUpdated, loading, error, refresh, isPolling };
}
