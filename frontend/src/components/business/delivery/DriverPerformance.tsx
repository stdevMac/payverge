"use client";

import React, { useCallback, useEffect, useMemo, useState } from "react";
import {
  Button,
  Card,
  CardBody,
  Chip,
  Skeleton,
  Table,
  TableBody,
  TableCell,
  TableColumn,
  TableHeader,
  TableRow,
} from "@nextui-org/react";
import { RefreshCcw, TrendingUp, Eye } from "lucide-react";
import { deliveryApi, DriverPerformanceDto } from "@/api/delivery";
import { useDeliveryBusiness } from "./useDeliveryQueries";
import { useSimpleLocale, getTranslation } from "@/i18n/SimpleTranslationProvider";
import { useToast } from "@/contexts/ToastContext";
import DriverScorecardModal from "./performance/DriverScorecardModal";

interface DriverPerformanceProps {
  businessId: number;
}

const POLLING_INTERVAL_MS = 30_000;

export default function DriverPerformance({ businessId }: DriverPerformanceProps) {
  const { locale } = useSimpleLocale();
  const { showError } = useToast();

  const tString = useCallback(
    (key: string, vars?: Record<string, string>): string => {
      const result = getTranslation(`deliverySettings.${key}`, locale);
      let val = Array.isArray(result) ? result[0] || key : (result as string);
      if (typeof val !== "string") return key;
      if (vars) {
        Object.entries(vars).forEach(([k, v]) => {
          val = (val as string).replace(`{${k}}`, v);
        });
      }
      return val as string;
    },
    [locale],
  );

  const [drivers, setDrivers] = useState<DriverPerformanceDto[]>([]);
  const [loading, setLoading] = useState(true);
  const [refreshing, setRefreshing] = useState(false);
  const { data: business } = useDeliveryBusiness(businessId);
  const businessCurrency = business?.default_currency || "USD";
  const [selectedDriverId, setSelectedDriverId] = useState<number | null>(null);

  const load = useCallback(
    async (background = false) => {
      try {
        if (background) {
          setRefreshing(true);
        } else {
          setLoading(true);
        }
        const result = await deliveryApi.listDriverPerformance(businessId);
        setDrivers(result.drivers);
      } catch {
        showError(tString("performance.toasts.loadError"));
      } finally {
        setLoading(false);
        setRefreshing(false);
      }
    },
    [businessId, showError, tString],
  );

  useEffect(() => {
    void load();
  }, [load]);

  // Auto-refresh while at least one driver has live work.
  const hasLiveWork = useMemo(
    () => drivers.some((d) => d.in_progress_count > 0),
    [drivers],
  );

  // Match DispatchConsole poll hygiene: pause while the tab is hidden so a
  // background dashboard does not keep re-running the performance aggregate.
  useEffect(() => {
    if (!hasLiveWork) return;
    let intervalId: number | null = null;
    const start = () => {
      if (intervalId != null) return;
      intervalId = window.setInterval(() => {
        void load(true);
      }, POLLING_INTERVAL_MS);
    };
    const stop = () => {
      if (intervalId != null) {
        window.clearInterval(intervalId);
        intervalId = null;
      }
    };
    const onVisibility = () => {
      if (document.hidden) stop();
      else start();
    };
    if (!document.hidden) start();
    document.addEventListener("visibilitychange", onVisibility);
    return () => {
      document.removeEventListener("visibilitychange", onVisibility);
      stop();
    };
  }, [hasLiveWork, load]);

  const formatMinutes = (m: number | null): string => {
    if (m == null) return tString("performance.placeholders.noData");
    if (m < 1) return `<1 ${tString("performance.units.min")}`;
    return `${Math.round(m)} ${tString("performance.units.min")}`;
  };

  const formatRate = (r: number | null): string => {
    if (r == null) return tString("performance.placeholders.noData");
    return `${Math.round(r * 100)}%`;
  };

  const formatRating = (r: number | null): string => {
    if (r == null) return tString("performance.placeholders.noData");
    return r.toFixed(1);
  };

  // Half this table is windowed and half is not: loadDriverPerformanceAggregates
  // date-bounds only completed_today and completed_week, so avg delivery,
  // on-time rate, rating, cancellations and failures are lifetime figures. Read
  // beside a "Today" column they look like today's numbers, which is how an
  // operator ends up judging a shift by a driver's career average. (L3-44)
  const allTimeHeader = (labelKey: string) => (
    <span className="flex flex-col leading-tight">
      <span>{tString(`performance.columns.${labelKey}`)}</span>
      <span className="text-[10px] font-normal normal-case text-ink-400">
        {tString("performance.windows.allTime")}
      </span>
    </span>
  );

  const statusChipColor = (status: string): "success" | "warning" | "default" => {
    switch (status) {
      case "online":
        return "success";
      case "busy":
        return "warning";
      default:
        return "default";
    }
  };

  if (loading) {
    return (
      <div className="space-y-3" data-testid="performance-loading">
        <Skeleton className="h-10 w-full rounded-xl" />
        <Skeleton className="h-32 w-full rounded-xl" />
      </div>
    );
  }

  if (drivers.length === 0) {
    return (
      <Card>
        <CardBody className="py-10 text-center">
          <TrendingUp className="mx-auto mb-3 h-10 w-10 text-ink-400" />
          <p className="font-medium">{tString("performance.empty.title")}</p>
          <p className="mt-1 text-sm text-ink-500">
            {tString("performance.empty.subtitle")}
          </p>
        </CardBody>
      </Card>
    );
  }

  return (
    <div className="space-y-4">
      <div className="flex items-center justify-between">
        <div>
          <h2 className="text-lg font-semibold">{tString("performance.title")}</h2>
          <p className="text-sm text-ink-500">{tString("performance.subtitle")}</p>
        </div>
        <Button
          size="sm"
          variant="flat"
          startContent={<RefreshCcw className="h-3.5 w-3.5" />}
          isLoading={refreshing}
          onPress={() => void load(true)}
          aria-label={tString("performance.actions.refresh")}
        >
          {tString("performance.actions.refresh")}
        </Button>
      </div>

      <Card>
        <CardBody className="p-0">
          <Table
            aria-label={tString("performance.title")}
            removeWrapper
            data-testid="performance-table"
          >
            <TableHeader>
              <TableColumn>{tString("performance.columns.driver")}</TableColumn>
              <TableColumn>{tString("performance.columns.status")}</TableColumn>
              <TableColumn>{tString("performance.columns.inProgress")}</TableColumn>
              <TableColumn>{tString("performance.columns.completedToday")}</TableColumn>
              <TableColumn>{tString("performance.columns.completedWeek")}</TableColumn>
              <TableColumn>{allTimeHeader("avgDelivery")}</TableColumn>
              <TableColumn>{allTimeHeader("onTime")}</TableColumn>
              <TableColumn>{allTimeHeader("rating")}</TableColumn>
              <TableColumn>{allTimeHeader("issues")}</TableColumn>
              <TableColumn>
                <span className="sr-only">{tString("performance.columns.actions")}</span>
              </TableColumn>
            </TableHeader>
            <TableBody>
              {drivers.map((d) => (
                <TableRow key={d.driver_id} data-testid={`perf-row-${d.driver_id}`}>
                  <TableCell className="font-medium">{d.driver_name}</TableCell>
                  <TableCell>
                    <Chip size="sm" color={statusChipColor(d.status)} variant="flat">
                      {tString(`dispatch.drivers.status.${d.status}`) || d.status}
                    </Chip>
                  </TableCell>
                  <TableCell>
                    {d.in_progress_count > 0 ? (
                      <Chip size="sm" color="primary" variant="flat">
                        {d.in_progress_count}
                      </Chip>
                    ) : (
                      <span className="text-ink-400">0</span>
                    )}
                  </TableCell>
                  <TableCell>{d.completed_today}</TableCell>
                  <TableCell>{d.completed_week}</TableCell>
                  <TableCell>{formatMinutes(d.avg_delivery_minutes)}</TableCell>
                  <TableCell>{formatRate(d.on_time_rate)}</TableCell>
                  <TableCell>{formatRating(d.average_rating)}</TableCell>
                  <TableCell>
                    {d.failed_count + d.cancelled_count > 0 ? (
                      <Chip
                        size="sm"
                        color={d.failed_count > 0 ? "danger" : "warning"}
                        variant="flat"
                        aria-label={tString("performance.issuesAria", {
                          failed: String(d.failed_count),
                          cancelled: String(d.cancelled_count),
                        })}
                      >
                        {d.failed_count > 0
                          ? `${d.failed_count} ${tString("performance.kpi.failed").toLowerCase()}`
                          : `${d.cancelled_count} ${tString("performance.kpi.cancelled").toLowerCase()}`}
                      </Chip>
                    ) : (
                      <span className="text-ink-400">0</span>
                    )}
                  </TableCell>
                  <TableCell>
                    <Button
                      size="sm"
                      variant="flat"
                      startContent={<Eye className="h-3.5 w-3.5" />}
                      onPress={() => setSelectedDriverId(d.driver_id)}
                      aria-label={tString("performance.actions.view", {
                        name: d.driver_name,
                      })}
                    >
                      {tString("performance.actions.viewLabel")}
                    </Button>
                  </TableCell>
                </TableRow>
              ))}
            </TableBody>
          </Table>
        </CardBody>
      </Card>

      {selectedDriverId !== null ? (
        <DriverScorecardModal
          businessId={businessId}
          driverId={selectedDriverId}
          isOpen
          onClose={() => setSelectedDriverId(null)}
          tString={tString}
          currency={businessCurrency}
        />
      ) : null}
    </div>
  );
}
