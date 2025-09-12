import { axiosInstance } from './tools/instance';

// Backend API endpoints for Phase 3 (actual implementation)
export const PUBLIC_ENDPOINTS = {
  // Table information - using actual backend routes
  getTableInfo: (tableCode: string) => `/guest/table/${tableCode}`,
  getTableBusiness: (tableCode: string) => `/guest/table/${tableCode}/business`,
  getTableMenu: (tableCode: string) => `/guest/table/${tableCode}/menu`,
  getTableStatus: (tableCode: string) => `/guest/table/${tableCode}/status`,
  
  // Bill information  
  getCurrentBill: (tableCode: string) => `/guest/table/${tableCode}/bill`,
  getBillByNumber: (billNumber: string) => `/guest/bill/${billNumber}`,
} as const;

// Legacy endpoints for backward compatibility
export const LEGACY_ENDPOINTS = {
  getTableByCode: (tableCode: string) => `/table/${tableCode}`,
} as const;

// Helper function to get the appropriate endpoint
export function getTableInfoEndpoint(tableCode: string): string {
  return PUBLIC_ENDPOINTS.getTableInfo(tableCode);
}

export function getCurrentBillEndpoint(tableCode: string): string {
  return PUBLIC_ENDPOINTS.getCurrentBill(tableCode);
}

export function getTableBusinessEndpoint(tableCode: string): string {
  return PUBLIC_ENDPOINTS.getTableBusiness(tableCode);
}

export function getTableMenuEndpoint(tableCode: string): string {
  return PUBLIC_ENDPOINTS.getTableMenu(tableCode);
}

// Standardized Public API endpoints as per Phase 3 specification
export const publicAPI = {
  // Get table info and menu (no auth required)
  getTableInfo: async (tableCode: string) => {
    const response = await axiosInstance.get(getTableInfoEndpoint(tableCode));
    return response.data;
  },

  // Get current bill for table (no auth required)
  getTableBill: async (tableCode: string) => {
    const response = await axiosInstance.get(getCurrentBillEndpoint(tableCode));
    return response.data;
  },

  // Validate table code exists and is active
  validateTable: async (tableCode: string) => {
    const response = await axiosInstance.get(`/public/tables/${tableCode}/validate`);
    return response.data;
  }
};

// Migration helper to use standardized endpoints
export const migrateToStandardizedAPI = {
  // Map old endpoints to new standardized ones
  getTableByCode: publicAPI.getTableInfo,
  getOpenBillByTableCode: publicAPI.getTableBill,
  
  // Keep existing endpoints for backward compatibility
  getBusinessByTableCode: async (code: string) => {
    const response = await axiosInstance.get(`/guest/table/${code}/business`);
    return response.data;
  },
  
  getMenuByTableCode: async (code: string) => {
    const response = await axiosInstance.get(`/guest/table/${code}/menu`);
    return response.data;
  }
};
