import esMessages from "../es";
import esArOverrides from "../es-ar";
import enMessages from "../en";
import { deepMerge } from "../../deepMerge";

// Counter service copy contract.
//
// R2-9: CounterToggle blocks a blank prefix with `tString("prefixRequired")`.
// The key exists in neither en nor es, so the operator sees the sentence-cased
// leaf fallback ("Prefix required") plus a missing-translation report instead
// of the real instruction.
//
// R2-8: base `es` is the neutral tuteo tier — Argentine voseo belongs in the
// es-AR override layer only (see ES_AR_OVERRIDE_LAYER.md). The counter
// validation strings shipped "Ingresá …" into base es.
type Tree = Record<string, any>;

const en = enMessages as unknown as Tree;
const es = esMessages as unknown as Tree;
const esAr = deepMerge(es, esArOverrides as unknown as Tree) as Tree;

function counterManager(tree: Tree): Tree {
  return tree.businessDashboard.dashboard.counterManager;
}

describe("counter service operator copy", () => {
  describe("honesty contract (dinner-service readiness)", () => {
    for (const [name, tree] of [
      ["en", en],
      ["es", es],
    ] as Array<[string, Tree]>) {
      it(`${name} ships an honesty banner that does not pitch a live queue`, () => {
        const honesty = counterManager(tree).honesty;
        expect(typeof honesty?.title).toBe("string");
        expect(typeof honesty?.body).toBe("string");
        expect(String(honesty.title).trim().length).toBeGreaterThan(0);
        expect(String(honesty.body).trim().length).toBeGreaterThan(0);
        const blob = `${honesty.title} ${honesty.body}`.toLowerCase();
        // Must not claim a live ops surface.
        expect(blob).not.toMatch(
          /pickup rail|counter-ready|cola de retiro en vivo lista/,
        );
        // Must admit incompleteness for dinner/takeaway ops.
        expect(
          /setup only|solo configuración|not ready|no listo|no live|no hay cola|not a live/i.test(
            blob,
          ),
        ).toBe(true);
      });

      it(`${name} sidebar/page labels stay consistent as Counters / Mostradores (#193)`, () => {
        const tabs = tree.businessDashboard.tabs;
        const cm = counterManager(tree);
        const title = cm.title;
        expect(tabs.counter).toBe(title);
        // Operator-facing destination name must match everywhere (#193).
        expect(cm.activeCounters?.title).toBe(title);
        expect(cm.header?.counters).toBe(title);
        expect(cm.header?.eyebrow).toBe(title);
        const shortcut =
          tree.businessDashboard.overview?.roleSpecific?.server
            ?.counterShortcut;
        if (typeof shortcut === "string") {
          expect(shortcut).toBe(title);
        }
        const actionTitle =
          tree.businessDashboard.overview?.roleSpecificActions?.server
            ?.counterService?.title;
        if (typeof actionTitle === "string") {
          expect(actionTitle).toBe(title);
        }
        const desc = String(tabs.counterDesc).toLowerCase();
        expect(desc).toMatch(/setup|configuración|naming|nombres/);
        expect(desc).toMatch(
          /not a live|no es una cola|setup only|solo configuración/,
        );
      });

      it(`${name} ships apply-prefix copy for default-name mismatches (#192)`, () => {
        const active = counterManager(tree).activeCounters;
        for (const key of [
          "applyPrefix",
          "applyingPrefix",
          "applyPrefixHint",
          "customNamesKept",
          "willBecome",
          "customKeptBadge",
        ]) {
          expect(typeof active?.[key]).toBe("string");
          expect(String(active[key]).trim().length).toBeGreaterThan(0);
        }
        expect(typeof counterManager(tree).success?.prefixApplied).toBe(
          "string",
        );
      });

      it(`${name} ships archived-counter copy for leftover inactive rows (#393)`, () => {
        const archived = counterManager(tree).archivedCounters;
        for (const key of ["title", "subtitle", "show", "hide"]) {
          expect(typeof archived?.[key]).toBe("string");
          expect(String(archived[key]).trim().length).toBeGreaterThan(0);
        }
        expect(String(archived.show)).toContain("{count}");
      });
    }
  });

  describe("toggle prefix guard (R2-9)", () => {
    for (const [name, tree] of [
      ["en", en],
      ["es", es],
    ] as Array<[string, Tree]>) {
      it(`${name} ships toggle.prefixRequired`, () => {
        const value = counterManager(tree).toggle?.prefixRequired;
        expect(typeof value).toBe("string");
        expect(String(value).trim().length).toBeGreaterThan(0);
      });
    }

    it("es differs from en (it is really translated)", () => {
      expect(counterManager(es).toggle.prefixRequired).not.toBe(
        counterManager(en).toggle.prefixRequired,
      );
    });
  });

  describe("validation register (R2-8)", () => {
    // `\b` never fires after an accented vowel (JS \w is ASCII-only), so the
    // voseo form is matched without a trailing boundary.
    const STARTS_WITH_ENTER_VERB = /Ingres[aá]/;
    const isVoseo = (s: string) => /Ingresá/.test(s);
    const isTuteo = (s: string) => /\bIngresa\b/.test(s);

    const validationKeys = ["counterCountInvalid", "counterPrefixInvalid"];

    for (const key of validationKeys) {
      it(`base es keeps ${key} in tuteo`, () => {
        const line = String(counterManager(es).settings[key] ?? "");
        expect(line).toMatch(STARTS_WITH_ENTER_VERB); // sanity: verb still there
        expect(isVoseo(line)).toBe(false);
        expect(isTuteo(line)).toBe(true);
      });

      it(`es-AR overrides ${key} with voseo`, () => {
        const line = String(counterManager(esAr).settings[key] ?? "");
        expect(isVoseo(line)).toBe(true);
      });
    }
  });
});
