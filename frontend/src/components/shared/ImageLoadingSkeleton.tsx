"use client";

import React from "react";

/**
 * True when an <img> element already has its bitmap available (browser
 * cache / synchronous decode). Used from ref callbacks so cached images are
 * marked loaded on mount and never flash a skeleton — `onLoad` may have
 * already fired before React attached its listener.
 */
export function isImageAlreadyLoaded(
  node: HTMLImageElement | null,
): node is HTMLImageElement {
  return !!node && node.complete && node.naturalWidth > 0;
}

/**
 * Pulse overlay shown inside a `relative` media frame while its image is in
 * flight. Callers pass the surface tone (e.g. `bg-warm-100`) and any stacking
 * class needed to sit above the image but below controls. A CSS pulse (not a
 * spinner) keeps a grid of many loading cards calm, and `motion-reduce`
 * freezes it to a static fill for reduced-motion users.
 */
export default function ImageLoadingSkeleton({
  className = "",
}: {
  className?: string;
}) {
  return (
    <div
      aria-hidden="true"
      data-testid="image-loading-skeleton"
      className={`pointer-events-none absolute inset-0 animate-pulse motion-reduce:animate-none ${className}`}
    />
  );
}
