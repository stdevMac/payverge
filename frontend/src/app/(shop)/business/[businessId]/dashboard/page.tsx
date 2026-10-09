"use client";

import React, { useState, useEffect, useCallback, useRef } from "react";
import {
  useSimpleLocale,
  getTranslation,
} from "@/i18n/SimpleTranslationProvider";
import { useRouter, useSearchParams } from "next/navigation";
import { ToastProvider } from "../../../../../contexts/ToastContext";
import {
  MoneyMomentsProvider,
  useMoneyMoments,
} from "@/components/dashboard/moneyMoments/MoneyMomentsProvider";
// IMP-14: wraps the dashboard so destructive bill actions can demand a
// manager PIN via useWithManagerPin().
import { ManagerPinProvider } from "../../../../../components/business/managerPin";
import { usePageTracking, useClickTracking } from "@/hooks/useAnalytics";
import { localDateKey } from "@/lib/localDate";
import { useQueryClient } from "@tanstack/react-query";
import { queryKeys } from "../../../../../api/queryKeys";

// Custom hooks
import { useBusinessDashboard } from "@/hooks/useBusinessDashboard";
import { useAuth } from "@/providers/HybridAuthProvider";
import {
  getPermissionCheckedActiveTab,
  hasStaffPermission,
  isDashboardPrincipalResolved,
} from "@/utils/staffAuth";
import { resolveStaffTabs } from "@/constants/staffTabAccess";
import { useStaffPermissions } from "@/hooks/useStaffPermissions";
import { StaffPermissionsProvider } from "@/contexts/StaffPermissionsContext";
import { useDisclosure } from "@nextui-org/react";
import {
  hasUnsavedChanges,
  clearAll as clearUnsavedChanges,
} from "@/hooks/unsavedChangesRegistry";
import ConfirmationModal from "@/components/business/modals/ConfirmationModal";
import toast from "react-hot-toast";

// API imports for global polling
import { Bill, isActiveBillStatus } from "../../../../../api/bills";
import { fetchActiveBillsFeed } from "./activeBillsFeed";
import { asDollars } from "@/types/money";
import {
  Order,
  OrderDeliveryStatus,
  getAllActiveOrders,
  updateOrderStatus,
} from "../../../../../api/orders";
import { reservationAPI, Reservation } from "../../../../../api/reservations";
import { businessCRMAPI } from "@/api/crm";
import { getKitchenOrdersStatus } from "../../../../../api/kitchenOrders";
import { kitchenEnabledFromBusiness } from "@/components/business/kitchenEnabled";
import { useSSEEvents, SSEEvent } from "../../../../../hooks/useSSEEvents";
import {
  isValidOrder,
  isValidBill,
  isValidReservation,
} from "@/utils/sseValidation";
import { ordersMapEquals } from "@/utils/ordersMapEquals";
import { billsEquals } from "@/utils/billsEquals";
import { useToast } from "../../../../../contexts/ToastContext";
import { usePolling } from "@/hooks/usePolling";

// Components — shell stays static; rail panels are dynamic() so the landing
// tab does not pay for every rail's JS (L3-1). Match AI rail options: ssr:false
// + PremiumLazyTabSkeleton.
import DashboardLayout from "../../../../../components/business/DashboardLayout";
import { getNumericBusinessId } from "../../../../../utils/businessId";
import { recalledVenueName } from "@/utils/openVenueMemory";
import { Business } from "../../../../../api/business";
import { ErrorBoundary } from "react-error-boundary";
// Rail panels (dynamic-imported, code-split per tab), rail metadata, and the
// deep-link entitlement gates are owned by the tab registry — one definition
// point so the page switch, the sidebar, and the prefetcher never drift.
import {
  BusinessOverview,
  Dashboard,
  AccountingDashboard,
  MenuBuilder,
  InventoryManager,
  TableManager,
  CounterManager,
  BillManager,
  CashRegisterDashboard,
  ScheduleBuilder,
  Kitchen,
  StaffManagement,
  CRMManager,
  ReservationManager,
  PluginManager,
  BusinessSettings,
  PrintersSettings,
  BusinessPageEditor,
  DeliveryAdmin,
  AiWaiterDashboard,
  DirectorConsoleDashboard,
  MarketingDashboard,
  TAB_KEYS,
  isDeepLinkPageGated,
} from "@/components/business/tabs/tabRegistry";
import TabErrorFallback from "../../../../../components/business/TabErrorFallback";
import { useBusinessAccess } from "@/hooks/useBusinessAccess";
import { nextTabSearchParams, TAB_ALIASES } from "./tabParams";
import { shouldScheduleGlobalBillPoll } from "./pollPolicy";
import {
  applyReconcileCycle,
  failedReconcileLeg,
  isRateLimitedReconcileError,
  RECONCILE_LEG_OK,
  reconcileLegFromSettled,
  shouldShowStaleBanner,
  reconcileLegAccess,
  skippedReconcileLeg,
} from "./reconcileFailures";
import { fetchOccupiedTablesCount } from "@/components/business/sidebar/occupiedTablesCount";
import { resolveSubTab, SUB_PARAM, type SubTabResult } from "@/lib/subTabs";
import { buildSearchWithParams } from "@/hooks/urlState";
import { EmptyState } from "@/components/ui/EmptyState";
import FeatureUnavailable from "@/components/instance/FeatureUnavailable";
import { useInstance } from "@/hooks/useInstance";
import { instanceOffFeatureForTab } from "@/lib/instance/featureGates";
import { SearchX } from "lucide-react";
import { getTabLockMeta } from "@/components/business/commandPalette/tabAccess";
import { BusinessLockNotice } from "@/components/business/BusinessLockNotice";
import { getSearchParam } from "@/utils/nextRouteParams";
import { OperationalAlertsProvider } from "@/components/business/operational-alerts/OperationalAlertsProvider";
import BrowserPrintAgent from "@/components/business/printers/BrowserPrintAgent";
import DashboardPwaRecorder from "@/components/pwa/DashboardPwaRecorder";
import { useBrowserPrintStation } from "@/components/business/printers/useBrowserPrintStation";
import { DashboardTabTransition } from "@/components/business/premium";
import {
  PRIMARY_TABS,
  SECONDARY_TABS,
  type TabKey,
} from "@/components/business/sidebar/sidebarConfig";

// Valid rail keys come from the registry (single source of truth). printers is
// included: it is a standalone route but a valid ?tab= value historically.
const validTabs: string[] = [...TAB_KEYS];

// TAB_ALIASES lives in ./tabParams (L4-1 / F-9) so unit tests can assert
// normalization without mounting the dashboard. fiscal keeps its dedicated
// path above (→ accounting + ?sub=invoices).

const DAILY_TAB_MOTION_ORDER: TabKey[] = [
  "overview",
  "kitchen",
  "reservations",
];

const DASHBOARD_TAB_MOTION_ORDER: TabKey[] = [
  ...DAILY_TAB_MOTION_ORDER,
  ...PRIMARY_TABS.filter((tab) => !DAILY_TAB_MOTION_ORDER.includes(tab)),
  ...SECONDARY_TABS,
];

const DASHBOARD_TAB_MOTION_INDEX = new Map(
  DASHBOARD_TAB_MOTION_ORDER.map((tab, index) => [tab, index]),
);

function getDashboardTabMotionIndex(tab: string) {
  return DASHBOARD_TAB_MOTION_INDEX.get(tab as TabKey) ?? -1;
}

// Global 60s bills/orders poll is allowlist-gated (L3-12 / Root C). Policy
// lives in ./pollPolicy so new rails default to "no poll". SSE still drives
// badges on every tab.

// Valid inner sub-tabs of the merged Accounting screen, used to validate the
// `?sub=` deep-link param. Unknown values surface a not-found state (Task 31)
// rather than silently falling back and blanking/misrouting the panel.
const ACCOUNTING_SUB_TABS = [
  "overview",
  "entries",
  "payroll",
  "invoices",
  "reports",
  "outstanding",
] as const;

const CRM_SUB_TABS = ["customers", "segments", "loyalty"] as const;
type CrmSubTab = (typeof CRM_SUB_TABS)[number];

// Valid inner sub-tabs of the Team screen (People / Positions / Communication).
// Shares the same `?sub=` param — only the active tab interprets it.
const TEAM_SUB_TABS = ["people", "positions", "communication"] as const;
const TEAM_SUB_ALIASES: Record<string, (typeof TEAM_SUB_TABS)[number]> = {
  tips: "people",
  tip: "people",
};

/**
 * Bridges SSE events to toast notifications. Must be rendered inside
 * ToastProvider.
 *
 * Toast ownership rule (dedupe with OperationalAlertsProvider): any domain
 * event that ALSO produces an operational alert (`alert.created` frame) —
 * order.created → order_new, bill.created → bill_new, payment.received →
 * payment_received / payment_refund_review, reservation.new →
 * reservation_new/reservation_approval, service_call, ai_takeover — is
 * toasted by OperationalAlertsProvider ONLY (localized, settings-aware).
 * This bridge keeps just (a) the payment "money moment" celebration, which
 * is a distinct premium surface, not a toast, and (b) toasts for event types
 * with no alert counterpart (chat.announcement). Do not re-add raw-event
 * toasts here for alert-backed types, or operators get two toasts per event.
 */
function SSEToastBridge({
  listenerRef,
}: {
  listenerRef: React.MutableRefObject<((event: SSEEvent) => void) | null>;
}) {
  const toast = useToast();
  // Payments get the premium "money moment" celebration instead of a flat toast.
  const { celebrate } = useMoneyMoments();
  const { locale } = useSimpleLocale();

  useEffect(() => {
    const t = (
      key: string,
      params?: Record<string, string | number>,
    ): string => {
      const result = getTranslation(
        `businessDashboard.notifications.sse.${key}`,
        locale,
        params,
      );
      return Array.isArray(result) ? result[0] || key : (result as string);
    };
    listenerRef.current = (event: SSEEvent) => {
      switch (event.type) {
        // order.created / bill.created / reservation.new intentionally have
        // NO cases here: their toasts are owned by OperationalAlertsProvider
        // via the corresponding alert.created frames (see ownership rule in
        // the component docblock).
        case "payment.received": {
          // Money-moment celebration only (not a toast). The toast for this
          // event is owned by OperationalAlertsProvider (payment_received
          // alert), so no fallback toast here either.
          celebrate(event.data);
          break;
        }
        case "chat.announcement": {
          toast.showInfo(t("newAnnouncementTitle"), t("newAnnouncementBody"));
          break;
        }
      }
    };
    return () => {
      listenerRef.current = null;
    };
  }, [toast, celebrate, listenerRef, locale]);

  return null;
}

interface BusinessDashboardProps {
  params: Promise<{ businessId: string }>;
}

export default function BusinessDashboardPage({
  params,
}: BusinessDashboardProps) {
  const { businessId } = React.use(params);
  // Translation setup
  const { locale } = useSimpleLocale();
  const { instance: instanceInfo } = useInstance();

  const tString = useCallback(
    (key: string, params?: Record<string, string | number>): string => {
      const fullKey = `businessDashboard.${key}`;
      const result = getTranslation(fullKey, locale, params);
      return Array.isArray(result) ? result[0] || key : (result as string);
    },
    [locale],
  );

  // Navigation hooks
  const router = useRouter();
  const searchParams = useSearchParams();

  // State
  const initialTab = getSearchParam(searchParams, "tab");
  // Inner sub-tab via the one `?sub=` convention (Task 31). `?tab=fiscal`
  // (Setup → Invoices) renders the accounting screen's invoices sub-tab.
  // Unknown keys surface not-found rather than silently falling back.
  const rawSub = getSearchParam(searchParams, SUB_PARAM);
  const accountingResolve = React.useMemo<
    SubTabResult<(typeof ACCOUNTING_SUB_TABS)[number]>
  >(
    () =>
      initialTab === "fiscal"
        ? {
            status: "ok",
            sub: "invoices",
            needsDefaultInUrl: false,
          }
        : resolveSubTab(
            { sub: rawSub },
            ACCOUNTING_SUB_TABS,
            "overview",
          ),
    [initialTab, rawSub],
  );
  const accountingSubTab =
    accountingResolve.status === "ok" ? accountingResolve.sub : "overview";
  const accountingUnknownSub =
    initialTab === "accounting" && accountingResolve.status === "unknown"
      ? accountingResolve.badKey
      : null;
  // Team screen sub-tab (people|positions|communication). Same `?sub=` param;
  // only interpreted when the Team tab is active.
  const teamResolve = React.useMemo(
    () =>
      resolveSubTab(
        { sub: rawSub },
        TEAM_SUB_TABS,
        "people",
        TEAM_SUB_ALIASES,
      ),
    [rawSub],
  );
  const teamSubTab =
    teamResolve.status === "ok" ? teamResolve.sub : ("people" as const);
  const teamUnknownSub =
    initialTab === "staff" && teamResolve.status === "unknown"
      ? teamResolve.badKey
      : null;
  const crmResolve = React.useMemo(
    () => resolveSubTab({ sub: rawSub }, CRM_SUB_TABS, "customers"),
    [rawSub],
  );
  const crmSubTab: CrmSubTab =
    crmResolve.status === "ok" ? crmResolve.sub : "customers";
  const crmUnknownSub =
    initialTab === "crm" && crmResolve.status === "unknown"
      ? crmResolve.badKey
      : null;
  // Present-but-invalid `?tab=` values are kept in the URL and named in a
  // not-found panel (Task 31) — no silent redirect that hides typos.
  const [unknownTabKey, setUnknownTabKey] = useState<string | null>(null);
  const [activeTab, setActiveTab] = useState(
    initialTab && validTabs.includes(initialTab) ? initialTab : "overview",
  );
  const [sidebarOpen, setSidebarOpen] = useState(false);

  // H2a: shared unsaved-changes confirmation for in-app tab navigation. When a
  // tracked form is dirty, a tab click is intercepted and this modal asks the
  // operator to discard before switching. The pending spec is held until they
  // confirm (discard) — cancel leaves them on the current tab, edits intact.
  const {
    isOpen: isUnsavedOpen,
    onOpen: onUnsavedOpen,
    onOpenChange: onUnsavedOpenChange,
  } = useDisclosure();
  const pendingTabSpecRef = useRef<string | null>(null);

  // Global polling state - shared across all tabs
  const [globalBills, setGlobalBills] = useState<Bill[]>([]);
  const [globalBillsCapped, setGlobalBillsCapped] = useState(false);
  // Distinguishes "first active-bills poll not done yet" from "loaded empty"
  // so Bills header stats do not paint a paged partial snapshot (#96).
  const [globalBillsLoaded, setGlobalBillsLoaded] = useState(false);
  // Distinguishes "reconcile threw" from "reconcile returned no open bills"
  // so Bills cannot paint a fake empty till after a failed live fetch (#647).
  const [globalBillsFailed, setGlobalBillsFailed] = useState(false);
  const [globalOrders, setGlobalOrders] = useState<Record<number, Order[]>>({});
  const [globalOrdersCapped, setGlobalOrdersCapped] = useState(false);
  // K-1: distinguishes "first poll not done yet" from "loaded and genuinely
  // empty" so Kitchen never falls back to its own per-tick API call when the
  // dashboard already owns the data.
  const [globalOrdersLoaded, setGlobalOrdersLoaded] = useState(false);
  // Mesas occupancy from /tables/status — leftover-kitchen seats stay occupied
  // after the check is closed/abandoned. Must not reuse the kitchen
  // activeBillsOnly feed (#774).
  const [occupiedTablesCount, setOccupiedTablesCount] = useState(0);
  const [upcomingReservations, setUpcomingReservations] = useState<
    Reservation[]
  >([]);
  // M4: the global reconcile loop (bills/orders/reservations) swallowed its
  // failures, so a mid-shift 403 after a permission change froze the live board
  // silently. Track CONSECUTIVE failed reconcile cycles; after a couple in a row
  // we surface a dismissible "data may be stale" banner (mirrors GuestBill's
  // repeated-failure notice). A cycle where every probed source 200s resets
  // the counter and re-arms the banner for the next outage.
  const [reconcileFailureCount, setReconcileFailureCount] = useState(0);
  const reconcileFailureCountRef = useRef(0);
  reconcileFailureCountRef.current = reconcileFailureCount;
  const [staleBannerDismissed, setStaleBannerDismissed] = useState(false);
  const [sseConnected, setSseConnected] = useState(false);
  // Bumped each time the overview tab activates so the OnboardingHub re-fetches
  // its setup status (e.g., after the operator completes a step on another tab).
  const [setupRefreshKey, setSetupRefreshKey] = useState(0);

  // SSE event listener ref for forwarding to toast-capable inner component
  const sseEventListenerRef = useRef<((event: SSEEvent) => void) | null>(null);
  // Ref to loadGlobalBills for use in SSE callback without creating circular deps
  const loadGlobalBillsRef = useRef<() => void>(() => {});
  const loadFloorOccupancyRef = useRef<() => void>(() => {});
  // Coalesce bursty SSE-triggered reconciles (payment.received): a busy service
  // can fire many payment events in a second; each previously kicked a full
  // bills+orders+reservations reconcile. Trailing-debounce to one poll.
  const billsReconcileTimerRef = useRef<ReturnType<typeof setTimeout> | null>(
    null,
  );
  const scheduleGlobalBillsReconcile = useCallback(() => {
    if (billsReconcileTimerRef.current) {
      clearTimeout(billsReconcileTimerRef.current);
    }
    billsReconcileTimerRef.current = setTimeout(() => {
      billsReconcileTimerRef.current = null;
      loadGlobalBillsRef.current();
    }, 1000);
  }, []);
  useEffect(
    () => () => {
      if (billsReconcileTimerRef.current) {
        clearTimeout(billsReconcileTimerRef.current);
      }
    },
    [],
  );

  // React Query client for cache invalidation
  const queryClient = useQueryClient();

  // Get authentication context
  const {
    isWeb3User,
    isStaffUser,
    isOAuthUser,
    oauthData: _oauthData,
    staffData,
  } = useAuth();

  // Analytics tracking
  usePageTracking();
  const trackClick = useClickTracking();

  // Kitchen/orders enabled state — hoisted here so allowedTabs (below)
  // can filter Bills off the staff sidebar when kitchen is disabled.
  // Other consumers (Kitchen tab, BillManager) continue to read from
  // these same setters via onKitchenStatusChange.
  const [kitchenEnabled, setKitchenEnabled] = useState<boolean | null>(null);
  const [kitchenStatusLoading, setKitchenStatusLoading] = useState(true);
  // Distinguishes loading-with-no-data from error-with-no-data so the tab
  // filter can fail closed only on errors. Without this, a transient status
  // API outage would leave kitchenEnabled=null after the spinner clears,
  // exposing the owner-only activation CTA inside Bills/Kitchen for
  // non-manager staff.
  const [kitchenStatusError, setKitchenStatusError] = useState(false);

  // Effective permissions for staff nav + capability gates (React Query;
  // shared with StaffPermissionsProvider via the same query key).
  const staffPermsBusinessId = staffData ? String(staffData.business_id) : "";
  const {
    data: staffPermsData,
    isLoading: permsLoading,
    isError: permsError,
  } = useStaffPermissions(
    staffPermsBusinessId,
    staffData?.id,
    Boolean(isStaffUser && staffData?.id),
  );
  const permissions = React.useMemo(
    () => staffPermsData?.permissions ?? [],
    [staffPermsData?.permissions],
  );

  // Live-board legs this principal may read. Read through a ref so the poll
  // callback keeps a stable identity when the permission query settles.
  const reconcileLegAccessRef = useRef(
    reconcileLegAccess({
      isStaffUser: false,
      permissionsKnown: false,
      permissions: [],
    }),
  );
  reconcileLegAccessRef.current = reconcileLegAccess({
    isStaffUser: Boolean(isStaffUser),
    permissionsKnown: Boolean(staffPermsData) && !permsError,
    permissions,
  });

  // Surface a visible error when staff permissions fail (nav already fail-closed).
  useEffect(() => {
    if (!isStaffUser || !permsError) return;
    toast.error(tString("error.staffPermissionsLoadFailed"));
  }, [isStaffUser, permsError, tString]);

  // Get allowed tabs based on user type and effective permissions.
  // Staff users get permission-resolved tabs filtered by business state —
  // Bills/Kitchen are hidden from non-activators when kitchen/orders are off
  // (or when the status check failed) because the activation CTA inside is
  // settings:write / owner only.
  const allowedTabs = React.useMemo(() => {
    if (isStaffUser && staffData) {
      if (permsError) return ["overview"]; // fail closed
      if (permsLoading && permissions.length === 0) return ["overview"];
      // Treat the status as "off" both when the API explicitly says off AND
      // when the API errored. Loading still falls through as undefined so a
      // first paint before the response doesn't strip core nav.
      const treatAsOff =
        kitchenEnabled === false ||
        (kitchenStatusError && !kitchenStatusLoading);
      // Empty resolve must not mean "unrestricted" — sidebar/palette treat
      // length===0 as owner-style pass-through.
      const tabs = resolveStaffTabs(permissions, {
        kitchenOrdersEnabled: treatAsOff ? false : undefined,
      });
      return tabs.length > 0 ? tabs : ["overview"];
    }
    if (isWeb3User || isOAuthUser) {
      // Business owners (Web3 or OAuth) get all tabs
      return validTabs;
    }
    return ["overview"];
  }, [
    isStaffUser,
    staffData,
    isWeb3User,
    isOAuthUser,
    kitchenEnabled,
    kitchenStatusError,
    kitchenStatusLoading,
    permissions,
    permsLoading,
    permsError,
  ]);

  // Custom hooks - MUST be called before useEffects that depend on their return values
  const {
    business,
    loading,
    authLoading: webAuthLoading,
    error: webError,
    refreshBusiness,
    user: _user,
    isConnected,
    address: _address,
  } = useBusinessDashboard(businessId);

  // For staff users, we don't need the Web3 auth loading and can bypass business loading
  const authLoading = isStaffUser ? false : webAuthLoading;
  const error = isStaffUser ? null : webError;
  const authResolved = isDashboardPrincipalResolved({
    isStaffUser,
    hasStaffData: Boolean(staffData),
    staffPermissionsLoading: permsLoading,
    isWeb3User,
    isOAuthUser,
  });
  const visibleActiveTab = React.useMemo(
    () =>
      getPermissionCheckedActiveTab({
        activeTab,
        allowedTabs,
        authResolved,
      }),
    [activeTab, allowedTabs, authResolved],
  );
  const visibleActiveTabRef = useRef(visibleActiveTab);
  visibleActiveTabRef.current = visibleActiveTab;
  const previousVisibleTabRef = useRef(visibleActiveTab);
  const tabTransitionDirection = React.useMemo<-1 | 0 | 1>(() => {
    const previous = previousVisibleTabRef.current;
    if (previous === visibleActiveTab) return 0;

    const previousIndex = getDashboardTabMotionIndex(previous);
    const nextIndex = getDashboardTabMotionIndex(visibleActiveTab);
    if (previousIndex === -1 || nextIndex === -1) return 0;

    return nextIndex > previousIndex ? 1 : -1;
  }, [visibleActiveTab]);

  useEffect(() => {
    previousVisibleTabRef.current = visibleActiveTab;
  }, [visibleActiveTab]);

  // Commits a tab change: validates, checks staff permission, updates state and
  // URL. Split out from handleSetActiveTab so the unsaved-changes guard (H2a)
  // can defer this exact work until the operator confirms the discard.
  const commitTabChange = useCallback(
    (spec: string) => {
      const [tab, queryString] = spec.split("?");

      if (!validTabs.includes(tab)) return;

      // Check if staff user has permission for this tab
      if (isStaffUser && !allowedTabs.includes(tab)) {
        console.warn(
          `Staff user does not have permission to access tab: ${tab}`,
        );
        return;
      }

      // Track tab navigation
      trackClick(
        `dashboard-tab-${tab}`,
        "navigation",
        `business-${businessId}`,
      );

      setActiveTab(tab);

      // Update URL without page reload. Rebuild the query from a whitelist
      // (audit M2): keep only `tab` plus the params the INCOMING tab declares in
      // TAB_PARAM_WHITELIST, so no tab inherits another's stale deep-link param
      // (e.g. analytics' `?sub=` bleeding onto inventory). `queryString` carries
      // explicit deep-link params from a composite spec (e.g. "bills?tableId=5").
      const url = new URL(window.location.href);
      const search = nextTabSearchParams(url.searchParams, tab, queryString);

      // Overview is the default tab and carries no query params.
      if (tab === "overview") {
        search.delete("tab");
      }

      // L1-19 / Root C: user-initiated tab changes push so Back walks the
      // operator through the tabs they actually opened. Programmatic
      // normalize/cleanup call sites elsewhere in this file stay on replace.
      router.push(url.pathname + (search.toString() ? `?${search}` : ""), {
        scroll: false,
      });
    },
    [allowedTabs, businessId, isStaffUser, router, trackClick],
  );

  // Custom setActiveTab that syncs with URL and respects permissions.
  // Accepts composite specs like "bills?tableId=5" — the query portion is
  // forwarded to the URL so child components can read it via useSearchParams.
  //
  // H2a: before committing, consult the shared unsaved-changes registry. If any
  // tracked form on the current tab is dirty, defer the change and surface the
  // shared confirmation modal — the tab only switches once the operator elects
  // to discard (see handleUnsavedConfirm). This never fires for deep-link/URL
  // loads, which set state via setActiveTab directly, not through this handler.
  const handleSetActiveTab = useCallback(
    (spec: string) => {
      if (hasUnsavedChanges()) {
        pendingTabSpecRef.current = spec;
        onUnsavedOpen();
        return;
      }
      commitTabChange(spec);
    },
    [commitTabChange, onUnsavedOpen],
  );

  // Operator confirmed the discard: clear every tracked dirty flag so the guard
  // can't re-fire on the deferred change, then commit the pending tab spec.
  const handleUnsavedConfirm = useCallback(() => {
    const spec = pendingTabSpecRef.current;
    pendingTabSpecRef.current = null;
    clearUnsavedChanges();
    if (spec) commitTabChange(spec);
  }, [commitTabChange]);

  // Read tab from URL on page load and validate permissions
  useEffect(() => {
    // Don't redirect while loading
    if (authLoading || loading) return;

    // Don't redirect while auth state is still indeterminate
    // (allowedTabs defaults to ["overview"] before user type is known)
    const tabFromUrl = getSearchParam(searchParams, "tab");

    if (tabFromUrl && validTabs.includes(tabFromUrl)) {
      // Task 32: fiscal is a first-class Setup sidebar tab again (was
      // rewritten to accounting&sub=invoices). Keep the tab key so the
      // sidebar highlight matches the URL; content still lands on invoices.
      if (!authResolved) {
        // Auth hasn't resolved yet — trust the URL, don't redirect
        setUnknownTabKey(null);
        setActiveTab(tabFromUrl);
      } else if (allowedTabs.includes(tabFromUrl)) {
        setUnknownTabKey(null);
        setActiveTab(tabFromUrl);
      } else {
        // L3-22 reclass: not a last-active-business restore — this is the
        // tab-permission gate. Keep the operator on overview but do NOT
        // silently erase their deep link: surface a not-permitted key so the
        // panel can name the forbidden tab (same UX as unknown-tab).
        setUnknownTabKey(tabFromUrl);
        setActiveTab("__unknown__");
      }
    } else if (!tabFromUrl) {
      setUnknownTabKey(null);
      setActiveTab("overview");
    } else {
      // Present-but-invalid `?tab=` value (typo, stale link, or an obvious
      // singular like `plugin`). Alias the safe singulars onto their
      // canonical slug. Genuinely unknown keys keep the URL and surface a
      // not-found panel that names the bad key — never a silent redirect (Task 31).
      const aliased = TAB_ALIASES[tabFromUrl];
      if (aliased && (!authResolved || allowedTabs.includes(aliased))) {
        setUnknownTabKey(null);
        setActiveTab(aliased);
        const url = new URL(window.location.href);
        if (aliased === "overview") {
          url.searchParams.delete("tab");
        } else {
          url.searchParams.set("tab", aliased);
        }
        router.replace(url.pathname + url.search, { scroll: false });
      } else {
        setUnknownTabKey(tabFromUrl);
        setActiveTab("__unknown__");
      }
    }
  }, [searchParams, allowedTabs, router, authLoading, loading, authResolved]);

  // Task 31: write the default sub into the URL so a shared link is
  // reproducible (landing on accounting/staff/crm without ?sub= yields
  // ?sub=<default>).
  useEffect(() => {
    if (typeof window === "undefined") return;
    const tab = getSearchParam(searchParams, "tab");
    if (!tab) return;
    let needsDefault: SubTabResult<string> | null = null;
    if (tab === "accounting") needsDefault = accountingResolve;
    else if (tab === "staff") needsDefault = teamResolve;
    else if (tab === "crm") needsDefault = crmResolve;
    if (!needsDefault || needsDefault.status !== "ok") return;
    if (!needsDefault.needsDefaultInUrl) return;
    const url = new URL(window.location.href);
    url.searchParams.set(SUB_PARAM, needsDefault.sub);
    router.replace(url.pathname + url.search, { scroll: false });
  }, [searchParams, router, accountingResolve, teamResolve, crmResolve]);

  // Re-fetch onboarding setup status whenever the operator lands on overview.
  useEffect(() => {
    if (visibleActiveTab === "overview") {
      setSetupRefreshKey((k) => k + 1);
    }
  }, [visibleActiveTab]);

  // Create a mock business object for staff users to bypass loading
  const staffBusiness =
    isStaffUser && staffData
      ? ({
          // Keep the numeric id numeric (Business.id is a number). Stringifying
          // it makes getNumericBusinessId() fall through to 0 on slug-based
          // dashboard URLs (staff land on /business/<slug>/dashboard), which
          // then drives tab components like StaffManagement to query business
          // "0" → 404 → access lockdown ("access denied").
          id: staffData.business_id,
          business_id: staffData.business_slug || undefined,
          name: staffData.business_name || `Business ${staffData.business_id}`,
          owner_address: "",
          logo: "",
          address: "",
          settlement_address: "",
          description: "",
          phone: "",
          email: "",
          website: "",
          social_media: "",
          business_hours: {},
          // #662: expo/kitchen staff reach this tab through this synthesized
          // row. Without timezone, KDS fire times and aging fall back to UTC
          // (22:12 on EN/es) while es-AR still looks venue-local. Copy the
          // zone from the already-fetched business row when it lands.
          timezone: business?.timezone,
          created_at: new Date().toISOString(),
          updated_at: new Date().toISOString(),
        } as unknown as Business)
      : null;

  const finalBusiness = isStaffUser ? staffBusiness : business;
  const finalLoading = isStaffUser ? false : loading;

  // Additional validation for staff users - ensure they belong to this business
  useEffect(() => {
    if (isStaffUser && staffData && business) {
      const numericMatch = staffData.business_id.toString() === businessId;
      const slugMatch =
        staffData.business_slug && staffData.business_slug === businessId;
      if (!numericMatch && !slugMatch) {
        console.error("Staff user does not belong to this business");
        router.push("/staff/login");
        return;
      }
    }
  }, [isStaffUser, staffData, business, businessId, router]);

  // Check the business lock for the global polling guard. We key the request by
  // the resolved numeric business id (from finalBusiness) once available;
  // before that we keep the URL-supplied identifier so the slug call
  // resolves through the backend's GetBusinessByIdOrBusinessId helper.
  // Sub-components like BusinessOverview also call useBusinessAccess with
  // business.id directly — using the same identifier here keeps React
  // Query's cache key aligned and prevents a duplicate request (one for
  // the slug, one for the numeric id).
  const tierLookupId =
    finalBusiness && typeof finalBusiness.id === "number"
      ? finalBusiness.id
      : businessId;
  const {
    hasAccess,
    loading: accessLoading,
    isError: accessError,
    isSuspended: tierSuspended,
  } = useBusinessAccess(tierLookupId);

  // M7: never HARD-gate live-data loading on a FAILED access fetch. An error
  // leaves hasAccess=false — indistinguishable from a locked business — which
  // would otherwise make the SSE stream, the initial hydrate, and the 60s
  // reconcile poll all silently never run, freezing the board for a whole shift
  // on a transient network blip. Mirror the sidebar/deep-link never-gate
  // policy (DashboardSidebar getTabLockMeta, renderTabContent deep-link gate):
  // when the fetch errored, load optimistically and let the backend's 403s
  // be the authoritative gate rather than a stale client-side denial.
  const effectiveAccess = hasAccess || accessError;

  // Compute numeric businessId for components that still expect numbers
  // This is a temporary solution during the transition
  const numericBusinessId = finalBusiness
    ? getNumericBusinessId(finalBusiness, businessId)
    : 0;
  const {
    printerId: browserPrintStationId,
    stopStation: stopBrowserPrintStation,
  } = useBrowserPrintStation(numericBusinessId);

  // SSE event handler — patches state directly from event data
  const handleSSEEvent = useCallback(
    (event: SSEEvent) => {
      const { type, data } = event;

      switch (type) {
        case "order.created": {
          if (!isValidOrder(data)) return;
          const order = data as unknown as Order;
          if (order.bill_id) {
            setGlobalOrders((prev) => {
              const existing = prev[order.bill_id] || [];
              // Deduplicate: only append if this order isn't already present
              if (existing.some((o) => o.id === order.id)) {
                return prev;
              }
              return { ...prev, [order.bill_id]: [...existing, order] };
            });
          }
          // Forward to toast listener (inside ToastProvider)
          sseEventListenerRef.current?.(event);
          // Invalidate React Query caches
          queryClient.invalidateQueries({ queryKey: queryKeys.orders.all });
          void queryClient.invalidateQueries({
            queryKey: queryKeys.notifications.dashboard(businessId),
          });
          break;
        }
        case "order.updated": {
          if (!isValidOrder(data)) return;
          const order = data as unknown as Order;
          if (order.bill_id) {
            setGlobalOrders((prev) => {
              const existing = prev[order.bill_id] || [];
              const found = existing.some((o) => o.id === order.id);
              return {
                ...prev,
                [order.bill_id]: found
                  ? existing.map((o) => (o.id === order.id ? order : o))
                  : [...existing, order],
              };
            });
          }
          // Invalidate React Query caches
          queryClient.invalidateQueries({ queryKey: queryKeys.orders.all });
          break;
        }
        case "order.cancelled": {
          // Guest-initiated cancels (and the dedicated cancel endpoint) emit
          // `order.cancelled` with a minimal payload — NOT a full `order.updated`.
          // Without this case the dashboard silently dropped them, leaving a
          // cancelled order on the live board (the operator could prep it). Patch
          // the matching order to "cancelled", mirroring the optimistic
          // operator-cancel representation so both paths reconcile identically.
          const cancelData = data as {
            order_id?: number;
            bill_id?: number;
            reason?: string;
            cancelled_by?: string;
          };
          if (cancelData.order_id && cancelData.bill_id) {
            const billId = cancelData.bill_id;
            const cancelledAt = new Date().toISOString();
            setGlobalOrders((prev) => {
              const existing = prev[billId];
              if (!existing) return prev;
              return {
                ...prev,
                [billId]: existing.map((o) =>
                  o.id === cancelData.order_id
                    ? {
                        ...o,
                        status: "cancelled" as Order["status"],
                        updated_at: cancelledAt,
                        cancelled_at: o.cancelled_at ?? cancelledAt,
                        cancelled_by: cancelData.cancelled_by ?? o.cancelled_by,
                        cancel_reason: cancelData.reason ?? o.cancel_reason,
                      }
                    : o,
                ),
              };
            });
          }
          // Reconcile any query-backed order views (mirrors order.updated).
          void queryClient.invalidateQueries({
            queryKey: queryKeys.orders.all,
          });
          break;
        }
        case "bill.created": {
          if (!isValidBill(data)) return;
          const bill = data as unknown as Bill;
          // Dedup by id (mirrors order.created): a double-dispatch or an event
          // that straddles the initial loadGlobalBills hydrate would otherwise
          // show the same bill as two identical cards on the live kanban until
          // the 60s poll's full replace de-dupes it. (Audit E-04.)
          setGlobalBills((prev) =>
            prev.some((b) => b.id === bill.id) ? prev : [...prev, bill],
          );
          void sseEventListenerRef.current?.(event);
          // Invalidate React Query caches
          queryClient.invalidateQueries({ queryKey: queryKeys.payments.all });
          void queryClient.invalidateQueries({
            queryKey: queryKeys.notifications.dashboard(businessId),
          });
          break;
        }
        case "bill.updated": {
          // Support both the minimal payload ({bill_id, status}) that the plugin
          // payment webhook emits today and the full Bill shape emitted when a
          // staff edit triggers a recalc. publishBillUpdatedEvent nests the row
          // under `bill` (dollars via MarshalJSON); older emitters put amounts
          // at the top level. Merge either shape so Tables/Analytics/list do
          // not keep a stale total beside a fresh bill detail modal.
          const billData = data as Partial<Bill> & {
            bill_id?: number;
            bill?: Partial<Bill>;
          };
          const nested = billData.bill ?? {};
          const targetId = billData.id ?? nested.id ?? billData.bill_id;
          if (typeof targetId === "number") {
            setGlobalBills((prev) =>
              prev.map((b) => {
                if (b.id !== targetId) return b;
                const merged: Bill = { ...b };
                const status = billData.status ?? nested.status;
                if (status) merged.status = status as Bill["status"];
                const pickAmount = (key: keyof Bill) => {
                  const top = billData[key];
                  const inner = nested[key];
                  if (typeof top === "number") return top;
                  if (typeof inner === "number") return inner;
                  return undefined;
                };
                const total = pickAmount("total_amount");
                if (typeof total === "number")
                  merged.total_amount = asDollars(total);
                const paid = pickAmount("paid_amount");
                if (typeof paid === "number")
                  merged.paid_amount = asDollars(paid);
                const tip = pickAmount("tip_amount");
                if (typeof tip === "number") merged.tip_amount = asDollars(tip);
                const service = pickAmount("service_fee_amount");
                if (typeof service === "number")
                  merged.service_fee_amount = asDollars(service);
                const tax = pickAmount("tax_amount");
                if (typeof tax === "number") merged.tax_amount = asDollars(tax);
                const subtotal = pickAmount("subtotal");
                if (typeof subtotal === "number")
                  merged.subtotal = asDollars(subtotal);
                const updatedAt = billData.updated_at ?? nested.updated_at;
                if (updatedAt) merged.updated_at = updatedAt;
                return merged;
              }),
            );
            if (billData.status && !isActiveBillStatus(billData.status)) {
              setGlobalOrders((prev) => {
                const next = { ...prev };
                delete next[targetId];
                return next;
              });
              // Kitchen feed drops leftover tickets here; occupancy must not.
              loadFloorOccupancyRef.current();
            }
          }
          // Invalidate React Query caches
          queryClient.invalidateQueries({ queryKey: queryKeys.payments.all });
          break;
        }
        case "bill.closed": {
          const billData = data as { bill_id?: number };
          if (billData.bill_id) {
            setGlobalBills((prev) =>
              prev.filter((b) => b.id !== billData.bill_id),
            );
            setGlobalOrders((prev) => {
              const next = { ...prev };
              delete next[billData.bill_id as number];
              return next;
            });
            loadFloorOccupancyRef.current();
          }
          // Invalidate React Query caches
          queryClient.invalidateQueries({ queryKey: queryKeys.payments.all });
          break;
        }
        case "payment.received": {
          sseEventListenerRef.current?.(event);
          // Coalesce the full-state poll — a burst of payment events now folds
          // into a single trailing reconcile instead of one per event.
          scheduleGlobalBillsReconcile();
          // Invalidate React Query caches
          queryClient.invalidateQueries({ queryKey: queryKeys.payments.all });
          void queryClient.invalidateQueries({
            queryKey: queryKeys.notifications.dashboard(businessId),
          });
          break;
        }
        case "delivery.updated": {
          // A DeliveryOrder payload arrives. Patch the matching order's delivery
          // sub-object so the pending queue card stays live without a full poll.
          const deliveryPayload = data as {
            order_id?: number;
            status?: OrderDeliveryStatus;
            payment_expires_at?: string | null;
          };
          if (deliveryPayload.order_id) {
            const targetOrderId = deliveryPayload.order_id;
            setGlobalOrders((prev) => {
              const next: Record<number, Order[]> = {};
              for (const [billId, billOrders] of Object.entries(prev)) {
                next[Number(billId)] = billOrders.map((o) => {
                  if (o.id !== targetOrderId) return o;
                  return {
                    ...o,
                    delivery: o.delivery
                      ? {
                          ...o.delivery,
                          delivery_status:
                            deliveryPayload.status ??
                            o.delivery.delivery_status,
                          payment_expires_at:
                            deliveryPayload.payment_expires_at !== undefined
                              ? deliveryPayload.payment_expires_at
                              : o.delivery.payment_expires_at,
                        }
                      : o.delivery,
                  };
                });
              }
              return next;
            });
          }
          void queryClient.invalidateQueries({
            queryKey: queryKeys.orders.all,
          });
          break;
        }
        case "delivery.cancelled": {
          // Mark the delivery as cancelled so the countdown/badge updates
          // immediately; the order itself stays pending until the operator acts.
          const cancelledPayload = data as {
            order_id?: number;
          };
          if (cancelledPayload.order_id) {
            const targetOrderId = cancelledPayload.order_id;
            setGlobalOrders((prev) => {
              const next: Record<number, Order[]> = {};
              for (const [billId, billOrders] of Object.entries(prev)) {
                next[Number(billId)] = billOrders.map((o) => {
                  if (o.id !== targetOrderId) return o;
                  return {
                    ...o,
                    delivery: o.delivery
                      ? { ...o.delivery, delivery_status: "cancelled" }
                      : o.delivery,
                  };
                });
              }
              return next;
            });
          }
          void queryClient.invalidateQueries({
            queryKey: queryKeys.orders.all,
          });
          break;
        }
        case "reservation.new": {
          if (!isValidReservation(data)) return;
          const reservation = data as unknown as Reservation;
          setUpcomingReservations((prev) => [...prev, reservation]);
          sseEventListenerRef.current?.(event);
          // Invalidate React Query caches
          queryClient.invalidateQueries({
            queryKey: queryKeys.notifications.dashboard(businessId),
          });
          break;
        }
        case "chat.announcement": {
          sseEventListenerRef.current?.(event);
          break;
        }
        case "reservation.updated": {
          if (!isValidReservation(data)) return;
          const reservation = data as unknown as Reservation;
          setUpcomingReservations((prev) =>
            prev.map((r) => (r.id === reservation.id ? reservation : r)),
          );
          void queryClient.invalidateQueries({
            queryKey: queryKeys.notifications.dashboard(businessId),
          });
          break;
        }
      }
    },
    [businessId, queryClient, scheduleGlobalBillsReconcile],
  );

  // Optimistic order status updater — mutates globalOrders immediately,
  // calls the API, then lets SSE reconcile the authoritative state.
  const handleOrderStatusChange = useCallback(
    async (
      orderId: number,
      newStatus: string,
      approvedBy = "staff",
      reason = "",
    ) => {
      const nowIso = new Date().toISOString();
      // 1. Optimistic update: patch globalOrders right now
      setGlobalOrders((prev) => {
        const next: Record<number, Order[]> = {};
        for (const [billId, billOrders] of Object.entries(prev)) {
          next[Number(billId)] = billOrders.map((o) =>
            o.id === orderId
              ? {
                  ...o,
                  status: newStatus as Order["status"],
                  updated_at: nowIso,
                  cancelled_at:
                    newStatus === "cancelled" ? nowIso : o.cancelled_at,
                  cancelled_by:
                    newStatus === "cancelled" ? approvedBy : o.cancelled_by,
                  cancel_reason:
                    newStatus === "cancelled" && reason.trim()
                      ? reason.trim()
                      : o.cancel_reason,
                }
              : o,
          );
        }
        return next;
      });

      // 2. Fire the API call (SSE order.updated will confirm/correct)
      try {
        await updateOrderStatus(numericBusinessId, orderId, {
          status: newStatus as Order["status"],
          approved_by: approvedBy,
          reason: reason.trim() || undefined,
        });
      } catch (error) {
        console.error("Error updating order status:", error);
        // On failure, trigger a full refresh so we don't stay in stale state
        loadGlobalBillsRef.current();
        throw error; // re-throw so callers can show UI feedback
      }
    },
    [numericBusinessId],
  );

  // IMP-01: invalidate every domain key the dashboard touches when the SSE
  // pipe recovers from a drop. The hook only fires this on recovery, not
  // first connect (initial hydrate is already covered by each query's
  // on-mount fetch). Sweeping the dashboard caches is the safe default —
  // each consumer page then refetches on its own schedule.
  const handleSSEReconnect = useCallback(() => {
    queryClient.invalidateQueries({ queryKey: queryKeys.orders.all });
    queryClient.invalidateQueries({ queryKey: queryKeys.payments.all });
    void queryClient.invalidateQueries({
      queryKey: queryKeys.notifications.dashboard(businessId),
    });
    // globalBills/globalOrders/upcomingReservations are plain useState, not
    // React Query — invalidation does nothing for them. Refresh them directly
    // (one pass) so the live board reconciles on reconnect instead of staying
    // stale until the next 60s safety poll when the outage outran the SSE
    // replay window. (Audit E-03.)
    loadGlobalBillsRef.current();
  }, [businessId, queryClient]);

  // SSE connection (must be after numericBusinessId is defined)
  const {
    degraded: sseDegraded,
    blocked: sseBlocked,
    blockedReason: sseBlockedReason,
    reconnect: sseReconnect,
  } = useSSEEvents({
    businessId: numericBusinessId,
    // M7: never-gate on access error — effectiveAccess lets the stream open on a
    // failed access fetch; a truly-denied stream is closed by the server (which
    // then surfaces the sseBlocked banner) rather than never opening at all.
    enabled: effectiveAccess && !accessLoading && numericBusinessId > 0,
    onEvent: handleSSEEvent,
    onConnectionChange: setSseConnected,
    onReconnect: handleSSEReconnect,
  });

  // Seed ON from the already-loaded business row so a cold ?tab=kitchen
  // load can render the live board without waiting on kitchen-orders-status.
  // Never seed OFF — a false pair may be a stub/zero-value projection and
  // would flash "kitchen off" before the status fetch (#663). The dedicated
  // status fetch below still overwrites once it returns.
  useEffect(() => {
    if (kitchenEnabled != null) return;
    const seeded = kitchenEnabledFromBusiness(finalBusiness);
    if (seeded == null) return;
    setKitchenEnabled(seeded);
  }, [finalBusiness, kitchenEnabled]);

  // Load kitchen/orders enabled status once at dashboard level
  useEffect(() => {
    if (!numericBusinessId) {
      // Keep loading=true. Clearing it here (business row not resolved yet)
      // plus a false seed was enough for Kitchen to paint "kitchen off" on
      // a hard ?tab=kitchen load before kitchen-orders-status ran (#663).
      return;
    }
    // Keep loading=true while the access gate is unresolved so Kitchen cannot
    // flash "disabled" on a cold ?tab=kitchen load (kitchenEnabled is still
    // null and used to be coerced to false).
    if (accessLoading) {
      return;
    }
    if (!hasAccess && !accessError) {
      setKitchenStatusLoading(false);
      return;
    }
    let cancelled = false;
    setKitchenStatusLoading(true);
    (async () => {
      try {
        const status = await getKitchenOrdersStatus(numericBusinessId);
        if (!cancelled) {
          setKitchenEnabled(status.kitchen_enabled && status.orders_enabled);
          setKitchenStatusError(false);
        }
      } catch (error) {
        console.error("Failed to load kitchen/orders status:", error);
        if (!cancelled) setKitchenStatusError(true);
      } finally {
        if (!cancelled) setKitchenStatusLoading(false);
      }
    })();
    return () => {
      cancelled = true;
    };
  }, [numericBusinessId, accessLoading, hasAccess, accessError]);

  // Global polling for bills and orders — scheduled only on allowlisted tabs
  // (pollPolicy / L3-12). SSE still patches badges everywhere.
  // Returns true on success, false on failure so the parent reconcile cycle
  // (loadGlobalBills) can track consecutive failures and surface a stale-data
  // banner instead of freezing silently (M4).
  const loadGlobalOrders = useCallback(async (): Promise<{
    ok: boolean;
    rateLimited: boolean;
  }> => {
    // Active-order loader avoids first-page truncation.

    try {
      // Split live queue from terminal statuses so cancelled/delivered history
      // cannot eat the row budget and hide a pending approval (Bills "0 pending"
      // while Kitchen still showed the ticket — money-trust #94).
      // Kitchen board: active_bills_only=true. Leftover-kitchen occupancy for
      // Mesas is a separate /tables/status fetch (fetchOccupiedTablesCount).
      const [liveOrdersResult, terminalOrdersResult] = await Promise.all([
        getAllActiveOrders(numericBusinessId, {
          statuses: ["pending", "approved", "in_kitchen", "ready"],
          activeBillsOnly: true,
          // Kitchen is a FIFO board: ask oldest-first so if the row cap is hit we
          // drop the NEWEST orders, never the oldest ones a cook works next.
          sort: "asc",
        }),
        getAllActiveOrders(numericBusinessId, {
          statuses: ["cancelled", "delivered"],
          activeBillsOnly: true,
          maxRows: 50,
          sort: "desc",
        }),
      ]);
      const capped = liveOrdersResult.capped || terminalOrdersResult.capped;
      setGlobalOrdersCapped(capped);

      if (capped) {
        console.warn(
          "Dashboard active order live data cap reached",
          liveOrdersResult.warning || terminalOrdersResult.warning,
        );
      }

      const allOrders = [
        ...liveOrdersResult.items,
        ...terminalOrdersResult.items,
      ];

      // Group orders by bill_id
      const ordersMap: Record<number, Order[]> = {};
      allOrders.forEach((order) => {
        if (!ordersMap[order.bill_id]) {
          ordersMap[order.bill_id] = [];
        }
        ordersMap[order.bill_id].push(order);
      });

      // K-1: keep the reference stable when nothing board-relevant changed,
      // so child effects don't re-fire on every quiet 60s tick.
      setGlobalOrders((prev) =>
        ordersMapEquals(prev, ordersMap) ? prev : ordersMap,
      );
      setGlobalOrdersLoaded(true);
      return { ok: true, rateLimited: false };
    } catch (error) {
      console.error("❌ [GLOBAL POLLING] Error loading global orders:", error);
      return { ok: false, rateLimited: isRateLimitedReconcileError(error) };
    }
  }, [numericBusinessId]);

  // Load upcoming reservations. Returns true on success, false on failure so the
  // parent reconcile cycle can track consecutive failures (M4).
  const loadUpcomingReservations = useCallback(async (): Promise<{
    ok: boolean;
    rateLimited: boolean;
  }> => {
    try {
      // Get current date in local timezone
      const currentTime = new Date();

      // Start from beginning of today (local timezone)
      const today = new Date(
        currentTime.getFullYear(),
        currentTime.getMonth(),
        currentTime.getDate(),
      );

      // End at end of day after tomorrow (local timezone) to cover all possible timezones
      const dayAfterTomorrow = new Date(today);
      dayAfterTomorrow.setDate(dayAfterTomorrow.getDate() + 2);

      const startDateStr = localDateKey(today);
      const endDateStr = localDateKey(dayAfterTomorrow);

      const response = await reservationAPI.getReservations(
        numericBusinessId,
        startDateStr,
        endDateStr,
        "confirmed,pending",
      );

      const reservations = response.reservations || [];

      // Holds the full multi-day confirmed/pending list; the -15min/+30min
      // active-window filter for the sidebar badge is applied downstream in
      // DashboardSidebar (upcomingReservationsCount), so we deliberately store
      // the unfiltered list here and don't re-derive the window.
      setUpcomingReservations([...reservations]); // Create new array reference to trigger re-render
      return { ok: true, rateLimited: false };
    } catch (error) {
      console.error("❌ [GLOBAL POLLING] Error loading reservations:", error);
      return { ok: false, rateLimited: isRateLimitedReconcileError(error) };
    }
  }, [numericBusinessId]);

  const loadFloorOccupancy = useCallback(async () => {
    if (!numericBusinessId) return;
    // tables/status needs tables:read; a role without it (kitchen) would
    // 403 on every poll and every SSE bill close.
    if (!reconcileLegAccessRef.current.floorOccupancy) return;
    try {
      const occupied = await fetchOccupiedTablesCount(numericBusinessId);
      setOccupiedTablesCount(occupied);
    } catch (error) {
      console.error("❌ [GLOBAL POLLING] Error loading floor occupancy:", error);
    }
  }, [numericBusinessId]);

  const loadCrmReconcile = useCallback(async (): Promise<{
    ok: boolean;
    rateLimited: boolean;
  }> => {
    try {
      await businessCRMAPI.getCRMStatus(numericBusinessId);
      return { ok: true, rateLimited: false };
    } catch (error) {
      console.error("❌ [GLOBAL POLLING] Error loading CRM:", error);
      return { ok: false, rateLimited: isRateLimitedReconcileError(error) };
    }
  }, [numericBusinessId]);

  const loadGlobalBills = useCallback(async () => {
    if (!numericBusinessId) return;

    // A staff role without a leg's read permission (kitchen has no
    // reservations:read / crm:read / tables:read) would 403 on every cycle
    // and pin the stale banner; those legs are skipped, not failed.
    const legAccess = reconcileLegAccessRef.current;
    const ordersPromise = loadGlobalOrders();
    const probeReservations = legAccess.reservations;
    const reservationsPromise = probeReservations
      ? loadUpcomingReservations()
      : Promise.resolve(skippedReconcileLeg());
    void loadFloorOccupancy();
    // CRM is not on the 60s operational allowlist, but dinner-rush 429s on
    // the CRM tab still used to pin the amber strip. Probe CRM while the
    // operator is on that rail (or while a pinned banner still needs a
    // later OK to heal).
    const probeCrm =
      legAccess.crm &&
      (visibleActiveTabRef.current === "crm" ||
        reconcileFailureCountRef.current > 0);
    const crmPromise = probeCrm
      ? loadCrmReconcile()
      : Promise.resolve(skippedReconcileLeg());

    let billsLeg = failedReconcileLeg(undefined);
    const feed = await fetchActiveBillsFeed(numericBusinessId);
    try {
      if (feed.ok) {
        const billsList = feed.items;
        setGlobalBillsCapped(feed.capped);
        if (feed.capped) {
          console.warn("Active bill live data cap reached", feed.warning);
        }

        // Keep the reference stable when nothing board-relevant changed, so the
        // BillManager live-refresh effect (which watches globalBills identity)
        // doesn't fire a redundant second fetch every reconcile cycle.
        setGlobalBills((prev) =>
          billsEquals(prev, billsList) ? prev : billsList,
        );
        billsLeg = RECONCILE_LEG_OK;
        setGlobalBillsFailed(false);
      } else {
        console.error("❌ [GLOBAL POLLING] Error loading bills:", feed.error);
        billsLeg = failedReconcileLeg(feed.error);
        setGlobalBillsFailed(true);
      }
    } finally {
      // Always unblock Bills stats after the first attempt so a failed
      // hydrate cannot leave the tab on the skeleton forever (#96).
      setGlobalBillsLoaded(true);
    }

    // Isolate orders/reservations/CRM throws so a rejected leg cannot skip
    // the heal path. applyReconcileCycle is the single verdict: every probed
    // source must 200 before the banner clears; a 429-only miss holds the
    // count so a dinner-rush 429 cannot walk a pinned strip off (#623).
    let ordersLeg = failedReconcileLeg(undefined);
    let reservationsLeg = probeReservations
      ? failedReconcileLeg(undefined)
      : skippedReconcileLeg();
    let crmLeg = probeCrm
      ? failedReconcileLeg(undefined)
      : skippedReconcileLeg();
    try {
      const settled = await Promise.allSettled([
        ordersPromise,
        reservationsPromise,
        crmPromise,
      ]);
      ordersLeg = reconcileLegFromSettled(settled[0]);
      reservationsLeg = probeReservations
        ? reconcileLegFromSettled(settled[1])
        : skippedReconcileLeg();
      crmLeg = probeCrm
        ? reconcileLegFromSettled(settled[2])
        : skippedReconcileLeg();
    } catch (error) {
      ordersLeg = failedReconcileLeg(error);
      reservationsLeg = probeReservations
        ? failedReconcileLeg(error)
        : skippedReconcileLeg();
      crmLeg = probeCrm ? failedReconcileLeg(error) : skippedReconcileLeg();
    }

    const cycleLegs = {
      bills: billsLeg,
      orders: ordersLeg,
      reservations: reservationsLeg,
      crm: crmLeg,
    };
    const { allOk } = applyReconcileCycle(
      reconcileFailureCountRef.current,
      cycleLegs,
    );
    setReconcileFailureCount(
      (prev) => applyReconcileCycle(prev, cycleLegs).nextCount,
    );
    if (allOk) {
      setStaleBannerDismissed(false);
    }
  }, [
    numericBusinessId,
    loadGlobalOrders,
    loadUpcomingReservations,
    loadFloorOccupancy,
    loadCrmReconcile,
  ]);

  // Keep ref in sync
  useEffect(() => {
    loadGlobalBillsRef.current = loadGlobalBills;
  }, [loadGlobalBills]);

  useEffect(() => {
    loadFloorOccupancyRef.current = loadFloorOccupancy;
  }, [loadFloorOccupancy]);

  // A business switch must not keep the previous venue's loaded flags or
  // Bills will paint stale counts, then jump (#96).
  useEffect(() => {
    setGlobalBills([]);
    setGlobalBillsCapped(false);
    setGlobalBillsLoaded(false);
    setGlobalBillsFailed(false);
    setGlobalOrders({});
    setGlobalOrdersCapped(false);
    setGlobalOrdersLoaded(false);
    setOccupiedTablesCount(0);
  }, [numericBusinessId]);

  // Initial load. M7: gate on effectiveAccess (hasAccess || access error) so a
  // failed access fetch loads optimistically instead of never hydrating the
  // board; backend 403s remain authoritative for suspended or closed businesses.
  useEffect(() => {
    if (
      numericBusinessId &&
      numericBusinessId > 0 &&
      !accessLoading &&
      effectiveAccess
    ) {
      loadGlobalBills();
    }
  }, [numericBusinessId, loadGlobalBills, accessLoading, effectiveAccess]);

  // Continuous polling every 60 seconds (reduced from 10s — SSE handles
  // real-time). Allowlist of operational rails only (L3-12); menu/config/AI
  // tabs do not schedule. Hidden tabs pause via usePolling.
  usePolling({
    callback: loadGlobalBills,
    interval: 60_000,
    enabled:
      !accessLoading &&
      effectiveAccess &&
      (shouldScheduleGlobalBillPoll(visibleActiveTab) ||
        reconcileFailureCount > 0),
    immediate: false,
    pauseWhenHidden: true,
  });

  // Analytics hook removed since Dashboard component handles its own data fetching

  // Mirror the Accounting inner tab onto the URL so the sidebar highlight and a
  // refresh/deep-link both resolve to the same view (?tab=accounting&sub=...).
  // Always write `sub` (including the default) so a shared link is reproducible
  // (Task 31). Clear status/filter deep-link seeds when the operator switches
  // via chrome so a NeedsAttentionStrip Review filter doesn't stick across
  // sub-tabs.
  const handleAccountingSubTab = useCallback(
    (sub: string) => {
      const url = new URL(window.location.href);
      url.searchParams.set("tab", "accounting");
      url.searchParams.set(SUB_PARAM, sub);
      url.searchParams.delete("status");
      url.searchParams.delete("filter");
      router.replace(url.pathname + url.search, { scroll: false });
    },
    [router],
  );

  // Mirror the Team inner tab onto the URL (?tab=staff&sub=...) so a refresh or
  // a cross-tab deep-link (the Schedule setup guide → "Add positions") lands on
  // the right section. Always write `sub` including the default (Task 31).
  const handleTeamSubTab = useCallback(
    (sub: string) => {
      const url = new URL(window.location.href);
      url.searchParams.set("tab", "staff");
      url.searchParams.set(SUB_PARAM, sub);
      router.replace(url.pathname + url.search, { scroll: false });
    },
    [router],
  );

  const handleCrmSubTab = useCallback(
    (sub: CrmSubTab) => {
      // Chrome-only writer. Segment drilldown must not call this — it would
      // replace from a stale snapshot and drop `focus` (#376).
      const url = new URL(window.location.href);
      const nextSearch = buildSearchWithParams(url.searchParams, [
        { key: "tab", value: "crm" },
        { key: SUB_PARAM, value: sub },
      ]);
      router.replace(
        url.pathname + (nextSearch ? `?${nextSearch}` : ""),
        { scroll: false },
      );
    },
    [router],
  );

  // Render tab content
  const renderTabContent = () => {
    if (!finalBusiness) return null;

    // Task 31: unknown ?tab= or ?sub= keys surface a named not-found state —
    // never a silent redirect that hides typos and breaks deep links.
    if (unknownTabKey) {
      // L3-22: distinguish "not a real tab" from "real tab, role cannot access".
      // URL is intentionally preserved so the operator can share/debug the link.
      const isKnownButForbidden = validTabs.includes(unknownTabKey);
      const notPermittedTitle = (() => {
        const r = getTranslation("urlState.tabNotPermittedTitle", locale);
        return Array.isArray(r) ? r[0] : (r as string);
      })();
      const notPermittedBody = (() => {
        const r = getTranslation("urlState.tabNotPermittedBody", locale, {
          tab: unknownTabKey,
        });
        return Array.isArray(r) ? r[0] : (r as string);
      })();
      return (
        <EmptyState
          panel
          icon={SearchX}
          title={
            isKnownButForbidden
              ? notPermittedTitle
              : tString("error.tabNotFound")
          }
          subtitle={
            isKnownButForbidden
              ? notPermittedBody
              : tString("error.tabNotFoundBody", { key: unknownTabKey })
          }
          actionLabel={tString("error.goToOverview")}
          onAction={() => handleSetActiveTab("overview")}
          data-testid={
            isKnownButForbidden
              ? "dashboard-tab-not-permitted"
              : "dashboard-tab-not-found"
          }
        />
      );
    }
    const unknownSub = accountingUnknownSub || teamUnknownSub || crmUnknownSub;
    if (unknownSub) {
      return (
        <EmptyState
          panel
          icon={SearchX}
          title={tString("error.subNotFound")}
          subtitle={tString("error.subNotFoundBody", { key: unknownSub })}
          actionLabel={tString("error.goToDefaultSub")}
          onAction={() => {
            if (accountingUnknownSub) handleAccountingSubTab("overview");
            else if (teamUnknownSub) handleTeamSubTab("people");
            else handleCrmSubTab("customers");
          }}
          data-testid="dashboard-sub-not-found"
        />
      );
    }

    // Integration not configured on this install (GET /instance features):
    // the rail is hidden, and a deep link explains what is missing.
    const offFeature = instanceOffFeatureForTab(visibleActiveTab, instanceInfo);
    if (offFeature) {
      return <FeatureUnavailable feature={offFeature} />;
    }

    // Deep-link admin-lock gate. The sidebar/palette already lock these tabs
    // while the business is suspended or closed, but a direct ?tab=… link
    // bypasses that. Menu/Tables/Schedule/cash-register do not render their
    // own lock notice, so consult the SAME lock metadata the sidebar uses and
    // render the notice instead. Only owners see it (locked && !hidden); staff
    // fall through to the backend-gated component. The gate list lives in
    // the tab registry (isDeepLinkPageGated).
    if (isDeepLinkPageGated(visibleActiveTab)) {
      const lockMeta = getTabLockMeta(
        visibleActiveTab,
        {
          // A failed access fetch must not lock the deep-linked tab — treat
          // it like loading (never gate).
          loading: accessLoading || accessError,
          isSuspended: tierSuspended,
        },
        isStaffUser,
      );
      if (lockMeta.locked && !lockMeta.hidden) {
        return (
          <div className="mx-auto max-w-6xl p-4">
            <BusinessLockNotice businessId={numericBusinessId} />
          </div>
        );
      }
    }

    switch (visibleActiveTab) {
      case "overview":
        return finalBusiness ? (
          <BusinessOverview
            business={finalBusiness}
            onNavigateToTab={handleSetActiveTab}
            isStaffUser={isStaffUser}
            staffRole={staffData?.role}
            setupRefreshKey={setupRefreshKey}
          />
        ) : null;

      case "analytics":
        return (
          <Dashboard
            businessId={numericBusinessId.toString()}
            currency={finalBusiness.default_currency}
            businessTimezone={finalBusiness.timezone ?? null}
            country={finalBusiness.address?.country}
            onNavigateToTab={handleSetActiveTab}
          />
        );

      case "accounting":
      case "fiscal":
        return (
          <AccountingDashboard
            businessId={numericBusinessId.toString()}
            businessTimezone={finalBusiness.timezone ?? null}
            country={finalBusiness.address?.country}
            initialTab={
              accountingSubTab as
                | "overview"
                | "entries"
                | "payroll"
                | "invoices"
                | "reports"
                | "outstanding"
            }
            onTabChange={handleAccountingSubTab}
            canManagePayroll={!isStaffUser}
            fiscalEntry={visibleActiveTab === "fiscal"}
          />
        );

      case "menu":
        return <MenuBuilder businessId={numericBusinessId} />;

      case "inventory":
        return (
          <InventoryManager
            businessId={numericBusinessId}
            onNavigateToTab={handleSetActiveTab}
          />
        );

      case "tables":
        return (
          <TableManager
            businessId={numericBusinessId}
            businessName={business?.name || ""}
          />
        );

      case "counter":
        return <CounterManager businessId={numericBusinessId} />;

      case "bills":
        return (
          <BillManager
            businessId={numericBusinessId}
            globalBills={globalBills}
            globalBillsCapped={globalBillsCapped}
            globalBillsLoaded={globalBillsLoaded}
            globalBillsFailed={globalBillsFailed}
            onRefreshLiveBills={loadGlobalBills}
            globalOrders={globalOrders}
            globalOrdersLoaded={globalOrdersLoaded}
            kitchenEnabled={kitchenEnabled ?? false}
            kitchenStatusLoading={kitchenStatusLoading}
            onKitchenStatusChange={setKitchenEnabled}
            onOrderStatusChange={handleOrderStatusChange}
            isBusinessOwner={!isStaffUser}
          />
        );

      case "cash-register":
        return (
          <CashRegisterDashboard
            businessId={numericBusinessId.toString()}
            onNavigateToTab={handleSetActiveTab}
            // Fix 13: thread the already-loaded business currency + IANA
            // timezone so CashRegisterDashboard skips its own getBusiness fetch
            // (it dedupes only when BOTH props are defined; finalBusiness is
            // guaranteed here — renderTabContent early-returns when it's null).
            currency={finalBusiness.default_currency ?? "USD"}
            businessTimezone={finalBusiness.timezone ?? null}
          />
        );

      case "schedule":
        // The Schedule tab is now a focused scheduling workspace: the weekly
        // grid plus the approvals/timesheet inbox it owns. Team chat,
        // announcements, and engagement moved to the Team tab's Communication
        // sub-area (they were never about building the week).
        return (
          <ScheduleBuilder
            businessId={numericBusinessId.toString()}
            // Owners (no staff role) and managers with financial:read may see
            // labor $; the backend independently gates every amount too.
            canViewFinancials={
              !isStaffUser || hasStaffPermission(permissions, "financial")
            }
            currency={finalBusiness?.default_currency ?? "USD"}
            businessTimezone={finalBusiness?.timezone ?? null}
            // Lets the cold-start setup checklist jump to Team → Positions.
            onNavigate={handleSetActiveTab}
          />
        );

      case "kitchen":
        return (
          <Kitchen
            businessId={numericBusinessId}
            globalOrders={globalOrders}
            globalOrdersCapped={globalOrdersCapped}
            globalOrdersLoaded={globalOrdersLoaded}
            kitchenEnabled={kitchenEnabled}
            kitchenStatusLoading={kitchenStatusLoading}
            onKitchenStatusChange={setKitchenEnabled}
            onOrderStatusChange={handleOrderStatusChange}
            businessTimezone={
              finalBusiness.timezone ?? business?.timezone ?? null
            }
          />
        );

      case "staff":
        // The Team tab is manager/owner only (getAllowedStaffTabs), the same
        // audience that could moderate chat on the old Schedule tab, so the
        // relocated Communication tools inherit that gate. Backend re-checks.
        return (
          <StaffManagement
            businessId={numericBusinessId.toString()}
            subTab={teamSubTab}
            onSubTabChange={handleTeamSubTab}
            canManageCommunication={
              !isStaffUser || hasStaffPermission(permissions, "staff")
            }
            canViewTips={
              !isStaffUser || hasStaffPermission(permissions, "financial")
            }
            currency={finalBusiness?.default_currency ?? "USD"}
            // Lets the cold-start setup checklist jump to the Schedule tab.
            onNavigate={handleSetActiveTab}
            // R17: render staff/invitation dates in the business wall-clock, not
            // the operator device's timezone (falls back to UTC when absent).
            businessTimezone={finalBusiness.timezone ?? null}
            ownerAddress={finalBusiness.owner_address ?? null}
            ownerName={finalBusiness.owner_name ?? null}
          />
        );

      case "crm":
        return (
          <CRMManager
            businessId={numericBusinessId}
            subTab={crmSubTab}
            onSubTabChange={handleCrmSubTab}
            onNavigateToTab={handleSetActiveTab}
          />
        );

      case "reservations":
        return (
          <ReservationManager
            businessId={numericBusinessId}
            onNavigateToTab={handleSetActiveTab}
          />
        );

      case "plugins":
        return <PluginManager businessId={businessId} />;

      case "settings":
        return <BusinessSettings businessId={numericBusinessId} />;

      case "printers":
        return <PrintersSettings businessId={numericBusinessId} />;

      case "business-page":
        return <BusinessPageEditor businessId={numericBusinessId} />;

      case "delivery":
        return (
          <DeliveryAdmin
            businessId={numericBusinessId}
            onSave={() => refreshBusiness()}
          />
        );

      case "ai-waiter":
        return (
          <AiWaiterDashboard
            business={finalBusiness}
            onUpdateBusiness={refreshBusiness}
            onNavigateToTab={handleSetActiveTab}
          />
        );

      case "director-console":
        return (
          <DirectorConsoleDashboard
            business={finalBusiness}
            onNavigateToTab={handleSetActiveTab}
          />
        );

      case "marketing":
        return (
          <MarketingDashboard
            business={finalBusiness}
            onNavigateToTab={handleSetActiveTab}
          />
        );

      default:
        return (
          <div className="p-6">
            <div className="text-center text-gray-700">
              {tString("error.tabNotFound")}
            </div>
          </div>
        );
    }
  };

  return (
    <ToastProvider>
      <MoneyMomentsProvider currency={finalBusiness?.default_currency ?? "USD"}>
        <ManagerPinProvider>
          <SSEToastBridge listenerRef={sseEventListenerRef} />
          <StaffPermissionsProvider
            businessId={staffPermsBusinessId || String(numericBusinessId || "")}
            staffId={staffData?.id}
            enabled={Boolean(isStaffUser && staffData?.id)}
          >
            <OperationalAlertsProvider
              businessId={numericBusinessId}
              enabled={hasAccess && !accessLoading && numericBusinessId > 0}
            >
              <DashboardPwaRecorder
                ready={Boolean(
                  authResolved && finalBusiness && !finalLoading && !error,
                )}
              />
              {/* One background agent per explicitly selected browser station.
                No local station selection means no claim polling or surprise
                print dialog in an unrelated dashboard tab. */}
              <BrowserPrintAgent
                businessId={numericBusinessId}
                printerId={browserPrintStationId}
                enabled={hasAccess && !accessLoading && numericBusinessId > 0}
                isOwner={!isStaffUser}
                onStopStation={stopBrowserPrintStation}
              />
              <DashboardLayout
                business={finalBusiness}
                venueName={
                  finalBusiness?.name || recalledVenueName(businessId) || null
                }
                loading={finalLoading}
                authLoading={authLoading}
                error={error}
                activeTab={visibleActiveTab}
                setActiveTab={handleSetActiveTab}
                sidebarOpen={sidebarOpen}
                setSidebarOpen={setSidebarOpen}
                isConnected={isConnected}
                allowedTabs={allowedTabs}
                isStaffUser={isStaffUser}
                staffData={staffData}
                globalOrders={globalOrders}
                globalOrdersLoaded={globalOrdersLoaded}
                occupiedTablesCount={occupiedTablesCount}
                upcomingReservations={upcomingReservations}
              >
                {/* SSE terminal-block banner — shown when the server closed the stream
            with a permanent gate denial (business suspended or closed by an
            administrator, or access revoked). Reconnection has STOPPED, so there is NO "Reconnect"
            affordance — retrying a denied stream is futile until the gate
            clears. Takes precedence over the transient "disconnected" hint. */}
                {sseBlocked ? (
                  <div className="mx-4 mt-2 flex items-center rounded-lg border border-amber-200 bg-amber-50 px-4 py-2 text-sm text-amber-800">
                    <span>
                      {tString(
                        sseBlockedReason === "business_suspended" ||
                          sseBlockedReason === "business_closed"
                          ? "dashboard.kitchenManager.warnings.realtimeBlocked"
                          : sseBlockedReason === "access_denied"
                            ? "dashboard.kitchenManager.warnings.realtimeAccessDenied"
                            : "dashboard.kitchenManager.warnings.realtimeBlockedGeneric",
                      )}
                    </span>
                  </div>
                ) : (
                  /* SSE disconnected banner — shown while the stream is down past the
             grace window; auto-clears when the connection self-heals. */
                  sseDegraded &&
                  !sseConnected && (
                    <div className="mx-4 mt-2 flex items-center justify-between rounded-lg border border-amber-200 bg-amber-50 px-4 py-2 text-sm text-amber-800">
                      <span>
                        {tString(
                          "dashboard.kitchenManager.warnings.realtimeDisconnected",
                        )}
                      </span>
                      <button
                        onClick={sseReconnect}
                        className="ml-3 whitespace-nowrap font-medium underline hover:text-amber-900"
                      >
                        {tString("dashboard.kitchenManager.warnings.reconnect")}
                      </button>
                    </div>
                  )
                )}
                {globalOrdersCapped && (
                  <div className="mx-6 mt-4 bg-amber-50 text-amber-800 px-4 py-2 text-sm rounded-lg">
                    {tString(
                      "dashboard.kitchenManager.warnings.activeOrdersCapped",
                    )}
                  </div>
                )}
                {globalBillsCapped && (
                  <div className="mx-6 mt-4 bg-amber-50 text-amber-800 px-4 py-2 text-sm rounded-lg">
                    {tString(
                      "dashboard.kitchenManager.warnings.activeBillsCapped",
                    )}
                  </div>
                )}
                {/* M4: stale-data banner. After a couple of consecutive reconcile
                  cycles fail (e.g. a 403 after a mid-shift permission change),
                  the live board would otherwise freeze silently. Surface a
                  dismissible notice so the operator knows the numbers may be
                  stale. It stays up while any probed source is still failing
                  and clears only after every probed source 200s. */}
                {shouldShowStaleBanner(
                  reconcileFailureCount,
                  staleBannerDismissed,
                ) && (
                  <div className="mx-6 mt-4 flex items-center justify-between rounded-lg border border-amber-200 bg-amber-50 px-4 py-2 text-sm text-amber-800">
                    <span>
                      {tString("dashboard.kitchenManager.warnings.staleData")}
                    </span>
                    <button
                      type="button"
                      onClick={() => setStaleBannerDismissed(true)}
                      aria-label={tString("buttons.close")}
                      className="ml-3 whitespace-nowrap font-medium underline hover:text-amber-900"
                    >
                      {tString("buttons.close")}
                    </button>
                  </div>
                )}
                <ErrorBoundary
                  resetKeys={[visibleActiveTab]}
                  FallbackComponent={(props) => (
                    <TabErrorFallback {...props} tString={tString} />
                  )}
                  onError={(err) =>
                    void import("@/utils/errorLogger").then(({ logError }) =>
                      logError(
                        err instanceof Error ? err : new Error(String(err)),
                        "DashboardTabErrorBoundary",
                        visibleActiveTab,
                      ),
                    )
                  }
                >
                  <DashboardTabTransition
                    tabKey={visibleActiveTab}
                    direction={tabTransitionDirection}
                  >
                    {renderTabContent()}
                  </DashboardTabTransition>
                </ErrorBoundary>
              </DashboardLayout>
              {/* H2a: shared discard prompt for in-app tab navigation while a
                tracked form holds unsaved edits. Confirm = discard + switch. */}
              <ConfirmationModal
                isOpen={isUnsavedOpen}
                onOpenChange={onUnsavedOpenChange}
                title={tString("unsavedChanges.title")}
                description={tString("unsavedChanges.description")}
                confirmLabel={tString("unsavedChanges.discard")}
                cancelLabel={tString("unsavedChanges.keepEditing")}
                isDanger
                onConfirm={handleUnsavedConfirm}
              />
            </OperationalAlertsProvider>
          </StaffPermissionsProvider>
        </ManagerPinProvider>
      </MoneyMomentsProvider>
    </ToastProvider>
  );
}
