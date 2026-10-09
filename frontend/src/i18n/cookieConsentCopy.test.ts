/**
 * @jest-environment jsdom
 */
import {
  isGuestSurfacePath,
  loadCookieConsentCopy,
  resolveCookieConsentLocale,
  COOKIE_CONSENT_GUEST_KEYS,
} from "./cookieConsentCopy";

describe("cookieConsentCopy (NEW-3)", () => {
  it("detects guest surface paths", () => {
    expect(isGuestSurfacePath("/t/ABC/menu")).toBe(true);
    expect(isGuestSurfacePath("/b/demo")).toBe(true);
    expect(isGuestSurfacePath("/scan")).toBe(true);
    expect(isGuestSurfacePath("/pricing")).toBe(false);
    expect(isGuestSurfacePath("/business/register")).toBe(false);
  });

  it("returns Arabic banner copy on guest path with ar locale cookie", async () => {
    document.cookie = "payverge_guest_locale=ar; path=/";
    // Pass pathname explicitly — jsdom location is non-configurable.
    const { copy, locale } = await loadCookieConsentCopy("en", "/t/ABC/bill");
    expect(locale).toBe("ar");
    expect(copy.title).toBe("نحن نقدر خصوصيتك");
    expect(copy.title).not.toBe("We value your privacy");
    expect(copy.acceptAll).toBe("قبول الكل");
    expect(copy.declineAll).toBeTruthy();
  });

  it("uses operator locale on marketing paths", async () => {
    const { copy, locale } = await loadCookieConsentCopy("es", "/pricing");
    expect(locale).toBe("es");
    expect(copy.title).toBe("Tu privacidad nos importa");
    expect(resolveCookieConsentLocale("es", "/contact")).toBe("es");
  });

  it("lists all required guest cookie keys", () => {
    expect(COOKIE_CONSENT_GUEST_KEYS.length).toBeGreaterThanOrEqual(15);
    expect(COOKIE_CONSENT_GUEST_KEYS).toContain("cookies.banner.title");
    expect(COOKIE_CONSENT_GUEST_KEYS).toContain("cookies.footer.preferences");
  });
});
