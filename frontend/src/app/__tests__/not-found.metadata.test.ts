/**
 * #721 — the 404 template must own a localized title and a 404-shaped social
 * card.
 *
 * `not-found.tsx` ships a hardcoded English `export const metadata` with only
 * `title` + `robots`, so:
 *   - `/es/legal` renders a Spanish body under `<title>Page not found — Payverge</title>`
 *   - no page-level `openGraph` / `twitter` block exists, so the root layout's
 *     homepage product card (`Payverge - AI-Powered Restaurant Management
 *     System` / `Payverge - AI-Driven Restaurant Operations`) is what gets
 *     shared for a dead URL.
 *
 * The real `generateMetadata` export is exercised here — only `next/headers`
 * is stubbed so the locale header can be driven.
 */
export {};

const mockHeaders = jest.fn();

jest.mock("next/headers", () => ({
  headers: () => mockHeaders(),
}));

const ROOT_OG_TITLE = "Payverge - AI-Powered Restaurant Management System";
const ROOT_TWITTER_TITLE = "Payverge - AI-Driven Restaurant Operations";

describe("not-found generateMetadata is locale-aware (#721)", () => {
  beforeEach(() => {
    mockHeaders.mockReset();
    jest.resetModules();
  });

  async function loadMetadata(localeHeader: string | null) {
    mockHeaders.mockResolvedValue({
      get: (name: string) =>
        name === "x-payverge-locale" ? localeHeader : null,
    });
    const mod = await import("../not-found");
    if (typeof mod.generateMetadata !== "function") {
      throw new Error(
        "not-found.tsx exports no generateMetadata — the 404 title cannot follow the request locale",
      );
    }
    return mod.generateMetadata();
  }

  it("keeps the English title and stays noindex by default", async () => {
    const meta = await loadMetadata(null);

    expect(meta.title).toBe("Page not found — Payverge");
    expect(meta.robots).toEqual({ index: false, follow: false });
  });

  it("localizes the document title for es", async () => {
    const meta = await loadMetadata("es");

    expect(meta.title).toBe("Página no encontrada — Payverge");
  });

  it("localizes the document title for es-AR", async () => {
    const meta = await loadMetadata("es-AR");

    expect(meta.title).toBe("Página no encontrada — Payverge");
    expect(meta.openGraph?.locale).toBe("es_AR");
  });

  it("owns a 404 social card instead of leaking the homepage product card", async () => {
    const en = await loadMetadata("en");

    expect(en.openGraph?.title).toBe("Page not found — Payverge");
    expect(en.twitter?.title).toBe("Page not found — Payverge");
    expect(en.openGraph?.title).not.toBe(ROOT_OG_TITLE);
    expect(en.twitter?.title).not.toBe(ROOT_TWITTER_TITLE);

    const es = await loadMetadata("es");

    expect(es.openGraph?.title).toBe("Página no encontrada — Payverge");
    expect(es.twitter?.title).toBe("Página no encontrada — Payverge");
    expect(es.openGraph?.locale).toBe("es_ES");
  });

  it("does not advertise the homepage as the 404 canonical", async () => {
    const meta = await loadMetadata("es");

    expect(meta.alternates?.canonical).toBeUndefined();
    expect(meta.openGraph?.url).toBeUndefined();
  });
});
