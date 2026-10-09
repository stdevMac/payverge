// Shared /auth/refresh client. Both the proactive timer (HybridAuthProvider)
// and the axios 401 interceptor must go through this so concurrent callers
// share one in-flight POST. Refresh tokens are one-time-use (CAS rotation);
// two parallel POSTs make the loser get AUTH_REFRESH_ROTATED even though the
// session is still alive — and clearing payverge_had_session on that 401 used
// to permanently kill proactive refresh ~15 minutes after login.

import {
  authRefreshCoordinator,
  type AuthRefreshCoordinator,
} from "./authRefreshCoordinator";

export const SESSION_HINT_KEY = "payverge_had_session";

export type RefreshAuthResult =
  | { ok: true }
  | {
      ok: false;
      status: number;
      code?: string;
      // True when the server rejected a just-rotated refresh token. The
      // session may still be live, but recovery has not necessarily completed.
      alreadyRotated: boolean;
      // True when the refresh_token is genuinely dead/revoked/missing.
      sessionDead: boolean;
    };

const MAX_REFRESH_ATTEMPTS = 2;
const REFRESH_REQUEST_TIMEOUT_MS = 30_000;
const ROTATED_RECOVERY_WAIT_MS = 7_000;
const SESSION_PROBE_TIMEOUT_MS = 5_000;

export interface RefreshAuthClientDependencies {
  coordinator?: AuthRefreshCoordinator;
  fetch?: typeof fetch;
  refreshRequestTimeoutMs?: number;
  rotatedRecoveryWaitMs?: number;
  sessionProbeTimeoutMs?: number;
}

class RefreshRejected extends Error {
  constructor(readonly result: Exclude<RefreshAuthResult, { ok: true }>) {
    super("Auth refresh was rejected");
    this.name = "RefreshRejected";
  }
}

async function parseRefreshBody(
  response: Response,
): Promise<{ code?: string; error?: string }> {
  try {
    const data = (await response.clone().json()) as {
      code?: unknown;
      error?: unknown;
    };
    return {
      code: typeof data.code === "string" ? data.code : undefined,
      error: typeof data.error === "string" ? data.error : undefined,
    };
  } catch {
    return {};
  }
}

function isAlreadyRotated(code?: string): boolean {
  return code === "AUTH_REFRESH_ROTATED";
}

async function performRefresh(
  apiUrl: string,
  fetchFn: typeof fetch,
  timeoutMs: number,
): Promise<RefreshAuthResult> {
  let lastNetworkError = false;

  for (let attempt = 0; attempt < MAX_REFRESH_ATTEMPTS; attempt++) {
    const controller = new AbortController();
    let timeout: ReturnType<typeof setTimeout> | undefined;
    try {
      // Mirror refreshCustomerAuth: race the POST against a hard deadline so a
      // stalled LB / blackholed connection cannot wedge every awaiter forever.
      const timeoutPromise = new Promise<never>((_, reject) => {
        timeout = setTimeout(() => {
          controller.abort();
          reject(new Error("Auth refresh request timed out"));
        }, timeoutMs);
      });
      const response = await Promise.race([
        fetchFn(`${apiUrl}/auth/refresh`, {
          method: "POST",
          credentials: "include",
          signal: controller.signal,
        }),
        timeoutPromise,
      ]);

      if (response.ok) {
        return { ok: true };
      }

      const { code } = await parseRefreshBody(response);
      const alreadyRotated = isAlreadyRotated(code);
      const sessionDead =
        !alreadyRotated &&
        (response.status === 401 || response.status === 403);

      // 5xx / 429 are transient — retry once inside this coalesced call.
      if (
        !sessionDead &&
        !alreadyRotated &&
        (response.status >= 500 || response.status === 429) &&
        attempt < MAX_REFRESH_ATTEMPTS - 1
      ) {
        continue;
      }

      return {
        ok: false,
        status: response.status,
        code,
        alreadyRotated,
        sessionDead,
      };
    } catch {
      lastNetworkError = true;
      if (attempt < MAX_REFRESH_ATTEMPTS - 1) continue;
    } finally {
      if (timeout !== undefined) clearTimeout(timeout);
    }
  }

  // Network/CORS/timeout after retries — not a dead session; caller should retry later.
  return {
    ok: false,
    status: lastNetworkError ? 0 : 500,
    alreadyRotated: false,
    sessionDead: false,
  };
}

// #821: the probe is tri-state. "alive"/"dead" are explicit server answers;
// network errors, timeouts, and 5xx/429 are "unknown" — a transient outage
// must never be read as "not authenticated" (that escalated a Cmd-K burst
// into the full sign-in wall on a still-live session).
type SessionProbeResult = "alive" | "dead" | "unknown";

async function probeCookieSession(
  apiUrl: string,
  fetchFn: typeof fetch,
  timeoutMs: number,
): Promise<SessionProbeResult> {
  const controller = new AbortController();
  const abortTimeout = setTimeout(() => controller.abort(), timeoutMs);
  try {
    const response = await fetchFn(`${apiUrl}/auth/session-info`, {
      method: "GET",
      credentials: "include",
      signal: controller.signal,
    });
    if (!response.ok) {
      // 401/403 is the server saying "no session". Anything else (5xx, 429,
      // proxy errors) proves nothing about the cookie session.
      return response.status === 401 || response.status === 403
        ? "dead"
        : "unknown";
    }
    const body = (await response.json()) as { authenticated?: unknown };
    return body.authenticated === true ? "alive" : "dead";
  } catch {
    return "unknown";
  } finally {
    clearTimeout(abortTimeout);
  }
}

const createRefreshAuthClientController = (
  dependencies: RefreshAuthClientDependencies = {},
): {
  refresh: (apiUrl: string) => Promise<RefreshAuthResult>;
  reset: () => void;
} => {
  const coordinator = dependencies.coordinator ?? authRefreshCoordinator;
  // Keep the production lookup dynamic so test realms and browser polyfills
  // can replace global fetch after this module is evaluated.
  const fetchFn =
    dependencies.fetch ??
    ((input: RequestInfo | URL, init?: RequestInit) =>
      globalThis.fetch(input, init));
  const refreshRequestTimeoutMs =
    typeof dependencies.refreshRequestTimeoutMs === "number" &&
    Number.isFinite(dependencies.refreshRequestTimeoutMs) &&
    dependencies.refreshRequestTimeoutMs > 0
      ? Math.max(1, Math.floor(dependencies.refreshRequestTimeoutMs))
      : REFRESH_REQUEST_TIMEOUT_MS;
  const rotatedRecoveryWaitMs =
    dependencies.rotatedRecoveryWaitMs ?? ROTATED_RECOVERY_WAIT_MS;
  const sessionProbeTimeoutMs =
    typeof dependencies.sessionProbeTimeoutMs === "number" &&
    Number.isFinite(dependencies.sessionProbeTimeoutMs) &&
    dependencies.sessionProbeTimeoutMs > 0
      ? Math.max(1, Math.floor(dependencies.sessionProbeTimeoutMs))
      : SESSION_PROBE_TIMEOUT_MS;
  let refreshPromise: Promise<RefreshAuthResult> | null = null;

  const runCoordinatedRefresh = async (
    apiUrl: string,
  ): Promise<RefreshAuthResult> => {
    const startingGeneration = coordinator.getGeneration();
    try {
      const coordinated = await coordinator.runExclusive(
        async () => {
          const result = await performRefresh(
            apiUrl,
            fetchFn,
            refreshRequestTimeoutMs,
          );
          // The coordinator publishes after its work resolves, so failures must
          // reject the work even though this public client resolves typed results.
          if (!result.ok) throw new RefreshRejected(result);
          return result;
        },
        startingGeneration,
      );

      if (coordinated.status === "peer-refreshed") return { ok: true };
      return coordinated.value;
    } catch (error) {
      if (!(error instanceof RefreshRejected)) {
        // Coordination/storage failures are retryable and do not prove the
        // cookie session is dead.
        return {
          ok: false,
          status: 0,
          alreadyRotated: false,
          sessionDead: false,
        };
      }

      const result = error.result;
      if (!result.alreadyRotated) {
        // Refresh 401/403 is only a dead session when the access cookie is
        // gone too. A missing Strict refresh cookie (or a stale shadowed
        // refresh_token) used to mark sessionDead and bounce a still-valid
        // operator principal. #821: only an explicit "dead" probe answer may
        // confirm it — "unknown" (network blip, timeout, 5xx) fails open so
        // the caller retries later instead of walling the operator.
        if (result.sessionDead) {
          const probe = await probeCookieSession(
            apiUrl,
            fetchFn,
            sessionProbeTimeoutMs,
          );
          if (probe !== "dead") {
            return { ...result, sessionDead: false };
          }
        }
        return result;
      }

      // Never retry the stale one-time refresh credential. Give the winning
      // realm time to publish, then ask the server whether its fresh cookies
      // are visible in this realm.
      try {
        await coordinator.waitForGenerationAfter(
          startingGeneration,
          rotatedRecoveryWaitMs,
        );
      } catch {
        // A raw cookie-session probe remains useful when coordination metadata
        // is unavailable or storage access fails.
      }

      if (
        (await probeCookieSession(apiUrl, fetchFn, sessionProbeTimeoutMs)) ===
        "alive"
      ) {
        return { ok: true };
      }
      return result;
    }
  };

  return {
    refresh: (apiUrl: string): Promise<RefreshAuthResult> => {
      if (!refreshPromise) {
        refreshPromise = runCoordinatedRefresh(apiUrl).finally(() => {
          refreshPromise = null;
        });
      }
      return refreshPromise;
    },
    reset: (): void => {
      refreshPromise = null;
    },
  };
};

/** Creates an isolated realm-local client while sharing injected browser coordination. */
export function createRefreshAuthClient(
  dependencies: RefreshAuthClientDependencies = {},
): (apiUrl: string) => Promise<RefreshAuthResult> {
  return createRefreshAuthClientController(dependencies).refresh;
}

const productionRefreshClient = createRefreshAuthClientController();

/**
 * Deduplicated POST /auth/refresh. Concurrent callers share one in-flight
 * request so a timer tick racing the axios 401 interceptor cannot burn the
 * one-time refresh token twice.
 */
export function refreshAuthSession(apiUrl: string): Promise<RefreshAuthResult> {
  return productionRefreshClient.refresh(apiUrl);
}

/** Test-only: drop the in-flight promise so suites don't leak across cases. */
export function __resetRefreshAuthForTests(): void {
  productionRefreshClient.reset();
}
