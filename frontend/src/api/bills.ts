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
  counter_id?: number;
  bill_number: string;
  notes: string;
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
  table_id?: number;
  counter_id?: number;
  notes?: string;
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
}

export interface BillWithItemsResponse {
  bill: Bill;
  items: any[];
}

// Bill API functions (Protected routes - require authentication)
export const createBill = async (businessId: number, data: CreateBillRequest): Promise<BillResponse> => {
  const response = await axiosInstance.post(`/inside/businesses/${businessId}/bills`, data);
  return response.data;
};

// Get all bills for a business
export const getBusinessBills = async (businessId: number): Promise<{ bills: Bill[] }> => {
  const response = await axiosInstance.get(`/inside/businesses/${businessId}/bills`);
  return response.data;
};

// Get only open bills for a business (for table filtering)
export const getOpenBusinessBills = async (businessId: number): Promise<{ bills: Bill[] }> => {
  const response = await axiosInstance.get(`/inside/businesses/${businessId}/bills/open`);
  return response.data;
};

export const getBill = async (billId: number): Promise<BillWithItemsResponse> => {
  const response = await axiosInstance.get(`/inside/bills/${billId}`);
  return response.data;
};

export const updateBill = async (billId: number, data: UpdateBillRequest): Promise<BillResponse> => {
  const response = await axiosInstance.put(`/inside/bills/${billId}`, data);
  return response.data;
};

export const addBillItem = async (billId: number, data: AddBillItemRequest): Promise<BillResponse> => {
  const response = await axiosInstance.post(`/inside/bills/${billId}/items`, data);
  return response.data;
};

export const removeBillItem = async (billId: number, itemId: string): Promise<BillResponse> => {
  const response = await axiosInstance.delete(`/inside/bills/${billId}/items/${itemId}`);
  return response.data;
};

export const closeBill = async (billId: number): Promise<BillResponse> => {
  const response = await axiosInstance.post(`/inside/bills/${billId}/close`);
  return response.data;
};

// Payment control functions for staff
export interface MarkPaidRequest {
  payment_method: 'cash' | 'card' | 'crypto';
  amount_paid: number;
  tip_amount?: number;
  notes?: string;
}

export const markBillAsPaid = async (billId: number, data: MarkPaidRequest): Promise<BillResponse> => {
  const response = await axiosInstance.post(`/inside/bills/${billId}/mark-paid`, data);
  return response.data;
};

export const approveCashPayment = async (billId: number, data: MarkPaidRequest): Promise<BillResponse> => {
  const response = await axiosInstance.post(`/inside/bills/${billId}/approve-cash`, data);
  return response.data;
};

// Update bill with crypto payment details
export interface UpdateBillPaymentRequest {
  transaction_hash: string;
  tip_amount?: number;
  payment_method: 'crypto';
  blockchain_network?: string;
}

export const createOnChainBill = async (billId: number, billData: {
  business_address: string;
  total_amount: number;
}): Promise<any> => {
  const response = await axiosInstance.post(`/guest/bills/${billId}/create-onchain`, billData);
  return response.data;
};

export const updateBillPayment = async (billId: number, paymentData: {
  transaction_hash: string;
  amount_paid: number;
  tip_amount: number;
  payment_method: string;
  blockchain_network: string;
}): Promise<any> => {
  const response = await axiosInstance.post(`/guest/bills/${billId}/crypto-payment`, paymentData);
  return response.data;
};

// Guest API functions (no authentication required)
export const getTableByCode = async (code: string) => {
  const response = await axiosInstance.get(`/guest/table/${code}`);
  return response.data;
};


// Get open bill by table code (Public route for guests)
export const getOpenBillByTableCode = async (tableCode: string): Promise<BillWithItemsResponse> => {
  const response = await axiosInstance.get(`/guest/table/${tableCode}/bill`);
  return response.data;
};

// Create new bill by table code (Public route for guests)
export const createBillByTableCode = async (tableCode: string): Promise<BillResponse> => {
  const response = await axiosInstance.post(`/guest/table/${tableCode}/bill`);
  return response.data;
};

export const createGuestKitchenOrder = async (
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
  const response = await axiosInstance.post(`/guest/table/${tableCode}/kitchen/order`, orderData);
  return response.data;
};

export const getBusinessByTableCode = async (code: string) => {
  const response = await axiosInstance.get(`/guest/table/${code}/business`);
};

export const getMenuByTableCode = async (code: string, language?: string) => {
  const params = language ? { language } : {};
  const response = await axiosInstance.get(`/guest/table/${code}/menu`, { params });
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
