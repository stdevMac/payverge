/** @jest-environment jsdom */
/**
 * Issues 173 / 177 / 195 / 240 — operator-facing AI Waiter honesty:
 * idle copy when service is on with no chats, Overview landing,
 * consistent empty KPI language, and an upselling confirm.
 */
import React from "react";
import { act, render, screen, waitFor } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { goToMonitorTab } from "./_goToMonitorTab";

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
  default: ({
    isOpen,
    title,
    description,
    confirmLabel,
    cancelLabel,
    onConfirm,
    onOpenChange,
  }: {
    isOpen: boolean;
    title: string;
    description: string;
    confirmLabel?: string;
    cancelLabel?: string;
    onConfirm: () => void;
    onOpenChange: () => void;
  }) =>
    isOpen ? (
      <div role="dialog" data-testid="confirm-dialog">
        <h2>{title}</h2>
        <p>{description}</p>
        <button type="button" onClick={onConfirm}>
          {confirmLabel}
        </button>
        <button type="button" onClick={onOpenChange}>
          {cancelLabel}
        </button>
      </div>
    ) : null,
}));

jest.mock("../../DashboardLockedTabView", () => ({
  __esModule: true,
  default: () => null,
}));

jest.mock("@/providers/HybridAuthProvider", () => ({
  useAuth: () => ({ staffData: null }),
}));

jest.mock("@/api/business", () => ({
  __esModule: true,
  getMenu: jest.fn().mockResolvedValue({ categories: [] }),
}));

import AiWaiterDashboard, {
  metricsHaveConversationData,
} from "../AiWaiterDashboard";
import { axiosInstance } from "@/api/tools/instance";
import { useBusinessAccess } from "@/hooks/useBusinessAccess";

const mockedAxios = axiosInstance as jest.Mocked<typeof axiosInstance>;

const makeBusiness = (
  overrides?: Partial<{ ai_enabled: boolean; ai_priority: string }>,
) =>
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
      ai_enabled: overrides?.ai_enabled ?? true,
      ai_name: "Sage",
      ai_priority: overrides?.ai_priority ?? "balanced",
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
  totalConversations?: number;
  conversations?: Array<Record<string, unknown>>;
}) {
  mockedAxios.get.mockImplementation((url: string) => {
    if (url.includes("/ai/insights")) {
      return Promise.resolve({
        data: {
          total_conversations: opts.totalConversations ?? 0,
          total_messages: 0,
          upsell_success_rate: 0,
          conversation_trends_7d: [],
          busiest_hours: [],
        },
      });
    }
    if (url.includes("/ai/conversations?")) {
      return Promise.resolve({
        data: {
          conversations: opts.conversations ?? [],
          total_pages: 1,
          active_count: (opts.conversations ?? []).filter(
            (c) => c.status === "active",
          ).length,
        },
      });
    }
    if (url.includes("/whatsapp/status")) {
      return Promise.resolve({ data: { status: "disconnected", enabled: false, built: false } });
    }
    return Promise.resolve({ data: {} });
  });
}

async function renderDashboard(
  business = makeBusiness(),
): Promise<void> {
  await act(async () => {
    render(
      <AiWaiterDashboard
        business={business}
        onUpdateBusiness={jest.fn()}
      />,
    );
  });
}

describe("AiWaiterDashboard operator honesty (173 / 177 / 195 / 240)", () => {
  beforeEach(() => {
    jest.clearAllMocks();
    mockTierActive();
  });

  it("lands on Overview & Settings, not Live Monitor (#177)", async () => {
    mockApis({ totalConversations: 0 });
    await renderDashboard();

    expect(
      await screen.findByText("aiWaiterDashboard.settings.personalityTitle"),
    ).toBeInTheDocument();
    expect(
      screen.queryByText("aiWaiterDashboard.monitor.recentConversations"),
    ).not.toBeInTheDocument();
    expect(screen.getAllByTestId("ai-waiter-toggle").length).toBeGreaterThan(0);
    expect(screen.getByTestId("ai-allergen-guardrail")).toBeInTheDocument();
  });

  it("does not say Autonomous service on, and explains nothing is happening yet (#173)", async () => {
    mockApis({ totalConversations: 0 });
    await renderDashboard();

    await waitFor(() => {
      expect(screen.getByTestId("ai-idle-banner")).toBeInTheDocument();
    });

    const body = document.body.textContent || "";
    expect(body).not.toMatch(/Autonomous service on/i);
    expect(body).toContain("aiWaiterDashboard.shell.serviceOnIdle");
    expect(body).toContain("aiWaiterDashboard.shell.serviceOnIdleDetail");
    expect(body).toContain("aiWaiterDashboard.shell.idleBanner");
    expect(screen.queryByText("aiWaiterDashboard.shell.liveChats")).not.toBeInTheDocument();
  });

  it("keeps guest-chat status (not idle) once conversations exist", async () => {
    mockApis({
      totalConversations: 4,
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
    await renderDashboard();

    await waitFor(() => {
      expect(
        screen.getByText("aiWaiterDashboard.shell.serviceOn"),
      ).toBeInTheDocument();
    });
    expect(screen.queryByTestId("ai-idle-banner")).not.toBeInTheDocument();
    expect(
      screen.queryByText("aiWaiterDashboard.shell.serviceOnIdle"),
    ).not.toBeInTheDocument();
  });

  it("shows the same No data yet language on every empty KPI tile (#195)", async () => {
    mockApis({ totalConversations: 0 });
    await renderDashboard();
    await goToMonitorTab();

    await waitFor(() => {
      expect(
        screen.getByTestId("metric-total-conversations").getAttribute("data-state"),
      ).toBe("empty");
    });

    for (const id of [
      "metric-total-conversations",
      "metric-upsell-success-rate",
      "metric-avg-messages",
    ]) {
      const tile = screen.getByTestId(id);
      expect(tile.getAttribute("data-state")).toBe("empty");
      expect(tile.textContent).toBe("aiWaiterDashboard.stats.noData");
      expect(tile.textContent).not.toMatch(/^0(\.0%)?$/);
    }
    expect(metricsHaveConversationData(0)).toBe(false);
  });

  it("keeps the ACTION column wide enough to stay visible (#177)", async () => {
    mockApis({
      totalConversations: 1,
      conversations: [
        {
          id: 1,
          session_id: "sess-abc12345",
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
    await renderDashboard();
    await goToMonitorTab();

    const tableWrap = await screen.findByTestId("ai-conversations-table");
    expect(tableWrap.querySelector("table")?.className).toMatch(/min-w-\[52rem\]/);
    expect(tableWrap.firstElementChild?.className).toMatch(/pr-16/);
    expect(screen.getByTestId("ai-monitor-actions-col")).toHaveTextContent(
      "aiWaiterDashboard.monitor.table.action",
    );
    expect(screen.getByText("aiWaiterDashboard.monitor.viewChat")).toBeInTheDocument();
    expect(screen.getByText("aiWaiterDashboard.monitor.pickUp")).toBeInTheDocument();
  });

  it("confirms before switching Priority Mode to upselling (#240)", async () => {
    mockApis({ totalConversations: 0 });
    await renderDashboard();

    expect(screen.getByTestId("ai-allergen-guardrail")).toBeInTheDocument();
    expect(screen.queryByTestId("ai-upselling-guardrail")).not.toBeInTheDocument();

    const user = userEvent.setup();
    await user.click(screen.getByTestId("ai-priority-select"));
    await user.click(
      await screen.findByRole("option", {
        name: /settings\.priorities\.upselling/i,
      }),
    );

    const dialog = await screen.findByTestId("confirm-dialog");
    expect(dialog).toHaveTextContent(
      "aiWaiterDashboard.settings.upsellingConfirmTitle",
    );
    expect(dialog).toHaveTextContent(
      "aiWaiterDashboard.settings.upsellingConfirmDescription",
    );

    await user.click(
      screen.getByRole("button", {
        name: "aiWaiterDashboard.settings.upsellingConfirmCancel",
      }),
    );
    expect(screen.queryByTestId("ai-upselling-guardrail")).not.toBeInTheDocument();

    await user.click(screen.getByTestId("ai-priority-select"));
    await user.click(
      await screen.findByRole("option", {
        name: /settings\.priorities\.upselling/i,
      }),
    );
    await user.click(
      await screen.findByRole("button", {
        name: "aiWaiterDashboard.settings.upsellingConfirmConfirm",
      }),
    );
    expect(await screen.findByTestId("ai-upselling-guardrail")).toBeInTheDocument();
    expect(screen.getByTestId("ai-allergen-guardrail")).toBeInTheDocument();
  });

  it("shows the upselling guardrail when the saved mode is already upselling", async () => {
    mockApis({ totalConversations: 0 });
    await renderDashboard(makeBusiness({ ai_priority: "upselling" }));

    expect(await screen.findByTestId("ai-upselling-guardrail")).toBeInTheDocument();
    expect(screen.getByTestId("ai-allergen-guardrail")).toBeInTheDocument();
    expect(screen.queryByTestId("confirm-dialog")).not.toBeInTheDocument();
  });
});
