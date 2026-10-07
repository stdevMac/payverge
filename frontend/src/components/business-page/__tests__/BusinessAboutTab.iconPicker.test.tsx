/** @jest-environment jsdom */
/**
 * Word-boundary regression test for feature icon resolution.
 *
 * Icon keys live in featureIcons.tsx (shared with the Special Features editor).
 * The previous implementation used `.includes()` substring matching, which
 * silently picked the wrong icon for titles like "Delivery available" — the
 * substring "live" inside "deLIVEry" matched the `live` key.
 */
import fs from "node:fs";
import path from "node:path";
import { resolveFeatureIconKey } from "@/components/business-page/featureIcons";

const SOURCE = fs.readFileSync(
  path.resolve(__dirname, "../featureIcons.tsx"),
  "utf-8",
);
const ABOUT = fs.readFileSync(
  path.resolve(__dirname, "../BusinessAboutTab.tsx"),
  "utf-8",
);

describe("BusinessAboutTab — feature icon picker", () => {
  test("uses pickFeatureIcon from featureIcons (stored key + title fallback)", () => {
    expect(ABOUT).toMatch(/pickFeatureIcon/);
    expect(ABOUT).toMatch(/from\s+["']\.\/featureIcons["']/);
  });

  test("has an explicit delivery → Truck mapping", () => {
    expect(SOURCE).toMatch(/delivery:\s*Truck/);
  });

  test("matches feature words via tokenizer, not naive .includes()", () => {
    expect(SOURCE).toMatch(/tokenizeFeatureTitle/);
    expect(SOURCE).toMatch(/words\.has\(key\)/);
    expect(SOURCE).not.toMatch(/lower\.includes\(key\)/);
  });

  test("loyalty / rewards / family / local features have their own icons", () => {
    expect(SOURCE).toMatch(/loyalty:\s*["']award["']/);
    expect(SOURCE).toMatch(/rewards:\s*["']award["']/);
    expect(SOURCE).toMatch(/family:\s*["']users["']/);
    expect(SOURCE).toMatch(/local:\s*["']salad["']/);
  });

  test("does not confuse delivery with live music", () => {
    expect(resolveFeatureIconKey(undefined, "Delivery available")).toBe(
      "delivery",
    );
    expect(resolveFeatureIconKey(undefined, "Live music nightly")).toBe(
      "music",
    );
  });
});
