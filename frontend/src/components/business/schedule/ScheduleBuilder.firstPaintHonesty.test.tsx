/** @jest-environment jsdom */
/**
 * Related to issue 830: Schedule cold open stuck on "Loading…". A transient
 * blip in any of the four grid queries either sat in silent retry backoff or
 * (settings, retry:false) collapsed into a dead-end error panel with no way
 * back but a full remount. Cold open must render the grid or a REAL error —
 * one with a retry affordance — within bounded time.
 */
import React from "react";
import { render, screen } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import ScheduleBuilder from "./ScheduleBuilder";
import { scheduleApi } from "@/api/schedule";
import { scheduleSettingsApi } from "@/api/scheduleSettings";
import { positionsApi } from "@/api/positions";
import { getBusinessStaff } from "@/api/staff";
import { availabilityApi } from "@/api/availability";

jest.mock("@/i18n/SimpleTranslationProvider", () => ({
  useSimpleLocale: () => ({ locale: "en", setLocale: jest.fn() }),
  getTranslation: (key: string) => key,
}));
jest.mock("@/contexts/ToastContext", () => ({
  useToast: () => ({
    showSuccess: jest.fn(),
    showError: jest.fn(),
    showInfo: jest.fn(),
    showWarning: jest.fn(),
    showToast: jest.fn(),
  }),
}));
jest.mock("@/api/schedule", () => ({
  scheduleApi: {
    get: jest.fn(),
    createDraft: jest.fn(),
    createShift: jest.fn(),
    updateShift: jest.fn(),
    deleteShift: jest.fn(),
    publish: jest.fn(),
    laborPreview: jest.fn(),
    copyWeek: jest.fn(),
  },
}));
jest.mock("@/api/scheduleSettings", () => ({ scheduleSettingsApi: { get: jest.fn() } }));
jest.mock("@/api/positions", () => ({ positionsApi: { list: jest.fn() } }));
jest.mock("@/api/staff", () => ({ getBusinessStaff: jest.fn() }));
// Embedded panels fetch on mount; stub their APIs so no real XHR leaks
// (same rationale as ScheduleBuilder.test.tsx).
jest.mock("@/api/coverage", () => ({
  coverageApi: {
    listOpen: jest.fn(() =>
      Promise.resolve({
        open_shifts: [],
        swap_inbox: [],
        pending_approvals: { swaps: [], claims: [] },
      }),
    ),
    history: jest.fn(() => Promise.resolve([])),
    decide: jest.fn(() => Promise.resolve({})),
  },
}));
jest.mock("@/api/timeclock", () => ({
  timeclockApi: {
    liveFloor: jest.fn(() =>
      Promise.resolve({
        date: "2026-06-29",
        rows: [],
        summary: { on_clock: 0, scheduled: 0, late: 0, no_show: 0, done: 0 },
      }),
    ),
    kioskRoster: jest.fn(() => Promise.resolve([])),
    kioskPunch: jest.fn(() => Promise.resolve({})),
  },
}));
jest.mock("@/api/laborCost", () => ({
  laborCostApi: {
    getLaborCost: jest.fn(() =>
      Promise.resolve({
        period: "week",
        labor_cost: 0,
        net_sales: 0,
        labor_cost_pct: 0,
        payroll_run_count: 0,
        has_data: false,
        contributions: [],
        food_cost_pct: 0,
      }),
    ),
  },
}));
jest.mock("@/api/logbook", () => ({
  logbookApi: { list: jest.fn(() => Promise.resolve([])) },
}));
jest.mock("@/api/availability", () => ({
  availabilityApi: { listTimeOff: jest.fn(), decide: jest.fn(), getTeam: jest.fn() },
}));
jest.mock("@/hooks/useStaffRealtime", () => ({
  useStaffRealtime: () => ({ degraded: false, blocked: false, reconnect: jest.fn() }),
}));
jest.mock("./TimesheetReview", () => ({
  __esModule: true,
  default: () => null,
}));

const mockedSchedule = scheduleApi as unknown as Record<string, jest.Mock>;
const mockedSettings = scheduleSettingsApi as unknown as { get: jest.Mock };
const mockedPositions = positionsApi as unknown as { list: jest.Mock };
const mockedStaff = getBusinessStaff as unknown as jest.Mock;
const mockedAvailability = availabilityApi as unknown as {
  listTimeOff: jest.Mock;
  decide: jest.Mock;
  getTeam: jest.Mock;
};

function renderBuilder() {
  const qc = new QueryClient({ defaultOptions: { queries: { retry: false } } });
  return render(
    <QueryClientProvider client={qc}>
      <ScheduleBuilder businessId="42" businessTimezone={null} />
    </QueryClientProvider>,
  );
}

beforeEach(() => {
  jest.clearAllMocks();
  mockedSettings.get.mockResolvedValue({ week_start_day: 1 });
  mockedPositions.list.mockResolvedValue([
    {
      id: 3,
      business_id: 42,
      name: "Server",
      color_hex: "#1a6b6a",
      department: "FOH",
      is_active: true,
      sort_order: 0,
    },
  ]);
  mockedStaff.mockResolvedValue({
    staff: [
      {
        id: 7,
        name: "Dana",
        email: "dana@example.com",
        role: "server",
        business_id: 42,
        created_at: "",
        updated_at: "",
      },
    ],
    pending_invitations: [],
  });
  mockedSchedule.get.mockResolvedValue({
    schedule: {
      id: 5,
      business_id: 42,
      week_start: "2026-06-29",
      status: "draft",
      published_at: null,
      published_by_staff_id: null,
      notes: "",
      created_at: "",
      updated_at: "",
    },
    shifts: [],
  });
  mockedSchedule.laborPreview.mockResolvedValue({
    schedule_id: 5,
    can_see_dollars: false,
    total_hours: 0,
    labor_cost_pct: 0,
    positions: [],
    warnings: [],
  });
  mockedAvailability.listTimeOff.mockResolvedValue([]);
  mockedAvailability.getTeam.mockResolvedValue({});
});

describe("ScheduleBuilder first-paint honesty (issue 830)", () => {
  it("recovers the grid when the settings fetch blips once (cold-open flake)", async () => {
    mockedSettings.get
      .mockRejectedValueOnce(new Error("network blip"))
      .mockResolvedValue({ week_start_day: 1 });
    renderBuilder();
    expect(
      await screen.findByText("Dana", {}, { timeout: 6000 }),
    ).toBeInTheDocument();
  }, 15000);

  it("surfaces a real error with a retry button when settings keep failing, and retry recovers", async () => {
    mockedSettings.get.mockRejectedValue(new Error("still down"));
    renderBuilder();
    expect(
      await screen.findByText("dashboardSchedule.error", {}, { timeout: 6000 }),
    ).toBeInTheDocument();
    const retry = screen.getByRole("button", {
      name: "dashboardSchedule.errorRetry",
    });
    mockedSettings.get.mockResolvedValue({ week_start_day: 1 });
    await userEvent.click(retry);
    expect(
      await screen.findByText("Dana", {}, { timeout: 6000 }),
    ).toBeInTheDocument();
  }, 15000);

  it("surfaces a retryable error (not an endless skeleton) when the week read keeps failing", async () => {
    mockedSchedule.get.mockRejectedValue(new Error("still down"));
    renderBuilder();
    expect(
      await screen.findByText("dashboardSchedule.error", {}, { timeout: 6000 }),
    ).toBeInTheDocument();
    const retry = screen.getByRole("button", {
      name: "dashboardSchedule.errorRetry",
    });
    mockedSchedule.get.mockResolvedValue({
      schedule: {
        id: 5,
        business_id: 42,
        week_start: "2026-06-29",
        status: "draft",
        published_at: null,
        published_by_staff_id: null,
        notes: "",
        created_at: "",
        updated_at: "",
      },
      shifts: [],
    });
    await userEvent.click(retry);
    expect(
      await screen.findByText("Dana", {}, { timeout: 6000 }),
    ).toBeInTheDocument();
  }, 15000);
});
