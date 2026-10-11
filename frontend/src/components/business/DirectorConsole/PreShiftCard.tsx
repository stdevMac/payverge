"use client";

import React from "react";
import { ArrowRight } from "lucide-react";

import {
  getTranslation,
  useSimpleLocale,
} from "@/i18n/SimpleTranslationProvider";
import { formatCurrency } from "@/api/currency";
import { intlLocaleFor } from "@/utils/intlLocale";
import {
  localizeInsightCardParams,
  type PreShiftCardModel,
  type PreShiftTone,
} from "./insightCopy";

const TONE_ICON: Record<PreShiftTone, string> = {
  urgent: "bg-rose-100 text-rose-600",
  watch: "bg-amber-100 text-amber-600",
  info: "bg-brand/10 text-brand",
};

// Tone reads at a glance from the whole card, not just the icon — the most
// urgent item is visibly warmer than a routine watch/info note. Kept subtle
// (a soft wash + matching border) rather than a loud one-sided color bar.
const TONE_CARD: Record<PreShiftTone, string> = {
  urgent: "border-rose-200 bg-rose-50/40 hover:border-rose-300",
  watch: "border-warm-200 bg-white hover:border-warm-300",
  info: "border-warm-200 bg-white hover:border-warm-300",
};

interface PreShiftCardProps {
  model: PreShiftCardModel;
  onOpen: (tab: string) => void;
  /** business currency code (e.g. "EUR", "ARS") — drives money-card symbol/separators */
  currency: string;
}

export default function PreShiftCard({
  model,
  onOpen,
  currency,
}: PreShiftCardProps) {
  const { locale } = useSimpleLocale();
  const t = (key: string, params?: Record<string, string | number>): string => {
    const v = getTranslation(`directorConsole.${key}`, locale, params);
    return Array.isArray(v) ? v[0] || key : (v as string);
  };

  // The `amount` param (waste/labor cards) is a business-currency figure — format
  // it with the same locale/currency rules the Accounting tab uses (the tab this
  // card deep-links into) so the symbol and separators match instead of a
  // hardcoded "$" with no thousands grouping.
  // L1-23: always re-localize duration via t() so Spanish cards never show
  // English "40 days" from backend FormatInsightDuration / missing minutes.
  let params: Record<string, string | number> = localizeInsightCardParams(
    { ...model.params },
    t,
  );
  if (typeof model.params.amount === "number") {
    params = {
      ...params,
      amount: formatCurrency(
        model.params.amount,
        currency,
        undefined,
        intlLocaleFor(locale),
      ),
    };
  }

  const Icon = model.icon;
  const line = t(`preShift.cards.${model.copyKey}`, params);

  // The whole card is the click target so the hover-lift is honest: tapping
  // anywhere opens the tab the operator needs to act in.
  return (
    <button
      type="button"
      data-testid="preshift-card"
      onClick={() => onOpen(model.tab)}
      aria-label={`${t("preShift.action")} · ${line}`}
      className={`group w-full text-left rounded-2xl border transition-all hover:shadow-sm focus:outline-none focus-visible:ring-2 focus-visible:ring-brand focus-visible:ring-offset-2 ${TONE_CARD[model.tone]}`}
    >
      <span className="flex flex-row items-center gap-4 px-5 py-4">
        <span
          className={`flex h-10 w-10 flex-shrink-0 items-center justify-center rounded-full ${TONE_ICON[model.tone]}`}
        >
          <Icon className="h-5 w-5" />
        </span>
        <span className="min-w-0 flex-1 text-body-sm text-ink-800">{line}</span>
        <span
          data-testid="preshift-card-action"
          className="flex flex-shrink-0 items-center gap-1 rounded-full px-3 py-1.5 text-body-sm font-semibold text-brand transition-colors group-hover:bg-brand/10"
        >
          {t("preShift.action")}
          <ArrowRight className="h-4 w-4" />
        </span>
      </span>
    </button>
  );
}
