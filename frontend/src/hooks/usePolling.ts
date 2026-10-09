import { useEffect, useRef, useCallback, useState } from 'react';
import { logError } from '@/utils/errorLogger';

interface UsePollingOptions {
  // The poll body. Receives an AbortSignal scoped to THIS cycle: it aborts when
  // the cycle is superseded by a newer one, when polling stops/pauses, or on
  // unmount. Callbacks that thread the signal into their API calls (bills.ts /
  // orders.ts accept an optional trailing signal) get free cancellation of
  // superseded and unmounted in-flight requests. Zero-arg callbacks are fine —
  // the param is optional.
  callback: (signal?: AbortSignal) => Promise<void> | void;
  interval?: number; // milliseconds
  enabled?: boolean;
  immediate?: boolean; // Run immediately on mount
  // Called after N consecutive failures so the caller can surface a toast or
  // pause polling. Receives the last error and the consecutive-failure count.
  onRepeatedFailure?: (error: unknown, consecutiveFailures: number) => void;
  // Threshold at which onRepeatedFailure fires. Defaults to 3 — enough to
  // absorb a single transient blip without silently hiding a real outage.
  failureThreshold?: number;
  // Pause the loop while the tab is backgrounded (document.hidden) and fire an
  // immediate catch-up poll the moment it becomes visible again. Defaults to
  // true: a 3s guest poll running in a hidden tab burns the edge rate-limit for
  // data nobody is looking at, and the operator/guest wants fresh data the
  // instant they return, not up to `interval` ms later. Opt out (false) only if
  // the caller genuinely needs background progress.
  pauseWhenHidden?: boolean;
}

// True only in a real browser with a foregrounded tab. On the server, or before
// the Visibility API exists, we treat the tab as visible so SSR/first-mount
// polling behaves exactly as it did before this guard existed.
const isDocumentHidden = (): boolean =>
  typeof document !== 'undefined' && document.visibilityState === 'hidden';

// Same ceiling as a 60s dashboard reconcile: never wait less than the caller's
// interval, never stretch a short guest poll past a minute.
export const POLL_MAX_BACKOFF_MS = 60_000;

// Guest split stream / SharedSSE pattern: after N consecutive failures grow
// the wait (capped exponential); always apply half-to-full jitter so two hooks
// that mounted together do not stay aligned.
export function nextPollDelayMs(
  intervalMs: number,
  consecutiveFailures: number,
  failureThreshold = 3,
  random: () => number = Math.random,
): number {
  const base = Math.max(0, intervalMs);
  const threshold = Math.max(1, failureThreshold);
  let ceiling = base;
  if (consecutiveFailures >= threshold) {
    const exponent = consecutiveFailures - threshold + 1;
    ceiling = Math.min(base * 2 ** exponent, Math.max(base, POLL_MAX_BACKOFF_MS));
  }
  return ceiling / 2 + random() * (ceiling / 2);
}

export const usePolling = ({
  callback,
  interval = 5000, // Default 5 seconds
  enabled = true,
  immediate = true,
  onRepeatedFailure,
  failureThreshold = 3,
  pauseWhenHidden = true,
}: UsePollingOptions) => {
  const intervalRef = useRef<NodeJS.Timeout | null>(null);
  // Back isPolling with state so consumers re-render when polling starts/stops.
  // A ref-derived value (intervalRef.current !== null) is non-reactive.
  const [isPolling, setIsPolling] = useState(false);
  const callbackRef = useRef(callback);
  const failuresRef = useRef(0);
  const failureCbRef = useRef(onRepeatedFailure);
  const thresholdRef = useRef(failureThreshold);
  const startPollingRef = useRef<() => void>(() => {});

  // Monotonic cycle id. Each runOnce claims the next generation; only the
  // newest generation is allowed to mutate the shared failure counter, so a
  // slow response can't rewrite state a newer poll already settled. Centralizes
  // the staleness guard that GuestBill/BillManager used to hand-roll.
  const genRef = useRef(0);
  // Set while a cycle is awaiting its callback. A new tick that lands before the
  // previous cycle resolves is skipped rather than stacked, so overlapping polls
  // can't pile up on a slow network.
  const inFlightRef = useRef(false);
  // AbortController for the in-flight cycle. Superseding, stopping, or unmounting
  // aborts it so the callback's threaded API request is cancelled instead of
  // resolving into a stale write.
  const activeControllerRef = useRef<AbortController | null>(null);

  // Update callback ref when callback changes — refs keep the setInterval
  // closure stable so the poll loop isn't torn down on every render.
  useEffect(() => {
    callbackRef.current = callback;
  }, [callback]);

  useEffect(() => {
    failureCbRef.current = onRepeatedFailure;
  }, [onRepeatedFailure]);

  useEffect(() => {
    thresholdRef.current = failureThreshold;
  }, [failureThreshold]);

  const isAbortError = (error: unknown): boolean =>
    (error instanceof DOMException && error.name === 'AbortError') ||
    (error instanceof Error && error.name === 'AbortError') ||
    (typeof error === 'object' &&
      error !== null &&
      (error as { code?: string }).code === 'ERR_CANCELED'); // axios cancel

  const runOnce = useCallback(async () => {
    // Skip-if-in-flight: never stack a second cycle on top of a still-pending
    // one. The existing interval keeps ticking; the next tick after this cycle
    // settles will pick up fresh data.
    if (inFlightRef.current) {
      return;
    }

    const gen = ++genRef.current;
    // Abort any lingering controller (defensive — a settled cycle clears its
    // own) and arm a fresh one scoped to this cycle.
    if (activeControllerRef.current) {
      activeControllerRef.current.abort();
    }
    const controller = new AbortController();
    activeControllerRef.current = controller;
    inFlightRef.current = true;

    try {
      await callbackRef.current(controller.signal);
      // Only the newest, un-aborted generation may settle shared state. A slow
      // response from a superseded cycle is silently discarded.
      if (gen !== genRef.current || controller.signal.aborted) {
        return;
      }
      const hadFailures = failuresRef.current > 0;
      failuresRef.current = 0;
      if (hadFailures) {
        startPollingRef.current();
      }
    } catch (error) {
      // A superseded/stopped/unmounted cycle aborting is expected, not a
      // failure — never count it or log it toward onRepeatedFailure.
      if (isAbortError(error) || controller.signal.aborted) {
        return;
      }
      // A superseded (but non-aborted) generation must not touch the counter.
      if (gen !== genRef.current) {
        return;
      }
      failuresRef.current += 1;
      void logError(error instanceof Error ? error : String(error), 'usePolling', 'poll');
      if (failuresRef.current >= thresholdRef.current && failureCbRef.current) {
        try {
          failureCbRef.current(error, failuresRef.current);
        } catch (cbErr) {
          void logError(cbErr instanceof Error ? cbErr : String(cbErr), 'usePolling', 'onRepeatedFailure');
        }
      }
      if (failuresRef.current >= thresholdRef.current) {
        startPollingRef.current();
      }
    } finally {
      // Release the in-flight lock only for the cycle that owns it. A superseded
      // cycle resolving late must not unlock the newer cycle that replaced it.
      if (gen === genRef.current) {
        inFlightRef.current = false;
      }
      if (activeControllerRef.current === controller) {
        activeControllerRef.current = null;
      }
    }
  }, []);

  const startPolling = useCallback(() => {
    if (intervalRef.current) {
      clearInterval(intervalRef.current);
    }
    // Honor visibility: while hidden with pause enabled, don't arm a timer —
    // the visibilitychange handler re-arms (and catches up) on return.
    if (pauseWhenHidden && isDocumentHidden()) {
      setIsPolling(false);
      return;
    }
    const delay = nextPollDelayMs(
      interval,
      failuresRef.current,
      thresholdRef.current,
    );
    intervalRef.current = setInterval(runOnce, delay);
    setIsPolling(true);
  }, [interval, runOnce, pauseWhenHidden]);

  const stopPolling = useCallback(() => {
    if (intervalRef.current) {
      clearInterval(intervalRef.current);
      intervalRef.current = null;
    }
    // Cancel any in-flight cycle so a request in progress doesn't resolve into a
    // stale write after the caller asked us to stop.
    genRef.current++;
    if (activeControllerRef.current) {
      activeControllerRef.current.abort();
      activeControllerRef.current = null;
    }
    inFlightRef.current = false;
    setIsPolling(false);
  }, []);

  // Track whether this is the first activation so an interval change restarts
  // the timer at the new rate WITHOUT firing an extra immediate fetch.
  const startedRef = useRef(false);

  // Mirror the current option values into refs so the STABLE visibilitychange
  // handler (installed once) reads live values without being re-bound on every
  // interval/enabled change.
  const enabledRef = useRef(enabled);
  const pauseWhenHiddenRef = useRef(pauseWhenHidden);
  useEffect(() => {
    enabledRef.current = enabled;
  }, [enabled]);
  useEffect(() => {
    pauseWhenHiddenRef.current = pauseWhenHidden;
  }, [pauseWhenHidden]);

  // Keep the latest startPolling in a ref for the same reason — the visibility
  // handler must call the current one (bound to the current interval), not a
  // stale closure captured when the listener was first attached. Assigned
  // during render so a failure/success re-arm inside runOnce never hits the
  // empty first-render stub.
  startPollingRef.current = startPolling;
  useEffect(() => {
    startPollingRef.current = startPolling;
  }, [startPolling]);

  // Pause on background, wake on foreground. Modeled on the SSE wake pattern
  // (useSSEEvents): browsers throttle/freeze background timers, and a hidden 3s
  // guest poll is pure waste — so we tear the timer down when hidden and fire an
  // immediate catch-up the instant the tab returns. Installed once; reads live
  // state via refs.
  useEffect(() => {
    if (typeof document === 'undefined') return;
    const handleVisibilityChange = () => {
      if (!pauseWhenHiddenRef.current || !enabledRef.current) return;
      if (document.visibilityState === 'hidden') {
        // Pause: drop the timer but keep isPolling semantics via stopPolling's
        // reset. Use the interval-only teardown (not stopPolling) so we don't
        // abort an in-flight cycle just for going to the background — let it
        // finish. The next visible tick supersedes it anyway.
        if (intervalRef.current) {
          clearInterval(intervalRef.current);
          intervalRef.current = null;
        }
        setIsPolling(false);
      } else {
        // Wake: immediate catch-up poll, then re-arm the timer at the current
        // rate. runOnce's in-flight guard makes a double-fire (visibility +
        // any lingering tick) a no-op.
        void runOnce();
        startPollingRef.current();
      }
    };
    document.addEventListener('visibilitychange', handleVisibilityChange);
    return () => {
      document.removeEventListener('visibilitychange', handleVisibilityChange);
    };
  }, [runOnce]);

  useEffect(() => {
    if (enabled) {
      // Only the initial enable honors `immediate`; a later interval change
      // just re-times the loop. Skip the immediate fetch while hidden — the
      // visibilitychange handler fires the catch-up when the tab returns.
      if (immediate && !startedRef.current && !(pauseWhenHidden && isDocumentHidden())) {
        void runOnce();
      }
      startedRef.current = true;
      startPolling();
    } else {
      startedRef.current = false;
      stopPolling();
    }

    return () => {
      // Clear the timer directly rather than calling stopPolling() — this
      // cleanup runs on unmount, where calling setState (setIsPolling) is a
      // no-op at best. The effect body re-syncs isPolling on any re-run.
      if (intervalRef.current) {
        clearInterval(intervalRef.current);
        intervalRef.current = null;
      }
    };
    // `interval` is a primitive, so it only lands here when the rate actually
    // changes (not on every render from inline options objects) — including it
    // is what lets an adaptive poll rate (e.g. GuestBill's 3s↔10s) restart the
    // timer instead of staying stuck at the mount-time interval.
  }, [enabled, immediate, interval, pauseWhenHidden, runOnce, startPolling, stopPolling]);

  // Abort any in-flight cycle on unmount so a late resolution never writes into
  // an unmounted tree. Separate from the timer-teardown effect above so it runs
  // exactly once at unmount, not on every interval change.
  useEffect(() => {
    // Capture ref objects (stable) so cleanup mutates through the same refs
    // without reading .current at effect-setup time for DOM-like teardown.
    const gen = genRef;
    const activeController = activeControllerRef;
    const inFlight = inFlightRef;
    return () => {
      gen.current += 1;
      if (activeController.current) {
        activeController.current.abort();
        activeController.current = null;
      }
      inFlight.current = false;
    };
  }, []);

  return {
    startPolling,
    stopPolling,
    isPolling,
  };
};
