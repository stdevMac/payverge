import { axiosInstance } from "./tools/instance";
import { getPublicConfig } from "@/config/publicConfig";
import type { Business, MenuCategory, Offer, Bundle } from "./business";
import type { Dollars } from "@/types/money";
import { fetchAllPages } from "./pagination";
import { getApiErrorData, getApiErrorStatus } from "@/utils/apiError";
import type {
  FetchAllPagesResult,
  PaginatedResponse,
  PaginationOptions,
} from "./pagination";

type BillStatus =
  | "open"
  | "partial"
  | "paid"
  | "closed"
  | "voided"
  | "abandoned";

// Bill interfaces
//
// Money-field convention: every price/amount on the Bill API is in DOLLARS
// (the backend's MarshalJSON emits dollars from int64 cents). Fields are
// typed as `Dollars` (see @/types/money) so mixing with Stripe-native cents
// fields or raw arithmetic surfaces as a compile error at call sites.
export interface BillItem {
  id: string;
  menu_item_id: string;
  menu_item?: {
    name?: string;
  };
  name: string;
  price: Dollars;
  quantity: number;
  options: MenuItemOption[];
  item_type?: "menu_item" | "bundle" | "bundle_item" | "discount";
  itemType?: "menu_item" | "bundle" | "bundle_item" | "discount";
  bundle_id?: number;
  bundleId?: number;
  parent_bundle_id?: number;
  parentBundleId?: number;
  bundle_occurrence_id?: string;
  bundleOccurrenceId?: string;
  source_offer_id?: number;
  sourceOfferId?: number;
  /** Order that produced this line (promo discounts are per-order cohorts). */
  order_id?: number;
  orderId?: number;
  subtotal: Dollars;
}

interface MenuItemOption {
  name: string;
  price: Dollars;
}

// Payment — partial view of the backend Payment row, only what the bill modal
// needs to render the refund picker (IMP-15). Amount/tip are in DOLLARS
// (matches the Bill JSON wire contract via models_json.go).
export interface Payment {
  id: number;
  bill_id: number;
  payer_address?: string;
  amount: Dollars;
  tip_amount: Dollars;
  currency: string;
  tx_hash?: string;
  status:
    | "pending"
    | "confirmed"
    | "failed"
    | "reversed"
    | "refund_pending"
    | "refunded";
  payment_method?: string;
  confirmed_at?: string;
  reversed_at?: string;
  created_at: string;
  updated_at: string;
}

export interface AlternativeBillPayment {
  id: number;
  bill_id: number;
  participant_address?: string;
  participant_name?: string;
  amount: Dollars;
  payment_method: "cash" | "card" | "venmo" | "other" | string;
  status:
    | "pending"
    | "confirmed"
    | "failed"
    | "refunded"
    | "expired"
    | "cancelled"
    | "rejected";
  confirmed_by?: string;
  confirmed_at?: string;
  resolved_by?: string;
  resolution_reason?: string;
  resolved_at?: string;
  created_at: string;
  updated_at: string;
  // Tip captured with this tender, emitted by AlternativePayment.MarshalJSON as
  // CENTS (models_json.go: tip_amount_cents). Present and 0 when there is no
  // tip. Rendered in the ledger + refund picker so the true cash returned is
  // visible and matches what RefundBillAlternativePayment reverses (R3-BP-5,
  // ties to R3-BP-1). Divide by 100 to get dollars.
  tip_amount_cents?: number;
}

export interface Bill {
  id: number;
  business_id: number;
  table_id: number;
  counter_id?: number;
  bill_number: string;
  /** Unguessable guest capability for /guest/bill routes (backend ≥ mig 000128). */
  public_token?: string;
  notes: string;
  /**
   * Line array on the wire (#771). Empty snapshots are omitted. Legacy
   * quoted JSON strings are still accepted by {@link parseBillItems}.
   */
  items?: string | BillItem[];
  // Count of live bill_items rows, projected by the list endpoints. The
  // legacy `items` JSON snapshot is empty for bills whose items live in
  // bill_items, so list UIs must prefer this and only fall back to parsing
  // `items` when it's 0/absent (audit G-03).
  item_count?: number;
  /** Sum of quantities for kitchen-preparable menu and bundle-child rows. */
  physical_item_quantity?: number;
  subtotal: Dollars;
  tax_amount: Dollars;
  service_fee_amount: Dollars;
  total_amount: Dollars;
  paid_amount: Dollars;
  remaining?: Dollars;
  tip_amount: Dollars;
  // Loyalty redemption baked into total_amount by the backend (RedeemPoints).
  // Emitted by Bill.MarshalJSON as dollars (field `loyalty_discount`, not the
  // struct tag `loyalty_discount_cents`). Present and 0 on bills with no
  // redemption; the UI renders a "Loyalty discount" line only when > 0 so the
  // itemized totals reconcile with the reduced total on reload / operator view.
  loyalty_discount?: Dollars;
  currency: string;
  status: BillStatus;
  settlement_address: string;
  tipping_address: string;
  created_at: string;
  updated_at: string;
  closed_at?: string;
  // Friendly table name. The paginated bill-list endpoints use the BillListRow
  // projection, which carries the table's name as a FLAT `table_name` (populated
  // via LEFT JOIN tables) rather than a nested `table` relation. Prefer this on
  // list views so the operator sees "Window 1" instead of the raw "Table 36".
  table_name?: string;
  // Optional preloaded relation. Endpoints that hydrate the full Bill (e.g.
  // GetBillByID) include the nested `table`; list endpoints use `table_name`
  // above. Nullable because counter-only bills don't have a table.
  table?: { id: number; name: string; table_code: string } | null;
  // GetBillByID preloads Payments so the bill-details modal can render the
  // refund picker (IMP-15) without an extra round-trip. Optional because
  // most list endpoints omit the relation to keep responses small.
  payments?: Payment[];
  alternative_payments?: AlternativeBillPayment[];
  // Fiscal customer identity (AFIP/ARCA). Nullable — present and null until set
  // at checkout. The doc number is customer PII; do not log it.
  fiscal_customer_doc_type?: string | null;
  fiscal_customer_doc_number?: string | null;
  fiscal_customer_tax_condition?: string | null;
  fiscal_customer_name?: string | null;
  fiscal_customer_email?: string | null;
}

/**
 * The unguessable capability guests put in /guest/bill/:id URLs. A display
 * bill number is never a valid substitute: sending it recreates the split and
 * checkout 404 loop and weakens the endpoint's capability boundary.
 *
 * Prefer {@link tryGuestBillRef} during render so a missing token becomes a
 * recoverable empty state instead of an uncaught render throw
 * (PV-LIVE-20260720-001). Keep this throwing helper for call sites that must
 * fail closed before issuing a network request.
 */
export function tryGuestBillRef(
  bill: Pick<Bill, "bill_number" | "public_token">,
): string | null {
  const token = bill.public_token?.trim();
  return token || null;
}

export function guestBillRef(
  bill: Pick<Bill, "bill_number" | "public_token">,
): string {
  const token = tryGuestBillRef(bill);
  if (!token) {
    throw new Error("Bill is missing public access token");
  }
  return token;
}

export interface BillHistoryEvent {
  id: number;
  bill_id: number;
  business_id: number;
  event_type:
    | "bill.created"
    | "bill.updated"
    | "bill.closed"
    | "bill_item.added"
    | "bill_item.removed"
    | "bill_item.voided"
    | "bill_item.quantity_updated"
    | "order.approved"
    | "order.cancelled"
    | string;
  actor?: string;
  reason?: string;
  order_id?: number;
  order_number?: string;
  bill_item_id?: string;
  item_name?: string;
  details?: Record<string, unknown>;
  created_at: string;
}

export interface CreateBillRequest {
  table_id?: number;
  counter_id?: number;
  notes?: string;
  items: BillItem[];
}

/** Returns the bill an operator should open after losing an occupancy race. */
export function getActiveBillConflictID(error: unknown): number | null {
  if (getApiErrorStatus(error) !== 409) return null;
  const data = getApiErrorData(error);
  if (typeof data !== "object" || data === null) return null;
  const activeBillID = (data as { active_bill_id?: unknown }).active_bill_id;
  return typeof activeBillID === "number" &&
    Number.isSafeInteger(activeBillID) &&
    activeBillID > 0
    ? activeBillID
    : null;
}

// Fiscal customer identity captured at checkout for AFIP/ARCA e-invoicing.
// All fields are optional at the API layer — the fiscal policy enforces the
// above-threshold document requirement at issuance time. Blank fields clear
// the column to NULL (consumidor final / unidentified). The doc number is
// customer PII; never log it.
export interface FiscalCustomerPayload {
  fiscal_customer_doc_type?: string;
  fiscal_customer_doc_number?: string;
  fiscal_customer_tax_condition?: string;
  fiscal_customer_name?: string;
  fiscal_customer_email?: string;
}

export interface AddBillItemRequest {
  menu_item_id: string;
  name: string;
  price: Dollars;
  quantity: number;
  options: MenuItemOption[];
}

export interface AdjustBillItemRequest {
  quantity?: number;
  void?: boolean;
  reason?: string;
}

/** Normalizes bill.items from an array or a legacy JSON string (#771). */
export function parseBillItems(raw: unknown): BillItem[] {
  if (Array.isArray(raw)) {
    return raw as BillItem[];
  }
  if (typeof raw !== "string" || !raw.trim()) {
    return [];
  }
  try {
    const parsed = JSON.parse(raw) as unknown;
    return Array.isArray(parsed) ? (parsed as BillItem[]) : [];
  } catch {
    return [];
  }
}

export interface BillResponse {
  bill: Bill;
}

export interface BillWithItemsResponse {
  bill: Bill;
  items: BillItem[];
  history?: BillHistoryEvent[];
  /** Total history events on this bill (present when the server bounded the
   * `history` list via history_limit). Lets the modal show "N of total" and a
   * "show older" affordance without loading every event up front. */
  history_total?: number;
}

export interface BillsResponse extends PaginatedResponse<Bill> {
  bills: Bill[];
  total?: number;
}

export function parseBillsResponse(data: unknown): BillsResponse {
  if (typeof data !== "object" || data === null) {
    throw new Error("invalid bills list payload");
  }
  const bills = (data as BillsResponse).bills;
  if (!Array.isArray(bills)) {
    throw new Error("invalid bills list payload");
  }
  return data as BillsResponse;
}

export interface BillListOptions extends PaginationOptions {
  status?: BillStatus | string;
  search?: string;
  dateFrom?: string;
  dateTo?: string;
  /** CRM customer id → backend ?customer= filter (wave 4). */
  customerId?: number;
}

export const isActiveBillStatus = (
  status: Bill["status"] | string | undefined,
): boolean => status === "open" || status === "partial";

// Guest-facing view of a table, returned by the public table-by-code routes.
// Exactly the public columns the backend emits for a guest table lookup
// (buildPublicGuestTableResponse: table_code / name / capacity / is_active).
// The old shape invented `status`/`code`/`seats`/`qr_code_url` fields the payload
// never sends, which is how the /scan validator ended up checking a `status`
// that was always undefined.
interface GuestTableInfo {
  table_code: string;
  name: string;
  capacity: number;
  is_active: boolean;
}

// Public guest menu payload — the backend emits the raw category JSON as a
// string (buildPublicGuestMenuResponse); parsed categories arrive separately.
interface GuestMenuInfo {
  categories: string;
  item_orderability?: Record<string, import("./orders").Orderability>;
}

export interface TableByCodeResponse {
  table: GuestTableInfo;
  business: Business;
  menu: GuestMenuInfo;
  categories: MenuCategory[];
  offers?: Offer[];
  bundles?: Bundle[];
}

export interface MenuByTableCodeResponse {
  menu: GuestMenuInfo;
  categories: MenuCategory[];
  parsed_categories?: MenuCategory[];
  language?: string;
  offers?: Offer[];
  bundles?: Bundle[];
}

// Bill API functions (Protected routes - require authentication)
export const createBill = async (
  businessId: number,
  data: CreateBillRequest,
): Promise<BillResponse> => {
  const response = await axiosInstance.post(
    `/inside/businesses/${businessId}/bills`,
    data,
  );
  return response.data;
};

const buildBillListParams = (options?: BillListOptions): string => {
  const params = new URLSearchParams();
  if (options?.page) params.append("page", String(options.page));
  if (options?.pageSize) params.append("page_size", String(options.pageSize));
  if (options?.status) params.append("status", options.status);
  if (options?.search?.trim()) params.append("search", options.search.trim());
  if (options?.dateFrom) params.append("date_from", options.dateFrom);
  if (options?.dateTo) params.append("date_to", options.dateTo);
  if (options?.customerId)
    params.append("customer", String(options.customerId));
  const query = params.toString();
  return query ? `?${query}` : "";
};

// Get all bills for a business
export const getBusinessBills = async (
  businessId: number,
  options?: BillListOptions,
  signal?: AbortSignal,
): Promise<BillsResponse> => {
  // Pass a config object only when a signal is present so the call shape stays
  // identical for the (vast majority of) callers that don't cancel.
  const response = await axiosInstance.get(
    `/inside/businesses/${businessId}/bills${buildBillListParams(options)}`,
    ...(signal ? [{ signal }] : []),
  );
  return parseBillsResponse(response.data);
};

// Get active bills for a business (open and partial, via the legacy /open route)
const getOpenBusinessBills = async (
  businessId: number,
  options?: Omit<BillListOptions, "status">,
  signal?: AbortSignal,
): Promise<BillsResponse> => {
  const response = await axiosInstance.get(
    `/inside/businesses/${businessId}/bills/open${buildBillListParams(options)}`,
    ...(signal ? [{ signal }] : []),
  );
  return parseBillsResponse(response.data);
};

export const getAllActiveBills = async (
  businessId: number,
  options: {
    pageSize?: number;
    maxPages?: number;
    maxRows?: number;
  } = {},
): Promise<FetchAllPagesResult<Bill>> =>
  fetchAllPages<Bill>(
    (page, pageSize) =>
      getOpenBusinessBills(businessId, {
        page,
        pageSize,
      }) as Promise<BillsResponse>,
    (response) => (response as BillsResponse).bills,
    options,
  );

export const getAllBillsForStatus = async (
  businessId: number,
  status: BillListOptions["status"],
  options: {
    pageSize?: number;
    maxPages?: number;
    maxRows?: number;
  } = {},
): Promise<FetchAllPagesResult<Bill>> =>
  fetchAllPages<Bill>(
    (page, pageSize) =>
      getBusinessBills(businessId, {
        page,
        pageSize,
        status,
      }) as Promise<BillsResponse>,
    (response) => (response as BillsResponse).bills,
    options,
  );

export const getBill = async (
  billId: number,
  signal?: AbortSignal,
  // Newest-N history cap (BE-first: omit for the legacy full-history read).
  historyLimit?: number,
): Promise<BillWithItemsResponse> => {
  const query =
    historyLimit && historyLimit > 0 ? `?history_limit=${historyLimit}` : "";
  const response = await axiosInstance.get(
    `/inside/bills/${billId}${query}`,
    ...(signal ? [{ signal }] : []),
  );
  return response.data;
};

export const addBillItem = async (
  billId: number,
  data: AddBillItemRequest,
): Promise<BillResponse> => {
  const response = await axiosInstance.post(
    `/inside/bills/${billId}/items`,
    data,
  );
  return response.data;
};

export interface AdjustBillItemOptions {
  /**
   * IMP-14: manager PIN to attach to the request as `X-Manager-Pin`. When
   * present, the backend's RequireManagerPIN middleware will verify it
   * against the calling staff's bcrypt hash. Owners and staff who haven't
   * enrolled a PIN can omit this safely.
   */
  pin?: string;
}

export const adjustBillItem = async (
  billId: number,
  itemId: string,
  data: AdjustBillItemRequest,
  options: AdjustBillItemOptions = {},
): Promise<BillWithItemsResponse> => {
  const config = options.pin
    ? { headers: { "X-Manager-Pin": options.pin } }
    : undefined;
  const response = await axiosInstance.patch(
    `/inside/bills/${billId}/items/${itemId}`,
    data,
    config,
  );
  return response.data;
};

export const closeBill = async (billId: number): Promise<BillResponse> => {
  const response = await axiosInstance.post(`/inside/bills/${billId}/close`);
  return response.data;
};

// IMP-15: bill void + per-payment refund. Both endpoints share the same
// X-Manager-Pin + Idempotency-Key contract — the PIN gate is the IMP-14
// middleware, the idempotency key prevents a retried network call from
// double-applying a refund/void if the operator hits the button twice on a
// flaky connection.

export interface VoidBillRequest {
  reason: string;
}

export interface RefundBillPaymentRequest {
  payment_id?: number;
  alternative_payment_id?: number;
  reason: string;
}

export interface ReversalRequestOptions {
  pin?: string;
  /**
   * Stable UUID identifying the operator-initiated request. The backend's
   * IMP-03 idempotency middleware will replay the original response on the
   * exact-same key + endpoint pair, so we generate one per bill action and
   * thread it through retries (PIN prompt + resubmit).
   */
  idempotencyKey: string;
}

interface ReversalResponse {
  bill: Bill;
}

interface RefundResponse {
  bill: Bill;
  payment?: Payment;
  alternative_payment?: AlternativeBillPayment;
}

const buildReversalConfig = (options: ReversalRequestOptions) => {
  const headers: Record<string, string> = {
    "Idempotency-Key": options.idempotencyKey,
  };
  if (options.pin) {
    headers["X-Manager-Pin"] = options.pin;
  }
  return { headers };
};

export const voidBill = async (
  billId: number,
  data: VoidBillRequest,
  options: ReversalRequestOptions,
): Promise<ReversalResponse> => {
  const response = await axiosInstance.post(
    `/inside/bills/${billId}/void`,
    data,
    buildReversalConfig(options),
  );
  return response.data;
};

export const refundBillPayment = async (
  billId: number,
  data: RefundBillPaymentRequest,
  options: ReversalRequestOptions,
): Promise<RefundResponse> => {
  const response = await axiosInstance.post(
    `/inside/bills/${billId}/refund`,
    data,
    buildReversalConfig(options),
  );
  return response.data;
};

export interface BillAuditEntry {
  id: number;
  business_id: number;
  staff_id?: number;
  target_type: "bill" | "bill_item" | "payment" | string;
  target_id: string;
  action: "comp" | "void" | "refund" | string;
  reason?: string;
  amount_cents?: number;
  pin_present: boolean;
  ip_address?: string;
  user_agent?: string;
  created_at: string;
}

export interface BillAuditResponse {
  entries: BillAuditEntry[];
}

export const getBillAudit = async (
  billId: number,
): Promise<BillAuditResponse> => {
  const response = await axiosInstance.get(`/inside/bills/${billId}/audit`);
  return response.data;
};

// ---------------------------------------------------------------------------
// Wave 4 REFUND track — durable noncustodial crypto refund lifecycle.
// Status values are honest: only "confirmed" means refunded. Never label
// requested/submitted/confirming as refunded in UI copy.
// ---------------------------------------------------------------------------

export type CryptoRefundStatus =
  | "requested"
  | "approved"
  | "awaiting_signature"
  | "submitted"
  | "confirming"
  | "confirmed"
  | "failed"
  | "rejected"
  | "cancelled";

export interface CryptoRefund {
  id: number;
  business_id: number;
  bill_id: number;
  payment_id: number;
  chain_id: number;
  token: string;
  /** Integer USDC micro-units (6 decimals). Never float. */
  amount_base_units: number;
  masked_recipient: string;
  recipient_override: boolean;
  reason: string;
  status: CryptoRefundStatus;
  honest_lifecycle_label: string;
  idempotency_key: string;
  submitted_tx_hash?: string | null;
  confirmations: number;
  last_error?: string | null;
  mainnet_submission_off: boolean;
  manual_tx_hash_required: boolean;
  explorer_url?: string;
  requested_by: string;
  approved_by?: string | null;
  requested_at: string;
  confirmed_at?: string | null;
  created_at: string;
  updated_at: string;
}

export interface RefundDestinationView {
  payment_id: number;
  has_evidence: boolean;
  manual_support_required: boolean;
  chain_id?: number;
  token?: string;
  amount_base_units?: number;
  masked_address?: string;
  evidence_type?: string;
  refundable_base_units?: number;
}

export interface UnsignedRefundTransferRequest {
  chain_id: number;
  token: string;
  token_address: string;
  from: string;
  to: string;
  amount_base_units: number;
  refund_id: number;
  mainnet_disabled: boolean;
  message: string;
}

export const getPaymentRefundDestination = async (
  businessId: number,
  paymentId: number,
): Promise<RefundDestinationView> => {
  const response = await axiosInstance.get(
    `/inside/businesses/${businessId}/payments/${paymentId}/refund-destination`,
  );
  return response.data.destination;
};

export const listCryptoRefunds = async (
  businessId: number,
  // When set, the backend returns EVERY refund for that one payment (scoped to
  // the business) instead of the business-wide newest-100 window — so older
  // refunds for a specific payment can't be truncated away.
  paymentId?: number,
): Promise<CryptoRefund[]> => {
  const query = paymentId ? `?payment_id=${paymentId}` : "";
  const response = await axiosInstance.get(
    `/inside/businesses/${businessId}/crypto-refunds${query}`,
  );
  return response.data.refunds ?? [];
};

export interface RequestCryptoRefundBody {
  bill_id: number;
  payment_id: number;
  amount_base_units: number;
  reason: string;
  idempotency_key: string;
  recipient_override?: string;
  override_reason?: string;
}

export const requestCryptoRefund = async (
  businessId: number,
  body: RequestCryptoRefundBody,
  options: ReversalRequestOptions,
): Promise<CryptoRefund> => {
  const response = await axiosInstance.post(
    `/inside/businesses/${businessId}/crypto-refunds`,
    body,
    buildReversalConfig(options),
  );
  return response.data.refund;
};

export const approveCryptoRefund = async (
  businessId: number,
  refundId: number,
  options: ReversalRequestOptions,
): Promise<CryptoRefund> => {
  const response = await axiosInstance.post(
    `/inside/businesses/${businessId}/crypto-refunds/${refundId}/approve`,
    {},
    buildReversalConfig(options),
  );
  return response.data.refund;
};

export const rejectCryptoRefund = async (
  businessId: number,
  refundId: number,
  reason: string,
  options: ReversalRequestOptions,
): Promise<CryptoRefund> => {
  const response = await axiosInstance.post(
    `/inside/businesses/${businessId}/crypto-refunds/${refundId}/reject`,
    { reason },
    buildReversalConfig(options),
  );
  return response.data.refund;
};

export const getCryptoRefundUnsignedRequest = async (
  businessId: number,
  refundId: number,
): Promise<UnsignedRefundTransferRequest> => {
  const response = await axiosInstance.get(
    `/inside/businesses/${businessId}/crypto-refunds/${refundId}/unsigned-request`,
  );
  return response.data.unsigned_request;
};

export const submitCryptoRefundTx = async (
  businessId: number,
  refundId: number,
  txHash: string,
  options: ReversalRequestOptions,
): Promise<CryptoRefund> => {
  const response = await axiosInstance.post(
    `/inside/businesses/${businessId}/crypto-refunds/${refundId}/submit-tx`,
    { tx_hash: txHash },
    buildReversalConfig(options),
  );
  return response.data.refund;
};

/** True only when status is confirmed — never for submitted/confirming. */
export const isCryptoRefundCompleted = (
  status: CryptoRefundStatus | string,
): boolean => status === "confirmed";

// Update bill with crypto payment details. Backend expects dollars (see
// internal/handlers/payments.go and @/types/money wire contract).
export interface UpdateBillPaymentRequest {
  transaction_hash: string;
  amount_paid: Dollars;
  tip_amount?: Dollars;
  payment_method: "crypto";
  blockchain_network?: string;
  // Signed quote token from getCryptoQuote. The backend locks the USD
  // settlement amount at quote time and requires this token at settlement
  // (see internal/handlers/payments.go quote verification).
  quote_token: string;
  split_share_id?: number;
}

// Locks the USD settlement amount for a crypto payment. The backend converts
// the local-currency amount to USD, reserves a persisted quote and signs it.
// The caller must transfer EXACTLY usd_microunits on-chain (not "at least")
// and return quote_token at settlement.
export interface CryptoQuoteRequest {
  amount_paid: number;
  tip_amount: number;
  split_share_id?: number;
  payment_method: "usdc_payment" | "cross_chain_payment";
}

export interface CryptoQuoteResponse {
  // Persisted quote row the token is bound to. A quote settles one payment.
  quote_id: number;
  // EXACT micro-USDC to transfer (6 decimals). It includes a unique sub-cent
  // offset (1..9999 micro-USDC) that ties the on-chain transfer to this
  // quote, so a transfer of any other amount is never applied to the bill.
  usd_microunits: number;
  // usd_microunits / 1e6, for display only. Never round it into the transfer.
  usd_amount: number;
  rate: number;
  settlement_address: string;
  chain_id: number;
  token: "USDC";
  expires_at: number;
  quote_token: string;
}

// Guest crypto helpers rethrow the original (sanitized) axios error so callers
// can read `response.data.code` via getApiErrorCode / presentGuestPaymentError.
// Do not wrap as `new Error(english)` — that strips the structured code.

export const getCryptoQuote = async (
  billToken: string,
  body: CryptoQuoteRequest,
): Promise<CryptoQuoteResponse> => {
  const response = await axiosInstance.post(
    `/guest/bill/${billToken}/crypto-quote`,
    body,
  );
  return response.data as CryptoQuoteResponse;
};

export const updateBillPayment = async (
  billToken: string,
  paymentData: UpdateBillPaymentRequest,
): Promise<unknown> => {
  const response = await axiosInstance.post(
    `/guest/bill/${billToken}/crypto-payment`,
    paymentData,
  );
  return response.data;
};

// Cross-chain payment via LI.FI. Backend expects dollars (see
// internal/handlers/payments.go and @/types/money wire contract).
export interface CrossChainPaymentRequest {
  transaction_hash: string;
  amount_paid: Dollars;
  tip_amount: Dollars;
  source_chain: string;
  source_token: string;
  lifi_route_id?: string;
  // Signed quote token from getCryptoQuote. Required by the backend to verify
  // the locked USD settlement amount (see internal/handlers/payments.go).
  quote_token: string;
  split_share_id?: number;
}

export const processCrossChainPayment = async (
  billToken: string,
  paymentData: CrossChainPaymentRequest,
): Promise<unknown> => {
  const response = await axiosInstance.post(
    `/guest/bill/${billToken}/cross-chain-payment`,
    paymentData,
  );
  return response.data;
};

// Guest API functions (no authentication required)
export const getTableByCode = async (
  code: string,
  language?: string,
): Promise<TableByCodeResponse> => {
  const params = language ? { language } : undefined;
  const response = await axiosInstance.get<TableByCodeResponse>(
    `/guest/table/${code}`,
    params ? { params } : undefined,
  );
  return response.data;
};

// Get business info with languages by table code (for guest language selection)
export const getBusinessByTableCode = async (code: string) => {
  const response = await axiosInstance.get(`/guest/table/${code}/business`);
  return response.data;
};

// Get open bill by table code (Public route for guests)
export const getOpenBillByTableCode = async (
  tableCode: string,
  options?: { disableCache?: boolean },
  signal?: AbortSignal,
): Promise<BillWithItemsResponse> => {
  const config: any = {};

  // Disable caching if requested (for real-time polling)
  if (options?.disableCache) {
    config._useCache = false;
    // Also add timestamp to URL to bust any browser cache
    config.params = { _t: Date.now() };
  }

  // Thread the poll cycle's AbortSignal so a superseded/unmounted guest poll
  // cancels its in-flight request instead of resolving into a stale write.
  if (signal) {
    config.signal = signal;
  }

  try {
    const response = await axiosInstance.get(
      `/guest/table/${tableCode}/bill`,
      config,
    );
    return response.data;
  } catch (error: unknown) {
    // H3: a not-yet-deployed backend still answers 404 for the no-active-bill
    // case. Normalize that (and the new 200 {bill:null}) to the same empty
    // shape so this FE is deploy-order-agnostic vs. the BE change.
    if (getApiErrorStatus(error) === 404) {
      return { bill: null, items: [] } as unknown as BillWithItemsResponse;
    }
    // NEW-4: edge/app rate limits must surface a typed cooldown so the bill
    // page can show "busy, retrying in Ns" instead of a blank/empty state.
    if (getApiErrorStatus(error) === 429) {
      throw new RateLimitCooldownError(parseRetryAfterSeconds(error));
    }
    throw error;
  }
};

// Guest service call ("call waiter") — enum reasons only, no free text.
export type ServiceCallReason = "water" | "order" | "check";
export type ServiceCallStatus = "none" | "open" | "acknowledged" | "resolved";

// Thrown when a guest route answers 429. Carries the server's retry window so
// the UI can show an honest "busy, retrying in Ns" cooldown (NEW-4 bill page,
// service-call cooldown, etc.).
export class RateLimitCooldownError extends Error {
  retryAfterSeconds: number;

  constructor(retryAfterSeconds: number, message = "Request is rate limited") {
    super(message);
    this.name = "RateLimitCooldownError";
    this.retryAfterSeconds = retryAfterSeconds;
  }
}

// Service-call specialty of RateLimitCooldownError (a call was just resolved).
export class ServiceCallCooldownError extends RateLimitCooldownError {
  constructor(retryAfterSeconds: number) {
    super(retryAfterSeconds, "Service call is cooling down");
    this.name = "ServiceCallCooldownError";
  }
}

/**
 * Prefer body.retry_after_seconds, then body.retry_after, then the HTTP
 * Retry-After response header (edge 429s are often bodyless — REV-4); else fallback.
 */
export function parseRetryAfterSeconds(error: unknown, fallback = 5): number {
  const data = getApiErrorData(error) as
    | { retry_after_seconds?: unknown; retry_after?: unknown }
    | undefined;
  const bodyRaw = data?.retry_after_seconds ?? data?.retry_after;
  if (bodyRaw !== undefined && bodyRaw !== null && bodyRaw !== "") {
    const n = Number(bodyRaw);
    if (Number.isFinite(n) && n >= 0) return Math.ceil(n);
  }

  const headerRaw = getRetryAfterHeaderSeconds(error);
  if (headerRaw != null) return headerRaw;

  return fallback;
}

/** Read Retry-After as delta-seconds (HTTP-date forms are ignored → null). */
function getRetryAfterHeaderSeconds(error: unknown): number | null {
  if (typeof error !== "object" || error === null) return null;
  const response = (error as { response?: { headers?: unknown } }).response;
  const headers = response?.headers;
  if (headers == null) return null;

  let raw: unknown;
  if (typeof (headers as { get?: (k: string) => unknown }).get === "function") {
    raw =
      (headers as { get: (k: string) => unknown }).get("retry-after") ??
      (headers as { get: (k: string) => unknown }).get("Retry-After");
  } else if (typeof headers === "object") {
    const h = headers as Record<string, unknown>;
    raw = h["retry-after"] ?? h["Retry-After"];
  }
  if (raw === undefined || raw === null || raw === "") return null;
  // Retry-After may be delta-seconds or an HTTP-date. Only accept numeric seconds.
  const n = Number(raw);
  if (!Number.isFinite(n) || n < 0) return null;
  return Math.ceil(n);
}

export type ServiceCallSnapshot = {
  status: ServiceCallStatus;
  reason: ServiceCallReason | null;
};

const SERVICE_CALL_REASONS: readonly ServiceCallReason[] = [
  "water",
  "order",
  "check",
];

function parseServiceCallReason(raw: unknown): ServiceCallReason | null {
  if (typeof raw !== "string") return null;
  return (SERVICE_CALL_REASONS as readonly string[]).includes(raw)
    ? (raw as ServiceCallReason)
    : null;
}

// Raise a hand at the table (public route for guests). A second reason while a
// call is still open/acked upserts the same row (staff copy + metadata) rather
// than stacking a duplicate.
export const createServiceCall = async (
  tableCode: string,
  reason: ServiceCallReason,
): Promise<ServiceCallStatus> => {
  try {
    const response = await axiosInstance.post(
      `/guest/table/${tableCode}/service-call`,
      { reason },
    );
    return response.data?.status ?? "open";
  } catch (error: unknown) {
    if (getApiErrorStatus(error) === 429) {
      throw new ServiceCallCooldownError(parseRetryAfterSeconds(error, 0));
    }
    throw error;
  }
};

// Poll the current call's status (public route for guests). Cache-busted like
// getOpenBillByTableCode since this backs a real-time acknowledge loop.
// Reason is the last guest-selected enum (water/order/check) so the open/acked
// UI can echo it after a reload (#82).
export const getServiceCallStatus = async (
  tableCode: string,
): Promise<ServiceCallSnapshot> => {
  const config: any = { _useCache: false, params: { _t: Date.now() } };
  const response = await axiosInstance.get(
    `/guest/table/${tableCode}/service-call`,
    config,
  );
  return {
    status: response.data?.status ?? "none",
    reason: parseServiceCallReason(response.data?.reason),
  };
};

export const getMenuByTableCode = async (
  code: string,
  language?: string,
): Promise<MenuByTableCodeResponse> => {
  const params = language ? { language } : {};
  const response = await axiosInstance.get<MenuByTableCodeResponse>(
    `/guest/table/${code}/menu`,
    { params },
  );
  return response.data;
};

/** Guest bill detail by unguessable public_token (legacy bill_number fallback). */
export const getBillByNumber = async (
  billToken: string,
  signal?: AbortSignal,
): Promise<BillWithItemsResponse> => {
  // Pass a config object only when a signal is present so the call shape stays
  // identical for the callers that don't cancel.
  const response = await axiosInstance.get(
    `/guest/bill/${billToken}`,
    ...(signal ? [{ signal }] : []),
  );
  return response.data;
};

/** Guest-initiated receipt email for a settled bill (public_token capability). */
export const emailBillReceipt = async (
  billToken: string,
  email: string,
  language?: string,
  details?: { paymentMethod?: string; transactionId?: string },
): Promise<void> => {
  await axiosInstance.post(`/guest/bill/${billToken}/email-receipt`, {
    email,
    ...(language ? { language } : {}),
    ...(details?.paymentMethod
      ? { payment_method: details.paymentMethod }
      : {}),
    ...(details?.transactionId
      ? { transaction_id: details.transactionId }
      : {}),
  });
};

// Operator: set the fiscal customer identity on a bill by id (authenticated).
// Mirrors the sibling PUT /inside/bills/:id route auth.
export const setBillFiscalCustomer = async (
  billId: number,
  data: FiscalCustomerPayload,
): Promise<BillResponse> => {
  const response = await axiosInstance.put(
    `/inside/bills/${billId}/fiscal-customer`,
    data,
  );
  return response.data;
};

// Guest: set the fiscal customer identity at checkout, keyed by the unguessable
// public_token (public route — no auth, scoped to the resolved bill's business).
export const setBillFiscalCustomerByNumber = async (
  billToken: string,
  data: FiscalCustomerPayload,
): Promise<{ success: boolean }> => {
  const response = await axiosInstance.post(
    `/guest/bill/${billToken}/fiscal-customer`,
    data,
  );
  return response.data;
};

// Guest factura status for a settled bill (public_token capability). Issuance
// is async (outbox worker) — callers poll while status is "none"/"pending".
export interface GuestFiscalReceipt {
  status: "none" | "pending" | "authorized";
  receipt_type?: string;
  receipt_number?: string;
  auth_code?: string;
  auth_expires_at?: string | null;
  issued_at?: string | null;
  qr_payload?: string;
  pdf_available?: boolean;
}

export const getGuestFiscalReceipt = async (
  billToken: string,
): Promise<GuestFiscalReceipt> => {
  const response = await axiosInstance.get(
    `/guest/bill/${billToken}/fiscal-receipt`,
  );
  return response.data;
};

// Direct browser-navigable URL for the factura PDF (streamed by the backend
// from protected S3 — the token in the path is the capability).
export const guestFiscalReceiptPdfUrl = (billToken: string): string =>
  `${getPublicConfig().apiUrl}/guest/bill/${billToken}/fiscal-receipt/pdf`;
