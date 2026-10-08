"use client";

import React from "react";
import { motion } from "framer-motion";
import type { LucideIcon } from "lucide-react";
import { AnimatedBadge } from "./AnimatedBadge";
import { dashboardSpring } from "./motion";

interface PremiumSegmentedTab {
  key: string;
  label: string;
  icon?: LucideIcon;
  badge?: number;
  /**
   * Optional badge cap override. Pass `null` for no cap so result-total
   * badges (e.g. BillManager history) match the header exact count.
   * Omit to keep AnimatedBadge's default (9 → "9+").
   */
  badgeCap?: number | null;
}

interface PremiumSegmentedTabsProps {
  tabs: PremiumSegmentedTab[];
  activeKey: string;
  onChange: (key: string) => void;
  size?: "sm" | "md";
  className?: string;
  ariaLabel?: string;
  /**
   * When set, each tab gets `id={`${idPrefix}-tab-${key}`}` and
   * `aria-controls={`${idPrefix}-panel-${key}`}` so it associates with its own
   * single-live tabpanel. Omit for the legacy (unwired) rendering.
   */
  idPrefix?: string;
}

export function PremiumSegmentedTabs({
  tabs,
  activeKey,
  onChange,
  size = "md",
  className = "",
  ariaLabel,
  idPrefix,
}: PremiumSegmentedTabsProps) {
  const localLayoutId = React.useId();
  const activeLayoutId = `premium-segmented-tabs-active-${localLayoutId}`;
  const pad =
    size === "sm"
      ? "px-2.5 py-1.5 text-xs sm:px-3"
      : "px-2.5 py-2 text-sm sm:px-3.5";
  const iconSize = size === "sm" ? "h-3.5 w-3.5" : "h-4 w-4";

  // Roving tabindex: only the active tab is tabbable, and Arrow/Home/End move
  // selection AND focus so keyboard users land on the newly-selected tab
  // (WAI-ARIA tabs pattern). Focus follows selection to match the manual
  // tablists this strip replaces.
  const tabRefs = React.useRef<Record<string, HTMLButtonElement | null>>({});
  const handleKeyDown = (e: React.KeyboardEvent<HTMLButtonElement>) => {
    const currentIdx = tabs.findIndex((t) => t.key === activeKey);
    if (currentIdx === -1) return;
    let nextIdx: number | null = null;
    if (e.key === "ArrowRight" || e.key === "ArrowDown") {
      nextIdx = (currentIdx + 1) % tabs.length;
    } else if (e.key === "ArrowLeft" || e.key === "ArrowUp") {
      nextIdx = (currentIdx - 1 + tabs.length) % tabs.length;
    } else if (e.key === "Home") {
      nextIdx = 0;
    } else if (e.key === "End") {
      nextIdx = tabs.length - 1;
    }
    if (nextIdx === null) return;
    e.preventDefault();
    const nextKey = tabs[nextIdx].key;
    onChange(nextKey);
    tabRefs.current[nextKey]?.focus();
  };

  // Scroll affordance: on narrow viewports the strip overflows horizontally
  // (overflow-x-auto) with no built-in hint that more tabs exist off-screen.
  // Track the real scroll state and show an edge fade only on the side(s)
  // that can still scroll.
  const scrollRef = React.useRef<HTMLDivElement | null>(null);
  const [canScrollLeft, setCanScrollLeft] = React.useState(false);
  const [canScrollRight, setCanScrollRight] = React.useState(false);
  const updateScrollState = React.useCallback(() => {
    const el = scrollRef.current;
    if (!el) return;
    setCanScrollLeft(el.scrollLeft > 0);
    setCanScrollRight(el.scrollLeft + el.clientWidth < el.scrollWidth - 1);
  }, []);
  React.useEffect(() => {
    updateScrollState();
    const el = scrollRef.current;
    // jsdom has no ResizeObserver; the scroll-event path still works there.
    if (!el || typeof ResizeObserver === "undefined") return;
    const observer = new ResizeObserver(updateScrollState);
    observer.observe(el);
    return () => observer.disconnect();
    // Re-measure when the tab set changes (scrollWidth is content-driven).
  }, [updateScrollState, tabs.length]);

  return (
    <div
      className={["relative w-full max-w-full sm:inline-block sm:w-auto", className]
        .filter(Boolean)
        .join(" ")}
    >
    <div
      ref={scrollRef}
      onScroll={updateScrollState}
      role="tablist"
      aria-label={ariaLabel}
      className="flex w-full max-w-full items-center gap-0.5 overflow-x-auto overflow-y-hidden rounded-2xl border border-warm-200/90 bg-warm-100/70 p-1 shadow-inner shadow-white/60 sm:inline-flex sm:w-auto sm:gap-1"
    >
      {tabs.map(({ key, label, icon: Icon, badge, badgeCap }) => {
        const active = key === activeKey;
        return (
          <button
            key={key}
            type="button"
            role="tab"
            id={idPrefix ? `${idPrefix}-tab-${key}` : undefined}
            // Only the active tab controls a live panel — the shell mounts a
            // single tabpanel, so inactive tabs must not reference absent ids.
            aria-controls={
              idPrefix && active ? `${idPrefix}-panel-${key}` : undefined
            }
            aria-selected={active}
            tabIndex={active ? 0 : -1}
            ref={(el) => {
              tabRefs.current[key] = el;
            }}
            onKeyDown={handleKeyDown}
            onClick={() => onChange(key)}
            className={[
              // relative (not isolate): isolate + negative z-index parked the
              // active pill behind the track, and Framer's layoutId clone then
              // painted an opaque white slab over neighboring labels while also
              // swallowing the first click during the spring.
              "relative inline-flex min-w-max flex-none items-center justify-center gap-1.5 rounded-xl font-medium transition-colors focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-brand",
              pad,
              active ? "text-brand-dark" : "text-ink-500 hover:text-ink-900",
            ].join(" ")}
          >
            {active ? (
              <motion.span
                layoutId={activeLayoutId}
                aria-hidden="true"
                className="pointer-events-none absolute inset-0 z-0 rounded-xl bg-white shadow-sm ring-1 ring-warm-200/80"
                transition={dashboardSpring}
              />
            ) : null}
            {Icon ? (
              <Icon className={`relative z-10 ${iconSize}`} aria-hidden="true" />
            ) : null}
            <span className="relative z-10 whitespace-nowrap">{label}</span>
            <AnimatedBadge
              count={badge}
              label={label}
              cap={badgeCap}
              className="relative z-10 ml-0.5"
              ariaHidden
            />
          </button>
        );
      })}
    </div>
    {/* Edge fades: soft gradients matching the track bg, shown only on the
        side(s) with off-screen tabs. pointer-events-none + narrow width keep
        edge tabs clickable and their focus ring visible; inset-y-px/edge-px
        keeps the fade inside the rounded 1px border. */}
    {canScrollLeft ? (
      <span
        aria-hidden="true"
        data-testid="segmented-tabs-fade-left"
        className="pointer-events-none absolute inset-y-px left-px w-8 rounded-l-2xl bg-gradient-to-r from-warm-100 to-transparent"
      />
    ) : null}
    {canScrollRight ? (
      <span
        aria-hidden="true"
        data-testid="segmented-tabs-fade-right"
        className="pointer-events-none absolute inset-y-px right-px w-8 rounded-r-2xl bg-gradient-to-l from-warm-100 to-transparent"
      />
    ) : null}
    </div>
  );
}
