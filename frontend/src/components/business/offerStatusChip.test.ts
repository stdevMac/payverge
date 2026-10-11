/**
 * #835: on the live demo venue offer 13 "$5 Off the Steak Plate" comes back
 * `is_active: true` AND `inventory_blocked: true` while Premium Beef sits at
 * 0 kg. The operator card rendered BOTH a green "Active" chip and a warning
 * "Paused by inventory" chip — two chips asserting opposite things about the
 * same row. Guests already never see the offer (filterGuestLivePromotions),
 * so the effective state is paused and only one chip may speak.
 */
import { offerManualActive, offerStatusChip } from "./offerStatusChip";

describe("#835 offer status chip", () => {
  it("does not paint Active when the target dish is inventory-blocked", () => {
    expect(offerStatusChip({ is_active: true, inventory_blocked: true })).toEqual({
      tone: "warning",
      labelKey: "inventoryBlockedChip",
      tooltipKey: "inventoryBlockedTooltip",
    });
  });

  it("keeps Active for an offer on a sellable dish", () => {
    expect(offerStatusChip({ is_active: true, inventory_blocked: false })).toEqual({
      tone: "success",
      labelKey: "status.active",
      tooltipKey: undefined,
    });
    // Flag absent (offer targets a category/bundle, or an older payload).
    expect(offerStatusChip({ is_active: true })).toEqual({
      tone: "success",
      labelKey: "status.active",
      tooltipKey: undefined,
    });
  });

  it("lets the operator's own off switch win over inventory", () => {
    // An offer the operator turned off is Inactive, full stop — inventory is
    // moot and a second chip would just be noise.
    expect(offerStatusChip({ is_active: false, inventory_blocked: true })).toEqual({
      tone: "default",
      labelKey: "status.inactive",
      tooltipKey: undefined,
    });
    expect(offerStatusChip({ is_active: false, inventory_blocked: false })).toEqual({
      tone: "default",
      labelKey: "status.inactive",
      tooltipKey: undefined,
    });
  });

  it("never returns the Active label alongside the blocked tooltip", () => {
    const blocked = offerStatusChip({ is_active: true, inventory_blocked: true });
    expect(blocked.labelKey).not.toBe("status.active");
  });
});

/**
 * #835 round two: the chip alone was presentation-only. The live verifier reads
 * the payload, and GetOffers on venue 142 still answered `is_active: true` for
 * "AR$ 3.000 menos en el bife" with `inventory_blocked: true` on the same row.
 * The backend now reports is_active as the EFFECTIVE answer and moves the
 * operator's stored switch to `manual_active`, so the chip has to read the
 * switch from there or a blocked offer regresses to a plain "Inactive" chip.
 */
describe("#835 offerManualActive", () => {
  it("reads the stored switch, not the inventory-adjusted flag", () => {
    // The live 142 shape after the backend fix.
    expect(
      offerManualActive({
        is_active: false,
        manual_active: true,
      }),
    ).toBe(true);
  });

  it("reports an offer the operator switched off", () => {
    expect(offerManualActive({ is_active: false, manual_active: false })).toBe(false);
  });

  it("falls back to is_active when the field is absent", () => {
    expect(offerManualActive({ is_active: true })).toBe(true);
    expect(offerManualActive({ is_active: false })).toBe(false);
  });
});

describe("#835 chip on the post-fix payload", () => {
  it("still says Paused by inventory when is_active is the effective false", () => {
    expect(
      offerStatusChip({
        is_active: false,
        manual_active: true,
        inventory_blocked: true,
      }),
    ).toEqual({
      tone: "warning",
      labelKey: "inventoryBlockedChip",
      tooltipKey: "inventoryBlockedTooltip",
    });
  });

  it("says Inactive when the operator's own switch is off", () => {
    expect(
      offerStatusChip({
        is_active: false,
        manual_active: false,
        inventory_blocked: true,
      }),
    ).toEqual({
      tone: "default",
      labelKey: "status.inactive",
      tooltipKey: undefined,
    });
  });

  it("keeps Active for a stocked target", () => {
    expect(
      offerStatusChip({
        is_active: true,
        manual_active: true,
        inventory_blocked: false,
      }),
    ).toEqual({
      tone: "success",
      labelKey: "status.active",
      tooltipKey: undefined,
    });
  });
});
