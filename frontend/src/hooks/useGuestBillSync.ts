"use client";

import { getPublicConfig } from "@/config/publicConfig";
import { useCallback, useEffect, useRef, useState } from "react";
import { getOpenBillByTableCode } from "@/api/bills";

const RECOVERY_POLL_MS = 15_000;
/** Consecutive transport errors before the stream is parked and polling takes over. */
export const MAX_CONSECUTIVE_STREAM_ERRORS = 5;
/** Jittered park window before a fresh EventSource is attempted (30–120s). */
export const STREAM_BACKOFF_MIN_MS = 30_000;
export const STREAM_BACKOFF_MAX_MS = 120_000;
/** EventSource.CLOSED; a literal so test doubles without the static still work. */
const EVENT_SOURCE_CLOSED = 2;

export function streamBackoffDelay(random: () => number = Math.random): number {
  const span = STREAM_BACKOFF_MAX_MS - STREAM_BACKOFF_MIN_MS;
  return STREAM_BACKOFF_MIN_MS + Math.floor(random() * span);
}

function isTerminalCapacityFrame(event: Event): boolean {
  const data = (event as MessageEvent).data;
  if (typeof data !== "string" || data === "") return false;
  try {
    return (JSON.parse(data) as { code?: unknown }).code === "capacity";
  } catch {
    return false;
  }
}

interface UseGuestBillSyncOptions {
  tableCode: string;
  hasActiveBill: boolean;
  onActiveBillChange: (hasActiveBill: boolean) => void;
  /** Fired on every `table.bill_changed` after active-state handling. */
  onBillEvent?: () => void;
}

export function useGuestBillSync({
  tableCode,
  hasActiveBill,
  onActiveBillChange,
  onBillEvent,
}: UseGuestBillSyncOptions) {
  const [connected, setConnected] = useState(false);
  // Bumped after a parked backoff window to open a fresh EventSource.
  const [streamAttempt, setStreamAttempt] = useState(0);
  const activeRef = useRef(hasActiveBill);
  const onChangeRef = useRef(onActiveBillChange);
  const onBillEventRef = useRef(onBillEvent);
  activeRef.current = hasActiveBill;
  onChangeRef.current = onActiveBillChange;
  onBillEventRef.current = onBillEvent;

  const recover = useCallback(async () => {
    try {
      const response = await getOpenBillByTableCode(tableCode);
      const nextActive = Boolean(response.bill);
      if (nextActive !== activeRef.current) onChangeRef.current(nextActive);
      return nextActive;
    } catch {
      return activeRef.current;
    }
  }, [tableCode]);

  useEffect(() => {
    if (typeof EventSource === "undefined") return;
    const base = getPublicConfig().apiUrl;
    const source = new EventSource(
      `${base}/guest/table/${encodeURIComponent(tableCode)}/events`,
      { withCredentials: true },
    );
    let consecutiveErrors = 0;
    let parked = false;
    let backoffTimer: number | undefined;
    // Stop the browser's automatic reconnect loop and let the consumer's
    // polling cover the gap; retry the stream once after a jittered window so
    // a full table hub is not hammered by every guest at the same instant.
    const park = () => {
      if (parked) return;
      parked = true;
      source.close();
      setConnected(false);
      void recover();
      backoffTimer = window.setTimeout(
        () => setStreamAttempt((attempt) => attempt + 1),
        streamBackoffDelay(),
      );
    };
    const apply = (event: MessageEvent) => {
      try {
        const payload = JSON.parse(event.data) as { has_active_bill?: boolean };
        if (typeof payload.has_active_bill === "boolean" && payload.has_active_bill !== activeRef.current) {
          onChangeRef.current(payload.has_active_bill);
        }
      } catch {
        void recover();
      }
    };
    source.addEventListener("connected", ((event: Event) => {
      consecutiveErrors = 0;
      setConnected(true);
      apply(event as MessageEvent);
    }) as EventListener);
    source.addEventListener("table.bill_changed", ((event: Event) => {
      apply(event as MessageEvent);
      onBillEventRef.current?.();
    }) as EventListener);
    // The server answers an over-cap subscribe with a terminal
    // `event: error` frame whose data carries code "capacity".
    source.addEventListener("error", ((event: Event) => {
      if (isTerminalCapacityFrame(event)) park();
    }) as EventListener);
    source.onopen = () => {
      consecutiveErrors = 0;
      setConnected(true);
    };
    source.onerror = () => {
      if (parked) return;
      setConnected(false);
      consecutiveErrors += 1;
      // A non-200 or non-event-stream response (a 429 from a rate limiter, a
      // proxy 502 during a deploy) closes the EventSource for good: the
      // browser never reconnects it. Park so the backoff opens a fresh one.
      if (
        source.readyState === EVENT_SOURCE_CLOSED ||
        consecutiveErrors >= MAX_CONSECUTIVE_STREAM_ERRORS
      ) {
        park();
        return;
      }
      // EventSource reconnects automatically; this recovery read closes any
      // event gap while the connection was down.
      void recover();
    };
    return () => {
      parked = true;
      if (backoffTimer !== undefined) window.clearTimeout(backoffTimer);
      setConnected(false);
      source.close();
    };
  }, [recover, tableCode, streamAttempt]);

  useEffect(() => {
    if (!hasActiveBill) return;
    const interval = window.setInterval(() => void recover(), RECOVERY_POLL_MS);
    const onFocus = () => void recover();
    window.addEventListener("focus", onFocus);
    return () => {
      window.clearInterval(interval);
      window.removeEventListener("focus", onFocus);
    };
  }, [hasActiveBill, recover]);

  return { refresh: recover, connected };
}
