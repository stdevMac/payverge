import axios from 'axios';
import { translateApiError } from '@/i18n/apiErrors';
import enApiErrors from '@/i18n/locales/en/apiErrors.json';

// Residual English map for codes not yet (or never) in apiErrors.json.
// Prefer the catalog for anything present there — see catalogMessageForCode.
const ERROR_CODE_MESSAGES: Record<string, string> = {
  // Canonical code (H1). The legacy AUTH_SESSION_REVOKED alias is kept for the
  // deploy window so a stale backend still resolves a friendly message.
  AUTH_SESSION_REVOKED: 'Your session has been ended. Please sign in again.',
};

const catalogCodes = enApiErrors as Record<string, string>;

function catalogMessageForCode(code: string, locale: string = 'en'): string | undefined {
  if (!code || !(code in catalogCodes)) {
    return undefined;
  }
  return translateApiError({ code }, locale);
}

// Generic error messages for production
const GENERIC_MESSAGES = {
  NETWORK_ERROR: 'Unable to connect to the server. Please check your internet connection and try again.',
  SERVER_ERROR: 'Something went wrong on our end. Please try again later.',
  TIMEOUT_ERROR: 'The request took too long. Please try again.',
  RATE_LIMIT_ERROR: 'Too many requests. Please wait a moment and try again.',
  VALIDATION_ERROR: 'Please check your input and try again.',
  AUTHENTICATION_ERROR: 'Please sign in to continue.',
  AUTHORIZATION_ERROR: 'You do not have permission to perform this action.',
  NOT_FOUND_ERROR: 'The requested resource was not found.',
  CONFLICT_ERROR: 'This action conflicts with existing data. Please refresh and try again.',
  DEFAULT_ERROR: 'An unexpected error occurred. Please try again.'
};

// Narrow view of an axios/fetch error shape — enough for status/message
// extraction without taking on the whole AxiosError surface.
type ApiErrorLike = {
  message?: unknown;
  response?: {
    status?: unknown;
    data?: {
      message?: unknown;
      error?: unknown;
      code?: unknown;
    };
  };
};

function asApiErrorLike(error: unknown): ApiErrorLike | null {
  return typeof error === "object" && error !== null ? (error as ApiErrorLike) : null;
}

function asString(value: unknown): string | undefined {
  return typeof value === "string" ? value : undefined;
}

function asNumber(value: unknown): number | undefined {
  return typeof value === "number" ? value : undefined;
}

export function getGenericErrorMessage(error: unknown): string {
  const like = asApiErrorLike(error);

  // For development, return detailed errors
  if (process.env.NODE_ENV === 'development') {
    const detailMessage = asString(like?.response?.data?.message);
    if (detailMessage) return detailMessage;
    const detailError = asString(like?.response?.data?.error);
    if (detailError) return detailError;
    const topMessage = asString(like?.message);
    if (topMessage) return topMessage;
    return String(error);
  }

  // Network failure (offline/DNS/TLS) is only reliably detectable on the RAW
  // AxiosError — the shared interceptor sanitizes errors into a plain Error that
  // strips `isAxiosError` AND drops the response, so a sanitized network error is
  // indistinguishable from a generic JS error downstream. Detect it here for the
  // in-interceptor caller; downstream it safely falls through to DEFAULT below.
  if (axios.isAxiosError(error) && !error.response) {
    return GENERIC_MESSAGES.NETWORK_ERROR;
  }

  // Map by HTTP status off the PRESERVED `.response.status`, not behind an
  // `axios.isAxiosError` gate — that gate is always false at every downstream
  // caller (sanitizeError → here), which silently demoted real 4xx/5xx errors to
  // the generic default. The sanitized shape keeps `.response.status`, so honor it.
  const status = asNumber(like?.response?.status);
  switch (status) {
    case 400:
      return GENERIC_MESSAGES.VALIDATION_ERROR;
    case 401:
      return GENERIC_MESSAGES.AUTHENTICATION_ERROR;
    case 403:
      return GENERIC_MESSAGES.AUTHORIZATION_ERROR;
    case 404:
      return GENERIC_MESSAGES.NOT_FOUND_ERROR;
    case 408:
      return GENERIC_MESSAGES.TIMEOUT_ERROR;
    case 409:
      return GENERIC_MESSAGES.CONFLICT_ERROR;
    case 429:
      return GENERIC_MESSAGES.RATE_LIMIT_ERROR;
    case 500:
    case 502:
    case 503:
    case 504:
      return GENERIC_MESSAGES.SERVER_ERROR;
    default:
      // No usable status → a non-HTTP client error (TypeError, etc.) or a
      // sanitized network error. Generic default keeps the Sentry stack intact.
      return GENERIC_MESSAGES.DEFAULT_ERROR;
  }
}

export function sanitizeError(
  error: unknown,
): { message: string; status?: number; code?: string } {
  const like = asApiErrorLike(error);
  const status = asNumber(like?.response?.status);
  const code = asString(like?.response?.data?.code);

  // Prefer structured catalog message (locale-aware via translateApiError).
  // sanitizeError stays English for legacy callers; UI toasts use useApiErrorMessage.
  if (code) {
    const fromCatalog = catalogMessageForCode(code, 'en');
    if (fromCatalog) {
      return { message: fromCatalog, status, code };
    }
    if (ERROR_CODE_MESSAGES[code]) {
      return { message: ERROR_CODE_MESSAGES[code], status, code };
    }
  }

  // Fall back to status-based generic messages
  const message = getGenericErrorMessage(error);
  return { message, status };
}

// K-10: stable, translatable error identity. `sanitizeError` above returns
// English strings (kept untouched for legacy callers); components that render
// TRANSLATED toasts should resolve a key here and translate it via
// common.errors.* (see utils/localizedError.ts).
export type SanitizedErrorKey =
  | 'network'
  | 'server'
  | 'timeout'
  | 'rateLimited'
  | 'validation'
  | 'authentication'
  | 'authorization'
  | 'notFound'
  | 'conflict'
  | 'default';

export function sanitizeErrorKey(error: unknown): {
  key: SanitizedErrorKey;
  status?: number;
  code?: string;
} {
  const like = asApiErrorLike(error);
  const status = asNumber(like?.response?.status);
  const code = asString(like?.response?.data?.code);

  if (axios.isAxiosError(error) && !error.response) {
    return { key: 'network', status, code };
  }

  switch (status) {
    case 400:
      return { key: 'validation', status, code };
    case 401:
      return { key: 'authentication', status, code };
    case 403:
      return { key: 'authorization', status, code };
    case 404:
      return { key: 'notFound', status, code };
    case 408:
      return { key: 'timeout', status, code };
    case 409:
      return { key: 'conflict', status, code };
    case 429:
      return { key: 'rateLimited', status, code };
    case 500:
    case 502:
    case 503:
    case 504:
      return { key: 'server', status, code };
    default:
      return { key: 'default', status, code };
  }
}
