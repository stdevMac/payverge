import {
  calendarDayDelta,
  minutesUntilStart,
  isInProgress,
  upcomingOwnShifts,
  pickNextShift,
  todayChecklistRuns,
  incompleteRunCount,
  pendingRequestSummary,
  latestAnnouncement,
} from "./todayHome";
import type { Shift } from "@/api/schedule";
import type { MyCoverage } from "@/api/coverage";
import type { ChecklistRun } from "@/api/engagement";
import type { AnnouncementFeed } from "@/api/chat";
import type { TimeOffRequest } from "@/api/availability";

function shift(partial: Partial<Shift> & Pick<Shift, "id" | "starts_at" | "ends_at">): Shift {
  return {
    business_id: 1,
    schedule_id: 1,
    staff_id: 7,
    position_id: 2,
    break_minutes: 0,
    status: "filled",
    published: true,
    notes: "",
    created_by_staff_id: 1,
    created_at: "2026-06-01T00:00:00Z",
    updated_at: "2026-06-01T00:00:00Z",
    ...partial,
  };
}

// A stable "now": Mon 2026-06-29 10:00 local time.
const NOW = new Date(2026, 5, 29, 10, 0, 0).getTime();
const atLocal = (y: number, mo: number, d: number, h: number, mi = 0) =>
  new Date(y, mo, d, h, mi, 0).getTime();
const iso = (ms: number) => new Date(ms).toISOString();

describe("calendarDayDelta", () => {
  it("is 0 for the same local day regardless of clock time", () => {
    expect(calendarDayDelta(atLocal(2026, 5, 29, 23), NOW)).toBe(0);
    expect(calendarDayDelta(atLocal(2026, 5, 29, 0, 5), NOW)).toBe(0);
  });
  it("is 1 for tomorrow and 2 for the day after", () => {
    expect(calendarDayDelta(atLocal(2026, 5, 30, 6), NOW)).toBe(1);
    expect(calendarDayDelta(atLocal(2026, 6, 1, 6), NOW)).toBe(2);
  });
  it("is negative for past days", () => {
    expect(calendarDayDelta(atLocal(2026, 5, 28, 23), NOW)).toBe(-1);
  });
});

describe("minutesUntilStart", () => {
  it("counts forward and goes negative once started", () => {
    expect(minutesUntilStart(NOW + 90 * 60_000, NOW)).toBe(90);
    expect(minutesUntilStart(NOW - 30 * 60_000, NOW)).toBe(-30);
  });
});

describe("isInProgress", () => {
  it("is true only between start and end", () => {
    const live = shift({ id: 1, starts_at: iso(NOW - 3_600_000), ends_at: iso(NOW + 3_600_000) });
    const future = shift({ id: 2, starts_at: iso(NOW + 3_600_000), ends_at: iso(NOW + 7_200_000) });
    const past = shift({ id: 3, starts_at: iso(NOW - 7_200_000), ends_at: iso(NOW - 3_600_000) });
    expect(isInProgress(live, NOW)).toBe(true);
    expect(isInProgress(future, NOW)).toBe(false);
    expect(isInProgress(past, NOW)).toBe(false);
  });
});

describe("upcomingOwnShifts", () => {
  const mine1 = shift({ id: 1, staff_id: 7, starts_at: iso(NOW + 7_200_000), ends_at: iso(NOW + 10_800_000) });
  const mineLive = shift({ id: 2, staff_id: 7, starts_at: iso(NOW - 600_000), ends_at: iso(NOW + 3_600_000) });
  const mineLater = shift({ id: 3, staff_id: 7, starts_at: iso(NOW + 86_400_000), ends_at: iso(NOW + 90_000_000) });
  const other = shift({ id: 4, staff_id: 99, starts_at: iso(NOW + 3_600_000), ends_at: iso(NOW + 7_200_000) });
  const ended = shift({ id: 5, staff_id: 7, starts_at: iso(NOW - 7_200_000), ends_at: iso(NOW - 3_600_000) });
  const open = shift({ id: 6, staff_id: null, starts_at: iso(NOW + 3_600_000), ends_at: iso(NOW + 7_200_000) });

  it("keeps only own, not-yet-ended shifts, earliest first", () => {
    const result = upcomingOwnShifts(
      [mine1, mineLive, mineLater, other, ended, open],
      7,
      NOW,
    );
    expect(result.map((s) => s.id)).toEqual([2, 1, 3]);
  });

  it("de-duplicates by id across merged weeks", () => {
    const result = upcomingOwnShifts([mine1, { ...mine1 }], 7, NOW);
    expect(result).toHaveLength(1);
  });

  it("pickNextShift returns the in-progress shift, or null when none", () => {
    expect(pickNextShift([mine1, mineLive], 7, NOW)?.id).toBe(2);
    expect(pickNextShift([ended, other], 7, NOW)).toBeNull();
  });
});

describe("todayChecklistRuns + incompleteRunCount", () => {
  const runs: ChecklistRun[] = [
    { id: 1, business_id: 1, template_id: 1, assigned_staff_id: 7, shift_id: null, for_date: "2026-06-29", status: "pending", completed_at: null },
    { id: 2, business_id: 1, template_id: 1, assigned_staff_id: 7, shift_id: null, for_date: "2026-06-29T00:00:00Z", status: "complete", completed_at: "x" },
    { id: 3, business_id: 1, template_id: 1, assigned_staff_id: 7, shift_id: null, for_date: "2026-06-30", status: "pending", completed_at: null },
  ];
  it("matches today by date-only, tolerating a time suffix", () => {
    const today = todayChecklistRuns(runs, "2026-06-29");
    expect(today.map((r) => r.id)).toEqual([1, 2]);
  });
  it("counts only not-complete runs", () => {
    expect(incompleteRunCount(todayChecklistRuns(runs, "2026-06-29"))).toBe(1);
  });
});

describe("pendingRequestSummary", () => {
  const mine: MyCoverage = {
    claims: [
      { id: 1, shift_id: 1, status: "pending", created_at: "x" },
      { id: 2, shift_id: 2, status: "approved", created_at: "x" },
    ],
    swaps: [
      { id: 3, shift_id: 3, kind: "swap", status: "open", requesting_staff_id: 7, accepting_staff_id: null, created_at: "x" },
      { id: 4, shift_id: 4, kind: "giveup", status: "denied", requesting_staff_id: 7, accepting_staff_id: null, created_at: "x" },
    ],
  };
  const off = (id: number, status: TimeOffRequest["status"]): TimeOffRequest => ({
    id,
    business_id: 1,
    staff_id: 7,
    starts_at: "x",
    ends_at: "y",
    reason: "",
    status,
    decided_by_staff_id: null,
    decided_at: null,
    created_at: "x",
    updated_at: "x",
  });
  const timeOff: TimeOffRequest[] = [off(1, "pending"), off(2, "approved")];
  it("counts only unresolved coverage + time-off requests", () => {
    const s = pendingRequestSummary(mine, timeOff);
    expect(s).toEqual({ total: 3, coverage: 2, timeOff: 1 });
  });
  it("is all-zero for null/empty inputs", () => {
    expect(pendingRequestSummary(null, [])).toEqual({ total: 0, coverage: 0, timeOff: 0 });
  });
});

describe("latestAnnouncement", () => {
  const feed: AnnouncementFeed = {
    announcements: [
      { id: 10, business_id: 1, author_staff_id: 1, title: "Newest", content: "c", require_ack: true, audience_filter: "all", created_at: "2026-06-29T09:00:00Z", updated_at: "x" },
      { id: 9, business_id: 1, author_staff_id: 1, title: "Older", content: "c", require_ack: false, audience_filter: "all", created_at: "2026-06-28T09:00:00Z", updated_at: "x" },
    ],
    acked: { "9": true },
  };
  it("returns the first (newest) announcement with its ack state", () => {
    const r = latestAnnouncement(feed);
    expect(r?.announcement.id).toBe(10);
    expect(r?.acked).toBe(false);
  });
  it("returns null for an empty feed", () => {
    expect(latestAnnouncement({ announcements: [], acked: {} })).toBeNull();
    expect(latestAnnouncement(null)).toBeNull();
  });
});
