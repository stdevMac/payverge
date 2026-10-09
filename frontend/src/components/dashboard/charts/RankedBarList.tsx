// src/components/dashboard/charts/RankedBarList.tsx
import React from "react";
import { ANALYTICS_BAR_HUE } from "./analyticsTheme";

interface RankedBarItem {
  label: string;
  value: number;
  sublabel?: string;
}
interface RankedBarListProps {
  items: RankedBarItem[];
  formatValue?: (n: number) => string;
  max?: number;
  ariaLabel: string;
  /**
   * Optional, already-localized empty-state caption. When omitted we render a
   * locale-neutral em-dash placeholder so the list never collapses to a blank
   * gap (and we don't bake an untranslated English string into the UI).
   */
  emptyLabel?: string;
}

// Replaces doughnuts AND progress-bar lists: sorted horizontal bars with the
// label and value on the bar. Single hue; zero baseline; ranks correctly.
export function RankedBarList({ items, formatValue = (n) => String(n), max, ariaLabel, emptyLabel }: RankedBarListProps) {
  if (!items || items.length === 0) {
    return (
      <p
        role="status"
        aria-label={ariaLabel}
        className="py-10 text-center text-sm text-ink-500"
      >
        {emptyLabel ?? "—"}
      </p>
    );
  }
  const sorted = [...items].sort((a, b) => b.value - a.value);
  const peak = max ?? Math.max(0, ...sorted.map((i) => i.value));
  return (
    <ul role="list" aria-label={ariaLabel} className="space-y-2">
      {sorted.map((item) => {
        const pct = peak > 0 ? (item.value / peak) * 100 : 0;
        return (
          <li key={item.label} className="relative">
            <div className="flex items-center justify-between gap-3 px-1 py-1.5">
              <span className="z-10 truncate text-sm font-medium text-ink-800">
                {item.label}
                {item.sublabel && <span className="ml-2 text-xs text-ink-500">{item.sublabel}</span>}
              </span>
              <span className="z-10 text-sm font-semibold tabular-nums text-ink-900">
                {formatValue(item.value)}
              </span>
            </div>
            <div
              data-testid="ranked-bar-fill"
              className="absolute inset-y-0 left-0 rounded-md"
              style={{ width: `${pct}%`, backgroundColor: ANALYTICS_BAR_HUE, opacity: 0.12 }}
            />
          </li>
        );
      })}
    </ul>
  );
}
