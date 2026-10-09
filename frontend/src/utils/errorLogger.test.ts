export {};

const mockCaptureClientError = jest.fn();

jest.mock("@/lib/sentry/reporting", () => ({
  captureClientError: mockCaptureClientError,
}));

const originalEnv = process.env;
const originalFetch = global.fetch;

describe("errorLogger", () => {
  beforeEach(() => {
    jest.resetModules();
    jest.clearAllMocks();
    jest.spyOn(Date, "now").mockReturnValue(10000);
    jest.spyOn(console, "error").mockImplementation(() => undefined);
    jest.spyOn(console, "warn").mockImplementation(() => undefined);

    process.env = {
      ...originalEnv,
      API_URL: "https://api.example.test/api/v1",
    };
    global.fetch = jest.fn().mockResolvedValue({ ok: true }) as jest.Mock;
  });

  afterEach(() => {
    jest.restoreAllMocks();
  });

  afterAll(() => {
    process.env = originalEnv;
    global.fetch = originalFetch;
  });

  it("mirrors logged errors to Sentry and posts to the backend", async () => {
    const { logError } = await import("@/utils/errorLogger");
    const error = new Error("checkout failed");
    const additionalInfo = { business_id: 42 };

    await logError(error, "CheckoutButton", "submit", additionalInfo, "req-1");

    expect(mockCaptureClientError).toHaveBeenCalledWith({
      error,
      component: "CheckoutButton",
      functionName: "submit",
      additionalInfo,
      requestId: "req-1",
    });
    expect(global.fetch).toHaveBeenCalledWith(
      "https://api.example.test/api/v1/logs/error",
      expect.objectContaining({
        method: "POST",
        headers: { "Content-Type": "application/json" },
        credentials: "include",
      }),
    );

    const [, requestInit] = (global.fetch as jest.Mock).mock.calls[0];
    expect(JSON.parse(requestInit.body)).toMatchObject({
      error: "checkout failed",
      component: "CheckoutButton",
      function: "submit",
      requestId: "req-1",
      additionalInfo,
    });
  });

  it("debounces both the Sentry capture and the backend post for repeated signatures", async () => {
    const { logError } = await import("@/utils/errorLogger");

    // Same (component, function, message) signature within the debounce window:
    // ONE shared debounce gates BOTH the Sentry mirror and the backend post, so
    // an error loop cannot burn unbounded Sentry quota.
    await logError(new Error("checkout failed"), "CheckoutButton", "submit");
    await logError(new Error("checkout failed"), "CheckoutButton", "submit");

    expect(mockCaptureClientError).toHaveBeenCalledTimes(1);
    expect(global.fetch).toHaveBeenCalledTimes(1);
  });

  it("does not debounce distinct error signatures (per-signature keying)", async () => {
    const { logError } = await import("@/utils/errorLogger");

    // Two DIFFERENT messages in the same second: both must reach Sentry AND the
    // backend because the debounce is keyed per (component, function, message),
    // not on a single global timestamp.
    await logError(new Error("error A"), "CheckoutButton", "submit");
    await logError(new Error("error B"), "CheckoutButton", "submit");

    expect(mockCaptureClientError).toHaveBeenCalledTimes(2);
    expect(global.fetch).toHaveBeenCalledTimes(2);
  });

  it("still mirrors to Sentry when the backend log POST fails", async () => {
    global.fetch = jest.fn().mockRejectedValue(new Error("offline")) as jest.Mock;
    const { logError } = await import("@/utils/errorLogger");
    const error = new Error("checkout failed");

    await logError(error, "CheckoutButton", "submit");

    expect(mockCaptureClientError).toHaveBeenCalledWith({
      error,
      component: "CheckoutButton",
      functionName: "submit",
      additionalInfo: undefined,
      requestId: undefined,
    });
    expect(global.fetch).toHaveBeenCalledTimes(1);
  });
});
