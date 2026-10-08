/**
 * Compact human-readable elapsed/occupancy durations from whole minutes.
 * Units (m/h/d) are language-neutral; locale is reserved for future wording.
 *
 * Caps multi-day ages to "Nd" so UIs never show walls like "38858 min" or "450h".
 */

export type DurationLocale = "en" | "es";

/** Urgency for elapsed kitchen/occupancy ages. Past calmAfter → "stale" (no red pulse). */
export type UrgencyTone = "none" | "warn" | "critical" | "stale";

const MINUTES_PER_HOUR = 60;
const MINUTES_PER_DAY = 24 * MINUTES_PER_HOUR;

/**
 * Format a minute count as a compact duration string.
 * - < 60 → "45m"
 * - < 1 day → "2h 15m" (drops zero remainder minutes)
 * - 1 day → "1d" / "1d 2h"
 * - ≥ 2 days → "27d" (days only)
 * - Negative / NaN / non-finite → "0m"
 */
export function humanizeDurationMinutes(
  minutes: number,
  _locale: DurationLocale = "en",
): string {
  if (!Number.isFinite(minutes) || minutes < 0) {
    return "0m";
  }

  const total = Math.floor(minutes);

  if (total < MINUTES_PER_HOUR) {
    return `${total}m`;
  }

  const days = Math.floor(total / MINUTES_PER_DAY);
  const hours = Math.floor((total % MINUTES_PER_DAY) / MINUTES_PER_HOUR);
  const mins = total % MINUTES_PER_HOUR;

  if (days >= 2) {
    return `${days}d`;
  }

  if (days === 1) {
    return hours > 0 ? `1d ${hours}h` : "1d";
  }

  // Under 24 hours
  return mins > 0 ? `${hours}h ${mins}m` : `${hours}h`;
}

/**
 * Urgency tone for elapsed kitchen / table occupancy ages.
 * Defaults: warn >15m, critical >30m, stale ≥24h (neutralize rose/pulse).
 */
export function elapsedUrgency(
  minutes: number,
  opts?: {
    warnAfterMinutes?: number;
    criticalAfterMinutes?: number;
    calmAfterMinutes?: number;
  },
): UrgencyTone {
  if (!Number.isFinite(minutes) || minutes < 0) {
    return "none";
  }

  const warnAfter = opts?.warnAfterMinutes ?? 15;
  const criticalAfter = opts?.criticalAfterMinutes ?? 30;
  const calmAfter = opts?.calmAfterMinutes ?? MINUTES_PER_DAY;

  if (minutes >= calmAfter) {
    return "stale";
  }
  if (minutes > criticalAfter) {
    return "critical";
  }
  if (minutes > warnAfter) {
    return "warn";
  }
  return "none";
}
