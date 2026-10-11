// jsdom suites (/** @jest-environment jsdom */) have no WHATWG fetch
// primitives — Node's native fetch/Response are not injected into the jsdom
// global. Polyfill only when missing so node-env suites keep native undici.
if (
  typeof globalThis.Response === "undefined" ||
  typeof globalThis.fetch === "undefined"
) {
  require("whatwg-fetch");
}

// Canonical origin fixture. Canonicals/OG/sitemap URLs are derived at runtime
// from PUBLIC_URL (src/config/publicConfig.ts); without it the dev default is
// http://localhost:3000. Pin a stable origin so URL-shape assertions stay
// deterministic. Suites that test origin handling set/delete it themselves
// (see robots/sitemap/security.txt two-origin tests). Never triggers server
// fetches: getServerApiUrl only hairpins through PUBLIC_URL in production.
// Pinned unconditionally, and the other runtime config names cleared, so a
// shell that exports a deployment's env cannot change test outcomes.
process.env.PUBLIC_URL = "https://payverge.io";
for (const name of [
  "API_URL",
  "BACKEND_INTERNAL_URL",
  "INTERNAL_API_URL",
  "SUPPORT_EMAIL",
  "SECURITY_EMAIL",
  "NETWORK",
  "PUBLIC_RPC_URL",
  "MAINTENANCE_MODE",
  "VAPID_PUBLIC_KEY",
  "MEDIA_ORIGINS",
  "FRONTEND_SENTRY_DSN",
  "FRONTEND_SENTRY_ENVIRONMENT",
  "FRONTEND_SENTRY_ENABLED",
  "FRONTEND_SENTRY_TRACES_SAMPLE_RATE",
  "FRONTEND_SENTRY_REPLAYS_SESSION_SAMPLE_RATE",
  "FRONTEND_SENTRY_REPLAYS_ON_ERROR_SAMPLE_RATE",
  "POSTHOG_KEY",
  "POSTHOG_HOST",
  "LIFI_INTEGRATOR",
  "SEO_INDEXING",
  "SEO_TWITTER_HANDLE",
  "CLOUDFLARE_INSIGHTS",
  "EDGE",
]) {
  delete process.env[name];
}

import "@testing-library/jest-dom";
import { configure as configureTestingLibrary } from "@testing-library/dom";

// Default asyncUtilTimeout is 1000ms. Under full-suite worker contention the
// event loop is starved so findBy*/waitFor can miss legitimate renders that
// finish in a few hundred ms in isolation — rotating "Unable to find" /
// Exceeded-timeout flakes (BusinessPageEditor, ReservationManager, BillManager,
// AiWaiter, …). 5s still fails real hangs quickly relative to jest testTimeout.
configureTestingLibrary({ asyncUtilTimeout: 5000 });

// Treat React scheduling leaks and missing canvas implementations as test
// failures. Individual tests that intentionally exercise an error may replace
// console.error for that test and restore it afterwards.
let consoleErrorGuard;
let consoleWarnGuard;

const guardedConsole =
  (original) =>
  (...args) => {
    const testPath = (expect.getState().testPath || "").replace(/\\/g, "/");
    const guardsMarketingSurface =
      testPath.includes("/src/components/business/Marketing/") ||
      /\/src\/api\/marketing\.test\.[jt]sx?$/.test(testPath);
    const output = args
      .map((value) =>
        value instanceof Error
          ? `${value.name}: ${value.message}`
          : String(value),
      )
      .join(" ");
    const hasUnexpectedWarning =
      /not wrapped in act\(\.\.\.\)/i.test(output) ||
      /The current testing environment is not configured to support act/i.test(
        output,
      ) ||
      /Not implemented: HTMLCanvasElement\.prototype\.(?:getContext|toDataURL|toBlob)/i.test(
        output,
      );
    if (guardsMarketingSurface && hasUnexpectedWarning) {
      throw new Error(`Unexpected test warning: ${output}`);
    }
    original(...args);
  };

beforeEach(() => {
  // M6: sessionStorage-backed tab state is a product behavior now — every test
  // must start from a clean session, like the fresh DOM. localStorage gets the
  // same treatment so cross-test persistence surprises fail loudly here.
  try {
    window.sessionStorage?.clear();
    window.localStorage?.clear();
  } catch {
    /* node-env suites have no window */
  }
  const originalError = console.error;
  const originalWarn = console.warn;
  consoleErrorGuard = jest
    .spyOn(console, "error")
    .mockImplementation(guardedConsole(originalError));
  consoleWarnGuard = jest
    .spyOn(console, "warn")
    .mockImplementation(guardedConsole(originalWarn));
});

// jsdom does not expose several Node built-in globals that viem/wagmi's crypto
// and ABI encoding paths rely on (TextEncoder/TextDecoder for byte<->hex, the
// WebCrypto API for hashing). Node provides them under node:util / node:crypto;
// inject them only when missing so node-env suites keep their native globals.
if (typeof globalThis.TextEncoder === "undefined") {
  globalThis.TextEncoder = require("node:util").TextEncoder;
}
if (typeof globalThis.TextDecoder === "undefined") {
  globalThis.TextDecoder = require("node:util").TextDecoder;
}
if (typeof globalThis.crypto === "undefined") {
  globalThis.crypto = require("node:crypto").webcrypto;
}

// NextUI wraps buttons, ripples, and modals in Framer Motion's LazyMotion.
// In jsdom that async feature loader settles after React Testing Library's
// render act boundary, which prints act warnings unrelated to component logic.
jest.mock("framer-motion", () => {
  const actual = jest.requireActual("framer-motion");
  const React = jest.requireActual("react");
  return {
    ...actual,
    LazyMotion: ({ children }) =>
      React.createElement(React.Fragment, null, children),
  };
});

// Mock Next.js router
jest.mock("next/router", () => ({
  useRouter() {
    return {
      route: "/",
      pathname: "/",
      query: {},
      asPath: "/",
      push: jest.fn(),
      pop: jest.fn(),
      reload: jest.fn(),
      back: jest.fn(),
      prefetch: jest.fn(),
      beforePopState: jest.fn(),
      events: {
        on: jest.fn(),
        off: jest.fn(),
        emit: jest.fn(),
      },
    };
  },
}));

// Mock Next.js navigation
jest.mock("next/navigation", () => ({
  useRouter() {
    return {
      push: jest.fn(),
      replace: jest.fn(),
      prefetch: jest.fn(),
      back: jest.fn(),
      forward: jest.fn(),
      refresh: jest.fn(),
    };
  },
  useSearchParams() {
    return new URLSearchParams();
  },
  usePathname() {
    return "/";
  },
  useParams() {
    return {};
  },
}));

// Mock window.matchMedia when running with a DOM-like environment.
if (typeof window !== "undefined") {
  Object.defineProperty(window, "matchMedia", {
    writable: true,
    value: jest.fn().mockImplementation((query) => ({
      matches: false,
      media: query,
      onchange: null,
      addListener: jest.fn(), // deprecated
      removeListener: jest.fn(), // deprecated
      addEventListener: jest.fn(),
      removeEventListener: jest.fn(),
      dispatchEvent: jest.fn(),
    })),
  });
}

// jsdom implements no scroll geometry, so `Element.prototype.scrollTo` does not
// exist. Components that keep a chat log pinned to the bottom call it in an
// effect, so any suite that mounts one crashes unless something polyfilled it
// first. Several suites did that at module scope, which made the whole class
// order-dependent: a suite passed or failed on which worker it landed in and
// what ran before it. Own it here once so no suite inherits another's polyfill.
if (typeof Element !== "undefined" && !Element.prototype.scrollTo) {
  Element.prototype.scrollTo = function scrollTo(options, y) {
    if (typeof options === "number") {
      this.scrollLeft = options;
      this.scrollTop = typeof y === "number" ? y : this.scrollTop;
      return;
    }
    if (options && typeof options === "object") {
      if (typeof options.left === "number") this.scrollLeft = options.left;
      if (typeof options.top === "number") this.scrollTop = options.top;
    }
  };
}

// Safety net: individual suites own their teardown, but reset timer mode and
// clear any timer a suite leaked so the next test in the same worker is not
// stuck on fake timers (many suites call useFakeTimers without a balanced
// useRealTimers on every path) and so open timers cannot hold the process.
afterEach(() => {
  consoleErrorGuard?.mockRestore();
  consoleWarnGuard?.mockRestore();
  jest.useRealTimers();
  jest.clearAllTimers();
});
