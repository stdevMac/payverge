/** @jest-environment node */
// "/" serves the venue: the primary (or only) published storefront, a
// directory when several are published, or /dashboard when none is.
import type { HomeInfo } from "@/lib/instance/serverHome";

let mockHome: HomeInfo | null = null;
jest.mock("@/lib/instance/serverHome", () => ({
  getServerHome: async () => mockHome,
}));
jest.mock("@/lib/instance/serverInstance", () => ({
  getServerInstanceInfo: async () => null,
}));
jest.mock("@/i18n/guestLocale.server", () => ({
  resolveGuestLocaleFromRequest: async () => ({ locale: "en" }),
}));
const mockStorefrontMetadata = jest.fn(async () => ({ title: "Alpha" }));
const mockStorefrontView = jest.fn(() => "storefront");
jest.mock("@/app/b/[customUrl]/storefrontRender", () => ({
  storefrontMetadata: (...a: unknown[]) => mockStorefrontMetadata(...(a as [])),
  StorefrontView: (...a: unknown[]) => mockStorefrontView(...(a as [])),
}));
const mockRedirect = jest.fn((url: string) => {
  throw new Error(`NEXT_REDIRECT ${url}`);
});
jest.mock("next/navigation", () => ({
  redirect: (url: string) => mockRedirect(url),
}));

import HomePage, { dynamic, generateMetadata } from "../page";
import VenueDirectory from "@/components/venue-directory/VenueDirectory";
import { getSiteUrl } from "@/config/publicConfig";

const venue = (slug: string, id = 1) => ({
  id,
  name: slug,
  logo: "",
  custom_url: slug,
  city: "",
});

afterEach(() => {
  mockHome = null;
  jest.clearAllMocks();
});

describe("instance root page", () => {
  it("renders per request", () => {
    expect(dynamic).toBe("force-dynamic");
  });

  it("renders the primary venue's storefront at /", async () => {
    mockHome = { mode: "venue", primary: venue("alpha"), venues: [] };
    const sp = { lang: "es" };
    await expect(HomePage({ searchParams: Promise.resolve(sp) })).resolves.toBe(
      "storefront",
    );
    expect(mockStorefrontView).toHaveBeenCalledWith({
      customUrl: "alpha",
      searchParams: sp,
      pagePath: "/",
    });
    await generateMetadata({ searchParams: Promise.resolve(sp) });
    expect(mockStorefrontMetadata).toHaveBeenCalledWith({
      customUrl: "alpha",
      searchParams: sp,
      pagePath: "/",
    });
  });

  it("renders an indexable directory when several venues are published", async () => {
    mockHome = {
      mode: "directory",
      primary: null,
      venues: [venue("alpha", 1), venue("beta", 2)],
    };
    const el = (await HomePage({})) as React.ReactElement<{
      venues: unknown[];
    }>;
    expect(el.type).toBe(VenueDirectory);
    expect(el.props.venues).toHaveLength(2);
    const meta = await generateMetadata({});
    expect(meta.robots).toBeUndefined();
    expect(meta.alternates?.canonical).toBe(`${getSiteUrl()}/`);
    // R2-1: the directory owns its social card under the instance name.
    expect(meta.openGraph?.title).toBe("Payverge");
    expect(meta.openGraph?.siteName).toBe("Payverge");
    expect(String(meta.openGraph?.title)).not.toMatch(/Restaurant Management/);
  });

  it.each([
    ["no venue is published", { mode: "empty", primary: null, venues: [] } as HomeInfo],
    ["the backend is unreachable", null],
  ])("redirects to /dashboard when %s, keeping the query", async (_l, home) => {
    mockHome = home;
    await expect(
      HomePage({ searchParams: Promise.resolve({ invite_code: "abc" }) }),
    ).rejects.toThrow("NEXT_REDIRECT");
    expect(mockRedirect).toHaveBeenCalledWith("/dashboard?invite_code=abc");
    const meta = await generateMetadata({});
    expect(meta.robots).toEqual({ index: false, follow: false });
  });
});
