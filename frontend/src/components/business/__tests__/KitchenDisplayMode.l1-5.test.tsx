import fs from "fs";
import path from "path";

/**
 * L1-5 structural contract on KitchenDisplayMode source.
 * (Full RTL of KDS is heavy; assert the shipped seams.)
 */
describe("KitchenDisplayMode L1-5", () => {
  const src = fs.readFileSync(
    path.join(__dirname, "..", "KitchenDisplayMode.tsx"),
    "utf8",
  );

  it("exits browser fullscreen on KDS exit", () => {
    expect(src).toMatch(/document\.exitFullscreen/);
    expect(src).toMatch(/handleExit/);
    expect(src).toMatch(/useDialogKeyboard\(containerRef,\s*handleExit\)/);
  });

  it("uses z-[100] and &:fullscreen sizing (no dead isFullscreen padding ternary)", () => {
    expect(src).toMatch(/z-\[100\]/);
    // Arbitrary variants need the `&` marker: `[:fullscreen]:h-screen` is
    // silently dropped by Tailwind (compiles to nothing); `[&:fullscreen]:`
    // emits `.…:fullscreen { … }`. Verified via `npx tailwindcss` output.
    expect(src).toMatch(/\[&:fullscreen\]:h-screen/);
    expect(src).toMatch(/\[&:fullscreen\]:w-screen/);
    expect(src).not.toMatch(/\[:fullscreen\]/);
    expect(src).not.toMatch(/isFullscreen \? "p-0" : "p-0"/);
  });
});
