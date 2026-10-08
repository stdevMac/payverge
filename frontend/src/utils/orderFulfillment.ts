import { parseOrderItems, type OrderItem } from "@/api/orders";

export interface FulfillmentLine extends OrderItem {
  key: string;
}

/** Lines that become kitchen prep tickets (components, not sellable parents). */
const physicalTypes = new Set(["menu_item", "bundle_item"]);

/**
 * Sellable / covers units for operator summary chips (Needs approval, Tables
 * "prepared units", bill headers). Counts `menu_item` + `bundle` quantities and
 * ignores expanded `bundle_item` children + discounts — otherwise 4× Date Night
 * reads as "16 items" / inflated prepared units.
 */
const commercialTypes = new Set(["menu_item", "bundle"]);

/**
 * Projects commercial order rows into physical kitchen work. Bundle grouping
 * and discount rows are accounting constructs and must never become tickets.
 */
export function toFulfillmentLines(
  value: string | OrderItem[] | undefined,
): FulfillmentLine[] {
  const items = parseOrderItems(value);
  const grouped = new Map<string, FulfillmentLine>();

  for (const item of items) {
    const itemType = item.item_type || "menu_item";
    if (!physicalTypes.has(itemType) || item.quantity <= 0) continue;
    const identity = item.menu_item_id || item.id || item.menu_item_name;
    const key = `${itemType}:${identity}`;
    // Different modifiers/notes remain separate prep lines even when the menu
    // item is the same; truly identical lines are combined for a stable count.
    const signature = `${key}:${item.special_requests || ""}:${JSON.stringify(item.options || [])}`;
    const existing = grouped.get(signature);
    if (existing) {
      existing.quantity += item.quantity;
      existing.subtotal = ((existing.subtotal || 0) + (item.subtotal || 0)) as OrderItem["subtotal"];
      continue;
    }
    grouped.set(signature, { ...item, item_type: itemType, key });
  }
  return Array.from(grouped.values());
}

/**
 * Restaurant-native unit count for expo / host / approval summaries.
 * Prefer this (and the identically-scoped backend `physical_item_quantity`)
 * over summing fulfillment lines when the label is "items" / "prepared units".
 */
export function physicalItemQuantity(
  value: string | OrderItem[] | undefined,
): number {
  const items = parseOrderItems(value);
  return items.reduce((sum, item) => {
    const itemType = item.item_type || "menu_item";
    if (!commercialTypes.has(itemType) || item.quantity <= 0) return sum;
    return sum + item.quantity;
  }, 0);
}
