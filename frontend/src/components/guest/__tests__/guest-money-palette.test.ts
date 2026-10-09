import fs from "fs";
import path from "path";

const read = (rel: string) =>
  fs.readFileSync(path.resolve(__dirname, rel), "utf8");

describe("guest money surfaces use brand tokens, not cheaper green/gray", () => {
  it("GuestBill paid/closed/cashier blocks use emerald/warm/ink, no bg-green-*/bg-gray-*/font-light headings", () => {
    const src = read("../GuestBill.tsx");
    expect(src).not.toMatch(/bg-green-50/);
    expect(src).not.toMatch(/bg-green-100/);
    expect(src).not.toMatch(/text-green-600/);
    // status/cashier headings must not be thin
    expect(src).not.toMatch(/text-xl font-light text-gray-/);
  });
  it("GuestMenuViews add-confirmation uses emerald, not chemical green-500, and headings are not font-light", () => {
    const src = read("../GuestMenuViews.tsx");
    expect(src).not.toMatch(/bg-green-500/);
    expect(src).not.toMatch(/ring-green-500/);
    expect(src).not.toMatch(/text-(?:lg|xl|2xl|3xl)[^"`]*font-light/);
  });
  it("MobileMenuItem add-confirmation uses emerald, no green-500 or stray border-green-200", () => {
    const src = read("../MobileMenuItem.tsx");
    expect(src).not.toMatch(/bg-green-500/);
    expect(src).not.toMatch(/ring-green-500/);
    expect(src).not.toMatch(/border-green-200/);
  });
});
