import { getPublicConfig } from "@/config/publicConfig";
import { axiosInstance as apiClient } from './tools/instance';
import { logError } from '@/utils/errorLogger';
import { asDollars, type Dollars } from '@/types/money';

// Split API money fields are in DOLLARS (see internal/handlers/splitting.go).
export interface SplitOptions {
  success: boolean;
  bill: {
    bill_number: string;
    total_amount: Dollars;
    paid_amount: Dollars;
    remaining_amount: Dollars;
    subtotal: Dollars;
    tax_amount: Dollars;
    service_fee_amount: Dollars;
    status: string;
  };
  items: Array<{
    id: string;
    name: string;
    price: Dollars;
    quantity: number;
    subtotal: Dollars;
    item_type?: string;
  }>;
  split_options: {
    equal: {
      available: boolean;
      description: string;
      min_people: number;
      max_people: number;
    };
    custom: {
      available: boolean;
      description: string;
      min_people: number;
      max_people: number;
    };
    items: {
      available: boolean;
      description: string;
      min_people: number;
      max_people: number;
      total_items: number;
    };
  };
}

export interface SplitResult {
  split_method: 'equal' | 'custom' | 'items';
  total_amount: Dollars;
  tip_amount: Dollars;
  grand_total: Dollars;
  people: Array<{
    person_id: string;
    name: string;
    base_amount: Dollars;
    tax_amount: Dollars;
    service_fee_amount: Dollars;
    tip_amount: Dollars;
    total_amount: Dollars;
    items?: Array<{
      id: string;
      name: string;
      price: Dollars;
      quantity: number;
      subtotal: Dollars;
    }>;
  }>;
  created_at: string;
  breakdown?: Record<string, unknown>;
}

export interface SplitShare {
  id: number;
  display_name?: string;
  mode: 'equal' | 'custom' | 'items';
  amount: Dollars;
  amount_cents: number;
  tip_amount?: Dollars;
  tip_cents?: number;
  status: 'held' | 'settled' | 'released' | 'failed';
  hold_expires_at?: string | null;
  tender?: string;
  settled_at?: string | null;
  released_at?: string | null;
  claimed_item_ids?: string[];
  claimed_fractions?: Record<string, string>;
}

export interface SplitState {
  bill_number: string;
  status: string;
  total_amount: Dollars;
  total_cents: number;
  paid_amount: Dollars;
  paid_cents: number;
  held_amount: Dollars;
  held_cents: number;
  available_amount: Dollars;
  available_cents: number;
  updated_at: string;
  shares: SplitShare[];
}

export interface SplitHoldRequest {
  mode: 'equal' | 'custom' | 'items';
  amount?: Dollars;
  cover_remaining?: boolean;
  num_people?: number;
  shares_covered?: number;
  display_name?: string;
  idempotency_key?: string;
  claimed_item_ids?: string[];
  claimed_fractions?: Record<string, string>;
}

export interface SplitHoldResponse {
  success: boolean;
  share: SplitShare;
  state: SplitState;
}

export interface ExecuteHeldShareRequest {
  share_id: number;
  payment_method: string;
  transaction_hash?: string;
  payer_address?: string;
  tip_amount?: Dollars;
  idempotency_key?: string;
  source_chain?: string;
  source_token?: string;
  lifi_route_id?: string;
}

export interface ExecuteHeldShareResponse {
  success: boolean;
  applied: boolean;
  share: SplitShare;
  bill_status: string;
  remaining_amount: Dollars;
  remaining_cents: number;
}

interface SplitShareReceiptItem {
  id: string;
  name: string;
  fraction: string;
  quantity: number;
  unit_price: Dollars;
  unit_price_cents: number;
  subtotal: Dollars;
  subtotal_cents: number;
}

export interface SplitShareReceipt {
  share_id: number;
  bill_number: string;
  display_name?: string;
  mode: 'equal' | 'custom' | 'items';
  status: 'settled';
  tender?: string;
  subtotal: Dollars;
  subtotal_cents: number;
  tax: Dollars;
  tax_cents: number;
  service_fee: Dollars;
  service_fee_cents: number;
  amount: Dollars;
  amount_cents: number;
  tip_amount: Dollars;
  tip_cents: number;
  grand_total: Dollars;
  grand_total_cents: number;
  items: SplitShareReceiptItem[];
  settled_at?: string | null;
}

interface SplitShareReceiptResponse {
  success: boolean;
  receipt: SplitShareReceipt;
}

function formatDollarsForSplit(amount: Dollars): string {
  return Number(amount).toFixed(2);
}

function newSplitRequestId(): string {
  if (typeof crypto !== 'undefined' && 'randomUUID' in crypto) {
    return crypto.randomUUID();
  }
  return `split-${Date.now()}-${Math.random().toString(36).slice(2)}`;
}

export function getSplitEventsURL(billToken: string): string {
  const base = getPublicConfig().apiUrl;
  return `${base}/guest/bill/${encodeURIComponent(billToken)}/split/events`;
}

// The split calculators address a bill the same way every other guest route in
// this file does: with the bill's unguessable capability token (public_token).
// `/guest/bill/:bill_token/split/*` resolves solely through
// `bills.public_token = ?`, so a display bill number can only ever 404 there —
// and it must stay that way, since a guessable number is not an authorization.
export interface EqualSplitRequest {
  bill_token: string;
  num_people: number;
  people: Record<string, string>; // person_id -> name
}

export interface CustomSplitRequest {
  bill_token: string;
  amounts: Record<string, Dollars>; // person_id -> amount in dollars
  people: Record<string, string>; // person_id -> name
}

export interface ItemSplitRequest {
  bill_token: string;
  item_selections: Record<string, string[]>; // person_id -> item_ids
  people: Record<string, string>; // person_id -> name
}

export interface SplitValidationRequest {
  bill_token: string;
  split_method: 'equal' | 'custom' | 'items';
  data: EqualSplitRequest | CustomSplitRequest | ItemSplitRequest;
}

export interface SplitValidationResult {
  valid: boolean;
  errors: string[];
  warnings: string[];
  total_check: {
    expected: Dollars;
    calculated: Dollars;
    difference: Dollars;
  };
}

export class SplittingAPI {
  /**
   * Get split options for a bill
   */
  static async getSplitOptions(billToken: string): Promise<SplitOptions> {
    const response = await apiClient.get(`/guest/bill/${billToken}/split/options`);
    return response.data;
  }

  static async getSplitState(billToken: string): Promise<SplitState> {
    const response = await apiClient.get(`/guest/bill/${billToken}/split/state`);
    return response.data.state;
  }

  static async getMySplitShares(billToken: string): Promise<SplitShare[]> {
    const response = await apiClient.get(`/guest/bill/${billToken}/split/my-shares`);
    return response.data.shares ?? [];
  }

  static async releaseHeldShare(
    billToken: string,
    shareID: number,
  ): Promise<SplitHoldResponse> {
    const response = await apiClient.post(
      `/guest/bill/${billToken}/split/shares/${shareID}/release`,
    );
    return response.data;
  }

  static async createSplitHold(
    billToken: string,
    request: SplitHoldRequest,
  ): Promise<SplitHoldResponse> {
    const payload: Record<string, unknown> = {
      mode: request.mode,
    };
    if (request.amount !== undefined) {
      payload.amount = formatDollarsForSplit(request.amount);
    }
    if (request.cover_remaining !== undefined) payload.cover_remaining = request.cover_remaining;
    if (request.num_people !== undefined) payload.num_people = request.num_people;
    if (request.shares_covered !== undefined) payload.shares_covered = request.shares_covered;
    if (request.display_name) payload.display_name = request.display_name;
    if (request.claimed_item_ids) payload.claimed_item_ids = request.claimed_item_ids;
    if (request.claimed_fractions) payload.claimed_fractions = request.claimed_fractions;

    const response = await apiClient.post(
      `/guest/bill/${billToken}/split/holds`,
      payload,
      { headers: { 'X-Request-Id': request.idempotency_key ?? newSplitRequestId() } },
    );
    return response.data;
  }

  // Execute split payment coordination
  static async executeSplitPayment(
    billToken: string,
    splitResult: SplitResult,
    paymentInfo: Record<string, unknown>,
  ) {
    const response = await apiClient.post(`/guest/bill/${billToken}/split/execute`, {
      split_result: splitResult,
      payment_info: paymentInfo,
    });
    return response.data;
  }

  static async executeHeldShare(
    billToken: string,
    request: ExecuteHeldShareRequest,
  ): Promise<ExecuteHeldShareResponse> {
    const payload: Record<string, unknown> = {
      share_id: request.share_id,
      payment_method: request.payment_method,
    };
    if (request.transaction_hash) payload.transaction_hash = request.transaction_hash;
    if (request.payer_address) payload.payer_address = request.payer_address;
    if (request.tip_amount !== undefined) payload.tip_amount = formatDollarsForSplit(request.tip_amount);
    if (request.idempotency_key) payload.idempotency_key = request.idempotency_key;
    if (request.source_chain) payload.source_chain = request.source_chain;
    if (request.source_token) payload.source_token = request.source_token;
    if (request.lifi_route_id) payload.lifi_route_id = request.lifi_route_id;

    const response = await apiClient.post(
      `/guest/bill/${billToken}/split/execute`,
      payload,
      { headers: { 'X-Request-Id': request.idempotency_key ?? newSplitRequestId() } },
    );
    return response.data;
  }

  static async getSplitShareReceipt(
    billToken: string,
    shareID: number,
  ): Promise<SplitShareReceipt> {
    const response = await apiClient.get(
      `/guest/bill/${billToken}/split/shares/${shareID}/receipt`,
    );
    return (response.data as SplitShareReceiptResponse).receipt;
  }

  /**
   * Calculate equal split
   */
  static async calculateEqualSplit(request: EqualSplitRequest): Promise<SplitResult> {
    const response = await apiClient.post(
      `/guest/bill/${request.bill_token}/split/equal`,
      {
        num_people: request.num_people,
        people: request.people,
      },
    );
    return response.data.result;
  }

  /**
   * Calculate custom split
   */
  static async calculateCustomSplit(request: CustomSplitRequest): Promise<SplitResult> {
    const response = await apiClient.post(
      `/guest/bill/${request.bill_token}/split/custom`,
      {
        amounts: request.amounts,
        people: request.people,
      },
    );
    return response.data.result;
  }

  /**
   * Calculate item-based split
   */
  static async calculateItemSplit(request: ItemSplitRequest): Promise<SplitResult> {
    const response = await apiClient.post(
      `/guest/bill/${request.bill_token}/split/items`,
      {
        item_selections: request.item_selections,
        people: request.people,
      },
    );
    return response.data.result;
  }

  /**
   * Validate a split configuration
   */
  static async validateSplit(request: SplitValidationRequest): Promise<SplitValidationResult> {
    const payload: Record<string, unknown> = {
      method: request.split_method,
    };

    if (request.split_method === 'equal') {
      const data = request.data as EqualSplitRequest;
      payload.num_people = data.num_people;
      payload.people = data.people;
    } else if (request.split_method === 'custom') {
      const data = request.data as CustomSplitRequest;
      payload.amounts = data.amounts;
      payload.people = data.people;
    } else {
      const data = request.data as ItemSplitRequest;
      payload.item_selections = data.item_selections;
      payload.people = data.people;
    }

    const response = await apiClient.post(
      `/guest/bill/${request.bill_token}/split/validate`,
      payload,
    );

    return {
      valid: Boolean(response.data.valid),
      errors: response.data.error ? [response.data.error] : [],
      warnings: [],
      total_check: {
        expected: asDollars(response.data.result?.total_amount ?? 0),
        calculated: asDollars(
          response.data.result?.grand_total ??
            response.data.result?.total_amount ??
            0,
        ),
        difference: asDollars(0),
      },
    };
  }
}

// Cache for split options to prevent repeated requests
const splitOptionsCache = new Map<string, { data: SplitOptions; timestamp: number }>();
const CACHE_DURATION = 30000; // 30 seconds

// Request throttling to prevent rapid successive calls
const pendingRequests = new Map<string, Promise<SplitOptions>>();

// Hook for using splitting API with error handling and caching
export const useSplittingAPI = () => {
  const getSplitOptions = async (billToken: string) => {
    try {
      // Check cache first
      const cached = splitOptionsCache.get(billToken);
      if (cached && Date.now() - cached.timestamp < CACHE_DURATION) {
        return cached.data;
      }

      // Check if there's already a pending request for this billToken
      const pending = pendingRequests.get(billToken);
      if (pending) {
        return await pending;
      }

      // Create new request and store it as pending
      const requestPromise = SplittingAPI.getSplitOptions(billToken);
      pendingRequests.set(billToken, requestPromise);

      try {
        const data = await requestPromise;
        
        // Cache the result
        splitOptionsCache.set(billToken, {
          data,
          timestamp: Date.now()
        });
        
        return data;
      } finally {
        // Remove from pending requests
        pendingRequests.delete(billToken);
      }
    } catch (error: unknown) {
      // Handle rate limiting specifically
      if (
        error &&
        typeof error === "object" &&
        "response" in error &&
        typeof error.response === "object" &&
        error.response &&
        "status" in error.response &&
        error.response.status === 429
      ) {
        console.warn(`Rate limited for bill ${billToken}, using cached data if available`);
        const cached = splitOptionsCache.get(billToken);
        if (cached) {
          return cached.data;
        }
      }
      void logError(error instanceof Error ? error : String(error), 'splitting', 'getSplitOptions');
      console.error('Failed to get split options:', error);
      throw error;
    }
  };

  const calculateEqualSplit = async (request: EqualSplitRequest) => {
    try {
      return await SplittingAPI.calculateEqualSplit(request);
    } catch (error) {
      void logError(error instanceof Error ? error : String(error), 'splitting', 'calculateEqualSplit');
      console.error('Failed to calculate equal split:', error);
      throw error;
    }
  };

  const getSplitState = async (billToken: string) => {
    try {
      return await SplittingAPI.getSplitState(billToken);
    } catch (error) {
      void logError(error instanceof Error ? error : String(error), 'splitting', 'getSplitState');
      console.error('Failed to get split state:', error);
      throw error;
    }
  };

  const getMySplitShares = async (billToken: string) => {
    try {
      return await SplittingAPI.getMySplitShares(billToken);
    } catch (error) {
      void logError(error instanceof Error ? error : String(error), 'splitting', 'getMySplitShares');
      console.error('Failed to get guest split shares:', error);
      throw error;
    }
  };

  const releaseHeldShare = async (billToken: string, shareID: number) => {
    try {
      return await SplittingAPI.releaseHeldShare(billToken, shareID);
    } catch (error) {
      void logError(error instanceof Error ? error : String(error), 'splitting', 'releaseHeldShare');
      console.error('Failed to release held split share:', error);
      throw error;
    }
  };

  const createSplitHold = async (billToken: string, request: SplitHoldRequest) => {
    try {
      return await SplittingAPI.createSplitHold(billToken, request);
    } catch (error) {
      void logError(error instanceof Error ? error : String(error), 'splitting', 'createSplitHold');
      console.error('Failed to create split hold:', error);
      throw error;
    }
  };

  const executeHeldShare = async (
    billToken: string,
    request: ExecuteHeldShareRequest,
  ) => {
    try {
      return await SplittingAPI.executeHeldShare(billToken, request);
    } catch (error) {
      void logError(error instanceof Error ? error : String(error), 'splitting', 'executeHeldShare');
      console.error('Failed to execute held split share:', error);
      throw error;
    }
  };

  const getSplitShareReceipt = async (billToken: string, shareID: number) => {
    try {
      return await SplittingAPI.getSplitShareReceipt(billToken, shareID);
    } catch (error) {
      void logError(error instanceof Error ? error : String(error), 'splitting', 'getSplitShareReceipt');
      console.error('Failed to get split share receipt:', error);
      throw error;
    }
  };

  const calculateCustomSplit = async (request: CustomSplitRequest) => {
    try {
      return await SplittingAPI.calculateCustomSplit(request);
    } catch (error) {
      void logError(error instanceof Error ? error : String(error), 'splitting', 'calculateCustomSplit');
      console.error('Failed to calculate custom split:', error);
      throw error;
    }
  };

  const calculateItemSplit = async (request: ItemSplitRequest) => {
    try {
      return await SplittingAPI.calculateItemSplit(request);
    } catch (error) {
      void logError(error instanceof Error ? error : String(error), 'splitting', 'calculateItemSplit');
      console.error('Failed to calculate item split:', error);
      throw error;
    }
  };

  const validateSplit = async (request: SplitValidationRequest) => {
    try {
      return await SplittingAPI.validateSplit(request);
    } catch (error) {
      void logError(error instanceof Error ? error : String(error), 'splitting', 'validateSplit');
      console.error('Failed to validate split:', error);
      throw error;
    }
  };

  const executeSplitPayment = async (
    billToken: string,
    splitResult: SplitResult,
    paymentInfo: Record<string, unknown>,
  ) => {
    try {
      return await SplittingAPI.executeSplitPayment(billToken, splitResult, paymentInfo);
    } catch (error) {
      void logError(error instanceof Error ? error : String(error), 'splitting', 'executeSplitPayment');
      console.error('Failed to execute split payment:', error);
      throw error;
    }
  };

  const clearSplitOptionsCache = (billToken?: string) => {
    if (billToken) {
      splitOptionsCache.delete(billToken);
      pendingRequests.delete(billToken);
    } else {
      splitOptionsCache.clear();
      pendingRequests.clear();
    }
  };

  return {
    getSplitOptions,
    getSplitState,
    getMySplitShares,
    releaseHeldShare,
    createSplitHold,
    executeHeldShare,
    getSplitShareReceipt,
    calculateEqualSplit,
    calculateCustomSplit,
    calculateItemSplit,
    validateSplit,
    executeSplitPayment,
    clearSplitOptionsCache,
  };
};
