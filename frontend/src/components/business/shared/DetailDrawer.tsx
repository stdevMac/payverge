"use client";

import React, { useEffect, useRef, useState } from "react";
import {
  Drawer,
  DrawerBody,
  DrawerContent,
  DrawerFooter,
} from "@nextui-org/react";
import { useReducedMotion } from "framer-motion";
import { X } from "lucide-react";
import ConfirmationModal from "@/components/business/modals/ConfirmationModal";

/**
 * Fully-translated copy for the dirty-discard ConfirmationModal.
 *
 * R2-3b: every field is required and the shell ships NO English defaults. The
 * previous `?? <English literal>` fallbacks leaked English into a money-adjacent
 * confirm for every non-en operator, and because the props were optional nothing
 * forced a caller to supply localized copy. The guard test in
 * DetailDrawer.dirty.test.tsx scans this file for those literals, so do not
 * reintroduce them — not even inside a comment.
 */
interface DetailDrawerDiscardConfirm {
  title: string;
  description: string;
  confirmLabel: string;
  cancelLabel: string;
}

interface DetailDrawerBaseProps {
  open: boolean;
  onClose: () => void;
  title: string;
  subtitle?: string;
  /** NextUI drawer size. Defaults to `"md"`. */
  size?: "md" | "lg";
  /** Primary/secondary action slot rendered in a sticky footer. */
  footer?: React.ReactNode;
  children: React.ReactNode;
}

/**
 * `dirty` and `discardConfirm` travel together: a drawer that can block its own
 * close MUST supply the confirm copy, and a read-only drawer must not carry
 * dead copy. The union makes that a compile error instead of an English leak.
 */
export type DetailDrawerProps = DetailDrawerBaseProps &
  (
    | {
        /**
         * L6-18: when true, Esc / backdrop / close button open a discard
         * confirm instead of closing immediately.
         */
        dirty: boolean;
        discardConfirm: DetailDrawerDiscardConfirm;
      }
    | { dirty?: false; discardConfirm?: never }
  );

/**
 * Shared right-side detail drawer for accounting and CRUD flows.
 *
 * Mirrors the InsightsDrawer shell: NextUI Drawer for focus trap / Esc /
 * backdrop, full-height sheet on mobile via size, and `disableAnimation` when
 * the user prefers reduced motion (NextUI's slide ignores prefers-reduced-motion
 * unless we pass that flag).
 *
 * Close routing (#259): NextUI Drawer dismiss (Esc / backdrop) fires
 * `onOpenChange(false)` — not always `onClose`. We gate both through
 * `requestClose` so Escape and the custom X share one dismiss path.
 */
export default function DetailDrawer({
  open,
  onClose,
  title,
  subtitle,
  size = "md",
  footer,
  children,
  dirty = false,
  discardConfirm,
}: DetailDrawerProps) {
  // NextUI's Drawer (ModalContent under the hood) drives its slide via a
  // framer-motion transform, which ignores prefers-reduced-motion — only
  // `disableAnimation` skips it. framer's default MotionConfig reducedMotion is
  // "never", so we read the media query ourselves and pass it through.
  const prefersReducedMotion = !!useReducedMotion();
  const [confirmOpen, setConfirmOpen] = useState(false);

  useEffect(() => {
    if (!open) setConfirmOpen(false);
  }, [open]);

  const requestClose = () => {
    // `discardConfirm` is type-guaranteed present whenever `dirty` is passed;
    // the runtime check keeps a JS-land caller from trapping the operator in a
    // drawer that blocks close but renders no confirm.
    if (dirty && discardConfirm) {
      setConfirmOpen(true);
      return;
    }
    onClose();
  };
  const requestCloseRef = useRef(requestClose);
  requestCloseRef.current = requestClose;

  // issue 259: NextUI Drawer can swallow Escape without calling onOpenChange
  // when hideCloseButton is set. Capture-phase listener is the fallback so
  // Issue invoice (and every other DetailDrawer) still dismisses.
  useEffect(() => {
    if (!open) return;
    const onKeyDown = (event: KeyboardEvent) => {
      if (event.key !== "Escape") return;
      if (confirmOpen) return;
      event.preventDefault();
      event.stopPropagation();
      requestCloseRef.current();
    };
    document.addEventListener("keydown", onKeyDown, true);
    return () => document.removeEventListener("keydown", onKeyDown, true);
  }, [open, confirmOpen]);

  return (
    <>
      <Drawer
        isOpen={open}
        onOpenChange={(nextOpen) => {
          if (!nextOpen) requestClose();
        }}
        onClose={requestClose}
        placement="right"
        size={size}
        isDismissable
        shouldBlockScroll
        disableAnimation={prefersReducedMotion}
        classNames={{
          // Full-screen sheet below `sm`; sized side panel on desktop. The old
          // `sm:max-w-none` had this inverted — it REMOVED the cap at ≥640px, so
          // every drawer became a full-viewport takeover with its footer actions
          // pushed off-screen. `max-w-none` (mobile) + an explicit sm cap per
          // size restores the intended panel.
          base:
            size === "lg"
              ? "bg-warm-50 max-w-none sm:max-w-2xl"
              : "bg-warm-50 max-w-none sm:max-w-xl",
          // Keep NextUI's built-in close out of the hit-test path so it cannot
          // cover / offset our sticky header X (#259).
          closeButton: "hidden pointer-events-none",
        }}
        hideCloseButton
      >
        <DrawerContent
          data-testid="detail-drawer"
          aria-label={title}
          className="border-l border-warm-200 bg-warm-50 shadow-2xl shadow-warm-900/15"
        >
          {() => (
            <>
              <DrawerBody className="px-5 py-6 sm:px-6">
                <div className="sticky top-0 z-20 mb-4 -mx-1 flex items-start justify-between gap-3 bg-warm-50/95 px-1 pb-1 backdrop-blur-sm">
                  <div className="min-w-0">
                    <h2 className="font-title text-heading-md text-ink-900">
                      {title}
                    </h2>
                    {subtitle ? (
                      <p className="mt-1 text-sm text-ink-500">{subtitle}</p>
                    ) : null}
                  </div>
                  <button
                    type="button"
                    onClick={requestClose}
                    aria-label="Close"
                    data-testid="detail-drawer-close"
                    className="relative z-50 flex min-h-11 min-w-11 shrink-0 items-center justify-center rounded-full p-1.5 text-ink-500 transition-colors hover:bg-warm-100 focus:outline-none focus-visible:ring-2 focus-visible:ring-brand"
                  >
                    <X className="h-5 w-5" aria-hidden="true" />
                  </button>
                </div>
                {children}
              </DrawerBody>
              {footer != null ? (
                <DrawerFooter
                  data-testid="detail-drawer-footer"
                  className="justify-end gap-2 border-t border-warm-200/80 bg-warm-50/70 px-5 py-4 sm:px-6"
                >
                  {footer}
                </DrawerFooter>
              ) : null}
            </>
          )}
        </DrawerContent>
      </Drawer>

      {discardConfirm ? (
        <ConfirmationModal
          isOpen={confirmOpen}
          onOpenChange={() => setConfirmOpen(false)}
          title={discardConfirm.title}
          description={discardConfirm.description}
          confirmLabel={discardConfirm.confirmLabel}
          cancelLabel={discardConfirm.cancelLabel}
          isDanger
          onConfirm={() => {
            setConfirmOpen(false);
            onClose();
          }}
        />
      ) : null}
    </>
  );
}
