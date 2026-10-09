/**
 * MIN-2: /business/register document title must localize for es (and es-AR).
 */

export {};

const mockHeaders = jest.fn();

jest.mock("next/headers", () => ({
  headers: () => mockHeaders(),
}));

describe("business register layout metadata (MIN-2)", () => {
  beforeEach(() => {
    mockHeaders.mockReset();
    jest.resetModules();
  });

  async function loadMetadata(localeHeader: string | null) {
    mockHeaders.mockResolvedValue({
      get: (name: string) =>
        name === "x-payverge-locale" ? localeHeader : null,
    });
    const mod = await import("../layout");
    return mod.generateMetadata();
  }

  it("uses the English document title by default", async () => {
    const meta = await loadMetadata(null);
    expect(meta.title).toBe("Register your restaurant — Payverge");
    expect(typeof meta.description).toBe("string");
    expect((meta.description as string).length).toBeGreaterThan(20);
  });

  it("uses Spanish document title when locale is es", async () => {
    const meta = await loadMetadata("es");
    expect(meta.title).toBe("Registra tu restaurante — Payverge");
    expect(meta.openGraph?.title).toBe("Registra tu restaurante — Payverge");
    expect(meta.twitter?.title).toBe("Registra tu restaurante — Payverge");
  });

  it("uses Rioplatense document title when locale is es-ar", async () => {
    const meta = await loadMetadata("es-ar");
    expect(meta.title).toBe("Registrá tu restaurante — Payverge");
  });

  it("uses Rioplatense document title for middleware casing es-AR", async () => {
    const meta = await loadMetadata("es-AR");
    expect(meta.title).toBe("Registrá tu restaurante — Payverge");
  });


  it("self-canonicalizes /es/business/register with hreflang (#911)", async () => {
    const meta = await loadMetadata("es");
    expect(meta.alternates?.canonical).toBe(
      "https://payverge.io/es/business/register",
    );
    const languages = meta.alternates?.languages as Record<string, string>;
    expect(languages.en).toBe("https://payverge.io/business/register");
    expect(languages.es).toBe("https://payverge.io/es/business/register");
    expect(languages["es-AR"]).toBe(
      "https://payverge.io/es-ar/business/register",
    );
    expect(languages["x-default"]).toBe(
      "https://payverge.io/business/register",
    );
  });
});
