import { localDateKey } from "@/lib/localDate";
import { businessDateKey } from "@/utils/businessTime";

export type ReservationDateRangeInput = {
  dateFilter: string;
  now?: Date;
  maxAdvanceDays?: number;
  /** YYYY-MM-DD — used when dateFilter is "custom". */
  customStartDate?: string;
  /** YYYY-MM-DD — used when dateFilter is "custom". */
  customEndDate?: string;
  /**
   * Business IANA timezone (L1-10). When set, "today"/horizon day keys use the
   * restaurant calendar day instead of the operator device day so Overview
   * and Reservas count the same bookings.
   */
  businessTimeZone?: string | null;
};

/**
 * Legacy date-filter keys mapped onto the surviving option (R2-B6).
 *
 * After L1-7 the picker carried two entries with identical labels and
 * identical bounds — "all" and "all_time". The "all" item is gone; any state
 * still holding it (a restored preference, a deep link) must resolve to
 * "all_time" or the Select finds no matching item and renders blank.
 */
export function normalizeReservationDateFilter(dateFilter: string): string {
  return dateFilter === "all" ? "all_time" : dateFilter;
}

function dayKey(now: Date, businessTimeZone?: string | null): string {
  if (businessTimeZone) {
    const key = businessDateKey(now, businessTimeZone);
    if (key) return key;
  }
  return localDateKey(now);
}

/** Add whole calendar days in absolute time then re-key in the chosen zone. */
function offsetDayKey(
  now: Date,
  dayDelta: number,
  businessTimeZone?: string | null,
): string {
  const shifted = new Date(now.getTime() + dayDelta * 24 * 60 * 60_000);
  return dayKey(shifted, businessTimeZone);
}

/**
 * Server date-range (YYYY-MM-DD) for the today/upcoming/past/custom reservation
 * filters, keyed on the business calendar day when `businessTimeZone` is set
 * (L1-10), otherwise the device-local calendar day.
 *
 * Using UTC (toISOString slicing) here resolved to tomorrow for Americas
 * evenings, so the "today" range queried the wrong day and hid tonight's
 * reservations (audit L6 #16). Pure + exported so it is unit-testable without
 * the heavy ReservationManager component graph.
 */
export function computeReservationDateRange(input: ReservationDateRangeInput): {
  startDate?: string;
  endDate?: string;
} {
  const now = input.now ?? new Date();
  const maxAdvanceDays = input.maxAdvanceDays;
  const dateFilter = input.dateFilter;
  const tz = input.businessTimeZone;

  if (dateFilter === "today") {
    const key = dayKey(now, tz);
    return { startDate: key, endDate: key };
  }
  if (dateFilter === "upcoming") {
    // Cover the whole bookable horizon: guests can book up to
    // max_advance_days out, and anything past the window is invisible in the
    // dashboard until it drifts into range (30-day floor; the backend treats
    // end_date as end-of-day, so the boundary day is fully included).
    const horizon = Math.max(30, maxAdvanceDays ?? 0);
    return {
      startDate: dayKey(now, tz),
      endDate: offsetDayKey(now, horizon, tz),
    };
  }
  if (dateFilter === "past") {
    return {
      startDate: offsetDayKey(now, -30, tz),
      endDate: dayKey(now, tz),
    };
  }
  if (dateFilter === "all_time" || dateFilter === "all") {
    // "Todo (incluye pasadas)" (L1-28) and the former open-horizon "all" option
    // (L1-7): reach past and orphaned rows. Both bounds must be explicit —
    // the server defaults an absent start_date to today (hiding the past) and
    // derives an absent end_date from start_date, which here would land the
    // window in the year 2000. Sending undefined for "all" made it identical
    // to "upcoming" while the label claimed a default open horizon.
    const horizon = Math.max(30, maxAdvanceDays ?? 0);
    return { startDate: "2000-01-01", endDate: offsetDayKey(now, horizon, tz) };
  }
  if (dateFilter === "custom") {
    const start = input.customStartDate?.trim() || undefined;
    const end = input.customEndDate?.trim() || undefined;
    // Honor partial ranges (from-only / to-only) so operators can open history
    // without both pickers filled. If neither is set, fall through to open
    // range (backend defaults to today→horizon).
    if (start || end) {
      // If both present and inverted, swap so the server BETWEEN still works.
      if (start && end && start > end) {
        return { startDate: end, endDate: start };
      }
      return { startDate: start, endDate: end };
    }
    return { startDate: undefined, endDate: undefined };
  }
  // Unknown filters: open range (backend defaults to today→horizon).
  return { startDate: undefined, endDate: undefined };
}
