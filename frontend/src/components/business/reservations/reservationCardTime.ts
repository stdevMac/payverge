/**
 * Whether the active list window spans more than one local calendar day (L1-14).
 * Kanban cards use time-only when the window is a single day; multi-day windows
 * need a day prefix so operators can tell which day a card belongs to.
 */
export function rangeSpansMultipleDays(
  startDate?: string,
  endDate?: string,
): boolean {
  if (!startDate || !endDate) {
    // Open / partial ranges are multi-day by definition for display purposes.
    return true;
  }
  return startDate !== endDate;
}

/**
 * Pick the display mode for a reservation card time under the active range.
 */
export function reservationCardTimeMode(
  startDate?: string,
  endDate?: string,
): "time_only" | "date_time" {
  return rangeSpansMultipleDays(startDate, endDate) ? "date_time" : "time_only";
}
