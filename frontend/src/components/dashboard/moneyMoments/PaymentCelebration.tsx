"use client";

import React from "react";
import { Sparkles, X } from "lucide-react";
import { formatCurrency } from "@/api/currency";
import { useSimpleLocale } from "@/i18n/SimpleTranslationProvider";
import { intlLocaleFor } from "@/utils/intlLocale";
import { useCountUp } from "./useCountUp";
import { formatMethodLabel } from "./paymentMoment";

export interface MoneyMoment {
  id: string;
  amount: number;
  method: string;
  billId: number | null;
}

export interface PaymentCelebrationProps {
  moments: MoneyMoment[];
  currency: string;
  /** Localized-string lookup (full key path). */
  t: (key: string) => string;
  onDismiss: (id: string) => void;
}

function MoneyMomentCard({
  moment,
  currency,
  t,
  onDismiss,
}: {
  moment: MoneyMoment;
  currency: string;
  t: (key: string) => string;
  onDismiss: (id: string) => void;
}) {
  // The amount ticks up from zero — the little hit of dopamine that makes an
  // operator want to keep the tab open.
  const animated = useCountUp(moment.amount, { startFrom: 0 });
  const methodLabel = formatMethodLabel(moment.method, t);
  // Route the amount through the operator locale so es/es-AR see comma-grouped
  // separators, matching the dashboard summary cards — not the en-US default
  // formatCurrency falls back to when the locale arg is omitted.
  const { locale } = useSimpleLocale();

  const meta = [
    methodLabel ? `${t("businessDashboard.moneyMoments.via")} ${methodLabel}` : null,
    moment.billId
      ? `${t("businessDashboard.moneyMoments.bill")} #${moment.billId}`
      : null,
  ]
    .filter(Boolean)
    .join(" · ");

  return (
    <div className="pointer-events-auto flex items-center gap-3 rounded-2xl border border-brand-100 bg-warm-50 px-4 py-3 shadow-xl shadow-brand-900/10 motion-safe:animate-[money-moment-in_260ms_cubic-bezier(0.16,1,0.3,1)]">
      <span className="flex h-10 w-10 shrink-0 items-center justify-center rounded-full bg-brand/10 text-brand">
        <Sparkles className="h-5 w-5" aria-hidden="true" />
      </span>
      <div className="min-w-0 flex-1">
        <p className="text-label uppercase tracking-wider text-brand">
          {t("businessDashboard.moneyMoments.received")}
        </p>
        <p
          data-testid="money-moment-amount"
          className="font-serif text-heading-md leading-tight text-ink-900 tabular-nums"
        >
          {formatCurrency(animated, currency, undefined, intlLocaleFor(locale))}
        </p>
        {meta && <p className="truncate text-[12px] text-ink-500">{meta}</p>}
      </div>
      <button
        type="button"
        onClick={() => onDismiss(moment.id)}
        aria-label={t("businessDashboard.moneyMoments.dismiss")}
        className="shrink-0 rounded-lg p-1 text-ink-400 transition-colors hover:bg-warm-100 hover:text-ink-600 focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-brand"
      >
        <X className="h-4 w-4" aria-hidden="true" />
      </button>
    </div>
  );
}

/**
 * Bottom-right stack of payment-received celebrations. Wrapped in a polite
 * live region so screen readers announce incoming payments without stealing
 * focus from whatever the operator is doing.
 */
export default function PaymentCelebration({
  moments,
  currency,
  t,
  onDismiss,
}: PaymentCelebrationProps) {
  if (moments.length === 0) return null;

  return (
    <div
      role="status"
      aria-live="polite"
      aria-atomic="false"
      className="pointer-events-none fixed bottom-4 right-4 z-[120] flex w-[min(92vw,22rem)] flex-col gap-2"
    >
      {moments.map((moment) => (
        <MoneyMomentCard
          key={moment.id}
          moment={moment}
          currency={currency}
          t={t}
          onDismiss={onDismiss}
        />
      ))}
    </div>
  );
}
