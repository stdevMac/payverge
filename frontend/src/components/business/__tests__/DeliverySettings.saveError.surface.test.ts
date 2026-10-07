import { deliverySaveErrorMessage } from "@/components/business/DeliverySettings";
import esApiErrors from "@/i18n/locales/es/apiErrors.json";

/**
 * L3-36: zone validation 400 must surface through the shared error path.
 */
describe("deliverySaveErrorMessage via surfaceBackendError (L3-36)", () => {
  const fallback = "Failed to save delivery settings";

  it("surfaces a coded zone priority error in Spanish", () => {
    const err = {
      response: {
        status: 400,
        data: {
          code: "delivery_zone_priority_duplicate",
          error: "duplicate active zone priority: 1",
        },
      },
    };
    expect(deliverySaveErrorMessage(err, fallback, "es")).toBe(
      esApiErrors.delivery_zone_priority_duplicate,
    );
  });

  it("surfaces uncoded CO-1 domain copy on 400", () => {
    const err = {
      response: {
        status: 400,
        data: { error: "zone 2: name required" },
      },
    };
    expect(deliverySaveErrorMessage(err, fallback, "en")).toBe(
      "zone 2: name required",
    );
  });

  it("does not silently return empty for non-API throws", () => {
    const msg = deliverySaveErrorMessage(new TypeError("boom"), fallback);
    expect(msg).not.toBe("boom");
    expect(msg.length).toBeGreaterThan(0);
  });
});
