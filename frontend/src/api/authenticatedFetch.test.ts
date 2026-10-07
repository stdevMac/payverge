/** @jest-environment jsdom */

import { authenticatedFetch } from "./authenticatedFetch";
import { refreshAuthSession } from "@/utils/refreshAuth";

jest.mock("@/utils/refreshAuth", () => ({
  refreshAuthSession: jest.fn(),
}));

const mockedRefresh = refreshAuthSession as jest.MockedFunction<
  typeof refreshAuthSession
>;

const originalFetch = global.fetch;

const response = (status: number): Response =>
  ({
    ok: status >= 200 && status < 300,
    status,
  }) as Response;

beforeEach(() => {
  global.fetch = jest.fn();
  mockedRefresh.mockReset();
});

afterAll(() => {
  global.fetch = originalFetch;
});

describe("authenticatedFetch", () => {
  it("refreshes an expired session and retries the original request once", async () => {
    (global.fetch as jest.Mock)
      .mockResolvedValueOnce(response(401))
      .mockResolvedValueOnce(response(200));
    mockedRefresh.mockResolvedValue({ ok: true });

    const init: RequestInit = {
      method: "POST",
      credentials: "include",
      headers: { "Content-Type": "application/json" },
      body: JSON.stringify({ station: 4 }),
    };
    const result = await authenticatedFetch(
      "https://api.example.test/api/v1/inside/jobs/claim",
      init,
    );

    expect(result.status).toBe(200);
    expect(mockedRefresh).toHaveBeenCalledWith(
      "https://api.example.test/api/v1",
    );
    expect(global.fetch).toHaveBeenCalledTimes(2);
    expect(global.fetch).toHaveBeenNthCalledWith(
      2,
      "https://api.example.test/api/v1/inside/jobs/claim",
      init,
    );
  });

  it("returns the original 401 when refresh cannot recover the session", async () => {
    const unauthorized = response(401);
    (global.fetch as jest.Mock).mockResolvedValueOnce(unauthorized);
    mockedRefresh.mockResolvedValue({
      ok: false,
      status: 401,
      alreadyRotated: false,
      sessionDead: true,
    });

    const onExpired = jest.fn();
    window.addEventListener("auth:session-expired", onExpired);
    await expect(
      authenticatedFetch("https://api.example.test/api/v1/inside/alerts", {
        method: "GET",
      }),
    ).resolves.toBe(unauthorized);
    window.removeEventListener("auth:session-expired", onExpired);
    expect(global.fetch).toHaveBeenCalledTimes(1);
    expect(onExpired).toHaveBeenCalledTimes(1);
  });

  it("does not refresh or expire on an RBAC 401", async () => {
    const unauthorized = {
      ok: false,
      status: 401,
      clone() {
        return {
          json: async () => ({ code: "AUTH_INSUFFICIENT_ROLE" }),
        };
      },
    } as Response;
    (global.fetch as jest.Mock).mockResolvedValueOnce(unauthorized);

    const onExpired = jest.fn();
    window.addEventListener("auth:session-expired", onExpired);
    await expect(
      authenticatedFetch(
        "https://api.example.test/api/v1/inside/businesses/1/reservations",
        { method: "GET" },
      ),
    ).resolves.toBe(unauthorized);
    window.removeEventListener("auth:session-expired", onExpired);
    expect(mockedRefresh).not.toHaveBeenCalled();
    expect(onExpired).not.toHaveBeenCalled();
  });

  it("does not refresh successful requests or retry a second 401", async () => {
    (global.fetch as jest.Mock).mockResolvedValueOnce(response(204));

    await authenticatedFetch(
      "https://api.example.test/api/v1/inside/alerts",
      { method: "GET" },
    );
    expect(mockedRefresh).not.toHaveBeenCalled();

    (global.fetch as jest.Mock)
      .mockResolvedValueOnce(response(401))
      .mockResolvedValueOnce(response(401));
    mockedRefresh.mockResolvedValueOnce({ ok: true });

    const result = await authenticatedFetch(
      "https://api.example.test/api/v1/inside/alerts",
      { method: "GET" },
    );
    expect(result.status).toBe(401);
    expect(global.fetch).toHaveBeenCalledTimes(3);
    expect(mockedRefresh).toHaveBeenCalledTimes(1);
  });
});
