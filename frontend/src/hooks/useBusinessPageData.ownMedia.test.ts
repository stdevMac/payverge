/** @jest-environment jsdom */
import { normalizeExternalPartnerLinks } from "./useBusinessPageData";

describe("normalizeExternalPartnerLinks icon_url", () => {
  const link = (icon_url: string) => ({
    name: "Partner",
    url: "https://partner.example.com/r/acme",
    icon_url,
  });

  it("keeps uploaded icons stored as relative /media URLs", () => {
    const [out] = normalizeExternalPartnerLinks([
      link("/media/businesses/1/partner-icons/0123456789abcdef_icon.png"),
    ]);
    expect(out?.icon_url).toBe(
      "/media/businesses/1/partner-icons/0123456789abcdef_icon.png",
    );
  });

  it("keeps absolute http(s) icons and drops other relative paths", () => {
    expect(
      normalizeExternalPartnerLinks([link("https://cdn.example.com/i.png")])[0]
        ?.icon_url,
    ).toBe("https://cdn.example.com/i.png");
    expect(normalizeExternalPartnerLinks([link("/api/v1/x.png")])).toEqual([]);
    expect(
      normalizeExternalPartnerLinks([link("/media/../api/x.png")]),
    ).toEqual([]);
  });
});
