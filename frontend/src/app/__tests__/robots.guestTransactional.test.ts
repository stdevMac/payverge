/** @jest-environment node */
// PG-2: guest transactional table pages (/t/*) must not be indexed by Googlebot.
// The wildcard UA already disallows /t/*; Googlebot had a narrower list that
// omitted it, leaving table codes crawlable while other guest paths were not.
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

function rulesFor(userAgent: string) {
  const cfg = robots();
  const list = Array.isArray(cfg.rules) ? cfg.rules : [cfg.rules];
  return list.find((r) => r.userAgent === userAgent);
}

function disallowList(userAgent: string): string[] {
  const rule = rulesFor(userAgent);
  expect(rule).toBeDefined();
  const d = rule!.disallow;
  return Array.isArray(d) ? (d as string[]) : [d as string];
}

describe("robots — guest transactional paths consistent (PG-2)", () => {
  it("disallows /t/* for the wildcard user-agent", () => {
    expect(disallowList("*")).toContain("/t/*");
  });

  it("disallows /t/* for Googlebot (parity with wildcard)", () => {
    expect(disallowList("Googlebot")).toContain("/t/*");
  });

  it("keeps /b/* crawlable for SEO on both agents (storefront is public marketing)", () => {
    expect(disallowList("*")).not.toContain("/b/*");
    expect(disallowList("Googlebot")).not.toContain("/b/*");
  });

  it("still blocks auth-only surfaces for Googlebot", () => {
    const disallow = disallowList("Googlebot");
    expect(disallow).toContain("/admin");
    expect(disallow).toContain("/dashboard");
    expect(disallow).toContain("/api/*");
  });
});
