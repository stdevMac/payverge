import fs from "fs";
import path from "path";
import {
  formatKdsFireTime,
  kdsElapsedMinutes,
  kdsElapsedUrgency,
  parseOrderInstant,
} from "./kdsTicketTime";
import {
  FILED_NAIVE_FIRE,
  FILED_NOW,
  FILED_UTC_FIRE,
  compactClock,
  expectVenueNyFireClock,
} from "@/components/business/__tests__/_kdsVenueClock";

const VENUE = "America/New_York";

describe("formatKdsFireTime — filed #662 UTC stamp vs venue clock", () => {
  it.each(["en", "es", "es-AR"] as const)(
    "prints the New York clock for the filed 22:12Z stamp in %s",
    (locale) => {
      expectVenueNyFireClock(
        formatKdsFireTime(FILED_UTC_FIRE, locale, VENUE),
        locale,
      );
    },
  );

  it.each(["en", "es", "es-AR"] as const)(
    "treats the naive 22:12 stamp as UTC, not device-local, in %s",
    (locale) => {
      expectVenueNyFireClock(
        formatKdsFireTime(FILED_NAIVE_FIRE, locale, VENUE),
        locale,
      );
      expect(parseOrderInstant(FILED_NAIVE_FIRE).toISOString()).toBe(
        FILED_UTC_FIRE,
      );
    },
  );

  it("is the UTC 22:12 clock only when the zone is missing (failed lookup)", () => {
    expect(compactClock(formatKdsFireTime(FILED_UTC_FIRE, "es", null))).toMatch(
      /22:12/,
    );
  });

  it("parses the fire stamp before formatting (not new Date(naive))", () => {
    const src = fs.readFileSync(path.join(__dirname, "kdsTicketTime.ts"), "utf8");
    expect(src).toMatch(/const instant = parseOrderInstant\(createdAt\)/);
    expect(src).toMatch(/formatBusinessTime\(instant,/);
  });
});

describe("kdsElapsedMinutes — real minutes, not UTC-wall minus NY-wall", () => {
  const now = new Date(FILED_NOW);

  it.each(["en", "es", "es-AR"] as const)(
    "is 5 minutes after the filed 22:12Z fire in %s, not 4h 5m",
    () => {
      expect(kdsElapsedMinutes(FILED_UTC_FIRE, now)).toBe(5);
      expect(kdsElapsedMinutes(FILED_NAIVE_FIRE, now)).toBe(5);
      expect(kdsElapsedMinutes(FILED_UTC_FIRE, now)).not.toBe(4 * 60 + 5);
      expect(kdsElapsedUrgency(kdsElapsedMinutes(FILED_UTC_FIRE, now))).toBe(
        "none",
      );
    },
  );

  it("does not go negative when the clock is slightly behind the fire stamp", () => {
    expect(kdsElapsedMinutes(FILED_NOW, new Date(FILED_UTC_FIRE))).toBe(0);
  });
});
