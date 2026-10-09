/** @jest-environment jsdom */
/**
 * Phase 5 wiring: proposals arriving on the stream's response.complete render
 * as ProposalCards under the transcript; dismiss removes a card; sending a
 * new ask clears all staged cards (a fresh answer supersedes them).
 */
import React from "react";
import { render, screen, fireEvent, waitFor, act } from "@testing-library/react";

import type { UseDirectorStreamApi } from "@/hooks/useDirectorStream";

jest.mock("@/i18n/SimpleTranslationProvider", () => ({
  useSimpleLocale: () => ({ locale: "en", setLocale: jest.fn() }),
  getTranslation: (key: string) =>
    key === "directorConsole.quickPrompts" ? [] : key,
}));

jest.mock("@/hooks/useBusinessAccess", () => ({
  useBusinessAccess: () => ({
    hasAccess: true,
    isSuspended: false,
    aiConfigured: true,
    loading: false,
  }),
}));

// Stable instance: the dashboard's stream-complete effect lists `trackClick`
// in its deps, so a mock returning a fresh jest.fn() per render would re-fire
// the effect on every render and loop forever once `complete` is set.
jest.mock("@/hooks/useAnalytics", () => {
  const mockTrack = jest.fn();
  return { useClickTracking: () => mockTrack };
});

jest.mock("next/navigation", () => ({
  useRouter: () => ({ push: jest.fn() }),
}));

jest.mock("@/api/directorConsole", () => {
  const actual = jest.requireActual("@/api/directorConsole");
  return {
    ...actual,
    listDirectorThreads: jest.fn().mockResolvedValue({ threads: [] }),
    getDirectorThreadMessages: jest.fn().mockResolvedValue({ messages: [] }),
    applyDirectorAction: jest.fn(),
    undoDirectorAction: jest.fn(),
  };
});

// Mock the stream hook: this spec exercises the dashboard's wiring of the
// `complete` payload, not SSE parsing (covered by useDirectorStream tests).
const mockStreamState: { api: UseDirectorStreamApi } = {
  api: {
    toolCalls: [],
    streaming: false,
    complete: null,
    error: null,
    aborted: false,
    start: jest.fn().mockResolvedValue(undefined),
    abort: jest.fn(),
    reset: jest.fn(),
  },
};

jest.mock("@/hooks/useDirectorStream", () => ({
  useDirectorStream: () => mockStreamState.api,
}));

// The Pre-Shift deck (mounted as the dashboard front door) pulls in the
// proactive-insights API + shared SSE EventSource, which jsdom lacks and these
// chat-behavior suites do not exercise. PreShift has its own dedicated tests
// (PreShift / PreShiftCard / insightCopy / preShiftI18n), so stub it here.
// The BriefingStrip (dashboard front door) pulls in the briefing API + shared
// SSE EventSource, which jsdom lacks and these chat-behavior suites do not
// exercise. BriefingStrip has its own dedicated tests, so stub it (and its
// drawer) here. (PreShift is no longer rendered by the dashboard.)
jest.mock("../BriefingStrip", () => ({ __esModule: true, default: () => null }));
jest.mock("../InsightsDrawer", () => ({ __esModule: true, default: () => null }));

import DirectorConsoleDashboard from "../DirectorConsoleDashboard";
import type { Business } from "@/api/business";

const business = { id: 42, ai_settings: { ai_name: "Sage" } } as unknown as Business;

const proposal = {
  id: "pa_wire1",
  kind: "menu.adjust_prices" as const,
  title: "Raise dessert prices 5%",
  description: "",
  preview: { affected_count: 1, summary: "1 price changes", examples: [] },
  warnings: [],
  menu_version: 2,
  requires_reconfirm: false,
  expires_at: "2026-06-11T23:00:00Z",
};

function setComplete(rerender: (ui: React.ReactElement) => void) {
  mockStreamState.api = {
    ...mockStreamState.api,
    complete: {
      message_id: 1,
      thread_id: 9,
      proposed_actions: [proposal],
    },
  };
  rerender(<DirectorConsoleDashboard business={business} />);
}

describe("DirectorConsoleDashboard proposals wiring", () => {
  beforeEach(() => {
    jest.clearAllMocks();
    mockStreamState.api = {
      ...mockStreamState.api,
      complete: null,
      streaming: false,
      start: jest.fn().mockResolvedValue(undefined),
      abort: jest.fn(),
      reset: jest.fn(),
    };
  });

  it("renders proposal cards from the stream complete payload and dismisses on click", async () => {
    const { rerender } = render(<DirectorConsoleDashboard business={business} />);
    expect(screen.queryByTestId("dc-proposal-card")).not.toBeInTheDocument();

    await act(async () => setComplete(rerender));

    expect(await screen.findByTestId("dc-proposals")).toBeInTheDocument();
    expect(screen.getByText("Raise dessert prices 5%")).toBeInTheDocument();
    expect(screen.getByTestId("dc-proposal-disclosure")).toBeInTheDocument();

    fireEvent.click(screen.getByTestId("dc-proposal-dismiss"));
    await waitFor(() => {
      expect(screen.queryByTestId("dc-proposal-card")).not.toBeInTheDocument();
    });
  });

  it("clears staged proposals when a new ask is sent", async () => {
    const { rerender } = render(<DirectorConsoleDashboard business={business} />);
    await act(async () => setComplete(rerender));
    expect(await screen.findByTestId("dc-proposals")).toBeInTheDocument();

    const textarea = screen.getByRole("textbox");
    fireEvent.change(textarea, { target: { value: "Another question" } });
    fireEvent.keyDown(textarea, { key: "Enter", metaKey: true });

    await waitFor(() => {
      expect(screen.queryByTestId("dc-proposals")).not.toBeInTheDocument();
    });
  });
});
