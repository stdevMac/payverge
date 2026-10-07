/** Leading + trailing coalesce window for reservation SSE/mutation refreshes. */
export const RESERVATION_SSE_COALESCE_MS = 1500;

export type CoalesceDecision =
  | { action: "run_now" }
  | { action: "schedule_trailing"; delayMs: number }
  | { action: "covered_by_pending_trailing" };

/**
 * Decide how to handle a refresh trigger given last-run time and whether a
 * trailing timer is already armed (DispatchConsole pattern).
 */
export function decideReservationRefreshCoalesce(
  nowMs: number,
  lastLoadAtMs: number,
  hasPendingTrailing: boolean,
  windowMs: number = RESERVATION_SSE_COALESCE_MS,
): CoalesceDecision {
  if (hasPendingTrailing) {
    return { action: "covered_by_pending_trailing" };
  }
  const elapsed = nowMs - lastLoadAtMs;
  if (elapsed >= windowMs) {
    return { action: "run_now" };
  }
  return { action: "schedule_trailing", delayMs: windowMs - elapsed };
}
