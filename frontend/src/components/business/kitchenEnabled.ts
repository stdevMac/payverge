/** Derive the Kitchen tab on/off flag from an already-loaded business row. */

export function kitchenEnabledFromBusiness(biz: {
  kitchen_enabled?: boolean;
  orders_enabled?: boolean;
} | null | undefined): boolean | null {
  if (!biz) return null;
  if (
    typeof biz.kitchen_enabled !== "boolean" ||
    typeof biz.orders_enabled !== "boolean"
  ) {
    return null;
  }
  // Only seed the ON state. A false pair may be a Go zero-value from a stub
  // or a narrow projection (staff dashboard row, omitted SELECT) and must
  // not paint "kitchen off" before kitchen-orders-status settles (#663).
  if (biz.kitchen_enabled && biz.orders_enabled) return true;
  return null;
}

export type KitchenBoardGate = "loading" | "on" | "off";

/**
 * First-paint gate for the Kitchen tab.
 * - known-on renders the live board even while kitchen-orders-status refetches
 * - known-off is trusted only after that fetch settles
 * - anything else (null, or false-while-loading) is a real loading state
 */
export function kitchenBoardGate(opts: {
  kitchenEnabled: boolean | null;
  kitchenStatusLoading: boolean;
}): KitchenBoardGate {
  if (opts.kitchenEnabled === true) return "on";
  if (opts.kitchenEnabled === false && !opts.kitchenStatusLoading) return "off";
  return "loading";
}
