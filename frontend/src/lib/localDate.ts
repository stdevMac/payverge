/**
 * Local-timezone date keys for UI logic ("today", date-range filters,
 * date-input min/max, datetime-local values).
 *
 * Replaces the toISOString-based YYYY-MM-DD slicing idiom (UTC day), which
 * yields the UTC day — wrong near midnight for any non-UTC user (e.g. blocks
 * same-day reservation booking for evening guests in the Americas).
 * Guarded by scripts/check-utc-datekeys.js.
 */
export function localDateKey(date: Date = new Date()): string {
  const y = date.getFullYear();
  const m = String(date.getMonth() + 1).padStart(2, "0");
  const d = String(date.getDate()).padStart(2, "0");
  return `${y}-${m}-${d}`;
}

/** Local value for `<input type="datetime-local">` (YYYY-MM-DDTHH:mm). */
export function localDateTimeInputValue(date: Date = new Date()): string {
  const h = String(date.getHours()).padStart(2, "0");
  const min = String(date.getMinutes()).padStart(2, "0");
  return `${localDateKey(date)}T${h}:${min}`;
}
