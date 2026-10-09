"use client";

import React, { createContext, useContext, useEffect, useState, useCallback, useRef } from "react";

interface ConnectivityState {
  isOnline: boolean;
  since: Date | null;
}

const ConnectivityContext = createContext<ConnectivityState>({
  isOnline: true,
  since: null,
});

export function useConnectivity() {
  return useContext(ConnectivityContext);
}

// Bounded backoff for confirming reachability after an `online` event. The
// browser only fires `online`/`offline` on hard transitions, so a single
// failed probe used to latch the offline banner forever on a flaky-but-up
// connection. Retry a few times before giving up.
const CONFIRM_RETRY_DELAYS_MS = [1000, 2000, 4000, 8000];

export function ConnectivityProvider({ children }: { children: React.ReactNode }) {
  const [isOnline, setIsOnline] = useState(true);
  const [since, setSince] = useState<Date | null>(null);
  const debounceRef = useRef<ReturnType<typeof setTimeout> | null>(null);
  const retryRef = useRef<ReturnType<typeof setTimeout> | null>(null);
  const epochRef = useRef(0);

  const confirmOnline = useCallback(async (epoch: number) => {
    try {
      // Ping our OWN origin's health route to confirm reachability. The backend
      // path "/api/v1/health/live" does not exist on the frontend origin (there
      // is no /api/v1 rewrite), so it 404'd — confirmOnline always returned
      // false and the offline banner never cleared after reconnecting. "/api/health"
      // is the Next route handler (GET → HEAD auto-handled) that returns 200.
      const res = await fetch("/api/health", {
        method: "HEAD",
        cache: "no-store",
        signal: AbortSignal.timeout(5000),
      });
      if (epochRef.current !== epoch) return false;
      return res.ok;
    } catch {
      return false;
    }
  }, []);

  const handleChange = useCallback(
    (online: boolean) => {
      if (debounceRef.current) clearTimeout(debounceRef.current);
      if (retryRef.current) clearTimeout(retryRef.current);
      const epoch = ++epochRef.current;

      // Attempt a confirmation probe; on failure schedule a bounded backoff
      // retry instead of giving up (which previously latched the banner).
      const attemptConfirm = async (attempt: number) => {
        if (epochRef.current !== epoch) return;
        const confirmed = await confirmOnline(epoch);
        if (epochRef.current !== epoch) return;
        if (confirmed) {
          setIsOnline(true);
          setSince(new Date());
          return;
        }
        if (attempt < CONFIRM_RETRY_DELAYS_MS.length) {
          retryRef.current = setTimeout(
            () => void attemptConfirm(attempt + 1),
            CONFIRM_RETRY_DELAYS_MS[attempt],
          );
        }
      };

      debounceRef.current = setTimeout(() => {
        if (online) {
          void attemptConfirm(0);
        } else {
          setIsOnline(false);
          setSince(new Date());
        }
      }, 500);
    },
    [confirmOnline],
  );

  useEffect(() => {
    if (typeof window === "undefined") return;

    setIsOnline(navigator.onLine);

    const onOnline = () => handleChange(true);
    const onOffline = () => handleChange(false);

    window.addEventListener("online", onOnline);
    window.addEventListener("offline", onOffline);
    return () => {
      window.removeEventListener("online", onOnline);
      window.removeEventListener("offline", onOffline);
      if (debounceRef.current) clearTimeout(debounceRef.current);
      if (retryRef.current) clearTimeout(retryRef.current);
    };
  }, [handleChange]);

  return (
    <ConnectivityContext.Provider value={{ isOnline, since }}>
      {children}
    </ConnectivityContext.Provider>
  );
}
