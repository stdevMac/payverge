/** @jest-environment jsdom */
// Pin a device timezone that differs from the business timezone under test so
// the "renders in browser TZ" bug is deterministic: 19:00Z is 16:00 in
// Buenos Aires (UTC-3) but 15:00 in America/New_York (EDT).
process.env.TZ = "America/Argentina/Buenos_Aires";
import React from "react";
import { render, screen, waitFor } from "@testing-library/react";
import "@testing-library/jest-dom";
import ReservationsTodayCard from "../ReservationsTodayCard";

jest.mock("@/i18n/SimpleTranslationProvider", () => ({
  useSimpleLocale: () => ({ locale: "en", setLocale: jest.fn() }),
  getTranslation: (key: string) => key,
}));

const todayISO = (h: number) => {
  const d = new Date();
  d.setHours(h, 30, 0, 0);
  return d.toISOString();
};

const mockGetStats = jest.fn(() =>
  Promise.resolve({
    total: 4,
    pending: 1,
    confirmed: 2,
    waitlist: 0,
    seated: 0,
    completed: 0,
    cancelled: 1,
    no_show: 1,
  }),
);

const mockGetUpcoming = jest.fn(() =>
  Promise.resolve({
    reservations: [
      { id: 1, status: "confirmed", reservation_time: todayISO(23) },
      { id: 2, status: "confirmed", reservation_time: todayISO(22) },
    ],
    total: 2,
  }),
);

// Horizon-wide pending approvals: total 3 pending (one today, two future) —
// deliberately more than stats.pending (today-only = 1) so the test proves the
// chip counts every pending request, not just today's.
const mockGetPendingList = jest.fn(() =>
  Promise.resolve({
    reservations: [{ id: 9, status: "pending", reservation_time: todayISO(21) }],
    total: 3,
    page: 1,
    page_size: 1,
    total_pages: 3,
  }),
);

jest.mock("@/api/reservations", () => ({
  reservationAPI: {
    getStats: (...args: unknown[]) => mockGetStats(...(args as [])),
    getUpcomingReservations: (...args: unknown[]) =>
      mockGetUpcoming(...(args as [])),
    getReservations: (...args: unknown[]) =>
      mockGetPendingList(...(args as [])),
  },
}));

beforeEach(() => {
  mockGetStats.mockClear();
  mockGetUpcoming.mockClear();
  mockGetPendingList.mockClear();
  mockGetStats.mockImplementation(() =>
    Promise.resolve({
      total: 4,
      pending: 1,
      confirmed: 2,
      waitlist: 0,
      seated: 0,
      completed: 0,
      cancelled: 1,
      no_show: 1,
    }),
  );
  mockGetUpcoming.mockImplementation(() =>
    Promise.resolve({
      reservations: [
        { id: 1, status: "confirmed", reservation_time: todayISO(23) },
        { id: 2, status: "confirmed", reservation_time: todayISO(22) },
      ],
      total: 2,
    }),
  );
});

it("shows covers = total - cancelled - no_show from stats, plus pending", async () => {
  render(<ReservationsTodayCard businessId={1} onNavigate={jest.fn()} />);

  await waitFor(() =>
    expect(
      screen.getByText(/reservations\.todayCard\.title/),
    ).toBeInTheDocument(),
  );
  // 4 - 1 cancelled - 1 no_show = 2 covers
  expect(screen.getByText(/reservations\.todayCard\.title/)).toHaveTextContent(
    "2",
  );
  expect(screen.getByText(/todayCard\.pending/)).toBeInTheDocument();
  const card = screen.getByTestId("reservations-today-card");
  expect(card.textContent).toMatch(/todayCard\.(nextAt|nextOn|noneUpcoming)/);
});

it("counts pending approvals across the whole horizon, not just today", async () => {
  render(<ReservationsTodayCard businessId={1} onNavigate={jest.fn()} />);

  await waitFor(() =>
    expect(
      screen.getByText(/reservations\.todayCard\.title/),
    ).toBeInTheDocument(),
  );
  // stats.pending (today-only) is 1, but the undated pending list reports
  // total 3 — a far-future request awaiting approval must still surface.
  expect(screen.getByText(/reservations\.todayCard\.title/)).toHaveTextContent(
    "3",
  );
  // The pending total rides a 1-row page: server-exact COUNT, no hydrate.
  // L9-2: AbortSignal is threaded through options.signal.
  expect(mockGetPendingList).toHaveBeenCalledWith(
    1,
    undefined,
    undefined,
    "pending",
    expect.objectContaining({ pageSize: 1, signal: expect.any(AbortSignal) }),
  );
});

it("queries stats for local today and a bounded upcoming list", async () => {
  render(<ReservationsTodayCard businessId={7} onNavigate={jest.fn()} />);

  await waitFor(() => expect(mockGetStats).toHaveBeenCalledTimes(1));
  // L9-2: third arg is AbortSignal for real cancel-on-unmount
  expect(mockGetUpcoming).toHaveBeenCalledWith(7, 5, expect.any(AbortSignal));

  const now = new Date();
  const pad = (n: number) => String(n).padStart(2, "0");
  const localToday = `${now.getFullYear()}-${pad(now.getMonth() + 1)}-${pad(now.getDate())}`;

  expect(mockGetStats).toHaveBeenCalledWith(
    7,
    localToday,
    localToday,
    expect.any(AbortSignal),
  );
});

it("L1-10: with businessTimezone, stats use the restaurant day (not device day)", async () => {
  // Device TZ is America/Argentina/Buenos_Aires (process.env.TZ above).
  // Pick an instant that is still "yesterday" in America/Los_Angeles.
  jest.useFakeTimers({ advanceTimers: true });
  try {
    jest.setSystemTime(new Date("2026-03-15T03:30:00.000Z"));

    const { businessDateKey } =
      require("@/utils/businessTime") as typeof import("@/utils/businessTime");
    const businessToday = businessDateKey(
      new Date("2026-03-15T03:30:00.000Z"),
      "America/Los_Angeles",
    );
    expect(businessToday).toBe("2026-03-14");

    // Device-local key under America/Argentina/Buenos_Aires for that UTC instant
    // is 2026-03-15 (00:30 ART) — deliberately different from LA business day.
    const deviceToday = "2026-03-15";

    render(
      <ReservationsTodayCard
        businessId={7}
        businessTimezone="America/Los_Angeles"
        onNavigate={jest.fn()}
      />,
    );

    await waitFor(() => expect(mockGetStats).toHaveBeenCalled());
    expect(mockGetStats).toHaveBeenCalledWith(
      7,
      businessToday,
      businessToday,
      expect.any(AbortSignal),
    );
    // Must not query the device day — that was the L1-10 Overview/Reservas split.
    expect(mockGetStats).not.toHaveBeenCalledWith(
      7,
      deviceToday,
      deviceToday,
      expect.any(AbortSignal),
    );
  } finally {
    jest.useRealTimers();
  }
});

it("shows the stale banner instead of fabricating zero when the API fails (L9-2)", async () => {
  mockGetStats.mockImplementation(() => Promise.reject(new Error("boom")));

  render(<ReservationsTodayCard businessId={1} onNavigate={jest.fn()} />);

  await waitFor(() =>
    expect(screen.getByTestId("stale-widget-banner")).toBeInTheDocument(),
  );
  // Must not invent "Reservas de hoy: 0" as if it were real data
  expect(screen.queryByTestId("reservations-today-card")).not.toBeInTheDocument();
});

it("uses nextUpcomingArrival statuses and labels multi-day arrivals with a date", async () => {
  // Tomorrow 19:00 local — must not look like "today at 7pm" without a date.
  const tomorrow = new Date();
  tomorrow.setDate(tomorrow.getDate() + 1);
  tomorrow.setHours(19, 0, 0, 0);

  mockGetUpcoming.mockImplementation(() =>
    Promise.resolve({
      reservations: [
        // cancelled must be ignored by the shared predicate
        {
          id: 1,
          status: "cancelled",
          reservation_time: todayISO(23),
        },
        {
          id: 2,
          status: "confirmed",
          reservation_time: tomorrow.toISOString(),
        },
      ],
      total: 2,
    }),
  );

  render(
    <ReservationsTodayCard
      businessId={1}
      businessTimezone="America/Argentina/Buenos_Aires"
      onNavigate={jest.fn()}
    />,
  );

  await waitFor(() =>
    expect(
      screen.getByText(/reservations\.todayCard\.title/),
    ).toBeInTheDocument(),
  );

  // With getTranslation mocked to return the key, the multi-day branch uses nextOn.
  expect(screen.getByText(/todayCard\.nextOn/)).toBeInTheDocument();
  expect(screen.queryByText(/todayCard\.nextAt$/)).not.toBeInTheDocument();
});

it("shows noneUpcoming when only cancelled/seated remain", async () => {
  mockGetUpcoming.mockImplementation(() =>
    Promise.resolve({
      reservations: [
        { id: 1, status: "cancelled", reservation_time: todayISO(23) },
        { id: 2, status: "seated", reservation_time: todayISO(22) },
      ],
      total: 2,
    }),
  );

  render(<ReservationsTodayCard businessId={1} onNavigate={jest.fn()} />);

  await waitFor(() =>
    expect(
      screen.getByText(/reservations\.todayCard\.title/),
    ).toBeInTheDocument(),
  );
  expect(screen.getByText(/todayCard\.noneUpcoming/)).toBeInTheDocument();
});
