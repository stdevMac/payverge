import React, { useState, useEffect } from "react";
import { Button } from "@nextui-org/react";
import { Clock, CheckCircle, XCircle, AlertCircle, Truck } from "lucide-react";
import { Order, OrderItem, parseOrderItems } from "../../api/orders";
import { useSimpleLocale } from "@/i18n/SimpleTranslationProvider";
import OperationalAlertClaimStatus from "./operational-alerts/OperationalAlertClaimStatus";
import { DATE_TIME_SHORT, formatBusinessDateTime } from "@/utils/businessTime";
import { formatCountdownLabel } from "@/utils/formatCountdownLabel";

// Re-export for existing import sites / tests.
export { formatCountdownLabel } from "@/utils/formatCountdownLabel";

// K-5: "recently cancelled" means the last 2 hours, capped so an unusual day
// can't flood the review panel. Older cancellations live in bill history.
const RECENT_CANCEL_WINDOW_MS = 2 * 60 * 60 * 1000;
const MAX_CANCELLED_CARDS = 20;
// Dinner-service QA #124: pending queue can spike during a rush — show a
// bounded first page, then expand on demand (cancelled already capped at 20).
const MAX_PENDING_CARDS_COLLAPSED = 6;
const pendingOrderCardClass = "min-w-0 w-full rounded-xl bg-white p-4";
const pendingOrderMetaClass = "text-xs text-ink-600";
const pendingOrderItemClass = "text-sm text-ink-700";
const cancelledOrderCardClass = "min-w-0 w-full rounded-xl bg-white p-4";
// Auto-fit keeps a lone card full-width instead of a ~200px squeezed column
// in a forced 3-col grid (#90 / #121).
const orderCardGridClass =
  "grid grid-cols-1 gap-4 sm:grid-cols-2 xl:grid-cols-[repeat(auto-fit,minmax(280px,1fr))]";

/** Food/bundle lines only — promo discounts are not orderable (#107). */
export function foodOrderItems(itemsJson: string | OrderItem[]): OrderItem[] {
  return parseOrderItems(itemsJson).filter(
    (item) => item.item_type !== "discount",
  );
}

// ── Delivery payment countdown ────────────────────────────────────────────────

interface CountdownProps {
  until: string;
  className?: string;
  expiredLabel: string;
}

function Countdown({ until, className, expiredLabel }: CountdownProps) {
  const [remaining, setRemaining] = useState<number>(() =>
    Math.max(0, new Date(until).getTime() - Date.now()),
  );

  useEffect(() => {
    // Declared before the synchronous first tick so clearInterval never hits
    // the temporal dead zone when the expiry has already passed.
    let id: ReturnType<typeof setInterval> | undefined;
    const tick = () => {
      const ms = Math.max(0, new Date(until).getTime() - Date.now());
      setRemaining(ms);
      if (ms <= 0 && id !== undefined) {
        clearInterval(id);
      }
    };
    tick();
    id = setInterval(tick, 1_000);
    return () => clearInterval(id);
  }, [until]);

  if (remaining <= 0) {
    return <span className={className}>{expiredLabel}</span>;
  }

  return <span className={className}>{formatCountdownLabel(remaining)}</span>;
}

// ─────────────────────────────────────────────────────────────────────────────

interface PendingOrdersSectionProps {
  orders: Record<number, Order[]>;
  actionLoading: number | null;
  onApproveOrder: (orderId: number) => void;
  onRejectOrder: (orderId: number) => void;
  tString: (key: string) => string;
  isPollingActive?: boolean;
  businessTimezone?: string | null;
  /** Strip layout so Active Bills stay on the first screen. */
  compact?: boolean;
}

export const PendingOrdersSection: React.FC<PendingOrdersSectionProps> = ({
  orders,
  actionLoading,
  onApproveOrder,
  onRejectOrder,
  tString,
  isPollingActive: _isPollingActive = false,
  businessTimezone = null,
  compact = false,
}) => {
  const { locale } = useSimpleLocale();
  const [showAllPending, setShowAllPending] = useState(false);

  // Dedupe by order id in a SINGLE Map pass (was O(n²): Array.findIndex inside
  // a filter), then derive the pending/cancelled queues. All memoized on the
  // orders map so a 30s poll tick / unrelated re-render doesn't redo the work.
  const { pendingOrders, cancelledOrders } = React.useMemo(() => {
    const byId = new Map<number, Order & { billId: string }>();
    for (const [billId, billOrders] of Object.entries(orders)) {
      for (const order of billOrders) {
        if (!byId.has(order.id)) byId.set(order.id, { ...order, billId });
      }
    }
    const uniqueOrders = Array.from(byId.values());

    // Pending queue: oldest first (FIFO) so staff work arrival order.
    const pending = uniqueOrders
      .filter((order) => order.status === "pending")
      .sort(
        (a, b) =>
          new Date(a.created_at).getTime() - new Date(b.created_at).getTime(),
      );
    const cancelled = uniqueOrders
      .filter((order) => {
        if (order.status !== "cancelled") return false;
        const cancelledAtMs = new Date(
          order.cancelled_at || order.updated_at || order.created_at,
        ).getTime();
        return (
          Number.isFinite(cancelledAtMs) &&
          Date.now() - cancelledAtMs <= RECENT_CANCEL_WINDOW_MS
        );
      })
      .sort(
        (a, b) =>
          new Date(b.cancelled_at || b.updated_at || b.created_at).getTime() -
          new Date(a.cancelled_at || a.updated_at || a.created_at).getTime(),
      )
      .slice(0, MAX_CANCELLED_CARDS);

    return { pendingOrders: pending, cancelledOrders: cancelled };
  }, [orders]);

  if (pendingOrders.length === 0 && cancelledOrders.length === 0) {
    return null;
  }

  const formatTimestamp = (dateString?: string) => {
    if (!dateString) return "";
    return formatBusinessDateTime(
      dateString,
      locale,
      businessTimezone,
      DATE_TIME_SHORT,
    );
  };

  const renderOrderItems = (order: Order) =>
    foodOrderItems(order.items)
      .slice(0, 3)
      .map((item, index) => (
        <div key={index} className={pendingOrderItemClass}>
          <span className="font-medium">{item.quantity}×</span>{" "}
          {item.menu_item_name}
        </div>
      ));

  const renderAdditionalItemCount = (order: Order) => {
    const items = foodOrderItems(order.items);
    if (items.length <= 3) return null;

    return (
      <div className={pendingOrderMetaClass}>
        {tString("additionalItems").replace(
          "{count}",
          String(items.length - 3),
        )}
      </div>
    );
  };

  const uniquePendingOrders = pendingOrders;
  const pendingOverflow =
    uniquePendingOrders.length > MAX_PENDING_CARDS_COLLAPSED;
  const visiblePendingOrders =
    showAllPending || !pendingOverflow
      ? uniquePendingOrders
      : uniquePendingOrders.slice(0, MAX_PENDING_CARDS_COLLAPSED);
  const hiddenPendingCount =
    uniquePendingOrders.length - visiblePendingOrders.length;

  const renderOrderHeading = (order: Order & { billId: string }) => {
    const heading = tString("orderNumber").replace(
      "{orderNumber}",
      order.order_number,
    );
    return (
      <div className="min-w-0 flex-1">
        <h4
          className="truncate font-semibold text-ink-950"
          title={heading}
          data-testid="pending-order-number"
        >
          {heading}
        </h4>
        <p className={pendingOrderMetaClass}>
          {tString("billNumber")} #{order.billId}
        </p>
      </div>
    );
  };

  return (
    <div
      className={compact ? "mb-4 space-y-3" : "mb-6 space-y-4"}
      data-testid="pending-orders-section"
      data-compact={compact ? "true" : "false"}
    >
      {uniquePendingOrders.length > 0 && (
        <div
          className={
            compact
              ? "rounded-2xl border border-amber-200 bg-amber-50/90 p-3"
              : "rounded-2xl border border-amber-200 bg-amber-50/90 p-5 sm:p-6"
          }
          data-testid="pending-orders-panel"
        >
          <div
            className={
              compact
                ? "mb-3 flex items-center gap-2"
                : "mb-6 flex items-center gap-3"
            }
          >
            <div
              className={
                compact
                  ? "flex h-8 w-8 items-center justify-center rounded-xl border border-amber-200 bg-white/80"
                  : "flex h-10 w-10 items-center justify-center rounded-2xl border border-amber-200 bg-white/80"
              }
            >
              <Clock
                className={
                  compact ? "h-4 w-4 text-amber-700" : "h-5 w-5 text-amber-700"
                }
              />
            </div>
            <div>
              <h3
                className={
                  compact
                    ? "text-sm font-semibold text-ink-950"
                    : "text-lg font-semibold text-ink-950"
                }
              >
                {tString("pendingOrdersTitle")}
              </h3>
              <p
                className={
                  compact
                    ? "text-xs leading-4 text-amber-800"
                    : "text-sm leading-5 text-amber-800"
                }
              >
                {uniquePendingOrders.length === 1
                  ? tString("pendingOrdersCountOne")
                  : tString("pendingOrdersCount").replace(
                      "{count}",
                      String(uniquePendingOrders.length),
                    )}
              </p>
            </div>
          </div>

          <div
            className={`${orderCardGridClass}${
              showAllPending && pendingOverflow
                ? " max-h-[36rem] overflow-y-auto pr-1"
                : ""
            }`}
            data-testid="pending-orders-grid"
          >
            {visiblePendingOrders.map((order) => (
              <div
                key={order.id}
                className={
                  compact
                    ? "min-w-0 w-full rounded-xl bg-white px-3 py-2.5"
                    : pendingOrderCardClass
                }
              >
                <div
                  className={
                    compact
                      ? "mb-2 flex items-start justify-between gap-2"
                      : "mb-3 flex items-start justify-between gap-2"
                  }
                >
                  {renderOrderHeading(order)}
                  <div className="shrink-0 rounded-full border border-amber-200 bg-amber-50 px-2.5 py-1 text-xs font-semibold text-amber-800">
                    {tString("pending")}
                  </div>
                </div>
                <OperationalAlertClaimStatus
                  resourceType="order"
                  resourceId={order.id}
                  className="mb-3"
                />

                <div className={compact ? "mb-2 space-y-1" : "mb-4 space-y-2"}>
                  {renderOrderItems(order)}
                  {renderAdditionalItemCount(order)}
                </div>

                {order.notes && (
                  <div
                    className={
                      compact
                        ? "mb-2 rounded-lg border border-brand/20 bg-brand/10 px-2 py-1.5"
                        : "mb-4 rounded-lg border border-brand/20 bg-brand/10 p-3"
                    }
                  >
                    {compact ? (
                      <p className="truncate text-xs text-brand">
                        <span className="font-medium text-brand-dark">
                          {tString("notesLabel")}:
                        </span>{" "}
                        {order.notes}
                      </p>
                    ) : (
                      <>
                        <p className="mb-1 text-xs font-medium text-brand-dark">
                          {tString("notesLabel")}
                        </p>
                        <p className="text-sm text-brand">{order.notes}</p>
                      </>
                    )}
                  </div>
                )}

                {order.delivery && (
                  <div className="mt-2 mb-4 space-y-1 rounded-2xl border border-brand/20 bg-brand/5 px-3 py-2">
                    <div className="flex items-center gap-2">
                      <Truck className="w-4 h-4 text-brand" />
                      <span className="text-xs font-semibold text-brand-dark uppercase tracking-wide">
                        {tString("pendingOrders.deliveryBadge")}
                      </span>
                      <span className="font-mono text-xs text-ink-500">
                        {order.delivery.delivery_number}
                      </span>
                    </div>
                    <p className="text-sm font-medium text-ink-800">
                      {order.delivery.customer_name} ·{" "}
                      {order.delivery.customer_phone}
                    </p>
                    <p className="text-sm text-ink-600">
                      {order.delivery.street}, {order.delivery.city}
                    </p>
                  </div>
                )}

                {(() => {
                  const deliveryStatus = order.delivery?.delivery_status;
                  const expiresAt = order.delivery?.payment_expires_at;
                  const expiresMs = expiresAt
                    ? new Date(expiresAt).getTime()
                    : NaN;
                  const awaitingPayment =
                    deliveryStatus === "confirmed" &&
                    !!expiresAt &&
                    Number.isFinite(expiresMs) &&
                    expiresMs > Date.now();
                  // confirmed + past expiry: Accept is invalid (backend only
                  // accepts delivery.status === pending). Keep Cancel so the
                  // card is never a dead end while the sweeper catches up.
                  const paymentExpired =
                    deliveryStatus === "confirmed" &&
                    !!expiresAt &&
                    Number.isFinite(expiresMs) &&
                    expiresMs <= Date.now();
                  const showApprove = !awaitingPayment && !paymentExpired;

                  return (
                    <div className="mt-2 space-y-2">
                      {(awaitingPayment || paymentExpired) && (
                        <div className="flex items-center justify-between gap-2">
                          <span className="text-sm text-amber-700">
                            {awaitingPayment
                              ? tString("pendingOrders.awaitingPayment")
                              : tString("pendingOrders.expired")}
                          </span>
                          {awaitingPayment && expiresAt && (
                            <Countdown
                              until={expiresAt}
                              className="text-sm font-mono text-amber-700"
                              expiredLabel={tString("pendingOrders.expired")}
                            />
                          )}
                        </div>
                      )}
                      <div className="flex gap-2">
                        {showApprove && (
                          <Button
                            size="sm"
                            color="success"
                            startContent={<CheckCircle className="w-4 h-4" />}
                            onPress={() => onApproveOrder(order.id)}
                            isLoading={actionLoading === order.id}
                            className="flex-1"
                          >
                            {tString("approve")}
                          </Button>
                        )}
                        <Button
                          size="sm"
                          color="danger"
                          variant="light"
                          className={`!text-rose-700 dark:!text-rose-300 ${showApprove ? "" : "flex-1"}`}
                          startContent={<XCircle className="w-4 h-4" />}
                          onPress={() => onRejectOrder(order.id)}
                          isLoading={actionLoading === order.id}
                        >
                          {tString("cancelOrder")}
                        </Button>
                      </div>
                    </div>
                  );
                })()}
              </div>
            ))}
          </div>

          {pendingOverflow && (
            <div className="mt-4 flex justify-center">
              <Button
                size="sm"
                variant="bordered"
                className="border-amber-300 bg-white/80 font-medium text-amber-900"
                onPress={() => setShowAllPending((v) => !v)}
                data-testid="pending-orders-toggle"
              >
                {showAllPending
                  ? tString("pendingOrdersShowFewer")
                  : tString("pendingOrdersShowAll").replace(
                      "{count}",
                      String(hiddenPendingCount),
                    )}
              </Button>
            </div>
          )}
        </div>
      )}

      {cancelledOrders.length > 0 && (
        <div className="rounded-2xl border border-rose-200 bg-rose-50/90 p-5 sm:p-6">
          <div className="flex items-center gap-3 mb-6">
            <div className="flex h-10 w-10 items-center justify-center rounded-2xl border border-rose-200 bg-white/80">
              <AlertCircle className="h-5 w-5 text-rose-700" />
            </div>
            <div>
              <h3 className="text-lg font-semibold text-ink-950">
                {tString("cancelledOrdersTitle")}
              </h3>
              <p className="text-sm leading-5 text-rose-700">
                {tString("cancelledOrdersSubtitle")}
              </p>
            </div>
          </div>

          <div className={orderCardGridClass}>
            {cancelledOrders.map((order) => (
              <div key={order.id} className={cancelledOrderCardClass}>
                <div className="mb-3 flex items-start justify-between gap-2">
                  {renderOrderHeading(order)}
                  <div className="shrink-0 rounded-full border border-rose-200 bg-rose-50 px-2.5 py-1 text-xs font-semibold text-rose-700">
                    {tString("cancelled")}
                  </div>
                </div>

                <div className="space-y-2 mb-4 opacity-70">
                  {renderOrderItems(order)}
                  {renderAdditionalItemCount(order)}
                </div>

                {order.notes && (
                  <div className="mb-3 rounded-2xl border border-rose-200 bg-rose-50 p-3">
                    <p className="mb-1 text-xs font-semibold text-rose-700">
                      {tString("notesLabel")}
                    </p>
                    <p className="text-sm text-rose-700">{order.notes}</p>
                  </div>
                )}

                {order.cancel_reason && (
                  <div className="mb-3 rounded-2xl border border-rose-200 bg-white p-3">
                    <p className="mb-1 text-xs font-semibold text-rose-700">
                      {tString("cancelReasonLabel")}
                    </p>
                    <p className="text-sm text-ink-700">
                      {order.cancel_reason}
                    </p>
                  </div>
                )}

                <p className="text-xs text-rose-700">
                  {tString("cancelledOrderMeta").replace(
                    "{time}",
                    formatTimestamp(
                      order.cancelled_at ||
                        order.updated_at ||
                        order.created_at,
                    ),
                  )}
                </p>
                {order.cancelled_by && (
                  <p className="mt-1 text-xs text-ink-600">
                    {tString("cancelledByLabel").replace(
                      "{actor}",
                      order.cancelled_by,
                    )}
                  </p>
                )}
              </div>
            ))}
          </div>

          <div className="mt-4 rounded-2xl border border-rose-200 bg-white/80 p-3">
            <p className="flex items-center gap-2 text-sm text-rose-800">
              <AlertCircle className="w-4 h-4" />
              {tString("cancelledOrdersNote")}
            </p>
          </div>
        </div>
      )}
    </div>
  );
};
