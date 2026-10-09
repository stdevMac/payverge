import {
  brandRgbChannels,
  isPublicDemo,
  legalNameOf,
  siteNameOf,
  parseInstanceInfo,
  productNameOf,
  serializeInstanceScript,
} from "./instanceInfo";

const base = {
  registration_mode: "invite",
  features: { ai: false, email: true },
};

describe("parseInstanceInfo", () => {
  it("rejects unknown modes and non-objects", () => {
    // billing_mode is gone; a body without it (what the server sends) parses.
    expect(parseInstanceInfo({ ...base })).not.toBeNull();
    expect(parseInstanceInfo(null)).toBeNull();
    expect(parseInstanceInfo({ ...base, registration_mode: "x" })).toBeNull();
  });

  it("defaults the brand and sanitises urls, emails and colours", () => {
    const info = parseInstanceInfo({
      ...base,
      logo_url: "javascript:alert(1)",
      support_email: "not-an-email",
      brand_color: "red",
      public_url: "https://pos.example/",
    });
    expect(info).not.toBeNull();
    expect(productNameOf(info)).toBe("Payverge");
    expect(info!.logo_url).toBe("");
    expect(info!.support_email).toBe("");
    expect(info!.brand_color).toBe("#1a6b6a");
    expect(info!.public_url).toBe("https://pos.example");
    expect(info!.features.ai).toBe(false);
    expect(info!.features.email).toBe(true);
    expect(info!.features.crypto).toBe(false);
  });

  it("names the legal entity before the company and the product", () => {
    const info = parseInstanceInfo({ ...base, product_name: "Trattoria OS", company_name: "Trattoria SRL" });
    expect(legalNameOf(info)).toBe("Trattoria SRL");
    expect(legalNameOf(null)).toBe("Payverge");
  });

  it("names the site after the company, then the product", () => {
    const info = parseInstanceInfo({ ...base, product_name: "Trattoria OS", company_name: "Trattoria", legal_entity: "Trattoria SRL" });
    expect(siteNameOf(info)).toBe("Trattoria");
    expect(siteNameOf(parseInstanceInfo({ ...base, product_name: "Trattoria OS", company_name: "" }))).toBe("Trattoria OS");
    expect(siteNameOf(null)).toBe("Payverge");
  });

  it("emits CSS channels only for a non-default colour", () => {
    expect(brandRgbChannels(parseInstanceInfo({ ...base, brand_color: "#1a6b6a" }))).toBeNull();
    expect(brandRgbChannels(parseInstanceInfo({ ...base, brand_color: "#f00" }))).toBe("255 0 0");
  });

  it("escapes the seed script against </script>", () => {
    const info = parseInstanceInfo({ ...base, product_name: "</script><b>" });
    expect(serializeInstanceScript(info)).not.toContain("</script>");
  });

  it("reads the public-demo mode and reset time", () => {
    const off = parseInstanceInfo({ ...base, demo: { enabled: true } });
    expect(off!.demo).toEqual({ enabled: true, mode: false, reset_utc: "" });
    expect(isPublicDemo(off)).toBe(false);
    expect(isPublicDemo(null)).toBe(false);

    const on = parseInstanceInfo({ ...base, demo: { enabled: true, mode: true, reset_utc: "04:30" } });
    expect(isPublicDemo(on)).toBe(true);
    expect(on!.demo.reset_utc).toBe("04:30");

    const junk = parseInstanceInfo({ ...base, demo: { enabled: true, mode: "yes", reset_utc: "<b>" } });
    expect(isPublicDemo(junk)).toBe(false);
    const badTime = parseInstanceInfo({ ...base, demo: { enabled: true, mode: true, reset_utc: "25:99" } });
    expect(badTime!.demo.reset_utc).toBe("03:00");
  });
});
