import { useCallback, useEffect, useRef, useState } from "react";

import { useSSEEvents, type SSEEvent } from "@/hooks/useSSEEvents";
import { isActiveBillStatus } from "@/api/bills";

// A guest-initiated payment (payment.pending) that is never confirmed — an
// abandoned crypto/alternative-payment flow — must not leave a stale "awaiting
// confirmation" badge forever. Drop it after this window; a real confirmation
// clears it sooner via payment.received / a paid bill.updated.
const PENDING_TTL_MS = 10 * 60 * 1000;

/**
 * Tracks bills that have a guest-initiated payment awaiting operator
 * confirmation, surfaced as a subtle row badge in the bills surface. State is
 * intentionally ephemeral (client-only, not persisted): a page reload clears it,
 * which is correct — the operator re-reads live bill state on load. A pending
 * bill self-expires after PENDING_TTL_MS so an abandoned flow doesn't strand the
 * badge, and clears immediately on payment.received, payment request
 * cancellation/rejection, bill.closed, or a bill.updated that moves the bill
 * to a settled (non-active) status.
 */
export function usePendingPaymentBills(
  businessId: number,
  enabled = true,
): Set<number> {
  const [pending, setPending] = useState<Set<number>>(() => new Set());
  const timersRef = useRef<Map<number, ReturnType<typeof setTimeout>>>(
    new Map(),
  );

  const clearBill = useCallback((billId: number) => {
    const timers = timersRef.current;
    const timer = timers.get(billId);
    if (timer) {
      clearTimeout(timer);
      timers.delete(billId);
    }
    setPending((prev) => {
      if (!prev.has(billId)) return prev;
      const next = new Set(prev);
      next.delete(billId);
      return next;
    });
  }, []);

  const handleEvent = useCallback(
    (event: SSEEvent) => {
      const data = event.data as {
        bill_id?: number;
        id?: number;
        status?: string;
      };
      const billId = typeof data.bill_id === "number" ? data.bill_id : data.id;
      if (typeof billId !== "number") return;

      switch (event.type) {
        case "payment.pending": {
          setPending((prev) => {
            if (prev.has(billId)) return prev;
            const next = new Set(prev);
            next.add(billId);
            return next;
          });
          const timers = timersRef.current;
          const existing = timers.get(billId);
          if (existing) clearTimeout(existing);
          timers.set(
            billId,
            setTimeout(() => clearBill(billId), PENDING_TTL_MS),
          );
          break;
        }
        case "payment.received":
        case "payment.request.resolved":
        case "bill.closed":
          clearBill(billId);
          break;
        case "bill.updated":
          // Only a transition to a settled (non-active) status means the pending
          // payment resolved. An unrelated bill.updated (e.g. a menu edit) keeps
          // the badge until confirmation or the TTL.
          if (data.status && !isActiveBillStatus(data.status)) {
            clearBill(billId);
          }
          break;
      }
    },
    [clearBill],
  );

  useSSEEvents({ businessId, enabled, onEvent: handleEvent });

  useEffect(() => {
    const timers = timersRef.current;
    return () => {
      for (const timer of timers.values()) clearTimeout(timer);
      timers.clear();
    };
  }, []);

  return pending;
}
