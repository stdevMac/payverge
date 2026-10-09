import { translateApiError } from "@/i18n/apiErrors";
import { localizedErrorMessage } from "@/utils/localizedError";
import { deliverySaveErrorMessage } from "@/components/business/DeliverySettings";

describe("inventory + delivery operator error codes", () => {
  it("localizes inventory_insufficient_stock for es-AR", () => {
    expect(
      translateApiError({ code: "inventory_insufficient_stock" }, "es-AR"),
    ).toContain("stock suficiente");
  });

  it("localizedErrorMessage prefers catalog code over status-only map", () => {
    const err = {
      response: {
        status: 409,
        data: {
          code: "inventory_insufficient_stock",
          error: "Insufficient stock to approve this order",
        },
      },
    };
    expect(localizedErrorMessage(err, "es-AR")).toContain("stock suficiente");
  });

  it("deliverySaveErrorMessage uses catalog when locale + code are present", () => {
    const err = {
      response: {
        status: 400,
        data: {
          code: "delivery_fee_negative",
          error: "delivery fees and minimums cannot be negative",
        },
      },
    };
    expect(deliverySaveErrorMessage(err, "fallback", "es")).toContain(
      "no pueden ser negativos",
    );
  });

  it("deliverySaveErrorMessage still surfaces raw English without locale", () => {
    const err = {
      response: {
        data: { error: "duplicate active zone priority: 1" },
      },
    };
    expect(deliverySaveErrorMessage(err, "fallback")).toBe(
      "duplicate active zone priority: 1",
    );
  });
});
