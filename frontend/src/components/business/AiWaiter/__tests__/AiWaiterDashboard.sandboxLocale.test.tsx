/** @jest-environment jsdom */
import React from "react";
import { fireEvent, render, screen } from "@testing-library/react";
import { resolveAiWaiterSandboxLocale } from "../resolveAiWaiterSandboxLocale";

jest.mock("react-hot-toast", () => {
  const errorFn = jest.fn();
  const successFn = jest.fn();
  const toastFn = Object.assign(jest.fn(), {
    error: errorFn,
    success: successFn,
  });
  return { __esModule: true, default: toastFn, toast: toastFn };
});

jest.mock("@/i18n/SimpleTranslationProvider", () => ({
  useSimpleLocale: () => ({ locale: "es-AR", setLocale: jest.fn() }),
  getTranslation: (key: string) => key,
}));

jest.mock("@/hooks/useBusinessAccess", () => ({
  useBusinessAccess: () => ({
    hasAccess: true,
    isSuspended: false,
    aiConfigured: true,
    loading: false,
  }),
}));

jest.mock("@/api/tools/instance", () => {
  const m = {
    axiosInstance: { get: jest.fn(), post: jest.fn(), put: jest.fn() },
  };
  (m.axiosInstance.get as jest.Mock).mockImplementation((url: string) => {
    if (url.includes("/conversations?"))
      return Promise.resolve({ data: { conversations: [], total_pages: 1 } });
    if (url.includes("/insights"))
      return Promise.resolve({
        data: { total_conversations: 0, total_messages: 0 },
      });
    return Promise.resolve({ data: [] });
  });
  return m;
});

jest.mock("@/providers/HybridAuthProvider", () => ({
  useAuth: () => ({ staffData: null }),
}));

jest.mock("@/api/business", () => ({
  __esModule: true,
  getMenu: jest.fn().mockResolvedValue({ categories: [] }),
}));

jest.mock("../AiWaiterTestChat", () => ({
  __esModule: true,
  default: ({ language }: { language: string }) => (
    <div data-testid="sandbox-language">{language}</div>
  ),
}));

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
  default_language: "en",
  ai_settings: {
    ai_enabled: true,
    ai_name: "Sage",
    ai_priority: "balanced",
    special_instructions: "",
    business_page_enabled: false,
  },
} as any;

describe("owner sandbox locale contract", () => {
  it("prefers the active owner locale over the business default", () => {
    expect(resolveAiWaiterSandboxLocale("es-AR", "en")).toBe("es-AR");
    expect(resolveAiWaiterSandboxLocale("es", "en")).toBe("es");
    expect(resolveAiWaiterSandboxLocale("en", "es-AR")).toBe("en");
    expect(resolveAiWaiterSandboxLocale("", "es-AR")).toBe("es-AR");
    expect(resolveAiWaiterSandboxLocale(undefined, undefined)).toBe("en");
  });

  it("wires the dashboard sandbox to the owner locale, not the business default", async () => {
    render(
      <AiWaiterDashboard business={business} onUpdateBusiness={jest.fn()} />,
    );
    const overviewTab = await screen.findByRole("tab", {
      name: /tabs\.overview/i,
    });
    fireEvent.click(overviewTab);
    expect(await screen.findByTestId("sandbox-language")).toHaveTextContent(
      "es-AR",
    );
  });
});
