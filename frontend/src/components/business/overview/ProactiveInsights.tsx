"use client";

import React from "react";
import Link from "next/link";
import { Sparkles, ArrowRight } from "lucide-react";

import { insightDurationMinutes } from "../DirectorConsole/insightCopy";

interface ProactiveInsight {
  id: string | number;
  type: string;
  params: Record<string, unknown>;
  cta: { tab: string };
}

const KNOWN_INSIGHT_TYPES = new Set([
  "inventory_out_of_stock",
  "inventory_low_stock",
  "stale_open_bills",
  "ai_conversations_pending",
  "food_cost_high",
  "waste_high",
  "labor_high",
]);

interface ProactiveInsightsProps {
  insights: ProactiveInsight[];
  loading?: boolean;
  t: (key: string, params?: Record<string, string | number>) => string;
  onOpenConsole: () => void;
}

function getInsightTitle(
  t: (key: string, params?: Record<string, string | number>) => string,
  insight: ProactiveInsight,
): string {
  const count = Number(insight.params.count ?? 0);
  let subKey = count === 1 ? "title_one" : "title_other";
  const params: Record<string, string | number> = { count };
  if (insight.type === "stale_open_bills") {
    // #817: `count` is the bills as old as the rendered duration; `stale_count`
    // is every bill past the 2h threshold. Business 86 has 1 and 2 — bill 761
    // at ~7.4d and bill 1143 at ~1.2d. A single-age title either smears 7 days
    // over both or drops 1143 from the briefing, so when the counts disagree
    // report the real total and attribute the age to the oldest bill alone.
    const staleTotal = Number(insight.params.stale_count ?? 0);
    if (staleTotal > count) {
      params.count = staleTotal;
      subKey = "title_mixed";
    }
    params.duration = formatInsightDuration(t, staleInsightMinutes(insight.params));
  }
  if (insight.params.amount != null) params.amount = Math.round(Number(insight.params.amount));
  if (insight.params.ingredient != null) params.ingredient = String(insight.params.ingredient);
  if (insight.params.pct != null) params.pct = Math.round(Number(insight.params.pct) * 100);
  return t(`proactive.types.${insight.type}.${subKey}`, params);
}

function formatInsightDuration(
  t: (key: string, params?: Record<string, string | number>) => string,
  minutes: number,
): string {
  const safeMinutes = Math.max(0, Math.floor(Number.isFinite(minutes) ? minutes : 0));
  const [unit, count] =
    safeMinutes >= 1440
      ? ["days", Math.floor(safeMinutes / 1440)]
      : safeMinutes >= 60
        ? ["hours", Math.floor(safeMinutes / 60)]
        : ["minutes", safeMinutes];
  const key = `proactive.duration.${unit}_${count === 1 ? "one" : "other"}`;
  return t(key, { count });
}

function staleInsightMinutes(params: Record<string, unknown>): number {
  const fromInsight = insightDurationMinutes(params);
  if (fromInsight > 0) return fromInsight;
  const threshold = Number(params.threshold_minutes);
  if (Number.isFinite(threshold) && threshold > 0) return Math.floor(threshold);
  return 120;
}

function getInsightSummary(
  t: (key: string, params?: Record<string, string | number>) => string,
  insight: ProactiveInsight,
): string {
  const count = Number(insight.params.count ?? 0);
  const items =
    (insight.params.item_names as string[] | undefined) ?? [];
  const itemsList = items.slice(0, 3).join(", ");
  return t(`proactive.types.${insight.type}.summary`, {
    count,
    items: itemsList,
    threshold_minutes: Number(insight.params.threshold_minutes ?? 0),
    duration:
      insight.type === "stale_open_bills"
        ? formatInsightDuration(t, staleInsightMinutes(insight.params))
        : String(insight.params.duration ?? ""),
    amount: Math.round(Number(insight.params.amount ?? 0)),
    ingredient: String(insight.params.ingredient ?? ""),
    pct: Math.round(Number(insight.params.pct ?? 0) * 100),
  });
}

function getCTALabel(
  t: (key: string, params?: Record<string, string | number>) => string,
  tab: string,
): string {
  return t(`proactive.ctas.${tab}`);
}

export default function ProactiveInsights({
  insights,
  loading,
  t,
  onOpenConsole,
}: ProactiveInsightsProps) {
  if (loading) {
    return (
      <div className="rounded-2xl border border-warm-200 bg-white p-6 h-32 animate-pulse" />
    );
  }

  if (!insights.length) {
    // Nothing flagged is the happy path — one quiet line, not a boxed panel.
    return (
      <p className="flex items-center gap-1.5 text-sm text-ink-500">
        <Sparkles className="h-3.5 w-3.5 flex-shrink-0" aria-hidden="true" />
        <span>
          <span className="font-medium text-ink-600">
            {t("proactive.empty.label")}
          </span>{" "}
          {t("proactive.empty.copy")}
        </span>
      </p>
    );
  }

  return (
    <div className="rounded-2xl border border-warm-200 bg-white p-6">
      <div className="flex items-center justify-between mb-4">
        <p className="text-label uppercase text-ink-500 inline-flex items-center gap-1">
          <Sparkles className="w-3 h-3 text-brand" aria-hidden="true" />
          {t("proactive.label")}
        </p>
        <button
          type="button"
          onClick={onOpenConsole}
          className="text-body-sm font-semibold text-brand hover:text-brand-dark inline-flex items-center gap-1"
        >
          {t("proactive.openConsole")}
          <ArrowRight className="w-3 h-3" aria-hidden="true" />
        </button>
      </div>
      <ul className="divide-y divide-warm-100">
        {(() => {
          const renderable = insights.filter((i) => KNOWN_INSIGHT_TYPES.has(i.type)).slice(0, 3);
          if (renderable.length === 0 && insights.length > 0) {
            console.warn("[ProactiveInsights] All insights have unknown types; none rendered.", insights.map((i) => i.type));
          }
          return renderable;
        })().map((insight) => (
          <li key={insight.id} className="py-3 first:pt-0 last:pb-0">
            <h4 className="text-sm font-semibold text-ink-900">
              {getInsightTitle(t, insight)}
            </h4>
            <p className="text-body-sm text-ink-600 mt-1 mb-2">
              {getInsightSummary(t, insight)}
            </p>
            <Link
              href={`?tab=${insight.cta.tab}`}
              className="text-body-sm font-semibold text-brand hover:text-brand-dark inline-flex items-center gap-1"
            >
              {getCTALabel(t, insight.cta.tab)}
              <ArrowRight className="w-3 h-3" aria-hidden="true" />
            </Link>
          </li>
        ))}
      </ul>
    </div>
  );
}
