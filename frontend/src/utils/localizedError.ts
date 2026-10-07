import { getTranslation } from "@/i18n/getTranslation";
import type { Locale } from "@/i18n/config";
import enApiErrors from "@/i18n/locales/en/apiErrors.json";
import { translateApiError } from "@/i18n/apiErrors";
import {
  apiErrorDetail,
  getApiErrorCode,
  getApiErrorMessage,
  getApiErrorStatus,
  getLocalizedApiError,
} from "./apiError";
import { sanitizeErrorKey } from "./errorMessages";

const catalogCodes = enApiErrors as Record<string, string>;

function translationString(key: string, locale: Locale): string {
  const result = getTranslation(key, locale);
  return Array.isArray(result) ? (result[0] ?? "") : result;
}

/**
 * Wave C (T-2/S-3): shared operator backend-error surface.
 *
 * Precedence:
 *  1. Known structured `code` → apiErrors.json (localized, interpolated)
 *  2. HTTP 413 → common.errors.rangeTooLarge (export/range caps)
 *  3. Product-safe backend `error` string for client 4xx (CO-1 domain copy)
 *  4. Status-keyed common.errors.* generic
 *  5. Optional caller fallback, then GENERIC_ERROR
 *
 * Pure function — no I/O. Call from catch blocks / toast sites.
 */
export function surfaceBackendError(
  error: unknown,
  locale: Locale,
  fallback?: string,
): string {
  const code = getApiErrorCode(error);
  const status = getApiErrorStatus(error);

  // 1. Catalog hit
  if (code && code in catalogCodes) {
    return getLocalizedApiError(error, locale);
  }

  // 2. Payload / range too large (analytics export 31-day cap)
  if (status === 413) {
    const rangeMsg = translationString("common.errors.rangeTooLarge", locale);
    if (rangeMsg && !rangeMsg.startsWith("common.errors.")) {
      return rangeMsg;
    }
    const detail413 = apiErrorDetail(error);
    if (detail413) return detail413;
  }

  // 3. Product-safe backend domain copy for client errors (not 5xx internals).
  // Also honor a body with no status (tests / non-axios shapes).
  if (status === undefined || (status >= 400 && status < 500)) {
    const detail = apiErrorDetail(error);
    if (detail) return detail;
  }

  // 4. Explicit caller fallback for non-HTTP throws (domain helpers pass
  // action-specific copy). Prefer it over the status-keyed generic.
  if (status === undefined && fallback && fallback.trim().length > 0) {
    return fallback;
  }

  // 5. Status-based operator generic
  const { key } = sanitizeErrorKey(error);
  const generic = translationString(`common.errors.${key}`, locale);
  if (generic && !generic.startsWith("common.errors.")) {
    return generic;
  }

  // 6. Caller fallback or catalog generic
  if (fallback && fallback.trim().length > 0) {
    return fallback;
  }
  return translateApiError({ code: "GENERIC_ERROR" }, locale);
}

/**
 * K-10: resolve an API error to a message in the operator's locale.
 * Delegates to {@link surfaceBackendError} (Wave C single convention).
 */
export function localizedErrorMessage(error: unknown, locale: Locale): string {
  return surfaceBackendError(error, locale);
}
