import type { FiscalStatus } from "@/api/fiscal";

/**
 * A fiscal receipt is a "failure" when AFIP/ARCA rejected it, cancelled it, or
 * a retry/permanent error left it un-issued. Shared by AccountingDashboard's
 * inner badge and the sidebar's persistent nav badge so the two never drift.
 */
export const INVOICE_FAILURE_STATUSES: readonly FiscalStatus[] = [
  "failed_retryable",
  "failed_permanent",
  "rejected",
  "cancelled",
] as const;

export function isInvoiceFailure(status: FiscalStatus): boolean {
  return (INVOICE_FAILURE_STATUSES as readonly string[]).includes(status);
}
