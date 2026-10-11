/**
 * @jest-environment jsdom
 */
import {
  getOrCreateCheckoutRequestId,
  clearCheckoutRequestId,
  computeCheckoutFingerprint,
} from "./checkoutRequestId";

const FP_A = "fp-cart-a";
const FP_B = "fp-cart-b";

describe("delivery checkout request id", () => {
  beforeEach(() => sessionStorage.clear());

  it("mints an id on first call and reuses it until cleared (retry-safe)", () => {
    const first = getOrCreateCheckoutRequestId(42, FP_A);
    expect(first).toBeTruthy();
    // A retry after a network failure must reuse the SAME id so the backend
    // replay path dedupes instead of creating a second order.
    expect(getOrCreateCheckoutRequestId(42, FP_A)).toBe(first);
  });

  it("scopes ids per business", () => {
    const a = getOrCreateCheckoutRequestId(42, FP_A);
    const b = getOrCreateCheckoutRequestId(43, FP_A);
    expect(a).not.toBe(b);
  });

  it("clears on success so the next order gets a fresh id", () => {
    const first = getOrCreateCheckoutRequestId(42, FP_A);
    clearCheckoutRequestId(42);
    expect(getOrCreateCheckoutRequestId(42, FP_A)).not.toBe(first);
  });

  it("mints a NEW id when the checkout content changes (fingerprint mismatch)", () => {
    // Money-path edge: order A created server-side but the response was lost,
    // then the guest EDITS the cart and resubmits. Reusing the old id would
    // make the backend replay order A while the guest believes they ordered B.
    const first = getOrCreateCheckoutRequestId(42, FP_A);
    const second = getOrCreateCheckoutRequestId(42, FP_B);
    expect(second).not.toBe(first);
    // The new pair is now the persisted one: retrying B reuses B's id.
    expect(getOrCreateCheckoutRequestId(42, FP_B)).toBe(second);
  });

  it("ignores a legacy plain-string stored id (pre-fingerprint format)", () => {
    sessionStorage.setItem("payverge_delivery_checkout_rid:42", "legacy-id");
    const id = getOrCreateCheckoutRequestId(42, FP_A);
    expect(id).not.toBe("legacy-id");
    expect(getOrCreateCheckoutRequestId(42, FP_A)).toBe(id);
  });
});

describe("computeCheckoutFingerprint", () => {
  it("is deterministic for identical content", () => {
    const payload = {
      items: [{ menu_item_name: "Burger", quantity: 1, price: 12 }],
      delivery_address: { street: "1 Main St", city: "Dubai", country: "AE" },
      driver_tip: 2,
    };
    expect(computeCheckoutFingerprint(payload)).toBe(
      computeCheckoutFingerprint(JSON.parse(JSON.stringify(payload))),
    );
  });

  it("changes when cart content changes", () => {
    const base = {
      items: [{ menu_item_name: "Burger", quantity: 1, price: 12 }],
      delivery_address: { street: "1 Main St", city: "Dubai", country: "AE" },
    };
    const edited = {
      ...base,
      items: [{ menu_item_name: "Burger", quantity: 2, price: 12 }],
    };
    expect(computeCheckoutFingerprint(edited)).not.toBe(
      computeCheckoutFingerprint(base),
    );
  });
});
