import React, { useContext } from "react";
import { GuestTranslationContext } from "@/i18n/GuestTranslationProvider";
import { getMenuLayoutClass, getSectionPadding } from "./designClasses";

const PLACEHOLDER_COUNT = 6;

interface MenuSkeletonProps {
  /** design_settings.menu_layout — the skeleton must occupy the same geometry
   *  as the loaded menu (aspect-[4/3] media, same container layout) or every
   *  menu load is a guaranteed CLS event (storefront plan 1.3). */
  layout?: "grid" | "list";
  /** design_settings.section_density — mirrors the loaded section padding. */
  density?: string;
}

export default function MenuSkeleton({
  layout = "grid",
  density,
}: MenuSkeletonProps) {
  // Skeletons can render outside the provider tree (loading states +
  // unit tests). Read context directly and fall back to English when
  // the provider isn't mounted yet.
  const ctx = useContext(GuestTranslationContext);
  const ariaLabel =
    (ctx?.t("menu.loadingMenuAria") as string | undefined) || "Loading menu";
  const isList = layout === "list";
  return (
    <section
      data-testid="menu-skeleton"
      role="status"
      aria-label={ariaLabel}
      className={getSectionPadding(density)}
    >
      <div className="max-w-6xl mx-auto px-6">
        {/* Header placeholder */}
        <div className="mb-10 flex flex-col items-start gap-3">
          <div className="h-4 w-24 rounded bg-warm-200 animate-pulse" />
          <div className="h-10 w-2/3 max-w-md rounded-lg bg-warm-200 animate-pulse" />
          <div className="h-4 w-1/2 max-w-sm rounded bg-warm-200 animate-pulse" />
        </div>

        {/* Search placeholder */}
        <div className="mb-8 max-w-2xl">
          <div className="h-12 rounded-xl bg-warm-100 animate-pulse" />
        </div>

        {/* Category tab placeholders */}
        <div className="mb-8 flex gap-2 overflow-x-auto pb-2">
          {[1, 2, 3, 4].map((i) => (
            <div key={i} className="h-9 w-24 rounded-lg bg-warm-100 animate-pulse flex-shrink-0" />
          ))}
        </div>

        {/* Item card placeholders — same container + media geometry as the
            loaded grid/list (aspect-[4/3] media; list = side-by-side on md+). */}
        <div className={getMenuLayoutClass(layout)}>
          {Array.from({ length: PLACEHOLDER_COUNT }).map((_, i) => (
            <div
              key={i}
              data-testid="menu-skeleton-card"
              className={`rounded-xl border border-warm-200 bg-white overflow-hidden ${
                isList ? "md:flex md:items-stretch" : ""
              }`}
            >
              <div
                data-testid="menu-skeleton-media"
                className={`aspect-[4/3] bg-warm-100 animate-pulse ${
                  isList ? "md:w-56 md:flex-shrink-0" : ""
                }`}
              />
              <div className="p-6 space-y-3 flex-1">
                <div className="flex justify-between items-start">
                  <div className="h-5 w-3/4 rounded bg-warm-200 animate-pulse" />
                  <div className="h-5 w-12 rounded bg-warm-200 animate-pulse" />
                </div>
                <div className="h-4 w-full rounded bg-warm-100 animate-pulse" />
                <div className="h-4 w-2/3 rounded bg-warm-100 animate-pulse" />
              </div>
            </div>
          ))}
        </div>
      </div>
    </section>
  );
}
