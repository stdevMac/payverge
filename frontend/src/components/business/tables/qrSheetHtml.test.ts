import { buildQrSheetHtml, escapeHtml } from "./qrSheetHtml";

const strings = {
  scanCaption: "Scan to view the menu & order",
  documentTitle: "Trattoria — QR codes",
  poweredBy: "Powered by Payverge",
};

const card = (n: number) => ({
  tableName: `Table ${n}`,
  tableCode: `CODE${n}`,
  qrDataUrl: `data:image/png;base64,QR${n}`,
});

describe("buildQrSheetHtml (P2-16 print-all sheet)", () => {
  it("renders one card per table with business name, table label and short URL", () => {
    const html = buildQrSheetHtml("Trattoria Bella <Vista>", [card(1), card(2)], strings);
    expect(html.match(/class="card"/g)).toHaveLength(2);
    // Business name appears on every card, HTML-escaped.
    expect(html.match(/Trattoria Bella &lt;Vista&gt;/g)).toHaveLength(2);
    expect(html).toContain("Table 1");
    expect(html).toContain("/t/CODE1");
    expect(html).toContain('src="data:image/png;base64,QR1"');
    expect(html).toContain("Scan to view the menu &amp; order");
    expect(html).toContain("Powered by Payverge");
    expect(html).toContain('class="powered"');
  });

  it("carries printer-friendly pagination CSS: no mid-card breaks, 6 cards per page", () => {
    const html = buildQrSheetHtml("Biz", [card(1)], strings);
    expect(html).toContain("break-inside: avoid");
    expect(html).toContain("page-break-inside: avoid");
    expect(html).toContain(".card:nth-child(6n)");
    expect(html).toContain("page-break-after: always");
    expect(html).toContain("@page");
  });

  it("escapes hostile table names (guards the print iframe document)", () => {
    const html = buildQrSheetHtml("Biz", [
      { tableName: '<img onerror=x>', tableCode: 'SAFE"1', qrDataUrl: "data:image/png;base64,QR" },
    ], strings);
    expect(html).not.toContain("<img onerror");
    expect(html).toContain("&lt;img onerror=x&gt;");
    expect(html).toContain("SAFE&quot;1");
  });

  it("escapeHtml covers & < > \"", () => {
    expect(escapeHtml('a&b<c>"d"')).toBe("a&amp;b&lt;c&gt;&quot;d&quot;");
  });
});
