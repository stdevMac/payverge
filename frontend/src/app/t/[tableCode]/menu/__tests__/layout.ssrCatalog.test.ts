/** @jest-environment node */
import React from "react";
import { renderToStaticMarkup } from "react-dom/server";

const mockGetServerApiUrl = jest.fn(() => "http://api.test/api/v1");
jest.mock("@/lib/serverApiUrl", () => ({
  getServerApiUrl: () => mockGetServerApiUrl(),
}));

jest.mock("next/navigation", () => ({
  notFound: jest.fn(() => {
    throw new Error("NEXT_NOT_FOUND");
  }),
}));

const realFetch = global.fetch;

afterEach(() => {
  global.fetch = realFetch;
  mockGetServerApiUrl.mockReset();
  mockGetServerApiUrl.mockReturnValue("http://api.test/api/v1");
  jest.clearAllMocks();
});

describe("MenuLayout SSR catalog", () => {
  it("embeds dish names in the first HTML paint", async () => {
    global.fetch = jest.fn().mockResolvedValue({
      ok: true,
      status: 200,
      json: async () => ({
        business: { name: "Payverge AI Pro Demo Lounge" },
        categories: [
          {
            name: "Mains",
            items: [{ name: "Harvest Bowl" }, { name: "Steak Plate" }],
          },
        ],
        bundles: [{ name: "Date Night for Two" }],
      }),
    }) as unknown as typeof fetch;

    const { default: MenuLayout } = await import("../layout");
    const tree = await MenuLayout({
      children: null,
      params: Promise.resolve({ tableCode: "M03Y18GB3P" }),
    });
    const html = renderToStaticMarkup(tree as React.ReactElement);
    expect(html).toContain("Harvest Bowl");
    expect(html).toContain("Steak Plate");
    expect(html).toContain("Date Night for Two");
    expect(html).toContain("guest-menu-ssr-catalog");
  });
});
