/** @jest-environment jsdom */
import React from "react";
import { render, screen, waitFor } from "@testing-library/react";
import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import StaffTodayHome, {
  type StaffTodayHomeLabels,
} from "./StaffTodayHome";
import type { TodayCardLabels } from "./TodayCard";
import type { StaffData } from "@/utils/staffAuth";

jest.mock("@/hooks/useStaffRealtime", () => ({
  useStaffRealtime: jest.fn(),
}));
jest.mock("@/contexts/ToastContext", () => ({
  useToast: () => ({ showSuccess: jest.fn(), showError: jest.fn() }),
}));
jest.mock("@/api/schedule", () => ({
  scheduleApi: { get: jest.fn().mockResolvedValue({ shifts: [] }) },
}));
jest.mock("@/api/scheduleSettings", () => ({
  scheduleSettingsApi: { get: jest.fn().mockResolvedValue({ week_start_day: 1 }) },
}));
jest.mock("@/api/positions", () => ({
  positionsApi: { list: jest.fn().mockResolvedValue([]) },
}));
jest.mock("@/api/coverage", () => ({
  coverageApi: {
    listOpen: jest.fn().mockResolvedValue({ open_shifts: [] }),
    listMine: jest.fn().mockResolvedValue({ claims: [], swaps: [] }),
    claim: jest.fn(),
  },
}));
jest.mock("@/api/availability", () => ({
  availabilityApi: { listTimeOff: jest.fn().mockResolvedValue([]) },
}));
jest.mock("@/api/engagement", () => ({
  checklistsApi: { listRuns: jest.fn().mockResolvedValue([]) },
}));
jest.mock("@/api/chat", () => ({
  chatApi: {
    listAnnouncements: jest
      .fn()
      .mockResolvedValue({ announcements: [], acked: {} }),
    ackAnnouncement: jest.fn(),
  },
}));
jest.mock("@/api/timeclock", () => ({
  timeclockApi: {
    myTimesheet: jest.fn().mockResolvedValue([]),
    clockIn: jest.fn(),
    clockOut: jest.fn(),
    addBreak: jest.fn(),
  },
}));

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

const clockLabels: TodayCardLabels = {
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
  clockInError: "err",
  clockOutError: "err",
  breakError: "err",
};

const labels: StaffTodayHomeLabels = {
  nextShiftTitle: "Your next shift",
  onShiftNow: "On shift now",
  startsInMinutes: "Starts in {minutes}m",
  startsInHours: "Starts in {hours}h {minutes}m",
  startsTomorrow: "Tomorrow at {time}",
  noNextShiftTitle: "No upcoming shifts",
  noNextShiftHint: "You're all clear.",
  moreThisWeek: "{count} more coming up",
  seeSchedule: "See full schedule",
  positionFallback: "Shift",
  loading: "Loading your day…",
  openShiftsTitle: "Open shifts to pick up",
  claim: "Claim",
  claiming: "Claiming…",
  claimSuccess: "Claim sent",
  seeAllOpen: "See all open shifts",
  pendingTitle: "Your pending requests",
  pendingCoverage: "{count} coverage",
  pendingTimeOff: "{count} time off",
  manageRequests: "Manage requests",
  checklistTitle: "Today's checklist",
  checklistRemaining: "{count} left to finish",
  checklistDone: "All done",
  openChecklist: "Open checklist",
  announcementTitle: "Latest announcement",
  acknowledge: "Acknowledge",
  acknowledging: "Acknowledging…",
  acknowledged: "Acknowledged",
  ackSuccess: "Thanks",
  allAnnouncements: "All announcements",
  actionError: "Something went wrong.",
};

function renderHome() {
  const qc = new QueryClient({
    defaultOptions: { queries: { retry: false } },
  });
  return render(
    <QueryClientProvider client={qc}>
      <StaffTodayHome
        staff={staff}
        locale="en"
        labels={labels}
        clockLabels={clockLabels}
        onOpenSchedule={jest.fn()}
        onOpenSection={jest.fn()}
      />
    </QueryClientProvider>,
  );
}

describe("StaffTodayHome layout priority", () => {
  it("renders the clock-in card above the next-shift card", async () => {
    renderHome();

    // Clock-in action is the primary above-the-fold control.
    await screen.findByRole("button", { name: "Clock in" });

    const clockSection = screen.getByRole("region", { name: "Today" });
    const nextShiftSection = screen.getByRole("region", {
      name: "Your next shift",
    });

    // Clock card precedes the next-shift card in DOM order.
    const position = clockSection.compareDocumentPosition(nextShiftSection);
    expect(position & Node.DOCUMENT_POSITION_FOLLOWING).toBeTruthy();
  });

  it("keeps the next-shift card present as a secondary surface", async () => {
    renderHome();
    await waitFor(() =>
      expect(
        screen.getByRole("region", { name: "Your next shift" }),
      ).toBeInTheDocument(),
    );
    // Staff-facing guard: never a dollar amount.
    expect(document.body.textContent).not.toMatch(/\$\d/);
  });
});
