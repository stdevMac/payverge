/** L3-14: type-aware discount bounds (mirrors BE menu_enhancements.go). */
export function isOfferDiscountValid(
  discountType: "percentage" | "fixed",
  discountValue: number,
): boolean {
  if (!Number.isFinite(discountValue) || discountValue <= 0) return false;
  if (discountType === "percentage" && discountValue > 100) return false;
  return true;
}

export function offerDiscountErrorKey(
  discountType: "percentage" | "fixed",
  discountValue: number,
): string | null {
  if (!Number.isFinite(discountValue) || discountValue <= 0) {
    return "messages.discountValueRequired";
  }
  if (discountType === "percentage" && discountValue > 100) {
    return "messages.discountPercentMax";
  }
  return null;
}
