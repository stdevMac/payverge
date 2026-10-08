/**
 * Tenant-isolation guard for operator bill views (audit L9-1).
 *
 * `getBill` is keyed by bill id only; operators with multi-business access can
 * deep-link a foreign `?billId=` into the wrong business shell. Callers must
 * compare the nested `bill.business_id` from `BillWithItemsResponse` against
 * the business currently being viewed before opening payment/void UI.
 *
 * Returns true when the bill must NOT be shown in the current shell.
 * Missing / non-numeric business_id fails closed (treat as foreign).
 */
export function isForeignBill(
  billBusinessId: number | null | undefined,
  currentBusinessId: number,
): boolean {
  if (billBusinessId == null || !Number.isFinite(Number(billBusinessId))) {
    return true;
  }
  return Number(billBusinessId) !== Number(currentBusinessId);
}
