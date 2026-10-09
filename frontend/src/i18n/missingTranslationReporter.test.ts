import {
  __resetReporterForTests,
  reportMissingTranslation,
} from "./missingTranslationReporter";

describe("missingTranslationReporter", () => {
  const originalWindow = global.window;
  const originalNavigator = global.navigator;
  const originalFetch = global.fetch;
  const originalApiUrl = process.env.API_URL;

  beforeEach(() => {
    // Same-origin API base (the production default behind the Next proxy).
    process.env.API_URL = "/api/v1";
    __resetReporterForTests();
    jest.spyOn(console, "warn").mockImplementation(() => {});
    Object.defineProperty(global, "window", {
      configurable: true,
      value: { location: { pathname: "/business/1/dashboard" } },
    });
  });

  afterEach(() => {
    jest.restoreAllMocks();
    Object.defineProperty(global, "window", {
      configurable: true,
      value: originalWindow,
    });
    Object.defineProperty(global, "navigator", {
      configurable: true,
      value: originalNavigator,
    });
    global.fetch = originalFetch;
    if (originalApiUrl === undefined) {
      delete process.env.API_URL;
    } else {
      process.env.API_URL = originalApiUrl;
    }
  });

  it("uses sendBeacon when available", () => {
    const sendBeacon = jest.fn();
    Object.defineProperty(global, "navigator", {
      configurable: true,
      value: { sendBeacon },
    });

    reportMissingTranslation({
      locale: "es",
      key: "dashboard.missing",
      fallbackUsed: "english",
    });

    expect(sendBeacon).toHaveBeenCalledWith(
      "/api/v1/analytics/missing_translation",
      expect.any(Blob),
    );
  });

  it("falls back to fetch keepalive when sendBeacon is unavailable", () => {
    const fetchMock = jest.fn().mockResolvedValue({ ok: true });
    Object.defineProperty(global, "navigator", {
      configurable: true,
      value: {},
    });
    global.fetch = fetchMock;

    reportMissingTranslation({
      locale: "es",
      key: "dashboard.missing",
      fallbackUsed: "leaf",
    });

    expect(fetchMock).toHaveBeenCalledWith(
      "/api/v1/analytics/missing_translation",
      expect.objectContaining({
        method: "POST",
        keepalive: true,
        headers: { "Content-Type": "application/json" },
      }),
    );
  });

  it("uses the configured backend API origin when present", () => {
    process.env.API_URL = "https://api.example.test/api/v1";
    const sendBeacon = jest.fn();
    Object.defineProperty(global, "navigator", {
      configurable: true,
      value: { sendBeacon },
    });

    reportMissingTranslation({
      locale: "es",
      key: "dashboard.configured-host",
      fallbackUsed: "english",
    });

    expect(sendBeacon).toHaveBeenCalledWith(
      "https://api.example.test/api/v1/analytics/missing_translation",
      expect.any(Blob),
    );
  });
});
