import { guestLandingBillPresentation } from "./guestLandingBill";

describe("guestLandingBillPresentation", () => {
  it("shows remaining due and partial status for a half-paid bill", () => {
    const view = guestLandingBillPresentation({
      total_amount: 36.08,
      paid_amount: 18.04,
      status: "partial",
    });
    expect(view.amount).toBeCloseTo(18.04);
    expect(view.statusKey).toBe("partial");
  });

  it("keeps the gross total and Open for an unpaid open bill", () => {
    const view = guestLandingBillPresentation({
      total_amount: 36.08,
      paid_amount: 0,
      status: "open",
    });
    expect(view.amount).toBeCloseTo(36.08);
    expect(view.statusKey).toBe("open");
  });
});
