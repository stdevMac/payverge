"use client";

import React, { useCallback, useMemo } from "react";
import { useQuery } from "@tanstack/react-query";
import { Chip } from "@nextui-org/react";
import { Clock, AlertTriangle, UserX } from "lucide-react";
import {
  timeclockApi,
  type LiveFloorRow,
  type LiveFloorStatus,
} from "@/api/timeclock";
import { queryKeys } from "@/api/queryKeys";
import {
  getTranslation,
  useSimpleLocale,
} from "@/i18n/SimpleTranslationProvider";
import { formatBusinessTime } from "@/utils/businessTime";

interface LiveFloorBoardProps {
  businessId: string;
  /** Venue IANA zone — shift/punch clocks must not use the device zone. */
  businessTimezone?: string | null;
}

// Which rows we surface individually (problems + who's on the clock now) and in
// what order. "scheduled" and "done" are shown as footer counts only, not rows —
// the board is about what needs attention right now, not the whole roster.
const ATTENTION_ORDER: Record<LiveFloorStatus, number> = {
  no_show: 0,
  late: 1,
  on_clock: 2,
  scheduled: 3,
  done: 4,
};

const DOT_CLASS: Record<string, string> = {
  on_clock: "bg-emerald-500",
  late: "bg-amber-500",
  no_show: "bg-red-500",
};

/**
 * LiveFloorBoard is the manager "who's on the floor today" pulse on the operator
 * Schedule tab: who's on the clock, running late, or a no-show right now. It
 * polls the timeclock:manage-gated live-floor endpoint (60s) so late/no-show
 * stay fresh as the wall clock advances. Money-free — hours-context only, never
 * a pay figure. Renders nothing when there's nothing to show (no clutter) or the
 * read fails (a non-manager 403 just stays hidden — the server is the real gate).
 */
export default function LiveFloorBoard({
  businessId,
  businessTimezone = null,
}: LiveFloorBoardProps) {
  const { locale } = useSimpleLocale();

  const t = useCallback(
    (key: string): string => {
      const v = getTranslation(`dashboardLiveFloor.${key}`, locale);
      return Array.isArray(v) ? v[0] || key : (v as string);
    },
    [locale],
  );

  const fmtTime = useCallback(
    (iso?: string): string => {
      if (!iso) return "";
      return formatBusinessTime(iso, locale, businessTimezone);
    },
    [locale, businessTimezone],
  );

  const query = useQuery({
    queryKey: queryKeys.liveFloor.day(businessId),
    queryFn: () => timeclockApi.liveFloor(businessId),
    refetchInterval: 60_000, // late/no-show shift with the clock; keep it live
    refetchOnWindowFocus: true,
    retry: false, // a 403 (non-manager) must not retry-thrash; stay hidden
  });

  const data = query.data;
  const rows = useMemo<LiveFloorRow[]>(() => data?.rows ?? [], [data]);

  const attentionRows = useMemo(
    () =>
      rows
        .filter(
          (r) =>
            r.status === "no_show" ||
            r.status === "late" ||
            r.status === "on_clock",
        )
        .sort(
          (a, b) =>
            ATTENTION_ORDER[a.status] - ATTENTION_ORDER[b.status] ||
            a.staff_name.localeCompare(b.staff_name),
        ),
    [rows],
  );

  // Nothing scheduled and nobody punched in → render nothing. Also hides the
  // error/loading case so the tab never shows an empty box or a non-manager 403.
  if (!data || rows.length === 0) return null;

  const s = data.summary;

  const context = (r: LiveFloorRow): string => {
    switch (r.status) {
      case "on_clock":
        return t("since").replace("{time}", fmtTime(r.clock_in_at));
      case "late":
        return t("minLate").replace("{minutes}", String(r.late_minutes ?? 0));
      case "no_show":
        return `${fmtTime(r.shift_start)}–${fmtTime(r.shift_end)}`;
      default:
        return "";
    }
  };

  const footerParts: string[] = [];
  if (s.scheduled > 0)
    footerParts.push(t("footerScheduled").replace("{count}", String(s.scheduled)));
  if (s.done > 0)
    footerParts.push(t("footerDone").replace("{count}", String(s.done)));

  return (
    <section
      aria-label={t("title")}
      className="rounded-xl border border-warm-200"
    >
      <div className="flex flex-wrap items-center justify-between gap-3 border-b border-warm-200 p-4">
        <h2 className="text-sm font-semibold text-ink-900">{t("title")}</h2>
        <div className="flex flex-wrap items-center gap-2">
          {s.on_clock > 0 ? (
            <Chip
              size="sm"
              variant="flat"
              color="success"
              startContent={<Clock className="h-3.5 w-3.5" />}
            >
              {s.on_clock} {t("chipOnClock")}
            </Chip>
          ) : null}
          {s.late > 0 ? (
            <Chip
              size="sm"
              variant="flat"
              color="warning"
              startContent={<AlertTriangle className="h-3.5 w-3.5" />}
            >
              {s.late} {t("chipLate")}
            </Chip>
          ) : null}
          {s.no_show > 0 ? (
            <Chip
              size="sm"
              variant="flat"
              color="danger"
              startContent={<UserX className="h-3.5 w-3.5" />}
            >
              {s.no_show} {t("chipNoShow")}
            </Chip>
          ) : null}
        </div>
      </div>

      {attentionRows.length > 0 ? (
        <ul className="divide-y divide-warm-100">
          {attentionRows.map((r) => (
            <li
              key={`${r.staff_id}-${r.shift_id ?? "x"}`}
              className="flex items-center justify-between gap-3 px-4 py-3"
            >
              <div className="flex min-w-0 items-center gap-3">
                <span
                  aria-hidden
                  className={`h-2.5 w-2.5 shrink-0 rounded-full ${DOT_CLASS[r.status] ?? "bg-warm-300"}`}
                />
                <span className="truncate font-medium text-ink-900">
                  {r.staff_name || t("unknownStaff")}
                </span>
              </div>
              <span className="shrink-0 text-sm text-ink-500">
                {context(r)}
              </span>
            </li>
          ))}
        </ul>
      ) : null}

      {footerParts.length > 0 ? (
        <div className="border-t border-warm-100 px-4 py-2 text-xs text-ink-500">
          {footerParts.join(" · ")}
        </div>
      ) : null}
    </section>
  );
}
