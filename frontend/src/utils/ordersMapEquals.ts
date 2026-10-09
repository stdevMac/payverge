import type { Order } from "@/api/orders";

/**
 * K-1: shallow-compares two bill_id → Order[] maps by the fields a poll tick
 * can change in a board-relevant way: id, status, updated_at. The dashboard
 * uses this to keep `globalOrders` referentially stable across no-op 60s
 * polls, so child effects (Kitchen list + full-screen KDS) don't re-fire and
 * tear the board down during quiet periods.
 */
export function ordersMapEquals(
  a: Record<number, Order[]>,
  b: Record<number, Order[]>,
): boolean {
  const aKeys = Object.keys(a);
  const bKeys = Object.keys(b);
  if (aKeys.length !== bKeys.length) return false;
  for (const key of aKeys) {
    const aList = a[Number(key)];
    const bList = b[Number(key)];
    if (!bList || aList.length !== bList.length) return false;
    for (let i = 0; i < aList.length; i += 1) {
      const x = aList[i];
      const y = bList[i];
      if (
        x.id !== y.id ||
        x.status !== y.status ||
        x.updated_at !== y.updated_at
      ) {
        return false;
      }
    }
  }
  return true;
}
