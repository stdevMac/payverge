"use client";

import React, { useContext, useEffect, useRef } from "react";
import { GuestTranslationContext } from "@/i18n/GuestTranslationProvider";
import { preferredScrollBehavior } from "@/lib/scrollBehavior";

export interface CategoryTabItem {
  slug: string;
  name: string;
}

export interface CategoryTabsProps {
  categories: CategoryTabItem[];
  activeIndex?: number;
  /**
   * Override for the anchor element id. Defaults to `cat-${slug}`.
   * Allows callers whose section ids follow a different convention
   * (e.g. existing `category-${index}` markers) to reuse this component
   * without having to rewrite section anchors.
   */
  getAnchorId?: (category: CategoryTabItem, index: number) => string;
  /**
   * Called after the scroll + hash update so parents can sync any
   * controlled active-category state.
   */
  onSelect?: (index: number, category: CategoryTabItem) => void;
  className?: string;
  buttonClassName?: (active: boolean) => string;
}

const defaultButtonClassName = (active: boolean) =>
  `inline-flex min-h-[44px] flex-shrink-0 items-center justify-center whitespace-nowrap rounded-full px-4 py-2.5 text-sm font-semibold tracking-wide transition-colors ${
    active
      ? "bg-ink-900 text-white shadow-sm"
      : "border border-warm-200 bg-white text-ink-600 hover:border-ink-300 hover:text-ink-900"
  }`;

const defaultGetAnchorId = (category: CategoryTabItem) => `cat-${category.slug}`;

function scrollToCategoryAnchor(anchorId: string): boolean {
  if (typeof document === "undefined") return false;
  const el = document.getElementById(anchorId);
  if (!el) return false;
  el.scrollIntoView({ behavior: preferredScrollBehavior(), block: "start" });
  if (typeof history !== "undefined" && typeof history.replaceState === "function") {
    history.replaceState(null, "", `#${anchorId}`);
  }
  return true;
}

export default function CategoryTabs({
  categories,
  activeIndex,
  getAnchorId = defaultGetAnchorId,
  onSelect,
  className,
  buttonClassName = defaultButtonClassName,
}: CategoryTabsProps) {
  const ctx = useContext(GuestTranslationContext);
  const t = ctx?.t ?? ((_k: string) => "");
  const pillRefs = useRef<Array<HTMLButtonElement | null>>([]);
  // Index of the pill we last scrolled the rail to. Prevents re-issuing
  // scrollIntoView (and any resulting scroll loop) when the active index
  // hasn't actually moved.
  const lastScrolledIndex = useRef<number | undefined>(undefined);

  // Keep the active pill visible in the horizontal rail whenever activeIndex
  // changes — including scroll-driven changes from the parent page's scroll
  // spy, not just clicks. Only scroll the rail when the pill isn't already
  // fully in view, and only when the target index actually changed.
  useEffect(() => {
    if (activeIndex === undefined) return;
    if (lastScrolledIndex.current === activeIndex) return;
    const pill = pillRefs.current[activeIndex];
    const rail = pill?.parentElement;
    if (!pill || !rail) return;

    const pillLeft = pill.offsetLeft;
    const pillRight = pillLeft + pill.offsetWidth;
    const viewLeft = rail.scrollLeft;
    const viewRight = viewLeft + rail.clientWidth;
    const fullyVisible = pillLeft >= viewLeft && pillRight <= viewRight;

    lastScrolledIndex.current = activeIndex;
    if (fullyVisible) return;
    pill.scrollIntoView({ inline: "nearest", block: "nearest" });
  }, [activeIndex]);

  const handleClick = (category: CategoryTabItem, index: number) => {
    const anchorId = getAnchorId(category, index);
    scrollToCategoryAnchor(anchorId);
    onSelect?.(index, category);
  };

  return (
    <div
      className={
        className ??
        "-mx-1 mt-3 flex gap-2 overflow-x-auto px-1 pb-1 scrollbar-thin scrollbar-thumb-warm-200 scrollbar-track-transparent"
      }
      aria-label={t("menu.categories") || "Menu categories"}
    >
      {categories.map((category, index) => {
        const active = activeIndex === index;
        return (
          <button
            key={`${category.slug}-${index}`}
            ref={(node) => {
              pillRefs.current[index] = node;
            }}
            type="button"
            aria-current={active ? "true" : undefined}
            onClick={() => handleClick(category, index)}
            className={buttonClassName(active)}
          >
            {category.name}
          </button>
        );
      })}
    </div>
  );
}
