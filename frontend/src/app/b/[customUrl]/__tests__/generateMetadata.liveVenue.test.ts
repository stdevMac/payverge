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

jest.mock("../BusinessPageClient", () => ({
  __esModule: true,
  default: jest.fn(() => null),
}));

jest.mock("@/components/customer/CustomerAuthShell", () => ({
  children,
}: {
  children: unknown;
}) => children);

import BusinessPage, { generateMetadata } from "../page";
import BusinessPageClient from "../BusinessPageClient";
import {
  clearStorefrontBusinessSsrCache,
  fetchStorefrontBusiness,
} from "@/lib/storefront/serverData";

const SLUG = "payverge-ai-pro-demo-lounge";
const LOUNGE = "Payverge AI Pro Demo Lounge";
const mockedClient = BusinessPageClient as unknown as jest.Mock;

const realFetch = global.fetch;
const originalPublic = process.env.API_URL;

afterEach(() => {
  global.fetch = realFetch;
  if (originalPublic === undefined) delete process.env.API_URL;
  else process.env.API_URL = originalPublic;
  mockedClient.mockClear();
  clearStorefrontBusinessSsrCache();
});

function loungePayload() {
  return {
    id: 86,
    name: LOUNGE,
    custom_url: SLUG,
    description: "AI-powered hospitality demo lounge",
    is_demo: true,
    kind: "demo",
  };
}

function okResponse(body: unknown) {
  return {
    status: 200,
    ok: true,
    json: async () => body,
  };
}

function failResponse(status = 503) {
  return {
    status,
    ok: false,
    json: async () => ({}),
  };
}

function walkElements(
  node: unknown,
  visit: (el: { type: unknown; props: Record<string, unknown> }) => void,
): void {
  if (node == null || typeof node !== "object") return;
  if (Array.isArray(node)) {
    for (const child of node) walkElements(child, visit);
    return;
  }
  const el = node as { type?: unknown; props?: { children?: unknown } };
  if (!("type" in el)) return;
  visit(el as { type: unknown; props: Record<string, unknown> });
  walkElements(el.props?.children, visit);
}

function clientPropsFromPage(pageNode: unknown): {
  initialBusiness?: { name?: string };
  initialReason?: string;
} | undefined {
  let found:
    | { initialBusiness?: { name?: string }; initialReason?: string }
    | undefined;
  walkElements(pageNode, (el) => {
    if (el.type === mockedClient) {
      found = el.props as typeof found;
    }
  });
  return found;
}

describe("live venue SSR after a later fetch stampede (#685)", () => {
  it("does not emit Loading business… once the lounge has been seen", async () => {
    process.env.API_URL = "http://api.test/api/v1";
    global.fetch = jest
      .fn()
      .mockResolvedValueOnce(okResponse(loungePayload()))
      .mockResolvedValue(failResponse(503)) as unknown as typeof fetch;

    const first = await fetchStorefrontBusiness(SLUG);
    expect(first.business?.name).toBe(LOUNGE);

    const [meta, pageNode] = await Promise.all([
      generateMetadata({ params: Promise.resolve({ customUrl: SLUG }) }),
      BusinessPage({ params: Promise.resolve({ customUrl: SLUG }) }),
    ]);

    expect(pageNode).toBeTruthy();
    expect(String(meta.title)).not.toMatch(/Loading business/);
    expect(String(meta.title)).toContain(LOUNGE);
    expect(meta.robots).not.toEqual(
      expect.objectContaining({ index: false, follow: false }),
    );

    const props = clientPropsFromPage(pageNode);
    expect(props?.initialBusiness?.name).toBe(LOUNGE);
    expect(props?.initialReason).toBeUndefined();
  });

  it("drops last-known-good on a confirmed 404 unpublish", async () => {
    process.env.API_URL = "http://api.test/api/v1";
    global.fetch = jest
      .fn()
      .mockResolvedValueOnce(okResponse(loungePayload()))
      .mockResolvedValue(failResponse(404)) as unknown as typeof fetch;

    const first = await fetchStorefrontBusiness(SLUG);
    expect(first.business?.name).toBe(LOUNGE);

    const meta = await generateMetadata({
      params: Promise.resolve({ customUrl: SLUG }),
    });
    expect(String(meta.title)).toMatch(/Business Not Found/i);
    expect(String(meta.title)).not.toContain(LOUNGE);
    expect(String(meta.title)).not.toMatch(/Loading business/);

    await expect(
      BusinessPage({ params: Promise.resolve({ customUrl: SLUG }) }),
    ).rejects.toThrow("NEXT_NOT_FOUND");
  });
});
