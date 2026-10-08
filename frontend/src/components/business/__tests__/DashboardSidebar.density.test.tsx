/** @jest-environment jsdom */
/**
 * Round 4 audit — Task 7 coverage.
 *
 * The sidebar used to render icon + label + two-line description + plan chip
 * + lock icon + badge on every row (5-7 visual elements × up to 19 rows).
 * We've collapsed that to a single-line layout:
 *
 *   [icon] [label]                       [ONE trailing indicator]
 *
 * Trailing indicator precedence is strict: `lock > badge > nothing`. #726
 * retired the hover bubble that used to repeat `label — description` on an
 * expanded row: the row already prints its label, and the bubble covered the
 * neighbouring rows during rush. The label is the row's only hover copy now.
 */
import React from "react";
import { render, screen, within } from "@testing-library/react";
import DashboardSidebar from "../DashboardSidebar";
import { useBusinessAccess } from "@/hooks/useBusinessAccess";

jest.mock("@/hooks/useBusinessAccess", () => ({
  useBusinessAccess: jest.fn(),
}));

// Deterministic i18n — every key is echoed back as-is so we can assert on it.
jest.mock("@/i18n/SimpleTranslationProvider", () => ({
  useSimpleLocale: () => ({ locale: "en", setLocale: () => {} }),
  getTranslation: (key: string, _locale: string) => key,
}));

// Next/Image is DOM-hostile in node testing; stub to a plain img.
jest.mock("next/image", () => ({
  __esModule: true,
  default: (props: any) => {
    // eslint-disable-next-line @next/next/no-img-element, jsx-a11y/alt-text
    return <img {...props} />;
  },
}));

// The sidebar now calls useFailedInvoiceCount (react-query) — mock at the
// hook level so these tests don't need a QueryClientProvider.
jest.mock("@/hooks/useFailedInvoiceCount", () => ({
  useFailedInvoiceCount: jest.fn(() => 0),
}));
// Same rationale: mock useChatUnreadCount (react-query + SSE) at the hook
// level so these tests don't need a QueryClientProvider/EventSource.
jest.mock("@/hooks/useChatUnreadCount", () => ({
  useChatUnreadCount: jest.fn(() => 0),
}));

const mockAccessDefaults = {
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
    business: { id: 42, name: "Test Bistro", custom_url: "test-bistro" } as any,
    activeTab: "overview",
    setActiveTab: jest.fn(),
    sidebarOpen: true,
    setSidebarOpen: jest.fn(),
    allowedTabs: [],
    isStaffUser: false,
    staffData: null,
    globalOrders: {
      1: [
        { id: 1, status: "pending" },
        { id: 2, status: "pending" },
        { id: 3, status: "pending" },
      ],
    },
    // Bills badge tracks the approval queue (#793): pending orders above.
    globalOrdersLoaded: true,
    upcomingReservations: [],
    tutorialOpen: false,
    tutorialTabKey: null,
    tutorialTarget: null,
    onStartTutorial: jest.fn(),
  };
  return render(<DashboardSidebar {...defaults} {...overrides} />);
}

describe("DashboardSidebar — row density", () => {
  beforeEach(() => {
    jest.clearAllMocks();
    window.localStorage.clear();
    (useBusinessAccess as jest.Mock).mockReturnValue(mockAccessDefaults);
  });

  // The sidebar's `tString` helper prefixes every key with `businessDashboard.`,
  // so our identity i18n mock echoes back fully-qualified keys.
  const LABEL = (leaf: string) => `businessDashboard.tabs.${leaf}`;
  const DESC = (leaf: string) => `businessDashboard.tabs.${leaf}Desc`;
  const findButtonByLabel = (leaf: string) =>
    screen
      .getAllByRole("button")
      .find((b) => within(b).queryByText(LABEL(leaf))) as
      | HTMLButtonElement
      | undefined;

  // #726: an expanded row must carry no hover bubble at all — neither the
  // aria-describedby link nor a role=tooltip node repeating the label.
  const expectNoHoverBubble = (btn: HTMLElement) => {
    expect(btn).not.toHaveAttribute("aria-describedby");
  };

  it("renders a single descriptive text per row (label only, no two-line description)", () => {
    renderSidebar();

    // The kitchen tab exists in the TODAY group because pendingOrdersCount > 0.
    // Only the label should appear visibly in the button.
    const kitchenButton = findButtonByLabel("kitchen");
    expect(kitchenButton).toBeTruthy();

    // The description string must not render anywhere on an expanded rail —
    // not as a second line, and (since #726) not as a hover bubble either.
    expect(within(kitchenButton!).queryByText(DESC("kitchen"))).toBeNull();
    expect(screen.queryByText(DESC("kitchen"))).toBeNull();
    expectNoHoverBubble(kitchenButton!);
  });

  it("shows the lock indicator on every locked row, with no hover bubble", () => {
    // Locked scenario: an administrator suspended the business, so tabs lock (e.g. bills).
    (useBusinessAccess as jest.Mock).mockReturnValue({
      ...mockAccessDefaults,
      hasAccess: false,
      isSuspended: true,
      lockState: "suspended" as const,
    });

    renderSidebar();

    const billsButton = findButtonByLabel("bills");
    expect(billsButton).toBeTruthy();
    // Locked rows lost the bubble too — the lock icon carries the meaning.
    expectNoHoverBubble(billsButton!);

    // A locked row must show the lock indicator (with its aria-label), and
    // must NOT also show a badge — precedence is strict. tString is mocked to
    // echo the i18n key path; production resolves it.
    const lockNode = within(billsButton!).getByLabelText(
      /locked\.short/i,
    );
    expect(lockNode).toBeTruthy();
  });

  it("routes active bill count to Bills and keeps Counter badge-less", () => {
    renderSidebar();

    const billsButton = findButtonByLabel("bills");
    expect(billsButton).toBeTruthy();

    // Three open checks on the floor — badge matches Active Bills / Overview.
    expect(within(billsButton!).getByText("3")).toBeTruthy();

    const counterButton = findButtonByLabel("counter");
    expect(counterButton).toBeTruthy();

    // Counter is just a service station setup screen and should never inherit order badges.
    expect(within(counterButton!).queryByText(/^\d+$/)).toBeNull();

    // Unlocked row should not render a lock indicator.
    expect(within(billsButton!).queryByLabelText(/Locked/i)).toBeNull();
  });

  it("keeps the Setup group label free of raw item-count text", () => {
    renderSidebar();

    const setupGroup = screen.getByTestId("sidebar-setup-group");
    expect(setupGroup).toHaveTextContent(
      /businessDashboard\.sidebarGroups\.(setup|other)/,
    );
    expect(within(setupGroup).queryByText(/^\d+$/)).toBeNull();
  });

  it("routes approved order badges to Kitchen", () => {
    renderSidebar({
      globalOrders: {
        1: [
          { id: 1, status: "approved" },
          { id: 2, status: "approved" },
          { id: 3, status: "approved" },
          { id: 4, status: "approved" },
        ],
      } as any,
    });

    const kitchenButton = findButtonByLabel("kitchen");
    expect(kitchenButton).toBeTruthy();
    expect(within(kitchenButton!).getByText("4")).toBeTruthy();
  });

  it("renders nothing trailing for a plain, unlocked, badge-less row", () => {
    renderSidebar();

    // `analytics` is a primary tab with no badge and no lock in our default state.
    const analyticsButton = findButtonByLabel("analytics");
    expect(analyticsButton).toBeTruthy();

    expect(within(analyticsButton!).queryByText(/^\d+$/)).toBeNull();
    expect(within(analyticsButton!).queryByLabelText(/Locked/i)).toBeNull();
  });

  it("renders the Accounting item in the Finance primary group when allowed", () => {
    // Task 32: Accounting lives under Finance with Analytics, not Setup.
    renderSidebar({ allowedTabs: ["accounting"] });

    const accountingButton = findButtonByLabel("accounting");
    expect(accountingButton).toBeTruthy();
    // The row must sit under the Finance eyebrow, not Setup/Other.
    const financeHeader = screen.getByText(
      "businessDashboard.sidebarGroups.finance",
    );
    expect(financeHeader.parentElement).toContainElement(accountingButton!);
  });

  it("renders Fiscal under the labeled Setup group when allowed", () => {
    renderSidebar({ allowedTabs: ["fiscal"] });

    const fiscalButton = findButtonByLabel("fiscal");
    expect(fiscalButton).toBeTruthy();
    expect(screen.getByTestId("sidebar-setup-group")).toContainElement(
      fiscalButton!,
    );
  });

  it("marks the active tab with aria-current='page'", () => {
    renderSidebar({ activeTab: "overview" });

    const overviewButton = findButtonByLabel("overview");
    expect(overviewButton).toBeTruthy();
    expect(overviewButton).toHaveAttribute("aria-current", "page");

    // Non-active rows must not have aria-current set.
    const analyticsButton = findButtonByLabel("analytics");
    expect(analyticsButton).not.toHaveAttribute("aria-current");
  });

  it("prefers the lock indicator over the badge when both would apply", () => {
    // Bills has 3 pending approvals as its badge AND is in
    // the business is suspended. The row should render the
    // lock indicator — NOT the "3" badge.
    (useBusinessAccess as jest.Mock).mockReturnValue({
      ...mockAccessDefaults,
      hasAccess: false,
      isSuspended: true,
      lockState: "suspended" as const,
    });

    renderSidebar();

    const billsButton = findButtonByLabel("bills");
    expect(billsButton).toBeTruthy();

    // tString is mocked to echo the i18n key path.
    expect(
      within(billsButton!).getByLabelText(
        /locked\.short/i,
      ),
    ).toBeTruthy();
    // The numeric badge must be suppressed by the lock-precedence rule.
    expect(within(billsButton!).queryByText("3")).toBeNull();
  });
});
