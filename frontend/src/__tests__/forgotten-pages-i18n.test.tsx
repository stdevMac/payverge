/**
 * Regression + smoke test for Round-3 Task 14.
 *
 * Asserts that /forgot-password and /scan pages have their
 * guest-facing English strings wired to the i18n layer instead of being
 * hardcoded. Also verifies the TopMenu "Staff Dashboard" string is
 * resolved via tString() rather than being a literal.
 */

import fs from "fs";
import path from "path";

const FRONTEND_ROOT = path.resolve(__dirname, "..", "..");

function read(relative: string): string {
  return fs.readFileSync(path.join(FRONTEND_ROOT, relative), "utf8");
}

function readJson<T = Record<string, unknown>>(relative: string): T {
  return JSON.parse(read(relative)) as T;
}

describe("forgotten guest-facing pages are translated", () => {
  describe("/forgot-password", () => {
    // Server page owns metadata; client page owns guest-facing copy.
    const serverFile = "src/app/forgot-password/page.tsx";
    const clientFile = "src/app/forgot-password/ForgotPasswordClient.tsx";
    let serverSrc: string;
    let clientSrc: string;
    let src: string;

    beforeAll(() => {
      serverSrc = read(serverFile);
      clientSrc = read(clientFile);
      src = `${serverSrc}\n${clientSrc}`;
    });

    it("uses the simple translation layer", () => {
      expect(serverSrc).toMatch(/getTranslation/);
      expect(clientSrc).toMatch(
        /SimpleTranslationProvider|useSimpleLocale|getTranslation/,
      );
      expect(clientSrc).toMatch(/\bt\s*\(/);
    });

    it("no longer hardcodes English guest copy", () => {
      expect(src).not.toMatch(/>\s*Check your inbox\s*</);
      expect(src).not.toMatch(/>\s*Reset link sent\s*</);
      expect(src).not.toMatch(/>\s*Return to login\s*</);
      expect(src).not.toMatch(/>\s*Account access\s*</);
      expect(src).not.toMatch(/>\s*Reset your password\s*</);
      expect(src).not.toMatch(/>\s*Enter your email to receive a reset link\.?\s*</);
      expect(src).not.toMatch(/>\s*Send reset link\s*</);
      expect(src).not.toMatch(/>\s*Back to login\s*</);
    });
  });

  describe("/scan", () => {
    const file = "src/app/scan/page.tsx";
    let src: string;

    beforeAll(() => {
      src = read(file);
    });

    it("uses GuestTranslationProvider (guest-facing, 21-locale tier)", () => {
      expect(src).toMatch(/GuestTranslationProvider/);
    });

    it("no longer hardcodes English guest copy", () => {
      expect(src).not.toMatch(/>\s*Access Your Table\s*</);
      expect(src).not.toMatch(/>\s*Scan QR Code\s*</);
      expect(src).not.toMatch(/>\s*Access Table\s*</);
      expect(src).not.toMatch(/Scan the QR code at your table/);
      expect(src).not.toMatch(/Need help\? Contact restaurant staff\./);
    });
  });

  describe("TopMenu Staff Dashboard label", () => {
    const file = "src/components/ui/top-menu/TopMenu.tsx";
    let src: string;

    beforeAll(() => {
      src = read(file);
    });

    it("no longer hardcodes the Staff Dashboard string", () => {
      expect(src).not.toMatch(/name:\s*["']Staff Dashboard["']/);
    });

    it("uses tString for the staff dashboard label", () => {
      expect(src).toMatch(/tString\(\s*["']staffDashboard["']\s*\)/);
    });
  });
});

describe("i18n JSON bundles exist for new namespaces", () => {
  const namespaces = ["forgotPassword"] as const;
  for (const ns of namespaces) {
    for (const locale of ["en", "es"] as const) {
      it(`${locale}/${ns}.json has non-empty keys`, () => {
        const data = readJson(`src/i18n/messages/${locale}/${ns}.json`);
        expect(typeof data).toBe("object");
        expect(Object.keys(data).length).toBeGreaterThan(0);
      });
    }
  }

  it("navigation bundle has staffDashboard key in both locales", () => {
    const en = readJson<Record<string, string>>("src/i18n/messages/en/navigation.json");
    const es = readJson<Record<string, string>>("src/i18n/messages/es/navigation.json");
    expect(en.staffDashboard).toBeTruthy();
    expect(es.staffDashboard).toBeTruthy();
  });
});
