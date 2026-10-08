/** @jest-environment jsdom */
/**
 * Stream A — nav orientation: activeTab === item.key drives exactly one
 * aria-current="page" row; labels match the deep-linked tab.
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
      "tabs.aiWaiter": "AI Waiter",
      "tabs.directorConsole": "Director Console",
      "tabs.marketing": "Marketing",
      "tabs.bills": "Bills",
      "tabs.kitchen": "Kitchen",
      "tabs.tables": "Tables",
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
      "tabs.fiscal": "Fiscal",
      "sidebar.badgeCount": "{count} in {label}",
      "sidebar.billsAlertBadge": "{count} orders awaiting approval",
      "sidebar.billsAlertBadge_one": "{count} order awaiting approval",
      "sidebar.billsAlertBadge_other": "{count} orders awaiting approval",
      "sidebar.kitchenReadyBadge": "{count} tickets to cook",
      "sidebar.kitchenReadyBadge_one": "{count} ticket to cook",
      "sidebar.kitchenReadyBadge_other": "{count} tickets to cook",
      "sidebar.activeServiceCalls": "Active service calls: {count}",
      "sidebar.occupiedTablesBadge": "{count} occupied tables",
      "sidebar.occupiedTablesBadge_one": "{count} occupied table",
      "sidebar.occupiedTablesBadge_other": "{count} occupied tables",
      "sidebarGroups.today": "Today",
      "sidebarGroups.operations": "Operations",
      "sidebarGroups.ai": "AI",
      "sidebarGroups.management": "Management",
      "sidebarGroups.financeAndSettings": "Finance & Settings",
      "sidebarGroups.more": "More",
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

describe("DashboardSidebar — active tab aria-current (Stream A)", () => {
  beforeEach(() => {
    jest.clearAllMocks();
    window.localStorage.clear();
    (useBusinessAccess as jest.Mock).mockReturnValue(mockTierDefaults);
    // jsdom has no real scroll; stub so scroll-into-view effect does not throw
    Element.prototype.scrollIntoView = jest.fn();
  });

  it.each([
    ["ai-waiter", "AI Waiter"],
    ["director-console", "Director Console"],
    ["marketing", "Marketing"],
  ] as const)(
    "deep-link activeTab=%s marks exactly one aria-current page named %s",
    (tab, label) => {
      renderSidebar({ activeTab: tab });

      const current = screen.getAllByRole("button").filter(
        (el) => el.getAttribute("aria-current") === "page",
      );
      expect(current).toHaveLength(1);
      expect(within(current[0]).getByText(label)).toBeInTheDocument();
    },
  );

  it("scrolls the active row into view when activeTab changes (non-tutorial)", () => {
    const scrollIntoView = jest.fn();
    Element.prototype.scrollIntoView = scrollIntoView;

    const { rerender } = renderSidebar({ activeTab: "overview" });
    expect(scrollIntoView).toHaveBeenCalled();

    scrollIntoView.mockClear();
    rerender(
      <DashboardSidebar
        business={{ id: 51, name: "AI Pro Lounge", custom_url: "ai-pro" } as any}
        activeTab="director-console"
        setActiveTab={jest.fn()}
        sidebarOpen
        setSidebarOpen={jest.fn()}
        allowedTabs={[]}
        isStaffUser={false}
        staffData={null}
        globalOrders={{}}
        upcomingReservations={[]}
        tutorialOpen={false}
        tutorialTabKey={null}
        tutorialTarget={null}
        onStartTutorial={jest.fn()}
      />,
    );
    expect(scrollIntoView).toHaveBeenCalledWith(
      expect.objectContaining({ block: "nearest" }),
    );
  });
});

describe("DashboardSidebar — badge aria semantics (Stream B)", () => {
  beforeEach(() => {
    jest.clearAllMocks();
    window.localStorage.clear();
    (useBusinessAccess as jest.Mock).mockReturnValue(mockTierDefaults);
    Element.prototype.scrollIntoView = jest.fn();
  });

  it("labels bills badge as the approval queue (#793, not open checks)", () => {
    renderSidebar({
      activeTab: "bills",
      globalOrdersLoaded: true,
      // 3 pending + 1 approved: Bills counts approvals, Kitchen counts
      // pending + approved (tickets waiting on the cook, #792).
      globalOrders: {
        1: [
          { id: 1, status: "pending" },
          { id: 2, status: "pending" },
          { id: 3, status: "pending" },
          { id: 4, status: "approved" },
        ],
      },
    });

    expect(
      screen.getByLabelText("3 orders awaiting approval"),
    ).toBeInTheDocument();
    expect(
      screen.queryByLabelText(/active bills/i),
    ).not.toBeInTheDocument();
    expect(screen.getByLabelText("4 tickets to cook")).toBeInTheDocument();
  });

  it("uses singular bills badge name at exact count 1", () => {
    renderSidebar({
      activeTab: "bills",
      globalOrdersLoaded: true,
      globalOrders: { 1: [{ id: 1, status: "pending" }] },
    });

    expect(
      screen.getByLabelText("1 order awaiting approval"),
    ).toBeInTheDocument();
    expect(
      screen.queryByLabelText("1 orders awaiting approval"),
    ).not.toBeInTheDocument();
  });

  it("labels kitchen badge as tickets to cook (#792 pending + approved)", () => {
    renderSidebar({
      activeTab: "kitchen",
      globalOrdersLoaded: true,
      globalOrders: {
        1: [
          { id: 1, status: "pending" },
          { id: 2, status: "approved" },
          { id: 3, status: "approved" },
          { id: 4, status: "in_kitchen" },
        ],
      },
    });

    expect(screen.getByLabelText("3 tickets to cook")).toBeInTheDocument();
  });

  it("labels tables badge as occupied tables when occupancy is on the floor (#774)", () => {
    renderSidebar({
      activeTab: "tables",
      globalOrdersLoaded: true,
      globalOrders: {
        1: [
          { id: 1, status: "pending" },
          { id: 2, status: "pending" },
        ],
      },
      occupiedTablesCount: 5,
    });

    expect(screen.getByLabelText("5 occupied tables")).toBeInTheDocument();
    expect(
      screen.getByLabelText("2 orders awaiting approval"),
    ).toBeInTheDocument();
    expect(
      screen.queryByLabelText("Active service calls: 5"),
    ).not.toBeInTheDocument();
  });
});
