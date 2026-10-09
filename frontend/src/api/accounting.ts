import { axiosInstance } from "@/api/tools/instance";
import { API_BASE_URL } from "@/api/tools/baseUrl";
import type { Dollars } from "@/types/money";

// Accounting/payroll API emits money values as DOLLARS. Manual ledger amounts
// are stored as int64 cents + serialized via MarshalJSON (see
// internal/database/models_json.go); payroll runs and accounting summaries
// already emit float64 dollars from internal/accounting/service.go.
export interface CategoryTotal {
  category: string;
  total: Dollars;
}

interface PayrollSummary {
  paid_runs: number;
  total_gross: Dollars;
  total_bonus: Dollars;
  total_deduction: Dollars;
  total_net: Dollars;
}

export interface AccountingSummary {
  start_date: string;
  end_date: string;
  currency: string;
  auto_income_total: Dollars;
  manual_income_total: Dollars;
  expense_total: Dollars;
  payroll_total: Dollars;
  // Net P/L is not on Summary — use ProfitLossPeriod.net (includes estimated COGS).
  billed_total: Dollars;
  collected_total: Dollars;
  collection_gap: Dollars;
  income_breakdown: CategoryTotal[];
  expense_breakdown: CategoryTotal[];
  payroll_summary: PayrollSummary;
  skipped_manual_entries?: number;
  warnings?: AccountingFXWarning[];
}

interface AccountingFXWarning {
  code: string;
  params?: Record<string, string>;
}

interface ProfitLossLine {
  category: string;
  amount: Dollars;
}

interface ProfitLossPeriod {
  start_date: string;
  end_date: string;
  currency: string;
  revenue: Dollars;
  other_income: Dollars;
  other_income_by_category: ProfitLossLine[];
  cogs: Dollars;
  labor: Dollars;
  opex: Dollars;
  opex_by_category: ProfitLossLine[];
  net: Dollars;
}

interface ProfitLossDelta {
  revenue: Dollars;
  other_income: Dollars;
  cogs: Dollars;
  labor: Dollars;
  opex: Dollars;
  net: Dollars;
}

export interface RecurringTemplate {
  id: number;
  business_id: number;
  entry_type: "income" | "expense";
  category: string;
  amount: Dollars;
  amount_cents: number;
  currency: string;
  description: string;
  notes?: string;
  reference?: string;
  cadence: "monthly" | "weekly";
  anchor_day: number;
  next_run_on: string;
  active: boolean;
  needs_attention?: boolean;
  last_generated_at?: string | null;
}

export interface RecurringTemplateInput {
  entry_type: "income" | "expense";
  category: string;
  amount: number;
  currency?: string;
  description: string;
  notes?: string;
  reference?: string;
  cadence: "monthly" | "weekly";
  anchor_day: number;
  next_run_on: string;
  active?: boolean;
}

export interface LedgerAttachment {
  id: number;
  entry_id: number;
  business_id: number;
  file_name: string;
  content_type: string;
  size_bytes: number;
  created_at?: string;
}

export interface PeriodLockState {
  locked_through: string | null;
}

export interface AccountingCategoryRow {
  id?: number;
  key: string;
  label: string;
  entry_type: "income" | "expense" | string;
  source: "default" | "custom" | string;
  active?: boolean;
  position?: number;
}

export interface AccountingCategoriesResponse {
  defaults: AccountingCategoryRow[];
  custom: AccountingCategoryRow[];
}

export interface ProfitLossStatement {
  current: ProfitLossPeriod;
  previous?: ProfitLossPeriod | null;
  delta?: ProfitLossDelta | null;
}

/**
 * One outstanding (unpaid) bill row — money fields are dollars on the wire.
 * `currency` names the unit those dollars are in, so a row can be formatted
 * from its own response instead of borrowing summary.currency (which defaults
 * to USD and printed an ARS book as dollars). `table_id` links the row back to
 * its table without matching on the display label.
 */
export interface UnpaidBill {
  id: number;
  created_at: string;
  table_id?: number;
  table_label?: string;
  total: number;
  paid: number;
  outstanding: number;
  currency?: string;
  status: string;
  age_days: number;
}

export interface UnpaidBillsParams {
  start: string;
  end: string;
  page?: number;
  page_size?: number;
}

export interface UnpaidBillsPage {
  bills: UnpaidBill[];
  /** Denominates carried_over_amount as well as every row. */
  currency?: string;
  total: number;
  page: number;
  page_size: number;
  total_pages: number;
}

/** Nested staff actor when the backend preloads created_by_staff / voided_by_staff. */
export interface LedgerActorStaff {
  id?: number;
  name?: string;
}

/** Owner user actor when list/detail preloads created_by_user (L6-15). */
interface LedgerActorUser {
  id?: number;
  name?: string;
  email?: string;
}

export interface ManualLedgerEntry {
  id: number;
  business_id: number;
  entry_type: "income" | "expense";
  category: string;
  amount: Dollars;
  currency: string;
  occurred_at: string;
  description: string;
  notes?: string;
  reference?: string;
  voided_at?: string | null;
  created_at?: string;
  updated_at?: string;
  created_by_user_id?: number | null;
  created_by_staff_id?: number | null;
  voided_by_user_id?: number | null;
  voided_by_staff_id?: number | null;
  /** Present when list/detail payloads preload staff relations. */
  created_by_staff?: LedgerActorStaff | null;
  /** Owner-created entries (L6-15). */
  created_by_user?: LedgerActorUser | null;
  voided_by_staff?: LedgerActorStaff | null;
  /** Count of ledger_entry_attachments for this row (list DTO). */
  attachment_count?: number;
}

export interface EntriesPage {
  entries: ManualLedgerEntry[];
  total: number;
  page: number;
  page_size: number;
  total_pages: number;
}

/** Query params for paginated manual ledger list (Phase A listEntries). */
export interface EntriesListParams {
  start: string;
  end: string;
  type?: "income" | "expense";
  status?: "all" | "active" | "voided";
  category?: string;
  q?: string;
  /** Backend accepts occurred_at_asc|amount_desc|amount_asc|"" (default DESC). */
  sort?: "occurred_at_desc" | "occurred_at_asc" | "amount_desc" | "amount_asc";
  page?: number;
  page_size?: number;
}

export interface CreateEntryRequest {
  entry_type: "income" | "expense";
  category: string;
  amount: Dollars;
  currency?: string;
  occurred_at: string;
  description: string;
  notes?: string;
  reference?: string;
}

export interface PayrollLineItem {
  id?: number;
  payroll_run_id?: number;
  business_id?: number;
  payee_type: "staff" | "contractor";
  staff_id?: number | null;
  payee_name: string;
  gross_amount: Dollars;
  bonus_amount: Dollars;
  deduction_amount: Dollars;
  net_amount: Dollars;
  notes?: string;
}

export interface PayrollRun {
  id: number;
  business_id?: number;
  period_start: string;
  period_end: string;
  status: "draft" | "paid" | "void";
  currency?: string;
  paid_at?: string | null;
  voided_at?: string | null;
  notes?: string;
  gross_total: Dollars;
  bonus_total: Dollars;
  deduction_total: Dollars;
  net_total: Dollars;
  line_items: PayrollLineItem[];
  created_at?: string;
  updated_at?: string;
  created_by_user_id?: number | null;
  created_by_staff_id?: number | null;
  paid_by_user_id?: number | null;
  paid_by_staff_id?: number | null;
  voided_by_user_id?: number | null;
  voided_by_staff_id?: number | null;
  /** Present when GetPayrollRun preloads staff relations. */
  created_by_staff?: LedgerActorStaff | null;
  paid_by_staff?: LedgerActorStaff | null;
  voided_by_staff?: LedgerActorStaff | null;
}

/** Payroll run list row with aggregate payee_count (no line_items required). */
export type PayrollRunWithCount = PayrollRun & { payee_count: number };

export interface PayrollRunsPage {
  runs: PayrollRunWithCount[];
  total: number;
  page: number;
  page_size: number;
  total_pages: number;
}

export interface PayrollRunsListParams {
  start?: string;
  end?: string;
  status?: "draft" | "paid" | "void" | string;
  page?: number;
  page_size?: number;
}

interface TimeseriesPoint {
  date: string;
  income: number;
  expense: number;
  payroll: number;
}

export interface Timeseries {
  start: string;
  end: string;
  bucket: "day";
  currency: string;
  series: TimeseriesPoint[];
}

export interface PayrollExportParams {
  start: string;
  end: string;
  status?: string;
  /**
   * Operator UI locale (SimpleTranslationProvider). Same contract as
   * EntriesExportParams.lang — the backend localizes the payroll CSV header row
   * and only falls back to Accept-Language when this is absent (#930).
   */
  lang?: string;
}

export interface EntriesExportParams {
  start: string;
  end: string;
  type?: "income" | "expense";
  status?: "all" | "active" | "voided";
  category?: string;
  q?: string;
  /**
   * Operator UI locale (SimpleTranslationProvider). The backend localizes the
   * streamed CSV headers and only falls back to Accept-Language when this is
   * absent, so sending it keeps the file in the language the dashboard is in
   * rather than the language the browser happens to advertise.
   */
  lang?: string;
}

export interface CreatePayrollLineItemRequest {
  payee_type: "staff" | "contractor";
  staff_id?: number;
  payee_name?: string;
  gross_amount: Dollars;
  bonus_amount?: Dollars;
  deduction_amount?: Dollars;
  notes?: string;
}

export interface CreatePayrollRunRequest {
  period_start: string;
  period_end: string;
  notes?: string;
  line_items: CreatePayrollLineItemRequest[];
}

export interface FoodCostItem {
  menu_item_id: string;
  menu_item_name: string;
  unit_cost: Dollars;
  avg_price: Dollars;
  food_cost_pct: number; // 0..1
  margin_per_unit: Dollars;
  qty_sold: number;
  has_complete_cost: boolean;
}

export interface FoodCostReport {
  period: string;
  items: FoodCostItem[];
  blended_food_cost_pct: number; // 0..1
  estimated_cogs: Dollars;
  total_revenue: Dollars;
  items_missing_cost: number;
  items_without_recipe: number;
  /** False when the window recorded no sold units — sale-derived fields are structurally zero (#797). */
  has_sales: boolean;
}

const accountingDatePattern = /^\d{4}-\d{2}-\d{2}$/;

function normalizeAccountingDate(value: string): string {
  if (!accountingDatePattern.test(value)) {
    throw new Error("Accounting dates must use YYYY-MM-DD");
  }
  return value;
}

/** Build axios/query params: drop empties; map occurred_at_desc → omit (backend default). */
function toEntriesQueryParams(
  params: EntriesListParams,
): Record<string, string | number> {
  const out: Record<string, string | number> = {
    start: normalizeAccountingDate(params.start),
    end: normalizeAccountingDate(params.end),
  };
  if (params.type) out.type = params.type;
  if (params.status) out.status = params.status;
  if (params.category) out.category = params.category;
  if (params.q) out.q = params.q;
  // Backend accepts "" | occurred_at_asc | amount_desc | amount_asc only.
  // occurred_at_desc is the default when sort is omitted.
  if (params.sort && params.sort !== "occurred_at_desc") {
    out.sort = params.sort;
  }
  if (params.page !== undefined) out.page = params.page;
  if (params.page_size !== undefined) out.page_size = params.page_size;
  return out;
}

function toPayrollQueryParams(
  params: PayrollRunsListParams,
  opts?: { alwaysPage?: boolean },
): Record<string, string | number> {
  const out: Record<string, string | number> = {};
  if (params.start) out.start = normalizeAccountingDate(params.start);
  if (params.end) out.end = normalizeAccountingDate(params.end);
  if (params.status) out.status = params.status;
  if (opts?.alwaysPage) {
    out.page = params.page ?? 1;
  } else if (params.page !== undefined) {
    out.page = params.page;
  }
  if (params.page_size !== undefined) out.page_size = params.page_size;
  return out;
}

function buildExportUrl(
  path: string,
  query: Record<string, string | undefined>,
): string {
  const sp = new URLSearchParams();
  for (const [key, value] of Object.entries(query)) {
    if (value === undefined || value === "") continue;
    sp.set(key, value);
  }
  const qs = sp.toString();
  const base = API_BASE_URL.replace(/\/$/, "");
  return `${base}${path}${qs ? `?${qs}` : ""}`;
}

export const accountingApi = {
  getProfitLoss: async (
    businessId: string,
    start: string,
    end: string,
    compare?: "prev",
  ): Promise<ProfitLossStatement> => {
    const response = await axiosInstance.get(
      "/inside/businesses/" + businessId + "/accounting/profit-loss",
      {
        params: {
          start: normalizeAccountingDate(start),
          end: normalizeAccountingDate(end),
          ...(compare ? { compare } : {}),
        },
      },
    );
    return response.data.data;
  },

  /**
   * String href for the P&L CSV export anchor (not a fetch). `lang` is the
   * operator UI locale — the backend localizes the header row and only falls
   * back to Accept-Language when it is absent (#930).
   */
  profitLossExportUrl: (
    businessId: string,
    start: string,
    end: string,
    lang?: string,
  ): string => {
    return buildExportUrl(
      `/inside/businesses/${businessId}/accounting/profit-loss/export.csv`,
      {
        start: normalizeAccountingDate(start),
        end: normalizeAccountingDate(end),
        lang,
      },
    );
  },

  getUnpaidBills: async (
    businessId: string,
    params: UnpaidBillsParams,
  ): Promise<UnpaidBillsPage> => {
    const response = await axiosInstance.get(
      `/inside/businesses/${businessId}/accounting/unpaid-bills`,
      {
        params: {
          start: normalizeAccountingDate(params.start),
          end: normalizeAccountingDate(params.end),
          page: params.page ?? 1,
          page_size: params.page_size ?? 20,
        },
      },
    );
    const data = response.data?.data ?? {};
    return {
      bills: data.bills ?? [],
      currency: data.currency,
      total: data.total ?? 0,
      page: data.page ?? 1,
      page_size: data.page_size ?? params.page_size ?? 20,
      total_pages: data.total_pages ?? 0,
    };
  },

  listRecurringTemplates: async (businessId: string) => {
    const response = await axiosInstance.get(
      `/inside/businesses/${businessId}/accounting/recurring-templates`,
    );
    return response.data.data as RecurringTemplate[];
  },
  createRecurringTemplate: async (
    businessId: string,
    payload: RecurringTemplateInput,
  ) => {
    const response = await axiosInstance.post(
      `/inside/businesses/${businessId}/accounting/recurring-templates`,
      payload,
    );
    return response.data.data as RecurringTemplate;
  },
  updateRecurringTemplate: async (
    businessId: string,
    templateId: number,
    payload: RecurringTemplateInput,
  ) => {
    const response = await axiosInstance.patch(
      `/inside/businesses/${businessId}/accounting/recurring-templates/${templateId}`,
      payload,
    );
    return response.data.data as RecurringTemplate;
  },
  deleteRecurringTemplate: async (businessId: string, templateId: number) => {
    await axiosInstance.delete(
      `/inside/businesses/${businessId}/accounting/recurring-templates/${templateId}`,
    );
  },

  listEntryAttachments: async (
    businessId: string,
    entryId: number,
  ): Promise<LedgerAttachment[]> => {
    const response = await axiosInstance.get(
      `/inside/businesses/${businessId}/accounting/entries/${entryId}/attachments`,
    );
    return response.data.data ?? [];
  },
  uploadEntryAttachment: async (
    businessId: string,
    entryId: number,
    file: File,
  ): Promise<LedgerAttachment> => {
    const form = new FormData();
    form.append("file", file);
    const response = await axiosInstance.post(
      `/inside/businesses/${businessId}/accounting/entries/${entryId}/attachments`,
      form,
      { headers: { "Content-Type": "multipart/form-data" } },
    );
    return response.data.data;
  },
  downloadEntryAttachmentUrl: (
    businessId: string,
    entryId: number,
    attachmentId: number,
  ): string => {
    const base = API_BASE_URL.replace(/\/$/, "");
    return `${base}/inside/businesses/${businessId}/accounting/entries/${entryId}/attachments/${attachmentId}/download`;
  },
  deleteEntryAttachment: async (
    businessId: string,
    entryId: number,
    attachmentId: number,
  ): Promise<void> => {
    await axiosInstance.delete(
      `/inside/businesses/${businessId}/accounting/entries/${entryId}/attachments/${attachmentId}`,
    );
  },
  getPeriodLock: async (businessId: string): Promise<PeriodLockState> => {
    const response = await axiosInstance.get(
      `/inside/businesses/${businessId}/accounting/period-lock`,
    );
    return response.data.data;
  },
  postPeriodLock: async (
    businessId: string,
    payload: { locked_through: string | null; note: string },
  ): Promise<PeriodLockState> => {
    const response = await axiosInstance.post(
      `/inside/businesses/${businessId}/accounting/period-lock`,
      payload,
    );
    return response.data.data;
  },
  listAccountingCategories: async (
    businessId: string,
    entryType?: "income" | "expense",
    opts?: { includeInactive?: boolean },
  ): Promise<AccountingCategoriesResponse> => {
    const response = await axiosInstance.get(
      `/inside/businesses/${businessId}/accounting/categories`,
      {
        params: {
          ...(entryType ? { entry_type: entryType } : {}),
          ...(opts?.includeInactive ? { include_inactive: "true" } : {}),
        },
      },
    );
    return response.data.data;
  },
  createAccountingCategory: async (
    businessId: string,
    payload: { key: string; label: string; entry_type: "income" | "expense" },
  ): Promise<AccountingCategoryRow> => {
    const response = await axiosInstance.post(
      `/inside/businesses/${businessId}/accounting/categories`,
      payload,
    );
    return response.data.data;
  },
  updateAccountingCategory: async (
    businessId: string,
    categoryId: number,
    payload: { label?: string; active?: boolean; position?: number },
  ): Promise<AccountingCategoryRow> => {
    const response = await axiosInstance.patch(
      `/inside/businesses/${businessId}/accounting/categories/${categoryId}`,
      payload,
    );
    return response.data.data;
  },

  getSummary: async (
    businessId: string,
    start: string,
    end: string,
  ): Promise<AccountingSummary> => {
    const response = await axiosInstance.get(
      "/inside/businesses/" + businessId + "/accounting/summary",
      {
        params: {
          start: normalizeAccountingDate(start),
          end: normalizeAccountingDate(end),
        },
      },
    );
    return response.data.data;
  },

  getTimeseries: async (
    businessId: string,
    start: string,
    end: string,
  ): Promise<Timeseries> => {
    const response = await axiosInstance.get(
      "/inside/businesses/" + businessId + "/accounting/timeseries",
      {
        params: {
          start: normalizeAccountingDate(start),
          end: normalizeAccountingDate(end),
        },
      },
    );
    return response.data.data;
  },

  getFoodCost: async (
    businessId: string,
    period: "day" | "week" | "month" = "week",
    range?: { start: string; end: string },
  ): Promise<FoodCostReport> => {
    const params = range
      ? {
          start: normalizeAccountingDate(range.start),
          end: normalizeAccountingDate(range.end),
        }
      : { period };
    const response = await axiosInstance.get(
      "/inside/businesses/" + businessId + "/accounting/food-cost",
      { params },
    );
    return response.data.data;
  },

  listEntries: async (
    businessId: string,
    params: EntriesListParams,
  ): Promise<EntriesPage> => {
    const response = await axiosInstance.get(
      "/inside/businesses/" + businessId + "/accounting/entries",
      {
        params: toEntriesQueryParams(params),
      },
    );
    return response.data.data;
  },

  createEntry: async (
    businessId: string,
    payload: CreateEntryRequest,
  ): Promise<ManualLedgerEntry> => {
    const response = await axiosInstance.post(
      "/inside/businesses/" + businessId + "/accounting/entries",
      payload,
    );
    return response.data.data;
  },

  voidEntry: async (businessId: string, entryId: number): Promise<void> => {
    await axiosInstance.post(
      "/inside/businesses/" +
        businessId +
        "/accounting/entries/" +
        entryId +
        "/void",
      {},
    );
  },

  /**
   * Paginated payroll runs with payee_count. Always sends `page` so the backend
   * returns the PayrollRunsPage envelope (legacy without page returns an array).
   */
  listPayrollRunsPage: async (
    businessId: string,
    params: PayrollRunsListParams = {},
  ): Promise<PayrollRunsPage> => {
    const response = await axiosInstance.get(
      "/inside/businesses/" + businessId + "/accounting/payroll-runs",
      { params: toPayrollQueryParams(params, { alwaysPage: true }) },
    );
    return response.data.data;
  },

  getPayrollRun: async (
    businessId: string,
    runId: number,
  ): Promise<PayrollRun> => {
    const response = await axiosInstance.get(
      "/inside/businesses/" + businessId + "/accounting/payroll-runs/" + runId,
    );
    return response.data.data;
  },

  createPayrollRun: async (
    businessId: string,
    payload: CreatePayrollRunRequest,
  ): Promise<PayrollRun> => {
    const response = await axiosInstance.post(
      "/inside/businesses/" + businessId + "/accounting/payroll-runs",
      payload,
    );
    return response.data.data;
  },

  markPayrollRunPaid: async (
    businessId: string,
    runId: number,
  ): Promise<PayrollRun> => {
    const response = await axiosInstance.post(
      "/inside/businesses/" +
        businessId +
        "/accounting/payroll-runs/" +
        runId +
        "/mark-paid",
      {},
    );
    return response.data.data;
  },

  deletePayrollRun: async (
    businessId: string,
    runId: number,
  ): Promise<void> => {
    await axiosInstance.delete(
      "/inside/businesses/" + businessId + "/accounting/payroll-runs/" + runId,
    );
  },

  voidPayrollRun: async (
    businessId: string,
    runId: number,
  ): Promise<PayrollRun> => {
    const response = await axiosInstance.post(
      "/inside/businesses/" +
        businessId +
        "/accounting/payroll-runs/" +
        runId +
        "/void",
      {},
    );
    return response.data.data;
  },

  /** String href for CSV export anchor (not a fetch). */
  entriesExportUrl: (
    businessId: string,
    params: EntriesExportParams,
  ): string => {
    return buildExportUrl(
      `/inside/businesses/${businessId}/accounting/entries/export.csv`,
      {
        start: normalizeAccountingDate(params.start),
        end: normalizeAccountingDate(params.end),
        type: params.type,
        status: params.status,
        category: params.category,
        q: params.q,
        lang: params.lang,
      },
    );
  },

  /** String href for payroll CSV export anchor (not a fetch). */
  payrollExportUrl: (
    businessId: string,
    params: PayrollExportParams,
  ): string => {
    return buildExportUrl(
      `/inside/businesses/${businessId}/accounting/payroll-runs/export.csv`,
      {
        start: normalizeAccountingDate(params.start),
        end: normalizeAccountingDate(params.end),
        status: params.status,
        lang: params.lang,
      },
    );
  },
};
