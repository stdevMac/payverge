/** @jest-environment jsdom */
import { PUBLIC_ENV_READY_EVENT } from "@/config/publicConfig";
import {
  CONSENT_CHANGE_EVENT,
  CONSENT_STORAGE_KEY,
  writeConsent,
} from "@/lib/analytics/consentGate";

// instrumentation-client is evaluated by Next's async main chunk, which can
// run before the root layout's window.__PAYVERGE_ENV__ script. Sentry's DSN is
// runtime config, so init must wait for that script instead of reading the
// build-time fallback at module load.

const init = jest.fn();
const addIntegration = jest.fn();
const replayStop = jest.fn(() => Promise.resolve());
const replayStart = jest.fn();
const replayStartBuffering = jest.fn();
const replayIntegration = jest.fn(() => ({
  name: "Replay",
  stop: replayStop,
  start: replayStart,
  startBuffering: replayStartBuffering,
}));

const consentListeners = new Set<EventListener>();
const nativeAddEventListener = window.addEventListener.bind(window);
const nativeRemoveEventListener = window.removeEventListener.bind(window);

function loadInstrumentationClient() {
  jest.isolateModules(() => {
    jest.doMock("@sentry/nextjs", () => ({
      __esModule: true,
      init,
      addIntegration,
      replayIntegration,
      captureRouterTransitionStart: jest.fn(),
    }));
    require("../../../instrumentation-client");
  });
}

function setReadyState(state: DocumentReadyState) {
  Object.defineProperty(document, "readyState", {
    configurable: true,
    get: () => state,
  });
}

const runtimeEnv = {
  SENTRY_DSN: "https://public@o1.ingest.sentry.example.test/7",
  SENTRY_ENVIRONMENT: "production",
  SENTRY_ENABLED: "true",
};

const analyticsConsent = JSON.stringify({
  version: 1,
  analytics: true,
  marketing: false,
  decidedAt: "2026-01-01T00:00:00Z",
});

beforeAll(() => {
  window.addEventListener = ((
    type: string,
    listener: EventListenerOrEventListenerObject,
    options?: boolean | AddEventListenerOptions,
  ) => {
    if (type === CONSENT_CHANGE_EVENT && typeof listener === "function") {
      consentListeners.add(listener);
    }
    nativeAddEventListener(type, listener, options);
  }) as typeof window.addEventListener;
});

afterEach(() => {
  for (const listener of consentListeners) {
    nativeRemoveEventListener(CONSENT_CHANGE_EVENT, listener);
  }
  consentListeners.clear();
  init.mockReset();
  addIntegration.mockReset();
  replayStop.mockClear();
  replayStart.mockClear();
  replayStartBuffering.mockClear();
  replayIntegration.mockClear();
  window.localStorage.clear();
  delete window.__PAYVERGE_ENV__;
  delete (document as unknown as { readyState?: unknown }).readyState;
  jest.dontMock("@sentry/nextjs");
});

afterAll(() => {
  delete (window as unknown as { addEventListener?: unknown }).addEventListener;
});

function replayEnv() {
  return {
    ...runtimeEnv,
    SENTRY_REPLAYS_SESSION_SAMPLE_RATE: "0.1",
  };
}

function initIntegrations(): unknown[] {
  const options = init.mock.calls[0]?.[0] as
    | { integrations?: unknown[] }
    | undefined;
  return options?.integrations ?? [];
}

describe("instrumentation-client Sentry init", () => {
  it("waits for the runtime config script while the document is parsing", () => {
    setReadyState("loading");
    loadInstrumentationClient();
    expect(init).not.toHaveBeenCalled();

    window.__PAYVERGE_ENV__ = runtimeEnv;
    window.dispatchEvent(new Event(PUBLIC_ENV_READY_EVENT));

    expect(init).toHaveBeenCalledTimes(1);
    expect(init.mock.calls[0][0]).toMatchObject({
      dsn: runtimeEnv.SENTRY_DSN,
      environment: "production",
    });
  });

  it("initializes immediately when the config is already present", () => {
    setReadyState("loading");
    window.__PAYVERGE_ENV__ = runtimeEnv;
    loadInstrumentationClient();
    expect(init).toHaveBeenCalledTimes(1);
  });

  it("stays off when the runtime config carries no DSN", () => {
    setReadyState("loading");
    loadInstrumentationClient();
    window.__PAYVERGE_ENV__ = { SENTRY_DSN: "" };
    window.dispatchEvent(new Event(PUBLIC_ENV_READY_EVENT));
    document.dispatchEvent(new Event("DOMContentLoaded"));
    expect(init).not.toHaveBeenCalled();
  });

  it("does not add Replay when analytics consent is missing", () => {
    setReadyState("loading");
    window.__PAYVERGE_ENV__ = replayEnv();
    loadInstrumentationClient();

    expect(init).toHaveBeenCalledTimes(1);
    expect(initIntegrations()).not.toEqual(
      expect.arrayContaining([expect.objectContaining({ name: "Replay" })]),
    );
    expect(addIntegration).not.toHaveBeenCalled();
  });

  it("adds masked Replay once when analytics consent is already stored", () => {
    setReadyState("loading");
    window.localStorage.setItem(CONSENT_STORAGE_KEY, analyticsConsent);
    window.__PAYVERGE_ENV__ = replayEnv();
    loadInstrumentationClient();

    expect(addIntegration).toHaveBeenCalledTimes(1);
    expect(replayIntegration).toHaveBeenCalledWith(
      expect.objectContaining({ maskAllText: true }),
    );
    expect(initIntegrations()).not.toEqual(
      expect.arrayContaining([expect.objectContaining({ name: "Replay" })]),
    );
  });

  it("adds Replay when analytics consent is granted after init", () => {
    setReadyState("loading");
    window.__PAYVERGE_ENV__ = replayEnv();
    loadInstrumentationClient();
    expect(addIntegration).not.toHaveBeenCalled();

    writeConsent({ analytics: true, marketing: false });

    expect(addIntegration).toHaveBeenCalledTimes(1);
    expect(replayIntegration).toHaveBeenCalledWith(
      expect.objectContaining({ maskAllText: true }),
    );
  });

  it("stops Replay when analytics consent is withdrawn", () => {
    setReadyState("loading");
    window.localStorage.setItem(CONSENT_STORAGE_KEY, analyticsConsent);
    window.__PAYVERGE_ENV__ = replayEnv();
    loadInstrumentationClient();
    expect(addIntegration).toHaveBeenCalledTimes(1);

    writeConsent({ analytics: false, marketing: false });

    expect(replayStop).toHaveBeenCalledTimes(1);
  });

  it("restarts Replay when consent is granted again after a withdrawal", () => {
    setReadyState("loading");
    window.localStorage.setItem(CONSENT_STORAGE_KEY, analyticsConsent);
    window.__PAYVERGE_ENV__ = replayEnv();
    loadInstrumentationClient();

    writeConsent({ analytics: false, marketing: false });
    expect(replayStop).toHaveBeenCalledTimes(1);
    expect(replayStart).not.toHaveBeenCalled();

    writeConsent({ analytics: true, marketing: false });
    expect(addIntegration).toHaveBeenCalledTimes(1);
    expect(replayStart).toHaveBeenCalledTimes(1);
  });

  it("skips Replay entirely when both sample rates are zero", () => {
    setReadyState("loading");
    window.localStorage.setItem(CONSENT_STORAGE_KEY, analyticsConsent);
    window.__PAYVERGE_ENV__ = runtimeEnv;
    loadInstrumentationClient();

    expect(init).toHaveBeenCalledTimes(1);
    expect(replayIntegration).not.toHaveBeenCalled();
    expect(addIntegration).not.toHaveBeenCalled();

    writeConsent({ analytics: true, marketing: false });
    expect(addIntegration).not.toHaveBeenCalled();
  });
});
