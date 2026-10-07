import { localDateKey, localDateTimeInputValue } from "./localDate";

describe("localDateKey", () => {
  it("formats a local date as YYYY-MM-DD from its local calendar components", () => {
    // The multi-arg Date constructor interprets its args as LOCAL time, so the
    // local calendar day is Jan 15 in every timezone — this test is
    // deterministic regardless of the runner's TZ (CI runs in UTC).
    const d = new Date(2026, 0, 15, 23, 30, 0); // 23:30 local on Jan 15
    // A toISOString()-based impl returns the UTC day, which on any non-zero
    // offset machine (e.g. the Americas in the evening, where UTC has already
    // rolled to Jan 16) differs from the local day. Asserting against the
    // local-getter key catches that regression wherever it can manifest; the
    // concrete value pins the expected result.
    const localKey = `${d.getFullYear()}-${String(d.getMonth() + 1).padStart(2, "0")}-${String(d.getDate()).padStart(2, "0")}`;
    expect(localDateKey(d)).toBe(localKey);
    expect(localDateKey(d)).toBe("2026-01-15");
  });

  it("zero-pads month and day", () => {
    expect(localDateKey(new Date(2026, 2, 5))).toBe("2026-03-05");
  });

  it("defaults to now", () => {
    const now = new Date();
    const expected = `${now.getFullYear()}-${String(now.getMonth() + 1).padStart(2, "0")}-${String(now.getDate()).padStart(2, "0")}`;
    expect(localDateKey()).toBe(expected);
  });
});

describe("localDateTimeInputValue", () => {
  it("formats local YYYY-MM-DDTHH:mm for datetime-local inputs", () => {
    const d = new Date(2026, 5, 6, 9, 5);
    expect(localDateTimeInputValue(d)).toBe("2026-06-06T09:05");
  });
});
