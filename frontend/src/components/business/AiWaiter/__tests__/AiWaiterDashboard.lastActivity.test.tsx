/** @jest-environment jsdom */
/**
 * L4-2 — Recent conversations sorted by updated_at but UI only shows created_at.
 * Fix: add "Last activity" column rendering updated_at.
 */
import React from "react";
import { act, render, screen, waitFor } from "@testing-library/react";

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
    timezone: "UTC",
    ai_settings: {
      ai_enabled: true,
      ai_name: "Sage",
      ai_priority: "balanced",
      special_instructions: "",
      business_page_ai_enabled: false,
    },
  }) as any;

describe("AiWaiterDashboard last activity column (L4-2)", () => {
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
            total_messages: 2,
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
                session_id: "sess-abc12345",
                table_code: "T1",
                language: "en",
                status: "active",
                // Started early morning; last message much later — sort uses updated_at.
                created_at: "2026-05-12T08:00:00Z",
                updated_at: "2026-05-12T18:30:00Z",
                message_count: 5,
                is_paused: false,
                cart_items_added: 0,
              },
            ],
            total_pages: 1,
            active_count: 1,
          },
        });
      }
      if (url.includes("/whatsapp/status")) {
        return Promise.resolve({ data: { status: "disconnected", enabled: false, built: false } });
      }
      return Promise.resolve({ data: {} });
    });
  });

  it("shows a Last activity column header and renders updated_at for each row", async () => {
    await act(async () => {
      render(
        <AiWaiterDashboard
          business={makeBusiness()}
          onUpdateBusiness={jest.fn()}
        />,
      );
    });

    // Switch to monitor tab so the conversations table mounts.
    const monitorTab =
      screen.queryByText("aiWaiterDashboard.tabs.monitor") ||
      screen.queryByRole("tab", { name: /monitor/i });
    // Dashboard may land on overview; ensure monitor content is reachable.
    // The table uses translation keys as labels under our mock.
    await waitFor(() => {
      // Navigate: shell tabs call setActiveTab — click monitor if present.
      const tabs = screen.queryAllByText(/aiWaiterDashboard\.tabs\.monitor|Live Monitor|monitor/i);
      if (tabs[0]) {
        tabs[0].click();
      }
    });

    // Force monitor by looking for the table once active.
    // Many tests land on overview first; open monitor via tab strip.
    const monitorLabels = screen.getAllByText(
      (content) =>
        content === "aiWaiterDashboard.tabs.monitor" ||
        content === "aiWaiterDashboard.monitor.recentConversations" ||
        content.includes("monitor"),
    );
    // Click the tab key label if it's a button/tab.
    for (const el of monitorLabels) {
      const clickable =
        el.closest("button") || el.closest("[role='tab']") || el;
      if (clickable && clickable !== el.closest("table")) {
        await act(async () => {
          (clickable as HTMLElement).click();
        });
        break;
      }
    }

    await waitFor(() => {
      expect(
        screen.getByText("aiWaiterDashboard.monitor.table.lastActivity"),
      ).toBeInTheDocument();
    });

    // Row must surface the later updated_at activity time (18:30), not only 08:00 start.
    // formatBusinessDateTime with en/UTC short styles includes the time component.
    await waitFor(() => {
      const body = document.body.textContent || "";
      // Must include evening activity hour somewhere in the table region.
      expect(body).toMatch(/6:30|18:30/);
    });
  });
});
