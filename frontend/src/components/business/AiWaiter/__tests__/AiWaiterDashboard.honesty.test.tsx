/** @jest-environment jsdom */
/**
 * Task 34 / Findings 54, 56 — AI Waiter tells the truth:
 * - "Pick up" is not offered on a closed conversation
 * - 0.0% upsell rate uses Metric state="empty", not a success figure
 */
import React from "react";
import { act, render, screen, waitFor, within } from "@testing-library/react";
import userEvent from "@testing-library/user-event";

jest.mock("react-hot-toast", () => {
  const errorFn = jest.fn();
  const successFn = jest.fn();
  const toastFn = Object.assign(jest.fn(), {
    error: errorFn,
    success: successFn,
  });
  return {
    __esModule: true,
    default: toastFn,
    toast: toastFn,
  };
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
  default: () => null,
}));
jest.mock("../../modals/ConfirmationModal", () => ({
  __esModule: true,
  default: () => null,
}));
jest.mock("../../DashboardLockedTabView", () => ({
  __esModule: true,
  default: () => null,
}));

// Keep overview Metric for the non-upsell tiles; upsell must use ui/Metric.
jest.mock("../../overview/Metric", () => ({
  __esModule: true,
  default: ({ label, value }: { label: string; value: string }) => (
    <div data-testid={`metric-${label}`}>
      <span>{label}</span>
      <span data-testid={`metric-value-${label}`}>{value}</span>
    </div>
  ),
}));

jest.mock("@/providers/HybridAuthProvider", () => ({
  useAuth: () => ({ staffData: null }),
}));

import AiWaiterDashboard from "../AiWaiterDashboard";
import { axiosInstance } from "@/api/tools/instance";
import { useBusinessAccess } from "@/hooks/useBusinessAccess";

const mockedAxios = axiosInstance as jest.Mocked<typeof axiosInstance>;

const makeBusiness = () =>
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
    ai_settings: {
      ai_enabled: true,
      ai_name: "Sage",
      ai_priority: "balanced",
      special_instructions: "",
      business_page_ai_enabled: false,
    },
  }) as any;

const mockTierActive = () => {
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
};

function mockApis(opts: {
  upsellRate: number;
  conversations: Array<Record<string, unknown>>;
}) {
  mockedAxios.get.mockImplementation((url: string) => {
    if (url.includes("/ai/insights")) {
      return Promise.resolve({
        data: {
          total_conversations: opts.conversations.length,
          total_messages: 0,
          upsell_success_rate: opts.upsellRate,
          conversation_trends_7d: [],
          busiest_hours: [],
        },
      });
    }
    if (url.includes("/ai/conversations?")) {
      return Promise.resolve({
        data: {
          conversations: opts.conversations,
          total_pages: 1,
          active_count: opts.conversations.filter((c) => c.status === "active")
            .length,
        },
      });
    }
    if (url.includes("/whatsapp/status")) {
      return Promise.resolve({ data: { status: "disconnected", enabled: false, built: false } });
    }
    if (url.includes("/ai/conversations/") && url.includes("/messages")) {
      return Promise.resolve({ data: [] });
    }
    return Promise.resolve({ data: {} });
  });
}

describe("AiWaiterDashboard honesty (Task 34)", () => {
  beforeEach(() => {
    jest.clearAllMocks();
    mockTierActive();
  });

  it("shows consistent No data yet across KPI tiles when there are no conversations", async () => {
    mockApis({
      upsellRate: 0,
      conversations: [],
    });

    await act(async () => {
      render(
        <AiWaiterDashboard
          business={makeBusiness()}
          onUpdateBusiness={jest.fn()}
        />,
      );
    });

    // Navigate to Live Monitor where the KPI row lives.
    const monitorTab = await screen.findByRole("tab", { name: /tabs\.monitor/i });
    await act(async () => {
      monitorTab.click();
    });

    await waitFor(() => {
      expect(
        screen.getByTestId("metric-upsell-success-rate").getAttribute("data-state"),
      ).toBe("empty");
    });

    expect(
      screen.getByTestId("metric-total-conversations").getAttribute("data-state"),
    ).toBe("empty");
    expect(
      screen.getByTestId("metric-avg-messages").getAttribute("data-state"),
    ).toBe("empty");
    expect(screen.getByTestId("metric-upsell-success-rate").textContent).not.toMatch(
      /0\.0%/,
    );
  });

  it("shows a real 0.0% upsell rate once conversations exist (not mixed empty copy)", async () => {
    mockApis({
      upsellRate: 0,
      conversations: [
        {
          id: 1,
          session_id: "sess-1",
          table_code: "T1",
          language: "en",
          status: "active",
          created_at: "2026-05-12T12:00:00Z",
          updated_at: "2026-05-12T12:05:00Z",
          message_count: 2,
          is_paused: false,
          cart_items_added: 0,
        },
      ],
    });

    await act(async () => {
      render(
        <AiWaiterDashboard
          business={makeBusiness()}
          onUpdateBusiness={jest.fn()}
        />,
      );
    });

    const monitorTab = await screen.findByRole("tab", { name: /tabs\.monitor/i });
    await act(async () => {
      monitorTab.click();
    });

    await waitFor(() => {
      const upsell = screen.getByTestId("metric-upsell-success-rate");
      expect(upsell.getAttribute("data-state")).toBe("ok");
    });

    const upsell = screen.getByTestId("metric-upsell-success-rate");
    expect(upsell.textContent).toMatch(/0\.0%/);
    expect(upsell.className).not.toMatch(/emerald|green/);
  });

  it('does not offer "Pick up" on a closed conversation', async () => {
    const user = userEvent.setup();
    mockApis({
      upsellRate: 10,
      conversations: [
        {
          id: 9,
          session_id: "sess-closed",
          table_code: "T9",
          language: "en",
          status: "closed",
          created_at: "2026-05-12T12:00:00Z",
          updated_at: "2026-05-12T13:00:00Z",
          message_count: 4,
          is_paused: true,
          cart_items_added: 0,
          claimed_by_staff_id: null,
          claimed_at: null,
        },
      ],
    });

    await act(async () => {
      render(
        <AiWaiterDashboard
          business={makeBusiness()}
          onUpdateBusiness={jest.fn()}
        />,
      );
    });

    const monitorTab = await screen.findByRole("tab", { name: /tabs\.monitor/i });
    await user.click(monitorTab);

    await waitFor(() => {
      expect(
        screen.getByText("aiWaiterDashboard.monitor.viewChat"),
      ).toBeInTheDocument();
    });

    // Row actions: closed chats are view-only — no Pick up in the table.
    expect(
      screen.queryByText("aiWaiterDashboard.monitor.pickUp"),
    ).not.toBeInTheDocument();

    // Open transcript: still no Pick up.
    await user.click(screen.getByText("aiWaiterDashboard.monitor.viewChat"));
    await waitFor(() => {
      expect(
        screen.getByText("aiWaiterDashboard.transcript.title"),
      ).toBeInTheDocument();
    });

    const modal = screen
      .getByText("aiWaiterDashboard.transcript.title")
      .closest("[role='dialog']") || document.body;
    expect(
      within(modal as HTMLElement).queryByText(
        "aiWaiterDashboard.monitor.pickUp",
      ),
    ).not.toBeInTheDocument();
  });
});
