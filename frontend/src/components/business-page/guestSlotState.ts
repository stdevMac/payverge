export type SlotState = "open" | "waitlist" | "unavailable";

export type RecommendableSlot = {
  time: string;
  available_tables: number;
  recommended?: boolean;
  reason_code?: string;
};

// Reason codes whose slots can never be booked OR waitlisted: the backend's
// window validation rejects these times before its waitlist fallback runs, so
// offering them as selectable guarantees a failed submission.
const HARD_UNAVAILABLE_REASONS = new Set([
  "outside_advance_window",
  "outside_operating_window",
]);

export function getGuestSlotState(
  slot: { available_tables: number; reason_code?: string },
  waitlistAvailable: boolean,
): SlotState {
  if (slot.available_tables > 0) {
    return "open";
  }
  if (
    waitlistAvailable &&
    !HARD_UNAVAILABLE_REASONS.has(slot.reason_code ?? "")
  ) {
    return "waitlist";
  }
  return "unavailable";
}

/**
 * Pick at most one open slot to badge as Recommended.
 *
 * A badge is only useful when slots differ. Fully equivalent open days, a
 * single open slot, waitlist-only times, and closed windows get no badge.
 * When capacities differ, prefer the soonest max-capacity slot (or the
 * backend's already-sparse mark among those max-capacity slots).
 */
export function pickRecommendedSlotTime(
  slots: RecommendableSlot[],
  waitlistAvailable: boolean,
): string | null {
  const open = slots.filter(
    (slot) => getGuestSlotState(slot, waitlistAvailable) === "open",
  );
  if (open.length < 2) {
    return null;
  }

  let minTables = open[0].available_tables;
  let maxTables = open[0].available_tables;
  for (const slot of open) {
    if (slot.available_tables < minTables) minTables = slot.available_tables;
    if (slot.available_tables > maxTables) maxTables = slot.available_tables;
  }

  if (minTables === maxTables) {
    return null;
  }

  const maxCapacity = open.filter((slot) => slot.available_tables === maxTables);
  const backendMarked = maxCapacity.filter((slot) => slot.recommended);
  const pool =
    backendMarked.length > 0 && backendMarked.length < maxCapacity.length
      ? backendMarked
      : maxCapacity;

  return [...pool].sort(
    (left, right) =>
      new Date(left.time).getTime() - new Date(right.time).getTime(),
  )[0]?.time ?? null;
}
