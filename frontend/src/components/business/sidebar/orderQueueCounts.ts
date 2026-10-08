/**
 * Rail-badge order queues, derived from the dashboard's shared orders feed
 * (`globalOrders`, grouped by bill id) — the same feed the Kitchen tab and the
 * Bills approval strip render.
 *
 * #792: the Kitchen rail badge previously counted `kitchen_order_ready`
 * alerts, so it could read 1 (or blank) while two Start Cooking cards sat on
 * the Approved tab. The badge now means "tickets waiting on the cook right
 * now": needs-approval (pending) + approved.
 *
 * #793: the Bills rail badge previously counted open checks, so a host chasing
 * the red pip landed on an empty approval queue. The badge now means "tickets
 * waiting for approval" (pending); open/partial check totals stay as table
 * copy inside the tab.
 */

type BadgeOrder = { status?: string };
type OrdersByBill =
  | Record<number | string, BadgeOrder[] | undefined>
  | null
  | undefined;

function flatten(orders: OrdersByBill): BadgeOrder[] {
  if (!orders) return [];
  return Object.values(orders).flatMap((group) => group ?? []);
}

/** Kitchen rail badge: tickets waiting on the cook (pending + approved). */
export function countCookQueue(orders: OrdersByBill): number {
  return flatten(orders).filter(
    (o) => o.status === "pending" || o.status === "approved",
  ).length;
}

/** Bills rail badge: tickets waiting for approval (pending only). */
export function countPendingApprovals(orders: OrdersByBill): number {
  return flatten(orders).filter((o) => o.status === "pending").length;
}
