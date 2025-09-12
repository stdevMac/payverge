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

// Hook for using splitting API with error handling
export const useSplittingAPI = () => {
  const getSplitOptions = async (billId: number) => {
    try {
      return await SplittingAPI.getSplitOptions(billId);
    } catch (error) {
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

  return {
    getSplitOptions,
    calculateEqualSplit,
    calculateCustomSplit,
    calculateItemSplit,
    validateSplit,
  };
};

export default SplittingAPI;
