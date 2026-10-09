"use client";

import { useMemo } from "react";
import { useQueries } from "@tanstack/react-query";
import { analyticsApi, type Timeseries } from "@/api/analytics";
import { queryKeys } from "@/api/queryKeys";
import {
  mergeTimeseriesByCurrency,
  type CombinedCurrencySeries,
} from "@/components/dashboard/mergeTimeseries";

interface UseCombinedAnalyticsTimeseriesResult {
  seriesByCurrency: CombinedCurrencySeries[];
  loading: boolean;
  error: string | null;
}

export function useCombinedAnalyticsTimeseries(
  businesses: Array<{ id: number; default_currency?: string }>,
  opts: { from: string; to: string; enabled?: boolean },
): UseCombinedAnalyticsTimeseriesResult {
  const { from, to, enabled = true } = opts;
  const shouldFetch = enabled && businesses.length > 0;

  const queries = useQueries({
    queries: businesses.map((business) => {
      const cacheKey = String(business.id);
      return {
        queryKey: [
          ...queryKeys.business.analytics(cacheKey),
          "timeseries",
          { from, to, bucket: "day" },
        ],
        queryFn: () =>
          analyticsApi.getTimeseries(cacheKey, { from, to, bucket: "day" }),
        enabled: shouldFetch,
        staleTime: 5 * 60 * 1000,
      };
    }),
  });

  const seriesByCurrency = useMemo(() => {
    const entries = businesses.map((business, index) => ({
      currency: business.default_currency || "USD",
      series: (queries[index]?.data ?? null) as Timeseries | null,
    }));
    return mergeTimeseriesByCurrency(entries);
  }, [businesses, queries]);

  const loading = shouldFetch && queries.some((query) => query.isLoading);
  const error =
    shouldFetch && queries.length > 0 && queries.every((query) => query.error)
      ? (queries[0].error as Error).message
      : null;

  return { seriesByCurrency, loading, error };
}
