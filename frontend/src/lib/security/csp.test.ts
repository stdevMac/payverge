import { resolvePublicEnv, toPublicConfig } from "@/config/publicConfig";

import {
  buildContentSecurityPolicy,
  contentSecurityPolicyFor,
  cspOrigin,
  parseMediaOrigins,
} from "./csp";

function config(env: Record<string, string>) {
  return toPublicConfig(
    resolvePublicEnv(env, "production", { includeBuildFallback: false }),
  );
}

function directive(csp: string, name: string): string {
  const found = csp
    .split(";")
    .map((part) => part.trim())
    .find((part) => part.startsWith(`${name} `));
  return found ?? "";
}

const NONCE = "bm9uY2UtMTIz";

describe("buildContentSecurityPolicy", () => {
  it("is same-origin only for a default self-hosted install", () => {
    const csp = buildContentSecurityPolicy({
      nonce: NONCE,
      dev: false,
      config: config({ PUBLIC_URL: "https://pos.example.test" }),
      cloudflareInsights: false,
    });
    expect(directive(csp, "default-src")).toBe("default-src 'self'");
    expect(directive(csp, "script-src")).toBe(
      `script-src 'self' 'nonce-${NONCE}' 'strict-dynamic'`,
    );
    expect(directive(csp, "connect-src")).toBe(
      "connect-src 'self' https://sepolia.base.org",
    );
    expect(csp).not.toMatch(/payverge\.io/);
    expect(csp).not.toMatch(/cloudflareinsights/);
    expect(csp).not.toMatch(/posthog/);
    expect(csp).not.toMatch(/sentry/);
    expect(csp).not.toContain("'unsafe-eval'");
    expect(csp).toContain("frame-ancestors 'none'");
    expect(csp).toContain("object-src 'none'");
  });

  it("adds a split-origin API to connect-src and img-src", () => {
    const csp = buildContentSecurityPolicy({
      nonce: NONCE,
      dev: false,
      config: config({
        PUBLIC_URL: "https://pos.example.test",
        API_URL: "https://api.pos.example.test/api/v1",
      }),
      cloudflareInsights: false,
    });
    expect(directive(csp, "connect-src")).toContain(
      "https://api.pos.example.test",
    );
    expect(directive(csp, "img-src")).toContain("https://api.pos.example.test");
  });

  it("allows both loopback spellings for a local backend", () => {
    const csp = buildContentSecurityPolicy({
      nonce: NONCE,
      dev: true,
      config: config({ API_URL: "http://localhost:8080/api/v1" }),
      cloudflareInsights: false,
    });
    const connect = directive(csp, "connect-src");
    expect(connect).toContain("http://localhost:8080");
    expect(connect).toContain("http://127.0.0.1:8080");
    expect(directive(csp, "script-src")).toContain("'unsafe-eval'");
  });

  it("adds MEDIA_ORIGINS to img-src, dropping anything unsafe", () => {
    const csp = buildContentSecurityPolicy({
      nonce: NONCE,
      dev: false,
      config: config({
        MEDIA_ORIGINS:
          "https://bucket.s3.example.test, cdn.example.test, http://plain.example.test, https://u:p@creds.example.test, 'unsafe-inline'",
      }),
      cloudflareInsights: false,
    });
    const img = directive(csp, "img-src");
    expect(img).toContain("https://bucket.s3.example.test");
    expect(img).toContain("https://cdn.example.test");
    expect(img).toContain("https://images.unsplash.com");
    expect(img).toContain("https://lh3.googleusercontent.com");
    expect(img).not.toContain("plain.example.test");
    expect(img).not.toContain("creds.example.test");
    expect(img).not.toContain("'unsafe-inline'");
  });

  it("adds the LOGO_URL host to img-src only, and only when it is a clean URL", () => {
    const base = {
      nonce: NONCE,
      dev: false,
      config: config({}),
      cloudflareInsights: false,
    };
    const csp = buildContentSecurityPolicy({
      ...base,
      logoUrl: "https://cdn.example.test/brand/logo.svg",
    });
    expect(directive(csp, "img-src")).toContain("https://cdn.example.test");
    expect(directive(csp, "connect-src")).not.toContain("cdn.example.test");
    for (const logoUrl of ["/images/logo.png", "javascript:alert(1)", ""]) {
      const img = directive(
        buildContentSecurityPolicy({ ...base, logoUrl }),
        "img-src",
      );
      expect(img).not.toContain("javascript");
      expect(img).toBe(directive(buildContentSecurityPolicy(base), "img-src"));
    }
  });

  it("adds Sentry, PostHog and RPC hosts only when configured", () => {
    const csp = buildContentSecurityPolicy({
      nonce: NONCE,
      dev: false,
      config: config({
        FRONTEND_SENTRY_DSN: "https://publickey@o42.ingest.sentry.io/7",
        POSTHOG_HOST: "https://eu.i.posthog.com",
        PUBLIC_RPC_URL: "https://rpc.example.test/v1/abc",
        NETWORK: "base",
      }),
      cloudflareInsights: false,
      rpcUrls: ["https://rpc.example.test/v1/abc", "https://sepolia.base.org"],
    });
    const connect = directive(csp, "connect-src");
    expect(connect).toContain("https://o42.ingest.sentry.io");
    expect(connect).not.toContain("publickey");
    expect(connect).toContain("https://eu.i.posthog.com");
    expect(connect).toContain("https://rpc.example.test");
    expect(connect).not.toContain("/v1/abc");
    expect(connect).toContain("https://sepolia.base.org");
  });

  it("allows Cloudflare Insights only when enabled", () => {
    const csp = buildContentSecurityPolicy({
      nonce: NONCE,
      dev: false,
      config: config({}),
      cloudflareInsights: true,
    });
    expect(directive(csp, "script-src")).toContain(
      "https://static.cloudflareinsights.com",
    );
    expect(directive(csp, "connect-src")).toContain(
      "https://cloudflareinsights.com",
    );
  });

  it("rejects a nonce that could break out of the header", () => {
    expect(() =>
      buildContentSecurityPolicy({
        nonce: "abc'; script-src *",
        dev: false,
        config: config({}),
        cloudflareInsights: false,
      }),
    ).toThrow();
  });
});

describe("helpers", () => {
  it("cspOrigin keeps only the origin and rejects relative or plain-http public URLs", () => {
    expect(cspOrigin("https://api.example.test/api/v1?x=1")).toBe(
      "https://api.example.test",
    );
    expect(cspOrigin("/api/v1")).toBeNull();
    expect(cspOrigin("http://api.example.test")).toBeNull();
    expect(cspOrigin("http://127.0.0.1:8080/api/v1")).toBe(
      "http://127.0.0.1:8080",
    );
    expect(cspOrigin("javascript:alert(1)")).toBeNull();
    // LAN installs: a split-origin backend on a private address is allowed.
    expect(cspOrigin("http://192.168.1.20:8080/api/v1")).toBe(
      "http://192.168.1.20:8080",
    );
  });

  it("parseMediaOrigins de-duplicates and normalizes", () => {
    expect(
      parseMediaOrigins(
        "cdn.example.test, https://cdn.example.test/, https://x.example.test:8443",
      ),
    ).toEqual(["https://cdn.example.test", "https://x.example.test:8443"]);
    expect(parseMediaOrigins(undefined)).toEqual([]);
  });
});

describe("contentSecurityPolicyFor (runtime env)", () => {
  const KEYS = ["MEDIA_ORIGINS", "CLOUDFLARE_INSIGHTS", "EDGE", "API_URL"];
  const saved: Record<string, string | undefined> = {};
  beforeEach(() => {
    for (const key of KEYS) {
      saved[key] = process.env[key];
      delete process.env[key];
    }
  });
  afterEach(() => {
    for (const key of KEYS) {
      if (saved[key] === undefined) delete process.env[key];
      else process.env[key] = saved[key];
    }
  });

  it("re-reads MEDIA_ORIGINS and the Cloudflare switch per call", () => {
    expect(contentSecurityPolicyFor(NONCE)).not.toContain(
      "bucket.example.test",
    );
    process.env.MEDIA_ORIGINS = "https://bucket.example.test";
    process.env.EDGE = "cloudflare";
    const csp = contentSecurityPolicyFor(NONCE);
    expect(csp).toContain("https://bucket.example.test");
    expect(csp).toContain("https://static.cloudflareinsights.com");
    process.env.CLOUDFLARE_INSIGHTS = "false";
    expect(contentSecurityPolicyFor(NONCE)).not.toContain("cloudflareinsights");
  });

  it("covers the RPC hosts for both supported chains", () => {
    const connect = directive(contentSecurityPolicyFor(NONCE), "connect-src");
    expect(connect).toContain("https://mainnet.base.org");
    expect(connect).toContain("https://sepolia.base.org");
  });
});
