import fs from "fs";
import path from "path";
import { getTranslation } from "@/i18n/getTranslation";

// Regression guard for the production homepage crash where the landing page
// (and other marketing pages) showed the "Shop error" boundary.
//
// Root cause: server `generateMetadata()` functions called `getTranslation`,
// which was only exported from the `"use client"` SimpleTranslationProvider
// module. In React Server Components, every export of a "use client" module
// becomes a client reference when imported by server code, so *calling* it
// throws: "Attempted to call getTranslation() from the server but
// getTranslation is on the client." That throw surfaced through the metadata
// outlet and tripped the (shop) error boundary, replacing the whole page.
//
// The invariant: any SERVER module (no "use client" directive) that imports
// `getTranslation` must import it from a server-safe module — never from a
// "use client" module. Jest cannot reproduce the RSC boundary itself (it
// imports modules directly), so we assert the structural invariant that the
// boundary enforces at runtime.

const SRC_DIR = path.join(__dirname, "..", "..");
const APP_DIR = path.join(SRC_DIR, "app");

function isUseClient(file: string): boolean {
  const head = fs.readFileSync(file, "utf8").trimStart();
  return head.startsWith('"use client"') || head.startsWith("'use client'");
}

function walk(dir: string): string[] {
  const out: string[] = [];
  for (const entry of fs.readdirSync(dir, { withFileTypes: true })) {
    const full = path.join(dir, entry.name);
    if (entry.isDirectory()) {
      out.push(...walk(full));
    } else if (/\.(ts|tsx)$/.test(entry.name)) {
      out.push(full);
    }
  }
  return out;
}

// Resolve an "@/..." import specifier to an absolute source file path.
function resolveAliasImport(spec: string): string | null {
  if (!spec.startsWith("@/")) return null;
  const base = path.join(SRC_DIR, spec.slice(2));
  for (const ext of [".ts", ".tsx", ".js", ".jsx"]) {
    if (fs.existsSync(base + ext)) return base + ext;
  }
  for (const ext of [".ts", ".tsx"]) {
    const idx = path.join(base, "index" + ext);
    if (fs.existsSync(idx)) return idx;
  }
  return null;
}

const GET_TRANSLATION_IMPORT =
  /import\s*(?:type\s*)?\{[^}]*\bgetTranslation\b[^}]*\}\s*from\s*["']([^"']+)["']/;

describe("server-safe getTranslation", () => {
  it("is never imported into a server module from a \"use client\" module", () => {
    const files = walk(APP_DIR).filter((f) => !f.includes("__tests__"));
    const offenders: string[] = [];

    for (const file of files) {
      if (isUseClient(file)) continue; // client files may use the client provider
      const src = fs.readFileSync(file, "utf8");
      const match = src.match(GET_TRANSLATION_IMPORT);
      if (!match) continue;
      const target = resolveAliasImport(match[1]);
      if (target && isUseClient(target)) {
        offenders.push(
          `${path.relative(SRC_DIR, file)} imports getTranslation from "${match[1]}" which is a "use client" module`,
        );
      }
    }

    expect(offenders).toEqual([]);
  });

  it("exposes a server-safe module that resolves operator metadata keys", () => {
    const title = getTranslation("landing.metaTitle", "en");
    expect(typeof title).toBe("string");
    expect((title as string).length).toBeGreaterThan(0);
  });

  it("the getTranslation module itself carries no \"use client\" directive", () => {
    const target = resolveAliasImport("@/i18n/getTranslation");
    expect(target).not.toBeNull();
    expect(isUseClient(target as string)).toBe(false);
  });
});
