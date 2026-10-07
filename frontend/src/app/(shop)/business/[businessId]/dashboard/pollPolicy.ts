/**
 * Global bills/orders/reservations reconcile poll policy (L3-12 / Root C).
 *
 * Allowlist of rails that surface live operational data and therefore need the
 * 60s full-replace poll. Everything else (Menú, analytics, config, AI, …) relies
 * on SSE for badges and must not background-poll.
 *
 * Inverted from the former POLL_PAUSED_TABS denylist so a new rail defaults to
 * "no poll" instead of silently opting in.
 */
export const POLL_ACTIVE_TABS = new Set([
  "overview",
  "bills",
  "cash-register",
  "kitchen",
  "counter",
  "tables",
  "reservations",
  "delivery",
]);

/** True when the 60s global bills/orders interval should run for `tab`. */
export function shouldScheduleGlobalBillPoll(tab: string): boolean {
  return POLL_ACTIVE_TABS.has(tab);
}
