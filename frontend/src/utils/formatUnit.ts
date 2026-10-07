type UnitNoun = {
  singular: string;
  plural: string;
};

type UnitTranslations = Record<string, UnitNoun>;

// Symbol units never pluralize.
const SYMBOL_UNITS = new Set(["g", "kg", "ml"]);

const TRANSLATIONS: Record<string, UnitTranslations> = {
  en: {
    unit: { singular: "unit", plural: "units" },
    liter: { singular: "liter", plural: "liters" },
    box: { singular: "box", plural: "boxes" },
  },
  es: {
    unit: { singular: "unidad", plural: "unidades" },
    liter: { singular: "litro", plural: "litros" },
    box: { singular: "caja", plural: "cajas" },
  },
};

function pickLocale(locale: string): string {
  // Accept "en", "en-US", "es-ES", "es-MX", etc.
  const short = (locale || "").toLowerCase().split(/[-_]/)[0];
  return short in TRANSLATIONS ? short : "en";
}

/**
 * Formats a quantity the way the inventory UI elsewhere does it: integers
 * print bare ("3 units"), non-integers print with two locale decimals
 * ("1.50 units" / "1,50 units"). Mirrors InventoryManager's local
 * formatQuantity helper so Warnings/Items/Reorder match Movements.
 *
 * S-4: never use toFixed(2) — that hard-codes en-US decimals on es operator UI
 * (live: "Disponible actual: -0.35 kg" under Spanish locale).
 */
export function formatCount(count: number, locale: string = "en"): string {
  if (Number.isInteger(count)) {
    return String(count);
  }
  return new Intl.NumberFormat(locale || "en", {
    minimumFractionDigits: 2,
    maximumFractionDigits: 2,
  }).format(count);
}

/**
 * Formats a count + unit pair for display.
 * - Symbol units (g, kg, ml) skip pluralization and translation.
 * - Word units (unit, liter, box) pick singular/plural per locale.
 * - Unknown units (admin-created free-form values) pass through verbatim.
 * - Accepts BCP-47 (`en-US`) or short (`en`) locale tags.
 */
export function formatUnit(
  count: number,
  unitKey: string,
  locale: string,
): string {
  const display = formatCount(count, locale);

  if (SYMBOL_UNITS.has(unitKey)) {
    return `${display} ${unitKey}`;
  }

  const table = TRANSLATIONS[pickLocale(locale)];
  const noun = table[unitKey];
  if (!noun) {
    // Unknown unit (admin-created free-form value) — pass through verbatim.
    return `${display} ${unitKey}`;
  }

  return `${display} ${count === 1 ? noun.singular : noun.plural}`;
}
