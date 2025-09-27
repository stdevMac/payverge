import { axiosInstance } from './index';

export interface DashboardStats {
  totalRevenue: number;
  totalOrders: number;
  averageOrderValue: number;
  topItems: Array<{
    name: string;
    quantity: number;
    revenue: number;
  }>;
}

export interface RevenueData {
  date: string;
  revenue: number;
  orders: number;
}

// Get dashboard analytics data
export const getDashboardAnalytics = async (businessId: number): Promise<DashboardStats> => {
  const response = await axiosInstance.get<DashboardStats>(`/inside/businesses/${businessId}/analytics/dashboard`);
  return response.data;
};

// Get revenue data for charts
export const getRevenueData = async (
  businessId: number,
  period: string = '7d'
): Promise<RevenueData[]> => {
  const response = await axiosInstance.get<RevenueData[]>(
    `/inside/businesses/${businessId}/analytics/revenue`,
    { params: { period } }
  );
  return response.data;
};

// Get live bills data
export const getLiveBills = async (businessId: number) => {
  const response = await axiosInstance.get(`/inside/businesses/${businessId}/analytics/live-bills`);
  return response.data.data || [];
};
