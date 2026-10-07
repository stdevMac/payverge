const fs = require("fs");
const os = require("os");
const path = require("path");
const {
  runValidator,
  runCriticalValidator,
  placeholdersIn,
  computeSameAsEnCounts,
} = require("../check-guest-locales");

const fixtureDir = (name) => path.resolve(__dirname, "fixtures", name);

// Writes a throwaway guest-messages tree to a temp dir and returns its path.
// `locales` maps a locale code to its flat key/value object.
function makeTree(en, locales, allowlist) {
  const tmp = fs.mkdtempSync(path.join(os.tmpdir(), "ratchet-"));
  fs.writeFileSync(path.join(tmp, "en.json"), JSON.stringify(en));
  if (allowlist) {
    fs.writeFileSync(
      path.join(tmp, ".same-as-en-allowlist.json"),
      JSON.stringify(allowlist),
    );
  }
  for (const [loc, obj] of Object.entries(locales)) {
    fs.writeFileSync(path.join(tmp, `${loc}.json`), JSON.stringify(obj));
  }
  return tmp;
}

describe("check-guest-locales validator", () => {
  test("clean tree exits 0 with no errors", () => {
    const result = runValidator({ dir: fixtureDir("clean") });
    expect(result.exitCode).toBe(0);
    expect(result.hardErrors).toEqual([]);
  });

  test("missing key is a hard error (exit 1)", () => {
    const result = runValidator({ dir: fixtureDir("missing-key") });
    expect(result.exitCode).toBe(1);
    expect(result.hardErrors).toEqual(
      expect.arrayContaining([
        expect.objectContaining({
          locale: "es",
          type: "missing_key",
          key: "farewell",
        }),
      ]),
    );
  });

  test("broken placeholder is a hard error (exit 1)", () => {
    const result = runValidator({ dir: fixtureDir("broken-placeholder") });
    expect(result.exitCode).toBe(1);
    expect(result.hardErrors).toEqual(
      expect.arrayContaining([
        expect.objectContaining({
          locale: "es",
          type: "placeholder_mismatch",
          key: "welcome",
          missing: ["{name}"],
        }),
      ]),
    );
  });

  test("extra key in locale is a hard error (exit 1)", () => {
    const result = runValidator({ dir: fixtureDir("extra-key") });
    expect(result.exitCode).toBe(1);
    expect(result.hardErrors).toEqual(
      expect.arrayContaining([
        expect.objectContaining({
          locale: "es",
          type: "extra_key",
          key: "extraKey",
        }),
      ]),
    );
  });

  test("allowlisted same-as-en values pass; the rest are hard errors", () => {
    const result = runValidator({ dir: fixtureDir("same-as-en") });
    const sameAsEn = result.hardErrors.filter((e) => e.type === "same_as_en");
    expect(sameAsEn).toEqual([
      expect.objectContaining({ locale: "es", key: "untranslated" }),
    ]);
    expect(result.softWarnings.filter((w) => w.type === "same_as_en")).toEqual(
      [],
    );
  });

  test("an English leaf left in a locale fails the check in every mode (exit 1)", () => {
    for (const strict of [false, true]) {
      const result = runValidator({
        dir: fixtureDir("same-as-en"),
        strict,
        coverageThreshold: 95,
      });
      expect(result.exitCode).toBe(1);
    }
  });

  test("the real guest catalogs carry no English leaves outside the allowlist", () => {
    const result = runValidator({});
    expect(result.hardErrors).toEqual([]);
  });

  test("coverage counts allowlisted same-as-EN as healthy and never exceeds 100%", () => {
    // Fixture: 2 keys, 0 translated, 1 same-as-en in allowlist, 1 same-as-en not in
    // allowlist. Healthy = translated + allowlisted_same_as_en = 0 + 1 = 1 of 2 = 50%.
    const result = runValidator({ dir: fixtureDir("same-as-en") });
    expect(result.coverage.es.coverage).toBe(50);
  });

  test("clean tree reports 100% coverage", () => {
    const result = runValidator({ dir: fixtureDir("clean") });
    expect(result.coverage.es.coverage).toBe(100);
  });

  test("invalid JSON is a hard error (exit 1)", () => {
    const fs = require("fs");
    const os = require("os");
    const tmp = fs.mkdtempSync(path.join(os.tmpdir(), "validator-"));
    fs.writeFileSync(path.join(tmp, "en.json"), '{"valid": true}');
    fs.writeFileSync(path.join(tmp, "es.json"), "{ not json");
    const result = runValidator({ dir: tmp });
    expect(result.exitCode).toBe(1);
    expect(result.hardErrors).toEqual(
      expect.arrayContaining([
        expect.objectContaining({ locale: "es", type: "parse_error" }),
      ]),
    );
  });

  test("ICU plural blocks are not treated as required placeholders", () => {
    // Inner `{item}` `{items}` are ICU plural form options inside the outer
    // `{{...}}` block; translations may use different forms per language and
    // must not be forced to keep the English vocabulary.
    const value = "{count} {{count, plural, one {item} other {items}}} in cart";
    expect(placeholdersIn(value)).toEqual(["{count}"]);
  });
});

describe("launch-critical guest locale gate", () => {
  function criticalConfig(keys = ["payment.failed"]) {
    return {
      version: 1,
      locales: ["en", "es", "es-AR"],
      domains: { payment: keys },
      publicErrorCodes: { payment_failed: "payment.failed" },
    };
  }

  function writeCriticalTree({ en, es, esAR, config, invariants = [] }) {
    const dir = makeTree(en, { es, "es-AR": esAR });
    const root = fs.mkdtempSync(path.join(os.tmpdir(), "critical-i18n-"));
    const configPath = path.join(root, "critical.json");
    const invariantsPath = path.join(root, "invariants.json");
    fs.writeFileSync(configPath, JSON.stringify(config));
    fs.writeFileSync(
      invariantsPath,
      JSON.stringify({ version: 1, invariants }),
    );
    return { dir, configPath, invariantsPath, publicErrorCodes: [] };
  }

  test("passes reviewed en/es/es-AR values and exact placeholders", () => {
    const fixture = writeCriticalTree({
      en: { payment: { failed: "Payment {id} failed" } },
      es: { payment: { failed: "Falló el pago {id}" } },
      esAR: { payment: { failed: "Falló el pago {id}" } },
      config: criticalConfig(),
    });
    expect(runCriticalValidator(fixture)).toEqual(
      expect.objectContaining({ exitCode: 0, hardErrors: [] }),
    );
  });

  test("hard-fails a missing source key", () => {
    const fixture = writeCriticalTree({
      en: {},
      es: {},
      esAR: {},
      config: criticalConfig(),
    });
    const result = runCriticalValidator(fixture);
    expect(result.hardErrors).toContainEqual(
      expect.objectContaining({ locale: "en", type: "critical_missing_key" }),
    );
  });

  test("hard-fails an inherited es-AR value that is not explicitly reviewed", () => {
    const fixture = writeCriticalTree({
      en: { payment: { failed: "Payment failed" } },
      es: { payment: { failed: "Falló el pago" } },
      esAR: { payment: {} },
      config: criticalConfig(),
    });
    const result = runCriticalValidator(fixture);
    expect(result.hardErrors).toContainEqual(
      expect.objectContaining({
        locale: "es-AR",
        type: "critical_inherited_unreviewed",
        key: "payment.failed",
      }),
    );
  });

  test("hard-fails English equality unless a reasoned invariant allows it", () => {
    const base = {
      en: { payment: { failed: "USDC" } },
      es: { payment: { failed: "USDC" } },
      esAR: { payment: { failed: "USDC" } },
      config: criticalConfig(),
    };
    const failing = runCriticalValidator(writeCriticalTree(base));
    expect(failing.hardErrors).toContainEqual(
      expect.objectContaining({ locale: "es", type: "critical_same_as_en" }),
    );

    const passing = runCriticalValidator(
      writeCriticalTree({
        ...base,
        invariants: [
          {
            key: "payment.failed",
            locales: ["es", "es-AR"],
            reason: "USDC is the currency ticker.",
          },
        ],
      }),
    );
    expect(passing.exitCode).toBe(0);
  });

  test("hard-fails missing, extra, or duplicated interpolation placeholders", () => {
    const fixture = writeCriticalTree({
      en: { payment: { failed: "Payment {id} failed for {id}" } },
      es: { payment: { failed: "Falló el pago {id}" } },
      esAR: { payment: { failed: "Falló el pago {id} para {other}" } },
      config: criticalConfig(),
    });
    const result = runCriticalValidator(fixture);
    expect(
      result.hardErrors.filter(
        (e) => e.type === "critical_placeholder_mismatch",
      ),
    ).toHaveLength(2);
  });

  test("hard-fails a backend public error code without a localized mapping", () => {
    const fixture = writeCriticalTree({
      en: { payment: { failed: "Payment failed" } },
      es: { payment: { failed: "Falló el pago" } },
      esAR: { payment: { failed: "Falló el pago" } },
      config: { ...criticalConfig(), publicErrorCodes: {} },
    });
    fixture.publicErrorCodes = ["payment_failed"];
    const result = runCriticalValidator(fixture);
    expect(result.hardErrors).toContainEqual(
      expect.objectContaining({
        type: "public_error_code_unmapped",
        code: "payment_failed",
      }),
    );
  });

  test("hard-fails a mapped public code that no guest error presenter consumes", () => {
    const fixture = writeCriticalTree({
      en: { payment: { failed: "Payment failed" } },
      es: { payment: { failed: "Falló el pago" } },
      esAR: { payment: { failed: "Falló el pago" } },
      config: criticalConfig(),
    });
    const repoRoot = fs.mkdtempSync(path.join(os.tmpdir(), "error-map-"));
    const mappingDir = path.join(repoRoot, "frontend/src/lib");
    fs.mkdirSync(mappingDir, { recursive: true });
    fs.writeFileSync(
      path.join(mappingDir, "guestOrderErrors.ts"),
      "export {};\n",
    );
    const result = runCriticalValidator({
      ...fixture,
      repoRoot,
      publicErrorCodes: ["payment_failed"],
    });
    expect(result.hardErrors).toContainEqual(
      expect.objectContaining({
        type: "public_error_code_not_consumed",
        code: "payment_failed",
      }),
    );
  });

  test("the production launch-critical manifest passes", () => {
    const result = runCriticalValidator();
    expect(result.hardErrors).toEqual([]);
    expect(result.exitCode).toBe(0);
  });

  test("the production manifest covers launch cart, stock, cancellation, and recovery copy", () => {
    const manifest = JSON.parse(
      fs.readFileSync(
        path.resolve(
          __dirname,
          "../../src/i18n/critical-guest-keys.json",
        ),
        "utf8",
      ),
    );
    const criticalKeys = new Set(Object.values(manifest.domains).flat());

    for (const key of [
      "menu.remove",
      "menu.cartItemsRemovedUnavailable",
      "menu.quantityCapReached",
      "orders.alreadyCancelled",
      "reservationConfirmation.action.alreadyCancelledBody",
      "reservationConfirmation.action.windowClosedBody",
      "errors.networkErrorDescription",
      "errors.serverErrorDescription",
      "deliveryTracking.loadError",
    ]) {
      expect(criticalKeys.has(key)).toBe(true);
    }
  });
});

describe("same-as-en ratchet (GUEST-3)", () => {
  const en = { a: "Alpha", b: "Bravo", c: "Charlie" };
  // Same-as-en leaves outside the allowlist are hard errors on their own; the
  // passing ratchet cases allowlist them so only the ratchet is under test.
  const allowAll = { all: ["a", "b", "c"] };

  test("computeSameAsEnCounts returns the raw per-locale same-as-en count", () => {
    // es: 2 same-as-en (a, b). fr: 1 (a). Allowlist must NOT affect the raw count
    // — the ratchet measures untranslated leaves, allowlisted or not.
    const dir = makeTree(en, {
      es: { a: "Alpha", b: "Bravo", c: "Carlos" },
      fr: { a: "Alpha", b: "Bravo-FR", c: "Charlie-FR" },
    });
    const counts = computeSameAsEnCounts({ dir });
    expect(counts).toEqual({ es: 2, fr: 1 });
  });

  test("count below baseline passes (exit 0)", () => {
    const dir = makeTree(en, {
      es: { a: "Alpha", b: "Bravo-ES", c: "Carlos" }, // 1 same-as-en
   }, allowAll);
    const result = runValidator({
      dir,
      strict: true,
      ratchetBaseline: { es: 2 },
    });
    expect(result.exitCode).toBe(0);
    expect(
      result.hardErrors.filter((e) => e.type === "ratchet_exceeded"),
    ).toEqual([]);
  });

  test("count equal to baseline passes (exit 0)", () => {
    const dir = makeTree(en, {
      es: { a: "Alpha", b: "Bravo", c: "Carlos" }, // 2 same-as-en
   }, allowAll);
    const result = runValidator({
      dir,
      strict: true,
      ratchetBaseline: { es: 2 },
    });
    expect(result.exitCode).toBe(0);
  });

  test("count exceeding baseline is a hard failure (exit 1)", () => {
    const dir = makeTree(en, {
      es: { a: "Alpha", b: "Bravo", c: "Charlie" }, // 3 same-as-en
    });
    const result = runValidator({
      dir,
      strict: true,
      ratchetBaseline: { es: 2 },
    });
    expect(result.exitCode).toBe(1);
    expect(result.hardErrors).toEqual(
      expect.arrayContaining([
        expect.objectContaining({
          locale: "es",
          type: "ratchet_exceeded",
          count: 3,
          baseline: 2,
        }),
      ]),
    );
  });

  test("a brand-new locale missing from the baseline must not exceed 0", () => {
    // A locale absent from the baseline is treated as baseline 0, so it cannot
    // silently introduce a fresh backlog of untranslated leaves.
    const dir = makeTree(en, {
      de: { a: "Alpha", b: "Bravo-DE", c: "Charlie-DE" }, // 1 same-as-en
    });
    const result = runValidator({
      dir,
      strict: true,
      ratchetBaseline: {}, // de not listed -> baseline 0
    });
    expect(result.exitCode).toBe(1);
    expect(result.hardErrors).toEqual(
      expect.arrayContaining([
        expect.objectContaining({
          locale: "de",
          type: "ratchet_exceeded",
          count: 1,
          baseline: 0,
        }),
      ]),
    );
  });

  test("ratchet is inert when no baseline is provided", () => {
    const dir = makeTree(
      en,
      {
        es: { a: "Alpha", b: "Bravo", c: "Charlie" }, // 3 same-as-en
      },
      allowAll,
    );
    const result = runValidator({ dir, strict: true });
    expect(result.exitCode).toBe(0);
  });
});

describe("production allowlist + baseline integrity (GUEST-5/GUEST-6)", () => {
  const guestDir = path.resolve(
    __dirname,
    "..",
    "..",
    "src",
    "i18n",
    "guest-messages",
  );
  const allowlist = JSON.parse(
    fs.readFileSync(path.join(guestDir, ".same-as-en-allowlist.json"), "utf8"),
  );

  test("allowlist has no malformed (non-array, non-_comment) top-level entry", () => {
    // GUEST-6: a stray `bill` OBJECT used to sit here masquerading as a locale.
    // loadAllowlist silently coerced it to [], so it allowlisted nothing while
    // misleading maintainers. Every top-level value (besides `_comment` and the
    // `all` array) must be a per-locale string array.
    for (const [key, value] of Object.entries(allowlist)) {
      if (key === "_comment") {
        expect(typeof value).toBe("string");
        continue;
      }
      expect(Array.isArray(value)).toBe(true);
    }
  });

  test("the production validator passes its own ratchet + keys (GUEST-3 self-check)", () => {
    // The committed baseline must reflect the real tree. After M3 translations
    // the counts only went down, so the production tree at or below baseline.
    const baseline = JSON.parse(
      fs.readFileSync(
        path.resolve(__dirname, "..", "guest-same-as-en-baseline.json"),
        "utf8",
      ),
    );
    const result = runValidator({
      dir: guestDir,
      strict: true,
      ratchetBaseline: baseline,
    });
    const ratchetFails = result.hardErrors.filter(
      (e) => e.type === "ratchet_exceeded",
    );
    expect(ratchetFails).toEqual([]);
  });
});

describe("critical locale CI ordering", () => {
  test("ci.yml lists the critical gate ahead of the Jest and build jobs", () => {
    const workflow = fs.readFileSync(
      path.resolve(
        __dirname,
        "..",
        "..",
        "..",
        ".github",
        "workflows",
        "ci.yml",
      ),
      "utf8",
    );
    const critical = workflow.indexOf("check-guest-locales.js --critical");
    const jest = workflow.indexOf("npx jest --ci");
    const build = workflow.indexOf("npm run build");
    expect(critical).toBeGreaterThan(-1);
    expect(critical).toBeLessThan(jest);
    expect(critical).toBeLessThan(build);
  });
});
