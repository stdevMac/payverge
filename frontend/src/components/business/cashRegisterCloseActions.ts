/** Wave 4: the close-of-day panel offers "Record variance adjustment" only
 *  when the drawer actually came up over/short. */
export function shouldOfferVarianceAdjustment(
  variance: number | undefined | null,
): boolean {
  return typeof variance === "number" && variance !== 0;
}
