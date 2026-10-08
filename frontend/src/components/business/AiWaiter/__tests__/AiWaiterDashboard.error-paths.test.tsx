/** @jest-environment jsdom */
/**
 * Error-path coverage for AiWaiterDashboard.
 *
 * Each handler (save, pause-in-modal, send-reply, close-conversation) used to
 * swallow its API failure to console.error, leaving the owner to guess whether
 * the action landed. handleInlinePause was the only handler doing it right
 * (toast.error). This suite locks in the pattern across the rest and verifies
 * the fetch-on-mount trio shows an inline retry banner instead of a silent
 * empty state.
 */

import React from "react";
import {
  act,
  fireEvent,
  render,
  screen,
  waitFor,
} from "@testing-library/react";
import { goToMonitorTab } from "./_goToMonitorTab";

// Mock react-hot-toast — we assert toast.error was called with the right key.
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

// Mock translation provider — return the key so assertions are predictable.
jest.mock("@/i18n/SimpleTranslationProvider", () => ({
  useSimpleLocale: () => ({ locale: "en", setLocale: jest.fn() }),
  getTranslation: (key: string) => key,
}));

// Mock axios instance — every axios call resolves/rejects per mock.
jest.mock("@/api/tools/instance", () => ({
  axiosInstance: {
    get: jest.fn(),
    post: jest.fn(),
    put: jest.fn(),
  },
}));

// Mock business-tier hook — pretend AI plan is active so the dashboard renders.
jest.mock("@/hooks/useBusinessAccess", () => ({
  useBusinessAccess: jest.fn(),
}));

// Child components that make their own API calls — stub them out entirely.
jest.mock("../AiWaiterToggle", () => ({
  __esModule: true,
  default: () => null,
}));
// Lightweight ConfirmationModal stub: renders the confirm control (wired to
// onConfirm) only while open, so confirmation-gated flows are drivable in tests
// without pulling in the real NextUI modal.
jest.mock("../../modals/ConfirmationModal", () => ({
  __esModule: true,
  default: ({
    isOpen,
    confirmLabel,
    onConfirm,
  }: {
    isOpen: boolean;
    confirmLabel?: string;
    onConfirm: () => void;
  }) =>
    isOpen ? (
      <button type="button" onClick={onConfirm}>
        {confirmLabel}
      </button>
    ) : null,
}));
jest.mock("../../DashboardLockedTabView", () => ({
  __esModule: true,
  default: () => <div data-testid="locked-ai-waiter" />,
}));
jest.mock("../../overview/Metric", () => ({
  __esModule: true,
  default: ({ label, value }: { label: string; value: string }) => (
    <div>
      <span>{label}</span>
      <span>{value}</span>
    </div>
  ),
}));

// HybridAuthProvider transitively imports wagmi (ESM); stub useAuth to an
// owner principal (staffData: null) so the dashboard renders the full view.
jest.mock("@/providers/HybridAuthProvider", () => ({
  useAuth: () => ({ staffData: null }),
}));

import AiWaiterDashboard from "../AiWaiterDashboard";
import { axiosInstance } from "@/api/tools/instance";
import { useBusinessAccess } from "@/hooks/useBusinessAccess";
import toast from "react-hot-toast";

// Handy alias — references the same mocked fn the component invokes.
const mockToastError = (toast as unknown as { error: jest.Mock }).error;
const mockToastSuccess = (toast as unknown as { success: jest.Mock }).success;

const mockedAxios = axiosInstance as jest.Mocked<typeof axiosInstance>;

const expectedConsoleErrorPrefixes = [
  "Failed to fetch conversations:",
  "Failed to fetch insights:",
  "Failed to fetch WA status",
  "Failed to update settings:",
  "Failed to toggle pause:",
  "Failed to send reply:",
  "Failed to close conversation:",
];

let consoleErrorSpy: jest.SpyInstance;
let unexpectedConsoleErrors: unknown[][];

// Minimal Business fixture satisfying the props; fields the component reads
// have real values, the rest are cast as needed.
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

const SAMPLE_CONV = {
  id: 7,
  session_id: "abcdef0123456789",
  table_code: "T1",
  language: "en",
  status: "active",
  created_at: "2026-04-15T12:00:00Z",
  updated_at: "2026-04-15T12:05:00Z",
  message_count: 3,
  is_paused: true, // paused so the modal's reply input renders
  cart_items_added: 0,
};

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

// Happy-path responses for the on-mount fetches. Tests override as needed.
const mockDefaultGetResponses = (
  convOverride: Record<string, unknown> = {},
) => {
  mockedAxios.get.mockImplementation((url: string) => {
    if (url.includes("/ai/conversations?")) {
      return Promise.resolve({
        data: {
          conversations: [{ ...SAMPLE_CONV, ...convOverride }],
          total_pages: 1,
        },
      });
    }
    if (url.includes("/whatsapp/status")) {
      return Promise.resolve({ data: { status: "disconnected", enabled: false, built: false } });
    }
    if (url.includes("/ai/conversations/") && url.includes("/messages")) {
      return Promise.resolve({ data: [] });
    }
    if (url.includes("/ai/insights")) {
      return Promise.resolve({
        data: {
          total_conversations: 0,
          total_messages: 0,
          upsell_success_rate: 0,
        },
      });
    }
    return Promise.resolve({ data: {} });
  });
};

describe("AiWaiterDashboard error paths", () => {
  beforeEach(() => {
    jest.clearAllMocks();
    unexpectedConsoleErrors = [];
    consoleErrorSpy = jest
      .spyOn(console, "error")
      .mockImplementation((...args: unknown[]) => {
        const firstArg = String(args[0] ?? "");
        const isExpected = expectedConsoleErrorPrefixes.some((prefix) =>
          firstArg.startsWith(prefix),
        );
        if (!isExpected) {
          unexpectedConsoleErrors.push(args);
        }
      });
    mockTierActive();
    mockDefaultGetResponses();
  });

  afterEach(() => {
    try {
      expect(unexpectedConsoleErrors).toEqual([]);
    } finally {
      consoleErrorSpy.mockRestore();
    }
  });

  it("issues no AI feature request while an administrator suspended the business", async () => {
    (useBusinessAccess as jest.Mock).mockReturnValue({
      hasAccess: false,
      isSuspended: true,
      aiConfigured: true,
      loading: false,
    });
    render(
      <AiWaiterDashboard
        business={makeBusiness()}
        onUpdateBusiness={jest.fn()}
      />,
    );
    expect(await screen.findByTestId("locked-ai-waiter")).toBeInTheDocument();
    expect(mockedAxios.get).not.toHaveBeenCalled();
    expect(mockedAxios.post).not.toHaveBeenCalled();
  });

  it("waits for the access check before loading monitor data", async () => {
    (useBusinessAccess as jest.Mock).mockReturnValue({
      hasAccess: false,
      isSuspended: false,
      aiConfigured: false,
      loading: true,
    });
    const business = makeBusiness();
    const view = render(
      <AiWaiterDashboard business={business} onUpdateBusiness={jest.fn()} />,
    );
    expect(mockedAxios.get).not.toHaveBeenCalled();
    mockTierActive();
    view.rerender(
      <AiWaiterDashboard business={business} onUpdateBusiness={jest.fn()} />,
    );
    await goToMonitorTab();
    await waitFor(() =>
      expect(mockedAxios.get).toHaveBeenCalledWith(
        "/inside/businesses/42/ai/conversations?page=1",
      ),
    );
    expect(mockedAxios.get).toHaveBeenCalledWith(
      "/inside/businesses/42/ai/insights",
    );
  });

  it("toasts and shows retry banner when fetchConversations fails on mount", async () => {
    mockedAxios.get.mockImplementation((url: string) => {
      if (url.includes("/ai/conversations?")) {
        return Promise.reject(new Error("boom"));
      }
      if (url.includes("/whatsapp/status")) {
        return Promise.resolve({ data: { status: "disconnected", enabled: false, built: false } });
      }
      return Promise.resolve({ data: {} });
    });

    render(
      <AiWaiterDashboard
        business={makeBusiness()}
        onUpdateBusiness={jest.fn()}
      />,
    );

    await goToMonitorTab();

    const banner = await screen.findByTestId("fetch-error-conversations");
    expect(banner).toBeInTheDocument();
    expect(banner).toHaveTextContent("errors.fetchConversationsFailed");
    expect(screen.queryByText("monitor.emptyTitle")).not.toBeInTheDocument();

    // Retry button is inside the banner — clicking it retries the fetch.
    const retryBtn = screen.getByRole("button", { name: /errors\.retry/i });
    // Fix the mock so the retry succeeds.
    mockedAxios.get.mockImplementation((url: string) => {
      if (url.includes("/ai/conversations?")) {
        return Promise.resolve({ data: { conversations: [], total_pages: 1 } });
      }
      if (url.includes("/whatsapp/status")) {
        return Promise.resolve({ data: { status: "disconnected", enabled: false, built: false } });
      }
      return Promise.resolve({ data: {} });
    });
    await act(async () => {
      fireEvent.click(retryBtn);
    });
    await waitFor(() => {
      expect(
        screen.queryByTestId("fetch-error-conversations"),
      ).not.toBeInTheDocument();
    });
  });

  it("does not claim there are no conversations when the list has rows (#677)", async () => {
    mockedAxios.get.mockImplementation((url: string) => {
      if (url.includes("/ai/conversations?")) {
        return Promise.resolve({
          data: {
            conversations: Array.from({ length: 20 }, (_, i) => ({
              ...SAMPLE_CONV,
              id: i + 1,
              session_id: `sess-${i + 1}`,
            })),
            total_pages: 2,
            total_count: 40,
            active_count: 26,
          },
        });
      }
      if (url.includes("/whatsapp/status")) {
        return Promise.resolve({ data: { status: "disconnected", enabled: false, built: false } });
      }
      if (url.includes("/ai/insights")) {
        return Promise.resolve({
          data: {
            total_conversations: 26,
            total_messages: 80,
            upsell_success_rate: 0,
          },
        });
      }
      return Promise.resolve({ data: {} });
    });

    render(
      <AiWaiterDashboard
        business={makeBusiness()}
        onUpdateBusiness={jest.fn()}
      />,
    );
    await goToMonitorTab();

    await waitFor(() => {
      expect(screen.getByTestId("ai-conversations-table")).toBeInTheDocument();
    });
    expect(screen.queryByText("monitor.emptyTitle")).not.toBeInTheDocument();
    expect(
      screen.queryByTestId("fetch-error-conversations"),
    ).not.toBeInTheDocument();
  });

  it("does not show the empty-conversations title when the list payload is null but KPIs are non-zero (#677)", async () => {
    mockedAxios.get.mockImplementation((url: string) => {
      if (url.includes("/ai/conversations?")) {
        return Promise.resolve({
          data: {
            conversations: null,
            total_pages: 2,
            total_count: 40,
            active_count: 26,
          },
        });
      }
      if (url.includes("/whatsapp/status")) {
        return Promise.resolve({ data: { status: "disconnected", enabled: false, built: false } });
      }
      if (url.includes("/ai/insights")) {
        return Promise.resolve({
          data: {
            total_conversations: 26,
            total_messages: 80,
            upsell_success_rate: 0,
          },
        });
      }
      return Promise.resolve({ data: {} });
    });

    render(
      <AiWaiterDashboard
        business={makeBusiness()}
        onUpdateBusiness={jest.fn()}
      />,
    );
    await goToMonitorTab();

    await waitFor(() => {
      expect(screen.getByTestId("ai-conversations-table")).toBeInTheDocument();
    });
    expect(screen.queryByText("monitor.emptyTitle")).not.toBeInTheDocument();
  });

  it("shows retry banner when fetchInsights fails after switching to insights tab", async () => {
    mockedAxios.get.mockImplementation((url: string) => {
      if (url.includes("/ai/insights")) {
        return Promise.reject(new Error("insights down"));
      }
      if (url.includes("/ai/conversations?")) {
        return Promise.resolve({ data: { conversations: [], total_pages: 1 } });
      }
      if (url.includes("/whatsapp/status")) {
        return Promise.resolve({ data: { status: "disconnected", enabled: false, built: false } });
      }
      return Promise.resolve({ data: {} });
    });

    render(
      <AiWaiterDashboard
        business={makeBusiness()}
        onUpdateBusiness={jest.fn()}
      />,
    );

    const insightsTab = await screen.findByRole("tab", {
      name: /tabs\.insights/i,
    });
    await act(async () => {
      fireEvent.click(insightsTab);
    });

    const banner = await screen.findByTestId("fetch-error-insights");
    expect(banner).toHaveTextContent("errors.fetchInsightsFailed");
  });

  it("toasts when handleSaveSettings PUT fails", async () => {
    mockedAxios.put.mockRejectedValue(new Error("save down"));

    render(
      <AiWaiterDashboard
        business={makeBusiness()}
        onUpdateBusiness={jest.fn()}
      />,
    );

    // Switch to the Overview tab (where the save button lives).
    const overviewTab = await screen.findByRole("tab", {
      name: /tabs\.overview/i,
    });
    await act(async () => {
      fireEvent.click(overviewTab);
    });

    const saveBtn = await screen.findByRole("button", {
      name: /settings\.saveButton/i,
    });
    await act(async () => {
      fireEvent.click(saveBtn);
    });

    await waitFor(() => {
      expect(mockToastError).toHaveBeenCalledWith(
        "aiWaiterDashboard.errors.saveFailed",
      );
    });
  });

  // D1 / L4-4: success path must toast (was silent).
  // Revert-proof: remove toast.success from handleSaveSettings → this fails.
  it("toasts success when handleSaveSettings PUT succeeds", async () => {
    mockedAxios.put.mockResolvedValue({ data: makeBusiness() });
    const onUpdateBusiness = jest.fn();

    render(
      <AiWaiterDashboard
        business={makeBusiness()}
        onUpdateBusiness={onUpdateBusiness}
      />,
    );

    const overviewTab = await screen.findByRole("tab", {
      name: /tabs\.overview/i,
    });
    await act(async () => {
      fireEvent.click(overviewTab);
    });

    const saveBtn = await screen.findByRole("button", {
      name: /settings\.saveButton/i,
    });
    await act(async () => {
      fireEvent.click(saveBtn);
    });

    await waitFor(() => {
      expect(mockToastSuccess).toHaveBeenCalledWith(
        "aiWaiterDashboard.errors.saveSuccess",
      );
    });
    expect(onUpdateBusiness).toHaveBeenCalled();
  });

  // L4-4 residual: backend-shaped rejection surfaces via surfaceBackendError
  // (catalog code), not only the generic saveFailed key.
  it("surfaces backend-shaped save errors through surfaceBackendError", async () => {
    mockedAxios.put.mockRejectedValue(
      Object.assign(new Error("request failed"), {
        status: 400,
        response: {
          status: 400,
          data: {
            code: "VALIDATION_INVALID_INPUT",
            error: "ai_name is required",
          },
        },
      }),
    );

    render(
      <AiWaiterDashboard
        business={makeBusiness()}
        onUpdateBusiness={jest.fn()}
      />,
    );

    const overviewTab = await screen.findByRole("tab", {
      name: /tabs\.overview/i,
    });
    await act(async () => {
      fireEvent.click(overviewTab);
    });

    const saveBtn = await screen.findByRole("button", {
      name: /settings\.saveButton/i,
    });
    await act(async () => {
      fireEvent.click(saveBtn);
    });

    await waitFor(() => {
      expect(mockToastError).toHaveBeenCalled();
    });
    const msg = mockToastError.mock.calls[
      mockToastError.mock.calls.length - 1
    ][0] as string;
    // Catalog message (en), not the try-again saveFailed key alone.
    expect(msg.toLowerCase()).toMatch(/check|form|try again|invalid|required/);
    expect(msg).not.toBe("aiWaiterDashboard.errors.saveFailed");
  });

  it("toasts when handlePauseAi POST fails (from transcript modal)", async () => {
    // Default GETs succeed so we can open the modal.
    render(
      <AiWaiterDashboard
        business={makeBusiness()}
        onUpdateBusiness={jest.fn()}
      />,
    );

    await goToMonitorTab();

    // Click "View Chat" to open the transcript modal.
    const viewChat = await screen.findByRole("button", {
      name: /monitor\.viewChat/i,
    });
    await act(async () => {
      fireEvent.click(viewChat);
    });

    // Now the "Pause AI" / "Resume AI" button is in the modal. is_paused=true
    // so the button reads "resumeAi".
    const resumeBtn = await screen.findByRole("button", {
      name: /transcript\.resumeAi/i,
    });

    mockedAxios.post.mockRejectedValueOnce(new Error("pause down"));
    await act(async () => {
      fireEvent.click(resumeBtn);
    });

    await waitFor(() => {
      expect(mockToastError).toHaveBeenCalledWith(
        "aiWaiterDashboard.errors.pauseFailed",
      );
    });
  });

  it("toasts when handleSendReply POST fails", async () => {
    // Since decision #4 the composer also requires holding the claim — owners
    // included. useAuth is stubbed to the owner principal, whose held-claim
    // shape carries no staff id.
    mockDefaultGetResponses({
      claimed_by_staff_id: null,
      claimed_by_name: "Owner",
      claimed_by_role: "owner",
      claimed_at: "2026-04-15T12:05:00Z",
    });
    render(
      <AiWaiterDashboard
        business={makeBusiness()}
        onUpdateBusiness={jest.fn()}
      />,
    );

    await goToMonitorTab();

    const viewChat = await screen.findByRole("button", {
      name: /monitor\.viewChat/i,
    });
    await act(async () => {
      fireEvent.click(viewChat);
    });

    // The reply input only renders when the conversation is_paused=true.
    const replyInput = await screen.findByPlaceholderText(
      /transcript\.staffReplyPlaceholder/i,
    );
    await act(async () => {
      fireEvent.change(replyInput, { target: { value: "Hello guest" } });
    });

    mockedAxios.post.mockRejectedValueOnce(new Error("reply down"));

    // Find the send button (icon-only, aria has no explicit name — it's
    // the only button adjacent to the input). Query by the Send icon's
    // containing button via role + position.
    const allButtons = screen.getAllByRole("button");
    // Pick the button right after the reply input; it's the only icon-only
    // button whose form is the modal footer row with the input.
    // Simpler: the send button comes immediately after the input in DOM order.
    let sendBtn: HTMLElement | null = null;
    for (const btn of allButtons) {
      if (btn.querySelector("svg") && !btn.textContent?.trim()) {
        // icon-only button — the first one after replyInput in DOM order is send
        if (
          replyInput.compareDocumentPosition(btn) &
          Node.DOCUMENT_POSITION_FOLLOWING
        ) {
          sendBtn = btn;
          break;
        }
      }
    }
    expect(sendBtn).not.toBeNull();

    await act(async () => {
      fireEvent.click(sendBtn!);
    });

    await waitFor(() => {
      expect(mockToastError).toHaveBeenCalledWith(
        "aiWaiterDashboard.errors.sendReplyFailed",
      );
    });
  });

  it("toasts when handleCloseConversation POST fails", async () => {
    render(
      <AiWaiterDashboard
        business={makeBusiness()}
        onUpdateBusiness={jest.fn()}
      />,
    );

    await goToMonitorTab();

    const viewChat = await screen.findByRole("button", {
      name: /monitor\.viewChat/i,
    });
    await act(async () => {
      fireEvent.click(viewChat);
    });

    const closeBtn = await screen.findByRole("button", {
      name: /transcript\.closeConversation/i,
    });

    mockedAxios.post.mockRejectedValueOnce(new Error("close down"));
    // Closing is now confirmation-gated (fix 10): the first click opens a
    // ConfirmationModal; the actual POST fires only after confirming.
    await act(async () => {
      fireEvent.click(closeBtn);
    });
    const confirmBtn = await screen.findByText(
      "aiWaiterDashboard.transcript.closeConfirmConfirm",
    );
    await act(async () => {
      fireEvent.click(confirmBtn);
    });

    await waitFor(() => {
      expect(mockToastError).toHaveBeenCalledWith(
        "aiWaiterDashboard.errors.closeFailed",
      );
    });
  });

  it("does not close the conversation until the confirmation is accepted", async () => {
    render(
      <AiWaiterDashboard
        business={makeBusiness()}
        onUpdateBusiness={jest.fn()}
      />,
    );

    await goToMonitorTab();

    const viewChat = await screen.findByRole("button", {
      name: /monitor\.viewChat/i,
    });
    await act(async () => {
      fireEvent.click(viewChat);
    });

    const closeBtn = await screen.findByRole("button", {
      name: /transcript\.closeConversation/i,
    });

    const closeCallsBefore = mockedAxios.post.mock.calls.filter(
      (call) => String(call[0]).includes("/close"),
    ).length;

    // Clicking the close button opens the confirmation but must NOT POST /close.
    await act(async () => {
      fireEvent.click(closeBtn);
    });
    const closeCallsAfterOpen = mockedAxios.post.mock.calls.filter(
      (call) => String(call[0]).includes("/close"),
    ).length;
    expect(closeCallsAfterOpen).toBe(closeCallsBefore);

    // The confirmation control is present (gate is in place).
    expect(
      await screen.findByText(
        "aiWaiterDashboard.transcript.closeConfirmConfirm",
      ),
    ).toBeInTheDocument();
  });
});
