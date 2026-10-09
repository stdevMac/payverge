import { instantToWallTime } from "@/utils/zonedDateTime";

/** Venue-local lunch window used for the week-header coverage chips (#143). */
export const LUNCH_WINDOW = { startMinute: 11 * 60, endMinute: 15 * 60 };
/** Venue-local dinner window used for the week-header coverage chips (#143). */
export const DINNER_WINDOW = { startMinute: 17 * 60, endMinute: 22 * 60 };

export type CoverageWindow = {
  startMinute: number;
  endMinute: number;
};

function parseWallMinutes(wall: string): number | null {
  const match = /T(\d{2}):(\d{2})$/.exec(wall);
  if (!match) return null;
  const hour = Number(match[1]);
  const minute = Number(match[2]);
  if (!Number.isFinite(hour) || !Number.isFinite(minute)) return null;
  return hour * 60 + minute;
}

function overlapMinutes(
  start: number,
  end: number,
  window: CoverageWindow,
): number {
  return Math.max(0, Math.min(end, window.endMinute) - Math.max(start, window.startMinute));
}

/**
 * Minutes of one shift that land inside `window` on `dayKey` (YYYY-MM-DD)
 * in the venue timezone. Overnight tails that cross into the next day are
 * clipped to this day's 00:00–24:00.
 */
export function shiftWindowOverlapMinutes(
  startsAt: string,
  endsAt: string,
  dayKey: string,
  timeZone: string,
  window: CoverageWindow,
): number {
  let startWall: string;
  let endWall: string;
  try {
    startWall = instantToWallTime(startsAt, timeZone);
    endWall = instantToWallTime(endsAt, timeZone);
  } catch {
    return 0;
  }

  const startDay = startWall.slice(0, 10);
  const endDay = endWall.slice(0, 10);
  if (endDay < dayKey || startDay > dayKey) return 0;

  const startMinutes =
    startDay === dayKey ? (parseWallMinutes(startWall) ?? 0) : 0;
  const endMinutes =
    endDay === dayKey ? (parseWallMinutes(endWall) ?? 0) : 24 * 60;
  if (endMinutes <= startMinutes) return 0;
  return overlapMinutes(startMinutes, endMinutes, window);
}

export function daypartCoverageMinutes(
  shifts: Array<{ starts_at: string; ends_at: string }>,
  dayKey: string,
  timeZone: string,
  window: CoverageWindow,
): number {
  return shifts.reduce(
    (sum, shift) =>
      sum +
      shiftWindowOverlapMinutes(
        shift.starts_at,
        shift.ends_at,
        dayKey,
        timeZone,
        window,
      ),
    0,
  );
}
