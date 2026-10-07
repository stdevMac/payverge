"use client";

import React, { useEffect, useMemo, useRef, useState } from "react";
import { Button, Chip, Divider, Input, Spinner } from "@nextui-org/react";
import {
  AlertTriangle,
  ArrowLeft,
  CheckCircle,
  Clock,
  DollarSign,
  Eye,
  History,
  Maximize,
  Minimize,
  Plus,
  Search,
  X,
  XCircle,
} from "lucide-react";
import {
  Bill,
  BillHistoryEvent,
  BillItem,
  BillWithItemsResponse,
  getBill,
  isActiveBillStatus,
  parseBillItems,
} from "../../api/bills";
import { Order } from "../../api/orders";
import { isAbortError } from "../../api/tools/abort";
import { formatCurrency as formatCurrencyIntl } from "../../api/currency";
import { formatPreparedUnits } from "@/utils/formatPreparedUnits";
import { useSimpleLocale } from "@/i18n/SimpleTranslationProvider";
import { BillItemEditor } from "./BillItemEditor";
import { BillRecordPayment } from "./BillRecordPayment";
import {
  DATE_TIME_SHORT,
  formatBusinessDateTime,
  formatBusinessTime,
} from "@/utils/businessTime";
import { humanizeDurationMinutes } from "@/utils/humanizeDuration";
import { StatusChip } from "../ui/StatusChip";
import { getBillLocationLabel } from "@/lib/tableLabel";
import { OperatorBillNumber } from "./OperatorBillNumber";
import { shouldAutoSelectBillDetail } from "@/lib/billDisplayAutoSelect";
import {
  isDiscountBillLine,
  isOperatorFoodBillLine,
} from "@/lib/billBundleGrouping";

// ── Display-mode drift fixes + perf recipe (mirror of KitchenDisplayMode) ─────
// The whole component ticks a 1s clock; previously every card was rebuilt
// inline on each tick, re-rendering the entire kanban (~30k DOM nodes at cap).
// The card below is module-scope + React.memo and takes minute-granularity
// `ageMinutes`, so a sub-minute tick recomputes the same age → no card repaint.

/**
 * N-7 / L1-29: single elapsed-label path via humanizeDurationMinutes so
 * multi-day open bills never render "hace 450h 5m".
 */
export const formatElapsedLabel = (
  ageMinutes: number,
  tString: (key: string) => string,
): string => {
  if (ageMinutes < 1) {
    return tString("display.justNow");
  }
  const duration = humanizeDurationMinutes(ageMinutes);
  return tString("display.elapsedAgo").replace("{duration}", duration);
};

interface BillDisplayModeProps {
  bills: Bill[];
  orders: Record<number, Order[]>;
  onExit: () => void;
  onCloseBill: (billId: number) => void;
  onCreateBill: () => void;
  onBillUpdated: () => void;
  businessId: number;
  tString: (key: string) => string;
  // Business default currency for every amount shown in the live
  // KDS-style bills view. Was hardcoded "$" prior to this prop.
  currency?: string;
  /** When false, payment section is read-only (host/kitchen). */
  canRecordPayment?: boolean;
  businessTimezone?: string | null;
  /** Business address / fiscal country — hides US-only tenders (#649). */
  country?: string | null;
}

interface DisplayBillEntry {
  bill: Bill;
  label: string;
  itemCount: number;
  orderCount: number;
  pendingCount: number;
  activeTicketCount: number;
  cancelledCount: number;
  ageMinutes: number;
  notesPreview: string;
  needsAttention: boolean;
  isOverdue: boolean;
}

interface DisplayBillCardProps {
  entry: DisplayBillEntry;
  tone: "attention" | "active" | "completed";
  isSelected: boolean;
  tString: (key: string) => string;
  formatCurrency: (value: number) => string;
  onView: (billId: number) => void;
  onClose: (billId: number) => void;
}

// Module-scope memoized card. The comparator repaints only when a card-relevant
// field changed — critically NOT on the 1s clock tick unless `entry.ageMinutes`
// (integer minutes) actually advanced.
const DisplayBillCard = React.memo<DisplayBillCardProps>(
  function DisplayBillCard({
    entry,
    tone,
    isSelected,
    tString,
    formatCurrency,
    onView,
    onClose,
  }) {
    const toneClasses =
      tone === "attention"
        ? "border-orange-200 hover:border-orange-300 bg-white/95"
        : tone === "active"
          ? "border-emerald-200 hover:border-emerald-300 bg-white/95"
          : "border-warm-200 hover:border-warm-300 bg-white/90";

    return (
      <button
        type="button"
        onClick={() => onView(entry.bill.id)}
        className={`w-full rounded-3xl border p-4 text-left shadow-sm transition-all ${
          isSelected
            ? "border-ink-900 bg-white shadow-lg ring-2 ring-warm-200"
            : toneClasses
        }`}
      >
        <div className="flex items-start justify-between gap-3">
          <div className="min-w-0">
            <p className="text-[11px] font-semibold uppercase tracking-[0.2em] text-ink-500">
              {entry.label}
            </p>
            <h3 className="mt-1 text-lg font-semibold text-ink-900">
              <OperatorBillNumber
                id={entry.bill.id}
                bill_number={entry.bill.bill_number}
                copyLabel={tString("bills.copyBillNumber")}
                copiedLabel={tString("bills.copiedBillNumber")}
              />
            </h3>
          </div>
          {/* Completed column: the honest status pill (was raw
              status.toUpperCase()). Active/attention columns: elapsed age. */}
          {tone === "completed" ? (
            <StatusChip
              kind="bill"
              status={entry.bill.status}
              labelOverride={tString(`billStatuses.${entry.bill.status}`)}
            />
          ) : (
            <Chip
              size="sm"
              variant="flat"
              color={
                entry.isOverdue
                  ? "danger"
                  : entry.pendingCount > 0
                    ? "warning"
                    : "success"
              }
            >
              {formatElapsedLabel(entry.ageMinutes, tString)}
            </Chip>
          )}
        </div>

        <div className="mt-4 flex flex-wrap gap-2">
          <Chip size="sm" variant="flat">
            {formatPreparedUnits(
              entry.itemCount,
              tString,
              "display.preparedUnits",
            )}
          </Chip>
          {entry.pendingCount > 0 ? (
            <Chip size="sm" variant="flat" color="warning">
              {tString("display.pendingTicketsCount").replace(
                "{count}",
                String(entry.pendingCount),
              )}
            </Chip>
          ) : null}
          {entry.activeTicketCount > 0 ? (
            <Chip size="sm" variant="flat" color="primary">
              {tString("display.activeTicketsCount").replace(
                "{count}",
                String(entry.activeTicketCount),
              )}
            </Chip>
          ) : null}
        </div>

        {entry.notesPreview ? (
          <p className="mt-4 line-clamp-2 text-sm text-ink-600">
            {entry.notesPreview}
          </p>
        ) : (
          <p className="mt-4 text-sm text-ink-500">
            {tString("display.noNotes")}
          </p>
        )}

        <div className="mt-5 grid grid-cols-2 gap-3 border-t border-warm-100 pt-4">
          <div>
            <p className="text-xs uppercase tracking-wide text-ink-500">
              {tString("display.cardTotal")}
            </p>
            <p className="mt-1 text-base font-semibold text-ink-900 whitespace-nowrap">
              {formatCurrency(entry.bill.total_amount)}
            </p>
          </div>
          <div>
            <p className="text-xs uppercase tracking-wide text-ink-500">
              {tString("display.cardTickets")}
            </p>
            <p className="mt-1 text-base font-semibold text-ink-900">
              {entry.orderCount}
            </p>
          </div>
        </div>

        <div className="mt-4 flex gap-2">
          <Button
            size="sm"
            variant="flat"
            color="primary"
            className="flex-1"
            startContent={<Eye className="w-4 h-4" />}
            onPress={() => onView(entry.bill.id)}
          >
            {tString("display.inspectBill")}
          </Button>
          {entry.bill.status === "open" ? (
            <Button
              size="sm"
              variant="light"
              className="flex-1 text-ink-600"
              startContent={<XCircle className="w-4 h-4" />}
              onPress={() => onClose(entry.bill.id)}
            >
              {tString("buttons.closeBill")}
            </Button>
          ) : null}
        </div>
      </button>
    );
  },
  (prev, next) =>
    prev.entry.bill.id === next.entry.bill.id &&
    prev.entry.bill.status === next.entry.bill.status &&
    prev.entry.bill.bill_number === next.entry.bill.bill_number &&
    prev.entry.bill.total_amount === next.entry.bill.total_amount &&
    prev.entry.label === next.entry.label &&
    prev.entry.itemCount === next.entry.itemCount &&
    prev.entry.orderCount === next.entry.orderCount &&
    prev.entry.pendingCount === next.entry.pendingCount &&
    prev.entry.activeTicketCount === next.entry.activeTicketCount &&
    prev.entry.ageMinutes === next.entry.ageMinutes &&
    prev.entry.notesPreview === next.entry.notesPreview &&
    prev.entry.isOverdue === next.entry.isOverdue &&
    prev.tone === next.tone &&
    prev.isSelected === next.isSelected &&
    prev.tString === next.tString &&
    prev.formatCurrency === next.formatCurrency &&
    prev.onView === next.onView &&
    prev.onClose === next.onClose,
);

const BillDisplayMode: React.FC<BillDisplayModeProps> = ({
  bills,
  orders,
  onExit,
  onCloseBill,
  onCreateBill,
  onBillUpdated,
  businessId,
  tString,
  currency = "USD",
  canRecordPayment = true,
  businessTimezone = null,
  country = null,
}) => {
  const [isFullscreen, setIsFullscreen] = useState(false);
  const [currentTime, setCurrentTime] = useState(new Date());
  const [searchQuery, setSearchQuery] = useState("");
  const [selectedBill, setSelectedBill] =
    useState<BillWithItemsResponse | null>(null);
  const [detailLoading, setDetailLoading] = useState(false);
  const [isEditing, setIsEditing] = useState(false);
  // "Already auto-selected once" is a ref, never state: as state it was also an
  // effect dependency, so flipping it re-ran the effect whose cleanup aborted
  // the fetch it had just started (auto-select never loaded).
  const hasAutoSelectedRef = useRef(false);
  const containerRef = useRef<HTMLDivElement>(null);
  const searchInputRef = useRef<HTMLInputElement | null>(null);

  useEffect(() => {
    const timer = setInterval(() => setCurrentTime(new Date()), 1000);
    return () => clearInterval(timer);
  }, []);

  useEffect(() => {
    const handleFullscreenChange = () => {
      setIsFullscreen(Boolean(document.fullscreenElement));
    };

    document.addEventListener("fullscreenchange", handleFullscreenChange);
    return () => {
      document.removeEventListener("fullscreenchange", handleFullscreenChange);
    };
  }, []);

  const toggleFullscreen = () => {
    if (!document.fullscreenElement) {
      containerRef.current?.requestFullscreen().catch((error) => {
        console.error(`Error enabling fullscreen: ${error.message}`);
      });
      return;
    }

    document.exitFullscreen().catch((error) => {
      console.error(`Error exiting fullscreen: ${error.message}`);
    });
  };

  const { locale } = useSimpleLocale();
  // Operator-locale-aware number grouping (Audit A-04). Memoized so the
  // module-scope card memo isn't defeated by a fresh closure every clock tick.
  const formatCurrency = React.useCallback(
    (amount: number) => formatCurrencyIntl(amount, currency, undefined, locale),
    [currency, locale],
  );

  const formatDateTime = (dateString: string) =>
    formatBusinessDateTime(dateString, locale, businessTimezone, DATE_TIME_SHORT);

  const getElapsedMinutes = (dateString: string) => {
    const created = new Date(dateString);
    const diffMs = currentTime.getTime() - created.getTime();
    return Math.max(0, Math.floor(diffMs / 60000));
  };

  // N-7: reuse the module-scope formatter (no second unbounded hours path).
  const getElapsedLabel = (dateString: string) =>
    formatElapsedLabel(getElapsedMinutes(dateString), tString);

  const getElapsedTone = (minutes: number) => {
    if (minutes >= 60) return "danger";
    if (minutes >= 45) return "warning";
    return "default";
  };

  const getBillItems = (bill: Bill) => parseBillItems(bill.items);

  // Prefer the server aggregate (sellable lines: menu + bundle parents).
  // Fallback matches that contract — kitchen prep counts stay elsewhere.
  const getBillPhysicalQuantity = (bill: Bill) => {
    if (typeof bill.physical_item_quantity === "number") {
      return bill.physical_item_quantity;
    }
    return getBillItems(bill).reduce((sum: number, item: any) => {
      const type = item.item_type || item.itemType || "menu_item";
      if (type === "bundle_item" || type === "discount") return sum;
      return sum + (item.quantity || 1);
    }, 0);
  };

  const getUniqueOrdersForBill = (billId: number) => {
    const billOrders = orders[billId] ?? [];
    return billOrders.filter(
      (order, index, array) =>
        array.findIndex((candidate) => candidate.id === order.id) === index,
    );
  };

  const getBillLabel = (bill: Bill) =>
    // Shared location label (named table wins → counter → delivery). Was a raw
    // "{Table} {table_id}" literal that leaked the internal id and never
    // honored a renamed table, drifting from BillsTable's label logic.
    getBillLocationLabel(bill, {
      table: tString("table"),
      counter: tString("counter"),
      delivery: tString("delivery"),
    });

  const matchesSearch = (bill: Bill, label: string) => {
    const query = searchQuery.trim().toLowerCase();
    if (!query) return true;

    return [
      bill.bill_number,
      label,
      bill.notes,
      String(bill.table_id ?? ""),
      String(bill.counter_id ?? ""),
      String(bill.total_amount),
    ]
      .filter(Boolean)
      .some((value) => value.toLowerCase().includes(query));
  };

  const buildEntry = (bill: Bill): DisplayBillEntry | null => {
    const label = getBillLabel(bill);
    if (!matchesSearch(bill, label)) {
      return null;
    }

    const uniqueOrders = getUniqueOrdersForBill(bill.id);
    const pendingCount = uniqueOrders.filter(
      (order) => order.status === "pending",
    ).length;
    const cancelledCount = uniqueOrders.filter(
      (order) => order.status === "cancelled",
    ).length;
    const activeTicketCount = uniqueOrders.filter((order) =>
      ["approved", "in_kitchen", "ready", "delivered"].includes(order.status),
    ).length;
    const ageMinutes = getElapsedMinutes(bill.created_at);
    const notesPreview = bill.notes?.trim() || "";
    const needsAttention = pendingCount > 0 || ageMinutes >= 45;
    const isOverdue = ageMinutes >= 60;

    return {
      bill,
      label,
      itemCount: getBillPhysicalQuantity(bill),
      orderCount: uniqueOrders.length,
      pendingCount,
      activeTicketCount,
      cancelledCount,
      ageMinutes,
      notesPreview,
      needsAttention,
      isOverdue,
    };
  };

  const openBillEntries = bills
    .filter((bill) => isActiveBillStatus(bill.status))
    .map(buildEntry)
    .filter((entry): entry is DisplayBillEntry => entry !== null)
    .sort((left, right) => {
      if (Number(right.needsAttention) !== Number(left.needsAttention)) {
        return Number(right.needsAttention) - Number(left.needsAttention);
      }
      if (right.pendingCount !== left.pendingCount) {
        return right.pendingCount - left.pendingCount;
      }
      if (right.ageMinutes !== left.ageMinutes) {
        return right.ageMinutes - left.ageMinutes;
      }
      return (
        new Date(right.bill.created_at).getTime() -
        new Date(left.bill.created_at).getTime()
      );
    });

  const attentionEntries = openBillEntries.filter(
    (entry) => entry.needsAttention,
  );
  const activeEntries = openBillEntries.filter(
    (entry) => !entry.needsAttention,
  );
  const COMPLETED_COLUMN_LIMIT = 16;
  const completedEntriesAll = bills
    .filter((bill) => bill.status === "paid" || bill.status === "closed")
    .map(buildEntry)
    .filter((entry): entry is DisplayBillEntry => entry !== null)
    .sort(
      (left, right) =>
        new Date(right.bill.updated_at).getTime() -
        new Date(left.bill.updated_at).getTime(),
    );
  // Cap the rendered cards but keep the true count so the column chip is honest
  // ("N", with a "latest 16" note) instead of silently showing the truncated
  // count as if it were the total.
  const completedEntries = completedEntriesAll.slice(0, COMPLETED_COLUMN_LIMIT);
  const completedTruncated = completedEntriesAll.length > completedEntries.length;

  const totalPendingTickets = openBillEntries.reduce(
    (sum, entry) => sum + entry.pendingCount,
    0,
  );

  const navigableEntries = useMemo(
    () => [...attentionEntries, ...activeEntries, ...completedEntries],
    [activeEntries, attentionEntries, completedEntries],
  );

  const defaultSelectionId =
    attentionEntries[0]?.bill.id ??
    activeEntries[0]?.bill.id ??
    completedEntries[0]?.bill.id ??
    null;

  const selectedBillLive = selectedBill
    ? (bills.find((bill) => bill.id === selectedBill.bill.id) ??
      selectedBill.bill)
    : null;

  const selectedBillOrders = selectedBillLive
    ? getUniqueOrdersForBill(selectedBillLive.id)
    : [];

  const selectedPendingTickets = selectedBillOrders.filter(
    (order) => order.status === "pending",
  ).length;
  const selectedActiveTickets = selectedBillOrders.filter((order) =>
    ["approved", "in_kitchen", "ready", "delivered"].includes(order.status),
  ).length;
  const selectedCancelledTickets = selectedBillOrders.filter(
    (order) => order.status === "cancelled",
  ).length;

  // Match BillDetailsModal: discounts have their own emerald band — keep them
  // out of the food/item cards so promo lines never look orderable (#107).
  const selectedVisibleItems = (selectedBill?.items || []).filter(
    isOperatorFoodBillLine,
  );

  const selectedDiscountItems = (selectedBill?.items || []).filter(
    isDiscountBillLine,
  );

  const getNumericDetail = (entry: BillHistoryEvent, key: string) => {
    const value = entry.details?.[key];
    if (typeof value === "number") return value;
    if (typeof value === "string") {
      const parsed = Number(value);
      return Number.isFinite(parsed) ? parsed : null;
    }
    return null;
  };

  const getHistoryCopy = (entry: BillHistoryEvent) => {
    const withReason = (body: string) =>
      entry.reason?.trim()
        ? `${body} ${tString("history.reasonLabel").replace(
            "{reason}",
            entry.reason.trim(),
          )}`
        : body;

    switch (entry.event_type) {
      case "bill.created":
        return {
          title: tString("history.events.billCreated"),
          body: tString("history.events.billCreatedDescription")
            .replace(
              "{count}",
              String(getNumericDetail(entry, "items_count") ?? 0),
            )
            .replace(
              "{total}",
              formatCurrency(getNumericDetail(entry, "total_amount") ?? 0),
            ),
        };
      case "bill.updated":
        return {
          title: tString("history.events.billUpdated"),
          body: tString("history.events.billUpdatedDescription")
            .replace(
              "{before}",
              String(getNumericDetail(entry, "items_before") ?? 0),
            )
            .replace(
              "{after}",
              String(getNumericDetail(entry, "items_after") ?? 0),
            ),
        };
      case "bill.closed":
        return {
          title: tString("history.events.billClosed"),
          body: tString("history.events.billClosedDescription").replace(
            "{total}",
            formatCurrency(
              getNumericDetail(entry, "total_amount") ??
                selectedBillLive?.total_amount ??
                0,
            ),
          ),
        };
      case "bill_item.added":
        return {
          title: tString("history.events.itemAdded").replace(
            "{item}",
            entry.item_name || tString("unknownItem"),
          ),
          body: tString("history.events.itemAddedDescription")
            .replace(
              "{quantity}",
              String(getNumericDetail(entry, "quantity") ?? 0),
            )
            .replace(
              "{subtotal}",
              formatCurrency(getNumericDetail(entry, "subtotal") ?? 0),
            ),
        };
      case "bill_item.removed":
        return {
          title: tString("history.events.itemRemoved").replace(
            "{item}",
            entry.item_name || tString("unknownItem"),
          ),
          body: tString("history.events.itemRemovedDescription")
            .replace(
              "{quantity}",
              String(getNumericDetail(entry, "quantity") ?? 0),
            )
            .replace(
              "{subtotal}",
              formatCurrency(getNumericDetail(entry, "subtotal") ?? 0),
            ),
        };
      case "bill_item.voided":
        return {
          title: tString("history.events.itemVoided").replace(
            "{item}",
            entry.item_name || tString("unknownItem"),
          ),
          body: withReason(
            tString("history.events.itemVoidedDescription")
              .replace(
                "{quantity}",
                String(getNumericDetail(entry, "quantity") ?? 0),
              )
              .replace(
                "{subtotal}",
                formatCurrency(getNumericDetail(entry, "subtotal") ?? 0),
              ),
          ),
        };
      case "bill_item.quantity_updated":
        return {
          title: tString("history.events.itemQuantityUpdated").replace(
            "{item}",
            entry.item_name || tString("unknownItem"),
          ),
          body: tString("history.events.itemQuantityUpdatedDescription")
            .replace(
              "{before}",
              String(getNumericDetail(entry, "quantity_before") ?? 0),
            )
            .replace(
              "{after}",
              String(getNumericDetail(entry, "quantity_after") ?? 0),
            ),
        };
      case "order.approved":
        return {
          title: tString("history.events.orderApproved").replace(
            "{orderNumber}",
            entry.order_number || "",
          ),
          // Guest-checkout orders are billed atomically at checkout, so the
          // backend logs items_added: 0 + already_billed on approval.
          body:
            entry.details?.already_billed === true
              ? tString("history.events.orderApprovedAlreadyBilledDescription")
              : tString("history.events.orderApprovedDescription").replace(
                  "{count}",
                  String(getNumericDetail(entry, "items_added") ?? 0),
                ),
        };
      case "order.cancelled":
        return {
          title: tString("history.events.orderCancelled").replace(
            "{orderNumber}",
            entry.order_number || "",
          ),
          body:
            entry.reason?.trim() ||
            tString("history.events.orderCancelledDescription"),
        };
      default:
        return {
          title: entry.event_type,
          body: "",
        };
    }
  };

  // Stable identity (only setState deps) so it can be passed straight to the
  // memoized card without breaking its comparator.
  // L2-27: request-id gate so abort cleanup cannot leave detailLoading stuck
  // and so a superseded response cannot clobber a newer selection.
  const detailRequestIdRef = useRef(0);

  const handleViewBill = React.useCallback(
    async (billId: number, signal?: AbortSignal) => {
      const requestId = detailRequestIdRef.current + 1;
      detailRequestIdRef.current = requestId;
      setDetailLoading(true);
      setIsEditing(false);
      try {
        const details = await getBill(billId, signal);
        if (requestId !== detailRequestIdRef.current) return;
        setSelectedBill(details);
      } catch (error) {
        // An aborted request (component unmounted / selection changed) is not a
        // real failure — axios raises a CanceledError (code ERR_CANCELED).
        if (isAbortError(error)) return;
        console.error("Error loading bill details:", error);
      } finally {
        // Clear spinner only for the live request (not a superseded one).
        if (requestId === detailRequestIdRef.current) {
          setDetailLoading(false);
        }
      }
    },
    [],
  );

  const handleItemsChanged = async () => {
    onBillUpdated();
    if (!selectedBill) return;

    try {
      const details = await getBill(selectedBill.bill.id);
      setSelectedBill(details);
    } catch (error) {
      console.error("Error refreshing bill:", error);
    }
  };

  const closeDetailPanel = () => {
    setSelectedBill(null);
    setIsEditing(false);
  };

  useEffect(() => {
    // L2-27: do NOT list detailLoading in deps — handleViewBill sets it true,
    // which re-ran this effect, aborted its own request, and left the spinner
    // forever (hasAutoSelected blocked retry). Request-id gate owns races.
    // The once-only latch is a ref for the same reason: as a dependency it made
    // the effect abort its own fetch on the very next render.
    if (
      !shouldAutoSelectBillDetail({
        hasAutoSelected: hasAutoSelectedRef.current,
        defaultSelectionId,
      })
    ) {
      return;
    }

    hasAutoSelectedRef.current = true;
    // Cancel the in-flight bill fetch if this effect re-runs or the component
    // unmounts before it resolves, so a stale response can't clobber state.
    const controller = new AbortController();
    void handleViewBill(defaultSelectionId, controller.signal);
    return () => controller.abort();
    // handleViewBill is stable (useCallback([])).
  }, [defaultSelectionId, handleViewBill]);

  useEffect(() => {
    const isTypingTarget = (target: EventTarget | null) => {
      if (!(target instanceof HTMLElement)) return false;
      const tagName = target.tagName.toLowerCase();
      return (
        tagName === "input" ||
        tagName === "textarea" ||
        target.isContentEditable
      );
    };

    const moveSelection = (direction: 1 | -1) => {
      if (navigableEntries.length === 0) return;

      const currentIndex = selectedBill
        ? navigableEntries.findIndex(
            (entry) => entry.bill.id === selectedBill.bill.id,
          )
        : -1;

      const nextIndex =
        currentIndex === -1
          ? direction === 1
            ? 0
            : navigableEntries.length - 1
          : (currentIndex + direction + navigableEntries.length) %
            navigableEntries.length;

      void handleViewBill(navigableEntries[nextIndex].bill.id);
    };

    const onKeyDown = (event: KeyboardEvent) => {
      const targetIsTyping = isTypingTarget(event.target);

      if (event.key === "Escape") {
        if (isEditing) {
          event.preventDefault();
          setIsEditing(false);
          return;
        }

        if (selectedBill) {
          event.preventDefault();
          closeDetailPanel();
        }
        return;
      }

      if (targetIsTyping) {
        return;
      }

      if (event.key === "/") {
        event.preventDefault();
        searchInputRef.current?.focus();
        searchInputRef.current?.select();
        return;
      }

      if (event.key === "ArrowDown" || event.key.toLowerCase() === "j") {
        event.preventDefault();
        moveSelection(1);
        return;
      }

      if (event.key === "ArrowUp" || event.key.toLowerCase() === "k") {
        event.preventDefault();
        moveSelection(-1);
        return;
      }

      if (event.key.toLowerCase() === "f") {
        event.preventDefault();
        toggleFullscreen();
        return;
      }

      if (event.key.toLowerCase() === "n") {
        event.preventDefault();
        onCreateBill();
        return;
      }

      if (
        event.key.toLowerCase() === "e" &&
        selectedBillLive?.status === "open"
      ) {
        event.preventDefault();
        setIsEditing((current) => !current);
        return;
      }

      if (
        event.key.toLowerCase() === "c" &&
        selectedBillLive?.status === "open" &&
        !isEditing
      ) {
        event.preventDefault();
        onCloseBill(selectedBillLive.id);
      }
    };

    window.addEventListener("keydown", onKeyDown);
    return () => {
      window.removeEventListener("keydown", onKeyDown);
    };
    // handleViewBill is stable (useCallback([])).
  }, [
    isEditing,
    navigableEntries,
    onCloseBill,
    onCreateBill,
    selectedBill,
    selectedBillLive,
    handleViewBill,
  ]);

  const renderBillCard = (
    entry: DisplayBillEntry,
    tone: "attention" | "active" | "completed",
  ) => (
    <DisplayBillCard
      key={entry.bill.id}
      entry={entry}
      tone={tone}
      isSelected={selectedBill?.bill.id === entry.bill.id}
      tString={tString}
      formatCurrency={formatCurrency}
      onView={handleViewBill}
      onClose={onCloseBill}
    />
  );

  const renderColumn = (
    title: string,
    subtitle: string,
    entries: DisplayBillEntry[],
    tone: "attention" | "active" | "completed",
    emptyMessage: string,
    // When the rendered `entries` are a truncated view, `truncatedNote` labels
    // the cut ("Latest 16") so the count chip doesn't misrepresent the total.
    truncatedNote?: string,
  ) => (
    <section className="flex min-w-[320px] max-w-[420px] flex-1 flex-col rounded-2xl border border-warm-100 bg-white shadow-sm">
      <div className="border-b border-warm-200/70 px-5 py-4">
        <div className="flex items-start justify-between gap-3">
          <div>
            <h2 className="text-base font-semibold text-ink-900">{title}</h2>
            <p className="mt-1 text-sm text-ink-500">{subtitle}</p>
            {truncatedNote ? (
              <p className="mt-0.5 text-xs text-ink-400">{truncatedNote}</p>
            ) : null}
          </div>
          <Chip
            size="sm"
            variant="flat"
            color={
              tone === "attention"
                ? "warning"
                : tone === "active"
                  ? "success"
                  : "default"
            }
          >
            {entries.length}
          </Chip>
        </div>
      </div>

      <div className="flex-1 space-y-3 overflow-y-auto px-4 py-4 custom-scrollbar">
        {entries.length ? (
          entries.map((entry) => renderBillCard(entry, tone))
        ) : (
          <div className="flex h-full min-h-[220px] flex-col items-center justify-center rounded-[24px] border border-dashed border-warm-200 bg-warm-50/80 px-6 text-center">
            {tone === "attention" ? (
              <AlertTriangle className="mb-3 h-10 w-10 text-ink-500" />
            ) : tone === "completed" ? (
              <CheckCircle className="mb-3 h-10 w-10 text-ink-500" />
            ) : (
              <DollarSign className="mb-3 h-10 w-10 text-ink-500" />
            )}
            <p className="text-sm text-ink-500">{emptyMessage}</p>
          </div>
        )}
      </div>
    </section>
  );

  return (
    <div
      ref={containerRef}
      className="fixed inset-0 z-50 flex flex-col overflow-hidden bg-[radial-gradient(circle_at_top_left,_rgba(225,239,228,0.95),_transparent_30%),radial-gradient(circle_at_top_right,_rgba(255,235,214,0.9),_transparent_26%),linear-gradient(180deg,#f8fafc_0%,#eef2f7_100%)]"
    >
      <div className="border-b border-warm-200/80 bg-white/80 px-4 py-4 backdrop-blur md:px-6">
        <div className="flex flex-col gap-4 xl:flex-row xl:items-center xl:justify-between">
          <div className="flex items-start gap-3">
            <Button
              isIconOnly
              variant="light"
              aria-label={tString("display.exitDisplayAria")}
              className="mt-1 text-ink-700"
              onPress={onExit}
            >
              <X className="h-5 w-5" />
            </Button>

            <div>
              <div className="flex items-center gap-2 text-ink-900">
                <DollarSign className="h-5 w-5" />
                <h1 className="text-lg font-semibold tracking-tight sm:text-xl">
                  {tString("display.title")}
                </h1>
              </div>
              <p className="mt-1 text-sm text-ink-500">
                {tString("display.subtitle")}
              </p>
              <div className="mt-2 flex items-center gap-2 text-xs text-ink-500">
                <span className="h-2 w-2 rounded-full bg-emerald-500" />
                <span>{tString("display.liveBadge")}</span>
                <span>&bull;</span>
                <span>
                  {formatBusinessTime(currentTime, locale, businessTimezone, {
                    hour: "2-digit",
                    minute: "2-digit",
                  })}
                </span>
              </div>
            </div>
          </div>

          <div className="flex flex-col gap-3 xl:min-w-[520px]">
            <div className="grid grid-cols-2 gap-3 md:grid-cols-4">
              <div className="rounded-2xl border border-white/70 bg-white/80 px-3 py-2 shadow-sm sm:px-4 sm:py-3">
                <p className="text-xs uppercase tracking-wide text-ink-500">
                  {tString("display.stats.open")}
                </p>
                <p className="mt-1 text-xl font-semibold text-ink-900 sm:text-2xl">
                  {openBillEntries.length}
                </p>
              </div>
              <div className="rounded-2xl border border-white/70 bg-white/80 px-3 py-2 shadow-sm sm:px-4 sm:py-3">
                <p className="text-xs uppercase tracking-wide text-ink-500">
                  {tString("display.stats.attention")}
                </p>
                <p className="mt-1 text-xl font-semibold text-ink-900 sm:text-2xl">
                  {attentionEntries.length}
                </p>
              </div>
              <div className="rounded-2xl border border-white/70 bg-white/80 px-3 py-2 shadow-sm sm:px-4 sm:py-3">
                <p className="text-xs uppercase tracking-wide text-ink-500">
                  {tString("display.stats.pending")}
                </p>
                <p className="mt-1 text-xl font-semibold text-ink-900 sm:text-2xl">
                  {totalPendingTickets}
                </p>
              </div>
              <div className="rounded-2xl border border-white/70 bg-white/80 px-3 py-2 shadow-sm sm:px-4 sm:py-3">
                <p className="text-xs uppercase tracking-wide text-ink-500">
                  {tString("display.stats.completed")}
                </p>
                <p className="mt-1 text-xl font-semibold text-ink-900 sm:text-2xl">
                  {completedEntries.length}
                </p>
              </div>
            </div>

            <div className="flex flex-col gap-3 md:flex-row">
              <Input
                aria-label={tString("display.searchPlaceholder")}
                placeholder={tString("display.searchPlaceholder")}
                value={searchQuery}
                onValueChange={setSearchQuery}
                ref={searchInputRef}
                startContent={<Search className="h-4 w-4 text-ink-400" />}
                classNames={{
                  inputWrapper:
                    "bg-white/90 border border-white/70 shadow-sm data-[hover=true]:bg-white",
                }}
              />

              <div className="flex gap-2">
                <Button
                  variant="flat"
                  className="bg-ink-900 px-5 text-white"
                  startContent={<Plus className="h-4 w-4" />}
                  onPress={onCreateBill}
                >
                  {tString("buttons.createBill")}
                </Button>
                <Button
                  isIconOnly
                  variant="flat"
                  aria-label={
                    isFullscreen ? "Exit fullscreen" : "Enter fullscreen"
                  }
                  aria-pressed={isFullscreen}
                  className="bg-white/90 text-ink-700 shadow-sm"
                  onPress={toggleFullscreen}
                >
                  {isFullscreen ? (
                    <Minimize className="h-5 w-5" />
                  ) : (
                    <Maximize className="h-5 w-5" />
                  )}
                </Button>
              </div>
            </div>

            <div className="hidden flex-wrap gap-2 xl:flex">
              <Chip size="sm" variant="flat">
                / {tString("display.shortcuts.search")}
              </Chip>
              <Chip size="sm" variant="flat">
                ↑↓ {tString("display.shortcuts.navigate")}
              </Chip>
              <Chip size="sm" variant="flat">
                E {tString("display.shortcuts.edit")}
              </Chip>
              <Chip size="sm" variant="flat">
                C {tString("display.shortcuts.closeBill")}
              </Chip>
              <Chip size="sm" variant="flat">
                F {tString("display.shortcuts.fullscreen")}
              </Chip>
              <Chip size="sm" variant="flat">
                N {tString("display.shortcuts.newBill")}
              </Chip>
              <Chip size="sm" variant="flat">
                Esc {tString("display.shortcuts.back")}
              </Chip>
            </div>
          </div>
        </div>
      </div>

      <div
        className={`relative flex-1 overflow-hidden p-3 transition-all sm:p-4 md:p-6 ${selectedBill ? "xl:pr-[38rem]" : ""}`}
      >
        <div className="flex h-full gap-4 overflow-x-auto pb-2 custom-scrollbar">
          {renderColumn(
            tString("display.attentionBills"),
            tString("display.attentionSubtitle"),
            attentionEntries,
            "attention",
            tString("display.emptyAttention"),
          )}
          {renderColumn(
            tString("display.openBills"),
            tString("display.openSubtitle"),
            activeEntries,
            "active",
            tString("display.emptyActive"),
          )}
          {renderColumn(
            tString("display.completedBills"),
            tString("display.completedSubtitle"),
            completedEntries,
            "completed",
            tString("display.emptyCompleted"),
            completedTruncated
              ? tString("display.latestCount").replace(
                  "{count}",
                  String(COMPLETED_COLUMN_LIMIT),
                )
              : undefined,
          )}
        </div>

        {(selectedBill || detailLoading) && (
          <>
            <div
              className="absolute inset-0 z-10 bg-black/30 xl:hidden"
              onClick={closeDetailPanel}
              aria-hidden="true"
            />
            <aside className="absolute inset-y-0 right-0 z-20 w-full border-l border-warm-200 bg-white/95 shadow-2xl backdrop-blur xl:w-[36rem]">
            {detailLoading ? (
              <div className="flex h-full items-center justify-center">
                <Spinner size="lg" />
              </div>
            ) : selectedBill && selectedBillLive ? (
              <div className="flex h-full flex-col">
                <div className="border-b border-warm-200 bg-warm-50/80 px-4 py-4 md:px-6">
                  <div className="flex items-start justify-between gap-3">
                    <div className="flex items-start gap-3">
                      <Button
                        isIconOnly
                        size="sm"
                        variant="light"
                        aria-label={tString("display.backToBillsAria")}
                        onPress={closeDetailPanel}
                      >
                        <ArrowLeft className="h-4 w-4" />
                      </Button>
                      <div>
                        <div className="flex flex-wrap items-center gap-2">
                          <h2 className="text-xl font-semibold text-ink-900">
                            <OperatorBillNumber
                              id={selectedBillLive.id}
                              bill_number={selectedBillLive.bill_number}
                              copyLabel={tString("bills.copyBillNumber")}
                              copiedLabel={tString("bills.copiedBillNumber")}
                            />
                          </h2>
                          {/* Honest status pill (was raw
                              status.toUpperCase() with an ad-hoc color map that
                              never covered voided/closed correctly). */}
                          <StatusChip
                            kind="bill"
                            status={selectedBillLive.status}
                            labelOverride={tString(
                              `billStatuses.${selectedBillLive.status}`,
                            )}
                          />
                          <Chip
                            size="sm"
                            variant="flat"
                              color={getElapsedTone(
                                getElapsedMinutes(selectedBillLive.created_at),
                              )}
                          >
                            {getElapsedLabel(selectedBillLive.created_at)}
                          </Chip>
                        </div>
                        <p className="mt-1 text-sm text-ink-500">
                          {getBillLabel(selectedBillLive)}
                        </p>
                      </div>
                    </div>

                    <Button
                      isIconOnly
                      variant="light"
                      aria-label={tString("display.closeDetailPanelAria")}
                      onPress={closeDetailPanel}
                      className="min-w-[44px] min-h-[44px]"
                    >
                      <X className="h-5 w-5" />
                    </Button>
                  </div>

                  <div className="mt-4 grid grid-cols-2 gap-3 md:grid-cols-4">
                    <div className="rounded-2xl border border-warm-200 bg-white px-2 py-2 sm:px-3 sm:py-3">
                      <p className="text-xs uppercase tracking-wide text-ink-500">
                        {tString("display.metrics.items")}
                      </p>
                      <p className="mt-1 text-lg font-semibold text-ink-900">
                        {selectedVisibleItems.length}
                      </p>
                    </div>
                    <div className="rounded-2xl border border-warm-200 bg-white px-2 py-2 sm:px-3 sm:py-3">
                      <p className="text-xs uppercase tracking-wide text-ink-500">
                        {tString("display.metrics.total")}
                      </p>
                      <p className="mt-1 text-lg font-semibold text-ink-900 whitespace-nowrap">
                        {formatCurrency(selectedBillLive.total_amount)}
                      </p>
                    </div>
                    <div className="rounded-2xl border border-warm-200 bg-white px-2 py-2 sm:px-3 sm:py-3">
                      <p className="text-xs uppercase tracking-wide text-ink-500">
                        {tString("display.metrics.pendingTickets")}
                      </p>
                      <p className="mt-1 text-lg font-semibold text-ink-900">
                        {selectedPendingTickets}
                      </p>
                    </div>
                    <div className="rounded-2xl border border-warm-200 bg-white px-2 py-2 sm:px-3 sm:py-3">
                      <p className="text-xs uppercase tracking-wide text-ink-500">
                        {tString("display.metrics.remaining")}
                      </p>
                      <p className="mt-1 text-lg font-semibold text-ink-900 whitespace-nowrap">
                        {formatCurrency(
                          Math.max(
                            0,
                              selectedBillLive.total_amount -
                                selectedBillLive.paid_amount,
                          ),
                        )}
                      </p>
                    </div>
                  </div>
                </div>

                <div className="flex-1 space-y-5 overflow-y-auto px-4 py-5 md:px-6 custom-scrollbar">
                  {selectedBillLive.notes ? (
                    <section className="rounded-3xl border border-brand/20 bg-brand/10 p-4">
                      <p className="text-xs font-semibold uppercase tracking-[0.2em] text-brand">
                        {tString("display.notesTitle")}
                      </p>
                      <p className="mt-2 text-sm leading-6 text-brand-dark">
                        {selectedBillLive.notes}
                      </p>
                    </section>
                  ) : null}

                  <section className="rounded-3xl border border-warm-200 bg-white p-4">
                    <div className="flex items-center justify-between gap-3">
                      <div>
                        <h3 className="text-sm font-semibold uppercase tracking-[0.2em] text-ink-500">
                          {tString("display.ticketSummaryTitle")}
                        </h3>
                        <p className="mt-1 text-sm text-ink-500">
                          {tString("display.ticketSummarySubtitle")}
                        </p>
                      </div>
                      <Clock className="h-5 w-5 text-ink-500" />
                    </div>

                    <div className="mt-4 grid grid-cols-3 gap-3">
                      <div className="rounded-2xl bg-amber-50 px-3 py-3 text-center">
                        <p className="text-xs uppercase tracking-wide text-amber-700">
                          {tString("display.ticketMetrics.pending")}
                        </p>
                        <p className="mt-1 text-xl font-semibold text-amber-900">
                          {selectedPendingTickets}
                        </p>
                      </div>
                      <div className="rounded-2xl bg-emerald-50 px-3 py-3 text-center">
                        <p className="text-xs uppercase tracking-wide text-emerald-700">
                          {tString("display.ticketMetrics.active")}
                        </p>
                        <p className="mt-1 text-xl font-semibold text-emerald-900">
                          {selectedActiveTickets}
                        </p>
                      </div>
                      <div className="rounded-2xl bg-rose-50 px-3 py-3 text-center">
                        <p className="text-xs uppercase tracking-wide text-rose-700">
                          {tString("display.ticketMetrics.cancelled")}
                        </p>
                        <p className="mt-1 text-xl font-semibold text-rose-900">
                          {selectedCancelledTickets}
                        </p>
                      </div>
                    </div>
                  </section>

                  <section className="rounded-3xl border border-warm-200 bg-white p-4">
                    <div className="flex items-center justify-between gap-3">
                      <div>
                        <h3 className="text-sm font-semibold uppercase tracking-[0.2em] text-ink-500">
                          {tString("display.itemsTitle")}
                        </h3>
                        <p className="mt-1 text-sm text-ink-500">
                          {tString("display.itemsSubtitle")}
                        </p>
                      </div>

                      {selectedBillLive.status === "open" && isEditing ? (
                        <Button
                          size="sm"
                          variant="flat"
                          color="primary"
                          onPress={() => setIsEditing(false)}
                        >
                          {tString("buttons.doneEditing")}
                        </Button>
                      ) : null}
                    </div>

                    {isEditing && selectedBillLive.status === "open" ? (
                      <div className="mt-4 rounded-2xl border border-brand/20 bg-warm-50 p-3">
                        <BillItemEditor
                          billId={selectedBillLive.id}
                          businessId={businessId}
                          currentItems={selectedBill.items || []}
                          onItemsChanged={handleItemsChanged}
                          tString={tString}
                          currency={currency}
                          locale={locale}
                        />
                      </div>
                    ) : (
                      <div className="mt-4 space-y-3">
                        {selectedVisibleItems.length ? (
                            selectedVisibleItems.map(
                              (item: BillItem, index: number) => (
                            <div
                              key={item.id || index}
                              className="flex items-center justify-between gap-3 rounded-2xl border border-warm-100 bg-warm-50/80 px-4 py-3"
                            >
                              <div className="min-w-0">
                                <p className="truncate font-medium text-ink-900">
                                      {item.quantity || 1}x{" "}
                                      {item.name || tString("unknownItem")}
                                </p>
                                <p className="mt-1 text-sm text-ink-500">
                                  {formatCurrency(item.price)}
                                </p>
                              </div>
                              <p className="text-sm font-semibold text-ink-900">
                                {formatCurrency(item.subtotal)}
                              </p>
                            </div>
                              ),
                            )
                        ) : (
                          <div className="rounded-2xl border border-dashed border-warm-200 bg-warm-50 px-4 py-6 text-center text-sm text-ink-500">
                            {tString("noItemsDescription")}
                          </div>
                        )}

                        {selectedDiscountItems.length ? (
                          <div className="rounded-2xl border border-emerald-200 bg-emerald-50/80 p-4">
                            <p className="text-xs font-semibold uppercase tracking-[0.2em] text-emerald-700">
                              {tString("editItems.autoDiscounts")}
                            </p>
                            <div className="mt-3 space-y-2">
                                {selectedDiscountItems.map(
                                  (item: BillItem, index: number) => {
                                    const orderId = item.order_id ?? item.orderId;
                                    return (
                                      <div
                                        key={item.id || index}
                                        className="flex items-center justify-between gap-3 text-sm text-emerald-900"
                                        data-testid="display-discount-line"
                                      >
                                        <div className="min-w-0">
                                          <span className="block truncate">
                                            {item.name ||
                                              tString("discountLabel")}
                                          </span>
                                          {orderId ? (
                                            <span className="block text-xs text-emerald-800/80">
                                              {tString(
                                                "discountOrderAttribution",
                                              ).replace(
                                                "{orderId}",
                                                String(orderId),
                                              )}
                                            </span>
                                          ) : null}
                                        </div>
                                        <span className="shrink-0 font-semibold tabular-nums">
                                          {formatCurrency(
                                            item.subtotal || item.price,
                                          )}
                                        </span>
                                      </div>
                                    );
                                  },
                                )}
                            </div>
                          </div>
                        ) : null}
                      </div>
                    )}

                    {selectedBillLive.status === "open" && !isEditing ? (
                      <button
                        type="button"
                        onClick={() => setIsEditing(true)}
                        className="mt-4 flex w-full items-center justify-center gap-2 rounded-2xl border-2 border-dashed border-brand/30 bg-brand/10 py-3 text-sm font-medium text-brand-dark transition-colors hover:border-brand hover:bg-brand/10"
                      >
                        <Plus className="h-4 w-4" />
                        {tString("buttons.editItems")}
                      </button>
                    ) : null}
                  </section>

                  <section className="rounded-3xl border border-warm-200 bg-white p-4">
                    <div className="flex items-center justify-between gap-3">
                      <div>
                        <h3 className="text-sm font-semibold uppercase tracking-[0.2em] text-ink-500">
                          {tString("history.title")}
                        </h3>
                        <p className="mt-1 text-sm text-ink-500">
                          {tString("display.historySubtitle")}
                        </p>
                      </div>
                      <History className="h-5 w-5 text-ink-500" />
                    </div>

                    <div className="mt-4 space-y-3">
                      {selectedBill.history?.length ? (
                        selectedBill.history.slice(0, 8).map((entry) => {
                          const copy = getHistoryCopy(entry);
                          return (
                            <div
                              key={entry.id}
                              className="rounded-2xl border border-warm-100 bg-warm-50/80 px-4 py-3"
                            >
                              <div className="flex items-start justify-between gap-3">
                                <div>
                                    <p className="font-medium text-ink-900">
                                      {copy.title}
                                    </p>
                                  {copy.body ? (
                                      <p className="mt-1 text-sm text-ink-600">
                                        {copy.body}
                                      </p>
                                  ) : null}
                                  {entry.actor ? (
                                    <p className="mt-2 text-xs text-ink-500">
                                      {tString("history.byLabel").replace(
                                        "{actor}",
                                        entry.actor,
                                      )}
                                    </p>
                                  ) : null}
                                </div>
                                <p className="whitespace-nowrap text-xs text-ink-500">
                                  {formatDateTime(entry.created_at)}
                                </p>
                              </div>
                            </div>
                          );
                        })
                      ) : (
                        <div className="rounded-2xl border border-dashed border-warm-200 bg-warm-50 px-4 py-6 text-center text-sm text-ink-500">
                          {tString("history.empty")}
                        </div>
                      )}
                    </div>
                  </section>

                  {!isEditing ? (
                    <section className="rounded-3xl border border-warm-200 bg-white p-4">
                      <h3 className="text-sm font-semibold uppercase tracking-[0.2em] text-ink-500">
                        {tString("display.billTotalsTitle")}
                      </h3>

                      <div className="mt-4 space-y-3">
                        <div className="flex items-center justify-between text-sm">
                            <span className="text-ink-500">
                              {tString("subtotal")}
                            </span>
                          <span className="font-semibold text-ink-900">
                            {formatCurrency(selectedBillLive.subtotal)}
                          </span>
                        </div>
                        <div className="flex items-center justify-between text-sm">
                            <span className="text-ink-500">
                              {tString("tax")}
                            </span>
                          <span className="font-semibold text-ink-900">
                            {formatCurrency(selectedBillLive.tax_amount)}
                          </span>
                        </div>
                        {selectedBillLive.service_fee_amount > 0 ? (
                          <div className="flex items-center justify-between text-sm">
                              <span className="text-ink-500">
                                {tString("serviceFee")}
                              </span>
                            <span className="font-semibold text-ink-900">
                                {formatCurrency(
                                  selectedBillLive.service_fee_amount,
                                )}
                            </span>
                          </div>
                        ) : null}
                        <Divider />
                        <div className="flex items-center justify-between text-base">
                            <span className="font-medium text-ink-900">
                              {tString("total")}
                            </span>
                          <span className="font-semibold text-ink-900">
                            {formatCurrency(selectedBillLive.total_amount)}
                          </span>
                        </div>
                        {selectedBillLive.paid_amount > 0 ? (
                          <>
                            <div className="flex items-center justify-between text-sm text-emerald-700">
                              <span>{tString("paid")}</span>
                              <span className="font-semibold">
                                {formatCurrency(selectedBillLive.paid_amount)}
                              </span>
                            </div>
                            {selectedBillLive.tip_amount > 0 ? (
                              <div className="flex items-center justify-between text-sm text-emerald-700">
                                <span>{tString("tips")}</span>
                                <span className="font-semibold">
                                    {formatCurrency(
                                      selectedBillLive.tip_amount,
                                    )}
                                </span>
                              </div>
                            ) : null}
                          </>
                        ) : null}
                      </div>
                    </section>
                  ) : null}

                    {selectedBill &&
                    isActiveBillStatus(selectedBillLive.status) ? (
                    <BillRecordPayment
                      billId={selectedBill.bill.id}
                      billNumber={selectedBill.bill.bill_number}
                      billStatus={selectedBillLive.status}
                      currency={currency}
                      remainingAmount={
                          selectedBillLive.total_amount -
                          selectedBillLive.paid_amount
                      }
                      payments={selectedBill.bill.payments}
                        alternativePayments={
                          selectedBill.bill.alternative_payments
                        }
                      onPaymentRecorded={() => {
                        void handleViewBill(selectedBill.bill.id);
                        onBillUpdated();
                      }}
                      tString={tString}
                      businessId={businessId}
                      compact
                      canRecordPayment={canRecordPayment}
                      country={country}
                    />
                  ) : null}
                </div>

                {selectedBillLive.status === "open" ? (
                  <div className="border-t border-warm-200 bg-white/95 px-4 py-4 md:px-6 space-y-2">
                    <Button
                      fullWidth
                      variant="light"
                      className="text-ink-600"
                      startContent={<XCircle className="h-4 w-4" />}
                      onPress={() => onCloseBill(selectedBillLive.id)}
                    >
                      {selectedBillLive.paid_amount === 0
                        ? tString("buttons.closeWithoutPayment")
                        : tString("buttons.closeBill")}
                    </Button>
                  </div>
                ) : null}
              </div>
            ) : null}
          </aside>
          </>
        )}
      </div>

      <style jsx global>{`
        .custom-scrollbar::-webkit-scrollbar {
          width: 8px;
          height: 8px;
        }

        .custom-scrollbar::-webkit-scrollbar-track {
          background: transparent;
        }

        .custom-scrollbar::-webkit-scrollbar-thumb {
          background-color: rgba(100, 116, 139, 0.22);
          border-radius: 9999px;
        }

        .custom-scrollbar:hover::-webkit-scrollbar-thumb {
          background-color: rgba(100, 116, 139, 0.36);
        }
      `}</style>
    </div>
  );
};

export default BillDisplayMode;
