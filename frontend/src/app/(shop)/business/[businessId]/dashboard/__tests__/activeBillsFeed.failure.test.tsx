/** @jest-environment jsdom */
import React from "react";
import { render, screen } from "@testing-library/react";
import { axiosInstance } from "@/api/tools/instance";
import { fetchActiveBillsFeed } from "../activeBillsFeed";
import { getTranslation } from "@/i18n/SimpleTranslationProvider";
import { BillManager } from "@/components/business/BillManager";

jest.mock("@/api/tools/instance", () => ({
  axiosInstance: {
    get: jest.fn(),
    post: jest.fn(),
    put: jest.fn(),
    patch: jest.fn(),
  },
}));

jest.mock("next/navigation", () => ({
  useSearchParams: () => ({ get: () => null }),
  useRouter: () => ({ push: jest.fn(), replace: jest.fn() }),
  usePathname: () => "/business/8/dashboard",
}));

jest.mock("@/i18n/SimpleTranslationProvider", () => {
  const actual = jest.requireActual("@/i18n/SimpleTranslationProvider");
  return {
    ...actual,
    useSimpleLocale: () => ({ locale: "es", setLocale: jest.fn() }),
  };
});

jest.mock("@/hooks/useBusinessAccess", () => ({
  useBusinessAccess: () => ({
    hasAccess: true,
    isSuspended: false,
    loading: false,
  }),
}));

jest.mock("react-hot-toast", () => ({
  __esModule: true,
  default: { error: jest.fn(), success: jest.fn() },
}));

jest.mock("@/components/business/BillCreator", () => ({
  BillCreator: () => null,
}));
jest.mock("@/components/business/BillDetailsModal", () => ({
  BillDetailsModal: () => null,
}));
jest.mock("@/components/business/BillFilters", () => ({
  BillFilters: () => null,
}));
jest.mock("@/components/business/PendingOrdersSection", () => ({
  PendingOrdersSection: () => null,
}));
jest.mock("@/components/business/KitchenOrdersToggle", () => ({
  KitchenOrdersToggle: () => <div>kitchen upsell</div>,
}));
jest.mock("@/components/business/BillDisplayMode", () => ({
  __esModule: true,
  default: () => null,
}));
jest.mock("@/components/business/BillsTable", () => ({
  BillsTable: () => null,
}));

const es = (key: string): string => {
  const value = getTranslation(`billManager.${key}`, "es");
  return Array.isArray(value) ? (value[0] ?? key) : (value as string);
};

const MONEY = /\d+[.,]\d{2}|US\s*\$|\$\s*\d/;

describe("rejected getAllActiveBills does not paint an empty till (#647)", () => {
  beforeEach(() => {
    (axiosInstance.get as jest.Mock).mockReset();
  });

  it("rejects the live /bills/open feed and withholds 0 Resultados / $0.00", async () => {
    (axiosInstance.get as jest.Mock).mockImplementation((url: string) => {
      const path = String(url);
      if (path.includes("/bills/open")) {
        return Promise.reject(new Error("network"));
      }
      if (path.includes("/bills")) {
        return Promise.resolve({
          data: { bills: [], total: 0, total_pages: 1 },
        });
      }
      return Promise.resolve({
        data: { default_currency: "USD", timezone: "UTC" },
      });
    });

    const feed = await fetchActiveBillsFeed(8);
    expect(feed.ok).toBe(false);
    expect(axiosInstance.get).toHaveBeenCalledWith(
      expect.stringContaining("/inside/businesses/8/bills/open"),
    );

    const { container } = render(
      <BillManager
        businessId={8}
        globalBills={feed.items}
        globalBillsLoaded
        globalBillsFailed={!feed.ok}
        globalOrders={{}}
        globalOrdersLoaded
        kitchenEnabled={false}
        kitchenStatusLoading={false}
        onKitchenStatusChange={jest.fn()}
        onOrderStatusChange={jest.fn()}
      />,
    );

    expect(
      await screen.findByTestId("bills-list-load-failed"),
    ).toBeInTheDocument();
    expect(
      screen.getByText(es("emptyState.listLoadFailed")),
    ).toBeInTheDocument();

    const rendered = container.textContent ?? "";
    expect(rendered).not.toContain(es("header.results"));
    expect(rendered).not.toContain(es("header.visibleValue"));
    expect(rendered).not.toMatch(/0\s+Resultados/);
    expect(rendered).not.toMatch(MONEY);
    expect(screen.queryByText("kitchen upsell")).not.toBeInTheDocument();
  });
});
