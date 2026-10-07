/** @jest-environment jsdom */
/**
 * The proposal expiry line ("Expires in 58 min") was computed once in a
 * useMemo keyed on expires_at, so it froze the moment the card mounted: an
 * operator reading an answer watched a stale countdown and never saw the card
 * flip to expired. It must re-render on a wall-clock tick.
 */
import React from "react";
import { render, screen, act } from "@testing-library/react";

import ProposalCard from "../ProposalCard";
import type { DirectorProposedAction } from "@/api/directorConsole";

jest.mock("@/api/directorConsole", () => {
  const actual = jest.requireActual("@/api/directorConsole");
  return {
    ...actual,
    applyDirectorAction: jest.fn(),
    undoDirectorAction: jest.fn(),
  };
});

// Surfaces the interpolated count so the assertions can read the countdown.
const t = (key: string, params?: Record<string, string | number>) =>
  params && "count" in params ? `${key}|${params.count}` : key;

const NOW = Date.parse("2026-06-11T12:00:00Z");

function makeProposal(expiresAt: string): DirectorProposedAction {
  return {
    id: "pa_tick",
    kind: "menu.adjust_prices",
    title: "Raise dessert prices 5%",
    description: "Desserts are underpriced.",
    preview: {
      affected_count: 1,
      summary: "1 price changes",
      examples: [{ name: "Tiramisu", before: 8, after: 8.4 }],
    },
    warnings: [],
    menu_version: 3,
    requires_reconfirm: false,
    expires_at: expiresAt,
  } as DirectorProposedAction;
}

function renderCard(minutesFromNow: number) {
  return render(
    <ProposalCard
      proposal={makeProposal(new Date(NOW + minutesFromNow * 60_000).toISOString())}
      businessId={42}
      t={t}
      onDismiss={jest.fn()}
    />,
  );
}

const expiryText = () =>
  screen.getByTestId("dc-proposal-expiry").textContent ?? "";

describe("ProposalCard expiry countdown ticks", () => {
  beforeEach(() => {
    jest.useFakeTimers();
    jest.setSystemTime(NOW);
  });

  afterEach(() => {
    jest.runOnlyPendingTimers();
    jest.useRealTimers();
  });

  it("recomputes the remaining minutes as time passes", () => {
    renderCard(58);
    expect(expiryText()).toContain("proposal.expiresInMinutes|58");

    act(() => {
      jest.advanceTimersByTime(60_000);
    });
    expect(expiryText()).toContain("proposal.expiresInMinutes|57");

    act(() => {
      jest.advanceTimersByTime(10 * 60_000);
    });
    expect(expiryText()).toContain("proposal.expiresInMinutes|47");
  });

  it("flips to the expired state while the card is on screen", () => {
    renderCard(2);
    expect(expiryText()).toContain("proposal.expiresInMinutes|2");

    act(() => {
      jest.advanceTimersByTime(3 * 60_000);
    });
    expect(expiryText()).toBe("proposal.expired");
  });

  it("clears its interval on unmount", () => {
    const clearSpy = jest.spyOn(global, "clearInterval");
    const { unmount } = renderCard(58);
    unmount();
    expect(clearSpy).toHaveBeenCalled();
    clearSpy.mockRestore();
  });
});
