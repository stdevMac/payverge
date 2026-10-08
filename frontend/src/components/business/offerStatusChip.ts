import type { Offer } from "@/api/business";

export type OfferStatusChip = {
  tone: "success" | "default" | "warning";
  labelKey: "status.active" | "status.inactive" | "inventoryBlockedChip";
  tooltipKey?: "inventoryBlockedTooltip";
};

/**
 * #835: the offers card used to render the is_active chip unconditionally and
 * then hang a warning "Paused by inventory" chip next to it, so offer 13
 * ("$5 Off the Steak Plate", inventory_blocked with Premium Beef at 0 kg)
 * showed a green Active chip and a paused chip side by side.
 *
 * Inventory already suppresses the offer on every guest surface
 * (filterGuestLivePromotions), so "Active" is not a second opinion — it is
 * wrong. One chip carries the effective state; is_active stays the operator's
 * own switch, editable in the modal, and is not repainted here.
 */
/**
 * The operator's own on/off switch, independent of inventory.
 *
 * Operator offer reads report `is_active` as the EFFECTIVE answer — false
 * while inventory blocks the target dish — and carry the stored column in
 * `manual_active` (#835). Anything that reads the switch rather than the
 * outcome has to round-trip this one. Payloads without the field (legacy
 * caches, the create form, optimistic local rows) fall back to is_active.
 */
export function offerManualActive(
  offer: Pick<Offer, "is_active" | "manual_active">,
): boolean {
  if (typeof offer.manual_active === "boolean") return offer.manual_active;
  return offer.is_active !== false;
}

export function offerStatusChip(
  offer: Pick<Offer, "is_active" | "inventory_blocked" | "manual_active">,
): OfferStatusChip {
  if (!offerManualActive(offer)) {
    // The operator turned it off. Inventory is moot; a second chip is noise.
    return { tone: "default", labelKey: "status.inactive", tooltipKey: undefined };
  }
  if (offer.inventory_blocked === true) {
    return {
      tone: "warning",
      labelKey: "inventoryBlockedChip",
      tooltipKey: "inventoryBlockedTooltip",
    };
  }
  return { tone: "success", labelKey: "status.active", tooltipKey: undefined };
}
