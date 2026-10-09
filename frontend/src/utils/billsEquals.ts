import type { Bill } from "@/api/bills";

/**
 * M-hygiene: shallow-compares two active-bill lists by the fields a 60s
 * reconcile (or an SSE-triggered refetch) can change in a board-relevant way:
 * id, status, paid_amount, total_amount, updated_at. The dashboard uses this so
 * `globalBills` keeps a stable reference across no-op polls — otherwise the
 * bills leg always allocated a fresh array (`setGlobalBills([...billsList])`),
 * whose identity change re-fired the BillManager live-refresh effect and forced
 * a redundant second server fetch every cycle.
 *
 * Mirrors ordersMapEquals. Order is significant (both lists come from the same
 * server ordering), so a positional walk is sufficient and cheap.
 */
export function billsEquals(a: Bill[], b: Bill[]): boolean {
  if (a === b) return true;
  if (a.length !== b.length) return false;
  for (let i = 0; i < a.length; i += 1) {
    const x = a[i];
    const y = b[i];
    if (
      x.id !== y.id ||
      x.status !== y.status ||
      x.paid_amount !== y.paid_amount ||
      x.total_amount !== y.total_amount ||
      x.updated_at !== y.updated_at
    ) {
      return false;
    }
  }
  return true;
}
