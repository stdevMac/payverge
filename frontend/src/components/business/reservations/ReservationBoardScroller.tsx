"use client";

import React from "react";
import { TABLERO_BOARD_MIN_HEIGHT_CLASS } from "./reservationBoardLayout";

/** Fixed lane width — must stay in lockstep with `reservationBoardLaneClass`. */
const RESERVATION_BOARD_LANE_WIDTH_PX = 240;
const RESERVATION_BOARD_LANE_GAP_PX = 16;
export const RESERVATION_BOARD_LANE_COUNT = 5;

export const reservationBoardLaneClass =
  "h-auto min-h-0 w-[240px] min-w-[240px] max-w-[240px] shrink-0 self-start p-4";

/**
 * Minimum paint width of the five host-stand lanes plus the 16px flex gap.
 * A typical dashboard content pane (~768–1024px after sidebar) is narrower,
 * so the track must live inside a bounded overflow scroller — not `w-full`.
 */
export function reservationBoardMinTrackWidthPx(
  laneCount = RESERVATION_BOARD_LANE_COUNT,
): number {
  if (laneCount <= 0) {
    return 0;
  }
  return (
    laneCount * RESERVATION_BOARD_LANE_WIDTH_PX +
    (laneCount - 1) * RESERVATION_BOARD_LANE_GAP_PX
  );
}

type ReservationBoardScrollerProps = {
  children: React.ReactNode;
  scrollHint?: string;
  /**
   * Stretch into a flex parent so Tablero occupies the remaining dashboard
   * pane instead of starting below the fold of an overflow-hidden shell.
   */
  fill?: boolean;
  className?: string;
};

const scrollerOverflowStyle: React.CSSProperties = {
  overflowX: "auto",
  overflowY: "auto",
  minWidth: 0,
  maxWidth: "100%",
};

/**
 * Host-stand Tablero scrollport.
 *
 * Prior remediations put `overflow-x-auto` on a `w-full min-w-0 max-w-full`
 * flex row, so the row sized to the pane and never created overflow; the
 * dashboard `h-[100dvh] overflow-hidden` shell then clipped Waitlist /
 * Seated / Completed. A later pass moved overflow onto a `w-max` track
 * wrapper, but the board still painted at y≈701 under KPI chrome while
 * window `scrollHeight` stayed ≈viewport — later lanes stayed unreachable
 * at 1024-class heights.
 *
 * The track is `w-max` (content-sized). The scroller is the only overflow
 * node: bounded width (`min-w-0 max-w-full`) plus a remaining-viewport
 * height so it participates in layout inside the hidden shell.
 */
export function ReservationBoardScroller({
  children,
  scrollHint,
  fill = false,
  className = "",
}: ReservationBoardScrollerProps) {
  const outerClass = [
    "flex min-w-0 w-full max-w-full flex-col",
    fill ? "min-h-0 flex-1" : "",
    className,
  ]
    .filter(Boolean)
    .join(" ");

  const scrollerClass = [
    "min-w-0 w-full max-w-full overflow-x-auto overflow-y-auto overscroll-contain pb-2 scrollbar-thin scrollbar-thumb-warm-400/90 scrollbar-track-warm-100/80 [scrollbar-gutter:stable]",
    TABLERO_BOARD_MIN_HEIGHT_CLASS,
    fill ? "flex-1" : "max-h-[min(36rem,calc(100dvh-14rem))]",
  ].join(" ");

  return (
    <div className={outerClass}>
      {scrollHint ? (
        <p className="mb-1 shrink-0 text-xs text-ink-500">{scrollHint}</p>
      ) : null}
      <div
        data-testid="reservations-board-scroller"
        data-reservations-board-scroller="true"
        className={scrollerClass}
        style={scrollerOverflowStyle}
      >
        <div
          data-testid="reservations-board"
          className="flex w-max min-w-max items-start gap-4"
        >
          {children}
        </div>
      </div>
    </div>
  );
}
