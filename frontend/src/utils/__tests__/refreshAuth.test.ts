/**
 * @jest-environment jsdom
 */
import {
  createRefreshAuthClient,
  refreshAuthSession,
  __resetRefreshAuthForTests,
} from "../refreshAuth";
import {
  AUTH_REFRESH_GENERATION_KEY,
  createAuthRefreshCoordinator,
  type AuthRefreshCoordinationResult,
  type AuthRefreshCoordinator,
  type AuthRefreshLockManager,
} from "../authRefreshCoordinator";

class SharedLocks implements AuthRefreshLockManager {
  private tail: Promise<void> = Promise.resolve();

  request<T>(_name: string, callback: () => Promise<T> | T): Promise<T> {
    const previous = this.tail;
    let release = (): void => undefined;
    this.tail = new Promise<void>((resolve) => {
      release = resolve;
    });
    return previous.then(callback).finally(release);
  }
}

const createTestCoordinator = (
  overrides: Partial<AuthRefreshCoordinator> = {},
): AuthRefreshCoordinator => ({
  getGeneration: jest.fn(() => 0),
  runExclusive: jest.fn(async (work) => ({
    status: "executed" as const,
    value: await work(),
    generation: 1,
  })),
  waitForGenerationAfter: jest.fn(async () => null),
  dispose: jest.fn(),
  ...overrides,
});

describe("refreshAuthSession", () => {
  const originalFetch = global.fetch;

  beforeEach(() => {
    __resetRefreshAuthForTests();
    localStorage.clear();
    localStorage.setItem("payverge_had_session", "1");
  });

  afterEach(() => {
    global.fetch = originalFetch;
    __resetRefreshAuthForTests();
    localStorage.clear();
  });

  it("returns ok on a successful refresh", async () => {
    global.fetch = jest.fn().mockResolvedValue(
      new Response(JSON.stringify({ success: true }), { status: 200 }),
    ) as unknown as typeof fetch;

    await expect(refreshAuthSession("https://api.example.com")).resolves.toEqual({
      ok: true,
    });
    expect(localStorage.getItem("payverge_had_session")).toBe("1");
  });

  it("does not mark sessionDead when refresh 401s but session-info is still authenticated", async () => {
    const fetchMock = jest
      .fn()
      .mockResolvedValueOnce(
        new Response(
          JSON.stringify({
            error: "Refresh token missing",
            code: "AUTH_TOKEN_MISSING",
          }),
          { status: 401 },
        ),
      )
      .mockResolvedValueOnce(
        new Response(JSON.stringify({ authenticated: true }), { status: 200 }),
      );
    const client = createRefreshAuthClient({
      coordinator: createTestCoordinator(),
      fetch: fetchMock as unknown as typeof fetch,
    });

    await expect(client("https://api.example.com")).resolves.toEqual({
      ok: false,
      status: 401,
      code: "AUTH_TOKEN_MISSING",
      alreadyRotated: false,
      sessionDead: false,
    });
    expect(fetchMock).toHaveBeenNthCalledWith(
      2,
      "https://api.example.com/auth/session-info",
      expect.objectContaining({
        method: "GET",
        credentials: "include",
      }),
    );
  });

  it("marks AUTH_REFRESH_ROTATED as alreadyRotated (session still live)", async () => {
    const fetchMock = jest
      .fn()
      .mockResolvedValueOnce(
        new Response(
          JSON.stringify({
            error: "Refresh token already rotated",
            code: "AUTH_REFRESH_ROTATED",
          }),
          { status: 401 },
        ),
      )
      .mockResolvedValueOnce(
        new Response(JSON.stringify({ authenticated: false }), { status: 401 }),
      );
    const client = createRefreshAuthClient({
      coordinator: createTestCoordinator(),
      fetch: fetchMock as unknown as typeof fetch,
    });

    const result = await client("https://api.example.com");
    expect(result).toEqual({
      ok: false,
      status: 401,
      code: "AUTH_REFRESH_ROTATED",
      alreadyRotated: true,
      sessionDead: false,
    });
    expect(localStorage.getItem("payverge_had_session")).toBe("1");
  });

  it("does not infer a rotation race from the error text alone", async () => {
    const fetchMock = jest.fn().mockResolvedValue(
      new Response(
        JSON.stringify({
          error: "Refresh token already rotated",
          code: "AUTH_TOKEN_INVALID",
        }),
        { status: 401 },
      ),
    );
    const client = createRefreshAuthClient({
      coordinator: createTestCoordinator(),
      fetch: fetchMock as unknown as typeof fetch,
    });

    const result = await client("https://api.example.com");
    expect(result).toEqual(
      expect.objectContaining({ alreadyRotated: false, sessionDead: true }),
    );
  });

  it("coordinates independent refresh clients so only one realm posts", async () => {
    const locks = new SharedLocks();
    const common = {
      storage: localStorage,
      locks,
      createChannel: () => ({
        onmessage: null,
        postMessage: jest.fn(),
        close: jest.fn(),
      }),
      subscribeToStorage: () => () => undefined,
      now: () => 10_000,
    };
    const firstCoordinator = createAuthRefreshCoordinator({
      ...common,
      createOwnerId: () => "realm-a",
    });
    const secondCoordinator = createAuthRefreshCoordinator({
      ...common,
      createOwnerId: () => "realm-b",
    });
    const fetchMock = jest.fn().mockResolvedValue(
      new Response(JSON.stringify({ success: true }), { status: 200 }),
    );
    const firstClient = createRefreshAuthClient({
      coordinator: firstCoordinator,
      fetch: fetchMock as unknown as typeof fetch,
    });
    const secondClient = createRefreshAuthClient({
      coordinator: secondCoordinator,
      fetch: fetchMock as unknown as typeof fetch,
    });

    await expect(
      Promise.all([
        firstClient("https://api.example.com"),
        secondClient("https://api.example.com"),
      ]),
    ).resolves.toEqual([{ ok: true }, { ok: true }]);
    expect(fetchMock).toHaveBeenCalledTimes(1);
    expect(fetchMock).toHaveBeenCalledWith(
      "https://api.example.com/auth/refresh",
      expect.objectContaining({ method: "POST", credentials: "include" }),
    );

    firstCoordinator.dispose();
    secondCoordinator.dispose();
  });

  it("waits for a peer generation and probes the cookie session after rotation", async () => {
    const waitForGenerationAfter = jest.fn(async () => 8);
    const coordinator = createTestCoordinator({
      getGeneration: jest.fn(() => 7),
      waitForGenerationAfter,
    });
    const fetchMock = jest
      .fn()
      .mockResolvedValueOnce(
        new Response(
          JSON.stringify({ code: "AUTH_REFRESH_ROTATED" }),
          { status: 401 },
        ),
      )
      .mockResolvedValueOnce(
        new Response(JSON.stringify({ authenticated: true }), { status: 200 }),
      );
    const client = createRefreshAuthClient({
      coordinator,
      fetch: fetchMock as unknown as typeof fetch,
      rotatedRecoveryWaitMs: 25,
    });

    await expect(client("https://api.example.com")).resolves.toEqual({ ok: true });
    expect(waitForGenerationAfter).toHaveBeenCalledWith(7, 25);
    expect(fetchMock).toHaveBeenNthCalledWith(
      2,
      "https://api.example.com/auth/session-info",
      expect.objectContaining({
        method: "GET",
        credentials: "include",
        signal: expect.any(AbortSignal),
      }),
    );
  });

  it("passes one captured generation through coordination and rotated recovery", async () => {
    let generation = 0;
    const runExclusiveSpy = jest.fn();
    const runExclusive: AuthRefreshCoordinator["runExclusive"] = async <T>(
      work: () => Promise<T>,
      capturedGeneration?: number,
    ): Promise<AuthRefreshCoordinationResult<T>> => {
      runExclusiveSpy(work, capturedGeneration);
      generation = 9;
      if (capturedGeneration === 0) {
        return { status: "peer-refreshed" as const, generation };
      }
      return {
        status: "executed" as const,
        value: await work(),
        generation,
      };
    };
    const coordinator = createTestCoordinator({
      getGeneration: jest.fn(() => generation),
      runExclusive,
    });
    const fetchMock = jest.fn();
    const client = createRefreshAuthClient({
      coordinator,
      fetch: fetchMock as unknown as typeof fetch,
    });

    await expect(client("https://api.example.com")).resolves.toEqual({ ok: true });
    expect(runExclusiveSpy).toHaveBeenCalledWith(expect.any(Function), 0);
    expect(fetchMock).not.toHaveBeenCalled();
    expect(coordinator.waitForGenerationAfter).not.toHaveBeenCalled();
  });

  it.each([
    [
      "an unauthenticated probe",
      () =>
        Promise.resolve(
          new Response(JSON.stringify({ authenticated: false }), { status: 401 }),
        ),
    ],
    ["a failed probe", () => Promise.reject(new TypeError("network down"))],
  ])("keeps rotation non-dead during grace after %s", async (_label, probe) => {
    const coordinator = createTestCoordinator({
      waitForGenerationAfter: jest.fn(async () => null),
    });
    const fetchMock = jest
      .fn()
      .mockResolvedValueOnce(
        new Response(
          JSON.stringify({ code: "AUTH_REFRESH_ROTATED" }),
          { status: 401 },
        ),
      )
      .mockImplementationOnce(probe);
    const client = createRefreshAuthClient({
      coordinator,
      fetch: fetchMock as unknown as typeof fetch,
    });

    await expect(client("https://api.example.com")).resolves.toEqual({
      ok: false,
      status: 401,
      code: "AUTH_REFRESH_ROTATED",
      alreadyRotated: true,
      sessionDead: false,
    });
    expect(fetchMock).toHaveBeenCalledTimes(2);
    expect(fetchMock.mock.calls.filter(([, init]) => init?.method === "POST")).toHaveLength(
      1,
    );
  });

  it("times out a stalled session probe and clears its abort timer", async () => {
    jest.useFakeTimers();
    try {
      const coordinator = createTestCoordinator({
        waitForGenerationAfter: jest.fn(async () => null),
      });
      let probeSignal: AbortSignal | undefined;
      const fetchMock = jest
        .fn()
        .mockResolvedValueOnce(
          new Response(
            JSON.stringify({ code: "AUTH_REFRESH_ROTATED" }),
            { status: 401 },
          ),
        )
        .mockImplementationOnce(
          async (_url: RequestInfo | URL, init?: RequestInit) => {
            probeSignal = init?.signal ?? undefined;
            if (!probeSignal) throw new Error("missing session-probe abort signal");
            return {
              ok: true,
              status: 200,
              json: () =>
                new Promise((_resolve, reject) => {
                  probeSignal?.addEventListener("abort", () => {
                    reject(new DOMException("Aborted", "AbortError"));
                  });
                }),
            } as Response;
          },
        );
      const client = createRefreshAuthClient({
        coordinator,
        fetch: fetchMock as unknown as typeof fetch,
        sessionProbeTimeoutMs: 25,
      });

      const resultPromise = client("https://api.example.com");
      await jest.advanceTimersByTimeAsync(25);

      await expect(resultPromise).resolves.toEqual({
        ok: false,
        status: 401,
        code: "AUTH_REFRESH_ROTATED",
        alreadyRotated: true,
        sessionDead: false,
      });
      expect(probeSignal?.aborted).toBe(true);
      expect(jest.getTimerCount()).toBe(0);
    } finally {
      jest.useRealTimers();
    }
  });

  it("rejects authenticated JSON from a non-successful session probe", async () => {
    const coordinator = createTestCoordinator({
      waitForGenerationAfter: jest.fn(async () => null),
    });
    const fetchMock = jest
      .fn()
      .mockResolvedValueOnce(
        new Response(
          JSON.stringify({ code: "AUTH_REFRESH_ROTATED" }),
          { status: 401 },
        ),
      )
      .mockResolvedValueOnce(
        new Response(JSON.stringify({ authenticated: true }), { status: 503 }),
      );
    const client = createRefreshAuthClient({
      coordinator,
      fetch: fetchMock as unknown as typeof fetch,
    });

    await expect(client("https://api.example.com")).resolves.toEqual({
      ok: false,
      status: 401,
      code: "AUTH_REFRESH_ROTATED",
      alreadyRotated: true,
      sessionDead: false,
    });
  });

  it("recovers both clients when best-effort coordination permits duplicate posts", async () => {
    let generation = 0;
    const generationWaiters = new Set<() => void>();
    const makeBestEffortCoordinator = (): AuthRefreshCoordinator => ({
      getGeneration: () => generation,
      runExclusive: async (work) => {
        const value = await work();
        generation += 1;
        for (const wake of generationWaiters) wake();
        generationWaiters.clear();
        return { status: "executed", value, generation };
      },
      waitForGenerationAfter: async (captured) => {
        if (generation !== captured) return generation;
        await new Promise<void>((resolve) => generationWaiters.add(resolve));
        return generation;
      },
      dispose: jest.fn(),
    });
    const fetchMock = jest
      .fn()
      .mockResolvedValueOnce(
        new Response(JSON.stringify({ success: true }), { status: 200 }),
      )
      .mockResolvedValueOnce(
        new Response(
          JSON.stringify({ code: "AUTH_REFRESH_ROTATED" }),
          { status: 401 },
        ),
      )
      .mockResolvedValueOnce(
        new Response(JSON.stringify({ authenticated: true }), { status: 200 }),
      );
    const firstClient = createRefreshAuthClient({
      coordinator: makeBestEffortCoordinator(),
      fetch: fetchMock as unknown as typeof fetch,
    });
    const secondClient = createRefreshAuthClient({
      coordinator: makeBestEffortCoordinator(),
      fetch: fetchMock as unknown as typeof fetch,
    });

    const results = await Promise.all([
      firstClient("https://api.example.com"),
      secondClient("https://api.example.com"),
    ]);

    expect(results).toEqual([{ ok: true }, { ok: true }]);
    expect(fetchMock.mock.calls.filter(([, init]) => init?.method === "POST")).toHaveLength(
      2,
    );
    expect(fetchMock).toHaveBeenNthCalledWith(
      3,
      "https://api.example.com/auth/session-info",
      expect.objectContaining({
        method: "GET",
        credentials: "include",
        signal: expect.any(AbortSignal),
      }),
    );
  });

  it("marks a genuine 401 (invalid/expired) as sessionDead", async () => {
    const coordinator = createAuthRefreshCoordinator({
      storage: localStorage,
      locks: new SharedLocks(),
      createOwnerId: () => "invalid-token-realm",
      createChannel: () => ({
        onmessage: null,
        postMessage: jest.fn(),
        close: jest.fn(),
      }),
      subscribeToStorage: () => () => undefined,
    });
    const fetchMock = jest.fn().mockResolvedValue(
      new Response(
        JSON.stringify({
          error: "Invalid or expired refresh token",
          code: "AUTH_TOKEN_INVALID",
        }),
        { status: 401 },
      ),
    );
    const client = createRefreshAuthClient({
      coordinator,
      fetch: fetchMock as unknown as typeof fetch,
    });

    await expect(client("https://api.example.com")).resolves.toEqual({
      ok: false,
      status: 401,
      code: "AUTH_TOKEN_INVALID",
      alreadyRotated: false,
      sessionDead: true,
    });
    expect(coordinator.getGeneration()).toBe(0);
    expect(localStorage.getItem(AUTH_REFRESH_GENERATION_KEY)).toBeNull();
    coordinator.dispose();
  });

  it("coalesces concurrent callers into a single POST", async () => {
    let resolveFetch!: (value: Response) => void;
    const fetchPromise = new Promise<Response>((resolve) => {
      resolveFetch = resolve;
    });
    const fetchMock = jest.fn().mockReturnValue(fetchPromise);
    const client = createRefreshAuthClient({
      coordinator: createTestCoordinator(),
      fetch: fetchMock as unknown as typeof fetch,
    });

    const a = client("https://api.example.com");
    const b = client("https://api.example.com");
    expect(fetchMock).toHaveBeenCalledTimes(1);

    resolveFetch(new Response(JSON.stringify({ success: true }), { status: 200 }));
    const [ra, rb] = await Promise.all([a, b]);
    expect(ra).toEqual({ ok: true });
    expect(rb).toEqual({ ok: true });
    expect(fetchMock).toHaveBeenCalledTimes(1);
  });

  it.each([503, 429])("retries once after transient HTTP %i", async (status) => {
    const fetchMock = jest
      .fn()
      .mockResolvedValueOnce(new Response("transient", { status }))
      .mockResolvedValueOnce(
        new Response(JSON.stringify({ success: true }), { status: 200 }),
      );
    global.fetch = fetchMock as unknown as typeof fetch;

    await expect(refreshAuthSession("https://api.example.com")).resolves.toEqual({
      ok: true,
    });
    expect(fetchMock).toHaveBeenCalledTimes(2);
  });

  it("treats network errors as non-dead so the timer can retry", async () => {
    const fetchMock = jest.fn().mockRejectedValue(new TypeError("Failed to fetch"));
    global.fetch = fetchMock as unknown as typeof fetch;

    const result = await refreshAuthSession("https://api.example.com");
    expect(result).toEqual({
      ok: false,
      status: 0,
      alreadyRotated: false,
      sessionDead: false,
    });
    expect(fetchMock).toHaveBeenCalledTimes(2);
  });

  // Defect L3-37 / #1: operator refresh must bound the fetch like the customer twin.
  it("bounds a hanging refresh with AbortSignal and treats timeout as non-dead failure", async () => {
    jest.useFakeTimers();
    try {
      const fetchMock = jest
        .fn()
        .mockImplementationOnce(() => new Promise<Response>(() => undefined))
        .mockImplementationOnce(() => new Promise<Response>(() => undefined));
      const client = createRefreshAuthClient({
        coordinator: createTestCoordinator(),
        fetch: fetchMock as unknown as typeof fetch,
        refreshRequestTimeoutMs: 10,
      });

      const timedOut = client("https://api.example.com");
      await jest.advanceTimersByTimeAsync(30);
      await expect(timedOut).resolves.toEqual({
        ok: false,
        status: 0,
        alreadyRotated: false,
        sessionDead: false,
      });
      expect(fetchMock.mock.calls[0]?.[1]).toEqual(
        expect.objectContaining({ signal: expect.any(AbortSignal) }),
      );
    } finally {
      jest.useRealTimers();
    }
  });

  // Defect L3-37 / #2: settled-rejected refresh must clear the single-flight
  // cache so a later caller can start a fresh attempt (not await a dead promise).
  it("clears the coalesced promise after a timed-out refresh so the next call retries", async () => {
    jest.useFakeTimers();
    try {
      const fetchMock = jest
        .fn()
        .mockImplementationOnce(() => new Promise<Response>(() => undefined))
        .mockImplementationOnce(() => new Promise<Response>(() => undefined))
        .mockResolvedValueOnce(
          new Response(JSON.stringify({ success: true }), { status: 200 }),
        );
      const client = createRefreshAuthClient({
        coordinator: createTestCoordinator(),
        fetch: fetchMock as unknown as typeof fetch,
        refreshRequestTimeoutMs: 10,
      });

      const timedOut = client("https://api.example.com");
      await jest.advanceTimersByTimeAsync(30);
      await expect(timedOut).resolves.toEqual({
        ok: false,
        status: 0,
        alreadyRotated: false,
        sessionDead: false,
      });

      await expect(client("https://api.example.com")).resolves.toEqual({ ok: true });
      // Two timed-out attempts (MAX_REFRESH_ATTEMPTS) + one success.
      expect(fetchMock).toHaveBeenCalledTimes(3);
    } finally {
      jest.useRealTimers();
    }
  });

  it("clears the coalesced promise after a rejected (session-dead) refresh so the next call retries", async () => {
    const fetchMock = jest
      .fn()
      .mockResolvedValueOnce(
        new Response(
          JSON.stringify({ code: "AUTH_TOKEN_INVALID" }),
          { status: 401 },
        ),
      )
      .mockResolvedValueOnce(
        new Response(JSON.stringify({ authenticated: false }), { status: 401 }),
      )
      .mockResolvedValueOnce(
        new Response(JSON.stringify({ success: true }), { status: 200 }),
      );
    const client = createRefreshAuthClient({
      coordinator: createTestCoordinator(),
      fetch: fetchMock as unknown as typeof fetch,
    });

    await expect(client("https://api.example.com")).resolves.toEqual({
      ok: false,
      status: 401,
      code: "AUTH_TOKEN_INVALID",
      alreadyRotated: false,
      sessionDead: true,
    });

    await expect(client("https://api.example.com")).resolves.toEqual({ ok: true });
    expect(fetchMock).toHaveBeenCalledTimes(3);
  });

  // Related to issue 821: fast tab hops + Ctrl-K produced a burst of requests;
  // one transient fetch failure on the session-info probe was read as "not
  // authenticated" and escalated a still-live session into the sign-in wall.
  // Only an explicit server "dead" may confirm sessionDead — a network error
  // or 5xx on the probe is unknown, and unknown must fail open.
  describe("transient probe failures stay non-fatal (issue 821)", () => {
    it("does not mark sessionDead when the confirm probe network-fails", async () => {
      const fetchMock = jest
        .fn()
        .mockResolvedValueOnce(
          new Response(
            JSON.stringify({
              error: "Invalid or expired refresh token",
              code: "AUTH_TOKEN_INVALID",
            }),
            { status: 401 },
          ),
        )
        .mockRejectedValueOnce(new TypeError("Failed to fetch"));
      const client = createRefreshAuthClient({
        coordinator: createTestCoordinator(),
        fetch: fetchMock as unknown as typeof fetch,
      });

      await expect(client("https://api.example.com")).resolves.toEqual({
        ok: false,
        status: 401,
        code: "AUTH_TOKEN_INVALID",
        alreadyRotated: false,
        sessionDead: false,
      });
    });

    it("does not mark sessionDead when the confirm probe 5xxes", async () => {
      const fetchMock = jest
        .fn()
        .mockResolvedValueOnce(
          new Response(
            JSON.stringify({
              error: "Invalid or expired refresh token",
              code: "AUTH_TOKEN_INVALID",
            }),
            { status: 401 },
          ),
        )
        .mockResolvedValueOnce(
          new Response("Bad Gateway", { status: 502 }),
        );
      const client = createRefreshAuthClient({
        coordinator: createTestCoordinator(),
        fetch: fetchMock as unknown as typeof fetch,
      });

      await expect(client("https://api.example.com")).resolves.toEqual({
        ok: false,
        status: 401,
        code: "AUTH_TOKEN_INVALID",
        alreadyRotated: false,
        sessionDead: false,
      });
    });

    it("still marks sessionDead when the probe answers an explicit not-authenticated", async () => {
      const fetchMock = jest
        .fn()
        .mockResolvedValueOnce(
          new Response(
            JSON.stringify({
              error: "Invalid or expired refresh token",
              code: "AUTH_TOKEN_INVALID",
            }),
            { status: 401 },
          ),
        )
        .mockResolvedValueOnce(
          new Response(JSON.stringify({ authenticated: false }), {
            status: 200,
          }),
        );
      const client = createRefreshAuthClient({
        coordinator: createTestCoordinator(),
        fetch: fetchMock as unknown as typeof fetch,
      });

      await expect(client("https://api.example.com")).resolves.toEqual({
        ok: false,
        status: 401,
        code: "AUTH_TOKEN_INVALID",
        alreadyRotated: false,
        sessionDead: true,
      });
    });
  });
});
