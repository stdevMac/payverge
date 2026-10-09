/** @jest-environment node */
// A private-by-default install (SEO_INDEXING unset) disallows crawling in
// robots.txt, so the sitemap must not enumerate storefront slugs either.
import sitemap from "../sitemap";

jest.mock("@/lib/instance/serverHome", () => ({
  getServerHome: async () => null,
}));

const realFetch = global.fetch;
const saved = { SEO_INDEXING: process.env.SEO_INDEXING, API_URL: process.env.API_URL };

afterEach(() => {
  global.fetch = realFetch;
  for (const [key, value] of Object.entries(saved)) {
    if (value === undefined) delete process.env[key];
    else process.env[key] = value;
  }
});

it("is empty while SEO indexing is off, even with storefronts", async () => {
  delete process.env.SEO_INDEXING;
  process.env.API_URL = "http://backend.test/api/v1";
  const fetchMock = jest.fn().mockResolvedValue({
    ok: true,
    json: async () => ({ storefronts: [{ custom_url: "bodegon-mesa-larga", kind: "demo", is_demo: true }] }),
  });
  global.fetch = fetchMock as never;
  expect(await sitemap()).toEqual([]);
  expect(fetchMock).not.toHaveBeenCalled();
});

it("lists routes once SEO_INDEXING=true", async () => {
  process.env.SEO_INDEXING = "true";
  global.fetch = jest.fn().mockResolvedValue({ ok: false }) as never;
  expect((await sitemap()).length).toBeGreaterThan(0);
});
