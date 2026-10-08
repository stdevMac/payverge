/** @jest-environment node */
import * as fs from "fs";
import * as path from "path";

/**
 * Regression-lock: the collapsed w-16 rail hides labels, so two tabs sharing an
 * icon are indistinguishable. Icon assignments live in the tab registry since
 * the registry consolidation — this parses TAB_REGISTRY straight from source
 * and fails if any glyph is used by more than one tab.
 * (audit: Bills+Invoices=Receipt.)
 */
describe("tab registry — unique rail icons", () => {
  it("assigns a distinct lucide glyph to every tab", () => {
    const src = fs.readFileSync(
      path.join(__dirname, "..", "tabs", "tabRegistry.tsx"),
      "utf8",
    );
    // Collect every `icon: <IconName>` assignment from the TAB_REGISTRY table.
    const matcher = /\bicon:\s*([A-Z][A-Za-z0-9]+)\b/g;
    const icons = Array.from(src.matchAll(matcher), (m) => m[1]);

    const dupes = icons.filter((ic, i) => icons.indexOf(ic) !== i);
    expect(dupes).toEqual([]);
  });
});
