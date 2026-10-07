/**
 * N-4: Reservaciones header must print a human timezone label, not raw IANA
 * (e.g. "New York (EST/EDT)" instead of "America/New_York").
 */
import fs from "fs";
import path from "path";
import { getTimezoneLabel } from "@/utils/timezones";

const SRC = path.resolve(
  __dirname,
  "../ReservationManager.tsx",
);

describe("ReservationManager timezone header (N-4)", () => {
  it("wires getTimezoneLabel into the timesShownInTimezone caveat", () => {
    const src = fs.readFileSync(SRC, "utf8");
    expect(src).toMatch(/getTimezoneLabel/);
    expect(src).toMatch(/timesShownInTimezone/);
    // Must not pass the raw IANA id as the {tz} replacement alone.
    expect(src).toMatch(
      /timesShownInTimezone[\s\S]{0,200}getTimezoneLabel\(businessTimezone\)/,
    );
  });

  it("maps America/New_York to a human label", () => {
    expect(getTimezoneLabel("America/New_York")).toBe("New York (EST/EDT)");
    expect(getTimezoneLabel("America/New_York")).not.toBe("America/New_York");
  });
});
