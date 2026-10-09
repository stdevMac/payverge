export interface OrderStatusLike {
  id: number;
  status: string;
}

export interface OrderTransitions {
  staffCancelled: number[];
  becameReady: number[];
}

// Pure transition detector for the guest 3s order poll: an order fires only
// when we previously saw it in a DIFFERENT status — orders already
// cancelled/ready at mount never toast. Callers own dedup sets and updating
// the known-status map afterwards.
export function detectOrderTransitions(
  orders: OrderStatusLike[],
  knownStatuses: ReadonlyMap<number, string>,
): OrderTransitions {
  const staffCancelled: number[] = [];
  const becameReady: number[] = [];
  for (const order of orders) {
    const prev = knownStatuses.get(order.id);
    if (prev === undefined || prev === order.status) continue;
    if (order.status === "cancelled") staffCancelled.push(order.id);
    if (order.status === "ready") becameReady.push(order.id);
  }
  return { staffCancelled, becameReady };
}
