import { readdirSync, readFileSync, statSync } from "node:fs";
import path from "node:path";

/**
 * Hex → relative luminance per WCAG 2.x.
 */
function luminance(hex: string): number {
  const v = hex.replace("#", "");
  const r = parseInt(v.slice(0, 2), 16) / 255;
  const g = parseInt(v.slice(2, 4), 16) / 255;
  const b = parseInt(v.slice(4, 6), 16) / 255;
  const norm = (c: number) =>
    c <= 0.03928 ? c / 12.92 : Math.pow((c + 0.055) / 1.055, 2.4);
  return 0.2126 * norm(r) + 0.7152 * norm(g) + 0.0722 * norm(b);
}

function ratio(a: string, b: string): number {
  const la = luminance(a);
  const lb = luminance(b);
  const [hi, lo] = la > lb ? [la, lb] : [lb, la];
  return (hi + 0.05) / (lo + 0.05);
}

const SRC_ROOT = path.resolve(__dirname, "../..");
const TEXT_PATTERN = /(?<![:\w])text-(warm|ink|gray)-(50|100|200|300)\b/g;

/**
 * Files where light text on a dark background is intentional — add to this
 * list rather than changing the class. Paths relative to frontend/.
 */
const ALLOW_FILES = new Set<string>([
  // Pre-seeded dark-section components
  "src/components/Footer.tsx",
  // dark:text-gray-200/300 dark-mode variants in AI onboarding wizard
  "src/components/business/MenuBuilder/AIMenuOnboarding/AIWizard.tsx",
  "src/components/business/MenuBuilder/AIMenuOnboarding/PDFDigitizer.tsx",
  "src/components/business/MenuBuilder/AIMenuOnboarding/index.tsx",
  // text-gray-300 on bg-black camera permission overlay
  "src/components/qr/QRCodeScanner.tsx",
  // text-ink-300 subtitle on the bg-ink-900 AI-waiter chat header (D-3 re-skin, 10.6:1 AA)
  "src/components/guest/AiWaiter.tsx",
]);

function* walk(dir: string): Generator<string> {
  for (const name of readdirSync(dir)) {
    if (name === "node_modules" || name === ".next" || name.startsWith(".")) continue;
    const full = path.join(dir, name);
    const stat = statSync(full);
    if (stat.isDirectory()) {
      yield* walk(full);
    } else if (/\.(tsx?|jsx?)$/.test(name)) {
      yield full;
    }
  }
}

describe("contrast hygiene", () => {
  it("no text-*-{50,100,200,300} class used as text color outside the allow-list", () => {
    const frontendRoot = path.resolve(__dirname, "../../..");
    const violations: { file: string; line: number; class: string }[] = [];

    for (const file of walk(path.join(frontendRoot, "src"))) {
      const rel = path.relative(frontendRoot, file);
      if (ALLOW_FILES.has(rel)) continue;
      if (rel.includes("__tests__")) continue;

      const content = readFileSync(file, "utf8");
      const lines = content.split("\n");
      for (let i = 0; i < lines.length; i++) {
        const matches = lines[i].matchAll(TEXT_PATTERN);
        for (const m of matches) {
          violations.push({ file: rel, line: i + 1, class: m[0] });
        }
      }
    }

    if (violations.length) {
      const head = violations.slice(0, 30)
        .map(v => `  ${v.file}:${v.line}  ${v.class}`)
        .join("\n");
      const tail = violations.length > 30 ? `\n  …and ${violations.length - 30} more` : "";
      throw new Error(
        `text-*-{50,100,200,300} misuse on cream-background surface — ${violations.length} occurrences:\n${head}${tail}`
      );
    }
  });

  it("palette shades 500+ clear WCAG AA on cream (#faf9f6)", () => {
    const palette: Record<string, string> = {
      "500": "#6b6358",
      "600": "#544d44",
      "700": "#403b34",
      "800": "#2e2a25",
      "900": "#1c1917",
      "950": "#1c1917",
    };
    for (const [shade, hex] of Object.entries(palette)) {
      const r = ratio(hex, "#faf9f6");
      expect(r).toBeGreaterThanOrEqual(4.5);
    }
  });
});
