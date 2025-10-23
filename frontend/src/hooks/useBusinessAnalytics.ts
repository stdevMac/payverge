import { useState, useCallback } from 'react';
import analyticsApi, { SalesData } from '@/api/analytics';
import { getBusinessTables, Table } from '@/api/business';

export const useBusinessAnalytics = (businessId: number) => {
  const [salesData, setSalesData] = useState<SalesData | null>(null);
  const [analyticsLoading, setAnalyticsLoading] = useState(false);
  const [tables, setTables] = useState<Table[]>([]);

  // Fetch analytics data
  const fetchAnalytics = useCallback(async (period: string = 'today') => {
    if (!businessId) return;
    
    try {
      setAnalyticsLoading(true);
      const [salesResponse, tablesResponse] = await Promise.all([
        analyticsApi.getSalesAnalytics(businessId.toString(), period),
        getBusinessTables(businessId)
      ]);
      setSalesData(salesResponse);
      setTables(tablesResponse.tables || []);
    } catch (error) {
      console.error('Error fetching analytics:', error);
    } finally {
      setAnalyticsLoading(false);
    }
  }, [businessId]);

  const refreshAnalytics = useCallback((period?: string) => {
    fetchAnalytics(period);
  }, [fetchAnalytics]);

  return {
    salesData,
    analyticsLoading,
    tables,
    fetchAnalytics,
    refreshAnalytics
  };
};
