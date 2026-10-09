export {};

const mockHeaders = jest.fn();

jest.mock("next/headers", () => ({
  headers: () => mockHeaders(),
}));

describe("accept-invitation generateMetadata (#881)", () => {
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
    expect(meta.title).toBe("Accept invitation — Payverge");
    expect(meta.robots).toEqual({ index: false, follow: true });
  });

  it("translates the tab title for a Spanish staffer", async () => {
    const meta = await loadMetadata("es");
    expect(meta.title).toBe("Aceptar invitación — Payverge");
    expect(meta.description).toMatch(/^Acepta tu invitación/);
  });

  it("keeps the es-AR voseo description", async () => {
    const meta = await loadMetadata("es-ar");
    expect(meta.title).toBe("Aceptar invitación — Payverge");
    expect(meta.description).toMatch(/^Aceptá tu invitación/);
  });
});
