/**
 * N-6: ServiceCallsQueue ages must roll over to days via humanizeDuration.
 * Revert-proof: exercises the exported formatter, not a source grep.
 */
import { serviceCallTimeAgo } from "./ServiceCallsQueue";

const t = (key: string, p?: Record<string, string | number>): string => {
  if (key === "serviceCalls.justNow") return "Just now";
  if (key === "serviceCalls.elapsedAgo") {
    return `${p?.duration ?? ""} ago`;
  }
  return key;
};

describe("serviceCallTimeAgo (N-6)", () => {
  const now = Date.parse("2026-08-06T12:00:00Z");

  it("returns justNow under one minute", () => {
    expect(
      serviceCallTimeAgo("2026-08-06T11:59:30Z", t, now),
    ).toBe("Just now");
  });

  it("humanizes multi-day ages instead of raw hour walls", () => {
    // 26 days → 26d, never "648h"
    const iso = "2026-07-11T12:00:00Z";
    const label = serviceCallTimeAgo(iso, t, now);
    expect(label).toBe("26d ago");
    expect(label).not.toMatch(/648/);
    expect(label).not.toMatch(/\d{2,}h/);
  });

  it("formats under a day as compact hours", () => {
    const iso = "2026-08-06T09:00:00Z"; // 3h
    expect(serviceCallTimeAgo(iso, t, now)).toBe("3h ago");
  });
});
