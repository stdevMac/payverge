/** @jest-environment jsdom */
/**
 * L4-6 — Disabling AI Waiter must not orphan history: tabs + monitor remain
 * visible with a read-only banner; only write controls / guest runtime gate
 * on aiEnabled.
 */
import React from "react";
import { act, render, screen, waitFor } from "@testing-library/react";

jest.mock("react-hot-toast", () => {
  const toastFn = Object.assign(jest.fn(), {
    error: jest.fn(),
    success: jest.fn(),
  });
  return { __esModule: true, default: toastFn, toast: toastFn };
});

jest.mock("@/i18n/SimpleTranslationProvider", () => ({
  useSimpleLocale: () => ({ locale: "en", setLocale: jest.fn() }),
  getTranslation: (key: string) => key,
}));

jest.mock("@/api/tools/instance", () => ({
  axiosInstance: {
    get: jest.fn(),
    post: jest.fn(),
    put: jest.fn(),
  },
}));

jest.mock("@/hooks/useBusinessAccess", () => ({
  useBusinessAccess: jest.fn(),
}));

jest.mock("../AiWaiterToggle", () => ({
  __esModule: true,
  default: () => <div data-testid="ai-waiter-toggle" />,
}));
jest.mock("../../modals/ConfirmationModal", () => ({
  __esModule: true,
  default: () => null,
}));
jest.mock("../../DashboardLockedTabView", () => ({
  __esModule: true,
  default: () => null,
}));
jest.mock("../../overview/Metric", () => ({
  __esModule: true,
  default: () => null,
}));
jest.mock("@/providers/HybridAuthProvider", () => ({
  useAuth: () => ({ staffData: null }),
}));

import AiWaiterDashboard from "../AiWaiterDashboard";
import { axiosInstance } from "@/api/tools/instance";
import { useBusinessAccess } from "@/hooks/useBusinessAccess";

const mockedAxios = axiosInstance as jest.Mocked<typeof axiosInstance>;

const makeBusiness = (aiEnabled: boolean) =>
  ({
    id: 42,
    name: "Test Bistro",
    owner_address: "0x0",
    logo: "",
    address: {},
    settlement_address: "",
    tipping_address: "",
    tax_rate: 0,
    service_fee_rate: 0,
    tax_inclusive: false,
    service_inclusive: false,
    is_active: true,
    business_page_enabled: true,
    timezone: "UTC",
    ai_settings: {
      ai_enabled: aiEnabled,
      ai_name: "Sage",
      ai_priority: "balanced",
      special_instructions: "",
      business_page_ai_enabled: false,
    },
  }) as any;

describe("AiWaiterDashboard disabled workspace (L4-6)", () => {
  beforeEach(() => {
    jest.clearAllMocks();
    (useBusinessAccess as jest.Mock).mockReturnValue({
      access: null,
      loading: false,
      error: null,
      hasAccess: true,
      isSuspended: false,
      lockState: "active",
      aiConfigured: true,
      refetch: jest.fn(),
    });
    mockedAxios.get.mockImplementation((url: string) => {
      if (url.includes("/ai/insights")) {
        return Promise.resolve({
          data: {
            total_conversations: 1,
            total_messages: 1,
            upsell_success_rate: 0,
            conversation_trends_7d: [],
            busiest_hours: [],
          },
        });
      }
      if (url.includes("/ai/conversations?")) {
        return Promise.resolve({
          data: {
            conversations: [
              {
                id: 1,
                session_id: "sess-history",
                table_code: "T2",
                language: "en",
                status: "closed",
                created_at: "2026-05-12T08:00:00Z",
                updated_at: "2026-05-12T09:00:00Z",
                message_count: 3,
                is_paused: false,
                cart_items_added: 0,
              },
            ],
            total_pages: 1,
            active_count: 0,
          },
        });
      }
      if (url.includes("/whatsapp/status")) {
        return Promise.resolve({ data: { status: "disconnected", enabled: false, built: false } });
      }
      return Promise.resolve({ data: {} });
    });
  });

  it("keeps monitor tab + conversation history visible when AI Waiter is off", async () => {
    await act(async () => {
      render(
        <AiWaiterDashboard
          business={makeBusiness(false)}
          onUpdateBusiness={jest.fn()}
        />,
      );
    });

    // Tabs remain mounted (not gated on service toggle).
    expect(
      screen.getByText("aiWaiterDashboard.tabs.monitor"),
    ).toBeInTheDocument();

    const { goToMonitorTab } = await import("./_goToMonitorTab");
    await goToMonitorTab();

    // History table still loads.
    await waitFor(() => {
      expect(
        screen.getByText("aiWaiterDashboard.monitor.recentConversations"),
      ).toBeInTheDocument();
    });

    await waitFor(() => {
      expect(screen.getByText(/sess-his/i)).toBeInTheDocument();
    });

    // Read-only banner explains guest runtime is paused.
    expect(
      screen.getByText("aiWaiterDashboard.monitor.servicePausedBanner"),
    ).toBeInTheDocument();
  });
});
