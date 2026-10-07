import fs from "fs";
import path from "path";

const GUEST_FILES = [
  "../GuestTableView.tsx",
];

describe("guest surfaces: no glassmorphism / AI slop", () => {
  it.each(GUEST_FILES)("no backdrop-blur-* in %s", (rel) => {
    const src = fs.readFileSync(path.resolve(__dirname, rel), "utf8");
    expect(src).not.toMatch(/backdrop-blur-/);
  });

  it.each(GUEST_FILES)("no bg-white/40 in %s", (rel) => {
    const src = fs.readFileSync(path.resolve(__dirname, rel), "utf8");
    expect(src).not.toMatch(/bg-white\/40/);
  });

  it.each(GUEST_FILES)("no hover:-translate-y-2 in %s", (rel) => {
    const src = fs.readFileSync(path.resolve(__dirname, rel), "utf8");
    expect(src).not.toMatch(/hover:-translate-y-2/);
  });

  it.each(GUEST_FILES)("no duration-500 in %s", (rel) => {
    const src = fs.readFileSync(path.resolve(__dirname, rel), "utf8");
    expect(src).not.toMatch(/duration-500/);
  });

  it.each(GUEST_FILES)("no shadow-[0_8px_32px_0_rgba( literals in %s", (rel) => {
    const src = fs.readFileSync(path.resolve(__dirname, rel), "utf8");
    expect(src).not.toMatch(/shadow-\[0_8px_32px_0_rgba/);
  });

  // scale-110 should not appear on active-state category button classes
  // (matches the active-category `scale-110` AI-slop pattern).
  it.each(GUEST_FILES)("no scale-110 on active-state class strings in %s", (rel) => {
    const src = fs.readFileSync(path.resolve(__dirname, rel), "utf8");
    // Match scale-110 that appears in a `className`/template-literal line alongside
    // an `activeCategory`/`isSelected`/active-like identifier or `bg-brand text-white`.
    expect(src).not.toMatch(/activeCategory[^`]*scale-110/);
    expect(src).not.toMatch(/bg-brand\s+text-white[^`]*scale-110/);
  });
});
