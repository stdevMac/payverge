/**
 * CI net for the operator tier: every literal getTranslation("a.b.c") key used
 * in non-test source must resolve to a string (or string[]) in the English
 * operator tree. A missing key renders a sentence-cased leaf in the UI and is
 * only visible through telemetry, so catch it here instead.
 */
import fs from "fs";
import path from "path";
import { messages } from "../getTranslation";

const SRC_ROOT = path.resolve(__dirname, "../..");
const LITERAL_CALL = /getTranslation\(\s*["']([A-Za-z0-9_.-]+)["']/g;

function walk(dir: string, out: string[]): string[] {
  for (const entry of fs.readdirSync(dir, { withFileTypes: true })) {
    if (entry.name === "node_modules" || entry.name.startsWith(".")) continue;
    const full = path.join(dir, entry.name);
    if (entry.isDirectory()) {
      if (entry.name === "__tests__") continue;
      walk(full, out);
    } else if (
      /\.(ts|tsx)$/.test(entry.name) &&
      !/\.(test|spec)\.(ts|tsx)$/.test(entry.name)
    ) {
      out.push(full);
    }
  }
  return out;
}

function resolve(tree: Record<string, unknown>, key: string): unknown {
  return key.split(".").reduce<unknown>((node, part) => {
    if (node && typeof node === "object") {
      return (node as Record<string, unknown>)[part];
    }
    return undefined;
  }, tree);
}

function collectLiteralOperatorKeys(): Map<string, string[]> {
  const byKey = new Map<string, string[]>();
  for (const file of walk(SRC_ROOT, [])) {
    const source = fs.readFileSync(file, "utf8");
    for (const match of source.matchAll(LITERAL_CALL)) {
      const key = match[1];
      const files = byKey.get(key) ?? [];
      files.push(path.relative(SRC_ROOT, file));
      byKey.set(key, files);
    }
  }
  return byKey;
}

describe("operator getTranslation() key existence", () => {
  it("finds literal call sites to check", () => {
    expect(collectLiteralOperatorKeys().size).toBeGreaterThan(0);
  });

  it("resolves every literal getTranslation key in the English tree", () => {
    const missing: string[] = [];
    for (const [key, files] of collectLiteralOperatorKeys()) {
      const value = resolve(messages.en, key);
      // Strings and arrays (bullet lists) are leaves; a bare object means the
      // call site points at a namespace, and undefined means a missing key.
      const ok = typeof value === "string" || Array.isArray(value);
      if (!ok) missing.push(`${key} (${files.join(", ")})`);
    }
    expect(missing).toEqual([]);
  });

  it("carries common.required in en, es and es-AR (TrustpilotConfig gate)", () => {
    expect(resolve(messages.en, "common.required")).toBe("Required");
    expect(typeof resolve(messages.es, "common.required")).toBe("string");
    expect(typeof resolve(messages["es-AR"], "common.required")).toBe("string");
  });
});
