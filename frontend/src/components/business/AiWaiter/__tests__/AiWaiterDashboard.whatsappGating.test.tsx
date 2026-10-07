/** @jest-environment jsdom */
import React from "react";
import { act, render, screen, waitFor } from "@testing-library/react";

// The WhatsApp channel is compiled out of the default backend image (the
// `whatsapp` Go build tag links GPL-3.0 code; see docs/self-hosting/whatsapp.md).
// GET /whatsapp/status reports that as built=false and the dashboard must not
// mount any WhatsApp entry point.
let mockWhatsAppStatus: Record<string, unknown> = {};

jest.mock("react-hot-toast", () => { const e = jest.fn(); const s = jest.fn(); const f = Object.assign(jest.fn(), { error: e, success: s }); return { __esModule: true, default: f, toast: f }; });
jest.mock("@/i18n/SimpleTranslationProvider", () => ({ useSimpleLocale: () => ({ locale: "en", setLocale: jest.fn() }), getTranslation: (key: string) => key }));
jest.mock("@/hooks/useBusinessAccess", () => ({ useBusinessAccess: () => ({ hasAccess: true, isSuspended: false, aiConfigured: true, loading: false }) }));
jest.mock("@/api/tools/instance", () => {
  const m = { axiosInstance: { get: jest.fn(), post: jest.fn(), put: jest.fn() } };
  (m.axiosInstance.get as jest.Mock).mockImplementation((url: string) => {
    if (url.includes("/conversations?")) return Promise.resolve({ data: { conversations: [], total_pages: 1 } });
    if (url.includes("/insights")) return Promise.resolve({ data: { total_conversations: 0, total_messages: 0 } });
    if (url.includes("/whatsapp/status")) return Promise.resolve({ data: mockWhatsAppStatus });
    return Promise.resolve({ data: [] });
  });
  return m;
});
// HybridAuthProvider transitively imports wagmi (ESM); stub useAuth to an
// owner principal (staffData: null) so the dashboard renders the full view.
jest.mock("@/providers/HybridAuthProvider", () => ({ useAuth: () => ({ staffData: null }) }));

import AiWaiterDashboard from "../AiWaiterDashboard";
import { axiosInstance } from "@/api/tools/instance";

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
    ai_priority: "balanced",
    special_instructions: "",
    business_page_ai_enabled: false,
  },
} as any;

const mockedGet = axiosInstance.get as jest.Mock;

async function renderOverview() {
  render(<AiWaiterDashboard business={business} onUpdateBusiness={jest.fn()} />);
  // Overview is the default tab; the guardrail card proves it rendered.
  expect(await screen.findByTestId("ai-allergen-guardrail")).toBeInTheDocument();
  await waitFor(() =>
    expect(mockedGet).toHaveBeenCalledWith("/inside/businesses/1/whatsapp/status"),
  );
}

describe("AiWaiterDashboard WhatsApp build gating", () => {
  beforeEach(() => jest.clearAllMocks());

  it("hides every WhatsApp entry point on the default (untagged) backend", async () => {
    mockWhatsAppStatus = { status: "disconnected", enabled: false, built: false };
    await renderOverview();
    // Give the status response time to land, then assert nothing appeared.
    await act(async () => {
      await new Promise((resolve) => setTimeout(resolve, 0));
    });
    expect(screen.queryByTestId("whatsapp-channel-status")).not.toBeInTheDocument();
    expect(screen.queryByText("aiWaiterDashboard.whatsapp.title")).not.toBeInTheDocument();
    expect(screen.queryByText("aiWaiterDashboard.whatsapp.connect")).not.toBeInTheDocument();
    expect(document.body.textContent || "").not.toMatch(/whatsapp/i);
  });

  it("shows the channel card when the backend runs the whatsapp build", async () => {
    mockWhatsAppStatus = { status: "disconnected", enabled: true, built: true };
    await renderOverview();
    expect(await screen.findByTestId("whatsapp-channel-status")).toBeInTheDocument();
    expect(screen.getByText("aiWaiterDashboard.whatsapp.connect")).toBeInTheDocument();
  });
});
