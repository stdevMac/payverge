"use client";

import React, { useRef } from "react";
import { Eye, Grid3X3, LayoutGrid, List } from "lucide-react";

export type MenuViewMode = "detailed" | "compact" | "grid" | "category-tabs";

const MENU_VIEW_OPTIONS: Array<{
  mode: MenuViewMode;
  icon: typeof List;
}> = [
  { mode: "detailed", icon: List },
  { mode: "compact", icon: LayoutGrid },
  { mode: "grid", icon: Grid3X3 },
  { mode: "category-tabs", icon: Eye },
];

function isRtl(node: HTMLElement): boolean {
  const owner = node.closest("[dir]");
  if (owner) return owner.getAttribute("dir")?.toLowerCase() === "rtl";
  if (typeof window !== "undefined" && window.getComputedStyle) {
    return window.getComputedStyle(node).direction === "rtl";
  }
  return false;
}

interface MenuViewRadiogroupProps {
  value: MenuViewMode;
  onChange: (mode: MenuViewMode) => void;
  label: string;
  optionLabels: Record<MenuViewMode, string>;
}

export default function MenuViewRadiogroup({
  value,
  onChange,
  label,
  optionLabels,
}: MenuViewRadiogroupProps) {
  const optionRefs = useRef<Array<HTMLButtonElement | null>>([]);

  const focusAndSelect = (index: number) => {
    const next = MENU_VIEW_OPTIONS[index];
    if (!next) return;
    onChange(next.mode);
    optionRefs.current[index]?.focus();
  };

  const handleKeyDown = (event: React.KeyboardEvent<HTMLDivElement>) => {
    const current = MENU_VIEW_OPTIONS.findIndex(
      (option) => option.mode === value,
    );
    const last = MENU_VIEW_OPTIONS.length - 1;
    // Under dir="rtl" the visual order is mirrored, so ArrowLeft moves forward.
    const rtl = isRtl(event.currentTarget);
    const forwardKey = rtl ? "ArrowLeft" : "ArrowRight";
    const backwardKey = rtl ? "ArrowRight" : "ArrowLeft";
    if (event.key === forwardKey || event.key === "ArrowDown") {
      event.preventDefault();
      focusAndSelect(
        current < 0 ? 0 : (current + 1) % MENU_VIEW_OPTIONS.length,
      );
      return;
    }
    if (event.key === backwardKey || event.key === "ArrowUp") {
      event.preventDefault();
      focusAndSelect(current <= 0 ? last : current - 1);
      return;
    }
    if (event.key === "Home") {
      event.preventDefault();
      focusAndSelect(0);
      return;
    }
    if (event.key === "End") {
      event.preventDefault();
      focusAndSelect(last);
    }
  };

  return (
    <div
      className="hidden md:flex items-center gap-1 rounded-xl border border-warm-200 bg-white p-1"
      role="radiogroup"
      aria-label={label}
      onKeyDown={handleKeyDown}
    >
      {MENU_VIEW_OPTIONS.map(({ mode, icon: Icon }, index) => {
        const optionLabel = optionLabels[mode];
        const checked = value === mode;
        return (
          <button
            key={mode}
            type="button"
            role="radio"
            aria-checked={checked}
            aria-label={optionLabel}
            title={optionLabel}
            tabIndex={checked ? 0 : -1}
            ref={(node) => {
              optionRefs.current[index] = node;
            }}
            onClick={() => onChange(mode)}
            className={`inline-flex h-8 min-w-8 items-center justify-center rounded-lg ${
              checked
                ? "bg-brand text-white"
                : "bg-transparent text-ink-500 hover:text-ink-900"
            }`}
          >
            <Icon className="h-4 w-4" strokeWidth={1.75} aria-hidden />
          </button>
        );
      })}
    </div>
  );
}
