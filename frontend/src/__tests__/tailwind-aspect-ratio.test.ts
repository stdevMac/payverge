import fs from "fs";
import path from "path";

/**
 * Regression guard for the menu item-detail modal image (and every other
 * `aspect-square` / `aspect-[…]` surface).
 *
 * The deprecated `@tailwindcss/aspect-ratio` plugin DISABLES Tailwind's
 * native aspect-ratio utilities. With it installed, `aspect-square` resolves
 * to `aspect-ratio: auto`, collapsing any element that relies on it to
 * height:0 — which silently hid the guest menu item modal's image carousel
 * (it uses `w-full aspect-square`). The fix was removing the plugin so the
 * native utilities (Tailwind 3.0+) work. Don't let it creep back.
 */
describe("tailwind config", () => {
  const configSrc = fs.readFileSync(
    path.join(__dirname, "..", "..", "tailwind.config.ts"),
    "utf8",
  );

  // Strip line comments so the explanatory NOTE in the config doesn't trip us.
  const activeSrc = configSrc
    .split("\n")
    .filter((line) => !line.trim().startsWith("//"))
    .join("\n");

  it("does not use the deprecated @tailwindcss/aspect-ratio plugin", () => {
    expect(activeSrc).not.toMatch(/@tailwindcss\/aspect-ratio/);
  });

  it("does not list @tailwindcss/aspect-ratio as a dependency", () => {
    const pkg = JSON.parse(
      fs.readFileSync(path.join(__dirname, "..", "..", "package.json"), "utf8"),
    );
    const deps = { ...pkg.dependencies, ...pkg.devDependencies };
    expect(deps["@tailwindcss/aspect-ratio"]).toBeUndefined();
  });
});
