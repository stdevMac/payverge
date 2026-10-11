/** @jest-environment jsdom */
import React from "react";
import { render, screen } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import ScheduleWeekMobile from "./ScheduleWeekMobile";
import { localDateKey } from "@/lib/localDate";
import type { Shift } from "@/api/schedule";

const t = (key: string) => key;
const formatTimeRange = () => "9:00 AM – 5:00 PM";
const fmtMinutes = (m: number) => `${m}m`;

// Build a week where today is the second day, so we can prove the switcher
// defaults to today.
const today = new Date();
const dayBefore = new Date(today);
dayBefore.setDate(today.getDate() - 1);
const dayAfter = new Date(today);
dayAfter.setDate(today.getDate() + 1);
const days = [dayBefore, today, dayAfter].map(localDateKey);
const todayKey = localDateKey(today);

const rows = [
  { key: "r1", label: "Servers", positionId: 1, colorHex: "#1a6b6a" },
];

const todayShift = {
  id: 10,
  staff_id: null,
  position_id: 1,
  starts_at: `${todayKey}T09:00:00`,
  ends_at: `${todayKey}T17:00:00`,
  break_minutes: 0,
} as unknown as Shift;

function baseProps(overrides = {}) {
  return {
    days,
    rows,
    mode: "role" as const,
    locale: "en",
    t,
    shiftsForCell: (_row: unknown, dayKey: string) =>
      dayKey === todayKey ? [todayShift] : [],
    positionsById: new Map([[1, { id: 1, name: "Server" } as never]]),
    staffById: new Map(),
    teamAvailability: {},
    approvedTimeOff: [],
    dayTotalMinutes: new Map([[todayKey, 480]]),
    formatTimeRange,
    fmtMinutes,
    onEditShift: jest.fn(),
    onCreateShift: jest.fn(),
    ...overrides,
  };
}

it("uses a venue todayKey when the device calendar day is a different column", () => {
  const venueToday = days[0];
  render(<ScheduleWeekMobile {...baseProps({ todayKey: venueToday })} />);
  const selected = screen.getByRole("tab", { selected: true });
  expect(selected).toHaveAttribute("aria-current", "date");
  expect(screen.queryByText("9:00 AM – 5:00 PM")).not.toBeInTheDocument();
});

it("defaults to today and lists that day's shifts", () => {
  render(<ScheduleWeekMobile {...baseProps()} />);
  // Today's open shift is visible by default.
  expect(screen.getByText("9:00 AM – 5:00 PM")).toBeInTheDocument();
  expect(screen.getByText("open")).toBeInTheDocument();
  // Today's chip is selected.
  const selected = screen.getByRole("tab", { selected: true });
  expect(selected).toHaveAttribute("aria-current", "date");
});

it("switches days and shows an empty day with just the add-shift action", async () => {
  render(<ScheduleWeekMobile {...baseProps()} />);
  const tabs = screen.getAllByRole("tab");
  // Click the first (yesterday) chip — it has no shifts.
  await userEvent.click(tabs[0]);
  expect(screen.queryByText("9:00 AM – 5:00 PM")).not.toBeInTheDocument();
  // Add-shift is always offered.
  expect(screen.getByLabelText(/addShiftFor/)).toBeInTheDocument();
});

it("wires edit/create actions", async () => {
  const onEditShift = jest.fn();
  const onCreateShift = jest.fn();
  render(
    <ScheduleWeekMobile
      {...baseProps({ onEditShift, onCreateShift })}
    />,
  );
  await userEvent.click(screen.getByText("9:00 AM – 5:00 PM"));
  expect(onEditShift).toHaveBeenCalledWith(todayShift);

  await userEvent.click(screen.getByLabelText(/addShiftFor/));
  expect(onCreateShift).toHaveBeenCalledWith({
    dayKey: todayKey,
    staffId: null,
    positionId: 1,
  });
});
