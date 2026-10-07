import {
  createAuthRefreshCoordinator,
  type AuthRefreshCoordinator,
} from "./authRefreshCoordinator";

export type RefreshCustomerAuthResult =
  | { ok: true }
  | {
      ok: false;
      status: number;
      code?: string;
      alreadyRotated: boolean;
      sessionDead: boolean;
    };

export interface RefreshCustomerAuthClientDependencies {
  coordinator?: AuthRefreshCoordinator;
  fetch?: typeof fetch;
  refreshRequestTimeoutMs?: number;
  rotatedRecoveryWaitMs?: number;
  sessionProbeTimeoutMs?: number;
}

export const CUSTOMER_SESSION_EXPIRED_EVENT = "customer:session-expired";
export const CUSTOMER_SESSION_HINT_KEY = "payverge_had_customer_session";

const MAX_REFRESH_ATTEMPTS = 2;
const REFRESH_REQUEST_TIMEOUT_MS = 30_000;
const ROTATED_RECOVERY_WAIT_MS = 7_000;
const SESSION_PROBE_TIMEOUT_MS = 5_000;

class CustomerRefreshRejected extends Error {
  constructor(
    readonly result: Exclude<RefreshCustomerAuthResult, { ok: true }>,
  ) {
    super("Customer auth refresh was rejected");
    this.name = "CustomerRefreshRejected";
  }
}

const parseResponseBody = async (
  response: Response,
): Promise<{ code?: string; error?: string }> => {
  try {
    const body = (await response.clone().json()) as {
      code?: unknown;
      error?: unknown;
    };
    return {
      code: typeof body.code === "string" ? body.code : undefined,
      error: typeof body.error === "string" ? body.error : undefined,
    };
  } catch {
    return {};
  }
};

const isAlreadyRotated = (code?: string): boolean =>
  code === "AUTH_REFRESH_ROTATED";

const performCustomerRefresh = async (
  apiUrl: string,
  fetchFn: typeof fetch,
  timeoutMs: number,
): Promise<RefreshCustomerAuthResult> => {
  for (let attempt = 0; attempt < MAX_REFRESH_ATTEMPTS; attempt += 1) {
    const controller = new AbortController();
    let timeout: ReturnType<typeof setTimeout> | undefined;
    try {
      const timeoutPromise = new Promise<never>((_, reject) => {
        timeout = setTimeout(() => {
          controller.abort();
          reject(new Error("Customer refresh request timed out"));
        }, timeoutMs);
      });
      const response = await Promise.race([
        fetchFn(`${apiUrl}/customer/refresh`, {
          method: "POST",
          credentials: "include",
          signal: controller.signal,
        }),
        timeoutPromise,
      ]);
      if (response.ok) return { ok: true };

      const { code } = await parseResponseBody(response);
      const alreadyRotated = isAlreadyRotated(code);
      const sessionDead =
        !alreadyRotated &&
        response.status === 401;
      const retryable =
        !alreadyRotated &&
        !sessionDead &&
        (response.status === 429 || response.status >= 500);
      if (retryable && attempt < MAX_REFRESH_ATTEMPTS - 1) continue;

      return {
        ok: false,
        status: response.status,
        code,
        alreadyRotated,
        sessionDead,
      };
    } catch {
      if (attempt < MAX_REFRESH_ATTEMPTS - 1) continue;
    } finally {
      if (timeout !== undefined) clearTimeout(timeout);
    }
  }

  return {
    ok: false,
    status: 0,
    alreadyRotated: false,
    sessionDead: false,
  };
};

const probeCustomerSession = async (
  apiUrl: string,
  fetchFn: typeof fetch,
  timeoutMs: number,
): Promise<boolean> => {
  const controller = new AbortController();
  const timeout = setTimeout(() => controller.abort(), timeoutMs);
  try {
    const response = await fetchFn(`${apiUrl}/customer/session-info`, {
      method: "GET",
      credentials: "include",
      signal: controller.signal,
    });
    if (!response.ok) return false;
    const body = (await response.json()) as {
      authenticated?: unknown;
      type?: unknown;
    };
    return body.authenticated === true && body.type === "customer";
  } catch {
    return false;
  } finally {
    clearTimeout(timeout);
  }
};

const customerAuthRefreshCoordinator = createAuthRefreshCoordinator({
  scope: "customer",
});

const createClientController = (
  dependencies: RefreshCustomerAuthClientDependencies = {},
): {
  refresh: (apiUrl: string) => Promise<RefreshCustomerAuthResult>;
  reset: () => void;
} => {
  const coordinator =
    dependencies.coordinator ?? customerAuthRefreshCoordinator;
  const fetchFn =
    dependencies.fetch ??
    ((input: RequestInfo | URL, init?: RequestInit) =>
      globalThis.fetch(input, init));
  const recoveryWaitMs =
    dependencies.rotatedRecoveryWaitMs ?? ROTATED_RECOVERY_WAIT_MS;
  const refreshRequestTimeoutMs =
    typeof dependencies.refreshRequestTimeoutMs === "number" &&
    Number.isFinite(dependencies.refreshRequestTimeoutMs) &&
    dependencies.refreshRequestTimeoutMs > 0
      ? Math.max(1, Math.floor(dependencies.refreshRequestTimeoutMs))
      : REFRESH_REQUEST_TIMEOUT_MS;
  const probeTimeoutMs =
    typeof dependencies.sessionProbeTimeoutMs === "number" &&
    Number.isFinite(dependencies.sessionProbeTimeoutMs) &&
    dependencies.sessionProbeTimeoutMs > 0
      ? Math.floor(dependencies.sessionProbeTimeoutMs)
      : SESSION_PROBE_TIMEOUT_MS;
  let inFlight: Promise<RefreshCustomerAuthResult> | null = null;

  const run = async (apiUrl: string): Promise<RefreshCustomerAuthResult> => {
    const startingGeneration = coordinator.getGeneration();
    try {
      const coordinated = await coordinator.runExclusive(
        async () => {
          const result = await performCustomerRefresh(
            apiUrl,
            fetchFn,
            refreshRequestTimeoutMs,
          );
          if (!result.ok) throw new CustomerRefreshRejected(result);
          return result;
        },
        startingGeneration,
      );
      if (coordinated.status === "peer-refreshed") return { ok: true };
      return coordinated.value;
    } catch (error) {
      if (!(error instanceof CustomerRefreshRejected)) {
        return {
          ok: false,
          status: 0,
          alreadyRotated: false,
          sessionDead: false,
        };
      }
      if (!error.result.alreadyRotated) return error.result;

      try {
        await coordinator.waitForGenerationAfter(
          startingGeneration,
          recoveryWaitMs,
        );
      } catch {
        // The cookie probe still works if coordination metadata is unavailable.
      }
      if (await probeCustomerSession(apiUrl, fetchFn, probeTimeoutMs)) {
        return { ok: true };
      }
      return error.result;
    }
  };

  return {
    refresh: (apiUrl) => {
      if (!inFlight) {
        inFlight = run(apiUrl).finally(() => {
          inFlight = null;
        });
      }
      return inFlight;
    },
    reset: () => {
      inFlight = null;
    },
  };
};

export const createRefreshCustomerAuthClient = (
  dependencies: RefreshCustomerAuthClientDependencies = {},
): ((apiUrl: string) => Promise<RefreshCustomerAuthResult>) =>
  createClientController(dependencies).refresh;

const productionClient = createClientController();

export const refreshCustomerAuthSession = (
  apiUrl: string,
): Promise<RefreshCustomerAuthResult> => productionClient.refresh(apiUrl);

export const __resetRefreshCustomerAuthForTests = (): void => {
  productionClient.reset();
};
