import fs from "fs";
import path from "path";

const src = fs.readFileSync(path.resolve(__dirname, "../page.tsx"), "utf8");

describe("alternative-payments page: no emoji icons or animated gradient blobs", () => {
  it("uses lucide icons, not raw emoji glyphs, for payment methods", () => {
    expect(src).not.toMatch(/💵|💳|📱|🔄/u);
  });
  it("imports the lucide method icons", () => {
    expect(src).toMatch(/Banknote/);
    expect(src).toMatch(/Smartphone/);
    expect(src).toMatch(/Repeat/);
  });
  it("does not render motion-safe:animate-pulse blur blobs", () => {
    expect(src).not.toMatch(/motion-safe:animate-pulse/);
    expect(src).not.toMatch(/blur-3xl/);
  });
});
