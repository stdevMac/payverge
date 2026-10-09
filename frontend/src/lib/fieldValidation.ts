/**
 * Shared client-side field validators for operator forms (Session D S-5).
 * Prefer these + NextUI isInvalid/errorMessage over native HTML5 bubbles
 * (which render in the browser language, not the operator UI locale).
 */

/** Real email shape — not merely includes("@"). */
const EMAIL_RE = /^[^\s@]+@[^\s@]+\.[^\s@]+$/;

/**
 * Phone: optional leading +, digits with common separators, 7–20 significant
 * digits/spaces/parens/hyphens total. Mirrors guest reservation check so
 * operator and guest paths agree.
 */
const PHONE_RE = /^[+]?[\d\s()-]{7,20}$/;

export function isValidEmail(raw: string): boolean {
  const s = raw.trim();
  if (!s) return false;
  return EMAIL_RE.test(s);
}

export function isValidPhone(raw: string): boolean {
  const s = raw.trim();
  if (!s) return false;
  // Must contain at least 7 digits (not just punctuation).
  const digits = s.replace(/\D/g, "");
  if (digits.length < 7 || digits.length > 15) return false;
  return PHONE_RE.test(s);
}

/** Non-empty after trim. */
export function isNonEmptyTrimmed(raw: string): boolean {
  return raw.trim().length > 0;
}

/** True when the field has only whitespace (typed spaces, no content). */
export function isWhitespaceOnly(raw: string): boolean {
  return raw.length > 0 && raw.trim().length === 0;
}

/**
 * Integer in [min, max] inclusive. Rejects decimals, NaN, empty.
 */
export function isIntInRange(
  raw: string | number,
  min: number,
  max: number,
): boolean {
  const n = typeof raw === "number" ? raw : Number(String(raw).trim());
  if (!Number.isFinite(n) || !Number.isInteger(n)) return false;
  return n >= min && n <= max;
}

/** Finite number >= 0 (allows decimals). */
export function isNonNegativeNumber(raw: string | number): boolean {
  const n = typeof raw === "number" ? raw : Number(String(raw).trim());
  return Number.isFinite(n) && n >= 0;
}
