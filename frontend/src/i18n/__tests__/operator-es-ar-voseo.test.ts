import esMessages from "../messages/es";
import esArOverrides from "../messages/es-ar";
import { deepMerge } from "../deepMerge";

// Locks the operator es-AR voseo quality. The merged es-AR tree (es base +
// Argentine overrides, exactly as SimpleTranslationProvider assembles it) must
// contain NO Peninsular tú-form verbs — every second-person verb the operator
// dashboard shows should be voseo (tenés/podés/querés/configurá/…). A new es
// string with a tú-form that nobody Argentinized will fail here.
describe("operator es-AR has no Peninsular tú-form leaks", () => {
  const merged = deepMerge(
    esMessages as unknown as Record<string, unknown>,
    esArOverrides,
  );

  const flatten = (o: unknown, prefix = "", out: Record<string, string> = {}) => {
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

  // Two Peninsular detectors:
  // (1) present-indicative tú forms as complete words (no adjective collisions);
  // (2) tú affirmative imperatives at a sentence boundary FOLLOWED by a space +
  //     lowercase object — so standalone adjective/status labels ("Activa",
  //     "Completa") and infinitives/nouns ("Configurar"/"Configuración") do NOT
  //     false-positive, but "Activá el sistema" → "Activa el sistema" leaks do.
  //     (Earlier the gate only had present-indicative, so imperative leaks like
  //     "Crea Tu Cuenta" / "Elige Tu Plan" / "Conecta tu cuenta" slipped through.)
  const PENINSULAR_INDICATIVE =
    /\b(?:tienes|puedes|quieres|debes|necesitas|eres|vienes|haces|pones|dices|recibes|sabes|eliges|prefieres|confirmas|guardas|pagas|env[ií]as|conoces)\b/i;
  const PENINSULAR_IMPERATIVE =
    /(?:^|[.!:¡¿]\s+)(?:Configura|Elige|Selecciona|Guarda|Agrega|Añade|Ingresa|Introduce|Rellena|Revisa|Comparte|Escribe|Escanea|Conecta|Sube|Descarga|Copia|Pega|Verifica|Personaliza|Genera|Empieza|Comienza|Establece|Decide|Aprueba|Rechaza|Olvida|Activa|Completa|Pulsa|Crea|Actualiza)\s+[a-záéíóúñ]/;
  const isPeninsular = (s: string) =>
    PENINSULAR_INDICATIVE.test(s) || PENINSULAR_IMPERATIVE.test(s);

  // legal/terms used to be deferred. es-AR now ships Argentine overrides for the
  // informal (tú→vos) legal copy in `legal.json` (the terms + refund sections).
  // Formal usted passages — the privacy policy — intentionally stay usted
  // (Argentine legal register uses usted too) and the tú-detector does not
  // flag them. Nothing is excluded from the gate anymore.
  test("no operator es-AR string uses a Peninsular tú verb form", () => {
    const flat = flatten(merged);
    const leaks = Object.entries(flat)
      .filter(([, v]) => isPeninsular(v))
      .map(([k, v]) => `${k}: ${v}`);
    expect(leaks).toEqual([]);
  });

  test("es-AR genuinely diverges from es across many namespaces", () => {
    // Sanity: the override layer is substantial, not a token gesture.
    const namespacesWithOverrides = Object.values(esArOverrides).filter(
      (ns) => ns && Object.keys(ns).length > 0,
    ).length;
    expect(namespacesWithOverrides).toBeGreaterThan(30);
  });
});
