import { axiosInstance } from "@/api/tools/instance";
import type { Dollars } from "@/types/money";

export type FiscalMode =
  | "off"
  | "manual"
  | "automatic_non_blocking";

export type FiscalStatus =
  | "pending"
  | "authorized"
  | "rejected"
  | "failed_retryable"
  | "failed_permanent"
  | "cancelled"
  | "credited";

export type FiscalEnvironment = "sandbox" | "production";

export interface FiscalSettings {
  id: number;
  business_id: number;
  country: string;
  provider: string;
  mode: FiscalMode;
  environment: FiscalEnvironment;
  tax_id: string;
  tax_condition: string;
  point_of_sale: number | null;
  credentials_fingerprint?: string;
  credentials_expires_at?: string | null;
  provider_config?: Record<string, unknown>;
  setup_status: string;
  last_validated_at?: string | null;
  last_validation_error?: string | null;
  created_at?: string;
  updated_at?: string;
}

export interface UpdateFiscalSettingsPayload {
  country: string;
  provider: string;
  mode: FiscalMode;
  environment: FiscalEnvironment;
  tax_id: string;
  tax_condition?: string;
  point_of_sale?: number | null;
}

export interface FiscalReceipt {
  id: number;
  business_id: number;
  settings_id: number;
  bill_id: number;
  payment_id: number | null;
  alternative_payment_id: number | null;
  country: string;
  provider: string;
  action: string;
  receipt_type: string;
  receipt_number: string | null;
  provider_receipt_id: string | null;
  auth_code: string | null;
  auth_expires_at: string | null;
  qr_payload: string | null;
  qr_image_path: string | null;
  pdf_path: string | null;
  customer_doc_type: string | null;
  customer_doc_number: string | null;
  customer_name?: string | null;
  customer_tax_condition?: string | null;
  total_amount_cents: number;
  tip_amount_cents: number;
  currency: string;
  status: FiscalStatus;
  error_code: string | null;
  error_message: string | null;
  issued_at: string | null;
  created_at: string;
  updated_at: string;
}

/** Compact delivery badge embedded on paginated receipt rows (no N+1). */
export interface ReceiptDeliveryBadge {
  task_id: number;
  channel: string;
  status: string;
}

/** Fiscal receipt list row with delivery badges + attention flag. */
export type ReceiptRow = FiscalReceipt & {
  delivery: ReceiptDeliveryBadge[];
  needs_attention: boolean;
  /** Dining table label for the underlying bill (dinner-service findability). */
  table_label?: string;
};

export interface ReceiptsPage {
  receipts: ReceiptRow[];
  total: number;
  page: number;
  page_size: number;
  total_pages: number;
}

export interface ReceiptsListParams {
  start?: string;
  end?: string;
  status?: FiscalStatus | string;
  receipt_type?: string;
  needs_attention?: boolean;
  /** Search bill # / table / guest / receipt number. */
  q?: string;
  page?: number;
  page_size?: number;
}

export interface IssueReceiverOverride {
  customer_doc_type?: string;
  customer_doc_number?: string;
  customer_tax_condition?: string;
  customer_name?: string;
}

export interface ResolveReceiptTypeResult {
  receipt_type: string;
  letter: string;
  /** ISO country from fiscal settings (e.g. US, AR). */
  country?: string;
}

/** Recent paid bill for the invoice picker (issue or view existing). */
export interface IssuableBill {
  bill_id: number;
  bill_number: string;
  table_label?: string;
  closed_at?: string | null;
  total_amount: Dollars;
  currency: string;
  customer_doc_type?: string;
  customer_doc_number?: string;
  customer_tax_condition?: string;
  customer_name?: string;
  /** Set when a blocking issue receipt already exists — open it instead of re-issuing. */
  existing_receipt_id?: number | null;
}

export interface FiscalActionResponse {
  ok: boolean;
  business_id?: number;
}

// Response from POST /fiscal/credentials. The backend deliberately never echoes
// the uploaded certificate or private key — only the derived fingerprint and the
// certificate's expiry, plus the updated setup_status. Never persist or display
// the key material on the client.
export interface UploadFiscalCredentialsResponse {
  fingerprint: string;
  expires_at: string | null;
  setup_status: string;
}

function businessFiscalPath(
  businessID: number | string,
  suffix: string,
): string {
  return `/inside/businesses/${businessID}/fiscal${suffix}`;
}

function toReceiptsQueryParams(
  params: ReceiptsListParams,
): Record<string, string | number> {
  const out: Record<string, string | number> = {
    // Always send page so backend returns top-level ReceiptsPage (not legacy items).
    page: params.page ?? 1,
  };
  if (params.start) out.start = params.start;
  if (params.end) out.end = params.end;
  if (params.status) out.status = params.status;
  if (params.receipt_type) out.receipt_type = params.receipt_type;
  if (params.needs_attention) out.needs_attention = "true";
  if (params.q && params.q.trim()) out.q = params.q.trim();
  if (params.page_size !== undefined) out.page_size = params.page_size;
  return out;
}

/** Fiscal chrome already renders an inline retry; skip the English network toast. */
const FISCAL_READ_CONFIG = { _skipErrorToast: true } as const;

export async function getSettings(
  businessID: number | string,
): Promise<FiscalSettings | null> {
  const response = await axiosInstance.get(
    businessFiscalPath(businessID, "/settings"),
    FISCAL_READ_CONFIG,
  );
  return response.data.settings;
}

export async function updateSettings(
  businessID: number | string,
  payload: UpdateFiscalSettingsPayload,
): Promise<FiscalSettings> {
  const response = await axiosInstance.put(
    businessFiscalPath(businessID, "/settings"),
    payload,
  );
  return response.data;
}

/** Legacy unpaginated receipts list (no page query → { items: FiscalReceipt[] }). */
export async function listReceipts(
  businessID: number | string,
  status?: FiscalStatus,
): Promise<FiscalReceipt[]> {
  const path = businessFiscalPath(businessID, "/receipts");
  const response = status
    ? await axiosInstance.get(path, { params: { status }, ...FISCAL_READ_CONFIG })
    : await axiosInstance.get(path, FISCAL_READ_CONFIG);
  return response.data.items;
}

/**
 * Paginated receipts with delivery badges. Always sends `page` so the backend
 * returns the top-level ReceiptsPage envelope (not legacy items-only).
 */
export async function listReceiptsPage(
  businessID: number | string,
  params: ReceiptsListParams = {},
): Promise<ReceiptsPage> {
  const response = await axiosInstance.get(
    businessFiscalPath(businessID, "/receipts"),
    { params: toReceiptsQueryParams(params), ...FISCAL_READ_CONFIG },
  );
  // Top-level body is ReceiptsPage (no success/data wrapper).
  return response.data;
}

/** Paid bills eligible for manual issue (invoice picker). */
export async function listIssuableBills(
  businessID: number | string,
  q?: string,
): Promise<IssuableBill[]> {
  const params: Record<string, string> = {};
  if (q) params.q = q;
  const response = await axiosInstance.get(
    businessFiscalPath(businessID, "/issuable-bills"),
    { params },
  );
  const body = response.data ?? {};
  return Array.isArray(body.bills) ? body.bills : [];
}

export async function issueReceipt(
  businessID: number | string,
  billID: number,
  receiver?: IssueReceiverOverride,
): Promise<FiscalActionResponse> {
  const body: Record<string, unknown> = { bill_id: billID };
  if (receiver) {
    if (receiver.customer_doc_type)
      body.customer_doc_type = receiver.customer_doc_type;
    if (receiver.customer_doc_number)
      body.customer_doc_number = receiver.customer_doc_number;
    if (receiver.customer_tax_condition)
      body.customer_tax_condition = receiver.customer_tax_condition;
    if (receiver.customer_name) body.customer_name = receiver.customer_name;
  }
  const response = await axiosInstance.post(
    businessFiscalPath(businessID, "/receipts/issue"),
    body,
  );
  return response.data;
}

/** Server-side letter resolution for the issue drawer preview. */
export async function resolveReceiptType(
  businessID: number | string,
  params: {
    doc_type?: string;
    doc_number?: string;
    tax_condition?: string;
  },
): Promise<ResolveReceiptTypeResult> {
  const response = await axiosInstance.get(
    businessFiscalPath(businessID, "/resolve-type"),
    { params },
  );
  return response.data;
}

export async function retryReceipt(
  businessID: number | string,
  receiptID: number,
): Promise<FiscalActionResponse> {
  const response = await axiosInstance.post(
    businessFiscalPath(businessID, `/receipts/${receiptID}/retry`),
    {},
  );
  return response.data;
}

// uploadFiscalCredentials sends the AFIP/ARCA certificate + private key as a
// multipart form (fields "certificate" and "private_key", matching the backend
// UploadCredentials handler). The private key never round-trips back: the
// response carries only fingerprint, expiry, and setup_status. Mirrors the
// multipart shape used by api/uploads.ts (uploadFile).
export async function uploadFiscalCredentials(
  businessID: number | string,
  cert: File,
  key: File,
): Promise<UploadFiscalCredentialsResponse> {
  const formData = new FormData();
  formData.append("certificate", cert);
  formData.append("private_key", key);
  const response = await axiosInstance.post(
    businessFiscalPath(businessID, "/credentials"),
    formData,
    { headers: { "Content-Type": "multipart/form-data" } },
  );
  return response.data;
}

// validateFiscalSettings triggers a live WSAA+FEDummy check on the backend.
// A *validation* failure is returned as HTTP 200 with the updated settings (whose
// setup_status stays non-"ready" and last_validation_error is populated), so the
// caller reads the returned settings rather than relying on a thrown error.
export async function validateFiscalSettings(
  businessID: number | string,
): Promise<FiscalSettings> {
  const response = await axiosInstance.post(
    businessFiscalPath(businessID, "/validate"),
    {},
  );
  return response.data.settings;
}

// creditFiscalReceipt enqueues a nota de crédito (credit note) for an authorized
// receipt. Reason is required. amountCents optional (omit/0 = full original total).
export async function creditFiscalReceipt(
  businessID: number | string,
  receiptID: number,
  reason: string,
  amountCents?: number,
): Promise<FiscalActionResponse> {
  const body: Record<string, unknown> = { reason: reason.trim() };
  if (amountCents != null && amountCents > 0) {
    body.amount_cents = amountCents;
  }
  const response = await axiosInstance.post(
    businessFiscalPath(businessID, `/receipts/${receiptID}/credit`),
    body,
  );
  return response.data;
}

// resendFiscalReceipt re-sends/re-prints an already-authorized receipt to the
// customer, forcing delivery even if it was already delivered. Mirrors
// retryReceipt's empty-body POST shape.
export async function resendFiscalReceipt(
  businessID: number | string,
  receiptID: number,
): Promise<FiscalActionResponse> {
  const response = await axiosInstance.post(
    businessFiscalPath(businessID, `/receipts/${receiptID}/resend`),
    {},
  );
  return response.data;
}

// Wave 4 durable per-channel delivery status (safe projection).
type FiscalDeliveryChannel = "artifact" | "email" | "print";
type FiscalDeliveryStatus =
  | "pending"
  | "leased"
  | "succeeded"
  | "dead";

export interface FiscalDeliveryTask {
  id: number;
  receipt_id: number;
  channel: FiscalDeliveryChannel | string;
  status: FiscalDeliveryStatus | string;
  attempts: number;
  max_attempts: number;
  next_attempt_at?: string | null;
  last_error_category?: string;
  masked_recipient?: string;
  succeeded_at?: string | null;
  dead_at?: string | null;
}

export async function listReceiptDelivery(
  businessID: number | string,
  receiptID: number,
): Promise<FiscalDeliveryTask[]> {
  const response = await axiosInstance.get(
    businessFiscalPath(businessID, `/receipts/${receiptID}/delivery`),
  );
  return response.data.items ?? [];
}

export async function retryDeliveryTask(
  businessID: number | string,
  taskID: number,
): Promise<FiscalActionResponse> {
  const response = await axiosInstance.post(
    businessFiscalPath(businessID, `/delivery-tasks/${taskID}/retry`),
    {},
  );
  return response.data;
}

export const fiscalApi = {
  getSettings,
  updateSettings,
  listReceipts,
  listReceiptsPage,
  listIssuableBills,
  issueReceipt,
  resolveReceiptType,
  retryReceipt,
  resendFiscalReceipt,
  listReceiptDelivery,
  retryDeliveryTask,
  uploadFiscalCredentials,
  validateFiscalSettings,
  creditFiscalReceipt,
};
