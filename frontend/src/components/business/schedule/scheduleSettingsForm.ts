import type {
  ScheduleSettings,
  ScheduleSettingsUpdate,
} from "@/api/scheduleSettings";

// Pure form model + conversions for the schedule-settings editor. Kept out of the
// component so the minute/hour ⇄ wire-shape math is unit-tested in isolation.
// Money-free (none of these settings are monetary). Backend contract (see
// backend/internal/handlers/schedule_settings.go): the three minute-of-day fields
// (quiet start/end, minor cutoff) accept -1 to CLEAR to NULL; the rest are plain
// bounded ints. Hours are a display convenience — lead is stored in hours, but
// shift length and the OT threshold are stored in MINUTES.

export interface ScheduleSettingsForm {
  weekStartDay: number; // 0 (Sun) .. 6 (Sat)
  shiftHours: number; // default shift length, hours
  reminderLeadHours: number; // 0 .. 168
  overtimeHours: number; // weekly OT threshold, hours
  postedLeadDays: number; // 0 .. 60
  quietEnabled: boolean;
  quietStart: string; // "HH:MM"
  quietEnd: string; // "HH:MM"
  minorEnabled: boolean;
  minorCutoff: string; // "HH:MM"
}

const DEFAULT_QUIET_START_MIN = 1320; // 22:00
const DEFAULT_QUIET_END_MIN = 420; // 07:00
const DEFAULT_MINOR_CUTOFF_MIN = 1320; // 22:00

export function clampInt(n: number, lo: number, hi: number): number {
  if (!Number.isFinite(n)) return lo;
  return Math.min(hi, Math.max(lo, Math.round(n)));
}

/** minute-of-day (0..1439) → "HH:MM" (24h, zero-padded). */
export function minuteToTime(min: number): string {
  const m = clampInt(min, 0, 1439);
  const hh = Math.floor(m / 60);
  const mm = m % 60;
  return `${String(hh).padStart(2, "0")}:${String(mm).padStart(2, "0")}`;
}

/** "HH:MM" → minute-of-day (0..1439). Malformed input → 0. */
export function timeToMinute(hhmm: string): number {
  const [h, m] = (hhmm || "").split(":").map((x) => parseInt(x, 10));
  if (!Number.isFinite(h) || !Number.isFinite(m)) return 0;
  return clampInt(h * 60 + m, 0, 1439);
}

/** minutes → hours for display (≤2 decimals, trailing zeros dropped). */
export function minutesToHours(minutes: number): number {
  return Math.round((minutes / 60) * 100) / 100;
}

/** hours → minutes for the wire, bounded to the backend's 0..10080 range. */
export function hoursToMinutes(hours: number): number {
  return clampInt(hours * 60, 0, 10080);
}

/** Seed the editable form from the loaded settings row. */
export function formFromSettings(s: ScheduleSettings): ScheduleSettingsForm {
  return {
    weekStartDay: s.week_start_day,
    shiftHours: minutesToHours(s.default_shift_minutes),
    reminderLeadHours: s.reminder_lead_hours,
    overtimeHours: minutesToHours(s.overtime_weekly_minutes),
    postedLeadDays: s.posted_lead_days,
    quietEnabled:
      s.quiet_hours_start_min != null && s.quiet_hours_end_min != null,
    quietStart: minuteToTime(s.quiet_hours_start_min ?? DEFAULT_QUIET_START_MIN),
    quietEnd: minuteToTime(s.quiet_hours_end_min ?? DEFAULT_QUIET_END_MIN),
    minorEnabled: s.minor_cutoff_min != null,
    minorCutoff: minuteToTime(s.minor_cutoff_min ?? DEFAULT_MINOR_CUTOFF_MIN),
  };
}

/**
 * Translate the form into the PUT body. Disabled nullable fields send -1 (the
 * backend's "clear to NULL" sentinel); enabled ones send their minute-of-day.
 */
export function buildScheduleSettingsUpdate(
  f: ScheduleSettingsForm,
): ScheduleSettingsUpdate {
  return {
    week_start_day: clampInt(f.weekStartDay, 0, 6),
    default_shift_minutes: hoursToMinutes(f.shiftHours),
    reminder_lead_hours: clampInt(f.reminderLeadHours, 0, 168),
    overtime_weekly_minutes: hoursToMinutes(f.overtimeHours),
    posted_lead_days: clampInt(f.postedLeadDays, 0, 60),
    quiet_hours_start_min: f.quietEnabled ? timeToMinute(f.quietStart) : -1,
    quiet_hours_end_min: f.quietEnabled ? timeToMinute(f.quietEnd) : -1,
    minor_cutoff_min: f.minorEnabled ? timeToMinute(f.minorCutoff) : -1,
  };
}
