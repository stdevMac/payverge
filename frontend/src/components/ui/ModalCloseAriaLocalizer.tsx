"use client";

/**
 * F4: NextUI Modal hardcodes aria-label="Close" on the visible X button
 * (`getCloseButtonProps` in @nextui-org/modal) and cloneElement forces that
 * label onto any custom `closeButton` too — there is no prop override.
 *
 * This client observer rewrites English "Close" on dialog close controls to
 * the chrome locale string (e.g. "Cerrar") whenever a modal mounts or
 * re-renders — including after an in-app guest es-AR switch (#26). Dismiss
 * is handled separately via NextUIProvider locale → @react-aria I18nProvider.
 */
import { useEffect } from "react";
import { useChromeLocale } from "@/i18n/useChromeLocale";
import { modalCloseAriaLabel } from "./modalCloseLabel";

const ENGLISH_CLOSE = "Close";

function rewriteCloseLabels(label: string) {
  if (typeof document === "undefined") return;
  // Only rewrite the English hardcode — never thrash already-localized labels.
  const nodes = document.querySelectorAll(
    '[role="dialog"] button[aria-label="Close"], [aria-modal="true"] button[aria-label="Close"]',
  );
  nodes.forEach((el) => {
    if (el.getAttribute("aria-label") === ENGLISH_CLOSE && label !== ENGLISH_CLOSE) {
      el.setAttribute("aria-label", label);
      el.setAttribute("data-payverge-close-localized", "1");
    }
  });
}

export function ModalCloseAriaLocalizer() {
  const locale = useChromeLocale();
  const label = modalCloseAriaLabel(locale);

  useEffect(() => {
    if (label === ENGLISH_CLOSE) {
      // English operator UI — leave NextUI defaults alone.
      return;
    }

    rewriteCloseLabels(label);

    const observer = new MutationObserver(() => {
      rewriteCloseLabels(label);
    });
    observer.observe(document.body, {
      childList: true,
      subtree: true,
      attributes: true,
      attributeFilter: ["aria-label"],
    });
    return () => observer.disconnect();
  }, [label]);

  return null;
}
