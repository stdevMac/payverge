// Provenance contract for the guest-menu allergen icons:
// node --test scripts/licenses/allergen-icons.test.mjs
//
// The icons are hand-written SVG in this repository (docs/licensing/CREDITS.md).
// They replaced Adobe Illustrator exports of unknown origin; this guard keeps a
// third-party export or an embedded raster from slipping back in.
import assert from "node:assert/strict";
import { readdirSync, readFileSync } from "node:fs";
import path from "node:path";
import { test } from "node:test";
import { fileURLToPath } from "node:url";

const ROOT = path.resolve(path.dirname(fileURLToPath(import.meta.url)), "../..");
const DIR = path.join(ROOT, "frontend/public/images/allergens");
const TAGS = readFileSync(path.join(ROOT, "frontend/src/constants/menu-tags.ts"), "utf8");

test("every allergen in menu-tags.ts has an icon and every icon is referenced", () => {
  const referenced = [...TAGS.matchAll(/"\/images\/allergens\/([a-z0-9]+\.svg)"/g)].map((m) => m[1]).sort();
  const files = readdirSync(DIR).sort();
  assert.equal(referenced.length, 14);
  assert.deepEqual(files, referenced);
});

test("allergen icons are project-authored SVG, not editor exports", () => {
  for (const file of readdirSync(DIR)) {
    const svg = readFileSync(path.join(DIR, file), "utf8");
    assert.doesNotMatch(svg, /Adobe|Illustrator|Layer_1|inkscape|sodipodi|Sketch|Figma/i, `${file} carries an editor export marker`);
    assert.doesNotMatch(svg, /<image\b|data:image\//, `${file} embeds a raster image`);
    assert.match(svg, /^<svg xmlns="http:\/\/www\.w3\.org\/2000\/svg"[^>]* viewBox="0 0 100 100"/, `${file} root element`);
    assert.match(svg, /Drawn in this repository; Apache-2\.0\./, `${file} provenance comment`);
    assert.ok(svg.length < 4096, `${file} should stay a small hand-written SVG`);
  }
});
