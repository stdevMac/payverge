/** @jest-environment jsdom */
/**
 * #60 — Setup destinations stay as discoverable as Bills: labeled group
 * chrome (not a collapsed disclosure), items visible on first paint.
 */
import React from "react";
import { render, screen, within } from "@testing-library/react";
import DashboardSidebar from "../DashboardSidebar";
import { useBusinessAccess } from "@/hooks/useBusinessAccess";

jest.mock("@/hooks/useBusinessAccess", () => ({
  useBusinessAccess: jest.fn(),
}));

jest.mock("@/i18n/SimpleTranslationProvider", () => ({
  useSimpleLocale: () => ({ locale: "en", setLocale: () => {} }),
  getTranslation: (key: string) => {
    const leaf = key.replace(/^businessDashboard\./, "");
    const labels: Record<string, string> = {
      "tabs.overview": "Overview",
      "tabs.bills": "Bills",
      "tabs.kitchen": "Kitchen",
      "tabs.reservations": "Reservations",
      "tabs.cashRegister": "Cash Register",
      "tabs.menu": "Menu",
      "tabs.analytics": "Analytics",
      "tabs.accounting": "Accounting",
      "tabs.crm": "CRM",
      "tabs.staff": "Team",
      "tabs.businessPage": "Business Page",
      "tabs.plugins": "Plugins",
      "tabs.counter": "Counters",
      "tabs.printers": "Printers",
      "tabs.inventory": "Inventory",
      "tabs.delivery": "Delivery",
      "tabs.schedule": "Schedule",
      "tabs.settings": "Settings",
      "tabs.fiscal": "Invoices",
      "tabs.aiWaiter": "AI Waiter",
      "tabs.directorConsole": "Director Console",
      "tabs.marketing": "Marketing",
      "sidebarGroups.operations": "Operations",
      "sidebarGroups.ai": "AI",
      "sidebarGroups.management": "Management",
      "sidebarGroups.finance": "Finance",
      "sidebarGroups.setup": "Setup",
      "sidebarGroups.other": "Other",
      "sidebar.badgeCount": "{count} in {label}",
      "sidebar.billsAlertBadge": "{count} active bills",
      "sidebar.billsAlertBadge_one": "{count} active bill",
      "sidebar.billsAlertBadge_other": "{count} active bills",
      "sidebar.kitchenReadyBadge": "{count} kitchen-ready orders",
      "sidebar.kitchenReadyBadge_one": "{count} kitchen-ready order",
      "sidebar.kitchenReadyBadge_other": "{count} kitchen-ready orders",
      "sidebar.activeServiceCalls": "Active service calls: {count}",
      "sidebar.occupiedTablesBadge": "{count} occupied tables",
      "sidebar.occupiedTablesBadge_one": "{count} occupied table",
      "sidebar.occupiedTablesBadge_other": "{count} occupied tables",
      "locked.short": "Locked",
      "locked.requires": "Locked — requires {plan}",
    };
    return labels[leaf] ?? key;
  },
}));

jest.mock("next/image", () => ({
  __esModule: true,
  default: (props: Record<string, unknown>) => {
    // eslint-disable-next-line @next/next/no-img-element, jsx-a11y/alt-text
    return <img {...props} />;
  },
}));

jest.mock("@/hooks/useFailedInvoiceCount", () => ({
  useFailedInvoiceCount: jest.fn(() => 0),
}));
jest.mock("@/hooks/useChatUnreadCount", () => ({
  useChatUnreadCount: jest.fn(() => 0),
}));

const mockTierDefaults = {
  access: null,
  loading: false,
  error: null,
  hasAccess: true,
  isSuspended: false,
  lockState: "active" as const,
  aiConfigured: true,
  refetch: jest.fn(),
};

function renderSidebar(
  overrides: Partial<React.ComponentProps<typeof DashboardSidebar>> = {},
) {
  const defaults: React.ComponentProps<typeof DashboardSidebar> = {
    business: { id: 51, name: "AI Pro Lounge", custom_url: "ai-pro" } as any,
    activeTab: "overview",
    setActiveTab: jest.fn(),
    sidebarOpen: true,
    setSidebarOpen: jest.fn(),
    allowedTabs: [],
    isStaffUser: false,
    staffData: null,
    globalOrders: {},
    upcomingReservations: [],
    tutorialOpen: false,
    tutorialTabKey: null,
    tutorialTarget: null,
    onStartTutorial: jest.fn(),
  };
  return render(<DashboardSidebar {...defaults} {...overrides} />);
}

describe("DashboardSidebar — Setup labeled group (#60)", () => {
  beforeEach(() => {
    jest.clearAllMocks();
    window.localStorage.clear();
    (useBusinessAccess as jest.Mock).mockReturnValue(mockTierDefaults);
    Element.prototype.scrollIntoView = jest.fn();
  });

  it("renders Setup with the same eyebrow chrome as Operations, not a disclosure", () => {
    renderSidebar();

    expect(screen.queryByTestId("sidebar-setup-disclosure")).not.toBeInTheDocument();
    const setup = screen.getByTestId("sidebar-setup-group");
    expect(within(setup).getByText("Setup")).toBeInTheDocument();
    expect(
      screen.queryByRole("button", { name: /^Setup$/i }),
    ).not.toBeInTheDocument();

    const operations = screen.getByText("Operations");
    expect(operations.tagName).toBe("DIV");
    expect(operations.className).toMatch(/uppercase/);
    expect(within(setup).getByText("Setup").className).toBe(operations.className);
  });

  it("shows Settings / Plugins / Invoices / Business Page without a click", () => {
    renderSidebar();

    const setup = screen.getByTestId("sidebar-setup-group");
    for (const label of [
      "Settings",
      "Plugins",
      "Invoices",
      "Business Page",
    ]) {
      // M5: the label also appears inside the row's hover tooltip bubble —
      // assert the visible one (inside the row's button) is present.
      const matches = within(setup).getAllByText(label);
      expect(matches.some((el) => el.closest("button"))).toBe(true);
    }
  });

  it("keeps Setup items visible when a primary tab is active", () => {
    renderSidebar({ activeTab: "bills" });

    const matches = within(
      screen.getByTestId("sidebar-setup-group"),
    ).getAllByText("Settings");
    expect(matches.some((el) => el.closest("button"))).toBe(true);
  });
});
