import { formatCountdownLabel } from "./PendingOrdersSection";

describe("formatCountdownLabel", () => {
  it("renders mm:ss under an hour", () => {
    expect(formatCountdownLabel(14 * 60_000 + 5_000)).toBe("14:05");
    expect(formatCountdownLabel(59 * 60_000 + 59_000)).toBe("59:59");
  });

  it("switches to Xh Ym at an hour and beyond — never a 1405:12 wall of minutes", () => {
    expect(formatCountdownLabel(60 * 60_000)).toBe("1h 0m");
    // 1405 minutes 12 seconds — the production regression
    expect(formatCountdownLabel((1405 * 60 + 12) * 1_000)).toBe("23h 25m");
  });
});
