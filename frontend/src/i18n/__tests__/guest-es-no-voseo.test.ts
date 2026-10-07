/**
 * #924 — base guest `es` is tuteo/neutral; voseo lives in `es-AR`.
 */
import es from "../guest-messages/es.json";
import esAR from "../guest-messages/es-AR.json";

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

const WB = "(?<![\\p{L}\\p{N}_])";
const WE = "(?![\\p{L}\\p{N}_])";
const VOSEO_INDICATIVE = new RegExp(
  `${WB}(?:[Pp]odés|[Tt]enés|[Qq]uerés|[Dd]ebés|[Nn]ecesitás|[Vv]enís|[Hh]acés|[Dd]ecís|[Ss]abés|[Pp]referís|[Cc]onocés|[Ee]scribís)${WE}`,
  "u",
);
// Voseo imperatives are the other half of the leak surface: "Revisá abajo"
// instead of "Revisa abajo". The oxytone accent (-á/-é/-í) and the
// unaccented pronominal forms (fijate/acordate/quedate) are what separate
// them from their tuteo counterparts, so they are safe to match by shape.
const VOSEO_IMPERATIVE = new RegExp(
  `${WB}(?:[Rr]evisá|[Pp]robá|[Ee]legí|[Ee]scaneá|[Ii]ngresá|[Mm]irá|[Pp]edí|[Ee]scribí|[Dd]ejá|[Aa]gregá|[Bb]uscá|[Ss]eleccioná|[Cc]onfirmá|[Vv]olvé|[Cc]ompartí|[Gg]uardá|[Vv]erificá|[Cc]ompletá|[Ee]nviá|[Ll]lamá|[Ee]sperá|[Uu]sá|[Pp]oné|[Hh]acé|[Dd]ecí|[Tt]ené|[Ff]ijate|[Aa]cordate|[Qq]uedate)${WE}`,
  "u",
);

const leaksIn = (bundle: unknown, re: RegExp) =>
  Object.entries(flatten(bundle))
    .filter(([, v]) => re.test(v))
    .map(([k, v]) => `${k}: ${v}`);

describe("guest base es has no voseo leftovers (#924)", () => {
  it("quantityCapReached is tuteo in es and voseo in es-AR", () => {
    expect(es.menu.quantityCapReached).toBe(
      "Solo puedes pedir hasta {max} del mismo artículo.",
    );
    expect(es.menu.quantityCapReached).not.toMatch(/podés/);
    expect(esAR.menu.quantityCapReached).toMatch(/podés/);
  });

  it("keeps tuteo noAccount in es", () => {
    expect(es.customerAuth.login.noAccount).toBe("¿No tienes una cuenta?");
  });

  it("has no accented voseo indicative leftovers in base es", () => {
    expect(leaksIn(es, VOSEO_INDICATIVE)).toEqual([]);
  });

  it("has no voseo imperative leftovers in base es", () => {
    expect(leaksIn(es, VOSEO_IMPERATIVE)).toEqual([]);
  });

  it("staffCancelledToast is tuteo in es and stays voseo in es-AR", () => {
    expect(es.orders.staffCancelledToast).toBe(
      "El personal canceló un pedido. Revisa abajo los detalles.",
    );
    expect(esAR.orders.staffCancelledToast).toMatch(/Revisá/);
  });
});
