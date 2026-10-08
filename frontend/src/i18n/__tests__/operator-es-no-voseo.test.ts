/**
 * Root A / L3-34 — inverse register guard.
 *
 * Policy (Session H, 59f98f4f8): base operator `es` is tuteo/neutral;
 * Argentine voseo lives only in the `es-ar` deep-merge override layer.
 *
 * Sibling `operator-es-ar-voseo.test.ts` asserts the merged es-AR tree has no
 * Peninsular tú forms. This file is the inverse: base `es` must contain no
 * voseo. Without it, Round-2 sessions kept shipping new base-es voseo
 * (e.g. deliverySettings "Ingresá…") while L3-34 sat open.
 *
 * Detectors deliberately prefer high-confidence voseo markers that do not
 * collide with first-person preterite ("abrí" in "Cuentas que abrí") or
 * adjective labels ("Activa", "Completa").
 */
import esMessages from "../messages/es";

const flatten = (
  o: unknown,
  prefix = "",
  out: Record<string, string> = {},
): Record<string, string> => {
  if (Array.isArray(o)) {
    o.forEach((v, i) => flatten(v, `${prefix}.${i}`, out));
  } else if (o && typeof o === "object") {
    for (const [k, v] of Object.entries(o as Record<string, unknown>)) {
      flatten(v, prefix ? `${prefix}.${k}` : k, out);
    }
  } else if (typeof o === "string") {
    out[prefix] = o;
  }
  return out;
};

/**
 * JS `\b` is ASCII-only: an accented final (á/é/í) is non-word, so
 * `/\bIngresá\b/` never matches "Ingresá un…". Use Unicode letter boundaries.
 */
const WB = "(?<![\\p{L}\\p{N}_])";
const WE = "(?![\\p{L}\\p{N}_])";

/**
 * Present-indicative voseo — ONLY accented finals (podés/tenés/querés/…).
 * Case-folded via character classes so "Podés" still hits.
 */
const VOSEO_INDICATIVE = new RegExp(
  `${WB}(?:[Pp]odés|[Tt]enés|[Qq]uerés|[Dd]ebés|[Nn]ecesitás|[Vv]enís|[Hh]acés|[Dd]ecís|[Ss]abés|[Pp]referís|[Cc]onocés|[Ee]scribís)${WE}`,
  "u",
);

/**
 * Accented voseo imperatives (Ingresá, Confirmá, Usá, Elegí, …). Peninsular
 * tú imperatives drop the final accent (Ingresa, Confirma, Usa, Elige).
 * Do NOT character-class the unaccented form — that collides with tuteo.
 */
const VOSEO_IMPERATIVE = new RegExp(
  `${WB}(?:Ingresá|ingresá|Confirmá|confirmá|Seleccioná|seleccioná|Usá|usá|Elegí|elegí|Registrá|registrá|Configurá|configurá|Activá|activá|Desactivá|desactivá|Guardá|guardá|Cancelá|cancelá|Revisá|revisá|Enviá|enviá|Publicá|publicá|Volvé|volvé|Creá|creá|Agregá|agregá|Quitá|quitá|Ajustá|ajustá|Anulá|anulá|Emití|emití|Amplíá|ampliá|Indicá|indicá|Mirá|mirá|Escribí|escribí|Actualizá|actualizá|Verificá|verificá|Descargá|descargá|Dejá|dejá|Probá|probá|Intentá|intentá|Reintentá|reintentá|Asegurate|asegurate|Aseguráte|aseguráte)${WE}`,
  "u",
);

/**
 * Voseo imperative + clitic without peninsular accent placement
 * (Intentalo vs Inténtalo; mandalo vs mándalo; escribinos vs escríbenos).
 * ASCII finals — plain `\b` is fine.
 */
const VOSEO_CLITIC =
  /\b(?:Intentalo|intentalo|Mandalo|mandalo|Escribinos|escribinos|Revisalo|revisalo|Configuralo|configuralo)\b/;

/**
 * Imperative "Abrí …" / "abrí …" at a sentence/clause start followed by a
 * determiner or noun — distinguishes voseo imperative from first-person
 * preterite ("Cuentas que abrí").
 */
const VOSEO_ABRI_IMPERATIVE =
  /(?:^|[.!:¡¿—]\s+|,\s+)[Aa]brí\s+(?:el|la|los|las|un|una|tu|su|este|esta|Cuenta|panel|archivo)/;

const isVoseo = (s: string): boolean =>
  VOSEO_INDICATIVE.test(s) ||
  VOSEO_IMPERATIVE.test(s) ||
  VOSEO_CLITIC.test(s) ||
  VOSEO_ABRI_IMPERATIVE.test(s);

describe("operator base es has no voseo (Root A / L3-34 inverse guard)", () => {
  test("no base-es operator string uses a voseo verb form", () => {
    const flat = flatten(esMessages);
    const leaks = Object.entries(flat)
      .filter(([, v]) => isVoseo(v))
      .map(([k, v]) => `${k}: ${v}`);
    expect(leaks).toEqual([]);
  });
});
