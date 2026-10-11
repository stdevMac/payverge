/**
 * @jest-environment jsdom
 */
import MockAdapter from "axios-mock-adapter";
import { axiosInstance } from "../instance";
import {
  AUTH_REFRESH_GENERATION_KEY,
  AUTH_REFRESH_LEASE_KEY,
} from "@/utils/authRefreshCoordinator";
import * as refreshAuth from "@/utils/refreshAuth";
import * as refreshCustomerAuth from "@/utils/refreshCustomerAuth";

function setLocation(pathname: string) {
  // jsdom's location.pathname is writable as long as we use the History API
  // or replaceState; this avoids triggering navigation.
  window.history.replaceState({}, "", pathname);
}

describe("axios refresh interceptor — guest-route guard", () => {
  let mock: MockAdapter;

  beforeEach(() => {
    refreshAuth.__resetRefreshAuthForTests();
    refreshCustomerAuth.__resetRefreshCustomerAuthForTests();
    localStorage.removeItem(AUTH_REFRESH_GENERATION_KEY);
    localStorage.removeItem(AUTH_REFRESH_LEASE_KEY);
    mock = new MockAdapter(axiosInstance);
    setLocation("/b/mara-core-kitchen");
  });

  afterEach(() => {
    mock.restore();
    refreshAuth.__resetRefreshAuthForTests();
    refreshCustomerAuth.__resetRefreshCustomerAuthForTests();
    localStorage.removeItem(AUTH_REFRESH_GENERATION_KEY);
    localStorage.removeItem(AUTH_REFRESH_LEASE_KEY);
    setLocation("/");
  });

  test("does not call /auth/refresh when 401 fires on a /b/ route", async () => {
    mock.onGet("/business/foo/menu").reply(401, { error: "no auth" });
    const refreshSpy = jest.fn();
    const originalFetch = global.fetch;
    global.fetch = jest.fn(async (url) => {
      refreshSpy(String(url));
      return new Response("", { status: 401 });
    }) as unknown as typeof fetch;

    await expect(
      axiosInstance.get("/business/foo/menu", { _maxRetries: 0 } as never),
    ).rejects.toThrow();
    expect(refreshSpy).not.toHaveBeenCalled();

    global.fetch = originalFetch;
  });

  test("does not call /auth/refresh when 401 fires on a /t/ route", async () => {
    setLocation("/t/TABLE123/menu");
    mock.onGet("/guest/table/TABLE123/bill").reply(401, { error: "no auth" });
    const refreshSpy = jest.fn();
    const originalFetch = global.fetch;
    global.fetch = jest.fn(async (url) => {
      refreshSpy(String(url));
      return new Response("", { status: 401 });
    }) as unknown as typeof fetch;

    await expect(
      axiosInstance.get("/guest/table/TABLE123/bill", { _maxRetries: 0 } as never),
    ).rejects.toThrow();
    expect(refreshSpy).not.toHaveBeenCalled();

    global.fetch = originalFetch;
  });

  test("does call /auth/refresh when 401 fires on /inside/ from a non-guest route", async () => {
    setLocation("/dashboard");
    mock.onGet("/inside/users/me").reply(401, { error: "expired" });

    const refreshSpy = jest.fn();
    const originalFetch = global.fetch;
    global.fetch = jest.fn(async (url) => {
      refreshSpy(String(url));
      return new Response("", { status: 401 });
    }) as unknown as typeof fetch;

    await expect(
      axiosInstance.get("/inside/users/me", { _maxRetries: 0 } as never),
    ).rejects.toThrow();
    expect(refreshSpy).toHaveBeenCalledWith(expect.stringContaining("/auth/refresh"));

    global.fetch = originalFetch;
  });

  test("recovers a customer 401 through /customer/refresh on a guest route", async () => {
    mock.onGet("/customer/profile").replyOnce(401, { error: "expired" });
    mock.onGet("/customer/profile").reply(200, { id: 42 });
    const customerRefresh = jest
      .spyOn(refreshCustomerAuth, "refreshCustomerAuthSession")
      .mockResolvedValue({ ok: true });
    const ownerRefresh = jest.spyOn(refreshAuth, "refreshAuthSession");

    try {
      await expect(
        axiosInstance.get("/customer/profile", { _maxRetries: 0 } as never),
      ).resolves.toMatchObject({ data: { id: 42 } });
      expect(customerRefresh).toHaveBeenCalledTimes(1);
      expect(ownerRefresh).not.toHaveBeenCalled();
      expect(
        mock.history.get.filter(({ url }) => url === "/customer/profile"),
      ).toHaveLength(2);
    } finally {
      customerRefresh.mockRestore();
      ownerRefresh.mockRestore();
    }
  });

  test("dispatches only customer expiry when customer refresh is dead", async () => {
    mock.onGet("/customer/profile").reply(401, { error: "expired" });
    const customerRefresh = jest
      .spyOn(refreshCustomerAuth, "refreshCustomerAuthSession")
      .mockResolvedValue({
        ok: false,
        status: 401,
        code: "AUTH_TOKEN_INVALID",
        alreadyRotated: false,
        sessionDead: true,
      });
    const onCustomerExpired = jest.fn();
    const onOwnerExpired = jest.fn();
    window.addEventListener("customer:session-expired", onCustomerExpired);
    window.addEventListener("auth:session-expired", onOwnerExpired);

    try {
      await expect(
        axiosInstance.get("/customer/profile", { _maxRetries: 0 } as never),
      ).rejects.toThrow();
      expect(onCustomerExpired).toHaveBeenCalledTimes(1);
      expect(onOwnerExpired).not.toHaveBeenCalled();
    } finally {
      window.removeEventListener("customer:session-expired", onCustomerExpired);
      window.removeEventListener("auth:session-expired", onOwnerExpired);
      customerRefresh.mockRestore();
    }
  });

  test("does not retry or dispatch expiry for a transient customer refresh", async () => {
    mock.onGet("/customer/profile").reply(401, { error: "expired" });
    const customerRefresh = jest
      .spyOn(refreshCustomerAuth, "refreshCustomerAuthSession")
      .mockResolvedValue({
        ok: false,
        status: 503,
        alreadyRotated: false,
        sessionDead: false,
      });
    const onCustomerExpired = jest.fn();
    const onOwnerExpired = jest.fn();
    window.addEventListener("customer:session-expired", onCustomerExpired);
    window.addEventListener("auth:session-expired", onOwnerExpired);

    try {
      await expect(
        axiosInstance.get("/customer/profile", { _maxRetries: 0 } as never),
      ).rejects.toThrow();
      expect(
        mock.history.get.filter(({ url }) => url === "/customer/profile"),
      ).toHaveLength(1);
      expect(onCustomerExpired).not.toHaveBeenCalled();
      expect(onOwnerExpired).not.toHaveBeenCalled();
    } finally {
      window.removeEventListener("customer:session-expired", onCustomerExpired);
      window.removeEventListener("auth:session-expired", onOwnerExpired);
      customerRefresh.mockRestore();
    }
  });

  test.each([
    "/customer/session-info",
    "/customer/refresh",
    "/customer/logout",
  ])("does not call /auth/refresh when 401 fires on %s", async (path) => {
    setLocation("/dashboard");
    mock.onAny(path).reply(401, { error: "customer auth failed" });
    const refreshSpy = jest.fn();
    const originalFetch = global.fetch;
    global.fetch = jest.fn(async (url) => {
      refreshSpy(String(url));
      return { ok: false, status: 401 } as Response;
    }) as unknown as typeof fetch;

    await expect(
      axiosInstance.get(path, { _maxRetries: 0 } as never),
    ).rejects.toThrow();
    expect(refreshSpy).not.toHaveBeenCalled();

    global.fetch = originalFetch;
  });

  test("shares one /auth/refresh request across concurrent 401 responses", async () => {
    setLocation("/dashboard");
    mock.onGet("/inside/users/me").replyOnce(401, { error: "expired" });
    mock.onGet("/inside/users/me").reply(200, { id: 1 });
    mock.onGet("/inside/businesses").replyOnce(401, { error: "expired" });
    mock.onGet("/inside/businesses").reply(200, { businesses: [] });

    let resolveRefresh!: () => void;
    let markRefreshStarted!: () => void;
    const refreshStarted = new Promise<void>((resolve) => {
      markRefreshStarted = resolve;
    });
    const originalFetch = global.fetch;
    global.fetch = jest.fn(
      () => {
        markRefreshStarted();
        return new Promise<Response>((resolve) => {
          resolveRefresh = () => resolve({ ok: true, status: 204 } as Response);
        });
      },
    ) as unknown as typeof fetch;

    const first = axiosInstance.get("/inside/users/me", { _maxRetries: 0 } as never);
    const second = axiosInstance.get("/inside/businesses", { _maxRetries: 0 } as never);

    // The browser-lock fallback deliberately stabilizes its lease before POST.
    // Wait for that observable event instead of assuming one event-loop tick.
    await refreshStarted;
    expect(global.fetch).toHaveBeenCalledTimes(1);

    resolveRefresh();
    await expect(Promise.all([first, second])).resolves.toHaveLength(2);
    expect(global.fetch).toHaveBeenCalledTimes(1);

    global.fetch = originalFetch;
  });

  test("does not retry or expire when rotated refresh recovery is inconclusive", async () => {
    setLocation("/dashboard");
    mock.onGet("/inside/users/me").replyOnce(401, { error: "expired" });
    mock.onGet("/inside/users/me").reply(200, { id: 1 });
    const refreshResult = jest
      .spyOn(refreshAuth, "refreshAuthSession")
      .mockResolvedValue({
        ok: false,
        status: 401,
        code: "AUTH_REFRESH_ROTATED",
        alreadyRotated: true,
        sessionDead: false,
      });
    const onSessionExpired = jest.fn();
    window.addEventListener("auth:session-expired", onSessionExpired);

    try {
      await expect(
        axiosInstance.get("/inside/users/me", { _maxRetries: 0 } as never),
      ).rejects.toThrow();

      expect(refreshResult).toHaveBeenCalledTimes(1);
      expect(
        mock.history.get.filter(({ url }) => url === "/inside/users/me"),
      ).toHaveLength(1);
      expect(onSessionExpired).not.toHaveBeenCalled();
    } finally {
      window.removeEventListener("auth:session-expired", onSessionExpired);
      refreshResult.mockRestore();
    }
  });

  test("does not refresh or expire on an RBAC 401 from Reservations/Tables", async () => {
    setLocation("/business/demo/dashboard?tab=reservations");
    mock.onGet("/inside/businesses/1/reservations").reply(401, {
      error: "Insufficient permissions for this action",
      code: "AUTH_INSUFFICIENT_ROLE",
    });
    const refreshSpy = jest
      .spyOn(refreshAuth, "refreshAuthSession")
      .mockResolvedValue({
        ok: false,
        status: 401,
        alreadyRotated: false,
        sessionDead: true,
      });
    const onSessionExpired = jest.fn();
    window.addEventListener("auth:session-expired", onSessionExpired);

    try {
      await expect(
        axiosInstance.get("/inside/businesses/1/reservations", {
          _maxRetries: 0,
        } as never),
      ).rejects.toThrow();
      expect(refreshSpy).not.toHaveBeenCalled();
      expect(onSessionExpired).not.toHaveBeenCalled();
    } finally {
      window.removeEventListener("auth:session-expired", onSessionExpired);
      refreshSpy.mockRestore();
    }
  });

  test("_skipAuthRefresh short-circuits refresh on a 401 from a dashboard route", async () => {
    // Use a route that is NOT in AUTH_REFRESH_SKIP_PATHS and is NOT an auth
    // endpoint, so the ONLY thing preventing the refresh attempt is the flag.
    setLocation("/dashboard");
    mock.onGet("/inside/profile").reply(401, { error: "expired" });
    const refreshSpy = jest.fn();
    const originalFetch = global.fetch;
    global.fetch = jest.fn(async (url) => {
      refreshSpy(String(url));
      return { ok: false, status: 401 } as Response;
    }) as unknown as typeof fetch;

    await expect(
      axiosInstance.get("/inside/profile", {
        _skipAuthRefresh: true,
        _maxRetries: 0,
      } as never),
    ).rejects.toThrow();
    expect(refreshSpy).not.toHaveBeenCalled();

    global.fetch = originalFetch;
  });
});
