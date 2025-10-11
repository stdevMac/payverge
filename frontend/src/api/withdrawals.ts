import { axiosInstance } from './tools/instance';

export interface WithdrawalHistory {
  id: number;
  business_id: number;
  transaction_hash: string;
  payment_amount: number;
  tip_amount: number;
  total_amount: number;
  withdrawal_address: string;
  blockchain_network: string;
  status: 'pending' | 'confirmed' | 'failed';
  created_at: string;
  updated_at: string;
  confirmed_at?: string;
}

export interface CreateWithdrawalRequest {
  transaction_hash: string;
  payment_amount: number;
  tip_amount: number;
  total_amount: number;
  withdrawal_address: string;
  blockchain_network: string;
}

export interface WithdrawalHistoryResponse {
  withdrawals: WithdrawalHistory[];
  pagination: {
    page: number;
    limit: number;
    total: number;
    pages: number;
  };
}

export interface UpdateWithdrawalStatusRequest {
  status: 'pending' | 'confirmed' | 'failed';
}

// Create a new withdrawal record
export const createWithdrawal = async (
  businessId: number,
  data: CreateWithdrawalRequest
): Promise<WithdrawalHistory> => {
  const response = await axiosInstance.post(
    `/inside/businesses/${businessId}/withdrawals`,
    data
  );
  return response.data;
};

// Get withdrawal history for a business
export const getWithdrawalHistory = async (
  businessId: number,
  page: number = 1,
  limit: number = 20
): Promise<WithdrawalHistoryResponse> => {
  const response = await axiosInstance.get(
    `/inside/businesses/${businessId}/withdrawals`,
    {
      params: { page, limit }
    }
  );
  return response.data;
};

// Get a specific withdrawal record
export const getWithdrawal = async (
  businessId: number,
  withdrawalId: number
): Promise<WithdrawalHistory> => {
  const response = await axiosInstance.get(
    `/inside/businesses/${businessId}/withdrawals/${withdrawalId}`
  );
  return response.data;
};

// Update withdrawal status (for blockchain confirmation)
export const updateWithdrawalStatus = async (
  businessId: number,
  withdrawalId: number,
  data: UpdateWithdrawalStatusRequest
): Promise<WithdrawalHistory> => {
  const response = await axiosInstance.put(
    `/inside/businesses/${businessId}/withdrawals/${withdrawalId}/status`,
    data
  );
  return response.data;
};
