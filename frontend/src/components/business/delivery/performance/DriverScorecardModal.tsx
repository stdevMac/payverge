"use client";

import React, { useEffect, useState } from "react";
import {
  Modal,
  ModalContent,
  ModalHeader,
  ModalBody,
  ModalFooter,
  Button,
  Chip,
  Spinner,
} from "@nextui-org/react";
import { Clock, MapPin, Package, Star, TrendingUp, X } from "lucide-react";
import { deliveryApi, DriverPerformanceDto, DriverQueueItem } from "@/api/delivery";
import { formatCurrency as formatCurrencyIntl } from "@/api/currency";
import { useSimpleLocale } from "@/i18n/SimpleTranslationProvider";
import { intlLocaleFor } from "@/utils/intlLocale";

interface DriverScorecardModalProps {
  businessId: number;
  driverId: number;
  isOpen: boolean;
  onClose: () => void;
  tString: (key: string, vars?: Record<string, string>) => string;
  /** Business default currency (ISO 4217); falls back to USD. */
  currency?: string;
}

const formatMinutes = (m: number | null, suffix: string, fallback: string): string => {
  if (m == null) return fallback;
  if (m < 1) return `<1 ${suffix}`;
  return `${Math.round(m)} ${suffix}`;
};

const formatPercent = (r: number | null, fallback: string): string => {
  if (r == null) return fallback;
  return `${Math.round(r * 100)}%`;
};

const formatRating = (r: number | null, fallback: string): string => {
  if (r == null) return fallback;
  return r.toFixed(1);
};

// Replaced by per-call formatCurrencyIntl(value, currency) so AED/EUR
// businesses don't see a forced "$" on driver tip totals.

const formatTime = (
  iso: string | null | undefined,
  intlLocale: string,
): string => {
  if (!iso) return "—";
  const parsed = new Date(iso);
  if (Number.isNaN(parsed.getTime())) return "—";
  return parsed.toLocaleTimeString(intlLocale, {
    hour: "numeric",
    minute: "2-digit",
  });
};

interface KpiCardProps {
  label: string;
  value: string;
  hint?: string;
  icon?: React.ReactNode;
}

function KpiCard({ label, value, hint, icon }: KpiCardProps) {
  return (
    <div className="rounded-xl border border-warm-200 bg-white p-4">
      <div className="flex items-center gap-2 text-xs text-ink-500">
        {icon}
        <span>{label}</span>
      </div>
      <p className="mt-1 text-xl font-semibold text-ink-900">{value}</p>
      {hint ? <p className="mt-1 text-xs text-ink-400">{hint}</p> : null}
    </div>
  );
}

interface QueueItemRowProps {
  item: DriverQueueItem;
  tString: DriverScorecardModalProps["tString"];
  currency: string;
  intlLocale: string;
}

function QueueItemRow({
  item,
  tString,
  currency,
  intlLocale,
}: QueueItemRowProps) {
  return (
    <div className="rounded-xl border border-warm-200 p-3">
      <div className="flex items-center justify-between gap-2">
        <div className="min-w-0">
          <p className="truncate font-medium">{item.delivery_number}</p>
          <p className="truncate text-xs text-ink-500">{item.customer_name}</p>
        </div>
        <Chip size="sm" variant="flat">
          {tString(`dispatch.filters.${item.status}`) || item.status}
        </Chip>
      </div>
      <div className="mt-2 flex flex-wrap items-center gap-x-4 gap-y-1 text-xs text-ink-600">
        <span className="flex items-center gap-1">
          <MapPin className="h-3 w-3" />
          {item.address_short || "—"}
        </span>
        <span className="flex items-center gap-1">
          <Clock className="h-3 w-3" />
          {tString("performance.queue.eta")}:{" "}
          {formatTime(item.estimated_delivery_time, intlLocale)}
        </span>
        <span>
          {tString("performance.queue.fee")}: {formatCurrencyIntl(item.delivery_fee, currency)}
        </span>
        {item.driver_tip > 0 ? (
          <span>
            {tString("performance.queue.tip")}: {formatCurrencyIntl(item.driver_tip, currency)}
          </span>
        ) : null}
      </div>
    </div>
  );
}

export default function DriverScorecardModal({
  businessId,
  driverId,
  isOpen,
  onClose,
  tString,
  currency = "USD",
}: DriverScorecardModalProps) {
  const { locale } = useSimpleLocale();
  const intlLocale = intlLocaleFor(locale);
  const [data, setData] = useState<DriverPerformanceDto | null>(null);
  const [loading, setLoading] = useState(true);
  const [error, setError] = useState("");

  useEffect(() => {
    let cancelled = false;
    setLoading(true);
    setError("");
    deliveryApi
      .getDriverPerformance(businessId, driverId)
      .then((dto) => {
        if (!cancelled) setData(dto);
      })
      .catch(() => {
        if (!cancelled) setError(tString("performance.errors.loadDetail"));
      })
      .finally(() => {
        if (!cancelled) setLoading(false);
      });
    return () => {
      cancelled = true;
    };
  }, [businessId, driverId, tString]);

  const noData = tString("performance.placeholders.noData");
  const min = tString("performance.units.min");

  return (
    <Modal
      isOpen={isOpen}
      onClose={onClose}
      size="3xl"
      scrollBehavior="inside"
      // L3-44(b): custom header X + footer Cerrar — hide the default close.
      hideCloseButton
    >
      <ModalContent>
        {() => (
          <>
            <ModalHeader className="flex items-center justify-between gap-2 pr-2">
              <div className="min-w-0">
                <p className="text-base font-semibold">
                  {data?.driver_name ?? tString("performance.scorecard.title")}
                </p>
                <p className="text-xs text-ink-500">
                  {tString("performance.scorecard.subtitle")}
                </p>
              </div>
              <Button
                isIconOnly
                size="sm"
                variant="light"
                onPress={onClose}
                aria-label={tString("performance.actions.close")}
              >
                <X className="h-4 w-4" />
              </Button>
            </ModalHeader>
            <ModalBody>
              {loading ? (
                <div className="flex justify-center py-10">
                  <Spinner data-testid="scorecard-spinner" />
                </div>
              ) : error || !data ? (
                <p className="py-10 text-center text-sm text-rose-600">
                  {error || tString("performance.errors.loadDetail")}
                </p>
              ) : (
                <div className="space-y-6" data-testid="scorecard-body">
                  <div className="grid grid-cols-2 gap-3 sm:grid-cols-4">
                    <KpiCard
                      icon={<Package className="h-4 w-4" />}
                      label={tString("performance.kpi.completedToday")}
                      value={String(data.completed_today)}
                    />
                    <KpiCard
                      icon={<Package className="h-4 w-4" />}
                      label={tString("performance.kpi.completedWeek")}
                      value={String(data.completed_week)}
                    />
                    <KpiCard
                      icon={<Package className="h-4 w-4" />}
                      label={tString("performance.kpi.completedAllTime")}
                      value={String(data.completed_all_time)}
                    />
                    <KpiCard
                      icon={<Package className="h-4 w-4" />}
                      label={tString("performance.kpi.cancelled")}
                      value={String(data.cancelled_count)}
                    />
                    {data.failed_count > 0 ? (
                      <KpiCard
                        icon={<Package className="h-4 w-4" />}
                        label={tString("performance.kpi.failed")}
                        value={String(data.failed_count)}
                      />
                    ) : null}
                  </div>

                  <div className="grid grid-cols-2 gap-3 sm:grid-cols-4">
                    <KpiCard
                      icon={<Clock className="h-4 w-4" />}
                      label={tString("performance.kpi.avgPickup")}
                      value={formatMinutes(data.avg_pickup_minutes, min, noData)}
                    />
                    <KpiCard
                      icon={<Clock className="h-4 w-4" />}
                      label={tString("performance.kpi.avgDelivery")}
                      value={formatMinutes(data.avg_delivery_minutes, min, noData)}
                    />
                    <KpiCard
                      icon={<TrendingUp className="h-4 w-4" />}
                      label={tString("performance.kpi.onTime")}
                      value={formatPercent(data.on_time_rate, noData)}
                    />
                    <KpiCard
                      icon={<Star className="h-4 w-4" />}
                      label={tString("performance.kpi.rating")}
                      value={formatRating(data.average_rating, noData)}
                    />
                  </div>

                  <div className="grid grid-cols-1 gap-3 sm:grid-cols-2">
                    <KpiCard
                      label={tString("performance.kpi.grossFees")}
                      value={formatCurrencyIntl(data.gross_fees_collected, currency)}
                      hint={tString("performance.kpi.readOnly")}
                    />
                    <KpiCard
                      label={tString("performance.kpi.grossTips")}
                      value={formatCurrencyIntl(data.gross_tips_collected, currency)}
                      hint={tString("performance.kpi.readOnly")}
                    />
                  </div>

                  <div>
                    <h3 className="mb-2 text-sm font-semibold">
                      {tString("performance.queue.title")}{" "}
                      <span className="text-ink-500">({data.active_queue.length})</span>
                    </h3>
                    {data.active_queue.length === 0 ? (
                      <p className="text-sm text-ink-500">
                        {tString("performance.queue.empty")}
                      </p>
                    ) : (
                      <div className="space-y-2" data-testid="scorecard-queue">
                        {data.active_queue.map((item) => (
                          <QueueItemRow
                            key={item.delivery_id}
                            item={item}
                            tString={tString}
                            currency={currency}
                            intlLocale={intlLocale}
                          />
                        ))}
                      </div>
                    )}
                  </div>
                </div>
              )}
            </ModalBody>
            <ModalFooter>
              <Button variant="light" onPress={onClose}>
                {tString("performance.actions.close")}
              </Button>
            </ModalFooter>
          </>
        )}
      </ModalContent>
    </Modal>
  );
}
