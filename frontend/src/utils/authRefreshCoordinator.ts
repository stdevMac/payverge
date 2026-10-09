export const AUTH_REFRESH_LEASE_KEY = "payverge:auth-refresh:lease";
export const AUTH_REFRESH_GENERATION_KEY = "payverge:auth-refresh:generation";
export const CUSTOMER_AUTH_REFRESH_LEASE_KEY =
  "payverge:customer-auth-refresh:lease";
export const CUSTOMER_AUTH_REFRESH_GENERATION_KEY =
  "payverge:customer-auth-refresh:generation";

const AUTH_REFRESH_LOCK_NAME = "payverge:auth-refresh";
const AUTH_REFRESH_CHANNEL_NAME = "payverge:auth-refresh";
const CUSTOMER_AUTH_REFRESH_LOCK_NAME = "payverge:customer-auth-refresh";
const CUSTOMER_AUTH_REFRESH_CHANNEL_NAME = "payverge:customer-auth-refresh";
const DEFAULT_LEASE_DURATION_MS = 5_000;
const DEFAULT_POLL_INTERVAL_MS = 50;
const DEFAULT_WAIT_TIMEOUT_MS = 7_000;
/** Strictly longer than operator REFRESH_REQUEST_TIMEOUT_MS (30s). */
const DEFAULT_LOCK_WAIT_TIMEOUT_MS = 45_000;
const MAX_GENERATION = Number.MAX_SAFE_INTEGER - 1;

export interface AuthRefreshStorage {
  getItem(key: string): string | null;
  setItem(key: string, value: string): void;
  removeItem(key: string): void;
}

interface AuthRefreshLockRequestOptions {
  signal?: AbortSignal;
}

export interface AuthRefreshLockManager {
  request<T>(
    name: string,
    callback: () => Promise<T> | T,
    options?: AuthRefreshLockRequestOptions,
  ): Promise<T>;
  tryRequest?<T>(
    name: string,
    callback: () => Promise<T> | T,
  ): Promise<AuthRefreshTryLockResult<T>>;
}

type AuthRefreshTryLockResult<T> =
  | { status: "acquired"; value: T }
  | { status: "unavailable" };

export interface AuthRefreshChannel {
  onmessage: ((event: { data: unknown }) => void) | null;
  postMessage(message: unknown): void;
  close(): void;
}

export interface AuthRefreshCoordinatorEnvironment {
  scope?: AuthRefreshCoordinationScope;
  now?: () => number;
  monotonicNow?: () => number;
  createOwnerId?: () => string;
  storage?: AuthRefreshStorage;
  locks?: AuthRefreshLockManager;
  createChannel?: (name: string) => AuthRefreshChannel;
  subscribeToStorage?: (
    listener: (key: string | null) => void,
  ) => () => void;
  sleep?: (milliseconds: number) => Promise<void>;
  schedule?: (callback: () => void, milliseconds: number) => () => void;
  leaseDurationMs?: number;
  pollIntervalMs?: number;
  waitTimeoutMs?: number;
  /** Bound for waiting to acquire navigator.locks when a peer holds the lock.
   * Must stay strictly longer than the operator refresh fetch timeout (30s) so
   * a healthy lock-holder can finish its own attempt first. Default 45s. */
  lockWaitTimeoutMs?: number;
}

type AuthRefreshCoordinationScope = "auth" | "customer";

interface AuthRefreshCoordinationNames {
  leaseKey: string;
  generationKey: string;
  lockName: string;
  channelName: string;
}

const coordinationNames = (
  scope: AuthRefreshCoordinationScope | undefined,
): AuthRefreshCoordinationNames => {
  if (scope === undefined || scope === "auth") {
    return {
      leaseKey: AUTH_REFRESH_LEASE_KEY,
      generationKey: AUTH_REFRESH_GENERATION_KEY,
      lockName: AUTH_REFRESH_LOCK_NAME,
      channelName: AUTH_REFRESH_CHANNEL_NAME,
    };
  }
  if (scope === "customer") {
    return {
      leaseKey: CUSTOMER_AUTH_REFRESH_LEASE_KEY,
      generationKey: CUSTOMER_AUTH_REFRESH_GENERATION_KEY,
      lockName: CUSTOMER_AUTH_REFRESH_LOCK_NAME,
      channelName: CUSTOMER_AUTH_REFRESH_CHANNEL_NAME,
    };
  }
  throw new Error("Unsupported auth refresh coordination scope");
};

export type AuthRefreshCoordinationResult<T> =
  | { status: "executed"; value: T; generation: number }
  | { status: "peer-refreshed"; generation: number };

export interface AuthRefreshCoordinator {
  getGeneration(): number;
  runExclusive<T>(
    work: () => Promise<T>,
    observedGeneration?: number,
  ): Promise<AuthRefreshCoordinationResult<T>>;
  waitForGenerationAfter(
    generation: number,
    timeoutMs?: number,
  ): Promise<number | null>;
  dispose(): void;
}

interface RefreshLease {
  owner: string;
  expiresAt: number;
}

interface RefreshGeneration {
  generation: number;
}

type StorageReadResult =
  | { status: "available"; value: string | null }
  | { status: "unavailable" };

type LeaseAcquisition =
  | { status: "acquired"; token: string }
  | { status: "contended" }
  | { status: "unavailable" };

type LeaseOwnership = "owned" | "lost" | "unavailable";

const parseGeneration = (value: unknown): number | null => {
  if (
    typeof value === "object" &&
    value !== null &&
    "generation" in value &&
    typeof value.generation === "number" &&
    Number.isSafeInteger(value.generation) &&
    value.generation >= 0 &&
    value.generation <= MAX_GENERATION
  ) {
    return value.generation;
  }
  return null;
};

const parseGenerationMessage = (value: unknown): number | null => {
  try {
    if (
      typeof value !== "object" ||
      value === null ||
      Array.isArray(value) ||
      Object.keys(value).length !== 1 ||
      !Object.prototype.hasOwnProperty.call(value, "generation")
    ) {
      return null;
    }
    const generation = parseGeneration(value);
    return generation !== null && generation > 0 ? generation : null;
  } catch {
    return null;
  }
};

const isNewerGeneration = (
  candidate: number,
  current: number,
): boolean => {
  if (candidate === 0 || candidate === current) return false;
  if (current === 0) return true;
  const forwardDistance =
    candidate > current
      ? candidate - current
      : MAX_GENERATION - current + candidate;
  return forwardDistance <= Math.floor(MAX_GENERATION / 2);
};

const generationChanged = (current: number, captured: number): boolean =>
  current !== 0 && current !== captured;

const parseLease = (value: string | null): RefreshLease | null => {
  if (value === null) return null;
  try {
    const parsed: unknown = JSON.parse(value);
    if (
      typeof parsed === "object" &&
      parsed !== null &&
      "owner" in parsed &&
      "expiresAt" in parsed &&
      typeof parsed.owner === "string" &&
      typeof parsed.expiresAt === "number" &&
      Number.isSafeInteger(parsed.expiresAt) &&
      parsed.expiresAt >= 0
    ) {
      return { owner: parsed.owner, expiresAt: parsed.expiresAt };
    }
  } catch {
    return null;
  }
  return null;
};

const browserStorage = (): AuthRefreshStorage | undefined => {
  if (typeof window === "undefined") return undefined;
  try {
    return window.localStorage;
  } catch {
    return undefined;
  }
};

const browserLocks = (): AuthRefreshLockManager | undefined => {
  if (typeof window === "undefined" || !navigator.locks) return undefined;
  return {
    request<T>(
      name: string,
      callback: () => Promise<T> | T,
      options?: AuthRefreshLockRequestOptions,
    ): Promise<T> {
      const runWhileLocked = async (): Promise<T> => {
        const result = await callback();
        return result;
      };
      // lib.dom models the native generic as the callback's direct return type.
      // Inferring Promise<T> here both keeps the Web Lock through async work and
      // lets Promise chaining flatten the declaration's Promise<Promise<T>>.
      // Pass AbortSignal so a wedged peer cannot pin this tab forever.
      const lockedWork =
        options?.signal !== undefined
          ? navigator.locks.request(
              name,
              { signal: options.signal },
              runWhileLocked,
            )
          : navigator.locks.request(name, runWhileLocked);
      return lockedWork.then((result) => result);
    },
    tryRequest<T>(
      name: string,
      callback: () => Promise<T> | T,
    ): Promise<AuthRefreshTryLockResult<T>> {
      const lockedWork = navigator.locks.request(
        name,
        { ifAvailable: true },
        async (lock): Promise<AuthRefreshTryLockResult<T>> => {
          if (!lock) return { status: "unavailable" };
          return { status: "acquired", value: await callback() };
        },
      );
      return lockedWork.then((result) => result);
    },
  };
};

const browserChannelFactory =
  (): ((name: string) => AuthRefreshChannel) | undefined => {
    if (typeof window === "undefined" || typeof BroadcastChannel === "undefined") {
      return undefined;
    }
    return (name) => {
      const nativeChannel = new BroadcastChannel(name);
      let messageHandler: AuthRefreshChannel["onmessage"] = null;
      nativeChannel.onmessage = (event: MessageEvent<unknown>): void => {
        messageHandler?.({ data: event.data });
      };
      return {
        get onmessage() {
          return messageHandler;
        },
        set onmessage(handler) {
          messageHandler = handler;
        },
        postMessage: (message): void => nativeChannel.postMessage(message),
        close: (): void => nativeChannel.close(),
      };
    };
  };

const browserStorageSubscription =
  ():
    | ((listener: (key: string | null) => void) => () => void)
    | undefined => {
    if (typeof window === "undefined") return undefined;
    return (listener) => {
      const handleStorage = (event: StorageEvent): void => listener(event.key);
      window.addEventListener("storage", handleStorage);
      return () => window.removeEventListener("storage", handleStorage);
    };
  };

const createBrowserOwnerId = (): string => {
  if (typeof window !== "undefined" && globalThis.crypto?.randomUUID) {
    return globalThis.crypto.randomUUID();
  }
  return `${Date.now()}-${Math.random().toString(36).slice(2)}`;
};

const defaultSleep = (milliseconds: number): Promise<void> =>
  new Promise((resolve) => {
    setTimeout(resolve, milliseconds);
  });

const defaultMonotonicNow = (): number => {
  if (typeof performance !== "undefined") {
    const value = performance.now();
    if (Number.isFinite(value)) return value;
  }
  return Date.now();
};

const defaultSchedule = (
  callback: () => void,
  milliseconds: number,
): (() => void) => {
  const timeout = setTimeout(callback, milliseconds);
  return () => clearTimeout(timeout);
};

const positiveDuration = (value: number | undefined, fallback: number): number =>
  typeof value === "number" && Number.isFinite(value) && value > 0
    ? Math.max(1, Math.floor(value))
    : fallback;

const boundedTimeout = (value: number | undefined, fallback: number): number => {
  if (value === undefined || !Number.isFinite(value)) return fallback;
  return Math.max(0, Math.floor(value));
};

export const createAuthRefreshCoordinator = (
  environment: AuthRefreshCoordinatorEnvironment = {},
): AuthRefreshCoordinator => {
  const names = coordinationNames(environment.scope);
  const now = environment.now ?? Date.now;
  const monotonicNow = environment.monotonicNow ?? defaultMonotonicNow;
  const storage = environment.storage ?? browserStorage();
  const locks = environment.locks ?? browserLocks();
  const createChannel = environment.createChannel ?? browserChannelFactory();
  const subscribeToStorage =
    environment.subscribeToStorage ?? browserStorageSubscription();
  const sleep = environment.sleep ?? defaultSleep;
  const schedule = environment.schedule ?? defaultSchedule;
  const leaseDurationMs = positiveDuration(
    environment.leaseDurationMs,
    DEFAULT_LEASE_DURATION_MS,
  );
  const pollIntervalMs = positiveDuration(
    environment.pollIntervalMs,
    DEFAULT_POLL_INTERVAL_MS,
  );
  const waitTimeoutMs = positiveDuration(
    environment.waitTimeoutMs,
    DEFAULT_WAIT_TIMEOUT_MS,
  );
  const lockWaitTimeoutMs = positiveDuration(
    environment.lockWaitTimeoutMs,
    DEFAULT_LOCK_WAIT_TIMEOUT_MS,
  );
  const coordinatorOwner = (environment.createOwnerId ?? createBrowserOwnerId)();
  const stabilizationMs = Math.min(
    pollIntervalMs,
    Math.max(1, Math.floor(leaseDurationMs / 4)),
  );
  const wakeWaiters = new Set<() => void>();
  const activeRenewalStops = new Set<() => void>();
  let knownGeneration = 0;
  let acquisitionSequence = 0;
  let lastMonotonicValue = 0;
  let disposed = false;

  const wallTime = (): number => {
    const candidate = now();
    return Number.isFinite(candidate)
      ? Math.min(Number.MAX_SAFE_INTEGER, Math.max(0, Math.floor(candidate)))
      : 0;
  };

  const monotonicTime = (): number => {
    const candidate = monotonicNow();
    if (Number.isFinite(candidate)) {
      lastMonotonicValue = Math.max(lastMonotonicValue, Math.max(0, candidate));
    }
    return lastMonotonicValue;
  };

  const localDeadlineAfter = (milliseconds: number): number =>
    Math.min(Number.MAX_SAFE_INTEGER, monotonicTime() + milliseconds);

  const leaseExpiryAfter = (milliseconds: number): number =>
    Math.min(Number.MAX_SAFE_INTEGER, wallTime() + milliseconds);

  const readStorage = (key: string): StorageReadResult => {
    if (!storage) return { status: "unavailable" };
    try {
      return { status: "available", value: storage.getItem(key) };
    } catch {
      return { status: "unavailable" };
    }
  };

  const writeStorage = (key: string, value: string): boolean => {
    try {
      if (!storage) return false;
      storage.setItem(key, value);
      return true;
    } catch {
      return false;
    }
  };

  const getGeneration = (): number => {
    const storedGeneration = readStorage(names.generationKey);
    if (storedGeneration.status === "available" && storedGeneration.value !== null) {
      try {
        const parsedGeneration = parseGeneration(
          JSON.parse(storedGeneration.value),
        );
        if (
          parsedGeneration !== null &&
          isNewerGeneration(parsedGeneration, knownGeneration)
        ) {
          knownGeneration = parsedGeneration;
        }
      } catch {
        // Ignore malformed coordination metadata.
      }
    }
    return knownGeneration;
  };

  const notifyWaiters = (): void => {
    for (const wake of wakeWaiters) wake();
    wakeWaiters.clear();
  };

  let channel: AuthRefreshChannel | undefined;
  try {
    channel = createChannel?.(names.channelName);
    if (channel) {
      channel.onmessage = (event): void => {
        const storedGeneration = readStorage(names.generationKey);
        if (
          storedGeneration.status === "available" &&
          storedGeneration.value !== null
        ) {
          // Read persisted metadata first as the baseline. A strictly newer
          // channel generation may repair a value left stale by a failed write.
          getGeneration();
        }
        // With storage unreadable, empty, or stale, Web Locks still serialize
        // refreshes but need this non-sensitive change token to let a queued
        // realm skip its work. Modular ordering rejects delayed pre-rollover
        // messages while accepting MAX -> 1 -> 2 as forward progress.
        const candidate = parseGenerationMessage(event.data);
        if (
          candidate !== null &&
          isNewerGeneration(candidate, knownGeneration)
        ) {
          knownGeneration = candidate;
          writeStorage(
            names.generationKey,
            JSON.stringify({ generation: candidate }),
          );
        }
        notifyWaiters();
      };
    }
  } catch {
    channel = undefined;
  }

  const unsubscribeStorage = subscribeToStorage?.((key) => {
    if (
      key === null ||
      key === names.generationKey ||
      key === names.leaseKey
    ) {
      getGeneration();
      notifyWaiters();
    }
  });

  const waitForWakeup = async (milliseconds: number): Promise<void> => {
    if (milliseconds <= 0) return;
    let wake = (): void => undefined;
    const signalled = new Promise<void>((resolve) => {
      wake = resolve;
      wakeWaiters.add(wake);
    });
    await Promise.race([sleep(milliseconds), signalled]);
    wakeWaiters.delete(wake);
  };

  const publishGeneration = (): number => {
    const currentGeneration = getGeneration();
    const candidate = Math.max(wallTime(), currentGeneration + 1);
    // Normal generations increase. Saturation is the sole exception: because
    // this value is a peer change token, roll to 1 instead of getting stuck.
    const generation =
      currentGeneration >= MAX_GENERATION || candidate > MAX_GENERATION
        ? 1
        : candidate;
    knownGeneration = generation;
    const metadata: RefreshGeneration = { generation };
    writeStorage(names.generationKey, JSON.stringify(metadata));
    try {
      channel?.postMessage(metadata);
    } catch {
      // Polling and storage events remain available as wakeups.
    }
    notifyWaiters();
    return generation;
  };

  const execute = async <T>(
    observedGeneration: number,
    work: () => Promise<T>,
  ): Promise<AuthRefreshCoordinationResult<T>> => {
    if (disposed) throw new Error("Auth refresh coordinator disposed");
    const currentGeneration = getGeneration();
    if (generationChanged(currentGeneration, observedGeneration)) {
      return { status: "peer-refreshed", generation: currentGeneration };
    }
    const value = await work();
    if (disposed) {
      throw new Error("Auth refresh coordinator disposed before success publication");
    }
    return { status: "executed", value, generation: publishGeneration() };
  };

  const readLease = ():
    | { status: "available"; lease: RefreshLease | null }
    | { status: "unavailable" } => {
    const storedLease = readStorage(names.leaseKey);
    if (storedLease.status === "unavailable") return storedLease;
    return { status: "available", lease: parseLease(storedLease.value) };
  };

  const leaseOwnership = (token: string): LeaseOwnership => {
    const current = readLease();
    if (current.status === "unavailable") return "unavailable";
    return current.lease?.owner === token && current.lease.expiresAt > wallTime()
      ? "owned"
      : "lost";
  };

  const renewLease = (token: string): boolean => {
    const current = readLease();
    if (current.status === "unavailable" || current.lease?.owner !== token) {
      return false;
    }
    const renewed: RefreshLease = {
      owner: token,
      expiresAt: leaseExpiryAfter(leaseDurationMs),
    };
    if (!writeStorage(names.leaseKey, JSON.stringify(renewed))) {
      return false;
    }
    const stored = readLease();
    return (
      stored.status === "available" &&
      stored.lease?.owner === token &&
      stored.lease.expiresAt === renewed.expiresAt
    );
  };

  const startLeaseRenewal = (token: string): (() => void) => {
    if (disposed) return () => undefined;
    const renewalIntervalMs = Math.max(1, Math.floor(leaseDurationMs / 2));
    let stopped = false;
    let cancelScheduledRenewal = (): void => undefined;
    const stop = (): void => {
      if (stopped) return;
      stopped = true;
      cancelScheduledRenewal();
      activeRenewalStops.delete(stop);
    };
    const renew = (): void => {
      if (stopped) return;
      if (!renewLease(token)) {
        stop();
        return;
      }
      cancelScheduledRenewal = schedule(renew, renewalIntervalMs);
    };
    cancelScheduledRenewal = schedule(renew, renewalIntervalMs);
    activeRenewalStops.add(stop);
    return stop;
  };

  const tryAcquireLease = async (): Promise<LeaseAcquisition> => {
    const current = readLease();
    if (current.status === "unavailable") return current;
    if (current.lease && current.lease.expiresAt > wallTime()) {
      return { status: "contended" };
    }
    acquisitionSequence += 1;
    const token = `${coordinatorOwner}:${acquisitionSequence}`;
    const lease: RefreshLease = {
      owner: token,
      expiresAt: leaseExpiryAfter(leaseDurationMs),
    };
    if (!writeStorage(names.leaseKey, JSON.stringify(lease))) {
      return { status: "unavailable" };
    }
    const immediate = readLease();
    if (immediate.status === "unavailable") return immediate;
    if (
      immediate.lease?.owner !== token ||
      immediate.lease.expiresAt !== lease.expiresAt
    ) {
      return { status: "contended" };
    }

    // Yield after the write so a contender that observed stale/missing state
    // can publish its fence before this attempt is allowed to enter work.
    await sleep(stabilizationMs);
    const stabilized = readLease();
    if (stabilized.status === "unavailable") return stabilized;
    if (
      stabilized.lease?.owner !== token ||
      stabilized.lease.expiresAt !== lease.expiresAt ||
      stabilized.lease.expiresAt <= wallTime()
    ) {
      return { status: "contended" };
    }
    return { status: "acquired", token };
  };

  const runWithFallback = async <T>(
    observedGeneration: number,
    work: () => Promise<T>,
  ): Promise<AuthRefreshCoordinationResult<T>> => {
    if (!storage) return execute(observedGeneration, work);

    // Web Locks remains authoritative. Web Storage has no atomic compare/swap,
    // so this fenced/stabilized lease is deliberately best-effort. The server's
    // refresh-token CAS and replay grace are the final authority; expiry only
    // recovers crashed owners and a fully suspended tab cannot be made a mutex.
    const deadline = localDeadlineAfter(waitTimeoutMs);
    while (true) {
      if (disposed) {
        throw new Error("Auth refresh coordinator disposed");
      }
      const currentGeneration = getGeneration();
      if (generationChanged(currentGeneration, observedGeneration)) {
        return { status: "peer-refreshed", generation: currentGeneration };
      }

      const acquisition = await tryAcquireLease();
      if (disposed) {
        throw new Error("Auth refresh coordinator disposed");
      }
      if (acquisition.status === "unavailable") {
        return execute(observedGeneration, work);
      }
      if (acquisition.status === "acquired") {
        const generationAfterAcquiring = getGeneration();
        if (generationChanged(generationAfterAcquiring, observedGeneration)) {
          return {
            status: "peer-refreshed",
            generation: generationAfterAcquiring,
          };
        }

        const stopLeaseRenewal = startLeaseRenewal(acquisition.token);
        try {
          const value = await work();
          if (disposed) {
            throw new Error(
              "Auth refresh coordinator disposed before success publication",
            );
          }
          const ownership = leaseOwnership(acquisition.token);
          if (ownership === "lost") {
            throw new Error(
              "Lost auth refresh lease before publishing success",
            );
          }
          return {
            status: "executed",
            value,
            generation: publishGeneration(),
          };
        } finally {
          stopLeaseRenewal();
        }
      }

      const remainingWaitMs = Math.max(0, deadline - monotonicTime());
      if (remainingWaitMs === 0) break;
      const currentLease = readLease();
      if (currentLease.status === "unavailable") {
        return execute(observedGeneration, work);
      }
      const untilExpiry = currentLease.lease
        ? Math.max(1, currentLease.lease.expiresAt - wallTime())
        : pollIntervalMs;
      const waitMs = Math.min(
        pollIntervalMs,
        untilExpiry,
        remainingWaitMs,
      );
      await waitForWakeup(waitMs);
    }

    throw new Error("Timed out waiting for cross-tab auth refresh coordination");
  };

  return {
    getGeneration,
    runExclusive: async <T>(
      work: () => Promise<T>,
      capturedGeneration?: number,
    ): Promise<AuthRefreshCoordinationResult<T>> => {
      if (disposed) throw new Error("Auth refresh coordinator disposed");
      const observedGeneration = capturedGeneration ?? getGeneration();
      // If storage and BroadcastChannel are both unavailable, Web Locks can
      // serialize realms but cannot tell a queued realm that its peer already
      // succeeded. Duplicate POSTs may then occur; server-side token CAS and
      // AUTH_REFRESH_ROTATED recovery remain the authority. No token is stored.
      if (!locks) return runWithFallback(observedGeneration, work);

      const isAbortError = (error: unknown): boolean =>
        (typeof DOMException !== "undefined" &&
          error instanceof DOMException &&
          error.name === "AbortError") ||
        (error instanceof Error && error.name === "AbortError");

      // Bound the wait for a peer-held Web Lock. A wedged holder must not pin
      // this tab forever; abort surfaces as a coordination failure so the
      // refresh client can treat it like any other non-dead refresh failure.
      // The abort only applies while waiting to acquire — once the callback
      // runs we clear the timer so a slow-but-healthy refresh (≤30s) is not
      // killed mid-flight after a long wait.
      const requestLock = async (
        callback: () => Promise<AuthRefreshCoordinationResult<T>>,
      ): Promise<AuthRefreshCoordinationResult<T>> => {
        const controller = new AbortController();
        let acquired = false;
        const abortTimer = setTimeout(() => {
          if (!acquired) controller.abort();
        }, lockWaitTimeoutMs);
        try {
          return await locks.request(
            names.lockName,
            async () => {
              acquired = true;
              clearTimeout(abortTimer);
              return callback();
            },
            { signal: controller.signal },
          );
        } finally {
          clearTimeout(abortTimer);
        }
      };

      let enteredLock = false;
      try {
        if (locks.tryRequest) {
          const immediate = await locks.tryRequest(names.lockName, async () => {
            enteredLock = true;
            return execute(observedGeneration, work);
          });
          if (immediate.status === "acquired") return immediate.value;

          return await requestLock(async () => {
            enteredLock = true;
            const generationAtGrant = getGeneration();
            if (generationChanged(generationAtGrant, observedGeneration)) {
              return {
                status: "peer-refreshed",
                generation: generationAtGrant,
              };
            }
            if (channel) {
              // BroadcastChannel delivery is task-queued and may trail Web Lock
              // handoff. Confirmed contenders take one bounded wake so the
              // winner's publication can arrive before duplicate work begins.
              await waitForWakeup(Math.min(pollIntervalMs, waitTimeoutMs));
            }
            return execute(observedGeneration, work);
          });
        }

        return await requestLock(async () => {
          enteredLock = true;
          return execute(observedGeneration, work);
        });
      } catch (error) {
        if (enteredLock) throw error;
        // Lock-wait abort is a terminal coordination failure for this attempt —
        // do not fall through to the storage lease path (which would race the
        // still-held peer). refreshAuth maps thrown coordination errors to a
        // non-dead refresh failure so the caller can retry later.
        if (isAbortError(error)) {
          throw new Error(
            "Timed out waiting for cross-tab auth refresh lock",
          );
        }
        return runWithFallback(observedGeneration, work);
      }
    },
    waitForGenerationAfter: async (
      generation: number,
      timeoutMs = waitTimeoutMs,
    ): Promise<number | null> => {
      const deadline = localDeadlineAfter(
        boundedTimeout(timeoutMs, waitTimeoutMs),
      );
      while (!disposed) {
        const currentGeneration = getGeneration();
        if (generationChanged(currentGeneration, generation)) {
          return currentGeneration;
        }
        const remainingWaitMs = Math.max(0, deadline - monotonicTime());
        if (remainingWaitMs === 0) return null;
        const waitMs = Math.min(pollIntervalMs, remainingWaitMs);
        await waitForWakeup(waitMs);
      }
      return null;
    },
    dispose: (): void => {
      disposed = true;
      for (const stopRenewal of [...activeRenewalStops]) stopRenewal();
      unsubscribeStorage?.();
      channel?.close();
      notifyWaiters();
    },
  };
};

export const authRefreshCoordinator = createAuthRefreshCoordinator();
