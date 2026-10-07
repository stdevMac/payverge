/** @jest-environment jsdom */
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
    cancelLabel,
    title,
    description,
    onConfirm,
    onOpenChange,
  }: {
    isOpen: boolean;
    confirmLabel?: string;
    cancelLabel?: string;
    title?: string;
    description?: string;
    onConfirm: () => void;
    onOpenChange: () => void;
  }) =>
    isOpen ? (
      <div data-testid="steal-confirm-modal">
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

const mockedAxios = axiosInstance as jest.Mocked<typeof axiosInstance>;

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

describe("AiWaiterDashboard steal confirmation", () => {
  beforeEach(() => {
    jest.clearAllMocks();
    jest.spyOn(console, "error").mockImplementation(() => {});
    (useBusinessAccess as jest.Mock).mockReturnValue({
      loading: false,
      hasAccess: true,
      isSuspended: false,
      aiConfigured: true,
      refetch: jest.fn(),
    });
    mockedAxios.get.mockImplementation((url: string) => {
      if (url.includes("/ai/conversations?")) {
        return Promise.resolve({
          data: { conversations: [SAMPLE_CONV], total_pages: 1 },
        });
      }
      if (url.includes("/whatsapp/status")) {
        return Promise.resolve({ data: { status: "disconnected", enabled: false, built: false } });
      }
      if (url.includes("/messages")) {
        return Promise.resolve({ data: [] });
      }
      if (url.includes("/ai/insights")) {
        return Promise.resolve({
          data: { total_conversations: 0, total_messages: 0, upsell_success_rate: 0 },
        });
      }
      return Promise.resolve({ data: {} });
    });
  });

  afterEach(() => {
    (console.error as jest.Mock).mockRestore?.();
  });

  it("renders ConfirmationModal and steals after confirm", async () => {
    mockedAxios.post.mockRejectedValueOnce({
      response: {
        status: 409,
        data: { code: "claim_steal_required", claimed_by_name: "Ana" },
      },
    });
    mockedAxios.post.mockResolvedValueOnce({
      data: { claimed_by_name: "Owner", claimed_by_staff_id: null },
    });

    render(
      <AiWaiterDashboard
        business={
          {
            id: 42,
            name: "Test Bistro",
            ai_settings: { ai_enabled: true, ai_name: "Sage" },
          } as never
        }
        onUpdateBusiness={jest.fn()}
      />,
    );
    await goToMonitorTab();

    fireEvent.click(await screen.findByRole("button", { name: /monitor\.pickUp/i }));

    const modal = await screen.findByTestId("steal-confirm-modal");
    expect(modal).toHaveTextContent(/stealRequired/);
    expect(screen.getByRole("button", { name: /stealConfirm/ })).toBeInTheDocument();
    expect(screen.getByRole("button", { name: /stealCancel/ })).toBeInTheDocument();

    await act(async () => {
      fireEvent.click(screen.getByRole("button", { name: /stealConfirm/ }));
    });

    await waitFor(() => {
      expect(mockedAxios.post).toHaveBeenLastCalledWith(
        "/inside/businesses/42/ai/conversations/7/claim",
        { steal: true },
      );
    });
  });
});
