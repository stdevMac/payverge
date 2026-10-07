// Table/counter display names are stored WITH the type word embedded
// ("Table 1", "Counter 1") for demo and default-provisioned businesses, but
// operators can rename a table to a bare custom label ("Patio A", "Bar 3").
// Several render sites prepend the type word again, producing "Table Table 1".
//
// L1-25 + PG-29: English seed names of the form `<EnglishTypeWord> <n>` are
// parsed into typed tokens and re-rendered with the active-locale label, so
// Spanish guests see "Mesa 1" (not English "Table 1") while English operators
// still see "Table 1" without double-prefixing ("Table Table 1").

const ENGLISH_TYPE_WORDS = ["table", "counter"] as const;

/** Match demo/default seed names: "Table 9", "Counter 3" (digits only). */
const ENGLISH_SEED_RE = /^(table|counter)\s+(\d+)$/i;

// formatEntityName behaviours (locked by tableLabel.test.ts):
//   formatEntityName("Table", "Table 1") -> "Table 1"   (no double prefix / L1-25)
//   formatEntityName("Mesa", "Table 9")  -> "Mesa 9"    (localize seed / PG-29)
//   formatEntityName("Table", "Patio A") -> "Table Patio A" (orientation kept)
//   formatEntityName("Mesa", "Patio A")  -> "Mesa Patio A"  (bare name prefixed)
//   formatEntityName("Mesa", "Tablecloth Corner") -> "Mesa Tablecloth Corner"
//     (custom name that merely starts with "table" as a longer word)
export function formatEntityName(label: string, name: string): string {
  const trimmed = name.trim();
  if (!trimmed) return label;
  const labelTrim = label.trim();
  const lower = trimmed.toLowerCase();
  if (lower.startsWith(labelTrim.toLowerCase())) {
    // Already starts with the active label — do not double-prefix.
    return trimmed;
  }

  const seed = ENGLISH_SEED_RE.exec(trimmed);
  if (seed) {
    // Localize the English type word; keep the number.
    return `${labelTrim} ${seed[2]}`;
  }

  // Bare English type word without a number ("Table") → just the locale label.
  for (const word of ENGLISH_TYPE_WORDS) {
    if (lower === word) {
      return labelTrim;
    }
  }

  return `${labelTrim} ${trimmed}`;
}

// Minimal shape of a bill row needed to derive its location label. Both the
// detailed (nested `table`) and list (`table_name`) projections are supported.
export interface BillLocationFields {
  table_id?: number;
  counter_id?: number;
  table_name?: string;
  table?: { name?: string | null } | null;
}

// Labels the operator UI passes in (already translated).
export interface BillLocationLabels {
  table: string;
  counter: string;
  delivery: string;
}

// Derives a human location label for a bill. Mirrors the branching used across
// the bills list and detail views: a named table wins; table_id === 0 is the
// "no table" sentinel, where a counter_id means a counter bill and everything
// else is a delivery order. Never surfaces the raw "Table 0".
export function getBillLocationLabel(
  bill: BillLocationFields,
  labels: BillLocationLabels,
): string {
  const name = bill.table?.name || bill.table_name;
  if (name) return formatEntityName(labels.table, name);
  if (!bill.table_id) {
    return bill.counter_id ? labels.counter : labels.delivery;
  }
  return `${labels.table} ${bill.table_id}`;
}
