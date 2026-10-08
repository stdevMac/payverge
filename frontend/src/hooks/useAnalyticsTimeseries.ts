'use client'

import { useQuery } from '@tanstack/react-query'
import { analyticsApi, type Timeseries } from '@/api/analytics'
import { queryKeys } from '@/api/queryKeys'

interface UseAnalyticsTimeseriesResult {
  series: Timeseries | null
  loading: boolean
  error: string | null
}

export function useAnalyticsTimeseries(
  businessId: string | number | undefined,
  opts: { from?: string; to?: string; bucket?: 'day' } = {},
): UseAnalyticsTimeseriesResult {
  const requestId = businessId == null ? '' : String(businessId)
  const cacheKey = /^\d+$/.test(requestId) ? String(parseInt(requestId, 10)) : requestId
  const { from, to, bucket = 'day' } = opts

  const { data = null, isLoading: loading, error: queryError } = useQuery({
    queryKey: [...queryKeys.business.analytics(cacheKey), 'timeseries', { from, to, bucket }],
    queryFn: () => analyticsApi.getTimeseries(requestId, { from, to, bucket }),
    enabled: !!cacheKey,
    staleTime: 5 * 60 * 1000,
  })

  return { series: data, loading, error: queryError ? (queryError as Error).message : null }
}
