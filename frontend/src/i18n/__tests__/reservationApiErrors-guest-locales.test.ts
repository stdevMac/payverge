import fs from "fs";
import path from "path";

// Reservation error codes a diner can hit from the public booking, confirm and
// cancel flows. Each guest locale must carry its own wording; an English value
// copied into fr/apiErrors.json renders English to a French diner.
const GUEST_REACHABLE_RESERVATION_CODES = [
  "reservation_status_conflict",
  "reservation_slot_unavailable",
  "reservation_min_advance",
  "reservation_in_past",
  "reservation_max_advance",
  "reservation_business_closed",
  "reservation_outside_hours",
  "reservation_party_size",
  "reservation_covers_limit",
  "reservation_no_tables",
  "reservation_cancel_window",
  "reservation_arrival_window",
  "reservation_invalid_transition",
  "reservation_invalid_duration",
  "reservation_invalid_time",
  "reservation_email_required",
] as const;

const localesDir = path.resolve(__dirname, "..", "locales");

function readCatalog(locale: string): Record<string, string> {
  return JSON.parse(
    fs.readFileSync(path.join(localesDir, locale, "apiErrors.json"), "utf8"),
  );
}

const en = readCatalog("en");
const nonEnglishLocales = fs
  .readdirSync(localesDir)
  .filter(
    (entry) =>
      entry !== "en" &&
      fs.existsSync(path.join(localesDir, entry, "apiErrors.json")),
  );

describe("reservation API errors in guest locales", () => {
  it("covers every guest locale directory", () => {
    expect(nonEnglishLocales.length).toBeGreaterThanOrEqual(20);
  });

  it.each(GUEST_REACHABLE_RESERVATION_CODES)(
    "%s has an English source string",
    (code) => {
      expect(typeof en[code]).toBe("string");
    },
  );

  it.each(nonEnglishLocales)("%s translates every reservation code", (locale) => {
    const catalog = readCatalog(locale);
    const untranslated = GUEST_REACHABLE_RESERVATION_CODES.filter(
      (code) => typeof catalog[code] !== "string" || catalog[code] === en[code],
    );
    expect(untranslated).toEqual([]);
  });
});
