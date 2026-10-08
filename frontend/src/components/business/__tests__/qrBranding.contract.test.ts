/**
 * Root D + L3-29 — QR footer branding is required and consistent.
 *
 * Four surfaces must share one branding treatment:
 * 1. QRCodeWithText (preview canvas) — poweredByText is a required prop
 * 2. QRCustomizationModal — passes translated poweredBy
 * 3. TableDetailModal — must pass poweredByText (no English default)
 * 4. qrDownload + qrSheetHtml — footer on PNG download and print sheet
 */
import { readFileSync } from "fs";
import { join } from "path";

const root = join(__dirname, "..");

describe("QR branding contract (Root D / L3-29)", () => {
  test("QRCodeWithText requires poweredByText (no English literal default)", () => {
    const src = readFileSync(join(root, "QRCodeWithText.tsx"), "utf8");
    // Required prop — no `?` optional marker on poweredByText
    expect(src).toMatch(/poweredByText:\s*string;/);
    expect(src).not.toMatch(/poweredByText\?:\s*string/);
    // No English literal default on the destructured prop
    expect(src).not.toMatch(
      /poweredByText\s*=\s*["']Powered by Payverge["']/,
    );
  });

  test("TableDetailModal passes poweredByText into QRCodeWithText", () => {
    const src = readFileSync(
      join(root, "tables/TableDetailModal.tsx"),
      "utf8",
    );
    expect(src).toMatch(/<QRCodeWithText[\s\S]*?poweredByText=/);
  });

  test("QRCustomizationModal passes translated poweredBy", () => {
    const src = readFileSync(join(root, "QRCustomizationModal.tsx"), "utf8");
    expect(src).toMatch(/poweredByText=\{t\(["']poweredBy["']\)\}/);
  });

  test("qrDownload accepts poweredByText and paints a footer", () => {
    const src = readFileSync(join(root, "tables/qrDownload.ts"), "utf8");
    expect(src).toMatch(/poweredByText/);
    expect(src).toMatch(/fillText/);
  });

  test("qrSheetHtml QrSheetStrings includes poweredBy and renders it", () => {
    const src = readFileSync(join(root, "tables/qrSheetHtml.ts"), "utf8");
    expect(src).toMatch(/poweredBy:\s*string/);
    expect(src).toMatch(/strings\.poweredBy/);
  });
});
