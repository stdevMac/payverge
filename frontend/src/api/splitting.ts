import { axiosInstance as apiClient } from './tools/instance';

export interface SplitOptions {
  bill_id: number;
  subtotal: number;
  tax_amount: number;
  service_fee_amount: number;
  total_amount: number;
  items: Array<{
    id: string;
    name: string;
    price: number;
    quantity: number;
    subtotal: number;
  }>;
  max_people: number;
}

export interface SplitResult {
  bill_id: number;
  split_method: 'equal' | 'custom' | 'items';
  total_amount: number;
  tip_amount: number;
  grand_total: number;
  people: Array<{
    person_id: string;
    name: string;
    base_amount: number;
    tax_amount: number;
    service_fee_amount: number;
    tip_amount: number;
    total_amount: number;
    items?: Array<{
      id: string;
      name: string;
      price: number;
      quantity: number;
      subtotal: number;
    }>;
  }>;
  created_at: string;
}

export interface EqualSplitRequest {
  bill_id: number;
  num_people: number;
  people: Record<string, string>; // person_id -> name
}

export interface CustomSplitRequest {
  bill_id: number;
  amounts: Record<string, number>; // person_id -> amount
  people: Record<string, string>; // person_id -> name
}

export interface ItemSplitRequest {
  bill_id: number;
  item_selections: Record<string, string[]>; // person_id -> item_ids
  people: Record<string, string>; // person_id -> name
}

export interface SplitValidationRequest {
  bill_id: number;
  split_method: 'equal' | 'custom' | 'items';
  data: EqualSplitRequest | CustomSplitRequest | ItemSplitRequest;
}

export interface SplitValidationResult {
  valid: boolean;
  errors: string[];
  warnings: string[];
  total_check: {
    expected: number;
    calculated: number;
    difference: number;
  };
}

export class SplittingAPI {
  /**
   * Get split options for a bill
   */
  static async getSplitOptions(billId: number): Promise<SplitOptions> {
    const response = await apiClient.get(`/bills/${billId}/split/options`);
    return response.data.result;
  }

  // Execute split payment coordination
  static async executeSplitPayment(billId: number, splitResult: SplitResult, paymentInfo: any) {
    const response = await apiClient.post(`/bills/${billId}/split/execute`, {
      split_result: splitResult,
      payment_info: paymentInfo,
    });
    return response.data;
  }

  // Get bill participants from blockchain
  static async getBillParticipants(billId: number) {
    const response = await apiClient.get(`/bills/${billId}/participants`);
    return response.data;
  }

  // Get specific participant info by address
  static async getParticipantInfo(billId: number, address: string) {
    const response = await apiClient.get(`/bills/${billId}/participants/${address}`);
    return response.data;
  }

  // Get enhanced bill summary with blockchain data
  static async getBillSummaryWithParticipants(billId: number) {
    const response = await apiClient.get(`/bills/${billId}/summary`);
    return response.data;
  }

  /**
   * Calculate equal split
   */
  static async calculateEqualSplit(request: EqualSplitRequest): Promise<SplitResult> {
    const response = await apiClient.post(`/bills/${request.bill_id}/split/equal`, request);
    return response.data.result;
  }

  /**
   * Calculate custom split
   */
  static async calculateCustomSplit(request: CustomSplitRequest): Promise<SplitResult> {
    const response = await apiClient.post(`/bills/${request.bill_id}/split/custom`, request);
    return response.data.result;
  }

  /**
   * Calculate item-based split
   */
  static async calculateItemSplit(request: ItemSplitRequest): Promise<SplitResult> {
    const response = await apiClient.post(`/bills/${request.bill_id}/split/items`, request);
    return response.data.result;
  }

  /**
   * Validate a split configuration
   */
  static async validateSplit(request: SplitValidationRequest): Promise<SplitValidationResult> {
    const response = await apiClient.post(`/bills/${request.bill_id}/split/validate`, request);
    return response.data.result;
  }
}

// Cache for split options to prevent repeated requests
const splitOptionsCache = new Map<number, { data: SplitOptions; timestamp: number }>();
const CACHE_DURATION = 30000; // 30 seconds

// Request throttling to prevent rapid successive calls
const pendingRequests = new Map<number, Promise<SplitOptions>>();

// Hook for using splitting API with error handling and caching
export const useSplittingAPI = () => {
  const getSplitOptions = async (billId: number) => {
    try {
      // Check cache first
      const cached = splitOptionsCache.get(billId);
      if (cached && Date.now() - cached.timestamp < CACHE_DURATION) {
        return cached.data;
      }

      // Check if there's already a pending request for this billId
      const pending = pendingRequests.get(billId);
      if (pending) {
        return await pending;
      }

      // Create new request and store it as pending
      const requestPromise = SplittingAPI.getSplitOptions(billId);
      pendingRequests.set(billId, requestPromise);

      try {
        const data = await requestPromise;
        
        // Cache the result
        splitOptionsCache.set(billId, {
          data,
          timestamp: Date.now()
        });
        
        return data;
      } finally {
        // Remove from pending requests
        pendingRequests.delete(billId);
      }
    } catch (error: any) {
      // Handle rate limiting specifically
      if (error?.response?.status === 429) {
        console.warn(`Rate limited for bill ${billId}, using cached data if available`);
        const cached = splitOptionsCache.get(billId);
        if (cached) {
          console.log('Returning stale cached data due to rate limiting');
          return cached.data;
        }
      }
      console.error('Failed to get split options:', error);
      throw error;
    }
  };

  const calculateEqualSplit = async (request: EqualSplitRequest) => {
    try {
      return await SplittingAPI.calculateEqualSplit(request);
    } catch (error) {
      console.error('Failed to calculate equal split:', error);
      throw error;
    }
  };

  const calculateCustomSplit = async (request: CustomSplitRequest) => {
    try {
      return await SplittingAPI.calculateCustomSplit(request);
    } catch (error) {
      console.error('Failed to calculate custom split:', error);
      throw error;
    }
  };

  const calculateItemSplit = async (request: ItemSplitRequest) => {
    try {
      return await SplittingAPI.calculateItemSplit(request);
    } catch (error) {
      console.error('Failed to calculate item split:', error);
      throw error;
    }
  };

  const validateSplit = async (request: SplitValidationRequest) => {
    try {
      return await SplittingAPI.validateSplit(request);
    } catch (error) {
      console.error('Failed to validate split:', error);
      throw error;
    }
  };

  const executeSplitPayment = async (billId: number, splitResult: SplitResult, paymentInfo: any) => {
    try {
      return await SplittingAPI.executeSplitPayment(billId, splitResult, paymentInfo);
    } catch (error) {
      console.error('Failed to execute split payment:', error);
      throw error;
    }
  };

  const getBillParticipants = async (billId: number) => {
    try {
      return await SplittingAPI.getBillParticipants(billId);
    } catch (error) {
      console.error('Failed to get bill participants:', error);
      throw error;
    }
  };

  const getParticipantInfo = async (billId: number, address: string) => {
    try {
      return await SplittingAPI.getParticipantInfo(billId, address);
    } catch (error) {
      console.error('Failed to get participant info:', error);
      throw error;
    }
  };

  const getBillSummaryWithParticipants = async (billId: number) => {
    try {
      return await SplittingAPI.getBillSummaryWithParticipants(billId);
    } catch (error) {
      console.error('Failed to get bill summary:', error);
      throw error;
    }
  };

  const clearSplitOptionsCache = (billId?: number) => {
    if (billId) {
      splitOptionsCache.delete(billId);
      pendingRequests.delete(billId);
    } else {
      splitOptionsCache.clear();
      pendingRequests.clear();
    }
  };

  return {
    getSplitOptions,
    calculateEqualSplit,
    calculateCustomSplit,
    calculateItemSplit,
    validateSplit,
    executeSplitPayment,
    getBillParticipants,
    getParticipantInfo,
    getBillSummaryWithParticipants,
    clearSplitOptionsCache,
  };
};

export default SplittingAPI;
