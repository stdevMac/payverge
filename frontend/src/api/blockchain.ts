import { axiosInstance as apiClient } from './tools/instance';

// Blockchain participant tracking interfaces
export interface Participant {
  address: string;
  paid_amount: number;
  payment_count: number;
  last_payment: number;
}

export interface BillSummary {
  success: boolean;
  bill: {
    id: number;
    bill_number: string;
    total_amount: number;
    paid_amount: number;
    remaining_amount: number;
    status: string;
  };
  blockchain?: {
    total_amount: number;
    paid_amount: number;
    participant_count: number;
    is_paid: boolean;
    remaining_amount: number;
  };
  participants?: string[];
  participant_count?: number;
}

export interface ParticipantsResponse {
  success: boolean;
  participants: string[];
  count: number;
}

export interface ParticipantInfoResponse {
  success: boolean;
  participant: Participant;
}

// API functions for blockchain integration
export const getBlockchainAPI = () => ({
  // Get all participants who have paid for a bill
  getBillParticipants: async (billId: number): Promise<ParticipantsResponse> => {
    const response = await apiClient.get(`/bills/${billId}/participants`);
    return response.data;
  },

  // Get specific participant information
  getParticipantInfo: async (billId: number, address: string): Promise<ParticipantInfoResponse> => {
    const response = await apiClient.get(`/bills/${billId}/participants/${address}`);
    return response.data;
  },

  // Get enhanced bill summary with blockchain data
  getBillSummary: async (billId: number): Promise<BillSummary> => {
    const response = await apiClient.get(`/bills/${billId}/summary`);
    return response.data;
  },

  // Execute split payment coordination
  executeSplitPayment: async (billId: number, splitResult: any, paymentInfo: any) => {
    const response = await apiClient.post(`/bills/${billId}/split/execute`, {
      split_result: splitResult,
      payment_info: paymentInfo,
    });
    return response.data;
  },
});

// Hook for blockchain API
export const useBlockchainAPI = () => {
  return getBlockchainAPI();
};

// Utility functions
export const formatAddress = (address: string): string => {
  if (!address || address.length < 10) return address;
  return `${address.slice(0, 6)}...${address.slice(-4)}`;
};

export const formatUSDCAmount = (amount: number): string => {
  // Convert from wei (6 decimals) to USD
  return (amount / 1000000).toFixed(2);
};

export const parseUSDCAmount = (amount: string): number => {
  // Convert from USD to wei (6 decimals)
  return Math.round(parseFloat(amount) * 1000000);
};
