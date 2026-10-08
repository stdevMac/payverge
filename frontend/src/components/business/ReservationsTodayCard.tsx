"use client";

import React, { useEffect, useState } from "react";
import { Card, CardBody } from "@nextui-org/react";
import { CalendarClock } from "lucide-react";
import {
  useSimpleLocale,
  getTranslation,
} from "@/i18n/SimpleTranslationProvider";
import { reservationAPI } from "@/api/reservations";
import { localDateKey } from "@/lib/localDate";
import {
  DATE_MONTH_DAY,
  TIME_SHORT,
  businessDateKey,
  formatBusinessDateTime,
  formatBusinessTime,
  isBusinessLocalToday,
} from "@/utils/businessTime";
import { nextUpcomingArrival } from "./reservationFormHelpers";
import StaleWidgetBanner from "./shared/StaleWidgetBanner";
import { loadReservationsToday } from "./overview/loadReservationsToday";

interface ReservationsTodayCardProps {
  businessId: number;
  /**
   * IANA business timezone. Next-arrival time renders in the business day, not
   * the operator's device timezone — matching the Reservations tab. Falls back
   * to the device timezone when unknown.
   */
  businessTimezone?: string | null;
  /** Bump to force a refetch (wired to the overview's refresh button). */
  refreshKey?: number;
  /** Navigate to the Reservations tab. */
  onNavigate: () => void;
}

/**
 * Compact "Today's reservations" stat card for the overview tab.
 *
 * Counts come from the server stats aggregate (exact totals, no 100-row cap).
 * Next arrival uses the shared `nextUpcomingArrival` predicate (pending /
 * confirmed / waitlist, still in the future) on a small bounded upcoming fetch.
 * Multi-day upcoming is kept: when the next arrival is not business-local
 * today, the label includes the date so Overview never contradicts the
 * Reservations "today" insight with a bare time [L1-10].
 */
export default function ReservationsTodayCard({
  businessId,
  businessTimezone,
  refreshKey = 0,
  onNavigate,
}: ReservationsTodayCardProps) {
  const { locale } = useSimpleLocale();

  const t = (
    key: string,
    params?: Record<string, string | number>,
  ): string => {
    const result = getTranslation(
      `businessDashboard.reservations.todayCard.${key}`,
      locale,
      params,
    );
    return Array.isArray(result) ? result[0] || key : (result as string);
  };

  const [todayCount, setTodayCount] = useState<number | null>(null);
  const [pendingCount, setPendingCount] = useState(0);
  const [nextArrival, setNextArrival] = useState<Date | null>(null);
  // L9-2 UX: distinguish "failed" from "zero reservations" — never fabricate 0.
  const [loadError, setLoadError] = useState(false);
  const [retryToken, setRetryToken] = useState(0);

  useEffect(() => {
    if (!businessId) return;
    const controller = new AbortController();

    // L1-10: Overview "today" must use the restaurant calendar day — same
    // key ReservationManager uses for the Reservas tab filter — not the
    // operator device day (localDateKey). Only fall back to device-local
    // when the business timezone is unknown.
    const today = businessTimezone
      ? businessDateKey(new Date(), businessTimezone) || localDateKey()
      : localDateKey();
    setLoadError(false);

    // L9-2: signal is threaded into reservationAPI axios calls (real cancel).
    void loadReservationsToday({
      businessId,
      today,
      signal: controller.signal,
      deps: {
        getStats: reservationAPI.getStats,
        getUpcomingReservations: reservationAPI.getUpcomingReservations,
        getReservations: reservationAPI.getReservations,
      },
    }).then((result) => {
      if (controller.signal.aborted || (!result.ok && result.aborted)) {
        return;
      }
      if (!result.ok) {
        // L9-2: do NOT fabricate todayCount=0 — that renders as real data.
        setLoadError(true);
        setTodayCount(null);
        setPendingCount(0);
        setNextArrival(null);
        return;
      }
      setTodayCount(result.todayCount);
      setPendingCount(result.pendingCount);
      const next = nextUpcomingArrival(
        (result.upcoming as Parameters<typeof nextUpcomingArrival>[0]) ?? [],
        new Date(),
      );
      setNextArrival(next ? new Date(next.reservation_time) : null);
      setLoadError(false);
    });

    return () => {
      controller.abort();
    };
  }, [businessId, businessTimezone, refreshKey, retryToken]);

  if (loadError) {
    return (
      <StaleWidgetBanner
        className="mb-0"
        onRetry={() => setRetryToken((n) => n + 1)}
      />
    );
  }

  // Still loading — render nothing rather than a skeleton flash.
  if (todayCount === null) return null;
  const nextArrivalLabel = (() => {
    if (!nextArrival) return t("noneUpcoming");
    const time = formatBusinessTime(
      nextArrival,
      locale,
      businessTimezone ?? null,
      TIME_SHORT,
    );
    if (isBusinessLocalToday(nextArrival, businessTimezone ?? null)) {
      return t("nextAt", { time });
    }
    // L8-2: named DATE_MONTH_DAY — never ad-hoc { month, day } at the call site.
    const date = formatBusinessDateTime(
      nextArrival,
      locale,
      businessTimezone ?? null,
      DATE_MONTH_DAY,
    );
    return t("nextOn", { date, time });
  })();

  return (
    <Card
      isPressable
      disableRipple
      shadow="none"
      onPress={onNavigate}
      className="w-full rounded-2xl border border-warm-200 bg-white transition-all hover:border-warm-300 hover:shadow-md focus:outline-none focus-visible:ring-2 focus-visible:ring-brand"
      data-testid="reservations-today-card"
    >
      <CardBody className="flex flex-row items-center gap-4 px-5 py-4">
        <span className="flex h-10 w-10 flex-shrink-0 items-center justify-center rounded-full bg-brand/10 text-brand">
          <CalendarClock className="h-5 w-5" />
        </span>
        <div className="flex min-w-0 flex-col text-left">
          <p className="text-body-sm font-semibold text-ink-900">
            {t("title")}: <span className="tabular-nums">{todayCount}</span>
            {pendingCount > 0 ? (
              <span className="text-amber-700">
                {" "}
                · <span className="tabular-nums">{pendingCount}</span>{" "}
                {t("pending")}
              </span>
            ) : null}
          </p>
          <p className="mt-0.5 text-body-sm text-ink-500">{nextArrivalLabel}</p>
        </div>
      </CardBody>
    </Card>
  );
}
