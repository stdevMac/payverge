import { useEffect, useRef, type RefObject } from "react";

/**
 * Minimal dialog keyboard behavior for hand-rolled `role="dialog"` overlays that
 * don't use NextUI's Modal: Escape closes, and Tab is trapped within the
 * container. On mount, focus moves into the dialog (first focusable, or the
 * container itself) so a screen-reader / keyboard user starts inside it.
 *
 * Pure DOM + effects, no React tree assumptions — safe to drop into any overlay.
 *
 * Root C: keydown is on `document` (not the container) so Escape still works
 * when focus has dropped to `<body>`. Focus is restored to the previously
 * focused element on unmount.
 *
 * @param enabled When false, document listeners are not attached (L5-30: while
 *   a nested ConfirmationModal owns focus, do not Tab-trap back into this dialog).
 */
export function useDialogKeyboard(
  containerRef: RefObject<HTMLElement | null>,
  onClose: () => void,
  enabled: boolean = true,
): void {
  // Keep the latest onClose in a ref so the effect can depend only on the
  // container ref identity — callers often pass inline arrows.
  const onCloseRef = useRef(onClose);
  useEffect(() => {
    onCloseRef.current = onClose;
  }, [onClose]);

  // Capture prior focus once per mount; restore only on unmount (not when
  // `enabled` flips false for a nested confirm).
  useEffect(() => {
    const previouslyFocused =
      document.activeElement instanceof HTMLElement
        ? document.activeElement
        : null;
    return () => {
      if (previouslyFocused && document.contains(previouslyFocused)) {
        previouslyFocused.focus();
      }
    };
  }, [containerRef]);

  // Move focus into the dialog once on mount.
  useEffect(() => {
    const container = containerRef.current;
    if (!container) return;

    const focusableSelector = [
      "a[href]",
      "button:not([disabled])",
      "textarea:not([disabled])",
      "input:not([disabled])",
      "select:not([disabled])",
      '[tabindex]:not([tabindex="-1"])',
    ].join(",");

    const focusable = (): HTMLElement[] =>
      Array.from(
        container.querySelectorAll<HTMLElement>(focusableSelector),
      ).filter(
        (el) =>
          el.offsetParent !== null ||
          el === document.activeElement ||
          el.getClientRects().length > 0,
      );

    if (!container.contains(document.activeElement)) {
      const first = focusable()[0];
      if (first) first.focus();
      else {
        container.setAttribute("tabindex", "-1");
        container.focus();
      }
    }
  }, [containerRef]);

  // Document Escape + Tab trap — only while enabled.
  useEffect(() => {
    if (!enabled) return;

    const container = containerRef.current;
    if (!container) return;

    const focusableSelector = [
      "a[href]",
      "button:not([disabled])",
      "textarea:not([disabled])",
      "input:not([disabled])",
      "select:not([disabled])",
      '[tabindex]:not([tabindex="-1"])',
    ].join(",");

    const focusable = (): HTMLElement[] =>
      Array.from(
        container.querySelectorAll<HTMLElement>(focusableSelector),
      ).filter(
        (el) =>
          el.offsetParent !== null ||
          el === document.activeElement ||
          el.getClientRects().length > 0,
      );

    const onKeyDown = (e: KeyboardEvent) => {
      if (e.key === "Escape") {
        // R2-4: a nested layer (portaled NextUI popover, select, date picker)
        // handles Escape on its own element and calls preventDefault, but the
        // event still bubbles up to this document listener. Closing here too
        // would tear down the whole dialog on the keypress that was meant to
        // dismiss just the popover. One Escape, one layer.
        if (e.defaultPrevented) return;
        e.preventDefault();
        e.stopPropagation();
        onCloseRef.current();
        return;
      }
      if (e.key !== "Tab") return;
      const items = focusable();
      if (items.length === 0) {
        e.preventDefault();
        return;
      }
      const first = items[0];
      const last = items[items.length - 1];
      const active = document.activeElement as HTMLElement | null;
      if (e.shiftKey) {
        if (active === first || !container.contains(active)) {
          e.preventDefault();
          last.focus();
        }
      } else if (active === last || !container.contains(active)) {
        e.preventDefault();
        first.focus();
      }
    };

    document.addEventListener("keydown", onKeyDown);
    return () => {
      document.removeEventListener("keydown", onKeyDown);
    };
  }, [containerRef, enabled]);
}
