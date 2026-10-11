/**
 * L6-8 — commit policy for PaymentHistory "Personalizado" date inputs.
 *
 * Segmented DatePickers fire onChange on every year/month/day keystroke, so
 * typing a 4-digit year produces intermediate values (and API calls if the
 * raw value drives the query). Mirror the search-box debounce: raw input
 * state → debounced committed range used by buildQuery/fetchPage.
 *
 * Only ISO calendar dates with a four-digit year ≥ 1000 are considered
 * complete; incomplete intermediates keep the previous committed value so a
 * mid-type pause does not fetch garbage years.
 */

export const DATE_RANGE_DEBOUNCE_MS = 350;

const COMPLETE_ISO = /^(\d{4})-(\d{2})-(\d{2})$/;

/** True for empty (clear) or a full YYYY-MM-DD with year ≥ 1000. */
export function isCompleteIsoDate(value: string): boolean {
  if (value === "") return true;
  const m = COMPLETE_ISO.exec(value);
  if (!m) return false;
  const year = Number(m[1]);
  const month = Number(m[2]);
  const day = Number(m[3]);
  if (year < 1000) return false;
  if (month < 1 || month > 12) return false;
  if (day < 1 || day > 31) return false;
  return true;
}

export type DateRange = { start: string; end: string };

/**
 * Produce the next committed range from raw (possibly intermediate) input.
 * Incomplete fields retain their previous committed value.
 */
export function commitDateRange(
  input: DateRange,
  previous: DateRange,
): DateRange {
  return {
    start: isCompleteIsoDate(input.start) ? input.start : previous.start,
    end: isCompleteIsoDate(input.end) ? input.end : previous.end,
  };
}
