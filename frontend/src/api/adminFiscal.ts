import { axiosInstance } from "@/api/tools/instance";

export interface AdminFiscalSummary {
  businesses_with_fiscal: number;
  jobs_by_status: Record<string, number>;
  receipts_by_status: Record<string, number>;
  due_jobs: number;
  failed_retryable_jobs: number;
  failed_permanent_jobs: number;
}

export interface FiscalJobRow {
  id: number;
  business_id: number;
  /** Display name joined from businesses (Task 15). Prefer over raw #id. */
  business_name?: string;
  bill_id: number;
  action: string;
  status: string;
  attempts: number;
  max_attempts: number;
  last_error_code?: string | null;
  last_error_message?: string | null;
  created_at: string;
  updated_at: string;
}

export interface FiscalReceiptRow {
  id: number;
  business_id: number;
  business_name?: string;
  bill_id: number;
  country: string;
  provider: string;
  receipt_type: string;
  receipt_number?: string | null;
  status: string;
  error_code?: string | null;
  error_message?: string | null;
  total_amount_cents: number;
  currency: string;
  created_at: string;
  updated_at: string;
}

/** Failed job statuses that expose an enabled Requeue action. */
export function isFiscalJobRequeueable(status: string): boolean {
  return status === "failed_retryable" || status === "failed_permanent";
}

export async function getAdminFiscalSummary(): Promise<AdminFiscalSummary> {
  const response = await axiosInstance.get<AdminFiscalSummary>("/admin/fiscal/summary", {
    _useCache: false,
  });
  return response.data;
}

export async function getAdminFiscalJobs(params?: {
  page?: number;
  limit?: number;
  status?: string;
  /** real (default) | demo | test | all — production queue excludes non-real. */
  kind?: string;
}): Promise<{ jobs: FiscalJobRow[]; total: number }> {
  const page = params?.page ?? 1;
  const limit = params?.limit ?? 25;
  // Intentionally omit kind when unset so the backend default (real) applies.
  const response = await axiosInstance.get<{ jobs: FiscalJobRow[]; total: number }>(
    "/admin/fiscal/jobs",
    {
      params: {
        page,
        limit,
        status: params?.status,
        ...(params?.kind ? { kind: params.kind } : {}),
      },
      _useCache: false,
    },
  );
  return { jobs: response.data.jobs || [], total: response.data.total || 0 };
}

export async function getAdminFiscalReceipts(params?: {
  page?: number;
  limit?: number;
  status?: string;
  kind?: string;
}): Promise<{ receipts: FiscalReceiptRow[]; total: number }> {
  const page = params?.page ?? 1;
  const limit = params?.limit ?? 25;
  const response = await axiosInstance.get<{ receipts: FiscalReceiptRow[]; total: number }>(
    "/admin/fiscal/receipts",
    {
      params: {
        page,
        limit,
        status: params?.status,
        ...(params?.kind ? { kind: params.kind } : {}),
      },
      _useCache: false,
    },
  );
  return { receipts: response.data.receipts || [], total: response.data.total || 0 };
}

// The backend refuses to requeue a failed_permanent job unless the caller says
// so explicitly; the admin page passes it for the row the admin clicked.
export async function requeueFiscalJob(id: number, allowPermanent = false): Promise<FiscalJobRow> {
  const response = await axiosInstance.post<{ job: FiscalJobRow }>(
    `/admin/fiscal/jobs/${id}/requeue`,
    allowPermanent ? { allow_permanent: true } : {},
  );
  return response.data.job;
}
