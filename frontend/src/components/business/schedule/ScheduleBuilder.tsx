"use client";

import React, {
  useCallback,
  useEffect,
  useMemo,
  useRef,
  useState,
} from "react";
import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query";
import { Button, Chip, useDisclosure } from "@nextui-org/react";
import {
  CalendarCheck2,
  CalendarOff,
  CalendarX2,
  ChevronLeft,
  ChevronRight,
  ClipboardCheck,
  Plus,
  Settings,
  Tablet,
} from "lucide-react";
import {
  scheduleApi,
  type Schedule,
  type Shift,
  type ScheduleWeek,
  type CopyWeekResult,
} from "@/api/schedule";
import { scheduleSettingsApi } from "@/api/scheduleSettings";
import { positionsApi, type Position } from "@/api/positions";
import { getBusinessStaff, type StaffMember } from "@/api/staff";
import { availabilityApi, type TimeOffRequest } from "@/api/availability";
import { laborCostApi } from "@/api/laborCost";
import { queryKeys } from "@/api/queryKeys";
import {
  getTranslation,
  useSimpleLocale,
} from "@/i18n/SimpleTranslationProvider";
import { useToast } from "@/contexts/ToastContext";
import { getApiErrorStatus } from "@/utils/apiError";
import { intlLocaleFor } from "@/utils/intlLocale";
import { addShiftAccessibleName } from "./addShiftLabel";
import {
  businessDateKey,
  formatBusinessTime,
  resolveBusinessTimeZone,
} from "@/utils/businessTime";
import { instantToWallTime, wallTimeToInstant } from "@/utils/zonedDateTime";
import {
  weekStartKey,
  shiftWeekKey,
  weekDayKeys,
  resolveWeekStartDay,
  weekStartDayName,
} from "@/utils/scheduleWeek";
import ShiftEditorModal, {
  type ShiftEditorLabels,
  type ShiftConflictLabels,
  type ShiftFormValues,
} from "./ShiftEditorModal";
import { cellPosture, type TeamAvailability } from "./scheduleAvailability";
import { translatePositionName } from "./positionLabel";
import ApprovalsPanel from "./ApprovalsPanel";
import TimesheetReview from "./TimesheetReview";
import LiveFloorBoard from "./LiveFloorBoard";
import OperatorLogbook from "./OperatorLogbook";
import ScheduleSettingsModal from "./ScheduleSettingsModal";
import ConfirmationModal from "../modals/ConfirmationModal";
import KioskClockIn from "./KioskClockIn";
import ScheduleWeekMobile from "./ScheduleWeekMobile";
import { useMediaQuery } from "@/hooks/useMediaQuery";
import {
  LaborPreviewFooter,
  type LaborPreviewLabels,
} from "./LaborPreviewFooter";
import {
  DINNER_WINDOW,
  LUNCH_WINDOW,
  daypartCoverageMinutes,
} from "./scheduleCoverage";
import SegmentedTabs from "../shared/SegmentedTabs";
import DashboardTabShell from "../shared/DashboardTabShell";
import DashboardTabLoadingSkeleton from "../shared/DashboardTabLoadingSkeleton";
import SetupChecklist from "../shared/SetupChecklist";
import { buildSetupSteps } from "../shared/setupSteps";
import { PremiumPanel } from "../premium";
import IconTile from "@/components/ui/IconTile";
import {
  btnPrimaryNextUI,
  btnSecondaryNextUI,
  btnGhostIcon,
} from "@/components/ui/buttonStyles";

interface ScheduleBuilderProps {
  businessId: string;
  /**
   * Whether the caller may see labor dollars (owner or manager with
   * financial:read). Forwarded to TimesheetReview; defaults to false so a
   * non-financial mount never reveals amounts. The server is the real gate.
   */
  canViewFinancials?: boolean;
  /** Business reporting currency (ISO code) for any labor-$ formatting. */
  currency?: string;
  /** Venue IANA timezone — all shift chips and editors render in this zone. */
  businessTimezone?: string | null;
  /**
   * Top-level dashboard tab navigator (the page's handleSetActiveTab). Used by
   * the cold-start setup checklist to jump to "Team → Positions" / "Team".
   * Accepts composite specs like "staff?sub=positions".
   */
  onNavigate?: (spec: string) => void;
}

type ViewMode = "employee" | "role";

type InboxView = "requests" | "timesheets";

type EditorState =
  | {
      kind: "create";
      dayKey: string;
      staffId: number | null;
      positionId: number | null;
    }
  | { kind: "edit"; shift: Shift }
  | null;

/** Stable fingerprint for one create-submit attempt (form fields + day set). */
function shiftCreateFingerprint(values: ShiftFormValues): string {
  const days = (values.days.length > 0 ? values.days : [values.dayKey])
    .slice()
    .sort();
  return [
    values.positionId,
    values.staffId ?? "open",
    values.startTime,
    values.endTime,
    values.breakMinutes,
    values.notes ?? "",
    days.join(","),
  ].join("|");
}

interface BuilderRow {
  key: string;
  label: string;
  staffId?: number | null;
  positionId?: number;
  colorHex?: string;
}

function ScheduleNoPositionsPanel({
  actionLabel,
  onAction,
  subtitle,
  title,
}: {
  actionLabel?: string;
  onAction?: () => void;
  subtitle: string;
  title: string;
}) {
  return (
    <div className="overflow-hidden rounded-2xl border border-brand/10 bg-brand/5 p-4 sm:p-5">
      <div>
        <div>
          <IconTile icon={CalendarX2} size="lg" className="mb-5" />
          <h3 className="font-title text-2xl text-ink-950">{title}</h3>
          <p className="mt-3 max-w-xl text-sm leading-6 text-ink-600">
            {subtitle}
          </p>
          {actionLabel && onAction ? (
            <Button
              radius="full"
              className={`mt-6 ${btnPrimaryNextUI}`}
              startContent={<Plus className="h-4 w-4" />}
              onPress={onAction}
            >
              {actionLabel}
            </Button>
          ) : null}
        </div>
      </div>
    </div>
  );
}

function toTimeInput(iso: string, timeZone: string | null): string {
  const wall = instantToWallTime(iso, resolveBusinessTimeZone(timeZone));
  return wall.slice(11, 16);
}

/** A shift's net minutes (span minus break), never negative. */
function shiftNetMinutes(sh: Shift): number {
  const span =
    (new Date(sh.ends_at).getTime() - new Date(sh.starts_at).getTime()) / 60000;
  const net = Math.round(span) - (sh.break_minutes || 0);
  return net > 0 ? net : 0;
}

/** Compact hours label for balance cues: "8h" / "7h 45m" / "45m". */
function fmtMinutes(total: number): string {
  const h = Math.floor(total / 60);
  const m = total % 60;
  if (h === 0) return `${m}m`;
  return m === 0 ? `${h}h` : `${h}h ${m}m`;
}

function DaypartCoverageChips({
  lunchMin,
  dinnerMin,
  dayKey,
  t,
}: {
  lunchMin: number;
  dinnerMin: number;
  dayKey: string;
  t: (key: string) => string;
}) {
  const lunchHours = lunchMin > 0 ? fmtMinutes(lunchMin) : t("coverage.none");
  const dinnerHours = dinnerMin > 0 ? fmtMinutes(dinnerMin) : t("coverage.none");
  return (
    <p
      className="mt-1 space-y-0.5 text-[10px] font-medium leading-tight text-ink-500"
      data-testid={`schedule-coverage-${dayKey}`}
    >
      <span className={`block ${lunchMin === 0 ? "text-ink-400" : ""}`}>
        {t("coverage.lunch").replace("{hours}", lunchHours)}
      </span>
      <span
        className={`block ${dinnerMin === 0 ? "text-rose-700" : "text-ink-600"}`}
      >
        {t("coverage.dinner").replace("{hours}", dinnerHours)}
      </span>
    </p>
  );
}

/** Interpret a day + HH:mm as a venue-local wall time → UTC instant. */
function businessDateTime(
  dayKey: string,
  time: string,
  timeZone: string | null,
): Date {
  return wallTimeToInstant(
    `${dayKey}T${time}`,
    resolveBusinessTimeZone(timeZone),
  );
}

function formatTimeRange(
  startIso: string,
  endIso: string,
  locale: string,
  timeZone: string | null,
): string {
  return `${formatBusinessTime(startIso, locale, timeZone)}–${formatBusinessTime(endIso, locale, timeZone)}`;
}

export default function ScheduleBuilder({
  businessId,
  canViewFinancials = false,
  currency = "USD",
  businessTimezone = null,
  onNavigate,
}: ScheduleBuilderProps) {
  const venueTimeZone = resolveBusinessTimeZone(businessTimezone);
  const { locale } = useSimpleLocale();
  const toast = useToast();
  const queryClient = useQueryClient();

  const t = useCallback(
    (key: string): string => {
      const v = getTranslation(`dashboardSchedule.${key}`, locale);
      return Array.isArray(v) ? v[0] || key : (v as string);
    },
    [locale],
  );

  const setupT = useCallback(
    (key: string): string => {
      const v = getTranslation(`dashboardSetup.${key}`, locale);
      return Array.isArray(v) ? v[0] || key : (v as string);
    },
    [locale],
  );

  // Seeded default position names (Manager/Server/...) get localized to match
  // the Equipo tab; operator-created names render as typed. See positionLabel.ts.
  const positionLabel = useCallback(
    (name: string): string => translatePositionName(name, t),
    [t],
  );

  const [mode, setMode] = useState<ViewMode>("employee");
  // Below md the 7-column grid is unusable; render the day-switcher list
  // instead. Gated on a real media query so only one view mounts (false under
  // SSR/jsdom, so the desktop grid tests are unaffected).
  const isPhone = useMediaQuery("(max-width: 767px)");
  const [settingsOpen, setSettingsOpen] = useState(false);
  const [kioskOpen, setKioskOpen] = useState(false);
  // Publish is one-way (no unpublish endpoint) and blasts a notification to the
  // whole team, so gate it behind a confirmation that summarizes the week first.
  const publishConfirm = useDisclosure();
  const [weekOffset, setWeekOffset] = useState(0);
  const [editor, setEditor] = useState<EditorState>(null);
  // Unified "Needs your approval" inbox: each embedded panel reports its own
  // pending count for the tab badge + the header total; both stay mounted (only
  // the inactive one is hidden) so counts stay live and queries don't refetch.
  const [inboxView, setInboxView] = useState<InboxView>("requests");
  const [reqCount, setReqCount] = useState(0);
  const [tsCount, setTsCount] = useState(0);
  // Until the operator picks a tab themselves, land on whichever tab actually
  // has items (requests wins ties) — a "1" badge on the header with an empty
  // default panel reads as a bug.
  const inboxTouched = useRef(false);
  // Last-week sales default applies once per mount when no stored target.
  const salesTargetDefaulted = useRef(false);
  useEffect(() => {
    if (inboxTouched.current) return;
    if (reqCount === 0 && tsCount > 0) setInboxView("timesheets");
    else if (reqCount > 0) setInboxView("requests");
  }, [reqCount, tsCount]);
  // Sales target (dollars) for the labor footer. The immediate value drives the
  // input; the debounced value drives the preview query so typing doesn't refetch
  // on every keystroke. Persisted per business — a weekly estimate rarely changes,
  // so re-typing it every visit was pure friction.
  const salesTargetStorageKey = `payverge.salesTarget.${businessId}`;
  const [salesTarget, setSalesTarget] = useState<number | undefined>(() => {
    if (typeof window === "undefined") return undefined;
    const stored = Number(window.localStorage.getItem(salesTargetStorageKey));
    return Number.isFinite(stored) && stored > 0 ? stored : undefined;
  });
  const [debouncedSalesTarget, setDebouncedSalesTarget] = useState<
    number | undefined
  >(salesTarget);

  // #830: bounded first paint. One quick retry rescues the cold-open network
  // blip (the 1/2 "Loading…" flake); anything worse surfaces the error panel
  // within ~2s instead of sitting in the default 3-retry exponential backoff
  // (or, for settings' old retry:false, dying on the first blip with no way
  // back but a remount).
  const boundedRetry = { retry: 1, retryDelay: 700 } as const;

  const settingsQuery = useQuery({
    queryKey: ["schedule", businessId, "settings"],
    queryFn: () => scheduleSettingsApi.get(businessId),
    staleTime: 5 * 60 * 1000,
    ...boundedRetry,
  });
  // L5-32: week grid must follow business schedule settings (0=Sun..6=Sat),
  // not a hard-coded Monday. settingsQuery is the same key the settings modal
  // writes via setQueryData so a saved week-start updates the grid immediately.
  const weekStartDay = resolveWeekStartDay(
    settingsQuery.data?.week_start_day,
  );
  // When the week-start day changes (settings save), drop any weekOffset so
  // the operator lands on "this week" under the new alignment instead of an
  // offset computed against the previous week-start key.
  useEffect(() => {
    setWeekOffset(0);
  }, [weekStartDay]);
  // Anchor the week to the same resolved venue zone that "today" and shift
  // bucketing use below. Passing the raw prop let a venue with no timezone
  // build the week from the device clock while "today" and shifts followed
  // UTC, so on a Sunday evening west of UTC the grid showed last week.
  const weekKey = useMemo(
    () =>
      shiftWeekKey(
        weekStartKey(new Date(), weekStartDay, venueTimeZone),
        weekOffset,
      ),
    [weekStartDay, weekOffset, venueTimeZone],
  );
  const weekQueryKey = queryKeys.schedule.week(businessId, weekKey);
  const days = useMemo(() => weekDayKeys(weekKey), [weekKey]);

  // Only read/build a week once the week-start setting has actually resolved.
  // A failed settings fetch silently falls back to Monday, which yields the
  // wrong week key and can create a duplicate draft under it — so hold the
  // schedule query until settings succeed and surface the error below.
  const settingsReady = settingsQuery.isSuccess;
  const scheduleQuery = useQuery({
    queryKey: weekQueryKey,
    queryFn: () => scheduleApi.get(businessId, weekKey),
    enabled: settingsReady,
    ...boundedRetry,
  });
  const positionsQuery = useQuery({
    queryKey: ["positions", businessId],
    queryFn: () => positionsApi.list(businessId),
    staleTime: 5 * 60 * 1000,
    ...boundedRetry,
  });
  const staffQuery = useQuery({
    queryKey: queryKeys.staff.list(businessId),
    queryFn: () => getBusinessStaff(businessId),
    staleTime: 5 * 60 * 1000,
    ...boundedRetry,
  });

  const schedule: Schedule | null = scheduleQuery.data?.schedule ?? null;
  const shifts = useMemo<Shift[]>(
    () => scheduleQuery.data?.shifts ?? [],
    [scheduleQuery.data],
  );
  const positions = useMemo<Position[]>(
    () => positionsQuery.data ?? [],
    [positionsQuery.data],
  );
  const staffList = useMemo<StaffMember[]>(
    () => staffQuery.data?.staff ?? [],
    [staffQuery.data],
  );
  const invitationCount = staffQuery.data?.pending_invitations?.length ?? 0;
  const positionsById = useMemo(() => {
    const map = new Map<number, Position>();
    positions.forEach((p) => map.set(p.id, p));
    return map;
  }, [positions]);
  const staffById = useMemo(() => {
    const map = new Map<number, StaffMember>();
    staffList.forEach((s) => map.set(s.id, s));
    return map;
  }, [staffList]);
  // Names-only view for surfaces that speak about people (labor warnings).
  const staffNamesById = useMemo(() => {
    const map = new Map<number, string>();
    staffList.forEach((s) => map.set(s.id, s.name));
    return map;
  }, [staffList]);

  const rows = useMemo<BuilderRow[]>(() => {
    if (mode === "role") {
      return positions.map((p) => ({
        key: `p${p.id}`,
        label: positionLabel(p.name),
        positionId: p.id,
        colorHex: p.color_hex,
      }));
    }
    return [
      ...staffList.map((s) => ({
        key: `s${s.id}`,
        label: s.name,
        staffId: s.id,
      })),
      { key: "open", label: t("openShifts"), staffId: null },
    ];
  }, [mode, positions, staffList, t, positionLabel]);

  // Balance cues: weekly minutes per row + scheduled minutes per day, so an
  // uneven week (Sam at 12h, Bea at 42h) is visible without opening anything.
  const rowShifts = useCallback(
    (row: BuilderRow): Shift[] =>
      shifts.filter((sh) =>
        mode === "role"
          ? sh.position_id === row.positionId
          : (sh.staff_id ?? null) === (row.staffId ?? null),
      ),
    [shifts, mode],
  );
  const rowWeeklyMinutes = useCallback(
    (row: BuilderRow): number =>
      rowShifts(row).reduce((acc, sh) => acc + shiftNetMinutes(sh), 0),
    [rowShifts],
  );
  const rowWeeklyBreakMinutes = useCallback(
    (row: BuilderRow): number =>
      rowShifts(row).reduce((acc, sh) => acc + (sh.break_minutes || 0), 0),
    [rowShifts],
  );
  const dayTotalMinutes = useMemo(() => {
    const map = new Map<string, number>();
    shifts.forEach((sh) => {
      const key = businessDateKey(sh.starts_at, venueTimeZone);
      map.set(key, (map.get(key) ?? 0) + shiftNetMinutes(sh));
    });
    return map;
  }, [shifts, venueTimeZone]);

  const dayCoverage = useMemo(() => {
    const map = new Map<string, { lunchMin: number; dinnerMin: number }>();
    for (const dayKey of days) {
      map.set(dayKey, {
        lunchMin: daypartCoverageMinutes(
          shifts,
          dayKey,
          venueTimeZone,
          LUNCH_WINDOW,
        ),
        dinnerMin: daypartCoverageMinutes(
          shifts,
          dayKey,
          venueTimeZone,
          DINNER_WINDOW,
        ),
      });
    }
    return map;
  }, [days, shifts, venueTimeZone]);

  const shiftsForCell = useCallback(
    (row: BuilderRow, dayKey: string): Shift[] => {
      return shifts
        .filter((sh) => businessDateKey(sh.starts_at, venueTimeZone) === dayKey)
        .filter((sh) =>
          mode === "role"
            ? sh.position_id === row.positionId
            : (sh.staff_id ?? null) === (row.staffId ?? null),
        )
        .sort(
          (a, b) =>
            new Date(a.starts_at).getTime() - new Date(b.starts_at).getTime(),
        );
    },
    [shifts, mode, venueTimeZone],
  );

  // Lazily get-or-create the draft so a shift always has a schedule_id. Never
  // create against an unresolved week key: if the settings fetch hasn't
  // succeeded, weekKey is the Monday fallback and a draft filed under it would
  // duplicate the real week's schedule row.
  const ensureScheduleId = useCallback(async (): Promise<number> => {
    if (schedule) return schedule.id;
    if (!settingsReady) throw new Error("schedule settings not loaded");
    const created = await scheduleApi.createDraft(businessId, weekKey);
    queryClient.setQueryData<ScheduleWeek>(weekQueryKey, (prev) => ({
      schedule: created,
      shifts: prev?.shifts ?? [],
    }));
    return created.id;
  }, [schedule, settingsReady, businessId, weekKey, queryClient, weekQueryKey]);

  const invalidateWeek = useCallback(
    () =>
      Promise.all([
        queryClient.invalidateQueries({ queryKey: weekQueryKey }),
        // The labor footer is a separate server-computed query — without this it
        // keeps showing pre-edit hours/cost until an unrelated refetch trigger.
        queryClient.invalidateQueries({
          queryKey: ["schedule", businessId, "laborPreview"],
        }),
      ]),
    [queryClient, weekQueryKey, businessId],
  );

  // L5-26 decision #10: one attempt id per user-initiated create submit. Kept
  // across partial-failure retries so days that already landed replay their
  // cached 201 via per-day Idempotency-Key (`${attemptId}:${dayKey}`). Cleared
  // on success or when the operator cancels the editor (a new open is a new attempt).
  const createAttemptRef = useRef<{
    fingerprint: string;
    attemptId: string;
  } | null>(null);

  const createShiftMutation = useMutation({
    mutationFn: async (values: ShiftFormValues) => {
      const scheduleId = await ensureScheduleId();
      // The editor's repeat-on row fans one form out across several days;
      // a plain create is just the single-day case of the same loop.
      const days = values.days.length > 0 ? values.days : [values.dayKey];
      const fingerprint = shiftCreateFingerprint(values);
      if (
        !createAttemptRef.current ||
        createAttemptRef.current.fingerprint !== fingerprint
      ) {
        createAttemptRef.current = {
          fingerprint,
          attemptId: crypto.randomUUID(),
        };
      }
      const attemptId = createAttemptRef.current.attemptId;
      for (const day of days) {
        const start = businessDateTime(day, values.startTime, venueTimeZone);
        let end = businessDateTime(day, values.endTime, venueTimeZone);
        // L5-28: equal times rejected in the editor; only true overnight wraps.
        if (end.getTime() === start.getTime()) {
          throw new Error("end_equals_start");
        }
        if (end.getTime() < start.getTime()) {
          end = new Date(end.getTime() + 24 * 60 * 60 * 1000);
        }
        // Per-day key stays under the middleware's 128-char limit
        // (UUID 36 + ":" + YYYY-MM-DD 10 = 47).
        await scheduleApi.createShift(
          businessId,
          {
            schedule_id: scheduleId,
            position_id: values.positionId,
            staff_id: values.staffId,
            starts_at: start.toISOString(),
            ends_at: end.toISOString(),
            break_minutes: values.breakMinutes,
            notes: values.notes,
          },
          { idempotencyKey: `${attemptId}:${day}` },
        );
      }
    },
    onSuccess: () => {
      createAttemptRef.current = null;
      void invalidateWeek();
      setEditor(null);
    },
    onError: (err) => {
      // Multi-day fan-out can fail partway. Refetch so the grid shows what
      // actually landed. Keep the editor open and retain createAttemptRef so a
      // resubmit reuses the same per-day keys (replay, not double-create).
      void invalidateWeek();
      const status = getApiErrorStatus(err);
      // 425 Too Early: original request still in flight — soft, not a hard fail.
      if (status === 425) {
        toast.showInfo(t("saveInFlightTitle"), t("saveInFlightBody"));
        return;
      }
      // 409: same key, different body (operator edited after a partial attempt).
      if (status === 409) {
        toast.showError(t("saveConflictTitle"), t("saveConflictBody"));
        return;
      }
      toast.showError(t("saveErrorTitle"));
    },
  });

  const updateShiftMutation = useMutation({
    mutationFn: async ({
      shiftId,
      values,
    }: {
      shiftId: number;
      values: ShiftFormValues;
    }) => {
      const start = businessDateTime(
        values.dayKey,
        values.startTime,
        venueTimeZone,
      );
      let end = businessDateTime(values.dayKey, values.endTime, venueTimeZone);
      if (end.getTime() === start.getTime()) {
        throw new Error("end_equals_start");
      }
      if (end.getTime() < start.getTime()) {
        end = new Date(end.getTime() + 24 * 60 * 60 * 1000);
      }
      return scheduleApi.updateShift(businessId, shiftId, {
        position_id: values.positionId,
        // staff_id:0 unassigns (server contract); a real id assigns/reassigns.
        staff_id: values.staffId ?? 0,
        starts_at: start.toISOString(),
        ends_at: end.toISOString(),
        break_minutes: values.breakMinutes,
        notes: values.notes,
      });
    },
    onSuccess: () => {
      void invalidateWeek();
      setEditor(null);
    },
    onError: () => toast.showError(t("saveErrorTitle")),
  });

  const deleteShiftMutation = useMutation({
    mutationFn: (shiftId: number) =>
      scheduleApi.deleteShift(businessId, shiftId),
    onSuccess: () => {
      void invalidateWeek();
      setEditor(null);
    },
    onError: () => toast.showError(t("deleteErrorTitle")),
  });

  // Copy-last-week: composes existing endpoints (get previous week, then create
  // each shift +7 days into this week's draft). Offered only on an empty week so
  // it can never double-book a week that's already in progress.
  // Copy last week into this one via the TRANSACTIONAL server endpoint — one call,
  // all-or-nothing, no more N serial per-shift POSTs that lost the button on a
  // partial failure. `onlyEmptyDays` fills only still-empty weekdays (idempotent)
  // so the button stays useful once the week is partially built.
  const copyLastWeekMutation = useMutation({
    mutationFn: (onlyEmptyDays: boolean): Promise<CopyWeekResult> =>
      scheduleApi.copyWeek(
        businessId,
        shiftWeekKey(weekKey, -1),
        weekKey,
        onlyEmptyDays,
      ),
    onSuccess: (res) => {
      void invalidateWeek();
      if (res.created === 0) {
        toast.showInfo(t("copyWeek.emptyTitle"), t("copyWeek.emptyBody"));
      } else {
        toast.showSuccess(
          t("copyWeek.successTitle"),
          t("copyWeek.successBody").replace("{count}", String(res.created)),
        );
      }
    },
    onError: () => {
      // Transactional: nothing partial landed, but refetch anyway for freshness.
      void invalidateWeek();
      toast.showError(t("copyWeek.errorTitle"));
    },
  });

  const publishMutation = useMutation({
    mutationFn: () => {
      if (!schedule) throw new Error("no draft");
      return scheduleApi.publish(businessId, schedule.id);
    },
    onSuccess: (data) => {
      queryClient.setQueryData<ScheduleWeek>(weekQueryKey, data);
      toast.showSuccess(t("publishSuccessTitle"), t("publishSuccessBody"));
    },
    onError: () => toast.showError(t("publishErrorTitle")),
  });

  const handleSubmit = useCallback(
    (values: ShiftFormValues) => {
      // L5-26: double-submit guard while either mutation is in flight.
      if (createShiftMutation.isPending || updateShiftMutation.isPending) {
        return;
      }
      if (editor?.kind === "edit") {
        updateShiftMutation.mutate({ shiftId: editor.shift.id, values });
      } else {
        createShiftMutation.mutate(values);
      }
    },
    [editor, updateShiftMutation, createShiftMutation],
  );

  const weekRange = useMemo(() => {
    const fmt = new Intl.DateTimeFormat(intlLocaleFor(locale), {
      month: "short",
      day: "numeric",
    });
    const toDate = (key: string) => {
      const [y, m, d] = key.split("-").map(Number);
      return new Date(y, m - 1, d);
    };
    return `${fmt.format(toDate(days[0]))} – ${fmt.format(toDate(days[6]))}`;
  }, [days, locale]);

  const editorLabels: ShiftEditorLabels = useMemo(
    () => ({
      createTitle: t("editor.createTitle"),
      editTitle: t("editor.editTitle"),
      startTime: t("editor.startTime"),
      endTime: t("editor.endTime"),
      position: t("editor.position"),
      assignee: t("editor.assignee"),
      unassignedOption: t("editor.unassignedOption"),
      breakMinutes: t("editor.breakMinutes"),
      breakMinutesInvalid: t("editor.breakMinutesInvalid"),
      notes: t("editor.notes"),
      save: t("editor.save"),
      saving: t("editor.saving"),
      delete: t("editor.delete"),
      deleting: t("editor.deleting"),
      cancel: t("editor.cancel"),
      positionRequired: t("editor.positionRequired"),
      endEqualsStart: t("editor.endEqualsStart"),
      close: t("editor.close"),
      deleteConfirmTitle: t("editor.deleteConfirmTitle"),
      deleteConfirm: t("editor.deleteConfirm"),
      deleteKeep: t("editor.deleteKeep"),
      duration: t("editor.duration"),
      repeatLabel: t("editor.repeatLabel"),
      viewTeamProfile: t("editor.viewTeamProfile"),
      discardConfirmTitle: t("editor.discardConfirmTitle"),
      discardConfirmDescription: t("editor.discardConfirmDescription"),
      discardConfirm: t("editor.discardConfirm"),
      discardKeep: t("editor.discardKeep"),
    }),
    [t],
  );

  const conflictLabels: ShiftConflictLabels = useMemo(
    () => ({
      heading: t("conflicts.heading"),
      timeoff: t("conflicts.timeoff"),
      unavailable: t("conflicts.unavailable"),
      unavailableAllDay: t("conflicts.unavailableAllDay"),
      doublebook: t("conflicts.doublebook"),
      overrideHint: t("conflicts.overrideHint"),
      dayPrefix: t("conflicts.dayPrefix"),
    }),
    [t],
  );

  const isError =
    settingsQuery.isError ||
    scheduleQuery.isError ||
    positionsQuery.isError ||
    staffQuery.isError;
  // scheduleQuery is gated on settingsReady, so while settings is loading/errored
  // it reports isLoading=true (pending + disabled). Don't let that hold the
  // spinner past a settings error — the error branch (checked after isLoading)
  // must win, so drop scheduleQuery from the loading signal until settings land.
  const isLoading =
    !isError &&
    (settingsQuery.isLoading ||
      (settingsReady && scheduleQuery.isLoading) ||
      positionsQuery.isLoading ||
      staffQuery.isLoading);
  const published = schedule?.status === "published";
  const inboxTotal = reqCount + tsCount;

  // Cold start: a brand-new business with nothing set up yet. Show the guided
  // setup checklist instead of an unusable empty grid + empty inbox.
  const hasTeam = staffList.length > 0 || invitationCount > 0;
  const coldStart =
    !isLoading && !isError && positions.length === 0 && !hasTeam;

  // Availability overlay (Phase 1): the manager grid shows each teammate's
  // recurring availability + approved time-off so shifts aren't built blind.
  // Read-only and informative — managers always override. Fetched once the
  // business has a team; skipped at cold start.
  const teamAvailabilityQuery = useQuery({
    queryKey: queryKeys.availability.team(businessId),
    queryFn: () => availabilityApi.getTeam(businessId),
    enabled: hasTeam,
    staleTime: 5 * 60 * 1000,
  });
  const approvedTimeOffQuery = useQuery({
    queryKey: queryKeys.timeOff.list(businessId, "approved"),
    queryFn: () => availabilityApi.listTimeOff(businessId, "approved"),
    enabled: hasTeam,
    staleTime: 5 * 60 * 1000,
  });
  const teamAvailability = useMemo<TeamAvailability>(
    () => teamAvailabilityQuery.data ?? {},
    [teamAvailabilityQuery.data],
  );
  const approvedTimeOff = useMemo<TimeOffRequest[]>(
    () => approvedTimeOffQuery.data ?? [],
    [approvedTimeOffQuery.data],
  );

  const setupSteps = useMemo(
    () =>
      buildSetupSteps(
        setupT,
        { hasPositions: positions.length > 0, hasTeam },
        {
          addPositions: () => onNavigate?.("staff?sub=positions"),
          invite: () => onNavigate?.("staff"),
          // Already on the Schedule tab; this step is the destination, never the
          // current actionable one at cold start, so it needs no handler here.
        },
      ),
    [setupT, positions.length, hasTeam, onNavigate],
  );

  // Debounce the sales-target input before it feeds the preview query, and
  // persist the settled value so it survives reloads.
  useEffect(() => {
    const id = setTimeout(() => {
      setDebouncedSalesTarget(salesTarget);
      if (typeof window !== "undefined") {
        if (salesTarget && salesTarget > 0) {
          window.localStorage.setItem(salesTargetStorageKey, String(salesTarget));
        } else {
          window.localStorage.removeItem(salesTargetStorageKey);
        }
      }
    }, 400);
    return () => clearTimeout(id);
  }, [salesTarget, salesTargetStorageKey]);

  // Labor preview (Slice 6): build-time scheduled-labor lens. Hours + labor-% for
  // every operator; labor-$ is server-gated on owner/financial:read and only
  // surfaced when canViewFinancials is also true (defense-in-depth in the footer).
  const scheduleId = schedule?.id ?? 0;
  const laborPreviewQuery = useQuery({
    queryKey: queryKeys.schedule.laborPreview(
      businessId,
      scheduleId,
      debouncedSalesTarget,
    ),
    queryFn: () =>
      scheduleApi.laborPreview(businessId, scheduleId, debouncedSalesTarget),
    enabled: scheduleId > 0,
    staleTime: 30 * 1000,
    retry: false,
  });

  // Default sales target from last week's recognized net sales so Labor % is
  // not blank by default (#165). Never overwrite a stored/manual value.
  const lastWeekKey = useMemo(() => shiftWeekKey(weekKey, -1), [weekKey]);
  const lastWeekDays = useMemo(() => weekDayKeys(lastWeekKey), [lastWeekKey]);
  useEffect(() => {
    if (!canViewFinancials || salesTarget != null || salesTargetDefaulted.current) {
      return;
    }
    let cancelled = false;
    salesTargetDefaulted.current = true;
    void laborCostApi
      .getLaborCost(businessId, "week", {
        start: lastWeekDays[0],
        end: lastWeekDays[6],
      })
      .then((report) => {
        if (cancelled) return;
        const net = Number(report.net_sales);
        if (Number.isFinite(net) && net > 0) {
          setSalesTarget(Math.round(net));
        }
      })
      .catch(() => {
        // Soft default — leave the input empty if last-week sales aren't readable.
      });
    return () => {
      cancelled = true;
    };
  }, [businessId, canViewFinancials, lastWeekDays, salesTarget]);

  // Publish-confirm summary: shift count, unassigned ("open") shifts, and the
  // count of warnings already computed server-side for the labor preview. An
  // unassigned shift is one with no staff_id (mirrors the grid's `open` check).
  const openShiftCount = useMemo(
    () => shifts.filter((sh) => sh.staff_id == null).length,
    [shifts],
  );
  const publishWarningCount = laborPreviewQuery.data?.warnings.length ?? 0;
  const publishConfirmDescription = t("publishConfirm.description")
    .replace("{shifts}", String(shifts.length))
    .replace("{open}", String(openShiftCount))
    .replace("{warnings}", String(publishWarningCount));

  const laborLabels: LaborPreviewLabels = useMemo(
    () => ({
      title: t("labor.title"),
      scheduledHours: t("labor.scheduledHours"),
      laborCost: t("labor.laborCost"),
      laborCostPct: t("labor.laborCostPct"),
      salesTarget: t("labor.salesTarget"),
      salesTargetPlaceholder: t("labor.salesTargetPlaceholder"),
      salesTargetHint: t("labor.salesTargetHint"),
      hoursOnly: t("labor.hoursOnly"),
      warnings: {
        overtime: t("warnings.overtime"),
        // On a draft the lead-time warning is a forecast, not a verdict — the
        // week hasn't been published yet, so speak in the future tense.
        postedLate:
          schedule?.status === "published"
            ? t("warnings.postedLate")
            : t("warnings.postedLateDraft"),
        minorLate: t("warnings.minorLate"),
      },
    }),
    [t, schedule?.status],
  );

  const scrollToInbox = useCallback(() => {
    if (reqCount === 0 && tsCount > 0) {
      inboxTouched.current = true;
      setInboxView("timesheets");
    } else if (reqCount > 0) {
      inboxTouched.current = true;
      setInboxView("requests");
    }
    if (typeof document !== "undefined") {
      document
        .getElementById("schedule-approvals-inbox")
        ?.scrollIntoView({ behavior: "smooth", block: "start" });
    }
  }, [reqCount, tsCount]);

  const editorInitial = useMemo(() => {
    if (editor?.kind === "edit") {
      const sh = editor.shift;
      return {
        dayKey: businessDateKey(sh.starts_at, venueTimeZone),
        initial: {
          startTime: toTimeInput(sh.starts_at, venueTimeZone),
          endTime: toTimeInput(sh.ends_at, venueTimeZone),
          positionId: sh.position_id,
          staffId: sh.staff_id ?? null,
          breakMinutes: sh.break_minutes,
          notes: sh.notes ?? "",
        },
      };
    }
    if (editor?.kind === "create") {
      // Prefill from the person's latest shift this week — most people work the
      // same hours day to day, so their last shift beats a 09:00–17:00 default.
      const lastShift =
        editor.staffId != null
          ? shifts
              .filter((sh) => sh.staff_id === editor.staffId)
              .sort(
                (a, b) =>
                  new Date(b.starts_at).getTime() -
                  new Date(a.starts_at).getTime(),
              )[0]
          : undefined;
      return {
        dayKey: editor.dayKey,
        initial: {
          startTime: lastShift
            ? toTimeInput(lastShift.starts_at, venueTimeZone)
            : "10:00",
          endTime: lastShift
            ? toTimeInput(lastShift.ends_at, venueTimeZone)
            : "18:00",
          positionId: editor.positionId ?? lastShift?.position_id ?? null,
          staffId: editor.staffId,
          breakMinutes: lastShift?.break_minutes ?? 0,
          notes: "",
        },
      };
    }
    return null;
  }, [editor, shifts, venueTimeZone]);
  // Week range and grid view live in the toolbar directly below the header —
  // repeating them as hero stats was pure noise. Only live schedule counts
  // earn a spot on the header's stat line.
  const shellStats = coldStart
    ? []
    : [
        { label: t("shell.team"), value: staffList.length },
        { label: t("shell.shifts"), value: shifts.length },
        {
          label: t("shell.inbox"),
          value: inboxTotal,
          title: t("shell.inboxHint"),
          onClick: scrollToInbox,
        },
      ];

  return (
    <DashboardTabShell
      width="wide"
      loading={
        isLoading ? (
          <DashboardTabLoadingSkeleton
            variant="table"
            labelKey="loadingTab"
            withPageChrome={false}
          />
        ) : null
      }
      header={{
        title: t("title"),
        subtitle: t("subtitle"),
        status: {
          label: published ? t("publishedBadge") : t("draftBadge"),
          tone: published ? "positive" : "attention",
        },
        stats: shellStats,
        actions: (
          <>
            {!coldStart ? (
              <Button
                size="sm"
                variant="bordered"
                radius="full"
                className={btnSecondaryNextUI}
                startContent={<Tablet className="h-4 w-4" />}
                onPress={() => setKioskOpen(true)}
              >
                {t("kiosk")}
              </Button>
            ) : null}
            <button
              type="button"
              className={btnGhostIcon}
              aria-label={t("settings")}
              title={t("settings")}
              onClick={() => setSettingsOpen(true)}
            >
              <Settings className="h-4 w-4" />
            </button>
            {/* Publish is one-way (no unpublish endpoint); once published the
                week stays live-editable, so a permanently disabled Publish
                button would be dead weight — hide it instead. */}
            {!published ? (
              <Button
                size="sm"
                radius="full"
                isLoading={publishMutation.isPending}
                isDisabled={!schedule}
                onPress={publishConfirm.onOpen}
                className={btnPrimaryNextUI}
              >
                {publishMutation.isPending ? t("publishing") : t("publish")}
              </Button>
            ) : null}
          </>
        ),
      }}
    >
      {settingsOpen ? (
        <ScheduleSettingsModal
          businessId={businessId}
          onClose={() => setSettingsOpen(false)}
        />
      ) : null}

      {kioskOpen ? (
        <KioskClockIn
          businessId={businessId}
          onClose={() => setKioskOpen(false)}
        />
      ) : null}

      <ConfirmationModal
        isOpen={publishConfirm.isOpen}
        onOpenChange={publishConfirm.onOpenChange}
        title={t("publishConfirm.title")}
        description={publishConfirmDescription}
        confirmLabel={t("publishConfirm.confirm")}
        cancelLabel={t("publishConfirm.cancel")}
        onConfirm={() => publishMutation.mutate()}
      />

      {!coldStart ? (
        <PremiumPanel
          className="flex flex-wrap items-center justify-between gap-3 p-3"
          withTexture={false}
        >
          <div className="flex items-center gap-1">
            <Button
              isIconOnly
              size="sm"
              variant="bordered"
              aria-label={t("prevWeek")}
              onPress={() => setWeekOffset((o) => o - 1)}
            >
              <ChevronLeft className="h-4 w-4" />
            </Button>
            <Button
              size="sm"
              variant="bordered"
              onPress={() => setWeekOffset(0)}
            >
              {t("thisWeek")}
            </Button>
            <Button
              isIconOnly
              size="sm"
              variant="bordered"
              aria-label={t("nextWeek")}
              onPress={() => setWeekOffset((o) => o + 1)}
            >
              <ChevronRight className="h-4 w-4" />
            </Button>
            <span className="ml-2 text-sm font-semibold text-ink-700">
              {weekRange}
            </span>
            <span
              className="ml-2 text-xs font-medium text-ink-500"
              data-testid="schedule-week-start-label"
              data-week-start-day={String(weekStartDay)}
              data-week-start-name={weekStartDayName(weekStartDay, locale)}
            >
              {t("weekStartsOn").replace(
                "{day}",
                weekStartDayName(weekStartDay, locale),
              )}
            </span>
          </div>
          <div className="flex flex-wrap items-center gap-2">
            {/* Bulk action lives with week chrome so it isn't a lone pale float (#231). */}
            <Button
              size="sm"
              variant="bordered"
              radius="full"
              onPress={() => copyLastWeekMutation.mutate(shifts.length > 0)}
              isLoading={copyLastWeekMutation.isPending}
              className={`${btnSecondaryNextUI} text-xs`}
              title={t("copyWeek.sourceHint")}
            >
              {copyLastWeekMutation.isPending
                ? t("copyWeek.copying")
                : shifts.length > 0
                  ? t("copyWeek.actionEmptyDays")
                  : t("copyWeek.action")}
            </Button>
            <SegmentedTabs
              ariaLabel={t("title")}
              size="sm"
              activeKey={mode}
              onChange={(key) => setMode(key as ViewMode)}
              tabs={[
                { key: "employee", label: t("byEmployee") },
                { key: "role", label: t("byRole") },
              ]}
            />
          </div>
        </PremiumPanel>
      ) : null}

      {coldStart ? (
        <SetupChecklist
          title={setupT("title")}
          subtitle={setupT("subtitle")}
          steps={setupSteps}
        />
      ) : isError ? (
        /* #830: a real error, not a dead end — retry refetches whichever
           queries failed instead of asking the operator to reload the tab. */
        <PremiumPanel className="p-4" withTexture={false}>
          <div className="flex flex-col items-start gap-3">
            <p className="text-sm text-ink-600">{t("error")}</p>
            <Button
              size="sm"
              radius="full"
              variant="bordered"
              onPress={() => {
                if (settingsQuery.isError) void settingsQuery.refetch();
                if (scheduleQuery.isError) void scheduleQuery.refetch();
                if (positionsQuery.isError) void positionsQuery.refetch();
                if (staffQuery.isError) void staffQuery.refetch();
              }}
            >
              {t("errorRetry")}
            </Button>
          </div>
        </PremiumPanel>
      ) : positions.length === 0 ? (
        <ScheduleNoPositionsPanel
          title={t("noPositionsTitle")}
          subtitle={t("noPositionsSubtitle")}
          actionLabel={onNavigate ? setupT("positionsAction") : undefined}
          onAction={
            onNavigate ? () => onNavigate("staff?sub=positions") : undefined
          }
        />
      ) : (
        <div className="space-y-2">
          {published ? (
            <p className="px-1 text-xs text-ink-500">{t("publishedEditNote")}</p>
          ) : shifts.length === 0 ? (
            <p className="px-1 text-xs text-ink-500">{t("emptyWeekHint")}</p>
          ) : null}
          {mode === "employee" ? (
            <div className="flex flex-wrap items-center gap-3 px-1 text-[11px] text-ink-500">
              <span className="inline-flex items-center gap-1">
                <span
                  className="h-2 w-2 rounded-sm bg-amber-100 ring-1 ring-amber-300"
                  aria-hidden="true"
                />
                {t("overlay.unavailable")}
              </span>
              <span className="inline-flex items-center gap-1">
                <span
                  className="h-2 w-2 rounded-sm bg-red-100 ring-1 ring-red-300"
                  aria-hidden="true"
                />
                {t("overlay.timeOff")}
              </span>
            </div>
          ) : null}
          {/* Phone view: the desktop grid needs ~860px; below md show a day
              switcher + one day's shifts as a vertical list (same data/actions).
              Only one of the two views mounts (isPhone is false under SSR/jsdom). */}
          {isPhone ? (
          <ScheduleWeekMobile
            days={days}
            todayKey={businessDateKey(new Date(), venueTimeZone)}
            rows={rows}
            mode={mode}
            locale={locale}
            t={t}
            shiftsForCell={shiftsForCell}
            positionsById={positionsById}
            staffById={staffById}
            teamAvailability={teamAvailability}
            approvedTimeOff={approvedTimeOff}
            dayTotalMinutes={dayTotalMinutes}
            dayCoverage={dayCoverage}
            formatTimeRange={(start, end, loc) =>
              formatTimeRange(start, end, loc, venueTimeZone)
            }
            fmtMinutes={fmtMinutes}
            onEditShift={(shift) => setEditor({ kind: "edit", shift })}
            onCreateShift={({ dayKey, staffId, positionId }) =>
              setEditor({ kind: "create", dayKey, staffId, positionId })
            }
          />
          ) : null}
          {/* min-w-0 keeps this flex descendant from growing to the 7-col
              min-width; overflow-x-auto then reaches Sat/Sun at ~1024px. */}
          <div
            data-testid="schedule-week-scroll"
            className="hidden min-w-0 w-full overflow-x-auto rounded-2xl md:block"
          >
          <PremiumPanel
            className="!overflow-visible p-0"
            withTexture={false}
          >
            <div className="grid min-w-[660px] grid-cols-[8rem_repeat(7,minmax(4.75rem,1fr))]">
              <div className="border-b border-r border-warm-200 bg-warm-50/80 p-2" />
              {days.map((dayKey, dayIdx) => {
                const [y, m, d] = dayKey.split("-").map(Number);
                const date = new Date(y, m - 1, d);
                const isToday =
                  dayKey === businessDateKey(new Date(), venueTimeZone);
                return (
                  <div
                    key={dayKey}
                    aria-current={isToday ? "date" : undefined}
                    data-schedule-col="day"
                    data-testid={
                      dayIdx === 0
                        ? "schedule-week-first-day"
                        : dayIdx === days.length - 1
                          ? "schedule-week-last-day"
                          : undefined
                    }
                    data-day-key={dayKey}
                    className={`border-b border-r border-warm-200 p-2 text-center last:border-r-0 ${
                      isToday ? "bg-brand/5" : "bg-warm-50/80"
                    }`}
                  >
                    <p
                      className={`text-xs font-semibold uppercase ${
                        isToday ? "text-brand" : "text-ink-500"
                      }`}
                    >
                      {new Intl.DateTimeFormat(intlLocaleFor(locale), {
                        weekday: "short",
                      }).format(date)}
                    </p>
                    <p
                      className={`text-xs ${isToday ? "text-brand/70" : "text-ink-500"}`}
                    >
                      {new Intl.DateTimeFormat(intlLocaleFor(locale), {
                        day: "numeric",
                        month: "short",
                      }).format(date)}
                    </p>
                    <DaypartCoverageChips
                      lunchMin={dayCoverage.get(dayKey)?.lunchMin ?? 0}
                      dinnerMin={dayCoverage.get(dayKey)?.dinnerMin ?? 0}
                      dayKey={dayKey}
                      t={t}
                    />
                  </div>
                );
              })}

              {rows.map((row) => {
                const weekMinutes = rowWeeklyMinutes(row);
                const weekBreak = rowWeeklyBreakMinutes(row);
                return (
                <React.Fragment key={row.key}>
                  <div className="flex items-center gap-2 border-b border-r border-warm-200 bg-white p-2">
                    {row.colorHex ? (
                      <span
                        className="h-2.5 w-2.5 shrink-0 rounded-full"
                        style={{ backgroundColor: row.colorHex }}
                        aria-hidden="true"
                      />
                    ) : null}
                    <span className="truncate text-sm font-semibold text-ink-800">
                      {row.label}
                    </span>
                    {weekMinutes > 0 ? (
                      <span
                        className="ml-auto shrink-0 text-[11px] font-medium tabular-nums text-ink-500"
                        title={
                          weekBreak > 0
                            ? t("rowHoursWithBreak")
                                .replace("{hours}", fmtMinutes(weekMinutes))
                                .replace("{break}", fmtMinutes(weekBreak))
                            : undefined
                        }
                      >
                        {weekBreak > 0
                          ? t("rowHoursWithBreak")
                              .replace("{hours}", fmtMinutes(weekMinutes))
                              .replace("{break}", fmtMinutes(weekBreak))
                          : fmtMinutes(weekMinutes)}
                      </span>
                    ) : null}
                  </div>
                  {days.map((dayKey) => {
                    const cellShifts = shiftsForCell(row, dayKey);
                    // Availability overlay: employee mode only, real teammates only
                    // (never the "open shifts" row). Exceptions only — timeoff and
                    // unavailable get a tint + chip; "preferred" stays quiet so the
                    // grid isn't wallpapered with "Available" on every cell.
                    const posture =
                      mode === "employee" && row.staffId != null
                        ? cellPosture(
                            row.staffId,
                            dayKey,
                            teamAvailability,
                            approvedTimeOff,
                          )
                        : { kind: "none" as const };
                    const tint =
                      posture.kind === "timeoff"
                        ? "bg-red-50/70"
                        : posture.kind === "unavailable"
                          ? "bg-amber-50/60"
                          : "bg-white";
                    return (
                      <div
                        key={`${row.key}-${dayKey}`}
                        className={`min-h-[64px] space-y-1.5 border-b border-r border-warm-200/70 p-1.5 last:border-r-0 ${tint}`}
                      >
                        {posture.kind === "timeoff" ? (
                          <span className="inline-flex items-center gap-1 rounded-md bg-red-100 px-1.5 py-0.5 text-[10px] font-medium text-red-700">
                            <CalendarOff
                              className="h-2.5 w-2.5"
                              aria-hidden="true"
                            />
                            {t("overlay.timeOff")}
                          </span>
                        ) : posture.kind === "unavailable" ? (
                          <span className="inline-flex items-center gap-1 rounded-md bg-amber-100 px-1.5 py-0.5 text-[10px] font-medium text-amber-700">
                            <CalendarX2
                              className="h-2.5 w-2.5"
                              aria-hidden="true"
                            />
                            {t("overlay.unavailable")}
                          </span>
                        ) : null}
                        {cellShifts.map((shift) => {
                          const open = shift.staff_id == null;
                          const position = positionsById.get(shift.position_id);
                          const assignee =
                            shift.staff_id != null
                              ? staffById.get(shift.staff_id)?.name
                              : null;
                          return (
                            <button
                              key={shift.id}
                              type="button"
                              onClick={() => setEditor({ kind: "edit", shift })}
                              className={`block w-full rounded-xl border px-2 py-1 text-left text-xs shadow-sm shadow-warm-900/5 transition hover:border-brand/30 ${
                                open
                                  ? "border-amber-200 bg-amber-50"
                                  : "border-warm-200 bg-white"
                              }`}
                            >
                              <span className="block font-semibold text-ink-950">
                                {formatTimeRange(
                                  shift.starts_at,
                                  shift.ends_at,
                                  locale,
                                  venueTimeZone,
                                )}
                              </span>
                              <span className="mt-0.5 flex items-center gap-1 text-[11px] text-ink-500">
                                {mode === "role" ? (
                                  open ? (
                                    <span className="font-medium text-amber-700">
                                      {t("open")}
                                    </span>
                                  ) : (
                                    assignee
                                  )
                                ) : (
                                  <>
                                    <span
                                      className="h-1.5 w-1.5 rounded-full bg-brand"
                                      style={
                                        position?.color_hex
                                          ? {
                                              backgroundColor:
                                                position.color_hex,
                                            }
                                          : undefined
                                      }
                                      aria-hidden="true"
                                    />
                                    {position ? positionLabel(position.name) : null}
                                  </>
                                )}
                              </span>
                            </button>
                          );
                        })}
                        {/* Always available: the backend supports live edits to a
                            published week (create notifies the assignee, update
                            fires shift.updated). The editor carries the
                            published-week note so the operator knows it's live. */}
                        <button
                          type="button"
                          aria-label={addShiftAccessibleName(
                            t("addShiftFor"),
                            row.label,
                            dayKey,
                            locale,
                          )}
                          onClick={() =>
                            setEditor({
                              kind: "create",
                              dayKey,
                              staffId: row.staffId ?? null,
                              positionId: row.positionId ?? null,
                            })
                          }
                          className="flex w-full items-center justify-center gap-1 rounded-xl border border-dashed border-warm-200 py-1 text-[11px] font-medium text-ink-400 transition hover:border-brand/40 hover:text-brand"
                        >
                          <Plus className="h-3 w-3" />
                          {t("addShift")}
                        </button>
                      </div>
                    );
                  })}
                </React.Fragment>
                );
              })}

              {/* Balance footer: scheduled hours under each day so a lopsided
                  week (all hands Friday, a skeleton Tuesday) reads at a glance. */}
              <div className="border-r border-warm-200 bg-warm-50/60 p-2 text-xs font-medium text-ink-500">
                {t("dayTotals")}
              </div>
              {days.map((dayKey) => {
                const mins = dayTotalMinutes.get(dayKey) ?? 0;
                return (
                  <div
                    key={`total-${dayKey}`}
                    className="border-r border-warm-200 bg-warm-50/60 p-2 text-center text-xs tabular-nums text-ink-600 last:border-r-0"
                  >
                    {mins > 0 ? fmtMinutes(mins) : "—"}
                  </div>
                );
              })}
            </div>
          </PremiumPanel>
          </div>
        </div>
      )}

      {/* Live labor preview (Slice 6): hours + labor-% for every operator; labor-$
          + the editable sales target only when canViewFinancials (the server also
          strips $ for non-financial callers). Operator-only — never staff-facing.
          Sits directly under the grid — it's the scheduled-labor lens on the week
          you're building. */}
      {scheduleId > 0 && laborPreviewQuery.data ? (
        <LaborPreviewFooter
          preview={laborPreviewQuery.data}
          labels={laborLabels}
          canViewFinancials={canViewFinancials}
          currency={currency}
          locale={locale}
          salesTarget={salesTarget}
          onSalesTargetChange={setSalesTarget}
          staffNames={staffNamesById}
          onPostedLateAction={() => setSettingsOpen(true)}
          postedLateActionLabel={t("warnings.postedLateAction")}
        />
      ) : null}

      {/* Today's live floor: who's on the clock / late / no-show right now. It
          self-hides when there's nothing to show (off-hours, or a non-manager
          403), so it only surfaces during service. Sits between the week grid
          and the approval inbox — the two operational-attention surfaces. */}
      {!coldStart && (
        <LiveFloorBoard
          businessId={businessId}
          businessTimezone={venueTimeZone}
        />
      )}

      {/* Today's shift-handover log, surfaced to managers (Slice 5b): the read
          side of the notes staff write from their own logbook, each stamped with
          its author. Self-hides when no notes today, so it only appears when
          there's a handover to read. */}
      {!coldStart && (
        <OperatorLogbook
          businessId={businessId}
          businessTimezone={venueTimeZone}
        />
      )}

      {/* Needs your approval — one inbox that consolidates what used to be two
          stacked cards: time-off / coverage requests (Slices 3 & 5) and timesheet
          review (Slice 4). Each is a tab carrying its own count badge; both
          panels stay mounted (the inactive one hidden) so the counts and the
          header total stay live without refetching on tab switch. */}
      {!coldStart && (
        <PremiumPanel
          as="section"
          id="schedule-approvals-inbox"
          aria-label={t("inbox.title")}
          className="overflow-hidden"
          withTexture={false}
        >
          <div className="flex flex-wrap items-center justify-between gap-3 border-b border-warm-200/80 bg-warm-50/60 p-4">
            <div className="flex items-center gap-2">
              <h2 className="font-title text-base text-ink-950">
                {t("inbox.title")}
              </h2>
              {inboxTotal > 0 ? (
                <Chip size="sm" variant="flat" color="warning">
                  {/* L5-35: same cap as SegmentedTabs badges — one source of truth. */}
                  {inboxTotal > 9 ? "9+" : inboxTotal}
                </Chip>
              ) : null}
            </div>
            <SegmentedTabs
              ariaLabel={t("inbox.title")}
              size="sm"
              activeKey={inboxView}
              onChange={(k) => {
                inboxTouched.current = true;
                setInboxView(k as InboxView);
              }}
              tabs={[
                {
                  key: "requests",
                  label: t("inbox.requests"),
                  icon: CalendarCheck2,
                  badge: reqCount,
                },
                {
                  key: "timesheets",
                  label: t("inbox.timesheets"),
                  icon: ClipboardCheck,
                  badge: tsCount,
                },
              ]}
            />
          </div>

          <div className={inboxView === "requests" ? "" : "hidden"}>
            <ApprovalsPanel
              businessId={businessId}
              businessTimezone={venueTimeZone}
              embedded
              onPendingCountChange={setReqCount}
            />
          </div>
          <div className={inboxView === "timesheets" ? "" : "hidden"}>
            <TimesheetReview
              businessId={businessId}
              businessTimezone={venueTimeZone}
              canViewFinancials={canViewFinancials}
              currency={currency}
              embedded
              onPendingCountChange={setTsCount}
            />
          </div>
        </PremiumPanel>
      )}

      {editorInitial ? (
        <ShiftEditorModal
          isEdit={editor?.kind === "edit"}
          dayKey={editorInitial.dayKey}
          weekDays={days}
          initial={editorInitial.initial}
          positions={positions}
          staff={staffList}
          labels={editorLabels}
          team={teamAvailability}
          approvedTimeOff={approvedTimeOff}
          weekShifts={shifts}
          editingShiftId={editor?.kind === "edit" ? editor.shift.id : undefined}
          conflictLabels={conflictLabels}
          locale={locale}
          submitting={
            createShiftMutation.isPending || updateShiftMutation.isPending
          }
          deleting={deleteShiftMutation.isPending}
          publishedNote={published ? t("publishedEditNote") : undefined}
          onCancel={() => {
            // New editor open after cancel is a fresh attempt (mint new UUID).
            createAttemptRef.current = null;
            setEditor(null);
          }}
          onSubmit={handleSubmit}
          onDelete={
            editor?.kind === "edit"
              ? () => deleteShiftMutation.mutate(editor.shift.id)
              : undefined
          }
          onViewStaff={(name) =>
            onNavigate?.(
              `staff?sub=people&staffSearch=${encodeURIComponent(name)}`,
            )
          }
        />
      ) : null}
    </DashboardTabShell>
  );
}
