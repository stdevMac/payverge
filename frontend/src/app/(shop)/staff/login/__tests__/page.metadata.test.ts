export {};

const mockHeaders = jest.fn();

jest.mock("next/headers", () => ({
  headers: () => mockHeaders(),
}));

describe("staff login generateMetadata (#404)", () => {
  beforeEach(() => {
    mockHeaders.mockReset();
    jest.resetModules();
  });

  async function loadMetadata(localeHeader: string | null) {
    mockHeaders.mockResolvedValue({
      get: (name: string) =>
        name === "x-payverge-locale" ? localeHeader : null,
    });
    const mod = await import("../page");
    return mod.generateMetadata();
  }

  it("uses the English document title by default", async () => {
    const meta = await loadMetadata(null);
    expect(meta.title).toBe("Staff login — Payverge");
    expect(String(meta.openGraph?.url)).toBe("https://payverge.io/staff/login");
    expect(meta.openGraph?.title).toBe(meta.title);
    expect(meta.robots).toEqual({ index: false, follow: true });
    expect(String((meta.twitter as { title?: string }).title)).toBe(
      String(meta.title),
    );
  });

  it("uses Spanish document title when locale is es", async () => {
    const meta = await loadMetadata("es");
    expect(meta.title).toBe("Inicio de sesión del personal — Payverge");
    expect(String((meta.twitter as { title?: string }).title)).toBe(
      String(meta.title),
    );
    expect(String((meta.twitter as { title?: string }).title)).not.toBe(
      "Payverge - AI-Driven Restaurant Operations",
    );
  });

  it("uses Argentine Spanish document title when locale is es-ar", async () => {
    const meta = await loadMetadata("es-ar");
    expect(meta.title).toBe("Inicio de sesión del personal — Payverge");
    expect(String((meta.twitter as { title?: string }).title)).toBe(
      String(meta.title),
    );
  });

  it("self-canonicalizes localized staff login with hreflang (#884)", async () => {
    const meta = await loadMetadata("es");
    expect(String(meta.alternates?.canonical)).toBe(
      "https://payverge.io/es/staff/login",
    );
    expect(String(meta.openGraph?.url)).toBe(
      "https://payverge.io/es/staff/login",
    );
    const languages = meta.alternates?.languages as Record<string, string>;
    expect(languages.en).toBe("https://payverge.io/staff/login");
    expect(languages.es).toBe("https://payverge.io/es/staff/login");
    expect(languages["es-AR"]).toBe("https://payverge.io/es-ar/staff/login");
    expect(languages["x-default"]).toBe("https://payverge.io/staff/login");
  });
});
