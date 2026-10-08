import type { CsvColumnManifest } from "../exportLocalizedCsv";

export const RECEIPTS_CSV_COLUMNS: CsvColumnManifest[] = [
  { key: "bill_id", headerKey: "invoices.csv.billId" },
  { key: "receipt_type", headerKey: "invoices.csv.receiptType" },
  { key: "receipt_number", headerKey: "invoices.csv.receiptNumber" },
  { key: "amount", headerKey: "invoices.csv.amount" },
  { key: "tip", headerKey: "invoices.csv.tip" },
  { key: "currency", headerKey: "invoices.csv.currency" },
  { key: "status", headerKey: "invoices.csv.status" },
  { key: "needs_attention", headerKey: "invoices.csv.needsAttention" },
  { key: "issued_at", headerKey: "invoices.csv.issuedAt" },
  { key: "error", headerKey: "invoices.csv.error" },
];
