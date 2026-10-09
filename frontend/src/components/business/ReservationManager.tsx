import React, {
  useState,
  useEffect,
  useCallback,
  useMemo,
  useRef,
} from "react";
import { usePathname, useRouter, useSearchParams } from "next/navigation";
import { useSubTab } from "@/lib/subTabs";
import toast from "react-hot-toast";
import {
  Input,
  Button,
  Table,
  TableHeader,
  TableColumn,
  TableBody,
  TableRow,
  TableCell,
  Chip,
  Modal,
  ModalContent,
  ModalHeader,
  ModalBody,
  ModalFooter,
  Select,
  SelectItem,
  Textarea,
  Pagination,
  DatePicker,
} from "@nextui-org/react";
import { parseDate } from "@internationalized/date";
import { Calendar, CalendarDays, Search, Plus, Eye, Edit, Users, Clock, Phone, CheckCircle, XCircle, AlertCircle, Settings, PanelRightOpen, PanelRightClose, MapPin } from "lucide-react";
import {
  reservationAPI,
  getAllUpcomingReservations,
  getReservationClaimConflict,
  Reservation,
  CreateReservationRequest,
  ReservationTableOption,
  ReservationStatus,
  ReservationStats,
} from "@/api/reservations";
import { ensureReservationClaim } from "./reservations/ensureReservationClaim";
import {
  businessApi,
  getBusiness,
  getBusinessOperatingHours,
  type BusinessOperatingHours,
  Table as TableType,
} from "@/api/business";
import {
  useSimpleLocale,
  getTranslation,
} from "@/i18n/SimpleTranslationProvider";
import { useBusinessAccess } from "@/hooks/useBusinessAccess";
import { useDialogBehavior } from "@/hooks/useDialogBehavior";
import { useDirtyForm } from "@/hooks/useDirtyForm";
import { useUnsavedChangesGuard } from "@/hooks/useUnsavedChangesGuard";
import { useAuth } from "@/providers/HybridAuthProvider";
import { getReservationCapabilities } from "@/utils/staffAuth";
import { useStaffPermissionsContext } from "@/contexts/StaffPermissionsContext";
import { localDateKey } from "@/lib/localDate";
import { getApiErrorCode } from "@/utils/apiError";
import {
  DATE_TIME_SHORT,
  TIME_SHORT,
  businessDateKey,
  formatBusinessDateTime,
  formatBusinessTime,
  resolveBusinessTimeZone,
} from "@/utils/businessTime";
import { getTimezoneLabel } from "@/utils/timezones";
import {
  instantToWallTime,
  nextBookableWallTime,
  wallTimeToInstant,
} from "@/utils/zonedDateTime";
import { reservationFitsClose } from "@/utils/reservationFitsClose";
import { useSSEEvents, type SSEEvent } from "@/hooks/useSSEEvents";
import {
  buildReservationUpdatePayload,
  clampPartySize,
  coversOnBusinessDay,
  formatNextArrivalInsight,
  mergeReservationBooks,
  nextUpcomingArrival,
  operatingWindowResolver,
  reservationErrorMessage,
  reservationWallTimeToISO,
} from "./reservationFormHelpers";
import { reservationDisplayStatus } from "./reservations/reservationArrivalClock";
import DashboardLockedTabView from "./DashboardLockedTabView";
import ReservationApprovalQueue from "./ReservationApprovalQueue";
import ReservationToggle, { useReservationStatus } from "./ReservationToggle";
import ReservationSettings from "./ReservationSettings";
import { ReservationsSkeleton } from "./ReservationsSkeleton";
import { StatusChip } from "../ui/StatusChip";
import { EmptyState } from "@/components/ui/EmptyState";
import { reservationPhoneGate } from "./reservationPhoneValidation";
import OperationalAlertClaimStatus from "./operational-alerts/OperationalAlertClaimStatus";
import { useOptionalOperationalAlerts } from "./operational-alerts/useOperationalAlerts";
import { showApprovedFollowUpToast } from "./reservations/approvalFollowUpToast";
import { type ReservationTimeEntryMode } from "./reservations/ReservationTimeField";
import {
  isClientOnlyReservationFiltersActive,
  resolveServerReservationFilters,
  shouldLoadCompleteReservations,
  shouldLoadPagedReservations,
} from "./reservations/reservationFilterMode";
import {
  decideReservationRefreshCoalesce,
  RESERVATION_SSE_COALESCE_MS,
} from "./reservations/reservationSseCoalesce";
import {
  shouldShowReservationsRefreshing,
  shouldShowReservationsSkeleton,
} from "./reservations/reservationLoadingGate";
import { capBoardColumnItems } from "./reservations/boardColumnCap";
import {
  ReservationBoardScroller,
  reservationBoardLaneClass,
} from "./reservations/ReservationBoardScroller";
import { TABLERO_CHROME_CLASS } from "./reservations/reservationBoardLayout";
import { ReservationFormModal } from "./reservations/ReservationFormModal";
import { ReservationDetailDrawer } from "./reservations/ReservationDetailDrawer";
import RowActionsMenu, { type RowActionItem } from "./shared/RowActionsMenu";
import {
  shouldFlipToUpcomingAfterCreate,
  shouldScheduleRefreshAfterCreate,
} from "./reservations/reservationPostCreate";
import {
  canMarkNoShowNow,
  canSeatNow,
} from "./reservations/canPerformArrivalActions";
import { normalizeReservationDateFilter } from "./reservationDateRange";
import ConfirmationModal from "./modals/ConfirmationModal";
import { resolveReservationEmptyKind } from "./reservations/reservationEmptyState";
import { reservationCardTimeMode } from "./reservations/reservationCardTime";
import DashboardTabShell from "./shared/DashboardTabShell";
import {
  AnimatedNumberText,
  DashboardTabTransition,
  PremiumPanel,
} from "./premium";

interface ReservationManagerProps {
  businessId: number;
  onNavigateToTab?: (tab: string) => void;
}

/** Derived attention chips only — time window lives in dateFilter alone. */
type ReservationAttentionFilter =
  | "none"
  | "next2hours"
  | "needs_table"
  | "no_show_risk";

type ReservationView = "operations" | "list";

function parseReservationsView(raw: string | null): ReservationView {
  return raw === "board" || raw === "operations" ? "operations" : "list";
}

const reservationMetricLabelClass =
  "text-xs font-semibold uppercase tracking-[0.18em] text-ink-500";
const reservationMetricValueClass =
  "mt-2 text-lg font-semibold text-ink-950 sm:text-xl";
const reservationIconPillClass =
  "flex h-11 w-11 items-center justify-center rounded-2xl border border-brand/15 bg-brand/5 text-brand shadow-sm shadow-brand/10";
const reservationFilterInactiveClass =
  "shrink-0 border border-warm-200/90 bg-white/85 text-ink-700 shadow-sm shadow-warm-900/5 hover:bg-brand/5 hover:text-brand-700";
const reservationBoardCardClass =
  "rounded-2xl border border-warm-200/80 bg-white/85 p-4 shadow-sm shadow-warm-900/5 transition-all duration-200 hover:-translate-y-0.5 hover:border-brand/25 hover:shadow-[0_16px_34px_rgba(46,42,37,0.08)]";
const reservationBadgeClass =
  "inline-flex items-center gap-1 rounded-full border border-warm-200 bg-warm-50/80 px-2.5 py-1 text-ink-600";
const reservationColumnEmptyClass =
  "h-auto min-h-0 rounded-2xl border border-dashed border-warm-300 bg-warm-50/70 px-3 py-2 text-center text-sm text-ink-600";
const reservationSectionTitleClass = "text-base font-semibold text-ink-950";
const reservationSectionDescriptionClass = "text-sm leading-5 text-ink-600";
const reservationSectionEyebrowClass =
  "text-xs font-semibold uppercase tracking-[0.18em] text-brand-700";
const reservationModalCopyClass = "text-sm leading-6 text-ink-600";
const reservationSoftBoxClass =
  "rounded-2xl border border-warm-200/80 bg-warm-50/70 p-4";
const reservationModalClassNames = {
  base: "border border-warm-200 bg-white shadow-[0_28px_80px_rgba(46,42,37,0.18)]",
  header: "border-b border-warm-200/80",
  body: "py-6",
  footer: "border-t border-warm-200/80 bg-warm-50/50",
};

const rowActionIconClass = "h-4 w-4";

function reservationListRowActions(
  reservation: Reservation,
  caps: { canEdit: boolean; canDelete: boolean },
  t: (key: string) => string,
): RowActionItem[] {
  const items: RowActionItem[] = [];
  if (caps.canEdit) {
    items.push({
      key: "edit",
      label: t("edit"),
      icon: <Edit className={rowActionIconClass} />,
    });
  }
  if (caps.canEdit && reservation.status === "pending") {
    items.push({
      key: "confirm",
      label: t("confirm"),
      icon: <CheckCircle className={rowActionIconClass} />,
    });
  }
  if (caps.canEdit && reservation.status === "waitlist") {
    items.push({
      key: "promote",
      label: t("promoteWaitlist"),
      icon: <CheckCircle className={rowActionIconClass} />,
    });
  }
  if (
    caps.canEdit &&
    reservation.status === "confirmed" &&
    canSeatNow(reservation)
  ) {
    items.push({
      key: "seat",
      label: t("markSeated"),
      icon: <Users className={rowActionIconClass} />,
    });
  }
  if (caps.canEdit && canMarkNoShowNow(reservation)) {
    items.push({
      key: "no-show",
      label: t("markNoShow"),
      icon: <AlertCircle className={rowActionIconClass} />,
    });
  }
  if (caps.canEdit && reservation.status === "seated") {
    items.push({
      key: "complete",
      label: t("markCompleted"),
      icon: <CheckCircle className={rowActionIconClass} />,
    });
  }
  if (caps.canDelete) {
    items.push({
      key: "cancel",
      label: t("cancel"),
      icon: <XCircle className={rowActionIconClass} />,
      tone: "danger",
    });
  }
  return items;
}

export default function ReservationManager({
  businessId,
  onNavigateToTab,
}: ReservationManagerProps) {
  const { locale } = useSimpleLocale();
  const [currentLocale, setCurrentLocale] = useState(locale);
  const operationalAlerts = useOptionalOperationalAlerts();

  const { hasAccess, loading: accessLoading } = useBusinessAccess(
    businessId.toString(),
  );
  const {
    enabled: reservationsEnabled,
    loading: reservationsLoading,
    setEnabled: setReservationsEnabled,
  } = useReservationStatus(businessId, !accessLoading && !hasAccess);
  const { permissions } = useStaffPermissionsContext();
  // Owners outside staff context get empty permissions; grant full caps when not staff.
  const { isStaffUser, staffData } = useAuth();
  // Steal mirrors backend reservationClaimActor: owner principal or manager role.
  const canStealReservationClaim = !staffData || staffData.role === "manager";
  const reservationCaps = isStaffUser
    ? getReservationCapabilities(permissions)
    : {
        canRead: true,
        canCreate: true,
        canEdit: true,
        canDelete: true,
        canManageSettings: true,
      };

  const RESERVATION_SUBS = ["reservations", "settings"] as const;
  const { sub, unknownSub, setSub } = useSubTab(
    "reservations",
    RESERVATION_SUBS,
    "reservations",
  );
  const activeTab =
    !reservationCaps.canManageSettings && sub === "settings"
      ? "reservations"
      : (sub ?? "reservations");
  const setActiveTab = (key: string) => {
    if ((RESERVATION_SUBS as readonly string[]).includes(key)) {
      setSub(key as (typeof RESERVATION_SUBS)[number]);
    }
  };
  useEffect(() => {
    if (unknownSub) {
      setSub("reservations");
      return;
    }
    if (sub === "settings" && !reservationCaps.canManageSettings) {
      setSub("reservations");
    }
  }, [unknownSub, sub, reservationCaps.canManageSettings, setSub]);

  const searchParams = useSearchParams();
  const router = useRouter();
  const pathname = usePathname();
  // Persist Board/List in ?reservationsView= so closing the detail drawer
  // (or a soft remount) cannot silently bounce the host back to List.
  const reservationsViewParam = searchParams?.get("reservationsView") ?? null;
  const [reservationView, setReservationViewState] = useState<ReservationView>(
    () => parseReservationsView(reservationsViewParam),
  );
  const boardScrollYRef = useRef(0);

  // Sync from the URL string only (not the searchParams object identity) so a
  // Board click is not clobbered by a stale empty-param effect before replace
  // lands — that race was resetting Board→List after every peek.
  useEffect(() => {
    setReservationViewState(parseReservationsView(reservationsViewParam));
  }, [reservationsViewParam]);

  const setReservationView = useCallback(
    (view: ReservationView) => {
      setReservationViewState(view);
      if (typeof window === "undefined") return;
      const params = new URLSearchParams(searchParams?.toString() ?? "");
      if (view === "list") {
        params.delete("reservationsView");
      } else {
        params.set("reservationsView", "board");
      }
      const qs = params.toString();
      const href = qs ? `${pathname}?${qs}` : pathname || "";
      router.replace(href, { scroll: false });
    },
    [pathname, router, searchParams],
  );
  // Attention chips only (next2hours / needs_table / no_show_risk). Time window
  // is solely dateFilter — no second competing "Today" control.
  const [attentionFilter, setAttentionFilter] =
    useState<ReservationAttentionFilter>("none");
  /** Expanded kanban columns (show all cards past the 25-cap). */
  const [expandedBoardColumns, setExpandedBoardColumns] = useState<
    Record<string, boolean>
  >({});
  const [pagedReservations, setPagedReservations] = useState<Reservation[]>([]);
  const [completeReservations, setCompleteReservations] = useState<
    Reservation[]
  >([]);
  // Today-window stats aggregate for insight KPIs (covers / waitlist). Exact
  // server totals — not a 100-row client hydrate.
  const [todayStats, setTodayStats] = useState<ReservationStats | null>(null);
  const todayStatsRequestIdRef = useRef(0);
  // Dedicated upcoming book for NEXT ARRIVAL. The list/board loaders follow
  // the Hoy / Próximas / custom date filter, so a confirmed tomorrow row can
  // sit in /reservations/upcoming (and the sidebar badge) while the today
  // page is only seated leftovers. KPI must not depend on that filter.
  const [upcomingKpiReservations, setUpcomingKpiReservations] = useState<
    Reservation[]
  >([]);
  const upcomingKpiRequestIdRef = useRef(0);
  const [tables, setTables] = useState<TableType[]>([]);
  // Business operating hours (for L1-17 default create slot clamp).
  const [operatingHours, setOperatingHours] = useState<
    BusinessOperatingHours[]
  >([]);
  const [reservationSettings, setReservationSettings] = useState<any | null>(
    null,
  );
  // The business's IANA timezone. Operator-facing reservation times must render
  // in the business day, not the operator's device timezone, so a manager
  // travelling (or a remote owner) reads the same clock the guest booked
  // against and the kitchen works to (R3-RS operator TZ).
  const [businessTimezone, setBusinessTimezone] = useState<string | null>(null);
  const [loading, setLoading] = useState(true);
  // Distinct from `loading` (which toggles on every refresh): flips true after
  // the first complete-reservations load resolves and never goes back. Insight
  // KPIs use it so they render a placeholder instead of a misleading `0`/`—`
  // before any data has arrived (F13).
  const [hasLoadedReservations, setHasLoadedReservations] = useState(false);
  // Per-source first-load flags: the skeleton/empty-state decision must key on
  // whether the ACTIVE view's data source has ever loaded, not on "any data
  // loaded". Otherwise switching to the board (complete hydrate) after the
  // paged list loaded flashes a dishonest "no reservations" empty state while
  // the hydrate is still in flight.
  const [hasLoadedComplete, setHasLoadedComplete] = useState(false);
  const [hasLoadedPaged, setHasLoadedPaged] = useState(false);
  const [search, setSearch] = useState("");
  const [statusFilter, setStatusFilter] = useState<string>("all");
  // Single source of truth for the active time window (default: local today).
  // R2-B6: every write is normalized, so a restored/legacy "all" preference
  // resolves to the surviving "all_time" key instead of leaving the Select
  // pointed at an item that no longer exists.
  const [dateFilter, setDateFilterState] = useState<string>("today");
  const setDateFilter = useCallback((value: string) => {
    setDateFilterState(normalizeReservationDateFilter(value));
  }, []);
  // Explicit custom history window (YYYY-MM-DD). Used when dateFilter=custom
  // so any historical range is reachable via start_date/end_date server params.
  const [customStartDate, setCustomStartDate] = useState("");
  const [customEndDate, setCustomEndDate] = useState("");
  const [reservationPage, setReservationPage] = useState(1);
  const [reservationPageSize] = useState(25);
  const [reservationTotal, setReservationTotal] = useState(0);
  const [reservationTotalPages, setReservationTotalPages] = useState(1);
  const [reservationDataCapped, setReservationDataCapped] = useState(false);
  const [stealPrompt, setStealPrompt] = useState<{
    name: string;
    resolve: (ok: boolean) => void;
  } | null>(null);
  const [actionLoadingId, setActionLoadingId] = useState<number | null>(null);
  // Guards the create/edit modal submit buttons against a double-click firing a
  // second booking before the first request resolves (duplicate reservations).
  const [isSubmitting, setIsSubmitting] = useState(false);
  const [pendingApprovals, setPendingApprovals] = useState<Reservation[]>([]);
  const [pendingApprovalsTotal, setPendingApprovalsTotal] = useState(0);
  const [pendingApprovalsPage, setPendingApprovalsPage] = useState(1);
  const [pendingApprovalsLoadingMore, setPendingApprovalsLoadingMore] =
    useState(false);
  const [approvalGuestHistory, setApprovalGuestHistory] = useState<
    Record<string, { prior_no_shows: number; prior_visits: number }>
  >({});
  const [declineTarget, setDeclineTarget] = useState<Reservation | null>(null);
  const [declineReason, setDeclineReason] = useState("");
  const pagedReservationsRequestIdRef = useRef(0);
  const completeReservationsRequestIdRef = useRef(0);
  const pendingApprovalsRequestIdRef = useRef(0);

  const [isCreateOpen, setIsCreateOpen] = useState(false);
  const [isEditOpen, setIsEditOpen] = useState(false);
  const [isViewOpen, setIsViewOpen] = useState(false);
  const [isCancelOpen, setIsCancelOpen] = useState(false);
  const [selectedReservation, setSelectedReservation] =
    useState<Reservation | null>(null);
  const [timeEntryMode, setTimeEntryMode] =
    useState<ReservationTimeEntryMode>("business");
  const [tableOptions, setTableOptions] = useState<ReservationTableOption[]>([]);
  const [occupancyOverrideAcknowledged, setOccupancyOverrideAcknowledged] =
    useState(false);
  const [tableOptionsRefreshVersion, setTableOptionsRefreshVersion] = useState(0);
  const tableOptionsRequestIdRef = useRef(0);

  const [formData, setFormData] = useState<CreateReservationRequest>({
    table_id: undefined,
    customer_name: "",
    customer_phone: "",
    customer_email: "",
    party_size: 2,
    reservation_time: "",
    duration: 120,
    special_requests: "",
    notes: "",
  });

  // L1-15: baseline-diff dirty (not "any field non-empty"). Create seeds
  // reservation_time via firstBookableSlot(); edit always has name/time —
  // those must not trip the guard until the operator edits.
  const {
    dirty: formDirtyFromBaseline,
    markClean: markReservationFormClean,
    clearBaseline: clearReservationFormBaseline,
  } = useDirtyForm(formData);
  const reservationFormDirty =
    (isCreateOpen || isEditOpen) && formDirtyFromBaseline;
  useUnsavedChangesGuard(reservationFormDirty, "reservation-manager-form");

  useEffect(() => {
    setCurrentLocale(locale);
  }, [locale]);

  const t = useCallback(
    (key: string, params?: Record<string, string | number>): string => {
      const fullKey = `businessDashboard.reservations.${key}`;
      const result = getTranslation(fullKey, currentLocale, params);
      return Array.isArray(result) ? result[0] || key : (result as string);
    },
    [currentLocale],
  );

  // English defaults for keys still pending the businessDashboard.json pass
  // (I18N-NEEDED). getTranslation returns the raw dotted key on a miss; detect
  // that and fall back so no key path leaks into the UI. Placeholders are
  // preserved so the caller's .replace() still works.
  const tWithFallback = useCallback(
    (key: string): string => {
      const value = t(key);
      if (!value.startsWith("businessDashboard.reservations.")) {
        return value;
      }
      switch (key) {
        case "totalCount":
          return "{count} reservations";
        case "timesShownInTimezone":
          return "Times shown in {tz}";
        case "timezoneEntryHint":
          return "Enter time in your device timezone ({tz})";
        case "validation.outsideOperatingWindow":
          return "That start time cannot finish before closing (duration and service buffer).";
        case "validation.timeOutsideHours":
          return "Selected time is outside operating hours.";
        default:
          return value;
      }
    },
    [t],
  );

  /** L4-8: claim before mutate; toast and abort when another operator holds. */
  const claimReservationForMutate = useCallback(
    async (reservationId: number): Promise<boolean> => {
      const result = await ensureReservationClaim({
        claim: (options) =>
          reservationAPI.claimReservation(businessId, reservationId, options),
        canSteal: canStealReservationClaim,
        confirmSteal: (name) =>
          new Promise<boolean>((resolve) => {
            setStealPrompt((prev) => {
              prev?.resolve(false);
              return { name, resolve };
            });
          }),
      });
      if (result.ok) return true;
      if (result.reason === "user_cancelled") return false;
      const conflict = result.conflict;
      if (conflict?.claimed_by_name) {
        toast.error(
          conflict.code === "claim_steal_required"
            ? t("claim.stealRequired", { name: conflict.claimed_by_name })
            : t("claim.conflict", { name: conflict.claimed_by_name }),
        );
      } else {
        toast.error(t("claim.needClaim"));
      }
      return false;
    },
    [businessId, canStealReservationClaim, t],
  );

  /**
   * H1: release after the mutate so front-line operators are not locked out
   * for the full 5m idle TTL. Best-effort — release errors never mask mutate
   * success.
   */
  const releaseReservationClaim = useCallback(
    async (reservationId: number) => {
      try {
        // selfOnly: never force-release if someone else stole mid-mutate.
        await reservationAPI.releaseReservation(businessId, reservationId, {
          selfOnly: true,
        });
      } catch {
        // Leave idle TTL as a backstop.
      }
    },
    [businessId],
  );

  /** Claim → run mutate → always release (short exclusive window). */
  const withReservationClaim = useCallback(
    async (
      reservationId: number,
      mutate: () => Promise<void>,
    ): Promise<boolean> => {
      if (!(await claimReservationForMutate(reservationId))) return false;
      try {
        await mutate();
        return true;
      } finally {
        await releaseReservationClaim(reservationId);
      }
    },
    [claimReservationForMutate, releaseReservationClaim],
  );

  const toastReservationClaimConflict = useCallback(
    (error: unknown, fallbackKey: string) => {
      const conflict = getReservationClaimConflict(error);
      if (conflict) {
        if (conflict.code === "claim_steal_required" && conflict.claimed_by_name) {
          toast.error(t("claim.stealRequired", { name: conflict.claimed_by_name }));
        } else if (conflict.claimed_by_name) {
          toast.error(t("claim.conflict", { name: conflict.claimed_by_name }));
        } else {
          toast.error(t("claim.needClaim"));
        }
        return true;
      }
      toast.error(reservationErrorMessage(error, t(fallbackKey), currentLocale));
      return false;
    },
    [currentLocale, t],
  );

  const noShowGraceMinutes =
    typeof reservationSettings?.no_show_grace_minutes === "number"
      ? reservationSettings.no_show_grace_minutes
      : 15;

  const displayStatusFor = useCallback(
    (reservation: { reservation_time: string; status: string }) =>
      reservationDisplayStatus(
        reservation,
        new Date(),
        noShowGraceMinutes,
        businessTimezone,
      ),
    [businessTimezone, noShowGraceMinutes],
  );

  const getStatusLabel = useCallback(
    (status: ReservationStatus | string) => {
      const translated = t(`status.${status}`);
      if (translated !== `businessDashboard.reservations.status.${status}`) {
        return translated;
      }

      switch (status) {
        case "waitlist":
          return "Waitlist";
        case "no_show":
          return "No Show";
        case "late":
          return "Late";
        default:
          return status.charAt(0).toUpperCase() + status.slice(1);
      }
    },
    [t],
  );

  const handleAttentionFilterChange = useCallback(
    (value: ReservationAttentionFilter) => {
      setReservationPage(1);
      setAttentionFilter(value);
    },
    [],
  );

  const handleSearchChange = useCallback((value: string) => {
    setReservationPage(1);
    setSearch(value);
  }, []);

  const handleDateFilterChange = useCallback(
    // `setDateFilter` is the normalizing wrapper around the raw state setter,
    // not the setter itself, so it has to be declared — it is memoized with an
    // empty dep list, so this stays referentially stable.
    (event: React.ChangeEvent<HTMLSelectElement>) => {
      setReservationPage(1);
      setDateFilter(event.target.value);
    },
    [setDateFilter],
  );

  const handleStatusFilterChange = useCallback(
    (event: React.ChangeEvent<HTMLSelectElement>) => {
      setReservationPage(1);
      setStatusFilter(event.target.value);
    },
    [],
  );

  // Client-only for derived attention filters. Text search is server-side via
  // the optional `q` param (debounced below). Server-mappable filters
  // (today / waitlist / date / status / q) use the paged list path.
  const clientOnlyFiltersActive = isClientOnlyReservationFiltersActive(
    search,
    attentionFilter,
    { searchIsServerSide: true },
  );

  // Debounced search drives the server `q` param (~300ms trailing) so each
  // keystroke does not fire a list request. Input state updates immediately.
  const [debouncedSearch, setDebouncedSearch] = useState("");
  useEffect(() => {
    const handle = window.setTimeout(() => {
      setDebouncedSearch(search.trim());
    }, 300);
    return () => window.clearTimeout(handle);
  }, [search]);

  const serverReservationFilters = useMemo(
    () =>
      resolveServerReservationFilters(
        attentionFilter,
        dateFilter,
        statusFilter,
        new Date(),
        reservationSettings?.max_advance_days,
        { startDate: customStartDate, endDate: customEndDate },
        businessTimezone,
      ),
    [
      attentionFilter,
      businessTimezone,
      customEndDate,
      customStartDate,
      dateFilter,
      reservationSettings?.max_advance_days,
      statusFilter,
    ],
  );

  // Date range for loaders: server-resolved window (today quick filter forces
  // local today; otherwise dateFilter presets). Derived client filters still
  // bound to this window so attention chips never scan unbounded history.
  const reservationDateRange = useMemo(
    () => ({
      startDate: serverReservationFilters.startDate,
      endDate: serverReservationFilters.endDate,
    }),
    [serverReservationFilters.endDate, serverReservationFilters.startDate],
  );

  const effectiveStatusFilter = serverReservationFilters.status;

  const loadCompleteReservations = useCallback(async () => {
    const requestId = completeReservationsRequestIdRef.current + 1;
    completeReservationsRequestIdRef.current = requestId;

    try {
      setLoading(true);
      const completeResponse = await getAllUpcomingReservations(businessId, {
        startDate: reservationDateRange.startDate,
        endDate: reservationDateRange.endDate,
        status: effectiveStatusFilter,
        q: debouncedSearch || undefined,
      });

      if (requestId !== completeReservationsRequestIdRef.current) {
        return;
      }

      setCompleteReservations(completeResponse.items);
      setReservationDataCapped(completeResponse.capped);
      if (completeResponse.capped) {
        console.warn(
          "Reservation data cap reached while loading reservations.",
          {
            businessId,
            warning: completeResponse.warning,
          },
        );
      }
    } catch (error) {
      if (requestId === completeReservationsRequestIdRef.current) {
        console.error("Failed to load complete reservations:", error);
      }
    } finally {
      if (requestId === completeReservationsRequestIdRef.current) {
        setLoading(false);
        setHasLoadedReservations(true);
        setHasLoadedComplete(true);
      }
    }
  }, [
    businessId,
    debouncedSearch,
    effectiveStatusFilter,
    reservationDateRange.endDate,
    reservationDateRange.startDate,
  ]);

  const loadUpcomingKpiReservations = useCallback(async () => {
    const requestId = upcomingKpiRequestIdRef.current + 1;
    upcomingKpiRequestIdRef.current = requestId;
    const loadUpcoming = reservationAPI.getUpcomingReservations;
    if (typeof loadUpcoming !== "function") {
      return;
    }
    try {
      const response = await loadUpcoming(businessId, 50);
      if (requestId !== upcomingKpiRequestIdRef.current) {
        return;
      }
      setUpcomingKpiReservations(response.reservations ?? []);
    } catch (error) {
      if (requestId === upcomingKpiRequestIdRef.current) {
        console.error(
          "Failed to load upcoming reservations for next-arrival KPI:",
          error,
        );
      }
    }
  }, [businessId]);

  // Status-independent today stats for insight KPIs (covers / waitlist / pending).
  // Keyed on the business calendar day so Overview and the list share a window.
  const loadTodayStats = useCallback(async () => {
    const requestId = todayStatsRequestIdRef.current + 1;
    todayStatsRequestIdRef.current = requestId;
    const dayKey =
      businessDateKey(new Date(), businessTimezone) || localDateKey(new Date());
    try {
      const stats = await reservationAPI.getStats(businessId, dayKey, dayKey);
      if (requestId !== todayStatsRequestIdRef.current) {
        return;
      }
      setTodayStats(stats);
    } catch (error) {
      if (requestId === todayStatsRequestIdRef.current) {
        console.error("Failed to load today reservation stats:", error);
      }
    }
  }, [businessId, businessTimezone]);

  const loadPagedReservations = useCallback(async () => {
    const requestId = pagedReservationsRequestIdRef.current + 1;
    pagedReservationsRequestIdRef.current = requestId;

    try {
      setLoading(true);
      const response = await reservationAPI.getReservations(
        businessId,
        reservationDateRange.startDate,
        reservationDateRange.endDate,
        effectiveStatusFilter,
        {
          page: reservationPage,
          pageSize: reservationPageSize,
          q: debouncedSearch || undefined,
        },
      );

      if (requestId !== pagedReservationsRequestIdRef.current) {
        return;
      }

      const responseReservations = response.reservations || [];
      const responseTotal = response.total || 0;
      const responseTotalPages = response.total_pages || 1;
      if (
        responseTotal > 0 &&
        responseReservations.length === 0 &&
        reservationPage > responseTotalPages
      ) {
        setReservationPage(responseTotalPages);
        return;
      }

      setPagedReservations(responseReservations);
      setReservationTotal(responseTotal);
      setReservationTotalPages(responseTotalPages);
    } catch (error) {
      if (requestId === pagedReservationsRequestIdRef.current) {
        console.error("Failed to load paged reservations:", error);
      }
    } finally {
      if (requestId === pagedReservationsRequestIdRef.current) {
        setLoading(false);
        setHasLoadedReservations(true);
        setHasLoadedPaged(true);
      }
    }
  }, [
    businessId,
    debouncedSearch,
    effectiveStatusFilter,
    reservationDateRange.endDate,
    reservationDateRange.startDate,
    reservationPage,
    reservationPageSize,
  ]);

  // Pending approval requests load with NO date range on purpose: the strip
  // must surface every request awaiting review (even far-future dates) no
  // matter which list filters are active, or a request can silently expire.
  const loadPendingApprovals = useCallback(
    async (options: { page?: number; append?: boolean } = {}) => {
      const page = options.page ?? 1;
      const append = options.append === true;
      const requestId = pendingApprovalsRequestIdRef.current + 1;
      pendingApprovalsRequestIdRef.current = requestId;

      try {
        if (append) setPendingApprovalsLoadingMore(true);
        const response = await reservationAPI.getReservations(
          businessId,
          undefined,
          undefined,
          "pending",
          { page, pageSize: 100 },
        );

        if (requestId !== pendingApprovalsRequestIdRef.current) {
          return;
        }

        const pending = (response.reservations || []).filter(
          (reservation) => reservation.status === "pending",
        );
        const total =
          typeof response.total === "number" ? response.total : pending.length;
        setPendingApprovalsTotal(total);
        setPendingApprovalsPage(page);
        setPendingApprovals((current) => {
          if (!append) return pending;
          const seen = new Set(current.map((row) => row.id));
          return [
            ...current,
            ...pending.filter((row) => !seen.has(row.id)),
          ];
        });
        setApprovalGuestHistory((current) =>
          append
            ? { ...current, ...(response.guest_history ?? {}) }
            : (response.guest_history ?? {}),
        );
      } catch (error) {
        if (requestId === pendingApprovalsRequestIdRef.current) {
          console.error("Failed to load pending approval requests:", error);
        }
      } finally {
        if (append) setPendingApprovalsLoadingMore(false);
      }
    },
    [businessId],
  );

  const loadTables = useCallback(async () => {
    try {
      const [tablesResponse, settingsResponse, hoursResponse] =
        await Promise.all([
          businessApi.getBusinessTables(businessId),
          reservationAPI.getSettings(businessId),
          getBusinessOperatingHours(businessId).catch(() => []),
        ]);
      setTables(
        (tablesResponse.tables || []).filter((table) => table.is_active),
      );
      setReservationSettings(settingsResponse);
      setOperatingHours(Array.isArray(hoursResponse) ? hoursResponse : []);
    } catch (error) {
      console.error("Failed to load reservation support data:", error);
    }
  }, [businessId]);

  useEffect(() => {
    if (!accessLoading && hasAccess && reservationsEnabled) {
      loadTables().catch((err) => console.error("loadTables failed:", err));
    }
  }, [
    reservationsEnabled,
    loadTables,
    accessLoading,
    hasAccess,
    reservationsLoading,
  ]);

  const canLoadReservations = !accessLoading && hasAccess && reservationsEnabled;

  const needsCompleteReservations = shouldLoadCompleteReservations(
    clientOnlyFiltersActive,
    reservationView,
  );
  const needsPagedReservations = shouldLoadPagedReservations(
    clientOnlyFiltersActive,
    reservationView,
  );
  // The visible surface reads the complete hydrate exactly when it is needed
  // (board view or derived client filters); otherwise the paged rows. The
  // skeleton must key on THIS source's first load, not on "any data loaded".
  const activeSourceLoaded = needsCompleteReservations
    ? hasLoadedComplete
    : hasLoadedPaged;

  useEffect(() => {
    if (canLoadReservations && needsCompleteReservations) {
      loadCompleteReservations().catch((err) =>
        console.error("loadCompleteReservations failed:", err),
      );
    }
  }, [canLoadReservations, loadCompleteReservations, needsCompleteReservations]);

  useEffect(() => {
    if (canLoadReservations) {
      loadTodayStats().catch((err) =>
        console.error("loadTodayStats failed:", err),
      );
      loadUpcomingKpiReservations().catch((err) =>
        console.error("loadUpcomingKpiReservations failed:", err),
      );
    }
  }, [canLoadReservations, loadTodayStats, loadUpcomingKpiReservations]);

  // Load the business timezone once so reservation times render in the business
  // day. A missing/failed/invalid value falls back to UTC, never the operator's
  // device timezone, so the same record is auditable from every workstation.
  useEffect(() => {
    if (accessLoading || !hasAccess) return;
    let cancelled = false;
    setBusinessTimezone(null);
    getBusiness(businessId)
      .then((business) => {
        if (!cancelled) {
          setBusinessTimezone(business?.timezone || "UTC");
        }
      })
      .catch((err) => console.error("Failed to load business timezone:", err));
    return () => {
      cancelled = true;
    };
  }, [businessId, hasAccess, accessLoading]);

  useEffect(() => {
    if (canLoadReservations && reservationCaps.canEdit) {
      loadPendingApprovals().catch((err) =>
        console.error("loadPendingApprovals failed:", err),
      );
    }
  }, [canLoadReservations, loadPendingApprovals, reservationCaps.canEdit]);

  useEffect(() => {
    if (canLoadReservations && needsPagedReservations) {
      loadPagedReservations().catch((err) =>
        console.error("loadPagedReservations failed:", err),
      );
    }
  }, [canLoadReservations, loadPagedReservations, needsPagedReservations]);

  const refreshReservationState = useCallback(
    async (reservationId?: number) => {
      const loaders: Promise<unknown>[] = [];
      if (needsCompleteReservations) {
        loaders.push(loadCompleteReservations());
      }
      loaders.push(loadTodayStats());
      loaders.push(loadUpcomingKpiReservations());
      if (reservationCaps.canEdit) {
        loaders.push(loadPendingApprovals());
      }
      if (needsPagedReservations) {
        loaders.push(loadPagedReservations());
      }
      await Promise.all(loaders);
      if (reservationId && isViewOpen) {
        try {
          const detailedReservation = await reservationAPI.getReservation(
            businessId,
            reservationId,
          );
          setSelectedReservation(detailedReservation);
        } catch (error) {
          console.error("Failed to refresh selected reservation:", error);
        }
      }
    },
    [
      businessId,
      isViewOpen,
      loadCompleteReservations,
      loadTodayStats,
      loadUpcomingKpiReservations,
      loadPagedReservations,
      loadPendingApprovals,
      needsCompleteReservations,
      needsPagedReservations,
      reservationCaps.canEdit,
    ],
  );

  // Leading + trailing coalesce (DispatchConsole pattern): a dinner-rush burst
  // of reservation.new/updated must not fire one full multi-loader refresh per
  // event. Mutations share this path so the backend SSE echo of our own write
  // collapses into the same window instead of double-refreshing.
  const sseLastLoadAtRef = useRef(0);
  const sseTrailingTimerRef = useRef<ReturnType<typeof setTimeout> | null>(
    null,
  );
  const pendingDetailReservationIdRef = useRef<number | null>(null);

  useEffect(() => {
    return () => {
      if (sseTrailingTimerRef.current != null) {
        clearTimeout(sseTrailingTimerRef.current);
        sseTrailingTimerRef.current = null;
      }
    };
  }, []);

  const scheduleReservationRefresh = useCallback(
    (reservationId?: number) => {
      if (reservationId != null) {
        pendingDetailReservationIdRef.current = reservationId;
      }
      const decision = decideReservationRefreshCoalesce(
        Date.now(),
        sseLastLoadAtRef.current,
        sseTrailingTimerRef.current != null,
        RESERVATION_SSE_COALESCE_MS,
      );
      if (decision.action === "covered_by_pending_trailing") {
        return;
      }
      if (decision.action === "run_now") {
        sseLastLoadAtRef.current = Date.now();
        const detailId = pendingDetailReservationIdRef.current ?? undefined;
        pendingDetailReservationIdRef.current = null;
        void refreshReservationState(detailId);
        return;
      }
      sseTrailingTimerRef.current = setTimeout(() => {
        sseTrailingTimerRef.current = null;
        sseLastLoadAtRef.current = Date.now();
        const detailId = pendingDetailReservationIdRef.current ?? undefined;
        pendingDetailReservationIdRef.current = null;
        void refreshReservationState(detailId);
      }, decision.delayMs);
    },
    [refreshReservationState],
  );

  // R3-RS-2: the approval inbox + lists never refreshed on their own, yet a
  // pending request can auto-decline as soon as created+30min. Subscribe to the
  // shared business SSE stream (one EventSource fanned across the dashboard) and
  // refresh on reservation.new/reservation.updated so a new booking or an
  // expiring request appears without a manual reload. On a recovered drop, the
  // events fired during the outage were not buffered, so re-fetch wholesale.
  const handleReservationSSEEvent = useCallback(
    (event: SSEEvent) => {
      if (
        event.type === "reservation.new" ||
        event.type === "reservation.updated"
      ) {
        scheduleReservationRefresh();
      }
    },
    [scheduleReservationRefresh],
  );

  useSSEEvents({
    businessId,
    enabled: canLoadReservations,
    onEvent: handleReservationSSEEvent,
    onReconnect: () => {
      // Force an immediate refresh after reconnect (events during the outage
      // were not buffered server-side).
      sseLastLoadAtRef.current = 0;
      if (sseTrailingTimerRef.current != null) {
        clearTimeout(sseTrailingTimerRef.current);
        sseTrailingTimerRef.current = null;
      }
      scheduleReservationRefresh();
    },
  });

  // The details drawer is a hand-rolled slide-over (not a NextUI Modal), so it
  // needs its own dialog affordances. useDialogBehavior moves focus into the
  // panel on open, traps Tab within it, closes on Escape, and restores focus to
  // the trigger on close.
  const detailsDrawerRef = useRef<HTMLDivElement | null>(null);
  const detailsTriggerRef = useRef<HTMLElement | null>(null);
  useDialogBehavior({
    isOpen: isViewOpen,
    onClose: () => setIsViewOpen(false),
    containerRef: detailsDrawerRef,
    restoreFocusRef: detailsTriggerRef,
  });

  const handleApproveReservation = async (reservationId: number) => {
    if (!reservationCaps.canEdit) return;

    try {
      void operationalAlerts?.claimByResource(
        "reservation",
        reservationId,
        "reservation_approval_approve",
      );
      setActionLoadingId(reservationId);
      const ok = await withReservationClaim(reservationId, async () => {
        await reservationAPI.updateReservation(businessId, reservationId, {
          status: "confirmed",
        });
      });
      if (!ok) return;
      scheduleReservationRefresh(reservationId);
      if (onNavigateToTab) {
        showApprovedFollowUpToast({
          message: t("approvalQueue.approved"),
          actionLabel: t("approvalQueue.viewTables"),
          onAction: () => onNavigateToTab("tables"),
        });
      } else {
        toast.success(t("approvalQueue.approved"));
      }
    } catch (error) {
      console.error("Failed to approve reservation request:", error);
      toastReservationClaimConflict(error, "toasts.statusError");
    } finally {
      setActionLoadingId(null);
    }
  };

  const handleDeclineRequest = (reservationId: number) => {
    if (!reservationCaps.canEdit) return;

    const reservation = pendingApprovals.find(
      (candidate) => candidate.id === reservationId,
    );
    if (!reservation) return;

    void operationalAlerts?.claimByResource(
      "reservation",
      reservationId,
      "reservation_approval_decline_open",
    );
    setDeclineReason("");
    setDeclineTarget(reservation);
  };

  const handleDeclineReservation = async () => {
    if (!reservationCaps.canEdit) return;
    if (!declineTarget) return;

    try {
      setActionLoadingId(declineTarget.id);
      const ok = await withReservationClaim(declineTarget.id, async () => {
        await reservationAPI.updateReservation(businessId, declineTarget.id, {
          status: "cancelled",
          cancellation_reason: declineReason.trim() || undefined,
        });
      });
      if (!ok) return;
      setDeclineTarget(null);
      setDeclineReason("");
      scheduleReservationRefresh();
      toast.success(t("approvalQueue.declined"));
    } catch (error) {
      console.error("Failed to decline reservation request:", error);
      toastReservationClaimConflict(error, "toasts.statusError");
    } finally {
      setActionLoadingId(null);
    }
  };

  const getRecommendedTables = useCallback(
    (partySize: number) =>
      [...tables]
        .filter((table) => table.capacity >= partySize)
        .sort((left, right) => left.capacity - right.capacity)
        .slice(0, 4),
    [tables],
  );

  const handleCreateReservation = async () => {
    if (!reservationCaps.canCreate) return;
    if (isSubmitting) return;

    // Client-side required fields — never round-trip empty creates that return
    // gin binding dumps (FIND-030).
    if (!formData.customer_name?.trim()) {
      toast.error(t("validation.customerNameRequired"));
      return;
    }
    // L1-16: create path — required + format (shared with edit).
    {
      const phoneGate = reservationPhoneGate(formData.customer_phone);
      if (phoneGate === "missing") {
        toast.error(t("validation.customerPhoneRequired"));
        return;
      }
      if (phoneGate === "invalid") {
        toast.error(t("validation.customerPhoneInvalid"));
        return;
      }
    }

    // L1-17: refuse starts that cannot finish duration+buffer before close
    // (same rule as backend reservationFitsOperatingWindow).
    {
      const wall = formData.reservation_time || "";
      const datePart = wall.slice(0, 10);
      const hhmm = wall.slice(11, 16);
      const resolve = operatingWindowResolver(operatingHours);
      const win = resolve?.(datePart) ?? null;
      const duration =
        formData.duration ||
        reservationSettings?.default_duration ||
        120;
      const buffer =
        reservationSettings?.service_buffer_minutes &&
        reservationSettings.service_buffer_minutes > 0
          ? reservationSettings.service_buffer_minutes
          : 0;
      if (
        win &&
        hhmm &&
        !reservationFitsClose(
          hhmm,
          win.closeHHMM,
          duration + buffer,
          win.openHHMM,
        )
      ) {
        toast.error(
          tWithFallback("validation.outsideOperatingWindow") ||
            tWithFallback("validation.timeOutsideHours"),
        );
        return;
      }
    }

    if (timeEntryMode === "business" && !businessTimezone) {
      toast.error(t("timeEntry.timezoneLoading"));
      return;
    }

    try {
      setIsSubmitting(true);
      const reservationData = {
        ...formData,
        table_id: formData.table_id || undefined,
        reservation_time: reservationWallTimeToISO(
          formData.reservation_time,
          entryTimezone,
        ),
        allow_occupied_table_override:
          occupancyOverrideAcknowledged || undefined,
      };

      const created = await reservationAPI.createReservation(
        businessId,
        reservationData,
      );
      setIsCreateOpen(false);
      resetForm();
      // A reservation for another day would be invisible under the default
      // "today" quick filter — the create would look like it silently failed.
      // L1-6: when the filter flips, loaders re-key via effects — skip the
      // concurrent scheduleReservationRefresh() so a stale pre-flip range
      // cannot race and overwrite the new window.
      const flipToUpcoming =
        !!created?.reservation_time &&
        shouldFlipToUpcomingAfterCreate({
          reservationTime: created.reservation_time,
          dateFilter,
        });
      if (flipToUpcoming) {
        setDateFilter("upcoming");
        setReservationPage(1);
      }
      if (shouldScheduleRefreshAfterCreate(flipToUpcoming)) {
        scheduleReservationRefresh();
      }
      toast.success(t("toasts.createSuccess"));
    } catch (error) {
      console.error("Failed to create reservation:", error);
      if (getApiErrorCode(error) === "reservation_table_occupied") {
        setTableOptionsRefreshVersion((value) => value + 1);
        setOccupancyOverrideAcknowledged(false);
        toast.error(t("occupancy.overrideRequired"));
      } else if (
        error instanceof Error &&
        error.message === "wall_time_does_not_exist"
      ) {
        toast.error(t("timeEntry.invalidGap"));
      } else {
        toast.error(reservationErrorMessage(error, t("toasts.createError"), currentLocale));
      }
    } finally {
      setIsSubmitting(false);
    }
  };

  const handleUpdateReservation = async () => {
    if (!reservationCaps.canEdit) return;
    if (!selectedReservation) return;
    if (isSubmitting) return;
    if (timeEntryMode === "business" && !businessTimezone) {
      toast.error(t("timeEntry.timezoneLoading"));
      return;
    }

    // L1-16: edit path must use the same phone gate as create.
    {
      const phoneGate = reservationPhoneGate(formData.customer_phone);
      if (phoneGate === "missing") {
        toast.error(t("validation.customerPhoneRequired"));
        return;
      }
      if (phoneGate === "invalid") {
        toast.error(t("validation.customerPhoneInvalid"));
        return;
      }
    }

    try {
      setIsSubmitting(true);
      // Send only what changed (supports clearing the table); resending every
      // field forced the backend to re-validate the booking window even for a
      // phone-number edit.
      const reservationData = buildReservationUpdatePayload(
        {
          ...formData,
          reservation_time: reservationWallTimeToISO(
            formData.reservation_time,
            entryTimezone,
          ),
        },
        selectedReservation,
      );
      if (occupancyOverrideAcknowledged) {
        reservationData.allow_occupied_table_override = true;
      }

      if (Object.keys(reservationData).length > 0) {
        const ok = await withReservationClaim(
          selectedReservation.id,
          async () => {
            await reservationAPI.updateReservation(
              businessId,
              selectedReservation.id,
              reservationData,
            );
          },
        );
        if (!ok) return;
      }
      setIsEditOpen(false);
      scheduleReservationRefresh(selectedReservation.id);
      toast.success(t("toasts.updateSuccess"));
    } catch (error) {
      console.error("Failed to update reservation:", error);
      if (getApiErrorCode(error) === "reservation_table_occupied") {
        setTableOptionsRefreshVersion((value) => value + 1);
        setOccupancyOverrideAcknowledged(false);
        toast.error(t("occupancy.overrideRequired"));
      } else if (
        error instanceof Error &&
        error.message === "wall_time_does_not_exist"
      ) {
        toast.error(t("timeEntry.invalidGap"));
      } else if (!getReservationClaimConflict(error)) {
        toast.error(reservationErrorMessage(error, t("toasts.updateError"), currentLocale));
      } else {
        toastReservationClaimConflict(error, "toasts.updateError");
      }
    } finally {
      setIsSubmitting(false);
    }
  };

  const handleUpdateStatus = async (
    reservationId: number,
    newStatus: ReservationStatus,
  ) => {
    if (!reservationCaps.canEdit) return;

    try {
      void operationalAlerts?.claimByResource(
        "reservation",
        reservationId,
        "reservation_status_change",
      );
      setActionLoadingId(reservationId);
      const ok = await withReservationClaim(reservationId, async () => {
        if (newStatus === "seated") {
          await reservationAPI.checkIn(businessId, reservationId);
        } else if (newStatus === "no_show") {
          await reservationAPI.markNoShow(businessId, reservationId, {
            reason: "token:no_show_manager",
          });
        } else {
          await reservationAPI.updateReservation(businessId, reservationId, {
            status: newStatus,
          });
        }
      });
      if (!ok) return;

      scheduleReservationRefresh(reservationId);
      toast.success(t("toasts.statusUpdated"));
    } catch (error) {
      console.error("Failed to update status:", error);
      toastReservationClaimConflict(error, "toasts.statusError");
    } finally {
      setActionLoadingId(null);
    }
  };

  const handlePromoteWaitlist = async (reservationId: number) => {
    if (!reservationCaps.canEdit) return;

    try {
      void operationalAlerts?.claimByResource(
        "reservation",
        reservationId,
        "reservation_waitlist_promote",
      );
      setActionLoadingId(reservationId);
      const ok = await withReservationClaim(reservationId, async () => {
        await reservationAPI.promoteWaitlist(businessId, reservationId, {
          notes: "token:promoted_from_manager",
        });
      });
      if (!ok) return;
      scheduleReservationRefresh(reservationId);
      toast.success(t("toasts.waitlistPromoted"));
    } catch (error) {
      console.error("Failed to promote waitlist reservation:", error);
      toastReservationClaimConflict(error, "toasts.waitlistError");
    } finally {
      setActionLoadingId(null);
    }
  };

  const handleAssignTable = async (reservationId: number, tableId: number) => {
    if (!reservationCaps.canEdit) return;

    try {
      void operationalAlerts?.claimByResource(
        "reservation",
        reservationId,
        "reservation_table_assign",
      );
      setActionLoadingId(reservationId);
      const ok = await withReservationClaim(reservationId, async () => {
        await reservationAPI.assignTable(businessId, reservationId, {
          table_id: tableId,
        });
      });
      if (!ok) return;
      scheduleReservationRefresh(reservationId);
      toast.success(t("toasts.tableAssigned"));
    } catch (error) {
      console.error("Failed to assign table:", error);
      toastReservationClaimConflict(error, "toasts.tableError");
    } finally {
      setActionLoadingId(null);
    }
  };

  const handleCancelReservation = (reservation: Reservation) => {
    if (!reservationCaps.canDelete) return;

    void operationalAlerts?.claimByResource(
      "reservation",
      reservation.id,
      "reservation_cancel_open",
    );
    setSelectedReservation(reservation);
    setIsViewOpen(false);
    setIsEditOpen(false);
    setIsCancelOpen(true);
  };

  const confirmCancelReservation = async () => {
    if (!reservationCaps.canDelete) return;
    if (!selectedReservation) return;

    try {
      setActionLoadingId(selectedReservation.id);
      const ok = await withReservationClaim(selectedReservation.id, async () => {
        await reservationAPI.cancelReservation(
          businessId,
          selectedReservation.id,
        );
      });
      if (!ok) return;
      setIsCancelOpen(false);
      setIsViewOpen(false);
      setSelectedReservation(null);
      scheduleReservationRefresh();
      toast.success(t("toasts.cancelSuccess"));
    } catch (error) {
      console.error("Failed to cancel reservation:", error);
      toastReservationClaimConflict(error, "toasts.cancelError");
    } finally {
      setActionLoadingId(null);
    }
  };

  const handleViewDetails = async (reservation: Reservation) => {
    // Capture the trigger before the detail request. NextUI's table may move
    // focus while this async request is pending, so waiting until the drawer
    // mounts can leave useDialogBehavior with document.body as its restore
    // target instead of the button the keyboard user activated.
    if (typeof window !== "undefined") {
      boardScrollYRef.current = window.scrollY;
    }
    detailsTriggerRef.current =
      document.activeElement instanceof HTMLElement
        ? document.activeElement
        : null;
    void operationalAlerts?.claimByResource(
      "reservation",
      reservation.id,
      "reservation_details_open",
    );
    setIsEditOpen(false);
    setIsCancelOpen(false);
    try {
      const detailedReservation = await reservationAPI.getReservation(
        businessId,
        reservation.id,
      );
      setSelectedReservation(detailedReservation);
    } catch (error) {
      console.error("Failed to load reservation details:", error);
      setSelectedReservation(reservation);
    }
    setIsViewOpen(true);
  };

  const handleReservationRowAction = (
    reservation: Reservation,
    key: string,
  ) => {
    switch (key) {
      case "edit":
        handleEditReservation(reservation);
        return;
      case "confirm":
        void handleUpdateStatus(reservation.id, "confirmed");
        return;
      case "promote":
        void handlePromoteWaitlist(reservation.id);
        return;
      case "seat":
        void handleUpdateStatus(reservation.id, "seated");
        return;
      case "no-show":
        void handleUpdateStatus(reservation.id, "no_show");
        return;
      case "complete":
        void handleUpdateStatus(reservation.id, "completed");
        return;
      case "cancel":
        handleCancelReservation(reservation);
        return;
      default:
        return;
    }
  };

  const handleEditReservation = (reservation: Reservation) => {
    if (!reservationCaps.canEdit) return;

    void operationalAlerts?.claimByResource(
      "reservation",
      reservation.id,
      "reservation_edit_open",
    );
    setIsViewOpen(false);
    setIsCancelOpen(false);
    setSelectedReservation(reservation);
    setTimeEntryMode("business");
    setOccupancyOverrideAcknowledged(false);
    const next: CreateReservationRequest = {
      table_id: reservation.table_id,
      customer_name: reservation.customer_name,
      customer_phone: reservation.customer_phone || "",
      customer_email: reservation.customer_email || "",
      party_size: reservation.party_size,
      reservation_time: instantToWallTime(
        reservation.reservation_time,
        resolveBusinessTimeZone(businessTimezone),
      ),
      duration: reservation.duration,
      special_requests: reservation.special_requests || "",
      notes: reservation.notes || "",
    };
    setFormData(next);
    // Pristine edit open must not look dirty (name/time already populated).
    markReservationFormClean(next);
    setIsEditOpen(true);
  };

  // First slot the backend will actually accept: now + min advance, rounded
  // up to the slot interval. Defaulting to plain "now" meant every untouched
  // create form was rejected by the min-advance validation.
  const firstBookableSlot = useCallback(() => {
    const tz = resolveBusinessTimeZone(businessTimezone);
    // R2-11: resolve hours per candidate DAY. The old shape found the next
    // open day's window but handed it over as a single daily window, so
    // nextBookableWallTime applied tomorrow's hours to today's date — on a
    // closed day the backend rejected the untouched form with
    // outside_operating_window (L1-17).
    const duration =
      reservationSettings?.default_duration && reservationSettings.default_duration > 0
        ? reservationSettings.default_duration
        : 120;
    const buffer =
      reservationSettings?.service_buffer_minutes &&
      reservationSettings.service_buffer_minutes > 0
        ? reservationSettings.service_buffer_minutes
        : 0;
    return nextBookableWallTime(
      new Date(),
      reservationSettings?.min_advance_minutes ?? 30,
      reservationSettings?.slot_interval_minutes ?? 30,
      tz,
      operatingWindowResolver(operatingHours),
      { serviceMinutes: duration + buffer },
    );
  }, [businessTimezone, operatingHours, reservationSettings]);

  const resetForm = () => {
    setTimeEntryMode("business");
    setOccupancyOverrideAcknowledged(false);
    setTableOptions([]);
    const next: CreateReservationRequest = {
      table_id: undefined,
      customer_name: "",
      customer_phone: "",
      customer_email: "",
      party_size: clampPartySize(
        2,
        reservationSettings?.min_party_size,
        reservationSettings?.max_party_size,
      ),
      reservation_time: firstBookableSlot(),
      duration: reservationSettings?.default_duration || 120,
      special_requests: "",
      notes: "",
    };
    setFormData(next);
    // Pristine create open must not look dirty (seeded reservation_time).
    markReservationFormClean(next);
  };

  // NOTE: Round 4 audit (Task 5) removed the legacy `getStatusColor` helper
  // that mapped ReservationStatus -> NextUI Chip color. Status rendering now
  // flows through the shared StatusChip primitive, which owns tone selection.

  const formatDateTime = useCallback(
    (dateString: string) =>
      formatBusinessDateTime(dateString, currentLocale, businessTimezone, DATE_TIME_SHORT),
    [currentLocale, businessTimezone],
  );

  const formatTimeOnly = useCallback(
    (dateString: string) =>
      formatBusinessTime(dateString, currentLocale, businessTimezone, TIME_SHORT),
    [currentLocale, businessTimezone],
  );

  // Only surface the timezone caveat when the business timezone actually
  // differs from the operator's device — otherwise it's noise. Times are
  // DISPLAYED in the business timezone, but the native datetime-local picker
  // still enters values in the device timezone (full wall-clock reinterpretation
  // needs a tz library we don't ship), so we tell the operator which clock the
  // picker uses to avoid an off-by-offset booking. (R3-RS operator TZ)
  const deviceTimezone =
    typeof Intl !== "undefined"
      ? Intl.DateTimeFormat().resolvedOptions().timeZone
      : "UTC";
  const resolvedBusinessTimezone = resolveBusinessTimeZone(businessTimezone);
  const entryTimezone =
    timeEntryMode === "business"
      ? resolvedBusinessTimezone
      : resolveBusinessTimeZone(deviceTimezone);

  useEffect(() => {
    if ((!isCreateOpen && !isEditOpen) || !formData.reservation_time) {
      setTableOptions([]);
      return;
    }
    let instant: Date;
    try {
      instant = wallTimeToInstant(formData.reservation_time, entryTimezone);
    } catch {
      setTableOptions([]);
      return;
    }
    const requestID = ++tableOptionsRequestIdRef.current;
    const timer = window.setTimeout(() => {
      void reservationAPI
        .getTableOptions(businessId, {
          reservationTime: instant.toISOString(),
          duration:
            formData.duration || reservationSettings?.default_duration || 120,
          partySize: formData.party_size,
          excludeReservationId: isEditOpen
            ? selectedReservation?.id
            : undefined,
        })
        .then((response) => {
          if (requestID === tableOptionsRequestIdRef.current) {
            setTableOptions(response.tables);
          }
        })
        .catch(() => {
          if (requestID === tableOptionsRequestIdRef.current) {
            setTableOptions([]);
          }
        });
    }, 250);
    return () => window.clearTimeout(timer);
  }, [
    businessId,
    entryTimezone,
    formData.duration,
    formData.party_size,
    formData.reservation_time,
    isCreateOpen,
    isEditOpen,
    reservationSettings?.default_duration,
    selectedReservation?.id,
    tableOptionsRefreshVersion,
  ]);

  // Client-side only for derived attention chips (next2hours / no_show_risk /
  // needs_table). Text search is server-side via `q` on the paged path; when
  // a derived filter forces complete hydrate, we still apply derived chips
  // only (no client re-search — server already filtered if we had q on
  // complete path; complete hydrate does not pass q, so for derived+search
  // we additionally filter by debouncedSearch over the bounded window).
  const applyClientReservationFilters = useCallback(
    (sourceReservations: Reservation[]) => {
      const searchLower = debouncedSearch.toLowerCase();
      const now = new Date();
      return sourceReservations.filter((reservation) => {
        const reservationDate = new Date(reservation.reservation_time);
        const minutesUntil =
          (reservationDate.getTime() - now.getTime()) / 60_000;

        const matchesAttention =
          attentionFilter === "none" ||
          (attentionFilter === "next2hours" &&
            minutesUntil >= 0 &&
            minutesUntil <= 120 &&
            ["pending", "confirmed", "waitlist"].includes(
              reservation.status,
            )) ||
          (attentionFilter === "needs_table" &&
            !reservation.table_id &&
            ["pending", "confirmed", "waitlist"].includes(
              reservation.status,
            )) ||
          (attentionFilter === "no_show_risk" &&
            ["pending", "confirmed"].includes(reservation.status) &&
            minutesUntil <= 15 &&
            minutesUntil >= -30);

        // Text filter when a derived chip forced the complete path (complete
        // hydrate already passes server `q` when set).
        const matchesSearch =
          searchLower === "" ||
          reservation.customer_name.toLowerCase().includes(searchLower) ||
          reservation.customer_phone?.toLowerCase().includes(searchLower) ||
          reservation.customer_email?.toLowerCase().includes(searchLower) ||
          reservation.table?.name?.toLowerCase().includes(searchLower);

        return matchesAttention && matchesSearch;
      });
    },
    [attentionFilter, debouncedSearch],
  );

  const filteredOperationsReservations = useMemo(
    () => applyClientReservationFilters(completeReservations),
    [applyClientReservationFilters, completeReservations],
  );

  const filteredListReservations = useMemo(
    () =>
      clientOnlyFiltersActive
        ? applyClientReservationFilters(completeReservations)
        : pagedReservations,
    [
      applyClientReservationFilters,
      clientOnlyFiltersActive,
      completeReservations,
      pagedReservations,
    ],
  );

  const visibleListReservations = useMemo(() => {
    if (!clientOnlyFiltersActive) {
      return filteredListReservations;
    }

    const startIndex = (reservationPage - 1) * reservationPageSize;
    return filteredListReservations.slice(
      startIndex,
      startIndex + reservationPageSize,
    );
  }, [
    clientOnlyFiltersActive,
    filteredListReservations,
    reservationPage,
    reservationPageSize,
  ]);

  const effectiveReservationTotal = clientOnlyFiltersActive
    ? filteredListReservations.length
    : reservationTotal;
  const effectiveReservationTotalPages = clientOnlyFiltersActive
    ? Math.max(
        1,
        Math.ceil(filteredListReservations.length / reservationPageSize),
      )
    : reservationTotalPages;
  const activeViewHasReservations =
    reservationView === "operations"
      ? filteredOperationsReservations.length > 0
      : visibleListReservations.length > 0;

  // L1-10: restaurant calendar day, not the operator device day.
  const todayKey =
    businessDateKey(new Date(), businessTimezone) || localDateKey(new Date());
  // Bookings today = active booking count (total − cancelled − no_show).
  // Covers today = SUM(party_size) when the backend sends `covers`.
  // Header must label both — bare "4 Today" looked like four bookings.
  const bookingsToday = todayStats
    ? Math.max(
        0,
        Number(todayStats.total) -
          Number(todayStats.cancelled) -
          Number(todayStats.no_show),
      )
    : null;
  const arrivalBooks = useMemo(
    () =>
      mergeReservationBooks(
        upcomingKpiReservations,
        visibleListReservations,
        filteredOperationsReservations,
        pagedReservations,
        completeReservations,
      ),
    [
      completeReservations,
      filteredOperationsReservations,
      pagedReservations,
      upcomingKpiReservations,
      visibleListReservations,
    ],
  );
  const coversFromListedToday = coversOnBusinessDay(
    arrivalBooks,
    todayKey,
    businessTimezone,
  );
  const coversToday = todayStats
    ? Math.max(
        Number(todayStats.covers ?? bookingsToday ?? 0),
        coversFromListedToday,
      )
    : hasLoadedReservations
      ? coversFromListedToday
      : null;
  const waitlistToday = todayStats ? Number(todayStats.waitlist) : null;

  // Exact needs-table count from the aggregate; fall back to the loaded rows
  // (page-bounded, best-effort) only when the backend predates `needs_table`.
  const needsTableFromRows = useMemo(() => {
    const source =
      completeReservations.length > 0
        ? completeReservations
        : pagedReservations;
    return source.filter(
      (reservation) =>
        (businessDateKey(reservation.reservation_time, businessTimezone) ||
          localDateKey(new Date(reservation.reservation_time))) === todayKey &&
        !reservation.table_id &&
        ["pending", "confirmed", "waitlist"].includes(reservation.status),
    ).length;
  }, [businessTimezone, completeReservations, pagedReservations, todayKey]);
  const needsTableToday =
    todayStats?.needs_table != null
      ? Number(todayStats.needs_table)
      : needsTableFromRows;

  // Next arrival is the soonest active booking on ANY loaded book. Rows the
  // host already sees on Próximas stay candidates even when the device TZ
  // clock would mark them late/expired. Covers today is at least the listed
  // venue-day party sum when stats come back 0.
  const nextArrival = useMemo(
    () =>
      nextUpcomingArrival(
        arrivalBooks,
        new Date(),
        noShowGraceMinutes,
        {
          businessTimeZone: businessTimezone,
          listedReservations: visibleListReservations,
        },
      ),
    [
      arrivalBooks,
      businessTimezone,
      noShowGraceMinutes,
      visibleListReservations,
    ],
  );

  const insightCards = useMemo(
    () => [
      {
        label: t("insights.coversToday"),
        value: coversToday ?? (hasLoadedReservations ? 0 : "—"),
      },
      {
        label: t("insights.waitlist"),
        value: waitlistToday ?? (hasLoadedReservations ? 0 : "—"),
      },
      {
        label: t("insights.needsTable"),
        value: needsTableToday,
      },
      {
        label: t("insights.nextArrival"),
        // Stable icon key — selecting by the localized label broke the clock
        // icon in Spanish (audit L6 #14).
        icon: "clock" as const,
        value: nextArrival
          ? formatNextArrivalInsight(
              nextArrival,
              formatDateTime(nextArrival.reservation_time),
            )
          : t("insights.none"),
      },
    ],
    [
      coversToday,
      formatDateTime,
      hasLoadedReservations,
      needsTableToday,
      nextArrival,
      t,
      waitlistToday,
    ],
  );

  const reservationBoard = useMemo(
    () => [
      {
        key: "pending",
        title: t("status.pending"),
        reservations: filteredOperationsReservations.filter(
          (reservation) => reservation.status === "pending",
        ),
      },
      {
        key: "confirmed",
        title: t("status.confirmed"),
        reservations: filteredOperationsReservations.filter(
          (reservation) => reservation.status === "confirmed",
        ),
      },
      {
        key: "waitlist",
        title: t("status.waitlist"),
        reservations: filteredOperationsReservations.filter(
          (reservation) => reservation.status === "waitlist",
        ),
      },
      {
        key: "seated",
        title: t("status.seated"),
        reservations: filteredOperationsReservations.filter(
          (reservation) => reservation.status === "seated",
        ),
      },
      {
        key: "completed",
        title: t("status.completed"),
        reservations: filteredOperationsReservations.filter(
          (reservation) => reservation.status === "completed",
        ),
      },
    ],
    [filteredOperationsReservations, t],
  );

  // Attention chips filter within the active date window (server-bounded).
  const attentionFilterOptions: Array<{
    key: ReservationAttentionFilter;
    label: string;
  }> = [
    { key: "none", label: t("attention.none") },
    { key: "next2hours", label: t("attention.next2hours") },
    { key: "needs_table", label: t("attention.needsTable") },
    { key: "no_show_risk", label: t("attention.noShowRisk") },
  ];

  const renderRecommendedTableButtons = (
    reservation: Pick<CreateReservationRequest, "party_size" | "table_id">,
    reservationId?: number,
  ) => {
    const recommendedTables = getRecommendedTables(reservation.party_size);
    if (reservationId && !reservationCaps.canEdit) {
      return null;
    }

    if (recommendedTables.length === 0) {
      return null;
    }

    return (
      <div className="space-y-2">
        <p className={reservationSectionEyebrowClass}>
          {t("recommendedTables")}
        </p>
        <div className="flex flex-wrap gap-2">
          {recommendedTables.map((table) => {
            const selected = reservation.table_id === table.id;
            return (
              <Button
                key={table.id}
                size="sm"
                variant={selected ? "solid" : "flat"}
                className={
                  selected
                    ? "bg-brand text-white"
                    : "border border-warm-200 bg-white/85 text-ink-700 hover:bg-brand/5 hover:text-brand-700"
                }
                onPress={() => {
                  if (reservationId) {
                    void handleAssignTable(reservationId, table.id);
                    return;
                  }
                  setFormData((current) => ({
                    ...current,
                    table_id: table.id,
                  }));
                }}
                isLoading={actionLoadingId === reservationId}
              >
                {table.name} ({table.capacity})
              </Button>
            );
          })}
        </div>
      </div>
    );
  };

  const renderQuickActionButtons = (reservation: Reservation) => {
    const actions: React.ReactNode[] = [];

    // All quick-action buttons drive `handleUpdateStatus` / `handlePromoteWaitlist`,
    // which require reservations:write. Server lacks that perm — render nothing.
    if (!reservationCaps.canEdit) {
      return actions;
    }

    if (reservation.status === "pending") {
      actions.push(
        <Button
          key="confirm"
          size="sm"
          variant="flat"
          color="success"
          onPress={() => handleUpdateStatus(reservation.id, "confirmed")}
          isLoading={actionLoadingId === reservation.id}
        >
          {t("confirm")}
        </Button>,
      );
    }

    if (reservation.status === "waitlist") {
      actions.push(
        <Button
          key="promote"
          size="sm"
          variant="flat"
          color="secondary"
          onPress={() => handlePromoteWaitlist(reservation.id)}
          isLoading={actionLoadingId === reservation.id}
        >
          {t("promote")}
        </Button>,
      );
    }

    // L1-13: seat / no-show require both status and temporal window — future
    // rows stay confirmable/promotable but cannot be marked arrived or no-show.
    // R2-B5: seating gets a 30-minute early-arrival grace; no-show does not.
    const seatOk = canSeatNow(reservation);

    if (reservation.status === "confirmed" && seatOk) {
      actions.push(
        <Button
          key="seat"
          size="sm"
          variant="flat"
          color="primary"
          onPress={() => handleUpdateStatus(reservation.id, "seated")}
          isLoading={actionLoadingId === reservation.id}
        >
          {t("seat")}
        </Button>,
      );
    }

    if (reservation.status === "seated") {
      actions.push(
        <Button
          key="complete"
          size="sm"
          variant="flat"
          color="success"
          onPress={() => handleUpdateStatus(reservation.id, "completed")}
          isLoading={actionLoadingId === reservation.id}
        >
          {t("complete")}
        </Button>,
      );
    }

    if (canMarkNoShowNow(reservation)) {
      actions.push(
        // No-show is a status mark, not a destructive action — use amber so
        // it doesn't read like a delete button to a new host.
        <Button
          key="no-show"
          size="sm"
          variant="flat"
          color="warning"
          onPress={() => handleUpdateStatus(reservation.id, "no_show")}
          isLoading={actionLoadingId === reservation.id}
        >
          {t("noShow")}
        </Button>,
      );
    }

    return actions;
  };

  if (!accessLoading && !hasAccess) {
    return (
      <DashboardLockedTabView
        title={t("title")}
        subtitle={t("description")}
        businessId={businessId}
      />
    );
  }

  return (
    <DashboardTabShell
      width="wide"
      fill={reservationView === "operations"}
      header={{
        title: t("title"),
        subtitle: t("description"),
        dense: reservationView === "operations",
        status: reservationsLoading
          ? undefined
          : {
              label: reservationsEnabled
                ? t("shell.enabled")
                : t("shell.disabled"),
              tone: reservationsEnabled ? "positive" : "attention",
            },
        stats:
          reservationsEnabled &&
          bookingsToday != null &&
          bookingsToday > 0
            ? [
                {
                  label:
                    bookingsToday === 1
                      ? t("shell.reservationCountLabel")
                      : t("shell.reservationCountLabel_other"),
                  value: bookingsToday,
                  title: t("shell.todayReservations"),
                },
                ...(coversToday != null
                  ? [
                      {
                        label: t("shell.coversCountLabel"),
                        value: coversToday,
                        title: t("insights.coversToday"),
                      },
                    ]
                  : []),
              ]
            : undefined,
        actions:
          activeTab === "reservations" &&
          reservationsEnabled &&
          reservationCaps.canCreate ? (
            <Button
              data-testid="reservation-create-button"
              className="bg-brand font-medium text-white hover:bg-brand-dark"
              onPress={() => {
                resetForm();
                setIsEditOpen(false);
                setIsViewOpen(false);
                setIsCancelOpen(false);
                setIsCreateOpen(true);
              }}
              startContent={<Plus className="w-4 h-4" />}
            >
              {t("createReservation")}
            </Button>
          ) : undefined,
      }}
      loading={reservationsLoading ? <ReservationsSkeleton /> : null}
      tabs={
        reservationsEnabled
          ? {
              items: [
                {
                  key: "reservations",
                  label: t("tabs.reservations"),
                  icon: Calendar,
                },
                ...(reservationCaps.canManageSettings
                  ? [
                      {
                        key: "settings",
                        label: t("tabs.settings"),
                        icon: Settings,
                      },
                    ]
                  : []),
              ],
              activeKey: activeTab,
              onChange: setActiveTab,
              ariaLabel: t("title"),
            }
          : undefined
      }
    >
      {!reservationsEnabled ? (
        reservationCaps.canManageSettings ? (
          <ReservationToggle
            businessId={businessId}
            // Must match the gate `useReservationStatus` above is given. Using
            // the raw `!hasAccess` locked this card out for the frames where
            // the access-gate query had not answered yet, so the tab rendered a
            // "paused" header over a literally empty body (#681).
            isLocked={!accessLoading && !hasAccess}
            variant="card"
            onToggled={setReservationsEnabled}
            onStatusResolved={setReservationsEnabled}
          />
        ) : (
          <PremiumPanel
            data-testid="reservations-disabled-placeholder"
            className="p-8 text-center sm:p-10"
            withTexture={false}
          >
            <h2 className="mb-2 font-title text-heading-lg text-ink-950">
              {t("disabled.title")}
            </h2>
            <p className="text-sm leading-6 text-ink-600">
              {t("disabled.contactOwner")}
            </p>
          </PremiumPanel>
        )
      ) : (
        <DashboardTabTransition tabKey={activeTab}>
          {activeTab === "reservations" ? (
            <div
              className={
                reservationView === "operations"
                  ? "flex min-h-0 min-w-0 w-full flex-1 flex-col gap-4"
                  : "min-w-0 w-full space-y-4"
              }
            >
              <div
                data-testid={
                  reservationView === "operations"
                    ? "reservation-board-chrome"
                    : undefined
                }
                className={
                  reservationView === "operations"
                    ? TABLERO_CHROME_CLASS
                    : undefined
                }
              >
              {reservationCaps.canEdit && (
                <ReservationApprovalQueue
                  pending={pendingApprovals}
                  onApprove={handleApproveReservation}
                  onDecline={handleDeclineRequest}
                  busyId={actionLoadingId}
                  businessTimezone={businessTimezone}
                  guestHistory={approvalGuestHistory}
                  total={pendingApprovalsTotal}
                  loadMoreLoading={pendingApprovalsLoadingMore}
                  onLoadMore={
                    pendingApprovalsTotal > pendingApprovals.length
                      ? () => {
                          void loadPendingApprovals({
                            page: pendingApprovalsPage + 1,
                            append: true,
                          });
                        }
                      : undefined
                  }
                />
              )}

              {reservationView === "operations" ? (
                <div
                  data-testid="reservation-board-insights"
                  className="flex min-w-0 shrink-0 flex-wrap items-baseline gap-x-4 gap-y-1 text-sm"
                >
                  {insightCards.map((card) => (
                    <p key={card.label} className="min-w-0 text-ink-600">
                      <span className="text-ink-400">{card.label}</span>{" "}
                      <span className="font-medium text-ink-900">
                        {!hasLoadedReservations ? (
                          <span className="text-warm-400" aria-hidden="true">
                            —
                          </span>
                        ) : typeof card.value === "number" ? (
                          <AnimatedNumberText value={card.value} />
                        ) : (
                          card.value
                        )}
                      </span>
                    </p>
                  ))}
                </div>
              ) : (
                <div className="grid grid-cols-1 md:grid-cols-2 xl:grid-cols-4 gap-4">
                  {insightCards.map((card) => (
                    <PremiumPanel
                      key={card.label}
                      className="p-5"
                      withTexture={false}
                    >
                      <div className="flex flex-row items-center justify-between gap-3">
                        <div>
                          <p className={reservationMetricLabelClass}>
                            {card.label}
                          </p>
                          <p className={reservationMetricValueClass}>
                            {!hasLoadedReservations ? (
                              <span className="text-warm-400" aria-hidden="true">
                                —
                              </span>
                            ) : typeof card.value === "number" ? (
                              <AnimatedNumberText value={card.value} />
                            ) : (
                              card.value
                            )}
                          </p>
                        </div>
                        <div className={reservationIconPillClass}>
                          {"icon" in card && card.icon === "clock" ? (
                            <Clock className="w-5 h-5" />
                          ) : (
                            <Users className="w-5 h-5" />
                          )}
                        </div>
                      </div>
                    </PremiumPanel>
                  ))}
                </div>
              )}

              <PremiumPanel
                as="section"
                className="shrink-0 p-4 sm:p-5"
                withTexture={false}
              >
                <div className="space-y-4">
                  <div className="flex items-start justify-between gap-3">
                    <div className="min-w-0 space-y-1">
                      <p className="text-xs font-semibold uppercase tracking-[0.2em] text-brand-700">
                        {t("shell.serviceLens")}
                      </p>
                      <p className="text-sm text-ink-600">
                        {t("shell.serviceLensDescription")}
                      </p>
                      {businessTimezone &&
                      businessTimezone !== deviceTimezone ? (
                        <p className="text-xs text-ink-500">
                          {tWithFallback("timesShownInTimezone").replace(
                            "{tz}",
                            // N-4: human label ("New York (EST/EDT)"), not raw IANA.
                            getTimezoneLabel(businessTimezone),
                          )}
                        </p>
                      ) : null}
                    </div>
                    <div
                      role="group"
                      aria-label={
                        t("viewToggle.list") + " / " + t("viewToggle.board")
                      }
                      className="inline-flex w-fit shrink-0 rounded-xl border border-warm-200 bg-white p-0.5 shadow-sm"
                    >
                      <Button
                        size="sm"
                        isIconOnly
                        aria-label={t("viewToggle.list")}
                        variant="light"
                        className={
                          reservationView === "list"
                            ? "bg-brand text-white"
                            : "text-ink-600"
                        }
                        onPress={() => setReservationView("list")}
                      >
                        <PanelRightClose className="w-4 h-4" />
                      </Button>
                      <Button
                        size="sm"
                        isIconOnly
                        aria-label={t("viewToggle.board")}
                        variant="light"
                        className={
                          reservationView === "operations"
                            ? "bg-brand text-white"
                            : "text-ink-600"
                        }
                        onPress={() => setReservationView("operations")}
                      >
                        <PanelRightOpen className="w-4 h-4" />
                      </Button>
                    </div>
                  </div>

                  {/* Single filter row: search + date window + status + attention.
                      Time window has exactly one source of truth (dateFilter). */}
                  <div className="flex flex-col gap-3">
                    <div className="flex flex-col lg:flex-row gap-3 lg:items-center">
                      <Input
                        aria-label={t("searchPlaceholder")}
                        placeholder={t("searchPlaceholder")}
                        value={search}
                        onValueChange={handleSearchChange}
                        startContent={
                          <Search className="w-4 h-4 text-ink-400" />
                        }
                        className="flex-1"
                        classNames={{
                          input: "text-ink-900 placeholder:text-ink-400",
                          inputWrapper:
                            "border-warm-200 bg-white/90 shadow-sm hover:border-brand/25 focus-within:border-brand",
                        }}
                      />
                      <Select
                        aria-label={t("dateFilter")}
                        selectedKeys={[dateFilter]}
                        onChange={handleDateFilterChange}
                        className="w-full lg:w-44"
                        size="sm"
                      >
                        <SelectItem key="today" value="today">
                          {t("filters.today")}
                        </SelectItem>
                        <SelectItem key="upcoming" value="upcoming">
                          {t("filters.upcoming")}
                        </SelectItem>
                        <SelectItem key="past" value="past">
                          {t("filters.past30Days")}
                        </SelectItem>
                        <SelectItem key="custom" value="custom">
                          {t("filters.customRange")}
                        </SelectItem>
                        {/* R2-B6: the retired "all" item was a duplicate of
                            "all_time" — identical label AND identical bounds. */}
                        <SelectItem key="all_time" value="all_time">
                          {t("filters.allIncludingPast")}
                        </SelectItem>
                      </Select>
                      {dateFilter === "custom" && (
                        <div className="flex w-full flex-col gap-2 sm:flex-row lg:w-auto">
                          <DatePicker
                            aria-label={t("filters.customFrom")}
                            size="sm"
                            className="w-full sm:w-40"
                            value={
                              customStartDate
                                ? parseDate(customStartDate)
                                : null
                            }
                            maxValue={
                              customEndDate
                                ? parseDate(customEndDate)
                                : undefined
                            }
                            onChange={(value) => {
                              setReservationPage(1);
                              setCustomStartDate(value?.toString() ?? "");
                            }}
                          />
                          <DatePicker
                            aria-label={t("filters.customTo")}
                            size="sm"
                            className="w-full sm:w-40"
                            value={
                              customEndDate ? parseDate(customEndDate) : null
                            }
                            minValue={
                              customStartDate
                                ? parseDate(customStartDate)
                                : undefined
                            }
                            onChange={(value) => {
                              setReservationPage(1);
                              setCustomEndDate(value?.toString() ?? "");
                            }}
                          />
                        </div>
                      )}
                      <Select
                        aria-label={t("statusFilter")}
                        selectedKeys={[statusFilter]}
                        onChange={handleStatusFilterChange}
                        className="w-full lg:w-44"
                        size="sm"
                      >
                        <SelectItem key="all" value="all">
                          {t("filters.allStatuses")}
                        </SelectItem>
                        <SelectItem key="pending" value="pending">
                          {getStatusLabel("pending")}
                        </SelectItem>
                        <SelectItem key="confirmed" value="confirmed">
                          {getStatusLabel("confirmed")}
                        </SelectItem>
                        <SelectItem key="waitlist" value="waitlist">
                          {getStatusLabel("waitlist")}
                        </SelectItem>
                        <SelectItem key="no_show" value="no_show">
                          {getStatusLabel("no_show")}
                        </SelectItem>
                        <SelectItem key="seated" value="seated">
                          {getStatusLabel("seated")}
                        </SelectItem>
                        <SelectItem key="completed" value="completed">
                          {getStatusLabel("completed")}
                        </SelectItem>
                        <SelectItem key="cancelled" value="cancelled">
                          {getStatusLabel("cancelled")}
                        </SelectItem>
                      </Select>
                      {(search.trim() !== "" ||
                        attentionFilter !== "none" ||
                        statusFilter !== "all" ||
                        dateFilter !== "today") && (
                        <Button
                          size="sm"
                          variant="light"
                          startContent={<XCircle className="w-4 h-4" />}
                          className="text-ink-600 hover:bg-brand/5 hover:text-brand-700 lg:self-center"
                          onPress={() => {
                            handleSearchChange("");
                            handleAttentionFilterChange("none");
                            setStatusFilter("all");
                            setDateFilter("today");
                            setCustomStartDate("");
                            setCustomEndDate("");
                            setReservationPage(1);
                          }}
                        >
                          {t("clearFilters")}
                        </Button>
                      )}
                    </div>
                    <div className="flex flex-wrap items-center gap-2">
                      <span className="text-xs font-semibold uppercase tracking-[0.14em] text-ink-500">
                        {t("attention.label")}
                      </span>
                      {attentionFilterOptions.map((option) => (
                        <Button
                          key={option.key}
                          size="sm"
                          variant={
                            attentionFilter === option.key ? "solid" : "flat"
                          }
                          className={
                            attentionFilter === option.key
                              ? "shrink-0 bg-brand text-white"
                              : reservationFilterInactiveClass
                          }
                          onPress={() =>
                            handleAttentionFilterChange(option.key)
                          }
                        >
                          {option.label}
                        </Button>
                      ))}
                      {attentionFilter !== "none" && (
                        <span className="text-xs text-ink-500">
                          {t("attention.boundedHint")}
                        </span>
                      )}
                    </div>
                  </div>
                </div>
              </PremiumPanel>
              </div>

              {reservationDataCapped && (
                <div className="flex items-center gap-2 rounded-lg border border-amber-200 bg-amber-50 px-3 py-2 text-sm text-amber-700">
                  <AlertCircle className="h-4 w-4 shrink-0" />
                  <span>{t("partialResultsNotice")}</span>
                </div>
              )}

              {shouldShowReservationsRefreshing(loading, activeSourceLoaded) && (
                <div
                  className="flex items-center gap-2 text-xs font-medium text-ink-500"
                  data-testid="reservations-refreshing"
                  role="status"
                  aria-live="polite"
                >
                  <span
                    className="inline-block h-3 w-3 animate-spin rounded-full border-2 border-brand/30 border-t-brand"
                    aria-hidden="true"
                  />
                  {t("refreshing")}
                </div>
              )}

              {shouldShowReservationsSkeleton(activeSourceLoaded) ? (
                <ReservationsSkeleton />
              ) : !activeViewHasReservations ? (
                // L1-8: "today" is a narrowing filter — empty Today must not
                // claim first-run and jump to Settings. Only a fully open
                // window with no search/status/attention may onboard.
                (() => {
                  const emptyKind = resolveReservationEmptyKind({
                    search,
                    attentionFilter,
                    statusFilter,
                    dateFilter,
                  });
                  if (emptyKind === "first_run") {
                    return (
                      <EmptyState
                        panel
                        icon={CalendarDays}
                        title={t("empty.title")}
                        subtitle={t("empty.subtitle")}
                        actionLabel={t("empty.action")}
                        onAction={() => setActiveTab("settings")}
                      />
                    );
                  }
                  if (emptyKind === "today_empty") {
                    return (
                      <EmptyState
                        panel
                        icon={CalendarDays}
                        title={t("empty.todayTitle")}
                        subtitle={t("empty.todaySubtitle")}
                        actionLabel={t("empty.viewUpcomingAction")}
                        onAction={() => {
                          setDateFilter("upcoming");
                          setReservationPage(1);
                        }}
                      />
                    );
                  }
                  return (
                    <EmptyState
                      panel
                      icon={Search}
                      title={t("empty.noMatchTitle")}
                      subtitle={t("empty.noMatchSubtitle")}
                      actionLabel={t("empty.clearFiltersAction")}
                      onAction={() => {
                        handleSearchChange("");
                        handleAttentionFilterChange("none");
                        setStatusFilter("all");
                        setDateFilter("today");
                        setCustomStartDate("");
                        setCustomEndDate("");
                        setReservationPage(1);
                      }}
                    />
                  );
                })()
              ) : reservationView === "operations" ? (
                <ReservationBoardScroller
                  fill
                  scrollHint={t("board.scrollHint")}
                >
                  {reservationBoard.map((column) => (
                    <PremiumPanel
                      key={column.key}
                      className={reservationBoardLaneClass}
                      withTexture={false}
                      data-testid="reservation-board-lane"
                      data-lane={column.key}
                      data-empty={
                        column.reservations.length === 0 ? "true" : "false"
                      }
                    >
                      <div className="flex items-center justify-between gap-3 pb-3">
                        <div>
                          <h3 className={reservationSectionTitleClass}>
                            {column.title}
                          </h3>
                          <p className={reservationSectionDescriptionClass}>
                            {t("columnCount").replace(
                              "{count}",
                              String(column.reservations.length),
                            )}
                          </p>
                        </div>
                        <Chip size="sm" variant="flat" color="default">
                          {column.reservations.length}
                        </Chip>
                      </div>
                      <div className="space-y-3">
                        {column.reservations.length === 0 ? (
                          <div
                            className={reservationColumnEmptyClass}
                            data-testid="reservation-board-empty"
                          >
                            {t("nothingInColumn").replace(
                              "{column}",
                              column.title.toLowerCase(),
                            )}
                          </div>
                        ) : (
                          (() => {
                            const capped = capBoardColumnItems(
                              column.reservations,
                              {
                                expanded:
                                  expandedBoardColumns[column.key] === true,
                              },
                            );
                            return (
                              <>
                          {capped.visible.map((reservation) => (
                            <div
                              key={reservation.id}
                              className={reservationBoardCardClass}
                            >
                              <div className="flex items-start justify-between gap-3">
                                <div>
                                  <p className="font-semibold text-ink-950">
                                    {reservation.customer_name}
                                  </p>
                                  <p className="mt-1 text-sm text-ink-600">
                                    {reservationCardTimeMode(
                                      reservationDateRange.startDate,
                                      reservationDateRange.endDate,
                                    ) === "date_time"
                                      ? formatDateTime(
                                          reservation.reservation_time,
                                        )
                                      : formatTimeOnly(
                                          reservation.reservation_time,
                                        )}{" "}
                                    · {reservation.party_size} {t("guests")}
                                  </p>
                                </div>
                                {/*
                                 * Round 4 audit (Task 5): was a color-only
                                 * Chip with inverted tone semantics vs other
                                 * screens. StatusChip unifies icon+label+tone.
                                 */}
                                <StatusChip
                                  kind="reservation"
                                  status={displayStatusFor(reservation)}
                                  labelOverride={getStatusLabel(
                                    displayStatusFor(reservation),
                                  )}
                                />
                                <OperationalAlertClaimStatus
                                  resourceType="reservation"
                                  resourceId={reservation.id}
                                />
                              </div>

                              <div className="mt-3 flex flex-wrap gap-2 text-xs">
                                <span className={reservationBadgeClass}>
                                  <MapPin className="w-3 h-3" />
                                  {reservation.table?.name ||
                                    t("serviceDetails.unassigned")}
                                </span>
                                {reservation.customer_phone ? (
                                  <span className={reservationBadgeClass}>
                                    <Phone className="w-3 h-3" />
                                    {reservation.customer_phone}
                                  </span>
                                ) : null}
                              </div>

                              {!reservation.table_id &&
                              ["pending", "confirmed", "waitlist"].includes(
                                reservation.status,
                              ) ? (
                                <div className="mt-3">
                                  {renderRecommendedTableButtons(
                                    reservation,
                                    reservation.id,
                                  )}
                                </div>
                              ) : null}

                              <div className="mt-4 flex flex-wrap gap-2">
                                <Button
                                  aria-label={`${t("viewReservationAria")} ${reservation.customer_name}, ${formatDateTime(reservation.reservation_time)}`}
                                  size="sm"
                                  variant="flat"
                                  startContent={<Eye className="w-4 h-4" />}
                                  onPress={() => handleViewDetails(reservation)}
                                >
                                  {t("view")}
                                </Button>
                                {renderQuickActionButtons(reservation)}
                              </div>
                            </div>
                          ))}
                          {capped.hiddenCount > 0 && (
                            <Button
                              size="sm"
                              variant="flat"
                              className="w-full border border-warm-200 bg-white/90 text-ink-700"
                              onPress={() =>
                                setExpandedBoardColumns((current) => ({
                                  ...current,
                                  [column.key]: true,
                                }))
                              }
                            >
                              {t("board.showMore").replace(
                                "{count}",
                                String(capped.hiddenCount),
                              )}
                            </Button>
                          )}
                              </>
                            );
                          })()
                        )}
                      </div>
                    </PremiumPanel>
                  ))}
                </ReservationBoardScroller>
              ) : (
                <PremiumPanel
                  className="overflow-visible"
                  style={{ overflow: "visible" }}
                  withTexture={false}
                  data-testid="reservation-list-panel"
                >
                  <div className="border-b border-warm-200/80 px-5 py-4">
                    <h3 className="text-lg font-semibold text-ink-950">
                      {t("reservationList")}
                    </h3>
                  </div>
                  <div className="p-2 sm:p-4">
                    <div
                      className="overflow-x-auto overflow-y-visible"
                      style={{ overflowY: "visible" }}
                    >
                      <Table aria-label={t("tableAria")}>
                        <TableHeader>
                          <TableColumn>{t("customer")}</TableColumn>
                          <TableColumn>{t("table")}</TableColumn>
                          <TableColumn>{t("dateTime")}</TableColumn>
                          <TableColumn>{t("partySize")}</TableColumn>
                          <TableColumn>{t("statusColumn")}</TableColumn>
                          <TableColumn>{t("actions")}</TableColumn>
                        </TableHeader>
                        <TableBody>
                          {visibleListReservations.map((reservation) => (
                            <TableRow key={reservation.id}>
                              <TableCell>
                                <div>
                                  <p className="font-medium">
                                    {reservation.customer_name}
                                  </p>
                                  {reservation.customer_phone ? (
                                    <p className="flex items-center gap-1 text-xs text-ink-600">
                                      <Phone className="w-3 h-3" />
                                      {reservation.customer_phone}
                                    </p>
                                  ) : null}
                                </div>
                              </TableCell>
                              <TableCell>
                                <Chip size="sm" variant="flat">
                                  {reservation.table?.name ||
                                    t("serviceDetails.unassigned")}
                                </Chip>
                              </TableCell>
                              <TableCell>
                                <div className="flex items-center gap-2">
                                  <Clock className="w-4 h-4 text-ink-400" />
                                  <span className="text-sm">
                                    {formatDateTime(
                                      reservation.reservation_time,
                                    )}
                                  </span>
                                </div>
                              </TableCell>
                              <TableCell>
                                <div className="flex items-center gap-1">
                                  <Users className="w-4 h-4 text-ink-400" />
                                  <span>{reservation.party_size}</span>
                                </div>
                              </TableCell>
                              <TableCell>
                                <StatusChip
                                  kind="reservation"
                                  status={displayStatusFor(reservation)}
                                  labelOverride={getStatusLabel(
                                    displayStatusFor(reservation),
                                  )}
                                />
                                <OperationalAlertClaimStatus
                                  resourceType="reservation"
                                  resourceId={reservation.id}
                                  className="mt-1"
                                />
                              </TableCell>
                              <TableCell>
                                <div className="flex items-center gap-2">
                                  <Button
                                    isIconOnly
                                    size="sm"
                                    variant="light"
                                    aria-label={`${t("viewReservationAria")} ${reservation.customer_name}, ${formatDateTime(reservation.reservation_time)}`}
                                    // React Aria's table uses row-level roving
                                    // focus. Keep this nested action as the
                                    // active element so Enter activates it.
                                    onFocus={(event) => event.stopPropagation()}
                                    onPress={() =>
                                      handleViewDetails(reservation)
                                    }
                                  >
                                    <Eye className="w-4 h-4" />
                                  </Button>
                                  {(reservationCaps.canEdit ||
                                    reservationCaps.canDelete) && (
                                    // Portaled row menu (issue 659): NextUI
                                    // Dropdown flipped over Acciones and
                                    // swallowed Escape inside the table.
                                    <RowActionsMenu
                                      aria-label={`${t("moreReservationActionsAria")} ${reservation.customer_name}, ${formatDateTime(reservation.reservation_time)}`}
                                      data-testid={`reservation-actions-${reservation.id}`}
                                      placement="bottom-end"
                                      items={reservationListRowActions(
                                        reservation,
                                        reservationCaps,
                                        t,
                                      )}
                                      onAction={(key) =>
                                        handleReservationRowAction(
                                          reservation,
                                          key,
                                        )
                                      }
                                    />
                                  )}
                                </div>
                              </TableCell>
                            </TableRow>
                          ))}
                        </TableBody>
                      </Table>
                    </div>
                    {effectiveReservationTotalPages > 1 && (
                      <div className="flex items-center justify-between gap-3 px-2 py-4">
                        <span className="text-sm text-ink-500">
                          {tWithFallback("totalCount").replace(
                            "{count}",
                            String(effectiveReservationTotal),
                          )}
                        </span>
                        <Pagination
                          total={effectiveReservationTotalPages}
                          page={reservationPage}
                          onChange={setReservationPage}
                          showControls
                          color="primary"
                        />
                      </div>
                    )}
                  </div>
                </PremiumPanel>
              )}
            </div>
          ) : activeTab === "settings" && reservationCaps.canManageSettings ? (
            <div className="space-y-4">
              <PremiumPanel
                data-testid="reservation-toggle-button"
                className="flex flex-col gap-4 p-5 sm:flex-row sm:items-center sm:justify-between"
                withTexture={false}
              >
                <div>
                  <h3 className="text-base font-semibold text-ink-950">
                    {t("title")}
                  </h3>
                  <p className="mt-0.5 text-sm leading-5 text-ink-600">
                    {t("description")}
                  </p>
                </div>
                <ReservationToggle
                  businessId={businessId}
                  isLocked={!accessLoading && !hasAccess}
                  variant="button"
                  onToggled={setReservationsEnabled}
                />
              </PremiumPanel>
              <ReservationSettings businessId={businessId} />
            </div>
          ) : null}
        </DashboardTabTransition>
      )}

      <ReservationFormModal
        mode="create"
        isOpen={isCreateOpen}
        onClose={() => {
          setIsCreateOpen(false);
          clearReservationFormBaseline();
        }}
        title={t("modals.create.title")}
        subtitle={t("modals.create.subtitle")}
        cancelLabel={t("modals.create.cancel")}
        submitLabel={t("modals.create.create")}
        formData={formData}
        onFormDataChange={(next) =>
          setFormData((current) => ({ ...current, ...next }))
        }
        fieldLabels={{
          customerInfo: t("modals.create.customerInfo"),
          customerName: t("modals.create.customerName"),
          customerPhone: t("modals.create.customerPhone"),
          customerEmail: t("modals.create.customerEmail"),
          reservationDetails: t("modals.create.reservationDetails"),
          partySize: t("modals.create.partySize"),
          dateTime: t("modals.create.dateTime"),
          duration: t("modals.create.duration"),
          minutes: t("minutes"),
          additionalInfo: t("modals.create.additionalInfo"),
          specialRequests: t("modals.create.specialRequests"),
          specialRequestsPlaceholder: t(
            "modals.create.specialRequestsPlaceholder",
          ),
          notes: t("modals.create.notes"),
          notesPlaceholder: t("modals.create.notesPlaceholder"),
          phoneInvalid: t("validation.customerPhoneInvalid"),
          phoneRequired: t("validation.customerPhoneRequired"),
          timeEntry: {
            modeLabel: t("timeEntry.modeLabel"),
            business: t("timeEntry.business"),
            device: t("timeEntry.device"),
            preview: t("timeEntry.preview"),
          },
          occupancy: {
            available: t("occupancy.available"),
            occupied: t("occupancy.occupied"),
            staleOccupied: t("occupancy.staleOccupied"),
            override: t("occupancy.override"),
            overrideRequired: t("occupancy.overrideRequired"),
            reservationConflict: t("occupancy.reservationConflict"),
            capacityConflict: t("occupancy.capacityConflict"),
            tableClearedForPartySize: t("occupancy.tableClearedForPartySize"),
          },
        }}
        timeEntryMode={timeEntryMode}
        onTimeEntryModeChange={setTimeEntryMode}
        businessTimeZone={resolvedBusinessTimezone}
        deviceTimeZone={resolveBusinessTimeZone(deviceTimezone)}
        minPartySize={reservationSettings?.min_party_size}
        maxPartySize={reservationSettings?.max_party_size}
        defaultDuration={reservationSettings?.default_duration}
        tableOptions={tableOptions}
        occupancyOverrideAcknowledged={occupancyOverrideAcknowledged}
        onOccupancyOverrideChange={setOccupancyOverrideAcknowledged}
        recommendedTablesSlot={renderRecommendedTableButtons(formData)}
        isSubmitting={isSubmitting}
        onTableClearedForPartySize={(reason) => toast.error(reason)}
        onSubmit={() => {
          void handleCreateReservation();
        }}
      />

      <ReservationFormModal
        mode="edit"
        isOpen={isEditOpen}
        onClose={() => {
          setIsEditOpen(false);
          clearReservationFormBaseline();
        }}
        title={t("modals.edit.title")}
        subtitle={t("modals.edit.subtitle")}
        cancelLabel={t("modals.edit.cancel")}
        submitLabel={t("modals.edit.save")}
        formData={formData}
        onFormDataChange={(next) =>
          setFormData((current) => ({ ...current, ...next }))
        }
        fieldLabels={{
          customerInfo: t("modals.edit.customerInfo"),
          customerName: t("modals.edit.customerName"),
          customerPhone: t("modals.edit.customerPhone"),
          customerEmail: t("modals.edit.customerEmail"),
          reservationDetails: t("modals.edit.reservationDetails"),
          partySize: t("modals.edit.partySize"),
          dateTime: t("modals.edit.dateTime"),
          duration: t("modals.edit.duration"),
          minutes: t("minutes"),
          additionalInfo: t("modals.edit.additionalInfo"),
          specialRequests: t("modals.edit.specialRequests"),
          specialRequestsPlaceholder: t(
            "modals.edit.specialRequestsPlaceholder",
          ),
          notes: t("modals.edit.notes"),
          notesPlaceholder: t("modals.edit.notesPlaceholder"),
          phoneInvalid: t("validation.customerPhoneInvalid"),
          phoneRequired: t("validation.customerPhoneRequired"),
          timeEntry: {
            modeLabel: t("timeEntry.modeLabel"),
            business: t("timeEntry.business"),
            device: t("timeEntry.device"),
            preview: t("timeEntry.preview"),
          },
          occupancy: {
            available: t("occupancy.available"),
            occupied: t("occupancy.occupied"),
            staleOccupied: t("occupancy.staleOccupied"),
            override: t("occupancy.override"),
            overrideRequired: t("occupancy.overrideRequired"),
            reservationConflict: t("occupancy.reservationConflict"),
            capacityConflict: t("occupancy.capacityConflict"),
            tableClearedForPartySize: t("occupancy.tableClearedForPartySize"),
          },
        }}
        timeEntryMode={timeEntryMode}
        onTimeEntryModeChange={setTimeEntryMode}
        businessTimeZone={resolvedBusinessTimezone}
        deviceTimeZone={resolveBusinessTimeZone(deviceTimezone)}
        minPartySize={reservationSettings?.min_party_size}
        maxPartySize={reservationSettings?.max_party_size}
        defaultDuration={reservationSettings?.default_duration}
        tableOptions={tableOptions}
        occupancyOverrideAcknowledged={occupancyOverrideAcknowledged}
        onOccupancyOverrideChange={setOccupancyOverrideAcknowledged}
        recommendedTablesSlot={renderRecommendedTableButtons(formData)}
        isSubmitting={isSubmitting}
        onTableClearedForPartySize={(reason) => toast.error(reason)}
        onSubmit={() => {
          void handleUpdateReservation();
        }}
      />

      {isViewOpen && selectedReservation ? (
        <ReservationDetailDrawer
          reservation={selectedReservation}
          drawerRef={detailsDrawerRef}
          onClose={() => {
            setIsViewOpen(false);
            // Restore Board scroll after focus returns so a peek at a booking
            // does not jump the host back to the top of the page.
            const y = boardScrollYRef.current;
            if (typeof window !== "undefined" && y > 0) {
              requestAnimationFrame(() => {
                window.scrollTo({ top: y, behavior: "auto" });
              });
            }
          }}
          t={t}
          getStatusLabel={getStatusLabel}
          formatDateTime={formatDateTime}
          canEdit={reservationCaps.canEdit}
          canDelete={reservationCaps.canDelete}
          tables={tables}
          actionLoadingId={actionLoadingId}
          renderRecommendedTableButtons={renderRecommendedTableButtons}
          renderQuickActionButtons={renderQuickActionButtons}
          onAssignTable={(reservationId, tableId) => {
            void handleAssignTable(reservationId, tableId);
          }}
          onEdit={handleEditReservation}
          onCancel={handleCancelReservation}
        />
      ) : null}

      <Modal
        isOpen={isCancelOpen}
        onOpenChange={setIsCancelOpen}
        size="md"
        classNames={reservationModalClassNames}
      >
        <ModalContent>
          {(onClose) => (
            <>
              <ModalHeader className="flex flex-col gap-1">
                <h3 className="text-lg font-semibold text-ink-950">
                  {t("confirmCancel")}
                </h3>
              </ModalHeader>
              <ModalBody>
                {selectedReservation ? (
                  <div className="space-y-4">
                    <p className={reservationModalCopyClass}>
                      {t("confirmCancelMessage")}
                    </p>
                    <div className={`${reservationSoftBoxClass} space-y-2`}>
                      <p className="text-sm">
                        <span className="font-medium text-ink-700">
                          {t("customer")}:
                        </span>{" "}
                        {selectedReservation.customer_name}
                      </p>
                      <p className="text-sm">
                        <span className="font-medium text-ink-700">
                          {t("dateTime")}:
                        </span>{" "}
                        {formatDateTime(selectedReservation.reservation_time)}
                      </p>
                      <p className="text-sm">
                        <span className="font-medium text-ink-700">
                          {t("partySize")}:
                        </span>{" "}
                        {selectedReservation.party_size} {t("guests")}
                      </p>
                    </div>
                    <p className="rounded-xl border border-rose-200 bg-rose-50 px-3 py-2 text-sm text-rose-700">
                      {t("confirmCancelWarning")}
                    </p>
                  </div>
                ) : null}
              </ModalBody>
              <ModalFooter>
                <Button color="default" variant="light" onPress={onClose}>
                  {t("modals.create.cancel")}
                </Button>
                <Button
                  color="danger"
                  onPress={confirmCancelReservation}
                  startContent={<XCircle className="w-4 h-4" />}
                  isLoading={
                    selectedReservation
                      ? actionLoadingId === selectedReservation.id
                      : false
                  }
                >
                  {t("cancelReservationButton")}
                </Button>
              </ModalFooter>
            </>
          )}
        </ModalContent>
      </Modal>

      <Modal
        isOpen={declineTarget !== null}
        onOpenChange={(open) => {
          if (!open) {
            setDeclineTarget(null);
            setDeclineReason("");
          }
        }}
        size="md"
        classNames={reservationModalClassNames}
      >
        <ModalContent>
          {(onClose) => (
            <>
              <ModalHeader className="flex flex-col gap-1">
                <h3 className="text-lg font-semibold text-ink-950">
                  {t("approvalQueue.confirmDecline")}
                </h3>
              </ModalHeader>
              <ModalBody>
                {declineTarget ? (
                  <div className="space-y-4">
                    <div className={`${reservationSoftBoxClass} space-y-2`}>
                      <p className="text-sm">
                        <span className="font-medium text-ink-700">
                          {t("customer")}:
                        </span>{" "}
                        {declineTarget.customer_name}
                      </p>
                      <p className="text-sm">
                        <span className="font-medium text-ink-700">
                          {t("dateTime")}:
                        </span>{" "}
                        {formatDateTime(declineTarget.reservation_time)}
                      </p>
                      <p className="text-sm">
                        <span className="font-medium text-ink-700">
                          {t("partySize")}:
                        </span>{" "}
                        {declineTarget.party_size} {t("guests")}
                      </p>
                    </div>
                    <Textarea
                      label={t("approvalQueue.declineReasonLabel")}
                      value={declineReason}
                      onValueChange={setDeclineReason}
                      minRows={2}
                    />
                  </div>
                ) : null}
              </ModalBody>
              <ModalFooter>
                <Button color="default" variant="light" onPress={onClose}>
                  {t("modals.create.cancel")}
                </Button>
                <Button
                  color="danger"
                  onPress={handleDeclineReservation}
                  startContent={<XCircle className="w-4 h-4" />}
                  isLoading={
                    declineTarget ? actionLoadingId === declineTarget.id : false
                  }
                >
                  {t("approvalQueue.decline")}
                </Button>
              </ModalFooter>
            </>
          )}
        </ModalContent>
      </Modal>

      <ConfirmationModal
        isOpen={stealPrompt !== null}
        onOpenChange={() => {
          setStealPrompt((prev) => {
            prev?.resolve(false);
            return null;
          });
        }}
        title={t("claim.takeOver")}
        description={t("claim.stealRequired", {
          name: stealPrompt?.name ?? "",
        })}
        confirmLabel={t("claim.takeOver")}
        cancelLabel={t("claim.cancel")}
        isDanger
        onConfirm={() => {
          setStealPrompt((prev) => {
            prev?.resolve(true);
            return null;
          });
        }}
      />
    </DashboardTabShell>
  );
}
