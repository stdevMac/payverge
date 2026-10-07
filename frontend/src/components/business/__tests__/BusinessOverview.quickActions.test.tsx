/** @jest-environment jsdom */
import React from "react";
import { render, screen, waitFor } from "@testing-library/react";
import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import BusinessOverview, {
  inventoryAlertDescriptionKey,
} from "../BusinessOverview";

// BusinessOverview reads the menu-item count via React Query now — wrap renders.
const renderWithQuery = (ui: React.ReactElement) => {
  const client = new QueryClient({
    defaultOptions: { queries: { retry: false, gcTime: 0 } },
  });
  return render(
    <QueryClientProvider client={client}>{ui}</QueryClientProvider>,
  );
};
import { businessApi, getMenu } from "@/api/business";
import { analyticsApi } from "@/api/analytics";
import { inventoryApi } from "@/api/inventory";
import { pluginAPI } from "@/api/plugins";
import { useBusinessAccess } from "@/hooks/useBusinessAccess";
import { useProactiveInsights } from "@/hooks/useProactiveInsights";

jest.mock("@/hooks/useBusinessAccess", () => ({
  useBusinessAccess: jest.fn(),
}));

jest.mock("@/hooks/useProactiveInsights", () => ({
  useProactiveInsights: jest.fn(),
}));

jest.mock("@/i18n/SimpleTranslationProvider", () => {
  const actual = jest.requireActual("@/i18n/getTranslation");
  return {
    useSimpleLocale: () => ({ locale: "en", setLocale: jest.fn() }),
    getTranslation: actual.getTranslation,
  };
});

jest.mock("@/api/business", () => ({
  businessApi: {
    getBusinessTables: jest.fn(),
  },
  getMenu: jest.fn(),
}));

jest.mock("@/api/analytics", () => ({
  analyticsApi: {
    getDashboardSummary: jest.fn(),
  },
}));

jest.mock("@/api/inventory", () => ({
  inventoryApi: {
    getSummary: jest.fn(),
  },
}));

jest.mock("@/api/plugins", () => ({
  pluginAPI: {
    protected: { getAllPlugins: jest.fn() },
    business: { getBusinessPlugins: jest.fn() },
  },
}));

const business = {
  id: 1,
  name: "Trattoria Bella Vista",
  onboarding_completed_at: "2026-01-01T00:00:00Z",
} as any;

describe("BusinessOverview — storefront quick action", () => {
  it.each([
    [0, "descriptionOutZero"],
    [1, "descriptionOutOne"],
    [2, "descriptionOutMany"],
  ])(
    "selects the inventory alert plural key for %s depleted items",
    (out, key) => {
      expect(inventoryAlertDescriptionKey(out)).toBe(key);
    },
  );

  beforeEach(() => {
    jest.clearAllMocks();
    (useBusinessAccess as jest.Mock).mockReturnValue({
      access: null,
      hasAccess: true,
      isSuspended: false,
      loading: false,
      aiConfigured: false,
    });
    (useProactiveInsights as jest.Mock).mockReturnValue({
      insights: [],
      loading: false,
    });
    (analyticsApi.getDashboardSummary as jest.Mock).mockResolvedValue({
      live: { active_bills: 0 },
    });
    (businessApi.getBusinessTables as jest.Mock).mockResolvedValue({
      tables: [{ id: 1, name: "Table 1", table_code: "T1" }],
    });
    (getMenu as jest.Mock).mockResolvedValue({
      categories: [{ items: [{ id: 1, name: "Item" }] }],
    });
    (inventoryApi.getSummary as jest.Mock).mockResolvedValue(null);
    // Plugin status fetch never resolves — pluginsLoaded stays false so the
    // usdc/cross-chain quick actions don't crowd out the evergreen slots.
    (pluginAPI.protected.getAllPlugins as jest.Mock).mockReturnValue(
      new Promise(() => {}),
    );
    (pluginAPI.business.getBusinessPlugins as jest.Mock).mockReturnValue(
      new Promise(() => {}),
    );
  });

  it("renders the renamed, clarified storefront quick action", async () => {
    renderWithQuery(<BusinessOverview business={business} />);

    await waitFor(() =>
      expect(screen.getByText("Share your storefront")).toBeInTheDocument(),
    );
    expect(
      screen.getByText(
        "Your public page at payverge.io/b/… — preview it and put the link everywhere.",
      ),
    ).toBeInTheDocument();
    expect(
      screen.queryByText("Polish your business page"),
    ).not.toBeInTheDocument();
  });

  it("names the deployment's own host in the storefront quick action", async () => {
    const w = window as unknown as { __PAYVERGE_ENV__?: Record<string, string> };
    const previous = w.__PAYVERGE_ENV__;
    w.__PAYVERGE_ENV__ = { PUBLIC_URL: "https://pos.example.test" };
    try {
      renderWithQuery(<BusinessOverview business={business} />);
      await waitFor(() =>
        expect(
          screen.getByText(
            "Your public page at pos.example.test/b/… — preview it and put the link everywhere.",
          ),
        ).toBeInTheDocument(),
      );
    } finally {
      w.__PAYVERGE_ENV__ = previous;
    }
  });

  it("leads with setup (print QR) not cross-chain when tables already exist (Task 33)", async () => {
    // Tables + menu already set up → first action is print QR / setup, never
    // "Enable Supported Cross-Chain Payments".
    renderWithQuery(<BusinessOverview business={business} />);

    await waitFor(() =>
      expect(screen.getByText("Print table QR codes")).toBeInTheDocument(),
    );
    const list = screen.getByRole("heading", {
      name: /quick actions/i,
    }).parentElement;
    const buttons = list?.querySelectorAll("button") ?? [];
    expect(buttons.length).toBeGreaterThan(0);
    expect(buttons[0].textContent).toMatch(/Print table QR codes/i);
    expect(buttons[0].textContent).not.toMatch(/Cross-Chain/i);
  });

  it("offers Enable card payments before USDC when only crypto is enabled", async () => {
    (pluginAPI.protected.getAllPlugins as jest.Mock).mockResolvedValue({
      plugins: [
        { id: 1, name: "usdc_payment" },
        { id: 2, name: "mercadopago" },
      ],
    });
    (pluginAPI.business.getBusinessPlugins as jest.Mock).mockResolvedValue({
      plugins: [
        { plugin_id: 1, is_enabled: true },
        { plugin_id: 2, is_enabled: false },
      ],
    });

    renderWithQuery(<BusinessOverview business={business} />);

    await waitFor(() =>
      expect(screen.getByText("Enable card payments")).toBeInTheDocument(),
    );
    expect(screen.queryByText("Enable USDC Payments")).not.toBeInTheDocument();
    expect(
      screen.queryByText("Enable Supported Cross-Chain Payments"),
    ).not.toBeInTheDocument();
  });

  it("does not treat a coming-soon cross-chain subscription as a working payment rail", async () => {
    // The catalog marks cross_chain_payment coming-soon while guests cannot
    // settle through it; an old enabled subscription must not make the
    // overview claim the venue is ready to accept payments.
    (pluginAPI.protected.getAllPlugins as jest.Mock).mockResolvedValue({
      plugins: [
        {
          id: 1,
          name: "cross_chain_payment",
          is_active: true,
          coming_soon: true,
        },
        { id: 2, name: "mercadopago", is_active: true, coming_soon: false },
      ],
    });
    (pluginAPI.business.getBusinessPlugins as jest.Mock).mockResolvedValue({
      plugins: [
        { plugin_id: 1, is_enabled: true },
        { plugin_id: 2, is_enabled: false },
      ],
    });

    renderWithQuery(<BusinessOverview business={business} />);

    await waitFor(() =>
      expect(screen.getByText("Enable card payments")).toBeInTheDocument(),
    );
    expect(
      screen.getByText(/Connect a payment method in Payments & apps/),
    ).toBeInTheDocument();
    expect(
      screen.queryByText(/Your business is ready to accept payments/),
    ).not.toBeInTheDocument();
    expect(
      screen.queryByText("Enable Supported Cross-Chain Payments"),
    ).not.toBeInTheDocument();
  });
});
