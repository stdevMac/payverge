/**
 * The account export (POST /inside/account/export) returns the profile and
 * owned businesses only. The copy must not promise data the file omits.
 */
import fs from "fs";
import path from "path";
import { messages } from "@/i18n/getTranslation";

const PANEL = fs.readFileSync(
  path.resolve(__dirname, "AccountPrivacyPanel.tsx"),
  "utf8",
);

describe("account export copy matches the export payload", () => {
  it.each([
    ["en", messages.en.account.privacy.exportDescription],
    ["es", messages.es.account.privacy.exportDescription],
    ["es-AR", messages["es-AR"].account.privacy.exportDescription],
  ])("%s copy", (_locale, copy) => {
    expect(copy).toMatchSnapshot();
    expect(copy).not.toMatch(/billing|factura/i);
  });

  it("panel fallback copy matches the en catalog", () => {
    expect(PANEL).toContain(
      `"${messages.en.account.privacy.exportDescription}"`,
    );
  });
});
