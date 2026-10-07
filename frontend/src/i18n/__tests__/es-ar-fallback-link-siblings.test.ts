/**
 * NEW-19: es-ar dialect overrides for *Fallback keys must also override the
 * sibling *FallbackLink so voseo copy never pairs with tuteo link text
 * (e.g. "¿Preferís email? Escríbenos" → must be "Escribinos").
 */
import fs from "node:fs";
import path from "node:path";

const ES_AR_DIR = path.join(__dirname, "../messages/es-ar");

function collectFallbackKeys(
  obj: unknown,
  prefix = "",
): { fallback: string; link: string | null }[] {
  const found: { fallback: string; link: string | null }[] = [];
  if (!obj || typeof obj !== "object" || Array.isArray(obj)) return found;
  for (const [key, value] of Object.entries(obj as Record<string, unknown>)) {
    const pathKey = prefix ? `${prefix}.${key}` : key;
    // NEW-19 only cares about email CTA pairs (demoFallback / ctaFallback),
    // not unrelated *Fallback placeholders (e.g. guestFallback, regionFallback).
    if (
      typeof value === "string" &&
      (key === "demoFallback" || key === "ctaFallback")
    ) {
      const linkKey = `${key}Link`;
      const parent = obj as Record<string, unknown>;
      const linkVal =
        typeof parent[linkKey] === "string" ? (parent[linkKey] as string) : null;
      found.push({ fallback: pathKey, link: linkVal });
    } else if (value && typeof value === "object") {
      found.push(...collectFallbackKeys(value, pathKey));
    }
  }
  return found;
}

describe("es-ar *Fallback sibling *FallbackLink (NEW-19)", () => {
  const files = fs
    .readdirSync(ES_AR_DIR)
    .filter((f) => f.endsWith(".json"));

  it("loads es-ar message JSON files", () => {
    expect(files.length).toBeGreaterThan(0);
  });

  for (const file of files) {
    it(`${file}: every *Fallback override has a *FallbackLink sibling`, () => {
      const raw = JSON.parse(
        fs.readFileSync(path.join(ES_AR_DIR, file), "utf8"),
      );
      const pairs = collectFallbackKeys(raw);
      for (const pair of pairs) {
        expect(pair.link).toEqual(
          expect.any(String),
        );
        expect((pair.link as string).length).toBeGreaterThan(0);
      }
    });
  }
});
