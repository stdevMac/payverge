"use client";

import React from "react";
import { Popover, PopoverContent, PopoverTrigger } from "@nextui-org/react";

import type { Business } from "@/api/business";
import {
  getTranslation,
  useSimpleLocale,
} from "@/i18n/SimpleTranslationProvider";
import { useDirectorBriefing } from "@/hooks/useDirectorBriefing";
import { useAuth } from "@/providers/HybridAuthProvider";
import { useUserStore } from "@/store/useUserStore";
import {
  greetingKey,
  localizeInsightCardParams,
  ownerFirstName,
  toBriefingChips,
  toPreShiftCards,
} from "./insightCopy";
import { formatCurrency } from "@/api/currency";
import { intlLocaleFor } from "@/utils/intlLocale";
import { TIME_SHORT, businessLocalHour, formatBusinessTime } from "@/utils/businessTime";
import InsightsDrawer from "./InsightsDrawer";

interface BriefingStripProps {
  business: Business;
  onOpenTab: (tab: string) => void;
}

const CHIP_TONE: Record<string, string> = {
  urgent: "border-rose-200 bg-rose-50 text-rose-700 hover:border-rose-300",
  watch: "border-amber-200 bg-amber-50 text-amber-700 hover:border-amber-300",
  info: "border-warm-200 bg-white text-ink-700 hover:border-warm-300",
};

/**
 * Session-bound display name for the front-door greeting.
 * Prefer the live principal (staff name, then owner username) over
 * business.owner_name, which is fixture/stale for demo rows ("Alex Demo").
 */
function sessionDisplayName(
  staffName?: string | null,
  username?: string | null,
): string | null {
  const staff = (staffName || "").trim();
  if (staff) return staff;
  const user = (username || "").trim();
  if (user) return user;
  return null;
}

/**
 * The compact briefing strip: a single greeting + count row over one row of
 * severity-sorted, horizontally-scrollable insight chips. Bounded height
 * (flex-none) so the chat below flex-fills the viewport. "View all" opens the
 * full InsightsDrawer. Replaces the stacked PreShift block on the console page.
 */
export default function BriefingStrip({
  business,
  onOpenTab,
}: BriefingStripProps) {
  const { locale } = useSimpleLocale();
  const currency = business.default_currency || "USD";
  const [drawerOpen, setDrawerOpen] = React.useState(false);
  const { staffData } = useAuth();
  const sessionUser = useUserStore((s) => s.user);

  const t = (key: string, params?: Record<string, string | number>): string => {
    const value = getTranslation(`directorConsole.${key}`, locale, params);
    return Array.isArray(value) ? value[0] || key : (value as string);
  };

  const { briefing, loading, fetchedAt } = useDirectorBriefing(business.id, true);

  // Greet the authenticated principal — never a demo/fixture owner_name.
  const firstName = ownerFirstName(
    sessionDisplayName(staffData?.name, sessionUser?.username),
  );
  const gKey = greetingKey(businessLocalHour(new Date(), business.timezone ?? null));
  const greeting = firstName
    ? t(`preShift.greeting.${gKey}Named`, { name: firstName })
    : t(`preShift.greeting.${gKey}`);
  const asOfLabel = fetchedAt
    ? t("strip.asOf", {
        time: formatBusinessTime(
          fetchedAt,
          locale,
          business.timezone ?? null,
          TIME_SHORT,
        ),
      })
    : null;

  const allCards = React.useMemo(
    () => toPreShiftCards(briefing?.insights),
    [briefing?.insights],
  );
  const chips = React.useMemo(
    () => toBriefingChips(briefing?.insights, 5),
    [briefing?.insights],
  );
  const total = allCards.length;

  const chipLabel = (card: (typeof chips)[number]): string => {
    let params = localizeInsightCardParams(card.params, t);
    if (typeof params.amount === "number") {
      params = {
        ...params,
        amount: formatCurrency(
          params.amount,
          currency,
          undefined,
          intlLocaleFor(locale),
        ),
      };
    }
    return t(`preShift.cards.${card.copyKey}`, params);
  };

  if (loading && !briefing) {
    return (
      <section
        data-testid="dc-strip-loading"
        aria-busy="true"
        className="flex-none border-b border-warm-100 pb-4"
      >
        <div className="h-6 w-64 animate-pulse rounded-full bg-warm-100" />
        <div className="mt-3 flex gap-2">
          {[0, 1, 2].map((i) => (
            <span
              key={i}
              className="h-8 w-40 animate-pulse rounded-full bg-warm-100"
            />
          ))}
        </div>
      </section>
    );
  }

  return (
    <section
      aria-label={t("title")}
      className="flex-none border-b border-warm-100 pb-4"
    >
      <div className="flex flex-wrap items-center gap-x-3 gap-y-1">
        <h1
          data-testid="preshift-greeting"
          className="font-title text-heading-lg text-ink-900"
        >
          {greeting}
        </h1>
        {asOfLabel ? (
          <span
            data-testid="dc-strip-as-of"
            className="text-body-sm text-ink-500"
          >
            {asOfLabel}
          </span>
        ) : null}
        {total > 0 ? (
          <span
            data-testid="dc-strip-count"
            className="inline-flex items-center rounded-full bg-brand/10 px-2.5 py-0.5 text-body-sm font-semibold text-brand"
          >
            {total === 1
              ? t("strip.countOne", { count: total })
              : t("strip.countOther", { count: total })}
          </span>
        ) : null}
        {total > 0 ? (
          <button
            type="button"
            data-testid="dc-strip-view-all"
            onClick={() => setDrawerOpen(true)}
            className="ml-auto inline-flex items-center rounded-full border border-brand/30 bg-brand/[0.06] px-3 py-1 text-body-sm font-semibold text-brand transition-colors hover:bg-brand/10 focus:outline-none focus-visible:ring-2 focus-visible:ring-brand"
          >
            {t("strip.viewAll", { count: total })}
          </button>
        ) : null}
      </div>

      {briefing?.pulse ? (
        <div className="mt-2 flex flex-wrap items-center gap-x-3 gap-y-1">
          <p
            data-testid="dc-strip-pulse"
            className="text-body-sm text-ink-600"
          >
            {t("strip.pulseToday", {
              revenue: formatCurrency(
                briefing.pulse.revenue,
                currency,
                undefined,
                intlLocaleFor(locale),
              ),
              orders: briefing.pulse.orders,
              avgTicket: formatCurrency(
                briefing.pulse.avg_ticket,
                currency,
                undefined,
                intlLocaleFor(locale),
              ),
            })}
            {(briefing.pulse.remaining ?? 0) > 0
              ? ` ${t("strip.pulseRemaining", {
                  remaining: formatCurrency(
                    briefing.pulse.remaining ?? 0,
                    currency,
                    undefined,
                    intlLocaleFor(locale),
                  ),
                })}`
              : ""}
          </p>
          <button
            type="button"
            data-testid="dc-strip-open-analytics"
            onClick={() => onOpenTab("analytics")}
            className="text-body-sm font-semibold text-brand hover:underline focus:outline-none focus-visible:ring-2 focus-visible:ring-brand"
          >
            {t("strip.openAnalytics")}
          </button>
        </div>
      ) : null}

      {total > 0 ? (
        <div className="mt-3 flex flex-wrap gap-2 pb-1">
          {chips.map((card) => {
            const label = chipLabel(card);
            return (
              <Popover key={card.id} placement="bottom-start">
                <PopoverTrigger>
                  <button
                    type="button"
                    data-testid="dc-briefing-chip"
                    data-insight-id={card.id}
                    aria-label={label}
                    title={label}
                    className={`inline-flex max-w-full items-start gap-1.5 rounded-2xl border px-3 py-1.5 text-left text-body-sm font-medium transition-colors focus:outline-none focus-visible:ring-2 focus-visible:ring-brand sm:max-w-[22rem] ${CHIP_TONE[card.tone] ?? CHIP_TONE.info}`}
                  >
                    <card.icon className="mt-0.5 h-3.5 w-3.5 flex-none" aria-hidden="true" />
                    <span className="line-clamp-2 whitespace-normal break-words">
                      {label}
                    </span>
                    <span
                      data-testid="dc-briefing-chip-expand"
                      aria-hidden="true"
                      className="mt-0.5 shrink-0 text-current/70"
                    >
                      ▾
                    </span>
                  </button>
                </PopoverTrigger>
                <PopoverContent className="max-w-sm rounded-2xl border border-warm-200 bg-white p-4 shadow-xl shadow-warm-900/10">
                  <div className="flex flex-col gap-3">
                    <p
                      data-testid="dc-briefing-chip-full"
                      className="text-body-sm font-medium text-ink-800 whitespace-normal break-words"
                    >
                      {label}
                    </p>
                    <button
                      type="button"
                      data-testid="dc-briefing-chip-open"
                      onClick={() => onOpenTab(card.tab)}
                      className="inline-flex items-center justify-center rounded-full border border-brand/30 bg-brand/[0.06] px-3 py-1.5 text-body-sm font-semibold text-brand transition-colors hover:bg-brand/10 focus:outline-none focus-visible:ring-2 focus-visible:ring-brand"
                    >
                      {t("strip.chipAction")}
                    </button>
                  </div>
                </PopoverContent>
              </Popover>
            );
          })}
        </div>
      ) : (
        <p className="mt-2 text-body-sm text-ink-500">{t("strip.allClear")}</p>
      )}

      {briefing ? (
        <InsightsDrawer
          open={drawerOpen}
          onClose={() => setDrawerOpen(false)}
          briefing={briefing}
          business={business}
          onOpenTab={onOpenTab}
          asOf={fetchedAt}
        />
      ) : null}
    </section>
  );
}
