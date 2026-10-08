import fs from "fs";
import path from "path";

/**
 * #682 parity guard for the MENU surface.
 *
 * /t/[code]/bill already retries a transient table miss and refuses to render
 * "Mesa No Encontrada" for a lost 200 body. The menu route — the surface a QR
 * actually lands on — accepted ANY fulfilled settlement, so an empty payload
 * set tableData falsy with tableLoadError null and fell through to the hard
 * 404 branch. Lock the classified path in.
 */
describe("guest menu transient table miss (#682)", () => {
  const source = fs.readFileSync(path.resolve(__dirname, "page.tsx"), "utf8");

  it("classifies the table settlement instead of blind-accepting a fulfilled one", () => {
    expect(source).toContain("resolveGuestTableSettlement");
    // The old shape: any fulfilled settlement was applied as table data.
    expect(source).not.toMatch(
      /tableResponse\.status === "fulfilled"[\s\S]{0,80}setTableData\(tableResponse\.value\)/,
    );
  });

  it("never derives the guest-facing error from anything but the classifier", () => {
    // classifyTableLoadFailure now lives behind resolveGuestTableSettlement, so
    // the page must not re-implement a second not_found path.
    expect(source).not.toContain("setTableLoadError(classifyTableLoadFailure(");
    expect(source).not.toContain('setTableLoadError("not_found")');
  });

  it("keeps the retryable copy split driven by tableLoadError", () => {
    expect(source).toMatch(/tableLoadError === "network"/);
    expect(source).toContain('t("errors.tableNotFound")');
    expect(source).toContain('t("errors.networkError")');
  });
});
