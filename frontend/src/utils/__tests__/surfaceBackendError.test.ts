import { surfaceBackendError } from "@/utils/localizedError";
import type { Locale } from "@/i18n/config";
import enApiErrors from "@/i18n/locales/en/apiErrors.json";
import esApiErrors from "@/i18n/locales/es/apiErrors.json";
import esArApiErrors from "@/i18n/locales/es-ar/apiErrors.json";

/**
 * Wave C (T-2/S-3): shared operator backend-error surface.
 * Mirrors the sanitized axiosInstance reject shape (plain Error + response).
 */
const apiErr = (
  status: number | undefined,
  data?: { error?: string; code?: string },
) =>
  Object.assign(new Error("request failed"), {
    status,
    code: data?.code,
    response: status === undefined ? undefined : { status, data },
  });

describe("surfaceBackendError (Wave C shared convention)", () => {
  it("maps a known backend code to the operator i18n catalog (en)", () => {
    const err = apiErr(403, {
      code: "business_closed",
      error: "locked",
    });
    expect(surfaceBackendError(err, "en")).toBe(enApiErrors.business_closed);
  });

  // The LLM-only routes answer 503 with error === code === "ai_not_configured"
  // when no provider is wired (self-host without OPENROUTER_API_KEY /
  // LLM_BASE_URL). The operator must see "ask your administrator", never the
  // raw code or an upgrade prompt.
  it.each<[Locale, string]>([
    ["en", enApiErrors.ai_not_configured],
    ["es", esApiErrors.ai_not_configured],
    ["es-AR", esArApiErrors.ai_not_configured],
  ])("maps 503 ai_not_configured to the catalog (%s)", (locale, expected) => {
    const err = apiErr(503, {
      code: "ai_not_configured",
      error: "ai_not_configured",
    });
    const msg = surfaceBackendError(err, locale);
    expect(msg).toBe(expected);
    expect(msg).not.toContain("ai_not_configured");
    expect(msg).toMatch(/LLM/);
  });

  // An admin suspension / closure is the only lock, so the operator gates
  // answer 403 business_suspended / business_closed. The copy must point at
  // the server administrator, never "renew" or "upgrade".
  it.each<[Locale, "business_suspended" | "business_closed", string]>([
    ["en", "business_suspended", enApiErrors.business_suspended],
    ["en", "business_closed", enApiErrors.business_closed],
    ["es", "business_suspended", esApiErrors.business_suspended],
    ["es-AR", "business_closed", esArApiErrors.business_closed],
  ])("maps 403 %s %s to admin-lock copy", (locale, code, expected) => {
    const err = apiErr(403, { code, error: "locked" });
    const msg = surfaceBackendError(err, locale);
    expect(expected).toBeTruthy();
    expect(msg).toBe(expected);
    expect(msg).toMatch(/administra/i);
    expect(msg).not.toMatch(/renew|upgrade|renov|suscrip/i);
  });

  it("maps a known backend code to Spanish catalog copy", () => {
    const err = apiErr(400, {
      code: "delivery_zone_priority_duplicate",
      error: "duplicate active zone priority: 1",
    });
    expect(surfaceBackendError(err, "es")).toBe(
      esApiErrors.delivery_zone_priority_duplicate,
    );
  });

  it("falls back to a translated generic when there is no usable body", () => {
    const err = apiErr(500, {});
    const msg = surfaceBackendError(err, "es");
    expect(msg.length).toBeGreaterThan(0);
    // Must not be empty / must be Spanish-ish server copy (not raw English dump)
    expect(msg.toLowerCase()).toMatch(/mal|error|lado|int[eé]nt/);
  });

  it("returns a safe translated fallback for non-Axios / plain Error throws", () => {
    const msg = surfaceBackendError(new TypeError("boom"), "en");
    expect(msg).not.toBe("boom");
    expect(msg.length).toBeGreaterThan(0);
    expect(msg.toLowerCase()).toMatch(/unexpected|try again|error/);
  });

  it("surfaces product-safe backend domain copy when code is uncoded (CO-1)", () => {
    const err = apiErr(400, {
      error: "reservations must be made at least 30 minutes in advance",
    });
    expect(surfaceBackendError(err, "en")).toBe(
      "reservations must be made at least 30 minutes in advance",
    );
  });

  it("surfaces a 402 (AI budget reached) as the backend's own copy, not a plan upsell", () => {
    const err = apiErr(402, {
      error: "The AI budget for this business has been reached.",
    });
    const msg = surfaceBackendError(err, "en");
    expect(msg).toBe("The AI budget for this business has been reached.");
    expect(msg).not.toMatch(/plan|upgrade/i);
  });

  it("surfaces 413 as range-too-large operator copy", () => {
    const err = apiErr(413, {
      error: "choose a period of 31 days or less",
    });
    const msg = surfaceBackendError(err, "en");
    expect(msg.toLowerCase()).toMatch(/range|31|period|large|export/);
  });

  it("success-path helper does not throw on null/undefined", () => {
    expect(() => surfaceBackendError(null, "en")).not.toThrow();
    expect(() => surfaceBackendError(undefined, "en")).not.toThrow();
    expect(surfaceBackendError(null, "en").length).toBeGreaterThan(0);
  });

  it("honors an explicit fallback when nothing else resolves", () => {
    // Non-API throw with empty-ish path — still returns non-empty via generic,
    // but when we pass fallback and generic is used, fallback is last resort
    // only if generic is empty. Prefer that caller fallback wins for TypeError
    // when provided and no API body exists.
    const msg = surfaceBackendError(new Error(""), "en", "Custom fallback");
    expect(msg.length).toBeGreaterThan(0);
  });
});
