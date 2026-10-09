import { wallTimeToInstant } from "@/utils/zonedDateTime";
import {
  DINNER_WINDOW,
  LUNCH_WINDOW,
  daypartCoverageMinutes,
  shiftWindowOverlapMinutes,
} from "./scheduleCoverage";

const TZ = "America/New_York";
const DAY = "2026-08-14";

describe("scheduleCoverage (#143)", () => {
  it("counts a dinner shift inside the dinner window and not lunch", () => {
    const start = wallTimeToInstant(`${DAY}T17:00`, TZ).toISOString();
    const end = wallTimeToInstant(`${DAY}T23:00`, TZ).toISOString();
    expect(
      shiftWindowOverlapMinutes(start, end, DAY, TZ, DINNER_WINDOW),
    ).toBe(5 * 60);
    expect(
      shiftWindowOverlapMinutes(start, end, DAY, TZ, LUNCH_WINDOW),
    ).toBe(0);
  });

  it("clips an overnight dinner closer onto both calendar days", () => {
    const start = wallTimeToInstant(`${DAY}T18:00`, TZ).toISOString();
    const end = wallTimeToInstant(`2026-08-15T02:00`, TZ).toISOString();
    expect(
      shiftWindowOverlapMinutes(start, end, DAY, TZ, DINNER_WINDOW),
    ).toBe(4 * 60);
    expect(
      shiftWindowOverlapMinutes(start, end, "2026-08-15", TZ, DINNER_WINDOW),
    ).toBe(0);
  });

  it("flags an unstaffed dinner when every published shift is a lunch block", () => {
    const lunch = {
      starts_at: wallTimeToInstant(`${DAY}T10:00`, TZ).toISOString(),
      ends_at: wallTimeToInstant(`${DAY}T15:00`, TZ).toISOString(),
    };
    expect(daypartCoverageMinutes([lunch], DAY, TZ, LUNCH_WINDOW)).toBe(4 * 60);
    expect(daypartCoverageMinutes([lunch], DAY, TZ, DINNER_WINDOW)).toBe(0);
  });
});
