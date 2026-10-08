/** @jest-environment jsdom */
/**
 * L4-7: closed conversations are a terminal state. The monitor list and the
 * transcript modal must stop offering pause / pick-up / close on them, and the
 * backend's 409 conversation_closed (race: closed in another tab between load
 * and click) must surface as honest copy — not "someone else is handling it"
 * or a generic failure.
 */

import React from "react";
import { act, fireEvent, render, screen, waitFor } from "@testing-library/react";
import { goToMonitorTab } from "./_goToMonitorTab";

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
jest.mock("@/providers/HybridAuthProvider", () => ({
  useAuth: () => ({ staffData: null }),
}));

import AiWaiterDashboard from "../AiWaiterDashboard";
import { axiosInstance } from "@/api/tools/instance";
import { useBusinessAccess } from "@/hooks/useBusinessAccess";
import toast from "react-hot-toast";

const mockToastError = (toast as unknown as { error: jest.Mock }).error;
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

const SAMPLE_CONV = {
  id: 7,
  session_id: "abcdef0123456789",
  table_code: "T1",
  language: "en",
  status: "active",
  created_at: "2026-04-15T12:00:00Z",
  updated_at: "2026-04-15T12:05:00Z",
  message_count: 3,
  is_paused: false,
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

const mockDefaultGetResponses = (convOverride: Record<string, unknown> = {}) => {
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

const conversationClosed409 = () => ({
  response: { status: 409, data: { code: "conversation_closed" } },
});

let consoleErrorSpy: jest.SpyInstance;

describe("AiWaiterDashboard closed-conversation state (L4-7)", () => {
  beforeEach(() => {
    jest.clearAllMocks();
    consoleErrorSpy = jest.spyOn(console, "error").mockImplementation(() => {});
    mockTierActive();
    mockDefaultGetResponses();
  });

  afterEach(() => {
    consoleErrorSpy.mockRestore();
  });

  it("offers no pause or pick-up affordance on a closed conversation row", async () => {
    mockDefaultGetResponses({ status: "closed", is_paused: false });

    render(
      <AiWaiterDashboard business={makeBusiness()} onUpdateBusiness={jest.fn()} />,
    );

    await goToMonitorTab();
    // The row itself renders (view chat stays available).
    await screen.findByRole("button", { name: /monitor\.viewChat/i });

    expect(
      screen.queryByRole("button", { name: /monitor\.pauseInline/i }),
    ).not.toBeInTheDocument();
    expect(
      screen.queryByRole("button", { name: /monitor\.resumeInline/i }),
    ).not.toBeInTheDocument();
    expect(
      screen.queryByRole("button", { name: /monitor\.pickUp/i }),
    ).not.toBeInTheDocument();
  });

  it("transcript modal of a closed conversation offers neither pause nor close-conversation", async () => {
    mockDefaultGetResponses({ status: "closed", is_paused: false });

    render(
      <AiWaiterDashboard business={makeBusiness()} onUpdateBusiness={jest.fn()} />,
    );


    await goToMonitorTab();
    const viewChat = await screen.findByRole("button", {
      name: /monitor\.viewChat/i,
    });
    await act(async () => {
      fireEvent.click(viewChat);
    });

    expect(
      await screen.findByTestId("transcript-closed-notice"),
    ).toBeInTheDocument();
    expect(
      screen.queryByRole("button", { name: /transcript\.pauseAi/i }),
    ).not.toBeInTheDocument();
    expect(
      screen.queryByRole("button", { name: /transcript\.resumeAi/i }),
    ).not.toBeInTheDocument();
    expect(
      screen.queryByRole("button", { name: /transcript\.closeConversation/i }),
    ).not.toBeInTheDocument();
  });

  it("maps the conversation_closed 409 on pick-up to honest copy, not claimConflict", async () => {
    render(
      <AiWaiterDashboard business={makeBusiness()} onUpdateBusiness={jest.fn()} />,
    );


    await goToMonitorTab();
    const pickUp = await screen.findByRole("button", {
      name: /monitor\.pickUp/i,
    });
    mockedAxios.post.mockRejectedValueOnce(conversationClosed409());
    await act(async () => {
      fireEvent.click(pickUp);
    });

    await waitFor(() => {
      expect(mockToastError).toHaveBeenCalledWith(
        "aiWaiterDashboard.transcript.conversationClosed",
      );
    });
    expect(mockToastError).not.toHaveBeenCalledWith(
      expect.stringContaining("claimConflict"),
    );
  });

  it("maps the conversation_closed 409 on inline pause to honest copy", async () => {
    render(
      <AiWaiterDashboard business={makeBusiness()} onUpdateBusiness={jest.fn()} />,
    );


    await goToMonitorTab();
    const pauseBtn = await screen.findByRole("button", {
      name: /monitor\.pauseInline/i,
    });
    mockedAxios.post.mockRejectedValueOnce(conversationClosed409());
    await act(async () => {
      fireEvent.click(pauseBtn);
    });

    await waitFor(() => {
      expect(mockToastError).toHaveBeenCalledWith(
        "aiWaiterDashboard.transcript.conversationClosed",
      );
    });
    expect(mockToastError).not.toHaveBeenCalledWith(
      "aiWaiterDashboard.monitor.pauseFailed",
    );
  });

  it("maps the conversation_closed 409 on modal pause to honest copy and degrades the modal", async () => {
    render(
      <AiWaiterDashboard business={makeBusiness()} onUpdateBusiness={jest.fn()} />,
    );


    await goToMonitorTab();
    const viewChat = await screen.findByRole("button", {
      name: /monitor\.viewChat/i,
    });
    await act(async () => {
      fireEvent.click(viewChat);
    });

    const pauseBtn = await screen.findByRole("button", {
      name: /transcript\.pauseAi/i,
    });
    mockedAxios.post.mockRejectedValueOnce(conversationClosed409());
    await act(async () => {
      fireEvent.click(pauseBtn);
    });

    await waitFor(() => {
      expect(mockToastError).toHaveBeenCalledWith(
        "aiWaiterDashboard.transcript.conversationClosed",
      );
    });
    // The modal now reflects the terminal state instead of keeping the
    // pause affordance armed.
    expect(
      await screen.findByTestId("transcript-closed-notice"),
    ).toBeInTheDocument();
    expect(
      screen.queryByRole("button", { name: /transcript\.pauseAi/i }),
    ).not.toBeInTheDocument();
  });

  it("maps the conversation_closed 409 on reply to honest copy, not claimExpired", async () => {
    // The composer only exists for the actor holding the claim (decision #4).
    // useAuth is stubbed to the owner principal, whose held-claim shape carries
    // no staff id. This test is about the 409 mapping, not the claim gate.
    mockDefaultGetResponses({
      claimed_by_staff_id: null,
      claimed_by_name: "Owner",
      claimed_by_role: "owner",
      claimed_at: "2026-04-15T12:05:00Z",
    });
    render(
      <AiWaiterDashboard business={makeBusiness()} onUpdateBusiness={jest.fn()} />,
    );


    await goToMonitorTab();
    const viewChat = await screen.findByRole("button", {
      name: /monitor\.viewChat/i,
    });
    await act(async () => {
      fireEvent.click(viewChat);
    });

    const input = await screen.findByPlaceholderText(
      /transcript\.staffReplyPlaceholder/i,
    );
    fireEvent.change(input, { target: { value: "hola" } });
    const sendBtn = screen.getByRole("button", {
      name: /transcript\.sendReply/i,
    });
    mockedAxios.post.mockRejectedValueOnce(conversationClosed409());
    await act(async () => {
      fireEvent.click(sendBtn);
    });

    await waitFor(() => {
      expect(mockToastError).toHaveBeenCalledWith(
        "aiWaiterDashboard.transcript.conversationClosed",
      );
    });
    expect(mockToastError).not.toHaveBeenCalledWith(
      "aiWaiterDashboard.transcript.claimExpired",
    );
    expect(
      await screen.findByTestId("transcript-closed-notice"),
    ).toBeInTheDocument();
  });
});
