"use client";

import React, { useCallback, useEffect, useMemo, useState } from "react";
import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query";
import { Button } from "@nextui-org/react";
import { SkeletonLine } from "@/components/ui/skeletons";
import {
  ArrowLeftRight,
  CalendarClock,
  Check,
  ChevronRight,
  ClipboardList,
  Megaphone,
} from "lucide-react";
import { scheduleApi, type Shift } from "@/api/schedule";
import { scheduleSettingsApi } from "@/api/scheduleSettings";
import { positionsApi, type Position } from "@/api/positions";
import { coverageApi } from "@/api/coverage";
import { availabilityApi } from "@/api/availability";
import { checklistsApi } from "@/api/engagement";
import { chatApi } from "@/api/chat";
import { queryKeys } from "@/api/queryKeys";
import { intlLocaleFor } from "@/utils/intlLocale";
import { localDateKey } from "@/lib/localDate";
import {
  resolveWeekStartDay,
  weekStartKey,
  shiftWeekKey,
} from "@/utils/scheduleWeek";
import { useStaffRealtime } from "@/hooks/useStaffRealtime";
import { useToast } from "@/contexts/ToastContext";
import type { StaffData } from "@/utils/staffAuth";
import TodayCard, { type TodayCardLabels } from "./TodayCard";
import { type Section } from "./StaffMore";
import {
  calendarDayDelta,
  incompleteRunCount,
  isInProgress,
  latestAnnouncement,
  minutesUntilStart,
  pendingRequestSummary,
  pickNextShift,
  todayChecklistRuns,
  upcomingOwnShifts,
} from "./todayHome";

export interface StaffTodayHomeLabels {
  // Next shift
  nextShiftTitle: string;
  onShiftNow: string;
  startsInMinutes: string; // "Starts in {minutes}m"
  startsInHours: string; // "Starts in {hours}h {minutes}m"
  startsTomorrow: string; // "Tomorrow at {time}"
  noNextShiftTitle: string;
  noNextShiftHint: string;
  moreThisWeek: string; // "{count} more coming up"
  seeSchedule: string;
  positionFallback: string;
  loading: string;
  // Open shifts
  openShiftsTitle: string;
  claim: string;
  claiming: string;
  claimSuccess: string;
  seeAllOpen: string;
  // Pending requests
  pendingTitle: string;
  pendingCoverage: string; // "{count} coverage"
  pendingTimeOff: string; // "{count} time off"
  manageRequests: string;
  // Today's checklist
  checklistTitle: string;
  checklistRemaining: string; // "{count} left to finish"
  checklistDone: string;
  openChecklist: string;
  // Latest announcement
  announcementTitle: string;
  acknowledge: string;
  acknowledging: string;
  acknowledged: string;
  ackSuccess: string;
  allAnnouncements: string;
  // Shared
  actionError: string;
}

export interface StaffTodayHomeProps {
  staff: StaffData;
  /** Display locale for Intl date/time formatting (not a translated string). */
  locale: string;
  labels: StaffTodayHomeLabels;
  /** Clock strings, forwarded to the embedded TodayCard. */
  clockLabels: TodayCardLabels;
  /** Jump to the top-level Schedule tab. */
  onOpenSchedule: () => void;
  /** Open a "More" sub-surface (coverage / checklists / announcements). */
  onOpenSection: (section: Section) => void;
}

const MAX_OPEN_SHOWN = 3;
const MAX_UPCOMING_SHOWN = 2;

export default function StaffTodayHome({
  staff,
  locale,
  labels,
  clockLabels,
  onOpenSchedule,
  onOpenSection,
}: StaffTodayHomeProps) {
  const businessId = String(staff.business_id);
  const queryClient = useQueryClient();
  const toast = useToast();
  const intlLocale = intlLocaleFor(locale);

  // Coarse 60s tick — enough to keep the "starts in Xh Ym" countdown honest
  // without a per-second render on a screen that isn't a stopwatch.
  const [now, setNow] = useState(() => Date.now());
  useEffect(() => {
    const id = window.setInterval(() => setNow(Date.now()), 60_000);
    return () => window.clearInterval(id);
  }, []);

  const settingsQuery = useQuery({
    queryKey: ["schedule", businessId, "settings"],
    queryFn: () => scheduleSettingsApi.get(businessId),
    staleTime: 5 * 60 * 1000,
    retry: false,
  });
  const weekStartDay = resolveWeekStartDay(settingsQuery.data?.week_start_day);
  // This week + next week cover the "next shift" horizon; a shift further out
  // isn't next-shift urgency. Keys match the operator builder + MyScheduleView
  // exactly so the cache is shared, not duplicated.
  const thisWeekKey = useMemo(
    () => weekStartKey(new Date(), weekStartDay),
    [weekStartDay],
  );
  const nextWeekKey = useMemo(() => shiftWeekKey(thisWeekKey, 1), [thisWeekKey]);
  const thisWeekQueryKey = queryKeys.schedule.week(businessId, thisWeekKey);
  const nextWeekQueryKey = queryKeys.schedule.week(businessId, nextWeekKey);

  const thisWeekQuery = useQuery({
    queryKey: thisWeekQueryKey,
    queryFn: () => scheduleApi.get(businessId, thisWeekKey),
    enabled: !settingsQuery.isLoading,
  });
  const nextWeekQuery = useQuery({
    queryKey: nextWeekQueryKey,
    queryFn: () => scheduleApi.get(businessId, nextWeekKey),
    enabled: !settingsQuery.isLoading,
  });
  const openKey = queryKeys.coverage.open(businessId);
  const mineKey = queryKeys.coverage.mine(businessId, staff.id);
  const openQuery = useQuery({
    queryKey: openKey,
    queryFn: () => coverageApi.listOpen(businessId),
  });
  const mineQuery = useQuery({
    queryKey: mineKey,
    queryFn: () => coverageApi.listMine(businessId),
  });
  const timeOffQuery = useQuery({
    queryKey: queryKeys.timeOff.list(businessId, "pending"),
    queryFn: () => availabilityApi.listTimeOff(businessId, "pending"),
  });
  const runsQuery = useQuery({
    queryKey: queryKeys.engagement.checklistRuns(businessId),
    queryFn: () => checklistsApi.listRuns(businessId),
  });
  const announcementsKey = queryKeys.chat.announcements(businessId);
  const announcementsQuery = useQuery({
    queryKey: announcementsKey,
    queryFn: () => chatApi.listAnnouncements(businessId),
  });
  const positionsQuery = useQuery({
    queryKey: ["positions", businessId],
    queryFn: () => positionsApi.list(businessId),
    staleTime: 5 * 60 * 1000,
  });

  const invalidateSchedule = useCallback(() => {
    void queryClient.invalidateQueries({ queryKey: thisWeekQueryKey });
    void queryClient.invalidateQueries({ queryKey: nextWeekQueryKey });
  }, [queryClient, thisWeekQueryKey, nextWeekQueryKey]);
  const invalidateCoverage = useCallback(() => {
    void queryClient.invalidateQueries({ queryKey: openKey });
    void queryClient.invalidateQueries({ queryKey: mineKey });
  }, [queryClient, openKey, mineKey]);

  // Live-refresh the home the same way the dedicated surfaces do: a published /
  // moved shift or any coverage event changes what's shown here too.
  useStaffRealtime({
    businessId: staff.business_id,
    onSchedulePublished: invalidateSchedule,
    onShiftAssigned: invalidateSchedule,
    onShiftUpdated: invalidateSchedule,
    onOpenShiftClaimed: invalidateCoverage,
    onOpenShiftDecided: invalidateCoverage,
    onSwapRequested: invalidateCoverage,
    onSwapAccepted: invalidateCoverage,
    onSwapDecided: invalidateCoverage,
    // Events emitted during an SSE gap were never delivered — resync both feeds.
    onReconnect: () => {
      invalidateSchedule();
      invalidateCoverage();
    },
  });

  const positionsById = useMemo(() => {
    const map = new Map<number, Position>();
    (positionsQuery.data ?? []).forEach((p) => map.set(p.id, p));
    return map;
  }, [positionsQuery.data]);
  const positionName = useCallback(
    (id: number) => positionsById.get(id)?.name || labels.positionFallback,
    [positionsById, labels.positionFallback],
  );

  const allShifts = useMemo<Shift[]>(
    () => [
      ...(thisWeekQuery.data?.shifts ?? []),
      ...(nextWeekQuery.data?.shifts ?? []),
    ],
    [thisWeekQuery.data, nextWeekQuery.data],
  );
  const upcoming = useMemo(
    () => upcomingOwnShifts(allShifts, staff.id, now),
    [allShifts, staff.id, now],
  );
  const nextShift = useMemo(
    () => pickNextShift(allShifts, staff.id, now),
    [allShifts, staff.id, now],
  );

  const claimMutation = useMutation({
    mutationFn: (shiftId: number) => coverageApi.claim(businessId, shiftId),
    onSuccess: () => toast.showSuccess(labels.claimSuccess),
    onError: () => toast.showError(labels.actionError),
    onSettled: () => invalidateCoverage(),
  });
  const ackMutation = useMutation({
    mutationFn: (announcementId: number) =>
      chatApi.ackAnnouncement(businessId, announcementId),
    onSuccess: () => toast.showSuccess(labels.ackSuccess),
    onError: () => toast.showError(labels.actionError),
    onSettled: () =>
      queryClient.invalidateQueries({ queryKey: announcementsKey }),
  });

  const formatTime = useCallback(
    (iso: string) =>
      new Intl.DateTimeFormat(intlLocale, {
        hour: "numeric",
        minute: "2-digit",
      }).format(new Date(iso)),
    [intlLocale],
  );
  const formatRange = useCallback(
    (shift: Shift) => `${formatTime(shift.starts_at)} – ${formatTime(shift.ends_at)}`,
    [formatTime],
  );
  const formatDay = useCallback(
    (iso: string) =>
      new Intl.DateTimeFormat(intlLocale, {
        weekday: "long",
        month: "short",
        day: "numeric",
      }).format(new Date(iso)),
    [intlLocale],
  );

  // The single "when" phrase for the next-shift hero: in-progress > minute
  // countdown > hour countdown (today) > tomorrow > absolute day.
  const nextShiftWhen = useCallback(
    (shift: Shift): string => {
      if (isInProgress(shift, now)) return labels.onShiftNow;
      const startMs = new Date(shift.starts_at).getTime();
      const mins = minutesUntilStart(startMs, now);
      if (mins < 60) {
        return labels.startsInMinutes.replace("{minutes}", String(Math.max(0, mins)));
      }
      const delta = calendarDayDelta(startMs, now);
      if (delta === 0) {
        return labels.startsInHours
          .replace("{hours}", String(Math.floor(mins / 60)))
          .replace("{minutes}", String(mins % 60));
      }
      if (delta === 1) {
        return labels.startsTomorrow.replace("{time}", formatTime(shift.starts_at));
      }
      return formatDay(shift.starts_at);
    },
    [now, labels, formatTime, formatDay],
  );

  const scheduleLoading =
    settingsQuery.isLoading || thisWeekQuery.isLoading || nextWeekQuery.isLoading;

  const pending = pendingRequestSummary(mineQuery.data ?? null, timeOffQuery.data ?? []);
  const openShifts = openQuery.data?.open_shifts ?? [];
  const todayRuns = todayChecklistRuns(
    runsQuery.data ?? [],
    localDateKey(new Date(now)),
  );
  const remainingRuns = incompleteRunCount(todayRuns);
  const announcement = latestAnnouncement(announcementsQuery.data ?? null);

  const claimBusyId = claimMutation.isPending
    ? (claimMutation.variables ?? null)
    : null;

  return (
    <div className="space-y-4">
      {/* Clock + service tools — the hero. The #1 mid-shift action ("clock in /
          out, take a break") sits above the fold on phones. TodayCard is already
          state-aware: clocked-out surfaces a primary "Clock in"; clocked-in shows
          the running elapsed timer with break / clock-out controls. */}
      <TodayCard staff={staff} locale={locale} labels={clockLabels} />

      {/* Next shift — a secondary card now. Answers "when am I next in?" but no
          longer buries the clock-in action below the fold. Always present (with a
          friendly empty state). */}
      <section
        className="rounded-xl border border-gray-200 p-4"
        aria-label={labels.nextShiftTitle}
      >
        <h2 className="font-title text-base text-gray-900">{labels.nextShiftTitle}</h2>
        {scheduleLoading ? (
          <div role="status" aria-live="polite" aria-label={labels.loading} className="mt-3">
            <div className="space-y-2 rounded-lg bg-warm-100 px-4 py-3 motion-safe:animate-pulse">
              <SkeletonLine width="30%" height="0.75rem" />
              <SkeletonLine width="55%" height="1.25rem" />
              <SkeletonLine width="40%" height="0.75rem" />
            </div>
          </div>
        ) : nextShift ? (
          <div className="mt-3 space-y-3">
            <div className="rounded-lg bg-brand-50 px-4 py-3">
              <p className="text-[11px] font-semibold uppercase tracking-wide text-brand">
                {nextShiftWhen(nextShift)}
              </p>
              <p className="mt-1 text-lg font-semibold text-brand-900">
                {formatRange(nextShift)}
              </p>
              <div className="mt-1.5 flex flex-wrap items-center gap-2 text-xs text-brand-800">
                <span className="inline-flex items-center gap-1.5">
                  <span
                    className="h-2 w-2 rounded-full bg-brand-dark"
                    style={
                      positionsById.get(nextShift.position_id)?.color_hex
                        ? {
                            backgroundColor: positionsById.get(nextShift.position_id)
                              ?.color_hex,
                          }
                        : undefined
                    }
                    aria-hidden="true"
                  />
                  {positionName(nextShift.position_id)}
                </span>
                <span className="text-brand-500">· {formatDay(nextShift.starts_at)}</span>
              </div>
            </div>

            {upcoming.length > 1 ? (
              <ul className="space-y-1.5">
                {upcoming.slice(1, 1 + MAX_UPCOMING_SHOWN).map((shift) => (
                  <li
                    key={shift.id}
                    className="flex items-center justify-between gap-3 text-sm"
                  >
                    <span className="min-w-0 truncate text-gray-600">
                      {formatDay(shift.starts_at)}
                    </span>
                    <span className="shrink-0 font-medium text-gray-800">
                      {formatRange(shift)}
                    </span>
                  </li>
                ))}
              </ul>
            ) : null}

            <button
              type="button"
              onClick={onOpenSchedule}
              className="inline-flex items-center gap-1 text-sm font-medium text-brand transition hover:text-brand-800"
            >
              {upcoming.length - 1 > MAX_UPCOMING_SHOWN
                ? labels.moreThisWeek.replace(
                    "{count}",
                    String(upcoming.length - 1 - MAX_UPCOMING_SHOWN),
                  )
                : labels.seeSchedule}
              <ChevronRight className="h-4 w-4" aria-hidden="true" />
            </button>
          </div>
        ) : (
          <div className="mt-3 flex flex-col items-center gap-2 rounded-lg bg-gray-50 px-4 py-6 text-center">
            <CalendarClock className="h-6 w-6 text-gray-400" aria-hidden="true" />
            <p className="text-sm font-medium text-gray-600">{labels.noNextShiftTitle}</p>
            <p className="text-xs text-gray-400">{labels.noNextShiftHint}</p>
          </div>
        )}
      </section>

      {/* Open shifts to pick up — inline claim, glance-sized (up to 3). */}
      {openShifts.length > 0 ? (
        <GlanceCard
          icon={ArrowLeftRight}
          title={labels.openShiftsTitle}
          action={
            openShifts.length > MAX_OPEN_SHOWN
              ? { label: labels.seeAllOpen, onClick: () => onOpenSection("coverage") }
              : undefined
          }
        >
          <ul className="space-y-2">
            {openShifts.slice(0, MAX_OPEN_SHOWN).map((o) => (
              <li
                key={o.shift_id}
                className="flex items-center justify-between gap-3 rounded-lg border border-amber-200 bg-amber-50 px-3 py-2"
              >
                <div className="min-w-0">
                  <p className="truncate text-sm font-medium text-gray-900">
                    {positionName(o.position_id)}
                  </p>
                  <p className="truncate text-xs text-gray-500">
                    {formatDay(o.starts_at)} · {formatTime(o.starts_at)} –{" "}
                    {formatTime(o.ends_at)}
                  </p>
                </div>
                <Button
                  size="sm"
                  color="primary"
                  variant="flat"
                  isLoading={claimBusyId === o.shift_id}
                  isDisabled={claimMutation.isPending}
                  onPress={() => claimMutation.mutate(o.shift_id)}
                >
                  {claimBusyId === o.shift_id ? labels.claiming : labels.claim}
                </Button>
              </li>
            ))}
          </ul>
        </GlanceCard>
      ) : null}

      {/* Pending requests — a read-only glance; manage them in Coverage. */}
      {pending.total > 0 ? (
        <GlanceCard
          icon={ArrowLeftRight}
          title={labels.pendingTitle}
          action={{ label: labels.manageRequests, onClick: () => onOpenSection("coverage") }}
        >
          <div className="flex flex-wrap gap-2">
            {pending.coverage > 0 ? (
              <span className="rounded-full bg-gray-100 px-3 py-1 text-xs font-medium text-gray-700">
                {labels.pendingCoverage.replace("{count}", String(pending.coverage))}
              </span>
            ) : null}
            {pending.timeOff > 0 ? (
              <span className="rounded-full bg-gray-100 px-3 py-1 text-xs font-medium text-gray-700">
                {labels.pendingTimeOff.replace("{count}", String(pending.timeOff))}
              </span>
            ) : null}
          </div>
        </GlanceCard>
      ) : null}

      {/* Today's checklist — status glance; the ticker lives in the full view. */}
      {todayRuns.length > 0 ? (
        <GlanceCard
          icon={ClipboardList}
          title={labels.checklistTitle}
          action={{ label: labels.openChecklist, onClick: () => onOpenSection("checklists") }}
        >
          <p className="flex items-center gap-2 text-sm text-gray-700">
            {remainingRuns > 0 ? (
              labels.checklistRemaining.replace("{count}", String(remainingRuns))
            ) : (
              <span className="inline-flex items-center gap-1.5 text-brand">
                <Check className="h-4 w-4" aria-hidden="true" />
                {labels.checklistDone}
              </span>
            )}
          </p>
        </GlanceCard>
      ) : null}

      {/* Latest announcement — read inline, ack inline when required. */}
      {announcement ? (
        <GlanceCard
          icon={Megaphone}
          title={labels.announcementTitle}
          action={{ label: labels.allAnnouncements, onClick: () => onOpenSection("announcements") }}
        >
          <p className="text-sm font-medium text-gray-900">
            {announcement.announcement.title}
          </p>
          <p className="mt-0.5 line-clamp-2 text-sm text-gray-600">
            {announcement.announcement.content}
          </p>
          {announcement.announcement.require_ack ? (
            announcement.acked ? (
              <p className="mt-2 inline-flex items-center gap-1.5 text-xs font-medium text-brand">
                <Check className="h-3.5 w-3.5" aria-hidden="true" />
                {labels.acknowledged}
              </p>
            ) : (
              <Button
                size="sm"
                color="primary"
                variant="flat"
                className="mt-2"
                isLoading={ackMutation.isPending}
                onPress={() => ackMutation.mutate(announcement.announcement.id)}
              >
                {ackMutation.isPending ? labels.acknowledging : labels.acknowledge}
              </Button>
            )
          ) : null}
        </GlanceCard>
      ) : null}
    </div>
  );
}

function GlanceCard({
  icon: Icon,
  title,
  action,
  children,
}: {
  icon: typeof ArrowLeftRight;
  title: string;
  action?: { label: string; onClick: () => void };
  children: React.ReactNode;
}) {
  return (
    <section className="rounded-xl border border-gray-200 p-4" aria-label={title}>
      <div className="mb-3 flex items-center gap-2">
        <span
          className="flex h-8 w-8 shrink-0 items-center justify-center rounded-full bg-brand-50 text-brand"
          aria-hidden="true"
        >
          <Icon className="h-4 w-4" />
        </span>
        <h2 className="font-title text-base text-gray-900">{title}</h2>
      </div>
      {children}
      {action ? (
        <button
          type="button"
          onClick={action.onClick}
          className="mt-3 inline-flex items-center gap-1 text-sm font-medium text-brand transition hover:text-brand-800"
        >
          {action.label}
          <ChevronRight className="h-4 w-4" aria-hidden="true" />
        </button>
      ) : null}
    </section>
  );
}
