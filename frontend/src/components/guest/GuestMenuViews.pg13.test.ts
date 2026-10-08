/**
 * PG-13 — /t vs /b menu-detail asymmetries. Freezes the five residual deltas
 * on the /t GuestMenuViews source (reference implementation = /b PublicMenuDisplay).
 */

import fs from "fs";
import path from "path";

const SRC = path.resolve(__dirname, "GuestMenuViews.tsx");

describe("GuestMenuViews PG-13 /t↔/b parity", () => {
  const src = fs.readFileSync(SRC, "utf8");

  it("#1 desktop title is a named open button (not icon-only-only entry)", () => {
    expect(src).toMatch(/accessibility\.openItem/);
    // Title is a real <button type="button">, not a plain <h3> text node only.
    expect(src).toMatch(
      /<h3[\s\S]{0,120}<button[\s\S]{0,200}accessibility\.openItem/,
    );
  });

  it("#2 onItemClick prop is invoked from handleItemClick (not dead wiring)", () => {
    expect(src).toMatch(/onItemClick\(item\)/);
  });

  it("#3 modal header includes CurrencyPrice for the selected item", () => {
    // Price sits near ModalHeader / selectedItem?.name block.
    const headerIdx = src.indexOf("<ModalHeader");
    const headerSlice = src.slice(headerIdx, headerIdx + 1800);
    expect(headerSlice).toMatch(/CurrencyPrice/);
    expect(headerSlice).toMatch(/selectedItem\.price/);
  });

  it("#4 description uses dir=auto for bidi", () => {
    expect(src).toMatch(/dir=["']auto["'][^>]*>\s*\{selectedItem\.description\}/);
  });

  it("#5 allergen icon alt uses translated label, not raw allergen.name key", () => {
    expect(src).toMatch(/alt=\{allergenLabel\}/);
    expect(src).not.toMatch(/alt=\{allergen\?\.name/);
  });
});
