/** Verify that every advertised live claim maps to an exact, unskipped
 * Playwright test that the browser-suite configuration actually runs. */
import * as fs from "node:fs";
import * as path from "node:path";
import ts from "typescript";

const REPO_ROOT = path.resolve(__dirname, "..", "..");
const REGISTRY = path.join(
  REPO_ROOT,
  "docs",
  "product",
  "advertised-feature-contract.json",
);
const TESTS_DIR = path.join(REPO_ROOT, "frontend", "tests");
const RELEASE_CONFIG = path.join(
  REPO_ROOT,
  "frontend",
  "playwright.config.ts",
);

type FeatureRow = { id: string; status: string; test_ids: string[] };
type DeclaredTest = {
  title: string;
  file: string;
  relativeFile: string;
  unsafe: boolean;
};
export type DiscoveryOptions = {
  testsDir: string;
  releaseConfigPath: string;
  references: string[];
};

function collectSpecFiles(dir: string): string[] {
  const out: string[] = [];
  for (const entry of fs.readdirSync(dir, { withFileTypes: true })) {
    const full = path.join(dir, entry.name);
    if (entry.isDirectory()) out.push(...collectSpecFiles(full));
    else if (entry.name.endsWith(".spec.ts")) out.push(full);
  }
  return out;
}

function stringValue(node: ts.Node | undefined): string | undefined {
  if (
    node &&
    (ts.isStringLiteral(node) || ts.isNoSubstitutionTemplateLiteral(node))
  )
    return node.text;
  return undefined;
}

function callPath(expr: ts.Expression): string {
  if (ts.isIdentifier(expr)) return expr.text;
  if (ts.isPropertyAccessExpression(expr))
    return `${callPath(expr.expression)}.${expr.name.text}`;
  return "";
}

function inspectSpec(file: string, testsDir: string): DeclaredTest[] {
  const source = fs.readFileSync(file, "utf8");
  const sf = ts.createSourceFile(
    file,
    source,
    ts.ScriptTarget.Latest,
    true,
    ts.ScriptKind.TS,
  );
  let unsafe = false;
  const tests: DeclaredTest[] = [];
  const visit = (node: ts.Node): void => {
    if (ts.isCallExpression(node)) {
      const callee = callPath(node.expression);
      if (
        /^test(?:\.describe)?\.(?:skip|fixme)$/.test(callee) ||
        callee === "test.skip" ||
        callee === "test.fixme"
      )
        unsafe = true;
      if (callee === "test") {
        const title = stringValue(node.arguments[0]);
        if (title)
          tests.push({
            title,
            file,
            relativeFile: path
              .relative(testsDir, file)
              .split(path.sep)
              .join("/"),
            unsafe: false,
          });
      }
    }
    ts.forEachChild(node, visit);
  };
  visit(sf);
  return tests.map((test) => ({ ...test, unsafe }));
}

function releasePatterns(configPath: string): string[] {
  const source = fs.readFileSync(configPath, "utf8");
  const sf = ts.createSourceFile(
    configPath,
    source,
    ts.ScriptTarget.Latest,
    true,
    ts.ScriptKind.TS,
  );
  const patterns: string[] = [];
  const visit = (node: ts.Node): void => {
    if (
      ts.isPropertyAssignment(node) &&
      node.name.getText(sf).replace(/["']/g, "") === "testMatch"
    ) {
      if (ts.isArrayLiteralExpression(node.initializer)) {
        for (const value of node.initializer.elements) {
          const pattern = stringValue(value);
          if (pattern) patterns.push(pattern);
        }
      } else {
        const pattern = stringValue(node.initializer);
        if (pattern) patterns.push(pattern);
      }
    }
    ts.forEachChild(node, visit);
  };
  visit(sf);
  return patterns;
}

function globToRegExp(glob: string): RegExp {
  let out = "^";
  for (let i = 0; i < glob.length; i += 1) {
    const ch = glob[i];
    if (ch === "*" && glob[i + 1] === "*") {
      if (glob[i + 2] === "/") {
        out += "(?:.*/)?";
        i += 2;
      } else {
        out += ".*";
        i += 1;
      }
    } else if (ch === "*") out += "[^/]*";
    else out += ch.replace(/[|\\{}()[\]^$+?.]/g, "\\$&");
  }
  return new RegExp(`${out}$`);
}

export function discoverAdvertisedJourneys(options: DiscoveryOptions): {
  errors: string[];
  declarations: DeclaredTest[];
} {
  const declarations = collectSpecFiles(options.testsDir).flatMap((file) =>
    inspectSpec(file, options.testsDir),
  );
  const patterns = releasePatterns(options.releaseConfigPath).map(globToRegExp);
  const errors: string[] = [];
  for (const reference of options.references) {
    const separator = reference.indexOf("::");
    if (separator < 1) {
      errors.push(
        `test_id "${reference}" must be <spec-file>::<exact test title>`,
      );
      continue;
    }
    const basename = reference.slice(0, separator).trim();
    const title = reference.slice(separator + 2).trim();
    const files = [
      ...new Set(
        declarations
          .filter((d) => path.basename(d.file) === basename)
          .map((d) => d.file),
      ),
    ];
    if (files.length === 0) {
      errors.push(`test_id "${reference}" has no spec file`);
      continue;
    }
    if (files.length > 1) {
      errors.push(`test_id "${reference}" has ambiguous spec basename`);
      continue;
    }
    const exact = declarations.find(
      (d) => d.file === files[0] && d.title === title,
    );
    if (!exact) {
      errors.push(`test_id "${reference}" has no exact test declaration`);
      continue;
    }
    if (exact.unsafe)
      errors.push(
        `test_id "${reference}" points to a spec containing test.skip/fixme`,
      );
    if (!patterns.some((pattern) => pattern.test(exact.relativeFile)))
      errors.push(`test_id "${reference}" is not selected by release config`);
  }
  return { errors, declarations };
}

function main(): void {
  const requireLiveCoverage = process.argv.includes("--require-live-coverage");
  const registry = JSON.parse(fs.readFileSync(REGISTRY, "utf8")) as {
    features: FeatureRow[];
  };
  const live = registry.features.filter((row) => row.status === "live");
  const uncovered = live
    .filter((row) => !row.test_ids?.length)
    .map((row) => row.id);
  const referenced = registry.features.flatMap((row) => row.test_ids ?? []);
  const result = discoverAdvertisedJourneys({
    testsDir: TESTS_DIR,
    releaseConfigPath: RELEASE_CONFIG,
    references: referenced,
  });
  if (requireLiveCoverage && uncovered.length)
    result.errors.push(
      `${uncovered.length} live row(s) have no test_ids: ${uncovered.join(", ")}`,
    );
  if (result.errors.length) {
    console.error(
      `advertised-contract-journeys: FAIL\n${result.errors.map((error) => `  - ${error}`).join("\n")}`,
    );
    process.exitCode = 1;
    return;
  }
  console.log(
    `advertised-contract-journeys: PASS — ${referenced.length} exact references; ${live.length - uncovered.length}/${live.length} live rows covered by release-selected, unskipped tests.`,
  );
}

if (require.main === module) main();
