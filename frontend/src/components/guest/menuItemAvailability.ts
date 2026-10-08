/** Inventory 86 that guest cards must not treat as a normal sellable dish. */
export function isInventoryEightySix(opts: {
  orderabilityState?: string | null;
  inventoryStatus?: string | null;
}): boolean {
  return (
    opts.inventoryStatus === "out_of_stock" ||
    opts.orderabilityState === "inventory_out"
  );
}

// Shared availability predicate for guest menu items. An item is orderable
// unless it is explicitly flagged unavailable (86'd). Handles both the raw
// backend shape (`is_available`) and the transformed guest shape
// (`isAvailable`); either being exactly `false` means not orderable.
// Inventory-out must also refuse sale — live catalog rows keep the stored
// manual flag true and only stamp inventory_status / item_orderability (#728).
export function isMenuItemOrderable(item: unknown): boolean {
  if (!item || typeof item !== "object") return false;
  const rec = item as Record<string, unknown>;
  if (rec.is_available === false) return false;
  if (rec.isAvailable === false) return false;
  if (
    isInventoryEightySix({
      orderabilityState:
        typeof rec.orderability_state === "string"
          ? rec.orderability_state
          : null,
      inventoryStatus:
        typeof rec.inventory_status === "string" ? rec.inventory_status : null,
    })
  ) {
    return false;
  }
  return true;
}

/** Venue-wide closed orderability state (hours), not item-level 86. */
export function isVenueClosedState(state?: string | null): boolean {
  return state === "business_closed";
}

/**
 * When the venue is closed, suppress rose "Unavailable" badges — the closed
 * banner owns that message. Item remains non-orderable / dimmed.
 * Inventory-out and manual 86 still badge when the venue is open.
 */
export function shouldShowItemUnavailableBadge(opts: {
  orderabilityState?: string | null;
  venueClosed?: boolean;
  isAvailable?: boolean;
  inventoryStatus?: string | null;
}): boolean {
  const inventoryEightySix = isInventoryEightySix(opts);
  if (opts.venueClosed || isVenueClosedState(opts.orderabilityState)) {
    // Closed banner owns venue-wide pause. An inventory 86 is still an 86
    // so the card is not a normal after-hours dish (#728).
    return inventoryEightySix;
  }
  if (opts.isAvailable === false) return true;
  return inventoryEightySix;
}

/**
 * Honest reason behind a set of blocked lines/children (#822). Quote and
 * orderability block reasons must not all collapse to "Out of Stock":
 * closed hours say closed, kitchen-off says ordering off, and only a real 86
 * says 86. Item-level reasons outrank venue-wide ones — an 86 is an 86 even
 * after hours, and the sticky catalog `inventory_status` survives the
 * `business_closed` remap ResolveOrderability applies to 86'd items (#727).
 */
export type BlockedReasonKind =
  | "out_of_stock"
  | "unavailable"
  | "business_closed"
  | "ordering_disabled";

const BLOCKED_REASON_RANK: Record<BlockedReasonKind, number> = {
  out_of_stock: 4,
  unavailable: 3,
  business_closed: 2,
  ordering_disabled: 1,
};

export function blockedReasonKind(
  blocked: Array<{
    orderabilityState?: string | null;
    inventoryStatus?: string | null;
  }>,
): BlockedReasonKind | null {
  let kind: BlockedReasonKind | null = null;
  for (const entry of blocked) {
    const state = entry.orderabilityState ?? null;
    const next: BlockedReasonKind = isInventoryEightySix(entry)
      ? "out_of_stock"
      : state === "business_closed"
        ? "business_closed"
        : state === "ordering_disabled"
          ? "ordering_disabled"
          : "unavailable";
    if (kind === null || BLOCKED_REASON_RANK[next] > BLOCKED_REASON_RANK[kind]) {
      kind = next;
    }
  }
  return kind;
}

/** Yellow promotion chips on cards — muted in Closed Mode (no after-hours merchandising). */
export function shouldShowItemPromotionOffers(venueClosed: boolean): boolean {
  return !venueClosed;
}

/**
 * Soft inventory warnings (low stock / empty-available) are also suppressed
 * when closed — venue banner owns the story.
 */
export function shouldShowItemInventoryWarningBadge(opts: {
  orderabilityState?: string | null;
  venueClosed?: boolean;
}): boolean {
  if (opts.venueClosed || isVenueClosedState(opts.orderabilityState)) {
    return false;
  }
  return opts.orderabilityState === "inventory_warning";
}
