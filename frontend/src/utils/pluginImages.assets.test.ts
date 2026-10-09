import fs from "fs";
import path from "path";
import { KNOWN_PLUGIN_NAMES } from "@/constants/plugins";
import { resolvePluginImageSrc } from "./pluginImages";

/**
 * Backend ConvertToDBPlugin always emits Image:
 *   /images/plugins/{GetName()}-logo.png
 * FE resolvePluginImageSrc falls back to curated paths. Both must exist so
 * catalog and config UI never 404 icons.
 */
describe("plugin image assets", () => {
  const publicDir = path.join(process.cwd(), "public");

  it("ships a file for every FE resolvePluginImageSrc fallback", () => {
    for (const name of KNOWN_PLUGIN_NAMES) {
      const src = resolvePluginImageSrc({ name });
      expect(src).toBeTruthy();
      const disk = path.join(publicDir, src!.replace(/^\//, ""));
      expect(fs.existsSync(disk)).toBe(true);
    }
  });

  it("ships backend-style {name}-logo.png for every known plugin name", () => {
    for (const name of KNOWN_PLUGIN_NAMES) {
      const disk = path.join(
        publicDir,
        "images",
        "plugins",
        `${name}-logo.png`,
      );
      expect(fs.existsSync(disk)).toBe(true);
    }
  });
});
