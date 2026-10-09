import {
  computeReservationDateRange,
  type ReservationDateRangeInput,
} from "../reservationDateRange";

/**
 * Quick-filter values that cannot be expressed as a pure server query
 * (time-relative or null-column derived). These force a bounded client filter
 * over a server date window.
 */
const DERIVED_CLIENT_QUICK_FILTERS = [
  "next2hours",
  "no_show_risk",
  "needs_table",
] as const;

export type DerivedClientQuickFilter =
  (typeof DERIVED_CLIENT_QUICK_FILTERS)[number];

export function isDerivedClientQuickFilter(
  quickFilter: string,
): quickFilter is DerivedClientQuickFilter {
  return (DERIVED_CLIENT_QUICK_FILTERS as readonly string[]).includes(
    quickFilter,
  );
}

/**
 * True when the list must hydrate client-side (derived filters and/or
 * client-only text search). Server-mappable filters (today, waitlist, all,
 * date range, status) keep this false so the paged list path is used.
 *
 * After server-side search lands, pass `searchIsServerSide: true` so text
 * search no longer forces the client hydrate path.
 */
export function isClientOnlyReservationFiltersActive(
  search: string,
  quickFilter: string,
  options: { searchIsServerSide?: boolean } = {},
): boolean {
  const searchClient =
    !options.searchIsServerSide && search.trim() !== "";
  return searchClient || isDerivedClientQuickFilter(quickFilter);
}

/**
 * Map UI filter state onto the server list query.
 * Time window is solely dateFilter (+ optional custom range). Status is solely
 * statusFilter. Attention chips never rewrite the server window.
 */
export function resolveServerReservationFilters(
  _attentionFilter: string,
  dateFilter: string,
  statusFilter: string,
  now: Date = new Date(),
  maxAdvanceDays?: number,
  customRange?: { startDate?: string; endDate?: string },
  businessTimeZone?: string | null,
): { startDate?: string; endDate?: string; status: string } {
  const rangeInput: ReservationDateRangeInput = {
    dateFilter,
    now,
    maxAdvanceDays,
    customStartDate: customRange?.startDate,
    customEndDate: customRange?.endDate,
    businessTimeZone,
  };
  const range = computeReservationDateRange(rangeInput);
  return {
    startDate: range.startDate,
    endDate: range.endDate,
    status: statusFilter,
  };
}

/**
 * Whether the complete (multi-page hydrate) loader is needed.
 * List view with only server-mappable filters uses the paged path alone.
 */
export function shouldLoadCompleteReservations(
  clientOnlyFiltersActive: boolean,
  reservationView: "operations" | "list",
): boolean {
  return clientOnlyFiltersActive || reservationView === "operations";
}

/**
 * Whether the server-paginated list loader is needed.
 */
export function shouldLoadPagedReservations(
  clientOnlyFiltersActive: boolean,
  reservationView: "operations" | "list",
): boolean {
  return reservationView === "list" && !clientOnlyFiltersActive;
}
