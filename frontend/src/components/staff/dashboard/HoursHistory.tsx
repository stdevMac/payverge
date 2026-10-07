"use client";

import React, { useMemo } from "react";
import { useQuery } from "@tanstack/react-query";
import { timeclockApi, type TimeEntry } from "@/api/timeclock";
import { SkeletonList } from "@/components/ui/skeletons";
import { localDateKey } from "@/lib/localDate";

interface HoursHistoryLabels {
  title: string;
  subtitle: string;
  loading: string;
  empty: string;
  weekTotal: string;
  hoursUnit: string;
  statusPending: string;
  statusApproved: string;
}

interface HoursHistoryProps {
  businessId: string;
  labels: HoursHistoryLabels;
  locale: string;
}

// ISO week key (YYYY-Www) for grouping; locale-neutral.
function isoWeekKey(d: Date): string {
  const date = new Date(Date.UTC(d.getUTCFullYear(), d.getUTCMonth(), d.getUTCDate()));
  const day = date.getUTCDay() || 7;
  date.setUTCDate(date.getUTCDate() + 4 - day);
  const yearStart = new Date(Date.UTC(date.getUTCFullYear(), 0, 1));
  const week = Math.ceil(((date.getTime() - yearStart.getTime()) / 86400000 + 1) / 7);
  return `${date.getUTCFullYear()}-W${String(week).padStart(2, "0")}`;
}

function HoursHistory({ businessId, labels, locale }: HoursHistoryProps) {
  const to = useMemo(() => new Date(), []);
  const from = useMemo(() => new Date(Date.now() - 56 * 86400000), []);
  const fmt = localDateKey;

  const { data: entries = [], isLoading } = useQuery({
    queryKey: ["staff", "hours", businessId, fmt(from), fmt(to)],
    queryFn: () => timeclockApi.listMineRange(businessId, fmt(from), fmt(to)),
  });

  const weeks = useMemo(() => {
    const byWeek = new Map<string, { entries: TimeEntry[]; minutes: number }>();
    for (const e of entries) {
      const key = isoWeekKey(new Date(e.clock_in_at));
      const bucket = byWeek.get(key) || { entries: [], minutes: 0 };
      bucket.entries.push(e);
      bucket.minutes += e.worked_minutes || 0;
      byWeek.set(key, bucket);
    }
    return Array.from(byWeek.entries()).sort((a, b) => (a[0] < b[0] ? 1 : -1));
  }, [entries]);

  return (
    <section aria-label={labels.title}>
      <header className="mb-3">
        <h2 className="text-lg font-semibold text-gray-900">{labels.title}</h2>
        <p className="text-sm text-gray-500">{labels.subtitle}</p>
      </header>
      {isLoading ? (
        <SkeletonList rows={3} ariaLabel={labels.loading} />
      ) : weeks.length === 0 ? (
        <p className="py-8 text-center text-sm text-gray-500">{labels.empty}</p>
      ) : (
        <div className="space-y-4">
          {weeks.map(([week, bucket]) => (
            <div key={week} className="rounded-2xl border border-gray-200 p-4">
              <div className="mb-2 flex items-center justify-between">
                <span className="text-sm font-medium text-gray-900">{week}</span>
                <span className="text-sm text-gray-600">
                  {labels.weekTotal}: {(bucket.minutes / 60).toFixed(1)}
                  {labels.hoursUnit}
                </span>
              </div>
              <ul className="space-y-1">
                {bucket.entries.map((e) => (
                  <li key={e.id} className="flex items-center justify-between text-sm text-gray-700">
                    <span>{new Date(e.clock_in_at).toLocaleDateString(locale)}</span>
                    <span className="flex items-center gap-2">
                      <span>
                        {((e.worked_minutes || 0) / 60).toFixed(1)}
                        {labels.hoursUnit}
                      </span>
                      <span className="text-xs text-gray-400">
                        {e.status === "approved" ? labels.statusApproved : labels.statusPending}
                      </span>
                    </span>
                  </li>
                ))}
              </ul>
            </div>
          ))}
        </div>
      )}
    </section>
  );
}

export default HoursHistory;
