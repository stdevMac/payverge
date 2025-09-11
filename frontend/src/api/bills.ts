import { axiosInstance } from './tools/instance';

// Bill interfaces
export interface BillItem {
  id: string;
  menu_item_id: string;
  name: string;
  price: number;
  quantity: number;
  options: MenuItemOption[];
  subtotal: number;
}

export interface MenuItemOption {
  name: string;
  price: number;
}

export interface Bill {
  id: number;
  business_id: number;
  table_id: number;
  bill_number: string;
  items: string; // JSON string
  subtotal: number;
  tax_amount: number;
  service_fee_amount: number;
  total_amount: number;
  paid_amount: number;
  tip_amount: number;
  status: 'open' | 'paid' | 'closed';
  settlement_address: string;
  tipping_address: string;
  created_at: string;
  updated_at: string;
  closed_at?: string;
}

export interface CreateBillRequest {
  table_id: number;
  items: BillItem[];
}

export interface UpdateBillRequest {
  items: BillItem[];
}

export interface AddBillItemRequest {
  menu_item_id: string;
  name: string;
  price: number;
  quantity: number;
  options: MenuItemOption[];
}

export interface BillResponse {
  bill: Bill;
  items: BillItem[];
}

// Bill API functions
export const createBill = async (businessId: number, data: CreateBillRequest): Promise<BillResponse> => {
  const response = await axiosInstance.post(`/businesses/${businessId}/bills`, data);
  return response.data;
};

export const getBusinessBills = async (businessId: number): Promise<{ bills: Bill[] }> => {
  const response = await axiosInstance.get(`/businesses/${businessId}/bills`);
  return response.data;
};

export const getBill = async (billId: number): Promise<BillResponse> => {
  const response = await axiosInstance.get(`/bills/${billId}`);
  return response.data;
};

export const updateBill = async (billId: number, data: UpdateBillRequest): Promise<BillResponse> => {
  const response = await axiosInstance.post('/bills', data);
  return response.data;
};

export const addBillItem = async (billId: number, data: AddBillItemRequest): Promise<BillResponse> => {
  const response = await axiosInstance.post(`/bills/${billId}/items`, data);
  return response.data;
};

export const removeBillItem = async (billId: number, itemId: string): Promise<BillResponse> => {
  const response = await axiosInstance.delete(`/bills/${billId}/items/${itemId}`);
  return response.data;
};

export const closeBill = async (billId: number): Promise<BillResponse> => {
  const response = await axiosInstance.post(`/bills/${billId}/close`);
  return response.data;
};

// Guest API functions (no authentication required)
export const getTableByCode = async (code: string) => {
  const response = await axiosInstance.get(`/guest/table/${code}`);
  return response.data;
};

export const getOpenBillByTableCode = async (code: string): Promise<BillResponse> => {
  const response = await axiosInstance.get(`/tables/${code}/bills`);
  return response.data;
};

export const getBusinessByTableCode = async (code: string) => {
  const response = await axiosInstance.get(`/guest/table/${code}/business`);
  return response.data;
};

export const getMenuByTableCode = async (code: string) => {
  const response = await axiosInstance.get(`/guest/table/${code}/menu`);
  return response.data;
};

export const getTableStatusByCode = async (code: string) => {
  const response = await axiosInstance.get(`/guest/table/${code}/status`);
  return response.data;
};

export const getBillByNumber = async (billNumber: string): Promise<BillResponse> => {
  const response = await axiosInstance.get(`/guest/bill/${billNumber}`);
  return response.data;
};
