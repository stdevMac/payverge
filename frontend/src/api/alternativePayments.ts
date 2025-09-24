import axios from 'axios';
import { 
  AlternativePayment, 
  AlternativePaymentRequest, 
  AlternativePaymentResponse, 
  BillPaymentBreakdown,
  PendingAlternativePayment,
  PaymentMethod 
} from '@/types/alternativePayments';

const API_BASE_URL = process.env.NEXT_PUBLIC_API_URL || 'http://localhost:8080';

// Create axios instance with default config
const api = axios.create({
  baseURL: API_BASE_URL,
  timeout: 30000,
  headers: {
    'Content-Type': 'application/json',
  },
});

// Add auth interceptor
api.interceptors.request.use((config) => {
  const token = localStorage.getItem('authToken');
  if (token) {
    config.headers.Authorization = `Bearer ${token}`;
  }
  return config;
});

// Response interceptor for error handling
api.interceptors.response.use(
  (response) => response,
  (error) => {
    console.error('Alternative Payments API Error:', error);
    throw error;
  }
);

export interface AlternativePaymentsAPI {
  // Business owner functions
  markAlternativePayment: (request: AlternativePaymentRequest) => Promise<AlternativePaymentResponse>;
  getPendingAlternativePayments: (billId: string) => Promise<PendingAlternativePayment[]>;
  
  // Guest/public functions
  getBillAlternativePayments: (billId: string) => Promise<AlternativePayment[]>;
  getBillPaymentBreakdown: (billId: string) => Promise<BillPaymentBreakdown>;
  
  // Real-time updates
  subscribeToBillUpdates: (billId: string, callback: (update: any) => void) => () => void;
}

/**
 * Mark an alternative payment (business owner only)
 */
export async function markAlternativePayment(
  request: AlternativePaymentRequest
): Promise<AlternativePaymentResponse> {
  try {
    const response = await api.post(
      `/api/v1/inside/bills/${request.billId}/alternative-payment`,
      {
        participant_address: request.participantAddress,
        amount: request.amount,
        payment_method: PaymentMethod[request.paymentMethod].toLowerCase(),
        business_confirmation: request.businessConfirmation
      }
    );

    return {
      success: true,
      transactionHash: response.data.transaction_hash,
      message: response.data.message || 'Alternative payment marked successfully',
      paymentBreakdown: response.data.payment_breakdown
    };
  } catch (error: any) {
    console.error('Error marking alternative payment:', error);
    return {
      success: false,
      message: error.response?.data?.error || 'Failed to mark alternative payment',
      paymentBreakdown: {
        totalAmount: '0',
        cryptoPaid: '0',
        alternativePaid: '0',
        remaining: '0',
        isComplete: false
      }
    };
  }
}

/**
 * Get pending alternative payments for a bill (business owner only)
 */
export async function getPendingAlternativePayments(
  billId: string
): Promise<PendingAlternativePayment[]> {
  try {
    const response = await api.get(`/api/v1/inside/bills/${billId}/pending-alternative-payments`);
    return response.data.pending_payments || [];
  } catch (error) {
    console.error('Error fetching pending alternative payments:', error);
    return [];
  }
}

/**
 * Get all alternative payments for a bill (public)
 */
export async function getBillAlternativePayments(
  billId: string
): Promise<AlternativePayment[]> {
  try {
    const response = await api.get(`/api/v1/bills/${billId}/alternative-payments`);
    return response.data.alternative_payments || [];
  } catch (error) {
    console.error('Error fetching bill alternative payments:', error);
    return [];
  }
}

/**
 * Get bill payment breakdown (public)
 */
export async function getBillPaymentBreakdown(
  billId: string
): Promise<BillPaymentBreakdown> {
  try {
    const response = await api.get(`/api/v1/bills/${billId}/payment-breakdown`);
    return response.data.breakdown || {
      totalAmount: '0',
      cryptoPaid: '0',
      alternativePaid: '0',
      remaining: '0',
      isComplete: false
    };
  } catch (error) {
    console.error('Error fetching bill payment breakdown:', error);
    return {
      totalAmount: '0',
      cryptoPaid: '0',
      alternativePaid: '0',
      remaining: '0',
      isComplete: false
    };
  }
}

/**
 * Request alternative payment (guest function)
 * This creates a pending payment request that business owner can confirm
 */
export async function requestAlternativePayment(
  billId: string,
  amount: string,
  paymentMethod: PaymentMethod,
  participantName?: string
): Promise<{ success: boolean; message: string; requestId?: string }> {
  try {
    const response = await api.post(`/api/v1/bills/${billId}/request-alternative-payment`, {
      amount,
      payment_method: PaymentMethod[paymentMethod].toLowerCase(),
      participant_name: participantName
    });

    return {
      success: true,
      message: 'Alternative payment request sent to business owner',
      requestId: response.data.request_id
    };
  } catch (error: any) {
    console.error('Error requesting alternative payment:', error);
    return {
      success: false,
      message: error.response?.data?.error || 'Failed to request alternative payment'
    };
  }
}

/**
 * WebSocket subscription for real-time bill updates
 */
export function subscribeToBillUpdates(
  billId: string, 
  callback: (update: any) => void
): () => void {
  const wsUrl = process.env.NEXT_PUBLIC_WS_URL || 'ws://localhost:8080/ws';
  
  try {
    const ws = new WebSocket(`${wsUrl}/bills/${billId}`);

    ws.onmessage = (event) => {
      try {
        const update = JSON.parse(event.data);
        callback(update);
      } catch (error) {
        console.error('Error parsing WebSocket message:', error);
      }
    };

    ws.onerror = (error) => {
      console.error('WebSocket error:', error);
    };

    // Return cleanup function
    return () => {
      ws.close();
    };
  } catch (error) {
    console.error('Failed to create WebSocket connection:', error);
    // Return a no-op cleanup function if WebSocket creation fails
    return () => {};
  }
}

// Export all functions as default API object
const alternativePaymentsAPI: AlternativePaymentsAPI = {
  markAlternativePayment,
  getPendingAlternativePayments,
  getBillAlternativePayments,
  getBillPaymentBreakdown,
  subscribeToBillUpdates
};

export default alternativePaymentsAPI;
