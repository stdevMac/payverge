/** @jest-environment node */

jest.mock("@/config/serverConfig", () => ({
  ...jest.requireActual("@/config/serverConfig"),
  isSeoIndexingEnabled: () => true,
}));
jest.mock("@/lib/instance/serverHome", () => ({
  getServerHome: async () => null,
}));

// CRITICAL: `sitemap.ts` reads `const API_URL = process.env.API_URL || ""`
// at MODULE SCOPE (mirroring page.tsx:11). Jest does NOT load API_URL,
// so a static top-of-file `import sitemap from "../sitemap"` would freeze API_URL = ""
// and `fetchStorefrontSlugs` would short-circuit (returns []), making the success
// test assert zero /b/ entries and FAIL. So: set the env var FIRST, then
// `await import("../sitemap")` inside each test (dynamic import re-evaluates the
// module against the set env). Reset in afterEach — mirrors the env-set/reset idiom
// in src/app/admin/stripe/page.test.tsx:80-87.

// Keeps this file a module. Without a top-level import/export, TS treats it as a
// global script and `realFetch` collides with the same name in sibling tests.
export {};

const realFetch = global.fetch;
const prevApiUrl = process.env.API_URL;
const prevInternalApiUrl = process.env.INTERNAL_API_URL;

afterEach(() => {
  global.fetch = realFetch;
  if (prevApiUrl === undefined) {
    delete (process.env as Record<string, string>).API_URL;
  } else {
    process.env.API_URL = prevApiUrl;
  }
  if (prevInternalApiUrl === undefined) {
    delete (process.env as Record<string, string>).INTERNAL_API_URL;
  } else {
    process.env.INTERNAL_API_URL = prevInternalApiUrl;
  }
  jest.resetModules();
  jest.clearAllMocks();
});

describe("sitemap — storefront slugs (SEO-2)", () => {
  it("includes published storefront /b/<slug> URLs from the endpoint", async () => {
    process.env.API_URL = "https://api.payverge.io/api/v1";
    global.fetch = jest.fn().mockResolvedValue({
      ok: true,
      json: async () => ({
        storefronts: [
          { custom_url: "alpha", updated_at: "2026-06-01T00:00:00Z" },
          { custom_url: "beta", updated_at: "2026-06-10T00:00:00Z" },
        ],
      }),
    }) as unknown as typeof fetch;

    // Dynamic import AFTER the env is set so module-scope API_URL is non-empty.
    const { default: sitemap } = await import("../sitemap");
    const entries = await sitemap();
    const urls = entries.map((e) => e.url);
    expect(global.fetch).toHaveBeenCalledWith(
      "https://api.payverge.io/api/v1/business/storefronts",
      expect.anything(),
    );
    expect(urls).toContain("https://payverge.io/b/alpha");
    expect(urls).toContain("https://payverge.io/b/beta");
  });

  it("emits hreflang alternates for storefront entries (SEO-0.4)", async () => {
    process.env.API_URL = "https://api.payverge.io/api/v1";
    global.fetch = jest.fn().mockResolvedValue({
      ok: true,
      json: async () => ({
        storefronts: [{ custom_url: "alpha", updated_at: "2026-06-01T00:00:00Z" }],
      }),
    }) as unknown as typeof fetch;

    const { default: sitemap } = await import("../sitemap");
    const entries = await sitemap();
    const alpha = entries.find((e) => e.url === "https://payverge.io/b/alpha");
    expect(alpha).toBeDefined();
    const languages = alpha?.alternates?.languages as Record<string, string>;
    // Mirrors /b/[customUrl] generateMetadata: ?lang=<code> + lang-less x-default.
    expect(languages["x-default"]).toBe("https://payverge.io/b/alpha");
    expect(languages.en).toBe("https://payverge.io/b/alpha?lang=en");
    expect(languages.es).toBe("https://payverge.io/es/b/alpha");
    expect(languages["es-AR"]).toBe("https://payverge.io/es-ar/b/alpha");
  });

  it("includes published demo showrooms and excludes kind=test (#612)", async () => {
    process.env.API_URL = "https://api.payverge.io/api/v1";
    global.fetch = jest.fn().mockResolvedValue({
      ok: true,
      json: async () => ({
        storefronts: [
          { custom_url: "real-bistro", updated_at: "2026-06-01T00:00:00Z" },
          {
            custom_url: "payverge-ai-pro-demo-lounge",
            updated_at: "2026-06-01T00:00:00Z",
            is_demo: true,
            kind: "demo",
          },
          {
            custom_url: "ci-test-kitchen",
            updated_at: "2026-06-01T00:00:00Z",
            kind: "test",
          },
        ],
      }),
    }) as unknown as typeof fetch;

    const { default: sitemap } = await import("../sitemap");
    const entries = await sitemap();
    const urls = entries.map((e) => e.url);
    expect(urls).toContain("https://payverge.io/b/real-bistro");
    expect(urls).toContain(
      "https://payverge.io/b/payverge-ai-pro-demo-lounge",
    );
    expect(urls).not.toContain("https://payverge.io/b/ci-test-kitchen");
  });

  it("uses INTERNAL_API_URL when API_URL is empty (#612)", async () => {
    process.env.API_URL = "";
    process.env.INTERNAL_API_URL = "http://backend:8080/api/v1";
    global.fetch = jest.fn().mockResolvedValue({
      ok: true,
      json: async () => ({
        storefronts: [
          { custom_url: "payverge-ai-pro-demo-lounge", updated_at: "2026-06-01T00:00:00Z" },
        ],
      }),
    }) as unknown as typeof fetch;

    const { default: sitemap } = await import("../sitemap");
    const entries = await sitemap();
    expect(global.fetch).toHaveBeenCalledWith(
      "http://backend:8080/api/v1/business/storefronts",
      expect.anything(),
    );
    expect(entries.map((e) => e.url)).toContain(
      "https://payverge.io/b/payverge-ai-pro-demo-lounge",
    );
    delete process.env.INTERNAL_API_URL;
  });

  it("degrades gracefully (no throw, still returns static routes) when the fetch fails", async () => {
    process.env.API_URL = "https://api.payverge.io/api/v1";
    global.fetch = jest
      .fn()
      .mockRejectedValue(new Error("network down")) as unknown as typeof fetch;
    const { default: sitemap } = await import("../sitemap");
    const entries = await sitemap();
    // Static routes still present.
    expect(entries.some((e) => e.url === "https://payverge.io/business/register")).toBe(
      true,
    );
    // No /b/ entries when the source failed (no silent garbage).
    expect(
      entries.some((e) => e.url.startsWith("https://payverge.io/b/")),
    ).toBe(false);
  });

  it("keeps static routes when the storefront endpoint returns non-success", async () => {
    process.env.API_URL = "https://api.payverge.io/api/v1";
    global.fetch = jest.fn().mockResolvedValue({
      ok: false,
      status: 503,
    }) as unknown as typeof fetch;
    const { default: sitemap } = await import("../sitemap");
    const entries = await sitemap();

    expect(entries.some((e) => e.url === "https://payverge.io/business/register")).toBe(
      true,
    );
    expect(
      entries.some((e) => e.url.startsWith("https://payverge.io/b/")),
    ).toBe(false);
  });

  it("keeps static routes when the storefront payload is malformed", async () => {
    process.env.API_URL = "https://api.payverge.io/api/v1";
    global.fetch = jest.fn().mockResolvedValue({
      ok: true,
      json: async () => ({ storefronts: "not-an-array" }),
    }) as unknown as typeof fetch;
    const { default: sitemap } = await import("../sitemap");
    const entries = await sitemap();

    expect(entries.some((e) => e.url === "https://payverge.io/business/register")).toBe(
      true,
    );
    expect(
      entries.some((e) => e.url.startsWith("https://payverge.io/b/")),
    ).toBe(false);
  });

  // The marketing site is gone: no pricing / blog / contact / intake URLs,
  // in any locale, and the static block never probes src/ at runtime.
  it("emits no marketing-site URLs and no fs probes", async () => {
    process.env.API_URL = "";
    const { default: sitemap } = await import("../sitemap");
    const entries = await sitemap();
    const urls = entries.map((e) => e.url);

    for (const gone of [
      "pricing",
      "features",
      "how-it-works",
      "ai-features",
      "mission",
      "blog",
      "book-demo",
      "contact",
      "custom-intake",
      "tools",
    ]) {
      expect(urls.some((u) => new RegExp(`/${gone}(?:/|$)`).test(u))).toBe(
        false,
      );
    }

    // Guard: sitemap source must not probe src/ at runtime.
    const fs = require("node:fs") as typeof import("node:fs");
    const path = require("node:path") as typeof import("node:path");
    const src = fs.readFileSync(
      path.join(__dirname, "../sitemap.ts"),
      "utf8",
    );
    // Production bug: runtime fs.existsSync on src/app always false in Docker.
    // Guard the import surface, not comment prose.
    expect(src).not.toMatch(/^import fs from ['"]node:fs['"]/m);
    expect(src).not.toMatch(/fs\.existsSync\(/);
  });
});
