/**
 * Diner routes must not statically reach the operator-only heavyweights:
 *   - the full operator i18n catalog (src/i18n/getTranslation.ts and
 *     src/i18n/messages/**, ~1.9 MB of en/es/es-AR source), and
 *   - wagmi / viem / @wagmi (the operator wallet stack).
 *
 * This walks the static import graph from the root layout and the guest
 * route entries. Dynamic `import()` (next/dynamic) is a split point and is
 * not followed; `import type` is erased and is not followed either.
 */
import fs from "fs";
import path from "path";

const SRC = path.join(__dirname, "..");
const EXTENSIONS = [".ts", ".tsx", ".js", ".jsx", ".json"];

const ENTRIES = [
  "app/layout.tsx",
  "app/providers.tsx",
  "app/t/[tableCode]/layout.tsx",
  "app/t/[tableCode]/page.tsx",
  "app/t/[tableCode]/menu/layout.tsx",
  "app/t/[tableCode]/menu/page.tsx",
  "app/t/[tableCode]/bill/layout.tsx",
  "app/t/[tableCode]/bill/page.tsx",
  "app/b/[customUrl]/page.tsx",
  "app/delivery/[deliveryNumber]/track/page.tsx",
  "app/delivery/[deliveryNumber]/pay/page.tsx",
  "app/reservations/[confirmationCode]/page.tsx",
  "app/scan/page.tsx",
];

// Static specifiers only: a `from` clause or a bare `import "x"`. `import(`
// never matches (no whitespace after `import`), and the clause cannot cross a
// quote or `;`, so it never runs into a later statement.
const STATIC_IMPORT =
  /^\s*(import|export)\s+(type\s+)?(?:[^;'"]*?\s+from\s+)?["']([^"']+)["']/gm;

function stripComments(source: string): string {
  return source
    .replace(/\/\*[\s\S]*?\*\//g, "")
    .replace(/(^|[^:"'`])\/\/.*$/gm, "$1");
}

function resolveFile(base: string): string | null {
  if (fs.existsSync(base) && fs.statSync(base).isFile()) return base;
  for (const ext of EXTENSIONS) {
    if (fs.existsSync(base + ext)) return base + ext;
  }
  for (const ext of EXTENSIONS) {
    const index = path.join(base, `index${ext}`);
    if (fs.existsSync(index)) return index;
  }
  return null;
}

function resolveSpecifier(from: string, spec: string): string | null {
  if (spec.startsWith("@/")) return resolveFile(path.join(SRC, spec.slice(2)));
  if (spec.startsWith(".")) {
    return resolveFile(path.resolve(path.dirname(from), spec));
  }
  return null;
}

type Graph = {
  parents: Map<string, string | null>;
  packages: Map<string, string>;
};

function walk(entries: string[]): Graph {
  const parents = new Map<string, string | null>();
  const packages = new Map<string, string>();
  const queue: string[] = [];
  for (const entry of entries) {
    const abs = path.join(SRC, entry);
    if (!fs.existsSync(abs)) throw new Error(`missing entry ${entry}`);
    parents.set(abs, null);
    queue.push(abs);
  }
  while (queue.length) {
    const file = queue.shift() as string;
    if (file.endsWith(".json")) continue;
    const source = stripComments(fs.readFileSync(file, "utf8"));
    for (const match of source.matchAll(STATIC_IMPORT)) {
      if (match[2]) continue; // import type / export type
      const spec = match[3];
      const resolved = resolveSpecifier(file, spec);
      if (!resolved) {
        if (!spec.startsWith(".") && !spec.startsWith("@/")) {
          if (!packages.has(spec)) packages.set(spec, file);
        }
        continue;
      }
      if (!parents.has(resolved)) {
        parents.set(resolved, file);
        queue.push(resolved);
      }
    }
  }
  return { parents, packages };
}

function chain(graph: Graph, file: string): string {
  const hops: string[] = [];
  let cur: string | null | undefined = file;
  while (cur) {
    hops.push(path.relative(SRC, cur));
    cur = graph.parents.get(cur);
  }
  return hops.reverse().join("\n  -> ");
}

describe("guest route import graph", () => {
  const graph = walk(ENTRIES);

  it("walks a non-trivial graph", () => {
    expect(graph.parents.size).toBeGreaterThan(100);
  });

  it("never reaches the full operator i18n catalog", () => {
    const offenders = [...graph.parents.keys()].filter((file) => {
      const rel = path.relative(SRC, file);
      return rel === "i18n/getTranslation.ts" || rel.startsWith("i18n/messages/");
    });
    expect(offenders.slice(0, 5).map((file) => chain(graph, file))).toEqual([]);
  });

  it("never reaches the operator wallet stack (wagmi / viem)", () => {
    const offenders = [...graph.packages.entries()]
      .filter(([spec]) => /^(wagmi|@wagmi\/|viem)(\/|$)/.test(spec))
      .map(([spec, file]) => `${spec} <- ${chain(graph, file)}`);
    expect(offenders).toEqual([]);
  });

  it("never reaches the wagmi provider or bridge component", () => {
    const offenders = [...graph.parents.keys()].filter((file) => {
      const rel = path.relative(SRC, file);
      return (
        rel === "providers/DynamicProvider.tsx" ||
        rel === "providers/WalletBridge.tsx" ||
        rel === "context/index.tsx"
      );
    });
    expect(offenders.map((file) => chain(graph, file))).toEqual([]);
  });
});
