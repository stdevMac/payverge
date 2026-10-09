/** Operator seed copy that must never appear on the public storefront (#718). */
export function isGuestFacingDeliveryNote(
  note?: string | null,
): boolean {
  const trimmed = (note || "").trim();
  if (!trimmed) return false;
  return !/dispatch board/i.test(trimmed);
}
