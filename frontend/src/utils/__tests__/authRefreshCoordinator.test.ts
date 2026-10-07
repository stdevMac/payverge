import {
  AUTH_REFRESH_GENERATION_KEY,
  AUTH_REFRESH_LEASE_KEY,
  CUSTOMER_AUTH_REFRESH_GENERATION_KEY,
  CUSTOMER_AUTH_REFRESH_LEASE_KEY,
  createAuthRefreshCoordinator,
  type AuthRefreshChannel,
  type AuthRefreshLockManager,
  type AuthRefreshStorage,
} from "../authRefreshCoordinator";

class SharedStorage implements AuthRefreshStorage {
  readonly writes: Array<{ key: string; value: string }> = [];

  private readonly values = new Map<string, string>();

  getItem(key: string): string | null {
    return this.values.get(key) ?? null;
  }

  setItem(key: string, value: string): void {
    this.values.set(key, value);
    this.writes.push({ key, value });
  }

  removeItem(key: string): void {
    this.values.delete(key);
  }
}

class StaleInterleavingStorage extends SharedStorage {
  private leaseReads = 0;

  getItem(key: string): string | null {
    if (key === AUTH_REFRESH_LEASE_KEY) {
      this.leaseReads += 1;
      if (this.leaseReads === 1 || this.leaseReads === 3) return null;
    }
    return super.getItem(key);
  }
}

class WriteFailingStorage extends SharedStorage {
  setItem(_key: string, _value: string): void {
    throw new Error("storage writes disabled");
  }
}

class ToggleWriteFailingStorage extends SharedStorage {
  writesDisabled = false;

  setItem(key: string, value: string): void {
    if (this.writesDisabled) throw new Error("storage writes disabled");
    super.setItem(key, value);
  }
}

class ReadFailingStorage extends SharedStorage {
  getItem(_key: string): string | null {
    throw new Error("storage reads disabled");
  }
}

class ToggleReadFailingStorage extends SharedStorage {
  readsDisabled = false;

  getItem(key: string): string | null {
    if (this.readsDisabled) throw new Error("storage reads disabled");
    return super.getItem(key);
  }
}

class RemovalTrackingStorage extends SharedStorage {
  removeCalls = 0;

  removeItem(_key: string): void {
    this.removeCalls += 1;
    throw new Error("storage removal disabled");
  }
}

class SharedLocks implements AuthRefreshLockManager {
  private tail: Promise<void> = Promise.resolve();

  request<T>(
    _name: string,
    callback: () => Promise<T> | T,
  ): Promise<T> {
    const previous = this.tail;
    let release = (): void => undefined;
    this.tail = new Promise<void>((resolve) => {
      release = resolve;
    });

    return previous.then(callback).finally(release);
  }
}

class NonblockingSharedLocks implements AuthRefreshLockManager {
  private locked = false;

  private readonly waiters: Array<() => void> = [];

  private release(): void {
    this.locked = false;
    this.waiters.shift()?.();
  }

  request<T>(
    _name: string,
    callback: () => Promise<T> | T,
  ): Promise<T> {
    return new Promise<T>((resolve, reject) => {
      const run = (): void => {
        this.locked = true;
        Promise.resolve()
          .then(callback)
          .then(resolve, reject)
          .finally(() => this.release());
      };
      if (this.locked) {
        this.waiters.push(run);
      } else {
        run();
      }
    });
  }

  async tryRequest<T>(
    _name: string,
    callback: () => Promise<T> | T,
  ): Promise<
    { status: "acquired"; value: T } | { status: "unavailable" }
  > {
    if (this.locked) return { status: "unavailable" };
    this.locked = true;
    try {
      return { status: "acquired", value: await callback() };
    } finally {
      this.release();
    }
  }
}

class SharedChannels {
  readonly messages: unknown[] = [];

  private readonly channels = new Set<AuthRefreshChannel>();

  broadcast(message: unknown): void {
    this.messages.push(message);
    for (const channel of this.channels) {
      channel.onmessage?.({ data: message });
    }
  }

  create = (): AuthRefreshChannel => {
    const channel: AuthRefreshChannel = {
      onmessage: null,
      postMessage: (message) => {
        this.messages.push(message);
        for (const peer of this.channels) {
          if (peer !== channel) {
            peer.onmessage?.({ data: message });
          }
        }
      },
      close: () => {
        this.channels.delete(channel);
      },
    };
    this.channels.add(channel);
    return channel;
  };
}

class QueuedSharedChannels {
  private readonly channels = new Set<AuthRefreshChannel>();

  create = (): AuthRefreshChannel => {
    const channel: AuthRefreshChannel = {
      onmessage: null,
      postMessage: (message) => {
        for (const peer of this.channels) {
          if (peer !== channel) {
            setTimeout(() => peer.onmessage?.({ data: message }), 0);
          }
        }
      },
      close: () => {
        this.channels.delete(channel);
      },
    };
    this.channels.add(channel);
    return channel;
  };
}

const deferred = <T>() => {
  let resolve = (_value: T): void => undefined;
  const promise = new Promise<T>((promiseResolve) => {
    resolve = promiseResolve;
  });
  return { promise, resolve };
};

describe("auth refresh coordinator", () => {
  test("customer coordination uses endpoint-scoped storage, lock, and channel names", async () => {
    const storage = new SharedStorage();
    const lockNames: string[] = [];
    const channelNames: string[] = [];
    const coordinator = createAuthRefreshCoordinator({
      scope: "customer",
      storage,
      locks: {
        request: async <T,>(name: string, callback: () => Promise<T> | T) => {
          lockNames.push(name);
          return callback();
        },
      },
      createChannel: (name) => {
        channelNames.push(name);
        return { onmessage: null, postMessage: jest.fn(), close: jest.fn() };
      },
      now: () => 900,
    });

    await coordinator.runExclusive(async () => "customer-session");

    expect(lockNames).toEqual(["payverge:customer-auth-refresh"]);
    expect(channelNames).toEqual(["payverge:customer-auth-refresh"]);
    expect(storage.getItem(CUSTOMER_AUTH_REFRESH_GENERATION_KEY)).not.toBeNull();
    expect(storage.getItem(CUSTOMER_AUTH_REFRESH_LEASE_KEY)).toBeNull();
    expect(storage.getItem(AUTH_REFRESH_GENERATION_KEY)).toBeNull();
    expect(storage.getItem(AUTH_REFRESH_LEASE_KEY)).toBeNull();
  });

  test("rejects unknown coordination scopes instead of silently sharing a namespace", () => {
    expect(() =>
      createAuthRefreshCoordinator({ scope: "typo" as "customer" }),
    ).toThrow("Unsupported auth refresh coordination scope");
  });

  test("simultaneous callers execute refresh work exactly once and the waiter resolves from its peer", async () => {
    const storage = new SharedStorage();
    const locks = new SharedLocks();
    const channels = new SharedChannels();
    const firstStarted = deferred<void>();
    const finishFirst = deferred<string>();
    let now = 1_000;
    let executions = 0;
    const common = {
      storage,
      locks,
      createChannel: channels.create,
      now: () => now,
      sleep: async () => undefined,
    };
    const first = createAuthRefreshCoordinator({
      ...common,
      createOwnerId: () => "tab-a",
    });
    const second = createAuthRefreshCoordinator({
      ...common,
      createOwnerId: () => "tab-b",
    });

    const firstResult = first.runExclusive(async () => {
      executions += 1;
      firstStarted.resolve();
      return finishFirst.promise;
    });
    await firstStarted.promise;
    const capturedByWaiter = second.getGeneration();
    const secondResult = second.runExclusive(async () => {
      executions += 1;
      return "unexpected";
    });

    now = 1_001;
    finishFirst.resolve("fresh-session");

    await expect(firstResult).resolves.toEqual({
      status: "executed",
      value: "fresh-session",
      generation: 1_001,
    });
    await expect(secondResult).resolves.toEqual({
      status: "peer-refreshed",
      generation: 1_001,
    });
    expect(executions).toBe(1);
    expect(capturedByWaiter).toBe(0);
    expect(second.getGeneration()).toBeGreaterThan(capturedByWaiter);
  });

  test.each([
    ["reads", () => new ReadFailingStorage()],
    ["writes", () => new WriteFailingStorage()],
  ])("broadcast generation coalesces Web Lock waiters when storage %s are unavailable", async (_mode, createStorage) => {
    const storage = createStorage();
    const locks = new SharedLocks();
    const channels = new SharedChannels();
    const firstStarted = deferred<void>();
    const finishFirst = deferred<string>();
    let now = 1_100;
    let executions = 0;
    const common = {
      storage,
      locks,
      createChannel: channels.create,
      now: () => now,
      sleep: async () => undefined,
    };
    const first = createAuthRefreshCoordinator({
      ...common,
      createOwnerId: () => "storage-disabled-tab-a",
    });
    const second = createAuthRefreshCoordinator({
      ...common,
      createOwnerId: () => "storage-disabled-tab-b",
    });

    const firstResult = first.runExclusive(async () => {
      executions += 1;
      firstStarted.resolve();
      return finishFirst.promise;
    });
    await firstStarted.promise;
    const capturedByWaiter = second.getGeneration();
    const secondResult = second.runExclusive(async () => {
      executions += 1;
      return "unexpected-duplicate";
    });

    now = 1_101;
    finishFirst.resolve("fresh-session");

    await expect(firstResult).resolves.toEqual({
      status: "executed",
      value: "fresh-session",
      generation: 1_101,
    });
    await expect(secondResult).resolves.toEqual({
      status: "peer-refreshed",
      generation: 1_101,
    });
    expect(capturedByWaiter).toBe(0);
    expect(executions).toBe(1);
  });

  test("newer channel generation supersedes readable storage left stale by write failure", async () => {
    const storage = new ToggleWriteFailingStorage();
    storage.setItem(
      AUTH_REFRESH_GENERATION_KEY,
      JSON.stringify({ generation: 1_125 }),
    );
    storage.writesDisabled = true;
    const locks = new SharedLocks();
    const channels = new SharedChannels();
    let executions = 0;
    const common = {
      storage,
      locks,
      createChannel: channels.create,
      now: () => 1_126,
      sleep: async () => undefined,
    };
    const first = createAuthRefreshCoordinator({
      ...common,
      createOwnerId: () => "stale-storage-tab-a",
    });
    const second = createAuthRefreshCoordinator({
      ...common,
      createOwnerId: () => "stale-storage-tab-b",
    });
    const firstGeneration = first.getGeneration();
    const secondGeneration = second.getGeneration();

    const outcomes = await Promise.all([
      first.runExclusive(async () => {
        executions += 1;
        return "fresh-session";
      }, firstGeneration),
      second.runExclusive(async () => {
        executions += 1;
        return "unexpected-duplicate";
      }, secondGeneration),
    ]);

    expect(outcomes).toEqual([
      {
        status: "executed",
        value: "fresh-session",
        generation: 1_126,
      },
      { status: "peer-refreshed", generation: 1_126 },
    ]);
    expect(second.getGeneration()).toBe(1_126);
    expect(executions).toBe(1);
  });

  test("queued BroadcastChannel delivery reaches a just-granted Web Lock before duplicate work", async () => {
    const storage = new ReadFailingStorage();
    const locks = new NonblockingSharedLocks();
    const channels = new QueuedSharedChannels();
    const firstStarted = deferred<void>();
    const finishFirst = deferred<string>();
    let executions = 0;
    let monotonicNow = 0;
    const common = {
      storage,
      locks,
      createChannel: channels.create,
      now: () => 1_175,
      monotonicNow: () => monotonicNow,
      pollIntervalMs: 5,
      waitTimeoutMs: 50,
    };
    const first = createAuthRefreshCoordinator({
      ...common,
      createOwnerId: () => "queued-channel-tab-a",
    });
    const second = createAuthRefreshCoordinator({
      ...common,
      createOwnerId: () => "queued-channel-tab-b",
    });

    const firstResult = first.runExclusive(async () => {
      executions += 1;
      firstStarted.resolve();
      return finishFirst.promise;
    });
    await firstStarted.promise;
    const secondResult = second.runExclusive(async () => {
      executions += 1;
      return "unexpected-duplicate";
    });
    monotonicNow = 1;
    finishFirst.resolve("fresh-session");

    await expect(firstResult).resolves.toMatchObject({ status: "executed" });
    await expect(secondResult).resolves.toEqual({
      status: "peer-refreshed",
      generation: 1_175,
    });
    expect(executions).toBe(1);
  });

  test("an async uncontended nonblocking lock executes without a coordination wake", async () => {
    const storage = new ReadFailingStorage();
    const channels = new SharedChannels();
    const sleep = jest.fn(async () => undefined);
    let monotonicNow = 0;
    const locks = {
      request: async <T,>(
        _name: string,
        callback: () => Promise<T> | T,
      ): Promise<T> => {
        monotonicNow = 1;
        await Promise.resolve();
        return callback();
      },
      tryRequest: async <T,>(
        _name: string,
        callback: () => Promise<T> | T,
      ): Promise<
        { status: "acquired"; value: T } | { status: "unavailable" }
      > => {
        monotonicNow = 1;
        await Promise.resolve();
        return { status: "acquired", value: await callback() };
      },
    };
    const coordinator = createAuthRefreshCoordinator({
      storage,
      locks,
      createChannel: channels.create,
      createOwnerId: () => "async-uncontended-tab",
      monotonicNow: () => monotonicNow,
      now: () => 1_190,
      sleep,
    });
    const work = jest.fn(async () => "fresh-session");

    await expect(coordinator.runExclusive(work)).resolves.toMatchObject({
      status: "executed",
      value: "fresh-session",
    });
    expect(work).toHaveBeenCalledTimes(1);
    expect(sleep).not.toHaveBeenCalled();
  });

  test("storage-disabled channel adoption rejects malformed data and stale generations", () => {
    const storage = new ReadFailingStorage();
    const channels = new SharedChannels();
    const coordinator = createAuthRefreshCoordinator({
      storage,
      createChannel: channels.create,
      createOwnerId: () => "channel-validation-tab",
      now: () => 1_200,
      sleep: async () => undefined,
    });

    for (const payload of [
      null,
      "generation=999",
      { generation: -1 },
      { generation: 1.5 },
      { generation: Number.MAX_SAFE_INTEGER },
      { generation: 999, refreshToken: "must-not-be-accepted" },
    ]) {
      channels.broadcast(payload);
    }
    expect(coordinator.getGeneration()).toBe(0);

    channels.broadcast({ generation: 1_200 });
    expect(coordinator.getGeneration()).toBe(1_200);

    channels.broadcast({ generation: 1_199 });
    expect(coordinator.getGeneration()).toBe(1_200);
  });

  test("degraded channel ordering stays forward-only across generation rollover", () => {
    const storage = new ReadFailingStorage();
    const channels = new SharedChannels();
    const coordinator = createAuthRefreshCoordinator({
      storage,
      createChannel: channels.create,
      createOwnerId: () => "rollover-ordering-tab",
      now: () => 1_250,
      sleep: async () => undefined,
    });
    const ceiling = Number.MAX_SAFE_INTEGER - 1;

    channels.broadcast({ generation: ceiling });
    expect(coordinator.getGeneration()).toBe(ceiling);
    channels.broadcast({ generation: 1 });
    expect(coordinator.getGeneration()).toBe(1);

    channels.broadcast({ generation: ceiling });
    expect(coordinator.getGeneration()).toBe(1);
    channels.broadcast({ generation: 2 });
    expect(coordinator.getGeneration()).toBe(2);
  });

  test("without storage or a channel, Web Locks serialize but cannot suppress duplicate work", async () => {
    const storage = new ReadFailingStorage();
    const locks = new SharedLocks();
    let executions = 0;
    const common = {
      storage,
      locks,
      createChannel: undefined,
      now: () => 1_300,
      sleep: async () => undefined,
    };
    const first = createAuthRefreshCoordinator({
      ...common,
      createOwnerId: () => "fully-degraded-tab-a",
    });
    const second = createAuthRefreshCoordinator({
      ...common,
      createOwnerId: () => "fully-degraded-tab-b",
    });
    const firstGeneration = first.getGeneration();
    const secondGeneration = second.getGeneration();

    const outcomes = await Promise.all([
      first.runExclusive(async () => {
        executions += 1;
        return "first";
      }, firstGeneration),
      second.runExclusive(async () => {
        executions += 1;
        return "second";
      }, secondGeneration),
    ]);

    expect(outcomes.map(({ status }) => status)).toEqual(["executed", "executed"]);
    expect(executions).toBe(2);
  });

  test("failed refresh work does not publish a success generation", async () => {
    const storage = new SharedStorage();
    const coordinator = createAuthRefreshCoordinator({
      storage,
      locks: new SharedLocks(),
      createOwnerId: () => "tab-a",
      now: () => 2_000,
      sleep: async () => undefined,
    });

    await expect(
      coordinator.runExclusive(async () => {
        throw new Error("refresh rejected");
      }),
    ).rejects.toThrow("refresh rejected");

    expect(coordinator.getGeneration()).toBe(0);
    expect(storage.getItem(AUTH_REFRESH_GENERATION_KEY)).toBeNull();
  });

  test("uses an explicit captured generation to short-circuit after peer publication", async () => {
    const storage = new SharedStorage();
    const locks = new SharedLocks();
    const common = {
      storage,
      locks,
      now: () => 2_500,
      sleep: async () => undefined,
    };
    const waitingRealm = createAuthRefreshCoordinator({
      ...common,
      createOwnerId: () => "waiting-realm",
    });
    const peerRealm = createAuthRefreshCoordinator({
      ...common,
      createOwnerId: () => "peer-realm",
    });
    const capturedGeneration = waitingRealm.getGeneration();
    await peerRealm.runExclusive(async () => "peer-success");
    const staleWork = jest.fn(async () => "must-not-run");

    await expect(
      waitingRealm.runExclusive(staleWork, capturedGeneration),
    ).resolves.toEqual({
      status: "peer-refreshed",
      generation: 2_500,
    });
    expect(staleWork).not.toHaveBeenCalled();
  });

  test("fallback does not publish or clobber a successor after fence loss", async () => {
    const storage = new SharedStorage();
    const coordinator = createAuthRefreshCoordinator({
      storage,
      createOwnerId: () => "tab-a",
      now: () => 3_000,
      sleep: async () => undefined,
    });

    await expect(
      coordinator.runExclusive(async () => {
        storage.setItem(
          AUTH_REFRESH_LEASE_KEY,
          JSON.stringify({ owner: "tab-b", expiresAt: 9_000 }),
        );
        return "done";
      }),
    ).rejects.toThrow("Lost auth refresh lease before publishing success");

    expect(JSON.parse(storage.getItem(AUTH_REFRESH_LEASE_KEY) ?? "{}")).toEqual({
      owner: "tab-b",
      expiresAt: 9_000,
    });
    expect(coordinator.getGeneration()).toBe(0);
  });

  test("fallback stabilization fences adversarial stale-read acquisition interleaving", async () => {
    const storage = new StaleInterleavingStorage();
    const channels = new SharedChannels();
    let executions = 0;
    const common = {
      storage,
      createChannel: channels.create,
      now: () => 3_500,
      sleep: async () => undefined,
      leaseDurationMs: 100,
      pollIntervalMs: 10,
      waitTimeoutMs: 100,
    };
    const first = createAuthRefreshCoordinator({
      ...common,
      createOwnerId: () => "tab-a",
    });
    const second = createAuthRefreshCoordinator({
      ...common,
      createOwnerId: () => "tab-b",
    });

    const outcomes = await Promise.allSettled([
      first.runExclusive(async () => {
        executions += 1;
        return "first";
      }),
      second.runExclusive(async () => {
        executions += 1;
        return "second";
      }),
    ]);

    expect(executions).toBe(1);
    expect(outcomes.every(({ status }) => status === "fulfilled")).toBe(true);
    expect(outcomes.map(({ status }) => status)).toEqual([
      "fulfilled",
      "fulfilled",
    ]);
    expect(
      outcomes.flatMap((outcome) =>
        outcome.status === "fulfilled" ? [outcome.value.status] : [],
      ).sort(),
    ).toEqual(["executed", "peer-refreshed"]);
    expect(
      JSON.parse(storage.getItem(AUTH_REFRESH_LEASE_KEY) ?? "{}").expiresAt,
    ).toBeGreaterThan(3_500);
  });

  test("fallback recovers an expired lease left by a crashed tab", async () => {
    const storage = new SharedStorage();
    let now = 4_000;
    storage.setItem(
      AUTH_REFRESH_LEASE_KEY,
      JSON.stringify({ owner: "crashed-tab", expiresAt: 4_050 }),
    );
    const coordinator = createAuthRefreshCoordinator({
      storage,
      createOwnerId: () => "surviving-tab",
      now: () => now,
      sleep: async (milliseconds) => {
        now += milliseconds;
      },
      leaseDurationMs: 100,
      pollIntervalMs: 25,
      waitTimeoutMs: 500,
    });
    let executions = 0;

    const result = await coordinator.runExclusive(async () => {
      executions += 1;
      return "recovered";
    });

    expect(result).toEqual({
      status: "executed",
      value: "recovered",
      generation: 4_075,
    });
    expect(executions).toBe(1);
    expect(JSON.parse(storage.getItem(AUTH_REFRESH_LEASE_KEY) ?? "{}")).toEqual({
      owner: "surviving-tab:1",
      expiresAt: 4_150,
    });
  });

  test("fallback renews its lease while long-running refresh work is active", async () => {
    jest.useFakeTimers();
    jest.setSystemTime(new Date(7_000));
    try {
      const storage = new SharedStorage();
      const channels = new SharedChannels();
      const firstStarted = deferred<void>();
      const finishFirst = deferred<string>();
      let executions = 0;
      const common = {
        storage,
        createChannel: channels.create,
        leaseDurationMs: 100,
        pollIntervalMs: 20,
        waitTimeoutMs: 500,
      };
      const first = createAuthRefreshCoordinator({
        ...common,
        createOwnerId: () => "long-running-tab",
      });
      const second = createAuthRefreshCoordinator({
        ...common,
        createOwnerId: () => "waiting-tab",
      });

      const firstResult = first.runExclusive(async () => {
        executions += 1;
        firstStarted.resolve();
        return finishFirst.promise;
      });
      await jest.advanceTimersByTimeAsync(20);
      await firstStarted.promise;
      const secondResult = second.runExclusive(async () => {
        executions += 1;
        return "unexpected-second-refresh";
      });

      await jest.advanceTimersByTimeAsync(350);
      expect(executions).toBe(1);

      finishFirst.resolve("long-refresh-complete");
      await expect(firstResult).resolves.toMatchObject({
        status: "executed",
        value: "long-refresh-complete",
      });
      await expect(secondResult).resolves.toMatchObject({
        status: "peer-refreshed",
      });
      expect(executions).toBe(1);
    } finally {
      jest.useRealTimers();
    }
  });

  test.each([
    ["write", new WriteFailingStorage()],
    ["read", new ReadFailingStorage()],
  ])("fallback runs work immediately when storage %s is unavailable", async (_kind, storage) => {
    const coordinator = createAuthRefreshCoordinator({
      storage,
      createOwnerId: () => "storage-disabled-tab",
      now: () => 7_500,
      sleep: async () => {
        throw new Error("must not wait for unavailable storage");
      },
    });
    let executions = 0;

    expect(coordinator.getGeneration()).toBe(0);
    await expect(
      coordinator.runExclusive(async () => {
        executions += 1;
        return "uncoordinated-refresh";
      }),
    ).resolves.toMatchObject({
      status: "executed",
      value: "uncoordinated-refresh",
    });
    expect(executions).toBe(1);
  });

  test("fallback preserves successful work when storage becomes unavailable", async () => {
    const storage = new ToggleReadFailingStorage();
    const coordinator = createAuthRefreshCoordinator({
      storage,
      createOwnerId: () => "storage-loss-tab",
      now: () => 7_600,
      sleep: async () => undefined,
    });

    await expect(
      coordinator.runExclusive(async () => {
        storage.readsDisabled = true;
        return "refresh-already-succeeded";
      }),
    ).resolves.toMatchObject({
      status: "executed",
      value: "refresh-already-succeeded",
    });
  });

  test("fallback completion never deletes its fenced lease", async () => {
    const storage = new RemovalTrackingStorage();
    const coordinator = createAuthRefreshCoordinator({
      storage,
      createOwnerId: () => "non-deleting-tab",
      now: () => 7_750,
      sleep: async () => undefined,
    });

    await coordinator.runExclusive(async () => "done");

    expect(storage.removeCalls).toBe(0);
    expect(storage.getItem(AUTH_REFRESH_LEASE_KEY)).not.toBeNull();
  });

  test("dispose cancels active renewal and prevents success publication", async () => {
    const storage = new SharedStorage();
    const started = deferred<void>();
    const finish = deferred<string>();
    let renewalCancelled = false;
    const coordinator = createAuthRefreshCoordinator({
      storage,
      createOwnerId: () => "disposed-tab",
      now: () => 7_900,
      sleep: async () => undefined,
      schedule: () => () => {
        renewalCancelled = true;
      },
    });

    const refresh = coordinator.runExclusive(async () => {
      started.resolve();
      return finish.promise;
    });
    await started.promise;
    coordinator.dispose();
    const cancelledAfterDispose = renewalCancelled;
    finish.resolve("must-not-publish");
    const outcome = await refresh.then(
      () => "fulfilled",
      () => "rejected",
    );

    expect(cancelledAfterDispose).toBe(true);
    expect(outcome).toBe("rejected");
    expect(storage.getItem(AUTH_REFRESH_GENERATION_KEY)).toBeNull();
    expect(storage.getItem(AUTH_REFRESH_LEASE_KEY)).not.toBeNull();
  });

  test("dispose during lease stabilization prevents renewal, work, and publication", async () => {
    const storage = new SharedStorage();
    const stabilizationStarted = deferred<void>();
    const finishStabilization = deferred<void>();
    let workExecutions = 0;
    let renewalSchedules = 0;
    const coordinator = createAuthRefreshCoordinator({
      storage,
      createOwnerId: () => "stabilizing-tab",
      now: () => 7_925,
      sleep: async () => {
        stabilizationStarted.resolve();
        return finishStabilization.promise;
      },
      schedule: () => {
        renewalSchedules += 1;
        return () => undefined;
      },
    });

    const refresh = coordinator.runExclusive(async () => {
      workExecutions += 1;
      return "must-not-run";
    });
    await stabilizationStarted.promise;
    coordinator.dispose();
    finishStabilization.resolve();

    await expect(refresh).rejects.toThrow("Auth refresh coordinator disposed");
    expect(workExecutions).toBe(0);
    expect(renewalSchedules).toBe(0);
    expect(storage.getItem(AUTH_REFRESH_GENERATION_KEY)).toBeNull();
  });

  test("invalid duration configuration is clamped to finite positive defaults", async () => {
    const storage = new SharedStorage();
    const scheduledDelays: number[] = [];
    const coordinator = createAuthRefreshCoordinator({
      storage,
      createOwnerId: () => "invalid-duration-tab",
      now: () => 7_950,
      sleep: async () => undefined,
      schedule: (_callback, milliseconds) => {
        scheduledDelays.push(milliseconds);
        return () => undefined;
      },
      leaseDurationMs: Number.NaN,
      pollIntervalMs: -20,
      waitTimeoutMs: Number.POSITIVE_INFINITY,
    });

    await expect(coordinator.runExclusive(async () => "done")).resolves.toMatchObject({
      status: "executed",
    });
    await expect(
      coordinator.waitForGenerationAfter(coordinator.getGeneration(), -10),
    ).resolves.toBeNull();
    expect(scheduledDelays.length).toBeGreaterThan(0);
    expect(scheduledDelays.every((delay) => Number.isFinite(delay) && delay > 0)).toBe(
      true,
    );
  });

  test("fallback timeout uses an absolute clock deadline", async () => {
    const storage = new SharedStorage();
    let now = 0;
    let sleeps = 0;
    storage.setItem(
      AUTH_REFRESH_LEASE_KEY,
      JSON.stringify({ owner: "busy-tab", expiresAt: 10_000 }),
    );
    const coordinator = createAuthRefreshCoordinator({
      storage,
      createOwnerId: () => "waiting-tab",
      now: () => now,
      monotonicNow: () => now,
      sleep: async () => {
        sleeps += 1;
        now += 10;
      },
      pollIntervalMs: 40,
      waitTimeoutMs: 100,
    });

    await expect(coordinator.runExclusive(async () => "unexpected")).rejects.toThrow(
      "Timed out waiting for cross-tab auth refresh coordination",
    );
    expect(now).toBe(100);
    expect(sleeps).toBe(10);
  });

  test("waitForGenerationAfter uses elapsed clock time rather than requested sleep", async () => {
    const storage = new SharedStorage();
    let now = 100;
    let sleeps = 0;
    const coordinator = createAuthRefreshCoordinator({
      storage,
      createOwnerId: () => "waiting-tab",
      now: () => now,
      monotonicNow: () => now,
      sleep: async () => {
        sleeps += 1;
        now += 5;
      },
      pollIntervalMs: 25,
      waitTimeoutMs: 100,
    });

    await expect(coordinator.waitForGenerationAfter(0, 20)).resolves.toBeNull();
    expect(now).toBe(120);
    expect(sleeps).toBe(4);
  });

  test("local timeout stays bounded when the shared wall clock moves backward", async () => {
    const storage = new SharedStorage();
    let wallNow = 1_000;
    let monotonicNow = 0;
    let sleeps = 0;
    storage.setItem(
      AUTH_REFRESH_LEASE_KEY,
      JSON.stringify({ owner: "busy-tab", expiresAt: 10_000 }),
    );
    const coordinator = createAuthRefreshCoordinator({
      storage,
      createOwnerId: () => "clock-adjustment-tab",
      now: () => wallNow,
      monotonicNow: () => monotonicNow,
      sleep: async () => {
        sleeps += 1;
        monotonicNow += 10;
        wallNow = sleeps < 3 ? wallNow - 100 : 1_100;
      },
      pollIntervalMs: 40,
      waitTimeoutMs: 100,
    });

    await expect(coordinator.runExclusive(async () => "unexpected")).rejects.toThrow(
      "Timed out waiting for cross-tab auth refresh coordination",
    );
    expect(sleeps).toBe(10);
    expect(monotonicNow).toBe(100);
  });

  test("waitForGenerationAfter is bounded and observes a later peer generation", async () => {
    const storage = new SharedStorage();
    let now = 5_000;
    let sleeps = 0;
    const coordinator = createAuthRefreshCoordinator({
      storage,
      createOwnerId: () => "tab-a",
      now: () => now,
      monotonicNow: () => now,
      sleep: async (milliseconds) => {
        sleeps += 1;
        now += milliseconds;
        if (sleeps === 2) {
          storage.setItem(
            AUTH_REFRESH_GENERATION_KEY,
            JSON.stringify({ generation: 5_075 }),
          );
        }
      },
      pollIntervalMs: 25,
      waitTimeoutMs: 100,
    });

    await expect(coordinator.waitForGenerationAfter(5_000)).resolves.toBe(5_075);
    await expect(coordinator.waitForGenerationAfter(5_075, 50)).resolves.toBeNull();
    expect(now).toBeLessThanOrEqual(5_125);
  });

  test("stored generation rejects an older channel payload", () => {
    const storage = new SharedStorage();
    const channels = new SharedChannels();
    storage.setItem(
      AUTH_REFRESH_GENERATION_KEY,
      JSON.stringify({ generation: 5_499 }),
    );
    const coordinator = createAuthRefreshCoordinator({
      storage,
      createChannel: channels.create,
      createOwnerId: () => "tab-a",
      now: () => 5_500,
      sleep: async () => undefined,
    });

    channels.broadcast({ generation: 5_498 });

    expect(coordinator.getGeneration()).toBe(5_499);
  });

  test("generation metadata rejects corrupt, unsafe, and saturated values", () => {
    const storage = new SharedStorage();
    const coordinator = createAuthRefreshCoordinator({
      storage,
      createOwnerId: () => "generation-validation-tab",
      now: () => 5_750,
      sleep: async () => undefined,
    });

    storage.setItem(AUTH_REFRESH_GENERATION_KEY, "{corrupt-json");
    expect(coordinator.getGeneration()).toBe(0);
    for (const generation of [
      Number.MAX_SAFE_INTEGER + 1,
      1.5,
      -1,
      Number.POSITIVE_INFINITY,
    ]) {
      storage.setItem(
        AUTH_REFRESH_GENERATION_KEY,
        JSON.stringify({ generation }),
      );
      expect(coordinator.getGeneration()).toBe(0);
    }
    storage.setItem(
      AUTH_REFRESH_GENERATION_KEY,
      JSON.stringify({ generation: 42 }),
    );
    expect(coordinator.getGeneration()).toBe(42);
    storage.setItem(
      AUTH_REFRESH_GENERATION_KEY,
      JSON.stringify({ generation: Number.MAX_SAFE_INTEGER }),
    );
    expect(coordinator.getGeneration()).toBe(42);
  });

  test("saturated generation metadata is ignored and replaced by a usable generation", async () => {
    const storage = new SharedStorage();
    storage.setItem(
      AUTH_REFRESH_GENERATION_KEY,
      JSON.stringify({ generation: Number.MAX_SAFE_INTEGER }),
    );
    const coordinator = createAuthRefreshCoordinator({
      storage,
      locks: new SharedLocks(),
      createOwnerId: () => "saturation-recovery-tab",
      now: () => 5_800,
      sleep: async () => undefined,
    });

    const observedGeneration = coordinator.getGeneration();
    const result = await coordinator.runExclusive(async () => "recovered");

    expect(observedGeneration).toBe(0);
    expect(result).toEqual({
      status: "executed",
      value: "recovered",
      generation: 5_800,
    });
    await expect(
      coordinator.waitForGenerationAfter(observedGeneration, 0),
    ).resolves.toBe(5_800);
    expect(JSON.parse(storage.getItem(AUTH_REFRESH_GENERATION_KEY) ?? "{}")).toEqual({
      generation: 5_800,
    });
  });

  test("highest accepted generation rolls over and wakes a peer waiting on the old token", async () => {
    const storage = new SharedStorage();
    const channels = new SharedChannels();
    const ceiling = Number.MAX_SAFE_INTEGER - 1;
    let monotonicNow = 0;
    storage.setItem(
      AUTH_REFRESH_GENERATION_KEY,
      JSON.stringify({ generation: ceiling }),
    );
    const waiter = createAuthRefreshCoordinator({
      storage,
      createChannel: channels.create,
      createOwnerId: () => "ceiling-waiter",
      now: () => 5_850,
      monotonicNow: () => monotonicNow,
      sleep: async () => {
        monotonicNow += 10;
      },
      pollIntervalMs: 20,
      waitTimeoutMs: 100,
    });
    const runner = createAuthRefreshCoordinator({
      storage,
      locks: new SharedLocks(),
      createChannel: channels.create,
      createOwnerId: () => "ceiling-runner",
      now: () => 5_850,
      sleep: async () => undefined,
    });
    const capturedGeneration = waiter.getGeneration();

    const peerResult = waiter.waitForGenerationAfter(capturedGeneration);
    const refreshResult = await runner.runExclusive(async () => "rolled-over");

    expect(refreshResult).toEqual({
      status: "executed",
      value: "rolled-over",
      generation: 1,
    });
    await expect(peerResult).resolves.toBe(1);
    expect(JSON.parse(storage.getItem(AUTH_REFRESH_GENERATION_KEY) ?? "{}")).toEqual({
      generation: 1,
    });
  });

  test("storage and broadcast payloads contain coordination metadata but no callback data", async () => {
    const storage = new SharedStorage();
    const channels = new SharedChannels();
    const coordinator = createAuthRefreshCoordinator({
      storage,
      createChannel: channels.create,
      createOwnerId: () => "non-sensitive-owner-id",
      now: () => 6_000,
      sleep: async () => undefined,
    });
    const sensitiveResult = {
      accessToken: "secret-access-token",
      refreshToken: "secret-refresh-token",
      user: { email: "guest@example.com" },
    };

    await coordinator.runExclusive(async () => sensitiveResult);

    const serializedPayloads = [
      ...storage.writes.map(({ value }) => value),
      ...channels.messages.map((message) => JSON.stringify(message)),
    ];
    expect(serializedPayloads.length).toBeGreaterThan(0);
    for (const payload of serializedPayloads) {
      const keys = Object.keys(JSON.parse(payload));
      expect(payload).not.toContain("secret-access-token");
      expect(payload).not.toContain("secret-refresh-token");
      expect(payload).not.toContain("guest@example.com");
      expect(keys.length).toBeGreaterThan(0);
      expect(keys.every((key) => ["owner", "expiresAt", "generation"].includes(key))).toBe(
        true,
      );
    }
  });

  // Defect L3-37 / #3: waiting forever on navigator.locks when the peer tab
  // is wedged must not hang this tab. Abort the lock wait after lockWaitTimeoutMs
  // and surface the same failure path a failed refresh takes.
  test("Web Lock wait aborts after lockWaitTimeoutMs when the holder never releases", async () => {
    jest.useFakeTimers();
    try {
      let requestOptions: { signal?: AbortSignal } | undefined;
      const wedgedLocks: AuthRefreshLockManager = {
        // Contended: tryRequest never acquires.
        tryRequest: async () => ({ status: "unavailable" }),
        // Never grants the lock unless the caller's AbortSignal fires.
        request: <T,>(
          _name: string,
          _callback: () => Promise<T> | T,
          options?: { signal?: AbortSignal },
        ): Promise<T> =>
          new Promise<T>((_resolve, reject) => {
            requestOptions = options;
            const signal = options?.signal;
            if (!signal) return; // hang forever if no signal — what we are fixing
            if (signal.aborted) {
              reject(new DOMException("The operation was aborted.", "AbortError"));
              return;
            }
            signal.addEventListener(
              "abort",
              () => {
                reject(new DOMException("The operation was aborted.", "AbortError"));
              },
              { once: true },
            );
          }),
      };

      const coordinator = createAuthRefreshCoordinator({
        locks: wedgedLocks,
        // Storage/channel unavailable so the only coordination path is Web Locks
        // (no lease fallback after lock-wait abort).
        storage: new ReadFailingStorage(),
        createChannel: undefined,
        createOwnerId: () => "waiting-tab",
        lockWaitTimeoutMs: 45,
      });

      const pending = coordinator.runExclusive(async () => "should-not-run");
      // Attach the rejection assertion before advancing timers so Jest does not
      // treat the abort as an unhandled rejection mid-flight.
      const assertion = expect(pending).rejects.toThrow(
        /lock wait|timed out|abort/i,
      );
      await jest.advanceTimersByTimeAsync(45);
      await assertion;
      expect(requestOptions?.signal).toBeInstanceOf(AbortSignal);
      expect(requestOptions?.signal?.aborted).toBe(true);
    } finally {
      jest.useRealTimers();
    }
  });
});
