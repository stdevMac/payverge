/** @jest-environment jsdom */
/**
 * Locks the Director Console front-door deep-link contract.
 *
 * The BriefingStrip is the dashboard's front door; each insight chip carries a
 * CTA that must route the operator into the destination tab. The only new
 * production logic is `openTab` in DirectorConsoleDashboard, which builds the
 * URL `/business/${business.id}/dashboard?tab=${tab}` and calls `router.push`.
 * Tabs are URL-driven (the dashboard page re-selects from `?tab=` on every query
 * change), so a wrong query key or a dropped id silently breaks deep-linking.
 *
 * This suite renders the REAL strip + chip (only `useDirectorBriefing` is
 * mocked, so the hook's shared-SSE `EventSource` is never reached) and asserts
 * the exact pushed URL when a chip is clicked.
 */
import React from "react";
import { render, screen, fireEvent } from "@testing-library/react";

// Stable router push spy. Prefixed `mock*` so babel-jest's hoist permits it
// inside the (hoisted) jest.mock factory below. The global jest.setup.js
// next/navigation mock returns a fresh push fn per call, which can't be
// asserted on — this file-level override captures a single spy instead.
const mockPush = jest.fn();
jest.mock("next/navigation", () => ({
  useRouter: () => ({
    push: mockPush,
    replace: jest.fn(),
    prefetch: jest.fn(),
    back: jest.fn(),
    forward: jest.fn(),
    refresh: jest.fn(),
  }),
  useSearchParams: () => new URLSearchParams(),
  usePathname: () => "/",
  useParams: () => ({}),
}));

jest.mock("@/i18n/SimpleTranslationProvider", () => ({
  useSimpleLocale: () => ({ locale: "en", setLocale: jest.fn() }),
  getTranslation: (key: string) => {
    if (key === "directorConsole.quickPrompts") return [];
    return key;
  },
}));
jest.mock("@/providers/HybridAuthProvider", () => ({
  useAuth: () => ({ staffData: null }),
}));
jest.mock("@/store/useUserStore", () => ({
  useUserStore: (selector: (s: { user: null }) => unknown) =>
    selector({ user: null }),
}));
jest.mock("@/hooks/useBusinessAccess", () => ({
  useBusinessAccess: () => ({ hasAccess: true, isSuspended: false, aiConfigured: true, loading: false }),
}));
jest.mock("@/hooks/useAnalytics", () => ({ useClickTracking: () => jest.fn() }));
jest.mock("@/api/directorConsole", () => ({
  askDirectorStreamURL: (id: string | number) =>
    `http://localhost/api/v1/inside/businesses/${id}/ai/director/ask/stream`,
  listDirectorThreads: jest.fn().mockResolvedValue({ threads: [] }),
  getDirectorThreadMessages: jest.fn().mockResolvedValue({ messages: [] }),
  submitDirectorFeedback: jest.fn(),
}));

// Render the REAL BriefingStrip + chip, but replace the data hook with a
// static return so the underlying useSSEEvents/EventSource is never opened.
// The single insight maps (via insightCopy.toBriefingChips) to a chip whose
// `tab` is the CTA tab — here "inventory".
jest.mock("@/hooks/useDirectorBriefing", () => ({
  useDirectorBriefing: () => ({
    briefing: {
      state: "active",
      pulse: {
        revenue: 0,
        projected: null,
        typical_day: null,
        pace_pct: null,
        orders: 0,
        avg_ticket: 0,
        food_cost_pct: null,
        labor_cost_pct: null,
        open_bills: 0,
      },
      insights: [
        { id: "1", type: "inventory_out_of_stock", params: { count: 1 }, cta: { tab: "inventory" } },
      ],
      play: null,
      win: null,
    },
    loading: false,
    error: false,
    refetch: jest.fn(),
  }),
}));

// NOTE: deliberately NOT mocking "../BriefingStrip" — this suite must exercise
// the real chip → onOpenTab → openTab → router.push path end to end.

import DirectorConsoleDashboard from "../DirectorConsoleDashboard";
import type { Business } from "@/api/business";

const business = { id: 42, ai_settings: { ai_name: "Sage" } } as unknown as Business;

describe("Director Console — briefing strip deep-link", () => {
  beforeEach(() => mockPush.mockReset());

  it("deep-links to the destination tab from the chip popover CTA", async () => {
    render(<DirectorConsoleDashboard business={business} />);

    const chip = await screen.findByTestId("dc-briefing-chip");
    fireEvent.click(chip);
    fireEvent.click(await screen.findByTestId("dc-briefing-chip-open"));

    expect(mockPush).toHaveBeenCalledWith("/business/42/dashboard?tab=inventory");
  });
});
