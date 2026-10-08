import { axiosInstance } from "@/api/tools/instance";
import { validatePaymentHistoryResponse } from './schemas/payments';
import type { Dollars } from '@/types/money';

export interface PaymentHistoryItem {
  id: number;
  bill_id: number;
  bill_number: string;
  table_name: string;
  payer_address: string;
  /** Payment amount in dollars (see wire contract in @/types/money). */
  amount: Dollars;
  /** Tip amount in dollars (see wire contract in @/types/money). */
  tip_amount: Dollars;
  currency: string;
  tx_hash: string;
  status: 'pending' | 'confirmed' | 'failed' | 'reversed' | 'refunded';
  /**
   * Canonical payment-method vocabulary value
   * (crypto|cross_chain|card|cash|wallet|other). See `@/lib/paymentMethod`.
   */
  method?: string;
  created_at: string;
  updated_at: string;
}

export interface PaymentHistoryQuery {
  period?: string;
  startDate?: string;
  endDate?: string;
  /** Server-side full-text search over bill number / table / method / tx hash. */
  q?: string;
  /**
   * Server-side method filter — must be a canonical Method key
   * (crypto|cross_chain|card|cash|wallet|other), never "stripe"/"manual"/"Online".
   */
  method?: string;
  /** Server-side status filter (e.g. "confirmed", "pending"). */
  status?: string;
  /** 1-based page. Omit for the legacy full-window array. */
  page?: number;
  /** Page size (server clamps to <=100). */
  pageSize?: number;
}

/** Paginated envelope returned when a `page` is requested (BE-first). */
export interface PaymentHistoryPage {
  items: PaymentHistoryItem[];
  total: number;
  page: number;
  page_size: number;
  /**
   * Canonical Methods that have ≥1 row for this business+window. The method
   * filter dropdown is generated from this list so it cannot offer a key that
   * matches zero rows (finding 62).
   */
  available_methods?: string[];
}

const buildPaymentHistoryParams = (
  query: PaymentHistoryQuery = {},
  format?: 'csv' | 'json',
  lang?: string,
) => {
  const params: Record<string, string> = {};

  if (query.startDate) {
    params.start_date = query.startDate;
  }

  if (query.endDate) {
    params.end_date = query.endDate;
  }

  if (!query.startDate && !query.endDate) {
    params.period = query.period ?? 'month';
  }

  if (query.q && query.q.trim()) {
    params.q = query.q.trim();
  }

  if (query.method && query.method !== 'all') {
    params.method = query.method;
  }

  if (query.status && query.status !== 'all') {
    params.status = query.status;
  }

  if (query.page && query.page > 0) {
    params.page = String(query.page);
    if (query.pageSize && query.pageSize > 0) {
      params.page_size = String(query.pageSize);
    }
  }

  if (lang) {
    params.lang = lang;
  }

  if (format) {
    params.format = format;
  }

  return params;
};

// Get a single server-paginated page of payment history. Always sends `page`, so
// the backend returns the {items,total,page,page_size} envelope.
export const getPaymentHistoryPage = async (
  businessId: number,
  query: PaymentHistoryQuery = {}
): Promise<PaymentHistoryPage> => {
  const response = await axiosInstance.get(
    `/inside/businesses/${businessId}/payments/history`,
    {
      params: buildPaymentHistoryParams({ ...query, page: query.page && query.page > 0 ? query.page : 1 })
    }
  );
  const data = response.data ?? {};
  const items = validatePaymentHistoryResponse(Array.isArray(data.items) ? data.items : []);
  const available =
    Array.isArray(data.available_methods)
      ? (data.available_methods as unknown[]).filter(
          (m): m is string => typeof m === "string" && m.length > 0,
        )
      : [];
  return {
    items,
    total: typeof data.total === 'number' ? data.total : items.length,
    page: typeof data.page === 'number' ? data.page : query.page ?? 1,
    page_size: typeof data.page_size === 'number' ? data.page_size : query.pageSize ?? items.length,
    available_methods: available,
  };
};

// Export payments data
export const exportPayments = async (
  businessId: number,
  query: PaymentHistoryQuery = {},
  format: 'csv' | 'json' = 'csv',
  lang?: string,
): Promise<Blob> => {
  const response = await axiosInstance.get(
    `/inside/businesses/${businessId}/payments/export`,
    {
      params: buildPaymentHistoryParams(query, format, lang),
      responseType: 'blob'
    }
  );
  return response.data;
};
