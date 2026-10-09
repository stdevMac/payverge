import { localDateKey } from "@/lib/localDate";

/**
 * After creating a reservation on a non-today day, the default "today" filter
 * would hide the new row (looks like a silent failure). Flip to "upcoming"
 * unless the operator is already on a multi-day window.
 */
export function shouldFlipToUpcomingAfterCreate(params: {
  reservationTime: string | Date;
  dateFilter: string;
  now?: Date;
}): boolean {
  const now = params.now ?? new Date();
  const reservationDay = localDateKey(new Date(params.reservationTime));
  if (reservationDay === localDateKey(now)) {
    return false;
  }
  return (
    params.dateFilter !== "upcoming" &&
    params.dateFilter !== "all" &&
    params.dateFilter !== "all_time"
  );
}

/**
 * When the date filter flips, memoized loaders re-key and load effects re-run.
 * A concurrent scheduleReservationRefresh() still closes over the pre-flip
 * range and can race (last writer wins with the wrong window). Skip the manual
 * refresh when the flip already re-keys loaders (L1-6).
 */
export function shouldScheduleRefreshAfterCreate(flippedFilter: boolean): boolean {
  return !flippedFilter;
}
