"use client";

import React, { useCallback, useEffect, useState } from "react";
import { AlertTriangle, PieChart } from "lucide-react";
import { getSegments } from "@/api/crm";
import {
  getTranslation,
  useSimpleLocale,
} from "@/i18n/SimpleTranslationProvider";
import { EmptyState } from "@/components/ui/EmptyState";
import { CRMSegmentsSkeleton } from "./CRMSegmentsSkeleton";

const SEGMENT_KEYS = ["lapsed", "vip", "new", "atRisk"] as const;
type SegmentKey = (typeof SEGMENT_KEYS)[number];

// Maps the UI segment key to the backend segment param the customer list filters
// on. atRisk → at-risk (the list endpoint accepts both, this is the canonical).
const SEGMENT_PARAM: Record<SegmentKey, string> = {
  lapsed: "lapsed",
  vip: "vip",
  new: "new",
  atRisk: "at-risk",
};

interface SegmentsTabProps {
  businessId: number;
  /**
   * Drill into the Customers tab filtered to a segment (fix 7). Called with the
   * backend segment param when a card (or the empty-state CTA) is clicked.
   */
  onJumpToCustomers: (segment?: string) => void;
  /** When set (e.g. from Marketing win-back deep link), visually emphasize one segment card. */
  highlightSegment?: SegmentKey;
}

export default function SegmentsTab({
  businessId,
  onJumpToCustomers,
  highlightSegment,
}: SegmentsTabProps) {
  const [counts, setCounts] = useState<Record<string, number>>({});
  const [loading, setLoading] = useState(true);
  const [error, setError] = useState(false);
  const [reloadKey, setReloadKey] = useState(0);
  const { locale } = useSimpleLocale();

  const t = useCallback(
    (key: string): string => {
      const fullKey = `businessDashboard.crm.segments.${key}`;
      const result = getTranslation(fullKey, locale);
      return typeof result === "string" ? result : fullKey;
    },
    [locale],
  );

  useEffect(() => {
    let cancelled = false;
    setLoading(true);
    setError(false);
    getSegments(businessId)
      .then((next) => {
        if (!cancelled) setCounts(next);
      })
      .catch(() => {
        // Surface failures as a retryable error card instead of swallowing them
        // into zeros that render a misleading "no segments yet" empty state
        // (audit §3.5 MED, fix 8).
        if (!cancelled) setError(true);
      })
      .finally(() => {
        if (!cancelled) setLoading(false);
      });
    return () => {
      cancelled = true;
    };
  }, [businessId, reloadKey]);

  if (loading) return <CRMSegmentsSkeleton />;

  if (error) {
    return (
      <EmptyState
        icon={AlertTriangle}
        title={getTranslation("businessDashboard.crm.segments.error.title", locale) as string}
        subtitle={getTranslation("businessDashboard.crm.segments.error.subtitle", locale) as string}
        actionLabel={getTranslation("businessDashboard.crm.segments.error.retry", locale) as string}
        onAction={() => setReloadKey((k) => k + 1)}
      />
    );
  }

  const allEmpty = SEGMENT_KEYS.every((key) => !counts[key]);

  if (allEmpty) {
    return (
      <EmptyState
        icon={PieChart}
        title={getTranslation("businessDashboard.crm.empty.segments.title", locale) as string}
        subtitle={getTranslation("businessDashboard.crm.empty.segments.subtitle", locale) as string}
        actionLabel={getTranslation("businessDashboard.crm.empty.segments.action", locale) as string}
        onAction={() => onJumpToCustomers()}
      />
    );
  }

  return (
    <div className="grid grid-cols-1 sm:grid-cols-2 xl:grid-cols-4 gap-3">
      {SEGMENT_KEYS.map((key) => {
        const count = counts[key] ?? 0;
        const clickable = count > 0;
        return (
          <button
            key={key}
            type="button"
            disabled={!clickable}
            onClick={() => onJumpToCustomers(SEGMENT_PARAM[key])}
            aria-label={`${t(`${key}.title`)} — ${count}`}
            className={`text-left border rounded-2xl p-4 bg-white transition-colors ${
              clickable
                ? "hover:border-brand/40 hover:bg-brand/[0.03] cursor-pointer"
                : "cursor-default opacity-80"
            } ${
              highlightSegment === key
                ? "border-brand/40 ring-2 ring-brand/20 shadow-sm"
                : "border-warm-100"
            }`}
          >
            <h3 className="font-semibold text-ink-900">{t(`${key}.title`)}</h3>
            <p className="text-ink-500 text-sm mb-3">{t(`${key}.description`)}</p>
            <p className="text-3xl font-semibold text-ink-900 tabular-nums">
              {count}
            </p>
            {clickable && (
              <span className="mt-2 inline-block text-xs font-medium text-brand">
                {getTranslation(
                  "businessDashboard.crm.segments.viewCustomers",
                  locale,
                ) as string}
              </span>
            )}
          </button>
        );
      })}
    </div>
  );
}
