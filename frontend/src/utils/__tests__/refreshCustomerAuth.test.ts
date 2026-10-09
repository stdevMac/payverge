import {
  createAuthRefreshCoordinator,
  type AuthRefreshChannel,
  type AuthRefreshLockManager,
  type AuthRefreshStorage,
} from "../authRefreshCoordinator";
import { createRefreshAuthClient } from "../refreshAuth";
import { createRefreshCustomerAuthClient } from "../refreshCustomerAuth";

class SharedStorage implements AuthRefreshStorage {
  private readonly values = new Map<string, string>();
  getItem(key: string): string | null { return this.values.get(key) ?? null; }
  setItem(key: string, value: string): void { this.values.set(key, value); }
  removeItem(key: string): void { this.values.delete(key); }
}

class EndpointScopedLocks implements AuthRefreshLockManager {
  private readonly tails = new Map<string, Promise<void>>();

  request<T>(name: string, callback: () => Promise<T> | T): Promise<T> {
    const previous = this.tails.get(name) ?? Promise.resolve();
    let release = (): void => undefined;
    this.tails.set(name, new Promise<void>((resolve) => { release = resolve; }));
    return previous.then(callback).finally(release);
  }
}

class SharedChannels {
  private readonly channels = new Map<string, Set<AuthRefreshChannel>>();

  create = (name: string): AuthRefreshChannel => {
    const peers = this.channels.get(name) ?? new Set<AuthRefreshChannel>();
    this.channels.set(name, peers);
    const channel: AuthRefreshChannel = {
      onmessage: null,
      postMessage: (message) => {
        for (const peer of peers) {
          if (peer !== channel) peer.onmessage?.({ data: message });
        }
      },
      close: () => peers.delete(channel),
    };
    peers.add(channel);
    return channel;
  };
}

const makeRealms = () => {
  const storage = new SharedStorage();
  const locks = new EndpointScopedLocks();
  const channels = new SharedChannels();
  const common = { storage, locks, createChannel: channels.create, now: () => 1000 };
  return {
    ownerA: createAuthRefreshCoordinator({ ...common, scope: "auth", createOwnerId: () => "owner-a" }),
    customerA: createAuthRefreshCoordinator({ ...common, scope: "customer", createOwnerId: () => "customer-a" }),
    customerB: createAuthRefreshCoordinator({ ...common, scope: "customer", createOwnerId: () => "customer-b" }),
  };
};

describe("customer refresh client", () => {
  it("coalesces two customer realms into one POST and both succeed", async () => {
    const { customerA, customerB } = makeRealms();
    const fetchMock = jest.fn(async () =>
      new Response(JSON.stringify({ success: true }), { status: 200 }),
    );
    const first = createRefreshCustomerAuthClient({ coordinator: customerA, fetch: fetchMock as typeof fetch });
    const second = createRefreshCustomerAuthClient({ coordinator: customerB, fetch: fetchMock as typeof fetch });

    await expect(Promise.all([first("https://api.example.com"), second("https://api.example.com")]))
      .resolves.toEqual([{ ok: true }, { ok: true }]);

    expect(fetchMock).toHaveBeenCalledTimes(1);
    expect(fetchMock).toHaveBeenCalledWith(
      "https://api.example.com/customer/refresh",
      expect.objectContaining({ method: "POST", credentials: "include" }),
    );
  });

  it("does not coalesce simultaneous owner and customer refresh endpoints", async () => {
    const { ownerA, customerA } = makeRealms();
    const paths: string[] = [];
    const fetchMock = jest.fn(async (input: RequestInfo | URL) => {
      paths.push(String(input));
      return new Response(JSON.stringify({ success: true }), { status: 200 });
    });
    const owner = createRefreshAuthClient({ coordinator: ownerA, fetch: fetchMock as typeof fetch });
    const customer = createRefreshCustomerAuthClient({ coordinator: customerA, fetch: fetchMock as typeof fetch });

    await expect(Promise.all([owner("https://api.example.com"), customer("https://api.example.com")]))
      .resolves.toEqual([{ ok: true }, { ok: true }]);

    expect(paths).toEqual(expect.arrayContaining([
      "https://api.example.com/auth/refresh",
      "https://api.example.com/customer/refresh",
    ]));
    expect(fetchMock).toHaveBeenCalledTimes(2);
  });

  it("recovers a rotated customer refresh only through customer session-info", async () => {
    const { customerA } = makeRealms();
    const fetchMock = jest
      .fn()
      .mockResolvedValueOnce(new Response(JSON.stringify({ code: "AUTH_REFRESH_ROTATED" }), { status: 401 }))
      .mockResolvedValueOnce(new Response(JSON.stringify({ authenticated: true, type: "customer" }), { status: 200 }));
    const customer = createRefreshCustomerAuthClient({
      coordinator: customerA,
      fetch: fetchMock as typeof fetch,
      rotatedRecoveryWaitMs: 1,
    });

    await expect(customer("https://api.example.com")).resolves.toEqual({ ok: true });
    expect(fetchMock).toHaveBeenCalledTimes(2);
    expect(fetchMock).toHaveBeenNthCalledWith(
      2,
      "https://api.example.com/customer/session-info",
      expect.objectContaining({ method: "GET", credentials: "include", signal: expect.any(AbortSignal) }),
    );
    expect(fetchMock.mock.calls.filter(([, init]) => init?.method === "POST")).toHaveLength(1);
  });

  it.each([
    ["rotated", new Response(JSON.stringify({ code: "AUTH_REFRESH_ROTATED" }), { status: 401 })],
    ["rate limited", new Response(JSON.stringify({ code: "RATE_LIMITED" }), { status: 429 })],
    ["server failure", new Response(JSON.stringify({ error: "unavailable" }), { status: 503 })],
  ])("keeps %s failures non-dead", async (_label, response) => {
    const { customerA } = makeRealms();
    const fetchMock = jest.fn()
      .mockResolvedValueOnce(response)
      .mockResolvedValue(response.status === 401
        ? new Response(JSON.stringify({ authenticated: false }), { status: 401 })
        : response.clone());
    const customer = createRefreshCustomerAuthClient({
      coordinator: customerA,
      fetch: fetchMock as typeof fetch,
      rotatedRecoveryWaitMs: 1,
    });

    await expect(customer("https://api.example.com")).resolves.toEqual(
      expect.objectContaining({ ok: false, sessionDead: false }),
    );
  });

  it("marks a genuine 401 as dead", async () => {
    const { customerA } = makeRealms();
    const fetchMock = jest.fn(async () =>
      new Response(JSON.stringify({ code: "AUTH_TOKEN_INVALID" }), { status: 401 }),
    );
    const customer = createRefreshCustomerAuthClient({ coordinator: customerA, fetch: fetchMock as typeof fetch });

    await expect(customer("https://api.example.com")).resolves.toEqual({
      ok: false,
      status: 401,
      code: "AUTH_TOKEN_INVALID",
      alreadyRotated: false,
      sessionDead: true,
    });
  });

  it("keeps an origin-policy 403 non-dead", async () => {
    const { customerA } = makeRealms();
    const fetchMock = jest.fn(async () =>
      new Response(JSON.stringify({ code: "origin_not_allowed" }), { status: 403 }),
    );
    const customer = createRefreshCustomerAuthClient({ coordinator: customerA, fetch: fetchMock as typeof fetch });

    await expect(customer("https://api.example.com")).resolves.toEqual({
      ok: false,
      status: 403,
      code: "origin_not_allowed",
      alreadyRotated: false,
      sessionDead: false,
    });
  });

  it("keeps network errors non-dead", async () => {
    const { customerA } = makeRealms();
    const fetchMock = jest.fn(async () => { throw new TypeError("offline"); });
    const customer = createRefreshCustomerAuthClient({ coordinator: customerA, fetch: fetchMock as typeof fetch });

    await expect(customer("https://api.example.com")).resolves.toEqual({
      ok: false,
      status: 0,
      alreadyRotated: false,
      sessionDead: false,
    });
  });

  it("bounds a hanging refresh and allows the next call to execute", async () => {
    jest.useFakeTimers();
    try {
      const { customerA } = makeRealms();
      const fetchMock = jest
        .fn()
        .mockImplementationOnce(() => new Promise<Response>(() => undefined))
        .mockImplementationOnce(() => new Promise<Response>(() => undefined))
        .mockResolvedValueOnce(
          new Response(JSON.stringify({ success: true }), { status: 200 }),
        );
      const customer = createRefreshCustomerAuthClient({
        coordinator: customerA,
        fetch: fetchMock as typeof fetch,
        refreshRequestTimeoutMs: 10,
      });

      const timedOut = customer("https://api.example.com");
      await jest.advanceTimersByTimeAsync(20);
      await expect(timedOut).resolves.toEqual({
        ok: false,
        status: 0,
        alreadyRotated: false,
        sessionDead: false,
      });

      await expect(customer("https://api.example.com")).resolves.toEqual({
        ok: true,
      });
      expect(fetchMock).toHaveBeenCalledTimes(3);
      expect(fetchMock.mock.calls[0]?.[1]).toEqual(
        expect.objectContaining({ signal: expect.any(AbortSignal) }),
      );
    } finally {
      jest.useRealTimers();
    }
  });
});
