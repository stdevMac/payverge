/** @jest-environment jsdom */
import { render, screen, fireEvent } from "@testing-library/react";
import Dashboard from "./Dashboard";

// next/dynamic wraps RevenuePanel + ServiceTipsPanel after P-4. Make it a
// synchronous pass-through so the panel mocks below still resolve and existing
// tab-switch assertions hold without async waitFor.
jest.mock("next/dynamic", () => ({
  __esModule: true,
  default: (importFn: () => Promise<{ default: unknown }>) => {
    const React = require("react") as typeof import("react");
    let Resolved: React.ComponentType | null = null;
    importFn().then((m) => {
      Resolved = (m as { default: React.ComponentType }).default;
    });
    const Wrapper = (props: Record<string, unknown>) =>
      Resolved ? React.createElement(Resolved, props) : null;
    Wrapper.displayName = "DynamicWrapper";
    return Wrapper;
  },
}));

// Stable router/searchParams mock so we can assert how often replace() fires.
// (The global jest.setup mock returns a fresh jest.fn() per useRouter() call,
// which can't be spied across renders.)
const mockReplace = jest.fn();
let mockSearch = "sub=today";
jest.mock("next/navigation", () => ({
  useRouter: () => ({
    replace: mockReplace,
    push: jest.fn(),
    prefetch: jest.fn(),
    back: jest.fn(),
    forward: jest.fn(),
    refresh: jest.fn(),
  }),
  useSearchParams: () => new URLSearchParams(mockSearch),
  usePathname: () => "/",
}));

jest.mock("@/hooks/useBusinessAccess", () => ({ useBusinessAccess: () => ({ hasAccess: true, loading: false }) }));
jest.mock("./panels/TodayLivePanel", () => ({ __esModule: true, default: () => <div data-testid="today" /> }));
jest.mock("./panels/RevenuePanel", () => ({ __esModule: true, default: () => <div data-testid="revenue" /> }));
jest.mock("./panels/MenuPanel", () => ({ __esModule: true, default: () => <div data-testid="menu" /> }));
jest.mock("./panels/ServiceTipsPanel", () => ({ __esModule: true, default: () => <div data-testid="svc" /> }));
jest.mock("./PaymentHistory", () => ({ __esModule: true, default: () => <div data-testid="payments" /> }));
jest.mock("./CostAnalyticsSection", () => ({
  __esModule: true,
  default: () => <div data-testid="cost-analytics-section" />,
}));

const selectedTabCount = () =>
  screen.getAllByRole("tab").filter((t) => t.getAttribute("aria-selected") === "true").length;

describe("Dashboard tabs", () => {
  beforeEach(() => {
    jest.clearAllMocks();
    mockSearch = "sub=today";
  });

  it("renders all five views in one tab row with the default Today/Live panel", () => {
    render(<Dashboard businessId="42" />);
    expect(screen.getAllByRole("tab")).toHaveLength(5);
    expect(screen.getByTestId("today")).toBeInTheDocument();
    // L6-4: cost health is scoped to the revenue sub-tab, not every analytics view.
    expect(screen.queryByTestId("cost-analytics-section")).not.toBeInTheDocument();
  });

  it("shows cost analytics only on the revenue sub-tab (L6-4)", async () => {
    render(<Dashboard businessId="42" />);
    expect(screen.queryByTestId("cost-analytics-section")).not.toBeInTheDocument();
    fireEvent.click(screen.getByRole("tab", { name: /revenue/i }));
    expect(await screen.findByTestId("revenue")).toBeInTheDocument();
    expect(screen.getByTestId("cost-analytics-section")).toBeInTheDocument();
  });

  // Regression: two NextUI <Tabs> sharing one selectedKey self-corrected via
  // onSelectionChange in an infinite loop, hammering router.replace ("force
  // refreshing"). A correct tab bar is a single source of truth: exactly one
  // tab is selected across both groups, and nothing navigates on mount when
  // sub= is already present.
  it("selects exactly one tab across both groups and does not navigate on mount when sub is set", () => {
    render(<Dashboard businessId="42" />);
    expect(selectedTabCount()).toBe(1);
    expect(mockReplace).not.toHaveBeenCalled();
  });

  it("switches panel + deep-links via sub= exactly once on a real tab click", async () => {
    render(<Dashboard businessId="42" />);
    fireEvent.click(screen.getByRole("tab", { name: /revenue/i }));
    expect(await screen.findByTestId("revenue")).toBeInTheDocument();
    expect(selectedTabCount()).toBe(1);
    expect(mockReplace).toHaveBeenCalledTimes(1);
    expect(mockReplace.mock.calls[0][0]).toContain("sub=revenue");
    expect(mockReplace.mock.calls[0][0]).not.toContain("view=");
  });

  it("honors a deep-linked ?sub= on first render", () => {
    mockSearch = "sub=menu";
    render(<Dashboard businessId="42" />);
    expect(screen.getByTestId("menu")).toBeInTheDocument();
    expect(
      screen.getByRole("tab", { name: /menu/i }).getAttribute("aria-selected"),
    ).toBe("true");
  });

  it("ignores a legacy ?view= param and keeps the default panel", () => {
    mockSearch = "view=menu";
    render(<Dashboard businessId="42" />);
    expect(screen.getByTestId("today")).toBeInTheDocument();
    expect(screen.queryByTestId("menu")).not.toBeInTheDocument();
    const written = mockReplace.mock.calls.map((c) => String(c[0])).join(" ");
    expect(written).not.toMatch(/sub=menu/);
  });

  // Regression for AA1: the active panel must follow external ?sub= changes
  // (browser back/forward, in-app deep-links) — not just the initial mount and
  // not just clicks. Previously activeTab seeded from the URL once and never
  // re-synced, so navigation left the panel stuck.
  it("syncs the active panel when ?sub= changes externally (back/forward)", async () => {
    mockSearch = "sub=today";
    const { rerender } = render(<Dashboard businessId="42" />);
    expect(screen.getByTestId("today")).toBeInTheDocument();

    // Simulate a browser back/forward landing on a different sub.
    mockSearch = "sub=service";
    rerender(<Dashboard businessId="42" />);

    expect(await screen.findByTestId("svc")).toBeInTheDocument();
    expect(selectedTabCount()).toBe(1);
    expect(
      screen
        .getByRole("tab", { name: /service|tips/i })
        .getAttribute("aria-selected"),
    ).toBe("true");
    // A pure URL-driven sync must NOT push another history entry. sub is
    // already set, so nothing is rewritten.
    expect(mockReplace).not.toHaveBeenCalled();
  });

  it("renders a not-found state naming an unknown sub key (no silent redirect)", () => {
    mockSearch = "sub=not-a-panel";
    render(<Dashboard businessId="42" />);
    expect(screen.getByTestId("analytics-sub-not-found")).toBeInTheDocument();
    // Body must name the bad key so typos stay visible.
    expect(screen.getByText(/not-a-panel/)).toBeInTheDocument();
    expect(screen.queryByTestId("today")).not.toBeInTheDocument();
  });
});
