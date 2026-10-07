import { getApiErrorStatus } from "@/utils/apiError";

/** Consecutive failed dashboard live-reconcile cycles before the stale banner. */
export const STALE_BANNER_THRESHOLD = 2;

/**
 * A cycle is clean only when EVERY live-refresh source that this verdict
 * includes succeeded. A mixed 502 + 200 is a stale floor (#623): bills can
 * be dead while reservations 200, and OR-healing hid the amber strip.
 *
 * `crmOk` is optional so an off-rail cycle (CRM never probed) does not have
 * to invent a CRM 200. `undefined` means "not in this verdict".
 */
export function isReconcileClean(legs: {
  billsOk: boolean;
  ordersOk: boolean;
  reservationsOk: boolean;
  crmOk?: boolean;
}): boolean {
  if (!legs.billsOk || !legs.ordersOk || !legs.reservationsOk) return false;
  if (legs.crmOk === undefined) return true;
  return legs.crmOk;
}

/**
 * Axios/fetch 429s must not increment the stale-banner counter.
 *
 * The shared axios interceptor sanitizes into a plain Error that copies
 * `.status` / `.response.status`. `RateLimitCooldownError` is a typed miss
 * with no status field — classify by name, never by message text.
 */
export function isRateLimitedReconcileError(error: unknown): boolean {
  if (Number(getApiErrorStatus(error)) === 429) return true;
  if (!error || typeof error !== "object") return false;
  const named = error as { name?: unknown; status?: unknown };
  if (named.name === "RateLimitCooldownError") return true;
  return named.status === "429";
}

/** One live-board leg's outcome for a single reconcile cycle. */
export type ReconcileLegOutcome = {
  ok: boolean;
  rateLimited: boolean;
  /**
   * Legs the cycle deliberately did not probe (CRM off-rail) are not
   * evidence either way and must not speak for the cycle.
   */
  skipped?: boolean;
};

export const RECONCILE_LEG_OK: ReconcileLegOutcome = {
  ok: true,
  rateLimited: false,
};

export function skippedReconcileLeg(): ReconcileLegOutcome {
  return { ok: false, rateLimited: false, skipped: true };
}

export function failedReconcileLeg(error: unknown): ReconcileLegOutcome {
  return { ok: false, rateLimited: isRateLimitedReconcileError(error) };
}

export function reconcileLegFromSettled(
  settled: PromiseSettledResult<ReconcileLegOutcome>,
): ReconcileLegOutcome {
  if (settled.status === "fulfilled") return settled.value;
  return failedReconcileLeg(settled.reason);
}

/**
 * Whether the whole cycle can be treated as backpressure rather than an
 * outage: true only when EVERY leg that actually missed missed with a 429.
 *
 * Anything weaker lets a sibling veto a real outage. A bills 502 riding
 * with a CRM 429 is not backpressure — the live board is still dead.
 * A leg that returned 200, and a leg the cycle never probed, are both
 * silent here; only misses get a vote.
 */
function isRateLimitedReconcileCycle(
  legs: readonly ReconcileLegOutcome[],
): boolean {
  const missed = legs.filter((leg) => !leg.skipped && !leg.ok);
  if (missed.length === 0) return false;
  return missed.every((leg) => leg.rateLimited);
}

export type ReconcileCycleLegs = {
  bills: ReconcileLegOutcome;
  orders: ReconcileLegOutcome;
  reservations: ReconcileLegOutcome;
  crm: ReconcileLegOutcome;
};

/**
 * One dashboard live-reconcile verdict. Page and tests share this so a
 * sibling 200 cannot drift the counter off a still-failing rail.
 */
export function applyReconcileCycle(
  prevCount: number,
  legs: ReconcileCycleLegs,
): { nextCount: number; allOk: boolean; rateLimited: boolean } {
  const allOk = isReconcileClean({
    billsOk: legs.bills.ok,
    ordersOk: legs.orders.ok,
    reservationsOk: legs.reservations.ok,
    crmOk: legs.crm.skipped ? undefined : legs.crm.ok,
  });
  const rateLimited = isRateLimitedReconcileCycle([
    legs.bills,
    legs.orders,
    legs.reservations,
    legs.crm,
  ]);
  return {
    allOk,
    rateLimited,
    nextCount: nextReconcileFailureCount(prevCount, allOk, { rateLimited }),
  };
}

/**
 * A clean cycle (every probed source 200) always heals. Hard misses
 * increment. Rate-limited misses HOLD the count — they are not recoveries.
 *
 * `opts.rateLimited` means "every leg that missed this cycle missed with a
 * 429" (see isRateLimitedReconcileCycle) — never "some leg 429'd".
 *
 * Decay used to walk a pinned banner off after two 502s and one dinner-rush
 * 429, with zero fresh reads (#623). Holding from 0 keeps a 429-only storm
 * from painting the strip; holding from the threshold keeps the strip up
 * until the failed sources actually 200.
 */
export function nextReconcileFailureCount(
  prev: number,
  allOk: boolean,
  opts?: { rateLimited?: boolean },
): number {
  if (allOk) return 0;
  if (opts?.rateLimited) return prev;
  return prev + 1;
}

export function shouldShowStaleBanner(
  failureCount: number,
  dismissed: boolean,
): boolean {
  return failureCount >= STALE_BANNER_THRESHOLD && !dismissed;
}
