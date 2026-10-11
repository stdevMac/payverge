// Fetch-based client so it stays safe in SSR contexts (axiosInstance
// short-circuits silently under SSR per axios_unsafe_in_server_components).

import { API_BASE_URL } from "@/api/tools/baseUrl";
import { authenticatedFetch } from "@/api/authenticatedFetch";

export interface Printer {
  id: number;
  business_id: number;
  location_id: number | null;
  name: string;
  role: "bill" | "kitchen" | "bar" | string;
  transport: "browser" | "cloudprnt";
  paper_width_mm: number;
  code_page: string;
  cloudprnt_last_seen_at: string | null;
  enabled: boolean;
}

/** Full backend PrintJob.status union. Unknown future values are allowed as string. */
export type PrintJobStatus =
  | "pending"
  | "routed"
  | "printing"
  | "printed"
  | "failed"
  | "failed_retryable"
  | "failed_permanent"
  | "cancelled"
  | (string & {});

export interface PrintJob {
  id: number;
  business_id: number;
  printer_id: number | null;
  order_id?: number | null;
  kind: "bill" | "receipt" | "kitchen" | "bar" | "void" | "modify";
  source_type: string;
  source_id: number;
  status: PrintJobStatus;
  payload_html: string | null;
  language: string;
  created_at: string;
  printed_at: string | null;
  presented_at?: string | null;
  claimed_by?: string | null;
  lease_expires_at?: string | null;
  last_error?: string | null;
  attempt_count?: number;
  max_attempts?: number;
  /** Enriched by ListJobs for operator labels (dinner QA #158). */
  bill_number?: string;
  table_name?: string;
}

export interface CreatePrinterPayload {
  name: string;
  role: string;
  transport: "browser" | "cloudprnt";
  paper_width_mm?: number;
  location_id?: number | null;
  code_page?: string;
  /** Optional on PATCH — soft-disable / re-enable without delete+recreate. */
  enabled?: boolean;
}

export type UpdatePrinterPayload = Partial<CreatePrinterPayload>;

export type PrintRequestOptions = {
  signal?: AbortSignal;
};

/**
 * Maps a backend PrintJob.status to a stable, locale-independent status key.
 * Components translate this via `printers.status.<key>` (see BrowserPrintAgent);
 * the returned value is NOT a user-facing string. Unknown backend statuses pass
 * through so future values still render (and stay findable) without a code change.
 */
export function formatPrintJobStatus(status: PrintJobStatus | null | undefined): string {
  if (!status) return "unknown";
  switch (status) {
    case "pending":
      return "pending";
    case "routed":
      return "queued";
    case "printing":
      return "printing";
    case "printed":
      return "printed";
    case "failed":
      return "failed";
    case "failed_retryable":
      return "failedRetryable";
    case "failed_permanent":
      return "failedPermanent";
    case "cancelled":
      return "cancelled";
    default:
      return String(status);
  }
}

/** Whether a status is terminal (no further browser agent work). */
export function isTerminalPrintJobStatus(status: PrintJobStatus | null | undefined): boolean {
  return (
    status === "printed" ||
    status === "failed" ||
    status === "failed_permanent" ||
    status === "cancelled"
  );
}

/**
 * A failed printers API call. Carries the same `status` / `code` /
 * `response.{status,data}` shape the shared axios interceptor produces, so
 * `getLocalizedApiError` and `getApiErrorStatus` read it like any other API
 * error. The message is the backend's own `error` string, never the raw JSON
 * body: a demo refusal used to surface as `HTTP 403: {"code":...}`.
 */
class PrintApiError extends Error {
  readonly status: number;
  readonly code?: string;
  readonly response: { status: number; data: unknown };

  constructor(status: number, data: unknown) {
    const body =
      typeof data === "object" && data !== null
        ? (data as { error?: unknown; message?: unknown; code?: unknown })
        : undefined;
    const detail =
      typeof body?.error === "string" && body.error
        ? body.error
        : typeof body?.message === "string" && body.message
          ? body.message
          : undefined;
    super(detail ?? `HTTP ${status}`);
    this.name = "PrintApiError";
    this.status = status;
    this.code = typeof body?.code === "string" ? body.code : undefined;
    this.response = { status, data };
  }
}

async function readErrorBody(res: Response): Promise<unknown> {
  try {
    const text = await res.text();
    if (!text) return undefined;
    try {
      return JSON.parse(text);
    } catch {
      return undefined;
    }
  } catch {
    return undefined;
  }
}

async function request<T>(path: string, init: RequestInit): Promise<T> {
  const res = await authenticatedFetch(`${API_BASE_URL}${path}`, {
    credentials: "include",
    headers: { "Content-Type": "application/json" },
    ...init,
  });
  if (!res.ok) {
    throw new PrintApiError(res.status, await readErrorBody(res));
  }
  return res.json();
}

export async function fetchPrinters(businessID: number): Promise<{ items: Printer[] }> {
  return request(`/inside/businesses/${businessID}/printers`, { method: "GET" });
}

export async function createPrinter(
  businessID: number,
  payload: CreatePrinterPayload,
): Promise<Printer> {
  return request(`/inside/businesses/${businessID}/printers`, {
    method: "POST",
    body: JSON.stringify(payload),
  });
}

export async function updatePrinter(
  businessID: number,
  printerID: number,
  payload: UpdatePrinterPayload,
): Promise<Printer> {
  return request(`/inside/businesses/${businessID}/printers/${printerID}`, {
    method: "PATCH",
    body: JSON.stringify(payload),
  });
}

export async function deletePrinter(businessID: number, printerID: number): Promise<void> {
  await request(`/inside/businesses/${businessID}/printers/${printerID}`, { method: "DELETE" });
}

export async function testPrint(businessID: number, printerID: number): Promise<PrintJob> {
  return request(`/inside/businesses/${businessID}/printers/${printerID}/test`, { method: "POST" });
}

export async function fetchJobs(
  businessID: number,
  query: {
    status?: string;
    transport?: string;
    printer_id?: number;
    kind?: string;
    limit?: number;
    offset?: number;
  } = {},
  opts: PrintRequestOptions = {},
): Promise<{ items: PrintJob[]; total?: number }> {
  const params = new URLSearchParams();
  if (query.status) params.set("status", query.status);
  if (query.transport) params.set("transport", query.transport);
  if (query.printer_id) params.set("printer_id", String(query.printer_id));
  if (query.kind) params.set("kind", query.kind);
  if (query.limit != null) params.set("limit", String(query.limit));
  if (query.offset != null) params.set("offset", String(query.offset));
  const search = params.toString();
  const path = `/inside/businesses/${businessID}/print/jobs${search ? `?${search}` : ""}`;
  return request(path, { method: "GET", signal: opts.signal });
}

export async function cancelJob(businessID: number, jobID: number): Promise<void> {
  await request(`/inside/businesses/${businessID}/print/jobs/${jobID}/cancel`, { method: "POST" });
}

export async function reprintJob(businessID: number, jobID: number): Promise<PrintJob> {
  return request(`/inside/businesses/${businessID}/print/jobs/${jobID}/reprint`, { method: "POST" });
}

/** Re-assign a printer to a pending/unassigned job (dinner QA #141). */
export async function rerouteJob(businessID: number, jobID: number): Promise<PrintJob> {
  return request(`/inside/businesses/${businessID}/print/jobs/${jobID}/reroute`, {
    method: "POST",
  });
}

export async function createBillPrintJob(
  businessID: number,
  billID: number,
  language?: string,
): Promise<PrintJob> {
  return request(`/inside/businesses/${businessID}/print/jobs`, {
    method: "POST",
    body: JSON.stringify({
      kind: "bill",
      source_type: "bill",
      source_id: billID,
      ...(language ? { language } : {}),
    }),
  });
}

// ---- Wave 4 browser agent lease API ----

export interface ClaimPrintJobResponse {
  job: PrintJob | null;
  pending_count: number;
}

export async function claimPrintJob(
  businessID: number,
  clientID: string,
  printerID: number,
  opts: PrintRequestOptions = {},
): Promise<ClaimPrintJobResponse> {
  return request(`/inside/businesses/${businessID}/print/jobs/claim`, {
    method: "POST",
    body: JSON.stringify({ client_id: clientID, printer_id: printerID }),
    signal: opts.signal,
  });
}

export async function renewPrintLease(
  businessID: number,
  jobID: number,
  clientID: string,
  opts: PrintRequestOptions = {},
): Promise<void> {
  await request(`/inside/businesses/${businessID}/print/jobs/${jobID}/renew`, {
    method: "POST",
    body: JSON.stringify({ client_id: clientID }),
    signal: opts.signal,
  });
}

export async function markPresented(
  businessID: number,
  jobID: number,
  clientID: string,
  opts: PrintRequestOptions = {},
): Promise<void> {
  await request(`/inside/businesses/${businessID}/print/jobs/${jobID}/presented`, {
    method: "POST",
    body: JSON.stringify({ client_id: clientID }),
    signal: opts.signal,
  });
}

export async function confirmPrinted(
  businessID: number,
  jobID: number,
  clientID: string,
  opts: PrintRequestOptions = {},
): Promise<void> {
  await request(`/inside/businesses/${businessID}/print/jobs/${jobID}/confirm`, {
    method: "POST",
    body: JSON.stringify({ client_id: clientID }),
    signal: opts.signal,
  });
}

export async function retryPrintJob(
  businessID: number,
  jobID: number,
  clientID: string,
  opts: PrintRequestOptions = {},
): Promise<void> {
  await request(`/inside/businesses/${businessID}/print/jobs/${jobID}/retry`, {
    method: "POST",
    body: JSON.stringify({ client_id: clientID }),
    signal: opts.signal,
  });
}

export async function agentCancelPrintJob(
  businessID: number,
  jobID: number,
  clientID: string,
  reason?: string,
  opts: PrintRequestOptions = {},
): Promise<void> {
  await request(`/inside/businesses/${businessID}/print/jobs/${jobID}/agent-cancel`, {
    method: "POST",
    body: JSON.stringify({ client_id: clientID, reason }),
    signal: opts.signal,
  });
}
