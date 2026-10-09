"use client";

import React from "react";

export interface SlashPaletteProps {
  open: boolean;
  items: string[];
  onSelect: (item: string) => void;
  onClose: () => void;
  /** Localized listbox accessible name (parent owns i18n). */
  ariaLabel?: string;
  /**
   * Scope keyboard + outside-click to this container (composer root).
   * When omitted, falls back to document (tests). L4-12d.
   */
  containerRef?: React.RefObject<HTMLElement | null>;
}

/**
 * SlashPalette — popover surfaced above the composer when the operator
 * types "/" at the start of the input. Renders a vertical listbox of
 * suggested prompts (quick prompts + recent thread titles).
 *
 * Pure controlled component: parent owns `open`, the candidate `items`,
 * and reacts to `onSelect` / `onClose`. Keyboard contract:
 *
 *   - Escape    → onClose
 *   - ArrowDown → move active index forward (clamped)
 *   - ArrowUp   → move active index backward (clamped)
 *   - Enter     → onSelect(items[active])
 *
 * Keyboard listeners attach to `containerRef` (composer) when provided so
 * they do not capture document-level keys outside the chat column (L4-12d).
 */
export default function SlashPalette({
  open,
  items,
  onSelect,
  onClose,
  ariaLabel,
  containerRef,
}: SlashPaletteProps) {
  const [active, setActive] = React.useState(0);
  const listRef = React.useRef<HTMLDivElement | null>(null);

  React.useEffect(() => {
    if (!open) return;
    setActive(0);
  }, [open, items]);

  React.useEffect(() => {
    if (!open) return;
    const target: EventTarget | null =
      containerRef?.current ?? (typeof document !== "undefined" ? document : null);
    if (!target) return;

    const handleKey = (e: Event) => {
      const ke = e as KeyboardEvent;
      if (ke.key === "Escape") {
        ke.preventDefault();
        ke.stopPropagation();
        onClose();
        return;
      }
      if (ke.key === "ArrowDown") {
        ke.preventDefault();
        setActive((i) => Math.min(i + 1, Math.max(items.length - 1, 0)));
        return;
      }
      if (ke.key === "ArrowUp") {
        ke.preventDefault();
        setActive((i) => Math.max(i - 1, 0));
        return;
      }
      if (ke.key === "Enter") {
        // ⌘↵ / ⌃↵ is the composer's "send now" binding, not a palette select.
        // Intercepting it here fired BOTH actions: the item was inserted AND
        // the raw "/token" draft was sent. Let modified Enter fall through.
        if (ke.metaKey || ke.ctrlKey || ke.altKey) return;
        // Only intercept Enter when palette is open and a choice exists.
        const choice = items[active];
        if (choice) {
          ke.preventDefault();
          onSelect(choice);
        }
      }
    };

    const handlePointerDown = (e: Event) => {
      const pe = e as PointerEvent;
      const root = containerRef?.current;
      const list = listRef.current;
      const node = pe.target as Node | null;
      if (!node) return;
      // Outside both list and composer → close.
      if (root && !root.contains(node) && list && !list.contains(node)) {
        onClose();
      } else if (!root && list && !list.contains(node)) {
        onClose();
      }
    };

    target.addEventListener("keydown", handleKey);
    document.addEventListener("pointerdown", handlePointerDown, true);
    return () => {
      target.removeEventListener("keydown", handleKey);
      document.removeEventListener("pointerdown", handlePointerDown, true);
    };
  }, [open, items, active, onSelect, onClose, containerRef]);

  if (!open) return null;

  return (
    <div
      ref={listRef}
      role="listbox"
      aria-label={ariaLabel ?? "Slash command palette"}
      data-testid="dc-slash-palette"
      className="absolute bottom-full mb-2 left-0 right-0 max-h-72 overflow-y-auto rounded-lg border border-warm-200 bg-white shadow-lg z-10"
    >
      {items.map((item, idx) => (
        <button
          key={`${item}-${idx}`}
          type="button"
          role="option"
          aria-selected={idx === active}
          onMouseEnter={() => setActive(idx)}
          onClick={() => onSelect(item)}
          className={`w-full text-left px-3 py-2 text-body-sm ${
            idx === active
              ? "bg-brand/5 text-ink-900"
              : "text-ink-700 hover:bg-warm-50"
          }`}
        >
          {item}
        </button>
      ))}
    </div>
  );
}
