/** @jest-environment jsdom */
/**
 * D1 / L8-2 residual: overview next-arrival must use named DATE_MONTH_DAY +
 * TIME_SHORT presets (no ad-hoc { month, day } / { hour, minute } objects).
 * Asserts the rendered card label, not only a source grep.
 */
process.env.TZ = "UTC";
import React from "react";
import { render, screen, waitFor } from "@testing-library/react";
import {
  DATE_MONTH_DAY,
  TIME_SHORT,
  formatBusinessDateTime,
  formatBusinessTime,
} from "@/utils/businessTime";

jest.mock("@/i18n/SimpleTranslationProvider", () => ({
  useSimpleLocale: () => ({ locale: "en", setLocale: jest.fn() }),
  getTranslation: (
    key: string,
    _locale?: string,
    params?: Record<string, string | number>,
  ) => {
    if (key.endsWith("todayCard.nextOn")) {
      return `Next on ${params?.date} at ${params?.time}`;
    }
    if (key.endsWith("todayCard.nextAt")) {
      return `Next at ${params?.time}`;
    }
    if (key.endsWith("todayCard.noneUpcoming")) return "None upcoming";
    if (key.endsWith("todayCard.title")) return `Today: ${params?.count ?? ""}`;
    if (key.endsWith("todayCard.pending")) return "Pending";
    return key;
  },
}));

// Future arrival so the card uses nextOn (date + time), not nextAt.
// mock* prefix required for jest.mock factory access.
const mockFuture = new Date();
mockFuture.setUTCDate(mockFuture.getUTCDate() + 3);
mockFuture.setUTCHours(19, 0, 0, 0);
const mockFutureIso = mockFuture.toISOString();

jest.mock("@/api/reservations", () => ({
  reservationAPI: {
    getStats: jest.fn(() =>
      Promise.resolve({
        total: 1,
        pending: 0,
        confirmed: 1,
        waitlist: 0,
        seated: 0,
        completed: 0,
        cancelled: 0,
        no_show: 0,
      }),
    ),
    getUpcomingReservations: jest.fn(() =>
      Promise.resolve({
        reservations: [
          {
            id: 1,
            status: "confirmed",
            reservation_time: mockFutureIso,
          },
        ],
        total: 1,
      }),
    ),
    getReservations: jest.fn(() =>
      Promise.resolve({
        reservations: [],
        total: 0,
        page: 1,
        page_size: 1,
        total_pages: 0,
      }),
    ),
  },
}));

import ReservationsTodayCard from "../ReservationsTodayCard";

describe("ReservationsTodayCard L8-2 named date presets (D1)", () => {
  it("renders next-on label with DATE_MONTH_DAY + TIME_SHORT output", async () => {
    render(
      <ReservationsTodayCard
        businessId={1}
        businessTimezone="UTC"
        onNavigate={jest.fn()}
      />,
    );

    const expectedDate = formatBusinessDateTime(
      mockFutureIso,
      "en",
      "UTC",
      DATE_MONTH_DAY,
    );
    const expectedTime = formatBusinessTime(
      mockFutureIso,
      "en",
      "UTC",
      TIME_SHORT,
    );

    await waitFor(() => {
      const card = screen.getByTestId("reservations-today-card");
      expect(card.textContent).toContain(expectedDate);
      expect(card.textContent).toContain(expectedTime);
      expect(card.textContent).toMatch(/Next on/);
      // Compact DATE_MONTH_DAY must not inject a year into the next-on line.
      // (DATE_SHORT would add ", 2026" and fail this.)
      const nextLine = (card.textContent || "").match(/Next on[^\n]*/)?.[0] || "";
      expect(nextLine).not.toMatch(/202\d/);
      expect(nextLine).toContain(expectedDate);
    });

    // Compact preset must not force a year into the overview chip.
    expect(expectedDate).not.toMatch(/202\d/);
  });

  it("source uses DATE_MONTH_DAY / TIME_SHORT (no ad-hoc month/hour objects)", () => {
    const fs = require("fs") as typeof import("fs");
    const path = require("path") as typeof import("path");
    const src = fs.readFileSync(
      path.join(__dirname, "..", "ReservationsTodayCard.tsx"),
      "utf8",
    );
    expect(src).toMatch(/DATE_MONTH_DAY/);
    expect(src).toMatch(/TIME_SHORT/);
    expect(src).not.toMatch(
      /formatBusinessDateTime\([\s\S]{0,120}\{\s*month:\s*["']short["']/,
    );
    expect(src).not.toMatch(
      /formatBusinessTime\([\s\S]{0,120}\{\s*hour:\s*["']numeric["']/,
    );
  });
});
