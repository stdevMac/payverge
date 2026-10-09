/** @jest-environment jsdom */
import React from "react";
import { act, fireEvent, render, screen, waitFor } from "@testing-library/react";

jest.mock("react-hot-toast", () => { const e = jest.fn(); const s = jest.fn(); const f = Object.assign(jest.fn(), { error: e, success: s }); return { __esModule: true, default: f, toast: f }; });
jest.mock("@/i18n/SimpleTranslationProvider", () => ({ useSimpleLocale: () => ({ locale: "en", setLocale: jest.fn() }), getTranslation: (key: string) => key }));
jest.mock("@/hooks/useBusinessAccess", () => ({ useBusinessAccess: () => ({ hasAccess: true, isSuspended: false, aiConfigured: true, loading: false }) }));
jest.mock("@/api/tools/instance", () => ({ axiosInstance: { get: jest.fn(), post: jest.fn(), put: jest.fn() } }));
// HybridAuthProvider transitively imports wagmi (ESM); stub useAuth to an
// owner principal (staffData: null) so the dashboard renders the full view.
jest.mock("@/providers/HybridAuthProvider", () => ({ useAuth: () => ({ staffData: null }) }));

import { axiosInstance } from "@/api/tools/instance";
import AiWaiterDashboard from "../AiWaiterDashboard";
import type { Business } from "@/api/business";

const mocked = axiosInstance as jest.Mocked<typeof axiosInstance>;
const business = { id: 1, ai_settings: { ai_name: "Sage", ai_enabled: true } } as unknown as Business;

describe("AiWaiterDashboard transcript error", () => {
  beforeEach(() => {
    jest.clearAllMocks();
    mocked.get.mockImplementation((url: string) => {
      if (url.includes("/conversations?")) return Promise.resolve({ data: { conversations: [{ id: 9, session_id: "s9", table_code: "T1", language: "en", status: "active", created_at: "", updated_at: "", message_count: 2, is_paused: false, cart_items_added: 0 }], total_pages: 1 } });
      if (url.includes("/insights")) return Promise.resolve({ data: { total_conversations: 1, total_messages: 2 } });
      if (url.includes("/whatsapp/status")) return Promise.resolve({ data: { status: "disconnected", enabled: false, built: false } });
      return Promise.resolve({ data: [] });
    });
  });

  it("shows a localized error + retry when the transcript fetch fails, then recovers", async () => {
    await act(async () => {
      render(<AiWaiterDashboard business={business} onUpdateBusiness={jest.fn()} />);
    });
    await waitFor(() => {
      expect(
        screen.getByText("aiWaiterDashboard.settings.personalityTitle"),
      ).toBeInTheDocument();
    });
    const { goToMonitorTab } = await import("./_goToMonitorTab");
    await goToMonitorTab();
    const viewBtn = await screen.findByRole("button", { name: /monitor\.viewChat/ });
    mocked.get.mockRejectedValueOnce(new Error("boom"));
    fireEvent.click(viewBtn);

    expect(await screen.findByText("aiWaiterDashboard.transcript.loadFailed")).toBeInTheDocument();
    const retry = screen.getByRole("button", { name: /aiWaiterDashboard.transcript.retry/i });

    mocked.get.mockResolvedValueOnce({ data: [{ id: 1, role: "user", content: "hi", created_at: "" }] });
    fireEvent.click(retry);
    await waitFor(() => expect(screen.getByText("hi")).toBeInTheDocument());
  });
});
