/** @jest-environment jsdom */
/**
 * Save-payload assertions for DeliverySettings.
 *
 * These tests exercise the real exported `buildDeliverySettingsPayload`
 * helper from DeliverySettings.tsx — NOT a local copy of the logic.
 *
 * Key invariants under test:
 *  (a) payload contains payment_mode
 *  (b) payload does NOT contain delivery_fee_type, distance_based_rate,
 *      delivery_radius, or auto_assign_drivers
 *  (c) new zones (negative ids) are stripped of their id before sending
 */

import { asDollars } from "@/types/money";
import type { DeliverySettingsDto } from "@/api/delivery";
import {
  buildDeliverySettingsPayload,
  deliverySaveErrorMessage,
} from "@/components/business/DeliverySettings";

const baseSettings: DeliverySettingsDto = {
  business_id: 1,
  delivery_enabled: true,
  in_house_delivery_enabled: true,
  third_party_enabled: false,
  payment_mode: "cash_on_delivery",
  online_payment_available: false,
  flat_delivery_fee: asDollars(3.99),
  free_delivery_minimum: asDollars(25),
  minimum_order_amount: asDollars(10),
  estimated_prep_time: 15,
  max_concurrent_deliveries: 5,
  delivery_hours_same_as_business: true,
  delivery_instructions: "",
  external_partner_links: [],
  zones: [],
  partner_fallback_available: false,
  estimated_delivery_minutes: 0,
};

describe("DeliverySettings save payload", () => {
  it("(a) payload contains payment_mode when online payments are available", () => {
    const settings = { ...baseSettings, online_payment_available: true };
    const payload = buildDeliverySettingsPayload(settings);
    expect(payload).toHaveProperty("payment_mode", "cash_on_delivery");
  });

  it("(a) payload carries updated payment_mode when online", () => {
    const settings = {
      ...baseSettings,
      payment_mode: "online" as const,
      online_payment_available: true,
    };
    const payload = buildDeliverySettingsPayload(settings);
    expect(payload.payment_mode).toBe("online");
  });

  // DEL-OP-2: the GET DTO carries the RESOLVED payment mode — the backend maps
  // a stored 'online' preference to 'cash_on_delivery' whenever online payments
  // are unavailable. Round-tripping that resolved value on save would silently
  // overwrite the operator's stored 'online' preference forever. While online
  // payments are unavailable the radio cannot be meaningfully changed, so the
  // payload must omit payment_mode and leave the stored preference untouched.
  it("(a) omits payment_mode when online payments are unavailable (DEL-OP-2)", () => {
    const settings = {
      ...baseSettings,
      payment_mode: "cash_on_delivery" as const,
      online_payment_available: false,
    };
    const payload = buildDeliverySettingsPayload(settings) as Record<string, unknown>;
    expect(payload).not.toHaveProperty("payment_mode");
  });

  it("(b) payload does NOT contain delivery_fee_type", () => {
    const payload = buildDeliverySettingsPayload(baseSettings) as Record<string, unknown>;
    expect(payload).not.toHaveProperty("delivery_fee_type");
  });

  it("(b) payload does NOT contain distance_based_rate", () => {
    const payload = buildDeliverySettingsPayload(baseSettings) as Record<string, unknown>;
    expect(payload).not.toHaveProperty("distance_based_rate");
  });

  it("(b) payload does NOT contain delivery_radius", () => {
    const payload = buildDeliverySettingsPayload(baseSettings) as Record<string, unknown>;
    expect(payload).not.toHaveProperty("delivery_radius");
  });

  it("(b) payload does NOT contain auto_assign_drivers", () => {
    const payload = buildDeliverySettingsPayload(baseSettings) as Record<string, unknown>;
    expect(payload).not.toHaveProperty("auto_assign_drivers");
  });

  it("(c) new zones with negative ids have id stripped", () => {
    const settings: DeliverySettingsDto = {
      ...baseSettings,
      zones: [
        {
          id: -1,
          name: "New Zone",
          delivery_fee: asDollars(4),
          minimum_order_amount: asDollars(15),
          estimated_time: 30,
          priority: 1,
          cutoff_buffer_minutes: 0,
          is_active: true,
          boundaries: {},
        },
      ],
    };
    const payload = buildDeliverySettingsPayload(settings);
    expect(payload.zones).toHaveLength(1);
    expect(payload.zones![0]).not.toHaveProperty("id");
  });

  it("(c) existing zones with positive ids retain their id", () => {
    const settings: DeliverySettingsDto = {
      ...baseSettings,
      zones: [
        {
          id: 42,
          name: "Existing Zone",
          delivery_fee: asDollars(5),
          minimum_order_amount: asDollars(20),
          estimated_time: 45,
          priority: 1,
          cutoff_buffer_minutes: 0,
          is_active: true,
          boundaries: {},
        },
      ],
    };
    const payload = buildDeliverySettingsPayload(settings);
    expect(payload.zones![0]).toHaveProperty("id", 42);
  });

  // L3-38: _client_key is a client-only expansion identity. It must never
  // reach the wire.
  it("(c) strips the client-only _client_key from zones", () => {
    const settings: DeliverySettingsDto = {
      ...baseSettings,
      zones: [
        {
          id: 42,
          _client_key: "zone-abc",
          name: "Existing Zone",
          delivery_fee: asDollars(5),
          minimum_order_amount: asDollars(20),
          estimated_time: 45,
          priority: 1,
          cutoff_buffer_minutes: 0,
          is_active: true,
          boundaries: {},
        },
        {
          id: -1,
          _client_key: "zone-def",
          name: "Draft Zone",
          delivery_fee: asDollars(4),
          minimum_order_amount: asDollars(15),
          estimated_time: 30,
          priority: 2,
          cutoff_buffer_minutes: 0,
          is_active: false,
          boundaries: {},
        },
      ],
    };
    const payload = buildDeliverySettingsPayload(settings);
    for (const zone of payload.zones ?? []) {
      expect(zone).not.toHaveProperty("_client_key");
    }
  });

  it("(c) mixed positive and negative ids handled independently", () => {
    const settings: DeliverySettingsDto = {
      ...baseSettings,
      zones: [
        {
          id: 7,
          name: "Saved Zone",
          delivery_fee: asDollars(3),
          minimum_order_amount: asDollars(10),
          estimated_time: 25,
          priority: 1,
          cutoff_buffer_minutes: 0,
          is_active: true,
          boundaries: {},
        },
        {
          id: -2,
          name: "Draft Zone",
          delivery_fee: asDollars(5),
          minimum_order_amount: asDollars(15),
          estimated_time: 30,
          priority: 2,
          cutoff_buffer_minutes: 0,
          is_active: true,
          boundaries: {},
        },
      ],
    };
    const payload = buildDeliverySettingsPayload(settings);
    expect(payload.zones).toHaveLength(2);
    expect(payload.zones![0]).toHaveProperty("id", 7);
    expect(payload.zones![1]).not.toHaveProperty("id");
  });
});

// DEL-OP-3: the backend 400 for zone-validation failures carries a specific
// message ("duplicate active zone priority: 1", "zone 2: name required", …).
// The save-error toast must surface it instead of collapsing to a generic
// "couldn't save".
describe("deliverySaveErrorMessage", () => {
  const fallback = "Failed to save delivery settings";

  it("surfaces the backend validation message from an axios-shaped error", () => {
    const err = {
      response: { data: { error: "duplicate active zone priority: 1" } },
    };
    expect(deliverySaveErrorMessage(err, fallback)).toBe(
      "duplicate active zone priority: 1",
    );
  });

  it("falls back for errors without a backend message", () => {
    expect(deliverySaveErrorMessage(new Error("network"), fallback)).toBe(fallback);
    expect(deliverySaveErrorMessage(undefined, fallback)).toBe(fallback);
    expect(
      deliverySaveErrorMessage({ response: { data: { error: "   " } } }, fallback),
    ).toBe(fallback);
    expect(
      deliverySaveErrorMessage({ response: { data: { error: 42 } } }, fallback),
    ).toBe(fallback);
  });
});
