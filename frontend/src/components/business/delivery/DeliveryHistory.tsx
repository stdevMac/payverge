"use client";

import React, { useCallback, useEffect, useState } from "react";
import { Button, Chip, Input, Skeleton } from "@nextui-org/react";
import { CalendarRange } from "lucide-react";
import {
  deliveryApi,
  type DeliveryOrder,
  type DeliveryStatus,
} from "@/api/delivery";
import { useDeliveryBusiness } from "./useDeliveryQueries";
import { useToast } from "@/contexts/ToastContext";
import {
  getTranslation,
  useSimpleLocale,
} from "@/i18n/SimpleTranslationProvider";
import { formatBusinessTime } from "@/utils/businessTime";
import OrderDetailDrawer from "./dispatch/OrderDetailDrawer";
import {
  DISPATCH_TERMINAL_STATUSES,
  DISPATCH_PAGE_SIZE,
} from "./dispatchBoardLoad";
import { localDateKey } from "@/lib/localDate";

interface DeliveryHistoryProps {
  businessId: number;
}

const HISTORY_STATUSES: Array<DeliveryStatus | "all"> = [
  "all",
  "delivered",
  "cancelled",
  "failed",
];

function toISODateStart(dateStr: string): string {
  // dateStr is YYYY-MM-DD from a text field; treat as local midnight. The wire
  // value MUST be RFC3339 (with offset) — the backend's
  // time.Parse(time.RFC3339, since) 400s on a bare local timestamp.
  const d = new Date(`${dateStr}T00:00:00`);
  if (Number.isNaN(d.getTime())) {
    // Fall back to the start of today for malformed input mid-typing.
    const today = new Date();
    today.setHours(0, 0, 0, 0);
    return today.toISOString();
  }
  return d.toISOString();
}

function daysAgoISODate(days: number): string {
  const d = new Date();
  d.setDate(d.getDate() - days);
  return localDateKey(d);
}

/**
 * History board segment: terminal deliveries with date-range + status filters
 * and offset pagination against the existing list endpoint. Rows open the same
 * OrderDetailDrawer as the live dispatch board.
 */
export default function DeliveryHistory({ businessId }: DeliveryHistoryProps) {
  const { showError, showSuccess } = useToast();
  const { locale } = useSimpleLocale();
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
    [locale],
  );

  const [fromDate, setFromDate] = useState(() => daysAgoISODate(7));
  const [toDate, setToDate] = useState(() => localDateKey());
  const [status, setStatus] = useState<DeliveryStatus | "all">("all");
  const [offset, setOffset] = useState(0);
  const [orders, setOrders] = useState<DeliveryOrder[]>([]);
  const [total, setTotal] = useState(0);
  const [hasMore, setHasMore] = useState(false);
  const [loading, setLoading] = useState(true);
  const [detailId, setDetailId] = useState<number | null>(null);
  const [drivers, setDrivers] = useState<
    Awaited<ReturnType<typeof deliveryApi.getAvailableDrivers>>
  >([]);

  // Shared React Query business read — same key as the other Delivery sub-tabs.
  const { data: business } = useDeliveryBusiness(businessId);
  const currency = business?.default_currency || "USD";
  const timezone = business?.timezone || undefined;

  const load = useCallback(async () => {
    setLoading(true);
    try {
      const since = toISODateStart(fromDate);
      // End-of-range is client-side: server only has `since`, so we fetch a
      // window starting at fromDate and filter toDate on the client after.
      // Offset pagination still applies against the server window.
      const statusFilter =
        status === "all" ? DISPATCH_TERMINAL_STATUSES : [status];
      const [result, availableDrivers] = await Promise.all([
        deliveryApi.getBusinessDeliveries(businessId, {
          status: statusFilter,
          since,
          limit: DISPATCH_PAGE_SIZE,
          offset,
        }),
        deliveryApi.getAvailableDrivers(businessId),
      ]);
      const endMs = new Date(`${toDate}T23:59:59.999Z`).getTime();
      const filtered = (result.deliveries ?? []).filter((o) => {
        const ts = new Date(o.updated_at ?? o.created_at).getTime();
        return !Number.isNaN(ts) && ts <= endMs;
      });
      setOrders(filtered);
      setTotal(result.total ?? filtered.length);
      setHasMore(Boolean(result.has_more));
      setDrivers(availableDrivers ?? []);
    } catch {
      showError(tString("dispatch.toasts.statusError"));
      setOrders([]);
    } finally {
      setLoading(false);
    }
  }, [businessId, fromDate, toDate, status, offset, showError, tString]);

  useEffect(() => {
    void load();
  }, [load]);

  // Reset offset when filters change.
  useEffect(() => {
    setOffset(0);
  }, [fromDate, toDate, status]);

  return (
    <section
      aria-labelledby="delivery-history-heading"
      className="space-y-4"
      data-testid="delivery-history"
    >
      <div className="flex items-center gap-2">
        <CalendarRange className="h-5 w-5 text-brand" />
        <h2
          id="delivery-history-heading"
          className="font-title text-heading-sm text-ink-900"
        >
          {tString("dispatch.history.title")}
        </h2>
      </div>
      <p className="text-sm text-ink-600">{tString("dispatch.history.subtitle")}</p>

      <div className="flex flex-wrap items-end gap-3">
        <Input
          type="text"
          label={tString("dispatch.history.from")}
          placeholder="YYYY-MM-DD"
          value={fromDate}
          onValueChange={setFromDate}
          className="max-w-[160px]"
          data-testid="history-from"
        />
        <Input
          type="text"
          label={tString("dispatch.history.to")}
          placeholder="YYYY-MM-DD"
          value={toDate}
          onValueChange={setToDate}
          className="max-w-[160px]"
          data-testid="history-to"
        />
        <div
          className="flex flex-wrap gap-2"
          role="tablist"
          aria-label={tString("dispatch.history.statusFilter")}
        >
          {HISTORY_STATUSES.map((key) => {
            const isActive = status === key;
            const label =
              key === "all"
                ? tString("dispatch.filters.all")
                : tString(`dispatch.filters.${key}`);
            return (
              <button
                key={key}
                role="tab"
                type="button"
                aria-selected={isActive}
                onClick={() => setStatus(key)}
                className="focus:outline-none focus-visible:ring-2 focus-visible:ring-brand rounded-full"
              >
                <Chip
                  variant={isActive ? "solid" : "flat"}
                  color={isActive ? "primary" : "default"}
                  classNames={{ base: "cursor-pointer" }}
                >
                  {label}
                </Chip>
              </button>
            );
          })}
        </div>
      </div>

      {loading ? (
        <div className="space-y-2">
          {[1, 2, 3].map((i) => (
            <Skeleton key={i} className="h-16 w-full rounded-xl" />
          ))}
        </div>
      ) : orders.length === 0 ? (
        <p
          className="rounded-xl border border-warm-200 bg-warm-50 p-6 text-center text-sm text-ink-600"
          data-testid="history-empty"
        >
          {tString("dispatch.history.empty")}
        </p>
      ) : (
        <ul className="space-y-2" data-testid="history-list">
          {orders.map((order) => (
            <li key={order.id}>
              <button
                type="button"
                data-testid={`history-row-${order.id}`}
                onClick={() => setDetailId(order.id)}
                className="flex w-full flex-wrap items-center justify-between gap-2 rounded-xl border border-warm-200 bg-white px-4 py-3 text-left transition-colors hover:border-brand/30 hover:bg-brand/5 focus:outline-none focus-visible:ring-2 focus-visible:ring-brand"
              >
                <div>
                  <p className="font-medium text-ink-900">
                    #{order.delivery_number} · {order.customer_name}
                  </p>
                  <p className="text-xs text-ink-500">
                    {formatBusinessTime(
                      order.updated_at ?? order.created_at,
                      locale,
                      timezone ?? null,
                      {
                        month: "short",
                        day: "numeric",
                        hour: "numeric",
                        minute: "2-digit",
                      },
                    )}
                  </p>
                </div>
                <Chip size="sm" variant="flat">
                  {tString(`dispatch.columns.${order.status}`) || order.status}
                </Chip>
              </button>
            </li>
          ))}
        </ul>
      )}

      <div className="flex items-center justify-between gap-2 text-sm text-ink-600">
        <span>
          {tString("dispatch.history.showing", {
            count: String(orders.length),
            total: String(total),
          })}
        </span>
        <div className="flex gap-2">
          <Button
            size="sm"
            variant="flat"
            isDisabled={offset === 0 || loading}
            onPress={() =>
              setOffset((o) => Math.max(0, o - DISPATCH_PAGE_SIZE))
            }
            data-testid="history-prev"
          >
            {tString("dispatch.history.prev")}
          </Button>
          <Button
            size="sm"
            variant="flat"
            isDisabled={!hasMore || loading}
            onPress={() => setOffset((o) => o + DISPATCH_PAGE_SIZE)}
            data-testid="history-next"
          >
            {tString("dispatch.history.next")}
          </Button>
        </div>
      </div>

      <OrderDetailDrawer
        businessId={businessId}
        orderId={detailId}
        isOpen={detailId != null}
        onClose={() => setDetailId(null)}
        drivers={drivers}
        onAdvance={async (id, next) => {
          await deliveryApi.updateDeliveryOrderStatus(businessId, id, next);
          showSuccess(tString("dispatch.toasts.statusUpdated"));
          void load();
        }}
        onAssignDriver={async (id, driverId) => {
          await deliveryApi.assignDriver(businessId, id, driverId);
          showSuccess(tString("dispatch.toasts.driverAssigned"));
          void load();
        }}
        onCancel={async (order) => {
          try {
            await deliveryApi.cancelDeliveryOrder(
              businessId,
              order.id,
              "Cancelled from history",
            );
            showSuccess(tString("dispatch.toasts.cancelled"));
            setDetailId(null);
            void load();
          } catch {
            showError(tString("dispatch.toasts.cancelError"));
          }
        }}
        tString={tString}
        currency={currency}
        locale={locale}
        businessTimezone={timezone ?? null}
      />
    </section>
  );
}
