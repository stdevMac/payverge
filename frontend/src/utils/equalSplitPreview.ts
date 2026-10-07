/**
 * Pure equal-split seat counting + floor preview (PG-23).
 *
 * FE dual of backend `equalSplitSeatsTakenTx` + `calculateEqualSplitCents`
 * in `backend/internal/database/bill_split.go`. No React — inject `nowMs`
 * so wall-clock is an explicit input (not component state).
 */

export const EQUAL_SHARE_META_KEY = "__equal_shares__";

export type EqualSplitShareLike = {
  mode?: string;
  status?: string;
  hold_expires_at?: string | null;
  claimed_fractions?: Record<string, string> | null;
};

/**
 * Count equal-mode seats already held or settled, matching BE
 * `equalSplitSeatsTakenTx`: skip held rows whose hold_expires_at is past
 * `nowMs` (status may still be "held" until the release job runs).
 */
export function countEqualSeatsTaken(
  shares: EqualSplitShareLike[] | null | undefined,
  nowMs: number,
): number {
  let seats = 0;
  for (const share of shares ?? []) {
    if (share.mode !== "equal") continue;
    if (share.status !== "held" && share.status !== "settled") continue;
    if (share.status === "held" && share.hold_expires_at) {
      const expiresMs = Date.parse(share.hold_expires_at);
      if (Number.isFinite(expiresMs) && expiresMs <= nowMs) {
        continue;
      }
    }
    let covered = 1;
    const raw = share.claimed_fractions?.[EQUAL_SHARE_META_KEY];
    if (raw != null && String(raw).trim() !== "") {
      const parsed = Number.parseInt(String(raw).trim(), 10);
      if (Number.isFinite(parsed) && parsed > 0) {
        covered = parsed;
      }
    }
    seats += covered;
  }
  return seats;
}

/**
 * Floor re-split of availableCents across remainingPeople for `covered` seats.
 * Dual of BE `calculateEqualSplitCents` (returns 0 instead of error for UI).
 */
export function equalPreviewCents(
  availableCents: number,
  remainingPeople: number,
  covered: number,
): number {
  if (
    !Number.isFinite(availableCents) ||
    availableCents <= 0 ||
    !Number.isFinite(remainingPeople) ||
    remainingPeople <= 0 ||
    !Number.isFinite(covered) ||
    covered <= 0 ||
    covered > remainingPeople
  ) {
    return 0;
  }
  const people = Math.trunc(remainingPeople);
  const cover = Math.trunc(covered);
  const available = Math.trunc(availableCents);
  if (cover === people) {
    return available;
  }
  let amount = Math.floor(available / people) * cover;
  if (amount <= 0) {
    // available < people: drain up to `cover` cents (BE penny-drain path).
    return available < cover ? available : cover;
  }
  return amount;
}

/** Preview dollars for equal mode given live seats + wall clock. */
export function equalPreviewDollars(args: {
  availableCents: number;
  requestedPeople: number;
  sharesCovered: number;
  shares: EqualSplitShareLike[] | null | undefined;
  nowMs: number;
}): number {
  const seatsTaken = countEqualSeatsTaken(args.shares, args.nowMs);
  const remainingPeople = args.requestedPeople - seatsTaken;
  if (remainingPeople <= 0) {
    return 0;
  }
  const covered = Math.min(
    Math.max(1, Math.trunc(args.sharesCovered) || 1),
    remainingPeople,
  );
  return equalPreviewCents(args.availableCents, remainingPeople, covered) / 100;
}
