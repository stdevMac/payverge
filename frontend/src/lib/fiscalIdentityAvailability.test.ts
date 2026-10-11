import { shouldShowFiscalIdentityFields } from "./fiscalIdentityAvailability";

describe("shouldShowFiscalIdentityFields (#548)", () => {
  it("hides the AFIP identity form for a NY/US venue", () => {
    expect(shouldShowFiscalIdentityFields({ country: "US" })).toBe(false);
    expect(shouldShowFiscalIdentityFields({ country: " us " })).toBe(false);
  });

  it("fails closed when country or settings are unknown", () => {
    expect(shouldShowFiscalIdentityFields({})).toBe(false);
    expect(shouldShowFiscalIdentityFields({ country: "" })).toBe(false);
    expect(shouldShowFiscalIdentityFields({ country: null })).toBe(false);
    expect(shouldShowFiscalIdentityFields({ country: undefined })).toBe(false);
    expect(
      shouldShowFiscalIdentityFields({ country: "AR", fiscalCountry: "" }),
    ).toBe(false);
    expect(
      shouldShowFiscalIdentityFields({ country: "AR", fiscalCountry: null }),
    ).toBe(false);
  });

  it("hides the form for non-AR fiscal countries (AE has no CUIT flow)", () => {
    expect(shouldShowFiscalIdentityFields({ country: "AE" })).toBe(false);
    expect(shouldShowFiscalIdentityFields({ fiscalCountry: "AE" })).toBe(false);
  });

  it("shows the form only for Argentina", () => {
    expect(shouldShowFiscalIdentityFields({ country: "AR" })).toBe(true);
    expect(shouldShowFiscalIdentityFields({ country: "ar" })).toBe(true);
    expect(shouldShowFiscalIdentityFields({ fiscalCountry: "AR" })).toBe(true);
  });

  it("lets already-loaded fiscal settings win over the business address", () => {
    expect(
      shouldShowFiscalIdentityFields({ country: "AR", fiscalCountry: "US" }),
    ).toBe(false);
    expect(
      shouldShowFiscalIdentityFields({ country: "US", fiscalCountry: "AR" }),
    ).toBe(true);
  });
});
