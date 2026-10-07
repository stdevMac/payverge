/** @jest-environment jsdom */
import {
  getPublicConfig,
  getPublicEnv,
  getRenderedSiteUrl,
  onPublicEnvReady,
  PUBLIC_ENV_READY_EVENT,
} from "./publicConfig";

describe("getPublicConfig (browser)", () => {
  const savedApi = process.env.NEXT_PUBLIC_API_URL;

  afterEach(() => {
    delete window.__PAYVERGE_ENV__;
    if (savedApi === undefined) delete process.env.NEXT_PUBLIC_API_URL;
    else process.env.NEXT_PUBLIC_API_URL = savedApi;
  });

  it("reads window.__PAYVERGE_ENV__ injected by the root layout", () => {
    window.__PAYVERGE_ENV__ = Object.freeze({
      PUBLIC_URL: "https://a.example.test",
      API_URL: "/api/v1",
      NETWORK: "base",
      MAINTENANCE_MODE: "true",
    });
    const config = getPublicConfig();
    expect(config.publicUrl).toBe("https://a.example.test");
    expect(config.apiUrl).toBe("/api/v1");
    expect(config.network).toBe("base");
    expect(config.maintenanceMode).toBe(true);
  });

  it("treats the injected object as authoritative over build-time values", () => {
    process.env.NEXT_PUBLIC_API_URL = "https://stale-build.example.test/api/v1";
    window.__PAYVERGE_ENV__ = { API_URL: "/api/v1" };
    expect(getPublicConfig().apiUrl).toBe("/api/v1");
  });

  it("ignores keys outside the whitelist", () => {
    window.__PAYVERGE_ENV__ = {
      PUBLIC_URL: "https://a.example.test",
      JWT_SECRET_KEY: "nope",
    };
    expect(Object.keys(getPublicEnv())).not.toContain("JWT_SECRET_KEY");
  });

  it("picks up a replaced object (no stale cache)", () => {
    window.__PAYVERGE_ENV__ = { PUBLIC_URL: "https://a.example.test" };
    expect(getPublicConfig().publicUrl).toBe("https://a.example.test");
    window.__PAYVERGE_ENV__ = { PUBLIC_URL: "https://b.example.test" };
    expect(getPublicConfig().publicUrl).toBe("https://b.example.test");
  });

  it("uses the page origin when the server published no PUBLIC_URL", () => {
    window.__PAYVERGE_ENV__ = { PUBLIC_URL: "", API_URL: "/api/v1" };
    expect(getPublicConfig().publicUrl).toBe(window.location.origin);
  });

  it("reports the server-rendered site URL for hydration (getRenderedSiteUrl)", () => {
    window.__PAYVERGE_ENV__ = { PUBLIC_URL: "", API_URL: "/api/v1" };
    // The server rendered its dev default; the browser resolves its origin.
    expect(getRenderedSiteUrl()).toBe("http://localhost:3000");
    window.__PAYVERGE_ENV__ = { PUBLIC_URL: "https://a.example.test" };
    expect(getRenderedSiteUrl()).toBe("https://a.example.test");
    expect(getRenderedSiteUrl()).toBe(getPublicConfig().publicUrl);
  });

  it("falls back to build-time values when nothing was injected", () => {
    process.env.NEXT_PUBLIC_API_URL = "https://built.example.test/api/v1";
    expect(getPublicConfig().apiUrl).toBe("https://built.example.test/api/v1");
  });
});

describe("onPublicEnvReady (browser)", () => {
  const readyState = Object.getOwnPropertyDescriptor(
    Document.prototype,
    "readyState",
  );

  function setReadyState(state: DocumentReadyState) {
    Object.defineProperty(document, "readyState", {
      configurable: true,
      get: () => state,
    });
  }

  afterEach(() => {
    delete window.__PAYVERGE_ENV__;
    // Drop the instance override so the prototype getter applies again.
    delete (document as unknown as { readyState?: unknown }).readyState;
    expect(
      Object.getOwnPropertyDescriptor(Document.prototype, "readyState"),
    ).toEqual(readyState);
  });

  it("runs immediately when the config is already injected", () => {
    setReadyState("loading");
    window.__PAYVERGE_ENV__ = { PUBLIC_URL: "https://a.example.test" };
    const callback = jest.fn();
    onPublicEnvReady(callback);
    expect(callback).toHaveBeenCalledTimes(1);
  });

  it("runs immediately once parsing is over, even without a config object", () => {
    setReadyState("interactive");
    const callback = jest.fn();
    onPublicEnvReady(callback);
    expect(callback).toHaveBeenCalledTimes(1);
  });

  it("waits for the config script while the document is still parsing", () => {
    setReadyState("loading");
    const seen: Array<string | undefined> = [];
    onPublicEnvReady(() => {
      seen.push(getPublicConfig().publicUrl);
    });
    expect(seen).toEqual([]);

    window.__PAYVERGE_ENV__ = { PUBLIC_URL: "https://late.example.test" };
    window.dispatchEvent(new Event(PUBLIC_ENV_READY_EVENT));
    document.dispatchEvent(new Event("DOMContentLoaded"));
    window.dispatchEvent(new Event(PUBLIC_ENV_READY_EVENT));

    expect(seen).toEqual(["https://late.example.test"]);
  });

  it("falls back to DOMContentLoaded when no config script ever runs", () => {
    setReadyState("loading");
    const callback = jest.fn();
    onPublicEnvReady(callback);
    document.dispatchEvent(new Event("DOMContentLoaded"));
    window.dispatchEvent(new Event(PUBLIC_ENV_READY_EVENT));
    expect(callback).toHaveBeenCalledTimes(1);
  });
});
