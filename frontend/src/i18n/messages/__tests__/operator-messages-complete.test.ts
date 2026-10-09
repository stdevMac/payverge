import fs from "node:fs";
import path from "node:path";

import { localeRegistry } from "../../generated/locales";

// OP-6: every locale that declares operatorLocale: true in the registry MUST
// ship a complete operator messages tree — frontend/src/i18n/messages/<src>/
// with every namespace en has plus the index.ts barrel. A missing namespace
// silently falls back to English at runtime (the operator dashboard would serve
// untranslated copy without any error), so this guard makes a forgotten
// operator-message wiring fail CI instead.
//
// es-AR is the documented exception: it is a thin Rioplatense OVERRIDE layer
// deep-merged over the es base (see locales/LANGUAGES.md, "es-AR is an OVERRIDE
// LAYER"), so namespaces with zero Argentine divergences are intentionally
// absent. The structural completeness of es-AR's effective tree is already
// pinned by check-operator-locales.js + operator-es-ar-voseo.test.ts; this
// guard only asserts es-AR ships an index.ts barrel.

const messagesRoot = path.resolve(__dirname, "..");

// Disk folder differs from the canonical code for region-qualified locales:
// canonical "es-AR" lives at messages/es-ar on a case-insensitive FS. The
// registry's sourceFolder field is the canonical disk name.
function sourceFolder(code: string): string {
  return localeRegistry[code as keyof typeof localeRegistry].sourceFolder;
}

function namespaceFiles(dir: string): string[] {
  return fs
    .readdirSync(dir)
    .filter((name) => name.endsWith(".json"))
    .sort();
}

const operatorCodes = (
  Object.keys(localeRegistry) as Array<keyof typeof localeRegistry>
).filter((code) => localeRegistry[code].operatorLocale);

const defaultCode = "en";

describe("operator locale message completeness (OP-6)", () => {
  const enDir = path.join(messagesRoot, sourceFolder(defaultCode));
  const enNamespaces = namespaceFiles(enDir);

  it("the default (en) operator tree ships namespaces and an index barrel", () => {
    expect(enNamespaces.length).toBeGreaterThan(0);
    expect(fs.existsSync(path.join(enDir, "index.ts"))).toBe(true);
  });

  it("registry declares en as an operator locale", () => {
    expect(operatorCodes).toContain(defaultCode);
  });

  for (const code of operatorCodes) {
    const folder = sourceFolder(code);
    const dir = path.join(messagesRoot, folder);

    describe(`${code} (messages/${folder})`, () => {
      it("has a messages directory with an index.ts barrel", () => {
        expect(fs.existsSync(dir)).toBe(true);
        expect(fs.existsSync(path.join(dir, "index.ts"))).toBe(true);
      });

      // es-AR is an intentional partial override layer; only full-bundle
      // operator locales must ship every en namespace.
      if (code !== "es-AR") {
        it("ships every namespace the en operator tree ships", () => {
          const present = new Set(namespaceFiles(dir));
          const missing = enNamespaces.filter((ns) => !present.has(ns));
          expect(missing).toEqual([]);
        });
      }
    });
  }
});
