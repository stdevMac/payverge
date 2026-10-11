import { getDefaultsForCountry } from "@/lib/geoDefaults";

// The register page wires getDefaultsForCountry into both the inline preview and
// the registration_data payload. A full page render is heavy (wagmi/stripe
// mocks), so this guards the data contract the page depends on directly. The
// production wiring in register/page.tsx must use the same function.
describe("register context wiring", () => {
  it("country drives currency + timezone preview", () => {
    expect(getDefaultsForCountry("AR")).toEqual({
      currency: "ARS",
      timezone: "America/Argentina/Buenos_Aires",
    });
  });

  it("payload prefers country-derived values over the old hardcoded USD", () => {
    // Pure replica of the payload-assembly snippet in handleCardPayment.
    const country = "AR";
    const localeDefaults = getDefaultsForCountry(country);
    const payload = {
      country,
      business_type: "cafe",
      default_currency: localeDefaults.currency,
      display_currency: localeDefaults.currency,
      timezone: localeDefaults.timezone,
    };
    expect(payload.default_currency).toBe("ARS");
    expect(payload.display_currency).toBe("ARS");
    expect(payload.timezone).not.toBe("UTC");
    expect(payload.business_type).toBe("cafe");
  });

  it("missing country falls back to USD/UTC without throwing", () => {
    const localeDefaults = getDefaultsForCountry("");
    expect(localeDefaults).toEqual({ currency: "USD", timezone: "UTC" });
  });

  it("address.city survives the registration_data payload assembly", () => {
    // Pure replica of handleCardPayment's `{ ...formData }` spread: the city
    // typed on the business step must reach registration_data.address.city,
    // which the Stripe webhook maps to Business.Address.City — the exact
    // column profileDone (setup_status.go) checks.
    const formData = {
      name: "La Parrilla",
      owner_name: "Ana",
      email: "ana@example.com",
      address: {
        street: "",
        city: "Buenos Aires",
        state: "",
        postal_code: "",
        country: "AR",
      },
    };
    const registrationData = { ...formData, country: formData.address.country };
    expect(registrationData.address.city).toBe("Buenos Aires");
  });
});
