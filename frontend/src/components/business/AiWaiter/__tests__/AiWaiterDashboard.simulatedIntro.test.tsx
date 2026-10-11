/** @jest-environment jsdom */
import React from "react";
import { fireEvent, render, screen, waitFor } from "@testing-library/react";

jest.mock("react-hot-toast", () => { const e = jest.fn(); const s = jest.fn(); const f = Object.assign(jest.fn(), { error: e, success: s }); return { __esModule: true, default: f, toast: f }; });
// Interpolate {param} placeholders so we can assert real menu-item names land
// in the composed simulated-intro line (the shared harness mock drops params).
jest.mock("@/i18n/SimpleTranslationProvider", () => ({
  useSimpleLocale: () => ({ locale: "en", setLocale: jest.fn() }),
  getTranslation: (key: string, _locale: string, params?: Record<string, string | number>) =>
    params ? key + " " + Object.values(params).join(" ") : key,
}));
jest.mock("@/hooks/useBusinessAccess", () => ({ useBusinessAccess: () => ({ hasAccess: true, isSuspended: false, aiConfigured: true, loading: false }) }));
jest.mock("@/api/tools/instance", () => {
  const m = { axiosInstance: { get: jest.fn(), post: jest.fn(), put: jest.fn() } };
  (m.axiosInstance.get as jest.Mock).mockImplementation((url: string) => {
    if (url.includes("/conversations?")) return Promise.resolve({ data: { conversations: [], total_pages: 1 } });
    if (url.includes("/insights")) return Promise.resolve({ data: { total_conversations: 0, total_messages: 0 } });
    return Promise.resolve({ data: [] });
  });
  return m;
});
jest.mock("@/providers/HybridAuthProvider", () => ({ useAuth: () => ({ staffData: null }) }));

const mockGetMenu = jest.fn();
jest.mock("@/api/business", () => ({ __esModule: true, getMenu: (...a: unknown[]) => mockGetMenu(...a) }));

import AiWaiterDashboard from "../AiWaiterDashboard";

const business = {
  id: 1,
  name: "Cafe",
  address: {},
  settlement_address: "",
  tipping_address: "",
  tax_rate: 0,
  service_fee_rate: 0,
  tax_inclusive: false,
  service_inclusive: false,
  is_active: true,
  business_page_enabled: true,
  ai_settings: {
    ai_enabled: true,
    ai_name: "Sage",
    ai_priority: "upselling",
    special_instructions: "",
    business_page_ai_enabled: false,
  },
} as any;

describe("AiWaiterDashboard simulated intro", () => {
  beforeEach(() => jest.clearAllMocks());

  it("composes the upsell intro from this business's real menu items (not hardcoded placeholders)", async () => {
    mockGetMenu.mockResolvedValue({
      categories: [
        { name: "Mains", items: [
          { name: "Grilled Salmon", is_available: true },
          { name: "Veggie Bowl", is_available: true },
          { name: "Sold Out Special", is_available: false },
        ] },
      ],
    });

    render(<AiWaiterDashboard business={business} onUpdateBusiness={jest.fn()} />);
    const overviewTab = await screen.findByRole("tab", { name: /tabs\.overview/i });
    fireEvent.click(overviewTab);

    await waitFor(() => {
      expect(screen.getByText(/Grilled Salmon/)).toBeInTheDocument();
    });
    expect(screen.getByText(/Veggie Bowl/)).toBeInTheDocument();
    // The old hardcoded placeholders must not appear.
    expect(screen.queryByText(/Truffle Fries/)).not.toBeInTheDocument();
    expect(screen.queryByText(/Mojito/)).not.toBeInTheDocument();
  });

  it("falls back to generic, non-fictional copy when the menu is empty", async () => {
    mockGetMenu.mockResolvedValue({ categories: [] });

    render(<AiWaiterDashboard business={business} onUpdateBusiness={jest.fn()} />);
    const overviewTab = await screen.findByRole("tab", { name: /tabs\.overview/i });
    fireEvent.click(overviewTab);

    await waitFor(() => {
      expect(
        screen.getByText(/settings\.introTemplates\.upsellingGeneric/),
      ).toBeInTheDocument();
    });
    expect(screen.queryByText(/Truffle Fries/)).not.toBeInTheDocument();
  });
});
