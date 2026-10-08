"use client";

import React from "react";
import { LayoutGrid, List as ListIcon, Search } from "lucide-react";
import { PremiumPanel } from "../premium";
import { btnGhostIcon, btnGhostIconActive } from "@/components/ui/buttonStyles";

interface ToolbarSearch {
  value: string;
  onChange: (value: string) => void;
  /** Required — doubles as the input's accessible-name fallback, so the field can never render nameless. */
  placeholder: string;
  /** Accessible name when the placeholder isn't enough (defaults to placeholder). */
  ariaLabel?: string;
}

interface ToolbarProps {
  search: ToolbarSearch;
  /** Right slot — filter Selects, ghost-icon buttons, the ViewToggle. */
  children?: React.ReactNode;
  className?: string;
}

/**
 * The one search strip for list-management tabs (Menu, Tables, Team, Plugins).
 * A textureless PremiumPanel holding a leading-icon search input on the left and
 * a caller-owned right slot for filters/actions. Replaces the four bespoke
 * toolbars the cohesion audit found (each with its own border/height/icon).
 */
export default function Toolbar({
  search,
  children,
  className = "",
}: ToolbarProps) {
  return (
    <PremiumPanel
      withTexture={false}
      className={`flex flex-wrap items-center gap-2 p-2 ${className}`.trim()}
    >
      <div className="relative min-w-0 flex-1">
        <Search
          className="pointer-events-none absolute left-3 top-1/2 h-4 w-4 -translate-y-1/2 text-ink-400"
          aria-hidden="true"
        />
        <input
          type="text"
          value={search.value}
          onChange={(event) => search.onChange(event.target.value)}
          placeholder={search.placeholder}
          aria-label={search.ariaLabel ?? search.placeholder}
          className="h-10 w-full rounded-xl border border-warm-200 bg-white px-3 pl-9 text-sm text-ink-900 outline-none transition focus:border-brand focus:ring-2 focus:ring-brand/20"
        />
      </div>
      {children ? (
        <div className="flex flex-wrap items-center gap-2">{children}</div>
      ) : null}
    </PremiumPanel>
  );
}

type ViewMode = "grid" | "list";

export interface ViewToggleProps {
  value: ViewMode;
  onChange: (value: ViewMode) => void;
  /** Accessible names for each option (i18n). Defaults to English. */
  labels?: { grid?: string; list?: string; group?: string };
}

/**
 * Grid/list switch. A labelled `role="group"` of plain buttons with
 * `aria-pressed` — the established toggle-button pattern. Radio semantics
 * would promise roving tabindex + arrow-key navigation that two independent
 * buttons don't deliver; aria-pressed on a two-state view switch announces
 * exactly what's rendered: two tabbable toggles where one is pressed.
 */
export function ViewToggle({ value, onChange, labels }: ViewToggleProps) {
  const gridLabel = labels?.grid ?? "Grid view";
  const listLabel = labels?.list ?? "List view";
  const options: Array<{
    mode: ViewMode;
    label: string;
    Icon: typeof LayoutGrid;
  }> = [
    { mode: "grid", label: gridLabel, Icon: LayoutGrid },
    { mode: "list", label: listLabel, Icon: ListIcon },
  ];

  return (
    <div
      role="group"
      aria-label={labels?.group ?? "View"}
      className="flex items-center gap-1"
    >
      {options.map(({ mode, label, Icon }) => {
        const active = value === mode;
        return (
          <button
            key={mode}
            type="button"
            aria-pressed={active}
            aria-label={label}
            title={label}
            onClick={() => onChange(mode)}
            className={active ? btnGhostIconActive : btnGhostIcon}
          >
            <Icon className="h-4 w-4" aria-hidden="true" />
          </button>
        );
      })}
    </div>
  );
}
