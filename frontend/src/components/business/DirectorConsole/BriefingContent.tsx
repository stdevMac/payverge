"use client";

import React from "react";

import type { Business } from "@/api/business";
import type { BriefingResponse } from "@/api/directorConsole";
import { formatCurrency } from "@/api/currency";
import { intlLocaleFor } from "@/utils/intlLocale";
import {
  getTranslation,
  useSimpleLocale,
} from "@/i18n/SimpleTranslationProvider";
import {
  briefingHealthKey,
  briefingReadKey,
  briefingWinKey,
  toPreShiftCards,
} from "./insightCopy";
import { playHasGroundedFigures } from "./revenueHonesty";
import { Figure, weave } from "./briefingProse";
import BriefingPlay from "./BriefingPlay";
import PreShiftCard from "./PreShiftCard";
import {
  TIME_SHORT,
  formatBusinessTime,
  resolveBusinessTimeZone,
} from "@/utils/businessTime";

interface BriefingContentProps {
  briefing: BriefingResponse;
  business: Business;
  onOpenTab: (tab: string) => void;
  /** When true, render every insight card; when false, render none (strip owns chips). Default true. */
  showCards?: boolean;
  /** Last successful briefing fetch — venue-local "as of" stamp. */
  asOf?: Date | null;
}

/**
 * The briefing body — "Needs you" cards + the GM-voice read (with health clause)
 * + the single play, or a win when there's no play. Extracted verbatim from
 * PreShift so both the (retired-from-page) PreShift and the new InsightsDrawer
 * render an identical block. All money/percent formatting is locale + business
 * currency aware, mirroring the Accounting tab.
 */
export default function BriefingContent({
  briefing,
  business,
  onOpenTab,
  showCards = true,
  asOf = null,
}: BriefingContentProps) {
  const { locale } = useSimpleLocale();
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

  const now = new Date();
  const venueZone = resolveBusinessTimeZone(business.timezone ?? null);
  const money = (n: number) => formatCurrency(n, currency, undefined, intlTag);
  const percentOfFraction = (frac: number) =>
    new Intl.NumberFormat(intlTag, {
      style: "percent",
      maximumFractionDigits: 0,
    }).format(frac);
  const count = (n: number) => new Intl.NumberFormat(intlTag).format(n);

  const { pulse, insights, play, win, state } = briefing;
  const cards = toPreShiftCards(insights);
  const isLearning = state === "learning";

  const readKey = briefingReadKey(state, pulse.pace_pct);
  const weekdayLong = new Intl.DateTimeFormat(intlTag, {
    weekday: "long",
    timeZone: venueZone,
  }).format(now);
  const weekdayCap = weekdayLong.charAt(0).toUpperCase() + weekdayLong.slice(1);

  const readParts: Record<string, React.ReactNode> = {
    weekday: weekdayCap,
    weekdayLower: weekdayLong,
    revenue: <Figure>{money(pulse.revenue)}</Figure>,
    orders: <Figure>{count(pulse.orders)}</Figure>,
    avgTicket: <Figure>{money(pulse.avg_ticket)}</Figure>,
  };
  if (pulse.pace_pct != null && pulse.projected != null) {
    readParts.projected = <Figure>{money(pulse.projected)}</Figure>;
    readParts.pace = (
      <Figure>{percentOfFraction(Math.abs(pulse.pace_pct) / 100)}</Figure>
    );
  }

  const healthKey = isLearning
    ? null
    : briefingHealthKey(pulse.food_cost_pct, pulse.labor_cost_pct);
  const healthParts: Record<string, React.ReactNode> = {};
  if (pulse.food_cost_pct != null) {
    healthParts.food = <Figure>{percentOfFraction(pulse.food_cost_pct)}</Figure>;
  }
  if (pulse.labor_cost_pct != null) {
    healthParts.labor = (
      <Figure>{percentOfFraction(pulse.labor_cost_pct)}</Figure>
    );
  }

  return (
    <>
      {showCards && cards.length > 0 ? (
        <div className="mt-4">
          <p className="text-body-sm font-medium text-ink-500">
            {t("preShift.intro")}
          </p>
          <div className="mt-3 flex flex-col gap-3">
            {cards.map((card) => (
              <PreShiftCard
                key={card.id}
                model={card}
                onOpen={onOpenTab}
                currency={currency}
              />
            ))}
          </div>
        </div>
      ) : null}

      {asOf ? (
        <p
          data-testid="dc-briefing-as-of"
          className={`text-body-sm text-ink-500 ${
            showCards && cards.length > 0 ? "mt-6" : "mt-3"
          }`}
        >
          {t("strip.asOf", {
            time: formatBusinessTime(asOf, locale, business.timezone ?? null, TIME_SHORT),
          })}
        </p>
      ) : null}

      <p
        data-testid="dc-briefing-read"
        className={`max-w-prose text-body text-ink-600 leading-relaxed ${
          asOf ? "mt-2" : showCards && cards.length > 0 ? "mt-6" : "mt-3"
        }`}
      >
        {isLearning ? tmpl(readKey) : weave(tmpl(readKey), readParts)}
        {healthKey ? <> {weave(tmpl(healthKey), healthParts)}</> : null}
      </p>

      {!isLearning ? (
        <button
          type="button"
          data-testid="dc-briefing-open-analytics"
          onClick={() => onOpenTab("analytics")}
          className="mt-2 text-body-sm font-semibold text-brand hover:underline focus:outline-none focus-visible:ring-2 focus-visible:ring-brand"
        >
          {t("strip.openAnalytics")}
        </button>
      ) : null}

      {!isLearning && play && playHasGroundedFigures(play) ? (
        <BriefingPlay
          play={play}
          business={business}
          locale={locale}
          onOpenTab={onOpenTab}
        />
      ) : !isLearning && play ? (
        <p
          data-testid="dc-revenue-not-enough-data"
          className="mt-4 max-w-prose text-body text-ink-600 leading-relaxed"
        >
          {t("honesty.notEnoughData")}
        </p>
      ) : !isLearning && win && Number.isFinite(win.pct) ? (
        <p
          data-testid="dc-briefing-win"
          className="mt-4 max-w-prose text-body text-ink-600 leading-relaxed"
        >
          {weave(tmpl(briefingWinKey(win.kind)), {
            pct: <Figure>{percentOfFraction(win.pct / 100)}</Figure>,
          })}
        </p>
      ) : null}

      {/* S3-Loop: marketing posts this period → Marketing Library. Not a schedule. */}
      {!isLearning &&
      briefing.marketing &&
      briefing.marketing.posts_this_period > 0 ? (
        <div
          data-testid="dc-briefing-marketing"
          className="mt-4 rounded-2xl border border-brand/15 bg-brand/[0.04] px-5 py-4"
        >
          <p className="max-w-prose text-body text-ink-700 leading-relaxed">
            {t(
              briefing.marketing.posts_this_period === 1
                ? "preShift.briefing.marketing.one"
                : "preShift.briefing.marketing.other",
              {
                count: briefing.marketing.posts_this_period,
                days: briefing.marketing.period_days || 7,
              },
            )}
            {briefing.marketing.channels?.length
              ? ` ${t("preShift.briefing.marketing.channelsSuffix", {
                  channels: briefing.marketing.channels.slice(0, 3).join(", "),
                })}`
              : null}
          </p>
          <button
            type="button"
            data-testid="dc-briefing-marketing-cta"
            onClick={() => onOpenTab(briefing.marketing?.tab || "marketing")}
            className="mt-3 rounded-full border border-brand/30 bg-white px-4 py-1.5 text-body-sm font-medium text-brand transition-colors hover:bg-brand/10 focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-brand"
          >
            {t("preShift.briefing.marketing.cta")}
          </button>
        </div>
      ) : null}
    </>
  );
}
