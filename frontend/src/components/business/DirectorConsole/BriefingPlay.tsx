"use client";

import React from "react";
import type { Business } from "@/api/business";
import type { BriefingPlay as BriefingPlayDTO } from "@/api/directorConsole";
import { formatCurrency } from "@/api/currency";
import { intlLocaleFor } from "@/utils/intlLocale";
import { getTranslation } from "@/i18n/SimpleTranslationProvider";
import type { Locale } from "@/i18n/localeRegistry";
import { briefingPlayCtaKey, briefingPlayKey } from "./insightCopy";
import { playHasGroundedFigures } from "./revenueHonesty";
import { Figure, weave } from "./briefingProse";

interface BriefingPlayProps {
  play: BriefingPlayDTO;
  business: Business;
  locale: Locale;
  onOpenTab: (tab: string) => void;
}

/**
 * The single "one move worth making" element: a thin rule + heading, the play
 * sentence (with its dollar figures woven inline), and ONE clear CTA that
 * deep-links into the Menu tab. A single distinct element — not a card in a
 * card — per the briefing's editorial direction.
 */
export default function BriefingPlay({
  play,
  business,
  locale,
  onOpenTab,
}: BriefingPlayProps) {
  const intlTag = intlLocaleFor(locale);
  const currency = business.default_currency || "USD";

  const t = (key: string, params?: Record<string, string | number>): string => {
    const value = getTranslation(`directorConsole.${key}`, locale, params);
    return Array.isArray(value) ? value[0] || key : (value as string);
  };
  const tmpl = (key: string): string => {
    const value = getTranslation(`directorConsole.${key}`, locale);
    return Array.isArray(value) ? value[0] || key : (value as string);
  };
  const money = (n: number) =>
    formatCurrency(n, currency, undefined, intlTag);

  if (!playHasGroundedFigures(play)) {
    return (
      <div data-testid="dc-briefing-play" className="mt-6 max-w-prose">
        <p
          data-testid="dc-revenue-not-enough-data"
          className="text-body text-ink-600 leading-relaxed"
        >
          {t("honesty.notEnoughData")}
        </p>
      </div>
    );
  }

  const parts: Record<string, React.ReactNode> = {
    item: <Figure>{play.item_name}</Figure>,
    impact: <Figure>{money(play.monthly_impact)}</Figure>,
  };
  if (play.current_price != null) {
    parts.current = <Figure>{money(play.current_price)}</Figure>;
  }
  if (play.suggested_price != null) {
    parts.suggested = <Figure>{money(play.suggested_price)}</Figure>;
  }

  return (
    <div data-testid="dc-briefing-play" className="mt-6 max-w-prose">
      <div className="flex items-center gap-3">
        <span className="h-px flex-1 bg-ink-200" aria-hidden="true" />
        <span className="text-label font-semibold uppercase tracking-wide text-ink-500">
          {t("preShift.briefing.moveHeading")}
        </span>
        <span className="h-px flex-1 bg-ink-200" aria-hidden="true" />
      </div>

      <p className="mt-3 text-body text-ink-600 leading-relaxed">
        {weave(tmpl(briefingPlayKey(play.kind)), parts)}
      </p>

      <div className="mt-3">
        <button
          type="button"
          data-testid="dc-briefing-play-cta"
          onClick={() => onOpenTab(play.tab)}
          aria-label={t("preShift.briefing.play.ctaAria", { item: play.item_name })}
          className="inline-flex items-center rounded-full border border-brand/30 bg-brand/5 px-4 py-2 text-body-sm font-semibold text-brand transition-colors hover:bg-brand/10 focus:outline-none focus-visible:ring-2 focus-visible:ring-brand focus-visible:ring-offset-2"
        >
          {t(briefingPlayCtaKey(play.kind))}
        </button>
      </div>
    </div>
  );
}
