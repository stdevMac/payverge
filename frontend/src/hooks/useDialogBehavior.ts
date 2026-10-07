"use client";

import { useEffect, useRef, type RefObject } from "react";

/**
 * Selector matching the elements considered focusable for the purpose of a Tab
 * trap. Mirrors the WAI-ARIA authoring-practices notion of "tabbable" content:
 * links/buttons/inputs/selects/textareas that are not disabled, plus anything
 * with an explicit non-negative tabindex. Elements with `tabindex="-1"` are
 * intentionally excluded — they can receive programmatic focus (e.g. the drawer
 * container itself) but must not be reachable via Tab.
 */
const FOCUSABLE_SELECTOR = [
  "a[href]",
  "area[href]",
  "input:not([disabled])",
  "select:not([disabled])",
  "textarea:not([disabled])",
  "button:not([disabled])",
  "iframe",
  "object",
  "embed",
  "[contenteditable]",
  '[tabindex]:not([tabindex="-1"])',
].join(",");

function getFocusableElements(container: HTMLElement): HTMLElement[] {
  return Array.from(
    container.querySelectorAll<HTMLElement>(FOCUSABLE_SELECTOR),
  ).filter((el) => {
    // Skip elements that are not actually rendered/interactive: display:none,
    // visibility:hidden, or otherwise collapsed produce a null offsetParent
    // (except for position:fixed, so also allow non-zero client rects).
    if (el.hasAttribute("disabled")) return false;
    if (el.getAttribute("aria-hidden") === "true") return false;
    return el.offsetParent !== null || el.getClientRects().length > 0;
  });
}

export interface UseDialogBehaviorOptions {
  /** Whether the dialog/drawer is currently open. */
  isOpen: boolean;
  /** Called when the user requests dismissal (Escape). */
  onClose: () => void;
  /** Ref to the dialog container element that owns the focus trap. */
  containerRef: RefObject<HTMLElement | null>;
  /**
   * Optional ref to the element that should receive focus on open. Falls back
   * to the first focusable child, then the container itself.
   */
  initialFocusRef?: RefObject<HTMLElement | null>;
  /**
   * Optional ref captured by the caller when opening begins. Use this when an
   * async open flow can move focus before `isOpen` becomes true.
   */
  restoreFocusRef?: RefObject<HTMLElement | null>;
}

/**
 * useDialogBehavior — shared keyboard/focus affordances for hand-rolled
 * dialogs and slide-over drawers that are NOT rendered through NextUI's Modal
 * (which brings its own focus management). Wire the drawer's container ref in
 * and this hook will, while `isOpen`:
 *
 *  1. Move focus into the dialog on open — to `initialFocusRef` if provided,
 *     otherwise the first focusable descendant, otherwise the container itself
 *     (which must carry `tabIndex={-1}` to be focusable).
 *  2. Trap Tab / Shift+Tab within the container so keyboard focus can't escape
 *     to the (inert) page behind the modal.
 *  3. Close on Escape via `onClose`.
 *  4. Restore focus to the element that was focused before the dialog opened
 *     when it closes or unmounts, so keyboard users land back where they were.
 *
 * The container should also declare `role="dialog"` and `aria-modal="true"` in
 * markup; this hook only owns behavior, not ARIA attributes.
 */
export function useDialogBehavior({
  isOpen,
  onClose,
  containerRef,
  initialFocusRef,
  restoreFocusRef,
}: UseDialogBehaviorOptions): void {
  // Keep the latest onClose in a ref so the effect can depend on `isOpen` alone.
  // Callers routinely pass an inline arrow for onClose (a fresh identity every
  // render); without this the effect would tear down and re-run on every parent
  // render while open, thrashing focus via the restore-on-cleanup path below.
  const onCloseRef = useRef(onClose);
  useEffect(() => {
    onCloseRef.current = onClose;
  }, [onClose]);

  useEffect(() => {
    if (!isOpen) return;

    const container = containerRef.current;
    if (!container) return;

    // Remember what had focus so we can restore it on close/unmount. Captured
    // synchronously before we move focus into the dialog.
    const previouslyFocused =
      restoreFocusRef?.current ??
      (document.activeElement instanceof HTMLElement
        ? document.activeElement
        : null);

    // Move focus into the dialog, but only if focus isn't already inside it.
    // Prefer the caller-specified initial target, then the first focusable
    // child, then the container itself.
    if (!container.contains(document.activeElement)) {
      const target =
        initialFocusRef?.current ??
        getFocusableElements(container)[0] ??
        container;
      target.focus();
    }

    const onKeyDown = (event: KeyboardEvent) => {
      if (event.key === "Escape") {
        event.stopPropagation();
        onCloseRef.current();
        return;
      }

      if (event.key !== "Tab") return;

      const focusable = getFocusableElements(container);
      if (focusable.length === 0) {
        // Nothing tabbable inside — keep focus pinned on the container so Tab
        // can't move focus out to the inert page behind the dialog.
        event.preventDefault();
        container.focus();
        return;
      }

      const first = focusable[0];
      const last = focusable[focusable.length - 1];
      const active = document.activeElement;

      // If focus somehow sits outside the container (e.g. it started on the
      // container itself), pull it back to an edge before wrapping.
      if (!(active instanceof HTMLElement) || !container.contains(active)) {
        event.preventDefault();
        (event.shiftKey ? last : first).focus();
        return;
      }

      if (event.shiftKey && active === first) {
        event.preventDefault();
        last.focus();
      } else if (!event.shiftKey && active === last) {
        event.preventDefault();
        first.focus();
      }
    };

    document.addEventListener("keydown", onKeyDown);

    return () => {
      document.removeEventListener("keydown", onKeyDown);
      // Restore focus to where the user was before opening, if that element is
      // still in the document and focusable.
      if (previouslyFocused && document.contains(previouslyFocused)) {
        previouslyFocused.focus();
      }
    };
  }, [containerRef, initialFocusRef, isOpen, restoreFocusRef]);
}
