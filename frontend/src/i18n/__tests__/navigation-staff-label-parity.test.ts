/**
 * #811 — The es-AR header staff link read "Personal" while es (and the es-AR
 * footer, which falls back to es) read "Acceso Personal". The clipped label
 * looked like an overflow bug, not a deliberate shortening. The staff link
 * label must match the es wording; the voseo divergence stays where it is
 * intentional (signIn "Ingresar").
 */
import fs from "fs";
import path from "path";

function readNavigation(localeDir: string): Record<string, string> {
  return JSON.parse(
    fs.readFileSync(
      path.join(
        process.cwd(),
        `src/i18n/messages/${localeDir}/navigation.json`,
      ),
      "utf8",
    ),
  );
}

describe("es-AR header staff label parity (#811)", () => {
  it("uses the same staff link label as es", () => {
    const es = readNavigation("es");
    const esAr = readNavigation("es-ar");
    expect(es.staffLogin).toBe("Acceso Personal");
    expect(esAr.staffLogin).toBe(es.staffLogin);
  });

  it("keeps the short voseo sign-in label so the header rail still fits", () => {
    const esAr = readNavigation("es-ar");
    expect(esAr.signIn).toBe("Ingresar");
  });
});
