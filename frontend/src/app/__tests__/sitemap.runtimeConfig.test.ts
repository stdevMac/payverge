/** @jest-environment node */
// The sitemap is rendered per request from the runtime PUBLIC_URL: every <loc>,
// hreflang alternate and storefront URL follows the deployment's origin.
import sitemap, { dynamic } from "../sitemap";

jest.mock("@/config/serverConfig", () => ({
  ...jest.requireActual("@/config/serverConfig"),
  isSeoIndexingEnabled: () => true,
}));
jest.mock("@/lib/instance/serverHome", () => ({
  getServerHome: async () => null,
}));

const realFetch = global.fetch;
const saved = {
  PUBLIC_URL: process.env.PUBLIC_URL,
  API_URL: process.env.API_URL,
  INTERNAL_API_URL: process.env.INTERNAL_API_URL,
};

afterEach(() => {
  global.fetch = realFetch;
  for (const [key, value] of Object.entries(saved)) {
    if (value === undefined) delete process.env[key];
    else process.env[key] = value;
  }
});

function allUrls(entries: Awaited<ReturnType<typeof sitemap>>): string[] {
  return entries.flatMap((entry) => [
    entry.url,
    ...Object.values(entry.alternates?.languages ?? {}).map(String),
  ]);
}

describe("sitemap runtime configuration", () => {
  it("is rendered per request, never frozen at build time", () => {
    expect(dynamic).toBe("force-dynamic");
  });

  it.each(["https://a.example.test", "https://b.example.test:8443"])(
    "builds every URL from PUBLIC_URL=%s",
    async (origin) => {
      process.env.PUBLIC_URL = `${origin}/`;
      process.env.INTERNAL_API_URL = "http://backend.internal:8080/api/v1";
      global.fetch = jest.fn().mockResolvedValue({
        ok: true,
        json: async () => ({
          storefronts: [{ custom_url: "alpha", updated_at: "2026-06-01T00:00:00Z" }],
        }),
      }) as unknown as typeof fetch;

      const entries = await sitemap();
      const urls = allUrls(entries);

      expect(urls.length).toBeGreaterThan(10);
      expect(urls).toContain(`${origin}/business/register`);
      expect(urls).toContain(`${origin}/es-ar/business/register`);
      expect(urls).toContain(`${origin}/b/alpha`);
      for (const url of urls) {
        expect(url.startsWith(origin)).toBe(true);
      }
      expect(JSON.stringify(entries)).not.toContain("payverge.io");
    },
  );
});
