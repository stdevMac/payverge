"use client";

import { useCallback, useEffect, useMemo, useRef } from "react";
import { useQuery, useQueryClient } from "@tanstack/react-query";
import { chatApi } from "@/api/chat";
import { queryKeys } from "@/api/queryKeys";
import { useSSEEvents, type SSEEvent } from "@/hooks/useSSEEvents";

/**
 * Total unread staff-chat messages across all channels, for the persistent
 * "Team" sidebar badge (Slice 7). Sourced from the same channels-list
 * endpoint/query key the comms tab uses (`queryKeys.chat.channels`), so both
 * consumers share one cache entry.
 *
 * `chat.message` / `chat.announcement` SSE nudges are content-free by design
 * (no message text or channel body — chat:read fans out per-business, and a
 * body would leak DMs), so we don't trust the payload: we invalidate and let
 * React Query refetch the real unread map.
 *
 * Fails silent to 0 (e.g. a staff role without chat:read gets a 403 from the
 * channels endpoint) — this badge must never crash or block the sidebar.
 *
 * The queryFn deliberately matches TeamChatPanel's exactly (throw on error,
 * default retry): both observers share this cache key, and whichever one
 * performs the fetch decides its semantics — swallowing the error here would
 * cache a bogus success and hide the comms tab's isError state. Fail-silence
 * lives in the read instead: on error `data` stays undefined → sum is 0.
 *
 * Unlike the comms tab (only mounted while it's open), this hook lives in the
 * always-mounted sidebar, so a busy channel would otherwise refetch on every
 * single `chat.message`. Trailing-edge debounce (2.5s) collapses a burst of
 * nudges into one refetch.
 */
const UNREAD_INVALIDATE_DEBOUNCE_MS = 2500;

export function useChatUnreadCount(businessId: number): number {
  const shouldFetch = businessId > 0;
  const queryClient = useQueryClient();
  const queryKey = queryKeys.chat.channels(String(businessId));
  const debounceRef = useRef<ReturnType<typeof setTimeout> | null>(null);

  const { data } = useQuery({
    queryKey,
    queryFn: () => chatApi.listChannels(String(businessId)),
    enabled: shouldFetch,
    staleTime: 30_000,
  });

  useEffect(() => {
    return () => {
      if (debounceRef.current) {
        clearTimeout(debounceRef.current);
        debounceRef.current = null;
      }
    };
  }, []);

  const onEvent = useCallback(
    (event: SSEEvent) => {
      if (event.type === "chat.message" || event.type === "chat.announcement") {
        if (debounceRef.current) return;
        debounceRef.current = setTimeout(() => {
          debounceRef.current = null;
          void queryClient.invalidateQueries({ queryKey });
        }, UNREAD_INVALIDATE_DEBOUNCE_MS);
      }
    },
    [queryClient, queryKey],
  );

  // Heal after an SSE gap: chat nudges that fired during the drop were never
  // delivered, so the badge would otherwise stay stale forever. A reconnect is
  // rare (not a burst), so skip the debounce and resync immediately.
  const onReconnect = useCallback(() => {
    if (debounceRef.current) {
      clearTimeout(debounceRef.current);
      debounceRef.current = null;
    }
    void queryClient.invalidateQueries({ queryKey });
  }, [queryClient, queryKey]);

  useSSEEvents({
    businessId,
    enabled: shouldFetch,
    onEvent,
    onReconnect,
  });

  return useMemo(() => {
    if (!data?.unread) return 0;
    return Object.values(data.unread).reduce((sum, n) => sum + n, 0);
  }, [data]);
}
