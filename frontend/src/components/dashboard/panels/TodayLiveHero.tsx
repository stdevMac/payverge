"use client";

import React from "react";
import { Receipt, UtensilsCrossed } from "lucide-react";
import { Metric } from "@/components/ui/Metric";

interface TodayLiveHeroProps {
  /** Numeric today collected revenue (dollars). Tone is derived — zero is never brand-green. */
  revenue: number;
  formatRevenue: (n: number) => string;
  bills: number;
  openTables: number;
  remaining?: number;
  labels: {
    todayRevenue: string;
    billsClosed: string;
    openTables: string;
    livePulse: string;
    remainingOnFloor?: string;
    /** Shown when revenue is empty (no sales yet). */
    noSalesYet?: string;
  };
}

export default function TodayLiveHero({
  revenue,
  formatRevenue,
  bills,
  openTables,
  remaining = 0,
  labels,
}: TodayLiveHeroProps) {
  // Zero revenue with no closed bills is "nothing", not a proud $0.00 in brand teal.
  const revenueEmpty = revenue <= 0 && bills <= 0;

  return (
    <div className="overflow-hidden rounded-2xl border border-warm-200 bg-white">
      <div className="grid gap-0 lg:grid-cols-[1.4fr_1fr]">
        <div className="bg-brand/[0.04] px-6 py-6 lg:px-8 lg:py-8">
          {revenueEmpty ? (
            <Metric
              label={labels.todayRevenue}
              value={0}
              state="empty"
              reason={labels.noSalesYet ?? "—"}
              format={formatRevenue}
              size="lg"
              className="[&_[data-testid]]:text-display-xl"
              data-testid="today-live-revenue"
            />
          ) : (
            <Metric
              label={labels.todayRevenue}
              value={revenue}
              state="ok"
              format={formatRevenue}
              size="lg"
              className="[&_[data-testid]]:text-display-xl [&_[data-state=ok]]:text-brand"
              data-testid="today-live-revenue"
            />
          )}
          <p className="mt-2 text-body-sm text-ink-600">{labels.livePulse}</p>
          {remaining > 0 ? (
            <p
              className="mt-2 text-body-sm text-ink-500"
              data-testid="today-live-remaining"
            >
              {labels.remainingOnFloor ?? "Remaining on open checks"}:{" "}
              {formatRevenue(remaining)}
            </p>
          ) : null}
        </div>
        <div className="divide-y divide-warm-100 border-t border-warm-100 lg:border-t-0 lg:border-l">
          <div className="flex items-center gap-4 px-6 py-5">
            <span className="flex h-10 w-10 shrink-0 items-center justify-center rounded-xl bg-warm-100 text-ink-700">
              <Receipt className="h-5 w-5" aria-hidden />
            </span>
            <div>
              <Metric
                label={labels.billsClosed}
                value={bills}
                state="ok"
                format="count"
                size="md"
                data-testid="today-live-bills"
              />
            </div>
          </div>
          <div className="flex items-center gap-4 px-6 py-5">
            <span
              className={`flex h-10 w-10 shrink-0 items-center justify-center rounded-xl ${
                openTables > 0
                  ? "bg-emerald-50 text-emerald-700"
                  : "bg-warm-100 text-ink-700"
              }`}
            >
              <UtensilsCrossed className="h-5 w-5" aria-hidden />
            </span>
            <div>
              <Metric
                label={labels.openTables}
                value={openTables}
                state="ok"
                format="count"
                size="md"
                data-testid="today-live-open-tables"
              />
            </div>
          </div>
        </div>
      </div>
    </div>
  );
}
