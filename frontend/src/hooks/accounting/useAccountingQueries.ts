import {
  keepPreviousData,
  useMutation,
  useQuery,
  useQueryClient,
} from "@tanstack/react-query";
import {
  accountingApi,
  type CreateEntryRequest,
  type CreatePayrollRunRequest,
  type EntriesListParams,
  type EntriesPage,
  type PayrollRun,
  type PayrollRunsListParams,
  type PayrollRunsPage,
  type ProfitLossStatement,
  type AccountingSummary,
  type Timeseries,
  type UnpaidBillsPage,
  type UnpaidBillsParams,
} from "@/api/accounting";
import {
  fiscalApi,
  type IssuableBill,
  type IssueReceiverOverride,
  type ReceiptsListParams,
  type ReceiptsPage,
} from "@/api/fiscal";

/** Date window shared by summary/timeseries queries. */
export type AccountingRange = {
  start: string;
  end: string;
};

/** Business id as used on API paths — numeric id or slug string. */
export type AccountingBizId = string | number;

export const accountingKeys = {
  root: (biz: AccountingBizId) => ["accounting", biz] as const,
  summary: (biz: AccountingBizId, r: AccountingRange) =>
    [...accountingKeys.root(biz), "summary", r] as const,
  timeseries: (biz: AccountingBizId, r: AccountingRange) =>
    [...accountingKeys.root(biz), "timeseries", r] as const,
  entries: (biz: AccountingBizId, p: EntriesListParams) =>
    [...accountingKeys.root(biz), "entries", p] as const,
  payrollRuns: (biz: AccountingBizId, p: object) =>
    [...accountingKeys.root(biz), "payroll", p] as const,
  payrollRun: (biz: AccountingBizId, id: number) =>
    [...accountingKeys.root(biz), "payroll-run", id] as const,
  receipts: (biz: AccountingBizId, p: object) =>
    [...accountingKeys.root(biz), "receipts", p] as const,
  issuableBills: (biz: AccountingBizId, q: string) =>
    [...accountingKeys.root(biz), "issuable-bills", q] as const,
  profitLoss: (biz: AccountingBizId, r: AccountingRange, compare: boolean) =>
    [...accountingKeys.root(biz), "profit-loss", r, compare] as const,
  unpaidBills: (biz: AccountingBizId, p: UnpaidBillsParams) =>
    [...accountingKeys.root(biz), "unpaid-bills", p] as const,
};

function bizId(businessId: AccountingBizId): string {
  return String(businessId);
}

function isBizEnabled(businessId: AccountingBizId): boolean {
  if (typeof businessId === "number") return businessId > 0;
  const trimmed = businessId.trim();
  return trimmed.length > 0 && trimmed !== "0";
}

function invalidateAccountingRoot(
  queryClient: ReturnType<typeof useQueryClient>,
  businessId: AccountingBizId,
) {
  return queryClient.invalidateQueries({
    queryKey: accountingKeys.root(businessId),
  });
}

// ---------------------------------------------------------------------------
// Queries
// ---------------------------------------------------------------------------

export function useSummary(
  businessId: AccountingBizId,
  range: AccountingRange,
) {
  return useQuery<AccountingSummary>({
    queryKey: accountingKeys.summary(businessId, range),
    queryFn: () =>
      accountingApi.getSummary(bizId(businessId), range.start, range.end),
    enabled: isBizEnabled(businessId) && !!range.start && !!range.end,
    // DashboardTabShell unmounts Overview while the parent skeleton shows, so
    // Overview/NeedsAttentionStrip remount onto the same key. Keep the range
    // result fresh long enough that remount does not fire a second network hop.
    staleTime: 30_000,
  });
}

export function useTimeseries(
  businessId: AccountingBizId,
  range: AccountingRange,
) {
  return useQuery<Timeseries>({
    queryKey: accountingKeys.timeseries(businessId, range),
    queryFn: () =>
      accountingApi.getTimeseries(bizId(businessId), range.start, range.end),
    enabled: isBizEnabled(businessId) && !!range.start && !!range.end,
  });
}

export function useEntries(
  businessId: AccountingBizId,
  params: EntriesListParams,
) {
  return useQuery<EntriesPage>({
    queryKey: accountingKeys.entries(businessId, params),
    queryFn: () => accountingApi.listEntries(bizId(businessId), params),
    enabled: isBizEnabled(businessId) && !!params.start && !!params.end,
    // Keep prior page visible while the next page loads (pagination UX).
    placeholderData: keepPreviousData,
  });
}

export function usePayrollRuns(
  businessId: AccountingBizId,
  params: PayrollRunsListParams = {},
) {
  return useQuery<PayrollRunsPage>({
    queryKey: accountingKeys.payrollRuns(businessId, params),
    queryFn: () => accountingApi.listPayrollRunsPage(bizId(businessId), params),
    enabled: isBizEnabled(businessId),
    // Keep prior page visible while the next page loads (pagination UX).
    placeholderData: keepPreviousData,
  });
}

export function usePayrollRun(
  businessId: AccountingBizId,
  id: number,
  options?: { enabled?: boolean },
) {
  const enabled =
    (options?.enabled ?? true) && isBizEnabled(businessId) && id > 0;
  return useQuery<PayrollRun>({
    queryKey: accountingKeys.payrollRun(businessId, id),
    queryFn: () => accountingApi.getPayrollRun(bizId(businessId), id),
    enabled,
  });
}

export function useReceipts(
  businessId: AccountingBizId,
  params: ReceiptsListParams = {},
) {
  const numericId =
    typeof businessId === "number"
      ? businessId
      : Number.parseInt(businessId, 10);
  return useQuery<ReceiptsPage>({
    queryKey: accountingKeys.receipts(businessId, params),
    queryFn: () => fiscalApi.listReceiptsPage(numericId, params),
    enabled:
      isBizEnabled(businessId) && Number.isFinite(numericId) && numericId > 0,
  });
}

/** P&L statement — cached under the accounting root so mutations invalidate it. */
export function useProfitLoss(
  businessId: AccountingBizId,
  range: AccountingRange,
  compare: boolean,
) {
  return useQuery<ProfitLossStatement>({
    queryKey: accountingKeys.profitLoss(businessId, range, compare),
    queryFn: () =>
      accountingApi.getProfitLoss(
        bizId(businessId),
        range.start,
        range.end,
        compare ? "prev" : undefined,
      ),
    enabled: isBizEnabled(businessId) && !!range.start && !!range.end,
  });
}

/** Outstanding (unpaid) bills page — cached under the accounting root. */
export function useUnpaidBills(
  businessId: AccountingBizId,
  params: UnpaidBillsParams,
) {
  return useQuery<UnpaidBillsPage>({
    queryKey: accountingKeys.unpaidBills(businessId, params),
    queryFn: () => accountingApi.getUnpaidBills(bizId(businessId), params),
    enabled: isBizEnabled(businessId) && !!params.start && !!params.end,
    // Keep prior page visible while the next page loads (pagination UX).
    placeholderData: keepPreviousData,
  });
}

/** Debounced search string is the caller's responsibility. */
export function useIssuableBills(
  businessId: AccountingBizId,
  q: string = "",
  options?: { enabled?: boolean },
) {
  const numericId =
    typeof businessId === "number"
      ? businessId
      : Number.parseInt(businessId, 10);
  return useQuery<IssuableBill[]>({
    queryKey: accountingKeys.issuableBills(businessId, q),
    queryFn: () => fiscalApi.listIssuableBills(numericId, q || undefined),
    enabled:
      (options?.enabled ?? true) &&
      isBizEnabled(businessId) &&
      Number.isFinite(numericId) &&
      numericId > 0,
  });
}

// ---------------------------------------------------------------------------
// Mutations — each invalidates the whole accounting tree for the business
// ---------------------------------------------------------------------------

export function useCreateEntry(businessId: AccountingBizId) {
  const queryClient = useQueryClient();
  return useMutation({
    mutationFn: (payload: CreateEntryRequest) =>
      accountingApi.createEntry(bizId(businessId), payload),
    onSuccess: () => invalidateAccountingRoot(queryClient, businessId),
  });
}

export function useVoidEntry(businessId: AccountingBizId) {
  const queryClient = useQueryClient();
  return useMutation({
    mutationFn: (entryId: number) =>
      accountingApi.voidEntry(bizId(businessId), entryId),
    onSuccess: () => invalidateAccountingRoot(queryClient, businessId),
  });
}

export function useCreatePayrollRun(businessId: AccountingBizId) {
  const queryClient = useQueryClient();
  return useMutation({
    mutationFn: (payload: CreatePayrollRunRequest) =>
      accountingApi.createPayrollRun(bizId(businessId), payload),
    onSuccess: () => invalidateAccountingRoot(queryClient, businessId),
  });
}

export function useMarkPayrollRunPaid(businessId: AccountingBizId) {
  const queryClient = useQueryClient();
  return useMutation({
    mutationFn: (runId: number) =>
      accountingApi.markPayrollRunPaid(bizId(businessId), runId),
    onSuccess: () => invalidateAccountingRoot(queryClient, businessId),
  });
}

export function useVoidPayrollRun(businessId: AccountingBizId) {
  const queryClient = useQueryClient();
  return useMutation({
    mutationFn: (runId: number) =>
      accountingApi.voidPayrollRun(bizId(businessId), runId),
    onSuccess: () => invalidateAccountingRoot(queryClient, businessId),
  });
}

export function useDeletePayrollRun(businessId: AccountingBizId) {
  const queryClient = useQueryClient();
  return useMutation({
    mutationFn: (runId: number) =>
      accountingApi.deletePayrollRun(bizId(businessId), runId),
    onSuccess: () => invalidateAccountingRoot(queryClient, businessId),
  });
}

/** Maps to fiscalApi.issueReceipt(businessId, billId, receiver?). */
export function useIssueInvoice(businessId: AccountingBizId) {
  const queryClient = useQueryClient();
  const numericId =
    typeof businessId === "number"
      ? businessId
      : Number.parseInt(String(businessId), 10);
  return useMutation({
    mutationFn: (args: { billId: number; receiver?: IssueReceiverOverride }) =>
      fiscalApi.issueReceipt(numericId, args.billId, args.receiver),
    onSuccess: () => invalidateAccountingRoot(queryClient, businessId),
  });
}

/** Maps to fiscalApi.retryDeliveryTask(businessId, taskId). */
export function useRetryDelivery(businessId: AccountingBizId) {
  const queryClient = useQueryClient();
  const numericId =
    typeof businessId === "number"
      ? businessId
      : Number.parseInt(String(businessId), 10);
  return useMutation({
    mutationFn: (taskId: number) =>
      fiscalApi.retryDeliveryTask(numericId, taskId),
    onSuccess: () => invalidateAccountingRoot(queryClient, businessId),
  });
}

/** Maps to fiscalApi.creditFiscalReceipt(businessId, receiptId, reason, amountCents?). */
export function useCreditNote(businessId: AccountingBizId) {
  const queryClient = useQueryClient();
  const numericId =
    typeof businessId === "number"
      ? businessId
      : Number.parseInt(String(businessId), 10);
  return useMutation({
    mutationFn: ({
      receiptId,
      reason,
      amountCents,
    }: {
      receiptId: number;
      reason: string;
      amountCents?: number;
    }) =>
      fiscalApi.creditFiscalReceipt(numericId, receiptId, reason, amountCents),
    onSuccess: () => invalidateAccountingRoot(queryClient, businessId),
  });
}

/** Maps to fiscalApi.resendFiscalReceipt(businessId, receiptId). */
export function useResendReceipt(businessId: AccountingBizId) {
  const queryClient = useQueryClient();
  const numericId =
    typeof businessId === "number"
      ? businessId
      : Number.parseInt(String(businessId), 10);
  return useMutation({
    mutationFn: (receiptId: number) =>
      fiscalApi.resendFiscalReceipt(numericId, receiptId),
    onSuccess: () => invalidateAccountingRoot(queryClient, businessId),
  });
}
