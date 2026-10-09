/**
 * #387 — Spanish storefront mixed-language add-ons + doubled reservation
 * "optional" suffix. Catalog values must be locale-pure; the form renderer
 * appends reservationForm.optional, so specialRequests must not bake it in.
 */
import fs from "fs";
import path from "path";

const GUEST_DIR = path.join(__dirname, "..", "guest-messages");
const OPTIONS_KEY = "menu.optionsAndAddons";
const REQUESTS_KEY = "businessPage.info.reservationForm.specialRequests";
const OPTIONAL_KEY = "businessPage.info.reservationForm.optional";

type Tree = Record<string, unknown>;

function load(file: string): Tree {
  return JSON.parse(fs.readFileSync(path.join(GUEST_DIR, file), "utf8"));
}

function getNested(obj: unknown, dotted: string): unknown {
  return dotted.split(".").reduce<unknown>((acc, key) => {
    if (acc && typeof acc === "object" && key in (acc as object)) {
      return (acc as Record<string, unknown>)[key];
    }
    return undefined;
  }, obj);
}

function leaf(obj: unknown, dotted: string): string {
  const value = getNested(obj, dotted);
  if (typeof value !== "string") {
    throw new Error(`missing string ${dotted}`);
  }
  return value;
}

const localeFiles = fs
  .readdirSync(GUEST_DIR)
  .filter((f) => f.endsWith(".json") && !f.startsWith("."))
  .sort();

const bundles = Object.fromEntries(
  localeFiles.map((file) => [file.replace(/\.json$/, ""), load(file)]),
);

describe("guest storefront mixed-language + optional-suffix catalog (#387)", () => {
  it("keeps key parity for optionsAndAddons and reservation specialRequests", () => {
    expect(localeFiles.length).toBe(21);
    for (const [, tree] of Object.entries(bundles)) {
      expect(leaf(tree, OPTIONS_KEY).trim().length).toBeGreaterThan(0);
      expect(leaf(tree, REQUESTS_KEY).trim().length).toBeGreaterThan(0);
      expect(leaf(tree, OPTIONAL_KEY).trim().length).toBeGreaterThan(0);
    }
  });

  it.each(["es", "es-AR"] as const)(
    "%s menu.optionsAndAddons is Spanish (no English Add-ons)",
    (code) => {
      const value = leaf(bundles[code], OPTIONS_KEY);
      expect(value).not.toMatch(/add-?ons/i);
      expect(value).not.toBe(leaf(bundles.en, OPTIONS_KEY));
      expect(value).toMatch(/opciones/i);
      expect(value).toMatch(/extras|adicionales|complementos/i);
    },
  );

  it.each(Object.keys(bundles))(
    "%s reservationForm.specialRequests does not bake in optional",
    (code) => {
      const value = leaf(bundles[code], REQUESTS_KEY);
      // Renderer appends `(${optional})`. A trailing parenthetical here
      // becomes "… (opcional) (opcional)" in es / es-AR.
      expect(value).not.toMatch(/(?:\(|（)[^)）]+(?:\)|）)\s*$/u);
      expect(value).not.toMatch(/opcional/i);
    },
  );
});
