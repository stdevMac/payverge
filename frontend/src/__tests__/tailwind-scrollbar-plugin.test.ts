import fs from "fs";
import path from "path";

/**
 * Regression guard for the `tailwind-scrollbar` plugin.
 *
 * Tailwind 3.4 core has no `scrollbar-thin` / `scrollbar-thumb-*` /
 * `scrollbar-track-*` utilities; they come from the plugin. Its only import
 * is the require() in tailwind.config.ts and its utilities live in className
 * strings, so a dead-code pass can mistake it for unused. Removing it once
 * silently dropped the thin warm scrollbar from the dashboard sidebar (whose
 * 23-item nav must visibly scroll), the dispatch board, the reservation board,
 * the mobile schedule, the AI menu wizard and the guest menu category tabs.
 *
 * If no component uses these utilities any more, delete this test together
 * with the plugin.
 */
const FRONTEND_ROOT = path.join(__dirname, "..", "..");
const PLUGIN_CLASS = /\bscrollbar-(?:thin|thumb-[\w-]+|track-[\w-]+)\b/;

function collectSourceFiles(dir: string, out: string[] = []): string[] {
  for (const entry of fs.readdirSync(dir, { withFileTypes: true })) {
    const full = path.join(dir, entry.name);
    if (entry.isDirectory()) {
      if (entry.name === "__tests__" || entry.name === "node_modules") continue;
      collectSourceFiles(full, out);
    } else if (
      /\.(tsx?|jsx?)$/.test(entry.name) &&
      !/\.test\./.test(entry.name)
    ) {
      out.push(full);
    }
  }
  return out;
}

describe("tailwind-scrollbar plugin", () => {
  const usages = ["src", "ds"]
    .map((d) => path.join(FRONTEND_ROOT, d))
    .filter((d) => fs.existsSync(d))
    .flatMap((d) => collectSourceFiles(d))
    .filter((file) => PLUGIN_CLASS.test(fs.readFileSync(file, "utf8")));

  const configSrc = fs
    .readFileSync(path.join(FRONTEND_ROOT, "tailwind.config.ts"), "utf8")
    .split("\n")
    .filter((line) => !line.trim().startsWith("//"))
    .join("\n");

  it("is still needed by production components", () => {
    expect(usages.length).toBeGreaterThan(0);
  });

  it("is registered in tailwind.config.ts", () => {
    expect(configSrc).toMatch(/require\(\s*["']tailwind-scrollbar["']\s*\)/);
  });

  it("is a declared dependency", () => {
    const pkg = JSON.parse(
      fs.readFileSync(path.join(FRONTEND_ROOT, "package.json"), "utf8"),
    );
    const deps = { ...pkg.dependencies, ...pkg.devDependencies };
    expect(deps["tailwind-scrollbar"]).toBeDefined();
  });

  it("generates the scrollbar utilities with the real tailwind config", async () => {
    const postcss = require("postcss");
    const tailwindcss = require("tailwindcss");
    const configModule = require("../../tailwind.config");
    const config = configModule.default ?? configModule;
    const markup = usages
      .map((file) => fs.readFileSync(file, "utf8"))
      .join("\n");
    const result = await postcss([
      tailwindcss({
        ...config,
        content: [{ raw: markup, extension: "tsx" }],
        corePlugins: { ...(config.corePlugins ?? {}), preflight: false },
      }),
    ]).process("@tailwind utilities;", { from: undefined });
    const used = new Set(
      markup.match(new RegExp(PLUGIN_CLASS.source, "g")) ?? [],
    );
    expect(used.size).toBeGreaterThan(0);
    for (const cls of used) {
      expect(result.css).toContain(`.${cls}`);
    }
  });
});
