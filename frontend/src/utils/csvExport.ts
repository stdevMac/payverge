/** Characters that spreadsheets (Excel/Sheets) interpret as formula prefixes. */
const FORMULA_PREFIX_RE = /^[=+\-@\t\r]/;

function escapeCsvCell(value: unknown): string {
  if (value === null || value === undefined) return "";
  const str = String(value);
  // Neutralize CSV/formula injection: prefix a leading apostrophe so spreadsheets
  // treat the value as text rather than evaluating it as a formula.
  const safe = FORMULA_PREFIX_RE.test(str) ? `'${str}` : str;
  if (safe.includes(",") || safe.includes('"') || safe.includes("\n")) {
    return `"${safe.replace(/"/g, '""')}"`;
  }
  return safe;
}

export function rowsToCsv<Row extends Record<string, unknown>>(
  columns: { key: keyof Row; header: string }[],
  rows: Row[],
): string {
  const headerLine = columns.map((c) => escapeCsvCell(c.header)).join(",");
  const dataLines = rows.map((row) =>
    columns.map((c) => escapeCsvCell(row[c.key])).join(","),
  );
  return [headerLine, ...dataLines].join("\n");
}

export function downloadCsv(filename: string, csv: string): void {
  if (typeof window === "undefined") return;
  const blob = new Blob([csv], { type: "text/csv;charset=utf-8" });
  const url = URL.createObjectURL(blob);
  const link = document.createElement("a");
  link.href = url;
  link.download = filename;
  document.body.appendChild(link);
  link.click();
  document.body.removeChild(link);
  URL.revokeObjectURL(url);
}
