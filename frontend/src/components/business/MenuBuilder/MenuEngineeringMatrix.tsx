"use client";

import React from "react";
import { Button, Tabs, Tab } from "@nextui-org/react";
import type { Dollars } from "@/types/money";
import type {
  MenuEngineeringReport,
  MenuEngineeringRollup,
  Quadrant,
} from "@/api/menuEngineering";
import { pctClasses } from "@/components/business/accounting/FoodCostTable";
import { reportHasNoSales } from "./menuEngineeringFallback";

type Period = "day" | "week" | "month";

const PERIOD_OPTIONS: readonly Period[] = ["day", "week", "month"];

function isPeriod(value: string): value is Period {
  return (PERIOD_OPTIONS as readonly string[]).includes(value);
}

// Canonical render order for the four quadrants (used by cards + scatter
// corner labels). Stars first as the "best" quadrant.
const QUADRANT_ORDER: readonly Quadrant[] = [
  "star",
  "plowhorse",
  "puzzle",
  "dog",
];

interface QuadrantStyle {
  // Tailwind text color for labels/accents.
  text: string;
  // SVG dot fill class for the scatter plot.
  dotFill: string;
  // Tailwind bg swatch (matches the dot fill tone) for card/legend accents.
  dot: string;
  // Card border tint.
  border: string;
  // Pill badge classes for the table quadrant column.
  badge: string;
}

// QUADRANT_STYLE is the SINGLE source of truth for quadrant color: the scatter
// dots, the action cards, and the table badges all read from here. Do not
// duplicate these color literals elsewhere.
const QUADRANT_STYLE: Record<Quadrant, QuadrantStyle> = {
  star: {
    text: "text-emerald-600",
    dotFill: "fill-emerald-500",
    dot: "bg-emerald-500",
    border: "border-emerald-200",
    badge: "bg-emerald-50 text-emerald-700",
  },
  plowhorse: {
    text: "text-amber-600",
    dotFill: "fill-amber-500",
    dot: "bg-amber-500",
    border: "border-amber-200",
    badge: "bg-amber-50 text-amber-700",
  },
  puzzle: {
    // `brand` (teal) is the only on-palette hue left once star=emerald,
    // plowhorse=amber, dog=rose are taken. brand has no numbered shades — it's
    // expressed via opacity modifiers (see ProposalCard's border-brand/25
    // bg-brand/[0.03]).
    text: "text-brand",
    dotFill: "fill-brand",
    dot: "bg-brand",
    border: "border-brand/30",
    badge: "bg-brand/10 text-brand",
  },
  dog: {
    text: "text-rose-600",
    dotFill: "fill-rose-500",
    dot: "bg-rose-500",
    border: "border-rose-200",
    badge: "bg-rose-50 text-rose-700",
  },
};

export interface MenuEngineeringMatrixProps {
  report: MenuEngineeringReport | null;
  loading: boolean;
  currency: string;
  period: Period;
  onPeriodChange: (period: Period) => void;
  /**
   * A wider window that DOES have recognized sales, when the selected one has
   * none (#834). Null/undefined means there is nothing honest to offer and the
   * empty state stays a plain statement.
   */
  widerPeriodWithSales?: Period | null;
  formatMoney: (value: Dollars, currency: string) => string;
  labels: {
    subtitle: string;
    periods: Record<Period, string>;
    axes: { margin: string; popularity: string };
    quadrants: Record<Quadrant, { label: string; action: string }>;
    cards: { revenueShare: string; dishes: string };
    table: {
      title: string;
      item: string;
      quadrant: string;
      foodCostPct: string;
      qty: string;
      price: string;
      margin: string;
      suggestion: string;
    };
    states: {
      /** Fallback: no dish data at all for the window. */
      empty: string;
      /**
       * Sales exist but no dish carries a plate/recipe cost (dishes empty,
       * items_needing_cost > 0). The blocker is costs, not sales (#834).
       */
      emptyMissingCost: string;
      /** Dishes are costed but nothing sold in the window (#834). */
      emptyNoSales: string;
      /**
       * Same window, but a wider one has sales. Carries a `{period}` slot for
       * the localized name of that period (#834).
       */
      emptyNoSalesWider: string;
      /** Button that switches to that wider period. Carries `{period}` (#834). */
      showWiderPeriod: string;
      sparse: string;
      needsCost: string;
      loading: string;
    };
    /** Read-only suggestion prefix, e.g. "Consider". Never a one-click apply. */
    suggest: string;
  };
}

// Build a complete per-quadrant rollup lookup. The backend only emits rollups
// for quadrants with count > 0, so we default the rest to zero so all four
// cards always render.
function buildRollupLookup(
  rollups: MenuEngineeringRollup[],
): Record<Quadrant, MenuEngineeringRollup> {
  const lookup: Record<Quadrant, MenuEngineeringRollup> = {
    star: { quadrant: "star", count: 0, revenue_share: 0 },
    plowhorse: { quadrant: "plowhorse", count: 0, revenue_share: 0 },
    puzzle: { quadrant: "puzzle", count: 0, revenue_share: 0 },
    dog: { quadrant: "dog", count: 0, revenue_share: 0 },
  };
  for (const r of rollups) {
    lookup[r.quadrant] = r;
  }
  return lookup;
}


/** True when the report has dishes but none sold — matrix axes are meaningless. */
function isZeroSalesReport(report: MenuEngineeringReport): boolean {
  if (report.dishes.length === 0) return false;
  return report.dishes.every((d) => d.qty_sold === 0);
}

// --- Scatter geometry (viewBox 0..100, inner padding) -----------------------
const SIZE = 100;
const PAD = 14;
const INNER = SIZE - PAD * 2;

function QuadrantScatter({
  report,
  labels,
}: {
  report: MenuEngineeringReport;
  labels: MenuEngineeringMatrixProps["labels"];
}) {
  const dishes = report.dishes;

  // X = popularity (qty_sold): low (left) → high (right).
  const xMax = Math.max(
    report.median_qty_sold,
    ...dishes.map((d) => d.qty_sold),
    1,
  );
  // Y = margin, INVERTED: high margin (low food_cost_pct) at TOP, low margin
  // (high food_cost_pct) at BOTTOM. So screen-Y grows with food_cost_pct.
  const yMax = Math.max(
    report.median_food_cost_pct,
    ...dishes.map((d) => d.food_cost_pct),
    0.01,
  );

  const xOf = (qty: number) => PAD + (qty / xMax) * INNER;
  const yOf = (pct: number) => PAD + (pct / yMax) * INNER;

  const medianX = xOf(report.median_qty_sold);
  const medianY = yOf(report.median_food_cost_pct);

  return (
    <svg
      viewBox={`0 0 ${SIZE} ${SIZE}`}
      role="img"
      aria-label={`${labels.axes.popularity} / ${labels.axes.margin}`}
      className="h-full w-full"
    >
      {/* Plot frame */}
      <rect
        x={PAD}
        y={PAD}
        width={INNER}
        height={INNER}
        className="fill-none stroke-warm-200"
        strokeWidth={0.4}
      />

      {/* Median crosshair (muted, dashed) */}
      <line
        x1={medianX}
        y1={PAD}
        x2={medianX}
        y2={PAD + INNER}
        className="stroke-warm-300"
        strokeWidth={0.5}
        strokeDasharray="2 2"
      />
      <line
        x1={PAD}
        y1={medianY}
        x2={PAD + INNER}
        y2={medianY}
        className="stroke-warm-300"
        strokeWidth={0.5}
        strokeDasharray="2 2"
      />

      {/* Quadrant corner labels */}
      <text
        x={PAD + INNER}
        y={PAD + 4}
        textAnchor="end"
        className={`${QUADRANT_STYLE.star.text} fill-current`}
        fontSize={4}
      >
        {labels.quadrants.star.label}
      </text>
      <text
        x={PAD}
        y={PAD + 4}
        textAnchor="start"
        className={`${QUADRANT_STYLE.puzzle.text} fill-current`}
        fontSize={4}
      >
        {labels.quadrants.puzzle.label}
      </text>
      <text
        x={PAD + INNER}
        y={PAD + INNER - 1.5}
        textAnchor="end"
        className={`${QUADRANT_STYLE.plowhorse.text} fill-current`}
        fontSize={4}
      >
        {labels.quadrants.plowhorse.label}
      </text>
      <text
        x={PAD}
        y={PAD + INNER - 1.5}
        textAnchor="start"
        className={`${QUADRANT_STYLE.dog.text} fill-current`}
        fontSize={4}
      >
        {labels.quadrants.dog.label}
      </text>

      {/* Dish dots */}
      {dishes.map((d) => (
        <circle
          key={d.menu_item_id}
          data-testid={`dish-dot-${d.menu_item_id}`}
          data-quadrant={d.quadrant}
          cx={xOf(d.qty_sold)}
          cy={yOf(d.food_cost_pct)}
          r={2.5}
          className={QUADRANT_STYLE[d.quadrant].dotFill}
        >
          <title>{`${d.menu_item_name} · ${Math.round(
            d.food_cost_pct * 100,
          )}% · ${d.qty_sold} sold`}</title>
        </circle>
      ))}

      {/* Axis labels */}
      <text
        x={SIZE / 2}
        y={SIZE - 1}
        textAnchor="middle"
        className="fill-ink-500"
        fontSize={4}
      >
        {labels.axes.popularity}
      </text>
      <text
        x={3.5}
        y={SIZE / 2}
        textAnchor="middle"
        transform={`rotate(-90, 3.5, ${SIZE / 2})`}
        className="fill-ink-500"
        fontSize={4}
      >
        {labels.axes.margin}
      </text>
    </svg>
  );
}

function PeriodTabs({
  period,
  onPeriodChange,
  labels,
}: {
  period: Period;
  onPeriodChange: (period: Period) => void;
  labels: MenuEngineeringMatrixProps["labels"];
}) {
  return (
    <Tabs
      aria-label={labels.subtitle}
      selectedKey={period}
      onSelectionChange={(key) => {
        const next = String(key);
        if (isPeriod(next)) {
          onPeriodChange(next);
        }
      }}
      size="sm"
      radius="full"
      variant="solid"
      className="shrink-0"
      classNames={{
        tabList: "h-7 gap-0 border border-warm-200 bg-warm-50 p-0.5 shadow-sm shadow-warm-900/5",
        tab: "h-6 px-2 text-[11px] font-medium text-ink-500",
        cursor: "bg-white shadow-sm shadow-warm-900/10",
      }}
    >
      {PERIOD_OPTIONS.map((p) => (
        <Tab key={p} title={labels.periods[p]} />
      ))}
    </Tabs>
  );
}

export default function MenuEngineeringMatrix({
  report,
  loading,
  currency,
  period,
  onPeriodChange,
  widerPeriodWithSales = null,
  formatMoney,
  labels,
}: MenuEngineeringMatrixProps) {
  const header = (
    <div className="flex flex-wrap items-start justify-between gap-2">
      <p className="min-w-0 text-sm text-ink-600">{labels.subtitle}</p>
      <PeriodTabs
        period={period}
        onPeriodChange={onPeriodChange}
        labels={labels}
      />
    </div>
  );

  // #834: business 86 opens on `week`, whose ISO window (Mon 00:00 → now) held
  // no recognized payment, while `month` carries $11,809.32 and a full matrix.
  // "No recognized sales in this period — try a wider period" is true and
  // useless: it makes the owner guess which period, next to an Accounting tab
  // showing $18,237.98. When the data owner has confirmed a wider window has
  // sales, name it and hand over the switch. `reportHasNoSales` gates this so
  // the missing-plate-cost copy — which asserts sales DO exist — is never
  // replaced by a no-sales offer.
  const salesElsewhere =
    reportHasNoSales(report) &&
    widerPeriodWithSales != null &&
    widerPeriodWithSales !== period
      ? widerPeriodWithSales
      : null;

  const renderEmpty = (testId: string, label: string) => (
    <div className="rounded-2xl border border-warm-200 bg-white px-4 py-3 shadow-sm shadow-warm-900/5">
      {header}
      <div
        data-testid={testId}
        className="mt-4 flex h-48 flex-col items-center justify-center gap-3 rounded-2xl border border-dashed border-warm-200 bg-warm-50/70 px-6 text-center text-sm text-ink-500"
      >
        <p className="min-w-0">
          {salesElsewhere
            ? labels.states.emptyNoSalesWider.replace(
                "{period}",
                labels.periods[salesElsewhere],
              )
            : label}
        </p>
        {salesElsewhere ? (
          <Button
            size="sm"
            variant="flat"
            data-testid="menu-engineering-show-wider-period"
            onPress={() => onPeriodChange(salesElsewhere)}
          >
            {labels.states.showWiderPeriod.replace(
              "{period}",
              labels.periods[salesElsewhere],
            )}
          </Button>
        ) : null}
      </div>
    </div>
  );

  // 1. Loading takes priority — skeleton placeholder, no scatter.
  if (loading) {
    return (
      <div className="rounded-2xl border border-warm-200 bg-white px-4 py-3 shadow-sm shadow-warm-900/5">
        {header}
        <div className="mt-4 flex h-48 animate-pulse items-center justify-center rounded-2xl border border-dashed border-warm-200 bg-warm-50/70 text-sm text-ink-500">
          {labels.states.loading}
        </div>
      </div>
    );
  }

  // 2. Nothing classified yet — empty state, no scatter/cards/table.
  // When the window HAS sales but no dish carries a complete cost, the
  // classifier emits dishes: [] with items_needing_cost > 0. The actual
  // blocker there is plate costs, so name it — telling an operator with real
  // billed income to "record some sales" is a lie (#834). But uncosted items
  // also exist with ZERO sales (a brand-new venue), so the missing-cost copy
  // — which asserts "sales are recorded" — must additionally be gated on the
  // backend's has_sales signal; otherwise the neutral empty copy wins (O2).
  if (!report || report.dishes.length === 0) {
    const emptyLabel =
      report && report.items_needing_cost > 0 && report.has_sales
        ? labels.states.emptyMissingCost
        : labels.states.empty;
    return renderEmpty("menu-engineering-empty", emptyLabel);
  }

  // 2b. Dishes present but all qty_sold === 0 (or total sales 0) — honest empty
  // rather than a matrix of stars pinned at 0,00 (L3-19). Copy names the real
  // condition: no recognized sales in the selected window (#834).
  if (isZeroSalesReport(report)) {
    return renderEmpty("menu-engineering-zero-sales", labels.states.emptyNoSales);
  }

  // 3. Full report.
  const rollupLookup = buildRollupLookup(report.rollups);

  return (
    <div className="rounded-2xl border border-warm-200 bg-white px-4 py-3 shadow-sm shadow-warm-900/5">
      {header}

      {report.items_needing_cost > 0 ? (
        <div className="mt-3 rounded-lg border border-amber-200 bg-amber-50 px-3 py-2 text-[13px] text-amber-800">
          {`${labels.states.needsCost} (${report.items_needing_cost})`}
        </div>
      ) : null}

      {report.sparse ? (
        <div className="mt-3 rounded-lg border border-brand/30 bg-brand/[0.05] px-3 py-2 text-[13px] text-brand">
          {labels.states.sparse}
        </div>
      ) : null}

      {/* Scatter + summary cards. Stack below xl so ~1024px never clips the
          2×2 card grid past the panel edge (#154). */}
      <div className="mt-4 grid min-w-0 gap-4 xl:grid-cols-[minmax(0,1fr)_minmax(0,1fr)]">
        <div className="mx-auto aspect-square w-full max-w-sm min-w-0">
          <QuadrantScatter report={report} labels={labels} />
        </div>

        {/* Summary cards — suggestion copy only, never one-click price apply. */}
        <div className="grid min-w-0 grid-cols-1 gap-3 sm:grid-cols-2">
          {QUADRANT_ORDER.map((q) => {
            const rollup = rollupLookup[q];
            const style = QUADRANT_STYLE[q];
            return (
              <div
                key={q}
                data-testid={`quadrant-card-${q}`}
                className={`min-w-0 overflow-hidden rounded-xl border ${style.border} bg-white p-3`}
              >
                <div className="flex min-w-0 items-start justify-between gap-2">
                  <span
                    className={`inline-flex min-w-0 items-center gap-1.5 text-sm font-semibold ${style.text}`}
                  >
                    <span className={`h-2 w-2 shrink-0 rounded-full ${style.dot}`} />
                    <span className="truncate">{labels.quadrants[q].label}</span>
                  </span>
                  <span className="shrink-0 text-xl font-semibold leading-none text-ink-950 sm:text-2xl">
                    {rollup.count}
                  </span>
                </div>
                <p className="mt-0.5 text-right text-[11px] text-ink-500">
                  {labels.cards.dishes}
                </p>
                <p className="mt-1 text-[11px] text-ink-500">
                  {`${Math.round(rollup.revenue_share * 100)}% ${
                    labels.cards.revenueShare
                  }`}
                </p>
                <p className="mt-2 line-clamp-3 text-xs leading-snug text-ink-600">
                  {labels.quadrants[q].action}
                </p>
              </div>
            );
          })}
        </div>
      </div>

      {/* Dish table */}
      <div
        data-testid="menu-engineering-table"
        className="mt-4 overflow-hidden rounded-2xl border border-warm-200 bg-white shadow-sm shadow-warm-900/5"
      >
        <p className="border-b border-warm-200/80 bg-warm-50/70 px-4 py-3 text-sm font-semibold text-ink-950">
          {labels.table.title}
        </p>
        <div className="overflow-x-auto">
          <table className="w-full text-sm">
            <thead>
              <tr className="text-left text-[11px] uppercase tracking-wide text-ink-500">
                <th className="px-4 py-2">{labels.table.item}</th>
                <th className="px-4 py-2">{labels.table.quadrant}</th>
                <th className="px-4 py-2 text-right">
                  {labels.table.foodCostPct}
                </th>
                <th className="px-4 py-2 text-right">{labels.table.qty}</th>
                <th className="px-4 py-2 text-right">{labels.table.price}</th>
                <th className="px-4 py-2 text-right">{labels.table.margin}</th>
                <th className="px-4 py-2 text-right">{labels.table.suggestion}</th>
              </tr>
            </thead>
            <tbody>
              {report.dishes.map((d) => {
                // Round once so the displayed % and its tone agree at the
                // food-cost boundaries (mirrors FoodCostTable).
                const rowPct = Math.round(d.food_cost_pct * 100);
                const style = QUADRANT_STYLE[d.quadrant];
                // Suggestion-only (#131): never render a one-click reprice.
                // Cap display at 2× so stale/extreme uplift stays hidden.
                const showSuggestion =
                  d.suggested_price > 0 &&
                  !(d.avg_price > 0 && d.suggested_price / d.avg_price > 2);
                return (
                  <tr
                    key={d.menu_item_id}
                    className="border-t border-warm-200/70"
                  >
                    <td className="px-4 py-2 font-medium text-ink-950">
                      {d.menu_item_name}
                    </td>
                    <td className="px-4 py-2">
                      <span
                        className={`inline-flex rounded-full px-2 py-0.5 text-[11px] font-medium ${style.badge}`}
                      >
                        {labels.quadrants[d.quadrant].label}
                      </span>
                    </td>
                    <td
                      className={`px-4 py-2 text-right font-medium ${pctClasses(
                        rowPct / 100,
                      )}`}
                    >
                      {rowPct}%
                    </td>
                    <td className="px-4 py-2 text-right text-ink-700">
                      {d.qty_sold}
                    </td>
                    <td className="px-4 py-2 text-right text-ink-700">
                      {formatMoney(d.avg_price, currency)}
                    </td>
                    <td className="px-4 py-2 text-right text-ink-700">
                      {formatMoney(d.margin_per_unit, currency)}
                    </td>
                    <td className="px-4 py-2 text-right text-ink-600">
                      {showSuggestion ? (
                        <span
                          data-testid={`suggest-${d.menu_item_id}`}
                          className="text-xs leading-snug"
                        >
                          {`${labels.suggest} ${formatMoney(
                            d.suggested_price,
                            currency,
                          )}`}
                        </span>
                      ) : (
                        <span className="text-ink-400">—</span>
                      )}
                    </td>
                  </tr>
                );
              })}
            </tbody>
          </table>
        </div>
      </div>
    </div>
  );
}
