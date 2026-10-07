"use client";

/**
 * F4: drop-in NextUI Modal for call sites that want an explicit localized
 * close control. The global {@link ModalCloseAriaLocalizer} already rewrites
 * NextUI's hardcoded "Close" under Spanish; this wrapper also injects a
 * custom `closeButton` so the first paint can carry the right label without
 * waiting for the observer (cloneElement still spreads "Close" on top of
 * children, so we re-apply via ref after mount).
 */
import React, { useEffect, useRef } from "react";
import { Modal, type ModalProps } from "@nextui-org/react";
import { useSimpleLocale } from "@/i18n/SimpleTranslationProvider";
import { modalCloseAriaLabel } from "./modalCloseLabel";

export function LocalizedModal({
  closeButton,
  isOpen,
  ...props
}: ModalProps) {
  const { locale } = useSimpleLocale();
  const label = modalCloseAriaLabel(locale);
  const wrapRef = useRef<HTMLDivElement | null>(null);

  useEffect(() => {
    if (!isOpen || label === "Close") return;
    // NextUI portals the dialog to document.body; rewrite any open dialog's Close.
    const apply = () => {
      document
        .querySelectorAll(
          '[role="dialog"] button[aria-label="Close"], [aria-modal="true"] button[aria-label="Close"]',
        )
        .forEach((el) => {
          el.setAttribute("aria-label", label);
        });
    };
    apply();
    // NextUI may set aria-label after first paint.
    const id = window.requestAnimationFrame(apply);
    return () => window.cancelAnimationFrame(id);
  }, [isOpen, label]);

  return (
    <div ref={wrapRef} className="contents">
      <Modal isOpen={isOpen} closeButton={closeButton} {...props} />
    </div>
  );
}
