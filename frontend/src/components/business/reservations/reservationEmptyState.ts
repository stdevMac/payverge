/**
 * Decide which empty-state branch the reservations list should show (L1-8).
 *
 * "Today" is a narrowing filter: an empty Today for a configured venue is not
 * "you have never used reservations" — it means no covers for this day. Only
 * a fully open window (all_time / all) with no search/status/attention may
 * claim first-run onboarding and jump to Settings.
 */
export type ReservationEmptyKind = "first_run" | "filtered" | "today_empty";

export function resolveReservationEmptyKind(params: {
  search: string;
  attentionFilter: string;
  statusFilter: string;
  dateFilter: string;
}): ReservationEmptyKind {
  const searchActive = params.search.trim() !== "";
  const attentionActive = params.attentionFilter !== "none";
  const statusActive = params.statusFilter !== "all";
  const dateIsOpen =
    params.dateFilter === "all" || params.dateFilter === "all_time";

  if (searchActive || attentionActive || statusActive) {
    return "filtered";
  }
  if (params.dateFilter === "today") {
    return "today_empty";
  }
  if (!dateIsOpen) {
    return "filtered";
  }
  return "first_run";
}
