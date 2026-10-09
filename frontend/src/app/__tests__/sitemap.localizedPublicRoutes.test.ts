/** @jest-environment node */

jest.mock("@/config/serverConfig", () => ({
  ...jest.requireActual("@/config/serverConfig"),
  isSeoIndexingEnabled: () => true,
}));
jest.mock("@/lib/instance/serverHome", () => ({
  getServerHome: async () => null,
}));

/**
 * #722 — the sitemap lists the English signup URL but
 * neither their `/es` + `/es-ar` siblings nor an hreflang cluster on the
 * English entry.
 *
 * Those locale URLs are real: middleware rewrites `/es/<route>` to the English
 * base for every entry in EDGE_PUBLIC_PREFIXES, which includes it.
 *
 * Drives the real `sitemap()` export — only the storefront fetch is stubbed so
 * the assertion is about the static route block.
 */
export {};

const realFetch = global.fetch;

const LOCALIZED_PUBLIC_ROUTES = ["/business/register"];

// The legal pages render the noindex generic template, so listing them would
// advertise URLs the page itself tells crawlers to drop.
const NOINDEX_LEGAL_ROUTES = [
  "/terms-and-conditions",
  "/privacy-policy",
  "/refund",
];

beforeEach(() => {
  // No API URL configured → fetchStorefrontSlugs short-circuits to [] and the
  // sitemap degrades to its static block, which is what we assert on.
  global.fetch = jest.fn().mockResolvedValue({
    ok: false,
    json: async () => ({}),
  }) as unknown as typeof fetch;
});

afterEach(() => {
  global.fetch = realFetch;
  jest.resetModules();
  jest.clearAllMocks();
});

async function loadEntries() {
  const { default: sitemap } = await import("../sitemap");
  return sitemap();
}

describe("sitemap covers localized public routes (#722)", () => {
  it("emits /es and /es-ar <loc> entries for the register page", async () => {
    const urls = (await loadEntries()).map((e) => e.url);

    for (const route of LOCALIZED_PUBLIC_ROUTES) {
      expect(urls).toContain(`https://payverge.io${route}`);
      expect(urls).toContain(`https://payverge.io/es${route}`);
      expect(urls).toContain(`https://payverge.io/es-ar${route}`);
    }
  });

  it("gives every entry in the cluster the reciprocal en/es/es-AR hreflang set", async () => {
    const entries = await loadEntries();
    const byUrl = new Map(entries.map((e) => [e.url, e]));

    for (const route of LOCALIZED_PUBLIC_ROUTES) {
      const expected = {
        en: `https://payverge.io${route}`,
        es: `https://payverge.io/es${route}`,
        "es-AR": `https://payverge.io/es-ar${route}`,
        "x-default": `https://payverge.io${route}`,
      };

      for (const prefix of ["", "/es", "/es-ar"]) {
        const entry = byUrl.get(`https://payverge.io${prefix}${route}`);
        expect(entry?.alternates?.languages).toEqual(expected);
      }
    }
  });

  it("omits the noindex legal template pages", async () => {
    const urls = (await loadEntries()).map((e) => e.url);
    for (const route of NOINDEX_LEGAL_ROUTES) {
      expect(urls.some((u) => u.endsWith(route))).toBe(false);
    }
  });

  it("does not duplicate any sitemap URL", async () => {
    const urls = (await loadEntries()).map((e) => e.url);

    expect(urls.length).toBe(new Set(urls).size);
  });
});
