/**
 * Helpers for narrowing `unknown` caught errors without changing behavior.
 *
 * The shared `axiosInstance` interceptor (api/tools/instance.ts) rejects with a
 * SANITIZED plain `Error` that copies `.status` / `.code` / `.response.{status,
 * data}` onto a fresh Error but carries NO `isAxiosError` flag. So
 * `axios.isAxiosError(e)` is ALWAYS false at call sites downstream of an
 * axiosInstance call, silently disabling any catch gated on it.
 *
 * These helpers read the sanitized shape (and a raw AxiosError) directly, so
 * catch clauses can stay on `catch (err: unknown)` and keep their existing
 * `... || "fallback"` precedence:
 *
 *   err?.response?.data?.error || "fallback"   →  apiErrorDetail(err) || "fallback"
 *   err.message || "fallback"                  →  errMessage(err) || "fallback"
 */

import { translateApiError, type ApiErrorPayload } from "@/i18n/apiErrors";

type ApiErrorShape = {
  status?: number;
  code?: string;
  response?: { status?: number; data?: unknown };
};

function apiErrorShape(err: unknown): ApiErrorShape {
  return typeof err === "object" && err !== null ? (err as ApiErrorShape) : {};
}

function apiErrorBody(err: unknown): { error?: unknown; code?: unknown } | undefined {
  const data = apiErrorShape(err).response?.data;
  return typeof data === "object" && data !== null
    ? (data as { error?: unknown; code?: unknown })
    : undefined;
}

/**
 * True when the error looks like it originated from axios (raw or sanitized) —
 * used to avoid mistaking an arbitrary thrown Error for a network failure.
 */
function isApiErrorLike(err: unknown): boolean {
  if ((err as { isAxiosError?: boolean })?.isAxiosError === true) {
    return true;
  }
  const e = apiErrorShape(err);
  return e.response != null || e.status != null || e.code != null;
}

/** HTTP status from the sanitized/axios error (`response.status`, then `.status`). */
export function getApiErrorStatus(err: unknown): number | undefined {
  const e = apiErrorShape(err);
  return e.response?.status ?? e.status;
}

/** The raw backend response body (`response.data`), or undefined. */
export function getApiErrorData(err: unknown): unknown {
  return apiErrorShape(err).response?.data;
}

/** The backend error-envelope string (`response.data.error`), or undefined. */
export function getApiErrorMessage(err: unknown): string | undefined {
  const body = apiErrorBody(err);
  return typeof body?.error === "string" ? body.error : undefined;
}

/**
 * Gin validator dumps look like:
 *   Key: 'CreateReservationInput.CustomerName' Error:Field validation for
 *   'CustomerName' failed on the 'required' tag
 * Operators and guests must never see these — they are not product copy.
 * Shared by getLocalizedApiError and reservationErrorMessage (FIND-030/031).
 */
export function isRawValidatorDump(detail: string): boolean {
  const s = detail.trim();
  if (!s) return false;
  return (
    s.includes("Key: '") ||
    s.includes("Error:Field validation") ||
    s.includes("failed on the '") ||
    /\bbinding:"/.test(s)
  );
}

/** True when a string is short, non-empty, and not a gin validator dump. */
function isProductSafeDetail(detail: string | undefined): detail is string {
  return (
    typeof detail === "string" &&
    detail.trim().length > 0 &&
    detail.length <= 200 &&
    !isRawValidatorDump(detail)
  );
}

/**
 * Axios/network transport text is not product copy (FIND-055). Wallet/viem
 * rejections use plain Error.message and should still surface when short.
 */
function isGenericAxiosTransportMessage(detail: string): boolean {
  const s = detail.trim();
  return (
    /^Request failed with status code \d+$/i.test(s) ||
    /^Network Error$/i.test(s) ||
    /^timeout of \d+ms exceeded$/i.test(s)
  );
}

/**
 * Safe product copy from a caught API error: prefers non-dump backend strings
 * under 200 chars, then a short non-transport Error.message (wallet reject),
 * otherwise the caller fallback (FIND-030/034/055).
 */
export function getSafeApiErrorMessage(
  err: unknown,
  fallback: string,
): string {
  const detail = getApiErrorMessage(err);
  const body = apiErrorBody(err) as { code?: unknown; message?: unknown } | undefined;
  // Allowlisted public 5xx codes (backend ErrorSanitizer, e.g. 503
  // ai_not_configured) carry error === code plus a server-authored `message`.
  // Show that explanation, never the bare machine code.
  if (
    typeof detail === "string" &&
    detail === body?.code &&
    typeof body?.message === "string"
  ) {
    const message = body.message.trim();
    if (message && message.length <= 400 && !isRawValidatorDump(message)) {
      return message;
    }
  }
  if (isProductSafeDetail(detail)) {
    return detail;
  }
  // Wallet / viem / user-reject paths throw plain Errors without a response
  // body. Prefer those messages over a generic fallback when they are safe.
  // Do NOT fall through to Error.message on API-shaped errors (sanitized axios
  // often has transport text like "request failed" while body was a dump).
  if (!isApiErrorLike(err)) {
    const msg = errMessage(err);
    if (isProductSafeDetail(msg) && !isGenericAxiosTransportMessage(msg)) {
      return msg;
    }
  }
  return fallback;
}

/** The backend error code (`response.data.code`), or undefined. */
export function getApiErrorCode(err: unknown): string | undefined {
  const body = apiErrorBody(err);
  return typeof body?.code === "string" ? body.code : undefined;
}

// Permission/authorization failures. A live session can still receive these
// (RBAC, wrong-business, not-admin). They must not trigger /auth/refresh or
// auth:session-expired — that used to revoke a one-time refresh token and
// bounce operators to Authentication Required from Reservations/Tables.
const NON_SESSION_AUTH_CODES = new Set([
  "AUTH_INSUFFICIENT_ROLE",
  "AUTH_FORBIDDEN",
  "AUTH_NOT_ADMIN",
  "BIZ_NOT_OWNER",
  "BIZ_STAFF_NO_ACCESS",
]);

export function isNonSessionAuthFailure(code: string | undefined): boolean {
  return typeof code === "string" && NON_SESSION_AUTH_CODES.has(code);
}

/**
 * The backend error `params` bag (`response.data.params`) for interpolation
 * (e.g. `{ field: "email" }`), narrowed to string/number values, or undefined.
 */
function getApiErrorParams(
  err: unknown,
): Record<string, string | number> | undefined {
  const data = apiErrorShape(err).response?.data;
  const params =
    typeof data === "object" && data !== null
      ? (data as { params?: unknown }).params
      : undefined;
  if (typeof params !== "object" || params === null) {
    return undefined;
  }
  const out: Record<string, string | number> = {};
  for (const [key, value] of Object.entries(params as Record<string, unknown>)) {
    if (typeof value === "string" || typeof value === "number") {
      out[key] = value;
    }
  }
  return Object.keys(out).length > 0 ? out : undefined;
}

/**
 * H1 (frontend): resolve a caught API error to a message in the caller's locale.
 *
 * The legacy `apiErrorDetail()` returns the raw backend `error` string verbatim
 * (English), so every coded toast stayed English in es / es-AR. This wires the
 * dead `translateApiError` localization layer into the live path:
 *
 *  1. If the backend envelope carries a structured `code` that maps to a known
 *     `apiErrors.json` entry, return its localized (interpolated) message.
 *  2. Otherwise fall back to the raw backend `error` string (still useful — the
 *     backend message is at least specific, even if English).
 *  3. If there is no code AND no error string (or a non-API throw), return a
 *     generic localized message so a toast never renders empty.
 *
 * `translateApiError` already degrades an unsupported locale to English and an
 * unknown code to the payload's `error`/`code`, so this stays graceful end to
 * end. Pass the active operator/guest locale from a component; non-React call
 * sites can use `getApiErrorMessage()` (raw) as before.
 */
export function getLocalizedApiError(err: unknown, locale: string): string {
  const code = getApiErrorCode(err);
  const error = getApiErrorMessage(err);
  const params = getApiErrorParams(err);

  // No structured code AND no backend string → nothing specific to show.
  // Render a generic localized message instead of an empty toast.
  if (!code && !error) {
    return translateApiError({ code: "GENERIC_ERROR" }, locale);
  }

  // Gin binding dumps are not product copy — map to a stable validation message
  // even when the backend still returns the raw validator string (FIND-031).
  if (error && isRawValidatorDump(error)) {
    return translateApiError(
      { code: code || "VALIDATION_INVALID_INPUT", error: undefined, params },
      locale,
    );
  }

  const payload: ApiErrorPayload = { code, error, params };
  return translateApiError(payload, locale);
}

/**
 * True when an axios-originated request produced no HTTP response
 * (offline/timeout/DNS). A non-axios thrown Error is NOT a network error.
 */
export function isApiNetworkError(err: unknown): boolean {
  return isApiErrorLike(err) && apiErrorShape(err).response == null;
}

/**
 * Product-safe backend error string for `apiErrorDetail(err) || fallback` call
 * sites (admin surfaces, API wrappers). Gin validator dumps and overlong
 * strings return undefined so the caller fallback wins — same policy as
 * getSafeApiErrorMessage (FIND-034 residual).
 */
export function apiErrorDetail(err: unknown): string | undefined {
  const detail = getApiErrorMessage(err);
  if (
    typeof detail === "string" &&
    detail.trim().length > 0 &&
    detail.length <= 200 &&
    !isRawValidatorDump(detail)
  ) {
    return detail;
  }
  return undefined;
}

/** A generic Error/AxiosError `.message`, or undefined for non-Error throws. */
export function errMessage(err: unknown): string | undefined {
  if (err instanceof Error) {
    return err.message;
  }
  return undefined;
}
