/** @jest-environment jsdom */
import React from "react";
import { render, screen, act, waitFor } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import TodayCard, { type TodayCardLabels } from "./TodayCard";
import { timeclockApi, type TimeEntry } from "@/api/timeclock";
import type { StaffData } from "@/utils/staffAuth";

jest.mock("@/api/timeclock", () => ({
  timeclockApi: {
    myTimesheet: jest.fn(),
    clockIn: jest.fn(),
    clockOut: jest.fn(),
    addBreak: jest.fn(),
  },
}));

const mocked = timeclockApi as unknown as {
  myTimesheet: jest.Mock;
  clockIn: jest.Mock;
  clockOut: jest.Mock;
  addBreak: jest.Mock;
};

const staff: StaffData = {
  id: 7,
  name: "Dana",
  email: "dana@example.com",
  role: "server",
  business_id: 42,
  business_name: "Cafe 42",
  business_slug: "cafe-42",
  is_active: true,
};

const labels: TodayCardLabels = {
  title: "Today",
  serviceToolsTitle: "Service tools",
  serviceToolsDescription: "Open the tools you use during your shift.",
  serviceToolsOpen: "Open service tools",
  loading: "Loading your day",
  clockIn: "Clock in",
  clockOut: "Clock out",
  clockingIn: "Clocking in",
  clockingOut: "Clocking out",
  onTheClock: "On the clock",
  sinceTemplate: "Since {time}",
  notClockedIn: "You're not clocked in yet.",
  workedToday: "Worked today",
  breakLabel: "Add a break",
  breakPreset: "+{minutes}m",
  hoursUnit: "h",
  minutesUnit: "m",
  clockInError: "We couldn't clock you in.",
  clockOutError: "We couldn't clock you out.",
  breakError: "We couldn't add that break.",
};

const openEntry: TimeEntry = {
  id: 1,
  business_id: 42,
  staff_id: 7,
  shift_id: null,
  clock_in_at: "2026-06-30T12:00:00.000Z",
  clock_out_at: null,
  break_minutes: 0,
  source: "staff_punch",
  status: "open",
  approved_by_staff_id: null,
  note: "",
  created_at: "",
  updated_at: "",
  worked_minutes: 0,
  worked_hours: 0,
};

function renderCard() {
  const qc = new QueryClient({ defaultOptions: { queries: { retry: false } } });
  return render(
    <QueryClientProvider client={qc}>
      <TodayCard staff={staff} labels={labels} locale="en" />
    </QueryClientProvider>,
  );
}

afterEach(() => {
  jest.useRealTimers();
  jest.clearAllMocks();
});

describe("TodayCard time clock", () => {
  it("not clocked in: shows Clock in and clicking it calls timeclock.clockIn", async () => {
    mocked.myTimesheet.mockResolvedValue([]);
    mocked.clockIn.mockResolvedValue(openEntry);

    renderCard();

    const btn = await screen.findByRole("button", { name: "Clock in" });
    expect(screen.getByText("You're not clocked in yet.")).toBeInTheDocument();

    await userEvent.click(btn);
    await waitFor(() => expect(mocked.clockIn).toHaveBeenCalledWith("42"));

    // The service-tools deep-link survives (the next-shift hero moved to
    // StaffTodayHome, which composes TodayCard).
    expect(screen.getByText("Open service tools")).toBeInTheDocument();
    // Staff-facing guard: never a dollar amount.
    expect(document.body.textContent).not.toMatch(/\$\d/);
  });

  it("clocked in: shows the running elapsed timer + Clock out, ticking each second", async () => {
    jest.useFakeTimers();
    jest.setSystemTime(new Date("2026-06-30T12:00:05.000Z"));
    mocked.myTimesheet.mockResolvedValue([openEntry]);

    renderCard();
    // Flush the resolved React Query promise under fake timers.
    await act(async () => {
      await jest.advanceTimersByTimeAsync(0);
    });

    // 5 seconds elapsed since clock-in.
    expect(screen.getByText("0:00:05")).toBeInTheDocument();
    expect(screen.getByRole("button", { name: "Clock out" })).toBeInTheDocument();
    // The clock control derives the open state — no Clock-in button while on the clock.
    expect(screen.queryByRole("button", { name: "Clock in" })).not.toBeInTheDocument();

    // One more second ticks the running timer.
    await act(async () => {
      await jest.advanceTimersByTimeAsync(1000);
    });
    expect(screen.getByText("0:00:06")).toBeInTheDocument();

    // Staff-facing guard: hours/minutes only, never a dollar amount.
    expect(document.body.textContent).not.toMatch(/\$\d/);
  });
});
