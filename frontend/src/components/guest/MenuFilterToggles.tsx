"use client";

import React from "react";
import { X } from "lucide-react";

export interface MenuFilterTogglesProps {
  filters: string[];
  selected: Set<string>;
  getLabel: (filter: string) => string;
  onToggle: (filter: string) => void;
  groupLabel?: string;
  id?: string;
}

/**
 * Accessible filter toggles for the guest table menu (GUEST-005).
 * Native buttons with aria-pressed — not clickable div/Chip shells.
 */
export function MenuFilterToggles({
  filters,
  selected,
  getLabel,
  onToggle,
  groupLabel = "Filters",
  id,
}: MenuFilterTogglesProps) {
  if (filters.length === 0) return null;

  return (
    <div
      id={id}
      className="mt-3 flex flex-wrap gap-2 rounded-xl border border-warm-200 bg-white p-3"
      role="group"
      aria-label={groupLabel}
      data-testid="menu-filter-toggles"
    >
      {filters.map((filter) => {
        const isSelected = selected.has(filter);
        return (
          <button
            key={filter}
            type="button"
            aria-pressed={isSelected}
            onClick={() => onToggle(filter)}
            className={`inline-flex min-h-9 items-center gap-1.5 rounded-full border px-3 py-1.5 text-sm font-medium transition-colors focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-brand focus-visible:ring-offset-2 ${
              isSelected
                ? "border-brand bg-brand text-white"
                : "border-warm-200 bg-white text-ink-600 hover:border-ink-300 hover:text-ink-900"
            }`}
          >
            {getLabel(filter)}
            {isSelected ? (
              <X
                className="h-3.5 w-3.5 opacity-90"
                strokeWidth={2}
                aria-hidden
              />
            ) : null}
          </button>
        );
      })}
    </div>
  );
}
