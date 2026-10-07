import type { BillItem } from "@/api/bills";

/** Catalog / parent id used to associate bundle parents with their children. */
function getBundleCatalogID(item: BillItem): string | number {
  return (
    item.parent_bundle_id ||
    item.parentBundleId ||
    item.bundle_id ||
    item.bundleId ||
    ""
  );
}

export function getBillItemType(item: BillItem): string {
  return item.item_type || item.itemType || "menu_item";
}

/** Promo / loyalty discount lines (rendered in their own emerald band). */
export function isDiscountBillLine(item: BillItem): boolean {
  return getBillItemType(item) === "discount";
}

/**
 * Operator food/item rows: excludes nested bundle children and discounts so
 * promos never look like orderable menu lines (#107).
 */
export function isOperatorFoodBillLine(item: BillItem): boolean {
  const type = getBillItemType(item);
  return type !== "bundle_item" && type !== "discount";
}

/**
 * Map each bill line id → a stable group key so repeated bundle occurrences
 * (same catalog bundle_id) nest only their own children.
 *
 * Prefer `bundle_occurrence_id` when present; otherwise reconstruct from
 * sequential parent/child order in legacy snapshots.
 */
export function buildBundleGroupKeys(items: BillItem[]): Map<string, string> {
  const groupByItemID = new Map<string, string>();
  const latestLegacyGroup = new Map<string, string>();
  const legacyCounts = new Map<string, number>();
  const pendingChildren = new Map<string, string[]>();

  for (const item of items) {
    const occurrenceID =
      item.bundle_occurrence_id || item.bundleOccurrenceId || "";
    if (occurrenceID) {
      groupByItemID.set(item.id, `occurrence:${occurrenceID}`);
      continue;
    }

    const itemType = getBillItemType(item);
    const bundleID = String(getBundleCatalogID(item));
    if (!bundleID) continue;

    if (itemType === "bundle") {
      const count = (legacyCounts.get(bundleID) || 0) + 1;
      legacyCounts.set(bundleID, count);
      const key = `legacy:${bundleID}:${count}`;
      groupByItemID.set(item.id, key);
      latestLegacyGroup.set(bundleID, key);
      for (const childID of pendingChildren.get(bundleID) || []) {
        groupByItemID.set(childID, key);
      }
      pendingChildren.delete(bundleID);
    } else if (itemType === "bundle_item") {
      const key = latestLegacyGroup.get(bundleID);
      if (key) {
        groupByItemID.set(item.id, key);
      } else {
        const pending = pendingChildren.get(bundleID) || [];
        pending.push(item.id);
        pendingChildren.set(bundleID, pending);
      }
    }
  }

  return groupByItemID;
}

export function resolveBundleGroupKey(
  item: BillItem,
  groupByItemID: Map<string, string>,
): string {
  const reconstructedKey = groupByItemID.get(item.id);
  if (reconstructedKey) return reconstructedKey;
  const occurrenceID =
    item.bundle_occurrence_id || item.bundleOccurrenceId || "";
  if (occurrenceID) return `occurrence:${occurrenceID}`;
  const bundleID = getBundleCatalogID(item);
  return bundleID ? `bundle:${bundleID}` : "";
}

/** Guest-facing line count: parents only (no nested bundle children / discounts). */
export function countGuestFacingBillItems(items: BillItem[] | undefined): number {
  if (!items?.length) return 0;
  return items.reduce((total, item) => {
    const type = getBillItemType(item);
    if (type === "bundle_item" || type === "discount") return total;
    return total + (item.quantity || 1);
  }, 0);
}

/** True when a bundle child is priced into the parent (no separate charge). */
export function isIncludedBundleChild(item: BillItem): boolean {
  const subtotal = Number(item.subtotal ?? 0);
  if (getBillItemType(item) === "bundle_item") {
    // Catalog Price on included children is display-only. Subtotal 0 is the
    // billable signal — requiring price===0 showed $0.00 after the stamp (#708).
    return subtotal === 0;
  }
  const price = Number(item.price ?? 0);
  return subtotal === 0 && price === 0;
}
