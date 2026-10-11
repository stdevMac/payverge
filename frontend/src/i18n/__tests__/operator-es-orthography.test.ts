/**
 * Root B / L1-27 — Spanish orthography blocklist over operator es + es-ar.
 *
 * High-frequency unaccented forms this repo actually produces. Not a full
 * spellchecker: it does not catch novel misspellings, correctly unaccented
 * words that share a stem with an accented form outside this list, or the
 * demonstrative "esta" vs verb "está" except for a few fixed phrases.
 */
import esMessages from "../messages/es";
import esArOverrides from "../messages/es-ar";

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

/** Unaccented lemma → required accented form (case-sensitive variants listed). */
const UNACCENTED_BLOCKLIST: Array<{ re: RegExp; label: string }> = [
  { re: /\bconexion\b/i, label: "conexion→conexión" },
  { re: /\bconfiguracion\b/i, label: "configuracion→configuración" },
  { re: /\bemision\b/i, label: "emision→emisión" },
  { re: /\bconfirmacion\b/i, label: "confirmacion→confirmación" },
  { re: /\bnotificacion\b/i, label: "notificacion→notificación" },
  { re: /\bUltima\b/, label: "Ultima→Última" },
  { re: /\bUltimo\b/, label: "Ultimo→Último" },
  { re: /\bvencio\b/i, label: "vencio→venció" },
  // Past-tense "falló" miswritten as "fallo." — does NOT match noun "fallo"
  // in "con fallo" / "Factura con fallo".
  { re: /\bfallo\./i, label: "fallo.→falló." },
  { re: /\bEnvia\b/, label: "Envia→Envía" },
  // Fixed phrase: copula "está" without accent.
  { re: /\besta conectado\b/i, label: "esta conectado→está conectado" },
  // L1-27 residual: high-frequency inventory / payments lemmas that slipped
  // the first enumerated pass (deposito / reposicion).
  { re: /\bdeposito\b/i, label: "deposito→depósito" },
  { re: /\breposicion\b/i, label: "reposicion→reposición" },
];

function orthographyLeaks(
  flat: Record<string, string>,
): string[] {
  const leaks: string[] = [];
  for (const [k, v] of Object.entries(flat)) {
    for (const { re, label } of UNACCENTED_BLOCKLIST) {
      if (re.test(v)) {
        leaks.push(`${k} [${label}]: ${v}`);
      }
    }
  }
  return leaks;
}

describe("operator Spanish orthography blocklist (Root B / L1-27)", () => {
  test("base es has no high-frequency unaccented forms from the blocklist", () => {
    const leaks = orthographyLeaks(flatten(esMessages));
    expect(leaks).toEqual([]);
  });

  test("es-ar overrides have no high-frequency unaccented forms from the blocklist", () => {
    const leaks = orthographyLeaks(flatten(esArOverrides));
    expect(leaks).toEqual([]);
  });
});
