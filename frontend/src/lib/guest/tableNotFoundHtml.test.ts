import { guestTableNotFoundHtml } from "./tableNotFoundHtml";

describe("guestTableNotFoundHtml", () => {
  it("uses table-specific copy, not storefront custom-URL copy", () => {
    const html = guestTableNotFoundHtml("en", "Table | Payverge");
    expect(html).toContain("We couldn't find that table");
    expect(html).toContain("table code isn't active");
    expect(html).not.toMatch(/custom URL/i);
    expect(html).not.toMatch(/couldn't find a business/i);
    expect(html).toContain("__PAYVERGE_BUILD__");
    expect(html).toContain("noindex");
    expect(html).toContain("<title>Table | Payverge</title>");
  });

  it("uses Spanish table copy for es", () => {
    const html = guestTableNotFoundHtml("es", "Mesa | Payverge");
    expect(html).toContain("No encontramos esa mesa");
    expect(html).not.toMatch(/URL personalizada|custom URL/i);
    expect(html).toContain("__PAYVERGE_BUILD__");
  });
});
