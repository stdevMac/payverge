/**
 * L6-11: operator `es` accounting copy must use tuteo/neutral, not voseo.
 * Voseo forms live in es-ar deep-merge overrides.
 */
import es from "./businessDashboard.json";
import esAr from "../es-ar/businessDashboard.json";

const VOSEO = /(Registrá|ampliá|anulá|volvé|Ingresá|ingresá|Seleccioná|seleccioná|ajustá|quitá|agregá|Agregá|Usá|Elegí|elegí|Emití|Creá)/;

function walk(obj: unknown, path = ""): string[] {
  if (obj && typeof obj === "object" && !Array.isArray(obj)) {
    return Object.entries(obj as Record<string, unknown>).flatMap(([k, v]) =>
      walk(v, path ? `${path}.${k}` : k),
    );
  }
  if (typeof obj === "string" && VOSEO.test(obj)) {
    return [`${path}: ${obj}`];
  }
  return [];
}

describe("L6-11 accountingDashboard es vs es-ar register", () => {
  it("es accountingDashboard has no voseo imperatives", () => {
    const hits = walk((es as { accountingDashboard: unknown }).accountingDashboard, "accountingDashboard");
    expect(hits).toEqual([]);
  });

  it("es-ar keeps at least one voseo override for accounting", () => {
    const ar = (esAr as { accountingDashboard?: unknown }).accountingDashboard;
    expect(ar).toBeTruthy();
    const hits = walk(ar, "accountingDashboard");
    expect(hits.length).toBeGreaterThan(0);
  });

  /**
   * The de-voseoization above rewrote twelve accounting strings from voseo to
   * neutral tuteo in base `es`. Every one of them is a second-person imperative
   * an Argentine operator reads, so every one needs an es-ar override — the
   * first pass shipped only four, silently downgrading the other eight to
   * peninsular register for es-ar users.
   */
  const VOSEO_REQUIRED_KEYS = [
    "accountingDashboard.entries.empty.subtitle",
    "accountingDashboard.entryDetail.immutableHint",
    "accountingDashboard.entryForm.errors.amountRequired",
    "accountingDashboard.entryForm.errors.descriptionRequired",
    "accountingDashboard.entryForm.errors.categoryRequired",
    "accountingDashboard.payrollForm.prefillHint",
    "accountingDashboard.payrollForm.errors.linesRequired",
    "accountingDashboard.payrollForm.errors.staffRequired",
    "accountingDashboard.payrollForm.errors.grossRequired",
    "accountingDashboard.invoices.empty.subtitle",
    "accountingDashboard.issueInvoice.issueByIdHint",
    "accountingDashboard.periodLock.dateRequired",
  ];

  function at(root: unknown, dotted: string): unknown {
    return dotted
      .split(".")
      .reduce<unknown>(
        (acc, k) =>
          acc && typeof acc === "object"
            ? (acc as Record<string, unknown>)[k]
            : undefined,
        root,
      );
  }

  it.each(VOSEO_REQUIRED_KEYS)(
    "es-ar overrides %s with a voseo form",
    (key) => {
      const base = at(es, key);
      expect(typeof base).toBe("string");

      const override = at(esAr, key);
      expect(typeof override).toBe("string");
      expect(VOSEO.test(override as string)).toBe(true);
      // An override that just copies the tuteo string is not a translation.
      expect(override).not.toEqual(base);
    },
  );
});
