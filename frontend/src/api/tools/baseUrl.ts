import { getPublicConfig } from "@/config/publicConfig";

/**
 * Single source of truth for the API base URL used by raw `fetch()` clients.
 *
 * Most API modules go through `axiosInstance` (see ./instance.ts), whose
 * baseURL is already configured — those call relative paths like `/admin/users`.
 *
 * A few modules deliberately use raw `fetch()` instead of `axiosInstance`
 * (axios short-circuits silently under SSR — see the
 * `axios_unsafe_in_server_components` gotcha). Those clients use this value
 * rather than each re-declaring the same literal.
 *
 * Resolved from runtime public config (default `/api/v1`, same-origin). The
 * browser reads window.__PAYVERGE_ENV__, which the root layout injects before
 * any page client module evaluates, so a module-level constant is safe there.
 * Do not import this from instrumentation-client (it can run earlier).
 */
function getApiBaseUrl(): string {
  return getPublicConfig().apiUrl;
}

export const API_BASE_URL = getApiBaseUrl();
