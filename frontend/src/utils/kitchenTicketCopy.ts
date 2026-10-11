import { getTranslation } from "@/i18n/getTranslation";
import type { Locale } from "@/i18n/config";
import { countKey } from "@/i18n/countForm";
import { allergenDisplayName } from "@/utils/allergenLabel";

export interface KitchenNoteLabels {
  additionalItems: string;
  initialOrder: string;
  table: string;
  counter: string;
}

/**
 * Guest checkout persists English chrome ("Table 1 - Additional Items").
 * Remap the known suffix, and — inside those chrome notes only — the English
 * `<Table|Counter> <digits>` seed prefix (demo/default table names). Renamed
 * tables ("Patio A"), Chopper notes, and freeform text keep their stored
 * identity untouched (#776).
 */
const GUEST_NOTE_CHROME = [
  { suffix: "Additional Items", kind: "additionalItems" as const },
  { suffix: "Initial Order", kind: "initialOrder" as const },
];

/** Same seed shape tableLabel.ts parses: English type word + digits only. */
const ENGLISH_SEED_PREFIX_RE = /^(table|counter)\s+(\d+)$/i;

export function kitchenAllergenLabel(token: string, locale: Locale): string {
  const translate = (key: string): string => {
    const result = getTranslation(key, locale);
    return Array.isArray(result) ? result[0] || key : result;
  };
  return allergenDisplayName(token, translate, locale);
}

export function kitchenNoteLabels(locale: Locale): KitchenNoteLabels {
  const read = (key: string): string => {
    const result = getTranslation(`kitchenDisplay.${key}`, locale);
    return Array.isArray(result) ? result[0] || key : result;
  };
  return {
    additionalItems: read("notes.additionalItems"),
    initialOrder: read("notes.initialOrder"),
    table: read("location.table"),
    counter: read("location.counter"),
  };
}

export function localizeKitchenOrderNote(
  note: string,
  labels: KitchenNoteLabels,
): string {
  const trimmed = note.trim();
  for (const { suffix, kind } of GUEST_NOTE_CHROME) {
    const needle = ` - ${suffix}`;
    if (
      trimmed.length <= needle.length ||
      !trimmed.toLowerCase().endsWith(needle.toLowerCase())
    ) {
      continue;
    }
    let prefix = trimmed.slice(0, trimmed.length - needle.length);
    const seed = ENGLISH_SEED_PREFIX_RE.exec(prefix.trim());
    if (seed) {
      const word =
        seed[1].toLowerCase() === "counter" ? labels.counter : labels.table;
      if (word && !word.includes(".")) {
        // Guard: a failed lookup returns the key path — keep English then.
        prefix = `${word} ${seed[2]}`;
      }
    }
    return `${prefix} - ${labels[kind]}`;
  }
  return note;
}

/**
 * "{count} items to prepare" with a proper singular form (#827). Operator
 * copy has no ICU plural engine; `countKey` picks `_one` vs `_other`.
 */
export function kitchenItemsToPrepareLabel(
  count: number,
  locale: Locale,
): string {
  const key = `businessDashboard.dashboard.kitchenManager.order.${countKey(
    "itemsToPrepare",
    count,
  )}`;
  const result = getTranslation(key, locale);
  const text = Array.isArray(result) ? result[0] || key : result;
  return text.replace("{count}", String(count));
}
