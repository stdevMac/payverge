/** @jest-environment node */
import robots from "../robots";

// These rules describe an indexable deployment; a default install is
// `Disallow: /` (see robots.runtimeConfig.test.ts).
const savedSeoIndexing = process.env.SEO_INDEXING;
beforeAll(() => {
  process.env.SEO_INDEXING = "true";
});
afterAll(() => {
  if (savedSeoIndexing === undefined) delete process.env.SEO_INDEXING;
  else process.env.SEO_INDEXING = savedSeoIndexing;
});

describe("robots — allow /b/ for all crawlers (SEO-1)", () => {
  let cfg: ReturnType<typeof robots>;
  let wildcard: Exclude<ReturnType<typeof robots>["rules"], unknown[]>;
  beforeAll(() => {
    cfg = robots();
    wildcard = (Array.isArray(cfg.rules) ? cfg.rules : [cfg.rules]).find(
      (r) => r.userAgent === "*",
    )!;
  });

  it("does NOT disallow /b/* for the wildcard user-agent", () => {
    const disallow = Array.isArray(wildcard.disallow)
      ? wildcard.disallow
      : [wildcard.disallow];
    expect(disallow).not.toContain("/b/*");
  });

  it("still references the sitemap", () => {
    expect(cfg.sitemap).toBe("https://payverge.io/sitemap.xml");
  });

  it("still blocks auth-only surfaces", () => {
    const disallow = Array.isArray(wildcard.disallow)
      ? wildcard.disallow
      : [wildcard.disallow];
    expect(disallow).toContain("/admin");
    expect(disallow).toContain("/dashboard");
  });
});
