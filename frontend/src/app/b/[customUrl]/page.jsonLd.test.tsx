/** @jest-environment node */

jest.mock("next/headers", () => ({
  cookies: async () => ({ get: () => undefined }),
  headers: async () => ({ get: () => null }),
}));

jest.mock("next/navigation", () => ({
  notFound: jest.fn(() => {
    throw new Error("NEXT_NOT_FOUND");
  }),
  permanentRedirect: jest.fn(() => {
    throw new Error("NEXT_REDIRECT");
  }),
}));

jest.mock("./BusinessPageClient", () => () => null);
jest.mock("@/components/customer/CustomerAuthShell", () => ({ children }: { children: unknown }) =>
  children,
);

export {};

const realFetch = global.fetch;

afterEach(() => {
  global.fetch = realFetch;
  jest.clearAllMocks();
});

function collectJsonLd(node: unknown): Array<Record<string, unknown>> {
  const out: Array<Record<string, unknown>> = [];
  const walk = (value: unknown) => {
    if (!value || typeof value !== "object") return;
    if (Array.isArray(value)) {
      value.forEach(walk);
      return;
    }
    const el = value as { props?: { type?: string; dangerouslySetInnerHTML?: { __html?: string }; children?: unknown } };
    if (el.props?.type === "application/ld+json" && el.props.dangerouslySetInnerHTML?.__html) {
      out.push(JSON.parse(el.props.dangerouslySetInnerHTML.__html) as Record<string, unknown>);
    }
    if (el.props?.children) walk(el.props.children);
  };
  walk(node);
  return out;
}

describe("storefront Restaurant JSON-LD (#637)", () => {
  it("emits Restaurant schema for a published demo showroom", async () => {
    global.fetch = jest.fn().mockImplementation(async (url: unknown) => {
      const u = String(url);
      if (u.includes("/menu")) {
        return { status: 200, ok: true, json: async () => ({ parsed_categories: [] }) };
      }
      return {
        status: 200,
        ok: true,
        json: async () => ({
          id: 9,
          name: "Payverge AI Pro Demo Lounge",
          description: "Neighborhood tasting room",
          custom_url: "payverge-ai-pro-demo-lounge",
          is_demo: true,
          kind: "demo",
          business_page_enabled: true,
          is_active: true,
        }),
      };
    }) as unknown as typeof fetch;

    const { default: BusinessPage } = await import("./page");
    const tree = await BusinessPage({
      params: Promise.resolve({ customUrl: "payverge-ai-pro-demo-lounge" }),
    });
    const blocks = collectJsonLd(tree);
    expect(blocks.some((block) => block["@type"] === "Restaurant")).toBe(true);
  });
});
