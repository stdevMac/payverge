/** @jest-environment jsdom */
import React from "react";
import { render, screen, waitFor } from "@testing-library/react";

jest.mock("react-hot-toast", () => { const e = jest.fn(); const s = jest.fn(); const f = Object.assign(jest.fn(), { error: e, success: s }); return { __esModule: true, default: f, toast: f }; });
jest.mock("@/i18n/SimpleTranslationProvider", () => ({ useSimpleLocale: () => ({ locale: "en", setLocale: jest.fn() }), getTranslation: (key: string) => key }));
jest.mock("@/hooks/useBusinessAccess", () => ({ useBusinessAccess: () => ({ hasAccess: true, isSuspended: false, aiConfigured: true, loading: false }) }));
jest.mock("@/api/tools/instance", () => {
  const m = { axiosInstance: { get: jest.fn(), post: jest.fn(), put: jest.fn() } };
  (m.axiosInstance.get as jest.Mock).mockImplementation((url: string) => {
    if (url.includes("/conversations?")) return Promise.resolve({ data: { conversations: [], total_pages: 1 } });
    if (url.includes("/insights")) return Promise.resolve({ data: { total_conversations: 0, total_messages: 0 } });
    if (url.includes("/whatsapp/status")) return Promise.resolve({ data: { status: "disconnected", enabled: false, built: false } });
    return Promise.resolve({ data: [] });
  });
  return m;
});
// HybridAuthProvider transitively imports wagmi (ESM); stub useAuth to an
// owner principal (staffData: null) so the dashboard renders the full view.
jest.mock("@/providers/HybridAuthProvider", () => ({ useAuth: () => ({ staffData: null }) }));

import AiWaiterDashboard from "../AiWaiterDashboard";

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

describe("AiWaiterDashboard special-instructions warning", () => {
  beforeEach(() => jest.clearAllMocks());

  it("renders the owner-facing guest-visibility warning under the special-instructions field", async () => {
    render(<AiWaiterDashboard business={business} onUpdateBusiness={jest.fn()} />);
    expect(await screen.findByText("aiWaiterDashboard.title")).toBeInTheDocument();
    // Overview is the default landing tab — guardrails should be visible without a click.
    await waitFor(() => {
      expect(screen.getByText("aiWaiterDashboard.settings.specialInstructionsWarning")).toBeInTheDocument();
    });
    expect(screen.getByTestId("ai-allergen-guardrail")).toBeInTheDocument();
    expect(screen.getByTestId("ai-waiter-test-chat")).toBeInTheDocument();
  });
});
