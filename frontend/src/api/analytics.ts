import { axiosInstance } from './tools/instance'

export interface SalesData {
  total_revenue: number
  total_tips: number
  transaction_count: number
  bill_count: number
  unique_customers: number
  average_ticket: number
  payment_methods?: Record<string, number>
  hourly_breakdown?: Record<string, {
    revenue: number
    bill_count: number
    transaction_count: number
  }>
  growth_rate?: number
  start_date?: string
  end_date?: string
}

export interface TipAnalytics {
  total_tips: number
  tip_count: number
  average_tip: number
  average_tip_rate: number
  tip_distribution: Record<string, number>
  top_tippers: Array<{
    payer_address: string
    total_tips: number
    tip_count: number
    average_tip: number
  }>
  hourly_tips: Record<string, number>
  daily_comparison: {
    today: number
    yesterday: number
    change_percentage: number
  }
}

export interface ItemStats {
  item_id: string
  item_name: string
  category: string
  total_sold: number
  revenue: number
  bills_featured: number
  avg_price: number
  popularity_rank: number
}

export interface LiveBill {
  id: number
  bill_number: string
  table_name: string
  table_code: string
  total_amount: number
  paid_amount: number
  remaining_amount: number
  tip_amount: number
  status: string
  created_at: string
  updated_at: string
}

export interface DashboardSummary {
  today: {
    revenue: number
    tips: number
    transactions: number
    bills: number
  }
  week: {
    revenue: number
    tips: number
    transactions: number
    bills: number
    unique_customers: number
    average_ticket: number
  }
  live: {
    active_bills: number
  }
  top_items: ItemStats[]
}

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
    return response.data.data
  },

  // Get item performance analytics
  getItemAnalytics: async (
    businessId: string,
    period: string = 'week'
  ): Promise<ItemStats[]> => {
    const response = await axiosInstance.get(
      `/inside/businesses/${businessId}/analytics/items?period=${period}`
    )
    return response.data.data
  },

  // Get live bills for real-time monitoring
  getLiveBills: async (businessId: string): Promise<LiveBill[]> => {
    const response = await axiosInstance.get(
      `/inside/businesses/${businessId}/analytics/live-bills`
    )
    return response.data.data
  },

  // Get dashboard summary with key metrics
  getDashboardSummary: async (businessId: string): Promise<DashboardSummary> => {
    const response = await axiosInstance.get(
      `/inside/businesses/${businessId}/analytics/dashboard`
    )
    return response.data.data
  },

  // Export sales data in CSV or JSON 
  exportAnalyticsData: async (
    businessId: string,
    period: string = 'week',
    format: 'csv' | 'json' = 'csv'
  ): Promise<Blob> => {
    const response = await axiosInstance.get(
      `/inside/businesses/${businessId}/reports/export`,
      {
        params: { period, format },
        responseType: 'blob'
      }
    )
    
    return response.data
  },
}

export default analyticsApi
