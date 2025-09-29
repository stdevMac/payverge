import { axiosInstance } from './index';

// Kitchen Order Types
export interface KitchenOrderItem {
  id: number;
  menu_item_name: string;
  quantity: number;
  price: number;
  status: 'pending' | 'in_progress' | 'ready' | 'cancelled';
  special_requests?: string;
  started_at?: string;
  ready_at?: string;
}

export interface KitchenOrder {
  id: number;
  business_id: number;
  bill_id?: number;
  table_id?: number;
  order_number: string;
  customer_name?: string;
  order_type: 'table' | 'counter' | 'takeout' | 'delivery';
  status: 'incoming' | 'in_progress' | 'ready' | 'delivered' | 'cancelled';
  priority: 'low' | 'normal' | 'high' | 'urgent';
  notes?: string;
  created_by: string;
  assigned_to?: string;
  estimated_time?: number;
  actual_time?: number;
  created_at: string;
  updated_at: string;
  ready_at?: string;
  delivered_at?: string;
  items: KitchenOrderItem[];
  table?: { name: string };
}

export interface CreateKitchenOrderRequest {
  bill_id?: number;
  table_id?: number;
  customer_name?: string;
  order_type: 'table' | 'counter' | 'takeout' | 'delivery';
  priority: 'low' | 'normal' | 'high' | 'urgent';
  notes?: string;
  items: {
    menu_item_name: string;
    quantity: number;
    price: number;
    special_requests?: string;
  }[];
}

export interface KitchenOrdersResponse {
  orders: KitchenOrder[];
  total: number;
}

export interface KitchenStats {
  business_id: number;
  date: string;
  total_orders: number;
  completed_orders: number;
  cancelled_orders: number;
  average_time: number;
  peak_hour: string;
  total_revenue: number;
}

// Kitchen API functions
export const createKitchenOrder = async (
  businessId: number,
  orderData: CreateKitchenOrderRequest
): Promise<KitchenOrder> => {
  const response = await axiosInstance.post(
    `/inside/businesses/${businessId}/kitchen/orders`,
    orderData
  );
  return response.data.order;
};

export const getKitchenOrders = async (
  businessId: number,
  status?: string,
  orderType?: string,
  limit?: number
): Promise<KitchenOrdersResponse> => {
  const params = new URLSearchParams();
  if (status) params.append('status', status);
  if (orderType) params.append('order_type', orderType);
  if (limit) params.append('limit', limit.toString());

  const response = await axiosInstance.get(
    `/inside/businesses/${businessId}/kitchen/orders?${params.toString()}`
  );
  return response.data;
};

export const getKitchenOrder = async (
  businessId: number,
  orderId: number
): Promise<KitchenOrder> => {
  const response = await axiosInstance.get(
    `/inside/businesses/${businessId}/kitchen/orders/${orderId}`
  );
  return response.data.order;
};

export const updateKitchenOrderStatus = async (
  businessId: number,
  orderId: number,
  status: string,
  assignedTo?: string,
  estimatedTime?: number,
  notes?: string
): Promise<KitchenOrder> => {
  const response = await axiosInstance.put(
    `/inside/businesses/${businessId}/kitchen/orders/${orderId}/status`,
    {
      status,
      assigned_to: assignedTo,
      estimated_time: estimatedTime,
      notes,
    }
  );
  return response.data.order;
};

export const updateKitchenOrderItemStatus = async (
  businessId: number,
  orderId: number,
  itemId: number,
  status: string
): Promise<void> => {
  await axiosInstance.put(
    `/inside/businesses/${businessId}/kitchen/orders/${orderId}/items/${itemId}/status`,
    { status }
  );
};

export const getKitchenOrdersByBillId = async (
  businessId: number,
  billId: number
): Promise<KitchenOrdersResponse> => {
  const response = await axiosInstance.get(
    `/inside/businesses/${businessId}/bills/${billId}/kitchen/orders`
  );
  return response.data;
};

export const getKitchenStats = async (
  businessId: number,
  date?: string
): Promise<KitchenStats> => {
  const params = date ? `?date=${date}` : '';
  const response = await axiosInstance.get(
    `/inside/businesses/${businessId}/kitchen/stats${params}`
  );
  return response.data;
};
