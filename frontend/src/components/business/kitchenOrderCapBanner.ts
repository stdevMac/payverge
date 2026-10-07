/**
 * #684 — oldest-live-orders sort hint must render once across both surfaces.
 *
 * Dashboard chrome always owns `warnings.activeOrdersCapped` when it passes
 * the order map (even `{}`). Kitchen may paint `warnings.kitchenOrdersCapped`
 * only in true standalone mode. Do not invert this: if Kitchen also paints
 * when `parentCapped`, a dashboard merge that keeps the parent strip will
 * stack the amber copy again.
 */

export function parentOwnsKitchenOrders(
  globalOrders: Record<number, unknown> | undefined,
): boolean {
  return globalOrders !== undefined;
}

/** Kitchen stays quiet whenever the parent owns the order map. */
export function shouldShowKitchenOrderCapBanner(input: {
  parentOwnsOrders: boolean;
  fallbackCapped: boolean;
}): boolean {
  return !input.parentOwnsOrders && input.fallbackCapped;
}
