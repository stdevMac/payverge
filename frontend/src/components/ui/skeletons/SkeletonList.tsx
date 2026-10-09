import React from "react";
import { SkeletonLine } from "./SkeletonLine";

interface SkeletonListProps {
  /** Number of placeholder rows to render. */
  rows?: number;
  /** Bordered card rows (default) or borderless compact rows for dropdowns. */
  bordered?: boolean;
  /** Accessible name for the live region (e.g. the localized "Loading…" string). */
  ariaLabel?: string;
  className?: string;
}

/**
 * SkeletonList is the shared loading placeholder for the staff dashboard's
 * card-list surfaces (announcements, polls, documents, checklists, logbook,
 * chat, hours). It mirrors the loaded shape — a vertical stack of rows, each a
 * title line over a shorter meta line — so the hand-off into real content
 * doesn't shift layout. Wrapped in a polite role="status" live region so it
 * reads as "loading" to assistive tech instead of an empty container.
 */
export function SkeletonList({
  rows = 3,
  bordered = true,
  ariaLabel,
  className = "",
}: SkeletonListProps) {
  return (
    <div
      role="status"
      aria-live="polite"
      aria-label={ariaLabel}
      className={`space-y-3 ${className}`}
    >
      {Array.from({ length: rows }, (_, i) => (
        <div
          key={i}
          className={`motion-safe:animate-pulse ${
            bordered ? "rounded-xl border border-warm-200 p-4" : "px-1 py-2"
          }`}
        >
          <SkeletonLine width="55%" height="0.9375rem" />
          <SkeletonLine width="32%" height="0.75rem" className="mt-2" />
        </div>
      ))}
    </div>
  );
}
