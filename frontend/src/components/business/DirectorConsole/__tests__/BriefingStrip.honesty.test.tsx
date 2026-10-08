/** @jest-environment jsdom */
/**
 * Task 34 / Findings 54, 56 — Director Console greets the session user, not a
 * fixture owner_name (demo "Alex Demo"), and the greeting heading appears once.
 */
import React from "react";
import { render, screen } from "@testing-library/react";

import BriefingStrip from "../BriefingStrip";
import type { Business } from "@/api/business";
import type { BriefingResponse, BriefingPulse } from "@/api/directorConsole";

const mockUserStore = { user: null as null | { username: string } };
jest.mock("@/store/useUserStore", () => ({
  useUserStore: (
    selector: (s: { user: { username: string } | null }) => unknown,
  ) => selector(mockUserStore),
}));

const mockAuth = { staffData: null as null | { name: string } };
jest.mock("@/providers/HybridAuthProvider", () => ({
  useAuth: () => mockAuth,
}));

jest.mock("@/i18n/SimpleTranslationProvider", () => ({
  useSimpleLocale: () => ({ locale: "en", setLocale: jest.fn() }),
  getTranslation: (
    key: string,
    _locale: string,
    params?: Record<string, string | number>,
  ) => {
    if (key === "directorConsole.preShift.greeting.eveningNamed" && params?.name) {
      return `Good evening, ${params.name}.`;
    }
    if (key === "directorConsole.preShift.greeting.morningNamed" && params?.name) {
      return `Good morning, ${params.name}.`;
    }
    if (key === "directorConsole.preShift.greeting.afternoonNamed" && params?.name) {
      return `Good afternoon, ${params.name}.`;
    }
    if (key.endsWith(".evening")) return "Good evening.";
    if (key.endsWith(".morning")) return "Good morning.";
    if (key.endsWith(".afternoon")) return "Good afternoon.";
    return params ? `${key} ${JSON.stringify(params)}` : key;
  },
}));

jest.mock("../InsightsDrawer", () => ({
  __esModule: true,
  default: () => null,
}));

// Greeting daypart uses venue-local hour (business.timezone → UTC fallback),
// not Date#getHours. Pin evening so this honesty suite stays about session
// name attribution, not CI host clock / timezone.
jest.mock("@/utils/businessTime", () => ({
  ...jest.requireActual("@/utils/businessTime"),
  businessLocalHour: () => 20,
}));

const mockUse = jest.fn();
jest.mock("@/hooks/useDirectorBriefing", () => ({
  useDirectorBriefing: (...args: unknown[]) => mockUse(...args),
}));

function pulse(): BriefingPulse {
  return {
    revenue: 100,
    projected: 120,
    typical_day: 100,
    pace_pct: 5,
    orders: 10,
    avg_ticket: 12,
    food_cost_pct: 0.28,
    labor_cost_pct: 0.24,
    open_bills: 1,
  };
}

function emptyBriefing(): BriefingResponse {
  return {
    state: "active",
    pulse: pulse(),
    insights: [],
    play: null,
    win: null,
  };
}

describe("BriefingStrip honesty (Task 34)", () => {
  beforeEach(() => {
    mockUse.mockReset();
    mockUse.mockReturnValue({
      briefing: emptyBriefing(),
      loading: false,
      error: false,
      refetch: jest.fn(),
    });
    mockUserStore.user = null;
    mockAuth.staffData = null;
  });

  it("greets the authenticated session user, not business.owner_name (Alex fixture)", () => {
    mockUserStore.user = { username: "Mara Mendez" };
    const business = {
      id: 42,
      default_currency: "USD",
      name: "Taqueria Verge",
      // Demo / fixture owner — must NOT leak into the greeting for the live user.
      owner_name: "Alex Demo",
    } as unknown as Business;

    render(<BriefingStrip business={business} onOpenTab={jest.fn()} />);

    const greeting = screen.getByTestId("preshift-greeting");
    expect(greeting).toHaveTextContent("Good evening, Mara.");
    expect(greeting).not.toHaveTextContent("Alex");
  });

  it("prefers staff session name over a stale business.owner_name", () => {
    mockAuth.staffData = { name: "Sofía Ruiz" };
    mockUserStore.user = { username: "ignored-owner" };
    const business = {
      id: 7,
      default_currency: "USD",
      name: "Café Norte",
      owner_name: "Alex Demo",
    } as unknown as Business;

    render(<BriefingStrip business={business} onOpenTab={jest.fn()} />);

    const greeting = screen.getByTestId("preshift-greeting");
    expect(greeting).toHaveTextContent("Good evening, Sofía.");
    expect(greeting).not.toHaveTextContent("Alex");
  });

  it("renders the greeting heading once (no duplicate name heading)", () => {
    mockUserStore.user = { username: "Mara Mendez" };
    const business = {
      id: 42,
      default_currency: "USD",
      name: "Taqueria Verge",
      owner_name: "Alex Demo",
    } as unknown as Business;

    render(<BriefingStrip business={business} onOpenTab={jest.fn()} />);

    expect(screen.getAllByTestId("preshift-greeting")).toHaveLength(1);
    // Business name is not a second H1 on the strip (page owns that once).
    const h1s = screen.getAllByRole("heading", { level: 1 });
    expect(h1s).toHaveLength(1);
    expect(h1s[0]).toHaveTextContent(/Good evening, Mara/);
    expect(
      screen.queryByRole("heading", { name: "Taqueria Verge" }),
    ).toBeNull();
  });
});
