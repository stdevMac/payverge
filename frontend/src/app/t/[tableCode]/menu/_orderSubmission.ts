import { asDollars } from "@/types/money";
import type { Dollars } from "@/types/money";
import type { CartItem } from "./_types";

export interface GuestOrderItemPayload {
  menu_item_name: string;
  menu_item_id?: string;
  quantity: number;
  price: Dollars;
  item_type: CartItem["itemType"];
  bundle_id?: number;
  parent_bundle_id?: number;
  source_offer_id?: number;
  options: {
    id: string;
    name: string;
    price_change: Dollars;
    is_required: boolean;
  }[];
  special_requests: string;
}

/** Shared cart → wire-payload mapping for BOTH guest submit paths. */
export function buildGuestOrderItems(
  cart: CartItem[],
  resolveMenuItemIdByName: (lowerName: string) => string | undefined,
): GuestOrderItemPayload[] {
  return cart.map((item) => {
    const addOns = item.itemType === "menu_item" ? item.addOns : undefined;
    const options = addOns
      ? addOns.map((addon) => ({
          // G-5: real option id end-to-end; the slug remains only as legacy
          // tolerance for carts persisted before ids were stored (backend
          // name-fallback still resolves those).
          id:
            addon.id ||
            `addon-${addon.name.toLowerCase().replace(/\s+/g, "-")}`,
          name: addon.name,
          price_change: asDollars(addon.price),
          is_required: false,
        }))
      : [];

    const resolvedMenuItemId =
      item.itemType === "menu_item"
        ? item.menuItemId || resolveMenuItemIdByName(item.name.toLowerCase())
        : "menuItemId" in item
          ? item.menuItemId
          : undefined;

    return {
      menu_item_name: item.name,
      menu_item_id: resolvedMenuItemId,
      quantity: item.quantity,
      price: asDollars(item.price),
      item_type: item.itemType,
      bundle_id: item.itemType === "bundle" ? item.bundleId : undefined,
      parent_bundle_id:
        item.itemType === "bundle_item" ? item.parentBundleId : undefined,
      source_offer_id:
        item.itemType === "bundle" || item.itemType === "discount"
          ? item.sourceOfferId
          : undefined,
      options,
      special_requests: item.specialRequests || "",
    };
  });
}

/**
 * G-3: the idempotency key is derived from cart CONTENT only — no path
 * discriminator ("create-bill"/"add-items"), no bill id, no notes. A retry
 * that switches from the create-bill path to the add-items path (G-2
 * recovery, or a network-failure retry after the server committed) must
 * reuse the same X-Request-Id so the backend dedupe catches it.
 */
export function buildCartSignature(
  tableCode: string,
  items: GuestOrderItemPayload[],
): string {
  return JSON.stringify({ tableCode, items });
}

/**
 * Matches a cart line against authoritative menu-item IDs without relying on
 * its array position. Bundle parents match when any resolved child is blocked.
 * Call this inside a functional state update so intervening cart edits cannot
 * shift an old index onto an unrelated line.
 */
export function cartItemContainsBlockedMenuItem(
  item: CartItem,
  blockedItemIds: ReadonlySet<string>,
  resolveBundleMenuItemIds: (bundleId: number) => readonly string[],
  resolveMenuItemIdByName: (lowerName: string) => string | undefined = () =>
    undefined,
): boolean {
  const directMenuItemId =
    "menuItemId" in item && item.menuItemId
      ? item.menuItemId
      : item.itemType === "menu_item"
        ? resolveMenuItemIdByName(item.name.toLowerCase())
        : undefined;
  if (directMenuItemId && blockedItemIds.has(directMenuItemId)) {
    return true;
  }
  return (
    item.itemType === "bundle" &&
    resolveBundleMenuItemIds(item.bundleId).some((menuItemId) =>
      blockedItemIds.has(menuItemId),
    )
  );
}
