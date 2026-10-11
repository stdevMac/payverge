/** @jest-environment jsdom */
import React from "react";
import { act, fireEvent, render, screen } from "@testing-library/react";

jest.mock("react-hot-toast", () => {
  const toastFn = Object.assign(jest.fn(), { error: jest.fn(), success: jest.fn() });
  return { __esModule: true, default: toastFn, toast: toastFn };
});

jest.mock("@/i18n/SimpleTranslationProvider", () => ({
  useSimpleLocale: () => ({ locale: "en", setLocale: jest.fn() }),
  getTranslation: (key: string) => key,
}));

jest.mock("@/api/tools/instance", () => ({
  axiosInstance: { get: jest.fn(), post: jest.fn(), put: jest.fn() },
}));

jest.mock("@/hooks/useBusinessAccess", () => ({ useBusinessAccess: jest.fn() }));
jest.mock("../AiWaiterToggle", () => ({ __esModule: true, default: () => null }));
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
  message_count: 1,
  is_paused: true,
  cart_items_added: 0,
  claimed_by_staff_id: null,
  claimed_by_name: "Owner",
  claimed_by_role: "owner",
  claimed_at: "2026-04-15T12:05:00Z",
};

function setDocumentHidden(hidden: boolean) {
  Object.defineProperty(document, "hidden", {
    configurable: true,
    get: () => hidden,
  });
  Object.defineProperty(document, "visibilityState", {
    configurable: true,
    get: () => (hidden ? "hidden" : "visible"),
  });
  act(() => {
    document.dispatchEvent(new Event("visibilitychange"));
  });
}

function messageGets() {
  return mockedAxios.get.mock.calls.filter(([url]) =>
    String(url).includes("/messages"),
  ).length;
}

describe("AiWaiterDashboard transcript poll visibility", () => {
  beforeEach(() => {
    jest.useFakeTimers({ advanceTimers: true });
    setDocumentHidden(false);
    jest.clearAllMocks();
    jest.spyOn(console, "error").mockImplementation(() => {});
    jest.spyOn(console, "warn").mockImplementation(() => {});
    (useBusinessAccess as jest.Mock).mockReturnValue({
      loading: false,
      hasAccess: true,
      isSuspended: false,
      aiConfigured: true,
      refetch: jest.fn(),
    });
    mockedAxios.get.mockImplementation((url: string) => {
      if (url.includes("/messages")) {
        return Promise.resolve({
          data: [
            {
              id: 1,
              role: "user",
              content: "Hi",
              created_at: "2026-04-15T12:00:00Z",
            },
          ],
        });
      }
      if (url.includes("/ai/conversations?")) {
        return Promise.resolve({
          data: { conversations: [SAMPLE_CONV], total_pages: 1 },
        });
      }
      if (url.includes("/whatsapp/status")) {
        return Promise.resolve({ data: { status: "disconnected", enabled: false, built: false } });
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
    setDocumentHidden(false);
    jest.useRealTimers();
    (console.error as jest.Mock).mockRestore?.();
    (console.warn as jest.Mock).mockRestore?.();
  });

  async function openTranscript() {
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
    const { goToMonitorTab } = await import("./_goToMonitorTab");
    await goToMonitorTab();
    const viewChat = await screen.findByRole("button", { name: /monitor\.viewChat/i });
    await act(async () => {
      fireEvent.click(viewChat);
    });
    await screen.findByText("Hi");
  }

  it("does not poll the transcript while the tab is hidden", async () => {
    await openTranscript();
    const afterOpen = messageGets();

    setDocumentHidden(true);

    await act(async () => {
      jest.advanceTimersByTime(12_000);
      await Promise.resolve();
    });

    expect(messageGets()).toBe(afterOpen);
  });
});
