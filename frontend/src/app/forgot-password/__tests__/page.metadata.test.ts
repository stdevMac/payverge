export {};

const mockHeaders = jest.fn();

jest.mock("next/headers", () => ({
  headers: () => mockHeaders(),
}));

describe("forgot-password generateMetadata (#404)", () => {
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
    expect(meta.title).toBe("Forgot password — Payverge");
  });

  it("uses Spanish document title when locale is es", async () => {
    const meta = await loadMetadata("es");
    expect(meta.title).toBe("Olvidaste tu contraseña — Payverge");
  });

  it("uses Argentine Spanish document title when locale is es-AR", async () => {
    const meta = await loadMetadata("es-AR");
    expect(meta.title).toBe("Olvidaste tu contraseña — Payverge");
  });
});
