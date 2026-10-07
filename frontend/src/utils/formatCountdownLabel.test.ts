import { formatCountdownLabel } from "./formatCountdownLabel";

describe("formatCountdownLabel", () => {
  it("renders mm:ss under an hour", () => {
    expect(formatCountdownLabel(14 * 60_000 + 5_000)).toBe("14:05");
    expect(formatCountdownLabel(59 * 60_000 + 59_000)).toBe("59:59");
    expect(formatCountdownLabel(0)).toBe("00:00");
  });

  it("switches to Xh Ym at an hour and beyond — never a 1405:12 wall of minutes", () => {
    expect(formatCountdownLabel(60 * 60_000)).toBe("1h 0m");
    // 1405 minutes 12 seconds — the production regression under a day
    expect(formatCountdownLabel((1405 * 60 + 12) * 1_000)).toBe("23h 25m");
  });

  it("switches to Xd Yh at a day and beyond — never a 1378:23 wall of minutes", () => {
    // 57 days + 10 hours + 23 minutes (demo delivery payment windows)
    const ms = ((57 * 24 + 10) * 60 + 23) * 60_000;
    expect(formatCountdownLabel(ms)).toBe("57d 10h");
    expect(formatCountdownLabel(24 * 60 * 60_000)).toBe("1d 0h");
  });
});
