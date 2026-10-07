import fs from "node:fs";
import path from "node:path";

import { uninstallPeerStamp } from "./lib/proxy/installPeerStamp";
import { getPeerStampNonce } from "./lib/proxy/peerStamp";

jest.mock("@sentry/nextjs", () => ({ captureRequestError: jest.fn() }));
jest.mock("../sentry.server.config", () => ({}));
jest.mock("../sentry.edge.config", () => ({}));

const FRONTEND = path.resolve(__dirname, "..");

describe("server instrumentation hook", () => {
  it("sits next to app/, the only place Next loads it from", () => {
    // next/dist/build/index.js: rootDir = join(pagesDir || appDir, "..").
    // A frontend-root instrumentation.ts never ran: no Sentry server init, no
    // boot config warnings, and no peer stamp, so the proxy sent no
    // X-Forwarded-For at all.
    expect(fs.existsSync(path.join(FRONTEND, "src", "app"))).toBe(true);
    expect(fs.existsSync(path.join(FRONTEND, "app"))).toBe(false);
    expect(fs.existsSync(path.join(__dirname, "instrumentation.ts"))).toBe(true);
    expect(fs.existsSync(path.join(FRONTEND, "instrumentation.ts"))).toBe(false);
    expect(fs.existsSync(path.join(FRONTEND, "instrumentation.js"))).toBe(false);
  });

  describe("register()", () => {
    const savedRuntime = process.env.NEXT_RUNTIME;
    let warn: jest.SpyInstance;

    beforeEach(() => {
      warn = jest.spyOn(console, "warn").mockImplementation(() => {});
      uninstallPeerStamp();
    });

    afterEach(() => {
      warn.mockRestore();
      uninstallPeerStamp();
      if (savedRuntime === undefined) delete process.env.NEXT_RUNTIME;
      else process.env.NEXT_RUNTIME = savedRuntime;
    });

    it("installs the peer stamp in the nodejs runtime", async () => {
      process.env.NEXT_RUNTIME = "nodejs";
      const { register } = await import("./instrumentation");
      expect(getPeerStampNonce()).toBeUndefined();
      await register();
      expect(getPeerStampNonce()).toEqual(expect.any(String));
    });

    it("does not install it in the edge runtime", async () => {
      process.env.NEXT_RUNTIME = "edge";
      const { register } = await import("./instrumentation");
      await register();
      expect(getPeerStampNonce()).toBeUndefined();
    });
  });
});
