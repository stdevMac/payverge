import type {
  CreateReservationRequest,
  Reservation,
  UpdateReservationRequest,
} from "@/api/reservations";
import type { Locale } from "@/i18n/config";
import { surfaceBackendError } from "@/utils/localizedError";
import { isRawValidatorDump } from "@/utils/apiError";
import { localDateKey } from "@/lib/localDate";
import { businessDateKey } from "@/utils/businessTime";
import {
  wallTimeToInstant,
  type OperatingDayWindow,
} from "@/utils/zonedDateTime";
import {
  isActiveArrivalStatus,
  isNextArrivalCandidate,
  reservationInstant,
} from "./reservations/reservationArrivalClock";

// Re-export for callers/tests that import from this module (FIND-030).
export { isRawValidatorDump };

/** Venue wall clock → RFC3339 UTC. America/New_York 7pm is 23:00Z / 00:00Z, not 19:00Z. */
export function reservationWallTimeToISO(
  wallTime: string,
  timeZone: string,
): string {
  return wallTimeToInstant(wallTime, timeZone).toISOString();
}

const toDateTimeLocalString = (date: Date): string => {
  const offset = date.getTimezoneOffset();
  return new Date(date.getTime() - offset * 60_000).toISOString().slice(0, 16);
};

/**
 * First bookable slot for the operator create modal: now + min advance,
 * rounded UP to the next slot-interval boundary (local wall clock). The old
 * default of plain "now" was always rejected by the backend's min-advance
 * validation, which is a big part of why creating reservations "looked
 * broken" from the dashboard.
 */
export function nextBookableSlotLocal(
  now: Date,
  minAdvanceMinutes: number,
  slotIntervalMinutes: number,
): string {
  const interval =
    slotIntervalMinutes && slotIntervalMinutes >= 5 ? slotIntervalMinutes : 30;
  const earliest = new Date(
    now.getTime() + Math.max(minAdvanceMinutes || 0, 0) * 60_000,
  );
  const intervalMs = interval * 60_000;
  // Round in local-shifted epoch so boundaries land on local wall-clock times.
  const localMs = earliest.getTime() - earliest.getTimezoneOffset() * 60_000;
  const roundedLocalMs = Math.ceil(localMs / intervalMs) * intervalMs;
  const rounded = new Date(
    roundedLocalMs + earliest.getTimezoneOffset() * 60_000,
  );
  return toDateTimeLocalString(rounded);
}

type OperatingHoursRow = {
  day_of_week: number;
  open_time?: string | null;
  close_time?: string | null;
  is_closed?: boolean;
};

/**
 * Build the per-day operating-window resolver the create-form slot picker needs
 * (R2-11 / L1-17).
 *
 * The previous shape picked the next open day's hours and handed them to
 * `nextBookableWallTime` as a single daily window — which then applied them to
 * TODAY. On a day the business is closed that produced a seeded time the
 * backend rejects with `outside_operating_window`. Keying the lookup by the
 * candidate's local date lets the picker roll the DATE forward as well, and
 * gets per-day hours right for free.
 *
 * Returns `null` when nothing in the schedule is bookable, so callers skip the
 * clamp entirely instead of walking the whole horizon of closed days.
 */
export function operatingWindowResolver(
  hours: ReadonlyArray<OperatingHoursRow>,
): ((localDate: string) => OperatingDayWindow | null) | null {
  const windowFor = (row: OperatingHoursRow | undefined) => {
    if (!row || row.is_closed) return null;
    const open = (row.open_time ?? "").slice(0, 5);
    const close = (row.close_time ?? "").slice(0, 5);
    if (!open || !close) return null;
    return { openHHMM: open, closeHHMM: close };
  };

  if (!hours.some((row) => windowFor(row))) {
    return null;
  }

  return (localDate: string) => {
    // Noon keeps the weekday stable regardless of the device's UTC offset.
    const day = new Date(`${localDate}T12:00:00`);
    if (Number.isNaN(day.getTime())) return null;
    return windowFor(hours.find((row) => row.day_of_week === day.getDay()));
  };
}

/** Clamp a party size to the business's configured bounds. */
export function clampPartySize(
  value: number,
  minPartySize?: number,
  maxPartySize?: number,
): number {
  const min = minPartySize && minPartySize > 0 ? minPartySize : 1;
  const max = maxPartySize && maxPartySize >= min ? maxPartySize : Infinity;
  return Math.min(Math.max(value, min), max);
}

/**
 * Operator-facing error message for reservation mutations (L1-4 / Wave C).
 * Delegates to the shared {@link surfaceBackendError} convention so coded
 * 400s localize and uncoded CO-1 domain copy still surfaces.
 *
 * Raw gin validator dumps are NOT display copy — fall back instead (FIND-030).
 */
export function reservationErrorMessage(
  error: unknown,
  fallback: string,
  locale: Locale = "en",
): string {
  return surfaceBackendError(error, locale, fallback);
}

/**
 * L1-11: when party size grows past the selected table's capacity, drop the
 * selection and signal so the operator sees *why* (not a silent English toast).
 * Keep the selection when capacity still fits.
 */
export function reconcileTableForPartySize(
  tableId: number | undefined | null,
  partySize: number,
  tables: ReadonlyArray<{ id: number; capacity: number }>,
): { tableId: number | undefined; cleared: boolean } {
  if (tableId == null) {
    return { tableId: undefined, cleared: false };
  }
  const table = tables.find((t) => t.id === tableId);
  if (!table) {
    // Options still loading or table not in list — keep selection.
    return { tableId, cleared: false };
  }
  if (table.capacity < partySize) {
    return { tableId: undefined, cleared: true };
  }
  return { tableId, cleared: false };
}

type ReservationEditForm = Pick<
  CreateReservationRequest,
  | "table_id"
  | "customer_name"
  | "customer_phone"
  | "customer_email"
  | "party_size"
  | "reservation_time"
  | "duration"
  | "special_requests"
  | "notes"
>;

const sameInstant = (localValue: string, isoValue: string): boolean => {
  const left = new Date(localValue);
  const right = new Date(isoValue);
  if (Number.isNaN(left.getTime()) || Number.isNaN(right.getTime())) {
    return false;
  }
  return left.getTime() === right.getTime();
};

/**
 * Diff the edit form against the loaded reservation and send only what
 * changed. Resending every field made the backend treat each edit as an
 * availability change (re-running booking-window validation), and the old
 * `table_id || undefined` shape made it impossible to ever clear a table.
 */
export function buildReservationUpdatePayload(
  form: ReservationEditForm,
  reservation: Reservation,
): UpdateReservationRequest {
  const payload: UpdateReservationRequest = {};

  if (form.customer_name !== reservation.customer_name) {
    payload.customer_name = form.customer_name;
  }
  if ((form.customer_phone || "") !== (reservation.customer_phone || "")) {
    payload.customer_phone = form.customer_phone;
  }
  if ((form.customer_email || "") !== (reservation.customer_email || "")) {
    payload.customer_email = form.customer_email;
  }
  if ((form.special_requests || "") !== (reservation.special_requests || "")) {
    payload.special_requests = form.special_requests;
  }
  if ((form.notes || "") !== (reservation.notes || "")) {
    payload.notes = form.notes;
  }
  if (form.party_size !== reservation.party_size) {
    payload.party_size = form.party_size;
  }
  if (form.duration && form.duration !== reservation.duration) {
    payload.duration = form.duration;
  }

  const formTableId = form.table_id ?? null;
  const currentTableId = reservation.table_id ?? null;
  if (formTableId !== currentTableId) {
    if (formTableId === null) {
      payload.clear_table = true;
    } else {
      payload.table_id = formTableId;
    }
  }

  if (
    form.reservation_time &&
    !sameInstant(form.reservation_time, reservation.reservation_time)
  ) {
    payload.reservation_time = new Date(form.reservation_time).toISOString();
  }

  return payload;
}

/**
 * Merge loaded reservation books (upcoming API, list page, board hydrate, …)
 * without dropping a row the host can already see. Next-arrival must not
 * prefer a leftover today hydrate over the Próximas / upcoming book.
 */
export function mergeReservationBooks<T extends { id: number }>(
  ...books: readonly T[][]
): T[] {
  const seen = new Set<number>();
  const merged: T[] = [];
  for (const book of books) {
    for (const row of book) {
      if (seen.has(row.id)) continue;
      seen.add(row.id);
      merged.push(row);
    }
  }
  return merged;
}

export type NextUpcomingArrivalOptions<
  T extends { reservation_time: string; status: string; id?: number } = {
    reservation_time: string;
    status: string;
    id?: number;
  },
> = {
  /** Venue IANA zone — naive reservation_time strings are wall clocks here. */
  businessTimeZone?: string | null;
  /**
   * Rows already on the host list (Próximas). Confirmed/pending/waitlist
   * stay KPI candidates even if the device clock marks them late/expired.
   */
  listedReservations?: readonly T[];
};

function isListedActiveArrival<
  T extends { reservation_time: string; status: string; id?: number },
>(reservation: T, listed: readonly T[] | undefined): boolean {
  if (!listed || listed.length === 0) return false;
  if (!isActiveArrivalStatus(reservation.status)) return false;
  if (reservation.id != null) {
    return listed.some(
      (row) => row.id === reservation.id && isActiveArrivalStatus(row.status),
    );
  }
  return listed.includes(reservation);
}

/**
 * Next arrival for the Reservations KPI card: the earliest ACTIVE reservation
 * still on the host clock (future, or pending/confirmed inside no-show grace).
 * A confirmed row the host can already see on Próximas is never dropped
 * just because the device TZ expired it.
 */
export function nextUpcomingArrival<
  T extends { reservation_time: string; status: string; id?: number },
>(
  reservations: T[],
  now: Date,
  graceMinutes?: number | null,
  options?: NextUpcomingArrivalOptions<T>,
): T | undefined {
  const zone = options?.businessTimeZone;
  return reservations
    .filter(
      (reservation) =>
        isListedActiveArrival(reservation, options?.listedReservations) ||
        isNextArrivalCandidate(reservation, now, graceMinutes, zone),
    )
    .sort(
      (left, right) =>
        reservationInstant(left.reservation_time, zone).getTime() -
        reservationInstant(right.reservation_time, zone).getTime(),
    )[0];
}

/** Party-size sum for pending/confirmed/waitlist rows on the venue day. */
export function coversOnBusinessDay<
  T extends { reservation_time: string; status: string; party_size?: number },
>(
  reservations: T[],
  todayKey: string,
  businessTimeZone?: string | null,
): number {
  return reservations.reduce((sum, row) => {
    if (!isActiveArrivalStatus(row.status)) return sum;
    const instant = reservationInstant(row.reservation_time, businessTimeZone);
    const key =
      businessDateKey(instant, businessTimeZone ?? null) ||
      localDateKey(instant);
    if (key !== todayKey) return sum;
    const party = Number(row.party_size ?? 0);
    return sum + (Number.isFinite(party) ? party : 0);
  }, 0);
}

/**
 * KPI label for the next arrival: time plus the party name so the strip
 * cannot collapse to "Ninguna" / "None" while the host can already see
 * the booking on Próximas.
 */
export function formatNextArrivalInsight(
  reservation: { customer_name?: string; party_size?: number },
  formattedTime: string,
): string {
  const name = reservation.customer_name?.trim();
  if (name) {
    return `${formattedTime} · ${name}`;
  }
  if (reservation.party_size && reservation.party_size > 0) {
    return `${formattedTime} · ${reservation.party_size}`;
  }
  return formattedTime;
}
