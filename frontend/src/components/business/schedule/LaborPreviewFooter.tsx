"use client";

import { Input } from "@nextui-org/react";
import type { LaborPreview } from "@/api/schedule";
import { intlLocaleFor } from "@/utils/intlLocale";
import { WarningChips, type WarningLabels } from "./WarningChips";

export interface LaborPreviewLabels {
  title: string;
  scheduledHours: string;
  laborCost: string;
  laborCostPct: string;
  salesTarget: string;
  salesTargetPlaceholder: string;
  /** One-liner under the target input explaining what the number feeds. */
  salesTargetHint: string;
  /** Shown to non-financial callers in place of the $ / target input. */
  hoursOnly: string;
  warnings: WarningLabels;
}

export interface LaborPreviewFooterProps {
  preview: LaborPreview;
  labels: LaborPreviewLabels;
  /**
   * UI affordance gate for labor dollars (owner / financial:read; Slice 4 prop
   * on ScheduleBuilder). Defaults false. Even when true the $ also requires the
   * server's `preview.can_see_dollars` — the backend independently strips $ on
   * the wire, so this is defense-in-depth, never the security boundary.
   */
  canViewFinancials?: boolean;
  /** Business reporting currency (ISO code) for labor-$ formatting. */
  currency?: string;
  /** Locale for number/currency formatting. */
  locale?: string;
  /** Current sales target (dollars) shown in the input; owned by the builder. */
  salesTarget?: number;
  onSalesTargetChange: (v: number | undefined) => void;
  /** staff_id → display name so warnings can name people, not ids. */
  staffNames?: Map<number, string>;
  /** Opens Schedule settings from a posted_late warning (lead-time fix path). */
  onPostedLateAction?: () => void;
  /** Accessible label for the posted_late action. */
  postedLateActionLabel?: string;
}

/** Currency symbol for the input adornment, derived per locale (no literal $). */
function currencySymbol(currency: string, locale: string): string {
  try {
    const parts = new Intl.NumberFormat(intlLocaleFor(locale), {
      style: "currency",
      currency: (currency || "USD").toUpperCase(),
    }).formatToParts(0);
    return parts.find((p) => p.type === "currency")?.value ?? currency;
  } catch {
    return currency;
  }
}

/**
 * Operator-only labor footer (Schedule tab). Shows scheduled hours for
 * everyone; labor-%, labor-$ and the editable sales target render ONLY when
 * the caller may see financials. Never imported by any staff shell.
 */
export function LaborPreviewFooter({
  preview,
  labels,
  canViewFinancials = false,
  currency = "USD",
  locale = "en",
  salesTarget,
  onSalesTargetChange,
  staffNames,
  onPostedLateAction,
  postedLateActionLabel,
}: LaborPreviewFooterProps) {
  const showDollars = canViewFinancials && preview.can_see_dollars;
  // Labor-% is pay divided by sales, so it is financial data: the server only
  // sends it to financial callers. With no sales target there is no
  // denominator — "0.0%" would read as a computed value.
  const hasTarget = (salesTarget ?? preview.sales_target ?? 0) > 0;
  const showPct = showDollars && preview.labor_cost_pct != null;
  const pct =
    !hasTarget || preview.labor_cost_pct == null
      ? "—"
      : `${(preview.labor_cost_pct * 100).toFixed(1)}%`;

  const fmtMoney = (value: number): string =>
    new Intl.NumberFormat(intlLocaleFor(locale), {
      style: "currency",
      currency: (currency || "USD").toUpperCase(),
    }).format(value);

  return (
    <section
      aria-label={labels.title}
      className="rounded-2xl border border-warm-200 bg-white p-4 shadow-sm shadow-warm-900/5"
    >
      <h3 className="text-sm font-semibold text-ink-950">{labels.title}</h3>

      <div className="mt-3 flex flex-wrap items-end gap-x-8 gap-y-3">
        <div>
          <p className="text-xs font-medium text-ink-500">{labels.scheduledHours}</p>
          <p className="text-xl font-semibold tabular-nums text-ink-950">
            {preview.total_hours.toFixed(1)}
          </p>
        </div>

        {showDollars ? (
          <div>
            <p className="text-xs font-medium text-ink-500">{labels.laborCost}</p>
            <p className="text-xl font-semibold tabular-nums text-brand-dark">
              {fmtMoney(preview.labor_cost ?? 0)}
            </p>
          </div>
        ) : null}

        {showPct ? (
          <div>
            <p className="text-xs font-medium text-ink-500">{labels.laborCostPct}</p>
            <p className="text-xl font-semibold tabular-nums text-ink-950">{pct}</p>
          </div>
        ) : null}

        {showDollars ? (
          <Input
            type="number"
            min={0}
            size="sm"
            variant="bordered"
            label={labels.salesTarget}
            placeholder={labels.salesTargetPlaceholder}
            value={salesTarget != null ? String(salesTarget) : ""}
            onValueChange={(v) => onSalesTargetChange(v === "" ? undefined : Number(v))}
            startContent={
              <span className="text-sm text-ink-500">{currencySymbol(currency, locale)}</span>
            }
            description={labels.salesTargetHint}
            className="max-w-[220px]"
          />
        ) : (
          <p className="self-center text-xs text-ink-500">{labels.hoursOnly}</p>
        )}
      </div>

      {preview.warnings.length > 0 ? (
        <div className="mt-4">
          <WarningChips
            warnings={preview.warnings}
            labels={labels.warnings}
            staffNames={staffNames}
            onPostedLateAction={onPostedLateAction}
            postedLateActionLabel={postedLateActionLabel}
          />
        </div>
      ) : null}
    </section>
  );
}
