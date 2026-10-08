"use client";

import React, { useMemo } from "react";
import type { CategoryTotal } from "@/api/accounting";
import { PremiumPanel } from "../premium";

export type CategoryBreakdownProps = {
  title: string;
  items: CategoryTotal[];
  currency: string;
  formatMoney: (value: number, currency: string) => string;
  formatCategory: (category: string) => string;
  emptyLabel: string;
  viewAllLabel?: string;
  onViewAll?: () => void;
};

function barWidthPct(total: number, max: number): number {
  if (max <= 0 || total <= 0) return 0;
  return Math.max(2, Math.round((total / max) * 100));
}

export default function CategoryBreakdown({
  title,
  items,
  currency,
  formatMoney,
  formatCategory,
  emptyLabel,
  viewAllLabel,
  onViewAll,
}: CategoryBreakdownProps) {
  const sorted = useMemo(
    () =>
      [...(items ?? [])].sort(
        (a, b) => Number(b.total || 0) - Number(a.total || 0),
      ),
    [items],
  );
  const max = sorted[0] ? Number(sorted[0].total || 0) : 0;

  return (
    <PremiumPanel className="p-5" withTexture={false}>
      <div className="mb-4 flex items-center justify-between gap-3">
        <h3 className="text-base font-semibold text-ink-950 sm:text-lg">
          {title}
        </h3>
        {onViewAll && viewAllLabel ? (
          <button
            type="button"
            onClick={onViewAll}
            className="text-sm font-medium text-brand transition-colors hover:text-brand-dark"
          >
            {viewAllLabel}
          </button>
        ) : null}
      </div>

      {sorted.length === 0 ? (
        <p className="text-sm text-ink-500">{emptyLabel}</p>
      ) : (
        <ul className="space-y-3">
          {sorted.map((item) => {
            const total = Number(item.total || 0);
            const width = barWidthPct(total, max);
            return (
              <li
                key={item.category}
                data-testid="category-breakdown-row"
                className="space-y-1.5"
              >
                <div className="flex items-baseline justify-between gap-3 text-sm">
                  <span className="min-w-0 truncate font-medium text-ink-900">
                    {formatCategory(item.category)}
                  </span>
                  <span className="shrink-0 tabular-nums text-ink-700">
                    {formatMoney(total, currency)}
                  </span>
                </div>
                <div className="h-2 overflow-hidden rounded-full bg-warm-100">
                  <div
                    data-testid="category-breakdown-bar"
                    className="h-full rounded-full bg-brand/70"
                    style={{ width: `${width}%` }}
                  />
                </div>
              </li>
            );
          })}
        </ul>
      )}
    </PremiumPanel>
  );
}
