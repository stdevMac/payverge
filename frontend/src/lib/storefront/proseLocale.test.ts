import {
  sanitizeStorefrontProse,
  storefrontProseMatchesLocale,
} from "./proseLocale";

const ARABIC = "مطعم نموذجي في قلب المدينة";
const CHINESE = "示范餐厅，提供现代菜单";
const ENGLISH = "A model restaurant in the heart of the city";

describe("storefrontProseMatchesLocale", () => {
  it("rejects Arabic prose on zh (#642)", () => {
    expect(storefrontProseMatchesLocale(ARABIC, "zh")).toBe(false);
    expect(storefrontProseMatchesLocale(CHINESE, "zh")).toBe(true);
  });

  it("rejects Arabic prose on en (#686)", () => {
    expect(storefrontProseMatchesLocale(ARABIC, "en")).toBe(false);
    expect(storefrontProseMatchesLocale(ENGLISH, "en")).toBe(true);
  });

  it("keeps Arabic when the requested locale is Arabic", () => {
    expect(storefrontProseMatchesLocale(ARABIC, "ar")).toBe(true);
  });
});

describe("sanitizeStorefrontProse", () => {
  it("falls back when ?lang=en would otherwise leak Arabic into OG copy", () => {
    expect(sanitizeStorefrontProse(ARABIC, "en", ENGLISH)).toBe(ENGLISH);
    expect(sanitizeStorefrontProse(CHINESE, "zh", ENGLISH)).toBe(CHINESE);
  });
});
