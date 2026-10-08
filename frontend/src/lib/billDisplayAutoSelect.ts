/**
 * L2-27: pure gate for the Pantalla auto-select effect.
 * detailLoading must NOT participate — including it re-fires the effect when
 * handleViewBill sets loading true, aborts its own request, and leaves the
 * spinner stuck (hasAutoSelected blocks retry).
 */
export function shouldAutoSelectBillDetail(params: {
  hasAutoSelected: boolean;
  defaultSelectionId: number | null;
}): boolean {
  return !params.hasAutoSelected && params.defaultSelectionId != null;
}
