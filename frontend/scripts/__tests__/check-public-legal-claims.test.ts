import * as fs from "node:fs";
import * as os from "node:os";
import * as path from "node:path";

import {
  REQUIRED_CLAIM_CLASSES,
  scanLocaleClaims,
  scanRepositoryLocaleClaims,
} from "../check-public-legal-claims";

describe("public legal claim scan", () => {
  test("covers every D7 claim class", () => {
    expect(REQUIRED_CLAIM_CLASSES).toEqual([
      "fiat_conversion",
      "setup_time",
      "monitoring",
      "security",
      "support",
      "uptime",
      "tax",
      "fiscal",
      "pci",
      "compliance",
    ]);
  });

  test("reports unsupported claims with locale, JSON path, and class", () => {
    const fixtureRoot = fs.mkdtempSync(
      path.join(os.tmpdir(), "payverge-legal-claims-"),
    );
    const localeDir = path.join(fixtureRoot, "messages", "en");
    fs.mkdirSync(localeDir, { recursive: true });
    fs.writeFileSync(
      path.join(localeDir, "landing.json"),
      JSON.stringify({
        fiat: "USDC automatically converts to fiat instantly.",
        uptime: "Guaranteed 99.99% uptime.",
        pci: "Payverge is PCI DSS certified.",
      }),
    );

    const result = scanLocaleClaims([fixtureRoot]);
    expect(result.findings).toEqual(
      expect.arrayContaining([
        expect.objectContaining({
          claimClass: "fiat_conversion",
          locale: "en",
          jsonPath: "fiat",
        }),
        expect.objectContaining({
          claimClass: "uptime",
          locale: "en",
          jsonPath: "uptime",
        }),
        expect.objectContaining({
          claimClass: "pci",
          locale: "en",
          jsonPath: "pci",
        }),
      ]),
    );
  });

  test("excludes fixtures and dotfiles from locale coverage", () => {
    const fixtureRoot = fs.mkdtempSync(
      path.join(os.tmpdir(), "payverge-legal-coverage-"),
    );
    const enDir = path.join(fixtureRoot, "messages", "en");
    const testsDir = path.join(fixtureRoot, "messages", "__tests__");
    const guestDir = path.join(fixtureRoot, "guest-messages");
    fs.mkdirSync(enDir, { recursive: true });
    fs.mkdirSync(testsDir, { recursive: true });
    fs.mkdirSync(guestDir, { recursive: true });
    fs.writeFileSync(path.join(enDir, "legal.json"), "{}");
    fs.writeFileSync(path.join(testsDir, "fixture.json"), "{}");
    fs.writeFileSync(path.join(guestDir, ".baseline.json"), "{}");
    fs.writeFileSync(path.join(guestDir, "es-AR.json"), "{}");

    const result = scanLocaleClaims([fixtureRoot]);
    expect(result.filesScanned).toBe(2);
    expect(result.localesScanned).toEqual(["en", "es-AR"]);
  });

  test("scans every repository locale catalog and rejects unsupported claims", () => {
    const repoRoot = path.resolve(__dirname, "..", "..", "..");
    const result = scanRepositoryLocaleClaims(repoRoot);

    expect(result.filesScanned).toBeGreaterThan(0);
    expect(result.localesScanned).toEqual(
      expect.arrayContaining(["en", "es", "es-AR"]),
    );
    expect(result.claimClassesChecked).toEqual(REQUIRED_CLAIM_CLASSES);
    expect(result.findings).toEqual([]);
  });
});
