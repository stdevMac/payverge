/** @jest-environment jsdom */
import fs from "node:fs";
import path from "node:path";

const SOURCE = fs.readFileSync(
  path.resolve(__dirname, "../PublicMenuDisplay.tsx"),
  "utf-8",
);

describe("PublicMenuDisplay tag presentation", () => {
  test("dietary chip rendering does not interpolate tag.emoji", () => {
    expect(SOURCE).not.toMatch(/\{tag\?\.emoji\s*\|\|\s*""\}/);
  });

  test("card-level allergen icon container is visible (no white/10 over white)", () => {
    expect(SOURCE).not.toMatch(/bg-white\/10\s+px-1\.5\s+py-0\.5\s+rounded\s+border\s+border-white\/20/);
  });

  test("card-level allergen icon container uses visible neutral background", () => {
    expect(SOURCE).toMatch(/bg-gray-50\s+border\s+border-gray-200/);
  });

  test("modal allergen section does not paint red-50 envelope", () => {
    expect(SOURCE).not.toMatch(/bg-red-50 border border-red-200 rounded-lg p-4/);
  });

  test("modal allergen chips are not saturated red solids", () => {
    expect(SOURCE).not.toMatch(/className="bg-red-600 text-white"/);
  });

  test("option toggles use single border (not border-2)", () => {
    const optionBlock = SOURCE.match(/p-4 rounded-lg border-2 cursor-pointer/);
    expect(optionBlock).toBeNull();
  });

  test("option toggles use brand-tinted selected state via inline style", () => {
    expect(SOURCE).toMatch(/border-brand|primary_color/);
  });
});
