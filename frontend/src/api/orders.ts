import { axiosInstance } from './tools/instance';

// Order interfaces
export interface OrderItem {
  id: string;
  menu_item_name: string;
  quantity: number;
  price: number;
  special_requests: string;
  subtotal: number;
}

export interface Order {
  id: number;
  bill_id: number;
  business_id: number;
  order_number: string;
  status: 'pending' | 'approved' | 'in_kitchen' | 'ready' | 'delivered' | 'cancelled';
  created_by: string;
  approved_by: string;
  notes: string;
  items: string; // JSON string of OrderItem[]
  created_at: string;
  updated_at: string;
  approved_at?: string;
  bill?: any;
  business?: any;
}

export interface CreateOrderRequest {
  bill_id: number;
  notes?: string;
  items: {
    menu_item_name: string;
    quantity: number;
    price: number;
    special_requests?: string;
  }[];
}

export interface UpdateOrderStatusRequest {
  status: 'pending' | 'approved' | 'in_kitchen' | 'ready' | 'delivered' | 'cancelled';
  approved_by?: string;
}

export interface OrdersResponse {
  orders: Order[];
  total: number;
}

export interface OrderResponse {
  order: Order;
}

// Order API functions

// Create a new order (for staff)
export const createOrder = async (
  businessId: number,
  orderData: CreateOrderRequest
): Promise<Order> => {
  const response = await axiosInstance.post(
    `/inside/businesses/${businessId}/orders`,
    orderData
  );
  return response.data.order;
};

// Get orders for a business
export const getOrders = async (
  businessId: number,
  status?: string
): Promise<OrdersResponse> => {
  const params = new URLSearchParams();
  if (status) params.append('status', status);

  const response = await axiosInstance.get(
    `/inside/businesses/${businessId}/orders?${params.toString()}`
  );
  return response.data;
};

// Get a specific order
export const getOrder = async (
  businessId: number,
  orderId: number
): Promise<Order> => {
  const response = await axiosInstance.get(
    `/inside/businesses/${businessId}/orders/${orderId}`
  );
  return response.data.order;
};

// Get orders for a specific bill
export const getOrdersByBillId = async (
  businessId: number,
  billId: number
): Promise<OrdersResponse> => {
  const response = await axiosInstance.get(
    `/inside/businesses/${businessId}/bills/${billId}/orders`
  );
  return response.data;
};

// Update order status (for staff approval/kitchen updates)
export const updateOrderStatus = async (
  businessId: number,
  orderId: number,
  statusData: UpdateOrderStatusRequest
): Promise<Order> => {
  const response = await axiosInstance.put(
    `/inside/businesses/${businessId}/orders/${orderId}/status`,
    statusData
  );
  return response.data.order;
};

// Guest order creation (public endpoint)
export const createGuestOrder = async (
  tableCode: string,
  orderData: {
    bill_id: number;
    items: {
      menu_item_name: string;
      quantity: number;
      price: number;
      special_requests?: string;
    }[];
    notes?: string;
  }
): Promise<any> => {
  const response = await axiosInstance.post(`/guest/table/${tableCode}/order`, orderData);
  return response.data;
};

// Helper function to parse order items from JSON string
export const parseOrderItems = (itemsJson: string): OrderItem[] => {
  try {
    return JSON.parse(itemsJson || '[]');
  } catch {
    return [];
  }
};

// Helper function to get order status color
export const getOrderStatusColor = (status: string) => {
  switch (status) {
    case 'pending':
      return 'warning';
    case 'approved':
      return 'primary';
    case 'in_kitchen':
      return 'secondary';
    case 'ready':
      return 'success';
    case 'delivered':
      return 'default';
    case 'cancelled':
      return 'danger';
    default:
      return 'default';
  }
};

// Helper function to get order status text
export const getOrderStatusText = (status: string) => {
  switch (status) {
    case 'pending':
      return 'Pending Approval';
    case 'approved':
      return 'Approved';
    case 'in_kitchen':
      return 'In Kitchen';
    case 'ready':
      return 'Ready';
    case 'delivered':
      return 'Delivered';
    case 'cancelled':
      return 'Cancelled';
    default:
      return status;
  }
};
