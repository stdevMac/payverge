/** @jest-environment node */
// robots.txt is rendered per request from runtime env: one image, any origin,
// and a fresh install is not crawlable until SEO_INDEXING=true.
import robots, { dynamic } from "../robots";

const saved = {
  PUBLIC_URL: process.env.PUBLIC_URL,
  SEO_INDEXING: process.env.SEO_INDEXING,
};

afterEach(() => {
  for (const [key, value] of Object.entries(saved)) {
    if (value === undefined) delete process.env[key];
    else process.env[key] = value;
  }
});

describe("robots runtime configuration", () => {
  it("is rendered per request, never frozen at build time", () => {
    expect(dynamic).toBe("force-dynamic");
  });

  it.each([undefined, "", "false", "0", "nope"])(
    "disallows everything when SEO_INDEXING=%p",
    (value) => {
      if (value === undefined) delete process.env.SEO_INDEXING;
      else process.env.SEO_INDEXING = value;
      const cfg = robots();
      expect(cfg.rules).toEqual([{ userAgent: "*", disallow: "/" }]);
      expect(cfg.sitemap).toBeUndefined();
      expect(cfg.host).toBeUndefined();
    },
  );

  it.each(["https://a.example.test", "https://b.example.test:8443"])(
    "points sitemap and host at PUBLIC_URL=%s when indexing is enabled",
    (origin) => {
      process.env.SEO_INDEXING = "true";
      process.env.PUBLIC_URL = `${origin}/`;
      const cfg = robots();
      expect(cfg.sitemap).toBe(`${origin}/sitemap.xml`);
      expect(cfg.host).toBe(origin);
      expect(JSON.stringify(cfg)).not.toContain("payverge.io");
    },
  );
});
