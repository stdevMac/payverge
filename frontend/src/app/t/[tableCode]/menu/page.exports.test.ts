import fs from "fs";
import path from "path";
import ts from "typescript";

const DEFAULT_PAGE_NAME = "GuestMenuPage";

const findInvalidPageExports = (source: string): string[] => {
  const sourceFile = ts.createSourceFile(
    "page.tsx",
    source,
    ts.ScriptTarget.Latest,
    true,
    ts.ScriptKind.TSX,
  );
  const violations: string[] = [];
  let defaultPageExports = 0;

  const describe = (statement: ts.Statement): string => {
    const line =
      sourceFile.getLineAndCharacterOfPosition(statement.getStart(sourceFile))
        .line + 1;
    return `${ts.SyntaxKind[statement.kind]} at line ${line}`;
  };

  for (const statement of sourceFile.statements) {
    if (
      ts.isExportDeclaration(statement) ||
      ts.isExportAssignment(statement) ||
      ts.isNamespaceExportDeclaration(statement)
    ) {
      violations.push(describe(statement));
      continue;
    }

    const modifiers = ts.canHaveModifiers(statement)
      ? ts.getModifiers(statement)
      : undefined;
    const isExported = modifiers?.some(
      (modifier) => modifier.kind === ts.SyntaxKind.ExportKeyword,
    );
    if (!isExported) continue;

    const isDefault = modifiers?.some(
      (modifier) => modifier.kind === ts.SyntaxKind.DefaultKeyword,
    );
    if (
      isDefault &&
      ts.isFunctionDeclaration(statement) &&
      statement.name?.text === DEFAULT_PAGE_NAME
    ) {
      defaultPageExports += 1;
      if (defaultPageExports > 1) violations.push(describe(statement));
      continue;
    }

    violations.push(describe(statement));
  }

  if (defaultPageExports === 0) violations.push("missing default page export");
  return violations;
};

describe("guest menu page route contract", () => {
  it("exports only the default page component", () => {
    const source = fs.readFileSync(path.resolve(__dirname, "page.tsx"), "utf8");

    expect(findInvalidPageExports(source)).toEqual([]);
  });

  it.each([
    ["a star re-export", 'export * from "./helpers";'],
    ["a named re-export", "export { helper };"],
    ["a type-only re-export", "export type { Helper };"],
    ["an ambient declaration", "export declare const helper: string;"],
    ["a CommonJS export assignment", "export = GuestMenuPage;"],
    ["a default export assignment", "export default GuestMenuPage;"],
    ["a namespace export", "export as namespace Payverge;"],
    [
      "a different default declaration",
      "export default function OtherPage() {}",
    ],
    [
      "a duplicate default page declaration",
      "export default function GuestMenuPage() {}",
    ],
  ])("rejects %s", (_name, source) => {
    expect(
      findInvalidPageExports(
        `export default function GuestMenuPage() {}\n${source}`,
      ),
    ).not.toEqual([]);
  });

  it("accepts the exact default page declaration", () => {
    expect(
      findInvalidPageExports("export default function GuestMenuPage() {}"),
    ).toEqual([]);
  });
});
