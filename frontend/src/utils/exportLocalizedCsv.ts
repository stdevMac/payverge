/**
 * S-14 shared client CSV export: localized headers via column manifests,
 * formula-injection safe (delegates to rowsToCsv/downloadCsv).
 */
import { downloadCsv, rowsToCsv } from "./csvExport";

export type CsvColumnManifest<K extends string = string> = {
  key: K;
  /** i18n key path resolved by `t` */
  headerKey: string;
};

export type ExportLocalizedCsvArgs<
  Row extends Record<string, unknown>,
  K extends string = string,
> = {
  columns: CsvColumnManifest<K>[];
  rows: Row[];
  filename: string;
  t: (key: string) => string;
};

/**
 * Build a CSV from a column manifest (i18n header keys) + row objects and
 * trigger a browser download. Headers are resolved with `t` so en/es (and
 * es-AR via the operator provider) stay in sync with UI labels.
 */
export function exportLocalizedCsv<
  Row extends Record<string, unknown>,
  K extends string = string,
>({ columns, rows, filename, t }: ExportLocalizedCsvArgs<Row, K>): string {
  const resolved = columns.map((c) => ({
    key: c.key as keyof Row & string,
    header: t(c.headerKey),
  }));
  const csv = rowsToCsv(resolved, rows);
  downloadCsv(filename, csv);
  return csv;
}
