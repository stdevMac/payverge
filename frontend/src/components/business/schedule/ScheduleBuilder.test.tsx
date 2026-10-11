/** @jest-environment jsdom */
import React from "react";
import { render, screen, waitFor } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import ScheduleBuilder from "./ScheduleBuilder";
import { scheduleApi } from "@/api/schedule";
import { scheduleSettingsApi } from "@/api/scheduleSettings";
import { positionsApi } from "@/api/positions";
import { getBusinessStaff } from "@/api/staff";
import { availabilityApi } from "@/api/availability";
import {
  weekStartKey,
  shiftWeekKey,
  weekDayKeys,
  weekStartDayName,
} from "@/utils/scheduleWeek";
import { wallTimeToInstant } from "@/utils/zonedDateTime";

jest.mock("@/i18n/SimpleTranslationProvider", () => ({
  useSimpleLocale: () => ({ locale: "en", setLocale: jest.fn() }),
  getTranslation: (key: string) => key,
}));

const mockShowSuccess = jest.fn();
const mockShowError = jest.fn();
const mockShowInfo = jest.fn();
jest.mock("@/contexts/ToastContext", () => ({
  useToast: () => ({
    showSuccess: mockShowSuccess,
    showError: mockShowError,
    showInfo: mockShowInfo,
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
// ApprovalsPanel (mounted inside the builder) reads the coverage inbox on mount;
// LiveFloorBoard reads the live floor; OperatorLogbook reads today's notes. None
// are mocked by the builder's own contract, so their on-mount useQuery calls fire
// real XHR through axiosInstance and leak sockets that keep the worker alive.
// Stub each to a minimal valid shape so no suite opens a real connection.
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
// ApprovalsPanel (mounted inside the builder) reads time-off + opens an SSE stream.
jest.mock("@/api/availability", () => ({
  availabilityApi: { listTimeOff: jest.fn(), decide: jest.fn(), getTeam: jest.fn() },
}));
jest.mock("@/hooks/useStaffRealtime", () => ({
  useStaffRealtime: () => ({ degraded: false, blocked: false, reconnect: jest.fn() }),
}));
// TimesheetReview (mounted inside the builder, Slice 4) has its own coverage and
// pulls the time-clock/labor APIs — stub it so the builder tests stay focused.
// It still reports a (configurable) pending count so the inbox's auto-select
// behavior can be exercised.
let mockTimesheetCount = 0;
jest.mock("./TimesheetReview", () => {
  const R = jest.requireActual<typeof React>("react");
  function TimesheetReviewStub({
    onPendingCountChange,
  }: {
    onPendingCountChange?: (n: number) => void;
  }) {
    R.useEffect(() => {
      onPendingCountChange?.(mockTimesheetCount);
    }, [onPendingCountChange]);
    return null;
  }
  return { __esModule: true, default: TimesheetReviewStub };
});

const mockedSchedule = scheduleApi as unknown as Record<string, jest.Mock>;
const mockedSettings = scheduleSettingsApi as unknown as { get: jest.Mock };
const mockedPositions = positionsApi as unknown as { list: jest.Mock };
const mockedStaff = getBusinessStaff as unknown as jest.Mock;
const mockedAvailability = availabilityApi as unknown as {
  listTimeOff: jest.Mock;
  decide: jest.Mock;
  getTeam: jest.Mock;
};

/** One availability window per weekday for staff 7, of the given kind. */
function allWeekWindows(kind: "preferred" | "unavailable") {
  return {
    7: [0, 1, 2, 3, 4, 5, 6].map((weekday, i) => ({
      id: i + 1,
      staff_id: 7,
      business_id: 42,
      weekday,
      start_min: 480,
      end_min: 960,
      kind,
    })),
  };
}

function renderBuilder(businessTimezone: string | null = null) {
  const qc = new QueryClient({ defaultOptions: { queries: { retry: false } } });
  return render(
    <QueryClientProvider client={qc}>
      <ScheduleBuilder
        businessId="42"
        businessTimezone={businessTimezone}
      />
    </QueryClientProvider>,
  );
}

beforeEach(() => {
  jest.clearAllMocks();
  mockTimesheetCount = 0;
  mockedSettings.get.mockResolvedValue({ week_start_day: 1 });
  mockedPositions.list.mockResolvedValue([
    { id: 3, business_id: 42, name: "Server", color_hex: "#1a6b6a", department: "FOH", is_active: true, sort_order: 0 },
  ]);
  mockedStaff.mockResolvedValue({
    staff: [{ id: 7, name: "Dana", email: "dana@example.com", role: "server", business_id: 42, created_at: "", updated_at: "" }],
    pending_invitations: [],
  });
  mockedSchedule.get.mockResolvedValue({
    schedule: { id: 5, business_id: 42, week_start: "2026-06-29", status: "draft", published_at: null, published_by_staff_id: null, notes: "", created_at: "", updated_at: "" },
    shifts: [],
  });
  // Default to the non-financial preview shape (no dollars) so the footer renders
  // hours + % only and the builder's "no staff-facing dollars" assertion holds.
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

describe("ScheduleBuilder", () => {
  it("renders the week grid with employee rows", async () => {
    renderBuilder();
    await waitFor(() => expect(screen.getByText("Dana")).toBeInTheDocument());
    // Open-shifts row label present in employee mode.
    expect(screen.getByText("dashboardSchedule.openShifts")).toBeInTheDocument();
    // Per-cell add affordances render while the schedule is a draft.
    expect(screen.getAllByLabelText(/dashboardSchedule.addShiftFor/).length).toBeGreaterThan(0);
    // No staff-facing dollars leaked into the builder in Slice 2.
    expect(document.body.textContent).not.toMatch(/\$\d/);
  });

  it("opens the shift editor when an add cell is clicked", async () => {
    renderBuilder();
    await waitFor(() => expect(screen.getByText("Dana")).toBeInTheDocument());
    await userEvent.click(screen.getAllByLabelText(/dashboardSchedule.addShiftFor/)[0]);
    expect(screen.getByRole("dialog")).toBeInTheDocument();
    expect(screen.getByText("dashboardSchedule.editor.createTitle")).toBeInTheDocument();
  });

  it("renders availability exceptions only — no 'Available' wallpaper", async () => {
    mockedAvailability.getTeam.mockResolvedValue(allWeekWindows("preferred"));
    renderBuilder();
    await waitFor(() => expect(screen.getByText("Dana")).toBeInTheDocument());
    // Preferred windows no longer paint chips (or a legend entry).
    expect(
      screen.queryByText("dashboardSchedule.overlay.preferred"),
    ).not.toBeInTheDocument();
  });

  it("still surfaces unavailable windows as chips", async () => {
    mockedAvailability.getTeam.mockResolvedValue(allWeekWindows("unavailable"));
    renderBuilder();
    await waitFor(() => expect(screen.getByText("Dana")).toBeInTheDocument());
    await waitFor(() =>
      expect(
        screen.getAllByText("dashboardSchedule.overlay.unavailable").length,
      ).toBeGreaterThan(1), // legend + at least one cell chip
    );
  });

  it("shows the first-run hint on an empty week and marks today's column", async () => {
    renderBuilder();
    await waitFor(() => expect(screen.getByText("Dana")).toBeInTheDocument());
    expect(
      screen.getByText("dashboardSchedule.emptyWeekHint"),
    ).toBeInTheDocument();
    // The current week always contains today.
    expect(document.querySelector('[aria-current="date"]')).not.toBeNull();
  });

  it("builds the week in the venue zone, not the device clock, when the venue has no timezone", async () => {
    // 2026-10-05T02:30Z is Monday in UTC (the zone a venue without a timezone
    // resolves to) but still Sunday evening on any device west of UTC. The
    // grid, "today" and shift bucketing must all agree on the UTC day, so the
    // requested week is the one starting Monday 2026-10-05 and today's column
    // is marked, whatever zone the machine running this test is in. Jest
    // sandboxes process.env, so the device zone cannot be forced from here;
    // run with TZ=America/Argentina/Buenos_Aires to exercise the split day.
    jest.useFakeTimers({
      now: new Date("2026-10-05T02:30:00Z"),
      doNotFake: [
        "nextTick",
        "setImmediate",
        "clearImmediate",
        "setInterval",
        "clearInterval",
        "setTimeout",
        "clearTimeout",
        "queueMicrotask",
        "requestAnimationFrame",
        "cancelAnimationFrame",
        "requestIdleCallback",
        "cancelIdleCallback",
        "hrtime",
        "performance",
      ],
    });
    try {
      renderBuilder();
      await waitFor(() => expect(screen.getByText("Dana")).toBeInTheDocument());
      expect(mockedSchedule.get).toHaveBeenCalledWith("42", "2026-10-05");
      const today = document.querySelector('[aria-current="date"]');
      expect(today).not.toBeNull();
      expect(screen.getByTestId("schedule-week-first-day")).toHaveAttribute(
        "data-day-key",
        "2026-10-05",
      );
    } finally {
      jest.useRealTimers();
    }
  });

  it("copies last week via the transactional endpoint from the empty-week affordance", async () => {
    const currentKey = weekStartKey(new Date(), 1, "UTC");
    const prevKey = shiftWeekKey(currentKey, -1);
    // Empty current week → the copy affordance is shown.
    mockedSchedule.get.mockResolvedValue({
      schedule: { id: 5, business_id: 42, week_start: currentKey, status: "draft", published_at: null, published_by_staff_id: null, notes: "", created_at: "", updated_at: "" },
      shifts: [],
    });
    mockedSchedule.copyWeek.mockResolvedValue({ created: 3, skipped: 0 });
    renderBuilder();
    await waitFor(() => expect(screen.getByText("Dana")).toBeInTheDocument());
    await userEvent.click(
      screen.getByRole("button", { name: "dashboardSchedule.copyWeek.action" }),
    );
    // ONE transactional call with the from/to week keys, not N per-shift POSTs.
    await waitFor(() =>
      expect(mockedSchedule.copyWeek).toHaveBeenCalledWith("42", prevKey, currentKey, false),
    );
    expect(mockedSchedule.createShift).not.toHaveBeenCalled();
    await waitFor(() => expect(mockShowSuccess).toHaveBeenCalled());
  });

  it("refetches the labor preview after a copy-week", async () => {
    const currentKey = weekStartKey(new Date(), 1, "UTC");
    mockedSchedule.get.mockResolvedValue({
      schedule: { id: 5, business_id: 42, week_start: currentKey, status: "draft", published_at: null, published_by_staff_id: null, notes: "", created_at: "", updated_at: "" },
      shifts: [],
    });
    mockedSchedule.copyWeek.mockResolvedValue({ created: 2, skipped: 0 });
    renderBuilder();
    await waitFor(() =>
      expect(mockedSchedule.laborPreview).toHaveBeenCalled(),
    );
    const callsBefore = mockedSchedule.laborPreview.mock.calls.length;
    await userEvent.click(
      screen.getByRole("button", { name: "dashboardSchedule.copyWeek.action" }),
    );
    await waitFor(() => expect(mockShowSuccess).toHaveBeenCalled());
    // The footer's numbers come from a separate server-computed query — a stale
    // 0.0h/$0.00 after copying a week is exactly the bug this guards against.
    await waitFor(() =>
      expect(mockedSchedule.laborPreview.mock.calls.length).toBeGreaterThan(
        callsBefore,
      ),
    );
  });

  it("fans a new shift out across the repeat-on days", async () => {
    mockedSchedule.createShift.mockResolvedValue({});
    renderBuilder();
    await waitFor(() => expect(screen.getByText("Dana")).toBeInTheDocument());
    // Open the editor on the first add cell (sole position auto-selects).
    await userEvent.click(
      screen.getAllByRole("button", { name: /dashboardSchedule.addShiftFor/ })[0],
    );
    // Pick a second weekday pill, then save.
    const pills = screen.getAllByRole("checkbox");
    const extra = pills.find(
      (p) => p.getAttribute("aria-checked") === "false",
    ) as HTMLElement;
    await userEvent.click(extra);
    await userEvent.click(
      screen.getByRole("button", { name: "dashboardSchedule.editor.save" }),
    );
    await waitFor(() =>
      expect(mockedSchedule.createShift).toHaveBeenCalledTimes(2),
    );
  });

  // L5-26 decision #10: one attempt id per submit, per-day Idempotency-Key so a
  // lost-response retry replays days that already landed instead of duplicating.
  it("sends attempt-scoped per-day Idempotency-Key on multi-day create", async () => {
    const uuidSpy = jest
      .spyOn(globalThis.crypto, "randomUUID")
      .mockReturnValue("11111111-1111-1111-1111-111111111111");
    mockedSchedule.createShift.mockResolvedValue({});
    renderBuilder();
    await waitFor(() => expect(screen.getByText("Dana")).toBeInTheDocument());
    await userEvent.click(
      screen.getAllByRole("button", { name: /dashboardSchedule.addShiftFor/ })[0],
    );
    const pills = screen.getAllByRole("checkbox");
    const extra = pills.find(
      (p) => p.getAttribute("aria-checked") === "false",
    ) as HTMLElement;
    await userEvent.click(extra);
    await userEvent.click(
      screen.getByRole("button", { name: "dashboardSchedule.editor.save" }),
    );
    await waitFor(() =>
      expect(mockedSchedule.createShift).toHaveBeenCalledTimes(2),
    );
    const keys = mockedSchedule.createShift.mock.calls.map(
      (c) => c[2]?.idempotencyKey as string,
    );
    expect(keys).toHaveLength(2);
    expect(keys[0]).toMatch(
      /^11111111-1111-1111-1111-111111111111:\d{4}-\d{2}-\d{2}$/,
    );
    expect(keys[1]).toMatch(
      /^11111111-1111-1111-1111-111111111111:\d{4}-\d{2}-\d{2}$/,
    );
    // Same attempt id; distinct day suffixes.
    expect(keys[0].split(":")[0]).toBe(keys[1].split(":")[0]);
    expect(keys[0]).not.toBe(keys[1]);
    // One UUID minted for the whole fan-out, not one per day.
    expect(uuidSpy).toHaveBeenCalledTimes(1);
    uuidSpy.mockRestore();
  });

  it("reuses the same per-day Idempotency-Key after a partial create failure", async () => {
    const uuidSpy = jest
      .spyOn(globalThis.crypto, "randomUUID")
      .mockReturnValue("22222222-2222-2222-2222-222222222222");
    mockedSchedule.createShift
      .mockResolvedValueOnce({})
      .mockRejectedValueOnce(
        Object.assign(new Error("network lost"), { status: 0 }),
      )
      .mockResolvedValue({});
    renderBuilder();
    await waitFor(() => expect(screen.getByText("Dana")).toBeInTheDocument());
    await userEvent.click(
      screen.getAllByRole("button", { name: /dashboardSchedule.addShiftFor/ })[0],
    );
    const pills = screen.getAllByRole("checkbox");
    const extra = pills.find(
      (p) => p.getAttribute("aria-checked") === "false",
    ) as HTMLElement;
    await userEvent.click(extra);
    await userEvent.click(
      screen.getByRole("button", { name: "dashboardSchedule.editor.save" }),
    );
    await waitFor(() => expect(mockShowError).toHaveBeenCalled());
    // Editor stays open so the operator can retry the same attempt.
    expect(screen.getByRole("dialog")).toBeInTheDocument();

    const firstKeys = mockedSchedule.createShift.mock.calls.map(
      (c) => c[2]?.idempotencyKey as string,
    );
    expect(firstKeys).toHaveLength(2);

    await userEvent.click(
      screen.getByRole("button", { name: "dashboardSchedule.editor.save" }),
    );
    await waitFor(() =>
      expect(mockedSchedule.createShift.mock.calls.length).toBeGreaterThanOrEqual(
        4,
      ),
    );
    const retryKeys = mockedSchedule.createShift.mock.calls
      .slice(2)
      .map((c) => c[2]?.idempotencyKey as string);
    expect(retryKeys[0]).toBe(firstKeys[0]);
    expect(retryKeys[1]).toBe(firstKeys[1]);
    // Still a single attempt id — no second UUID on the retry.
    expect(uuidSpy).toHaveBeenCalledTimes(1);
    uuidSpy.mockRestore();
  });

  it("surfaces a 425 still-in-flight as soft info, not a hard save error", async () => {
    mockedSchedule.createShift.mockRejectedValue(
      Object.assign(new Error("Original request still in flight"), {
        status: 425,
        response: { status: 425, data: { error: "Original request still in flight — retry shortly" } },
      }),
    );
    renderBuilder();
    await waitFor(() => expect(screen.getByText("Dana")).toBeInTheDocument());
    await userEvent.click(
      screen.getAllByRole("button", { name: /dashboardSchedule.addShiftFor/ })[0],
    );
    await userEvent.click(
      screen.getByRole("button", { name: "dashboardSchedule.editor.save" }),
    );
    await waitFor(() =>
      expect(mockShowInfo).toHaveBeenCalledWith(
        "dashboardSchedule.saveInFlightTitle",
        "dashboardSchedule.saveInFlightBody",
      ),
    );
    expect(mockShowError).not.toHaveBeenCalled();
    // Keep the editor so the operator can retry with the same attempt key.
    expect(screen.getByRole("dialog")).toBeInTheDocument();
  });

  it("surfaces a 409 idempotency body mismatch without closing the editor", async () => {
    mockedSchedule.createShift.mockRejectedValue(
      Object.assign(new Error("Idempotency-Key already used with a different request body"), {
        status: 409,
        response: {
          status: 409,
          data: { error: "Idempotency-Key already used with a different request body" },
        },
      }),
    );
    renderBuilder();
    await waitFor(() => expect(screen.getByText("Dana")).toBeInTheDocument());
    await userEvent.click(
      screen.getAllByRole("button", { name: /dashboardSchedule.addShiftFor/ })[0],
    );
    await userEvent.click(
      screen.getByRole("button", { name: "dashboardSchedule.editor.save" }),
    );
    await waitFor(() =>
      expect(mockShowError).toHaveBeenCalledWith(
        "dashboardSchedule.saveConflictTitle",
        "dashboardSchedule.saveConflictBody",
      ),
    );
    expect(screen.getByRole("dialog")).toBeInTheDocument();
  });

  it("prefills a new shift from the person's latest shift this week", async () => {
    mockedSchedule.get.mockResolvedValue({
      schedule: { id: 5, business_id: 42, week_start: "2026-06-29", status: "draft", published_at: null, published_by_staff_id: null, notes: "", created_at: "", updated_at: "" },
      shifts: [
        {
          id: 92, business_id: 42, schedule_id: 5, staff_id: 7, position_id: 3,
          starts_at: "2026-06-30T12:00:00.000Z", ends_at: "2026-06-30T20:30:00.000Z",
          break_minutes: 45, status: "scheduled", published: false, notes: "",
          created_by_staff_id: 1, created_at: "", updated_at: "",
        },
      ],
    });
    renderBuilder();
    await waitFor(() => expect(screen.getByText("Dana")).toBeInTheDocument());
    // Open + Add on Dana's row (any empty cell in her row carries her staffId).
    await userEvent.click(
      screen.getAllByRole("button", { name: /dashboardSchedule.addShiftFor/ })[1],
    );
    // The editor opens with her last shift's times + break, not 09:00–17:00.
    expect(
      screen.getByLabelText("dashboardSchedule.editor.startTime"),
    ).toHaveValue("12:00");
    expect(
      screen.getByLabelText("dashboardSchedule.editor.endTime"),
    ).toHaveValue("20:30");
    // breakMinutes is a text input (inputMode="numeric", S-5 pattern) since
    // L5-27/L5-33 — DOM value is the string form.
    expect(
      screen.getByLabelText("dashboardSchedule.editor.breakMinutes"),
    ).toHaveValue("45");
  });

  it("shows weekly hours next to each person and per-day totals under the grid", async () => {
    // The builder always renders the *current* week (weekKey derives from
    // new Date() in the venue zone, UTC when the venue has none), and only
    // shifts whose venue date lands in that week are summed. Build the two shift days off the live week so this test doesn't
    // rot as the calendar advances past a hard-coded date.
    const currentKey = weekStartKey(new Date(), 1, "UTC");
    // Day index 1 (Tuesday) and 2 (Wednesday) of the current Monday-start week.
    const [tuesday, wednesday] = weekDayKeys(currentKey).slice(1, 3);
    mockedSchedule.get.mockResolvedValue({
      schedule: { id: 5, business_id: 42, week_start: currentKey, status: "draft", published_at: null, published_by_staff_id: null, notes: "", created_at: "", updated_at: "" },
      shifts: [
        {
          id: 92, business_id: 42, schedule_id: 5, staff_id: 7, position_id: 3,
          starts_at: `${tuesday}T12:00:00Z`, ends_at: `${tuesday}T20:30:00Z`,
          break_minutes: 45, status: "scheduled", published: false, notes: "",
          created_by_staff_id: 1, created_at: "", updated_at: "",
        },
        {
          id: 93, business_id: 42, schedule_id: 5, staff_id: 7, position_id: 3,
          starts_at: `${wednesday}T12:00:00Z`, ends_at: `${wednesday}T20:00:00Z`,
          break_minutes: 0, status: "scheduled", published: false, notes: "",
          created_by_staff_id: 1, created_at: "", updated_at: "",
        },
      ],
    });
    renderBuilder();
    await waitFor(() => expect(screen.getByText("Dana")).toBeInTheDocument());
    // Dana's week: 7h45 + 8h = 15h 45m net, with break honesty when breaks > 0.
    // The jest i18n mock returns the key (placeholders not interpolated).
    expect(
      await screen.findByText("dashboardSchedule.rowHoursWithBreak"),
    ).toBeInTheDocument();
    // Day totals row: Tuesday carries 7h 45m, Wednesday 8h.
    expect(screen.getByText("dashboardSchedule.dayTotals")).toBeInTheDocument();
    expect(screen.getByText("7h 45m")).toBeInTheDocument();
    expect(screen.getByText("8h")).toBeInTheDocument();
  });

  it("persists the sales target per business and rehydrates it", async () => {
    window.localStorage.setItem("payverge.salesTarget.42", "5000");
    renderBuilder();
    await waitFor(() => expect(screen.getByText("Dana")).toBeInTheDocument());
    await waitFor(() =>
      expect(mockedSchedule.laborPreview).toHaveBeenCalledWith("42", 5, 5000),
    );
    window.localStorage.removeItem("payverge.salesTarget.42");
  });

  it("speaks posted-late in the future tense while the week is a draft", async () => {
    mockedSchedule.laborPreview.mockResolvedValue({
      schedule_id: 5,
      can_see_dollars: false,
      total_hours: 8,
      labor_cost_pct: 0,
      positions: [],
      warnings: [{ code: "posted_late", detail: 3 }],
    });
    renderBuilder(); // default schedule fixture is a draft
    await waitFor(() =>
      expect(
        screen.getByText(/dashboardSchedule\.warnings\.postedLateDraft/),
      ).toBeInTheDocument(),
    );
    expect(
      screen.queryByText(/dashboardSchedule\.warnings\.postedLate$/),
    ).not.toBeInTheDocument();
  });

  it("switches the copy affordance to copy-into-empty-days once the week has shifts", async () => {
    mockedSchedule.get.mockResolvedValue({
      schedule: { id: 5, business_id: 42, week_start: "2026-06-29", status: "draft", published_at: null, published_by_staff_id: null, notes: "", created_at: "", updated_at: "" },
      shifts: [
        {
          id: 92, business_id: 42, schedule_id: 5, staff_id: 7, position_id: 3,
          starts_at: "2026-06-30T12:00:00.000Z", ends_at: "2026-06-30T20:00:00.000Z",
          break_minutes: 0, status: "scheduled", published: false, notes: "",
          created_by_staff_id: 1, created_at: "", updated_at: "",
        },
      ],
    });
    mockedSchedule.copyWeek.mockResolvedValue({ created: 1, skipped: 1 });
    renderBuilder();
    await waitFor(() => expect(screen.getByText("Dana")).toBeInTheDocument());
    // A partially-built week keeps the copy button, but as the idempotent
    // "copy into empty days" variant — the wholesale action + empty hint are gone.
    expect(
      screen.queryByRole("button", { name: "dashboardSchedule.copyWeek.action" }),
    ).not.toBeInTheDocument();
    expect(
      screen.queryByText("dashboardSchedule.emptyWeekHint"),
    ).not.toBeInTheDocument();
    const emptyDaysBtn = screen.getByRole("button", {
      name: "dashboardSchedule.copyWeek.actionEmptyDays",
    });
    await userEvent.click(emptyDaysBtn);
    // The onlyEmptyDays flag rides through to the transactional endpoint.
    await waitFor(() =>
      expect(mockedSchedule.copyWeek).toHaveBeenCalledWith(
        "42",
        expect.any(String),
        expect.any(String),
        true,
      ),
    );
  });

  it("keeps add-shift affordances and drops the Publish button on a published week", async () => {
    mockedSchedule.get.mockResolvedValue({
      schedule: { id: 5, business_id: 42, week_start: "2026-06-29", status: "published", published_at: "2026-06-28T00:00:00Z", published_by_staff_id: 1, notes: "", created_at: "", updated_at: "" },
      shifts: [],
    });
    renderBuilder();
    await waitFor(() => expect(screen.getByText("Dana")).toBeInTheDocument());
    // Published weeks stay editable — the add affordance never disappears.
    expect(screen.getAllByLabelText(/dashboardSchedule.addShiftFor/).length).toBeGreaterThan(0);
    // A permanently disabled Publish button is dead weight once published.
    expect(
      screen.queryByRole("button", { name: "dashboardSchedule.publish" }),
    ).not.toBeInTheDocument();
  });

  it("shows the live-change note in the editor when the week is published", async () => {
    mockedSchedule.get.mockResolvedValue({
      schedule: { id: 5, business_id: 42, week_start: "2026-06-29", status: "published", published_at: "2026-06-28T00:00:00Z", published_by_staff_id: 1, notes: "", created_at: "", updated_at: "" },
      shifts: [],
    });
    renderBuilder();
    await waitFor(() => expect(screen.getByText("Dana")).toBeInTheDocument());
    // Published weeks also show the live-edit honesty note above the grid (#213).
    expect(
      screen.getByText("dashboardSchedule.publishedEditNote"),
    ).toBeInTheDocument();
    await userEvent.click(screen.getAllByLabelText(/dashboardSchedule.addShiftFor/)[0]);
    expect(screen.getByRole("dialog")).toBeInTheDocument();
    expect(
      screen.getAllByText("dashboardSchedule.publishedEditNote").length,
    ).toBeGreaterThanOrEqual(2);
  });

  it("opens the approval inbox on the tab that actually has items", async () => {
    mockTimesheetCount = 2;
    renderBuilder();
    await waitFor(() => expect(screen.getByText("Dana")).toBeInTheDocument());
    // Requests is empty, Timesheets carries the work — the inbox should not
    // greet the operator with an empty Requests panel.
    await waitFor(() =>
      expect(
        screen.getByRole("tab", {
          name: /dashboardSchedule\.inbox\.timesheets/,
        }),
      ).toHaveAttribute("aria-selected", "true"),
    );
  });

  it("publishes the draft via scheduleApi.publish and toasts success", async () => {
    mockedSchedule.publish.mockResolvedValue({
      schedule: { id: 5, status: "published" },
      shifts: [],
    });
    renderBuilder();
    await waitFor(() => expect(screen.getByText("Dana")).toBeInTheDocument());
    // Publish now opens a confirmation modal summarizing the week (M16); confirm it.
    await userEvent.click(screen.getByRole("button", { name: "dashboardSchedule.publish" }));
    await userEvent.click(
      await screen.findByRole("button", {
        name: "dashboardSchedule.publishConfirm.confirm",
      }),
    );
    await waitFor(() => expect(mockedSchedule.publish).toHaveBeenCalledWith("42", 5));
    await waitFor(() => expect(mockShowSuccess).toHaveBeenCalled());
  });

  // L5-32: Sunday-start (week_start_day: 0) must drive scheduleApi.get's week
  // key AND the first grid column's day-key/label. Asserting only the API key
  // (or only that a "Sun" string appears somewhere) is pass-on-revert theater.
  it("aligns the week grid to Sunday when week_start_day is 0 (L5-32)", async () => {
    mockedSettings.get.mockResolvedValue({ week_start_day: 0 });
    renderBuilder();
    await waitFor(() => expect(mockedSchedule.get).toHaveBeenCalled());
    const weekKey = mockedSchedule.get.mock.calls[0][1] as string;
    const [y, m, d] = weekKey.split("-").map(Number);
    const start = new Date(y, m - 1, d);
    expect(start.getDay()).toBe(0); // Sunday
    // Pure helper must agree with the builder's settings-derived key.
    expect(weekKey).toBe(weekStartKey(new Date(), 0, "UTC"));
    // Must NOT match Monday default — that was the live failure mode.
    expect(weekKey).not.toBe(weekStartKey(new Date(), 1, "UTC"));
    await waitFor(() =>
      expect(screen.getByTestId("schedule-week-first-day")).toBeInTheDocument(),
    );
    const firstCol = screen.getByTestId("schedule-week-first-day");
    expect(firstCol.getAttribute("data-day-key")).toBe(weekKey);
    const sundayLabel = new Intl.DateTimeFormat("en", {
      weekday: "short",
    }).format(start);
    expect(firstCol).toHaveTextContent(sundayLabel);
    const weekStartLabel = screen.getByTestId("schedule-week-start-label");
    expect(weekStartLabel).toHaveAttribute("data-week-start-day", "0");
    expect(weekStartLabel).toHaveAttribute(
      "data-week-start-name",
      weekStartDayName(0, "en"),
    );
  });

  it("aligns the first column to Monday when week_start_day is 1 (L5-32 default)", async () => {
    mockedSettings.get.mockResolvedValue({ week_start_day: 1 });
    renderBuilder();
    await waitFor(() => expect(mockedSchedule.get).toHaveBeenCalled());
    const weekKey = mockedSchedule.get.mock.calls[0][1] as string;
    expect(weekKey).toBe(weekStartKey(new Date(), 1, "UTC"));
    await waitFor(() =>
      expect(screen.getByTestId("schedule-week-first-day")).toBeInTheDocument(),
    );
    const firstCol = screen.getByTestId("schedule-week-first-day");
    expect(firstCol.getAttribute("data-day-key")).toBe(weekKey);
    const [y, m, d] = weekKey.split("-").map(Number);
    const start = new Date(y, m - 1, d);
    expect(start.getDay()).toBe(1);
    const weekStartLabel = screen.getByTestId("schedule-week-start-label");
    expect(weekStartLabel).toHaveAttribute("data-week-start-day", "1");
    expect(weekStartLabel).toHaveAttribute(
      "data-week-start-name",
      weekStartDayName(1, "en"),
    );
    expect(firstCol).toHaveTextContent(
      new Intl.DateTimeFormat("en", { weekday: "short" }).format(start),
    );
  });

  it("aligns Sunday-start to the venue calendar, not UTC Monday (#664)", async () => {
    mockedSettings.get.mockResolvedValue({ week_start_day: 0 });
    renderBuilder("America/New_York");
    await waitFor(() => expect(mockedSchedule.get).toHaveBeenCalled());
    const weekKey = mockedSchedule.get.mock.calls[0][1] as string;
    expect(weekKey).toBe(weekStartKey(new Date(), 0, "America/New_York"));
    expect(weekKey).not.toBe(weekStartKey(new Date(), 1, "America/New_York"));
    await waitFor(() =>
      expect(screen.getByTestId("schedule-week-first-day")).toBeInTheDocument(),
    );
    expect(screen.getByTestId("schedule-week-first-day")).toHaveAttribute(
      "data-day-key",
      weekKey,
    );
    const [y, m, d] = weekKey.split("-").map(Number);
    expect(new Date(y, m - 1, d).getDay()).toBe(0);
  });

  // #247 — UTC wall seeds + device-local render collapsed every chip to
  // "4:00 AM–12:00 PM" in America/New_York, so dinner looked empty.
  it("renders venue-local dinner coverage instead of collapsing to 4:00 AM–12:00 PM (#247)", async () => {
    const venue = "America/New_York";
    const weekKey = weekStartKey(new Date(), 1, venue);
    const day = weekDayKeys(weekKey)[2]; // mid-week cell on the grid
    const lunchStart = wallTimeToInstant(`${day}T10:00`, venue).toISOString();
    const lunchEnd = wallTimeToInstant(`${day}T18:00`, venue).toISOString();
    const dinnerStart = wallTimeToInstant(`${day}T16:00`, venue).toISOString();
    const dinnerEnd = wallTimeToInstant(`${day}T23:00`, venue).toISOString();

    mockedSchedule.get.mockResolvedValue({
      schedule: {
        id: 5,
        business_id: 42,
        week_start: weekKey,
        status: "published",
        published_at: `${day}T12:00:00.000Z`,
        published_by_staff_id: 7,
        notes: "",
        created_at: "",
        updated_at: "",
      },
      shifts: [
        {
          id: 11,
          business_id: 42,
          schedule_id: 5,
          staff_id: 7,
          position_id: 3,
          starts_at: lunchStart,
          ends_at: lunchEnd,
          break_minutes: 30,
          status: "filled",
          published: true,
          notes: "Server lunch",
          created_by_staff_id: 7,
          created_at: "",
          updated_at: "",
        },
        {
          id: 12,
          business_id: 42,
          schedule_id: 5,
          staff_id: 7,
          position_id: 3,
          starts_at: dinnerStart,
          ends_at: dinnerEnd,
          break_minutes: 30,
          status: "filled",
          published: true,
          notes: "Server dinner",
          created_by_staff_id: 7,
          created_at: "",
          updated_at: "",
        },
      ],
    });

    renderBuilder(venue);
    await waitFor(() => expect(screen.getByText("Dana")).toBeInTheDocument());

    const body = document.body.textContent || "";
    // Dinner must be visible as an evening chip in the venue zone.
    expect(body).toMatch(/4:00\s*PM/);
    expect(body).toMatch(/10:00\s*AM/);
    // The #247 collapse signature must not be the rendered roster.
    expect(body).not.toMatch(/4:00\s*AM/);
    const collapsedChips = body.match(/4:00\s*AM\s*[–-]\s*12:00\s*PM/g) || [];
    expect(collapsedChips).toHaveLength(0);
  });

  // #211 — at laptop width the 7-col min-width used to expand flex ancestors
  // and overflow-hidden clipped Sat/Sun with no working scroller.
  it("renders all 7 days inside a min-width-safe horizontal scroller (#211)", async () => {
    renderBuilder();
    await waitFor(() =>
      expect(screen.getByTestId("schedule-week-scroll")).toBeInTheDocument(),
    );
    const scroller = screen.getByTestId("schedule-week-scroll");
    expect(scroller.className).toMatch(/overflow-x-auto/);
    expect(scroller.className).toMatch(/min-w-0/);
    expect(scroller.className).toMatch(/\bw-full\b/);

    const weekKey = mockedSchedule.get.mock.calls[0][1] as string;
    const days = weekDayKeys(weekKey);
    expect(days).toHaveLength(7);
    expect(document.querySelectorAll("[data-schedule-col='day']")).toHaveLength(
      7,
    );
    expect(screen.getByTestId("schedule-week-first-day")).toHaveAttribute(
      "data-day-key",
      days[0],
    );
    const lastCol = screen.getByTestId("schedule-week-last-day");
    expect(lastCol).toHaveAttribute("data-day-key", days[6]);
    const [y, m, d] = days[6].split("-").map(Number);
    expect(new Date(y, m - 1, d).getDay()).toBe(0);
    const sundayLabel = new Intl.DateTimeFormat("en", {
      weekday: "short",
    }).format(new Date(y, m - 1, d));
    expect(lastCol).toHaveTextContent(sundayLabel);
  });
});
