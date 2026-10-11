/** @jest-environment jsdom */
/**
 * PG-12: reservation micro-pages honor the guest language cookie.
 */
import { resolveReservationInitialLanguage } from "@/lib/reservationLocale";
import { GUEST_LOCALE_COOKIE } from "@/i18n/guestLocaleResolver";

describe("resolveReservationInitialLanguage (PG-12)", () => {
  beforeEach(() => {
    document.cookie = `${GUEST_LOCALE_COOKIE}=; Max-Age=0; Path=/`;
  });

  it("prefers ?lang= over the guest cookie", () => {
    document.cookie = `${GUEST_LOCALE_COOKIE}=es`;
    expect(resolveReservationInitialLanguage("fr")).toBe("fr");
  });

  it("uses the guest cookie when ?lang= is absent", () => {
    document.cookie = `${GUEST_LOCALE_COOKIE}=ja`;
    expect(resolveReservationInitialLanguage(null)).toBe("ja");
  });

  it("defaults to English when neither signal is present", () => {
    expect(resolveReservationInitialLanguage(undefined)).toBe("en");
  });

  it("ignores an invalid guest cookie value", () => {
    document.cookie = `${GUEST_LOCALE_COOKIE}=not-a-locale`;
    expect(resolveReservationInitialLanguage(null)).toBe("en");
  });
});
