import React, { useState } from "react";
import { Chip, Button } from "@nextui-org/react";
import { AlertTriangle, Clock } from "lucide-react";
import type { DeliveryOrder, DeliveryDriver, DeliveryStatus } from "@/api/delivery";
import { DriverAssignment } from "./DriverAssignment";
import { formatCurrency } from "@/api/currency";
import { btnPrimaryNextUI } from "@/components/ui/buttonStyles";
import OperationalAlertClaimStatus from "../../operational-alerts/OperationalAlertClaimStatus";
import { formatBusinessTime } from "@/utils/businessTime";
import { useSimpleLocale } from "@/i18n/SimpleTranslationProvider";

const DELIVERY_STATUS_SEQUENCE = [
  "pending",
  "confirmed",
  "preparing",
  "ready",
  "assigned",
  "picked_up",
  "in_transit",
  "nearby",
  "delivered",
] as const;

function getNextStatus(
  current: string
): DeliveryStatus | null {
  const idx = DELIVERY_STATUS_SEQUENCE.indexOf(
    current as (typeof DELIVERY_STATUS_SEQUENCE)[number]
  );
  if (idx < 0 || idx >= DELIVERY_STATUS_SEQUENCE.length - 1) return null;
  return DELIVERY_STATUS_SEQUENCE[idx + 1];
}

/**
 * The operator-facing advance target, which differs from the raw sequence for
 * two states the backend forbids as manual destinations:
 *
 *  - `confirmed`: a prepay delivery sits at confirmed with payment still owed.
 *    ONLY the system payment path (HandleDeliveryBillPaid) may advance it; a
 *    dispatch:write operator advancing confirmed→preparing is rejected 400 by
 *    the backend. So we never offer an advance out of confirmed.
 *  - `pending`: intake lives in the Bills queue (Accept / Reject). For online
 *    prepay the backend rejects any fulfillment advance until Accept + pay;
 *    even for COD, Accept is the front door. Never offer a dispatch advance
 *    out of pending.
 *
 * Returns null when no operator advance is available (pending, confirmed, or terminal).
 */
export function getOperatorNextStatus(current: string): DeliveryStatus | null {
  if (current === "confirmed" || current === "pending") return null;
  return getNextStatus(current);
}

const STATUS_COLOR_MAP: Record<
  string,
  "default" | "primary" | "secondary" | "success" | "warning" | "danger"
> = {
  pending: "warning",
  confirmed: "primary",
  preparing: "primary",
  ready: "secondary",
  assigned: "secondary",
  picked_up: "secondary",
  in_transit: "primary",
  nearby: "primary",
  delivered: "success",
  cancelled: "danger",
  failed: "danger",
};

function cutoffState(
  cutoffAt: string | undefined
): "none" | "near" | "past" {
  if (!cutoffAt) return "none";
  const diff = new Date(cutoffAt).getTime() - Date.now();
  if (diff < 0) return "past";
  if (diff < 15 * 60 * 1000) return "near";
  return "none";
}

/** Title-case fallback for a status with no translation (e.g. "picked_up" -> "Picked up"). */
function humanizeStatus(status: string): string {
  if (!status) return "";
  const words = status.split("_");
  return words
    .map((word, i) =>
      i === 0 ? word.charAt(0).toUpperCase() + word.slice(1) : word,
    )
    .join(" ");
}

interface OrderRowProps {
  order: DeliveryOrder;
  drivers: DeliveryDriver[];
  onAdvance: (orderId: number, nextStatus: DeliveryStatus) => Promise<void>;
  onAssignDriver: (orderId: number, driverId: number) => Promise<void>;
  onCancel: (order: DeliveryOrder) => void;
  /** Open the order detail drawer (click on identity block). */
  onOpenDetail?: (order: DeliveryOrder) => void;
  tString: (key: string, vars?: Record<string, string>) => string;
  /** Business default currency (ISO 4217); falls back to USD. */
  currency?: string;
  /** Operator locale (BCP-47); falls back to the ambient SimpleLocale. */
  locale?: string;
  /** Business IANA timezone; null/invalid falls back to UTC (never device TZ). */
  businessTimezone?: string | null;
}

export function OrderRow({
  order,
  drivers,
  onAdvance,
  onAssignDriver,
  onCancel,
  onOpenDetail,
  tString,
  currency = "USD",
  locale,
  businessTimezone = null,
}: OrderRowProps) {
  // Fall back to the ambient operator locale when the parent omits it, so
  // times render in the operator's language even without an explicit prop.
  const { locale: ambientLocale } = useSimpleLocale();
  const effectiveLocale = locale ?? ambientLocale ?? "en";
  const formatTime = (iso: string | undefined): string =>
    iso
      ? formatBusinessTime(iso, effectiveLocale, businessTimezone, {
          hour: "2-digit",
          minute: "2-digit",
        })
      : "";
  // Operator-facing advance target: never offers pending/confirmed (Bills intake
  // + payment path only); intake still appears on the board's awaiting lane.
  const nextStatus = getOperatorNextStatus(order.status);
  // Per-order in-flight guard so a double-click (or a slow request) can't fire
  // the advance twice — the backend now no-ops a same-status write, but the
  // button must also stop re-submitting mid-flight.
  const [advancing, setAdvancing] = useState(false);
  const handleAdvance = async (next: DeliveryStatus) => {
    if (advancing) return;
    setAdvancing(true);
    try {
      await onAdvance(order.id, next);
    } finally {
      setAdvancing(false);
    }
  };
  // Humanized, translated status labels via the dispatch.filters.* map (which
  // already covers every status in both en + es) — operators must never see raw
  // snake_case like "picked_up" or "Advance to in_transit" (audit L6 #10).
  const statusLabel = (status: string): string => {
    const translated = tString(`dispatch.filters.${status}`);
    return translated && translated !== `dispatch.filters.${status}`
      ? translated
      : humanizeStatus(status);
  };
  const isTerminal =
    order.status === "delivered" ||
    order.status === "cancelled" ||
    order.status === "failed";
  const showAssign =
    ["ready", "assigned", "picked_up", "in_transit", "nearby"].includes(
      order.status
    ) && !order.driver_id;
  const cutoff = cutoffState(order.cutoff_at);

  const street = order.delivery_address?.street ?? "";
  const city = order.delivery_address?.city ?? "";
  const country = order.delivery_address?.country ?? "";
  const location = [street, city, country].filter(Boolean).join(", ");

  return (
    <div
      className={`rounded-xl border bg-white p-4 flex flex-col sm:flex-row gap-3 sm:items-start ${
        cutoff === "past"
          ? "border-red-300 bg-red-50"
          : cutoff === "near"
          ? "border-amber-300 bg-amber-50"
          : "border-warm-200"
      }`}
    >
      {/* Left: identity — clickable to open detail drawer */}
      {/* eslint-disable-next-line jsx-a11y/no-static-element-interactions -- role/tabIndex/keyboard are attached together with onClick when onOpenDetail exists */}
      <div
        className={`flex-1 min-w-0 ${onOpenDetail ? "cursor-pointer rounded-lg focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-brand" : ""}`}
        role={onOpenDetail ? "button" : undefined}
        tabIndex={onOpenDetail ? 0 : undefined}
        data-testid="order-row-open-detail"
        onClick={onOpenDetail ? () => onOpenDetail(order) : undefined}
        onKeyDown={
          onOpenDetail
            ? (e) => {
                if (e.key === "Enter" || e.key === " ") {
                  e.preventDefault();
                  onOpenDetail(order);
                }
              }
            : undefined
        }
      >
        <div className="flex items-center gap-2 flex-wrap mb-1">
          <span className="text-sm font-semibold text-ink-900">
            #{order.delivery_number}
          </span>
          <Chip
            size="sm"
            color={STATUS_COLOR_MAP[order.status] ?? "default"}
            variant="flat"
          >
            {statusLabel(order.status)}
          </Chip>
          {order.zone?.name && (
            <Chip size="sm" variant="flat" color="default">
              {order.zone.name}
            </Chip>
          )}
          <OperationalAlertClaimStatus
            resourceType="delivery"
            resourceId={order.id}
          />
        </div>
        <p className="text-sm text-ink-700 font-medium truncate">
          {order.customer_name}
          {order.customer_phone ? ` · ${order.customer_phone}` : ""}
        </p>
        {location && (
          <p className="text-xs text-ink-700 truncate">{location}</p>
        )}
        {order.driver && (
          <p className="text-xs text-ink-700 mt-0.5">
            {order.driver.name}
          </p>
        )}
      </div>

      {/* Middle: timing + fee */}
      <div className="flex flex-col gap-1 text-xs text-ink-700 min-w-[110px]">
        <span className="flex items-center gap-1">
          <Clock size={12} />
          {order.estimated_delivery_time
            ? formatTime(order.estimated_delivery_time)
            : tString("dispatch.order.noEta")}
        </span>
        {order.cutoff_at && (
          <span
            className={`flex items-center gap-1 ${
              cutoff === "past"
                ? "text-red-600 font-semibold"
                : cutoff === "near"
                ? "text-amber-600 font-medium"
                : ""
            }`}
          >
            {cutoff !== "none" && <AlertTriangle size={11} />}
            {tString("dispatch.order.cutoff")}: {formatTime(order.cutoff_at)}
          </span>
        )}
        <span>
          {tString("dispatch.order.fee")}: {formatCurrency(Number(order.delivery_fee ?? 0), currency)}
        </span>
      </div>

      {/* Right: actions */}
      <div className="flex flex-col gap-2 items-end min-w-[180px]">
        {nextStatus && !isTerminal && (
          <Button
            size="sm"
            radius="full"
            className={btnPrimaryNextUI}
            isLoading={advancing}
            isDisabled={advancing}
            aria-label={`${tString("dispatch.actions.advance", { next: statusLabel(nextStatus) })} order ${order.delivery_number}`}
            onPress={() => handleAdvance(nextStatus)}
          >
            {tString("dispatch.actions.advance", { next: statusLabel(nextStatus) })}
          </Button>
        )}
        {showAssign && (
          <DriverAssignment
            drivers={drivers}
            onAssign={(driverId) => onAssignDriver(order.id, driverId)}
            tString={tString}
          />
        )}
        {!isTerminal && (
          <Button
            size="sm"
            color="danger"
            variant="light"
            aria-label={`${tString("dispatch.actions.cancel")} order ${order.delivery_number}`}
            onPress={() => onCancel(order)}
          >
            {tString("dispatch.actions.cancel")}
          </Button>
        )}
      </div>
    </div>
  );
}
