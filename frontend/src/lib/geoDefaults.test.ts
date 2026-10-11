import {
  COUNTRY_OPTIONS,
  BUSINESS_TYPE_OPTIONS,
  getDefaultsForCountry,
  defaultRegisterCountryForLocale,
} from "./geoDefaults";

describe("getDefaultsForCountry", () => {
  it("derives Argentina", () => {
    expect(getDefaultsForCountry("AR")).toEqual({
      currency: "ARS",
      timezone: "America/Argentina/Buenos_Aires",
    });
  });
  it("derives US", () => {
    expect(getDefaultsForCountry("US")).toEqual({
      currency: "USD",
      timezone: "America/New_York",
    });
  });
  it("is case-insensitive and trims", () => {
    expect(getDefaultsForCountry("  mx ")).toEqual({
      currency: "MXN",
      timezone: "America/Mexico_City",
    });
  });
  it("preselects Argentina on es-AR register (#912)", () => {
    expect(defaultRegisterCountryForLocale("es-AR")).toBe("AR");
    expect(defaultRegisterCountryForLocale("es-ar")).toBe("AR");
    expect(getDefaultsForCountry(defaultRegisterCountryForLocale("es-AR"))).toEqual({
      currency: "ARS",
      timezone: "America/Argentina/Buenos_Aires",
    });
    expect(defaultRegisterCountryForLocale("en")).toBe("");
    expect(defaultRegisterCountryForLocale("es")).toBe("");
  });

  it("falls back to USD/UTC for unknown", () => {
    expect(getDefaultsForCountry("ZZ")).toEqual({ currency: "USD", timezone: "UTC" });
    expect(getDefaultsForCountry("")).toEqual({ currency: "USD", timezone: "UTC" });
  });
  it("derives the LatAm expansion countries", () => {
    expect(getDefaultsForCountry("CL")).toEqual({
      currency: "CLP",
      timezone: "America/Santiago",
    });
    expect(getDefaultsForCountry("CO")).toEqual({
      currency: "COP",
      timezone: "America/Bogota",
    });
    expect(getDefaultsForCountry("PE")).toEqual({
      currency: "PEN",
      timezone: "America/Lima",
    });
    expect(getDefaultsForCountry("EC")).toEqual({
      currency: "USD",
      timezone: "America/Guayaquil",
    });
  });
});

describe("option lists", () => {
  it("country options cover the derivation table", () => {
    expect(COUNTRY_OPTIONS.find((c) => c.code === "AR")).toBeTruthy();
    expect(COUNTRY_OPTIONS.length).toBeGreaterThanOrEqual(39);
  });
  it("business types include other as last resort", () => {
    expect(BUSINESS_TYPE_OPTIONS.map((b) => b.value)).toContain("other");
  });
});
