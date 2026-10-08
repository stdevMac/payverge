"use client";

import React, { useState, useEffect, useCallback, useRef } from "react";
import { useRouter, usePathname, useSearchParams } from "next/navigation";
import {
  Button,
  Modal,
  ModalContent,
  ModalHeader,
  ModalBody,
  ModalFooter,
  Textarea,
} from "@nextui-org/react";
import {
  ChefHat,
  Clock,
  Users,
  CheckCircle,
  Play,
  StickyNote,
  RefreshCw,
} from "lucide-react";
import toast from "react-hot-toast";
import {
  getAllActiveOrders,
  cancelOrder,
  Order,
} from "../../api/orders";
import { physicalItemQuantity, toFulfillmentLines } from "@/utils/orderFulfillment";
import {
  getBusiness,
  getMenu,
  MenuCategory,
  MenuItem,
  updateMenuItem,
} from "../../api/business";
import { formatCurrency as formatCurrencyIntl } from "../../api/currency";
import { StatusChip } from "../ui/StatusChip";
import {
  useSimpleLocale,
  getTranslation,
} from "@/i18n/SimpleTranslationProvider";
import { useBusinessAccess } from "@/hooks/useBusinessAccess";
import { useSessionTabState } from "@/hooks/useSessionTabState";
import DashboardLockedTabView from "./DashboardLockedTabView";
import { localizedErrorMessage } from "@/utils/localizedError";
import { sortOrdersFifo } from "@/utils/orderSorting";
import { useSSEEvents, SSEEvent } from "@/hooks/useSSEEvents";
import { KitchenOrdersToggle } from "./KitchenOrdersToggle";
import KitchenDisplayMode from "./KitchenDisplayMode";
import { KitchenSkeleton } from "./KitchenSkeleton";
import {
  parentOwnsKitchenOrders,
  shouldShowKitchenOrderCapBanner,
} from "./kitchenOrderCapBanner";
import { Maximize } from "lucide-react";
import { useQueryClient } from "@tanstack/react-query";
import { queryKeys } from "../../api/queryKeys";
import { useAuth } from "@/providers/HybridAuthProvider";
import { getKitchenCapabilities } from "@/utils/staffAuth";
import { useStaffPermissionsContext } from "@/contexts/StaffPermissionsContext";
import { hasPerm } from "@/constants/permissions";
import OperationalAlertClaimStatus from "./operational-alerts/OperationalAlertClaimStatus";
import { useOptionalOperationalAlerts } from "./operational-alerts/useOperationalAlerts";
import DashboardTabShell from "./shared/DashboardTabShell";
import { EmptyState } from "@/components/ui/EmptyState";
import {
  btnGhostIcon,
  btnPrimary,
  btnPrimaryNextUI,
} from "@/components/ui/buttonStyles";
import { PremiumPanel } from "./premium";
import { orderStatusTranslationKey } from "./kitchenStatusLabels";
import { KITCHEN_TICKET_IDENTITY_SEP } from "./kitchenTicketIdentity";
import { menuItemManualAvailable } from "./MenuBuilder/menuAvailabilityFilter";
import { KitchenAllergenChips } from "./KitchenAllergenChips";
import {
  kitchenItemsToPrepareLabel,
  kitchenNoteLabels,
  localizeKitchenOrderNote,
} from "@/utils/kitchenTicketCopy";
import { countKey } from "@/i18n/countForm";
import {
  formatKdsFireTime,
  kdsElapsedMinutes,
  kdsElapsedUrgency,
} from "@/utils/kdsTicketTime";
import { humanizeDurationMinutes } from "@/utils/humanizeDuration";
import ConfirmationModal from "./modals/ConfirmationModal";
import {
  kitchenBoardGate,
  kitchenEnabledFromBusiness,
} from "./kitchenEnabled";

type MenuItemRef = {
  categoryId: string;
  categoryIndex: number;
  itemIndex: number;
  itemId: string;
  item: MenuItem;
  available: boolean;
};

interface KitchenProps {
  businessId: number;
  globalOrders?: Record<number, Order[]>;
  globalOrdersCapped?: boolean;
  /** K-1: true once the dashboard's first global-orders poll completed. */
  globalOrdersLoaded?: boolean;
  /** null = status not resolved yet — never treat as disabled. */
  kitchenEnabled: boolean | null;
  kitchenStatusLoading: boolean;
  onKitchenStatusChange: (enabled: boolean) => void;
  onOrderStatusChange: (
    orderId: number,
    newStatus: string,
    approvedBy?: string,
    reason?: string,
  ) => Promise<void>;
  /** Venue IANA zone from the already-loaded business row. */
  businessTimezone?: string | null;
}

export function isTerminalKitchenOrder(status: string): boolean {
  return status === "delivered" || status === "cancelled";
}

const Kitchen: React.FC<KitchenProps> = ({
  businessId,
  globalOrders,
  globalOrdersCapped: _globalOrdersCapped = false,
  globalOrdersLoaded = false,
  kitchenEnabled: kitchenOrdersEnabled,
  kitchenStatusLoading: kitchenOrdersLoading,
  onKitchenStatusChange,
  onOrderStatusChange,
  businessTimezone: businessTimezoneProp = null,
}) => {
  // Translation setup
  const { locale } = useSimpleLocale();
  const [currentLocale, setCurrentLocale] = useState(locale);
  const operationalAlerts = useOptionalOperationalAlerts();
  const router = useRouter();
  const pathname = usePathname();

  // Operational lock (admin suspend/close) and RBAC access
  const {
    hasAccess,
    isSuspended,
    loading: accessLoading,
  } = useBusinessAccess(businessId);

  // RBAC capabilities — settings:write activates kitchen/orders; orders:status moves tickets.
  // Owners (non-staff) get full kitchen control; staff use effective permissions from context.
  const { isStaffUser } = useAuth();
  const { permissions } = useStaffPermissionsContext();
  const kitchenCaps = isStaffUser
    ? getKitchenCapabilities(permissions)
    : { canActivate: true, canUpdateOrderStatus: true };
  // Wave 4: one-tap 86 is menu:items gated (owners always).
  const canEightySix = !isStaffUser || hasPerm(permissions, "menu:items");

  // React Query client for cache invalidation
  const queryClient = useQueryClient();

  // Update translations when locale changes
  useEffect(() => {
    setCurrentLocale(locale);
  }, [locale]);

  // Translation helper
  const tString = useCallback(
    (key: string): string => {
      const fullKey = `businessDashboard.dashboard.kitchenManager.${key}`;
      const result = getTranslation(fullKey, currentLocale);
      return Array.isArray(result) ? result[0] || key : (result as string);
    },
    [currentLocale],
  );

  const formatKitchenNote = useCallback(
    (note: string) =>
      localizeKitchenOrderNote(note, kitchenNoteLabels(currentLocale)),
    [currentLocale],
  );

  const [orders, setOrders] = useState<Order[]>([]);
  // Brand-new (pending) orders are otherwise invisible in the Kitchen tab — a
  // kitchen-tab-only operator would miss them until someone approved them from
  // Bills. Surfaced in a "Needs approval" strip at the top of the board.
  const [pendingOrders, setPendingOrders] = useState<Order[]>([]);
  const [loading, setLoading] = useState(true);
  const [selectedOrder, setSelectedOrder] = useState<Order | null>(null);
  const [showOrderModal, setShowOrderModal] = useState(false);
  // M6: queue tab + list/KDS view survive rail switches for the browser-tab
  // session (a cook mid-service should land back on their queue), then reset.
  const [activeTab, setActiveTab] = useSessionTabState<string>(
    businessId ? `payverge_kitchen_status:${businessId}` : null,
    "approved",
    (v) => ["approved", "in_kitchen", "ready", "all"].includes(String(v)),
  );
  // #794: table rows deep-link leftover-kitchen tickets via
  // ?tab=kitchen&kitchenStatus=<queue>. Honor the param once on landing
  // (over the session-restored queue), then strip it so refreshes and the
  // session state win afterward. Invalid values are ignored.
  const searchParams = useSearchParams();
  const deepLinkKitchenStatus = searchParams?.get("kitchenStatus") ?? null;
  const deepLinkAppliedRef = useRef(false);
  useEffect(() => {
    if (deepLinkAppliedRef.current || !deepLinkKitchenStatus) return;
    deepLinkAppliedRef.current = true;
    if (["approved", "in_kitchen", "ready", "all"].includes(deepLinkKitchenStatus)) {
      setActiveTab(deepLinkKitchenStatus);
    }
    if (typeof window === "undefined") return;
    const url = new URL(window.location.href);
    if (!url.searchParams.has("kitchenStatus")) return;
    url.searchParams.delete("kitchenStatus");
    const nextUrl =
      url.pathname + (url.searchParams.toString() ? `?${url.searchParams}` : "");
    router.replace(nextUrl, { scroll: false });
  }, [deepLinkKitchenStatus, setActiveTab, router]);
  const [viewMode, setViewMode] = useSessionTabState<"list" | "kds">(
    businessId ? `payverge_kitchen_viewmode:${businessId}` : null,
    "list",
    (v) => v === "list" || v === "kds",
  );
  const [actionLoading, setActionLoading] = useState<number | null>(null);
  const [fallbackOrdersCapped, setFallbackOrdersCapped] = useState(false);
  const [cancelModalOrder, setCancelModalOrder] = useState<Order | null>(null);
  const [cancelReason, setCancelReason] = useState("");
  const [cancelLoading, setCancelLoading] = useState(false);
  // Used to format option-price-changes and per-item subtotals shown in
  // the order details modal. Default USD until the business loads.
  const [businessCurrency, setBusinessCurrency] = useState<string>("USD");
  const [businessTimezone, setBusinessTimezone] = useState<string | null>(
    businessTimezoneProp,
  );
  // #662: `resolveBusinessTimeZone(null)` collapses "venue zone unknown" into
  // "UTC", so any ticket painted before the venue zone is known shows a
  // wrong-but-plausible fire time. Owners get the zone as a prop (settled on
  // the first render); staff reach this tab through the dashboard's
  // synthesized staff business row, which carries no timezone, so the lookup
  // below is the only source. Track when that has settled — success OR
  // failure — and hold the existing skeleton until it does.
  const [venueTimeZoneSettled, setVenueTimeZoneSettled] = useState<boolean>(
    Boolean(businessTimezoneProp),
  );
  // IMP-13: menu_item_id → allergens lookup. Sourced from the active menu
  // so the kitchen card can surface allergen pills next to modifiers + notes.
  // Empty map = fail-open; rendering tolerates missing entries.
  const [allergensByItemId, setAllergensByItemId] = useState<
    Record<string, string[]>
  >({});
  // Wave 4: menu_item_id → identity + version for one-tap 86 (CAS via version).
  const [menuVersion, setMenuVersion] = useState<number | undefined>(undefined);
  const [menuItemIndex, setMenuItemIndex] = useState<
    Record<string, MenuItemRef>
  >({});
  const [pending86, setPending86] = useState<MenuItemRef | null>(null);
  const [eightySixBusy, setEightySixBusy] = useState(false);
  // Coarse wall-clock tick so elapsed labels + >15/>30-min urgency colors keep
  // advancing between poll/SSE refreshes (mirrors KitchenDisplayMode's clock,
  // but at 30s to avoid re-render churn since the list is card-dense). Elapsed
  // math + threshold checks read this `now` instead of Date.now() directly.
  const [now, setNow] = useState(() => Date.now());

  useEffect(() => {
    if (businessTimezoneProp) {
      setBusinessTimezone(businessTimezoneProp);
      setVenueTimeZoneSettled(true);
    }
  }, [businessTimezoneProp]);

  useEffect(() => {
    let cancelled = false;
    if (!businessId) {
      // Nothing left to learn — never strand the tab on the skeleton.
      setVenueTimeZoneSettled(true);
      return;
    }
    getBusiness(businessId)
      .then((biz) => {
        if (!cancelled) {
          if (biz?.default_currency) setBusinessCurrency(biz.default_currency);
          if (!businessTimezoneProp) {
            setBusinessTimezone(biz?.timezone ?? null);
          }
          // Staff land here with a synthesized dashboard row that has no
          // kitchen flags. This fetch (staff projection includes them) is
          // the first chance to seed ON and skip the false "kitchen off"
          // empty state on a hard ?tab=kitchen load (#663).
          if (kitchenEnabledFromBusiness(biz) === true) {
            onKitchenStatusChange(true);
          }
        }
      })
      .catch(() => {})
      .finally(() => {
        // Settled either way: a failed lookup falls back to the documented UTC
        // behavior rather than leaving the board permanently skeletonized.
        if (!cancelled) setVenueTimeZoneSettled(true);
      });
    return () => {
      cancelled = true;
    };
  }, [businessId, businessTimezoneProp, onKitchenStatusChange]);

  useEffect(() => {
    let cancelled = false;
    if (!businessId) return;
    getMenu(businessId)
      .then((menu) => {
        if (cancelled) return;
        const raw = menu?.parsed_categories ?? menu?.categories;
        const cats: MenuCategory[] = Array.isArray(raw)
          ? raw
          : typeof raw === "string"
            ? (() => {
                try {
                  return JSON.parse(raw) as MenuCategory[];
                } catch {
                  return [];
                }
              })()
            : [];
        const map: Record<string, string[]> = {};
        const index: Record<string, MenuItemRef> = {};
        cats.forEach((cat, categoryIndex) => {
          (cat.items || []).forEach((item, itemIndex) => {
            if (item.id && item.allergens && item.allergens.length > 0) {
              map[item.id] = item.allergens;
            }
            if (!item.id) return;
            index[item.id] = {
              categoryId: (cat as { id?: string }).id ?? "",
              categoryIndex,
              itemIndex,
              itemId: item.id,
              item,
              // #727: the operator menu reports is_available as the effective
              // flag, so an out-of-stock dish reads false there while the
              // operator's own 86 switch is still on. The Kitchen control
              // writes that switch, so it has to read the switch.
              available: menuItemManualAvailable(item),
            };
          });
        });
        setAllergensByItemId(map);
        setMenuItemIndex(index);
        setMenuVersion(menu?.version);
      })
      .catch(() => {
        // Fail-open: kitchen still functions without allergen pills / 86.
      });
    return () => {
      cancelled = true;
    };
  }, [businessId]);

  // Coarse 30s tick — keeps elapsed labels + age-urgency colors moving between
  // data refreshes so a threshold crossing (>15m / >30m) doesn't freeze until
  // the next poll. Cleared on unmount.
  useEffect(() => {
    const timer = setInterval(() => setNow(Date.now()), 30000);
    return () => clearInterval(timer);
  }, []);

  const formatCurrency = (amount: number) =>
    formatCurrencyIntl(amount, businessCurrency, undefined, currentLocale);
  const kitchenFeatureReady =
    !accessLoading &&
    hasAccess &&
    !isSuspended &&
    kitchenOrdersEnabled === true;
  const canUpdateOrderStatus = kitchenCaps.canUpdateOrderStatus;

  const confirmEightySix = async () => {
    if (!pending86) return;
    setEightySixBusy(true);
    const nextAvailable = !pending86.available;
    try {
      const response = await updateMenuItem(
        businessId,
        pending86.categoryIndex,
        pending86.itemIndex,
        { ...pending86.item, is_available: nextAvailable, manual_available: nextAvailable },
        menuVersion,
        pending86.categoryId,
        pending86.itemId,
      );
      setMenuVersion(response.version);
      setMenuItemIndex((prev) => ({
        ...prev,
        [pending86.itemId]: { ...pending86, available: nextAvailable },
      }));
      toast.success(
        tString(nextAvailable ? "eightySix.restoreSuccess" : "eightySix.success").replace(
          "{name}",
          pending86.item.name,
        ),
      );
    } catch (error) {
      // 409 = menu version drifted (someone edited in Menu Builder); refetch
      // resolves the index+version on the next load — surface and move on.
      toast.error(
        tString("eightySix.error").replace(
          "{message}",
          localizedErrorMessage(error, currentLocale),
        ),
      );
    } finally {
      setEightySixBusy(false);
      setPending86(null);
    }
  };

  const orderCounts = React.useMemo(() => {
    const counts = orders.reduce(
      (acc, order) => {
        if (order.status === "approved") acc.approved += 1;
        if (order.status === "in_kitchen") acc.inKitchen += 1;
        if (order.status === "ready") acc.ready += 1;
        acc.all += 1;
        return acc;
      },
      { approved: 0, inKitchen: 0, ready: 0, all: 0 },
    );
    // All Orders includes the Needs-approval queue so the tab never lies
    // empty while a pending ticket is visible in the strip / Overview.
    counts.all += pendingOrders.length;
    return counts;
  }, [orders, pendingOrders]);

  const hasLoadedOnceRef = useRef(false);

  const loadOrders = useCallback(
    async (isInitialLoad = false) => {
      // Embedded mode: trust the parent's data once its first poll completed —
      // INCLUDING "loaded and empty". The old code fell through to a redundant
      // per-tick API call whenever the map was empty, and that path flipped
      // `loading` on every tick (K-1: the skeleton swap unmounted
      // KitchenDisplayMode and dropped fullscreen).
      const hasEmbeddedData =
        !!globalOrders &&
        (globalOrdersLoaded || Object.keys(globalOrders).length > 0);

      if (hasEmbeddedData) {
        setFallbackOrdersCapped(false);
        const allOrders = Object.values(globalOrders).flat();

        // Kitchen board shows approved+ orders; pending orders feed the
        // "Needs approval" strip so they're not silently dropped.
        setOrders(
          allOrders.filter((order) =>
            ["approved", "in_kitchen", "ready", "delivered", "cancelled"].includes(
              order.status,
            ),
          ),
        );
        setPendingOrders(
          allOrders.filter((order) => order.status === "pending"),
        );
        setLoading(false);
        return;
      }

      // Standalone fallback (no parent data). `loading` may only flip before
      // the first successful load — background refreshes reuse the mounted UI.
      if (isInitialLoad) {
        setLoading(true);
      }

      try {
        const data = await getAllActiveOrders(businessId, {
          statuses: [
            "pending",
            "approved",
            "in_kitchen",
            "ready",
            "delivered",
            "cancelled",
          ],
          activeBillsOnly: true,
        });
        const ordersData = data.items;
        if (data.capped) {
          console.warn("Kitchen order live data cap reached", data.warning);
        }
        setFallbackOrdersCapped(data.capped);

        setOrders(
          ordersData.filter((order) =>
            ["approved", "in_kitchen", "ready", "delivered", "cancelled"].includes(
              order.status,
            ),
          ),
        );
        setPendingOrders(
          ordersData.filter((order) => order.status === "pending"),
        );
      } catch (error) {
        console.error("❌ [Kitchen] Error loading orders:", error);
      } finally {
        if (isInitialLoad) {
          setLoading(false);
        }
      }
    },
    [businessId, globalOrders, globalOrdersLoaded],
  );

  // Derive displayed orders from the active tab filter. Memoized so the parent's
  // 1s clock tick (and other unrelated re-renders) don't re-filter + re-sort the
  // full order set 3x per render. All Orders merges the pending queue so a
  // ticket called out in Needs approval is never missing from the board.
  const filteredOrders = React.useMemo(
    () =>
      sortOrdersFifo(
        activeTab === "all"
          ? [...orders, ...pendingOrders]
          : orders.filter((order) => order.status === activeTab),
      ),
    [orders, pendingOrders, activeTab],
  );
  // Parent dashboard already renders the global live-order cap banner whenever
  // it owns the order map (even an empty {}). Kitchen may only repeat that copy
  // in true standalone mode — otherwise the kitchen tab stacks the same string.
  const parentOwnsOrders = parentOwnsKitchenOrders(globalOrders);
  const showOrderCapWarning = shouldShowKitchenOrderCapBanner({
    parentOwnsOrders,
    fallbackCapped: fallbackOrdersCapped,
  });

  // Parse each order's items JSON ONCE per data change. Previously every card
  // called toFulfillmentLines(order.items) 3x per render (list + "+N more" +
  // count) and physicalItemQuantity(order.items) again — re-parsing the same
  // JSON blob on every 30s tick and every unrelated re-render.
  const fulfillmentByOrderId = React.useMemo(() => {
    const map = new Map<
      number,
      ReturnType<typeof toFulfillmentLines>
    >();
    for (const order of [...orders, ...pendingOrders]) {
      map.set(order.id, toFulfillmentLines(order.items));
    }
    return map;
  }, [orders, pendingOrders]);

  const updateOrderStatusHandler = async (orderId: number, status: string) => {
    if (!canUpdateOrderStatus) return;

    void operationalAlerts?.claimByResource(
      "order",
      orderId,
      "kitchen_status_change",
    );
    const previousOrders = orders;
    const previousPending = pendingOrders;
    const nextStatus = status as Order["status"];
    // Keep the ticket on the board immediately so "Comenzar a Cocinar" cannot
    // drop it from Aprobados without landing it in En Cocina.
    setOrders((prev) =>
      prev.map((o) => (o.id === orderId ? { ...o, status: nextStatus } : o)),
    );
    setPendingOrders((prev) => prev.filter((o) => o.id !== orderId));
    if (nextStatus === "in_kitchen") setActiveTab("in_kitchen");
    setActionLoading(orderId);
    try {
      await onOrderStatusChange(orderId, status, "kitchen");
      setShowOrderModal(false);
    } catch (error) {
      setOrders(previousOrders);
      setPendingOrders(previousPending);
      console.error("Error updating order status:", error);
      toast.error(
        tString("failedToUpdateStatus").replace(
          "{message}",
          localizedErrorMessage(error, currentLocale),
        ),
      );
    } finally {
      setActionLoading(null);
    }
  };

  // Approve a pending order — reuses the same status-change endpoint the Bills
  // approval flow uses (onOrderStatusChange(..., "approved", ...)). On success we
  // optimistically drop it from the strip; the next poll / SSE tick reconciles.
  const approvePendingOrder = async (orderId: number) => {
    if (!canUpdateOrderStatus) return;
    void operationalAlerts?.claimByResource(
      "order",
      orderId,
      "kitchen_approve",
    );
    setActionLoading(orderId);
    try {
      await onOrderStatusChange(orderId, "approved", "kitchen");
      setPendingOrders((prev) => prev.filter((o) => o.id !== orderId));
    } catch (error) {
      console.error("Error approving order:", error);
      toast.error(
        tString("needsApproval.approveError").replace(
          "{message}",
          localizedErrorMessage(error, currentLocale),
        ),
      );
    } finally {
      setActionLoading(null);
    }
  };

  const openCancelModal = (order: Order) => {
    void operationalAlerts?.claimByResource(
      "order",
      order.id,
      "kitchen_cancel_open",
    );
    setCancelModalOrder(order);
    setCancelReason("");
  };

  const handleCancelOrder = async () => {
    if (!cancelModalOrder || !cancelReason.trim()) return;
    setCancelLoading(true);
    try {
      await cancelOrder(businessId, cancelModalOrder.id, cancelReason.trim());
      // Optimistically update local state
      setOrders((prev) =>
        prev.map((o) =>
          o.id === cancelModalOrder.id
            ? {
                ...o,
                status: "cancelled" as Order["status"],
                cancel_reason: cancelReason.trim(),
              }
            : o,
        ),
      );
      setCancelModalOrder(null);
      setCancelReason("");
      // Invalidate caches
      queryClient.invalidateQueries({ queryKey: queryKeys.orders.all });
      queryClient.invalidateQueries({
        queryKey: queryKeys.notifications.dashboard(businessId.toString()),
      });
    } catch (error) {
      toast.error(
        tString("failedToCancelOrder").replace(
          "{message}",
          localizedErrorMessage(error, currentLocale),
        ),
      );
    } finally {
      setCancelLoading(false);
    }
  };

  // Initial load + reload whenever the parent's order map actually changes
  // (the dashboard only swaps the reference on real changes — K-1). The
  // skeleton is reserved for the very first load; every subsequent run is a
  // background refresh against the mounted board.
  useEffect(() => {
    if (!kitchenFeatureReady) return;
    const isFirstLoad = !hasLoadedOnceRef.current;
    hasLoadedOnceRef.current = true;
    loadOrders(isFirstLoad);
  }, [loadOrders, kitchenFeatureReady]);

  // SSE: when Kitchen is used standalone (no globalOrders from parent),
  // subscribe to real-time order events and trigger a refetch.
  const handleSSEEvent = useCallback(
    (event: SSEEvent) => {
      if (
        event.type === "order.created" ||
        event.type === "order.updated" ||
        event.type === "order.cancelled"
      ) {
        loadOrders(false);
        // Invalidate React Query caches so any other consumers of order data stay fresh
        queryClient.invalidateQueries({ queryKey: queryKeys.orders.all });
        queryClient.invalidateQueries({
          queryKey: queryKeys.notifications.dashboard(businessId.toString()),
        });
      }
    },
    [loadOrders, businessId, queryClient],
  );

  // Standalone == the parent isn't managing orders (so Kitchen owns its own SSE
  // + polling). Previously this also flipped true whenever globalOrders was
  // momentarily EMPTY ({}) even though the parent WAS managing them — opening a
  // duplicate SSE stream on an empty board. A parent that has loaded
  // (globalOrdersLoaded) owns the stream even when the board is empty.
  const isStandalone =
    !globalOrders ||
    (!globalOrdersLoaded && Object.keys(globalOrders).length === 0);

  // IMP-01: on SSE reconnect (after a drop), pull the orders we missed during
  // the outage. Same shape as the event handler — refetch + invalidate caches.
  const handleSSEReconnect = useCallback(() => {
    loadOrders(false);
    queryClient.invalidateQueries({ queryKey: queryKeys.orders.all });
    queryClient.invalidateQueries({
      queryKey: queryKeys.notifications.dashboard(businessId.toString()),
    });
  }, [loadOrders, businessId, queryClient]);

  useSSEEvents({
    businessId,
    enabled: kitchenFeatureReady && isStandalone && !!businessId,
    onEvent: handleSSEEvent,
    onReconnect: handleSSEReconnect,
  });

  const formatTime = (dateString: string) => {
    return formatKdsFireTime(dateString, currentLocale, businessTimezone);
  };

  const getElapsedTime = (dateString: string) => {
    const duration = humanizeDurationMinutes(kdsElapsedMinutes(dateString, now));
    return tString("time.elapsedAgo").replace("{duration}", duration);
  };

  // Age-urgency color, mirroring the full-screen KitchenDisplayMode so the
  // default list view also signals when an order has been waiting too long.
  // Past calmAfter (24h) → stale: no rose pulse for multi-day zombie orders.
  // Returns a complete static Tailwind literal (no runtime-built classes).
  const getElapsedClass = (dateString: string): string => {
    const diffMins = kdsElapsedMinutes(dateString, now);
    const tone = kdsElapsedUrgency(diffMins);
    if (tone === "critical")
      return "text-rose-600 font-bold motion-safe:animate-pulse";
    if (tone === "warn") return "text-amber-600 font-bold";
    if (tone === "stale") return "text-ink-500";
    return "";
  };

  // If an administrator suspended or closed the business, show lockdown (after all hooks)
  if (!accessLoading && (!hasAccess || isSuspended)) {
    return (
      <DashboardLockedTabView
        title={tString("title")}
        subtitle={tString("subtitle")}
        businessId={businessId}
      />
    );
  }

  // Unknown / still-loading-off → skeleton. A known-true flag (seeded from
  // the business row on cold load) must render the live board even while
  // kitchen-orders-status is still in flight. A false flag is only trusted
  // after that fetch settles — seeding or a zero-value projection was the
  // remaining "Cocina y pedidos no habilitados" flash (#663).
  // #662 folds into the same gate: an unresolved venue timezone would render
  // every fire time in UTC, which reads as a real clock and silently skews
  // expo's sense of how late a ticket is.
  const kitchenBoard = kitchenBoardGate({
    kitchenEnabled: kitchenOrdersEnabled,
    kitchenStatusLoading: kitchenOrdersLoading,
  });
  if (kitchenBoard === "loading" || !venueTimeZoneSettled) {
    return (
      <DashboardTabShell
        width="wide"
        header={{ title: tString("title"), subtitle: tString("subtitle") }}
        loading={<KitchenSkeleton />}
      />
    );
  }

  // If kitchen/orders are disabled, show activation card inline.
  // For non-managers we render a placeholder instead — the activation CTA is
  // owner-only and would 403 on settings:write.
  if (!accessLoading && hasAccess && !isSuspended && kitchenBoard === "off") {
    if (!isStaffUser) {
      // True owner (no staff role) — send them straight to the single source
      // of truth for this toggle (Settings → Kitchen) instead of duplicating
      // the activation control inline.
      return (
        <DashboardTabShell
          width="wide"
          header={{
            title: tString("title"),
            subtitle: tString("subtitle"),
            status: { label: tString("shell.off"), tone: "attention" },
          }}
        >
          <div data-testid="kitchen-owner-empty-state">
            <EmptyState
              panel
              icon={ChefHat}
              title={tString("disabled.title")}
              subtitle={tString("disabled.ownerBody")}
              action={
                <button
                  type="button"
                  onClick={() => router.push(`${pathname}?tab=settings`)}
                  className={btnPrimary}
                >
                  {tString("disabled.openSettings")}
                </button>
              }
            />
          </div>
        </DashboardTabShell>
      );
    }
    if (!kitchenCaps.canActivate) {
      return (
        <DashboardTabShell
          width="wide"
          header={{
            title: tString("title"),
            subtitle: tString("subtitle"),
            status: { label: tString("shell.off"), tone: "attention" },
          }}
        >
          <div data-testid="kitchen-disabled-placeholder">
            <EmptyState
              panel
              icon={ChefHat}
              title={tString("disabled.title")}
              subtitle={tString("disabled.contactOwner")}
            />
          </div>
        </DashboardTabShell>
      );
    }
    return (
      <DashboardTabShell
        width="wide"
        header={{
          title: tString("title"),
          subtitle: tString("subtitle"),
          status: { label: tString("shell.off"), tone: "attention" },
        }}
      >
        <div data-testid="kitchen-activation-toggle">
          <KitchenOrdersToggle
            businessId={businessId}
            isLocked={!hasAccess || isSuspended}
            variant="card"
            onStatusChange={(enabled) => onKitchenStatusChange(enabled)}
            externalEnabled={kitchenOrdersEnabled ?? false}
            externalLoading={kitchenOrdersLoading}
          />
        </div>
      </DashboardTabShell>
    );
  }

  if (loading) {
    return (
      <DashboardTabShell
        width="wide"
        header={{ title: tString("title"), subtitle: tString("subtitle") }}
        loading={<KitchenSkeleton />}
      />
    );
  }

  // Render KDS Mode if active
  if (viewMode === "kds" && canUpdateOrderStatus) {
    return (
      <KitchenDisplayMode
        orders={orders}
        onExit={() => setViewMode("list")}
        onRefresh={() => loadOrders()}
        businessId={businessId}
        locale={currentLocale}
        onOrderStatusChange={onOrderStatusChange}
        allergensByItemId={allergensByItemId}
        businessTimezone={businessTimezone}
      />
    );
  }

  const orderCapBanner = showOrderCapWarning ? (
    <div
      data-testid="kitchen-order-cap-banner"
      className="rounded-2xl border border-amber-200 bg-amber-50 px-4 py-3 text-sm font-medium text-amber-900"
    >
      {tString("warnings.kitchenOrdersCapped")}
    </div>
  ) : null;

  const needsApprovalBanner =
    canUpdateOrderStatus && pendingOrders.length > 0 ? (
      <section
        aria-label={tString("needsApproval.title")}
        data-testid="needs-approval-banner"
        className="rounded-2xl border border-amber-300 bg-amber-50/70 p-4"
      >
        <div className="mb-3 flex items-center gap-2">
          <Clock className="h-4 w-4 text-amber-700" aria-hidden="true" />
          <h3 className="text-sm font-semibold text-amber-900">
            {tString("needsApproval.title")}
          </h3>
          <span
            data-testid="needs-approval-count"
            className="inline-flex min-w-[20px] items-center justify-center rounded-full bg-amber-600 px-1.5 text-[11px] font-semibold text-white"
          >
            {pendingOrders.length}
          </span>
        </div>
        <p className="mb-3 text-xs text-amber-800">
          {tString("needsApproval.description")}
        </p>
        <ul className="space-y-2">
          {sortOrdersFifo(pendingOrders).map((order) => (
            <li
              key={order.id}
              className="flex items-center justify-between gap-3 rounded-xl border border-amber-200 bg-white px-3 py-2"
            >
              <div className="min-w-0">
                <p
                  className="truncate text-sm font-semibold text-ink-900"
                  data-testid={`needs-approval-title-${order.id}`}
                >
                  #{order.order_number}
                  <span className="ml-2 font-normal text-ink-500">
                    {" · "}
                    {tString(
                      countKey(
                        "needsApproval.items",
                        physicalItemQuantity(order.items),
                      ),
                    ).replace(
                      "{count}",
                      physicalItemQuantity(order.items).toString(),
                    )}
                  </span>
                </p>
                <p className="truncate text-xs text-ink-500">
                  <span data-testid="kitchen-ticket-fire-time">
                    {formatTime(order.created_at)}
                  </span>{" "}
                  ·{" "}
                  <span
                    data-testid="kitchen-ticket-elapsed"
                    className={getElapsedClass(order.created_at)}
                  >
                    {getElapsedTime(order.created_at)}
                  </span>
                </p>
              </div>
              <Button
                size="sm"
                radius="full"
                color="warning"
                startContent={<CheckCircle className="h-3 w-3" />}
                isLoading={actionLoading === order.id}
                onPress={() => approvePendingOrder(order.id)}
              >
                {tString("needsApproval.approve")}
              </Button>
            </li>
          ))}
        </ul>
      </section>
    ) : null;

  return (
    <DashboardTabShell
      width="wide"
      header={{
        title: tString("title"),
        subtitle: tString("subtitle"),
        status: { label: tString("shell.live"), tone: "positive" },
        actions: (
          <>
            <button
              type="button"
              onClick={() => loadOrders()}
              className={btnGhostIcon}
              aria-label={tString("buttons.refresh")}
              title={tString("buttons.refresh")}
            >
              <RefreshCw className="h-4 w-4" aria-hidden="true" />
            </button>
            {canUpdateOrderStatus ? (
              <Button
                className={btnPrimaryNextUI}
                radius="full"
                startContent={<Maximize className="w-4 h-4" />}
                onPress={() => setViewMode("kds")}
              >
                {
                  getTranslation(
                    "kitchenDisplay.launch",
                    currentLocale,
                  ) as string
                }
              </Button>
            ) : null}
          </>
        ),
      }}
      // Board-level strips ABOVE the stage tabs so Ready/All empty states
      // don't read as if the pending ticket "belongs" to that queue.
      banner={
        orderCapBanner || needsApprovalBanner ? (
          <div className="space-y-3">
            {orderCapBanner}
            {needsApprovalBanner}
          </div>
        ) : null
      }
      tabs={{
        items: [
          {
            key: "approved",
            label: tString("tabs.approved"),
            icon: Clock,
            badge: orderCounts.approved,
          },
          {
            key: "in_kitchen",
            label: tString("tabs.inKitchen"),
            icon: ChefHat,
            badge: orderCounts.inKitchen,
          },
          {
            key: "ready",
            label: tString("tabs.ready"),
            icon: CheckCircle,
            badge: orderCounts.ready,
          },
          {
            key: "all",
            label: tString("tabs.allOrders"),
            icon: Users,
            badge: orderCounts.all,
          },
        ],
        activeKey: activeTab,
        onChange: setActiveTab,
        ariaLabel: tString("title"),
      }}
    >
      <div className="space-y-4">
        {filteredOrders.length === 0 ? (
          <EmptyState
            panel
            icon={ChefHat}
            title={tString(`emptyStates.${activeTab}.title`)}
            subtitle={tString(`emptyStates.${activeTab}.description`)}
            data-testid="kitchen-board-empty"
          />
        ) : (
          <div className="grid grid-cols-1 md:grid-cols-2 lg:grid-cols-3 gap-4">
            {filteredOrders.map((order) => {
              const fulfillmentLines =
                fulfillmentByOrderId.get(order.id) ??
                toFulfillmentLines(order.items);
              const isTerminal = isTerminalKitchenOrder(order.status);
              const displayNote = order.notes
                ? formatKitchenNote(order.notes)
                : "";
              return (
              <PremiumPanel
                key={order.id}
                interactive
                as="article"
                className="cursor-pointer p-4"
                withTexture={false}
              >
                <div
                  className="rounded-2xl"
                >
                  <div className="flex justify-between items-start w-full pb-2">
                    <div data-testid="kitchen-ticket-identity">
                      <button
                        type="button"
                        aria-label={tString("order.viewOrderAria").replace(
                          "{orderNumber}",
                          String(order.order_number),
                        )}
                        onClick={() => {
                          void operationalAlerts?.claimByResource(
                            "order",
                            order.id,
                            "kitchen_order_open",
                          );
                          setSelectedOrder(order);
                          setShowOrderModal(true);
                        }}
                        className="rounded text-2xl font-semibold text-ink-900 focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-brand"
                      >
                        #{order.order_number}
                      </button>
                      <span aria-hidden="true">
                        {KITCHEN_TICKET_IDENTITY_SEP}
                      </span>
                      <button
                        type="button"
                        aria-label={tString("order.viewBillAria").replace(
                          "{billId}",
                          String(order.bill_id),
                        )}
                        onClick={(event) => {
                          event.stopPropagation();
                          router.push(
                            `${pathname}?tab=bills&billId=${order.bill_id}`,
                          );
                        }}
                        className="rounded text-sm text-brand underline decoration-brand/40 underline-offset-2 hover:text-brand-dark focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-brand"
                      >
                        {tString("order.billPrefix").replace(
                          "{billId}",
                          String(order.bill_id),
                        )}
                      </button>
                    </div>
                    <div className="flex flex-col items-end gap-1">
                      {/*
                       * Round 4 audit (Task 5): was a color-only Chip —
                       * success=in_kitchen etc. inverted from bills/tables.
                       * StatusChip renders icon + label + semantic tone.
                       */}
                      <StatusChip
                        kind="order"
                        status={order.status}
                        labelOverride={tString(
                          orderStatusTranslationKey(order.status),
                        )}
                      />
                      <OperationalAlertClaimStatus
                        resourceType="order"
                        resourceId={order.id}
                      />
                    </div>
                  </div>
                  <div className="space-y-3">
                    <div className="flex items-center gap-2 text-sm text-ink-600">
                      <Clock
                        className="w-4 h-4 text-brand"
                        aria-hidden="true"
                      />
                      <span>
                        <span data-testid="kitchen-ticket-fire-time">
                          {formatTime(order.created_at)}
                        </span>{" "}
                        (
                        <span
                          data-testid="kitchen-ticket-elapsed"
                          className={getElapsedClass(order.created_at)}
                        >
                          {getElapsedTime(order.created_at)}
                        </span>
                        )
                      </span>
                    </div>
                    <div className="space-y-1">
                      {!isTerminal && <div className="flex items-center gap-2 text-sm text-ink-500 mb-2">
                        <Users
                          className="w-4 h-4 text-brand"
                          aria-hidden="true"
                        />
                        <span>
                          {kitchenItemsToPrepareLabel(
                            physicalItemQuantity(order.items),
                            currentLocale,
                          )}
                        </span>
                      </div>}
                      {fulfillmentLines
                        .slice(0, 3)
                        .map((item, index) => {
                          const allergens =
                            (item.menu_item_id &&
                              allergensByItemId[item.menu_item_id]) ||
                            [];
                          return (
                            <div
                              key={index}
                              className="rounded-2xl border border-warm-200/80 bg-warm-50/80 p-3"
                            >
                              <div className="flex items-center gap-2">
                                <span className="font-semibold text-ink-900">
                                  {item.quantity}x
                                </span>
                                <span className="text-sm text-ink-800">
                                  {item.menu_item_name}
                                </span>
                              </div>
                              {/* IMP-13: surface modifiers + special requests + allergens
                                      so the line cook sees them at a glance. */}
                              {item.options && item.options.length > 0 && (
                                <div className="mt-0.5 pl-6 text-xs text-ink-700">
                                  {item.options.map((opt, i) => (
                                    <div key={i}>+ {opt.name}</div>
                                  ))}
                                </div>
                              )}
                              {item.special_requests && (
                                <div className="mt-0.5 pl-6 text-xs italic text-amber-800">
                                  &quot;{item.special_requests}&quot;
                                </div>
                              )}
                              <KitchenAllergenChips
                                allergens={allergens}
                                locale={currentLocale}
                                className="mt-1 flex flex-wrap gap-1 pl-6"
                              />
                            </div>
                          );
                        })}
                      {fulfillmentLines.length > 3 && (
                        <div className="text-center text-xs font-medium text-ink-500">
                          {tString(
                            countKey(
                              "order.moreItems",
                              fulfillmentLines.length - 3,
                            ),
                          ).replace(
                            "{count}",
                            (fulfillmentLines.length - 3).toString(),
                          )}
                        </div>
                      )}
                      {order.notes && (
                        <div className="mt-2 rounded-2xl border border-amber-200 bg-amber-50 p-3">
                          <p className="mb-1 text-xs font-semibold text-amber-900">
                            <span className="inline-flex items-center gap-1.5">
                              <StickyNote
                                className="w-4 h-4 text-amber-700"
                                aria-hidden="true"
                              />
                              {tString("order.notes")}
                            </span>
                          </p>
                          <p className="line-clamp-2 text-xs leading-5 text-amber-900">
                            {displayNote.length > 60
                              ? `${displayNote.substring(0, 60)}...`
                              : displayNote}
                          </p>
                        </div>
                      )}
                    </div>
                  </div>
                </div>

                {/* Cancelled badge */}
                {order.status === "cancelled" && (
                  <div className="mt-3 border-t border-warm-200 pt-2">
                    <span className="inline-flex items-center gap-1 rounded-full bg-rose-50 px-2 py-0.5 text-xs font-medium text-rose-700">
                      {tString("orderCancelled")}
                    </span>
                  </div>
                )}

                {/* Quick Action Buttons */}
                {canUpdateOrderStatus && !isTerminal ? (
                  // eslint-disable-next-line jsx-a11y/click-events-have-key-events, jsx-a11y/no-static-element-interactions -- structural stop-propagation wrapper
                  <div
                    className="flex gap-2 mt-3 pt-3 border-t border-warm-200"
                    onClick={(event) => event.stopPropagation()}
                  >
                    {order.status === "pending" && (
                      <Button
                        size="sm"
                        radius="full"
                        color="warning"
                        startContent={<CheckCircle className="w-3 h-3" />}
                        onPress={() => approvePendingOrder(order.id)}
                        isLoading={actionLoading === order.id}
                        className="flex-1"
                      >
                        {tString("needsApproval.approve")}
                      </Button>
                    )}
                    {order.status === "approved" && (
                      <Button
                        size="sm"
                        radius="full"
                        startContent={<Play className="w-3 h-3" />}
                        onPress={() =>
                          updateOrderStatusHandler(order.id, "in_kitchen")
                        }
                        isLoading={actionLoading === order.id}
                        className={`flex-1 ${btnPrimaryNextUI}`}
                      >
                        {tString("buttons.startCooking")}
                      </Button>
                    )}
                    {order.status === "in_kitchen" && (
                      <Button
                        size="sm"
                        color="success"
                        startContent={<CheckCircle className="w-3 h-3" />}
                        onPress={() =>
                          updateOrderStatusHandler(order.id, "ready")
                        }
                        isLoading={actionLoading === order.id}
                        className="flex-1"
                      >
                        {tString("buttons.markReady")}
                      </Button>
                    )}
                    {order.status === "ready" && (
                      <Button
                        size="sm"
                        color="default"
                        startContent={<CheckCircle className="w-3 h-3" />}
                        onPress={() =>
                          updateOrderStatusHandler(order.id, "delivered")
                        }
                        isLoading={actionLoading === order.id}
                        className="flex-1"
                      >
                        {tString("buttons.markDelivered")}
                      </Button>
                    )}
                    {(order.status === "pending" ||
                      order.status === "approved" ||
                      order.status === "in_kitchen" ||
                      order.status === "ready") && (
                      <Button
                        size="sm"
                        variant="bordered"
                        color="danger"
                        onPress={() => openCancelModal(order)}
                      >
                        {tString("cancelOrder")}
                      </Button>
                    )}
                  </div>
                ) : null}
              </PremiumPanel>
              );
            })}
          </div>
        )}
      </div>

      {/* Cancel Order Modal */}
      <Modal
        isOpen={!!cancelModalOrder}
        onClose={() => {
          setCancelModalOrder(null);
          setCancelReason("");
        }}
        size="md"
      >
        <ModalContent>
          <ModalHeader className="flex items-center gap-2">
            <span className="text-rose-600">{tString("cancelOrder")}</span>
          </ModalHeader>
          <ModalBody>
            <div className="space-y-3">
              <div className="bg-rose-50 border border-rose-200 rounded-lg px-3 py-2 text-sm text-rose-700">
                {tString("cancelWarning")}
              </div>
              <Textarea
                label={tString("cancelReason")}
                placeholder={tString("cancelReasonPlaceholder")}
                value={cancelReason}
                onValueChange={setCancelReason}
                minRows={2}
                isRequired
              />
            </div>
          </ModalBody>
          <ModalFooter>
            <Button
              variant="light"
              onPress={() => {
                setCancelModalOrder(null);
                setCancelReason("");
              }}
            >
              {tString("keepOrder")}
            </Button>
            <Button
              color="danger"
              isLoading={cancelLoading}
              isDisabled={!cancelReason.trim()}
              onPress={handleCancelOrder}
            >
              {tString("cancelConfirm")}
            </Button>
          </ModalFooter>
        </ModalContent>
      </Modal>

      {/* Order Detail Modal */}
      <Modal
        isOpen={showOrderModal}
        onClose={() => setShowOrderModal(false)}
        size="2xl"
      >
        <ModalContent>
          <ModalHeader>
            <div className="flex items-center gap-2">
              <ChefHat className="w-5 h-5" />
              <span>
                {tString("modal.orderNumber").replace(
                  "{orderNumber}",
                  selectedOrder?.order_number || "",
                )}
              </span>
              <StatusChip
                kind="order"
                status={selectedOrder?.status || ""}
                labelOverride={
                  selectedOrder
                    ? tString(orderStatusTranslationKey(selectedOrder.status))
                    : undefined
                }
              />
            </div>
          </ModalHeader>
          <ModalBody>
            {selectedOrder && (
              <div className="space-y-4">
                <div className="grid grid-cols-2 gap-4">
                  <div>
                    <p className="text-sm text-ink-500">
                      {tString("modal.billId")}
                    </p>
                    <button
                      type="button"
                      aria-label={tString("order.viewBillAria").replace(
                        "{billId}",
                        String(selectedOrder.bill_id),
                      )}
                      onClick={() =>
                        router.push(
                          `${pathname}?tab=bills&billId=${selectedOrder.bill_id}`,
                        )
                      }
                      className="rounded font-medium text-brand underline decoration-brand/40 underline-offset-2 hover:text-brand-dark focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-brand"
                    >
                      #{selectedOrder.bill_id}
                    </button>
                  </div>
                  <div>
                    <p className="text-sm text-ink-500">
                      {tString("modal.created")}
                    </p>
                    <p className="font-medium">
                      {formatTime(selectedOrder.created_at)}
                    </p>
                  </div>
                </div>

                {selectedOrder.notes && (
                  <div className="bg-brand/10 rounded-lg p-3">
                    <p className="text-sm font-medium text-brand-dark mb-1">
                      {tString("modal.orderNotes")}
                    </p>
                    <p className="text-brand">
                      {formatKitchenNote(selectedOrder.notes)}
                    </p>
                  </div>
                )}

                <div>
                  {!isTerminalKitchenOrder(selectedOrder.status) && (
                    <h4 className="font-medium mb-2">
                      {tString("modal.itemsToPrepare")}
                    </h4>
                  )}
                  <div className="space-y-2">
                    {toFulfillmentLines(selectedOrder.items).map((item, index) => {
                      const allergens =
                        (item.menu_item_id &&
                          allergensByItemId[item.menu_item_id]) ||
                        [];
                      return (
                        <div
                          key={index}
                          className="flex items-start justify-between rounded-2xl bg-warm-50/80 p-3"
                        >
                          <div>
                            <div className="flex items-center gap-2">
                              <span className="font-medium">
                                {item.quantity}x
                              </span>
                              <span>{item.menu_item_name}</span>
                            </div>
                            {item.options && item.options.length > 0 && (
                              <div className="mt-1">
                                {item.options.map((option, optionIndex) => (
                                  <p
                                    key={optionIndex}
                                    className="text-sm text-brand"
                                  >
                                    + {option.name}
                                    {option.price_change !== 0 && (
                                      <span className="ml-1">
                                        (
                                        {option.price_change > 0
                                          ? "+"
                                          : "\u2212"}
                                        {formatCurrency(
                                          Math.abs(option.price_change),
                                        )}
                                        )
                                      </span>
                                    )}
                                  </p>
                                ))}
                              </div>
                            )}
                            {item.special_requests && (
                              <p className="mt-1 text-sm text-amber-600">
                                {tString("modal.special")}{" "}
                                {item.special_requests}
                              </p>
                            )}
                            {/* IMP-13: allergen pills mirror the card view. */}
                            <KitchenAllergenChips
                              allergens={allergens}
                              locale={currentLocale}
                            />
                            {!isTerminalKitchenOrder(selectedOrder.status) &&
                            canEightySix &&
                            item.menu_item_id &&
                            menuItemIndex[item.menu_item_id] ? (
                              menuItemIndex[item.menu_item_id].available ? (
                                <Button
                                  size="sm"
                                  variant="bordered"
                                  color="danger"
                                  className="mt-2"
                                  onPress={() =>
                                    setPending86(
                                      menuItemIndex[item.menu_item_id!],
                                    )
                                  }
                                >
                                  {tString("eightySix.action")}
                                </Button>
                              ) : (
                                <div className="mt-2 flex flex-wrap items-center gap-2">
                                  <span className="inline-flex items-center rounded-full bg-rose-50 px-2 py-0.5 text-xs font-medium text-rose-700">
                                    {tString("eightySix.alreadyOut")}
                                  </span>
                                  <Button
                                    size="sm"
                                    variant="bordered"
                                    className="border-brand/30 text-brand"
                                    onPress={() =>
                                      setPending86(
                                        menuItemIndex[item.menu_item_id!],
                                      )
                                    }
                                  >
                                    {tString("eightySix.restoreAction")}
                                  </Button>
                                </div>
                              )
                            ) : null}
                          </div>
                          <span className="text-sm font-medium">
                            {formatCurrency(item.subtotal)}
                          </span>
                        </div>
                      );
                    })}
                  </div>
                </div>
              </div>
            )}
          </ModalBody>
          <ModalFooter>
            <Button variant="light" onPress={() => setShowOrderModal(false)}>
              {tString("modal.close")}
            </Button>
            {selectedOrder &&
            canUpdateOrderStatus &&
            !isTerminalKitchenOrder(selectedOrder.status) ? (
              <div className="flex gap-2">
                {selectedOrder.status === "approved" && (
                  <Button
                    radius="full"
                    className={btnPrimaryNextUI}
                    startContent={<Play className="w-4 h-4" />}
                    onPress={() => {
                      updateOrderStatusHandler(selectedOrder.id, "in_kitchen");
                    }}
                    isLoading={actionLoading === selectedOrder.id}
                  >
                    {tString("modal.startCooking")}
                  </Button>
                )}
                {selectedOrder.status === "in_kitchen" && (
                  <Button
                    color="success"
                    startContent={<CheckCircle className="w-4 h-4" />}
                    onPress={() =>
                      updateOrderStatusHandler(selectedOrder.id, "ready")
                    }
                    isLoading={actionLoading === selectedOrder.id}
                  >
                    {tString("modal.markReady")}
                  </Button>
                )}
                {selectedOrder.status === "ready" && (
                  <Button
                    color="default"
                    startContent={<CheckCircle className="w-4 h-4" />}
                    onPress={() =>
                      updateOrderStatusHandler(selectedOrder.id, "delivered")
                    }
                    isLoading={actionLoading === selectedOrder.id}
                  >
                    {tString("modal.markDelivered")}
                  </Button>
                )}
              </div>
            ) : null}
          </ModalFooter>
        </ModalContent>
      </Modal>

      <ConfirmationModal
        isOpen={pending86 !== null}
        onOpenChange={() => (eightySixBusy ? undefined : setPending86(null))}
        isDanger={pending86?.available !== false}
        title={tString(
          pending86?.available === false
            ? "eightySix.restoreConfirmTitle"
            : "eightySix.confirmTitle",
        ).replace("{name}", pending86?.item.name ?? "")}
        description={tString(
          pending86?.available === false
            ? "eightySix.restoreConfirmDescription"
            : "eightySix.confirmDescription",
        )}
        confirmLabel={tString(
          pending86?.available === false
            ? "eightySix.restoreConfirmLabel"
            : "eightySix.confirmLabel",
        )}
        cancelLabel={tString(
          pending86?.available === false
            ? "eightySix.restoreCancelLabel"
            : "eightySix.cancelLabel",
        )}
        onConfirm={() => void confirmEightySix()}
      />
    </DashboardTabShell>
  );
};

export default Kitchen;
