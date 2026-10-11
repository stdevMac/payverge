import fs from "node:fs";
import os from "node:os";
import path from "node:path";

import {
  generateGo,
  generateTypeScript,
  generatedFilePaths,
  runGenerate,
} from "../generate";
import { readRegistry, repoRootFromFrontend } from "../common";

const realRepoRoot = repoRootFromFrontend();

function writeRepoFromRealRegistry(): {
  root: string;
  tsPath: string;
  goPath: string;
} {
  const realRegistry = readRegistry(realRepoRoot);
  const root = fs.mkdtempSync(path.join(os.tmpdir(), "i18n-generate-"));
  fs.mkdirSync(path.join(root, "locales"), { recursive: true });
  fs.writeFileSync(
    path.join(root, "locales", "registry.json"),
    fs.readFileSync(path.join(realRepoRoot, "locales", "registry.json")),
  );

  const { frontendGeneratedPath, backendGeneratedPath } =
    generatedFilePaths(root);

  // Seed both generated files with the in-sync output so --check passes.
  fs.mkdirSync(path.dirname(frontendGeneratedPath), { recursive: true });
  fs.mkdirSync(path.dirname(backendGeneratedPath), { recursive: true });
  fs.writeFileSync(frontendGeneratedPath, generateTypeScript(realRegistry));
  fs.writeFileSync(backendGeneratedPath, generateGo(realRegistry));

  return { root, tsPath: frontendGeneratedPath, goPath: backendGeneratedPath };
}

describe("i18n generate", () => {
  test("the committed generated files are in sync with registry.json (current repo)", () => {
    const registry = readRegistry(realRepoRoot);
    const { frontendGeneratedPath, backendGeneratedPath } =
      generatedFilePaths(realRepoRoot);

    expect(fs.readFileSync(frontendGeneratedPath, "utf8")).toBe(
      generateTypeScript(registry),
    );
    expect(fs.readFileSync(backendGeneratedPath, "utf8")).toBe(
      generateGo(registry),
    );
  });

  test("runGenerate --check exits 0 (returns 0) when generated files are in sync", () => {
    const { root } = writeRepoFromRealRegistry();
    try {
      const exitCode = runGenerate({ repoRoot: root, check: true });
      expect(exitCode).toBe(0);
    } finally {
      fs.rmSync(root, { recursive: true, force: true });
    }
  });

  test("runGenerate --check exits non-zero when the frontend generated file is stale", () => {
    const { root, tsPath } = writeRepoFromRealRegistry();
    try {
      fs.writeFileSync(tsPath, "// stale drift\n");
      const exitCode = runGenerate({ repoRoot: root, check: true });
      expect(exitCode).not.toBe(0);
    } finally {
      fs.rmSync(root, { recursive: true, force: true });
    }
  });

  test("runGenerate --check exits non-zero when the backend generated file is stale", () => {
    const { root, goPath } = writeRepoFromRealRegistry();
    try {
      fs.writeFileSync(goPath, "// stale drift\n");
      const exitCode = runGenerate({ repoRoot: root, check: true });
      expect(exitCode).not.toBe(0);
    } finally {
      fs.rmSync(root, { recursive: true, force: true });
    }
  });

  test("runGenerate --check does NOT rewrite the on-disk files when they are stale", () => {
    const { root, tsPath } = writeRepoFromRealRegistry();
    try {
      fs.writeFileSync(tsPath, "// stale drift\n");
      runGenerate({ repoRoot: root, check: true });
      // check mode must never write — the stale content stays untouched.
      expect(fs.readFileSync(tsPath, "utf8")).toBe("// stale drift\n");
    } finally {
      fs.rmSync(root, { recursive: true, force: true });
    }
  });

  test("runGenerate in write mode regenerates stale files and returns 0", () => {
    const { root, tsPath, goPath } = writeRepoFromRealRegistry();
    try {
      fs.writeFileSync(tsPath, "// stale drift\n");
      fs.writeFileSync(goPath, "// stale drift\n");

      const exitCode = runGenerate({ repoRoot: root, check: false });
      expect(exitCode).toBe(0);

      const registry = readRegistry(root);
      expect(fs.readFileSync(tsPath, "utf8")).toBe(generateTypeScript(registry));
      expect(fs.readFileSync(goPath, "utf8")).toBe(generateGo(registry));

      // A subsequent --check now passes.
      expect(runGenerate({ repoRoot: root, check: true })).toBe(0);
    } finally {
      fs.rmSync(root, { recursive: true, force: true });
    }
  });
});
