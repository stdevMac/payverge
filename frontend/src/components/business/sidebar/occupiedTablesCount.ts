import { getTablesWithStatus } from "@/api/business";
import { isActiveBillStatus } from "@/api/bills";

/**
 * Sidebar Mesas badge source of truth: distinct occupied tables on the floor.
 *
 * Matches `/tables/status` occupancy (#774): an open/partial check occupies
 * its table, and leftover kitchen / pending guest tickets occupy a table even
 * after the check is closed or abandoned (#704 T2/T3/T5). Counter and
 * delivery rows (`table_id` 0) never inflate the host-stand count.
 */

const OCCUPYING_ORDER_STATUSES = new Set([
  "pending",
  "approved",
  "in_kitchen",
  "ready",
]);

export type OccupancyBill = {
  status?: string;
  table_id?: number | string | null;
};

type OccupancyOrder = {
  status?: string;
  table_id?: number | string | null;
  bill?: { table_id?: number | string | null } | null;
};

export type OccupancyOrders =
  | OccupancyOrder[]
  | Record<number | string, OccupancyOrder[] | undefined>
  | null
  | undefined;

function positiveTableId(value: unknown): number | null {
  if (typeof value === "number" && Number.isFinite(value) && value > 0) {
    return value;
  }
  if (typeof value === "string" && /^\d+$/.test(value)) {
    const parsed = Number(value);
    if (parsed > 0) return parsed;
  }
  return null;
}

function tableIdFromOrder(order: OccupancyOrder): number | null {
  return (
    positiveTableId(order.table_id) ?? positiveTableId(order.bill?.table_id)
  );
}

function flattenOrders(orders: OccupancyOrders): OccupancyOrder[] {
  if (!orders) return [];
  if (Array.isArray(orders)) return orders;
  return Object.values(orders).flatMap((group) => group ?? []);
}

/**
 * Host-stand occupancy from `/tables/status` — the same board as the Tables
 * header. Leftover-kitchen rows stay `occupied` with `active_bills_count: 0`
 * after the check is closed or abandoned (#704 T2/T3/T5).
 */
export type FloorOccupancyRow = {
  status?: string;
  table?: { is_active?: boolean } | null;
  active_bills_count?: number;
  active_bills?: unknown[] | null;
};

export function countOccupiedFromFloor(
  rows: FloorOccupancyRow[] | null | undefined,
): number {
  return (rows ?? []).filter((row) => {
    if (row.table && row.table.is_active === false) return false;
    return row.status === "occupied";
  }).length;
}

/** Mesas badge data source: floor status, not the kitchen `activeBillsOnly` feed. */
export async function fetchOccupiedTablesCount(
  businessId: number,
): Promise<number> {
  const { tables } = await getTablesWithStatus(businessId);
  return countOccupiedFromFloor(tables);
}

export function countOccupiedTables(
  bills: OccupancyBill[] | null | undefined,
  orders?: OccupancyOrders,
): number {
  const occupied = new Set<number>();

  for (const bill of bills ?? []) {
    if (!isActiveBillStatus(bill.status)) continue;
    const tableId = positiveTableId(bill.table_id);
    if (tableId !== null) occupied.add(tableId);
  }

  for (const order of flattenOrders(orders)) {
    if (!order.status || !OCCUPYING_ORDER_STATUSES.has(order.status)) {
      continue;
    }
    const tableId = tableIdFromOrder(order);
    if (tableId !== null) occupied.add(tableId);
  }

  return occupied.size;
}
