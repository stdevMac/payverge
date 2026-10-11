import { getPublicConfig } from "@/config/publicConfig";
import axios, { AxiosError, Cancel, InternalAxiosRequestConfig, AxiosResponse } from "axios";
import { isAbortError } from "@/api/tools/abort";
import toast from "react-hot-toast";
import { randomUUID } from "@/lib/randomUUID";
import { sanitizeError } from "@/utils/errorMessages";
import { isNonSessionAuthFailure } from "@/utils/apiError";
import { apiCache } from "@/utils/cache";
import { isGuestRoute } from "@/utils/guestRoute";
import { enqueue, getActiveUserId } from "@/lib/mutationQueue";
import type { ErrorEnvelope } from "@/api/auth";
import { refreshAuthSession as sharedRefreshAuthSession } from "@/utils/refreshAuth";
import {
  CUSTOMER_SESSION_EXPIRED_EVENT,
  refreshCustomerAuthSession as sharedRefreshCustomerAuthSession,
} from "@/utils/refreshCustomerAuth";

declare module "axios" {
  export interface AxiosRequestConfig {
    _skipAuthRefresh?: boolean;
    // Opt-in: when true, the response interceptor toasts the backend
    // `error` string for 4xx failures (excluding 401/403/402, which keep
    // their own auth/budget flows). Off by default so callers that render
    // their own inline/field errors are not double-toasted.
    _surfaceError?: boolean;
    // Opt-out: callers that render their own inline error (login form) skip
    // the interceptor toast so a transport failure is not stacked twice.
    _skipErrorToast?: boolean;
    // GET responses are UNCACHED by default (C1: the implicit 5-min GET cache
    // defeated SSE / React Query invalidations / polling freshness by serving
    // stale data underneath them). Set `_useCache: true` to opt a genuinely
    // static reference read (supported currencies/languages, plan config) into
    // the 5-min apiCache. Realtime, polling, and per-session reads must never
    // opt in — React Query owns request caching for RQ-backed reads.
    _useCache?: boolean;
  }
}

// Debounce toast emissions so a burst of failed requests surfaces one message,
// not ten. Keyed by a canonical message per error class.
const TOAST_COOLDOWN_MS = 3000;
const lastToastAt: Record<string, number> = {};

function emitErrorToast(key: string, message: string) {
  if (typeof window === 'undefined') return;
  const now = Date.now();
  if (lastToastAt[key] && now - lastToastAt[key] < TOAST_COOLDOWN_MS) return;
  lastToastAt[key] = now;
  toast.error(message);
}

// Narrow read of the backend error envelope (gin.H{"error": "..."}).
// ErrorEnvelope.error is typed `string`, but a non-conforming body could carry
// any shape — cast through `unknown` and read defensively so a non-string
// `error` can't throw here.
function backendErrorString(err: AxiosError): string | undefined {
  const data = err.response?.data as unknown as ErrorEnvelope | undefined;
  return typeof data?.error === "string" ? data.error : undefined;
}

// Surface network and 5xx failures to the user. Skip auth errors (401/403 have
// their own flows) and 402 AI-budget refusals (calling UI names the limit). Also
// skip canceled requests and retryable failures that will be retried.
//
// 4xx are silent by default — most callers render their own inline/field
// errors. A caller can opt in per request with `_surfaceError: true`, which
// toasts the backend `error` string (or a generic validation message) for the
// 4xx that are not 401/403/402.
function publicPageLocale(): "en" | "es" | "es-AR" {
  if (typeof document !== "undefined") {
    const lang = (document.documentElement.lang || "").toLowerCase();
    if (lang === "es-ar" || lang.startsWith("es-ar")) return "es-AR";
    if (lang === "es" || lang.startsWith("es")) return "es";
  }
  if (typeof window !== "undefined") {
    const path = window.location.pathname.toLowerCase();
    if (path === "/es-ar" || path.startsWith("/es-ar/")) return "es-AR";
    if (path === "/es" || path.startsWith("/es/")) return "es";
  }
  return "en";
}

function localizedNetworkToast(): string {
  switch (publicPageLocale()) {
    case "es-AR":
      return "No pudimos contactar al servidor. Revisá tu conexión e intentá de nuevo.";
    case "es":
      return "No pudimos contactar al servidor. Revisa tu conexión e inténtalo de nuevo.";
    default:
      return "Couldn't reach the server. Check your connection and try again.";
  }
}

function localizedServerToast(): string {
  switch (publicPageLocale()) {
    case "es-AR":
      return "Algo salió mal de nuestro lado. Intentá de nuevo.";
    case "es":
      return "Algo salió mal de nuestro lado. Inténtalo de nuevo.";
    default:
      return "Something went wrong on our end. Please try again.";
  }
}

function shouldSkipUnpromptedPublicToast(err: AxiosError): boolean {
  if (typeof window === "undefined") return false;
  const url = `${err.config?.baseURL ?? ""}${err.config?.url ?? ""}`;
  if (url.includes("/analytics/")) return true;
  return false;
}

function maybeToastFromAxiosError(err: AxiosError) {
  if (typeof window === 'undefined') return;
  if (axios.isCancel(err)) return;
  const cfg = err.config as RetryConfig | undefined;
  if (cfg?._skipErrorToast) return;
  if (shouldSkipUnpromptedPublicToast(err) && !cfg?._surfaceError) return;
  const status = err.response?.status;
  if (status === 401 || status === 403 || status === 402) return;
  if (!err.response) {
    emitErrorToast(`network:${publicPageLocale()}`, localizedNetworkToast());
    return;
  }
  if (status && status >= 500) {
    emitErrorToast(`server:${publicPageLocale()}`, localizedServerToast());
    return;
  }
  if (status && status >= 400) {
    if (cfg?._surfaceError) {
      const message =
        backendErrorString(err) ?? "Please check your input and try again.";
      emitErrorToast(`4xx:${status}:${message}`, message);
    }
  }
}

// M6 — what we persist for a cached GET. We deliberately store ONLY the
// serializable response payload (`data`) and the status line, NOT the full live
// AxiosResponse. The old code cached the whole response object (headers with
// prototype methods, the live `config`, the `request`) and re-served it 5 min
// later through a JSON round-trip that mangled those non-serializable members —
// "works first load, breaks within 5 minutes". On a cache hit we now
// reconstruct a minimal, explicit AxiosResponse from this entry plus the
// current request's live config.
interface CachedGetEntry {
  data: unknown;
  status: number;
  statusText: string;
}

// Cancel subclass that carries the reconstructed cached response by reference,
// so the response interceptor can resolve it WITHOUT a lossy JSON round-trip.
// It is still an axios Cancel, so `axios.isCancel` holds and monitoring tools
// (Sentry) that filter on cancels continue to ignore it.
class CachedResponseCancel extends Error {
  readonly _isCached = true as const;
  // Marks this as an axios cancel so `axios.isCancel` returns true (it discriminates
  // solely on `__CANCEL__`), keeping Sentry/monitoring cancel filters effective —
  // without extending the generic axios.Cancel/CanceledError type.
  readonly __CANCEL__ = true as const;
  constructor(public readonly cachedResponse: AxiosResponse) {
    super("cached-response");
  }
}

function isCachedResponseCancel(value: unknown): value is CachedResponseCancel {
  return (
    typeof value === "object" &&
    value !== null &&
    (value as { _isCached?: unknown })._isCached === true &&
    "cachedResponse" in (value as object)
  );
}

const CACHE_GROUPS: Record<string, string[]> = {
  '/orders': ['/orders', '/order-items'],
  '/payments': ['/payments'],
  '/business': ['/business', '/menu', '/staff'],
  '/auth': ['/auth', '/profile', '/inside'],
  '/reservations': ['/reservations'],
  '/delivery': ['/delivery'],
  '/crm': ['/crm', '/customers'],
};

const AUTH_REFRESH_SKIP_PATHS = [
  '/auth/logout',
  '/auth/refresh',
  '/auth/login',
  '/customer/logout',
  '/customer/refresh',
  '/customer/session-info',
  '/staff/logout',
];

function isCustomerAPIPath(requestPath: string): boolean {
  try {
    const pathname = new URL(requestPath, "http://payverge.local").pathname;
    return pathname === "/customer" || pathname.startsWith("/customer/");
  } catch {
    return requestPath === "/customer" || requestPath.startsWith("/customer/");
  }
}

function clearCacheForMutation(url: string): void {
  const matchingPrefixes: string[] = [];
  for (const [mutationPrefix, cachePrefixes] of Object.entries(CACHE_GROUPS)) {
    if (url.includes(mutationPrefix)) {
      matchingPrefixes.push(...cachePrefixes);
    }
  }

  if (matchingPrefixes.length === 0) {
    apiCache.clear();
    return;
  }

  apiCache.clearByPrefixes(matchingPrefixes);
}

interface RetryConfig extends InternalAxiosRequestConfig {
    _retryCount?: number;
    _maxRetries?: number;
    _retryDelay?: number;
    _useCache?: boolean;
    _cacheTTL?: number;
    _isRefreshRetry?: boolean;
    _skipAuthRefresh?: boolean;
    _surfaceError?: boolean;
    _skipErrorToast?: boolean;
}

// Rejection raised when a mutation is enqueued for offline replay. Callers that
// await axiosInstance(...) receive THIS instead of a fabricated 202 response, so
// typed consumers (which read `res.data.success`/`res.data.bill`) no longer fall
// into a failure branch on an undefined body while the interceptor toasts
// "saved". Recognise it via `err instanceof OfflineQueuedError` or the stable
// `name === "OfflineQueuedError"` discriminant, and treat it as a pending state,
// not a hard failure.
export class OfflineQueuedError extends Error {
    // Discriminant kept for consumers that receive a structurally-cloned or
    // cross-realm error where `instanceof` may not hold.
    readonly _offlineQueued = true as const;

    constructor(
        public readonly method: string,
        public readonly url: string,
    ) {
        super("Action saved — will sync when you reconnect");
        // Assign `name` in the constructor (not a class field) so it does not
        // shadow the base Error's `name` under useDefineForClassFields.
        this.name = "OfflineQueuedError";
        // Preserve prototype chain across transpilation targets so
        // `instanceof OfflineQueuedError` holds.
        Object.setPrototypeOf(this, OfflineQueuedError.prototype);
    }
}

export function isOfflineQueuedError(error: unknown): error is OfflineQueuedError {
    return (
        error instanceof OfflineQueuedError ||
        (typeof error === "object" &&
            error !== null &&
            (error as { name?: unknown }).name === "OfflineQueuedError")
    );
}

const shouldRetry = (error: AxiosError): boolean => {
    const method = error.config?.method?.toLowerCase();
    const isMutation = method && ['post', 'put', 'delete', 'patch'].includes(method);

    // Never retry mutations on network errors — the request may have reached
    // the server, and retrying would create duplicates without idempotency keys.
    if (!error.response) return !isMutation; // Network error: retry GETs only

    const status = error.response.status;
    // Only retry GETs on server errors — mutations may have partially executed.
    // 408 (timeout) is safe to retry for GETs only.
    if (status >= 500 || status === 408 || status === 429) return !isMutation;
    return false;
};

const delay = (ms: number): Promise<void> =>
    new Promise(resolve => setTimeout(resolve, ms));

// Re-export the shared refresh helper so existing imports from this module
// (e.g. useSSEEvents) keep working. The boolean wrapper matches the prior
// Promise<boolean> contract.
export async function refreshAuthSession(apiUrl: string): Promise<boolean> {
    const result = await sharedRefreshAuthSession(apiUrl);
    return result.ok;
}

// No baseURL here: reading getPublicConfig() at module evaluation can run
// before the root layout's runtime config script (window.__PAYVERGE_ENV__)
// and would freeze the build-time fallback for the page's lifetime. The
// request interceptor below resolves it per request instead.
export const axiosInstance = axios.create({
    timeout: 30000, // Reduced from 1000000 to reasonable 30s
    withCredentials: true,
});

function toSanitizedError(error: unknown): Error & {
    status?: number;
    code?: string;
    response?: {
        status?: number;
        data?: unknown;
        headers?: Record<string, string>;
    };
} {
    const sanitized = sanitizeError(error);
    const sanitizedError = new Error(sanitized.message) as Error & {
        status?: number;
        code?: string;
        response?: {
            status?: number;
            data?: unknown;
            headers?: Record<string, string>;
        };
    };

    sanitizedError.status = sanitized.status;
    const axiosError = error as AxiosError;
    // Prefer a backend envelope code; keep axios transport codes (ERR_NETWORK)
    // so login/error mappers can still classify a bodyless failure.
    sanitizedError.code = sanitized.code ?? axiosError.code;
    if (axiosError?.response) {
        // Preserve Retry-After for rate-limit cooldowns (NEW-4 / REV-4). Edge
        // 429s are often bodyless; only the header carries the wait window.
        // Copy just that one header — do not re-surface full response headers.
        const rawHeaders = axiosError.response.headers as
            | Record<string, unknown>
            | undefined;
        const retryAfter =
            rawHeaders?.["retry-after"] ??
            rawHeaders?.["Retry-After"] ??
            (typeof (rawHeaders as { get?: (k: string) => unknown } | undefined)?.get ===
            "function"
                ? (rawHeaders as { get: (k: string) => unknown }).get("retry-after")
                : undefined);
        const headers: Record<string, string> | undefined =
            retryAfter != null && String(retryAfter).length > 0
                ? { "retry-after": String(retryAfter) }
                : undefined;
        sanitizedError.response = {
            status: axiosError.response.status,
            data: axiosError.response.data,
            ...(headers ? { headers } : {}),
        };
    }

    return sanitizedError;
}

axiosInstance.interceptors.request.use(
    (config: RetryConfig) => {
        // Resolve the API base at request time (see axios.create above). An
        // explicit per-request or defaults baseURL still wins.
        if (!config.baseURL) {
            config.baseURL = getPublicConfig().apiUrl;
        }

        // Set default retry configuration.
        //
        // M1 — React Query is the SINGLE retry owner. Previously axios retried
        // GETs up to 3× (exp backoff) UNDERNEATH React Query's own retry, so one
        // outage query fanned out 8-16 requests and `isError` lagged 30-60s
        // before the UI could show an error. axios now defaults to ZERO retries;
        // React Query's online/focus-aware retry (createAppQueryClient +
        // per-hook overrides) is authoritative for RQ-backed reads. A non-RQ
        // caller that genuinely wants axios-level retry can still opt in by
        // passing an explicit `_maxRetries > 0`.
        //
        // Use nullish coalescing (not `||`) so an explicit `_maxRetries: 0`
        // from a caller/test is honoured instead of being coerced back to a
        // truthy default.
        config._retryCount = config._retryCount ?? 0;
        config._maxRetries = config._maxRetries ?? 0;
        config._retryDelay = config._retryDelay ?? 1000;

        // GET responses are UNCACHED by default (C1). The interceptor used to
        // cache every GET for 5 min (`_useCache !== false`), a second cache
        // layer React Query could not see that silently served stale data
        // underneath SSE events, RQ invalidations, and every poll. Caching is
        // now explicit opt-in — only genuinely static reference reads
        // (supported currencies/languages, plan config) pass `_useCache: true`.
        // Realtime/polling reads simply hit the network; RQ owns request
        // caching for RQ-backed reads.
        if (config.method?.toLowerCase() === 'get') {
            config._useCache = config._useCache === true; // Default to false; opt-in only
            config._cacheTTL = config._cacheTTL || 5 * 60 * 1000; // 5 minutes when opted in

            // Check cache for GET requests — return cached response directly.
            // We throw a recognizable cancel so the error interceptor can resolve
            // it, avoiding a real network request entirely. The cache holds only
            // the serializable payload (M6): reconstruct a minimal, explicit
            // AxiosResponse from it plus this request's live config.
            if (config._useCache && config.url) {
                const cached = apiCache.get<CachedGetEntry>(config.url, config.params);
                if (cached) {
                    const reconstructed: AxiosResponse = {
                        data: cached.data,
                        status: cached.status,
                        statusText: cached.statusText,
                        headers: {},
                        config,
                        request: undefined,
                    };
                    return Promise.reject(new CachedResponseCancel(reconstructed));
                }
            }
        }

        const existingRequestId =
          (config.headers as Record<string, unknown> | undefined)?.["X-Request-Id"];
        if (!existingRequestId) {
          config.headers['X-Request-Id'] = randomUUID();
        }

        return config;
    },
    (error) => {
        return Promise.reject(error);
    },
);

axiosInstance.interceptors.response.use(
    (response: AxiosResponse) => {
        const config = response.config as RetryConfig;

        // Cache successful GET responses. Persist ONLY the serializable payload
        // and status line (M6) — never the live response's headers/config/request,
        // which do not survive being re-served minutes later.
        if (config._useCache && config.method?.toLowerCase() === 'get' && config.url) {
            const entry: CachedGetEntry = {
                data: response.data,
                status: response.status,
                statusText: response.statusText,
            };
            apiCache.set(config.url, entry, config.params, config._cacheTTL);
        }

        // Clear cache on mutations (POST, PUT, DELETE, PATCH)
        const method = config.method?.toLowerCase();
        if (method && ['post', 'put', 'delete', 'patch'].includes(method)) {
            clearCacheForMutation(config.url || '');
        }

        return response;
    },
    async (error: AxiosError | Cancel | unknown) => {
        // Handle cached responses — these are Cancel objects, not real errors,
        // so monitoring tools (Sentry, etc.) that filter on axios.isCancel will
        // ignore them. The response was reconstructed at request time (M6) and
        // carried by reference on the cancel, so no lossy JSON round-trip runs.
        if (axios.isCancel(error) && isCachedResponseCancel(error)) {
            return Promise.resolve(error.cachedResponse);
        }

        // In-app tab switches abort in-flight GETs. Preserve the cancel
        // identity (do not sanitize into a generic network Error) and never
        // toast — DispatchConsole treats this as a non-failure (#715).
        if (isAbortError(error) || axios.isCancel(error)) {
            return Promise.reject(error);
        }

        const axiosErr = error as AxiosError;
        const config = axiosErr.config as RetryConfig;

        // Skip the refresh-on-401 path for auth endpoints themselves: a 401
        // from POST /auth/logout (called when the session is already gone) or
        // /auth/refresh (the refresh attempt itself) shouldn't re-enter this
        // path and try to refresh again. Bubble the error up cleanly.
        const requestPath =
          typeof config?.url === "string" ? config.url : "";
        const isAuthEndpoint = AUTH_REFRESH_SKIP_PATHS.some((path) =>
          requestPath.includes(path),
        );
        const isCustomerRequest = isCustomerAPIPath(requestPath);

        // Public business pages (`/b/<customUrl>`) are visited by unauthenticated
        // diners. Auth cookies are HttpOnly so we cannot probe them from JS;
        // skip the refresh attempt purely on pathname to avoid the per-page
        // 401 noise burst.
        const guestRoute = isGuestRoute();

        // Attempt token refresh on 401 before giving up. Use the detailed
        // shared helper so AUTH_REFRESH_ROTATED (benign concurrent race) does
        // not fire auth:session-expired — only a genuinely dead session does.
        // RBAC / wrong-business 401s are not session death: refreshing would
        // burn the one-time refresh token and can bounce a live operator.
        const responseCode =
          typeof (axiosErr.response?.data as { code?: unknown } | undefined)
            ?.code === "string"
            ? (axiosErr.response?.data as { code: string }).code
            : undefined;
        if (
          axiosErr.response?.status === 401 &&
          config &&
          !config._isRefreshRetry &&
          !config._skipAuthRefresh &&
          !isAuthEndpoint &&
          !isNonSessionAuthFailure(responseCode) &&
          (isCustomerRequest || !guestRoute)
        ) {
            const apiUrl = getPublicConfig().apiUrl;
            const refreshResult = isCustomerRequest
              ? await sharedRefreshCustomerAuthSession(apiUrl)
              : await sharedRefreshAuthSession(apiUrl);

            // Retry only after confirmed recovery. An unrecovered rotation is
            // non-dead, but replaying the original request would falsely treat
            // an inconclusive cookie-session probe as authenticated.
            if (refreshResult.ok) {
                config._isRefreshRetry = true;
                return axiosInstance(config);
            }

            if (refreshResult.sessionDead) {
                apiCache.endSession();
                if (typeof window !== 'undefined') {
                    window.dispatchEvent(
                      new CustomEvent(
                        isCustomerRequest
                          ? CUSTOMER_SESSION_EXPIRED_EVENT
                          : 'auth:session-expired',
                      ),
                    );
                }
            }
        }

        // Queue mutations for offline replay when the device is offline and
        // the endpoint is safe to retry later (not auth or payments).
        if (
            config &&
            !axiosErr.response &&
            typeof navigator !== "undefined" &&
            !navigator.onLine
        ) {
            const method = (config.method || "").toUpperCase();
            const url = config.url || "";
            const isMutation = ["POST", "PUT", "PATCH", "DELETE"].includes(method);
            const NEVER_QUEUE = ["/auth/", "/payments/", "/push-subscriptions"];
            const isSafe = !NEVER_QUEUE.some((p) => url.includes(p));
            const userId = getActiveUserId();

            if (isMutation && isSafe && userId) {
                const queued = enqueue(userId, {
                    method,
                    url,
                    body: config.data,
                });
                if (queued) {
                    emitErrorToast("offline-queued", "Action saved — will sync when you reconnect");
                    // M2 — reject with a typed OfflineQueuedError instead of
                    // resolving a fabricated 202. Typed callers used to read
                    // `res.data.success`/`res.data.bill` off the fake body, find
                    // them undefined, and drop into a failure branch while the
                    // interceptor toasted "saved". A rejection lets callers
                    // recognise the queued-pending state explicitly (via
                    // isOfflineQueuedError) and skip their hard-error handling.
                    return Promise.reject(new OfflineQueuedError(method, url));
                }
            }
        }

        // Don't retry if no config or if we shouldn't retry this error
        if (!config || !shouldRetry(axiosErr)) {
            maybeToastFromAxiosError(axiosErr);
            return Promise.reject(toSanitizedError(axiosErr));
        }

        // Don't retry if we've exceeded max retries
        if (config._retryCount! >= config._maxRetries!) {
            maybeToastFromAxiosError(axiosErr);
            return Promise.reject(toSanitizedError(axiosErr));
        }

        // Increment retry count
        config._retryCount = (config._retryCount || 0) + 1;

        // Calculate delay with exponential backoff and jitter
        const baseDelay = config._retryDelay || 1000;
        const exponentialDelay = baseDelay * Math.pow(2, config._retryCount - 1);
        const maxDelay = 10000; // Cap at 10 seconds
        const delayWithJitter = Math.min(exponentialDelay, maxDelay) + Math.random() * 1000;

        // Wait before retrying
        await delay(delayWithJitter);

        // Retry the request
        return axiosInstance(config);
    },
);
