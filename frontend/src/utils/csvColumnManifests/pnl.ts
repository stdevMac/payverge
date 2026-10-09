import type { CsvColumnManifest } from "../exportLocalizedCsv";

export const PNL_CSV_COLUMNS: CsvColumnManifest[] = [
  { key: "section", headerKey: "reports.csv.section" },
  { key: "label", headerKey: "reports.csv.label" },
  { key: "current", headerKey: "reports.csv.current" },
  { key: "previous", headerKey: "reports.csv.previous" },
  { key: "delta", headerKey: "reports.csv.delta" },
];
