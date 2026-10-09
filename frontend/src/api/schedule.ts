import { axiosInstance } from "@/api/tools/instance";

// Scheduling core (Slice 2). NO money anywhere on these types — staff see
// hours only; pay never crosses this wire. Mirrors the backend models in
// backend/internal/database/schedule_service.go.

type ScheduleStatus = "draft" | "published";
type ShiftStatus = "private_draft" | "open" | "filled" | "locked";

export interface Schedule {
  id: number;
  business_id: number;
  week_start: string; // RFC3339, business-TZ week start
  status: ScheduleStatus;
  published_at: string | null;
  published_by_staff_id: number | null;
  notes: string;
  created_at: string;
  updated_at: string;
}

export interface Shift {
  id: number;
  business_id: number;
  schedule_id: number;
  staff_id: number | null; // null = open shift
  position_id: number;
  starts_at: string; // RFC3339
  ends_at: string; // RFC3339
  break_minutes: number;
  status: ShiftStatus;
  published: boolean;
  notes: string;
  created_by_staff_id: number;
  reminded_at?: string | null;
  created_at: string;
  updated_at: string;
}

/** Envelope returned by GET /schedule and POST /schedule/:id/publish. */
export interface ScheduleWeek {
  schedule: Schedule | null;
  shifts: Shift[];
}

/** Result of a transactional copy-week: created + skipped (only-empty-days) counts. */
export interface CopyWeekResult {
  created: number;
  skipped: number;
}

export interface CreateShiftInput {
  schedule_id: number;
  position_id: number;
  /** Omit/null for an open (unassigned) shift. */
  staff_id?: number | null;
  starts_at: string;
  ends_at: string;
  break_minutes?: number;
  notes?: string;
}

/** Partial PATCH body. `staff_id: 0` unassigns (server contract). */
export type UpdateShiftInput = Partial<{
  position_id: number;
  staff_id: number;
  starts_at: string;
  ends_at: string;
  break_minutes: number;
  notes: string;
}>;

// ---- Labor preview (Slice 6) ----
// Display-only, build-time scheduled-labor lens. The ONLY money in the schedule
// surfaces is `labor_cost`/`sales_target`/per-position `labor_cost`, and they are
// owner / financial:read-gated on the server: a non-financial caller receives the
// hours-only shape (those dollar fields omitted, `can_see_dollars: false`).

/** Warning `code` values mirror the Go consts in labor/calculator.go. */
type LaborWarningCode = "overtime" | "posted_late" | "minor_late";

export interface LaborWarning {
  code: LaborWarningCode;
  /** Present for per-staff warnings (overtime, minor_late). */
  staff_id?: number;
  /** Over-threshold weekly minutes (overtime), lead days (posted_late), or
   *  cutoff minute-of-day (minor_late) — interpreted per code. */
  detail: number;
}

interface LaborPreviewPosition {
  position_id: number;
  shift_count: number;
  hours: number;
  /** Dollars; present only when `can_see_dollars`. */
  labor_cost?: number;
}

export interface LaborPreview {
  schedule_id: number;
  can_see_dollars: boolean;
  total_hours: number;
  /** Fraction; present only when `can_see_dollars` (it is a pay/sales ratio). */
  labor_cost_pct?: number;
  /** Dollars; present only when `can_see_dollars`. */
  labor_cost?: number;
  /** Dollars; the caller's sales-target echo, only when `can_see_dollars`. */
  sales_target?: number;
  positions: LaborPreviewPosition[];
  warnings: LaborWarning[];
}

const base = (businessId: string) => "/inside/businesses/" + businessId;

export const scheduleApi = {
  // Row-scoped on the server: managers get draft + all shifts; plain staff get
  // the week only if published, with only their own + open shifts.
  get: async (businessId: string, week: string): Promise<ScheduleWeek> => {
    const res = await axiosInstance.get(base(businessId) + "/schedule", {
      params: { week },
    });
    return res.data.data;
  },

  // Idempotent get-or-create of the draft for a week.
  createDraft: async (businessId: string, week: string): Promise<Schedule> => {
    const res = await axiosInstance.post(base(businessId) + "/schedule", { week });
    return res.data.data;
  },

  // Optional Idempotency-Key (L5-26): callers mint one attempt-scoped key per
  // day so a lost-response retry replays the cached 201 instead of duplicating.
  createShift: async (
    businessId: string,
    input: CreateShiftInput,
    options?: { idempotencyKey?: string },
  ): Promise<Shift> => {
    const url = base(businessId) + "/shifts";
    const res = options?.idempotencyKey
      ? await axiosInstance.post(url, input, {
          headers: { "Idempotency-Key": options.idempotencyKey },
        })
      : await axiosInstance.post(url, input);
    return res.data.data;
  },

  updateShift: async (
    businessId: string,
    shiftId: number,
    input: UpdateShiftInput,
  ): Promise<Shift> => {
    const res = await axiosInstance.patch(base(businessId) + "/shifts/" + shiftId, input);
    return res.data.data;
  },

  deleteShift: async (businessId: string, shiftId: number): Promise<void> => {
    await axiosInstance.delete(base(businessId) + "/shifts/" + shiftId);
  },

  publish: async (businessId: string, scheduleId: number): Promise<ScheduleWeek> => {
    const res = await axiosInstance.post(
      base(businessId) + "/schedule/" + scheduleId + "/publish",
    );
    return res.data.data;
  },

  // Transactional copy of all shifts from one week into another in ONE call
  // (replaces the client's N serial per-shift POSTs). Copied shifts land in the
  // destination DRAFT. `onlyEmptyDays` fills only weekdays with no shift yet
  // (idempotent). from/to are YYYY-MM-DD.
  copyWeek: async (
    businessId: string,
    fromWeek: string,
    toWeek: string,
    onlyEmptyDays = false,
  ): Promise<CopyWeekResult> => {
    const res = await axiosInstance.post(base(businessId) + "/schedule/copy-week", {
      from_week: fromWeek,
      to_week: toWeek,
      only_empty_days: onlyEmptyDays,
    });
    return res.data.data;
  },

  // Display-only labor preview. `salesTargetDollars` is the figure the operator
  // types in the footer (dollars). The server gates every $ field on
  // owner/financial:read, so a non-financial caller gets hours + % only.
  laborPreview: async (
    businessId: string,
    scheduleId: number,
    salesTargetDollars?: number,
  ): Promise<LaborPreview> => {
    const qs =
      salesTargetDollars != null
        ? "?salesTarget=" + encodeURIComponent(salesTargetDollars)
        : "";
    const res = await axiosInstance.get(
      base(businessId) + "/schedule/" + scheduleId + "/labor-preview" + qs,
    );
    return res.data.data;
  },
};
