import { axiosInstance } from "@/api/tools/instance";

// Time clock (Slice 4). NO money anywhere on these types — staff punch in/out and
// see worked minutes/hours only; pay never crosses this wire. Mirrors the backend
// TimeEntry model in backend/internal/database/timeentry_service.go.

type TimeEntrySource = "staff_punch" | "manager_manual";
export type TimeEntryStatus = "open" | "pending_review" | "approved" | "rejected";

/** Manager correction body: adjust an entry's clock-in/out/break/note. */
export interface EditTimeEntryInput {
  clock_in_at: string; // RFC3339
  clock_out_at: string; // RFC3339, strictly after clock_in_at
  break_minutes?: number;
  note?: string;
}

export interface TimeEntry {
  id: number;
  business_id: number;
  staff_id: number;
  shift_id: number | null;
  clock_in_at: string; // RFC3339
  clock_out_at: string | null; // RFC3339, null while open
  break_minutes: number;
  source: TimeEntrySource;
  status: TimeEntryStatus;
  approved_by_staff_id: number | null;
  note: string;
  created_at: string;
  updated_at: string;
  worked_minutes: number; // computed server-side; 0 while still open
  worked_hours: number; // computed server-side
}

// Manager live-floor board (Phase 3). Money-free: hours-context only, never a
// pay figure. Mirrors backend/internal/handlers/livefloor.go.
export type LiveFloorStatus =
  | "on_clock"
  | "done"
  | "scheduled"
  | "late"
  | "no_show";

export interface LiveFloorRow {
  staff_id: number;
  staff_name: string;
  status: LiveFloorStatus;
  shift_id?: number;
  shift_start?: string; // RFC3339
  shift_end?: string; // RFC3339
  clock_in_at?: string; // RFC3339
  late_minutes?: number;
}

interface LiveFloorSummary {
  on_clock: number;
  scheduled: number;
  late: number;
  no_show: number;
  done: number;
}

export interface LiveFloor {
  date: string; // YYYY-MM-DD (business-local day)
  rows: LiveFloorRow[];
  summary: LiveFloorSummary;
}

// Kiosk clock-in (Phase 4b). A shared terminal runs under a manager session
// (timeclock:manage); staff enter a PIN to toggle their OWN punch. Money-free —
// no pay figure ever crosses this wire. Mirrors backend/internal/database/
// kiosk_service.go + handlers/kiosk.go.
export interface KioskStaffMember {
  staff_id: number;
  name: string;
  role: string;
  has_pin: boolean;
  on_clock: boolean;
  clock_in_at?: string; // RFC3339, present when on_clock
}

type KioskPunchAction = "clocked_in" | "clocked_out";

export interface KioskPunchResult {
  action: KioskPunchAction;
  staff_id: number;
  name: string;
  entry: TimeEntry;
}

/** Manager-only manual entry body (status lands straight in pending_review). */
export interface CreateManualEntryInput {
  staff_id: number;
  shift_id?: number | null;
  clock_in_at: string; // RFC3339
  clock_out_at: string; // RFC3339, strictly after clock_in_at
  break_minutes?: number;
  note?: string;
}

const base = (businessId: string) => "/inside/businesses/" + businessId;

export const timeclockApi = {
  // Self-punch in (timeclock:punch). Optionally tied to a shift; the server also
  // matches a current shift if one is in window. 409 if already clocked in.
  clockIn: async (businessId: string, shiftId?: number): Promise<TimeEntry> => {
    const res = await axiosInstance.post(
      base(businessId) + "/me/clock-in",
      shiftId != null ? { shift_id: shiftId } : {},
    );
    return res.data.data;
  },

  // Closes the open entry and moves it to pending_review. 409 if not clocked in.
  clockOut: async (businessId: string): Promise<TimeEntry> => {
    const res = await axiosInstance.post(base(businessId) + "/me/clock-out");
    return res.data.data;
  },

  // Adds unpaid break minutes to the open entry (bounded server-side). 400 if
  // invalid/excess, 409 if not clocked in.
  addBreak: async (businessId: string, minutes: number): Promise<TimeEntry> => {
    const res = await axiosInstance.post(base(businessId) + "/me/break", { minutes });
    return res.data.data;
  },

  // The caller's OWN entries, newest first. The one with status "open" (if any)
  // is the currently-clocked-in entry.
  myTimesheet: async (businessId: string): Promise<TimeEntry[]> => {
    const res = await axiosInstance.get(base(businessId) + "/me/timesheet");
    return res.data.data;
  },

  // The caller's own entries in [from, to] (inclusive day), optional status.
  // Money-free — entries carry worked minutes/hours only.
  listMineRange: async (
    businessId: string,
    from: string, // YYYY-MM-DD
    to: string, // YYYY-MM-DD
    status?: string,
  ): Promise<TimeEntry[]> => {
    const res = await axiosInstance.get(base(businessId) + "/me/timesheet", {
      params: { from, to, status: status || undefined },
    });
    return res.data.data;
  },

  // Manager review queue (timeclock:manage). Defaults to pending_review server-side.
  // Legacy: no pagination params → bare array (back-compat).
  listForReview: async (
    businessId: string,
    status?: TimeEntryStatus,
  ): Promise<TimeEntry[]> => {
    const res = await axiosInstance.get(base(businessId) + "/timesheets", {
      params: status ? { status } : undefined,
    });
    return res.data.data;
  },

  // Paginated manager review queue (timeclock:manage). Passes offset/limit (and an
  // optional from/to YYYY-MM-DD window) so the UI can page with an honest total.
  // BE-first: the server returns {data,total,offset,limit} when paginated.
  listForReviewPaged: async (
    businessId: string,
    params: {
      status?: TimeEntryStatus;
      offset?: number;
      limit?: number;
      from?: string; // YYYY-MM-DD
      to?: string; // YYYY-MM-DD
    },
  ): Promise<{ data: TimeEntry[]; total: number; offset: number; limit: number }> => {
    const res = await axiosInstance.get(base(businessId) + "/timesheets", {
      params: {
        status: params.status || undefined,
        offset: params.offset ?? 0,
        limit: params.limit ?? 20,
        from: params.from || undefined,
        to: params.to || undefined,
      },
    });
    return {
      data: res.data.data ?? [],
      total: res.data.total ?? (res.data.data?.length ?? 0),
      offset: res.data.offset ?? params.offset ?? 0,
      limit: res.data.limit ?? params.limit ?? 20,
    };
  },

  // Approve a pending entry (timeclock:manage). 409 if no longer pending.
  approve: async (businessId: string, entryId: number): Promise<TimeEntry> => {
    const res = await axiosInstance.post(
      base(businessId) + "/timesheets/" + entryId + "/approve",
    );
    return res.data.data;
  },

  // Reject a pending entry (timeclock:manage) with an optional reason — returns
  // it to the staffer's attention (status → rejected). 409 if no longer pending.
  reject: async (
    businessId: string,
    entryId: number,
    reason?: string,
  ): Promise<TimeEntry> => {
    const res = await axiosInstance.post(
      base(businessId) + "/timesheets/" + entryId + "/reject",
      reason ? { reason } : {},
    );
    return res.data.data;
  },

  // Manager correction of a NON-approved entry (timeclock:manage): adjust
  // clock-in/out/break/note. The entry stays reviewable; original values are
  // kept in the server audit trail. 409 if already approved, 400 bad range.
  edit: async (
    businessId: string,
    entryId: number,
    input: EditTimeEntryInput,
  ): Promise<TimeEntry> => {
    const res = await axiosInstance.patch(
      base(businessId) + "/timesheets/" + entryId,
      input,
    );
    return res.data.data;
  },

  // Manager live-floor board for the business day (timeclock:manage): who's on
  // the clock / scheduled / late / no-show. Money-free. Optional ?date override
  // (YYYY-MM-DD, business-local); default is today.
  liveFloor: async (businessId: string, date?: string): Promise<LiveFloor> => {
    const res = await axiosInstance.get(base(businessId) + "/live-floor", {
      params: date ? { date } : undefined,
    });
    return res.data.data;
  },

  // Kiosk roster (timeclock:manage): active staff for the shared terminal, each
  // with has_pin + current on_clock state. Money-free.
  kioskRoster: async (businessId: string): Promise<KioskStaffMember[]> => {
    const res = await axiosInstance.get(base(businessId) + "/kiosk/roster");
    return res.data.data.roster;
  },

  // Kiosk punch (timeclock:manage): verify the staffer's PIN and TOGGLE their
  // clock state (backend decides in vs out). 404 unknown/foreign staff, 409 no
  // PIN set, 403 wrong PIN, 429 too many attempts.
  kioskPunch: async (
    businessId: string,
    staffId: number,
    pin: string,
  ): Promise<KioskPunchResult> => {
    const res = await axiosInstance.post(base(businessId) + "/kiosk/punch", {
      staff_id: staffId,
      pin,
    });
    return res.data.data;
  },

  // Manager manual entry (timeclock:manage). 400 bad range/break, 404 unknown staff.
  createManual: async (
    businessId: string,
    input: CreateManualEntryInput,
  ): Promise<TimeEntry> => {
    const res = await axiosInstance.post(base(businessId) + "/time-entries", input);
    return res.data.data;
  },
};
