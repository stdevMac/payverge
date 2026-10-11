import { getTranslation } from "../SimpleTranslationProvider";
import esMessages from "../messages/es";
import esArOverrides from "../messages/es-ar";

// es-AR is the `es` base + a Rioplatense override layer (messages/es-ar/*).
// These tests pin the two invariants of that architecture: overridden keys read
// Argentine, and every non-overridden key inherits its es value (never English,
// never a raw key). Assertions are property-based so they survive content reruns.
describe("operator es-AR override layering", () => {
  test("overridden keys read Rioplatense (voseo / local vocab)", () => {
    expect(getTranslation("paymentProcessor.connectWallet", "es-AR")).toBe(
      "Conectá tu Billetera",
    );
    expect(getTranslation("paymentProcessor.chooseHowToConnect", "es-AR")).toBe(
      "Elegí cómo conectar:",
    );
    // voseo in a nested override
    expect(
      getTranslation("paymentProcessor.steps.approval.description", "es-AR"),
    ).toContain("confirmá");
    // "factura" → "cuenta" for the open bill
    const title = getTranslation("billManager.title", "es-AR") as string;
    // NEW-5: bills = Cuentas (restaurant checks); fiscal invoices remain Facturas.
    expect(title).toContain("Cuenta");
  });

  test("neutral es keeps its tú-form (es-AR overrides do not leak into es)", () => {
    expect(getTranslation("paymentProcessor.connectWallet", "es")).toBe(
      "Conecta tu Billetera",
    );
    expect(getTranslation("billManager.title", "es")).toContain("Cuenta");
  });

  test("non-overridden es-AR keys inherit the es value (no English leak)", () => {
    // `paymentProcessor.total` is not in any es-AR override file, so it must
    // fall through to the es base — not English, not a raw key.
    expect(getTranslation("paymentProcessor.total", "es-AR")).toBe(
      getTranslation("paymentProcessor.total", "es"),
    );
    expect(getTranslation("paymentProcessor.total", "es-AR")).toBe("Total");
  });

  test("double-brace placeholders survive the override layer intact", () => {
    expect(getTranslation("paymentProcessor.paymentSuccessMessage", "es-AR")).toContain(
      "{{amount}}",
    );
  });
});

// Property-based guards over the ENTIRE override layer (not hand-picked keys) so
// a future assembler/edit can't silently introduce an orphan key (a key that
// doesn't exist in es → would never be reached at runtime) or drift an
// interpolation placeholder away from its es counterpart.
describe("operator es-AR override layer integrity", () => {
  const flatten = (
    o: unknown,
    prefix = "",
    out: Record<string, string> = {},
  ) => {
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

  const tokens = (s: string): string[] => {
    const m = s.match(/\{\{\s*\w+\s*\}\}|(?<!\{)\{\s*\w+\s*\}(?!\})/g) || [];
    return m.map((t) => t.replace(/\s/g, "")).sort();
  };

  const esFlat = flatten(esMessages);
  const ovFlat = flatten(esArOverrides);

  test("every es-AR override key exists in the es base (no orphan keys)", () => {
    const orphans = Object.keys(ovFlat).filter((k) => !(k in esFlat));
    expect(orphans).toEqual([]);
  });

  test("every es-AR override preserves its es placeholders exactly", () => {
    const drift = Object.entries(ovFlat)
      .filter(([k]) => k in esFlat)
      .filter(([k, v]) => tokens(v).join(",") !== tokens(esFlat[k]).join(","))
      .map(([k, v]) => `${k}: es=[${tokens(esFlat[k])}] es-AR=[${tokens(v)}]`);
    expect(drift).toEqual([]);
  });
});
