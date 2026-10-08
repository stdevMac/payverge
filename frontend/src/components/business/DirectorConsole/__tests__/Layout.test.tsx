/** @jest-environment jsdom */
/**
 * Task 6.1 + Phase 1.3 — Director's Console layout.
 *
 * The console used to render the chat column at the full grid width,
 * the composer floated at the bottom of a (very long) scroll region, and
 * the thread sidebar disappeared under `xl:` breakpoints — leaving narrow
 * laptops with no way to switch threads.
 *
 * Phase 1.3 then dropped the inner `max-h-[520px]` scroll cap and the
 * `max-w-3xl mx-auto` width cap on the chat column so the chat flex-fills
 * the viewport. The Composer's `sticky bottom-0 mt-auto` chrome was also
 * removed — the column is now a true flex column with a `flex-1 min-h-0`
 * message list above the composer.
 *
 * These tests lock in the current contract:
 *   1. The chat column carries `data-testid="dc-chat-column"` and uses a
 *      viewport-height flex column.
 *   2. The message list (`data-testid="dc-message-list"`) is `flex-1
 *      min-h-0` and no longer has the legacy `max-h-[520px]` cap.
 *   3. At narrow widths the thread sidebar collapses into a
 *      `<select role="combobox" aria-label="Threads">` dropdown so
 *      operators can still switch threads.
 */

import React from "react";
import { render, screen, waitFor } from "@testing-library/react";

// --- Mocks -----------------------------------------------------------------

jest.mock("@/i18n/SimpleTranslationProvider", () => ({
  useSimpleLocale: () => ({ locale: "en", setLocale: jest.fn() }),
  getTranslation: (key: string) => {
    if (key === "directorConsole.quickPrompts") {
      return ["Plan a promo", "Cut costs"];
    }
    return key;
  },
}));

jest.mock("@/hooks/useBusinessAccess", () => ({
  useBusinessAccess: jest.fn(),
}));

jest.mock("@/hooks/useAnalytics", () => ({
  useClickTracking: () => jest.fn(),
}));

jest.mock("@/api/directorConsole", () => ({
  getDirectorThreadMessages: jest.fn(),
  listDirectorThreads: jest.fn(),
  submitDirectorFeedback: jest.fn(),
  listDirectorAppliedActions: jest.fn().mockResolvedValue({ actions: [] }),
}));

// --- Imports (after mocks) -------------------------------------------------

// The BriefingStrip (dashboard front door) pulls in the briefing API + shared
// SSE EventSource, which jsdom lacks and these chat-behavior suites do not
// exercise. BriefingStrip has its own dedicated tests, so stub it (and its
// drawer) here. (PreShift is no longer rendered by the dashboard.)
jest.mock("../BriefingStrip", () => ({ __esModule: true, default: () => null }));
jest.mock("../InsightsDrawer", () => ({ __esModule: true, default: () => null }));

import DirectorConsoleDashboard from "../DirectorConsoleDashboard";
import {
  getDirectorThreadMessages,
  listDirectorThreads,
} from "@/api/directorConsole";
import { useBusinessAccess } from "@/hooks/useBusinessAccess";

const fakeBusiness = {
  id: 7,
  name: "Taqueria Verge",
  ai_settings: { ai_name: "Sage" },
} as any;

function primeMocks(threads: Array<{ id: number; title: string }> = []) {
  (useBusinessAccess as jest.Mock).mockReturnValue({
    access: null,
    loading: false,
    error: null,
    hasAccess: true,
    isSuspended: false,
    lockState: "active",
    aiConfigured: true,
    refetch: jest.fn(),
  });
  (listDirectorThreads as jest.Mock).mockResolvedValue({
    threads: threads.map((t) => ({
      id: t.id,
      title: t.title,
      updated_at: new Date().toISOString(),
    })),
  });
  (getDirectorThreadMessages as jest.Mock).mockResolvedValue({ messages: [] });
}

function setViewportWidth(width: number) {
  Object.defineProperty(window, "innerWidth", {
    value: width,
    writable: true,
    configurable: true,
  });
  // Wire matchMedia to honour the simulated width for `(min-width: …px)` queries.
  (window.matchMedia as jest.Mock).mockImplementation((query: string) => {
    const minWidthMatch = query.match(/\(min-width:\s*(\d+)px\)/);
    let matches = false;
    if (minWidthMatch) {
      matches = width >= parseInt(minWidthMatch[1], 10);
    }
    return {
      matches,
      media: query,
      onchange: null,
      addListener: jest.fn(),
      removeListener: jest.fn(),
      addEventListener: jest.fn(),
      removeEventListener: jest.fn(),
      dispatchEvent: jest.fn(),
    };
  });
}

describe("Director console layout", () => {
  beforeEach(() => {
    jest.clearAllMocks();
  });

  it("root is a dvh flex column and the chat column flex-fills, no inner max-h", async () => {
    setViewportWidth(1280);
    primeMocks([{ id: 1, title: "Sales review" }]);

    const { container } = render(
      <DirectorConsoleDashboard business={fakeBusiness} />,
    );

    const chatColumn = await screen.findByTestId("dc-chat-column");
    // Fixed viewport height is gone — the column fills its flex parent instead.
    expect(chatColumn.className).not.toMatch(/h-\[calc\(100vh-12rem\)\]/);
    expect(chatColumn.className).toContain("h-full");
    expect(chatColumn.className).toContain("min-h-0");

    const list = container.querySelector("[data-testid='dc-message-list']");
    expect(list).toBeTruthy();
    expect(list?.className ?? "").toContain("flex-1");
    expect(list?.className ?? "").toContain("min-h-0");
    expect(list?.className ?? "").not.toContain("max-h-[520px]");
  });

  it("composer is no longer pinned with sticky chrome", async () => {
    setViewportWidth(1280);
    primeMocks([{ id: 1, title: "Sales review" }]);

    render(<DirectorConsoleDashboard business={fakeBusiness} />);

    const composer = await screen.findByTestId("dc-composer");
    expect(composer.className).not.toMatch(/sticky/);
    expect(composer.className).not.toMatch(/mt-auto/);
  });

  it("renders the thread dropdown on narrow widths", async () => {
    setViewportWidth(600);
    primeMocks([
      { id: 1, title: "Sales review" },
      { id: 2, title: "Inventory" },
    ]);

    render(<DirectorConsoleDashboard business={fakeBusiness} />);

    await waitFor(() =>
      expect((listDirectorThreads as jest.Mock).mock.calls.length).toBeGreaterThan(0),
    );

    const dropdown = await screen.findByRole("combobox", { name: /threads/i });
    expect(dropdown.tagName).toBe("SELECT");
  });

  it("keeps applied/archived toggles outside the thread scroller so they cannot clip (#179)", async () => {
    setViewportWidth(1280);
    primeMocks([{ id: 1, title: "Sales review" }]);

    render(<DirectorConsoleDashboard business={fakeBusiness} />);

    const scroller = await screen.findByTestId("dc-thread-scroller");
    const applied = await screen.findByTestId("dc-applied-panel");
    expect(scroller).not.toContainElement(applied);
    expect(applied.className).toMatch(/shrink-0/);
    expect(applied.className).toMatch(/overflow-visible/);
    expect(screen.getByTestId("dc-applied-toggle")).toHaveClass("whitespace-nowrap");
    expect(screen.getByTestId("dc-archived-toggle")).toHaveClass("whitespace-nowrap");
    expect(screen.getByTestId("dc-applied-toggle")).toBeVisible();
    expect(screen.getByTestId("dc-archived-toggle")).toBeVisible();
  });

  it("does NOT render the thread dropdown on wide widths", async () => {
    setViewportWidth(1280);
    primeMocks([{ id: 1, title: "Sales review" }]);

    render(<DirectorConsoleDashboard business={fakeBusiness} />);

    await waitFor(() =>
      expect((listDirectorThreads as jest.Mock).mock.calls.length).toBeGreaterThan(0),
    );

    expect(
      screen.queryByRole("combobox", { name: /threads/i }),
    ).not.toBeInTheDocument();
  });
});
