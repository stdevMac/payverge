import React, { useState, useEffect, useRef } from "react";
import { useSearchParams, useRouter } from "next/navigation";
import { AlertTriangle, Maximize, Plus, X } from "lucide-react";
import toast from "react-hot-toast";
import {
  Modal,
  ModalContent,
  ModalHeader,
  ModalBody,
  ModalFooter,
  Button,
  Textarea,
  Pagination,
} from "@nextui-org/react";
import {
  getBill,
  getBusinessBills,
  BillWithItemsResponse,
  closeBill,
  Bill,
  isActiveBillStatus,
} from "../../api/bills";
import { getBusiness } from "../../api/business";
import { formatCurrency as formatCurrencyIntl } from "../../api/currency";
import { BillCreator } from "./BillCreator";
import { Order } from "../../api/orders";

import {
  useSimpleLocale,
  getTranslation,
} from "@/i18n/SimpleTranslationProvider";
import { useBusinessAccess } from "@/hooks/useBusinessAccess";
import DashboardLockedTabView from "./DashboardLockedTabView";

import { useOptionalOperationalAlerts } from "./operational-alerts/useOperationalAlerts";

// Import the new smaller components
import { BillsSkeleton } from "./BillsSkeleton";
import { BillsTable } from "./BillsTable";
import { BillFilters } from "./BillFilters";
import { PendingOrdersSection } from "./PendingOrdersSection";
import dynamic from "next/dynamic";
const BillDetailsModal = dynamic(
  () => import("./BillDetailsModal").then((mod) => mod.BillDetailsModal),
  {
    ssr: false,
    loading: () => (
      <div className="animate-pulse h-64 bg-default-100 rounded-lg" />
    ),
  },
);

// The bill-detail modal loads only the newest N history events by default —
// a long-running bill can accumulate hundreds, and this re-fetches on every
// SSE-relevant change while open. "Show older" re-fetches the full history.
const DEFAULT_BILL_HISTORY_LIMIT = 50;
import { KitchenOrdersToggle } from "./KitchenOrdersToggle";
import ConfirmationModal from "./modals/ConfirmationModal";
import BillDisplayMode from "./BillDisplayMode";
import SegmentedTabs from "./shared/SegmentedTabs";
import DashboardTabShell from "./shared/DashboardTabShell";
import { EmptyState } from "@/components/ui/EmptyState";
import { DashboardTabTransition, PremiumPanel } from "./premium";
import { getBillPaymentCapabilities } from "@/utils/staffAuth";
import { isForeignBill } from "@/utils/isForeignBill";
import { useStaffPermissionsContext } from "@/contexts/StaffPermissionsContext";
import { resolveBillsListHonesty } from "./billsListHonesty";

interface BillManagerProps {
  businessId: number;
  globalBills?: Bill[];
  globalBillsCapped?: boolean;
  globalOrders?: Record<number, Order[]>;
  /** True after the first active-orders reconcile completes (Kitchen uses the same flag). */
  globalOrdersLoaded?: boolean;
  /** True after the first active-bills reconcile completes. */
  globalBillsLoaded?: boolean;
  /**
   * True when the dashboard's active-bills reconcile threw. A loaded-empty
   * `globalBills` plus this flag is a failed fetch, not an empty till (#647).
   */
  globalBillsFailed?: boolean;
  /** Re-run the dashboard live bills feed (the request that toasts on failure). */
  onRefreshLiveBills?: () => void;
  kitchenEnabled: boolean;
  kitchenStatusLoading: boolean;
  onKitchenStatusChange: (enabled: boolean) => void;
  onOrderStatusChange: (
    orderId: number,
    newStatus: string,
    approvedBy?: string,
    reason?: string,
  ) => Promise<void>;
  /** True when the viewer is the business owner (wallet/OAuth), not staff. */
  isBusinessOwner?: boolean;
}

export const BillManager: React.FC<BillManagerProps> = ({
  businessId,
  globalBills,
  globalBillsCapped = false,
  globalOrders,
  // undefined = don't gate (unit tests / legacy callers). Dashboard passes
  // false until the first reconcile, then true.
  globalOrdersLoaded,
  globalBillsLoaded,
  globalBillsFailed = false,
  onRefreshLiveBills,
  kitchenEnabled: kitchenOrdersEnabled,
  kitchenStatusLoading: kitchenOrdersLoading,
  onKitchenStatusChange,
  onOrderStatusChange,
  isBusinessOwner = false,
}) => {
  const { permissions } = useStaffPermissionsContext();
  const paymentCapabilities = getBillPaymentCapabilities(
    permissions,
    isBusinessOwner,
  );
  const { locale: currentLocale } = useSimpleLocale();
  const operationalAlerts = useOptionalOperationalAlerts();

  // Read deep-link tableId from URL search params (set by the dashboard when
  // staff taps "Start new order" on a table in LiveTableGrid).
  const searchParams = useSearchParams();
  const router = useRouter();
  const deepLinkTableIdRaw = searchParams?.get("tableId") ?? null;
  const deepLinkTableId =
    deepLinkTableIdRaw !== null && !Number.isNaN(Number(deepLinkTableIdRaw))
      ? Number(deepLinkTableIdRaw)
      : undefined;
  const deepLinkBillIdRaw = searchParams?.get("billId") ?? null;
  const deepLinkBillId =
    deepLinkBillIdRaw !== null && !Number.isNaN(Number(deepLinkBillIdRaw))
      ? Number(deepLinkBillIdRaw)
      : undefined;

  // L21: bill-history pagination/filters mirror to the URL. Params are
  // bill-scoped (billTab/billPage/billSearch/billFrom/billTo/billStatus) so
  // they never collide with the dashboard's own top-level ?tab/?sub params.
  // Initial state is seeded from the URL once (via useState initializers) so a
  // refresh or deep-link restores the same view; subsequent changes are pushed
  // back with router.replace (no new history entry) by the URL-sync effect below.
  const initialBillTab =
    searchParams?.get("billTab") === "history" ? "history" : "active";
  const initialBillPageRaw = Number(searchParams?.get("billPage") ?? "1");
  const initialBillPage =
    Number.isInteger(initialBillPageRaw) && initialBillPageRaw >= 1
      ? initialBillPageRaw
      : 1;
  const initialBillSearch = searchParams?.get("billSearch") ?? "";
  const initialBillFrom = searchParams?.get("billFrom") ?? "";
  const initialBillTo = searchParams?.get("billTo") ?? "";
  const initialHistoryStatusRaw = searchParams?.get("billStatus");
  const initialHistoryStatus: "all" | "paid" | "closed" | "voided" =
    initialHistoryStatusRaw === "paid" ||
    initialHistoryStatusRaw === "closed" ||
    initialHistoryStatusRaw === "voided"
      ? initialHistoryStatusRaw
      : "all";
  const initialBillCustomerRaw = Number(
    searchParams?.get("billCustomer") ?? "",
  );
  const [customerFilter, setCustomerFilter] = useState<number | undefined>(
    Number.isInteger(initialBillCustomerRaw) && initialBillCustomerRaw > 0
      ? initialBillCustomerRaw
      : undefined,
  );

  // Operational lock (admin suspend/close) and RBAC access
  const {
    hasAccess,
    isSuspended,
    loading: accessLoading,
  } = useBusinessAccess(businessId);

  // Translation helper. Memoized on locale so callbacks that depend on it
  // (loadBills/handleViewBill error toasts) stay referentially stable.
  const tString = React.useCallback(
    (key: string): string => {
      const fullKey = `billManager.${key}`;
      const result = getTranslation(fullKey, currentLocale);
      return Array.isArray(result) ? result[0] || key : (result as string);
    },
    [currentLocale],
  );

  // State
  const [bills, setBills] = useState<Bill[]>([]);
  const [loading, setLoading] = useState(true);
  // L2-37: any in-flight list fetch (including filter refetches), so BillsTable
  // can skeleton instead of flashing the empty state over live filters.
  const [listFetching, setListFetching] = useState(false);
  const [listLoadFailed, setListLoadFailed] = useState(false);
  // False until a response with a real `bills` array is applied. Header
  // money stats must not fall back to the initial 0/0 before that (#647).
  const [listHydrated, setListHydrated] = useState(false);
  const [selectedBill, setSelectedBill] =
    useState<BillWithItemsResponse | null>(null);
  const [showBillDetails, setShowBillDetails] = useState(false);
  const [showBillCreator, setShowBillCreator] = useState(false);
  // Wave 4: BillCreator "Record payment" opens details with the record modal pre-opened.
  const [recordPaymentBillId, setRecordPaymentBillId] = useState<number | null>(
    null,
  );
  // Business fallback currency for amount formatting. Defaults to USD so an
  // in-flight load shows a valid amount; corrected after the business fetch
  // resolves. Without this, AED businesses rendered "$322.88" in the bills
  // list (BillsTable used a hardcoded `$` prefix). This is only the fallback
  // now — the bill rows carry their own resolved currency (see billsCurrency).
  const [businessCurrency, setBusinessCurrency] = useState<string>("USD");
  const [businessTimezone, setBusinessTimezone] = useState<string | null>(null);
  const [businessCountry, setBusinessCountry] = useState<string | null>(null);

  useEffect(() => {
    let cancelled = false;
    setBusinessTimezone(null);
    setBusinessCountry(null);
    getBusiness(businessId)
      .then((biz) => {
        if (!cancelled) {
          // display_currency is what the venue chose to show guests and
          // operators; default_currency is only the settlement fallback.
          // Reading default_currency alone labelled an ARS venue's peso
          // totals USD whenever display_currency was the field it set (#852).
          const fallbackCurrency =
            biz?.display_currency || biz?.default_currency;
          if (fallbackCurrency) setBusinessCurrency(fallbackCurrency);
          setBusinessTimezone(biz?.timezone ?? null);
          setBusinessCountry(biz?.address?.country ?? "");
        }
      })
      .catch(() => {
        // Best-effort — leave USD fallback in place; country stays unknown
        // so the AFIP identity form remains hidden (fail closed).
      });
    return () => {
      cancelled = true;
    };
  }, [businessId]);

  // Track whether we already consumed the deep-link so we don't re-open on re-renders
  const deepLinkConsumedRef = useRef<number | undefined>(undefined);

  // When a deep-link tableId arrives, open BillCreator automatically
  useEffect(() => {
    if (
      deepLinkTableId !== undefined &&
      deepLinkConsumedRef.current !== deepLinkTableId
    ) {
      deepLinkConsumedRef.current = deepLinkTableId;
      setShowBillCreator(true);
    }
    // When deep-link is cleared (user navigated away and back without tableId),
    // reset the consumed tracker so a future deep-link fires again.
    if (deepLinkTableId === undefined) {
      deepLinkConsumedRef.current = undefined;
    }
  }, [deepLinkTableId]);

  const [actionLoading, setActionLoading] = useState<number | null>(null);
  const [activeTab, setActiveTab] = useState<string>(initialBillTab);

  // L5-8: the seeds above run once per mount, and on a warm tab switch that
  // mount happens BEFORE useSearchParams() catches up. The dashboard's
  // commitTabChange does `setActiveTab(tab)` (urgent) then `router.push(url)`
  // (a transition), so BillManager first renders against the PREVIOUS URL. The
  // CRM's "view this customer's bills" deep link
  // (bills?billTab=history&billCustomer=<id>) therefore seeded nothing, and the
  // operator landed on an unfiltered, all-customer active list.
  //
  // Adopt the URL values during render (React's "adjust state when props
  // change") rather than in an effect: the adoption lands before the URL-sync
  // effect below runs, so the two can't ping-pong. The markers make this fire
  // only when the URL value actually CHANGES, so a tab the operator clicked is
  // never stomped by the URL echo of that same click.
  const urlBillTabRaw = searchParams?.get("billTab") ?? "";
  const urlBillCustomerRaw = searchParams?.get("billCustomer") ?? "";
  const [lastUrlBillTab, setLastUrlBillTab] = useState(urlBillTabRaw);
  const [lastUrlBillCustomer, setLastUrlBillCustomer] =
    useState(urlBillCustomerRaw);
  if (urlBillTabRaw !== lastUrlBillTab) {
    setLastUrlBillTab(urlBillTabRaw);
    setActiveTab(urlBillTabRaw === "history" ? "history" : "active");
  }
  if (urlBillCustomerRaw !== lastUrlBillCustomer) {
    setLastUrlBillCustomer(urlBillCustomerRaw);
    const nextCustomer = Number(urlBillCustomerRaw);
    setCustomerFilter(
      Number.isInteger(nextCustomer) && nextCustomer > 0
        ? nextCustomer
        : undefined,
    );
  }
  const [viewMode, setViewMode] = useState<"list" | "display">("list");
  const [closeBillConfirm, setCloseBillConfirm] = useState<number | null>(null);
  const [cancelOrderConfirm, setCancelOrderConfirm] = useState<number | null>(
    null,
  );
  const [cancelOrderReason, setCancelOrderReason] = useState("");
  const initialBillsLoadRef = useRef(true);

  // Filter states. Search/date filters seed from the URL only for the tab the
  // URL points at (billTab); the other tab starts empty so switching tabs
  // doesn't inherit an unrelated filter.
  const [activeSearchQuery, setActiveSearchQuery] = useState(
    initialBillTab === "active" ? initialBillSearch : "",
  );
  const [activeDateFrom, setActiveDateFrom] = useState<string>(
    initialBillTab === "active" ? initialBillFrom : "",
  );
  const [activeDateTo, setActiveDateTo] = useState<string>(
    initialBillTab === "active" ? initialBillTo : "",
  );
  const [historySearchQuery, setHistorySearchQuery] = useState(
    initialBillTab === "history" ? initialBillSearch : "",
  );
  const [historyStatusFilter, setHistoryStatusFilter] = useState<
    "all" | "paid" | "closed" | "voided"
  >(initialBillTab === "history" ? initialHistoryStatus : "all");
  const [historyDateFrom, setHistoryDateFrom] = useState<string>(
    initialBillTab === "history" ? initialBillFrom : "",
  );
  const [historyDateTo, setHistoryDateTo] = useState<string>(
    initialBillTab === "history" ? initialBillTo : "",
  );
  const [billPage, setBillPage] = useState(initialBillPage);
  const [billPageSize] = useState(25);
  // M8: the debounced current-tab search value that actually drives the server
  // fetch + URL sync. Keystrokes update the raw *SearchQuery states (which feed
  // the input) immediately; this trails ~300ms behind so we don't fire a fetch
  // (or push a URL) per keystroke. Seeded from the URL so the first load matches.
  const [debouncedSearchQuery, setDebouncedSearchQuery] =
    useState<string>(initialBillSearch);
  const [billTotal, setBillTotal] = useState(0);
  const [billTotalPages, setBillTotalPages] = useState(1);
  const billRequestIdRef = useRef(0);
  // Generation guard for handleViewBill: opening bill A then quickly bill B (or
  // a slow A response landing after B) must not set selectedBill to the stale
  // bill. Mirrors billRequestIdRef's pattern for the list loader.
  const viewRequestIdRef = useRef(0);
  // Live-refresh plumbing (Audit E-01): globalBills is the SSE-fed active-bills
  // slice (also 60s-reconciled). When it changes, a bill event landed, so the
  // standard active list should re-fetch from the server. We track the previous
  // reference to skip the initial value and debounce bursts of events.
  const prevGlobalBillsRef = useRef(globalBills);
  const liveReloadTimerRef = useRef<ReturnType<typeof setTimeout> | null>(null);
  // Audit A-03: the kanban's Completed column filters paid/closed bills out
  // of its `bills` prop, but BillManager only ever fed it active bills — the
  // column was structurally always empty. This slice feeds it bills completed
  // within the last hour, refreshed on display entry, on SSE bill events, and
  // every minute so the window slides.
  const [recentCompletedBills, setRecentCompletedBills] = useState<Bill[]>([]);
  const completedRequestIdRef = useRef(0);
  const prevGlobalBillsForCompletedRef = useRef(globalBills);
  const completedReloadTimerRef = useRef<ReturnType<typeof setTimeout> | null>(
    null,
  );

  // Use globalOrders directly from parent — no local copy needed.
  const orders = React.useMemo<Record<number, Order[]>>(
    () => globalOrders ?? {},
    [globalOrders],
  );

  const billStatusFilter = React.useMemo(
    () =>
      activeTab === "active"
        ? "open,partial"
        : historyStatusFilter === "all"
          ? // "All" history must include voided bills — they are real money
            // reversals and were previously unreachable in every list because
            // this omitted the voided status (audit HIGH/C2). The backend
            // accepts arbitrary comma-separated statuses.
            "paid,closed,voided"
          : historyStatusFilter,
    [activeTab, historyStatusFilter],
  );

  const loadBills = React.useCallback(
    async (isInitialLoad = false) => {
      const requestId = billRequestIdRef.current + 1;
      billRequestIdRef.current = requestId;

      // Only show page-level loading state on initial load, not during polling.
      // Always mark listFetching so the table can avoid empty-state flashes.
      if (isInitialLoad) {
        setLoading(true);
      }
      setListFetching(true);

      try {
        const data = await getBusinessBills(businessId, {
          page: billPage,
          pageSize: billPageSize,
          status: billStatusFilter,
          // M8: use the debounced current-tab search so the fetch fires ~300ms
          // after the last keystroke, not on every keypress.
          search: debouncedSearchQuery,
          dateFrom: activeTab === "active" ? activeDateFrom : historyDateFrom,
          dateTo: activeTab === "active" ? activeDateTo : historyDateTo,
          customerId: customerFilter,
        });
        if (billRequestIdRef.current !== requestId) return;

        // A 200 without a bills array used to collapse into the same
        // "0 Resultados · 0,00 US$" success header as a genuine empty till (#647).
        const billsList = data?.bills;
        if (!Array.isArray(billsList)) {
          throw new Error("invalid bills list payload");
        }
        const nextTotal = data.total ?? billsList.length;
        const nextTotalPages = data.total_pages || 1;

        if (
          nextTotal > 0 &&
          billsList.length === 0 &&
          billPage > nextTotalPages
        ) {
          setBillTotal(nextTotal);
          setBillTotalPages(nextTotalPages);
          setBillPage(nextTotalPages);
          return;
        }

        setBillTotal(nextTotal);
        setBillTotalPages(nextTotalPages);

        // Force new object reference for bills state
        setBills(() => {
          return [...billsList];
        });
        setListHydrated(true);
        setListLoadFailed(false);
      } catch (error) {
        if (billRequestIdRef.current === requestId) {
          console.error("Error loading bills:", error);
          // Surface the failure — without this the spinner just clears and the
          // operator is left staring at a stale or empty bill list with no
          // indication the data is wrong (risking missed open/unbilled tables).
          // Guarded on requestId so a superseded request can't toast.
          toast.error(tString("messages.loadBillsError"));
          setListLoadFailed(true);
        }
      } finally {
        if (billRequestIdRef.current !== requestId) return;
        setLoading(false);
        setListFetching(false);
      }
    },
    [
      businessId,
      billPage,
      billPageSize,
      activeTab,
      billStatusFilter,
      debouncedSearchQuery,
      activeDateFrom,
      activeDateTo,
      historyDateFrom,
      historyDateTo,
      customerFilter,
      tString,
    ],
  );

  // M8: debounce the search-driven reload. The raw search states update on
  // every keystroke (they feed the input), but the server fetch keys off
  // `debouncedSearchQuery`, which we only advance ~300ms after typing stops.
  // Mirrors the 400ms SSE debounce below (setTimeout + clear-on-cleanup). Only
  // the current tab's search is debounced; page/tab/date/status changes bypass
  // this and reload immediately via loadBills' own deps. When the value already
  // matches (e.g. after a tab switch that cleared the box) the timer still fires
  // but the state set is a no-op, so no extra fetch is scheduled.
  const currentTabSearch =
    activeTab === "active" ? activeSearchQuery : historySearchQuery;
  const searchDebounceTimerRef = useRef<ReturnType<
    typeof setTimeout
  > | null>(null);
  // Track the tab the debounce last ran for. Switching tabs must flush the new
  // tab's search value synchronously (0ms) instead of trailing 300ms — otherwise
  // the immediate loadBills triggered by the tab switch would fetch with the old
  // tab's search string before the debounce catches up.
  const searchDebounceTabRef = useRef(activeTab);
  useEffect(() => {
    const tabChanged = searchDebounceTabRef.current !== activeTab;
    searchDebounceTabRef.current = activeTab;
    if (searchDebounceTimerRef.current) {
      clearTimeout(searchDebounceTimerRef.current);
    }
    if (tabChanged) {
      // Flush immediately so the tab-switch reload uses the right search.
      setDebouncedSearchQuery(currentTabSearch);
      return;
    }
    searchDebounceTimerRef.current = setTimeout(() => {
      setDebouncedSearchQuery(currentTabSearch);
    }, 300);
    return () => {
      if (searchDebounceTimerRef.current) {
        clearTimeout(searchDebounceTimerRef.current);
      }
    };
  }, [currentTabSearch, activeTab]);

  // L21: mirror the bills tab/page/filters onto the URL so a refresh or
  // deep-link restores the same view. Bill-scoped param keys keep the
  // dashboard's own ?tab/?sub/?tableId params untouched (mirrors
  // handleAccountingSubTab in dashboard/page.tsx). Uses router.replace (no new
  // history entry) and keys off the debounced search, so a burst of keystrokes
  // produces at most one URL write when typing settles — not one per key.
  const currentTabFrom =
    activeTab === "active" ? activeDateFrom : historyDateFrom;
  const currentTabTo = activeTab === "active" ? activeDateTo : historyDateTo;
  useEffect(() => {
    if (typeof window === "undefined") return;
    const url = new URL(window.location.href);
    const params = url.searchParams;

    // This effect reads the LIVE window.location, which router.push has already
    // updated — so during the stale commit described above it can see deep-link
    // params this component has not adopted yet. Deleting one here would erase
    // the operator's link before it ever took effect (and leave no trace in the
    // address bar that it carried anything), so only clear a value we have
    // actually consumed.
    if (activeTab === "history") {
      params.set("billTab", "history");
    } else if ((params.get("billTab") ?? "") === lastUrlBillTab) {
      params.delete("billTab");
    }
    if (billPage > 1) {
      params.set("billPage", String(billPage));
    } else {
      params.delete("billPage");
    }
    if (debouncedSearchQuery.trim()) {
      params.set("billSearch", debouncedSearchQuery.trim());
    } else {
      params.delete("billSearch");
    }
    if (currentTabFrom) {
      params.set("billFrom", currentTabFrom);
    } else {
      params.delete("billFrom");
    }
    if (currentTabTo) {
      params.set("billTo", currentTabTo);
    } else {
      params.delete("billTo");
    }
    if (activeTab === "history" && historyStatusFilter !== "all") {
      params.set("billStatus", historyStatusFilter);
    } else {
      params.delete("billStatus");
    }
    if (customerFilter) {
      params.set("billCustomer", String(customerFilter));
    } else if ((params.get("billCustomer") ?? "") === lastUrlBillCustomer) {
      params.delete("billCustomer");
    }

    const nextUrl = url.pathname + (params.toString() ? `?${params}` : "");
    if (nextUrl !== window.location.pathname + window.location.search) {
      router.replace(nextUrl, { scroll: false });
    }
  }, [
    activeTab,
    billPage,
    debouncedSearchQuery,
    currentTabFrom,
    currentTabTo,
    historyStatusFilter,
    customerFilter,
    lastUrlBillTab,
    lastUrlBillCustomer,
    router,
  ]);

  // Reset paging when the business changes. Skip the very first mount so the
  // URL-seeded initialBillPage (L21) survives a refresh/deep-link; only an
  // actual business switch snaps back to page 1.
  const prevBusinessIdRef = useRef(businessId);
  useEffect(() => {
    initialBillsLoadRef.current = true;
    if (prevBusinessIdRef.current !== businessId) {
      prevBusinessIdRef.current = businessId;
      setBillPage(1);
      setListHydrated(false);
      setListLoadFailed(false);
    }
  }, [businessId]);

  useEffect(() => {
    if (!businessId) {
      return;
    }

    if (!accessLoading && hasAccess) {
      loadBills(initialBillsLoadRef.current);
      initialBillsLoadRef.current = false;
    }
  }, [businessId, loadBills, accessLoading, hasAccess]);

  // Audit E-01: the standard Bills LIST (BillsTable) reads BillManager's own
  // loadBills() result, which has no SSE/poll — so a guest payment did not move
  // a bill open->paid live; staff kept seeing a paid table as unpaid. The
  // dashboard already feeds the SSE-patched + 60s-reconciled `globalBills`. When
  // its reference changes (a bill.created/updated/closed/payment.received event,
  // or the safety poll), re-fetch the active list so it reflects live state.
  // Gated to the active tab (history is server-paginated/filtered and not
  // time-sensitive) and debounced so a burst of events triggers one refresh.
  useEffect(() => {
    if (prevGlobalBillsRef.current === globalBills) return; // skip initial value
    prevGlobalBillsRef.current = globalBills;
    if (activeTab !== "active") return;
    if (liveReloadTimerRef.current) clearTimeout(liveReloadTimerRef.current);
    liveReloadTimerRef.current = setTimeout(() => {
      void loadBills();
    }, 400);
    return () => {
      if (liveReloadTimerRef.current) clearTimeout(liveReloadTimerRef.current);
    };
  }, [globalBills, activeTab, loadBills]);

  // Audit A-03: fetch the recently-completed slice for the kanban's
  // Completed column. One page (newest first), client-filtered to bills
  // completed in the last hour (closed_at when present, else updated_at —
  // the server's created_at date filters would wrongly exclude an
  // old-created bill paid just now), capped at 20.
  const loadRecentCompleted = React.useCallback(async () => {
    const requestId = completedRequestIdRef.current + 1;
    completedRequestIdRef.current = requestId;
    try {
      const data = await getBusinessBills(businessId, {
        page: 1,
        pageSize: 50,
        status: "paid,closed",
      });
      if (completedRequestIdRef.current !== requestId) return;
      const cutoff = Date.now() - 60 * 60 * 1000;
      const recent = data.bills
        .filter((bill) => bill.status === "paid" || bill.status === "closed")
        .filter((bill) => {
          const completedAt = new Date(
            bill.closed_at || bill.updated_at,
          ).getTime();
          return Number.isFinite(completedAt) && completedAt >= cutoff;
        })
        .slice(0, 20);
      setRecentCompletedBills(recent);
    } catch (error) {
      // Non-fatal: the Completed column just stays empty for this cycle;
      // the active columns are unaffected.
      console.error("Error loading recently completed bills:", error);
    }
  }, [businessId]);

  // Load on entering display mode, then slide the one-hour window every
  // minute while the display stays up (matches the display's live cadence
  // without adding load on the standard list view).
  useEffect(() => {
    if (viewMode !== "display") return;
    void loadRecentCompleted();
    const interval = setInterval(() => {
      void loadRecentCompleted();
    }, 60_000);
    return () => clearInterval(interval);
  }, [viewMode, loadRecentCompleted]);

  // A payment/close event moves a bill out of the SSE-fed active set — pick
  // it up in Completed promptly instead of waiting for the minute tick.
  // Same debounce pattern as the E-01 live refresh above.
  useEffect(() => {
    if (prevGlobalBillsForCompletedRef.current === globalBills) return;
    prevGlobalBillsForCompletedRef.current = globalBills;
    if (viewMode !== "display") return;
    if (completedReloadTimerRef.current) {
      clearTimeout(completedReloadTimerRef.current);
    }
    completedReloadTimerRef.current = setTimeout(() => {
      void loadRecentCompleted();
    }, 400);
    return () => {
      if (completedReloadTimerRef.current) {
        clearTimeout(completedReloadTimerRef.current);
      }
    };
  }, [globalBills, viewMode, loadRecentCompleted]);

  // Strip ?billId= from the URL while preserving every other query param
  // (tab, billPage, filters, …). Used when a deep-link points at a bill that
  // does not belong to the business shell currently being viewed (L9-1).
  const clearBillIdSearchParam = React.useCallback(() => {
    if (typeof window === "undefined") return;
    const url = new URL(window.location.href);
    if (!url.searchParams.has("billId")) return;
    url.searchParams.delete("billId");
    const nextUrl =
      url.pathname + (url.searchParams.toString() ? `?${url.searchParams}` : "");
    router.replace(nextUrl, { scroll: false });
  }, [router]);

  // L9-3: write ?billId= so the open detail is addressable / refreshable.
  const writeBillIdSearchParam = React.useCallback(
    (billId: number) => {
      if (typeof window === "undefined") return;
      const url = new URL(window.location.href);
      if (url.searchParams.get("billId") === String(billId)) return;
      url.searchParams.set("billId", String(billId));
      const nextUrl =
        url.pathname + (url.searchParams.toString() ? `?${url.searchParams}` : "");
      router.replace(nextUrl, { scroll: false });
    },
    [router],
  );

  // Event handlers
  const handleViewBill = React.useCallback(
    async (billId: number, fullHistory = false) => {
      void operationalAlerts?.claimByResource(
        "bill",
        billId,
        "bill_details_open",
      );
      // L9-3 write-back: URL is the source of the open bill detail.
      writeBillIdSearchParam(billId);
      const requestId = viewRequestIdRef.current + 1;
      viewRequestIdRef.current = requestId;
      try {
        // Bounded history by default; "show older" re-requests without the cap.
        const billDetails = await getBill(
          billId,
          undefined,
          fullHistory ? undefined : DEFAULT_BILL_HISTORY_LIMIT,
        );
        // Drop a stale response: only the latest view click may set state.
        if (viewRequestIdRef.current !== requestId) return;
        // L9-1: getBill is keyed by bill id only. Multi-business operators can
        // deep-link a foreign ?billId= into this shell; refuse to open the
        // modal (payment/void UI) when nested bill.business_id mismatches.
        if (isForeignBill(billDetails.bill?.business_id, businessId)) {
          toast.error(tString("errors.billNotInThisBusiness"));
          clearBillIdSearchParam();
          return;
        }
        setSelectedBill(billDetails);
        setShowBillDetails(true);
      } catch (error) {
        if (viewRequestIdRef.current !== requestId) return;
        console.error("Error loading bill details:", error);
        // Surface the failure — without this the spinner just clears, the modal
        // never opens, and the operator can't reach the void/refund/audit UI
        // (hosted in BillDetailsModal) with no idea why. Mirrors confirmCloseBill.
        toast.error(tString("messages.viewBillError"));
      }
    },
    [operationalAlerts, tString, businessId, clearBillIdSearchParam, writeBillIdSearchParam],
  );

  // "Show older" from the modal: re-fetch the same bill with full history.
  const handleLoadFullBillHistory = React.useCallback(
    async (billId: number) => {
      await handleViewBill(billId, true);
    },
    [handleViewBill],
  );

  // Wave 4: a billId deep-link (kitchen ticket / operational alert) opens the
  // bill's details modal once, then resets when the param clears.
  const billDeepLinkConsumedRef = useRef<number | undefined>(undefined);
  useEffect(() => {
    if (
      deepLinkBillId !== undefined &&
      billDeepLinkConsumedRef.current !== deepLinkBillId
    ) {
      billDeepLinkConsumedRef.current = deepLinkBillId;
      void handleViewBill(deepLinkBillId);
    }
    if (deepLinkBillId === undefined) {
      billDeepLinkConsumedRef.current = undefined;
    }
  }, [deepLinkBillId, handleViewBill]);

  // Guarded refresh used by BillDetailsModal after item mutations.
  // Rapid +/- clicks in BillItemEditor previously fired loadBills() and
  // handleViewBill() in parallel, producing flickering stale intermediate
  // states when responses interleaved. This in-flight guard serializes the
  // refresh so at most one round-trip is live at a time; clicks that land
  // while a refresh is already running are coalesced.
  const refreshingItemChangeRef = useRef(false);
  const handleBillItemsRefresh = React.useCallback(
    async (billId: number) => {
      if (refreshingItemChangeRef.current) return;
      refreshingItemChangeRef.current = true;
      try {
        await loadBills();
        await handleViewBill(billId);
      } finally {
        refreshingItemChangeRef.current = false;
      }
    },
    [handleViewBill, loadBills],
  );

  // M9: refresh the OPEN bill modal on SSE payment/mutation events. globalBills
  // is the SSE-fed (+60s-reconciled) active-bills slice; when it changes while
  // the details modal is open, re-fetch the open bill so "Remaining" can't go
  // stale while a guest pays by QR (which would invite a duplicate manual
  // payment). We only refetch when the open bill's own row actually changed
  // (updated_at / money / status), guarded by an in-flight ref so a burst of
  // events can't stack refetches. handleViewBill's viewRequestIdRef still drops
  // any superseded response.
  const prevGlobalBillsForOpenRef = useRef(globalBills);
  const refreshingOpenBillRef = useRef(false);
  useEffect(() => {
    const prev = prevGlobalBillsForOpenRef.current;
    prevGlobalBillsForOpenRef.current = globalBills;
    if (prev === globalBills) return; // skip initial value / unchanged reference
    if (!showBillDetails || !selectedBill) return;
    const openId = selectedBill.bill.id;
    const liveRow = globalBills?.find((b) => b.id === openId);
    if (!liveRow) return; // bill left the active set (closed/paid) — leave as-is
    const current = selectedBill.bill;
    const changed =
      liveRow.updated_at !== current.updated_at ||
      liveRow.paid_amount !== current.paid_amount ||
      liveRow.total_amount !== current.total_amount ||
      liveRow.tip_amount !== current.tip_amount ||
      liveRow.status !== current.status;
    if (!changed) return;
    if (refreshingOpenBillRef.current) return;
    refreshingOpenBillRef.current = true;
    void (async () => {
      try {
        await handleViewBill(openId);
      } finally {
        refreshingOpenBillRef.current = false;
      }
    })();
  }, [globalBills, showBillDetails, selectedBill, handleViewBill]);

  // Opens confirmation modal instead of closing directly
  const handleCloseBill = (billId: number) => {
    setCloseBillConfirm(billId);
  };

  // Actually closes the bill after confirmation
  const confirmCloseBill = async () => {
    if (closeBillConfirm === null) return;
    const billId = closeBillConfirm;
    setCloseBillConfirm(null);
    setActionLoading(billId);
    try {
      await closeBill(billId);
      await loadBills();
      // Close details modal if viewing this bill
      if (selectedBill && selectedBill.bill.id === billId) {
        setShowBillDetails(false);
        setSelectedBill(null);
      }
    } catch (error) {
      console.error("Error closing bill:", error);
      // Surface the failure — without this the spinner just clears and the bill
      // stays open with no explanation, so the operator can't tell a failed
      // close from a no-op. Mirrors the cancel-order toast pattern below.
      toast.error(tString("messages.closeBillError"));
    } finally {
      setActionLoading(null);
    }
  };

  const handleApproveOrder = async (orderId: number) => {
    void operationalAlerts?.claimByResource(
      "order",
      orderId,
      "bill_order_approve",
    );
    setActionLoading(orderId);
    try {
      await onOrderStatusChange(orderId, "approved", "staff");
    } catch (error) {
      console.error("Error approving order:", error);
      // The parent optimistically flips the status then rolls back + re-throws
      // "so callers can show UI feedback" — honor that contract with a toast.
      toast.error(tString("messages.approveOrderError"));
    } finally {
      setActionLoading(null);
    }
  };

  const handleRejectOrder = async (orderId: number) => {
    void operationalAlerts?.claimByResource(
      "order",
      orderId,
      "bill_order_reject",
    );
    setCancelOrderReason("");
    setCancelOrderConfirm(orderId);
  };

  const confirmRejectOrder = async () => {
    if (cancelOrderConfirm === null) return;

    const orderId = cancelOrderConfirm;
    const matchingOrder = Object.values(orders)
      .flat()
      .find((order) => order.id === orderId);

    setCancelOrderConfirm(null);
    setActionLoading(orderId);
    try {
      await onOrderStatusChange(
        orderId,
        "cancelled",
        "staff",
        cancelOrderReason,
      );
      toast.success(
        tString("messages.orderCancelledSuccess").replace(
          "{orderNumber}",
          matchingOrder?.order_number || String(orderId),
        ),
      );
    } catch (error) {
      console.error("Error rejecting order:", error);
      toast.error(tString("messages.orderCancelledError"));
    } finally {
      setActionLoading(null);
      setCancelOrderReason("");
    }
  };

  // Filter functions
  const resetActiveFilters = () => {
    setActiveSearchQuery("");
    setActiveDateFrom("");
    setActiveDateTo("");
    setCustomerFilter(undefined);
    setBillPage(1);
    // Flush the M8 debounce immediately so a reset clears the search fetch/URL
    // in one pass instead of trailing 300ms behind the cleared inputs.
    setDebouncedSearchQuery("");
  };

  const resetHistoryFilters = () => {
    setHistorySearchQuery("");
    setHistoryStatusFilter("all");
    setHistoryDateFrom("");
    setHistoryDateTo("");
    setCustomerFilter(undefined);
    setBillPage(1);
    setDebouncedSearchQuery("");
  };

  const handleActiveSearchChange = (value: string) => {
    setActiveSearchQuery(value);
    setBillPage(1);
  };

  const handleActiveDateFromChange = (value: string) => {
    setActiveDateFrom(value);
    setBillPage(1);
  };

  const handleActiveDateToChange = (value: string) => {
    setActiveDateTo(value);
    setBillPage(1);
  };

  const handleHistorySearchChange = (value: string) => {
    setHistorySearchQuery(value);
    setBillPage(1);
  };

  const handleHistoryDateFromChange = (value: string) => {
    setHistoryDateFrom(value);
    setBillPage(1);
  };

  const handleHistoryDateToChange = (value: string) => {
    setHistoryDateTo(value);
    setBillPage(1);
  };

  const handleHistoryStatusFilterChange = (
    value: "all" | "paid" | "closed" | "voided",
  ) => {
    setHistoryStatusFilter(value);
    setBillPage(1);
  };

  // After a void the operator sits on the Active tab, but the just-voided bill
  // is no longer active — send them to History filtered to Voided so the bill
  // (and its void reason / audit trail) stays visible instead of vanishing.
  const handleBillVoided = React.useCallback(() => {
    setActiveTab("history");
    setHistoryStatusFilter("voided");
    setBillPage(1);
  }, []);

  const cancelOrderCandidate = React.useMemo(
    () =>
      cancelOrderConfirm === null
        ? null
        : Object.values(orders)
            .flat()
            .find((order) => order.id === cancelOrderConfirm) || null,
    [cancelOrderConfirm, orders],
  );

  // Filtered lists
  const activeBills = React.useMemo(() => {
    const list = bills.filter((b) => isActiveBillStatus(b.status));

    list.sort(
      (a, b) =>
        new Date(b.created_at).getTime() - new Date(a.created_at).getTime(),
    );
    return list;
  }, [bills]);

  const historyBills = React.useMemo(() => {
    // Voided bills belong in history alongside paid/closed — omitting them here
    // (the client-side mirror of billStatusFilter) would re-hide a voided bill
    // even after the server returned it.
    const list = bills.filter(
      (b) =>
        b.status === "paid" ||
        b.status === "closed" ||
        b.status === "voided",
    );

    list.sort(
      (a, b) =>
        new Date(b.created_at).getTime() - new Date(a.created_at).getTime(),
    );
    return list;
  }, [bills]);

  const visibleBills = activeTab === "active" ? activeBills : historyBills;
  // The bill payload carries the venue's resolved currency (display_currency,
  // then default_currency, resolved backend-side). Prefer it over the separate
  // business fetch, which can still be in flight or have failed, so the money
  // a row shows is labelled by the same row that carries the amount (#852).
  // Every row in this list belongs to one business, so the first labelled row
  // speaks for the list.
  const billsCurrency = React.useMemo(() => {
    const labelled =
      bills.find((bill) => bill?.currency)?.currency ||
      (globalBills ?? []).find((bill) => bill?.currency)?.currency;
    return labelled || businessCurrency;
  }, [bills, globalBills, businessCurrency]);
  const activeListFiltersApplied =
    Boolean(debouncedSearchQuery.trim()) ||
    Boolean(activeDateFrom) ||
    Boolean(activeDateTo) ||
    customerFilter != null;
  const activeGlobalBills = React.useMemo(
    () => (globalBills ?? []).filter((bill) => isActiveBillStatus(bill.status)),
    [globalBills],
  );
  // One snapshot for unfiltered Active: live reconcile rows, not the paged
  // list. Mixing those sources was the 2/$73.90 → 3/$477.51 first-paint jump (#96).
  const headerResultCount = React.useMemo(() => {
    if (activeTab !== "active" || activeListFiltersApplied) return billTotal;
    if (activeGlobalBills.length === 0) return billTotal;
    if (globalBillsCapped && billTotal > activeGlobalBills.length) {
      return billTotal;
    }
    return activeGlobalBills.length;
  }, [
    activeTab,
    activeListFiltersApplied,
    activeGlobalBills,
    billTotal,
    globalBillsCapped,
  ]);
  // Related to issue 371: paint the same live snapshot the header counts.
  const listedActiveBills = React.useMemo(() => {
    if (activeListFiltersApplied || activeGlobalBills.length === 0) {
      return activeBills;
    }
    return [...activeGlobalBills].sort(
      (a, b) =>
        new Date(b.created_at).getTime() - new Date(a.created_at).getTime(),
    );
  }, [activeListFiltersApplied, activeGlobalBills, activeBills]);
  const visibleBillValue = React.useMemo(() => {
    if (
      activeTab === "active" &&
      !activeListFiltersApplied &&
      activeGlobalBills.length > 0
    ) {
      return activeGlobalBills.reduce(
        (sum, bill) => sum + (bill.total_amount || 0),
        0,
      );
    }
    return visibleBills.reduce(
      (sum, bill) => sum + (bill.total_amount || 0),
      0,
    );
  }, [activeTab, activeListFiltersApplied, activeGlobalBills, visibleBills]);
  const formattedVisibleValue = formatCurrencyIntl(
    visibleBillValue,
    billsCurrency,
    undefined,
    currentLocale,
  );
  const queryIsScoped =
    activeTab !== "active" || activeListFiltersApplied;
  const listHonesty = React.useMemo(
    () =>
      resolveBillsListHonesty({
        listHydrated,
        listLoadFailed,
        globalBillsFailed,
        resultCount: headerResultCount,
        visibleValue: visibleBillValue,
        queryIsScoped,
      }),
    [
      listHydrated,
      listLoadFailed,
      globalBillsFailed,
      headerResultCount,
      visibleBillValue,
      queryIsScoped,
    ],
  );
  const retryBillsList = React.useCallback(() => {
    onRefreshLiveBills?.();
    void loadBills();
  }, [onRefreshLiveBills, loadBills]);
  // Pending count only once the shared orders feed has settled — avoids the
  // 1 → 0 → 3 flash while dashboard reconcile is still in flight (#96).
  // When the prop is omitted (tests), count from whatever orders are present.
  const pendingOrderCount =
    globalOrdersLoaded === false
      ? 0
      : Object.values(orders)
          .flat()
          .filter((order) => order.status === "pending").length;
  const canUseBillActions =
    (kitchenOrdersEnabled || !hasAccess || isSuspended) && hasAccess;

  // #793: the Cuentas rail badge counts pending approvals, so landing here
  // with a non-empty queue must surface the approval list. The decision is
  // taken exactly once, on ARRIVAL — the first render after the skeleton
  // gate clears (same composite as the early return below, so the anchor is
  // in the DOM on that commit). Landing with an empty queue latches with no
  // scroll: a later SSE/poll 0→1 transition must never yank the operator's
  // scroll position mid-work (adversarial finding O1).
  const approvalSurfacedRef = useRef(false);
  const pendingOrdersAnchorRef = useRef<HTMLDivElement | null>(null);
  const billsFeedsSettled =
    !loading && globalOrdersLoaded !== false && globalBillsLoaded !== false;
  useEffect(() => {
    if (approvalSurfacedRef.current || !billsFeedsSettled) return;
    // Latch on the arrival render regardless of whether a scroll happens —
    // only a landing with pending tickets may move the viewport.
    approvalSurfacedRef.current = true;
    if (pendingOrderCount === 0) return;
    const el = pendingOrdersAnchorRef.current;
    // jsdom and some embedded webviews lack Element.scrollIntoView.
    if (el && typeof el.scrollIntoView === "function") {
      el.scrollIntoView({ block: "start", behavior: "smooth" });
    }
  }, [billsFeedsSettled, pendingOrderCount]);

  const displayBills = React.useMemo(() => {
    const sourceBills =
      globalBills && globalBills.length > 0 ? globalBills : activeBills;
    const active = sourceBills.filter((bill) =>
      isActiveBillStatus(bill.status),
    );
    // A-03: append the recently-completed slice so the kanban's Completed
    // column (which filters paid/closed itself) has data. Dedupe by id with
    // the active copy winning — a bill mid-transition must not render twice.
    const activeIds = new Set(active.map((bill) => bill.id));
    const completed = recentCompletedBills.filter(
      (bill) => !activeIds.has(bill.id),
    );
    return [...active, ...completed];
  }, [activeBills, globalBills, recentCompletedBills]);

  const billPendingClose = React.useMemo(() => {
    if (closeBillConfirm === null) return null;
    const id = closeBillConfirm;
    return (
      activeBills.find((b) => b.id === id) ??
      historyBills.find((b) => b.id === id) ??
      displayBills.find((b) => b.id === id) ??
      (selectedBill?.bill.id === id ? selectedBill.bill : null)
    );
  }, [closeBillConfirm, activeBills, historyBills, displayBills, selectedBill]);

  const closeConfirmUsesUnpaidCopy =
    billPendingClose != null &&
    billPendingClose.status === "open" &&
    (billPendingClose.paid_amount ?? 0) === 0;

  // If business is not entitled (inactive or suspended), show lockdown.
  if (!accessLoading && (!hasAccess || isSuspended)) {
    return (
      <DashboardLockedTabView
        title={tString("title")}
        subtitle={tString("subtitle")}
        businessId={businessId}
      />
    );
  }

  // Loading state. S-9: the skeleton fills the shell's content slot so the
  // Bills title stays mounted. Header stats are omitted on purpose — result
  // counts and visible value are exactly what is still loading, and rendering
  // them as zeros reads as "no bills" rather than "not yet".
  // Also wait for the first global bills + orders reconcile so header
  // counts don't paint a paged/capped intermediate snapshot (#96). Only
  // gate when the dashboard explicitly reports loaded=false.
  if (
    loading ||
    globalOrdersLoaded === false ||
    globalBillsLoaded === false
  ) {
    return (
      <DashboardTabShell
        header={{ title: tString("title"), subtitle: tString("subtitle") }}
        loading={<BillsSkeleton />}
      />
    );
  }

  // Bill Display Mode (full-screen kanban)
  if (viewMode === "display") {
    return (
      <>
        <BillDisplayMode
          bills={displayBills}
          orders={orders}
          onExit={() => setViewMode("list")}
          onCloseBill={handleCloseBill}
          onCreateBill={() => setShowBillCreator(true)}
          onBillUpdated={loadBills}
          businessId={businessId}
          tString={tString}
          currency={billsCurrency}
          canRecordPayment={paymentCapabilities.canRecordPayment}
          businessTimezone={businessTimezone}
          country={businessCountry}
        />
        {globalBillsCapped && (
          <div className="fixed bottom-4 left-1/2 z-50 -translate-x-1/2 rounded-lg border border-amber-200 bg-amber-50 px-4 py-2 text-sm text-amber-800 shadow-lg">
            {tString("warnings.activeBillsCapped")}
          </div>
        )}
        <ConfirmationModal
          isOpen={closeBillConfirm !== null}
          onOpenChange={() => setCloseBillConfirm(null)}
          isDanger
          title={tString(
            closeConfirmUsesUnpaidCopy
              ? "confirmation.closeWithoutPaymentTitle"
              : "confirmation.closeBillTitle",
          )}
          description={tString(
            closeConfirmUsesUnpaidCopy
              ? "confirmation.closeWithoutPaymentDescription"
              : "confirmation.closeBillDescription",
          )}
          confirmLabel={tString(
            closeConfirmUsesUnpaidCopy
              ? "confirmation.closeWithoutPaymentConfirm"
              : "confirmation.closeBillConfirm",
          )}
          cancelLabel={tString("confirmation.closeBillCancel")}
          onConfirm={confirmCloseBill}
        />
      </>
    );
  }

  return (
    <DashboardTabShell
      header={{
        title: tString("title"),
        subtitle: tString("subtitle"),
        stats: listHonesty.showMoneyStats
          ? [
              { label: tString("header.results"), value: headerResultCount },
              {
                label: tString("header.visibleValue"),
                value: formattedVisibleValue,
              },
              {
                // PageHeader renders "{value} {label}" — keep the label's number
                // in agreement with the count ("1 Pending order").
                label:
                  pendingOrderCount === 1
                    ? tString("header.pendingOrder")
                    : tString("header.pendingOrders"),
                value: pendingOrderCount,
              },
            ]
          : undefined,
        actions: canUseBillActions ? (
          <>
            <Button
              variant="bordered"
              className="border-warm-200 bg-white/75 text-ink-700 font-medium hover:bg-white"
              radius="full"
              startContent={<Maximize className="w-4 h-4" />}
              onPress={() => setViewMode("display")}
            >
              {tString("buttons.launchDisplay")}
            </Button>
            <Button
              className="bg-brand text-white font-medium hover:bg-brand-dark"
              radius="full"
              startContent={<Plus className="w-4 h-4" />}
              onPress={() => setShowBillCreator(true)}
            >
              {tString("buttons.createBill")}
            </Button>
          </>
        ) : undefined,
      }}
    >
      {/* Kitchen & Orders Activation Card (only shows when disabled) — the
          Disable button moved to Settings → Business Profile (§B); this
          activation card stays so operators can re-enable from Bills. */}
      {!listHonesty.showFailureState || listHonesty.keepLastKnownList ? (
        <KitchenOrdersToggle
          businessId={businessId}
          isLocked={!hasAccess || isSuspended}
          variant="card"
          onStatusChange={onKitchenStatusChange}
          externalEnabled={kitchenOrdersEnabled}
          externalLoading={kitchenOrdersLoading}
        />
      ) : null}

      {listHonesty.showFailureState && !listHonesty.keepLastKnownList ? (
        <EmptyState
          data-testid="bills-list-load-failed"
          icon={AlertTriangle}
          title={tString("emptyState.listLoadFailed")}
          subtitle={tString("emptyState.listLoadFailedHint")}
          actionLabel={tString("emptyState.listLoadRetry")}
          onAction={retryBillsList}
        />
      ) : null}

      {listHonesty.showFailureState && listHonesty.keepLastKnownList ? (
        <div
          data-testid="bills-list-stale-banner"
          className="flex flex-wrap items-center justify-between gap-3 rounded-xl border border-amber-200 bg-amber-50 px-4 py-3 text-sm text-amber-900"
        >
          <p>{tString("emptyState.listLoadFailedHint")}</p>
          <button
            type="button"
            className="font-medium underline underline-offset-2"
            onClick={retryBillsList}
          >
            {tString("emptyState.listLoadRetry")}
          </button>
        </div>
      ) : null}

      {/* Only show content if kitchen/orders are enabled or still loading */}
      {(!listHonesty.showFailureState || listHonesty.keepLastKnownList) &&
        (kitchenOrdersEnabled ||
        kitchenOrdersLoading ||
        !hasAccess ||
        isSuspended) && (
        <>
          {/* Compact so Active rows stay on the first screen (issue 371). */}
          <div
            ref={pendingOrdersAnchorRef}
            data-testid="pending-orders-anchor"
            className="scroll-mt-4"
          />
          <PendingOrdersSection
            compact
            orders={orders}
            actionLoading={actionLoading}
            onApproveOrder={handleApproveOrder}
            onRejectOrder={handleRejectOrder}
            tString={tString}
            isPollingActive={true}
            businessTimezone={businessTimezone}
          />

          <Modal
            isOpen={cancelOrderConfirm !== null}
            onOpenChange={() => {
              setCancelOrderConfirm(null);
              setCancelOrderReason("");
            }}
            size="md"
          >
            <ModalContent>
              {(onClose) => (
                <>
                  <ModalHeader>
                    {tString("confirmation.cancelOrderTitle")}
                  </ModalHeader>
                  <ModalBody className="space-y-4">
                    <p className="text-ink-700">
                      {tString("confirmation.cancelOrderDescription").replace(
                        "{orderNumber}",
                        cancelOrderCandidate?.order_number || "",
                      )}
                    </p>
                    <Textarea
                      label={tString("confirmation.cancelOrderReasonLabel")}
                      placeholder={tString(
                        "confirmation.cancelOrderReasonPlaceholder",
                      )}
                      value={cancelOrderReason}
                      onValueChange={setCancelOrderReason}
                      minRows={3}
                    />
                    <p className="text-xs text-ink-600">
                      {tString("confirmation.cancelOrderReasonHint")}
                    </p>
                  </ModalBody>
                  <ModalFooter>
                    <Button
                      variant="light"
                      onPress={() => {
                        setCancelOrderReason("");
                        onClose();
                      }}
                    >
                      {tString("confirmation.cancelOrderCancel")}
                    </Button>
                    <Button
                      color="danger"
                      isLoading={actionLoading === cancelOrderConfirm}
                      onPress={async () => {
                        await confirmRejectOrder();
                        onClose();
                      }}
                    >
                      {tString("confirmation.cancelOrderConfirm")}
                    </Button>
                  </ModalFooter>
                </>
              )}
            </ModalContent>
          </Modal>

          {/* Tab Navigation — bare strip above the panel, like every other tab strip */}
          <SegmentedTabs
            tabs={[
              {
                key: "active",
                label: tString("tabs.active"),
                // Same source as header results (headerResultCount).
                // Unlimited cap so tab badge never collapses to "9+" while
                // the header shows the exact total [L2-16].
                badge: activeTab === "active" ? headerResultCount : undefined,
                badgeCap: null,
              },
              {
                key: "history",
                label: tString("tabs.history"),
                badge: activeTab === "history" ? billTotal : undefined,
                badgeCap: null,
              },
            ]}
            activeKey={activeTab}
            onChange={(key) => {
              setActiveTab(key);
              setBillPage(1);
            }}
            ariaLabel={tString("title")}
          />

          <DashboardTabTransition tabKey={activeTab} direction={0}>
          <PremiumPanel className="overflow-visible" withTexture={false}>
            {/* Tab Content */}
            <div className="p-4 sm:p-6">
              {activeTab === "active" ? (
                <div className="space-y-4">
                  {customerFilter ? (
                    <div className="flex items-center gap-2">
                      <span className="inline-flex items-center gap-1.5 rounded-full border border-brand/20 bg-brand/10 px-3 py-1 text-xs font-medium text-brand-dark">
                        {tString("filters.customerFilter")}
                        <button
                          type="button"
                          aria-label={tString("filters.clearCustomerFilter")}
                          onClick={() => setCustomerFilter(undefined)}
                          className="rounded-full hover:text-brand focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-brand"
                        >
                          <X className="h-3 w-3" aria-hidden="true" />
                        </button>
                      </span>
                    </div>
                  ) : null}
                  <BillFilters
                    searchQuery={activeSearchQuery}
                    onSearchChange={handleActiveSearchChange}
                    dateFrom={activeDateFrom}
                    onDateFromChange={handleActiveDateFromChange}
                    dateTo={activeDateTo}
                    onDateToChange={handleActiveDateToChange}
                    onReset={resetActiveFilters}
                    tString={tString}
                    collapseIdleDates
                  />
                  <BillsTable
                    bills={listedActiveBills}
                    isActive={true}
                    actionLoading={actionLoading}
                    loading={listFetching}
                    onViewBill={handleViewBill}
                    onCloseBill={handleCloseBill}
                    onCreateBill={() => setShowBillCreator(true)}
                    onResetFilters={resetActiveFilters}
                    tString={tString}
                    currency={billsCurrency}
                    locale={currentLocale}
                    businessId={businessId}
                    businessTimezone={businessTimezone}
                  />
                  {activeListFiltersApplied && billTotalPages > 1 && (
                    <div className="flex items-center justify-between gap-3 px-2 py-4">
                      <span className="text-sm text-ink-500">
                        {tString("pagination.totalBills").replace(
                          "{count}",
                          String(billTotal),
                        )}
                      </span>
                      <Pagination
                        total={billTotalPages}
                        page={billPage}
                        onChange={setBillPage}
                        showControls
                        color="primary"
                      />
                    </div>
                  )}
                </div>
              ) : (
                <div className="space-y-6">
                  {customerFilter ? (
                    <div className="flex items-center gap-2">
                      <span className="inline-flex items-center gap-1.5 rounded-full border border-brand/20 bg-brand/10 px-3 py-1 text-xs font-medium text-brand-dark">
                        {tString("filters.customerFilter")}
                        <button
                          type="button"
                          aria-label={tString("filters.clearCustomerFilter")}
                          onClick={() => setCustomerFilter(undefined)}
                          className="rounded-full hover:text-brand focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-brand"
                        >
                          <X className="h-3 w-3" aria-hidden="true" />
                        </button>
                      </span>
                    </div>
                  ) : null}
                  <BillFilters
                    searchQuery={historySearchQuery}
                    onSearchChange={handleHistorySearchChange}
                    dateFrom={historyDateFrom}
                    onDateFromChange={handleHistoryDateFromChange}
                    dateTo={historyDateTo}
                    onDateToChange={handleHistoryDateToChange}
                    statusFilter={historyStatusFilter}
                    onStatusFilterChange={handleHistoryStatusFilterChange}
                    onReset={resetHistoryFilters}
                    tString={tString}
                    showStatusFilter={true}
                  />
                  <BillsTable
                    bills={historyBills}
                    isActive={false}
                    actionLoading={actionLoading}
                    loading={listFetching}
                    onViewBill={handleViewBill}
                    onCloseBill={handleCloseBill}
                    onCreateBill={() => setShowBillCreator(true)}
                    onResetFilters={resetHistoryFilters}
                    tString={tString}
                    currency={billsCurrency}
                    locale={currentLocale}
                    businessId={businessId}
                    businessTimezone={businessTimezone}
                  />
                  {billTotalPages > 1 && (
                    <div className="flex items-center justify-between gap-3 px-2 py-4">
                      <span className="text-sm text-ink-500">
                        {tString("pagination.totalBills").replace(
                          "{count}",
                          String(billTotal),
                        )}
                      </span>
                      <Pagination
                        total={billTotalPages}
                        page={billPage}
                        onChange={setBillPage}
                        showControls
                        color="primary"
                      />
                    </div>
                  )}
                </div>
              )}
            </div>
          </PremiumPanel>
          </DashboardTabTransition>

          {/* Modals */}
          <BillDetailsModal
            isOpen={showBillDetails}
            onClose={() => {
              setShowBillDetails(false);
              setRecordPaymentBillId(null);
              // L9-3: closing the detail clears the addressable deep-link.
              clearBillIdSearchParam();
            }}
            bill={selectedBill}
            onCloseBill={handleCloseBill}
            onBillUpdated={() => {
              if (selectedBill) {
                void handleBillItemsRefresh(selectedBill.bill.id);
              } else {
                void loadBills();
              }
            }}
            businessId={businessId}
            tString={tString}
            currency={selectedBill?.bill?.currency || billsCurrency}
            canRecordPayment={paymentCapabilities.canRecordPayment}
            businessTimezone={businessTimezone}
            country={businessCountry}
            onBillVoided={handleBillVoided}
            onLoadFullHistory={handleLoadFullBillHistory}
            autoOpenRecordPayment={
              selectedBill != null &&
              selectedBill.bill.id === recordPaymentBillId
            }
          />

          <BillCreator
            isOpen={showBillCreator}
            onClose={() => setShowBillCreator(false)}
            businessId={businessId}
            onBillCreated={loadBills}
            initialTableId={deepLinkTableId}
            currency={billsCurrency}
            onViewBill={handleViewBill}
            onRecordPayment={(billId) => {
              setRecordPaymentBillId(billId);
              void handleViewBill(billId);
            }}
          />

          {/* Close Bill Confirmation Modal */}
          <ConfirmationModal
            isOpen={closeBillConfirm !== null}
            onOpenChange={() => setCloseBillConfirm(null)}
            isDanger
            title={tString(
              closeConfirmUsesUnpaidCopy
                ? "confirmation.closeWithoutPaymentTitle"
                : "confirmation.closeBillTitle",
            )}
            description={tString(
              closeConfirmUsesUnpaidCopy
                ? "confirmation.closeWithoutPaymentDescription"
                : "confirmation.closeBillDescription",
            )}
            confirmLabel={tString(
              closeConfirmUsesUnpaidCopy
                ? "confirmation.closeWithoutPaymentConfirm"
                : "confirmation.closeBillConfirm",
            )}
            cancelLabel={tString("confirmation.closeBillCancel")}
            onConfirm={confirmCloseBill}
          />
        </>
      )}
    </DashboardTabShell>
  );
};
