import { getTranslation } from "@/i18n/getTranslation";

describe("Director Console es-AR chrome (no localeMessageBundles collapse)", () => {
  it("resolves chat.inputPlaceholder with voseo Preguntá (not Peninsular Pregunta)", () => {
    const esAR = getTranslation("directorConsole.chat.inputPlaceholder", "es-AR");
    const es = getTranslation("directorConsole.chat.inputPlaceholder", "es");
    expect(String(esAR)).toContain("Preguntá");
    expect(String(es)).toContain("Pregunta");
    expect(String(esAR)).not.toBe(String(es));
  });
});
