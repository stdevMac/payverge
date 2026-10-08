"use client";

import React, { useEffect, useRef } from "react";
import {
  useSimpleLocale,
  getTranslation,
} from "@/i18n/SimpleTranslationProvider";
import { SkeletonLine } from "@/components/ui/skeletons";
import DashboardTabLoadingSkeleton from "./DashboardTabLoadingSkeleton";

/**
 * Route-level dashboard chrome skeleton. Mirrors the warm canvas, sidebar rail,
 * and content column so the first paint matches the loaded dashboard shell.
 */
export default function DashboardShellSkeleton() {
  const { locale } = useSimpleLocale();
  const label = getTranslation("common.loadingDashboard", locale) as string;
  // L1-20: React 18 does not type/forward `inert` reliably — set on the DOM node.
  const rootRef = useRef<HTMLDivElement | null>(null);
  useEffect(() => {
    const el = rootRef.current;
    if (!el) return;
    el.setAttribute("inert", "");
  }, []);

  return (
    <>
      {/*
       * R2-7: the announcement lives OUTSIDE the inert node, as a sibling.
       * `inert` removes its whole subtree from the accessibility tree, so a
       * role="status" / aria-live region nested inside the skeleton chrome was
       * never exposed and the "loading dashboard" announcement never fired.
       * Keep this span out of the inert subtree.
       */}
      <span
        role="status"
        aria-live="polite"
        aria-busy="true"
        className="sr-only"
      >
        {label}
      </span>
      <div
        ref={rootRef}
        className="pointer-events-none relative h-[100dvh] overflow-hidden bg-warm-50 text-ink-950"
        aria-hidden="true"
        data-skeleton-inert="true"
      >
        <div className="pointer-events-none absolute inset-0 bg-[radial-gradient(circle_at_12%_8%,rgba(26,107,106,0.10),transparent_28%),radial-gradient(circle_at_90%_12%,rgba(255,255,255,0.78),transparent_24%),linear-gradient(180deg,#faf9f6_0%,#f3f1ec_100%)]" />
        <div className="pointer-events-none absolute inset-0 opacity-[0.32] [background-image:linear-gradient(rgba(255,255,255,0.32)_1px,transparent_1px),linear-gradient(90deg,rgba(255,255,255,0.32)_1px,transparent_1px)] [background-size:28px_28px]" />

        <div className="relative z-[1] flex h-full flex-col">
          <div className="h-14 flex-shrink-0 md:h-16" aria-hidden="true" />

          <div className="flex flex-1 overflow-hidden">
            <aside
              className="hidden w-[17.5rem] flex-shrink-0 border-r border-warm-200/80 bg-white/70 px-4 py-5 lg:flex lg:flex-col"
              aria-hidden="true"
            >
              <SkeletonLine
                width="8rem"
                height="1.25rem"
                className="rounded-lg"
              />
              <div className="mt-8 space-y-2">
                {Array.from({ length: 8 }).map((_, index) => (
                  <SkeletonLine
                    key={index}
                    width="100%"
                    height="2.25rem"
                    className="rounded-xl"
                  />
                ))}
              </div>
            </aside>

            <div className="relative flex-1 overflow-hidden">
              <div
                className="border-b border-warm-200/80 bg-white/90 px-4 py-3 lg:hidden"
                aria-hidden="true"
              >
                <div className="flex items-center justify-between gap-3">
                  <SkeletonLine
                    width="2rem"
                    height="2rem"
                    className="rounded-lg"
                  />
                  <SkeletonLine
                    width="8rem"
                    height="1rem"
                    className="rounded-lg"
                  />
                  <SkeletonLine
                    width="2rem"
                    height="2rem"
                    className="rounded-lg"
                  />
                </div>
              </div>

              <div className="h-full overflow-y-auto">
                <DashboardTabLoadingSkeleton variant="overview" />
              </div>
            </div>
          </div>
        </div>
      </div>
    </>
  );
}
