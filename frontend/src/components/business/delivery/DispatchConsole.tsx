"use client";

import React, { useState, useEffect, useCallback, useRef } from "react";
import { Button, Chip, Skeleton } from "@nextui-org/react";
import {
  AlertCircle,
  Clock,
  MapPin,
  RefreshCw,
  Route,
  Truck,
} from "lucide-react";
import { useToast } from "@/contexts/ToastContext";
import { useAuth } from "@/providers/HybridAuthProvider";
import {
  deliveryApi,
  DeliveryDriver,
  DeliveryOrder,
  getDeliveryClaimConflict,
  type DeliveryStatus,
} from "@/api/delivery";
import { useSSEEvents, type SSEEvent } from "@/hooks/useSSEEvents";
import { useDeliveryBusiness } from "./useDeliveryQueries";
import {
  getTranslation,
  useSimpleLocale,
} from "@/i18n/SimpleTranslationProvider";
import { StatusFilter, FilterStatus } from "./dispatch/StatusFilter";
import { OrderRow } from "./dispatch/OrderRow";
import { CancelDeliveryModal } from "./dispatch/CancelDeliveryModal";
import DispatchBoard, {
  type BoardFilter,
} from "./dispatch/DispatchBoard";
import type { DispatchOrder } from "./dispatch/DispatchCard";
import { getOperatorNextStatus } from "./dispatch/OrderRow";
import { useOptionalOperationalAlerts } from "../operational-alerts/useOperationalAlerts";
import {
  buildDispatchBoardLoadPlan,
  DISPATCH_PAGE_SIZE,
  isDispatchLoadCanceled,
  mergeDispatchBoardResults,
} from "./dispatchBoardLoad";
import OrderDetailDrawer from "./dispatch/OrderDetailDrawer";
import ConfirmationModal from "@/components/business/modals/ConfirmationModal";

/** Statuses that warrant the paid-cancel warning in the cancel dialog. */
const PAID_CANCEL_WARN_STATUSES = new Set([
  "preparing",
  "ready",
  "assigned",
  "picked_up",
  "in_transit",
  "nearby",
]);

// Board filter chips: happy-path lanes + exception states (All clears the filter).
const BOARD_FILTER_OPTIONS: { key: BoardFilter; labelKey: string }[] = [
  { key: null, labelKey: "dispatch.filters.all" },
  { key: "awaiting_payment", labelKey: "dispatch.filters.awaiting_payment" },
  { key: "preparing", labelKey: "dispatch.filters.preparing" },
  { key: "ready", labelKey: "dispatch.filters.ready" },
  { key: "out_for_delivery", labelKey: "dispatch.filters.out_for_delivery" },
  { key: "delivered", labelKey: "dispatch.filters.delivered" },
  { key: "cancelled", labelKey: "dispatch.filters.cancelled" },
  { key: "failed", labelKey: "dispatch.filters.failed" },
];

/** Delivery dispatch keeps a slower heartbeat while idle so new work is not missed. */
const ACTIVE_REFRESH_MS = 30_000;
const IDLE_REFRESH_MS = 60_000;
/** Tailwind `lg` breakpoint — keep in sync with tailwind.config. */
const LG_BREAKPOINT_PX = 1024;

const emptyDispatchStops = [
  { icon: Clock, className: "bg-brand/15 text-brand" },
  { icon: MapPin, className: "bg-amber-100 text-amber-700" },
  { icon: Truck, className: "bg-emerald-100 text-emerald-700" },
];

interface DispatchConsoleProps {
  businessId: number;
}

const ACTIVE_STATUSES = new Set([
  "pending",
  "confirmed",
  "preparing",
  "ready",
  "assigned",
  "picked_up",
  "in_transit",
  "nearby",
]);

// Active board statuses include intake (pending/confirmed) so the header
// matches Bills/Kitchen when an unpaid delivery is waiting.
const BOARD_ACTIVE_STATUSES = new Set([
  "pending",
  "confirmed",
  "preparing",
  "ready",
  "assigned",
  "picked_up",
  "in_transit",
  "nearby",
]);

function formatDispatchAddress(order: DeliveryOrder): string {
  const a = order.delivery_address;
  if (!a) return "";
  if (a.formatted_address) return a.formatted_address;
  return [a.street, a.apartment, a.city, a.state, a.postal_code]
    .filter(Boolean)
    .join(", ");
}

function quoteNumber(
  meta: DeliveryOrder["quote_metadata"],
  key: "order_subtotal" | "item_count",
): number | undefined {
  if (!meta || typeof meta !== "object") return undefined;
  const raw = meta[key];
  return typeof raw === "number" && Number.isFinite(raw) ? raw : undefined;
}

function matchesFilter(order: DeliveryOrder, filter: FilterStatus): boolean {
  if (filter === "all") return true;
  if (filter === "pending") return order.status === "pending" || order.status === "confirmed";
  if (filter === "preparing") return order.status === "preparing";
  if (filter === "ready") return order.status === "ready" || order.status === "assigned";
  if (filter === "in_transit")
    return (
      order.status === "picked_up" ||
      order.status === "in_transit" ||
      order.status === "nearby"
    );
  return order.status === filter;
}

const TERMINAL_STATUSES = new Set(["delivered", "cancelled", "failed"]);

/** Calendar-day key for `d` in `timeZone` (falls back to the device day when
 *  the timezone is missing or invalid). */
function calendarDayKey(d: Date, timeZone?: string): string {
  if (timeZone) {
    try {
      // en-CA formats as YYYY-MM-DD — a stable, comparable day key.
      return new Intl.DateTimeFormat("en-CA", {
        timeZone,
        year: "numeric",
        month: "2-digit",
        day: "2-digit",
      }).format(d);
    } catch {
      // Invalid IANA zone — fall through to the device day.
    }
  }
  return `${d.getFullYear()}-${d.getMonth() + 1}-${d.getDate()}`;
}

/** True when `iso` falls on today's calendar day in the BUSINESS timezone
 *  (device day when no timezone is known). Used to drop stale terminal orders
 *  that would otherwise accumulate all-time in the Delivered/terminal columns
 *  (the list load is capped at 200 with no `since` filter). The business day
 *  matters: a traveling owner or overseas VA must see the restaurant's
 *  operating day, not their device's (DEL-OP-8). Exported for tests. */
export function isSameBusinessDay(
  iso: string | undefined,
  timeZone?: string,
  now: Date = new Date(),
): boolean {
  if (!iso) return false;
  const d = new Date(iso);
  if (Number.isNaN(d.getTime())) return false;
  return calendarDayKey(d, timeZone) === calendarDayKey(now, timeZone);
}

/** Trailing window for coalescing SSE-triggered reloads: the first event of a
 *  burst reloads immediately, the rest collapse into one trailing reload. */
const SSE_COALESCE_MS = 1_500;

function sortOrders(orders: DeliveryOrder[]): DeliveryOrder[] {
  return [...orders].sort((a, b) => {
    const aActive = ACTIVE_STATUSES.has(a.status);
    const bActive = ACTIVE_STATUSES.has(b.status);
    if (aActive !== bActive) return aActive ? -1 : 1;
    // Active orders stay FIFO (oldest first — the kitchen/dispatch queue). But
    // terminal orders (delivered/cancelled/failed) read best newest-first so the
    // most recently completed delivery sits at the top of its column instead of
    // an all-time backlog scrolling down from the oldest.
    const aTerminal = TERMINAL_STATUSES.has(a.status);
    const at = new Date(a.created_at).getTime();
    const bt = new Date(b.created_at).getTime();
    return aTerminal ? bt - at : at - bt;
  });
}

function DispatchEmptyPanel({
  subtitle,
  title,
}: {
  subtitle: string;
  title: string;
}) {
  return (
    <div className="overflow-hidden rounded-2xl border border-brand/10 bg-brand/5 p-4 sm:p-5">
      <div className="grid items-center gap-6 lg:grid-cols-[minmax(0,1fr)_300px]">
        <div>
          <div className="mb-5 flex h-12 w-12 items-center justify-center rounded-2xl border border-white/80 bg-white shadow-sm">
            <Truck className="h-5 w-5 text-brand" strokeWidth={1.7} />
          </div>
          <h3 className="font-title text-2xl text-ink-950">{title}</h3>
          <p className="mt-3 max-w-xl text-sm leading-6 text-ink-600">
            {subtitle}
          </p>
        </div>

        <div
          className="relative min-h-[220px] rounded-2xl border border-white/80 bg-white/85 p-4 shadow-[0_20px_54px_rgba(46,42,37,0.1)]"
          aria-hidden="true"
        >
          <div className="mb-5 flex items-center justify-between">
            <div className="h-2 w-24 rounded-full bg-warm-200" />
            <div className="flex h-9 w-9 items-center justify-center rounded-xl bg-brand/10">
              <Route className="h-4 w-4 text-brand" />
            </div>
          </div>

          <div className="relative space-y-4">
            <div className="absolute left-[18px] top-5 h-[120px] w-px bg-warm-200" />
            {emptyDispatchStops.map(({ icon: Icon, className }, index) => (
              <div
                key={className}
                className="relative flex items-center gap-3 rounded-xl border border-warm-200 bg-warm-50/80 p-3"
              >
                <span
                  className={`z-10 flex h-9 w-9 items-center justify-center rounded-xl ${className}`}
                >
                  <Icon className="h-4 w-4" />
                </span>
                <span className="h-2 flex-1 rounded-full bg-ink-200" />
                <span
                  className={`h-2 rounded-full ${
                    index === 1 ? "w-10 bg-amber-200" : "w-14 bg-emerald-200"
                  }`}
                />
              </div>
            ))}
          </div>

          <div className="mt-4 grid grid-cols-3 gap-2">
            <span className="h-8 rounded-lg bg-brand/15" />
            <span className="h-8 rounded-lg bg-warm-100" />
            <span className="h-8 rounded-lg bg-rose-100" />
          </div>
        </div>
      </div>
    </div>
  );
}

export default function DispatchConsole({ businessId }: DispatchConsoleProps) {
  const { showSuccess, showError } = useToast();
  const { locale } = useSimpleLocale();
  const operationalAlerts = useOptionalOperationalAlerts();
  const { staffData } = useAuth();
  const myStaffId = staffData?.id ?? null;
  // Steal mirrors backend: owner principal or manager role.
  const canSteal = !staffData || staffData.role === "manager";

  const tString = useCallback(
    (key: string, vars?: Record<string, string>): string => {
      const result = getTranslation(`deliverySettings.${key}`, locale);
      let val = Array.isArray(result) ? result[0] || key : (result as string);
      if (typeof val !== "string") return key;
      if (vars) {
        Object.entries(vars).forEach(([k, v]) => {
          val = val.replace(`{${k}}`, v);
        });
      }
      return val;
    },
    [locale]
  );

  const tDash = useCallback(
    (key: string): string => {
      const result = getTranslation(`businessDashboard.${key}`, locale);
      return Array.isArray(result) ? result[0] || key : (result as string) || key;
    },
    [locale]
  );

  const [orders, setOrders] = useState<DeliveryOrder[]>([]);
  const [drivers, setDrivers] = useState<DeliveryDriver[]>([]);
  const [loading, setLoading] = useState(true);
  const [loadError, setLoadError] = useState(false);
  const [filter, setFilter] = useState<FilterStatus>("all");
  const [boardFilter, setBoardFilter] = useState<BoardFilter>(null);
  const [cancelTarget, setCancelTarget] = useState<DeliveryOrder | null>(null);
  const [stealTarget, setStealTarget] = useState<{
    order: DispatchOrder;
    name: string;
  } | null>(null);
  const [detailOrderId, setDetailOrderId] = useState<number | null>(null);
  const [refreshing, setRefreshing] = useState(false);
  const [lastRefreshed, setLastRefreshed] = useState<Date | null>(null);
  const [secondsAgo, setSecondsAgo] = useState<number>(0);
  // Render-time breakpoint: mobile/list by default (SSR-safe); board kicks in on `lg+` after hydration.
  const [isLargeScreen, setIsLargeScreen] = useState(false);
  // Shared React Query business read — bouncing sub-tabs no longer re-fetch.
  const { data: business } = useDeliveryBusiness(businessId);
  const businessCurrency = business?.default_currency || "USD";
  const businessTimezone = business?.timezone || undefined;
  // Server-reported active total (not just the loaded window) so poll cadence
  // stays aggressive when the board is truncated at the busiest moments.
  const [activeTotal, setActiveTotal] = useState(0);
  const [terminalTotal, setTerminalTotal] = useState(0);
  const [activeHasMore, setActiveHasMore] = useState(false);
  const [terminalHasMore, setTerminalHasMore] = useState(false);
  const [activeLimit, setActiveLimit] = useState(DISPATCH_PAGE_SIZE);
  const [terminalLimit, setTerminalLimit] = useState(DISPATCH_PAGE_SIZE);
  // Reintentar re-drives the board through the effect-owned AbortController
  // instead of firing a bare, unsignalled load: an unsignalled retry can never
  // be recognised as cancelled (isDispatchLoadCanceled has no signal to test),
  // so an in-app abort landed on the board as "could not load" (#715).
  const [retryToken, setRetryToken] = useState(0);
  // Ref keeps the board-load callback identity stable when the business
  // timezone arrives asynchronously (avoids a skeleton flash + test races).
  const businessTimezoneRef = useRef<string | undefined>(undefined);
  useEffect(() => {
    businessTimezoneRef.current = businessTimezone;
  }, [businessTimezone]);
  const intervalRef = useRef<ReturnType<typeof setInterval> | null>(null);
  const tickRef = useRef<ReturnType<typeof setInterval> | null>(null);
  const showErrorRef = useRef(showError);
  const tStringRef = useRef(tString);
  showErrorRef.current = showError;
  tStringRef.current = tString;

  useEffect(() => {
    if (typeof window === "undefined" || !window.matchMedia) return;
    const mq = window.matchMedia(`(min-width: ${LG_BREAKPOINT_PX}px)`);
    const update = () => {
      const large = mq.matches;
      setIsLargeScreen(large);
      // The mobile StatusFilter is hidden on lg+, so a filter picked on mobile
      // would keep silently filtering the board after a resize with no visible
      // control to clear it. Reset to "all" when we cross into board mode.
      if (large) setFilter("all");
    };
    update();
    mq.addEventListener("change", update);
    return () => mq.removeEventListener("change", update);
  }, []);

  const load = useCallback(
    async (silent = false, signal?: AbortSignal) => {
      // The dashboard renders tabs before the business row resolves, so the
      // numeric id is briefly 0. Every sibling delivery read gates on
      // `businessId > 0` (useDeliveryQueries); the board must too, or the
      // unresolved-id request fails and latches the error board (#715).
      if (!(businessId > 0)) return;
      if (!silent) setLoading(true);
      else setRefreshing(true);
      try {
        // Two bounded queries replace the silent 200-row dump:
        // 1) all active statuses (no date filter — in-flight must stay visible)
        // 2) terminal statuses since business-day midnight
        const plan = buildDispatchBoardLoadPlan({
          activeLimit,
          terminalLimit,
          timeZone: businessTimezoneRef.current,
        });
        const [activeResult, terminalResult, availableDrivers] =
          await Promise.all([
            deliveryApi.getBusinessDeliveries(businessId, {
              status: plan.active.status,
              limit: plan.active.limit,
              signal,
            }),
            deliveryApi.getBusinessDeliveries(businessId, {
              status: plan.terminal.status,
              limit: plan.terminal.limit,
              since: plan.terminal.since,
              signal,
            }),
            deliveryApi.getAvailableDrivers(businessId, { signal }),
          ]);
        const merged = mergeDispatchBoardResults(
          activeResult ?? { deliveries: [], total: 0, has_more: false },
          terminalResult ?? { deliveries: [], total: 0, has_more: false },
        );
        setOrders(merged.orders);
        setActiveTotal(merged.activeTotal);
        setTerminalTotal(merged.terminalTotal);
        setActiveHasMore(merged.activeHasMore);
        setTerminalHasMore(merged.terminalHasMore);
        setDrivers(availableDrivers ?? []);
        setLastRefreshed(new Date());
        setSecondsAgo(0);
        setLoadError(false);
      } catch (err) {
        if (isDispatchLoadCanceled(err, signal)) {
          return;
        }
        if (!silent) {
          // R12: surface a visible error on initial load instead of silent empty board.
          setLoadError(true);
        } else {
          showErrorRef.current(tStringRef.current("dispatch.toasts.statusError"));
        }
      } finally {
        if (!signal?.aborted) {
          setLoading(false);
          setRefreshing(false);
        }
      }
    },
    [businessId, activeLimit, terminalLimit]
  );

  useEffect(() => {
    const ac = new AbortController();
    void load(false, ac.signal);
    return () => ac.abort();
  }, [load, retryToken]);

  // SSE reload coalescing (DEL-OP-5): a dinner-rush burst of delivery events
  // (each advance/assign/payment emits one) must not fire one full 200-row +
  // drivers reload per event. Leading edge keeps the immediate-refresh feel;
  // events inside the window collapse into a single trailing reload.
  const sseLastLoadAtRef = useRef(0);
  const sseTrailingTimerRef = useRef<ReturnType<typeof setTimeout> | null>(null);

  useEffect(() => {
    return () => {
      if (sseTrailingTimerRef.current != null) {
        clearTimeout(sseTrailingTimerRef.current);
        sseTrailingTimerRef.current = null;
      }
    };
  }, []);

  const handleSSEEvent = useCallback(
    (event: SSEEvent) => {
      if (
        event.type !== "delivery.created" &&
        event.type !== "delivery.updated" &&
        event.type !== "delivery.cancelled"
      ) {
        return;
      }
      // A trailing reload is already scheduled — this event is covered by it.
      if (sseTrailingTimerRef.current != null) return;
      const elapsed = Date.now() - sseLastLoadAtRef.current;
      if (elapsed >= SSE_COALESCE_MS) {
        sseLastLoadAtRef.current = Date.now();
        void load(true);
      } else {
        sseTrailingTimerRef.current = setTimeout(() => {
          sseTrailingTimerRef.current = null;
          sseLastLoadAtRef.current = Date.now();
          void load(true);
        }, SSE_COALESCE_MS - elapsed);
      }
    },
    [load]
  );

  // IMP-01: refetch the delivery list whenever the SSE connection comes back
  // after a drop. Any delivery.created/updated/cancelled events fired during
  // the outage are not buffered server-side, so a fresh load() is required.
  const handleSSEReconnect = useCallback(() => {
    void load(true);
  }, [load]);

  useSSEEvents({
    businessId,
    enabled: true,
    onEvent: handleSSEEvent,
    onReconnect: handleSSEReconnect,
  });

  // Prefer the server active total so poll cadence does not slow down exactly
  // when the board is truncated at peak volume. Fall back to the loaded window
  // only before the first successful response.
  const loadedActiveCount = orders.filter((o) => ACTIVE_STATUSES.has(o.status)).length;
  const activeCount = activeTotal > 0 ? activeTotal : loadedActiveCount;
  // Header count reflects what's on the board (excludes Bills-queue intake) so
  // it never disagrees with the visible columns. Polling still keys off
  // activeCount so an incoming pending order keeps the queue warm.
  const boardActiveCount = orders.filter((o) => BOARD_ACTIVE_STATUSES.has(o.status)).length;
  const boardTruncated = activeHasMore || terminalHasMore;

  const handleLoadMore = useCallback(() => {
    if (activeHasMore) {
      setActiveLimit((n) => n + DISPATCH_PAGE_SIZE);
    }
    if (terminalHasMore) {
      setTerminalLimit((n) => n + DISPATCH_PAGE_SIZE);
    }
  }, [activeHasMore, terminalHasMore]);

  // Auto-refresh: active deliveries refresh quickly; idle queues keep a slower heartbeat.
  // Polling pauses while the tab is hidden and resumes on visibility change.
  useEffect(() => {
    const refreshMs = activeCount > 0 ? ACTIVE_REFRESH_MS : IDLE_REFRESH_MS;
    const startPolling = () => {
      if (intervalRef.current) {
        clearInterval(intervalRef.current);
      }
      intervalRef.current = setInterval(() => {
        load(true);
      }, refreshMs);
    };
    const stopPolling = () => {
      if (intervalRef.current) {
        clearInterval(intervalRef.current);
        intervalRef.current = null;
      }
    };
    const handleVisibility = () => {
      if (document.hidden) {
        stopPolling();
      } else {
        startPolling();
      }
    };
    if (!document.hidden) {
      startPolling();
    }
    document.addEventListener("visibilitychange", handleVisibility);
    return () => {
      document.removeEventListener("visibilitychange", handleVisibility);
      stopPolling();
    };
  }, [activeCount, load]);

  // "Updated N s ago" ticker — increments every second after each fetch.
  useEffect(() => {
    if (tickRef.current) {
      clearInterval(tickRef.current);
      tickRef.current = null;
    }
    if (lastRefreshed !== null) {
      tickRef.current = setInterval(() => {
        setSecondsAgo(Math.round((Date.now() - lastRefreshed.getTime()) / 1000));
      }, 1000);
    }
    return () => {
      if (tickRef.current) {
        clearInterval(tickRef.current);
        tickRef.current = null;
      }
    };
  }, [lastRefreshed]);

  /** Force an immediate refresh without disabling the background heartbeat. */
  const handleManualRefresh = useCallback(() => {
    load(true);
  }, [load]);

  const toastClaimConflict = useCallback(
    (err: unknown, fallbackKey: string) => {
      const conflict = getDeliveryClaimConflict(err);
      if (conflict) {
        if (conflict.code === "claim_steal_required" && conflict.claimed_by_name) {
          showError(
            tString("dispatch.claim.stealRequired", {
              name: conflict.claimed_by_name,
            }),
          );
        } else if (conflict.claimed_by_name) {
          showError(
            tString("dispatch.claim.conflict", { name: conflict.claimed_by_name }),
          );
        } else {
          showError(tString("dispatch.claim.needClaim"));
        }
        return true;
      }
      showError(tString(fallbackKey));
      return false;
    },
    [showError, tString],
  );

  const handleAdvance = useCallback(
    async (orderId: number, nextStatus: DeliveryStatus) => {
      try {
        void operationalAlerts?.claimByResource(
          "delivery",
          orderId,
          "delivery_status_advance",
        );
        await deliveryApi.updateDeliveryOrderStatus(businessId, orderId, nextStatus);
        showSuccess(tString("dispatch.toasts.statusUpdated"));
        await load(true);
      } catch (err) {
        toastClaimConflict(err, "dispatch.toasts.statusError");
      }
    },
    [businessId, load, operationalAlerts, showSuccess, tString, toastClaimConflict]
  );

  const handleAssignDriver = useCallback(
    async (orderId: number, driverId: number) => {
      try {
        void operationalAlerts?.claimByResource(
          "delivery",
          orderId,
          "delivery_driver_assign",
        );
        await deliveryApi.assignDriver(businessId, orderId, driverId);
        showSuccess(tString("dispatch.toasts.driverAssigned"));
        await load(true);
      } catch (err) {
        toastClaimConflict(err, "dispatch.toasts.driverError");
        // Rethrow (mirroring handleCancel) so the inline assign panel stays
        // open and DriverAssignment keeps its selection instead of closing as
        // if the assignment succeeded (DEL-OP-4).
        throw err;
      }
    },
    [businessId, load, operationalAlerts, showSuccess, tString, toastClaimConflict]
  );

  const handleCancel = useCallback(
    async (reason: string) => {
      if (!cancelTarget) return;
      try {
        void operationalAlerts?.claimByResource(
          "delivery",
          cancelTarget.id,
          "delivery_cancel_confirm",
        );
        await deliveryApi.cancelDeliveryOrder(businessId, cancelTarget.id, reason);
        showSuccess(tString("dispatch.toasts.cancelled"));
        setCancelTarget(null);
        await load(true);
      } catch (err) {
        toastClaimConflict(err, "dispatch.toasts.cancelError");
        // Rethrow so the modal keeps itself open with the typed reason intact
        // instead of clearing on a failed cancel and showing a false success.
        throw err;
      }
    },
    [businessId, cancelTarget, load, operationalAlerts, showSuccess, tString, toastClaimConflict]
  );

  const handleClaim = useCallback(
    async (o: DispatchOrder, opts?: { steal?: boolean }) => {
      try {
        await deliveryApi.claimDeliveryOrder(businessId, o.id, {
          steal: opts?.steal,
        });
        if (opts?.steal && o.claimed_by_name) {
          showSuccess(
            tString("dispatch.claim.stolenToast", { name: o.claimed_by_name }),
          );
        }
        await load(true);
      } catch (err) {
        const conflict = getDeliveryClaimConflict(err);
        if (conflict?.code === "claim_steal_required" && canSteal && !opts?.steal) {
          setStealTarget({
            order: o,
            name: conflict.claimed_by_name || "?",
          });
          return;
        }
        toastClaimConflict(err, "dispatch.claim.claimFailed");
      }
    },
    [businessId, canSteal, load, showSuccess, tString, toastClaimConflict],
  );

  const handleRelease = useCallback(
    async (o: DispatchOrder) => {
      try {
        await deliveryApi.releaseDeliveryOrder(businessId, o.id);
        await load(true);
      } catch (err) {
        toastClaimConflict(err, "dispatch.claim.releaseFailed");
      }
    },
    [businessId, load, toastClaimConflict],
  );

  const availableDriverCount = drivers.length;
  // Terminal orders arrive already since-bounded from the server. Keep a
  // client-side same-day guard as a belt-and-suspenders for clock skew / zone
  // mismatch; active orders are never date-filtered.
  const visibleOrders = sortOrders(orders)
    .filter(
      (o) =>
        !TERMINAL_STATUSES.has(o.status) ||
        isSameBusinessDay(o.updated_at ?? o.created_at, businessTimezone)
    )
    .filter((o) => matchesFilter(o, filter));

  // Map DeliveryOrder -> DispatchOrder, then re-route advance/cancel back to the existing handlers.
  const boardOrders: DispatchOrder[] = visibleOrders.map((o) => {
    const expiresAt = o.payment_expires_at ?? undefined;
    const awaitingPayment =
      o.status === "confirmed" &&
      !!expiresAt &&
      new Date(expiresAt).getTime() > Date.now();
    return {
      id: o.id,
      code: o.delivery_number,
      status: o.status,
      customer_name: o.customer_name,
      customer_phone: o.customer_phone || undefined,
      address: formatDispatchAddress(o) || undefined,
      // Guest ETA is the primary clock; cutoff is urgency-only and must not
      // masquerade as ETA (hours-close cutoff ≈ order time looked broken).
      eta: o.estimated_delivery_time ?? undefined,
      cutoff: o.cutoff_at ?? undefined,
      fee: typeof o.delivery_fee === "number" ? o.delivery_fee : Number(o.delivery_fee ?? 0),
      order_total:
        typeof o.total === "number" && Number.isFinite(o.total)
          ? o.total
          : quoteNumber(o.quote_metadata, "order_subtotal"),
      item_count: quoteNumber(o.quote_metadata, "item_count"),
      awaiting_payment: awaitingPayment,
      payment_expires_at: expiresAt,
      driver: o.driver
        ? {
            name: o.driver.name ?? o.driver.email ?? "Driver",
            live: false,
            eta: o.estimated_delivery_time ?? undefined,
          }
        : null,
      claimed_by_staff_id: o.claimed_by_staff_id,
      claimed_by_name: o.claimed_by_name,
      claimed_by_role: o.claimed_by_role,
      claimed_at: o.claimed_at,
    };
  });

  const handleBoardAdvance = useCallback(
    (o: DispatchOrder) => {
      // A ready order with a driver advances straight to picked_up — the
      // backend `assigned` step is set by the assign endpoint, not by the
      // board advance action. Intake (pending/confirmed) has no operator advance.
      const next =
        o.status === "ready" && o.driver != null
          ? ("picked_up" as DeliveryStatus)
          : getOperatorNextStatus(o.status);
      if (!next) return;
      // Return the promise so DispatchCard can re-enable its advance button
      // once the request settles (DEL-OP-1).
      return handleAdvance(o.id, next);
    },
    [handleAdvance]
  );

  const handleBoardCancel = useCallback(
    (o: DispatchOrder) => {
      const original = orders.find((x) => x.id === o.id);
      if (original) {
        void operationalAlerts?.claimByResource(
          "delivery",
          original.id,
          "delivery_cancel_open",
        );
        setCancelTarget(original);
      }
    },
    [operationalAlerts, orders]
  );

  if (loading) {
    return (
      <div className="space-y-3">
        {[1, 2, 3].map((i) => (
          <Skeleton key={i} className="rounded-xl h-20 w-full" />
        ))}
      </div>
    );
  }

  // R12: visible fetch error with retry button instead of silent empty board.
  if (loadError) {
    return (
      <div
        data-testid="dispatch-load-error"
        className="rounded-xl border border-red-200 bg-red-50 p-6 flex flex-col items-center gap-3 text-center"
      >
        <AlertCircle className="w-8 h-8 text-red-400" />
        <p className="text-sm text-red-700">{tString("dispatch.loadError")}</p>
        <Button
          size="sm"
          color="danger"
          variant="flat"
          onPress={() => {
            setLoadError(false);
            setRetryToken((n) => n + 1);
          }}
        >
          {tString("dispatch.retry")}
        </Button>
      </div>
    );
  }

  return (
    <section aria-labelledby="dispatch-console-heading" className="space-y-4">
      {/* Visually hidden heading for screen readers */}
      <h2 id="dispatch-console-heading" className="sr-only">
        {tString("dispatch.heading") || "Dispatch Console"}
      </h2>

      {/* Header stats + refresh */}
      <div className="flex items-center justify-between flex-wrap gap-2">
        <div className="flex items-center gap-4 text-sm text-ink-700">
          <span>
            {boardActiveCount === 1
              ? tString("dispatch.header.activeOrders", { count: String(boardActiveCount) })
              : tString("dispatch.header.activeOrdersPlural", { count: String(boardActiveCount) })}
          </span>
          <span>
            {availableDriverCount === 1
              ? tString("dispatch.header.availableDrivers", {
                  count: String(availableDriverCount),
                })
              : tString("dispatch.header.availableDriversPlural", {
                  count: String(availableDriverCount),
                })}
          </span>
          {lastRefreshed !== null && (
            <span
              className="text-xs text-ink-500"
              aria-label={tString("dispatch.freshness.updatedAria", {
                seconds: String(secondsAgo),
              })}
            >
              {tString("dispatch.freshness.updated", {
                seconds: String(secondsAgo),
              })}
            </span>
          )}
        </div>
        <Button
          size="sm"
          variant="light"
          aria-label={tString("dispatch.actions.refresh") || "Refresh orders"}
          startContent={<RefreshCw size={14} className={refreshing ? "animate-spin" : ""} />}
          onPress={handleManualRefresh}
          isDisabled={refreshing}
        >
          {tString("dispatch.actions.refresh")}
        </Button>
      </div>

      {/* Explicit truncation banner — never silently drop in-flight orders. */}
      {boardTruncated && (
        <div
          data-testid="dispatch-truncation-banner"
          className="flex flex-wrap items-center justify-between gap-3 rounded-xl border border-amber-200 bg-amber-50 px-4 py-3 text-sm text-amber-900"
          role="status"
        >
          <span>
            {tString("dispatch.truncation.showing", {
              shown: String(orders.length),
              // Server totals for BOTH windows — loaded rows only as a
              // pre-first-response fallback.
              total: String(
                (activeTotal || loadedActiveCount) +
                  (terminalTotal ||
                    orders.filter((o) => TERMINAL_STATUSES.has(o.status))
                      .length),
              ),
            })}
          </span>
          <Button
            size="sm"
            color="warning"
            variant="flat"
            data-testid="dispatch-load-more"
            onPress={handleLoadMore}
            isDisabled={refreshing}
          >
            {tString("dispatch.truncation.loadMore")}
          </Button>
        </div>
      )}

      {/* Filter chips — list view only; board has its own terminal filter strip below. */}
      {!isLargeScreen && (
        <StatusFilter active={filter} onChange={setFilter} tString={tString} />
      )}

      {/* Order view: KDS board on lg+ even when empty so Out for delivery /
          Delivered stay reachable at 1024 (issue 238). List view keeps the
          illustration empty state. */}
      {isLargeScreen ? (
        <>
          {/* Board filter strip — happy path lanes + Cancelled / Failed */}
          <div
            className="flex flex-wrap gap-2"
            role="tablist"
            aria-label={tString("dispatch.filters.label") || "View"}
          >
            {BOARD_FILTER_OPTIONS.map(({ key, labelKey }) => {
              const isActive = boardFilter === key;
              const label = tString(labelKey);
              return (
                <button
                  key={String(key)}
                  role="tab"
                  aria-selected={isActive}
                  aria-label={label}
                  onClick={() => setBoardFilter(key)}
                  className="focus:outline-none focus-visible:ring-2 focus-visible:ring-offset-1 focus-visible:ring-primary rounded-full"
                >
                  <Chip
                    variant={isActive ? "solid" : "flat"}
                    color={isActive ? "primary" : "default"}
                    classNames={{ base: "cursor-pointer transition-colors" }}
                  >
                    {label}
                  </Chip>
                </button>
              );
            })}
          </div>

          <DispatchBoard
            orders={boardOrders}
            onAdvance={handleBoardAdvance}
            onCancel={handleBoardCancel}
            onOpenDetail={(o) => setDetailOrderId(o.id)}
            onAssignDriver={handleAssignDriver}
            onClaim={handleClaim}
            onRelease={handleRelease}
            myStaffId={myStaffId}
            canSteal={canSteal}
            drivers={drivers}
            currency={businessCurrency}
            businessTimezone={businessTimezone ?? null}
            boardFilter={boardFilter}
          />
          {boardOrders.length === 0 ? (
            <p
              className="mt-2 text-sm text-ink-500"
              data-testid="dispatch-empty-caption"
            >
              {tString("dispatch.emptyBoard.title") ||
                tDash("delivery.empty.title")}
            </p>
          ) : null}
        </>
      ) : visibleOrders.length === 0 ? (
        <DispatchEmptyPanel
          title={tString("dispatch.emptyBoard.title") || tDash("delivery.empty.title")}
          subtitle={
            tString("dispatch.emptyBoard.subtitle") ||
            tDash("delivery.empty.subtitle")
          }
        />
      ) : (
        <div className="space-y-3" role="list" aria-label={tString("dispatch.orderList") || "Delivery orders"}>
          {visibleOrders.map((order) => (
            <OrderRow
              key={order.id}
              order={order}
              drivers={drivers}
              onAdvance={handleAdvance}
              onAssignDriver={handleAssignDriver}
              onOpenDetail={(o) => setDetailOrderId(o.id)}
              onCancel={(order) => {
                void operationalAlerts?.claimByResource(
                  "delivery",
                  order.id,
                  "delivery_cancel_open",
                );
                setCancelTarget(order);
              }}
              tString={tString}
              currency={businessCurrency}
              locale={locale}
              businessTimezone={businessTimezone ?? null}
            />
          ))}
        </div>
      )}

      <OrderDetailDrawer
        businessId={businessId}
        orderId={detailOrderId}
        isOpen={detailOrderId != null}
        onClose={() => setDetailOrderId(null)}
        drivers={drivers}
        onAdvance={handleAdvance}
        onAssignDriver={handleAssignDriver}
        onCancel={(order) => {
          void operationalAlerts?.claimByResource(
            "delivery",
            order.id,
            "delivery_cancel_open",
          );
          setCancelTarget(order);
        }}
        tString={tString}
        currency={businessCurrency}
        locale={locale}
        businessTimezone={businessTimezone ?? null}
        onMutated={() => void load(true)}
      />

      {/* Cancel modal — paid-cancel warning for preparing+ statuses */}
      <CancelDeliveryModal
        isOpen={cancelTarget !== null}
        onClose={() => setCancelTarget(null)}
        onConfirm={handleCancel}
        tString={tString}
        showPaidWarning={
          cancelTarget !== null && PAID_CANCEL_WARN_STATUSES.has(cancelTarget.status)
        }
      />

      <ConfirmationModal
        isOpen={stealTarget !== null}
        onOpenChange={() => setStealTarget(null)}
        title={tString("dispatch.claim.steal")}
        description={tString("dispatch.claim.stealRequired", {
          name: stealTarget?.name ?? "",
        })}
        confirmLabel={tString("dispatch.claim.steal")}
        cancelLabel={tString("dispatch.claim.cancel")}
        isDanger
        onConfirm={async () => {
          if (!stealTarget) return;
          const target = stealTarget;
          setStealTarget(null);
          await handleClaim(target.order, { steal: true });
        }}
      />
    </section>
  );
}
