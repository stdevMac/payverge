import { resetWindowCopy } from "./dailyLimitReset";

describe("resetWindowCopy", () => {
  it.each([
    [0, "resetsSoon", 0],
    [300, "resetsSoon", 0],
    [1799, "resetsSoon", 0],
    [1800, "resetsHour", 1],
    [3600, "resetsHour", 1],
    [5399, "resetsHour", 1],
    [5400, "resets", 2],
    [21600, "resets", 6],
  ] as const)("maps %s seconds → %s (hours=%s)", (seconds, key, hours) => {
    expect(resetWindowCopy(seconds)).toEqual({ key, hours });
  });
});
