import {
  buildDeliveryToggleSettings,
  buildReservationTogglePayload,
} from "@/utils/partnerFallbackToggles";

describe("partner fallback toggle helpers", () => {
  it("flips only delivery_enabled when enabling, preserving sub-flag state", () => {
    const updated = buildDeliveryToggleSettings(
      {
        external_partner_links: [
          {
            name: "Talabat",
            url: "https://www.talabat.com/store/example",
            provider_key: "talabat",
          },
        ],
        third_party_enabled: false,
        in_house_delivery_enabled: false,
      },
      true,
    );

    // Master flag flipped on; sub-flags untouched (server accepts setup-in-progress).
    expect(updated.delivery_enabled).toBe(true);
    expect(updated.third_party_enabled).toBe(false);
    expect(updated.in_house_delivery_enabled).toBe(false);
    expect(updated.external_partner_links).toEqual([
      {
        name: "Talabat",
        url: "https://www.talabat.com/store/example",
        provider_key: "talabat",
      },
    ]);
  });

  it("preserves prior sub-flag state when re-enabling", () => {
    // Merchant who previously had partners enabled, disabled delivery, now
    // re-enables — partner mode should come back without manual reconfig.
    const updated = buildDeliveryToggleSettings(
      {
        third_party_enabled: true,
        in_house_delivery_enabled: false,
      },
      true,
    );

    expect(updated.delivery_enabled).toBe(true);
    expect(updated.third_party_enabled).toBe(true);
    expect(updated.in_house_delivery_enabled).toBe(false);
  });

  it("hides partner visibility when disabling delivery", () => {
    const updated = buildDeliveryToggleSettings(
      {
        external_partner_links: [
          {
            name: "Talabat",
            url: "https://www.talabat.com/store/example",
            provider_key: "talabat",
          },
        ],
        third_party_enabled: true,
      },
      false,
    );

    expect(updated.delivery_enabled).toBe(false);
    expect(updated.third_party_enabled).toBe(false);
    expect(updated.in_house_delivery_enabled).toBe(false);
    expect(updated.external_partner_links).toHaveLength(1);
  });

  it("does not clear reservation partner fallback links when enabling direct reservations", () => {
    const payload = buildReservationTogglePayload(true);

    expect(payload).toEqual({ enabled: true });
    expect(payload).not.toHaveProperty("external_partner_links");
  });

  it("strips resolved payment_mode so the toggle cannot clobber stored preference", () => {
    const updated = buildDeliveryToggleSettings(
      {
        payment_mode: "cash_on_delivery",
        online_payment_available: false,
        third_party_enabled: true,
      },
      true,
    );

    expect(updated.delivery_enabled).toBe(true);
    expect(updated).not.toHaveProperty("payment_mode");
  });
});
