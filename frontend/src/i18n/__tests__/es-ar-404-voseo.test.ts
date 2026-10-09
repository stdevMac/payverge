import { getTranslation } from "../getTranslation";
import esGuest from "../guest-messages/es.json";
import esArGuest from "../guest-messages/es-AR.json";

/**
 * #25 — es-AR 404 copy must be voseo.
 * Peninsular `es` keeps tuteo (buscas / Cancela).
 */
describe("es-AR 404 voseo (#25)", () => {
  it("operator 404 body is buscás in es-AR and buscas in es", () => {
    expect(getTranslation("notFound.body", "es-AR")).toBe(
      "La página que buscás no existe o se ha movido.",
    );
    expect(getTranslation("notFound.body", "es")).toBe(
      "La página que buscas no existe o se ha movido.",
    );
    expect(getTranslation("notFound.body", "es-AR")).not.toMatch(/\bbuscas\b/);
  });

  it("guest storefront 404 description is voseo only in es-AR", () => {
    expect(esArGuest.errors.notFoundDescription).toBe(
      "La página que buscás no existe",
    );
    expect(esGuest.errors.notFoundDescription).toBe(
      "La página que buscas no existe",
    );
  });
});
