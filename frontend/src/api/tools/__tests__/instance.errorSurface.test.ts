/**
 * @jest-environment jsdom
 */
jest.mock("react-hot-toast", () => {
  const errorFn = jest.fn();
  const successFn = jest.fn();
  const toastFn = Object.assign(jest.fn(), { error: errorFn, success: successFn });
  return { __esModule: true, default: toastFn, toast: toastFn };
});

import MockAdapter from "axios-mock-adapter";
import toast from "react-hot-toast";
import { axiosInstance } from "../instance";

const mockedError = toast.error as jest.Mock;

describe("axios interceptor — opt-in 4xx error surfacing", () => {
  let mock: MockAdapter;

  beforeEach(() => {
    mock = new MockAdapter(axiosInstance);
    mockedError.mockClear();
    // Defeat the 3s toast cooldown across tests with unique messages.
  });

  afterEach(() => {
    mock.restore();
  });

  test("toasts the backend error string on a 400 when _surfaceError is set", async () => {
    mock.onPost("/crm/register").reply(400, { error: "Email already taken" });

    await expect(
      axiosInstance.post(
        "/crm/register",
        { email: "x@y.z" },
        { _surfaceError: true, _maxRetries: 0 } as never,
      ),
    ).rejects.toThrow();

    expect(mockedError).toHaveBeenCalledWith("Email already taken");
  });

  test("stays silent on a 400 when _surfaceError is NOT set", async () => {
    mock.onPost("/orders").reply(400, { error: "Table is closed" });

    await expect(
      axiosInstance.post("/orders", {}, { _maxRetries: 0 } as never),
    ).rejects.toThrow();

    expect(mockedError).not.toHaveBeenCalled();
  });

  test("does not toast a 401 even when _surfaceError is set (auth flow owns it)", async () => {
    mock.onGet("/inside/users/me").reply(401, { error: "expired" });
    const originalFetch = global.fetch;
    // Refresh attempt fails so the request bubbles up; we only assert no toast.
    global.fetch = jest.fn(
      async () => ({ ok: false, status: 401 }) as Response,
    ) as unknown as typeof fetch;

    await expect(
      axiosInstance.get("/inside/users/me", {
        _surfaceError: true,
        _maxRetries: 0,
      } as never),
    ).rejects.toThrow();

    expect(mockedError).not.toHaveBeenCalled();
    global.fetch = originalFetch;
  });

  test("falls back to a generic message when _surfaceError is set but body has no error field", async () => {
    mock.onPost("/loyalty/redeem").reply(422, {});

    await expect(
      axiosInstance.post("/loyalty/redeem", {}, {
        _surfaceError: true,
        _maxRetries: 0,
      } as never),
    ).rejects.toThrow();

    expect(mockedError).toHaveBeenCalledWith(
      "Please check your input and try again.",
    );
  });
});

describe("axios interceptor — _skipErrorToast", () => {
  let mock: MockAdapter;

  beforeEach(() => {
    mock = new MockAdapter(axiosInstance);
    mockedError.mockClear();
  });

  afterEach(() => {
    mock.restore();
  });

  test("stays silent on a network failure when _skipErrorToast is set", async () => {
    mock.onPost("/auth/login").networkError();

    await expect(
      axiosInstance.post(
        "/auth/login",
        { email: "a@b.c", password: "x" },
        { _skipErrorToast: true, _maxRetries: 0 } as never,
      ),
    ).rejects.toThrow();

    expect(mockedError).not.toHaveBeenCalled();
  });

  // #596 — waiter chat requests set _skipErrorToast so a backend 5xx surfaces
  // only as the in-thread Sage error copy, never the global server toast.
  test("stays silent on a 5xx when _skipErrorToast is set", async () => {
    mock.onPost("/ai-waiter/5").reply(500, { error: "model budget exceeded" });

    await expect(
      axiosInstance.post(
        "/ai-waiter/5",
        { history: [] },
        { _skipErrorToast: true, _maxRetries: 0 } as never,
      ),
    ).rejects.toThrow();

    expect(mockedError).not.toHaveBeenCalled();
  });

  test("toasts a network failure by default", async () => {
    window.history.replaceState({}, "", "/business/86/dashboard");
    mock.onPost("/inside/orders").networkError();

    await expect(
      axiosInstance.post("/inside/orders", {}, { _maxRetries: 0 } as never),
    ).rejects.toThrow();

    expect(mockedError).toHaveBeenCalledWith(
      "Couldn't reach the server. Check your connection and try again.",
    );
    window.history.replaceState({}, "", "/");
  });

  test("toasts Spanish network copy on Spanish operator pages", async () => {
    document.documentElement.lang = "es";
    window.history.replaceState({}, "", "/business/86/dashboard");
    mock.onPost("/inside/orders").networkError();

    await expect(
      axiosInstance.post("/inside/orders", {}, { _maxRetries: 0 } as never),
    ).rejects.toThrow();

    expect(mockedError).toHaveBeenCalledWith(
      "No pudimos contactar al servidor. Revisa tu conexión e inténtalo de nuevo.",
    );
    document.documentElement.lang = "en";
    window.history.replaceState({}, "", "/");
  });

  test("does not toast analytics transport failures", async () => {
    mock.onPost("/analytics/page-view").networkError();

    await expect(
      axiosInstance.post("/analytics/page-view", {}, { _maxRetries: 0 } as never),
    ).rejects.toThrow();

    expect(mockedError).not.toHaveBeenCalled();
  });

  test("does not toast unprompted session-info failures on a Spanish public page", async () => {
    document.documentElement.lang = "es";
    window.history.replaceState({}, "", "/es");
    mock.onGet("/auth/session-info").networkError();

    await expect(
      axiosInstance.get("/auth/session-info", { _maxRetries: 0 } as never),
    ).rejects.toThrow();

    expect(mockedError).not.toHaveBeenCalled();
    document.documentElement.lang = "en";
    window.history.replaceState({}, "", "/");
  });
});
