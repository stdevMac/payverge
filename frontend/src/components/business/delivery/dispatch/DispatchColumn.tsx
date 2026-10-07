import React from "react";
import DispatchCard, { DispatchOrder } from "./DispatchCard";
import type { DeliveryDriver } from "@/api/delivery";

interface DispatchColumnProps {
  title: string;
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
  tString?: (key: string) => string;
  currency?: string;
  /** Operator locale (BCP-47) for wall-clock time display; threaded to each card. */
  locale?: string;
  /** Business IANA timezone; threaded to each card so ETA/cutoff render in business time. */
  businessTimezone?: string | null;
}

export default function DispatchColumn({
  title,
  orders,
  onAdvance,
  onCancel,
  onOpenDetail,
  onAssignDriver,
  onClaim,
  onRelease,
  myStaffId = null,
  canSteal = false,
  drivers,
  tString,
  currency,
  locale,
  businessTimezone = null,
}: DispatchColumnProps) {
  return (
    <div className="flex-1 min-w-0 bg-warm-50 rounded-xl p-3 flex flex-col h-full">
      <header className="mb-3 flex items-center justify-between gap-2">
        <h3
          className="min-w-0 truncate text-sm font-semibold text-ink-900 sm:text-base"
          title={title}
        >
          {title}
        </h3>
        <span className="shrink-0 text-sm tabular-nums text-ink-500">
          ({orders.length})
        </span>
      </header>
      <div className="flex-1 space-y-2 overflow-y-auto">
        {orders.map((o) => (
          <DispatchCard
            key={o.id}
            order={o}
            onAdvance={onAdvance}
            onCancel={onCancel}
            onOpenDetail={onOpenDetail}
            onAssignDriver={onAssignDriver}
            onClaim={onClaim}
            onRelease={onRelease}
            myStaffId={myStaffId}
            canSteal={canSteal}
            drivers={drivers}
            tString={tString}
            currency={currency}
            locale={locale}
            businessTimezone={businessTimezone}
          />
        ))}
      </div>
    </div>
  );
}
