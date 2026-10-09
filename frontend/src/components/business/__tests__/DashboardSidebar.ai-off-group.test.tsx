/** @jest-environment jsdom */
/**
 * With features.ai=false the AI-model tabs are hidden, so the sidebar must not
 * show an "AI" header over Marketing alone: Marketing folds into Management.
 */
import React from "react";
import { render, screen } from "@testing-library/react";
import DashboardSidebar from "../DashboardSidebar";
import { useBusinessAccess } from "@/hooks/useBusinessAccess";

jest.mock("@/hooks/useBusinessAccess", () => ({
  useBusinessAccess: jest.fn(),
}));

const mockInstance: { current: unknown } = { current: null };
jest.mock("@/hooks/useInstance", () => ({
  useInstance: () => ({ instance: mockInstance.current }),
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

function instanceWithAI(ai: boolean) {
  return {
    features: {
      ai,
      whatsapp: true,
      telegram: true,
      email: true,
      google_oauth: true,
      crypto: true,
      fiscal_ar: true,
    },
  };
}

describe("DashboardSidebar — AI group with AI off", () => {
  beforeEach(() => {
    jest.clearAllMocks();
    window.localStorage.clear();
    (useBusinessAccess as jest.Mock).mockReturnValue(mockTierDefaults);
    Element.prototype.scrollIntoView = jest.fn();
  });

  it("shows the AI header when the instance has AI", () => {
    mockInstance.current = instanceWithAI(true);
    renderSidebar();
    expect(screen.getByText("AI")).toBeInTheDocument();
    expect(screen.getAllByText("AI Waiter").length).toBeGreaterThan(0);
  });

  it("drops the lone AI header and keeps Marketing under Management", () => {
    mockInstance.current = instanceWithAI(false);
    renderSidebar();
    expect(screen.queryByText("AI")).not.toBeInTheDocument();
    expect(screen.queryAllByText("AI Waiter")).toHaveLength(0);
    const management = screen.getByText("Management").parentElement as HTMLElement;
    expect(
      Array.from(management.querySelectorAll("button")).some((b) =>
        (b.textContent ?? "").includes("Marketing"),
      ),
    ).toBe(true);
  });
});
