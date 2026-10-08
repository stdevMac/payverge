/** @jest-environment jsdom */
/**
 * #657 — Tablero vs the dashboard overflow-hidden shell.
 *
 * These tests do NOT mock scrollWidth/scrollLeft. jsdom has no layout engine,
 * so “scrollTo reached completed” is not evidence. They fail if space-y or
 * uncapped approval/filter chrome can still starve the board at 1024-class
 * heights inside `h-[100dvh] overflow-hidden`.
 */
import React from "react";
import { render, screen } from "@testing-library/react";
import {
  RESERVATION_BOARD_LANE_COUNT,
  ReservationBoardScroller,
  reservationBoardLaneClass,
  reservationBoardMinTrackWidthPx,
} from "./ReservationBoardScroller";
import {
  TABLERO_BOARD_MIN_HEIGHT_CLASS,
  TABLERO_CHROME_CLASS,
  TABLERO_VIEWPORT_1024,
  fillShellClipsBoard,
  reservationBoardChromeClipsPane,
  tableroHostStandBudget,
} from "./reservationBoardLayout";
import DashboardTabShell from "../shared/DashboardTabShell";

const LANES = ["pending", "confirmed", "waitlist", "seated", "completed"] as const;

function overflowHiddenBetween(inner: HTMLElement, boundary: HTMLElement) {
  const clips: HTMLElement[] = [];
  let node: HTMLElement | null = inner.parentElement;
  while (node && node !== boundary) {
    const className = node.className || "";
    if (className.includes("overflow-hidden")) {
      clips.push(node);
    }
    node = node.parentElement;
  }
  return clips;
}

function renderBoardInOverflowShell() {
  return render(
    <div
      data-testid="dashboard-shell"
      className="relative h-[100dvh] overflow-hidden bg-warm-50"
    >
      <DashboardTabShell
        fill
        width="wide"
        header={{ title: "Reservations", subtitle: "Host stand", dense: true }}
        tabs={{
          items: [
            { key: "reservations", label: "Reservations" },
            { key: "settings", label: "Settings" },
          ],
          activeKey: "reservations",
          onChange: () => {},
        }}
      >
        <div className="flex min-h-0 min-w-0 w-full flex-1 flex-col gap-3">
          <div
            data-testid="reservation-board-chrome"
            className={TABLERO_CHROME_CLASS}
          >
            <div style={{ height: 701 }}>Approval queue + filters (y≈701)</div>
          </div>
          <ReservationBoardScroller
            fill
            scrollHint="Scroll sideways to reach later columns"
          >
            {LANES.map((lane) => (
              <div
                key={lane}
                className={reservationBoardLaneClass}
                data-testid="reservation-board-lane"
                data-lane={lane}
              >
                {lane}
              </div>
            ))}
          </ReservationBoardScroller>
        </div>
      </DashboardTabShell>
    </div>,
  );
}

describe("ReservationBoardScroller inside the overflow-hidden dashboard shell", () => {
  it("fails if the fill shell still uses space-y (starves the board)", () => {
    const { container } = renderBoardInOverflowShell();
    const fillShell = container.querySelector(".h-full.min-h-0");
    expect(fillShell).not.toBeNull();
    expect(fillShellClipsBoard(fillShell?.className || "")).toBe(false);
    expect(fillShell?.className || "").not.toMatch(/\bspace-y-/);
    expect(fillShell?.className || "").toMatch(/\bgap-3\b/);
  });

  it("fails if approval + filter chrome is uncapped (y≈701 clip)", () => {
    renderBoardInOverflowShell();
    const chrome = screen.getByTestId("reservation-board-chrome");
    expect(reservationBoardChromeClipsPane(chrome.className)).toBe(false);
    expect(chrome.className).toMatch(/max-h-\[12rem\]/);
    expect(chrome.className).toMatch(/overflow-y-auto/);
    expect(tableroHostStandBudget(TABLERO_VIEWPORT_1024).fits).toBe(true);
  });

  it("keeps overflow-hidden on the shell, not between the scroller and later lanes", () => {
    renderBoardInOverflowShell();

    const shell = screen.getByTestId("dashboard-shell");
    const scroller = screen.getByTestId("reservations-board-scroller");
    const lanes = screen.getAllByTestId("reservation-board-lane");
    const lastLane = lanes[lanes.length - 1];

    expect(shell.className).toMatch(/overflow-hidden/);
    expect(shell.className).toMatch(/h-\[100dvh\]/);
    expect(lanes).toHaveLength(RESERVATION_BOARD_LANE_COUNT);
    expect(lastLane).toHaveAttribute("data-lane", "completed");
    expect(scroller.contains(lastLane)).toBe(true);
    expect(overflowHiddenBetween(lastLane, scroller)).toEqual([]);
  });

  it("sizes a 1264px track inside a 1024-class pane so later lanes overflow the scroller", () => {
    renderBoardInOverflowShell();

    const scroller = screen.getByTestId("reservations-board-scroller");
    const track = screen.getByTestId("reservations-board");
    const trackWidth = reservationBoardMinTrackWidthPx();

    expect(trackWidth).toBe(1264);
    expect(trackWidth).toBeGreaterThan(1024);
    expect(track.className).toMatch(/\bw-max\b/);
    expect(track.className).toMatch(/min-w-max/);
    expect(track.className).not.toMatch(/\bw-full\b/);
    expect(scroller.className).toMatch(/overflow-x-auto/);
    expect(scroller.className).toMatch(/min-w-0/);
    expect(scroller.className).toContain(TABLERO_BOARD_MIN_HEIGHT_CLASS);
    expect(scroller.className).toMatch(/flex-1/);
    expect(screen.getByText("waitlist")).toBeInTheDocument();
    expect(screen.getByText("seated")).toBeInTheDocument();
    expect(screen.getByText("completed")).toBeInTheDocument();
  });
});
