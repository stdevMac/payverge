import { canActivateZone, zoneActivationBlockReason } from "./zoneActivation";
import { buildDeliverySettingsPayload } from "../DeliverySettings";
import type { DeliverySettingsDto } from "@/api/delivery";
import { asDollars } from "@/types/money";
import { newZone } from "./ZonesSection";

describe("L3-40 zone activation validity", () => {
  it("rejects activation without name or geography", () => {
    expect(canActivateZone({ name: "", boundaries: {} })).toBe(false);
    expect(zoneActivationBlockReason({ name: "", boundaries: {} })).toBe("name");
    expect(
      canActivateZone({ name: "Centro", boundaries: {} }),
    ).toBe(false);
    expect(
      zoneActivationBlockReason({ name: "Centro", boundaries: {} }),
    ).toBe("geography");
  });

  it("allows activation with name + postal or cities", () => {
    expect(
      canActivateZone({
        name: "Centro",
        boundaries: { postal_codes: ["1000"] },
      }),
    ).toBe(true);
    expect(
      canActivateZone({
        name: "Norte",
        boundaries: { cities: ["CABA"] },
      }),
    ).toBe(true);
  });
});

describe("L3-40 in_house_delivery_enabled interaction via real payload builder", () => {
  const base = {
    business_id: 1,
    delivery_enabled: true,
    in_house_delivery_enabled: false,
    third_party_enabled: false,
    payment_mode: "cash_on_delivery" as const,
    online_payment_available: true,
    external_partner_links: [],
    zones: [] as DeliverySettingsDto["zones"],
  } as unknown as DeliverySettingsDto;

  it("newZone (inactive) does not flip in_house_delivery_enabled", () => {
    const zone = newZone([]);
    expect(zone.is_active).toBe(false);
    const payload = buildDeliverySettingsPayload({
      ...base,
      zones: [zone],
    });
    expect(payload.in_house_delivery_enabled).toBe(false);
  });

  it("activating a valid zone flips in_house_delivery_enabled via payload helper", () => {
    const zone = {
      ...newZone([]),
      name: "Centro",
      is_active: true,
      boundaries: { postal_codes: ["1000"] },
      delivery_fee: asDollars(0),
      minimum_order_amount: asDollars(0),
    };
    expect(canActivateZone(zone)).toBe(true);
    const payload = buildDeliverySettingsPayload({
      ...base,
      zones: [zone],
    });
    expect(payload.in_house_delivery_enabled).toBe(true);
  });
});
