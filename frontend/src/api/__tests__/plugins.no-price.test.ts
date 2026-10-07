/**
 * Contract: plugin catalog pricing is deleted. Plugins are free and included
 * with every plan — no `price` on Plugin / CreatePluginData and no formatPrice
 * helpers that invite reintroducing a catalog price.
 *
 * @jest-environment node
 */

import * as fs from "node:fs";
import * as path from "node:path";

const apiFile = path.join(__dirname, "..", "plugins.ts");
const pluginsDir = path.join(
  __dirname,
  "..",
  "..",
  "components",
  "business",
  "plugins",
);

describe("plugin pricing deleted", () => {
  it("Plugin / CreatePluginData interfaces have no price member", () => {
    const src = fs.readFileSync(apiFile, "utf8");
    // Match interface field declarations, not unrelated words.
    expect(src).not.toMatch(/^\s*price\s*:/m);
    expect(src).not.toMatch(/formatPrice\s*:/);
    expect(src).not.toMatch(/priceToWei\s*:/);
  });

  it("no plugin config component declares a price prop", () => {
    const files = fs
      .readdirSync(pluginsDir)
      .filter((name) => name.endsWith(".tsx") && !name.includes(".test."));
    const offenders: string[] = [];
    for (const name of files) {
      const src = fs.readFileSync(path.join(pluginsDir, name), "utf8");
      if (/^\s*price\s*:\s*string/m.test(src)) {
        offenders.push(name);
      }
    }
    expect(offenders).toEqual([]);
  });
});
