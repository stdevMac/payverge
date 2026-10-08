"use client";

import React, { useCallback, useMemo } from "react";
import { Button, Chip } from "@nextui-org/react";
import { CalendarClock, Check, Users, X } from "lucide-react";
import type { Reservation } from "@/api/reservations";
import {
  useSimpleLocale,
  getTranslation,
} from "@/i18n/SimpleTranslationProvider";
import {
  DATE_TIME_SHORT,
  formatBusinessDateTime,
} from "@/utils/businessTime";

/** Deadline mirror of backend ReservationApprovalDeadline (services/reservation_approval.go). */
export function approvalDeadline(
  createdAt: string,
  reservationTime: string,
): Date {
  const created = new Date(createdAt).getTime();
  const start = new Date(reservationTime).getTime();
  let deadline = start - 2 * 60 * 60 * 1000;
  deadline = Math.min(deadline, created + 24 * 60 * 60 * 1000);
  deadline = Math.max(deadline, created + 30 * 60 * 1000);
  return new Date(deadline);
}

interface GuestHistoryEntry {
  prior_no_shows: number;
  prior_visits: number;
}

interface ReservationApprovalQueueProps {
  pending: Reservation[];
  onApprove: (id: number) => void;
  onDecline: (id: number) => void;
  busyId: number | null;
  businessTimezone?: string | null;
  guestHistory?: Record<string, GuestHistoryEntry>;
  /** Server total for pending (may exceed the loaded page). */
  total?: number;
  /** Load another page when the list is truncated. */
  onLoadMore?: () => void;
  loadMoreLoading?: boolean;
}

/**
 * "Needs review" strip for manual-approval reservation requests.
 *
 * Renders above the reservations list independently of the list filters so a
 * pending request can never be hidden by a quick filter or date range. Rows
 * are sorted soonest-respond-by-deadline first (triage by urgency); the
 * backend auto-declines once the deadline passes.
 */
export default function ReservationApprovalQueue({
  pending,
  onApprove,
  onDecline,
  busyId,
  businessTimezone = null,
  guestHistory,
  total,
  onLoadMore,
  loadMoreLoading = false,
}: ReservationApprovalQueueProps) {
  const { locale } = useSimpleLocale();

  const t = useCallback(
    (key: string): string => {
      const fullKey = `businessDashboard.reservations.approvalQueue.${key}`;
      const result = getTranslation(fullKey, locale);
      return Array.isArray(result) ? result[0] || key : (result as string);
    },
    [locale],
  );

  const sorted = useMemo(
    () =>
      [...pending].sort(
        (a, b) =>
          approvalDeadline(a.created_at, a.reservation_time).getTime() -
          approvalDeadline(b.created_at, b.reservation_time).getTime(),
      ),
    [pending],
  );

  if (sorted.length === 0) {
    return null;
  }

  const serverTotal =
    typeof total === "number" && total > sorted.length ? total : sorted.length;
  const isTruncated = serverTotal > sorted.length;

  const formatDateTime = (value: Date) =>
    formatBusinessDateTime(value, locale, businessTimezone, DATE_TIME_SHORT);

  return (
    <div
      data-testid="reservation-approval-queue"
      className="rounded-2xl border border-amber-200 bg-amber-50/60 p-4 space-y-3"
    >
      <div className="flex flex-wrap items-center gap-2">
        <CalendarClock className="w-4 h-4 text-amber-700" />
        <h2 className="text-sm font-semibold text-amber-900">{t("title")}</h2>
        <Chip
          size="sm"
          classNames={{
            base: "bg-amber-100 border border-amber-300",
            content: "text-amber-900 font-medium",
          }}
        >
          {isTruncated ? serverTotal : sorted.length}
        </Chip>
        {isTruncated && (
          <span
            className="text-xs font-medium text-amber-800"
            data-testid="approval-queue-truncated"
          >
            {t("showingOf")
              .replace("{shown}", String(sorted.length))
              .replace("{total}", String(serverTotal))}
          </span>
        )}
      </div>

      <ul className="space-y-2">
        {sorted.map((reservation) => {
          const deadline = approvalDeadline(
            reservation.created_at,
            reservation.reservation_time,
          );
          const isBusy = busyId === reservation.id;

          return (
            <li
              key={reservation.id}
              className="flex flex-col sm:flex-row sm:items-center gap-3 rounded-xl border border-amber-200/80 bg-white px-4 py-3"
            >
              <div className="flex-1 min-w-0">
                <p className="text-sm font-medium text-ink-900 truncate">
                  {reservation.customer_name}
                </p>
                <p className="text-xs text-ink-700 mt-0.5 flex items-center gap-1.5">
                  <span>
                    {formatDateTime(new Date(reservation.reservation_time))}
                  </span>
                  <span aria-hidden="true">·</span>
                  <Users className="w-3 h-3 shrink-0" aria-hidden="true" />
                  <span>
                    {reservation.party_size} {t("guests")}
                  </span>
                </p>
                <p className="text-xs text-amber-700 mt-0.5">
                  {t("respondBy")} {formatDateTime(deadline)}
                </p>
                {(() => {
                  const history =
                    guestHistory?.[
                      (reservation.customer_email ?? "").toLowerCase()
                    ];
                  if (
                    !history ||
                    (history.prior_no_shows === 0 &&
                      history.prior_visits === 0)
                  ) {
                    return null;
                  }
                  return (
                    <p
                      className={`mt-0.5 text-xs ${
                        history.prior_no_shows > 0
                          ? "text-rose-700"
                          : "text-ink-600"
                      }`}
                    >
                      {
                        getTranslation(
                          "businessDashboard.reservations.approvalQueue.history.line",
                          locale,
                          {
                            noShows: history.prior_no_shows,
                            visits: history.prior_visits,
                          },
                        ) as string
                      }
                    </p>
                  );
                })()}
              </div>
              <div className="flex items-center gap-2 shrink-0">
                <Button
                  size="sm"
                  className="bg-brand text-white font-medium hover:bg-brand-dark"
                  startContent={
                    isBusy ? undefined : <Check className="w-4 h-4" />
                  }
                  isLoading={isBusy}
                  isDisabled={busyId !== null && !isBusy}
                  onPress={() => onApprove(reservation.id)}
                >
                  {t("approve")}
                </Button>
                <Button
                  size="sm"
                  variant="flat"
                  color="danger"
                  startContent={<X className="w-4 h-4" />}
                  isDisabled={busyId !== null}
                  onPress={() => onDecline(reservation.id)}
                >
                  {t("decline")}
                </Button>
              </div>
            </li>
          );
        })}
      </ul>

      {isTruncated && onLoadMore && (
        <div className="flex justify-center pt-1">
          <Button
            size="sm"
            variant="flat"
            className="border border-amber-300 bg-white text-amber-900"
            isLoading={loadMoreLoading}
            onPress={onLoadMore}
            data-testid="approval-queue-load-more"
          >
            {t("loadMore")}
          </Button>
        </div>
      )}
    </div>
  );
}
