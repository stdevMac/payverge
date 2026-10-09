/** @jest-environment jsdom */
import React from "react";
import { render, screen } from "@testing-library/react";
import {
  RESERVATION_BOARD_LANE_COUNT,
  ReservationBoardScroller,
  reservationBoardLaneClass,
  reservationBoardMinTrackWidthPx,
} from "./ReservationBoardScroller";

function renderBoard() {
  return render(
    <ReservationBoardScroller scrollHint="Scroll sideways to reach later columns">
      {["pending", "confirmed", "waitlist", "seated", "completed"].map(
        (lane) => (
          <div
            key={lane}
            className={reservationBoardLaneClass}
            data-testid="reservation-board-lane"
            data-lane={lane}
          >
            {lane}
          </div>
        ),
      )}
    </ReservationBoardScroller>,
  );
}

describe("ReservationBoardScroller", () => {
  it("sizes the five-lane track wider than a dashboard content pane", () => {
    expect(reservationBoardMinTrackWidthPx()).toBe(1264);
    expect(reservationBoardMinTrackWidthPx()).toBeGreaterThan(1024);
    expect(reservationBoardMinTrackWidthPx(1)).toBe(240);
    expect(reservationBoardMinTrackWidthPx(0)).toBe(0);
  });

  it("puts overflow on the scroller and keeps the track content-sized", () => {
    renderBoard();

    const scroller = screen.getByTestId("reservations-board-scroller");
    const track = screen.getByTestId("reservations-board");

    expect(scroller).toHaveAttribute("data-reservations-board-scroller", "true");
    expect(scroller).toHaveStyle({ overflowX: "auto", overflowY: "auto" });
    expect(scroller.className).toMatch(/overflow-x-auto/);
    expect(scroller.className).toMatch(/overflow-y-auto/);
    expect(scroller.className).toMatch(/min-w-0/);
    expect(scroller.className).toMatch(/\bw-full\b/);
    expect(scroller.className).toMatch(/max-w-full/);
    expect(scroller.className).toMatch(
      /min-h-\[min\(16rem,calc\(100dvh-22rem\)\)\]/,
    );
    expect(scroller.className).toMatch(/max-h-\[min\(36rem,calc\(100dvh-14rem\)\)\]/);

    expect(track.className).toMatch(/\bflex\b/);
    expect(track.className).toMatch(/\bw-max\b/);
    expect(track.className).toMatch(/min-w-max/);
    expect(track.className).not.toMatch(/\bw-full\b/);
    expect(track.className).not.toMatch(/max-w-full/);
    expect(track.className).not.toMatch(/min-w-0/);
    expect(track.className).not.toMatch(/overflow-x-auto/);

    const lanes = screen.getAllByTestId("reservation-board-lane");
    expect(lanes).toHaveLength(RESERVATION_BOARD_LANE_COUNT);
    lanes.forEach((lane) => {
      expect(lane.className).toMatch(/shrink-0/);
      expect(lane.className).toMatch(/min-w-\[240px\]/);
    });
    expect(screen.getByText("completed")).toBeInTheDocument();
  });

  it("fills a flex parent so the board occupies remaining dashboard height", () => {
    render(
      <div className="flex h-[40rem] flex-col">
        <ReservationBoardScroller fill scrollHint="Scroll sideways to reach later columns">
          {["pending", "confirmed", "waitlist", "seated", "completed"].map(
            (lane) => (
              <div
                key={lane}
                className={reservationBoardLaneClass}
                data-testid="reservation-board-lane"
                data-lane={lane}
              >
                {lane}
              </div>
            ),
          )}
        </ReservationBoardScroller>
      </div>,
    );

    const scroller = screen.getByTestId("reservations-board-scroller");
    expect(scroller.className).toMatch(/flex-1/);
    expect(scroller.className).toMatch(
      /min-h-\[min\(16rem,calc\(100dvh-22rem\)\)\]/,
    );
    expect(scroller.className).not.toMatch(/max-h-\[min\(36rem/);
  });
});
