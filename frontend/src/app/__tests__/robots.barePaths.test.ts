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

function disallowFor(userAgent: string): string[] {
  const cfg = robots();
  const list = Array.isArray(cfg.rules) ? cfg.rules : [cfg.rules];
  const rule = list.find((r) => r.userAgent === userAgent);
  expect(rule).toBeDefined();
  const d = rule!.disallow;
  return Array.isArray(d) ? (d as string[]) : [d as string];
}

describe("robots — bare /admin and /dashboard (#940)", () => {
  it.each(["*", "Googlebot"])(
    "disallows the exact /admin and /dashboard URLs for %s",
    (ua) => {
      const disallow = disallowFor(ua);
      expect(disallow).toContain("/admin");
      expect(disallow).toContain("/dashboard");
    },
  );

  it("does not rely on /* suffixes that miss the exact paths", () => {
    const wildcard = disallowFor("*");
    expect(wildcard).not.toContain("/admin/*");
    expect(wildcard).not.toContain("/dashboard/*");
  });
});
