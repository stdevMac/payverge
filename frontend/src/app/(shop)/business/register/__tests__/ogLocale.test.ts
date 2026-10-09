/**
 * #809 — /business/register emitted no og:locale on any locale while home/
 * features/pricing do (#687). Declare it per request locale. #911: localized
 * paths self-canonicalize instead of pointing at the English wizard.
 */
import { headers } from "next/headers";

jest.mock("next/headers", () => ({
  headers: jest.fn(),
}));

const mockHeaders = headers as jest.MockedFunction<typeof headers>;

function headerMap(locale: string | null) {
  return {
    get: (name: string) => (name === "x-payverge-locale" ? locale : null),
  } as Awaited<ReturnType<typeof headers>>;
}

describe("business/register og:locale (#809)", () => {
  beforeEach(() => {
    mockHeaders.mockReset();
  });

  it.each([
    ["en", "en_US"],
    ["es", "es_ES"],
    ["es-AR", "es_AR"],
  ])("locale %s emits og:locale %s", async (header, ogLocale) => {
    mockHeaders.mockResolvedValue(headerMap(header));
    const { generateMetadata } = await import("../layout");
    const meta = await generateMetadata();
    const og = meta.openGraph as { locale?: string };
    expect(og.locale).toBe(ogLocale);
  });

  it("self-canonicalizes og:url on /es/business/register (#911)", async () => {
    mockHeaders.mockResolvedValue(headerMap("es"));
    const { generateMetadata } = await import("../layout");
    const meta = await generateMetadata();
    expect(String(meta.alternates?.canonical)).toBe(
      "https://payverge.io/es/business/register",
    );
    const og = meta.openGraph as { url?: string };
    expect(String(og.url)).toBe("https://payverge.io/es/business/register");
    const twitter = meta.twitter as { title?: string } | undefined;
    expect(String(twitter?.title)).toBe(String(meta.title));
  });
});
