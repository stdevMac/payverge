import { axiosInstance } from './index';

export interface PaymentWebhookData {
  billId: number;
  transactionHash: string;
  amount: number;
  tipAmount?: number;
  payerAddress: string;
  timestamp: string;
}

export interface PaymentHistoryItem {
  id: number;
  bill_id: number;
  bill_number: string;
  table_name: string;
  payer_address: string;
  amount: number;
  tip_amount: number;
  tx_hash: string;
  status: 'pending' | 'confirmed' | 'failed';
  created_at: string;
  updated_at: string;
}

// Notify backend of payment completion
export const notifyPaymentWebhook = async (data: PaymentWebhookData): Promise<void> => {
  await axiosInstance.post('/payments/webhook', data);
};

// Get payment history for a business
export const getPaymentHistory = async (businessId: number): Promise<PaymentHistoryItem[]> => {
  // Since there's no dedicated payments endpoint, return empty array for now
  // TODO: Implement proper payments endpoint or use analytics data
  console.warn('Payment history endpoint not implemented, returning empty array');
  return [];
};

// Export payments data
export const exportPayments = async (
  businessId: number,
  period: string = 'custom',
  format: 'csv' | 'json' = 'csv'
): Promise<Blob> => {
  const response = await axiosInstance.get(
    `/inside/businesses/${businessId}/reports/export`,
    {
      params: { period, format },
      responseType: 'blob'
    }
  );
  return response.data;
};
