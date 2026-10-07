/** @jest-environment jsdom */
import React from "react";
import { fireEvent, render, screen, within } from "@testing-library/react";
import DashboardSidebar from "../DashboardSidebar";
import { useBusinessAccess } from "@/hooks/useBusinessAccess";

jest.mock("@/hooks/useBusinessAccess", () => ({ useBusinessAccess: jest.fn() }));

// Identity i18n — keys echo back so we can assert on them.
jest.mock("@/i18n/SimpleTranslationProvider", () => ({
  useSimpleLocale: () => ({ locale: "en", setLocale: () => {} }),
  getTranslation: (key: string) => key,
}));

jest.mock("next/image", () => ({
  __esModule: true,
  // eslint-disable-next-line @next/next/no-img-element, jsx-a11y/alt-text
  default: (props: any) => <img {...props} />,
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
      ],
    },
    globalOrdersLoaded: true,
    upcomingReservations: [],
    tutorialOpen: false,
    tutorialTabKey: null,
    tutorialTarget: null,
    onStartTutorial: jest.fn(),
  };
  return render(<DashboardSidebar {...defaults} {...overrides} />);
}

// The sidebar root is the nearest ancestor carrying the width class.
function getSidebarRoot(): HTMLElement {
  const el = document.querySelector('[data-testid="dashboard-sidebar-root"]');
  if (!el) throw new Error("sidebar root not found");
  return el as HTMLElement;
}

describe("DashboardSidebar — collapse toggle", () => {
  beforeEach(() => {
    jest.clearAllMocks();
    window.localStorage.clear();
    (useBusinessAccess as jest.Mock).mockReturnValue(mockTierDefaults);
  });

  it("starts expanded (lg:w-72) and exposes a collapse toggle", () => {
    renderSidebar();
    expect(getSidebarRoot().className).toContain("lg:w-72");
    expect(getSidebarRoot().className).not.toContain("lg:w-16");

    const toggle = screen.getByRole("button", {
      name: /businessDashboard\.sidebar\.collapse/i,
    });
    expect(toggle).toHaveAttribute("aria-expanded", "true");
  });

  it("collapses to the icon rail (lg:w-16) when the toggle is clicked", () => {
    renderSidebar();
    fireEvent.click(
      screen.getByRole("button", {
        name: /businessDashboard\.sidebar\.collapse/i,
      }),
    );

    expect(getSidebarRoot().className).toContain("lg:w-16");
    expect(getSidebarRoot().className).not.toContain("lg:w-72");

    const toggle = screen.getByRole("button", {
      name: /businessDashboard\.sidebar\.expand/i,
    });
    expect(toggle).toHaveAttribute("aria-expanded", "false");
  });

  it("persists collapsed state to per-business localStorage", () => {
    renderSidebar();
    fireEvent.click(
      screen.getByRole("button", {
        name: /businessDashboard\.sidebar\.collapse/i,
      }),
    );
    expect(window.localStorage.getItem("payverge_sidebar_collapsed:42")).toBe(
      "true",
    );
  });
});

describe("DashboardSidebar — keyboard shortcut", () => {
  beforeEach(() => {
    jest.clearAllMocks();
    window.localStorage.clear();
    (useBusinessAccess as jest.Mock).mockReturnValue(mockTierDefaults);
  });

  it("toggles collapse on Meta+\\", () => {
    renderSidebar();
    expect(getSidebarRoot().className).toContain("lg:w-72");

    fireEvent.keyDown(window, { key: "\\", metaKey: true });
    expect(getSidebarRoot().className).toContain("lg:w-16");

    fireEvent.keyDown(window, { key: "\\", metaKey: true });
    expect(getSidebarRoot().className).toContain("lg:w-72");
  });

  it("ignores the shortcut while typing in an input", () => {
    renderSidebar();
    const input = document.createElement("input");
    document.body.appendChild(input);
    input.focus();

    fireEvent.keyDown(window, { key: "\\", metaKey: true });
    expect(getSidebarRoot().className).toContain("lg:w-72"); // unchanged

    document.body.removeChild(input);
  });

  it("ignores the shortcut while the tutorial is open", () => {
    renderSidebar({ tutorialOpen: true });
    expect(getSidebarRoot().className).toContain("lg:w-72");
    fireEvent.keyDown(window, { key: "\\", metaKey: true });
    expect(getSidebarRoot().className).toContain("lg:w-72"); // unchanged
  });
});

describe("DashboardSidebar — collapsed row rendering", () => {
  beforeEach(() => {
    jest.clearAllMocks();
    window.localStorage.clear();
    (useBusinessAccess as jest.Mock).mockReturnValue(mockTierDefaults);
  });

  const findButtonByLabel = (leaf: string) =>
    screen
      .getAllByRole("button")
      .find((b) => within(b).queryByText(`businessDashboard.tabs.${leaf}`)) as
      | HTMLButtonElement
      | undefined;

  it("centers the icon and hides the label when collapsed", () => {
    renderSidebar();
    fireEvent.click(
      screen.getByRole("button", {
        name: /businessDashboard\.sidebar\.collapse/i,
      }),
    );

    const overview = findButtonByLabel("overview")!;
    expect(overview.className).toContain("lg:justify-center");

    const label = within(overview).getByText("businessDashboard.tabs.overview");
    expect(label.className).toContain("lg:w-0");
    expect(label.className).toContain("lg:opacity-0");

    // Mobile-safety invariant: the base gap must survive collapse so the mobile
    // drawer (which renders the same JSX when collapsed persists) keeps its gap.
    const innerSpan = overview.querySelector("span.flex.items-center");
    expect(innerSpan?.className).toContain("gap-3");
  });

  it("uses the tab label as the tooltip when collapsed", () => {
    renderSidebar();
    fireEvent.click(
      screen.getByRole("button", {
        name: /businessDashboard\.sidebar\.collapse/i,
      }),
    );
    const overview = findButtonByLabel("overview")!;
    // M5: styled tooltip (aria-describedby → role=tooltip), not native title.
    const tipId = overview.getAttribute("aria-describedby");
    expect(tipId).toBeTruthy();
    const tip = document.getElementById(tipId!);
    expect(tip?.textContent).toBe("businessDashboard.tabs.overview");
    // #744: the ink-950 bubble must live on document.body, not inside the
    // overflow-hidden / overflow-y-auto nav that used to clip it to a sliver.
    const overflowClip = getSidebarRoot().querySelector(
      ".flex-1.overflow-hidden",
    );
    const nav = getSidebarRoot().querySelector("nav");
    expect(overflowClip).toBeTruthy();
    expect(nav).toBeTruthy();
    expect(overflowClip!.contains(tip)).toBe(false);
    expect(nav!.contains(tip)).toBe(false);
    expect(document.body.contains(tip)).toBe(true);
    expect(tip?.className).toContain("fixed");
  });
});

describe("DashboardSidebar — rail badge (icon → number swap)", () => {
  beforeEach(() => {
    jest.clearAllMocks();
    window.localStorage.clear();
    (useBusinessAccess as jest.Mock).mockReturnValue(mockTierDefaults);
  });

  const collapse = () =>
    fireEvent.click(
      screen.getByRole("button", {
        name: /businessDashboard\.sidebar\.collapse/i,
      }),
    );

  it("shows the count as a rail badge when collapsed (Bills = 2 pending)", () => {
    renderSidebar(); // default globalOrders => 2 pending approvals
    collapse();
    const railBadge = screen.getByTestId("rail-badge-bills");
    expect(railBadge).toHaveTextContent("2");
  });

  it("does not render a rail badge for a count-less tab", () => {
    renderSidebar();
    collapse();
    expect(screen.queryByTestId("rail-badge-overview")).toBeNull();
  });

  it("does not render rail badges when expanded", () => {
    renderSidebar();
    expect(screen.queryByTestId("rail-badge-bills")).toBeNull();
  });

  it("caps a large count at 9+", () => {
    renderSidebar({
      globalOrders: {
        1: Array.from({ length: 12 }, (_, i) => ({
          id: i + 1,
          status: "pending",
        })),
      },
    });
    collapse();
    expect(screen.getByTestId("rail-badge-bills")).toHaveTextContent("9+");
  });
});

describe("DashboardSidebar — collapsed More overflow", () => {
  beforeEach(() => {
    jest.clearAllMocks();
    window.localStorage.clear();
    (useBusinessAccess as jest.Mock).mockReturnValue(mockTierDefaults);
  });

  it("shows a single More (⋯) button when collapsed and expands on click", () => {
    renderSidebar();
    fireEvent.click(
      screen.getByRole("button", {
        name: /businessDashboard\.sidebar\.collapse/i,
      }),
    );
    expect(getSidebarRoot().className).toContain("lg:w-16");

    const moreOverflow = screen.getByTestId("rail-more");
    fireEvent.click(moreOverflow);

    // Clicking the ⋯ expands the sidebar back to the full width.
    expect(getSidebarRoot().className).toContain("lg:w-72");
  });

  it("does not render the ⋯ overflow when expanded", () => {
    renderSidebar();
    expect(screen.queryByTestId("rail-more")).toBeNull();
  });
});

describe("DashboardSidebar — collapsed chrome (header/launcher/footer)", () => {
  beforeEach(() => {
    jest.clearAllMocks();
    window.localStorage.clear();
    (useBusinessAccess as jest.Mock).mockReturnValue(mockTierDefaults);
  });

  it("hides the business name block in the rail but keeps it in the DOM", () => {
    renderSidebar();
    fireEvent.click(
      screen.getByRole("button", {
        name: /businessDashboard\.sidebar\.collapse/i,
      }),
    );
    const name = screen.getByRole("heading", { name: "Test Bistro" });
    expect(
      name.closest('[data-collapsible="business-name"]')?.className,
    ).toContain("lg:hidden");
  });
});
