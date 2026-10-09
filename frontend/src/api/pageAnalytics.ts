import { axiosInstance } from "@/api/tools/instance";

export interface AnalyticsSummary {
  total_page_views: number;
  total_sessions: number;
  total_interactions: number;
  total_conversions: number;
  average_session_time: number;
  bounce_rate: number;
  conversion_rate: number;
  top_pages: PageStats[];
  top_interactions: InteractionStats[];
  device_breakdown: Record<string, number>;
  country_breakdown: Record<string, number>;
  hourly_traffic: HourlyStats[];
  conversion_funnel: FunnelStep[];
}

interface PageStats {
  page: string;
  views: number;
  unique_visitors: number;
  average_duration: number;
  bounce_rate: number;
}

interface InteractionStats {
  event_type: string;
  event_category: string;
  event_label: string;
  count: number;
  unique_sessions: number;
}

interface HourlyStats {
  hour: number;
  page_views: number;
  sessions: number;
  interactions: number;
}

interface FunnelStep {
  step: string;
  sessions: number;
  /** Null when the previous step is zero or this step increased. */
  dropoff_rate: number | null;
}

export interface SessionSummary {
  session_id: string;
  first_seen: string;
  last_seen: string;
  total_page_views: number;
  total_interactions: number;
  total_duration: number;
  pages_visited: string;
  converted: boolean;
  conversion_type: string;
  device_type: string;
  country: string;
}

export const pageAnalyticsAPI = {
  getAnalyticsSummary: async (startDate?: string, endDate?: string): Promise<AnalyticsSummary> => {
    const params: Record<string, string> = {};
    if (startDate) params.start_date = startDate;
    if (endDate) params.end_date = endDate;
    
    const response = await axiosInstance.get('/admin/analytics/summary', { params });
    return response.data;
  },

  getRecentSessions: async (limit: number = 50): Promise<SessionSummary[]> => {
    const response = await axiosInstance.get('/admin/analytics/sessions', { params: { limit } });
    return response.data;
  },
};
