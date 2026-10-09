"use client";

import React, { useEffect, useMemo, useState } from "react";
import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query";
import { Button } from "@nextui-org/react";
import { Coffee, ExternalLink, Play, Square } from "lucide-react";
import { timeclockApi, type TimeEntry } from "@/api/timeclock";
import { queryKeys } from "@/api/queryKeys";
import { SkeletonLine } from "@/components/ui/skeletons";
import { intlLocaleFor } from "@/utils/intlLocale";
import { localDateKey } from "@/lib/localDate";
import type { StaffData } from "@/utils/staffAuth";
import { resolveStaffTabs } from "@/constants/staffTabAccess";
import { useStaffPermissions } from "@/hooks/useStaffPermissions";

export interface TodayCardLabels {
  title: string;
  serviceToolsTitle: string;
  serviceToolsDescription: string;
  serviceToolsOpen: string;
  /** "Loading your day…" — reused from staffHome for the clock spinner aria. */
  loading: string;
  // Time clock (Slice 4) — hours/minutes only, never money.
  clockIn: string;
  clockOut: string;
  clockingIn: string;
  clockingOut: string;
  onTheClock: string;
  sinceTemplate: string; // "Since {time}"
  notClockedIn: string;
  workedToday: string;
  breakLabel: string;
  breakPreset: string; // "+{minutes}m"
  hoursUnit: string; // "h"
  minutesUnit: string; // "m"
  clockInError: string;
  clockOutError: string;
  breakError: string;
}

export interface TodayCardProps {
  staff: StaffData;
  labels: TodayCardLabels;
  /** Display locale for Intl time formatting (not a translated string). */
  locale: string;
}

// Break presets a server can punch in one tap. Pure minutes — no money.
const BREAK_PRESETS = [10, 15, 30];

function pad2(n: number): string {
  return String(n).padStart(2, "0");
}

// Elapsed running clock as H:MM:SS — staff see time on the clock, never pay.
function formatElapsed(totalSeconds: number): string {
  const h = Math.floor(totalSeconds / 3600);
  const m = Math.floor((totalSeconds % 3600) / 60);
  const s = totalSeconds % 60;
  return `${h}:${pad2(m)}:${pad2(s)}`;
}

function formatHoursMinutes(totalMinutes: number, labels: TodayCardLabels): string {
  const h = Math.floor(totalMinutes / 60);
  const m = totalMinutes % 60;
  const parts: string[] = [];
  if (h > 0) parts.push(`${h}${labels.hoursUnit}`);
  if (m > 0 || h === 0) parts.push(`${m}${labels.minutesUnit}`);
  return parts.join(" ");
}

export default function TodayCard({ staff, labels, locale }: TodayCardProps) {
  const businessId = String(staff.business_id);
  const queryClient = useQueryClient();
  const mineKey = queryKeys.timesheet.mine(businessId, staff.id);

  const [now, setNow] = useState(() => Date.now());
  const [error, setError] = useState<string | null>(null);

  const {
    data: staffPermsData,
    isLoading: permsLoading,
    isError: permsError,
  } = useStaffPermissions(businessId, staff.id, true);
  const permissions = useMemo(
    () => staffPermsData?.permissions ?? [],
    [staffPermsData?.permissions],
  );

  // Deep-link to the operator dashboard. Kitchen staff go straight to their KDS;
  // everyone else lands on the dashboard root, which resolves their allowed
  // default tab. Business is identified by slug (preferred) or numeric id.
  // Fail closed to overview-only when permissions are loading/error/empty.
  const serviceToolsHref = useMemo(() => {
    const biz = staff.business_slug || String(staff.business_id);
    const resolved =
      permsError || (permsLoading && permissions.length === 0)
        ? (["overview"] as string[])
        : resolveStaffTabs(permissions);
    // Empty resolve must not mean unrestricted (dashboard/sidebar treat [] as open).
    const allowed: string[] =
      resolved.length > 0 ? resolved : ["overview"];
    const preferred =
      staff.role === "kitchen" && allowed.includes("kitchen")
        ? "kitchen"
        : null;
    return `/business/${biz}/dashboard${preferred ? `?tab=${preferred}` : ""}`;
  }, [
    staff.business_slug,
    staff.business_id,
    staff.role,
    permissions,
    permsLoading,
    permsError,
  ]);

  const query = useQuery({
    queryKey: mineKey,
    queryFn: () => timeclockApi.myTimesheet(businessId),
  });

  const entries: TimeEntry[] = useMemo(() => query.data ?? [], [query.data]);
  const openEntry = useMemo(
    () => entries.find((e) => e.status === "open") ?? null,
    [entries],
  );
  const clockedIn = openEntry !== null;

  // Tick once a second only while clocked in; the running timer (and the live
  // worked-today total) are derived from `now`.
  useEffect(() => {
    if (!clockedIn) return;
    setNow(Date.now());
    const id = window.setInterval(() => setNow(Date.now()), 1000);
    return () => window.clearInterval(id);
  }, [clockedIn]);

  const onSettled = () => {
    void queryClient.invalidateQueries({ queryKey: mineKey });
  };

  const clockInMutation = useMutation({
    mutationFn: () => timeclockApi.clockIn(businessId),
    onMutate: () => setError(null),
    onError: () => setError(labels.clockInError),
    onSettled,
  });
  const clockOutMutation = useMutation({
    mutationFn: () => timeclockApi.clockOut(businessId),
    onMutate: () => setError(null),
    onError: () => setError(labels.clockOutError),
    onSettled,
  });
  const breakMutation = useMutation({
    mutationFn: (minutes: number) => timeclockApi.addBreak(businessId, minutes),
    onMutate: () => setError(null),
    onError: () => setError(labels.breakError),
    onSettled,
  });

  const elapsedSeconds = openEntry
    ? Math.max(0, Math.floor((now - new Date(openEntry.clock_in_at).getTime()) / 1000))
    : 0;

  const sinceTime = openEntry
    ? new Intl.DateTimeFormat(intlLocaleFor(locale), {
        hour: "numeric",
        minute: "2-digit",
      }).format(new Date(openEntry.clock_in_at))
    : "";

  const todayKey = localDateKey(new Date(now));
  const workedTodayMinutes = useMemo(() => {
    return entries.reduce((sum, e) => {
      if (localDateKey(new Date(e.clock_in_at)) !== todayKey) return sum;
      if (e.status === "open") {
        const liveMinutes = Math.floor((now - new Date(e.clock_in_at).getTime()) / 60000);
        return sum + Math.max(0, liveMinutes - Math.max(0, e.break_minutes || 0));
      }
      return sum + Math.max(0, e.worked_minutes || 0);
    }, 0);
  }, [entries, now, todayKey]);

  const hasTodayEntry = useMemo(
    () => entries.some((e) => localDateKey(new Date(e.clock_in_at)) === todayKey),
    [entries, todayKey],
  );

  const busy =
    clockInMutation.isPending || clockOutMutation.isPending || breakMutation.isPending;

  return (
    <div className="space-y-4">
      <section className="rounded-xl border border-gray-200 p-4" aria-label={labels.title}>
        <h2 className="font-title text-base text-gray-900">{labels.title}</h2>

        {query.isLoading ? (
          <div
            role="status"
            aria-live="polite"
            aria-label={labels.loading}
            className="mt-3 space-y-3 motion-safe:animate-pulse"
          >
            <SkeletonLine width="100%" height="4.5rem" className="rounded-lg" />
            <SkeletonLine width="100%" height="2.75rem" className="rounded-lg" />
          </div>
        ) : clockedIn ? (
          <div className="mt-3 space-y-3">
            <div className="rounded-lg bg-brand-50 px-4 py-3 text-center">
              <p className="text-[11px] font-semibold uppercase tracking-wide text-brand">
                {labels.onTheClock}
              </p>
              <p
                className="mt-1 font-mono text-2xl font-semibold tabular-nums text-brand-900"
                aria-live="polite"
              >
                {formatElapsed(elapsedSeconds)}
              </p>
              <p className="mt-0.5 text-xs text-brand">
                {labels.sinceTemplate.replace("{time}", sinceTime)}
              </p>
            </div>

            <Button
              color="danger"
              variant="flat"
              className="w-full"
              startContent={<Square className="h-4 w-4" />}
              isLoading={clockOutMutation.isPending}
              isDisabled={busy}
              onPress={() => clockOutMutation.mutate()}
            >
              {clockOutMutation.isPending ? labels.clockingOut : labels.clockOut}
            </Button>

            <div>
              <p className="mb-1.5 text-xs font-medium text-gray-500">{labels.breakLabel}</p>
              <div className="flex flex-wrap gap-2">
                {BREAK_PRESETS.map((minutes) => (
                  <Button
                    key={minutes}
                    size="sm"
                    variant="bordered"
                    startContent={<Coffee className="h-3.5 w-3.5" />}
                    isLoading={breakMutation.isPending && breakMutation.variables === minutes}
                    isDisabled={busy}
                    onPress={() => breakMutation.mutate(minutes)}
                  >
                    {labels.breakPreset.replace("{minutes}", String(minutes))}
                  </Button>
                ))}
              </div>
            </div>
          </div>
        ) : (
          <div className="mt-3 space-y-3">
            <p className="text-sm text-gray-600">{labels.notClockedIn}</p>
            <Button
              color="primary"
              className="w-full"
              startContent={<Play className="h-4 w-4" />}
              isLoading={clockInMutation.isPending}
              isDisabled={busy}
              onPress={() => clockInMutation.mutate()}
            >
              {clockInMutation.isPending ? labels.clockingIn : labels.clockIn}
            </Button>
          </div>
        )}

        {hasTodayEntry ? (
          <p className="mt-3 text-xs text-gray-500">
            {labels.workedToday}:{" "}
            <span className="font-medium text-gray-700">
              {formatHoursMinutes(workedTodayMinutes, labels)}
            </span>
          </p>
        ) : null}

        {error ? <p className="mt-2 text-sm text-red-600">{error}</p> : null}
      </section>

      <section className="rounded-xl border border-gray-200 p-4" aria-label={labels.serviceToolsTitle}>
        <h2 className="font-title text-base text-gray-900">{labels.serviceToolsTitle}</h2>
        <p className="mt-1 text-[13px] text-gray-500">{labels.serviceToolsDescription}</p>
        <Button
          as="a"
          href={serviceToolsHref}
          className="mt-3 w-full"
          variant="bordered"
          startContent={<ExternalLink className="h-4 w-4" />}
        >
          {labels.serviceToolsOpen}
        </Button>
      </section>
    </div>
  );
}
