/** @jest-environment jsdom */
import React from "react";
import { render, screen, fireEvent, within } from "@testing-library/react";

import InsightsDrawer from "../InsightsDrawer";
import type { Business } from "@/api/business";
import type {
  BriefingResponse,
  BriefingPulse,
  ProactiveInsightDTO,
} from "@/api/directorConsole";

jest.mock("@/i18n/SimpleTranslationProvider", () => ({
  useSimpleLocale: () => ({ locale: "en", setLocale: jest.fn() }),
  getTranslation: (
    key: string,
    _locale: string,
    params?: Record<string, string | number>,
  ) => (params ? `${key} ${JSON.stringify(params)}` : key),
}));

const business = { id: 42, default_currency: "USD" } as unknown as Business;

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

// 20 insights: 6 urgent (out-of-stock), 8 watch (stale bills), 6 info (ai pending).
function makeInsights(): ProactiveInsightDTO[] {
  const out: ProactiveInsightDTO[] = [];
  for (let i = 0; i < 6; i++)
    out.push({
      id: `u${i}`,
      type: "inventory_out_of_stock",
      params: { count: 1, item_names: [`Item${i}`] },
      cta: { tab: "inventory" },
    });
  for (let i = 0; i < 8; i++)
    out.push({
      id: `w${i}`,
      type: "stale_open_bills",
      params: { count: 2 },
      cta: { tab: "bills" },
    });
  for (let i = 0; i < 6; i++)
    out.push({
      id: `n${i}`,
      type: "ai_conversations_pending",
      params: { count: 1 },
      cta: { tab: "ai" },
    });
  return out;
}

function briefing(insights: ProactiveInsightDTO[]): BriefingResponse {
  return { state: "active", pulse: pulse(), insights, play: null, win: null };
}

describe("InsightsDrawer", () => {
  it("renders nothing when closed", () => {
    const { container } = render(
      <InsightsDrawer
        open={false}
        onClose={jest.fn()}
        briefing={briefing(makeInsights())}
        business={business}
        onOpenTab={jest.fn()}
      />,
    );
    expect(container.querySelector("[data-testid='dc-insights-drawer']")).toBeNull();
  });

  it("renders all 20 insights grouped urgent → watch → info, plus the prose read", () => {
    render(
      <InsightsDrawer
        open
        onClose={jest.fn()}
        briefing={briefing(makeInsights())}
        business={business}
        onOpenTab={jest.fn()}
      />,
    );
    // The briefing read (prose) sits at the top of the drawer.
    expect(screen.getByTestId("dc-briefing-read")).toBeInTheDocument();

    const cards = screen.getAllByTestId("preshift-card");
    expect(cards).toHaveLength(20);

    // First 6 are urgent (rose card), next 8 watch, last 6 info — assert the
    // first card is urgent-toned and the last is not, proving the grouping.
    expect(cards[0].className).toMatch(/rose/);
    expect(cards[19].className).not.toMatch(/rose/);
  });

  it("dispatches onOpenTab when an insight card action is clicked", () => {
    const onOpenTab = jest.fn();
    render(
      <InsightsDrawer
        open
        onClose={jest.fn()}
        briefing={briefing(makeInsights())}
        business={business}
        onOpenTab={onOpenTab}
      />,
    );
    const firstCard = screen.getAllByTestId("preshift-card")[0];
    fireEvent.click(within(firstCard).getByTestId("preshift-card-action"));
    expect(onOpenTab).toHaveBeenCalledWith("inventory");
  });
});
