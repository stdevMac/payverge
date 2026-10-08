import { isGuestFacingDeliveryNote } from "./deliveryGuestNote";

describe("isGuestFacingDeliveryNote", () => {
  it("hides the English operator dispatch-board seed", () => {
    expect(
      isGuestFacingDeliveryNote(
        "Use the delivery dispatch board for assignment.",
      ),
    ).toBe(false);
    expect(
      isGuestFacingDeliveryNote("Please use the Dispatch Board."),
    ).toBe(false);
  });

  it("keeps a real guest-facing note", () => {
    expect(isGuestFacingDeliveryNote("Ring the demo bell.")).toBe(true);
    expect(isGuestFacingDeliveryNote("Dejá en recepción")).toBe(true);
  });

  it("treats empty as not guest-facing copy", () => {
    expect(isGuestFacingDeliveryNote("")).toBe(false);
    expect(isGuestFacingDeliveryNote("   ")).toBe(false);
    expect(isGuestFacingDeliveryNote(null)).toBe(false);
  });
});
