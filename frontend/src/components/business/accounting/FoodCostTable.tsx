"use client";

import React from "react";
import type { FoodCostItem } from "@/api/accounting";
import type { Dollars } from "@/types/money";

export interface FoodCostTableProps {
  items: FoodCostItem[];
  currency: string;
  formatMoney: (value: Dollars, currency: string) => string;
  labels: {
    title: string;
    item: string;
    unitCost: string;
    price: string;
    foodCostPct: string;
    margin: string;
    qty: string;
  };
}

// pctClasses returns the text color for a food-cost ratio (0..1):
// <30% green, 30-35% amber, >35% red. Exported for boundary testing.
export function pctClasses(pct: number): string {
  if (pct <= 0) return "text-ink-400";
  if (pct < 0.3) return "text-emerald-600";
  if (pct <= 0.35) return "text-amber-600";
  return "text-rose-600";
}

export default function FoodCostTable({
  items,
  currency,
  formatMoney,
  labels,
}: FoodCostTableProps) {
  // Only dishes with a complete cost and actual sales are meaningful here.
  const rows = items.filter((it) => it.has_complete_cost && it.qty_sold > 0);
  if (rows.length === 0) return null;

  return (
    <div className="mt-4 overflow-hidden rounded-3xl border border-warm-200 bg-white shadow-sm shadow-warm-900/5">
      <p className="px-4 py-3 text-sm font-semibold text-ink-950">
        {labels.title}
      </p>
      <div className="overflow-x-auto">
        <table className="w-full text-sm">
          <thead>
            <tr className="border-t border-warm-100 bg-warm-50/70 text-left text-[11px] uppercase tracking-wide text-ink-500">
              <th className="px-4 py-2">{labels.item}</th>
              <th className="px-4 py-2 text-right">{labels.unitCost}</th>
              <th className="px-4 py-2 text-right">{labels.price}</th>
              <th className="px-4 py-2 text-right">{labels.foodCostPct}</th>
              <th className="px-4 py-2 text-right">{labels.margin}</th>
              <th className="px-4 py-2 text-right">{labels.qty}</th>
            </tr>
          </thead>
          <tbody>
            {rows.map((it) => {
              // Round once so the displayed % and its color agree at the
              // tone boundaries (mirrors the Food-Cost card).
              const rowPct = Math.round(it.food_cost_pct * 100);
              return (
                <tr key={it.menu_item_id} className="border-t border-warm-100">
                  <td className="px-4 py-2 font-medium text-ink-900">
                    {it.menu_item_name}
                  </td>
                  <td className="px-4 py-2 text-right text-ink-700">
                    {formatMoney(it.unit_cost, currency)}
                  </td>
                  <td className="px-4 py-2 text-right text-ink-700">
                    {formatMoney(it.avg_price, currency)}
                  </td>
                  <td
                    className={`px-4 py-2 text-right font-medium ${pctClasses(rowPct / 100)}`}
                  >
                    {rowPct}%
                  </td>
                  <td className="px-4 py-2 text-right text-ink-700">
                    {formatMoney(it.margin_per_unit, currency)}
                  </td>
                  <td className="px-4 py-2 text-right text-ink-700">
                    {it.qty_sold}
                  </td>
                </tr>
              );
            })}
          </tbody>
        </table>
      </div>
    </div>
  );
}
