import { shouldShowAiWaiter } from "./aiGate";

describe("shouldShowAiWaiter", () => {
  it("shows when ai_available is true and id present", () => {
    expect(shouldShowAiWaiter({ id: 1, ai_available: true })).toBe(true);
  });
  it("hides when ai_available is false", () => {
    expect(shouldShowAiWaiter({ id: 1, ai_available: false })).toBe(false);
  });
  it("hides when ai_available is undefined (no AI plan signal)", () => {
    expect(shouldShowAiWaiter({ id: 1 })).toBe(false);
  });
  it("hides when business is null", () => {
    expect(shouldShowAiWaiter(null)).toBe(false);
  });
});
