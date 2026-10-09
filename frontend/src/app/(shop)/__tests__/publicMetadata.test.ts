/**
 * SEO-3 / SEO-4 — public marketing metadata invariants.
 * @jest-environment node
 */
import { readdirSync, readFileSync, statSync } from "node:fs";
import { join } from "node:path";

const SHOP_ROOT = join(__dirname, "..");

function walk(dir: string): string[] {
  return readdirSync(dir).flatMap((entry) => {
    const full = join(dir, entry);
    if (statSync(full).isDirectory()) {
      if (entry === "__tests__" || entry === "node_modules") return [];
      return walk(full);
    }
    return /\.tsx?$/.test(entry) && !/\.test\.tsx?$/.test(entry) ? [full] : [];
  });
}

/**
 * Extract the balanced `{...}` body of the first `openGraph:` key in a source
 * file. Brace-counting rather than regex because the block nests (images is an
 * array of objects), and a lazy regex stops at the first inner `}`.
 */
function openGraphBlock(source: string): string | null {
  const key = source.indexOf("openGraph:");
  if (key === -1) return null;
  const open = source.indexOf("{", key);
  if (open === -1) return null;

  let depth = 0;
  for (let i = open; i < source.length; i += 1) {
    if (source[i] === "{") depth += 1;
    else if (source[i] === "}") {
      depth -= 1;
      if (depth === 0) return source.slice(open, i + 1);
    }
  }
  return null;
}

describe("SEO-3 · /business/register owns its canonical", () => {
  // MIN-2: layout uses generateMetadata() (locale-aware). Call it directly.
  const mockHeaders = jest.fn();
  beforeEach(() => {
    jest.resetModules();
    jest.doMock("next/headers", () => ({
      headers: () => mockHeaders(),
    }));
    mockHeaders.mockResolvedValue({
      get: (name: string) =>
        name === "x-payverge-locale" ? "en" : null,
    });
  });

  it("declares its own canonical, not the inherited root '/'", async () => {
    const { generateMetadata } = await import("../business/register/layout");
    const metadata = await generateMetadata();

    // #911: publicPageAlternates self-canonicalizes with the absolute URL plus
    // the hreflang cluster, matching home/features/pricing. The guard stays
    // the same — this page must never inherit the root canonical.
    expect(metadata.alternates?.canonical).toBe(
      "https://payverge.io/business/register",
    );
    expect(metadata.alternates?.canonical).not.toBe("https://payverge.io/");
    expect(metadata.alternates?.languages).toMatchObject({
      "es-AR": expect.stringContaining("/business/register"),
    });
  });

  it("points og:url at itself so shared signup links do not preview as home", async () => {
    const { generateMetadata } = await import("../business/register/layout");
    const metadata = await generateMetadata();
    const og = metadata.openGraph as { url?: string | URL; images?: unknown[] };

    expect(String(og?.url)).toBe("https://payverge.io/business/register");
    expect(String(og?.url)).not.toBe("https://payverge.io");
    expect(og?.images?.length).toBeGreaterThan(0);
  });
});

describe("SEO-4 · every openGraph override keeps an image", () => {
  // Next.js does NOT deep-merge `openGraph`: declaring the object in a page or
  // layout replaces the root layout's wholesale. Eight live routes set
  // { title, description, url } and silently dropped the root's images —
  // the whole blog, the whole tools suite, and /features — shipping bare text
  // cards. This pins the shape rather than the eight specific files, so a new
  // page that overrides openGraph fails here instead of in production.
  const offenders = walk(SHOP_ROOT)
    .filter((file) => {
      const src = readFileSync(file, "utf8");
      // REV-5: siteOpenGraphDefaults always includes defaultOpenGraphImages.
      // pageOpenGraphFromCanonical is a thin wrapper over the same helper.
      if (
        src.includes("siteOpenGraphDefaults") ||
        src.includes("pageOpenGraphFromCanonical")
      ) {
        return false;
      }
      const block = openGraphBlock(src);
      return block !== null && !block.includes("images");
    })
    .map((file) => file.slice(SHOP_ROOT.length + 1));

  it("has no page or layout overriding openGraph without images", () => {
    expect(offenders).toEqual([]);
  });
});
