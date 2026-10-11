"use client";

import React, { useCallback, useEffect, useRef, useState } from "react";
import { ChevronLeft, ChevronRight, X } from "lucide-react";
import { useDialogKeyboard } from "@/components/business/schedule/useDialogKeyboard";
import { Z_MODAL, zStyle } from "./designLayers";

interface GalleryLightboxImage {
  id?: number | string;
  image_url: string;
  caption?: string;
}

interface GalleryLightboxProps {
  images: GalleryLightboxImage[];
  initialIndex: number;
  t: (key: string) => string;
  onClose: () => void;
}

/**
 * Full-screen gallery viewer for the storefront About tab (plan 1.6).
 * Hand-rolled role="dialog" overlay following the repo's established guest
 * pattern (MenuItemDetailDrawer / CommandPalette): a role="presentation"
 * wrapper closes on outside mousedown, the panel stop-propagates, and
 * Escape / Tab-trap / initial-focus / focus-restore come from the shared
 * useDialogKeyboard hook. No new dependencies.
 *
 * Entrance is a scoped CSS keyframe fade; the global
 * `prefers-reduced-motion: reduce` rule in globals.css collapses it to an
 * instant mount for reduced-motion users.
 */
export default function GalleryLightbox({
  images,
  initialIndex,
  t,
  onClose,
}: GalleryLightboxProps) {
  const count = images.length;
  const [index, setIndex] = useState(() =>
    Math.min(Math.max(initialIndex, 0), Math.max(count - 1, 0)),
  );
  const [failed, setFailed] = useState(false);
  const dialogRef = useRef<HTMLDivElement>(null);

  const goPrev = useCallback(() => {
    setIndex((i) => (i - 1 + count) % count);
  }, [count]);
  const goNext = useCallback(() => {
    setIndex((i) => (i + 1) % count);
  }, [count]);

  // Escape closes, Tab is trapped inside the panel, focus moves into the
  // dialog on mount (the close button is first in DOM order) and the previous
  // focus is restored on unmount.
  useDialogKeyboard(dialogRef, onClose);

  // Lock body scroll while the lightbox is open.
  useEffect(() => {
    const previous = document.body.style.overflow;
    document.body.style.overflow = "hidden";
    return () => {
      document.body.style.overflow = previous;
    };
  }, []);

  // Clear a broken-image state when navigating to a different image.
  useEffect(() => {
    setFailed(false);
  }, [index]);

  if (count === 0) return null;
  const current = images[index];
  if (!current) return null;

  const caption = current.caption?.trim() ?? "";
  const alt = caption
    ? (current.caption as string)
    : t("businessPage.galleryImageAlt").replace("{index}", String(index + 1));

  return (
    <div
      role="presentation"
      className="storefront-gallery-lightbox fixed inset-0 flex items-center justify-center p-4"
      style={zStyle(Z_MODAL)}
      onMouseDown={onClose}
    >
      <style>{`
        @keyframes storefront-gallery-lightbox-in {
          from { opacity: 0; }
          to { opacity: 1; }
        }
        .storefront-gallery-lightbox .storefront-gallery-lightbox-fade {
          animation: storefront-gallery-lightbox-in 160ms ease-out;
        }
      `}</style>
      <div
        className="storefront-gallery-lightbox-fade absolute inset-0 bg-ink-950/80 backdrop-blur-sm"
        aria-hidden="true"
      />
      {/* eslint-disable-next-line jsx-a11y/no-noninteractive-element-interactions -- dialog stop-propagation via onMouseDown/onKeyDown prevents backdrop close (CommandPalette pattern) */}
      <div
        ref={dialogRef}
        role="dialog"
        aria-modal="true"
        aria-label={t("businessPage.galleryTitle") || "Gallery"}
        className="storefront-gallery-lightbox-fade relative flex max-w-full flex-col items-center"
        onMouseDown={(e) => e.stopPropagation()}
        onKeyDown={(e) => {
          if (count < 2) return;
          if (e.key === "ArrowLeft") {
            e.preventDefault();
            goPrev();
          } else if (e.key === "ArrowRight") {
            e.preventDefault();
            goNext();
          }
        }}
      >
        <div className="mb-3 flex w-full items-center justify-end gap-3">
          {count > 1 && (
            <span className="mr-auto text-xs font-medium tabular-nums text-white/80">
              {index + 1} / {count}
            </span>
          )}
          <button
            type="button"
            onClick={onClose}
            aria-label={t("businessPage.gallery.closeLightbox") || "Close"}
            className="rounded-full bg-white/10 p-2 text-white transition-colors hover:bg-white/20"
          >
            <X className="h-5 w-5" aria-hidden />
          </button>
        </div>

        <div className="relative flex items-center">
          {count > 1 && (
            <button
              type="button"
              onClick={goPrev}
              aria-label={
                t("businessPage.gallery.previousImage") || "Previous image"
              }
              className="absolute left-2 top-1/2 z-10 -translate-y-1/2 rounded-full bg-ink-950/60 p-2 text-white transition-colors hover:bg-ink-950/80"
            >
              <ChevronLeft className="h-5 w-5 rtl:rotate-180" aria-hidden />
            </button>
          )}
          {failed ? (
            <div className="flex h-[40vh] w-[80vw] max-w-2xl items-center justify-center rounded-lg bg-ink-800 px-6 text-center text-sm text-white/80">
              {alt}
            </div>
          ) : (
            // eslint-disable-next-line @next/next/no-img-element, jsx-a11y/no-noninteractive-element-interactions -- merchant-supplied external URLs; onError is a load failure, not a user interaction
            <img
              src={current.image_url}
              alt={alt}
              className="max-h-[70vh] max-w-[90vw] rounded-lg object-contain md:max-w-[min(80vw,64rem)]"
              onError={() => setFailed(true)}
            />
          )}
          {count > 1 && (
            <button
              type="button"
              onClick={goNext}
              aria-label={t("businessPage.gallery.nextImage") || "Next image"}
              className="absolute right-2 top-1/2 z-10 -translate-y-1/2 rounded-full bg-ink-950/60 p-2 text-white transition-colors hover:bg-ink-950/80"
            >
              <ChevronRight className="h-5 w-5 rtl:rotate-180" aria-hidden />
            </button>
          )}
        </div>

        {caption && (
          <p
            dir="auto"
            className="mt-3 max-w-prose text-center text-sm text-white/90"
          >
            {caption}
          </p>
        )}
      </div>
    </div>
  );
}
