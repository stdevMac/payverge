// Pure logic for the staff "Today home" (Phase 2). Kept framework-free and
// side-effect-free so the shift/coverage/checklist/announcement selection rules
// are unit-tested without React or the network. The container
// (StaffTodayHome.tsx) fetches the data and formats with Intl + i18n; this
// module only decides WHAT to show. NO money crosses any of these shapes.

import type { Shift } from "@/api/schedule";
import type { MyCoverage } from "@/api/coverage";
import type { ChecklistRun } from "@/api/engagement";
import type { AnnouncementFeed, Announcement } from "@/api/chat";
import type { TimeOffRequest } from "@/api/availability";

function ms(iso: string): number {
  return new Date(iso).getTime();
}

/** Local (business-device) midnight for a timestamp, as epoch ms. */
function localMidnight(atMs: number): number {
  const d = new Date(atMs);
  return new Date(d.getFullYear(), d.getMonth(), d.getDate()).getTime();
}

/**
 * Whole-calendar-day delta between two instants in local time: 0 = same day,
 * 1 = tomorrow, -1 = yesterday. Rounds so a DST hour shift never bleeds into an
 * off-by-one. Drives the "Today / Tomorrow / <weekday>" bucketing.
 */
export function calendarDayDelta(atMs: number, nowMs: number): number {
  return Math.round((localMidnight(atMs) - localMidnight(nowMs)) / 86_400_000);
}

/** Minutes until a shift starts; negative once it has begun. */
export function minutesUntilStart(startMs: number, nowMs: number): number {
  return Math.round((startMs - nowMs) / 60_000);
}

/** A shift is "in progress" when it has started but not yet ended. */
export function isInProgress(shift: Shift, nowMs: number): boolean {
  return ms(shift.starts_at) <= nowMs && ms(shift.ends_at) > nowMs;
}

/**
 * The caller's own shifts that haven't ended yet (in-progress or upcoming),
 * earliest first, de-duplicated by id. Open (unassigned) shifts are coverage
 * opportunities, not "your" shifts, so they're excluded here.
 */
export function upcomingOwnShifts(
  shifts: Shift[],
  staffId: number,
  nowMs: number,
): Shift[] {
  const seen = new Set<number>();
  const own: Shift[] = [];
  for (const s of shifts) {
    if (s.staff_id !== staffId) continue;
    if (ms(s.ends_at) <= nowMs) continue;
    if (seen.has(s.id)) continue;
    seen.add(s.id);
    own.push(s);
  }
  own.sort((a, b) => ms(a.starts_at) - ms(b.starts_at));
  return own;
}

/** The single most-relevant shift: in-progress if any, else the soonest. */
export function pickNextShift(
  shifts: Shift[],
  staffId: number,
  nowMs: number,
): Shift | null {
  return upcomingOwnShifts(shifts, staffId, nowMs)[0] ?? null;
}

/** Today's checklist runs, matched on the run's date (date-only compare). */
export function todayChecklistRuns(
  runs: ChecklistRun[],
  todayKey: string,
): ChecklistRun[] {
  return runs.filter((r) => (r.for_date ?? "").slice(0, 10) === todayKey);
}

/** Count of runs not yet complete (what still needs the staff member's action). */
export function incompleteRunCount(runs: ChecklistRun[]): number {
  return runs.filter((r) => r.status !== "complete").length;
}

// A swap is still "live" (awaiting a taker or a manager) in these states; once
// approved/denied/cancelled it's resolved and no longer a pending request.
const LIVE_SWAP_STATUSES: ReadonlySet<string> = new Set([
  "open",
  "accepted",
  "pending_approval",
]);

export interface PendingRequestSummary {
  total: number;
  /** Open-shift claims + swaps still awaiting resolution. */
  coverage: number;
  /** Time-off requests still pending a decision. */
  timeOff: number;
}

/** How many of the caller's own requests are still awaiting a decision. */
export function pendingRequestSummary(
  mine: MyCoverage | null,
  timeOff: TimeOffRequest[],
): PendingRequestSummary {
  const claims = (mine?.claims ?? []).filter((c) => c.status === "pending").length;
  const swaps = (mine?.swaps ?? []).filter((s) =>
    LIVE_SWAP_STATUSES.has(s.status),
  ).length;
  const off = timeOff.filter((t) => t.status === "pending").length;
  return { total: claims + swaps + off, coverage: claims + swaps, timeOff: off };
}

/** The newest announcement for the caller plus whether they've acked it. */
export function latestAnnouncement(
  feed: AnnouncementFeed | null,
): { announcement: Announcement; acked: boolean } | null {
  const a = feed?.announcements?.[0];
  if (!a) return null;
  return { announcement: a, acked: Boolean(feed?.acked?.[String(a.id)]) };
}
