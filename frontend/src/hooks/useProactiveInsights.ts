import { useCallback, useEffect, useState } from "react";

import {
  getDirectorProactiveInsights,
  type ProactiveInsightDTO,
} from "@/api/directorConsole";
import { useSSEEvents, type SSEEvent } from "@/hooks/useSSEEvents";

/**
 * Loads the operator dashboard's proactive-insights briefings and keeps them
 * fresh in realtime.
 *
 * The stuck-bill watchdog (backend `internal/jobs/stuck_bill_watchdog.go`)
 * publishes a `bill.stuck` SSE event specifically so this briefings panel
 * refetches the moment a bill crosses the stale threshold, instead of the
 * "N open bills are over 2 hours old" alert only appearing on a remount. That
 * hookup was previously missing on the frontend — the event was emitted into
 * the void and the insights were fetched once on mount with no refresh — so
 * this hook subscribes to the shared SSE stream and refetches on `bill.stuck`.
 * It also refetches on reconnect (mirroring Kitchen / DispatchConsole) to
 * recover anything missed during an SSE drop, since events are not buffered
 * server-side across the outage.
 *
 * Gated by `enabled` (the briefings are AI-Growth-owner-only); when disabled or
 * without a businessId it neither fetches nor opens an SSE subscription.
 */
export function useProactiveInsights(
  businessId: number | undefined,
  enabled: boolean,
): {
  insights: ProactiveInsightDTO[];
  loading: boolean;
  refetch: () => void;
} {
  const [insights, setInsights] = useState<ProactiveInsightDTO[]>([]);
  const [loading, setLoading] = useState(false);

  const load = useCallback(() => {
    if (!businessId || !enabled) return;
    setLoading(true);
    getDirectorProactiveInsights(businessId)
      .then((res) => setInsights(res.insights ?? []))
      .catch(() => setInsights([]))
      .finally(() => setLoading(false));
  }, [businessId, enabled]);

  useEffect(() => {
    if (!businessId || !enabled) {
      setInsights([]);
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

  return { insights, loading, refetch: load };
}
