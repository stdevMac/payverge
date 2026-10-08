import { resolveReportTimezone } from "./reportTimezone";
import {
  listIanaTimezoneIds,
  reportTimezoneSelectOptions,
} from "./reportTimezoneOptions";
import { TIMEZONE_OPTIONS } from "./timezones";

describe("resolveReportTimezone", () => {
  it("prefers a saved config timezone over the business timezone", () => {
    expect(
      resolveReportTimezone("America/New_York", "America/Argentina/Buenos_Aires"),
    ).toBe("America/New_York");
  });

  it("falls back to the business timezone when config is empty", () => {
    expect(resolveReportTimezone("", "America/Argentina/Buenos_Aires")).toBe(
      "America/Argentina/Buenos_Aires",
    );
    expect(resolveReportTimezone(null, "Europe/Madrid")).toBe("Europe/Madrid");
    expect(resolveReportTimezone(undefined, "Asia/Dubai")).toBe("Asia/Dubai");
  });

  it("falls back to UTC when both config and business are empty", () => {
    expect(resolveReportTimezone("", "")).toBe("UTC");
    expect(resolveReportTimezone(null, null)).toBe("UTC");
    expect(resolveReportTimezone(undefined, undefined)).toBe("UTC");
    expect(resolveReportTimezone("   ", "  ")).toBe("UTC");
  });

  it("treats whitespace-only config as unset so business TZ wins", () => {
    expect(resolveReportTimezone("  ", "America/Mexico_City")).toBe(
      "America/Mexico_City",
    );
  });
});

describe("reportTimezoneSelectOptions (L6-33)", () => {
  // Geodefaults AR zone — curated TIMEZONE_OPTIONS only had America/Buenos_Aires
  // so Select selectedKeys could not match resolveReportTimezone's default.
  const AR_GEODEFAULT = "America/Argentina/Buenos_Aires";

  it("includes the geodefaults AR IANA zone (not only the legacy city alias)", () => {
    const curated = TIMEZONE_OPTIONS.map((o) => o.value);
    // Documents the historical gap the fix closes.
    expect(curated).not.toContain(AR_GEODEFAULT);

    const ids = listIanaTimezoneIds(AR_GEODEFAULT);
    expect(ids).toContain(AR_GEODEFAULT);
    expect(ids).toContain("UTC");
  });

  it("always includes an arbitrary businessTimezone even if not curated", () => {
    const obscure = "Pacific/Easter";
    const ids = listIanaTimezoneIds(obscure);
    expect(ids).toContain(obscure);
  });

  it("returns options whose values cover default from resolveReportTimezone", () => {
    const defaultTz = resolveReportTimezone(null, AR_GEODEFAULT);
    const options = reportTimezoneSelectOptions(AR_GEODEFAULT);
    expect(options.some((o) => o.value === defaultTz)).toBe(true);
    // Full IANA list is far larger than the ~35-city curated set.
    expect(options.length).toBeGreaterThan(TIMEZONE_OPTIONS.length);
  });
});
