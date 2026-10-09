/**
 * Guest loyalty redeem/undo failures must not toast form-validation copy.
 *
 * The 11 loyalty codes ship in all 21 guest catalogs. en / es / es-AR stay
 * in the main bundle; the other 18 load on demand via loadApiErrorCatalog.
 * Until that load finishes, translateApiError still falls back to English,
 * which beats "Please check the form".
 */
import {
  translateApiError,
  loadApiErrorCatalog,
  __loadAllApiErrorCatalogsForTests,
} from "../apiErrors";
import { getLocalizedApiError } from "../../utils/apiError";
import enCatalog from "../locales/en/apiErrors.json";
import esCatalog from "../locales/es/apiErrors.json";
import esArCatalog from "../locales/es-ar/apiErrors.json";

/** Codes guest-reachable loyalty / table routes actually emit. */
export const GUEST_LOYALTY_API_ERROR_CODES = [
  "insufficient_points",
  "no_active_bill",
  "no_loyalty_connection",
  "visit_not_recorded",
  "request_id_required",
  "loyalty_discount_already_applied",
  "loyalty_undo_not_owner",
  "loyalty_not_enabled",
  "loyalty_rate_not_configured",
  "loyalty_points_too_small",
  "no_loyalty_discount",
] as const;

const OPERATOR_LOCALES = new Set(["en", "es", "es-AR"]);

const FORM_COPY = /check the form|revis[aá] el formulario|formu/i;

const apiError = (data: unknown, status = 400): unknown => ({
  status,
  response: { status, data },
});

describe("guest loyalty apiErrors catalog", () => {
  let catalogs: Record<string, Record<string, string>>;

  beforeAll(async () => {
    catalogs = await __loadAllApiErrorCatalogsForTests();
  });

  it("lists every guest-reachable loyalty code in en/es/es-AR", () => {
    for (const code of GUEST_LOYALTY_API_ERROR_CODES) {
      expect(enCatalog).toHaveProperty(code);
      expect(esCatalog).toHaveProperty(code);
      expect(esArCatalog).toHaveProperty(code);
      expect(enCatalog[code].trim().length).toBeGreaterThan(0);
      expect(esCatalog[code].trim().length).toBeGreaterThan(0);
      expect(esArCatalog[code].trim().length).toBeGreaterThan(0);
      expect(enCatalog[code]).not.toMatch(FORM_COPY);
      expect(esCatalog[code]).not.toMatch(FORM_COPY);
      expect(esArCatalog[code]).not.toMatch(FORM_COPY);
    }
  });

  it("ships every loyalty code in all 21 guest catalogs", () => {
    expect(Object.keys(catalogs)).toHaveLength(21);
    for (const [locale, catalog] of Object.entries(catalogs)) {
      for (const code of GUEST_LOYALTY_API_ERROR_CODES) {
        const value = catalog[code];
        expect(typeof value).toBe("string");
        expect(value.trim().length).toBeGreaterThan(0);
        expect(value).not.toMatch(FORM_COPY);
        if (!OPERATOR_LOCALES.has(locale)) {
          expect(value).not.toBe(enCatalog[code]);
        }
      }
    }
  });

  it("uses the guest catalog once that locale is loaded", async () => {
    const cases: Array<{
      code: (typeof GUEST_LOYALTY_API_ERROR_CODES)[number];
      english: string;
    }> = [
      {
        code: "insufficient_points",
        english: "insufficient loyalty points: have 40, need 3000",
      },
      {
        code: "no_active_bill",
        english: "No open bill found for this table",
      },
      {
        code: "loyalty_discount_already_applied",
        english:
          "a loyalty discount is already applied to this bill; undo it before redeeming again",
      },
      {
        code: "loyalty_undo_not_owner",
        english: "only the guest who applied this loyalty discount can undo it",
      },
    ];

    await loadApiErrorCatalog("ja");
    await loadApiErrorCatalog("ar");
    await loadApiErrorCatalog("th");

    for (const { code, english } of cases) {
      const ja = translateApiError({ code, error: english }, "ja");
      const ar = translateApiError({ code, error: english }, "ar");
      const th = translateApiError({ code, error: english }, "th");
      expect(ja).toBe(catalogs.ja[code]);
      expect(ar).toBe(catalogs.ar[code]);
      expect(th).toBe(catalogs.th[code]);
      expect(ja).not.toBe(enCatalog[code]);
      expect(ar).not.toBe(enCatalog[code]);
      expect(th).not.toBe(enCatalog[code]);
      expect(ja).not.toBe(english);
      expect(ja).not.toMatch(FORM_COPY);
    }
  });

  it("falls back to the English catalog string before that locale is loaded", () => {
    let freshTranslate: typeof translateApiError = translateApiError;
    jest.isolateModules(() => {
      freshTranslate = require("../apiErrors").translateApiError;
    });
    const ja = freshTranslate(
      {
        code: "insufficient_points",
        error: "insufficient loyalty points: have 40, need 3000",
      },
      "ja",
    );
    expect(ja).toBe(enCatalog.insufficient_points);
    expect(ja).not.toBe("insufficient loyalty points: have 40, need 3000");
    expect(ja).not.toMatch(FORM_COPY);
  });

  it("localizes operator locales instead of the English backend envelope", () => {
    const esAr = translateApiError(
      {
        code: "loyalty_discount_already_applied",
        error:
          "a loyalty discount is already applied to this bill; undo it before redeeming again",
      },
      "es-AR",
    );
    expect(esAr).toBe(esArCatalog.loyalty_discount_already_applied);
    expect(esAr).not.toMatch(FORM_COPY);
    expect(esAr).toMatch(/Anulalo/);

    const es = translateApiError(
      {
        code: "loyalty_undo_not_owner",
        error: "only the guest who applied this loyalty discount can undo it",
      },
      "es",
    );
    expect(es).toBe(esCatalog.loyalty_undo_not_owner);
    expect(es).not.toMatch(/formulario/i);
  });

  it("does not toast form-validation copy for the common already-applied redeem", () => {
    const err = apiError({
      code: "loyalty_discount_already_applied",
      error:
        "a loyalty discount is already applied to this bill; undo it before redeeming again",
    });
    const ja = getLocalizedApiError(err, "ja");
    const en = getLocalizedApiError(err, "en");
    expect(en).toBe(enCatalog.loyalty_discount_already_applied);
    expect(en).not.toBe(enCatalog.VALIDATION_INVALID_INPUT);
    expect(en.toLowerCase()).not.toMatch(/form/);
    expect(ja).toBe(catalogs.ja.loyalty_discount_already_applied);
    expect(ja).not.toBe(en);
    expect(ja).not.toMatch(FORM_COPY);
  });

  it("does not toast form-validation copy for undo-by-non-redeemer", () => {
    const err = apiError(
      {
        code: "loyalty_undo_not_owner",
        error: "only the guest who applied this loyalty discount can undo it",
      },
      403,
    );
    expect(getLocalizedApiError(err, "en")).not.toBe(
      enCatalog.VALIDATION_INVALID_INPUT,
    );
    expect(getLocalizedApiError(err, "es")).not.toMatch(/formulario/i);
    expect(getLocalizedApiError(err, "ja")).toBe(
      catalogs.ja.loyalty_undo_not_owner,
    );
  });
});
