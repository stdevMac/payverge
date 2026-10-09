/**
 * next/font/local serves only the latin DM Sans / DM Serif Display files, and
 * its generated size-adjusted fallback face (local Arial/Georgia, no
 * unicode-range) sits right after the primary family. Without a latin-ext face
 * ahead of it, ą ę ś ż ł ş ğ … in the pl/tr/cs/ro/… guest locales render in
 * that fallback inside DM text.
 *
 * Contract: every brand font stack starts with a family whose faces cover
 * only latin-ext (unicode-range never touches U+0000-00FF, so latin text never
 * downloads it) and point at the latin-ext woff2 files, followed by the
 * next/font variable.
 */
import fs from "fs";
import path from "path";

const css = fs.readFileSync(path.join(__dirname, "..", "globals.css"), "utf8");

function fontFaces(): {
  family: string;
  src: string;
  range: string;
  style: string;
}[] {
  const out: { family: string; src: string; range: string; style: string }[] =
    [];
  for (const m of css.matchAll(/@font-face\s*\{([^}]*)\}/g)) {
    const body = m[1];
    const get = (prop: string) =>
      (body.match(new RegExp(`${prop}\\s*:\\s*([^;]+);`)) ?? [])[1]?.trim() ??
      "";
    out.push({
      family: get("font-family").replace(/["']/g, ""),
      src: get("src"),
      range: get("unicode-range"),
      style: get("font-style") || "normal",
    });
  }
  return out;
}

function stack(name: string): string[] {
  const m = css.match(new RegExp(`${name}\\s*:\\s*([^;]+);`));
  if (!m) throw new Error(`${name} not declared in globals.css`);
  return m[1].split(",").map((s) => s.trim().replace(/["']/g, ""));
}

function rangeStarts(range: string): number[] {
  return range
    .split(",")
    .map((r) => parseInt(r.trim().replace(/^U\+/i, "").split("-")[0], 16));
}

describe("brand font stacks reach the latin-ext faces", () => {
  const cases: [string, string, string[]][] = [
    ["--font-inter", "--font-dm-sans", ["normal"]],
    ["--font-poppins", "--font-dm-sans", ["normal"]],
    ["--font-title", "--font-dm-serif-display", ["normal", "italic"]],
  ];

  it.each(cases)(
    "%s leads with a latin-ext-only family before var(%s)",
    (name, variable, styles) => {
      const families = stack(name);
      expect(families[1]).toBe(`var(${variable})`);

      const faces = fontFaces().filter((f) => f.family === families[0]);
      expect(faces.map((f) => f.style).sort()).toEqual([...styles].sort());
      for (const face of faces) {
        expect(face.src).toMatch(/-latin-ext\.woff2/);
        expect(face.range).not.toBe("");
        // No range may start inside basic latin / Latin-1 except the combining
        // marks Google's latin-ext subset shares with latin (U+0304/0308/0329).
        for (const start of rangeStarts(face.range)) {
          expect(start).toBeGreaterThan(0xff);
        }
        expect(face.range).toMatch(/U\+0100-02BA/i);
      }
    },
  );
});
