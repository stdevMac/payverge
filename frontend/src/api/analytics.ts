import { axiosInstance } from './tools/instance'
import { asDollars, type Dollars } from '@/types/money'

// Analytics money fields are emitted in DOLLARS by the backend (see
// internal/analytics/service.go + models_json.go wire contract).

export interface SalesData {
  total_revenue: Dollars
  total_tips: Dollars
  transaction_count: number
  bill_count: number
  unique_customers: number
  average_ticket: Dollars
  /** Transaction counts per method (backend emits map[string]int), not money. */
  payment_methods?: Record<string, number>
  hourly_breakdown?: Record<string, {
    revenue: Dollars
    bill_count: number
    transaction_count: number
  }>
  growth_rate?: number
  start_date?: string
  end_date?: string
}

export interface TipAnalytics {
  total_tips: Dollars
  tip_count: number
  average_tip: Dollars
  average_tip_rate: number
  tip_distribution: Record<string, number>
  top_tippers: Array<{
    payer_address: string
    /** CRM guest display name when the payer wallet is linked. */
    guest_name?: string
    total_tips: Dollars
    tip_count: number
    average_tip: Dollars
  }>
  hourly_tips: Record<string, Dollars>
  daily_comparison: {
    today: Dollars
    yesterday: Dollars
    change_percentage: number
  }
}

/** Wave 4: per-staff tip rollup row from GET .../analytics/tips-by-staff. */
export interface StaffTipRow {
  staff_id: number | null
  staff_name: string
  total_tips: number
  bill_count: number
}

export interface ItemStats {
  item_id: string
  item_name: string
  category: string
  total_sold: number
  revenue: Dollars
  bills_featured: number
  avg_price: Dollars
  popularity_rank: number
}

interface RawTipAnalytics {
  total_tips?: number
  tip_count?: number
  average_tip?: number
  average_tip_rate?: number
  tip_distribution?: Record<string, number>
  top_tippers?: Array<{
    payer_address: string
    guest_name?: string
    total_tips: number
    tip_count: number
    average_tip: number
  }>
  hourly_tips?: Record<string, number>
  daily_comparison?: {
    today?: number
    yesterday?: number
    change_percentage?: number
  }
}

interface RawItemStats extends Partial<ItemStats> {
  average_price?: number
  popularity?: number
}

interface RawDashboardSummary extends Partial<Omit<DashboardSummary, 'top_items'>> {
  top_items?: RawItemStats[]
}

export interface TimeseriesBucket {
  date: string
  revenue: Dollars
  tips: Dollars
  bills: number
  transactions: number
  average_ticket: Dollars
}

export interface Timeseries {
  buckets: TimeseriesBucket[]
  range: { from: string; to: string }
}

interface RawTimeseriesBucket {
  date?: string
  revenue?: number
  tips?: number
  bills?: number
  transactions?: number
  average_ticket?: number
}

const normalizeTimeseries = (
  data: { buckets?: RawTimeseriesBucket[]; range?: { from?: string; to?: string } } | null | undefined
): Timeseries => ({
  buckets: (data?.buckets ?? []).map((b) => ({
    date: b.date ?? '',
    revenue: asDollars(b.revenue ?? 0),
    tips: asDollars(b.tips ?? 0),
    bills: b.bills ?? 0,
    transactions: b.transactions ?? 0,
    average_ticket: asDollars(b.average_ticket ?? 0),
  })),
  range: { from: data?.range?.from ?? '', to: data?.range?.to ?? '' },
})

export interface LiveBill {
  id: number
  bill_number: string
  /** 0 is the "no table" sentinel (delivery/counter bills). */
  table_id?: number
  /** Set for counter bills; tableless bills without one are deliveries. */
  counter_id?: number | null
  table_name: string
  table_code: string
  total_amount: Dollars
  paid_amount: Dollars
  remaining_amount: Dollars
  tip_amount: Dollars
  status: string
  created_at: string
  updated_at: string
}

interface TodayByCurrentStaff {
  staff_id: number
  bills_created: number
  bills_paid: number
  revenue_from_paid: Dollars
  tips_from_paid: Dollars
}

interface ActiveBillsOnTable {
  count: number
  oldest_bill_id: number
  oldest_minutes: number
}

export interface DashboardSummary {
  today: {
    revenue: Dollars
    tips: Dollars
    transactions: number
    bills: number
    /** Tonight's kitchen tickets (pending / approved / in_kitchen). */
    order_count?: number
    /** Recognized (collected) tenders today, excluding live floor remaining. */
    collected_revenue?: Dollars
    /** Remaining due on currently open/partial checks (any created_at). */
    floor_remaining?: Dollars
    by_current_staff?: TodayByCurrentStaff
  }
  week: {
    revenue: Dollars
    tips: Dollars
    transactions: number
    bills: number
    unique_customers: number
    average_ticket: Dollars
  }
  live: {
    active_bills: number
    /** Distinct occupied tables (table_id > 0). Prefer this over active_bills for "Open tables". */
    open_tables?: number
    /** Open/partial checks with table_id=0 (delivery/counter). Do not hide these. */
    untabled_bills?: number
    active_bills_by_table?: Record<number, ActiveBillsOnTable>
  }
  top_items: ItemStats[]
}

const normalizeTipAnalytics = (
  data: RawTipAnalytics | null | undefined
): TipAnalytics => {
  const tipDistribution = data?.tip_distribution ?? {}
  const derivedTipCount = Object.values(tipDistribution).reduce(
    (total, count) => total + count,
    0
  )

  return {
    total_tips: asDollars(data?.total_tips ?? 0),
    tip_count: data?.tip_count ?? derivedTipCount,
    average_tip: asDollars(data?.average_tip ?? 0),
    average_tip_rate: data?.average_tip_rate ?? 0,
    tip_distribution: tipDistribution,
    top_tippers: (data?.top_tippers ?? []).map((tipper) => ({
      payer_address: tipper.payer_address ?? '',
      guest_name: tipper.guest_name?.trim() || undefined,
      tip_count: tipper.tip_count,
      total_tips: asDollars(tipper.total_tips),
      average_tip: asDollars(tipper.average_tip),
    })),
    hourly_tips: Object.fromEntries(
      Object.entries(data?.hourly_tips ?? {}).map(([k, v]) => [k, asDollars(v)])
    ),
    daily_comparison: {
      today: asDollars(data?.daily_comparison?.today ?? 0),
      yesterday: asDollars(data?.daily_comparison?.yesterday ?? 0),
      change_percentage: data?.daily_comparison?.change_percentage ?? 0,
    },
  }
}

const normalizeItemStats = (
  item: RawItemStats | null | undefined
): ItemStats => ({
  item_id: item?.item_id ?? '',
  item_name: item?.item_name ?? '',
  category: item?.category ?? 'General',
  total_sold: item?.total_sold ?? 0,
  revenue: asDollars(item?.revenue ?? 0),
  bills_featured: item?.bills_featured ?? 0,
  avg_price: asDollars(item?.avg_price ?? item?.average_price ?? 0),
  popularity_rank: item?.popularity_rank ?? item?.popularity ?? 0,
})

const normalizeDashboardSummary = (
  data: RawDashboardSummary | null | undefined
): DashboardSummary => ({
  today: {
    revenue: asDollars(
      data?.today?.collected_revenue ?? data?.today?.revenue ?? 0,
    ),
    tips: asDollars(data?.today?.tips ?? 0),
    transactions: data?.today?.transactions ?? 0,
    bills: data?.today?.bills ?? 0,
    order_count: data?.today?.order_count ?? 0,
    collected_revenue: asDollars(
      data?.today?.collected_revenue ?? data?.today?.revenue ?? 0,
    ),
    floor_remaining: asDollars(data?.today?.floor_remaining ?? 0),
    ...(data?.today?.by_current_staff
      ? {
          by_current_staff: {
            ...data.today.by_current_staff,
            revenue_from_paid: asDollars(
              data.today.by_current_staff.revenue_from_paid,
            ),
            tips_from_paid: asDollars(
              data.today.by_current_staff.tips_from_paid,
            ),
          },
        }
      : {}),
  },
  week: {
    revenue: asDollars(data?.week?.revenue ?? 0),
    tips: asDollars(data?.week?.tips ?? 0),
    transactions: data?.week?.transactions ?? 0,
    bills: data?.week?.bills ?? 0,
    unique_customers: data?.week?.unique_customers ?? 0,
    average_ticket: asDollars(data?.week?.average_ticket ?? 0),
  },
  live: {
    active_bills: data?.live?.active_bills ?? 0,
    // Prefer backend open_tables; fall back to distinct keys of by-table map so
    // delivery/counter bills (table_id=0) never inflate "Open tables".
    open_tables:
      data?.live?.open_tables ??
      (data?.live?.active_bills_by_table
        ? Object.keys(data.live.active_bills_by_table).length
        : 0),
    untabled_bills: data?.live?.untabled_bills ?? 0,
    ...(data?.live?.active_bills_by_table !== undefined && {
      active_bills_by_table: data.live.active_bills_by_table,
    }),
  },
  top_items: (data?.top_items ?? []).map((item) => normalizeItemStats(item)),
})

export const analyticsApi = {
  // Get sales analytics for a business
  getSalesAnalytics: async (
    businessId: string,
    period: string = 'today',
    date?: string
  ): Promise<SalesData> => {
    const params = new URLSearchParams({ period })
    if (date) params.append('date', date)
    
    const response = await axiosInstance.get(
      `/inside/businesses/${businessId}/analytics/sales?${params.toString()}`
    )
    return response.data.data
  },

  // Get tip analytics for a business
  getTipAnalytics: async (
    businessId: string,
    period: string = 'week'
  ): Promise<TipAnalytics> => {
    const response = await axiosInstance.get(
      `/inside/businesses/${businessId}/analytics/tips?period=${period}`
    )
    return normalizeTipAnalytics(response.data.data)
  },

  // Wave 4: per-staff tip rollup (Team tab panel)
  getTipsByStaff: async (
    businessId: string,
    period: string = 'week'
  ): Promise<StaffTipRow[]> => {
    const response = await axiosInstance.get(
      `/inside/businesses/${businessId}/analytics/tips-by-staff?period=${period}`
    )
    return response.data.data ?? []
  },

  // Get item performance analytics
  getItemAnalytics: async (
    businessId: string,
    period: string = 'week'
  ): Promise<ItemStats[]> => {
    const response = await axiosInstance.get(
      `/inside/businesses/${businessId}/analytics/items?period=${period}`
    )
    return (response.data.data ?? []).map((item: RawItemStats) =>
      normalizeItemStats(item)
    )
  },

  // Get live bills for real-time monitoring
  getLiveBills: async (businessId: string): Promise<LiveBill[]> => {
    const response = await axiosInstance.get(
      `/inside/businesses/${businessId}/analytics/live-bills`
    )
    return response.data.data
  },

  // Get live bills with the backend's capped/limit flags. Additive to
  // getLiveBills — used by the shared live pulse to surface an honest
  // "showing first N" banner when a busy board is truncated at the server LIMIT.
  getLiveBillsPage: async (
    businessId: string,
    signal?: AbortSignal
  ): Promise<{ bills: LiveBill[]; capped: boolean; limit: number }> => {
    const response = await axiosInstance.get(
      `/inside/businesses/${businessId}/analytics/live-bills`,
      signal ? { signal } : undefined
    )
    const data = response.data ?? {}
    return {
      bills: Array.isArray(data.data) ? data.data : [],
      capped: Boolean(data.capped),
      limit: typeof data.limit === 'number' ? data.limit : 0,
    }
  },

  // Get dashboard summary with key metrics
  getDashboardSummary: async (
    businessId: string,
    signal?: AbortSignal
  ): Promise<DashboardSummary> => {
    const response = await axiosInstance.get(
      `/inside/businesses/${businessId}/analytics/dashboard`,
      signal ? { signal } : undefined
    )
    return normalizeDashboardSummary(response.data.data)
  },

  // Get a zero-filled daily analytics series for trend charts.
  getTimeseries: async (
    businessId: string,
    opts: { from?: string; to?: string; bucket?: 'day'; period?: string } = {}
  ): Promise<Timeseries> => {
    const params = new URLSearchParams()
    if (opts.from) params.append('from', opts.from)
    if (opts.to) params.append('to', opts.to)
    if (opts.period) params.append('period', opts.period)
    params.append('bucket', opts.bucket ?? 'day')
    const response = await axiosInstance.get(
      `/inside/businesses/${businessId}/analytics/timeseries?${params.toString()}`
    )
    return normalizeTimeseries(response.data.data)
  },

  getDashboardSummaries: async (
    businessIds: string[]
  ): Promise<Record<string, DashboardSummary>> => {
    const numericBusinessIds = businessIds.map((businessId) => {
      const parsed = Number(businessId)
      if (!Number.isFinite(parsed) || parsed <= 0) {
        throw new Error(`Invalid business id: ${businessId}`)
      }
      return parsed
    })

    const response = await axiosInstance.post(
      '/inside/analytics/dashboard-summaries',
      { business_ids: numericBusinessIds }
    )

    return Object.fromEntries(
      Object.entries(response.data.data ?? {}).map(([businessId, summary]) => [
        businessId,
        normalizeDashboardSummary(summary as RawDashboardSummary),
      ])
    )
  },

  // Export sales data in CSV or JSON 
  exportAnalyticsData: async (
    businessId: string,
    period: string = 'week',
    format: 'csv' | 'json' = 'csv',
    lang?: string,
  ): Promise<Blob> => {
    const response = await axiosInstance.get(
      `/inside/businesses/${businessId}/reports/export`,
      {
        params: { period, format, ...(lang ? { lang } : {}) },
        responseType: 'blob'
      }
    )
    
    return response.data
  },
}
