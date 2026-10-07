/**
 * Fixture-driven unit tests for the advertised-feature contract validator.
 * Proves each rule (1–8) fails on a bad fixture and passes on a good fixture.
 */

import * as fs from "node:fs";
import * as os from "node:os";
import * as path from "node:path";
import {
  REQUIRED_CLAIM_KEYS,
  type FeatureRegistry,
  type FeatureRow,
  resolveJsonPointer,
  validateAdvertisedFeatureContract,
} from "../check-advertised-feature-contract";

const REPO_ROOT = path.resolve(__dirname, "..", "..", "..");
const REAL_REGISTRY = path.join(
  REPO_ROOT,
  "docs",
  "product",
  "advertised-feature-contract.json",
);
const REAL_SCHEMA = path.join(
  REPO_ROOT,
  "docs",
  "product",
  "advertised-feature-contract.schema.json",
);
const REAL_MESSAGES = path.join(
  REPO_ROOT,
  "frontend",
  "src",
  "i18n",
  "messages",
  "en",
);

function baseRow(overrides: Partial<FeatureRow> = {}): FeatureRow {
  return {
    id: "sample-feature",
    title: "Sample feature",
    keys: ["businessSettings.json#businessPage.googleReviewsDescription"],
    status: "live",
    audience: "public-marketing",
    markets: ["global"],
    prerequisites: [],
    evidence: [
      {
        path: "frontend/src/app/layout.tsx",
        note: "root layout exists",
      },
    ],
    test_ids: [],
    last_verified: { sha: "b88c4ddb6", date: "2026-07-11" },
    owner: "platform",
    ...overrides,
  };
}

function registryOf(...rows: FeatureRow[]): FeatureRegistry {
  return { version: 1, features: rows };
}

/** Cover every REQUIRED_CLAIM_KEYS entry so rule 7 is satisfied in good fixtures. */
function coverRequiredKeys(extraKeys: string[] = []): string[] {
  return Array.from(new Set([...REQUIRED_CLAIM_KEYS, ...extraKeys]));
}

function goodRegistry(rowOverrides: Partial<FeatureRow> = {}): FeatureRegistry {
  const { keys: overrideKeys, ...rest } = rowOverrides;
  return registryOf(
    baseRow({
      ...rest,
      // Ensure curated claim keys stay covered even when overrides add keys
      keys: coverRequiredKeys(Array.isArray(overrideKeys) ? overrideKeys : []),
    }),
  );
}

function hasRule(
  result: ReturnType<typeof validateAdvertisedFeatureContract>,
  rule: number | "schema",
): boolean {
  return result.errors.some((e) => e.rule === rule);
}

function validateFixture(
  registry: FeatureRegistry,
  opts: {
    strictTests?: boolean;
    pathExists?: (p: string) => boolean;
    playwrightCorpus?: string;
    requiredClaimKeys?: readonly string[];
    messagesDir?: string;
  } = {},
) {
  return validateAdvertisedFeatureContract({
    registry,
    schema: JSON.parse(fs.readFileSync(REAL_SCHEMA, "utf8")),
    repoRoot: REPO_ROOT,
    messagesDir: opts.messagesDir ?? REAL_MESSAGES,
    strictTests: opts.strictTests,
    pathExists:
      opts.pathExists ?? ((rel) => fs.existsSync(path.join(REPO_ROOT, rel))),
    playwrightCorpus: opts.playwrightCorpus,
    requiredClaimKeys: opts.requiredClaimKeys ?? REQUIRED_CLAIM_KEYS,
  });
}

describe("resolveJsonPointer", () => {
  it("resolves dotted paths and array indices", () => {
    const data = { a: { b: ["x", "y", { z: 1 }] } };
    expect(resolveJsonPointer(data, "a.b[1]")).toEqual({
      found: true,
      value: "y",
    });
    expect(resolveJsonPointer(data, "a.b[2].z")).toEqual({
      found: true,
      value: 1,
    });
    expect(resolveJsonPointer(data, "a.missing").found).toBe(false);
  });
});

describe("check-advertised-feature-contract rules", () => {
  test("good fixture passes all default rules (1–7)", () => {
    const result = validateFixture(goodRegistry());
    expect(result.ok).toBe(true);
    expect(result.exitCode).toBe(0);
    expect(result.errors).toEqual([]);
  });

  test("rule 1: unknown field fails", () => {
    const reg = goodRegistry() as FeatureRegistry & {
      features: Array<FeatureRow & { extraField?: string }>;
    };
    reg.features[0].extraField = "nope";
    const result = validateFixture(reg);
    expect(result.ok).toBe(false);
    expect(hasRule(result, 1)).toBe(true);
    expect(result.errors.some((e) => e.message.includes("unknown field"))).toBe(
      true,
    );
  });

  test("rule 1: unknown status enum fails", () => {
    const reg = goodRegistry({
      status: "shipped" as FeatureRow["status"],
    });
    const result = validateFixture(reg);
    expect(result.ok).toBe(false);
    expect(hasRule(result, 1)).toBe(true);
    expect(
      result.errors.some((e) => e.message.includes("unknown status")),
    ).toBe(true);
  });

  test("rule 1: last_verified.sha must be a real 7-40 character hex commit", () => {
    for (const sha of ["pending", "abc123", "not-a-commit", "a".repeat(41)]) {
      const result = validateFixture(
        goodRegistry({ last_verified: { sha, date: "2026-07-11" } }),
      );
      expect(result.ok).toBe(false);
      expect(
        result.errors.some((error) =>
          error.message.includes("7-40 character hexadecimal commit"),
        ),
      ).toBe(true);
    }
  });

  test("rule 2: duplicate ids fail", () => {
    const a = baseRow({
      id: "dup-id",
      keys: coverRequiredKeys(),
    });
    const b = baseRow({
      id: "dup-id",
      title: "Other",
      keys: [],
    });
    const result = validateFixture(registryOf(a, b));
    expect(result.ok).toBe(false);
    expect(hasRule(result, 2)).toBe(true);
    expect(result.errors.some((e) => e.message.includes("Duplicate id"))).toBe(
      true,
    );
  });

  test("rule 3: live row with empty evidence fails", () => {
    const result = validateFixture(
      goodRegistry({
        status: "live",
        evidence: [],
      }),
    );
    expect(result.ok).toBe(false);
    expect(hasRule(result, 3)).toBe(true);
    expect(
      result.errors.some((e) => e.message.includes("empty evidence")),
    ).toBe(true);
  });

  test("rule 4: missing evidence path fails", () => {
    const result = validateFixture(
      goodRegistry({
        evidence: [
          {
            path: "frontend/src/does-not-exist-ever.ts",
            note: "ghost file",
          },
        ],
      }),
    );
    expect(result.ok).toBe(false);
    expect(hasRule(result, 4)).toBe(true);
    expect(
      result.errors.some((e) => e.message.includes("does not exist")),
    ).toBe(true);
  });

  test("rule 4: evidence path must match repository path casing", () => {
    const caseMismatchedPath = "frontend/src/components/business/marketing";
    const result = validateFixture(
      goodRegistry({
        evidence: [
          {
            path: caseMismatchedPath,
            note: "case-mismatched marketing studio path",
          },
        ],
      }),
      {
        pathExists: (rel) =>
          rel === caseMismatchedPath
            ? false
            : fs.existsSync(path.join(REPO_ROOT, rel)),
      },
    );
    expect(result.ok).toBe(false);
    expect(hasRule(result, 4)).toBe(true);
    expect(
      result.errors.some((e) => e.message.includes("case mismatch")),
    ).toBe(true);
  });

  test("rule 5: stale i18n key fails", () => {
    const result = validateFixture(
      goodRegistry({
        keys: coverRequiredKeys([
          "businessSettings.json#businessPage.thisKeyDoesNotExist",
        ]),
      }),
    );
    expect(result.ok).toBe(false);
    expect(hasRule(result, 5)).toBe(true);
    expect(result.errors.some((e) => e.message.includes("stale key"))).toBe(
      true,
    );
  });

  test("rule 6: market_limited with only global markets fails", () => {
    const result = validateFixture(
      goodRegistry({
        status: "market_limited",
        markets: ["global"],
      }),
    );
    expect(result.ok).toBe(false);
    expect(hasRule(result, 6)).toBe(true);
    expect(
      result.errors.some((e) => e.message.includes("market_limited")),
    ).toBe(true);
  });

  test("rule 7: adding/changing a public claim key without a registry entry fails", () => {
    // Drop one curated claim key so coverage is incomplete.
    const incomplete: string[] = [...REQUIRED_CLAIM_KEYS.slice(1)];
    const result = validateFixture(
      registryOf(
        baseRow({
          keys: incomplete,
        }),
      ),
      { requiredClaimKeys: REQUIRED_CLAIM_KEYS },
    );
    expect(result.ok).toBe(false);
    expect(hasRule(result, 7)).toBe(true);
    expect(
      result.errors.some((e) => e.message.includes("Unrepresented claim key")),
    ).toBe(true);
    // The missing key is the first curated entry.
    expect(
      result.errors.some((e) => e.message.includes(REQUIRED_CLAIM_KEYS[0])),
    ).toBe(true);
  });

  test("rule 8 (strict-tests): live empty test_ids fails only when flag is on", () => {
    const reg = goodRegistry({
      status: "live",
      test_ids: [],
    });

    const defaultMode = validateFixture(reg, { strictTests: false });
    expect(defaultMode.ok).toBe(true);
    expect(hasRule(defaultMode, 8)).toBe(false);

    const strict = validateFixture(reg, { strictTests: true });
    expect(strict.ok).toBe(false);
    expect(hasRule(strict, 8)).toBe(true);
    expect(
      strict.errors.some((e) => e.message.includes("empty test_ids")),
    ).toBe(true);
  });

  test("rule 8 (strict-tests): missing Playwright test_id fails", () => {
    const reg = goodRegistry({
      status: "live",
      test_ids: ["journey-that-does-not-exist-xyz"],
    });
    const strict = validateFixture(reg, {
      strictTests: true,
      playwrightCorpus: "test('some other journey', async () => {})",
    });
    expect(strict.ok).toBe(false);
    expect(hasRule(strict, 8)).toBe(true);
    expect(
      strict.errors.some((e) =>
        e.message.includes("not found in any Playwright"),
      ),
    ).toBe(true);
  });

  test("rule 8 (strict-tests): present test_id in corpus passes", () => {
    const reg = goodRegistry({
      status: "live",
      test_ids: ["guest-checkout-happy-path"],
    });
    const strict = validateFixture(reg, {
      strictTests: true,
      playwrightCorpus:
        "test('guest-checkout-happy-path', async ({ page }) => {})",
    });
    expect(strict.ok).toBe(true);
    expect(hasRule(strict, 8)).toBe(false);
  });

  test("coming_soon may have empty evidence without failing rule 3", () => {
    const result = validateFixture(
      goodRegistry({
        status: "coming_soon",
        evidence: [],
        markets: ["global"],
      }),
    );
    expect(hasRule(result, 3)).toBe(false);
    expect(result.ok).toBe(true);
  });
});

describe("real registry integration", () => {
  test("real advertised-feature-contract.json passes default (non-strict) validation", () => {
    expect(fs.existsSync(REAL_REGISTRY)).toBe(true);
    expect(fs.existsSync(REAL_SCHEMA)).toBe(true);

    const result = validateAdvertisedFeatureContract({
      registryPath: REAL_REGISTRY,
      schemaPath: REAL_SCHEMA,
      repoRoot: REPO_ROOT,
      messagesDir: REAL_MESSAGES,
      strictTests: false,
    });

    if (!result.ok) {
      // Surface full messages for debugging when integration fails.
      // eslint-disable-next-line no-console
      console.error(result.errors);
    }
    expect(result.ok).toBe(true);
    expect(result.exitCode).toBe(0);
    expect(result.errors).toEqual([]);
  });

  test("schema and registry JSON parse cleanly", () => {
    const schema = JSON.parse(fs.readFileSync(REAL_SCHEMA, "utf8"));
    const registry = JSON.parse(fs.readFileSync(REAL_REGISTRY, "utf8"));
    expect(schema.$schema).toContain("2020-12");
    expect(registry.version).toBe(1);
    expect(Array.isArray(registry.features)).toBe(true);
    expect(registry.features.length).toBeGreaterThan(20);
  });

  test("tmp bad registry on disk fails CLI-equivalent path", () => {
    const tmp = fs.mkdtempSync(path.join(os.tmpdir(), "adv-contract-"));
    const badPath = path.join(tmp, "bad.json");
    fs.writeFileSync(
      badPath,
      JSON.stringify({
        version: 1,
        features: [
          baseRow({
            id: "bad-live",
            status: "live",
            evidence: [],
            keys: [...REQUIRED_CLAIM_KEYS],
          }),
        ],
      }),
    );
    const result = validateAdvertisedFeatureContract({
      registryPath: badPath,
      schemaPath: REAL_SCHEMA,
      repoRoot: REPO_ROOT,
      messagesDir: REAL_MESSAGES,
      strictTests: false,
    });
    expect(result.ok).toBe(false);
    expect(hasRule(result, 3)).toBe(true);
  });
});
