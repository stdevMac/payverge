/**
 * Issue 791 — desktop Sage must read as a real chat column, not a narrow
 * settings rail. On large screens the sheet widens so message bubbles,
 * suggestion cards, and the composer have room to breathe.
 */
import { AI_WAITER_PANEL_MODAL } from "./aiWaiterPanel";

describe("AI waiter desktop chat column (issue 791)", () => {
  const base = AI_WAITER_PANEL_MODAL.classNames.base;

  it("keeps the tablet-width right sheet as the sm baseline", () => {
    expect(base).toContain("sm:w-[26rem]");
    expect(base).toContain("sm:!max-w-[26rem]");
  });

  it("widens into a chat column on lg and xl desktops", () => {
    expect(base).toContain("lg:w-[30rem]");
    expect(base).toContain("lg:!max-w-[30rem]");
    expect(base).toContain("xl:w-[34rem]");
    expect(base).toContain("xl:!max-w-[34rem]");
  });
});
