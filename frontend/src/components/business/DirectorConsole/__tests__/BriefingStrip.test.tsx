/** @jest-environment jsdom */
import React from "react";
import { render, screen, fireEvent } from "@testing-library/react";

import BriefingStrip from "../BriefingStrip";
import type { Business } from "@/api/business";
import type {
  BriefingResponse,
  BriefingPulse,
  ProactiveInsightDTO,
} from "@/api/directorConsole";

// Session principal for the greeting (not business.owner_name).
jest.mock("@/providers/HybridAuthProvider", () => ({
  useAuth: () => ({ staffData: null }),
}));
jest.mock("@/store/useUserStore", () => ({
  useUserStore: (selector: (s: { user: { username: string } | null }) => unknown) =>
    selector({ user: { username: "Mara" } }),
}));

jest.mock("@/i18n/SimpleTranslationProvider", () => ({
  useSimpleLocale: () => ({ locale: "en", setLocale: jest.fn() }),
  getTranslation: (
    key: string,
    _locale: string,
    params?: Record<string, string | number>,
  ) => (params ? `${key} ${JSON.stringify(params)}` : key),
}));

// Stub the drawer so the strip test does not portal/animate; capture its props.
const drawerProps: { current: { open: boolean } | null } = { current: null };
jest.mock("../InsightsDrawer", () => ({
  __esModule: true,
  default: (props: { open: boolean }) => {
    drawerProps.current = { open: props.open };
    return props.open ? <div data-testid="mock-drawer" /> : null;
  },
}));

const mockUse = jest.fn();
jest.mock("@/hooks/useDirectorBriefing", () => ({
  useDirectorBriefing: (...args: unknown[]) => mockUse(...args),
}));

const business = {
  id: 42,
  default_currency: "USD",
  owner_name: "Mara",
  timezone: "America/New_York",
} as unknown as Business;

function pulse(overrides: Partial<BriefingPulse> = {}): BriefingPulse {
  return {
    revenue: 4200,
    projected: 4700,
    typical_day: 4200,
    pace_pct: 12,
    orders: 84,
    avg_ticket: 37,
    food_cost_pct: 0.28,
    labor_cost_pct: 0.24,
    open_bills: 3,
    ...overrides,
  };
}

function insight(
  id: string,
  type: string,
  tab: string,
  params: Record<string, unknown> = { count: 1 },
): ProactiveInsightDTO {
  return { id, type, params, cta: { tab } };
}

function briefing(insights: ProactiveInsightDTO[]): BriefingResponse {
  return { state: "active", pulse: pulse(), insights, play: null, win: null };
}

function setBriefing(
  b: BriefingResponse | null,
  loading = false,
  fetchedAt: Date | null = new Date("2026-08-11T22:27:00.000Z"),
) {
  mockUse.mockReturnValue({
    briefing: b,
    loading,
    error: false,
    refetch: jest.fn(),
    fetchedAt,
  });
}

describe("BriefingStrip", () => {
  beforeEach(() => {
    mockUse.mockReset();
    drawerProps.current = null;
  });

  it("enables the briefing hook for the business id", () => {
    setBriefing(briefing([]));
    render(<BriefingStrip business={business} onOpenTab={jest.fn()} />);
    expect(mockUse).toHaveBeenCalledWith(42, true);
  });

  it("renders top-5 severity-sorted chips and a count badge for 7 insights", () => {
    setBriefing(
      briefing([
        insight("n1", "ai_conversations_pending", "ai"),
        insight("w1", "stale_open_bills", "bills", { count: 2 }),
        insight("u1", "inventory_out_of_stock", "inventory"),
        insight("w2", "labor_high", "accounting", { pct: 0.3, amount: 100 }),
        insight("w3", "waste_high", "accounting", { amount: 50, ingredient: "x" }),
        insight("w4", "food_cost_high", "accounting", { count: 4 }),
        insight("u2", "inventory_out_of_stock", "inventory", { count: 3 }),
      ]),
    );
    render(<BriefingStrip business={business} onOpenTab={jest.fn()} />);

    const chips = screen.getAllByTestId("dc-briefing-chip");
    // Capped at 5, urgent first.
    expect(chips).toHaveLength(5);
    expect(chips[0]).toHaveAttribute("data-insight-id", "u1");
    expect(chips[1]).toHaveAttribute("data-insight-id", "u2");

    // Count badge shows the FULL count (7), not the capped 5.
    expect(screen.getByTestId("dc-strip-count")).toHaveTextContent("7");
  });

  it("shows full insight text in a popover and opens the tab from the CTA", () => {
    const onOpenTab = jest.fn();
    setBriefing(briefing([insight("u1", "inventory_out_of_stock", "inventory")]));
    render(<BriefingStrip business={business} onOpenTab={onOpenTab} />);
    expect(screen.getByTestId("dc-briefing-chip-expand")).toBeInTheDocument();
    fireEvent.click(screen.getByTestId("dc-briefing-chip"));
    // Chip press opens the popover — deep-link only from the explicit CTA so
    // truncated chips are never a dead end (#180).
    expect(onOpenTab).not.toHaveBeenCalled();
    expect(screen.getByTestId("dc-briefing-chip-full")).toBeInTheDocument();
    fireEvent.click(screen.getByTestId("dc-briefing-chip-open"));
    expect(onOpenTab).toHaveBeenCalledWith("inventory");
  });

  it("greets evening at 18:27 venue-local and stamps the briefing as-of (#194)", () => {
    jest.useFakeTimers();
    jest.setSystemTime(new Date("2026-08-11T22:27:00.000Z"));
    try {
      const onOpenTab = jest.fn();
      setBriefing(briefing([]));
      render(<BriefingStrip business={business} onOpenTab={onOpenTab} />);
      expect(screen.getByTestId("preshift-greeting")).toHaveTextContent(
        "directorConsole.preShift.greeting.eveningNamed",
      );
      expect(screen.getByTestId("dc-strip-as-of")).toBeInTheDocument();
      fireEvent.click(screen.getByTestId("dc-strip-open-analytics"));
      expect(onOpenTab).toHaveBeenCalledWith("analytics");
    } finally {
      jest.useRealTimers();
    }
  });

  it("opens the drawer when View all is clicked", () => {
    setBriefing(
      briefing([
        insight("u1", "inventory_out_of_stock", "inventory"),
        insight("w1", "stale_open_bills", "bills", { count: 2 }),
      ]),
    );
    render(<BriefingStrip business={business} onOpenTab={jest.fn()} />);
    expect(screen.queryByTestId("mock-drawer")).toBeNull();
    fireEvent.click(screen.getByTestId("dc-strip-view-all"));
    expect(drawerProps.current?.open).toBe(true);
    expect(screen.getByTestId("mock-drawer")).toBeInTheDocument();
  });

  it("shows the all-clear line (no chips, no view-all) when there are no insights", () => {
    setBriefing(briefing([]));
    render(<BriefingStrip business={business} onOpenTab={jest.fn()} />);
    expect(screen.queryByTestId("dc-briefing-chip")).toBeNull();
    expect(screen.queryByTestId("dc-strip-view-all")).toBeNull();
    expect(screen.getByText(/strip\.allClear/)).toBeInTheDocument();
  });

  it("renders a bounded loading placeholder before the briefing arrives", () => {
    setBriefing(null, true);
    render(<BriefingStrip business={business} onOpenTab={jest.fn()} />);
    expect(screen.getByTestId("dc-strip-loading")).toBeInTheDocument();
  });
});
