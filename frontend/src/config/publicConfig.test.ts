import vm from "node:vm";

import {
  PUBLIC_ENV_KEYS,
  absoluteSiteUrl,
  escapeJsonForScript,
  getPublicConfig,
  getServerApiUrl,
  resolvePublicEnv,
  serializePublicEnvScript,
  toPublicConfig,
  validatePublicEnv,
} from "./publicConfig";

const ENV_KEYS_TOUCHED = [
  "PUBLIC_URL",
  "API_URL",
  "BACKEND_INTERNAL_URL",
  "INTERNAL_API_URL",
  "SUPPORT_EMAIL",
  "NETWORK",
  "MEDIA_ORIGINS",
  "MAINTENANCE_MODE",
  "DB_PASSWORD",
  "JWT_SECRET_KEY",
  "NEXT_PUBLIC_API_URL",
  "NEXT_PUBLIC_BASE_URL",
  "NEXT_PUBLIC_APP_URL",
  "NEXT_PUBLIC_FRONTEND_URL",
  "NEXT_PUBLIC_PUBLIC_URL",
  "FRONTEND_URL",
  "NEXT_PUBLIC_RPC_URL",
  "RPC_URL",
  "SENTRY_DSN",
  "NEXT_PUBLIC_SENTRY_DSN",
];

const saved: Record<string, string | undefined> = {};
beforeEach(() => {
  for (const key of ENV_KEYS_TOUCHED) {
    saved[key] = process.env[key];
    delete process.env[key];
  }
});
afterEach(() => {
  for (const key of ENV_KEYS_TOUCHED) {
    if (saved[key] === undefined) delete process.env[key];
    else process.env[key] = saved[key];
  }
});

describe("resolvePublicEnv (server)", () => {
  it("reads the canonical runtime names", () => {
    const env = resolvePublicEnv(
      {
        PUBLIC_URL: "https://pos.example.test/",
        API_URL: "/api/v1/",
        SUPPORT_EMAIL: "help@example.test",
        NETWORK: "base",
        MAINTENANCE_MODE: "true",
      },
      "production",
      { includeBuildFallback: false },
    );
    expect(env.PUBLIC_URL).toBe("https://pos.example.test");
    expect(env.API_URL).toBe("/api/v1");
    expect(env.SUPPORT_EMAIL).toBe("help@example.test");
    expect(env.NETWORK).toBe("base");
    expect(env.RPC_URL).toBe("https://mainnet.base.org");
    expect(env.MAINTENANCE_MODE).toBe("true");
  });

  it("reads PUBLIC_URL only: retired BASE/APP/FRONTEND names are ignored", () => {
    for (const retired of [
      "NEXT_PUBLIC_BASE_URL",
      "NEXT_PUBLIC_APP_URL",
      "NEXT_PUBLIC_FRONTEND_URL",
      "FRONTEND_URL",
    ]) {
      const env = resolvePublicEnv(
        { [retired]: "https://retired.example.test" },
        "production",
        { includeBuildFallback: false },
      );
      expect(env.PUBLIC_URL).not.toContain("retired.example.test");
    }
  });

  it("never takes the backend's own API origin (BASE_URL/APP_BASE_URL) as PUBLIC_URL", () => {
    const env = resolvePublicEnv(
      {
        BASE_URL: "https://api.example.test",
        APP_BASE_URL: "https://api.example.test",
      },
      "production",
      { includeBuildFallback: false },
    );
    expect(env.PUBLIC_URL).not.toContain("api.example.test");
  });

  it("has no vendor support-email fallback", () => {
    const env = resolvePublicEnv({}, "production", {
      includeBuildFallback: false,
    });
    expect(env.SUPPORT_EMAIL).toBe("");
  });

  it("prefers PUBLIC_URL over its NEXT_PUBLIC_ build-time name", () => {
    const env = resolvePublicEnv(
      {
        PUBLIC_URL: "https://new.example.test",
        NEXT_PUBLIC_PUBLIC_URL: "https://old.example.test",
      },
      "production",
      { includeBuildFallback: false },
    );
    expect(env.PUBLIC_URL).toBe("https://new.example.test");
  });

  it("defaults the API to same-origin /api/v1 in production", () => {
    const env = resolvePublicEnv({}, "production", {
      includeBuildFallback: false,
    });
    expect(env.API_URL).toBe("/api/v1");
  });

  it("defaults the API to localhost:8080 in development unless the proxy is on", () => {
    expect(
      resolvePublicEnv({}, "development", { includeBuildFallback: false })
        .API_URL,
    ).toBe("http://localhost:8080/api/v1");
    expect(
      resolvePublicEnv(
        { BACKEND_INTERNAL_URL: "http://backend:8080" },
        "development",
        { includeBuildFallback: false },
      ).API_URL,
    ).toBe("/api/v1");
  });

  it("allows plain http on loopback hosts in production without any flag", () => {
    const env = resolvePublicEnv(
      {
        PUBLIC_URL: "http://localhost:3000",
        API_URL: "http://127.0.0.1:8080/api/v1",
      },
      "production",
      { includeBuildFallback: false },
    );
    expect(env.PUBLIC_URL).toBe("http://localhost:3000");
    expect(env.API_URL).toBe("http://127.0.0.1:8080/api/v1");
  });

  it("drops unsafe URLs back to defaults instead of publishing them", () => {
    const env = resolvePublicEnv(
      {
        PUBLIC_URL: "javascript:alert(1)",
        API_URL: "https://user:pass@api.example.test/api/v1",
        NEXT_PUBLIC_RPC_URL: "ftp://rpc.example.test",
        SUPPORT_EMAIL: "<script>@x.y",
      },
      "production",
      { includeBuildFallback: false },
    );
    expect(env.PUBLIC_URL).toBe("http://localhost:3000");
    expect(env.API_URL).toBe("/api/v1");
    expect(env.RPC_URL).toBe("https://sepolia.base.org");
    expect(env.SUPPORT_EMAIL).not.toContain("<");
  });

  it("rejects plain http on public hosts in production", () => {
    const env = resolvePublicEnv(
      { PUBLIC_URL: "http://pos.example.test" },
      "production",
      { includeBuildFallback: false },
    );
    expect(env.PUBLIC_URL).toBe("http://localhost:3000");
    expect(
      validatePublicEnv(
        { PUBLIC_URL: "http://pos.example.test" },
        "production",
      ),
    ).toContain(
      "PUBLIC_URL: HTTPS required in production (except localhost and private-network hosts)",
    );
  });

  it.each([
    "http://192.168.1.20:3000",
    "http://10.0.0.5",
    "http://172.20.1.1:8080",
    "http://pos.local",
    "http://till.home.arpa",
    "http://[fd12:3456::1]:3000",
  ])(
    "accepts plain http on a private-network PUBLIC_URL (%s) for LAN installs",
    (url) => {
      const env = resolvePublicEnv({ PUBLIC_URL: url }, "production", {
        includeBuildFallback: false,
      });
      expect(env.PUBLIC_URL).toBe(new URL(url).origin);
    },
  );

  it.each([
    "http://172.32.0.1",
    "http://192.169.0.1",
    "http://8.8.8.8",
    "http://local.example.test",
  ])("still rejects plain http on a public host (%s)", (url) => {
    const env = resolvePublicEnv({ PUBLIC_URL: url }, "production", {
      includeBuildFallback: false,
    });
    expect(env.PUBLIC_URL).toBe("http://localhost:3000");
  });

  it("never publishes the backend's own RPC_URL or SENTRY_DSN", () => {
    const env = resolvePublicEnv(
      {
        RPC_URL: "https://base-mainnet.g.alchemy.com/v2/secret-key",
        SENTRY_DSN: "https://backendkey@o1.ingest.sentry.io/2",
      },
      "production",
      { includeBuildFallback: false },
    );
    expect(env.RPC_URL).toBe("https://sepolia.base.org");
    expect(env.SENTRY_DSN).toBe("");
  });

  it("reads the process env at call time (no module-level freeze)", () => {
    process.env.PUBLIC_URL = "https://a.example.test";
    expect(getPublicConfig().publicUrl).toBe("https://a.example.test");
    process.env.PUBLIC_URL = "https://b.example.test";
    expect(getPublicConfig().publicUrl).toBe("https://b.example.test");
    expect(absoluteSiteUrl("/sitemap.xml")).toBe(
      "https://b.example.test/sitemap.xml",
    );
    expect(absoluteSiteUrl("/")).toBe("https://b.example.test");
  });
});

describe("validatePublicEnv", () => {
  it("reports key and rule only, never the value", () => {
    const secretish = "https://user:hunter2@pos.example.test";
    const issues = validatePublicEnv(
      { PUBLIC_URL: secretish, BACKEND_INTERNAL_URL: "http://backend:8080" },
      "production",
    );
    expect(issues).toEqual(["PUBLIC_URL: URL credentials are forbidden"]);
    expect(issues.join(" ")).not.toContain("hunter2");
  });

  it("requires PUBLIC_URL in production only", () => {
    expect(validatePublicEnv({}, "production")).toContain(
      "PUBLIC_URL: missing",
    );
    expect(validatePublicEnv({}, "development")).toEqual([]);
  });

  it("warns when a same-origin production install has no proxy target", () => {
    const missing =
      "BACKEND_INTERNAL_URL: missing (same-origin proxy off; optimized /media images will fail)";
    expect(
      validatePublicEnv(
        { PUBLIC_URL: "https://pos.example.test" },
        "production",
      ),
    ).toEqual([missing]);
    expect(
      validatePublicEnv(
        { PUBLIC_URL: "https://pos.example.test", API_URL: "/api/v1" },
        "production",
      ),
    ).toEqual([missing]);
    // Split origin: the browser talks to the API host directly.
    expect(
      validatePublicEnv(
        {
          PUBLIC_URL: "https://pos.example.test",
          API_URL: "https://api.example.test/api/v1",
        },
        "production",
      ),
    ).toEqual([]);
    expect(
      validatePublicEnv(
        {
          PUBLIC_URL: "https://pos.example.test",
          BACKEND_INTERNAL_URL: "http://backend:8080",
        },
        "production",
      ),
    ).toEqual([]);
    expect(
      validatePublicEnv({ PUBLIC_URL: "http://localhost:3000" }, "development"),
    ).toEqual([]);
  });

  it("flags unknown networks and malformed proxy targets", () => {
    const issues = validatePublicEnv(
      {
        PUBLIC_URL: "https://pos.example.test",
        NETWORK: "mainnet",
        BACKEND_INTERNAL_URL: "backend:8080",
      },
      "production",
    );
    expect(issues).toEqual(
      expect.arrayContaining([
        "NETWORK: must be base or baseSepolia",
        expect.stringMatching(/^BACKEND_INTERNAL_URL: /),
      ]),
    );
  });
});

describe("getServerApiUrl", () => {
  it("prefers INTERNAL_API_URL, then BACKEND_INTERNAL_URL", () => {
    expect(
      getServerApiUrl({
        INTERNAL_API_URL: "http://backend:8080/api/v1/",
        BACKEND_INTERNAL_URL: "http://other:8080",
      }),
    ).toBe("http://backend:8080/api/v1");
    expect(
      getServerApiUrl({ BACKEND_INTERNAL_URL: "http://backend:8080/" }),
    ).toBe("http://backend:8080/api/v1");
  });

  it("uses an absolute API_URL, else PUBLIC_URL + /api/v1", () => {
    expect(
      getServerApiUrl({ API_URL: "https://api.example.test/api/v1" }),
    ).toBe("https://api.example.test/api/v1");
    expect(
      getServerApiUrl(
        { PUBLIC_URL: "https://pos.example.test", API_URL: "/api/v1" },
        "production",
      ),
    ).toBe("https://pos.example.test/api/v1");
  });

  it("only hairpins through PUBLIC_URL in production", () => {
    expect(
      getServerApiUrl(
        { PUBLIC_URL: "https://pos.example.test" },
        "development",
      ),
    ).toBe("");
  });

  it("returns empty when nothing is configured", () => {
    expect(getServerApiUrl({})).toBe("");
  });
});

describe("browser serialization", () => {
  it("serializes exactly the whitelist and nothing else", () => {
    process.env.DB_PASSWORD = "do-not-leak-db";
    process.env.JWT_SECRET_KEY = "do-not-leak-jwt";
    process.env.RPC_URL = "https://base.example.test/v2/do-not-leak-rpc";
    process.env.SENTRY_DSN = "https://do-not-leak@o1.ingest.sentry.io/1";
    process.env.PUBLIC_URL = "https://pos.example.test";

    const script = serializePublicEnvScript();
    expect(script.startsWith("window.__PAYVERGE_ENV__=Object.freeze(")).toBe(
      true,
    );
    expect(script).not.toContain("do-not-leak");

    const readySuffix =
      ');if(window.dispatchEvent)window.dispatchEvent(new Event("payverge:public-env"));';
    expect(script.endsWith(readySuffix)).toBe(true);
    const json = script.slice(
      "window.__PAYVERGE_ENV__=Object.freeze(".length,
      -readySuffix.length,
    );
    const parsed = JSON.parse(json) as Record<string, string>;
    expect(Object.keys(parsed).sort()).toEqual([...PUBLIC_ENV_KEYS].sort());
    expect(parsed.PUBLIC_URL).toBe("https://pos.example.test");
  });

  it("publishes PUBLIC_URL blank when it is unset or invalid", () => {
    const published = () => {
      const script = serializePublicEnvScript();
      const start = "window.__PAYVERGE_ENV__=Object.freeze(".length;
      const end = script.indexOf(");if(window.dispatchEvent)");
      return (JSON.parse(script.slice(start, end)) as Record<string, string>)
        .PUBLIC_URL;
    };
    expect(published()).toBe("");
    process.env.PUBLIC_URL = "javascript:alert(1)";
    expect(published()).toBe("");
    process.env.PUBLIC_URL = "https://pos.example.test/";
    expect(published()).toBe("https://pos.example.test");
  });

  it("escapes values that could close the script element", () => {
    const hostile = resolvePublicEnv({}, "production", {
      includeBuildFallback: false,
    });
    const script = serializePublicEnvScript({
      ...hostile,
      VAPID_PUBLIC_KEY: "</script><script>alert(1)</script>&\u2028\u2029",
    });
    expect(script).not.toContain("</script");
    expect(script).not.toContain("<script");
    expect(script).not.toContain("&");
    expect(script).not.toContain("\u2028");
    expect(script).not.toContain("\u2029");
    // Still valid JS that round-trips the exact value.
    const sandbox: { window: { __PAYVERGE_ENV__?: Record<string, string> } } = {
      window: {},
    };
    vm.runInNewContext(script, sandbox);
    expect(sandbox.window.__PAYVERGE_ENV__?.VAPID_PUBLIC_KEY).toBe(
      "</script><script>alert(1)</script>&\u2028\u2029",
    );
  });

  it("announces itself once the config object is in place", () => {
    const seen: Array<{ type: string; key: string | undefined }> = [];
    const sandbox: {
      window: {
        __PAYVERGE_ENV__?: Record<string, string>;
        dispatchEvent?: (event: { type: string }) => boolean;
      };
      Event: typeof Event;
    } = {
      window: {},
      Event: class {
        constructor(public type: string) {}
      } as unknown as typeof Event,
    };
    sandbox.window.dispatchEvent = (event) => {
      seen.push({
        type: event.type,
        key: sandbox.window.__PAYVERGE_ENV__?.PUBLIC_URL,
      });
      return true;
    };
    vm.runInNewContext(
      serializePublicEnvScript(
        resolvePublicEnv(
          { PUBLIC_URL: "https://pos.example.test" },
          "production",
          {
            includeBuildFallback: false,
          },
        ),
      ),
      sandbox,
    );
    expect(seen).toEqual([
      { type: "payverge:public-env", key: "https://pos.example.test" },
    ]);
  });

  it("escapeJsonForScript leaves plain JSON untouched", () => {
    expect(escapeJsonForScript('{"a":"b"}')).toBe('{"a":"b"}');
  });

  it("toPublicConfig maps env keys to typed fields", () => {
    const config = toPublicConfig(
      resolvePublicEnv(
        { PUBLIC_URL: "https://pos.example.test", MAINTENANCE_MODE: "true" },
        "production",
        { includeBuildFallback: false },
      ),
    );
    expect(config.publicUrl).toBe("https://pos.example.test");
    expect(config.apiUrl).toBe("/api/v1");
    expect(config.maintenanceMode).toBe(true);
    expect(config.network).toBe("baseSepolia");
  });
});
