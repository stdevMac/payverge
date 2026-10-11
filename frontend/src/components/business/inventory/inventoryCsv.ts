import { InventoryItem } from "@/api/inventory";
import { itemStatus, selectItemValue } from "./inventorySelectors";

const HEADERS_EN = [
  "Name",
  "SKU",
  "Category",
  "Unit",
  "On hand",
  "Reorder at",
  "Unit cost",
  "Value",
  "Status",
];

const HEADERS_ES = [
  "Nombre",
  "SKU",
  "Categoría",
  "Unidad",
  "En stock",
  "Punto de reposición",
  "Costo unitario",
  "Valor",
  "Estado",
];

/** Characters that spreadsheets (Excel/Sheets) interpret as formula prefixes. */
const FORMULA_PREFIX_RE = /^[=+\-@\t\r]/;

function cell(value: string | number): string {
  const s = String(value ?? "");
  // Neutralize CSV/formula injection: prefix a leading apostrophe so spreadsheets
  // treat the value as text rather than evaluating it as a formula.
  const safe = FORMULA_PREFIX_RE.test(s) ? `'${s}` : s;
  if (/[",\n]/.test(safe)) {
    return `"${safe.replace(/"/g, '""')}"`;
  }
  return safe;
}

export interface BuildCsvOptions {
  /** Maps an item's status to a human label (localized by the caller). */
  statusLabel: (status: ReturnType<typeof itemStatus>) => string;
  /** Operator locale (es / es-AR → Spanish headers). */
  locale?: string;
}

function inventoryCsvHeaders(locale?: string): string[] {
  const lang = (locale || "en").toLowerCase();
  if (lang === "es" || lang.startsWith("es-") || lang.startsWith("es_")) {
    return HEADERS_ES;
  }
  return HEADERS_EN;
}

export function buildInventoryCsv(
  items: InventoryItem[],
  opts: BuildCsvOptions,
): string {
  const headers = inventoryCsvHeaders(opts.locale);
  const rows = items.map((it) =>
    [
      cell(it.name),
      cell(it.sku || ""),
      cell(it.category || ""),
      cell(it.unit),
      cell(it.current_quantity),
      cell(it.reorder_threshold),
      cell(it.cost_per_unit.toFixed(2)),
      cell(selectItemValue(it).toFixed(2)),
      cell(opts.statusLabel(itemStatus(it))),
    ].join(","),
  );
  return [headers.join(","), ...rows].join("\n") + "\n";
}

/** Triggers a client-side download of `csv` as `filename`. Browser-only. */
export function downloadCsv(filename: string, csv: string): void {
  const blob = new Blob([csv], { type: "text/csv;charset=utf-8;" });
  const url = URL.createObjectURL(blob);
  const link = document.createElement("a");
  link.href = url;
  link.download = filename;
  document.body.appendChild(link);
  link.click();
  document.body.removeChild(link);
  URL.revokeObjectURL(url);
}
