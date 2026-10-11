/** @jest-environment node */

// Module scope: without a top-level import/export TS treats this file as a
// global script and its top-level consts collide with page.locale.test.tsx.
export {};

const mockCookieGet = jest.fn();
const mockHeaderGet = jest.fn();

jest.mock("next/headers", () => ({
  cookies: async () => ({ get: mockCookieGet }),
  headers: async () => ({ get: mockHeaderGet }),
}));

describe("scan generateMetadata (#910)", () => {
  beforeEach(() => {
    mockCookieGet.mockReset();
    mockHeaderGet.mockReset();
    mockCookieGet.mockReturnValue(undefined);
    mockHeaderGet.mockReturnValue(null);
    jest.resetModules();
  });

  async function loadMetadata(localeHeader: string | null) {
    mockHeaderGet.mockImplementation((name: string) =>
      name === "x-payverge-locale" ? localeHeader : null,
    );
    const mod = await import("./page");
    return mod.generateMetadata();
  }

  it("keeps the English scan title by default", async () => {
    const meta = await loadMetadata(null);
    expect(String(meta.title)).toMatch(/scan/i);
    expect(String(meta.title)).toContain("Payverge");
    expect(meta.robots).toEqual({ index: false, follow: true });
  });

  it("localizes /es/scan document title (#910)", async () => {
    const meta = await loadMetadata("es");
    expect(String(meta.title)).not.toBe("Scan table code — Payverge");
    expect(String(meta.title).toLowerCase()).toMatch(/escanear|escanea|mesa/);
    expect(String(meta.title)).toContain("Payverge");
  });

  it("localizes /es-ar/scan document title (#910)", async () => {
    const meta = await loadMetadata("es-AR");
    expect(String(meta.title)).not.toBe("Scan table code — Payverge");
    expect(String(meta.title).toLowerCase()).toMatch(/escanear|escanea|mesa/);
  });
});
