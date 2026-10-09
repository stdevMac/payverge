"use client";

import React from "react";
import type { FormatId } from "./formats/formats";
import type { NormalizedRect } from "./templates/types";

export function platformLabelKey(aspect: FormatId): string {
  switch (aspect) {
    case "9:16":
      return "preview.platform.story";
    case "1:1":
      return "preview.platform.square";
    case "wide":
      return "preview.platform.wide";
    case "strip":
      return "preview.platform.strip";
    case "5:7":
      return "preview.platform.tent";
    default:
      return "preview.platform.feed";
  }
}

interface Props {
  aspect: FormatId;
  contentBounds: NormalizedRect;
  platformLabel: string;
  showSafeZones?: boolean;
  children: React.ReactNode;
  className?: string;
}

/** Frames the canvas preview with platform context and optional Story safe zones. */
export function PlatformPreviewFrame({
  aspect,
  contentBounds,
  platformLabel,
  showSafeZones = true,
  children,
  className = "",
}: Props) {
  const isStory = aspect === "9:16";
  const topCovered = contentBounds.y;
  const bottomCovered = 1 - contentBounds.y - contentBounds.h;

  return (
    <div className={`mx-auto w-full ${className || "max-w-[420px]"}`}>
      <div className="mb-2 flex items-center justify-between gap-2 px-0.5">
        <span className="text-[11px] font-semibold uppercase tracking-[0.12em] text-ink-500">
          {platformLabel}
        </span>
        <span className="rounded-full border border-warm-200 bg-warm-50 px-2 py-0.5 text-[10px] font-semibold tabular-nums text-ink-600">
          {aspect}
        </span>
      </div>
      <div
        className={
          isStory
            ? "rounded-[1.75rem] border-[3px] border-ink-900/90 bg-ink-900 p-1.5 shadow-[0_16px_40px_rgba(28,25,23,0.16)]"
            : "overflow-hidden rounded-2xl border border-warm-200/90 shadow-[0_10px_30px_rgba(46,42,37,0.08)]"
        }
      >
        <div className="relative overflow-hidden rounded-[1.25rem] bg-warm-50">
          {children}
          {showSafeZones && isStory && (
            <>
              <div
                className="pointer-events-none absolute inset-x-0 top-0 border-b border-dashed border-white/35 bg-ink-900/10"
                style={{ height: `${topCovered * 100}%` }}
                data-safe-zone="top"
                aria-hidden="true"
              />
              <div
                className="pointer-events-none absolute inset-x-0 bottom-0 border-t border-dashed border-white/35 bg-ink-900/10"
                style={{ height: `${bottomCovered * 100}%` }}
                data-safe-zone="bottom"
                aria-hidden="true"
              />
            </>
          )}
        </div>
      </div>
    </div>
  );
}
