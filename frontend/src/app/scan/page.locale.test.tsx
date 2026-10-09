/** @jest-environment node */
/**
 * #390: /scan must honor the diner's saved guest locale cookie before
 * Accept-Language, matching /t/... and /b/... SSR bootstrap (PG-21).
 */

const mockCookieGet = jest.fn();
const mockHeaderGet = jest.fn();

jest.mock("next/headers", () => ({
  cookies: async () => ({ get: mockCookieGet }),
  headers: async () => ({ get: mockHeaderGet }),
}));

beforeEach(() => {
  mockCookieGet.mockReset();
  mockHeaderGet.mockReset();
  mockCookieGet.mockReturnValue(undefined);
  mockHeaderGet.mockReturnValue(null);
});

describe("scan page guest locale bootstrap (#390)", () => {
  it("honors payverge_guest_locale before Accept-Language when ?lang= is absent", async () => {
    mockCookieGet.mockImplementation((name: string) =>
      name === "payverge_guest_locale" ? { value: "es" } : undefined,
    );
    mockHeaderGet.mockImplementation((name: string) =>
      name === "accept-language" ? "en-US,en;q=0.9" : null,
    );

    const { default: ScanPage } = await import("./page");
    const result = await ScanPage({ searchParams: Promise.resolve({}) });

    expect(result.props.initialLanguage).toBe("es");
    expect(result.props.preferInitialLanguage).toBe(true);
    expect(result.props.initialMessages.scan.header.title).toBe(
      "Accede a tu mesa",
    );
  });

  it("prefers ?lang= over the guest cookie", async () => {
    mockCookieGet.mockImplementation((name: string) =>
      name === "payverge_guest_locale" ? { value: "ja" } : undefined,
    );

    const { default: ScanPage } = await import("./page");
    const result = await ScanPage({
      searchParams: Promise.resolve({ lang: "es" }),
    });

    expect(result.props.initialLanguage).toBe("es");
    expect(result.props.preferInitialLanguage).toBe(true);
  });

  it("ignores the operator payverge_locale cookie", async () => {
    mockCookieGet.mockImplementation((name: string) =>
      name === "payverge_locale" ? { value: "es" } : undefined,
    );
    mockHeaderGet.mockImplementation((name: string) =>
      name === "accept-language" ? "en-US,en;q=0.9" : null,
    );

    const { default: ScanPage } = await import("./page");
    const result = await ScanPage({ searchParams: Promise.resolve({}) });

    expect(result.props.initialLanguage).toBe("en");
    expect(result.props.preferInitialLanguage).toBe(false);
  });
});
