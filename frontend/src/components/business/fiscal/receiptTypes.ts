/**
 * Country-aware fiscal receipt-type catalogue.
 *
 * AFIP letter types (factura_a/b/c) are Argentina-only. US / other non-AFIP
 * locales use invoice vs receipt. UAE uses standard_invoice (EIS readiness).
 */

export type ReceiptTypeOption = {
  key: string;
  labelKey: string;
};

const ALL_LETTERS: ReceiptTypeOption = {
  key: "",
  labelKey: "invoices.receiptType.all",
};

const ALL_TYPES: ReceiptTypeOption = {
  key: "",
  labelKey: "invoices.receiptType.allTypes",
};

const AR_OPTIONS: ReceiptTypeOption[] = [
  ALL_LETTERS,
  { key: "factura_a", labelKey: "invoices.receiptType.factura_a" },
  { key: "factura_b", labelKey: "invoices.receiptType.factura_b" },
  { key: "factura_c", labelKey: "invoices.receiptType.factura_c" },
];

const AE_OPTIONS: ReceiptTypeOption[] = [
  ALL_TYPES,
  {
    key: "standard_invoice",
    labelKey: "invoices.receiptType.standard_invoice",
  },
];

const US_OPTIONS: ReceiptTypeOption[] = [
  ALL_TYPES,
  { key: "invoice", labelKey: "invoices.receiptType.invoice" },
  { key: "receipt", labelKey: "invoices.receiptType.receipt" },
];

/** Normalize a fiscal country code for catalogue lookup. */
export function normalizeFiscalCountry(
  country: string | null | undefined,
): string {
  return (country || "").trim().toUpperCase();
}

/** Filter options for the Invoices type dropdown, keyed by fiscal country. */
export function receiptTypeFilterOptions(
  country: string | null | undefined,
): ReceiptTypeOption[] {
  switch (normalizeFiscalCountry(country)) {
    case "AR":
      return AR_OPTIONS;
    case "AE":
      return AE_OPTIONS;
    default:
      return US_OPTIONS;
  }
}

/** True when the type is an AFIP A/B/C letter (incl. credit-note variants). */
export function isArgentinaLetterType(receiptType: string): boolean {
  const rt = (receiptType || "").toLowerCase();
  if (!rt) return false;
  if (rt.includes("factura_a") || rt.includes("factura_b") || rt.includes("factura_c")) {
    return true;
  }
  if (rt.includes("credito") || rt.includes("credit") || rt.startsWith("nc")) {
    return (
      rt.endsWith("_a") ||
      rt.endsWith("_b") ||
      rt.endsWith("_c") ||
      rt.includes("nota_de_credito_a") ||
      rt.includes("nota_de_credito_b") ||
      rt.includes("nota_de_credito_c")
    );
  }
  return rt === "a" || rt === "b" || rt === "c";
}

/**
 * i18n key for a stored receipt_type, falling back to a generic slug key.
 * Callers should fall back to a humanized slug when the key is missing.
 */
export function receiptTypeLabelKey(receiptType: string): string {
  const rt = (receiptType || "").toLowerCase().trim();
  if (!rt) return "invoices.receiptType.unknown";
  return `invoices.receiptType.${rt}`;
}

/**
 * Map leftover AFIP letter types onto the venue catalogue so a US Invoices
 * tab never labels rows as Factura A/B/C (#255). AR rows are unchanged.
 */
export function displayReceiptType(
  country: string | null | undefined,
  receiptType: string,
): string {
  const cc = normalizeFiscalCountry(country);
  if (cc === "AR" || !isArgentinaLetterType(receiptType)) {
    return receiptType;
  }
  if (cc === "AE") return "standard_invoice";
  const rt = (receiptType || "").toLowerCase();
  if (
    rt.includes("factura_a") ||
    rt.endsWith("_a") ||
    rt === "a"
  ) {
    return "invoice";
  }
  return "receipt";
}

/**
 * History-table column key for the document type. AR venues keep Factura;
 * US / other venues use the Invoice label even in es/es-AR (#650).
 */
export function receiptHistoryColumnKey(
  country: string | null | undefined,
): string {
  return normalizeFiscalCountry(country) === "AR"
    ? "fiscal.history.columns.receipt"
    : "fiscal.labels.invoice";
}
