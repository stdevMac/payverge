/** @jest-environment node */
import fs from "node:fs";
import path from "node:path";

describe("admin page no longer depends on recharts (P-3)", () => {
  const adminSource = fs.readFileSync(
    path.resolve(__dirname, "../page.tsx"),
    "utf-8",
  );
  const pkg = JSON.parse(
    fs.readFileSync(path.resolve(__dirname, "../../../../package.json"), "utf-8"),
  );

  it("does not import recharts in the admin page", () => {
    expect(adminSource).not.toMatch(/from\s+["']recharts["']/);
  });

  it("removes recharts from package.json dependencies", () => {
    expect(pkg.dependencies?.recharts).toBeUndefined();
    expect(pkg.devDependencies?.recharts).toBeUndefined();
  });
});
