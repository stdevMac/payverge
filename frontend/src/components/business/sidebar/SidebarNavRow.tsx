"use client";

import React from "react";
import { motion } from "framer-motion";
import type { LucideIcon } from "lucide-react";
import { dashboardSpring } from "../premium/motion";
import { useReducedDashboardMotion } from "../premium/useReducedDashboardMotion";
import { AnimatedNavIcon } from "../premium/AnimatedNavIcon";
import { SidebarTooltip } from "./SidebarTooltip";

interface SidebarNavRowProps {
  label: string;
  /**
   * #726: no longer rendered. The expanded row already prints its label, and
   * the secondary "— description" line was the bubble copy operators asked us
   * to drop. Kept optional so tab config can keep passing it.
   */
  description?: string;
  icon: LucideIcon;
  active: boolean;
  collapsed: boolean;
  disabled?: boolean;
  title: string;
  children?: React.ReactNode;
  leadingBadge?: React.ReactNode;
  tutorialClasses?: string;
  onClick: () => void;
  /** M1: warm this row's tab chunk on hover/focus so the click feels instant. */
  onPrefetch?: () => void;
  buttonRef?: React.Ref<HTMLButtonElement>;
}

/**
 * One calm nav row: bare icon + label, dim when inactive, and a single
 * active affordance (the shared sliding highlight). The earlier version
 * boxed every icon in its own bordered tile with sheen/rotation micro-
 * animations and stacked three active decorations — that visual noise is
 * exactly what the sidebar redesign removes.
 */
export function SidebarNavRow({
  label,
  icon: Icon,
  active,
  collapsed,
  disabled = false,
  title,
  children,
  leadingBadge,
  tutorialClasses = "",
  onClick,
  onPrefetch,
  buttonRef,
}: SidebarNavRowProps) {
  const reduceMotion = useReducedDashboardMotion();
  const transition = reduceMotion ? { duration: 0 } : dashboardSpring;

  const iconNode = leadingBadge ?? (
    <AnimatedNavIcon icon={Icon} active={active} label={label} />
  );

  const row = (
    <button
      ref={buttonRef}
      type="button"
      onClick={onClick}
      onMouseEnter={onPrefetch}
      onFocus={onPrefetch}
      disabled={disabled}
      aria-current={active ? "page" : undefined}
      className={[
        "group relative isolate flex w-full items-center justify-between gap-3 rounded-xl px-3 py-2 text-sm outline-none transition-colors focus-visible:ring-2 focus-visible:ring-brand",
        collapsed ? "lg:justify-center lg:gap-0 lg:px-0" : "",
        active
          ? "font-medium text-brand-dark"
          : "text-ink-500 hover:bg-warm-100/70 hover:text-ink-900",
        tutorialClasses,
      ]
        .filter(Boolean)
        .join(" ")}
    >
      {active ? (
        <motion.span
          layoutId="dashboard-sidebar-active-row"
          className="absolute inset-0 -z-10 rounded-xl bg-brand/10"
          transition={transition}
        />
      ) : null}
      <span
        className={`flex min-w-0 items-center gap-3 ${
          collapsed ? "lg:gap-0" : ""
        }`}
      >
        {iconNode}
        <span
          className={`truncate transition-all duration-200 ${
            collapsed ? "lg:ml-0 lg:w-0 lg:overflow-hidden lg:opacity-0" : ""
          }`}
        >
          {label}
        </span>
      </span>
      <span className={`contents ${collapsed ? "lg:hidden" : ""}`}>
        {children}
      </span>
    </button>
  );

  // #726: an expanded row already prints its label, so a hover bubble repeating
  // it only covers the neighbouring rows. The bubble survives in exactly one
  // state — the collapsed desktop rail, where the label is width/opacity-zeroed.
  // Collapse is `lg:`-only, so the mobile drawer (labels intact) opts out too.
  // #744: SidebarTooltip portals the right-side bubble so the overflow-hidden
  // nav cannot clip it to a dark sliver.
  if (!collapsed) return row;

  return (
    <SidebarTooltip
      label={title}
      side="right"
      className="block w-full"
      bubbleClassName="max-lg:hidden"
    >
      {row}
    </SidebarTooltip>
  );
}
