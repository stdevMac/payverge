"use client";

import React, { useCallback, useMemo, useState } from "react";
import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query";
import { Button } from "@nextui-org/react";
import { CalendarX2 } from "lucide-react";
import { scheduleApi, type Shift } from "@/api/schedule";
import { scheduleSettingsApi } from "@/api/scheduleSettings";
import { positionsApi, type Position } from "@/api/positions";
import { coverageApi, type SwapKind } from "@/api/coverage";
import { queryKeys } from "@/api/queryKeys";
import { EmptyState } from "@/components/ui/EmptyState";
import { SkeletonLine } from "@/components/ui/skeletons";
import { intlLocaleFor } from "@/utils/intlLocale";
import { localDateKey } from "@/lib/localDate";
import {
  resolveWeekStartDay,
  weekStartKey,
  shiftWeekKey,
  weekDayKeys,
} from "@/utils/scheduleWeek";
import { useStaffRealtime } from "@/hooks/useStaffRealtime";
import { useToast } from "@/contexts/ToastContext";
import type { StaffData } from "@/utils/staffAuth";
import OfferCoverSheet, { type OfferCoverSheetLabels } from "./OfferCoverSheet";

// A swap/giveup is "still live" (occupying the shift) until it reaches a terminal
// state — those are the requests that should show as "Requested" and suppress a
// second offer on the same shift.
const TERMINAL_SWAP = new Set(["approved", "denied", "cancelled"]);

export interface MyScheduleViewLabels {
  title: string;
  subtitle: string;
  loading: string;
  error: string;
  emptyTitle: string;
  emptySubtitle: string;
  hoursUnit: string; // "h"
  minutesUnit: string; // "m"
  breakTemplate: string; // "{minutes}m break"
  positionFallback: string; // "Shift"
  offerAction: string; // row button — opens the hand-off sheet
  requested: string; // chip shown once a shift already has a live request
  thisWeek: string; // week toggle — current week
  nextWeek: string; // week toggle — next week (published schedule preview)
  offerSuccess: string; // toast after "offer for cover"
  giveUpSuccess: string; // toast after "give up"
  actionError: string; // toast on failure
  offer: OfferCoverSheetLabels;
}

export interface MyScheduleViewProps {
  staff: StaffData;
  labels: MyScheduleViewLabels;
  /** Display locale for Intl date/time formatting (not a translated string). */
  locale: string;
}

interface DayGroup {
  dateKey: string;
  shifts: Shift[];
}

// Net worked minutes = gross shift span minus the unpaid break. NO money — hours
// only on any staff-facing surface.
function netMinutes(shift: Shift): number {
  const start = new Date(shift.starts_at).getTime();
  const end = new Date(shift.ends_at).getTime();
  if (Number.isNaN(start) || Number.isNaN(end) || end <= start) return 0;
  const gross = Math.round((end - start) / 60000);
  return Math.max(0, gross - Math.max(0, shift.break_minutes || 0));
}

function formatDuration(shift: Shift, labels: MyScheduleViewLabels): string {
  const mins = netMinutes(shift);
  const h = Math.floor(mins / 60);
  const m = mins % 60;
  const parts: string[] = [];
  if (h > 0) parts.push(`${h}${labels.hoursUnit}`);
  if (m > 0 || h === 0) parts.push(`${m}${labels.minutesUnit}`);
  return parts.join(" ");
}

function formatTimeRange(shift: Shift, locale: string): string {
  const fmt = new Intl.DateTimeFormat(intlLocaleFor(locale), {
    hour: "numeric",
    minute: "2-digit",
  });
  return `${fmt.format(new Date(shift.starts_at))} – ${fmt.format(new Date(shift.ends_at))}`;
}

function formatDayHeading(dateKey: string, locale: string): string {
  const [y, m, d] = dateKey.split("-").map(Number);
  return new Intl.DateTimeFormat(intlLocaleFor(locale), {
    weekday: "long",
    month: "short",
    day: "numeric",
  }).format(new Date(y, m - 1, d));
}

export default function MyScheduleView({ staff, labels, locale }: MyScheduleViewProps) {
  const businessId = String(staff.business_id);
  const queryClient = useQueryClient();
  const toast = useToast();

  // This week / next week — staff can peek at next week's published schedule
  // once it's posted, not just the current week.
  const [weekOffset, setWeekOffset] = useState<0 | 1>(0);

  // Week-start day drives which YYYY-MM-DD key the schedule row is filed under.
  // It must match the operator builder's key exactly (see utils/scheduleWeek).
  const settingsQuery = useQuery({
    queryKey: ["schedule", businessId, "settings"],
    queryFn: () => scheduleSettingsApi.get(businessId),
    staleTime: 5 * 60 * 1000,
    retry: false,
  });
  // A failed settings fetch silently falls back to Monday, which files the week
  // under the wrong key and shows an empty/wrong schedule. Hold the schedule
  // read until settings resolve, and surface the error rather than fake "no
  // shifts".
  const settingsReady = settingsQuery.isSuccess;
  const weekStartDay = resolveWeekStartDay(settingsQuery.data?.week_start_day);
  const weekKey = useMemo(
    () => shiftWeekKey(weekStartKey(new Date(), weekStartDay), weekOffset),
    [weekStartDay, weekOffset],
  );
  const weekQueryKey = queryKeys.schedule.week(businessId, weekKey);

  const scheduleQuery = useQuery({
    queryKey: weekQueryKey,
    queryFn: () => scheduleApi.get(businessId, weekKey),
    enabled: settingsReady,
  });

  const positionsQuery = useQuery({
    queryKey: ["positions", businessId],
    queryFn: () => positionsApi.list(businessId),
    staleTime: 5 * 60 * 1000,
  });

  // My own coverage requests (shared cache with CoverageSection) — used to mark a
  // shift "Requested" and suppress a duplicate offer. Best-effort: a failure just
  // means the offer action stays available (the backend is the real guard).
  const mineKey = queryKeys.coverage.mine(businessId, staff.id);
  const mineQuery = useQuery({
    queryKey: mineKey,
    queryFn: () => coverageApi.listMine(businessId),
    retry: false,
  });

  // Live-patch: any scheduling event for this business refetches the open week
  // so a freshly published/assigned/moved shift appears without a manual reload.
  // Coverage events refresh the "Requested" markers.
  const invalidateCoverage = useCallback(() => {
    void queryClient.invalidateQueries({ queryKey: mineKey });
    void queryClient.invalidateQueries({ queryKey: queryKeys.coverage.open(businessId) });
  }, [queryClient, mineKey, businessId]);

  useStaffRealtime({
    businessId: staff.business_id,
    onSchedulePublished: () => queryClient.invalidateQueries({ queryKey: weekQueryKey }),
    onShiftAssigned: () => queryClient.invalidateQueries({ queryKey: weekQueryKey }),
    onShiftUpdated: () => queryClient.invalidateQueries({ queryKey: weekQueryKey }),
    onSwapRequested: invalidateCoverage,
    onSwapDecided: invalidateCoverage,
    // Events emitted during an SSE gap were never delivered — resync the week
    // grid and the coverage markers.
    onReconnect: () => {
      void queryClient.invalidateQueries({ queryKey: weekQueryKey });
      invalidateCoverage();
    },
  });

  // Shift ids with a still-live (non-terminal) swap/giveup I requested.
  const requestedShiftIds = useMemo(() => {
    const set = new Set<number>();
    (mineQuery.data?.swaps ?? []).forEach((s) => {
      if (!TERMINAL_SWAP.has(s.status)) set.add(s.shift_id);
    });
    return set;
  }, [mineQuery.data]);

  const [sheetShift, setSheetShift] = useState<Shift | null>(null);

  const requestMutation = useMutation({
    mutationFn: (vars: { shiftId: number; kind: SwapKind }) =>
      coverageApi.requestSwap(businessId, vars.shiftId, {
        kind: vars.kind,
        target: "all_in_role",
      }),
    onSuccess: (_data, vars) => {
      toast.showSuccess(vars.kind === "giveup" ? labels.giveUpSuccess : labels.offerSuccess);
      setSheetShift(null);
    },
    onError: () => toast.showError(labels.actionError),
    onSettled: () => invalidateCoverage(),
  });

  const positionsById = useMemo(() => {
    const map = new Map<number, Position>();
    (positionsQuery.data ?? []).forEach((p) => map.set(p.id, p));
    return map;
  }, [positionsQuery.data]);

  // Only the staff member's OWN assigned shifts — open (unassigned) shifts are
  // coverage opportunities surfaced elsewhere, not "your week".
  const dayGroups = useMemo<DayGroup[]>(() => {
    const own = (scheduleQuery.data?.shifts ?? [])
      .filter((s) => s.staff_id === staff.id)
      .slice()
      .sort((a, b) => new Date(a.starts_at).getTime() - new Date(b.starts_at).getTime());
    const byDay = new Map<string, Shift[]>();
    for (const shift of own) {
      const key = localDateKey(new Date(shift.starts_at));
      const bucket = byDay.get(key);
      if (bucket) bucket.push(shift);
      else byDay.set(key, [shift]);
    }
    return [...byDay.entries()].map(([dateKey, shifts]) => ({ dateKey, shifts }));
  }, [scheduleQuery.data, staff.id]);

  const weekRange = useMemo(() => {
    const days = weekDayKeys(weekKey);
    const fmt = new Intl.DateTimeFormat(intlLocaleFor(locale), {
      month: "short",
      day: "numeric",
    });
    const toDate = (key: string) => {
      const [y, m, d] = key.split("-").map(Number);
      return new Date(y, m - 1, d);
    };
    return `${fmt.format(toDate(days[0]))} – ${fmt.format(toDate(days[6]))}`;
  }, [weekKey, locale]);

  const isError = settingsQuery.isError || scheduleQuery.isError;
  // scheduleQuery is gated on settingsReady; while settings loads/errors it
  // reports isLoading=true (pending + disabled), so drop it from the loading
  // signal until settings land and let the error branch win.
  const isLoading =
    !isError &&
    (settingsQuery.isLoading || (settingsReady && scheduleQuery.isLoading));

  return (
    <section className="space-y-4" aria-label={labels.title}>
      <header className="flex flex-wrap items-start justify-between gap-3">
        <div>
          <h2 className="font-title text-base text-gray-900">{labels.title}</h2>
          <p className="text-sm text-gray-500">
            {labels.subtitle} · {weekRange}
          </p>
        </div>
        <div
          className="inline-flex shrink-0 rounded-full border border-gray-200 p-0.5"
          role="group"
          aria-label={labels.title}
        >
          {([0, 1] as const).map((offset) => {
            const active = weekOffset === offset;
            return (
              <button
                key={offset}
                type="button"
                onClick={() => setWeekOffset(offset)}
                aria-pressed={active}
                className={`rounded-full px-3 py-1 text-xs font-semibold transition ${
                  active
                    ? "bg-brand text-white shadow-sm"
                    : "text-gray-500 hover:text-gray-800"
                }`}
              >
                {offset === 0 ? labels.thisWeek : labels.nextWeek}
              </button>
            );
          })}
        </div>
      </header>

      {isLoading ? (
        <div
          role="status"
          aria-live="polite"
          aria-label={labels.loading}
          className="space-y-3"
        >
          {[0, 1].map((i) => (
            <div
              key={i}
              className="rounded-xl border border-warm-200 p-4 motion-safe:animate-pulse"
            >
              <SkeletonLine width="35%" height="0.875rem" />
              <div className="mt-3 space-y-3">
                {[0, 1].map((j) => (
                  <div key={j} className="flex items-center justify-between gap-3">
                    <div className="min-w-0 flex-1 space-y-1.5">
                      <SkeletonLine width="45%" height="0.875rem" />
                      <SkeletonLine width="30%" height="0.75rem" />
                    </div>
                    <SkeletonLine width="4rem" height="1.75rem" className="shrink-0 rounded-lg" />
                  </div>
                ))}
              </div>
            </div>
          ))}
        </div>
      ) : isError ? (
        <div className="rounded-xl border border-gray-200 p-4">
          <p className="text-sm text-gray-600">{labels.error}</p>
        </div>
      ) : dayGroups.length === 0 ? (
        <div className="rounded-xl border border-gray-200">
          <EmptyState
            icon={CalendarX2}
            title={labels.emptyTitle}
            subtitle={labels.emptySubtitle}
          />
        </div>
      ) : (
        <ul className="space-y-3">
          {dayGroups.map((group) => (
            <li key={group.dateKey} className="rounded-xl border border-gray-200 p-4">
              <h3 className="text-sm font-semibold capitalize text-gray-900">
                {formatDayHeading(group.dateKey, locale)}
              </h3>
              <ul className="mt-3 space-y-3">
                {group.shifts.map((shift) => {
                  const position = positionsById.get(shift.position_id);
                  const isFuture = new Date(shift.starts_at).getTime() > Date.now();
                  const alreadyRequested = requestedShiftIds.has(shift.id);
                  return (
                    <li
                      key={shift.id}
                      className="flex items-center justify-between gap-3 border-t border-gray-100 pt-3 first:border-0 first:pt-0"
                    >
                      <div className="min-w-0">
                        <p className="text-sm font-medium text-gray-900">
                          {formatTimeRange(shift, locale)}
                        </p>
                        <div className="mt-1 flex flex-wrap items-center gap-2">
                          <span className="inline-flex items-center gap-1.5 rounded-full bg-gray-100 px-2.5 py-1 text-xs font-medium text-gray-700">
                            <span
                              className="h-2 w-2 rounded-full bg-brand-dark"
                              style={
                                position?.color_hex
                                  ? { backgroundColor: position.color_hex }
                                  : undefined
                              }
                              aria-hidden="true"
                            />
                            {position?.name || labels.positionFallback}
                          </span>
                          {shift.break_minutes > 0 ? (
                            <span className="text-xs text-gray-400">
                              {labels.breakTemplate.replace(
                                "{minutes}",
                                String(shift.break_minutes),
                              )}
                            </span>
                          ) : null}
                        </div>
                      </div>
                      <div className="flex shrink-0 flex-col items-end gap-1.5">
                        <span className="rounded-lg bg-brand-50 px-2.5 py-1 text-sm font-semibold text-brand">
                          {formatDuration(shift, labels)}
                        </span>
                        {isFuture && alreadyRequested ? (
                          <span className="text-xs font-medium text-amber-600">
                            {labels.requested}
                          </span>
                        ) : isFuture ? (
                          <Button
                            size="sm"
                            variant="light"
                            className="h-auto min-w-0 px-2 py-0.5 text-xs text-gray-500"
                            onPress={() => setSheetShift(shift)}
                          >
                            {labels.offerAction}
                          </Button>
                        ) : null}
                      </div>
                    </li>
                  );
                })}
              </ul>
            </li>
          ))}
        </ul>
      )}

      {sheetShift ? (
        <OfferCoverSheet
          shiftLabel={`${formatDayHeading(localDateKey(new Date(sheetShift.starts_at)), locale)} · ${formatTimeRange(sheetShift, locale)}`}
          busy={requestMutation.isPending}
          onOffer={() => requestMutation.mutate({ shiftId: sheetShift.id, kind: "swap" })}
          onGiveUp={() => requestMutation.mutate({ shiftId: sheetShift.id, kind: "giveup" })}
          onClose={() => setSheetShift(null)}
          labels={labels.offer}
        />
      ) : null}
    </section>
  );
}
