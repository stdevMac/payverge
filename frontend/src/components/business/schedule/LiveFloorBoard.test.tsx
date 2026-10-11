/** @jest-environment jsdom */
import React from "react";
import { render, screen, waitFor } from "@testing-library/react";
import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import LiveFloorBoard from "./LiveFloorBoard";
import { timeclockApi, type LiveFloor } from "@/api/timeclock";

// Minimal en copy so assertions read against real strings (with placeholders).
const COPY: Record<string, string> = {
  "dashboardLiveFloor.title": "On the floor today",
  "dashboardLiveFloor.chipOnClock": "on the clock",
  "dashboardLiveFloor.chipLate": "late",
  "dashboardLiveFloor.chipNoShow": "no-show",
  "dashboardLiveFloor.since": "Since {time}",
  "dashboardLiveFloor.minLate": "{minutes} min late",
  "dashboardLiveFloor.footerScheduled": "{count} scheduled",
  "dashboardLiveFloor.footerDone": "{count} done",
  "dashboardLiveFloor.unknownStaff": "Team member",
};

jest.mock("@/i18n/SimpleTranslationProvider", () => ({
  useSimpleLocale: () => ({ locale: "en", setLocale: jest.fn() }),
  getTranslation: (key: string) => COPY[key] ?? key,
}));

jest.mock("@/api/timeclock", () => ({
  timeclockApi: { liveFloor: jest.fn() },
}));

const mockedLiveFloor = timeclockApi as unknown as { liveFloor: jest.Mock };

function renderBoard(businessTimezone: string | null = null) {
  const qc = new QueryClient({ defaultOptions: { queries: { retry: false } } });
  return render(
    <QueryClientProvider client={qc}>
      <LiveFloorBoard
        businessId="42"
        businessTimezone={businessTimezone}
      />
    </QueryClientProvider>,
  );
}

beforeEach(() => jest.clearAllMocks());

const emptyFloor: LiveFloor = {
  date: "2026-06-30",
  rows: [],
  summary: { on_clock: 0, scheduled: 0, late: 0, no_show: 0, done: 0 },
};

const busyFloor: LiveFloor = {
  date: "2026-06-30",
  rows: [
    {
      staff_id: 7,
      staff_name: "Dana",
      status: "on_clock",
      shift_id: 1,
      clock_in_at: "2026-06-30T17:00:00.000Z",
    },
    {
      staff_id: 8,
      staff_name: "Sam",
      status: "late",
      shift_id: 2,
      shift_start: "2026-06-30T18:00:00.000Z",
      late_minutes: 15,
    },
    {
      staff_id: 9,
      staff_name: "Alex",
      status: "no_show",
      shift_id: 3,
      shift_start: "2026-06-30T12:00:00.000Z",
      shift_end: "2026-06-30T20:00:00.000Z",
    },
    { staff_id: 10, staff_name: "Robin", status: "scheduled", shift_id: 4 },
    { staff_id: 11, staff_name: "Jess", status: "done", shift_id: 5 },
  ],
  summary: { on_clock: 1, scheduled: 1, late: 1, no_show: 1, done: 1 },
};

test("renders nothing when the floor is empty (no clutter off-hours)", async () => {
  mockedLiveFloor.liveFloor.mockResolvedValue(emptyFloor);
  const { container } = renderBoard();
  // Give the query a tick to resolve, then assert still nothing.
  await waitFor(() => expect(mockedLiveFloor.liveFloor).toHaveBeenCalled());
  expect(screen.queryByText("On the floor today")).toBeNull();
  expect(container.querySelector("section")).toBeNull();
});

test("renders nothing when the read fails (non-manager 403 stays hidden)", async () => {
  mockedLiveFloor.liveFloor.mockRejectedValue(new Error("403"));
  renderBoard();
  await waitFor(() => expect(mockedLiveFloor.liveFloor).toHaveBeenCalled());
  expect(screen.queryByText("On the floor today")).toBeNull();
});

test("surfaces on-clock / late / no-show rows with counts, collapses scheduled+done to a footer", async () => {
  mockedLiveFloor.liveFloor.mockResolvedValue(busyFloor);
  renderBoard();

  expect(await screen.findByText("On the floor today")).toBeInTheDocument();

  // Attention rows appear individually...
  expect(screen.getByText("Dana")).toBeInTheDocument();
  expect(screen.getByText("Sam")).toBeInTheDocument();
  expect(screen.getByText("Alex")).toBeInTheDocument();
  expect(screen.getByText("15 min late")).toBeInTheDocument();

  // ...scheduled/done are NOT rows, only footer counts.
  expect(screen.queryByText("Robin")).toBeNull();
  expect(screen.queryByText("Jess")).toBeNull();
  expect(screen.getByText(/1 scheduled/)).toBeInTheDocument();
  expect(screen.getByText(/1 done/)).toBeInTheDocument();

  // Summary chips carry the counts.
  expect(screen.getByText(/1 on the clock/)).toBeInTheDocument();
  expect(screen.getByText(/1 late/)).toBeInTheDocument();
  expect(screen.getByText(/1 no-show/)).toBeInTheDocument();
});

test("orders attention rows no-show → late → on-clock", async () => {
  mockedLiveFloor.liveFloor.mockResolvedValue(busyFloor);
  renderBoard();
  await screen.findByText("On the floor today");
  const names = screen.getAllByRole("listitem").map((li) => li.textContent);
  expect(names[0]).toContain("Alex"); // no_show first
  expect(names[1]).toContain("Sam"); // late
  expect(names[2]).toContain("Dana"); // on_clock
});

test("money-free: no dollar sign anywhere on the board", async () => {
  mockedLiveFloor.liveFloor.mockResolvedValue(busyFloor);
  const { container } = renderBoard();
  await screen.findByText("On the floor today");
  expect(container.textContent).not.toContain("$");
});

test("renders no-show shift walls in the venue zone, not 4:00 AM UTC (#247)", async () => {
  mockedLiveFloor.liveFloor.mockResolvedValue({
    date: "2026-08-11",
    rows: [
      {
        staff_id: 9,
        staff_name: "Alex",
        status: "no_show",
        shift_id: 3,
        // 16:00–00:00 America/New_York dinner; UTC-wall 08:00–16:00 is 4AM–12PM.
        shift_start: "2026-08-11T20:00:00.000Z",
        shift_end: "2026-08-12T04:00:00.000Z",
      },
    ],
    summary: { on_clock: 0, scheduled: 0, late: 0, no_show: 1, done: 0 },
  });
  renderBoard("America/New_York");
  await screen.findByText("Alex");
  const body = document.body.textContent || "";
  expect(body).toMatch(/4:00\s*PM/);
  expect(body).toMatch(/12:00\s*AM/);
  expect(body).not.toMatch(/4:00\s*AM/);
});
