import {
  displayReceiptType,
  isArgentinaLetterType,
  normalizeFiscalCountry,
  receiptHistoryColumnKey,
  receiptTypeFilterOptions,
  receiptTypeLabelKey,
} from "./receiptTypes";

describe("receiptTypes catalogue", () => {
  it("normalizes country codes", () => {
    expect(normalizeFiscalCountry(" us ")).toBe("US");
    expect(normalizeFiscalCountry(undefined)).toBe("");
  });

  it("returns AFIP letters only for AR", () => {
    const keys = receiptTypeFilterOptions("AR").map((o) => o.key);
    expect(keys).toEqual(["", "factura_a", "factura_b", "factura_c"]);
  });

  it("returns invoice/receipt for US and unknown countries", () => {
    expect(receiptTypeFilterOptions("US").map((o) => o.key)).toEqual([
      "",
      "invoice",
      "receipt",
    ]);
    expect(receiptTypeFilterOptions("").map((o) => o.key)).toEqual([
      "",
      "invoice",
      "receipt",
    ]);
  });

  it("returns standard_invoice for AE", () => {
    expect(receiptTypeFilterOptions("AE").map((o) => o.key)).toEqual([
      "",
      "standard_invoice",
    ]);
  });

  it("detects Argentina letter types", () => {
    expect(isArgentinaLetterType("factura_b")).toBe(true);
    expect(isArgentinaLetterType("nota_de_credito_a")).toBe(true);
    expect(isArgentinaLetterType("invoice")).toBe(false);
    expect(isArgentinaLetterType("receipt")).toBe(false);
  });

  it("builds label keys", () => {
    expect(receiptTypeLabelKey("invoice")).toBe("invoices.receiptType.invoice");
    expect(receiptTypeLabelKey("")).toBe("invoices.receiptType.unknown");
  });

  it("uses Factura column only for AR venues", () => {
    expect(receiptHistoryColumnKey("AR")).toBe("fiscal.history.columns.receipt");
    expect(receiptHistoryColumnKey("US")).toBe("fiscal.labels.invoice");
    expect(receiptHistoryColumnKey("ae")).toBe("fiscal.labels.invoice");
  });

  it("remaps leftover AFIP letters for US venues", () => {
    expect(displayReceiptType("US", "factura_a")).toBe("invoice");
    expect(displayReceiptType("US", "factura_b")).toBe("receipt");
    expect(displayReceiptType("US", "factura_c")).toBe("receipt");
    expect(displayReceiptType("AR", "factura_b")).toBe("factura_b");
    expect(displayReceiptType("US", "invoice")).toBe("invoice");
    expect(displayReceiptType("AE", "factura_a")).toBe("standard_invoice");
  });
});

describe("US catalogue i18n must not reuse AFIP Factura/Recibo words", () => {
  const esFiscal = require("@/i18n/messages/es/fiscal.json") as {
    labels: Record<string, string>;
  };
  const esDashboard = require("@/i18n/messages/es/businessDashboard.json") as {
    accountingDashboard: {
      invoices: { receiptType: Record<string, string> };
    };
  };

  it("keeps US invoice/receipt labels as Invoice/Receipt in Spanish", () => {
    expect(esFiscal.labels.invoice).toBe("Invoice");
    expect(esFiscal.labels.receipt).toBe("Receipt");
    expect(esDashboard.accountingDashboard.invoices.receiptType.invoice).toBe(
      "Invoice",
    );
    expect(esDashboard.accountingDashboard.invoices.receiptType.receipt).toBe(
      "Receipt",
    );
    expect(esFiscal.labels.factura_b).toBe("Factura B");
    expect(
      esDashboard.accountingDashboard.invoices.receiptType.factura_b,
    ).toBe("Factura B");
  });
});
