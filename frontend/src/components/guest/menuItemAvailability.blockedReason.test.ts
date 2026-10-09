/**
 * Issue 822 — quote/orderability block reasons must map to honest guest copy.
 * Closed hours say closed, kitchen-off says ordering off, and only a real 86
 * says out of stock. The sticky catalog `inventory_status` beats the masked
 * `business_closed` remap ResolveOrderability applies after hours (#727).
 */
import { blockedReasonKind } from "./menuItemAvailability";

describe("blockedReasonKind (issue 822)", () => {
  it("returns null for no blocked entries", () => {
    expect(blockedReasonKind([])).toBeNull();
  });

  it("maps business_closed to the closed-hours reason, not out-of-stock", () => {
    expect(
      blockedReasonKind([{ orderabilityState: "business_closed" }]),
    ).toBe("business_closed");
  });

  it("maps ordering_disabled to the ordering-off reason", () => {
    expect(
      blockedReasonKind([{ orderabilityState: "ordering_disabled" }]),
    ).toBe("ordering_disabled");
  });

  it("maps inventory_out to a real 86", () => {
    expect(blockedReasonKind([{ orderabilityState: "inventory_out" }])).toBe(
      "out_of_stock",
    );
  });

  it("keeps an 86 an 86 after hours: sticky inventory_status beats the business_closed mask", () => {
    expect(
      blockedReasonKind([
        {
          orderabilityState: "business_closed",
          inventoryStatus: "out_of_stock",
        },
      ]),
    ).toBe("out_of_stock");
  });

  it("maps manual_disabled to a plain unavailable", () => {
    expect(blockedReasonKind([{ orderabilityState: "manual_disabled" }])).toBe(
      "unavailable",
    );
  });

  it("prefers the item-level reason over venue-wide reasons in a mixed set", () => {
    expect(
      blockedReasonKind([
        { orderabilityState: "business_closed" },
        { orderabilityState: "inventory_out" },
      ]),
    ).toBe("out_of_stock");
    expect(
      blockedReasonKind([
        { orderabilityState: "ordering_disabled" },
        { orderabilityState: "manual_disabled" },
      ]),
    ).toBe("unavailable");
  });
});
