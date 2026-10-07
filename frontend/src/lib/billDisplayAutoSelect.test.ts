/** @jest-environment node */
import { shouldAutoSelectBillDetail } from "./billDisplayAutoSelect";

describe("shouldAutoSelectBillDetail (L2-27)", () => {
  it("auto-selects once when a default id is available", () => {
    expect(
      shouldAutoSelectBillDetail({
        hasAutoSelected: false,
        defaultSelectionId: 42,
      }),
    ).toBe(true);
  });

  it("does not re-select after the first pass (even if loading flips)", () => {
    expect(
      shouldAutoSelectBillDetail({
        hasAutoSelected: true,
        defaultSelectionId: 42,
      }),
    ).toBe(false);
  });

  it("waits when there is no default selection", () => {
    expect(
      shouldAutoSelectBillDetail({
        hasAutoSelected: false,
        defaultSelectionId: null,
      }),
    ).toBe(false);
  });
});
