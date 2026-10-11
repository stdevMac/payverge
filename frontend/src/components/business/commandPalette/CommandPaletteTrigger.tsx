"use client";

import React from "react";
import { Search } from "lucide-react";
import {
  useSimpleLocale,
  getTranslation,
} from "@/i18n/SimpleTranslationProvider";
import { useCommandPalette } from "./CommandPaletteProvider";

/**
 * Discoverable affordance for the ⌘K palette — a quiet "Search…" pill that
 * lives in the dashboard header. Shows the platform-correct hotkey hint
 * (⌘K on Mac, Ctrl K elsewhere) once mounted on the client.
 */
interface CommandPaletteTriggerProps {
  className?: string;
  /**
   * Full-width sidebar layout: stretches edge-to-edge with the label and
   * hotkey always visible (vs. the compact, responsive header pill).
   */
  fullWidth?: boolean;
  /**
   * Icon-only: hide the label + hotkey at all widths. Used by the collapsed
   * sidebar icon rail, where there is no room for text. Mutually exclusive
   * with fullWidth; iconOnly wins (no full-width stretch).
   */
  iconOnly?: boolean;
}

export default function CommandPaletteTrigger({
  className = "",
  fullWidth = false,
  iconOnly = false,
}: CommandPaletteTriggerProps) {
  const { openPalette, ready } = useCommandPalette();
  const { locale } = useSimpleLocale();

  const label = (() => {
    const result = getTranslation(
      "businessDashboard.commandPalette.search",
      locale,
    );
    return Array.isArray(result) ? result[0] || "Search" : (result as string);
  })();

  const isMac =
    ready &&
    typeof navigator !== "undefined" &&
    /mac|iphone|ipad|ipod/i.test(
      navigator.platform || navigator.userAgent || "",
    );

  const labelClass = iconOnly
    ? "hidden"
    : fullWidth
      ? "inline"
      : "hidden sm:inline";
  const kbdClass = iconOnly
    ? "hidden"
    : fullWidth
      ? "inline-flex"
      : "hidden sm:inline-flex";

  // `iconOnly` fully wins over `fullWidth`: an icon-only rail button must not
  // also stretch edge-to-edge.
  const effectiveFullWidth = fullWidth && !iconOnly;

  return (
    <button
      type="button"
      onClick={openPalette}
      aria-label={label}
      aria-keyshortcuts="Meta+K Control+K"
      className={`group inline-flex items-center gap-2 rounded-xl border border-warm-200 bg-warm-50 px-3 py-1.5 text-body-sm text-ink-500 transition-colors hover:border-warm-300 hover:bg-warm-100 hover:text-ink-600 focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-brand ${
        effectiveFullWidth ? "w-full justify-between" : ""
      } ${className}`}
    >
      <span className="flex items-center gap-2 min-w-0">
        <Search className="w-4 h-4 shrink-0" aria-hidden="true" />
        <span className={`truncate ${labelClass}`}>{label}</span>
      </span>
      <kbd
        className={`ml-1 items-center gap-0.5 rounded-md border border-warm-300 bg-warm-100 px-1.5 py-0.5 text-[11px] font-medium text-ink-500 ${kbdClass}`}
        aria-hidden="true"
      >
        {ready ? (isMac ? "⌘" : "Ctrl") : "⌘"}K
      </kbd>
    </button>
  );
}
