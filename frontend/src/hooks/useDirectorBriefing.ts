import { useCallback, useEffect, useState } from "react";

import {
  getDirectorBriefing,
  type BriefingResponse,
} from "@/api/directorConsole";
import { useSSEEvents, type SSEEvent } from "@/hooks/useSSEEvents";

/**
 * Loads the Director Console's always-on GM briefing and keeps it fresh in
 * realtime.
 *
 * Mirrors {@link useProactiveInsights}: the briefing folds the proactive
 * insights into a single owner-gated round-trip, so the stuck-bill watchdog's
 * `bill.stuck` SSE event must refetch the WHOLE briefing (its "Needs you"
 * section reuses the insight builder) the moment a bill crosses the stale
 * threshold, instead of only on a remount. It also refetches on reconnect
 * (mirroring Kitchen / DispatchConsole) to recover anything missed during an
 * SSE drop, since events are not buffered server-side across the outage.
 *
 * The last-good briefing is retained on a fetch error rather than cleared, so a
 * transient failure never collapses the front door to a void — the briefing is
 * the always-present container by design.
 *
 * Gated by `enabled` (the briefing is AI-Growth-owner-only); when disabled or
 * without a businessId it neither fetches nor opens an SSE subscription.
 */
export function useDirectorBriefing(
  businessId: number | undefined,
  enabled: boolean,
): {
  briefing: BriefingResponse | null;
  loading: boolean;
  error: boolean;
  /** Instant the last successful briefing fetch landed — venue "as of" stamp. */
  fetchedAt: Date | null;
  refetch: () => void;
} {
  const [briefing, setBriefing] = useState<BriefingResponse | null>(null);
  const [loading, setLoading] = useState(false);
  const [error, setError] = useState(false);
  const [fetchedAt, setFetchedAt] = useState<Date | null>(null);

  const load = useCallback(() => {
    if (!businessId || !enabled) return;
    setLoading(true);
    getDirectorBriefing(businessId)
      .then((res) => {
        setBriefing(res);
        setFetchedAt(new Date());
        setError(false);
      })
      // Keep the last-good briefing (and its stamp) on error (no void front door).
      .catch(() => setError(true))
      .finally(() => setLoading(false));
  }, [businessId, enabled]);

  useEffect(() => {
    if (!businessId || !enabled) {
      setBriefing(null);
      setFetchedAt(null);
      return;
    }
    load();
  }, [businessId, enabled, load]);

  const handleSSEEvent = useCallback(
    (event: SSEEvent) => {
      if (event.type === "bill.stuck") load();
    },
    [load],
  );

  useSSEEvents({
    businessId: businessId ?? 0,
    enabled: enabled && !!businessId,
    onEvent: handleSSEEvent,
    onReconnect: load,
  });

  return { briefing, loading, error, fetchedAt, refetch: load };
}
