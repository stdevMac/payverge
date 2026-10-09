import { axiosInstance } from './tools/instance';

export interface KitchenOrdersStatus {
  kitchen_enabled: boolean;
  orders_enabled: boolean;
}

export interface ToggleKitchenOrdersResponse {
  message: string;
  kitchen_enabled: boolean;
  orders_enabled: boolean;
}

// Get kitchen and orders status for a business
export const getKitchenOrdersStatus = async (businessId: number): Promise<KitchenOrdersStatus> => {
  const response = await axiosInstance.get(`/inside/businesses/${businessId}/kitchen-orders-status`);
  return response.data;
};

// Toggle kitchen and orders features
export const toggleKitchenAndOrders = async (
  businessId: number,
  enabled: boolean
): Promise<ToggleKitchenOrdersResponse> => {
  const response = await axiosInstance.post(
    `/inside/businesses/${businessId}/toggle-kitchen-orders`,
    { enabled }
  );
  return response.data;
};
