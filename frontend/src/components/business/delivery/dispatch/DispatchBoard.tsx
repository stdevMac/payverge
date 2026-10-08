import React from "react";
import DispatchColumn from "./DispatchColumn";
import { DispatchOrder } from "./DispatchCard";
import type { DeliveryDriver } from "@/api/delivery";
import {
  useSimpleLocale,
  getTranslation,
} from "@/i18n/SimpleTranslationProvider";

// Logistics board includes an intake lane so unpaid/pending deliveries are not
// invisible while Bills still shows them. Driver-leg statuses collapse into a
// single "out for delivery" column; terminal exception columns appear when the
// board filter is set to cancelled/failed.
const COLUMN_KEYS = [
  "awaiting_payment",
  "preparing",
  "ready",
  "out_for_delivery",
  "delivered",
] as const;

type BoardColumnKey = (typeof COLUMN_KEYS)[number];

const OUT_FOR_DELIVERY_STATUSES = new Set([
  "assigned",
  "picked_up",
  "in_transit",
  "nearby",
]);

function columnFor(status: string): string {
  if (status === "pending" || status === "confirmed") return "awaiting_payment";
  if (OUT_FOR_DELIVERY_STATUSES.has(status)) return "out_for_delivery";
  return status;
}

export type BoardFilter =
  | null
  | BoardColumnKey
  | "cancelled"
  | "failed";

interface DispatchBoardProps {
  orders: DispatchOrder[];
  onAdvance: (o: DispatchOrder) => void | Promise<void>;
  onCancel: (o: DispatchOrder) => void;
  onOpenDetail?: (o: DispatchOrder) => void;
  onAssignDriver?: (orderId: number, driverId: number) => Promise<void>;
  onClaim?: (o: DispatchOrder, opts?: { steal?: boolean }) => void | Promise<void>;
  onRelease?: (o: DispatchOrder) => void | Promise<void>;
  myStaffId?: number | null;
  canSteal?: boolean;
  drivers?: DeliveryDriver[];
  currency?: string;
  /** Business IANA timezone; null/invalid falls back to UTC (never device TZ). */
  businessTimezone?: string | null;
  /** When set, render a single filtered column instead of the full board. */
  boardFilter?: BoardFilter;
}

export default function DispatchBoard({
  orders,
  onAdvance,
  onCancel,
  onOpenDetail,
  onAssignDriver,
  onClaim,
  onRelease,
  myStaffId = null,
  canSteal = false,
  drivers = [],
  currency,
  businessTimezone = null,
  boardFilter = null,
}: DispatchBoardProps) {
  const { locale } = useSimpleLocale();
  const t = (status: string): string => {
    const result = getTranslation(
      `deliverySettings.dispatch.columns.${status}`,
      locale,
    );
    return typeof result === "string" && result.length > 0 ? result : status;
  };
  // The card needs its own slice of the dispatch namespace (advance labels,
  // Live/ETA/Cancel) — re-resolve relative to deliverySettings.dispatch.
  const cardT = (key: string): string => {
    const result = getTranslation(
      `deliverySettings.dispatch.${key}`,
      locale,
    );
    return typeof result === "string" ? result : key;
  };

  const columnProps = {
    onAdvance,
    onCancel,
    onOpenDetail,
    onAssignDriver,
    onClaim,
    onRelease,
    myStaffId,
    canSteal,
    drivers,
    tString: cardT,
    currency,
    locale,
    businessTimezone,
  };

  // Single-column filter: logistics lane or terminal exception.
  if (boardFilter) {
    const filtered =
      boardFilter === "cancelled" || boardFilter === "failed"
        ? orders.filter((o) => o.status === boardFilter)
        : orders.filter((o) => columnFor(o.status) === boardFilter);
    return (
      <div className="flex gap-3 overflow-x-auto snap-x snap-mandatory pb-1">
        <div className="snap-start min-w-[280px] w-full max-w-md">
          <DispatchColumn
            title={t(boardFilter)}
            orders={filtered}
            {...columnProps}
          />
        </div>
      </div>
    );
  }

  // Same #211 pattern as the schedule week grid: min-w-0 + w-full keeps this
  // descendant from growing to 5×200px and getting clipped by the dashboard
  // overflow-hidden shell. A thin visible scrollbar (sidebar M34) is the
  // affordance at ~1024px where Out for delivery / Delivered sit off-screen.
  return (
    <div className="relative min-w-0 w-full max-w-full">
      <div
        className="pointer-events-none absolute inset-y-0 left-0 z-10 w-6 bg-gradient-to-r from-white to-transparent"
        aria-hidden
      />
      <div
        className="pointer-events-none absolute inset-y-0 right-0 z-10 w-8 bg-gradient-to-l from-white to-transparent"
        aria-hidden
      />
      <p className="mb-1 text-xs text-ink-500">{cardT("scrollHint")}</p>
      <div
        data-testid="dispatch-board-scroll"
        className="flex min-w-0 w-full max-w-full gap-3 overflow-x-auto snap-x snap-mandatory pb-2 scrollbar-thin scrollbar-thumb-warm-400/90 scrollbar-track-warm-100/80 [scrollbar-gutter:stable]"
      >
        {COLUMN_KEYS.map((key) => (
          <div
            key={key}
            className="snap-start min-w-[200px] w-[min(260px,75vw)] shrink-0"
            data-testid={`dispatch-column-${key}`}
          >
            <DispatchColumn
              title={t(key)}
              orders={orders.filter((o) => columnFor(o.status) === key)}
              {...columnProps}
            />
          </div>
        ))}
      </div>
    </div>
  );
}
