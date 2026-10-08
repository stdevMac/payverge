"use client";

import React, { useMemo } from "react";
import { X } from "lucide-react";
import { ensureContrastWithWhiteText } from "./themedColor";

/**
 * Storefront dietary filter toggles (storefront plan 3.7) — ports the QR table
 * menu's MenuFilterToggles capability (/t/[tableCode]/menu) to the public
 * business page. The QR component itself is not reused because its selected
 * state is hardcoded to the Payverge `brand` teal; the storefront must honor
 * the merchant's design_settings primary color.
 *
 * Filter ids, item fields (item.dietary_tags), and OR-within-group semantics
 * mirror the QR menu exactly.
 */

/** Dietary filter ids in canonical display order (QR menu parity). */
const STOREFRONT_DIETARY_FILTERS = [
  "vegetarian",
  "vegan",
  "gluten-free",
  "dairy-free",
  "nut-free",
] as const;

/**
 * Collect the dietary filters carried by at least one item, in canonical
 * order. An empty result means the menu has no filterable flags and the
 * toggle row should not render at all.
 */
export function collectDietaryFiltersFromItems(
  items: ReadonlyArray<{ dietary_tags?: string[] | null }>,
): string[] {
  const present = new Set<string>();
  items.forEach((item) => {
    STOREFRONT_DIETARY_FILTERS.forEach((filter) => {
      if (item.dietary_tags?.includes(filter)) present.add(filter);
    });
  });
  return STOREFRONT_DIETARY_FILTERS.filter((filter) => present.has(filter));
}

/**
 * Item predicate — QR parity: with at least one selected dietary filter an
 * item matches when its dietary_tags include ANY selected filter (OR within
 * the dietary group).
 */
export function itemMatchesDietaryFilters(
  dietaryTags: string[] | null | undefined,
  selected: ReadonlySet<string>,
): boolean {
  if (selected.size === 0) return true;
  return Array.from(selected).some((filter) => dietaryTags?.includes(filter));
}

/**
 * Bundle predicate — QR parity: a bundle satisfies a dietary filter only when
 * EVERY resolved child item carries that tag (a bundle is vegan only if all
 * its components are). Multiple selected filters still OR within the group.
 */
export function bundleMatchesDietaryFilters(
  childDietaryTags: ReadonlyArray<ReadonlyArray<string>>,
  selected: ReadonlySet<string>,
): boolean {
  if (selected.size === 0) return true;
  return Array.from(selected).some(
    (filter) =>
      childDietaryTags.length > 0 &&
      childDietaryTags.every((tags) => tags.includes(filter)),
  );
}

interface StorefrontMenuFiltersProps {
  filters: string[];
  selected: ReadonlySet<string>;
  /** Merchant design_settings.primary_color for the selected state. */
  primaryColor: string;
  getLabel: (filter: string) => string;
  onToggle: (filter: string) => void;
  groupLabel: string;
}

function StorefrontMenuFilters({
  filters,
  selected,
  primaryColor,
  getLabel,
  onToggle,
  groupLabel,
}: StorefrontMenuFiltersProps) {
  const solid = useMemo(
    () => ensureContrastWithWhiteText(primaryColor),
    [primaryColor],
  );

  // Gate: render nothing when the menu carries no filterable flags.
  if (filters.length === 0) return null;

  return (
    <div
      className="mb-6 flex flex-wrap items-center gap-2"
      role="group"
      aria-label={groupLabel}
      data-testid="storefront-menu-filters"
    >
      {filters.map((filter) => {
        const isSelected = selected.has(filter);
        return (
          <button
            key={filter}
            type="button"
            aria-pressed={isSelected}
            onClick={() => onToggle(filter)}
            className={`inline-flex min-h-9 items-center gap-1.5 rounded-full border px-3 py-1.5 text-sm font-medium transition-colors focus:outline-none focus-visible:ring-2 focus-visible:ring-offset-2 ${
              isSelected
                ? "text-white"
                : "border-warm-200 bg-white text-ink-600 hover:border-ink-300 hover:text-ink-900"
            }`}
            style={
              isSelected
                ? ({
                    backgroundColor: solid,
                    borderColor: solid,
                    "--tw-ring-color": solid,
                  } as React.CSSProperties)
                : undefined
            }
          >
            {getLabel(filter)}
            {isSelected ? (
              <X className="h-3.5 w-3.5 opacity-90" strokeWidth={2} aria-hidden />
            ) : null}
          </button>
        );
      })}
    </div>
  );
}

export default StorefrontMenuFilters;
