"use client";

import React, { useCallback, useMemo } from "react";
import { useQuery } from "@tanstack/react-query";
import { Chip } from "@nextui-org/react";
import { NotebookPen } from "lucide-react";
import { logbookApi, type ShiftNote } from "@/api/logbook";
import { queryKeys } from "@/api/queryKeys";
import { getTranslation, useSimpleLocale } from "@/i18n/SimpleTranslationProvider";
import { localDateKey } from "@/lib/localDate";
import { businessDateKey, formatBusinessTime } from "@/utils/businessTime";

interface OperatorLogbookProps {
  businessId: string;
  businessTimezone?: string | null;
}

/**
 * OperatorLogbook surfaces today's staff shift-handover notes to managers on the
 * operator Schedule tab — the read side of the logbook staff already write from
 * their "More → Logbook". Each note shows WHO logged it (author_name, resolved
 * server-side) for accountability. Polls (60s) so notes logged mid-shift appear
 * without a reload. Money-free — handover context, never a pay figure. Renders
 * nothing when there are no notes today (keeps the tab clean) or the read fails
 * (a non-manager stays hidden — schedule:read is the real server-side gate).
 */
export default function OperatorLogbook({
  businessId,
  businessTimezone = null,
}: OperatorLogbookProps) {
  const { locale } = useSimpleLocale();
  const today = useMemo(
    () => businessDateKey(new Date(), businessTimezone) || localDateKey(),
    [businessTimezone],
  );

  const t = useCallback(
    (key: string): string => {
      const v = getTranslation(`dashboardLogbook.${key}`, locale);
      return Array.isArray(v) ? v[0] || key : (v as string);
    },
    [locale],
  );

  const fmtTime = useCallback(
    (iso: string): string => {
      const d = new Date(iso);
      if (Number.isNaN(d.getTime())) return "";
      return formatBusinessTime(d, locale, businessTimezone);
    },
    [locale, businessTimezone],
  );

  const query = useQuery({
    queryKey: queryKeys.logbook.list(businessId, today),
    queryFn: () => logbookApi.list(businessId, today),
    refetchInterval: 60_000, // handover notes accrue through the shift; keep live
    retry: false, // a 403 (non-manager) must not retry-thrash; stay hidden
  });

  const notes = useMemo<ShiftNote[]>(() => query.data ?? [], [query.data]);

  // Nothing logged today (or an errored/loading read) → render nothing, so the
  // Schedule tab stays uncluttered until there's a handover to read.
  if (notes.length === 0) return null;

  const count =
    notes.length === 1
      ? t("countOne")
      : t("countOther").replace("{count}", String(notes.length));

  return (
    <section aria-label={t("title")} className="rounded-xl border border-warm-200">
      <div className="flex flex-wrap items-center justify-between gap-3 border-b border-warm-200 p-4">
        <div className="flex items-center gap-2">
          <NotebookPen className="h-4 w-4 text-brand" aria-hidden="true" />
          <h2 className="font-title text-base text-ink-900">{t("title")}</h2>
        </div>
        <span className="text-xs text-ink-500">{count}</span>
      </div>

      <ul className="divide-y divide-warm-100">
        {notes.map((n) => (
          <li key={n.id} className="px-4 py-3">
            <div className="mb-1 flex items-center justify-between gap-3">
              <div className="flex min-w-0 items-center gap-2">
                <span className="truncate text-sm font-medium text-ink-900">
                  {n.author_name || t("authorFallback")}
                </span>
                <Chip size="sm" variant="flat" className="bg-brand-50 text-brand">
                  {t(`categories.${n.category}`)}
                </Chip>
              </div>
              <span className="shrink-0 text-xs text-ink-500">{fmtTime(n.created_at)}</span>
            </div>
            <p className="whitespace-pre-wrap text-sm text-ink-700">{n.content}</p>
          </li>
        ))}
      </ul>
    </section>
  );
}
