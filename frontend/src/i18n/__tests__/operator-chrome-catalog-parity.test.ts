import fs from "fs";
import path from "path";
import { getTranslation, messages } from "../getTranslation";
import { getChromeTranslation } from "../operatorChromeCatalog";
import {
  OPERATOR_CHROME_PATHS,
  pickOperatorChromeTree,
} from "../operatorChromePaths";

const chromeDir = path.join(__dirname, "..", "operator-chrome");
const LOCALES = ["en", "es", "es-AR"] as const;
const REGEN =
  "operator-chrome/*.json are generated from the full operator catalog. " +
  "Regenerate with: UPDATE_OPERATOR_CHROME=1 npx jest operator-chrome-catalog-parity";

function resolvePath(tree: unknown, dotted: string): unknown {
  return dotted
    .split(".")
    .reduce<unknown>(
      (node, part) =>
        node && typeof node === "object"
          ? (node as Record<string, unknown>)[part]
          : undefined,
      tree,
    );
}

describe("operator chrome slim catalog", () => {
  if (process.env.UPDATE_OPERATOR_CHROME === "1") {
    it("regenerates the slim catalogs", () => {
      for (const locale of LOCALES) {
        const picked = pickOperatorChromeTree(messages[locale]);
        fs.writeFileSync(
          path.join(chromeDir, `${locale}.json`),
          `${JSON.stringify(picked, null, 2)}\n`,
        );
      }
    });
    return;
  }

  it.each(LOCALES)("%s matches the picked full-catalog subtrees", (locale) => {
    const slim = JSON.parse(
      fs.readFileSync(path.join(chromeDir, `${locale}.json`), "utf8"),
    );
    expect({ note: REGEN, slim }).toEqual({
      note: REGEN,
      slim: pickOperatorChromeTree(messages[locale]),
    });
  });

  it("every chrome path resolves in the English catalog", () => {
    const missing = OPERATOR_CHROME_PATHS.filter(
      (p) => resolvePath(messages.en, p) === undefined,
    );
    expect(missing).toEqual([]);
  });

  it("the chrome catalog module never imports the full catalog", () => {
    const source = fs.readFileSync(
      path.join(__dirname, "..", "operatorChromeCatalog.ts"),
      "utf8",
    );
    const imports = [...source.matchAll(/from\s+["']([^"']+)["']/g)].map(
      (m) => m[1],
    );
    expect(
      imports.filter((s) =>
        /getTranslation|\/messages\/|SimpleTranslationProvider/.test(s),
      ),
    ).toEqual([]);
  });

  it("resolves the same strings as the full catalog", () => {
    for (const locale of LOCALES) {
      for (const key of [
        "common.close",
        "common.retry",
        "common.skipToMainContent",
        "authModal.sessionExpired.title",
        "notFound.body",
      ]) {
        expect(getChromeTranslation(key, locale)).toEqual(
          getTranslation(key, locale),
        );
      }
    }
  });
});
