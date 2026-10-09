/** @jest-environment node */
import fs from "node:fs";
import path from "node:path";

const SOURCE = fs.readFileSync(
  path.resolve(__dirname, "../BusinessSettings.tsx"),
  "utf-8",
);

describe("BusinessSettings — split profile/payments payloads (Stream 9 #5)", () => {
  test("persistProfile is section-aware (profile | payments)", () => {
    expect(SOURCE).toMatch(
      /persistProfile\s*=\s*async\s*\(\s*section:\s*"profile"\s*\|\s*"payments"/,
    );
  });

  test("profile section payload does not include payment or contact fields", () => {
    // The profile branch of the ternary should only send identity fields.
    const profileBranch = SOURCE.match(
      /section === "payments"\s*\?\s*\{[\s\S]*?\}\s*:\s*\{([\s\S]*?)\}/,
    );
    expect(profileBranch).not.toBeNull();
    const body = profileBranch![1];
    expect(body).toContain("name:");
    expect(body).toContain("logo:");
    expect(body).toContain("business_type:");
    expect(body).not.toContain("tax_rate");
    expect(body).not.toContain("phone");
    expect(body).not.toContain("website");
    expect(body).not.toContain("address");
    expect(body).not.toContain("settlement_address");
  });

  test("payments section payload does not include profile name/logo/contact", () => {
    const paymentsBranch = SOURCE.match(
      /section === "payments"\s*\?\s*\{([\s\S]*?)\}\s*:/,
    );
    expect(paymentsBranch).not.toBeNull();
    const body = paymentsBranch![1];
    expect(body).toContain("tax_rate");
    expect(body).toContain("service_fee_rate");
    expect(body).not.toContain("name:");
    expect(body).not.toContain("phone");
    expect(body).not.toContain("website");
  });

  test("profile tab deep-links contact editing to Business Page → Contact (#225)", () => {
    expect(SOURCE).toContain("getBusinessPageEditorPath");
    expect(SOURCE).toContain("contactReadOnlyTitle");
    expect(SOURCE).toContain("editContactOnBusinessPage");
    // Must pass the Contact section — bare ?tab=business-page lands on Essentials.
    expect(SOURCE).toMatch(
      /getBusinessPageEditorPath\s*\(\s*\{[\s\S]*?\}\s*,\s*"contact"\s*,?\s*\)/,
    );
    expect(SOURCE).toContain("edit-contact-on-business-page");
    expect(SOURCE).toContain("settings-guest-ordering");
  });
});
