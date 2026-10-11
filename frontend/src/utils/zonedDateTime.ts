import { formatInTimeZone, fromZonedTime } from "date-fns-tz";

const WALL_FORMAT = "yyyy-MM-dd'T'HH:mm";

function assertTimeZone(timeZone: string): void {
  try {
    new Intl.DateTimeFormat("en", { timeZone }).format(0);
  } catch {
    throw new Error("invalid_time_zone");
  }
}

export function instantToWallTime(
  value: Date | string,
  timeZone: string,
): string {
  assertTimeZone(timeZone);
  const instant = value instanceof Date ? value : new Date(value);
  if (Number.isNaN(instant.getTime())) throw new Error("invalid_instant");
  return formatInTimeZone(instant, timeZone, WALL_FORMAT);
}

export function wallTimeToInstant(wallTime: string, timeZone: string): Date {
  assertTimeZone(timeZone);
  if (!/^\d{4}-\d{2}-\d{2}T\d{2}:\d{2}$/.test(wallTime)) {
    throw new Error("invalid_wall_time");
  }

  const instant = fromZonedTime(`${wallTime}:00`, timeZone);
  if (formatInTimeZone(instant, timeZone, WALL_FORMAT) !== wallTime) {
    throw new Error("wall_time_does_not_exist");
  }
  return instant;
}

export function switchWallTimeZone(
  wallTime: string,
  fromTimeZone: string,
  toTimeZone: string,
): string {
  return instantToWallTime(
    wallTimeToInstant(wallTime, fromTimeZone),
    toTimeZone,
  );
}

export type OperatingDayWindow = {
  /** Wall-clock open time HH:mm in the business timezone. */
  openHHMM: string;
  /** Wall-clock close time HH:mm in the business timezone. */
  closeHHMM: string;
};

function parseHHMM(value: string): { hour: number; minute: number } | null {
  const m = /^(\d{1,2}):(\d{2})$/.exec(value.trim());
  if (!m) return null;
  const hour = Number(m[1]);
  const minute = Number(m[2]);
  if (
    !Number.isFinite(hour) ||
    !Number.isFinite(minute) ||
    hour < 0 ||
    hour > 23 ||
    minute < 0 ||
    minute > 59
  ) {
    return null;
  }
  return { hour, minute };
}

/**
 * Resolves the operating window for one local calendar day.
 *
 * `localDate` is `yyyy-MM-dd` in the business timezone. Return `null` for a
 * closed day.
 */
export type OperatingWindowResolver = (
  localDate: string,
) => OperatingDayWindow | null;

/**
 * Shift a `yyyy-MM-dd` local date string by whole days. Pure calendar math: the
 * UTC constructor and accessors are only a proleptic-Gregorian calculator here,
 * never a timezone conversion — in and out are both business-local date strings.
 */
function addLocalDays(localDate: string, days: number): string {
  const [year, month, day] = localDate.split("-").map(Number);
  const shifted = new Date(Date.UTC(year, month - 1, day));
  shifted.setUTCDate(shifted.getUTCDate() + days);
  return [
    String(shifted.getUTCFullYear()).padStart(4, "0"),
    String(shifted.getUTCMonth() + 1).padStart(2, "0"),
    String(shifted.getUTCDate()).padStart(2, "0"),
  ].join("-");
}

/**
 * Instant at the start of a local day. Midnight does not exist in every zone on
 * every date (DST gaps), so probe forward an hour at a time.
 */
function localDayStartInstant(localDate: string, timeZone: string): Date | null {
  for (const hh of ["00", "01", "02", "03"]) {
    try {
      return wallTimeToInstant(`${localDate}T${hh}:00`, timeZone);
    } catch {
      // Civil time skipped by DST — try the next hour.
    }
  }
  return null;
}

/**
 * First bookable wall time: now + minAdvance, rounded up to the slot step.
 *
 * When operating hours are provided (L1-17), the candidate is clamped into
 * open..lastStart for the candidate local day; if it falls after the last
 * start that still fits duration+buffer before close — or on a closed day —
 * it rolls forward day by day to the next open slot.
 *
 * `serviceMinutes` (default 0) is duration + service buffer. Backend rejects
 * starts that cannot finish before close with outside_operating_window; the
 * picker must not offer those slots.
 *
 * `operatingWindow` may be a single window applied to every day, or a resolver
 * keyed by the candidate's local date (R2-11). Businesses with per-day hours
 * MUST pass the resolver: passing one day's window while the candidate sits on
 * another day is exactly the bug that seeded the create form with today's date
 * and tomorrow's hours, which the backend rejects as outside_operating_window.
 */
export function nextBookableWallTime(
  now: Date,
  minAdvanceMinutes: number,
  intervalMinutes: number,
  timeZone: string,
  operatingWindow?: OperatingDayWindow | OperatingWindowResolver | null,
  options?: { maxDaysForward?: number; serviceMinutes?: number },
): string {
  const step = Math.max(intervalMinutes, 5) * 60_000;
  const earliest = now.getTime() + Math.max(minAdvanceMinutes, 0) * 60_000;
  let candidate = new Date(Math.ceil(earliest / step) * step);

  if (!operatingWindow) {
    return instantToWallTime(candidate, timeZone);
  }

  const resolveWindow: OperatingWindowResolver =
    typeof operatingWindow === "function"
      ? operatingWindow
      : () => operatingWindow;

  const maxDays = Math.max(1, options?.maxDaysForward ?? 14);
  const serviceMs = Math.max(0, options?.serviceMinutes ?? 0) * 60_000;

  // Roll the candidate to the start of the local day after `datePart`, snapped
  // back onto the slot grid.
  const rollToNextLocalDay = (datePart: string): Date => {
    const nextStart = localDayStartInstant(
      addLocalDays(datePart, 1),
      timeZone,
    );
    const base =
      nextStart && nextStart.getTime() > candidate.getTime()
        ? nextStart
        : new Date(candidate.getTime() + 24 * 60 * 60_000);
    return new Date(Math.ceil(base.getTime() / step) * step);
  };

  for (let dayOffset = 0; dayOffset < maxDays; dayOffset++) {
    const wall = instantToWallTime(candidate, timeZone); // yyyy-MM-dd'T'HH:mm
    const datePart = wall.slice(0, 10);

    const window = resolveWindow(datePart);
    if (!window) {
      // Closed day — the DATE has to move, not just the time.
      candidate = rollToNextLocalDay(datePart);
      continue;
    }

    const openParts = parseHHMM(window.openHHMM);
    const closeParts = parseHHMM(window.closeHHMM);
    if (!openParts || !closeParts) {
      return instantToWallTime(candidate, timeZone);
    }

    const openWall = `${datePart}T${String(openParts.hour).padStart(2, "0")}:${String(openParts.minute).padStart(2, "0")}`;
    const closeWall = `${datePart}T${String(closeParts.hour).padStart(2, "0")}:${String(closeParts.minute).padStart(2, "0")}`;

    let openInstant: Date;
    let closeInstant: Date;
    try {
      openInstant = wallTimeToInstant(openWall, timeZone);
      closeInstant = wallTimeToInstant(closeWall, timeZone);
    } catch {
      // Skip impossible local civil times (DST gaps).
      candidate = rollToNextLocalDay(datePart);
      continue;
    }

    // Last start that still fits a full seating (duration+buffer) before close.
    // serviceMinutes=0 → any start strictly before close (previous behavior).
    const lastStartMs = closeInstant.getTime() - serviceMs;

    // Before open → snap to open (still rounded to the slot step).
    if (candidate.getTime() < openInstant.getTime()) {
      const snapped = new Date(Math.ceil(openInstant.getTime() / step) * step);
      // If ceil pushed past last start, this day has no usable slot.
      if (snapped.getTime() > lastStartMs || snapped.getTime() >= closeInstant.getTime()) {
        candidate = rollToNextLocalDay(datePart);
        continue;
      }
      return instantToWallTime(snapped, timeZone);
    }

    // Inside bookable window (start + service fits before close) → accept.
    if (candidate.getTime() <= lastStartMs && candidate.getTime() < closeInstant.getTime()) {
      return instantToWallTime(candidate, timeZone);
    }

    // After last bookable start → try the next local day with that day's own hours.
    candidate = rollToNextLocalDay(datePart);
  }

  return instantToWallTime(candidate, timeZone);
}
