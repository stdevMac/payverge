/**
 * Locale-tolerant decimal parsing for user-typed money/number inputs.
 *
 * `parseFloat("5,50")` returns 5 — silently truncating cents for guests in
 * comma-decimal locales (most of the 21 supported guest languages). Worse,
 * treating a lone separator + 3+ fractional digits as thousands (strip sep)
 * corrupts pastes like "12.345" into 12345 (L3-17 1000× class).
 *
 * Rules:
 * 1. Strip currency symbols / spaces; keep digits, `.`, `,`, and a leading `-`.
 * 2. If both `.` and `,` appear: the LAST one is the decimal separator; the
 *    other is thousands grouping.
 * 3. If only one separator character appears more than once: all are thousands
 *    (no fractional part) — e.g. `1.234.567`.
 * 4. If only one separator appears once: it is ALWAYS the decimal separator
 *    (any fractional length). Result is then rounded to `maxFractionDigits`
 *    (default 2). This prevents silent 100×/1000× corruption on paste.
 * 5. Letter+digit mixtures (e.g. `12a34`) are garbage → 0 / null.
 * 6. Magnitudes larger than MAX_SAFE_INTEGER/100 are clamped to that bound.
 *
 * `parseLocaleDecimal` returns 0 for garbage (legacy `|| 0` call sites).
 * `tryParseLocaleDecimal` returns null for empty/garbage (validation UX).
 *
 * Pair text inputs with inputMode="decimal" (not type="number") so the browser
 * cannot strip commas before we see them.
 */

export type ParseLocaleDecimalOptions = {
  /** Fractional digits kept after rounding. Default 2 (money cents). */
  maxFractionDigits?: number;
  /**
   * Absolute magnitude clamp. Defaults to floor(MAX_SAFE_INTEGER / 100) so
   * cent-scale money stays representable as an integer number of cents.
   */
  maxAbs?: number;
};

const DEFAULT_MAX_FRAC = 2;
const DEFAULT_MAX_ABS = Math.floor(Number.MAX_SAFE_INTEGER / 100);

function roundTo(n: number, places: number): number {
  if (places <= 0) return Math.round(n);
  const f = 10 ** places;
  return Math.round(n * f) / f;
}

function clampAbs(n: number, maxAbs: number): number {
  if (!Number.isFinite(n)) return 0;
  if (n > maxAbs) return maxAbs;
  if (n < -maxAbs) return -maxAbs;
  return n;
}

/**
 * Core parse. Returns `{ ok:false }` for empty/garbage; `{ ok:true, value }`
 * for a successful parse (including 0).
 */
function parseCore(
  raw: string,
  opts?: ParseLocaleDecimalOptions,
): { ok: true; value: number } | { ok: false } {
  if (typeof raw !== "string") return { ok: false };

  const maxFrac =
    opts?.maxFractionDigits === undefined
      ? DEFAULT_MAX_FRAC
      : Math.max(0, Math.floor(opts.maxFractionDigits));
  const maxAbs =
    opts?.maxAbs === undefined ? DEFAULT_MAX_ABS : Math.abs(opts.maxAbs);

  const trimmed = raw.trim();
  if (!trimmed) return { ok: false };

  // Pure letters / no digits → garbage.
  if (!/\d/.test(trimmed)) return { ok: false };

  // Strip currency symbols and whitespace first, then refuse any remaining
  // letter/other junk (so "12a34", "12,3a", "12a" never silently strip to a
  // wrong integer). Currency marks themselves are allowed and dropped.
  const withoutCurrency = trimmed.replace(/[\s$€£¥₹₽₩¢]/gu, "");
  if (/[^\d.,-]/.test(withoutCurrency)) return { ok: false };

  const s = withoutCurrency.replace(/[^\d.,-]/g, "");
  if (!s || !/\d/.test(s)) return { ok: false };

  // Only a single LEADING minus is a sign. A hyphen anywhere else ("12-34",
  // "5-") is garbage — stripping it would silently concatenate the digits.
  const negative = s.startsWith("-");
  const body = negative ? s.slice(1) : s;
  if (!body || !/\d/.test(body)) return { ok: false };

  // Any remaining non-digit non-separator (incl. interior/trailing "-") means
  // garbage survived.
  if (/[^\d.,]/.test(body)) return { ok: false };

  const commas = (body.match(/,/g) || []).length;
  const dots = (body.match(/\./g) || []).length;

  let intPart: string;
  let fracPart = "";

  if (commas > 0 && dots > 0) {
    // Both present: last separator is decimal.
    const lastComma = body.lastIndexOf(",");
    const lastDot = body.lastIndexOf(".");
    const sepIndex = Math.max(lastComma, lastDot);
    intPart = body.slice(0, sepIndex).replace(/[^\d]/g, "");
    fracPart = body.slice(sepIndex + 1).replace(/[^\d]/g, "");
  } else if (commas > 1 || dots > 1) {
    // Repeated same separator → pure thousands grouping.
    intPart = body.replace(/[^\d]/g, "");
    fracPart = "";
  } else if (commas === 1 || dots === 1) {
    // Single separator, once → always decimal (any frac length).
    const sep = commas === 1 ? "," : ".";
    const sepIndex = body.indexOf(sep);
    intPart = body.slice(0, sepIndex).replace(/[^\d]/g, "");
    fracPart = body.slice(sepIndex + 1).replace(/[^\d]/g, "");
  } else {
    intPart = body.replace(/[^\d]/g, "");
    fracPart = "";
  }

  if (!intPart && !fracPart) return { ok: false };

  const num = parseFloat(`${intPart || "0"}.${fracPart || "0"}`);
  if (!Number.isFinite(num)) return { ok: false };

  const signed = negative ? -num : num;
  const rounded = roundTo(signed, maxFrac);
  return { ok: true, value: clampAbs(rounded, maxAbs) };
}

/**
 * Parse a locale-typed decimal. Returns 0 for empty/garbage (legacy `|| 0`
 * call-site semantics). Prefer `tryParseLocaleDecimal` when validation needs
 * to distinguish invalid input from zero.
 */
export function parseLocaleDecimal(
  raw: string,
  opts?: ParseLocaleDecimalOptions,
): number {
  const result = parseCore(raw, opts);
  return result.ok ? result.value : 0;
}

/**
 * Strict parse for form validation. Returns `null` for empty, whitespace,
 * or garbage; otherwise the rounded number (including 0).
 */
export function tryParseLocaleDecimal(
  raw: string,
  opts?: ParseLocaleDecimalOptions,
): number | null {
  const result = parseCore(raw, opts);
  return result.ok ? result.value : null;
}
