/**
 * Budget numbers only — primary D1 DOM proof is guestMenuChrome.rv4.dom.test.tsx.
 */
import {
  GUEST_MENU_CHROME_BUDGET_MAX_PX,
  GUEST_MENU_HEADER_OFFSET_PX,
  GUEST_MENU_PERMANENT_CHROME_PX,
  GUEST_NAV_HEIGHT_PX,
} from "./guestMenuChrome";

describe("RV-4 guest menu chrome budget (secondary)", () => {
  it("keeps permanent chrome under the budget gate", () => {
    expect(GUEST_NAV_HEIGHT_PX).toBe(88);
    expect(GUEST_MENU_HEADER_OFFSET_PX).toBeLessThan(200);
    expect(GUEST_MENU_PERMANENT_CHROME_PX).toBeLessThanOrEqual(
      GUEST_MENU_CHROME_BUDGET_MAX_PX,
    );
  });
});
