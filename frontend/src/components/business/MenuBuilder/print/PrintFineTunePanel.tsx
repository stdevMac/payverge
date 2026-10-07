"use client";

import React, { useEffect, useRef } from "react";
import { X } from "lucide-react";

interface PrintFineTuneSection {
  id: string;
  label: string;
  content: React.ReactNode;
}

export interface PrintFineTunePanelProps {
  isOpen: boolean;
  sections: PrintFineTuneSection[];
  tString: (key: string) => string;
  onClose: () => void;
  /** Exposed so the caller can move focus to a specific control inside. */
  panelRef?: React.RefObject<HTMLDivElement>;
}

const FOCUSABLE_SELECTOR = [
  "a[href]",
  "button:not([disabled])",
  "input:not([disabled])",
  "select:not([disabled])",
  "textarea:not([disabled])",
  '[tabindex]:not([tabindex="-1"])',
].join(",");

function focusableElements(container: HTMLElement): HTMLElement[] {
  return [
    ...container.querySelectorAll<HTMLElement>(FOCUSABLE_SELECTOR),
  ].filter((element) => element.getAttribute("aria-hidden") !== "true");
}

/**
 * One discreet surface for every advanced print control. It is a dialog, not a
 * settings dashboard: focus is trapped while open, Escape closes it, and focus
 * returns to whatever opened it.
 */
export function PrintFineTunePanel({
  isOpen,
  sections,
  tString,
  onClose,
  panelRef,
}: PrintFineTunePanelProps) {
  const fallbackRef = useRef<HTMLDivElement>(null);
  const containerRef = panelRef ?? fallbackRef;
  const closeButtonRef = useRef<HTMLButtonElement>(null);
  const restoreFocusRef = useRef<HTMLElement | null>(null);
  const onCloseRef = useRef(onClose);
  onCloseRef.current = onClose;

  useEffect(() => {
    if (!isOpen) return;
    const container = containerRef.current;
    restoreFocusRef.current =
      document.activeElement instanceof HTMLElement
        ? document.activeElement
        : null;
    closeButtonRef.current?.focus();

    // The listener is attached to the node rather than declared in JSX so the
    // dialog wrapper stays a plain landmark with no interactive semantics.
    const onKeyDown = (event: KeyboardEvent): void => {
      if (event.key === "Escape") {
        // Keeps Escape from reaching the surrounding modal, which would close
        // the whole print flow instead of this drawer.
        event.stopPropagation();
        onCloseRef.current();
        return;
      }
      if (event.key !== "Tab" || !container) return;
      const focusable = focusableElements(container);
      if (focusable.length === 0) return;
      const first = focusable[0];
      const last = focusable[focusable.length - 1];
      const active = document.activeElement;
      if (event.shiftKey && (active === first || active === container)) {
        event.preventDefault();
        last.focus();
      } else if (!event.shiftKey && active === last) {
        event.preventDefault();
        first.focus();
      }
    };
    container?.addEventListener("keydown", onKeyDown);

    return () => {
      container?.removeEventListener("keydown", onKeyDown);
      restoreFocusRef.current?.focus();
    };
    // `onClose` is read through a ref so a new inline callback on every parent
    // render cannot tear the trap down and yank focus mid-interaction.
  }, [containerRef, isOpen]);

  if (!isOpen) return null;

  return (
    <div
      data-testid="print-fine-tune-overlay"
      className="absolute inset-0 z-20 flex justify-end"
    >
      <button
        type="button"
        aria-label={tString("print.fineTune.close")}
        onClick={onClose}
        className="absolute inset-0 h-full w-full cursor-default bg-ink-950/20"
      />
      <div
        ref={containerRef}
        data-testid="print-fine-tune-panel"
        role="dialog"
        aria-modal="true"
        aria-label={tString("print.fineTune.title")}
        className="relative flex h-full w-full max-w-md flex-col border-s border-warm-200 bg-white shadow-xl shadow-ink-950/10"
      >
        <div className="flex items-start justify-between gap-3 border-b border-warm-200 px-5 py-4">
          <div className="space-y-0.5">
            <h3 className="text-base font-semibold text-ink-950">
              {tString("print.fineTune.title")}
            </h3>
            <p className="text-xs leading-relaxed text-ink-600">
              {tString("print.fineTune.help")}
            </p>
          </div>
          <button
            ref={closeButtonRef}
            type="button"
            aria-label={tString("print.fineTune.close")}
            onClick={onClose}
            className="-me-1 -mt-1 inline-flex h-9 w-9 items-center justify-center rounded-full text-ink-600 transition-colors hover:bg-warm-100 hover:text-ink-900 focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-brand"
          >
            <X size={18} />
          </button>
        </div>
        <div className="min-h-0 flex-1 overflow-y-auto px-5 py-5">
          <div className="space-y-8">
            {sections.map((section) => (
              <section
                key={section.id}
                aria-label={section.label}
                className="space-y-4"
              >
                <h4 className="text-xs font-semibold uppercase tracking-wide text-ink-500">
                  {section.label}
                </h4>
                {section.content}
              </section>
            ))}
          </div>
        </div>
        <div className="border-t border-warm-200 px-5 py-3 text-end">
          <button
            type="button"
            onClick={onClose}
            className="min-h-9 rounded-full bg-ink-950 px-5 text-sm font-medium text-white transition-colors hover:bg-ink-800 focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-brand focus-visible:ring-offset-2"
          >
            {tString("print.fineTune.done")}
          </button>
        </div>
      </div>
    </div>
  );
}
