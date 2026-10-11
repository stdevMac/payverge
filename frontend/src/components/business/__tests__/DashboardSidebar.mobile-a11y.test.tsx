/** @jest-environment jsdom */
import React from "react";
import { render, screen } from "@testing-library/react";
import DashboardSidebar from "../DashboardSidebar";

jest.mock("@/i18n/SimpleTranslationProvider", () => {
  const actual = jest.requireActual("@/i18n/getTranslation");
  return {
    useSimpleLocale: () => ({ locale: "en", setLocale: jest.fn() }),
    getTranslation: actual.getTranslation,
  };
});

jest.mock("@/hooks/useBusinessAccess", () => ({
  useBusinessAccess: () => ({
    hasAccess: true,
    isSuspended: false,
    aiConfigured: true,
    loading: false,
    isError: false,
  }),
}));

jest.mock("../operational-alerts/useOperationalAlerts", () => ({
  useOptionalOperationalAlerts: () => null,
}));

jest.mock("@/hooks/useFailedInvoiceCount", () => ({
  useFailedInvoiceCount: () => 0,
}));
jest.mock("@/hooks/useChatUnreadCount", () => ({
  useChatUnreadCount: () => 0,
}));

jest.mock("next/image", () => ({
  __esModule: true,
  default: (props: Record<string, unknown>) => {
    // eslint-disable-next-line @next/next/no-img-element, jsx-a11y/alt-text
    return <img {...props} />;
  },
}));

function mockMobileNav() {
  Object.defineProperty(window, "matchMedia", {
    writable: true,
    value: (query: string) => ({
      matches: query.includes("max-width: 1023px"),
      media: query,
      addEventListener: () => {},
      removeEventListener: () => {},
      addListener: () => {},
      removeListener: () => {},
      dispatchEvent: () => false,
    }),
  });
}

const baseProps: React.ComponentProps<typeof DashboardSidebar> = {
  business: { id: 51, name: "AI Pro Lounge", custom_url: "ai-pro" } as never,
  activeTab: "overview",
  setActiveTab: jest.fn(),
  sidebarOpen: false,
  setSidebarOpen: jest.fn(),
  allowedTabs: [],
};

describe("DashboardSidebar — mobile drawer a11y (#435)", () => {
  beforeEach(() => {
    mockMobileNav();
  });

  it("hides the closed drawer from AT and uses modal semantics when open", () => {
    const { rerender } = render(
      <DashboardSidebar {...baseProps} sidebarOpen={false} />,
    );
    const closed = screen.getByTestId("dashboard-sidebar-root");
    expect(closed.className).toMatch(/max-lg:invisible/);
    expect(closed).toHaveAttribute("aria-hidden", "true");
    expect(closed.getAttribute("role")).not.toBe("dialog");

    rerender(<DashboardSidebar {...baseProps} sidebarOpen />);
    const open = screen.getByTestId("dashboard-sidebar-root");
    expect(open).toHaveAttribute("role", "dialog");
    expect(open).toHaveAttribute("aria-modal", "true");
    expect(open).toHaveAccessibleName(/navigation menu/i);
    expect(open).not.toHaveAttribute("aria-hidden", "true");
  });
});
