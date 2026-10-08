import type { StaffAvailability, TimeOffRequest } from "@/api/availability";
import type { Shift } from "@/api/schedule";

/**
 * Availability overlay + conflict logic for the manager schedule builder.
 *
 * Two very different data shapes feed the overlay:
 *  - Availability is a RECURRING per-weekday pattern (weekday 0..6, minute-of-day
 *    windows, kind preferred|unavailable). The same windows apply to any week.
 *  - Time-off is an ABSOLUTE instant range ([starts_at, ends_at)); only approved
 *    requests matter for the overlay.
 *
 * Everything here uses the SAME local-date convention the grid uses to bucket
 * shifts (`localDateKey(new Date(iso))` / `new Date(y, m-1, d, h, mi)`), so a
 * time-off chip lands on exactly the cell a shift on that day would.
 *
 * Pure functions only — no React, no network — so the date/minute math is unit
 * tested in isolation.
 */

export type TeamAvailability = Record<number, StaffAvailability[]>;

/** Parse "HH:MM" into minute-of-day (0..1440). Returns null on malformed input. */
export function parseTimeToMinutes(time: string): number | null {
  const m = time.trim().match(/^(\d{1,2}):(\d{2})$/);
  if (!m) return null;
  const h = Number(m[1]);
  const min = Number(m[2]);
  if (h < 0 || h > 23 || min < 0 || min > 59) return null;
  return h * 60 + min;
}

/**
 * Weekday (0=Sun..6=Sat) for a YYYY-MM-DD day key, using the local-date
 * convention — matches `StaffAvailability.weekday` (JS getDay semantics).
 */
export function weekdayForDayKey(dayKey: string): number {
  const [y, m, d] = dayKey.split("-").map(Number);
  return new Date(y, m - 1, d).getDay();
}

/** Local [dayStart, nextDayStart) bounds for a day key. */
function dayBounds(dayKey: string): [Date, Date] {
  const [y, m, d] = dayKey.split("-").map(Number);
  return [new Date(y, m - 1, d, 0, 0, 0, 0), new Date(y, m - 1, d + 1, 0, 0, 0, 0)];
}

/** Half-open range overlap: [aStart,aEnd) ∩ [bStart,bEnd) ≠ ∅. */
function rangesOverlap(
  aStart: number,
  aEnd: number,
  bStart: number,
  bEnd: number,
): boolean {
  return aStart < bEnd && bStart < aEnd;
}

export interface DayAvailability {
  /** The staff's availability windows on this day's weekday, in input order. */
  windows: StaffAvailability[];
  hasPreferred: boolean;
  hasUnavailable: boolean;
  /** An "unavailable" window spans the whole day (00:00–24:00). */
  allDayUnavailable: boolean;
}

/** The staff member's availability posture on a given day. */
export function dayAvailability(
  staffId: number | null,
  dayKey: string,
  team: TeamAvailability,
): DayAvailability {
  const empty: DayAvailability = {
    windows: [],
    hasPreferred: false,
    hasUnavailable: false,
    allDayUnavailable: false,
  };
  if (staffId == null) return empty;
  const weekday = weekdayForDayKey(dayKey);
  const windows = (team[staffId] ?? []).filter((w) => w.weekday === weekday);
  if (windows.length === 0) return empty;
  return {
    windows,
    hasPreferred: windows.some((w) => w.kind === "preferred"),
    hasUnavailable: windows.some((w) => w.kind === "unavailable"),
    allDayUnavailable: windows.some(
      (w) => w.kind === "unavailable" && w.start_min <= 0 && w.end_min >= 1440,
    ),
  };
}

/**
 * The approved time-off request covering this local day for this staff, or null.
 * Day-granular: an approved request whose instant range intersects the calendar
 * day (in local time) covers it — matching how the cell represents a whole day.
 */
export function dayApprovedTimeOff(
  staffId: number | null,
  dayKey: string,
  approved: TimeOffRequest[],
): TimeOffRequest | null {
  if (staffId == null) return null;
  const [dayStart, dayEnd] = dayBounds(dayKey);
  for (const req of approved) {
    if (req.staff_id !== staffId || req.status !== "approved") continue;
    const start = new Date(req.starts_at);
    const end = new Date(req.ends_at);
    if (start.getTime() < dayEnd.getTime() && end.getTime() > dayStart.getTime()) {
      return req;
    }
  }
  return null;
}

/** The single dominant overlay posture for an employee cell (one per cell). */
export type CellPosture =
  | { kind: "timeoff"; request: TimeOffRequest }
  | { kind: "unavailable"; allDay: boolean; windows: StaffAvailability[] }
  | { kind: "preferred"; windows: StaffAvailability[] }
  | { kind: "none" };

/**
 * Resolve the dominant posture for one employee/day cell. Precedence:
 * approved time-off > unavailable > preferred. One indicator per cell keeps the
 * grid calm.
 */
export function cellPosture(
  staffId: number | null,
  dayKey: string,
  team: TeamAvailability,
  approved: TimeOffRequest[],
): CellPosture {
  if (staffId == null) return { kind: "none" };
  const timeoff = dayApprovedTimeOff(staffId, dayKey, approved);
  if (timeoff) return { kind: "timeoff", request: timeoff };
  const day = dayAvailability(staffId, dayKey, team);
  if (day.hasUnavailable) {
    return {
      kind: "unavailable",
      allDay: day.allDayUnavailable,
      windows: day.windows.filter((w) => w.kind === "unavailable"),
    };
  }
  if (day.hasPreferred) {
    return {
      kind: "preferred",
      windows: day.windows.filter((w) => w.kind === "preferred"),
    };
  }
  return { kind: "none" };
}

type ShiftConflictKind = "timeoff" | "unavailable" | "doublebook";

export interface ShiftConflict {
  kind: ShiftConflictKind;
  /** For "unavailable": the overlapping window (minute-of-day range). */
  window?: { start_min: number; end_min: number };
  /** For "timeoff": the covering approved request. */
  request?: TimeOffRequest;
  /** For "doublebook": the existing shift that overlaps this one. */
  shift?: Shift;
}

export interface ShiftConflictInput {
  staffId: number | null;
  dayKey: string;
  startTime: string; // "HH:MM"
  endTime: string; // "HH:MM"
  /**
   * Existing shifts to check for a same-staff time overlap (double-booking).
   * When omitted, no double-booking check runs (back-compat).
   */
  existingShifts?: Shift[];
  /**
   * A shift id to skip in the double-booking check — the shift currently being
   * edited must not conflict with itself.
   */
  ignoreShiftId?: number;
}

/** Absolute [start, end) instants for a proposed shift on a local day. */
function proposedShiftBounds(
  dayKey: string,
  startTime: string,
  endTime: string,
): [number, number] | null {
  const [y, mo, d] = dayKey.split("-").map(Number);
  const [sh, smi] = startTime.split(":").map(Number);
  const [eh, emi] = endTime.split(":").map(Number);
  if (![y, mo, d, sh, smi, eh, emi].every((n) => Number.isFinite(n))) {
    return null;
  }
  const start = new Date(y, mo - 1, d, sh, smi, 0, 0).getTime();
  let end = new Date(y, mo - 1, d, eh, emi, 0, 0).getTime();
  if (end <= start) end += 24 * 60 * 60 * 1000; // overnight
  return [start, end];
}

/**
 * Conflicts for a proposed/assigned shift against the staff member's availability
 * and approved time-off. Informative only — the caller (manager) may override, so
 * this never blocks a save. Returns an empty list for an open (unassigned) shift.
 *
 * - timeoff: the shift's instant range intersects an approved time-off request.
 * - unavailable: the shift's minute range overlaps an "unavailable" window on the
 *   shift's weekday. An overnight shift (end ≤ start) is treated as running to
 *   end-of-day for this same-weekday comparison.
 */
export function detectShiftConflicts(
  input: ShiftConflictInput,
  team: TeamAvailability,
  approved: TimeOffRequest[],
): ShiftConflict[] {
  const { staffId, dayKey, startTime, endTime, existingShifts, ignoreShiftId } =
    input;
  if (staffId == null) return [];
  const conflicts: ShiftConflict[] = [];

  // Double-booking: the proposed shift's instant range overlaps another shift
  // already assigned to the SAME staff member (any day — an overnight shift can
  // cross into the next). Informative like the others; the manager may override.
  if (existingShifts && existingShifts.length > 0) {
    const bounds = proposedShiftBounds(dayKey, startTime, endTime);
    if (bounds) {
      const [pStart, pEnd] = bounds;
      for (const sh of existingShifts) {
        if (sh.staff_id !== staffId) continue;
        if (ignoreShiftId != null && sh.id === ignoreShiftId) continue;
        const eStart = new Date(sh.starts_at).getTime();
        const eEnd = new Date(sh.ends_at).getTime();
        if (!Number.isFinite(eStart) || !Number.isFinite(eEnd)) continue;
        if (rangesOverlap(pStart, pEnd, eStart, eEnd)) {
          conflicts.push({ kind: "doublebook", shift: sh });
          break; // one overlap is enough to warn
        }
      }
    }
  }

  // Time-off: precise instant overlap in local time.
  const [y, mo, d] = dayKey.split("-").map(Number);
  const [sh, smi] = startTime.split(":").map(Number);
  const [eh, emi] = endTime.split(":").map(Number);
  if ([y, mo, d, sh, smi, eh, emi].every((n) => Number.isFinite(n))) {
    const shiftStart = new Date(y, mo - 1, d, sh, smi, 0, 0);
    let shiftEnd = new Date(y, mo - 1, d, eh, emi, 0, 0);
    if (shiftEnd.getTime() <= shiftStart.getTime()) {
      shiftEnd = new Date(shiftEnd.getTime() + 24 * 60 * 60 * 1000); // overnight
    }
    for (const req of approved) {
      if (req.staff_id !== staffId || req.status !== "approved") continue;
      const rs = new Date(req.starts_at).getTime();
      const re = new Date(req.ends_at).getTime();
      if (shiftStart.getTime() < re && rs < shiftEnd.getTime()) {
        conflicts.push({ kind: "timeoff", request: req });
        break; // one time-off conflict is enough to warn
      }
    }
  }

  // Unavailable windows: minute-of-day overlap on the shift's weekday.
  const startMin = parseTimeToMinutes(startTime);
  let endMin = parseTimeToMinutes(endTime);
  if (startMin != null && endMin != null) {
    if (endMin <= startMin) endMin = 1440; // overnight → to end-of-day
    const day = dayAvailability(staffId, dayKey, team);
    for (const w of day.windows) {
      if (w.kind !== "unavailable") continue;
      if (rangesOverlap(startMin, endMin, w.start_min, w.end_min)) {
        conflicts.push({
          kind: "unavailable",
          window: { start_min: w.start_min, end_min: w.end_min },
        });
      }
    }
  }

  return conflicts;
}

/** Format a minute-of-day range like "9:00–17:30" for a given locale. */
export function formatMinuteRange(
  startMin: number,
  endMin: number,
  locale: string,
): string {
  const fmt = (min: number) => {
    const h = Math.floor(min / 60) % 24;
    const m = min % 60;
    return new Intl.DateTimeFormat(locale, {
      hour: "numeric",
      minute: "2-digit",
    }).format(new Date(2000, 0, 1, h, m));
  };
  return `${fmt(startMin)}–${fmt(endMin >= 1440 ? 1439 : endMin)}`;
}
