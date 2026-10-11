/**
 * N-7: BillDisplayMode must use a single humanizeDuration-backed elapsed
 * formatter. Multi-day ages must never render "450h".
 */
import { formatElapsedLabel } from "../BillDisplayMode";

const tString = (key: string): string => {
  if (key === "display.justNow") return "Just now";
  if (key === "display.elapsedAgo") return "{duration} ago";
  return key;
};

describe("formatElapsedLabel (N-7)", () => {
  it("returns justNow under one minute", () => {
    expect(formatElapsedLabel(0, tString)).toBe("Just now");
  });

  it("humanizes multi-day ages (no raw hour walls)", () => {
    // 18 days ≈ 450h — the audit symptom.
    const label = formatElapsedLabel(450 * 60, tString);
    expect(label).toBe("18d ago");
    expect(label).not.toMatch(/450/);
    expect(label).not.toMatch(/hoursMinutesAgo/);
  });

  it("formats under a day compactly", () => {
    expect(formatElapsedLabel(135, tString)).toBe("2h 15m ago");
  });
});
