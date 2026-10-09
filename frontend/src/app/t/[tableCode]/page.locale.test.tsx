/** @jest-environment node */

jest.mock("next/dynamic", () => () => () => null);
jest.mock("@/lib/serverApiUrl", () => ({
  getServerApiUrl: () => "http://api.test/api/v1",
}));
jest.mock("next/headers", () => ({
  cookies: async () => ({ get: () => undefined }),
  headers: async () => ({ get: () => null }),
}));

const realFetch = global.fetch;

afterEach(() => {
  global.fetch = realFetch;
  jest.clearAllMocks();
});

function mockTableFetch() {
  global.fetch = jest.fn().mockResolvedValue({
    ok: true,
    status: 200,
    json: async () => ({ business: { name: "Mesa Sur" } }),
  }) as unknown as typeof fetch;
}

describe("table-link server locale bootstrap", () => {
  it("seeds Arabic into the initial provider for ?lang=ar", async () => {
    mockTableFetch();
    const { default: TablePage } = await import("./page");
    const result = await TablePage({
      params: Promise.resolve({ tableCode: "TABLE-42" }),
      searchParams: Promise.resolve({ lang: "ar" }),
    });

    expect(result.props.initialLanguage).toBe("ar");
    expect(result.props.preferInitialLanguage).toBe(true);
  });

  it("ships Spanish messages in the server render instead of waiting for an effect", async () => {
    mockTableFetch();
    const { default: TablePage } = await import("./page");
    const result = await TablePage({
      params: Promise.resolve({ tableCode: "TABLE-42" }),
      searchParams: Promise.resolve({ lang: "es" }),
    });

    expect(result.props.initialLanguage).toBe("es");
    expect(result.props.initialMessages.common.cancel).toBe("Cancelar");
  });

  it("falls back to English for an invalid locale", async () => {
    mockTableFetch();
    const { default: TablePage } = await import("./page");
    const result = await TablePage({
      params: Promise.resolve({ tableCode: "TABLE-42" }),
      searchParams: Promise.resolve({ lang: "not-a-locale" }),
    });

    expect(result.props.initialLanguage).toBe("en");
    expect(result.props.preferInitialLanguage).toBe(false);
  });
});
