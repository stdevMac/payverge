/** @jest-environment node */
// The instance root ("/") in the sitemap: the primary venue is listed once at
// "/" (its canonical) and never again at /b/<slug>; a directory lists "/"; an
// empty instance (root redirects to /dashboard) lists no root entry.
import type { HomeInfo } from "@/lib/instance/serverHome";

jest.mock("@/config/serverConfig", () => ({
  ...jest.requireActual("@/config/serverConfig"),
  isSeoIndexingEnabled: () => true,
}));

let mockHome: HomeInfo | null = null;
jest.mock("@/lib/instance/serverHome", () => ({
  getServerHome: async () => mockHome,
}));

const realFetch = global.fetch;
const prevApiUrl = process.env.API_URL;

beforeEach(() => {
  process.env.API_URL = "https://api.payverge.io/api/v1";
  global.fetch = jest.fn().mockResolvedValue({
    ok: true,
    json: async () => ({
      storefronts: [{ custom_url: "alpha" }, { custom_url: "beta" }],
    }),
  }) as unknown as typeof fetch;
});

afterEach(() => {
  global.fetch = realFetch;
  if (prevApiUrl === undefined) delete process.env.API_URL;
  else process.env.API_URL = prevApiUrl;
  mockHome = null;
  jest.resetModules();
});

async function loadEntries() {
  const { default: sitemap } = await import("../sitemap");
  return sitemap();
}

const venue = (slug: string) => ({
  id: 1,
  name: slug,
  logo: "",
  custom_url: slug,
  city: "",
});

describe("sitemap instance root", () => {
  it("lists the primary venue at / with its hreflang cluster and drops /b/<slug>", async () => {
    mockHome = { mode: "venue", primary: venue("Alpha"), venues: [] };
    const entries = await loadEntries();
    const urls = entries.map((e) => e.url);

    const root = entries.find((e) => e.url === "https://payverge.io/");
    expect(root?.priority).toBe(1);
    expect(root?.alternates?.languages).toMatchObject({
      "x-default": "https://payverge.io/",
      es: "https://payverge.io/es",
    });
    expect(urls).not.toContain("https://payverge.io/b/alpha");
    expect(urls).toContain("https://payverge.io/b/beta");
  });

  it("lists / for a venue directory and keeps every /b/<slug>", async () => {
    mockHome = {
      mode: "directory",
      primary: null,
      venues: [venue("alpha"), venue("beta")],
    };
    const urls = (await loadEntries()).map((e) => e.url);

    expect(urls).toContain("https://payverge.io/");
    expect(urls).toContain("https://payverge.io/b/alpha");
    expect(urls).toContain("https://payverge.io/b/beta");
  });

  it.each([
    ["empty", { mode: "empty", primary: null, venues: [] } as HomeInfo],
    ["unreachable", null],
  ])("omits / when the home is %s", async (_label, home) => {
    mockHome = home;
    const urls = (await loadEntries()).map((e) => e.url);
    expect(urls).not.toContain("https://payverge.io/");
  });
});
