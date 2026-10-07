import { COUNTRY_OPTIONS } from "./geoDefaults";
import { ISO_3166_ALPHA2_CODES, getAllCountryOptions } from "./isoCountries";

describe("isoCountries", () => {
  it("covers the full assigned ISO-3166 alpha-2 registry", () => {
    expect(ISO_3166_ALPHA2_CODES.length).toBe(249);
    expect(new Set(ISO_3166_ALPHA2_CODES).size).toBe(249);
    for (const code of ISO_3166_ALPHA2_CODES) {
      expect(code).toMatch(/^[A-Z]{2}$/);
    }
  });

  it("includes every curated geodefault country (registration cannot regress)", () => {
    const all = new Set(ISO_3166_ALPHA2_CODES);
    for (const c of COUNTRY_OPTIONS) {
      expect(all.has(c.code)).toBe(true);
    }
  });

  it("resolves localized display names and sorts by them", () => {
    const options = getAllCountryOptions("en");
    expect(options.length).toBe(249);
    const chile = options.find((o) => o.code === "CL");
    expect(chile?.name).toBe("Chile");
    const names = options.map((o) => o.name);
    expect(names).toEqual([...names].sort((a, b) => a.localeCompare(b, "en")));
  });

  it("previously blocked countries are selectable (NZ, TR, ZA)", () => {
    const codes = new Set(getAllCountryOptions().map((o) => o.code));
    expect(codes.has("NZ")).toBe(true);
    expect(codes.has("TR")).toBe(true);
    expect(codes.has("ZA")).toBe(true);
  });
});
