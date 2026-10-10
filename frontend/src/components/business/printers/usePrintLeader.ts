"use client";

import { useEffect, useRef, type MutableRefObject } from "react";
import { randomUUID } from "@/lib/randomUUID";

const BC_PREFIX = "payverge-print-leader-";
const LOCK_PREFIX = "payverge-print-leader-";

/**
 * Cross-tab print leadership for one business. Exactly one tab should poll the
 * browser print claim endpoint. Mirrors useSoundLeadership (Web Locks) and
 * adds a BroadcastChannel fallback when Web Locks is unavailable so only one
 * tab sets isLeader=true even without locks.
 *
 * Returns a ref (not state) — leadership flips must not re-render the agent
 * shell on every handoff; the poll loop reads the ref each tick.
 */
export function usePrintLeader(
  businessId: number,
  onAcquire?: () => void,
): MutableRefObject<boolean> {
  const isLeader = useRef(false);
  const onAcquireRef = useRef(onAcquire);
  onAcquireRef.current = onAcquire;

  useEffect(() => {
    if (!businessId || businessId <= 0) {
      isLeader.current = false;
      return;
    }

    const lockName = `${LOCK_PREFIX}${businessId}`;
    const locks =
      typeof navigator !== "undefined"
        ? (navigator as Navigator & { locks?: LockManager }).locks
        : undefined;

    if (locks?.request) {
      isLeader.current = false;
      let cancelled = false;
      let released: (() => void) | null = null;
      const releasedPromise = new Promise<void>((resolve) => {
        released = resolve;
      });

      void locks
        .request(lockName, () => {
          if (!cancelled) {
            isLeader.current = true;
            onAcquireRef.current?.();
          }
          return releasedPromise;
        })
        .catch(() => {
          /* aborted */
        });

      return () => {
        cancelled = true;
        isLeader.current = false;
        released?.();
      };
    }

    // BroadcastChannel fallback: first tab to claim wins; others yield.
    // A heartbeat every 2s lets a new tab take over if the leader dies.
    type Msg = { type: "claim" | "heartbeat" | "release"; tabId: string; at: number };
    const tabId = `${Date.now()}-${Math.random().toString(36).slice(2)}`;
    let channel: BroadcastChannel | null = null;
    let heartbeat: ReturnType<typeof setInterval> | null = null;
    let lastOtherLeader = 0;
    let cancelled = false;

    const becomeLeader = () => {
      if (cancelled || isLeader.current) return;
      isLeader.current = true;
      onAcquireRef.current?.();
      channel?.postMessage({ type: "claim", tabId, at: Date.now() } satisfies Msg);
    };

    const yieldLeadership = () => {
      isLeader.current = false;
    };

    try {
      channel = new BroadcastChannel(`${BC_PREFIX}${businessId}`);
      channel.onmessage = (ev: MessageEvent<Msg>) => {
        const msg = ev.data;
        if (!msg || msg.tabId === tabId) return;
        if (msg.type === "claim" || msg.type === "heartbeat") {
          lastOtherLeader = msg.at;
          // Deterministic yield: lower tabId wins if both claim; otherwise yield to heartbeat.
          if (isLeader.current && msg.tabId < tabId) {
            yieldLeadership();
          } else if (!isLeader.current) {
            // Stay follower while we see a recent leader.
          }
        }
        if (msg.type === "release") {
          // Leader left — wait a beat then try to claim.
          setTimeout(() => {
            if (!cancelled && Date.now() - lastOtherLeader > 2500) becomeLeader();
          }, 50);
        }
      };
    } catch {
      // BroadcastChannel unavailable — every tab leads (degraded, same as sound).
      becomeLeader();
      return () => {
        isLeader.current = false;
      };
    }

    // Initial claim after a short jitter so simultaneous mounts don't thrash.
    const claimTimer = setTimeout(becomeLeader, Math.floor(Math.random() * 40));
    heartbeat = setInterval(() => {
      if (cancelled) return;
      if (isLeader.current) {
        channel?.postMessage({ type: "heartbeat", tabId, at: Date.now() } satisfies Msg);
      } else if (Date.now() - lastOtherLeader > 4000) {
        becomeLeader();
      }
    }, 2000);

    return () => {
      cancelled = true;
      clearTimeout(claimTimer);
      if (heartbeat) clearInterval(heartbeat);
      if (isLeader.current) {
        channel?.postMessage({ type: "release", tabId, at: Date.now() } satisfies Msg);
      }
      isLeader.current = false;
      channel?.close();
    };
  }, [businessId]);

  return isLeader;
}

const CLIENT_ID_KEY = "payverge-print-client-id";

/** Stable UUID per browser profile for lease ownership across reloads. */
export function getOrCreatePrintClientId(): string {
  if (typeof window === "undefined") return "";
  try {
    const existing = window.localStorage.getItem(CLIENT_ID_KEY);
    if (existing && /^[0-9a-f-]{36}$/i.test(existing)) return existing;
    const id = randomUUID();
    window.localStorage.setItem(CLIENT_ID_KEY, id);
    return id;
  } catch {
    return `fallback-${Date.now()}`;
  }
}
