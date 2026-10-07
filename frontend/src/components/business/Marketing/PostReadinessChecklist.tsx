"use client";

import React from "react";
import { Check, Circle, AlertCircle } from "lucide-react";

export interface PostReadinessFlags {
  photoReady: boolean;
  captionReady: boolean;
  /**
   * True only when the live preview rendered successfully and generation is
   * not mid-flight. Must stay false for "rendering" and "failed" — export
   * never greens on a broken preview.
   */
  previewReady: boolean;
  /** Optional: preview failed (distinct from still rendering). */
  previewBroken?: boolean;
}

interface Props extends PostReadinessFlags {
  t: (key: string) => string;
  /** Sticky surface class for the editor preview column. */
  sticky?: boolean;
}

/**
 * Honest export readiness. The final "ready" chip is green only when every
 * gate is true — never when preview is broken or still rendering.
 */
export function computeExportReady(flags: PostReadinessFlags): boolean {
  return (
    flags.photoReady &&
    flags.captionReady &&
    flags.previewReady &&
    !flags.previewBroken
  );
}

export function PostReadinessChecklist({
  photoReady,
  captionReady,
  previewReady,
  previewBroken = false,
  t,
  sticky = false,
}: Props) {
  // Preview step is never "done" while broken, even if a stale ready flag
  // slipped through (callers should already set previewReady false).
  const previewDone = previewReady && !previewBroken;
  const allReady = computeExportReady({
    photoReady,
    captionReady,
    previewReady: previewDone,
    previewBroken,
  });

  const steps = [
    { key: "photo", done: photoReady, broken: false },
    { key: "caption", done: captionReady, broken: false },
    {
      key: "preview",
      done: previewDone,
      broken: previewBroken,
    },
    { key: "ready", done: allReady, broken: false },
  ] as const;

  return (
    <ul
      data-testid="post-readiness-checklist"
      data-export-ready={String(allReady)}
      data-preview-broken={String(previewBroken)}
      className={`flex flex-wrap gap-2 rounded-xl border border-warm-200 bg-warm-50/90 p-2.5 backdrop-blur-sm ${
        sticky
          ? "sticky top-0 z-10 shadow-sm shadow-warm-900/5"
          : ""
      }`}
    >
      {steps.map((step) => {
        const done = step.done;
        const broken = step.broken;
        return (
          <li
            key={step.key}
            data-step={step.key}
            data-ready={String(done)}
            data-broken={String(broken)}
            className={`inline-flex items-center gap-1.5 rounded-full px-2.5 py-1 text-[11px] font-medium ${
              broken
                ? "bg-rose-50 text-rose-800"
                : done
                  ? "bg-brand-50 text-brand-800"
                  : "bg-white text-ink-500"
            }`}
          >
            {broken ? (
              <AlertCircle className="h-3 w-3" aria-hidden="true" />
            ) : done ? (
              <Check className="h-3 w-3" aria-hidden="true" />
            ) : (
              <Circle className="h-3 w-3 opacity-40" aria-hidden="true" />
            )}
            {broken && step.key === "preview"
              ? t("editor.checklist.previewBroken")
              : t(`editor.checklist.${step.key}`)}
          </li>
        );
      })}
    </ul>
  );
}
