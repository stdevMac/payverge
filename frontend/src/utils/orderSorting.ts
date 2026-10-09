import type { Order } from "@/api/orders";

/**
 * K-7: kitchen FIFO — oldest ticket first (created_at ascending). The KDS
 * columns previously never sorted, the standalone fetch arrived
 * created_at DESC, and embedded mode used a third ordering; kitchens work
 * the longest-waiting ticket first. Returns a new array (no mutation).
 */
export function sortOrdersFifo<T extends Pick<Order, "created_at">>(
  orders: T[],
): T[] {
  return [...orders].sort(
    (a, b) =>
      new Date(a.created_at).getTime() - new Date(b.created_at).getTime(),
  );
}
