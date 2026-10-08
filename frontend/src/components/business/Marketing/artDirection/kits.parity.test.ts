import { existsSync, readFileSync } from "fs";
import { join } from "path";

import { KIT_ORDER } from "./kits";

/**
 * Mirrors backend/internal/server/marketing_kits_parity_test.go from the
 * other direction. That Go test can report a stale PASS on ANY run without
 * `-count=1` — including CI's own `go test -race ./...`, because
 * actions/setup-go caches GOCACHE on a go.sum-derived key and this frontend
 * file is outside the Go module (see the caching caveat documented at the top
 * of TestMarketingKitsWhitelistParity). Drift almost always originates here —
 * someone extends KIT_ORDER — and Jest has no equivalent stale-cache problem,
 * so this test is the guard that actually gates CI.
 */
const goHandlerPath = join(
  __dirname,
  "..",
  "..",
  "..",
  "..",
  "..",
  "..",
  "backend",
  "internal",
  "server",
  "marketing_activity_handlers.go",
);

const backendPresent = existsSync(goHandlerPath);

if (!backendPresent) {
  // Jest has no t.Skip-with-reason; this is the readable stand-in surfaced
  // next to the skipped test below.
  console.warn(
    `backend checkout not present at ${goHandlerPath}; skipping kit whitelist parity check`,
  );
}

// Removes `//` line comments and block comments while leaving double-quoted
// string literals intact. Without it a parked line inside the literal —
// `// TODO: add "neon" next wave` — scrapes as a real id and reports a
// confidently wrong "out of sync" drift. Mirrors stripSourceComments in
// backend/internal/server/marketing_kits_parity_test.go.
function stripSourceComments(source: string): string {
  let out = "";
  let inString = false;

  for (let index = 0; index < source.length; index += 1) {
    const char = source[index];

    if (inString) {
      if (char === "\\" && index + 1 < source.length) {
        out += char + source[index + 1];
        index += 1;
        continue;
      }
      if (char === '"') inString = false;
      out += char;
      continue;
    }

    if (char === '"') {
      inString = true;
      out += char;
      continue;
    }

    if (char === "/" && source[index + 1] === "/") {
      while (index < source.length && source[index] !== "\n") index += 1;
      // Keep the newline so line structure (and any following code) survives.
      if (index < source.length) out += "\n";
      continue;
    }

    if (char === "/" && source[index + 1] === "*") {
      const end = source.indexOf("*/", index + 2);
      // Unterminated block comment: everything after it is commented out.
      if (end < 0) return out;
      index = end + 1;
      continue;
    }

    out += char;
  }

  return out;
}

/**
 * Extracts the kit ids from the Go `marketingKits` slice declaration. Throws —
 * never returns an empty-but-successful list — for any shape it cannot read,
 * so a backend refactor surfaces as a loud failure rather than a vacuously
 * passing parity check.
 */
function parseBackendKits(source: string): string[] {
  // Match only the marketingKits slice declaration, not any other reference
  // to the identifier (e.g. the oneOf(kit, marketingKits...) call site), so an
  // unrelated edit elsewhere in the file can't accidentally satisfy (or
  // spuriously fail) this check.
  const sliceMatch = source.match(/var marketingKits = \[\]string\{([^}]*)\}/);
  if (!sliceMatch) {
    throw new Error(
      "could not find `var marketingKits = []string{...}` in " +
        `${goHandlerPath}; this test's regex needs updating to match the current file shape`,
    );
  }

  const idMatches = [
    ...stripSourceComments(sliceMatch[1]).matchAll(/"([^"]+)"/g),
  ];
  if (idMatches.length === 0) {
    throw new Error(
      `parsed zero kit ids out of marketingKits in ${goHandlerPath}; ` +
        "this test's regex likely no longer matches the file",
    );
  }

  return idMatches.map((m) => m[1]);
}

describe("kits parity with backend marketingKits", () => {
  // Skip (rather than silently pass) when the backend isn't checked out, so
  // the backend stays checkable standalone without this suite lying about
  // coverage it didn't actually run.
  const maybeIt = backendPresent ? it : it.skip;

  maybeIt(
    "matches Go's marketingKits whitelist exactly, including order",
    () => {
      const backendKits = parseBackendKits(readFileSync(goHandlerPath, "utf8"));

      if (JSON.stringify(backendKits) !== JSON.stringify(KIT_ORDER)) {
        throw new Error(
          "frontend/src/components/business/Marketing/artDirection/kits.ts KIT_ORDER and " +
            "backend/internal/server/marketing_activity_handlers.go marketingKits are out of sync " +
            "(order matters). Update both files together.\n" +
            `frontend KIT_ORDER:    ${JSON.stringify(KIT_ORDER)}\n` +
            `backend marketingKits: ${JSON.stringify(backendKits)}`,
        );
      }
    },
  );

  it("ignores ids parked on a commented-out line", () => {
    const source = [
      "var marketingKits = []string{",
      '\t"editorial",',
      '\t// TODO: add "neon" next wave',
      '\t"bold",',
      '\t"minimal", // keep last for now',
      "}",
    ].join("\n");

    // A commented-out id must not scrape as a real one: the resulting "out of
    // sync" report fails loudly but would send whoever hits it looking for a
    // frontend change that was never missing.
    expect(parseBackendKits(source)).toEqual(["editorial", "bold", "minimal"]);
  });

  // Pins the shapes this guard must keep rejecting or reading correctly.
  // Every one is a plausible backend refactor; none may silently pass.
  describe("never silently passes on a refactored backend declaration", () => {
    it.each([
      ["var (...) block", 'var (\n\tmarketingKits = []string{"editorial"}\n)'],
      ["spaces removed", 'var marketingKits=[]string{"editorial"}'],
      ["ids hoisted to constants", "var marketingKits = []string{editorialKit}"],
      ["single-quoted runes", "var marketingKits = []string{'editorial'}"],
      [
        "every id commented out",
        'var marketingKits = []string{\n\t// "editorial",\n}',
      ],
    ])("throws on %s", (_name, source) => {
      expect(() => parseBackendKits(source)).toThrow();
    });

    it("still reads a gofmt-wrapped multi-line declaration", () => {
      const source =
        'var marketingKits = []string{\n\t"editorial",\n\t"bold",\n}';
      expect(parseBackendKits(source)).toEqual(["editorial", "bold"]);
    });
  });
});
