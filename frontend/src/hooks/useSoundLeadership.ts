import { useEffect, useRef, type MutableRefObject } from "react";

/**
 * Cross-tab sound leadership. Exactly one tab per business should PLAY alert
 * sounds / post browser notifications (visuals render in every tab from its
 * own SSE stream). The Web Locks API gives us this in a few lines: the tab
 * holding the exclusive lock is the leader; when it closes, the browser hands
 * the lock to the next waiter. No BroadcastChannel, no heartbeats.
 *
 * Returns a ref (not state) — leadership changes must not re-render consumers;
 * the sound path just reads it at play time. Falls back to leader=true when
 * Web Locks is unavailable (each tab sounds, i.e. today's behavior).
 *
 * `onAcquire` (optional) fires when THIS tab becomes the leader — e.g. the
 * previous leader tab closed mid-alarm and the browser handed us the lock.
 * Consumers that schedule ongoing audio (repeating alarms) use it to
 * re-evaluate, since the ref alone never triggers a re-render/effect.
 * Identity changes to the callback are absorbed via a ref so they don't
 * re-run the lock effect (which would thrash leadership).
 */
export function useSoundLeadership(
  businessId: number,
  onAcquire?: () => void,
): MutableRefObject<boolean> {
  const isLeader = useRef(false);
  const onAcquireRef = useRef(onAcquire);
  onAcquireRef.current = onAcquire;

  useEffect(() => {
    const locks =
      typeof navigator !== "undefined"
        ? (navigator as Navigator & { locks?: LockManager }).locks
        : undefined;
    if (!locks?.request) {
      isLeader.current = true;
      onAcquireRef.current?.();
      return () => {
        isLeader.current = false;
      };
    }

    isLeader.current = false;
    let cancelled = false;
    let released: (() => void) | null = null;
    const releasedPromise = new Promise<void>((resolve) => {
      released = resolve;
    });

    void locks
      .request(`payverge-alert-sound-${businessId}`, () => {
        // The grant can arrive AFTER this effect run was cleaned up (e.g.
        // businessId switched while another tab still held the old lock). A
        // stale grant must not claim leadership: releasedPromise is already
        // resolved, so the lock is instantly handed back — flipping the ref
        // here would leave a still-mounted component "leading" with nothing.
        if (!cancelled) {
          isLeader.current = true;
          onAcquireRef.current?.();
        }
        return releasedPromise; // hold the lock until unmount
      })
      .catch(() => {
        // Lock request aborted (e.g. page teardown) — never a leader.
      });

    return () => {
      cancelled = true;
      isLeader.current = false;
      released?.();
    };
  }, [businessId]);

  return isLeader;
}
